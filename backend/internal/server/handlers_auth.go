package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/waifu-im/waifu-api/backend/internal/auth"
)

func (s *Server) RegisterAuthRoutes() {
	s.Handle("POST /auth/discord", http.HandlerFunc(s.handleDiscordLogin))
	s.Handle("GET /auth/api-keys", s.RequireAuth(http.HandlerFunc(s.handleListAPIKeys)))
	s.Handle("POST /auth/api-keys", s.RequireAuth(http.HandlerFunc(s.handleCreateAPIKey)))
	s.Handle("PATCH /auth/api-keys/{id}", s.RequireAuth(http.HandlerFunc(s.handleUpdateAPIKey)))
	s.Handle("DELETE /auth/api-keys/{id}", s.RequireAuth(http.HandlerFunc(s.handleRevokeAPIKey)))
}

type discordLoginRequest struct {
	Code string `json:"code"`
}

func (s *Server) handleDiscordLogin(w http.ResponseWriter, r *http.Request) {
	var req discordLoginRequest
	if err := decodeJSON(r, &req); err != nil {
		WriteValidationProblem(w, r, map[string][]string{
			"": {"The request could not be processed."},
		})
		return
	}

	accessToken, err := s.discord.ExchangeCode(r.Context(), req.Code)
	if err != nil {
		var oauthErr *auth.DiscordOAuthError
		if errors.As(err, &oauthErr) {
			InvalidOperation(w, r, oauthErr.Error())
			return
		}
		InternalError(w, r, s.log, err)
		return
	}

	profile, err := s.discord.GetUserProfile(r.Context(), accessToken)
	if err != nil {
		InvalidOperation(w, r, err.Error())
		return
	}

	user, err := s.upsertDiscordUser(r.Context(), profile)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	token, err := auth.IssueJWT([]byte(s.cfg.JWTKey), s.cfg.JWTIssuer, s.cfg.JWTAudience,
		user.ID, user.Name, user.Role, user.DiscordID)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

type discordUserRecord struct {
	ID        int64
	Name      string
	Role      auth.Role
	DiscordID string
}

func (s *Server) upsertDiscordUser(ctx context.Context, profile *auth.DiscordUser) (*discordUserRecord, error) {
	var avatarURL *string
	if profile.Avatar != "" {
		u := "https://cdn.discordapp.com/avatars/" + profile.ID + "/" + profile.Avatar + ".png"
		avatarURL = &u
	}

	var rec discordUserRecord
	var roleInt int
	err := s.db.QueryRow(ctx,
		`SELECT "Id", "Name", "Role", "DiscordId" FROM "Users" WHERE "DiscordId" = $1`,
		profile.ID).Scan(&rec.ID, &rec.Name, &roleInt, &rec.DiscordID)
	rec.Role, _ = auth.RoleFromInt(roleInt)
	if errors.Is(err, pgx.ErrNoRows) {

		tx, err := s.db.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)

		err = tx.QueryRow(ctx, `
			INSERT INTO "Users" ("Name","DiscordId","AvatarUrl","Role","IsBlacklisted","RequestCount","ApiKeyRequestCount","JwtRequestCount")
			VALUES ($1,$2,$3,0,false,0,0,0)
			RETURNING "Id"`,
			profile.Username, profile.ID, avatarURL).Scan(&rec.ID)
		if err != nil {
			return nil, err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO "Albums" ("Name","Description","IsDefault","UserId")
			VALUES ('Favorites','Your favorite images',true,$1)`, rec.ID)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		rec.Name = profile.Username
		rec.DiscordID = profile.ID
		rec.Role = auth.RoleUser
		return &rec, nil
	}
	if err != nil {
		return nil, err
	}

	_, err = s.db.Exec(ctx,
		`UPDATE "Users" SET "Name" = $1, "AvatarUrl" = $2 WHERE "Id" = $3`,
		profile.Username, avatarURL, rec.ID)
	if err != nil {
		return nil, err
	}
	rec.Name = profile.Username
	return &rec, nil
}

type apiKeyDto struct {
	ID             int64      `json:"id"`
	Description    string     `json:"description"`
	CreatedAt      time.Time  `json:"createdAt"`
	LastUsedAt     *time.Time `json:"lastUsedAt"`
	ExpirationDate *time.Time `json:"expirationDate"`
	UserID         int64      `json:"userId"`
}

type createdAPIKeyDto struct {
	apiKeyDto
	Key string `json:"key"`
}

type apiKeyRequest struct {
	Description    string     `json:"description"`
	ExpirationDate *time.Time `json:"expirationDate"`
}

func validateAPIKeyRequest(req *apiKeyRequest) map[string][]string {

	errs := map[string][]string{}
	if req.Description == "" {
		errs["Description"] = append(errs["Description"], "Description is required.")
	} else if len(req.Description) < 3 {
		errs["Description"] = append(errs["Description"], "Description must be at least 3 characters long.")
	} else if len(req.Description) > 100 {
		errs["Description"] = append(errs["Description"], "Description must not exceed 100 characters.")
	}
	return errs
}

func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	rows, err := s.db.Query(r.Context(), `
		SELECT "Id","Description","CreatedAt","LastUsedAt","ExpirationDate","UserId"
		FROM "ApiKeys" WHERE "UserId" = $1 ORDER BY "CreatedAt" DESC`, ident.UserID)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	defer rows.Close()

	keys := make([]apiKeyDto, 0)
	for rows.Next() {
		var k apiKeyDto
		if err := rows.Scan(&k.ID, &k.Description, &k.CreatedAt, &k.LastUsedAt, &k.ExpirationDate, &k.UserID); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		keys = append(keys, k)
	}
	writeJSON(w, http.StatusOK, keys)
}

func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	var req apiKeyRequest
	if err := decodeJSON(r, &req); err != nil {
		WriteValidationProblem(w, r, map[string][]string{
			"Description": {"Description is required."},
		})
		return
	}
	if errs := validateAPIKeyRequest(&req); len(errs) > 0 {
		WriteValidationProblem(w, r, errs)
		return
	}

	rawKey, err := auth.GenerateAPIKey()
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	var dto createdAPIKeyDto
	dto.Description = req.Description
	dto.UserID = ident.UserID
	dto.ExpirationDate = req.ExpirationDate
	dto.Key = rawKey
	err = s.db.QueryRow(r.Context(), `
		INSERT INTO "ApiKeys" ("KeyHash","Description","CreatedAt","ExpirationDate","UserId")
		VALUES ($1,$2,now(),$3,$4)
		RETURNING "Id","CreatedAt"`,
		auth.HashAPIKey(rawKey), dto.Description, dto.ExpirationDate, ident.UserID).
		Scan(&dto.ID, &dto.CreatedAt)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	w.Header().Set("Location", "/auth/api-keys")
	writeJSON(w, http.StatusCreated, dto)
}

func (s *Server) handleUpdateAPIKey(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	var req apiKeyRequest
	if err := decodeJSON(r, &req); err != nil {
		WriteValidationProblem(w, r, map[string][]string{
			"Description": {"Description is required."},
		})
		return
	}
	if errs := validateAPIKeyRequest(&req); len(errs) > 0 {
		WriteValidationProblem(w, r, errs)
		return
	}

	var dto apiKeyDto
	err = s.db.QueryRow(r.Context(), `
		UPDATE "ApiKeys" SET "Description" = $1, "ExpirationDate" = $2
		WHERE "Id" = $3 AND "UserId" = $4
		RETURNING "Id","Description","CreatedAt","LastUsedAt","ExpirationDate","UserId"`,
		req.Description, req.ExpirationDate, id, ident.UserID).
		Scan(&dto.ID, &dto.Description, &dto.CreatedAt, &dto.LastUsedAt, &dto.ExpirationDate, &dto.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, "API Key with ID "+strconv.FormatInt(id, 10)+" not found.")
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	_, err = s.db.Exec(r.Context(),
		`DELETE FROM "ApiKeys" WHERE "Id" = $1 AND "UserId" = $2`, id, ident.UserID)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
