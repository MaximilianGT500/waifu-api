package server

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

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

func (s *Server) loadBanner(ctx context.Context) *bannerRow {
	var b bannerRow
	err := s.db.QueryRow(ctx, `
		SELECT "Message", "Variant", "IsEnabled", "UpdatedAt", "UpdatedById"
		FROM "SiteBanners" WHERE "Id" = 1`).
		Scan(&b.Message, &b.Variant, &b.IsEnabled, &b.UpdatedAt, &b.UpdatedByID)
	if err != nil {
		return nil
	}
	return &b
}

func (s *Server) bannerPermissionDenied(w http.ResponseWriter, r *http.Request) {
	s.permissionRoleForbidden(w, r, s.cfg.Permissions.BannerManagementMinRole, auth.RoleModerator)
}

func (s *Server) handleGetBanner(w http.ResponseWriter, r *http.Request) {
	b := s.loadBanner(r.Context())
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

func (s *Server) handleGetBannerSettings(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	if !auth.MeetsPermission(ident.Role, s.cfg.Permissions.BannerManagementMinRole, auth.RoleModerator) {
		s.bannerPermissionDenied(w, r)
		return
	}
	b := s.loadBanner(r.Context())
	dto := bannerSettingsDto{}
	if b != nil {
		dto.Message = b.Message
		dto.Variant = bannerVariantString(b.Variant)
		dto.IsEnabled = b.IsEnabled
		dto.UpdatedAt = b.UpdatedAt
		if b.UpdatedByID != nil {
			var u userMinimalDto
			if err := s.db.QueryRow(r.Context(), `SELECT "Id","Name","AvatarUrl" FROM "Users" WHERE "Id" = $1`, *b.UpdatedByID).
				Scan(&u.ID, &u.Name, &u.AvatarURL); err == nil {
				dto.UpdatedBy = &u
			}
		}
	} else {
		dto.Variant = "Info"
		dto.IsEnabled = false
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
	if len(req.Message) > 500 {
		WriteValidationProblem(w, r, map[string][]string{
			"Message": {"Message must not exceed 500 characters."},
		})
		return
	}

	_, err := s.db.Exec(r.Context(), `
		INSERT INTO "SiteBanners" ("Id","Message","Variant","IsEnabled","UpdatedAt","UpdatedById")
		VALUES (1,$1,$2,$3,now(),$4)
		ON CONFLICT ("Id") DO UPDATE
		SET "Message" = EXCLUDED."Message", "Variant" = EXCLUDED."Variant",
			"IsEnabled" = EXCLUDED."IsEnabled", "UpdatedAt" = now(), "UpdatedById" = EXCLUDED."UpdatedById"`,
		req.Message, variant, req.IsEnabled, ident.UserID)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	b := s.loadBanner(r.Context())
	dto := bannerSettingsDto{Message: req.Message, Variant: bannerVariantString(variant), IsEnabled: req.IsEnabled}
	if b != nil {
		dto.UpdatedAt = b.UpdatedAt
		if b.UpdatedByID != nil {
			var u userMinimalDto
			if err := s.db.QueryRow(r.Context(), `SELECT "Id","Name","AvatarUrl" FROM "Users" WHERE "Id" = $1`, *b.UpdatedByID).
				Scan(&u.ID, &u.Name, &u.AvatarURL); err == nil {
				dto.UpdatedBy = &u
			}
		}
	}
	if dto.UpdatedBy == nil {
		dto.UpdatedBy = &userMinimalDto{ID: ident.UserID, Name: ident.Name}
	}
	writeJSON(w, http.StatusOK, dto)
}
