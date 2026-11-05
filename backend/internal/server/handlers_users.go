package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/waifu-im/waifu-api/backend/internal/auth"
)

func (s *Server) RegisterUserRoutes() {
	s.Handle("GET /users/me", s.RequireAuth(http.HandlerFunc(s.handleGetMe)))
	s.Handle("GET /users/me/permissions", s.RequireAuth(http.HandlerFunc(s.handleGetMyPermissions)))
	s.Handle("GET /users/{id}", s.RequireRole("Moderator")(http.HandlerFunc(s.handleGetUserMinimal)))
	s.Handle("GET /users", s.RequireRole("Admin")(http.HandlerFunc(s.handleListUsers)))
	s.Handle("PUT /users/{id}/role", s.RequireRole("Admin")(s.requireSession(adminSessionDetail, http.HandlerFunc(s.handleSetUserRole))))
	s.Handle("PUT /users/{id}/ban", s.RequireRole("Admin")(s.requireSession(adminSessionDetail, http.HandlerFunc(s.handleBanUser))))
}

const userColumns = `u."Id", u."Name", u."DiscordId", u."AvatarUrl", u."Role", u."IsBlacklisted", u."BlacklistReason",
	u."RequestCount", u."ApiKeyRequestCount", u."JwtRequestCount"`

func scanUser(row pgx.Row, extra ...any) (*userDto, error) {
	var u userDto
	var role int
	var name, discordID, avatar *string
	dest := append([]any{&u.ID, &name, &discordID, &avatar, &role, &u.IsBlacklisted, &u.BlacklistReason,
		&u.RequestCount, &u.APIKeyRequestCount, &u.JWTRequestCount}, extra...)
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	u.Name = orEmpty(name)
	u.DiscordID = orEmpty(discordID)
	u.AvatarURL = orEmpty(avatar)
	rRole, _ := auth.RoleFromInt(role)
	u.Role = rRole.String()
	return &u, nil
}

func (s *Server) handleGetMe(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	u, err := scanUser(s.db.QueryRow(r.Context(), `SELECT `+userColumns+` FROM "Users" u WHERE u."Id" = $1`, ident.UserID))
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, fmt.Sprintf("User with ID %d not found.", ident.UserID))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	u.UploadedImageCount = 0
	u.AlbumImageCount = 0
	writeJSON(w, http.StatusOK, u)
}

type uploadLimitsDto struct {
	MaxFileSizeMB       int      `json:"maxFileSizeMb"`
	AllowedContentTypes []string `json:"allowedContentTypes"`
	MinWidth            int      `json:"minWidth"`
	MinHeight           int      `json:"minHeight"`
	MaxWidth            int      `json:"maxWidth"`
	MaxHeight           int      `json:"maxHeight"`
	AnimatedMinWidth    int      `json:"animatedMinWidth"`
	AnimatedMinHeight   int      `json:"animatedMinHeight"`
	AnimatedMaxWidth    int      `json:"animatedMaxWidth"`
	AnimatedMaxHeight   int      `json:"animatedMaxHeight"`
	MaxPixels           int64    `json:"maxPixels"`
	MaxTags             int      `json:"maxTags"`
	MaxArtists          int      `json:"maxArtists"`
}

type permissionsDto struct {
	CanManageBanner bool            `json:"canManageBanner"`
	Upload          uploadLimitsDto `json:"upload"`
}

func orDefault(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}

func (s *Server) uploadLimits() uploadLimitsDto {
	img := s.cfg.Image
	return uploadLimitsDto{
		MaxFileSizeMB:       s.cfg.MaxUploadMB,
		AllowedContentTypes: []string{"image/jpeg", "image/png", "image/gif", "image/webp"},
		MinWidth:            img.MinWidth,
		MinHeight:           img.MinHeight,
		MaxWidth:            img.MaxWidth,
		MaxHeight:           img.MaxHeight,
		AnimatedMinWidth:    orDefault(img.AnimatedMinWidth, img.MinWidth),
		AnimatedMinHeight:   orDefault(img.AnimatedMinHeight, img.MinHeight),
		AnimatedMaxWidth:    orDefault(img.AnimatedMaxWidth, img.MaxWidth),
		AnimatedMaxHeight:   orDefault(img.AnimatedMaxHeight, img.MaxHeight),
		MaxPixels:           s.cfg.Limits.MaxImagePixels,
		MaxTags:             s.cfg.Limits.MaxTagsPerImage,
		MaxArtists:          s.cfg.Limits.MaxArtistsPerImage,
	}
}

func (s *Server) handleGetMyPermissions(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	writeJSON(w, http.StatusOK, permissionsDto{
		CanManageBanner: auth.MeetsPermission(ident.Role, s.cfg.Permissions.BannerManagementMinRole, auth.RoleModerator),
		Upload:          s.uploadLimits(),
	})
}

