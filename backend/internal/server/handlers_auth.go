package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/waifu-im/waifu-api/backend/internal/auth"
)

func (s *Server) RegisterAuthRoutes() {
	s.Handle("POST /auth/discord", s.withLimit(s.authLimiter, http.HandlerFunc(s.handleDiscordLogin)))
	s.Handle("GET /auth/api-keys", s.RequireAuth(http.HandlerFunc(s.handleListAPIKeys)))
	s.Handle("POST /auth/api-keys", s.RequireSession(http.HandlerFunc(s.handleCreateAPIKey)))
	s.Handle("PATCH /auth/api-keys/{id}", s.RequireSession(http.HandlerFunc(s.handleUpdateAPIKey)))
	s.Handle("DELETE /auth/api-keys/{id}", s.RequireAuth(http.HandlerFunc(s.handleRevokeAPIKey)))
}

type discordLoginRequest struct {
	Code string `json:"code"`
}

var oauthCodeRe = regexp.MustCompile(`^[A-Za-z0-9._~\-]{1,512}$`)

func (s *Server) handleDiscordLogin(w http.ResponseWriter, r *http.Request) {
	var req discordLoginRequest
	if err := decodeJSON(r, &req); err != nil {
		WriteValidationProblem(w, r, map[string][]string{
			"": {"The request could not be processed."},
		})
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	if !oauthCodeRe.MatchString(req.Code) {
		WriteValidationProblem(w, r, map[string][]string{
			"Code": {"A valid authorization code is required."},
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	accessToken, err := s.discord.ExchangeCode(ctx, req.Code)
	if err != nil {
		var oauthErr *auth.DiscordOAuthError
		if errors.As(err, &oauthErr) {
			InvalidOperation(w, r, oauthErr.Error())
			return
		}
		s.log.Warn("discord token exchange failed", "error", err)
		WriteProblem(w, r, http.StatusBadGateway, "https://tools.ietf.org/html/rfc9110#section-15.6.3",
			"Bad Gateway", "Discord could not be reached. Please try again later.")
		return
	}

	profile, err := s.discord.GetUserProfile(ctx, accessToken)
	if err != nil {
		if !errors.Is(err, auth.ErrDiscordProfile) {
			s.log.Warn("discord profile request failed", "error", err)
		}
		InvalidOperation(w, r, auth.ErrDiscordProfile.Error())
		return
	}

	user, err := s.upsertDiscordUser(ctx, profile)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if user.Blacklisted {
		detail := "Your account has been blacklisted."
		if user.BlacklistReason != nil && *user.BlacklistReason != "" {
			detail = "Your account has been blacklisted. Reason: " + *user.BlacklistReason
		}
		Forbidden(w, r, detail)
		return
	}

	token, err := auth.IssueJWT([]byte(s.cfg.JWTKey), s.cfg.JWTIssuer, s.cfg.JWTAudience,
		user.ID, user.Name, user.Role, user.DiscordID, s.cfg.JWTLifetime)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

type discordUserRecord struct {
	ID              int64
	Name            string
	Role            auth.Role
	DiscordID       string
	Blacklisted     bool
	BlacklistReason *string
}

func (s *Server) upsertDiscordUser(ctx context.Context, profile *auth.DiscordUser) (*discordUserRecord, error) {
	avatarURL := profile.AvatarURL()
	rec := &discordUserRecord{Name: profile.Username, DiscordID: profile.ID}

	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('discord-user'), hashtext($1))`, profile.ID); err != nil {
			return err
		}
		var roleInt int
		err := tx.QueryRow(ctx,
			`SELECT "Id", "Role", "IsBlacklisted", "BlacklistReason" FROM "Users" WHERE "DiscordId" = $1 ORDER BY "Id" LIMIT 1`,
			profile.ID).Scan(&rec.ID, &roleInt, &rec.Blacklisted, &rec.BlacklistReason)
		if errors.Is(err, pgx.ErrNoRows) {
			if err := tx.QueryRow(ctx, `
				INSERT INTO "Users" ("Name","DiscordId","AvatarUrl","Role","IsBlacklisted","RequestCount","ApiKeyRequestCount","JwtRequestCount")
				VALUES ($1,$2,$3,0,false,0,0,0)
				RETURNING "Id"`,
				profile.Username, profile.ID, avatarURL).Scan(&rec.ID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO "Albums" ("Name","Description","IsDefault","UserId")
				VALUES ('Favorites','Your favorite images',true,$1)`, rec.ID); err != nil {
				return err
			}
			rec.Role = auth.RoleUser
			return nil
		}
		if err != nil {
			return err
		}
		rec.Role, _ = auth.RoleFromInt(roleInt)
		_, err = tx.Exec(ctx, `UPDATE "Users" SET "Name" = $1, "AvatarUrl" = $2 WHERE "Id" = $3`,
			profile.Username, avatarURL, rec.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return rec, nil
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

func validateAPIKeyRequest(req *apiKeyRequest, creating bool) map[string][]string {
	errs := map[string][]string{}
	req.Description = strings.TrimSpace(req.Description)
	switch n := runeLen(req.Description); {
	case n == 0:
		errs["Description"] = append(errs["Description"], "Description is required.")
	case n < 3:
		errs["Description"] = append(errs["Description"], "Description must be at least 3 characters long.")
	case n > 100:
		errs["Description"] = append(errs["Description"], "Description must not exceed 100 characters.")
	}
	if creating && req.ExpirationDate != nil && !req.ExpirationDate.After(time.Now()) {
		errs["ExpirationDate"] = append(errs["ExpirationDate"], "Expiration date must be in the future.")
	}
	return errs
}

func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	rows, err := s.db.Query(r.Context(), `
		SELECT "Id","Description","CreatedAt","LastUsedAt","ExpirationDate","UserId"
		FROM "ApiKeys" WHERE "UserId" = $1 ORDER BY "CreatedAt" DESC, "Id" DESC`, ident.UserID)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	defer rows.Close()

	keys := make([]apiKeyDto, 0)
	for rows.Next() {
		var k apiKeyDto
		var desc *string
		if err := rows.Scan(&k.ID, &desc, &k.CreatedAt, &k.LastUsedAt, &k.ExpirationDate, &k.UserID); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		k.Description = orEmpty(desc)
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
	var req apiKeyRequest
	if err := decodeJSON(r, &req); err != nil {
		WriteValidationProblem(w, r, map[string][]string{
			"Description": {"Description is required."},
		})
		return
	}
	if errs := validateAPIKeyRequest(&req, true); len(errs) > 0 {
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
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('api-keys'), $1::int)`, int32(ident.UserID)); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM "ApiKeys" WHERE "UserId" = $1`, ident.UserID).Scan(&count); err != nil {
			return err
		}
		if count >= s.cfg.Limits.MaxAPIKeysPerUser {
			return errConflict(fmt.Sprintf("You have reached the maximum number of API keys (%d). Revoke an unused key first.", s.cfg.Limits.MaxAPIKeysPerUser))
		}
		return tx.QueryRow(ctx, `
			INSERT INTO "ApiKeys" ("KeyHash","Description","CreatedAt","ExpirationDate","UserId")
			VALUES ($1,$2,now(),$3,$4)
			RETURNING "Id","CreatedAt"`,
			auth.HashAPIKey(rawKey), dto.Description, dto.ExpirationDate, ident.UserID).
			Scan(&dto.ID, &dto.CreatedAt)
	})
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Location", "/auth/api-keys")
	w.Header().Set("Cache-Control", "no-store")
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
	if errs := validateAPIKeyRequest(&req, false); len(errs) > 0 {
		WriteValidationProblem(w, r, errs)
		return
	}

	var dto apiKeyDto
	var desc *string
	err = s.db.QueryRow(r.Context(), `
		UPDATE "ApiKeys" SET "Description" = $1, "ExpirationDate" = $2
		WHERE "Id" = $3 AND "UserId" = $4
		RETURNING "Id","Description","CreatedAt","LastUsedAt","ExpirationDate","UserId"`,
		req.Description, req.ExpirationDate, id, ident.UserID).
		Scan(&dto.ID, &desc, &dto.CreatedAt, &dto.LastUsedAt, &dto.ExpirationDate, &dto.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, "API Key with ID "+strconv.FormatInt(id, 10)+" not found.")
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	dto.Description = orEmpty(desc)
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	if _, err = s.db.Exec(r.Context(),
		`DELETE FROM "ApiKeys" WHERE "Id" = $1 AND "UserId" = $2`, id, ident.UserID); err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
