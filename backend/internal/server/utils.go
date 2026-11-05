package server

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/waifu-im/waifu-api/backend/internal/auth"
	"github.com/waifu-im/waifu-api/backend/internal/db"
)

func isUniqueViolation(err error) bool { return db.IsUniqueViolation(err) }

func isForeignKeyViolation(err error) bool { return db.IsForeignKeyViolation(err) }

func TitleOrEmpty(s string) string {
	if s == "" {
		return ""
	}
	r, size := utf8.DecodeRuneInString(s)
	return strings.ToUpper(string(r)) + s[size:]
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

func enumValue(value string, allowed ...string) (string, bool) {
	if value == "" {
		return "", false
	}
	for _, a := range allowed {
		if strings.EqualFold(value, a) {
			return a, true
		}
	}
	return "", false
}

func (s *Server) permissionRoleForbidden(w http.ResponseWriter, r *http.Request, configured string, defaultRole auth.Role) {
	min := defaultRole
	if configured != "" {
		if m, ok := auth.ParseRole(configured); ok {
			min = m
		}
	}
	Forbidden(w, r, fmt.Sprintf("This action requires the %s role or higher. This restriction is configured by the API administrator.", min))
}

func invalidEnumProblem(w http.ResponseWriter, r *http.Request, field, value string) {
	if runeLen(value) > 64 {
		value = string([]rune(value)[:64]) + "…"
	}
	WriteValidationProblem(w, r, map[string][]string{
		field: {fmt.Sprintf("The value '%s' is not valid for %s.", value, field)},
	})
}

func isModerator(ident *auth.Identity) bool {
	return ident != nil && ident.Role >= auth.RoleModerator
}

func isAdmin(ident *auth.Identity) bool {
	return ident != nil && ident.Role >= auth.RoleAdmin
}

func requireMinRole(ident *auth.Identity, minRole string) bool {
	return auth.MeetsPermission(ident.Role, minRole, auth.RoleUser)
}

func canSeeContent(ident *auth.Identity, reviewStatus int, ownerID *int64) bool {
	if reviewStatus == 1 || isModerator(ident) {
		return true
	}
	return ident != nil && ownerID != nil && *ownerID == ident.UserID
}

func orEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func orEmptyString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func strPtr(s string) *string { return &s }

func isValidWebURL(raw string) bool {
	if raw == "" || !utf8.ValidString(raw) {
		return false
	}
	for _, r := range raw {
		if unicode.IsSpace(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) ||
			unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r) || unicode.Is(unicode.Variation_Selector, r) ||
			r == '\\' {
			return false
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if u.Host == "" || u.User != nil || u.Hostname() == "" {
		return false
	}
	if strings.HasPrefix(u.Host, "[") {
		if a, err := netip.ParseAddr(u.Hostname()); err != nil || !a.Is6() {
			return false
		}
	}
	if p := u.Port(); p != "" {
		if n, err := strconv.Atoi(p); err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	return true
}

func imageKey(id int64, ext string) string {
	return strconv.FormatInt(id, 10) + ext
}

func parseImageKey(key string) (int64, string, bool) {
	dot := strings.IndexByte(key, '.')
	if dot <= 0 {
		return 0, "", false
	}
	id, err := strconv.ParseInt(key[:dot], 10, 64)
	if err != nil || id <= 0 {
		return 0, "", false
	}
	return id, key[dot:], true
}

func (s *Server) resolveTagsBySlugs(ctx context.Context, q querier, slugs []string, ident *auth.Identity) ([]tagDto, error) {
	if len(slugs) == 0 {
		return nil, nil
	}
	rows, err := q.Query(ctx, `
		SELECT "Id","Name","Slug","Description","ReviewStatus","CreatorId"
		FROM "Tags" WHERE "Slug" = ANY($1)`, slugs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	found := map[string]bool{}
	out := []tagDto{}
	for rows.Next() {
		var t tagDto
		var rs int
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.Description, &rs, &t.CreatorID); err != nil {
			return nil, err
		}
		if !canSeeContent(ident, rs, t.CreatorID) {
			continue
		}
		t.ReviewStatus = reviewStatusString(rs)
		out = append(out, t)
		found[t.Slug] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var missing []string
	for _, sl := range slugs {
		if !found[sl] {
			missing = append(missing, sl)
		}
	}
	if len(missing) > 0 {
		return nil, errNotFound(fmt.Sprintf("Tags with slugs %s not found.", strings.Join(missing, ", ")))
	}
	return out, nil
}

func (s *Server) resolveArtistsByIDs(ctx context.Context, q querier, ids []int64, ident *auth.Identity) ([]artistDto, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := q.Query(ctx, `
		SELECT "Id","Name","Patreon","Pixiv","Twitter","DeviantArt","ReviewStatus","CreatorId"
		FROM "Artists" WHERE "Id" = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	found := map[int64]bool{}
	out := []artistDto{}
	for rows.Next() {
		var a artistDto
		var rs int
		var patreon, pixiv, twitter, deviantArt *string
		if err := rows.Scan(&a.ID, &a.Name, &patreon, &pixiv, &twitter, &deviantArt, &rs, &a.CreatorID); err != nil {
			return nil, err
		}
		if !canSeeContent(ident, rs, a.CreatorID) {
			continue
		}
		a.ReviewStatus = reviewStatusString(rs)
		a.Patreon = orEmpty(patreon)
		a.Pixiv = orEmpty(pixiv)
		a.Twitter = orEmpty(twitter)
		a.DeviantArt = orEmpty(deviantArt)
		out = append(out, a)
		found[a.ID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var missing []string
	for _, id := range ids {
		if !found[id] {
			missing = append(missing, strconv.FormatInt(id, 10))
		}
	}
	if len(missing) > 0 {
		return nil, errNotFound(fmt.Sprintf("Artists with IDs %s not found.", strings.Join(missing, ", ")))
	}
	return out, nil
}

func cleanSlugList(values []string, max int, field string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		if len(v) > 100 {
			return nil, errValidation(fmt.Sprintf("Invalid value in '%s'.", field))
		}
		seen[v] = true
		out = append(out, v)
	}
	if len(out) > max {
		return nil, errValidation(fmt.Sprintf("Too many %s (maximum %d).", field, max))
	}
	return out, nil
}

func cleanIDFormList(values []string, max int, field string) ([]int64, error) {
	seen := map[int64]bool{}
	out := make([]int64, 0, len(values))
	for _, v := range values {
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil || n <= 0 || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	if len(out) > max {
		return nil, errValidation(fmt.Sprintf("Too many %s (maximum %d).", field, max))
	}
	return out, nil
}
