package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/waifu-im/waifu-api/backend/internal/auth"
)

func (s *Server) RegisterBannerRoutes() {
	s.Handle("GET /banner", http.HandlerFunc(s.handleGetBanner))
	s.Handle("GET /banner/settings", s.RequireAuth(http.HandlerFunc(s.handleGetBannerSettings)))
	s.Handle("PUT /banner/settings", s.RequireAuth(http.HandlerFunc(s.handlePutBannerSettings)))
}

type bannerRow struct {
	Message     string
	Variant     int
	IsEnabled   bool
	UpdatedAt   *time.Time
	UpdatedByID *int64
}

func (s *Server) loadBanner(ctx context.Context) (*bannerRow, error) {
	var b bannerRow
	var msg *string
	err := s.db.QueryRow(ctx, `
		SELECT "Message", "Variant", "IsEnabled", "UpdatedAt", "UpdatedById"
		FROM "SiteBanners" WHERE "Id" = 1`).
		Scan(&msg, &b.Variant, &b.IsEnabled, &b.UpdatedAt, &b.UpdatedByID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b.Message = orEmpty(msg)
	return &b, nil
}

func (s *Server) bannerPermissionDenied(w http.ResponseWriter, r *http.Request) {
	s.permissionRoleForbidden(w, r, s.cfg.Permissions.BannerManagementMinRole, auth.RoleModerator)
}

func (s *Server) handleGetBanner(w http.ResponseWriter, r *http.Request) {
	b, err := s.loadBanner(r.Context())
	if err != nil {
		s.log.Warn("banner lookup failed", "error", err)
	}
	if b == nil || !b.IsEnabled || strings.TrimSpace(b.Message) == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"message":   b.Message,
		"variant":   bannerVariantString(b.Variant),
		"updatedAt": b.UpdatedAt,
	})
}

func (s *Server) bannerUpdatedBy(ctx context.Context, id *int64) *userMinimalDto {
	if id == nil {
		return nil
	}
	var u userMinimalDto
	var name *string
	if err := s.db.QueryRow(ctx, `SELECT "Id","Name","AvatarUrl" FROM "Users" WHERE "Id" = $1`, *id).
		Scan(&u.ID, &name, &u.AvatarURL); err != nil {
		return nil
	}
	u.Name = orEmpty(name)
	return &u
}

func (s *Server) handleGetBannerSettings(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	if !auth.MeetsPermission(ident.Role, s.cfg.Permissions.BannerManagementMinRole, auth.RoleModerator) {
		s.bannerPermissionDenied(w, r)
		return
	}
	b, err := s.loadBanner(r.Context())
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	dto := bannerSettingsDto{Variant: "Info"}
	if b != nil {
		dto.Message = b.Message
		dto.Variant = bannerVariantString(b.Variant)
		dto.IsEnabled = b.IsEnabled
		dto.UpdatedAt = b.UpdatedAt
		dto.UpdatedBy = s.bannerUpdatedBy(r.Context(), b.UpdatedByID)
	}
	writeJSON(w, http.StatusOK, dto)
}

func parseBannerVariant(v string) (int, bool) {
	if v == "" {
		return 0, false
	}
	switch {
	case strings.EqualFold(v, "Info"):
		return 0, true
	case strings.EqualFold(v, "Success"):
		return 1, true
	case strings.EqualFold(v, "Warning"):
		return 2, true
	case strings.EqualFold(v, "Danger"):
		return 3, true
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 3 {
		return n, true
	}
	return 0, false
}

func (s *Server) handlePutBannerSettings(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	if !auth.MeetsPermission(ident.Role, s.cfg.Permissions.BannerManagementMinRole, auth.RoleModerator) {
		s.bannerPermissionDenied(w, r)
		return
	}
	var req struct {
		Message   string `json:"message"`
		Variant   string `json:"variant"`
		IsEnabled bool   `json:"isEnabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		WriteValidationProblem(w, r, map[string][]string{
			"": {"The request could not be processed."},
		})
		return
	}

	variant, ok := parseBannerVariant(req.Variant)
	if !ok {
		invalidEnumProblem(w, r, "variant", req.Variant)
		return
	}
	if req.IsEnabled && strings.TrimSpace(req.Message) == "" {
		BadRequest(w, r, "An enabled banner needs a message.")
		return
	}
	if runeLen(req.Message) > 500 {
		WriteValidationProblem(w, r, map[string][]string{
			"Message": {"Message must not exceed 500 characters."},
		})
		return
	}

	if _, err := s.db.Exec(r.Context(), `
		INSERT INTO "SiteBanners" ("Id","Message","Variant","IsEnabled","UpdatedAt","UpdatedById")
		VALUES (1,$1,$2,$3,now(),$4)
		ON CONFLICT ("Id") DO UPDATE
		SET "Message" = EXCLUDED."Message", "Variant" = EXCLUDED."Variant",
			"IsEnabled" = EXCLUDED."IsEnabled", "UpdatedAt" = now(), "UpdatedById" = EXCLUDED."UpdatedById"`,
		req.Message, variant, req.IsEnabled, ident.UserID); err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	dto := bannerSettingsDto{Message: req.Message, Variant: bannerVariantString(variant), IsEnabled: req.IsEnabled}
	if b, err := s.loadBanner(r.Context()); err == nil && b != nil {
		dto.UpdatedAt = b.UpdatedAt
		dto.UpdatedBy = s.bannerUpdatedBy(r.Context(), b.UpdatedByID)
	}
	if dto.UpdatedBy == nil {
		dto.UpdatedBy = &userMinimalDto{ID: ident.UserID, Name: ident.Name}
	}
	writeJSON(w, http.StatusOK, dto)
}
