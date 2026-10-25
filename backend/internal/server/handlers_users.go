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
	s.Handle("PUT /users/{id}/role", s.RequireRole("Admin")(http.HandlerFunc(s.handleSetUserRole)))
	s.Handle("PUT /users/{id}/ban", s.RequireRole("Admin")(http.HandlerFunc(s.handleBanUser)))
}

func (s *Server) handleGetMe(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	var u userDto
	var role int
	err := s.db.QueryRow(r.Context(), `
		SELECT u."Id", u."Name", u."DiscordId", u."AvatarUrl", u."Role", u."IsBlacklisted", u."BlacklistReason",
			u."RequestCount", u."ApiKeyRequestCount", u."JwtRequestCount"
		FROM "Users" u WHERE u."Id" = $1`, ident.UserID).
		Scan(&u.ID, &u.Name, &u.DiscordID, &u.AvatarURL, &role, &u.IsBlacklisted, &u.BlacklistReason,
			&u.RequestCount, &u.APIKeyRequestCount, &u.JWTRequestCount)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, fmt.Sprintf("User with ID %d not found.", ident.UserID))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	rRole, _ := auth.RoleFromInt(role)
	u.Role = rRole.String()

	u.UploadedImageCount = 0
	u.AlbumImageCount = 0
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) handleGetMyPermissions(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())

	canManageBanner := auth.MeetsPermission(ident.Role, s.cfg.Permissions.BannerManagementMinRole, auth.RoleModerator)
	writeJSON(w, http.StatusOK, map[string]bool{"canManageBanner": canManageBanner})
}

func (s *Server) handleGetUserMinimal(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}

	var u userMinimalDto
	err = s.db.QueryRow(r.Context(), `SELECT "Id", "Name" FROM "Users" WHERE "Id" = $1`, id).
		Scan(&u.ID, &u.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, fmt.Sprintf("User with ID %d not found.", id))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	page, pageSize := paginationParams(r, s.cfg.PageSizes.UserDefault, s.cfg.PageSizes.UserMax)
	name := r.URL.Query().Get("name")
	includedIDs, _ := parseLongList(repeatedQuery(r, "includedIds"))

	uploadedCount := `(SELECT COUNT(*) FROM "Images" i WHERE i."UploaderId" = u."Id" AND i."ReviewStatus" = 1)`
	albumCount := `(SELECT COUNT(*) FROM "AlbumItems" ai JOIN "Albums" al ON al."Id" = ai."AlbumId" WHERE al."UserId" = u."Id")`

	bypassPagination := len(includedIDs) > 0

	where := " WHERE TRUE"
	args := []any{}
	ph := func() string { return fmt.Sprintf("$%d", len(args)+1) }
	if name != "" && !bypassPagination {

		where += fmt.Sprintf(` AND (lower(u."Name") LIKE %s OR u."DiscordId" LIKE %s)`, ph(), ph())
		args = append(args, "%"+strings.ToLower(name)+"%", "%"+name+"%")
	}
	if bypassPagination {
		where += fmt.Sprintf(` AND u."Id" = ANY(%s)`, ph())
		args = append(args, includedIDs)
	}

	var total int64
	if err := s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM "Users" u`+where, args...).Scan(&total); err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	query := fmt.Sprintf(`
		SELECT u."Id", u."Name", u."DiscordId", u."AvatarUrl", u."Role", u."IsBlacklisted", u."BlacklistReason",
			u."RequestCount", u."ApiKeyRequestCount", u."JwtRequestCount", %s, %s
		FROM "Users" u%s`, uploadedCount, albumCount, where)
	if !bypassPagination {
		query += fmt.Sprintf(" ORDER BY %s DESC LIMIT %d OFFSET %d", uploadedCount, pageSize, (page-1)*pageSize)
	}

	rows, err := s.db.Query(r.Context(), query, args...)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	defer rows.Close()

	items := []userDto{}
	for rows.Next() {
		var u userDto
		var role int
		if err := rows.Scan(&u.ID, &u.Name, &u.DiscordID, &u.AvatarURL, &role, &u.IsBlacklisted, &u.BlacklistReason,
			&u.RequestCount, &u.APIKeyRequestCount, &u.JWTRequestCount,
			&u.UploadedImageCount, &u.AlbumImageCount); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		rRole, _ := auth.RoleFromInt(role)
		u.Role = rRole.String()
		items = append(items, u)
	}

	if bypassPagination {

		writeJSON(w, http.StatusOK, newPaginated(items, 1, len(items), int64(len(items)), s.cfg.PageSizes.UserMax, s.cfg.PageSizes.UserDefault))
		return
	}
	writeJSON(w, http.StatusOK, newPaginated(items, page, pageSize, total, s.cfg.PageSizes.UserMax, s.cfg.PageSizes.UserDefault))
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
	var u userEntity
	var roleInt int
	err := s.db.QueryRow(r.Context(), `
		SELECT "Id","Name","DiscordId","AvatarUrl","Role","IsBlacklisted","BlacklistReason",
			"RequestCount","ApiKeyRequestCount","JwtRequestCount"
		FROM "Users" WHERE "Id" = $1`, id).
		Scan(&u.ID, &u.Name, &u.DiscordID, &u.AvatarURL, &roleInt, &u.IsBlacklisted, &u.BlacklistReason,
			&u.RequestCount, &u.APIKeyRequestCount, &u.JWTRequestCount)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, fmt.Sprintf("User with ID %d not found.", id))
		return nil, false
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return nil, false
	}
	rRole, _ := auth.RoleFromInt(roleInt)
	u.Role = rRole.String()
	return &u, true
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
	if len(req.Reason) > 200 {
		WriteValidationProblem(w, r, map[string][]string{
			"Reason": {"Blacklist reason must not exceed 200 characters."},
		})
		return
	}

	var reason *string
	if req.IsBlacklisted {
		reason = orEmptyString(req.Reason)
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

	u, ok := s.loadUserEntity(w, r, id)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, u)
}