func (s *Server) handleGetUserMinimal(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	var u userMinimalDto
	var name *string
	err = s.db.QueryRow(r.Context(), `SELECT "Id", "Name" FROM "Users" WHERE "Id" = $1`, id).Scan(&u.ID, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, fmt.Sprintf("User with ID %d not found.", id))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	u.Name = orEmpty(name)
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page, pageSize := s.paginationParams(r, s.cfg.PageSizes.UserDefault, s.cfg.PageSizes.UserMax)
	name := strings.TrimSpace(queryGet(r, "name"))
	includedIDs, err := s.idListParam(r, "includedIds")
	if err != nil {
		BadRequest(w, r, err.Error())
		return
	}

	counts := ` CROSS JOIN LATERAL (
			SELECT
				(SELECT COUNT(*) FROM "Images" i WHERE i."UploaderId" = u."Id" AND i."ReviewStatus" = 1) AS uploaded,
				(SELECT COUNT(*) FROM "AlbumItems" ai JOIN "Albums" al ON al."Id" = ai."AlbumId" WHERE al."UserId" = u."Id") AS album
		) c`

	bypassPagination := len(includedIDs) > 0
	a := &sqlArgs{}
	where := " WHERE TRUE"
	if bypassPagination {
		where += ` AND u."Id" = ANY(` + a.add(includedIDs) + `)`
	} else if name != "" {
		where += fmt.Sprintf(` AND (lower(u."Name") LIKE %s OR u."DiscordId" LIKE %s)`,
			a.add(likePattern(strings.ToLower(name))), a.add(likePattern(name)))
	}

	var total int64
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM "Users" u`+where, a.args...).Scan(&total); err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	query := `SELECT ` + userColumns + `, c.uploaded, c.album FROM "Users" u` + counts + where
	if bypassPagination {
		query += ` ORDER BY u."Id"`
	} else {
		query += fmt.Sprintf(` ORDER BY c.uploaded DESC, u."Id" ASC LIMIT %d OFFSET %d`, pageSize, offsetOf(page, pageSize))
	}

	rows, err := s.db.Query(ctx, query, a.args...)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	defer rows.Close()

	items := []userDto{}
	for rows.Next() {
		var uploaded, album int64
		u, err := scanUser(rows, &uploaded, &album)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		u.UploadedImageCount = uploaded
		u.AlbumImageCount = album
		items = append(items, *u)
	}
	if err := rows.Err(); err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	if bypassPagination {
		writeJSON(w, http.StatusOK, s.newPaginated(items, 1, len(items), int64(len(items)), s.cfg.PageSizes.UserMax, s.cfg.PageSizes.UserDefault))
		return
	}
	writeJSON(w, http.StatusOK, s.newPaginated(items, page, pageSize, total, s.cfg.PageSizes.UserMax, s.cfg.PageSizes.UserDefault))
}

type userEntity struct {
	ID                 int64   `json:"id"`
	Name               string  `json:"name"`
	DiscordID          string  `json:"discordId"`
	AvatarURL          *string `json:"avatarUrl"`
	Role               string  `json:"role"`
	IsBlacklisted      bool    `json:"isBlacklisted"`
	BlacklistReason    *string `json:"blacklistReason"`
	RequestCount       int64   `json:"requestCount"`
	APIKeyRequestCount int64   `json:"apiKeyRequestCount"`
	JWTRequestCount    int64   `json:"jwtRequestCount"`

	UploadedImageCount int64 `json:"uploadedImageCount"`
	AlbumImageCount    int64 `json:"albumImageCount"`
}

func (s *Server) loadUserEntity(w http.ResponseWriter, r *http.Request, id int64) (*userEntity, bool) {
	u, err := scanUser(s.db.QueryRow(r.Context(), `SELECT `+userColumns+` FROM "Users" u WHERE u."Id" = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, fmt.Sprintf("User with ID %d not found.", id))
		return nil, false
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return nil, false
	}
	ent := &userEntity{
		ID: u.ID, Name: u.Name, DiscordID: u.DiscordID, Role: u.Role,
		IsBlacklisted: u.IsBlacklisted, BlacklistReason: u.BlacklistReason,
		RequestCount: u.RequestCount, APIKeyRequestCount: u.APIKeyRequestCount, JWTRequestCount: u.JWTRequestCount,
	}
	if u.AvatarURL != "" {
		ent.AvatarURL = &u.AvatarURL
	}
	return ent, true
}

func (s *Server) handleSetUserRole(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	if id == ident.UserID {
		InvalidOperation(w, r, "You cannot change your own role.")
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if err := decodeJSON(r, &req); err != nil {
		WriteValidationProblem(w, r, map[string][]string{
			"": {"The request could not be processed."},
		})
		return
	}
	role, ok := auth.ParseRole(req.Role)
	if !ok {
		invalidEnumProblem(w, r, "role", req.Role)
		return
	}
	res, err := s.db.Exec(r.Context(), `UPDATE "Users" SET "Role" = $1 WHERE "Id" = $2`, int(role), id)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if res.RowsAffected() == 0 {
		NotFound(w, r, fmt.Sprintf("User with ID %d not found.", id))
		return
	}
	s.log.Info("user role changed", "userId", id, "role", role.String(), "by", ident.UserID)

	u, ok := s.loadUserEntity(w, r, id)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) handleBanUser(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	if id == ident.UserID {
		InvalidOperation(w, r, "You cannot ban yourself.")
		return
	}
	var req struct {
		IsBlacklisted bool   `json:"isBlacklisted"`
		Reason        string `json:"reason"`
	}
	if err := decodeJSON(r, &req); err != nil {
		WriteValidationProblem(w, r, map[string][]string{
			"": {"The request could not be processed."},
		})
		return
	}
	reasonText := strings.TrimSpace(req.Reason)
	if runeLen(reasonText) > 200 {
		WriteValidationProblem(w, r, map[string][]string{
			"Reason": {"Blacklist reason must not exceed 200 characters."},
		})
		return
	}

	var reason *string
	if req.IsBlacklisted {
		reason = orEmptyString(reasonText)
	}
	res, err := s.db.Exec(r.Context(), `
		UPDATE "Users" SET "IsBlacklisted" = $1, "BlacklistReason" = $2 WHERE "Id" = $3`,
		req.IsBlacklisted, reason, id)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if res.RowsAffected() == 0 {
		NotFound(w, r, fmt.Sprintf("User with ID %d not found.", id))
		return
	}
	s.log.Info("user blacklist changed", "userId", id, "blacklisted", req.IsBlacklisted, "by", ident.UserID)

	u, ok := s.loadUserEntity(w, r, id)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, u)
}
