package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/waifu-im/waifu-api/backend/internal/auth"
	"github.com/waifu-im/waifu-api/backend/internal/db"
)

func isUniqueViolation(err error) bool { return db.IsUniqueViolation(err) }

func TitleOrEmpty(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

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
	WriteValidationProblem(w, r, map[string][]string{
		field: {fmt.Sprintf("The value '%s' is not valid for %s.", value, field)},
	})
}

func (s *Server) resolveTagsBySlugs(ctx context.Context, slugs []string) ([]tagDto, error) {
	if len(slugs) == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `
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
		t.ReviewStatus = reviewStatusString(rs)
		out = append(out, t)
		found[t.Slug] = true
	}
	var missing []string
	for _, sl := range slugs {
		if !found[sl] {
			missing = append(missing, sl)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("Tags with slugs %s not found.", strings.Join(missing, ", "))
	}
	return out, nil
}

func (s *Server) resolveArtistsByIDs(ctx context.Context, rawIDs []string) ([]artistDto, error) {
	if len(rawIDs) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(rawIDs))
	for _, v := range rawIDs {
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			ids = append(ids, n)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `
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
		a.ReviewStatus = reviewStatusString(rs)
		a.Patreon = orEmpty(patreon)
		a.Pixiv = orEmpty(pixiv)
		a.Twitter = orEmpty(twitter)
		a.DeviantArt = orEmpty(deviantArt)
		out = append(out, a)
		found[a.ID] = true
	}
	var missing []string
	for _, id := range ids {
		if !found[id] {
			missing = append(missing, strconv.FormatInt(id, 10))
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("Artists with IDs %s not found.", strings.Join(missing, ", "))
	}
	return out, nil
}

func (s *Server) purgeCDN(keys []string) {
	if s.cfg.Cloudflare.ZoneID == "" || s.cfg.Cloudflare.APIToken == "" || len(keys) == 0 {
		return
	}
	files := make([]string, 0, len(keys)*3)
	for _, k := range keys {
		files = append(files, s.s3.PublicURL(k))
		if s.cfg.ImgTransform.Enabled {
			files = append(files,
				s.s3.PublicURL("/cdn-cgi/image/format=auto,width="+strconv.Itoa(s.cfg.ImgTransform.CardWidth)+"/"+k),
				s.s3.PublicURL("/cdn-cgi/image/format=auto,width="+strconv.Itoa(s.cfg.ImgTransform.PreviewWidth)+"/"+k))
		}
	}

	body, _ := json.Marshal(map[string]any{"files": files})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.cloudflare.com/client/v4/zones/"+s.cfg.Cloudflare.ZoneID+"/purge_cache",
		bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.Cloudflare.APIToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.log.Warn("cdn purge failed", "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		s.log.Warn("cdn purge rejected", "status", resp.StatusCode)
	}
}

func requireMinRole(ident *auth.Identity, minRole string) bool {
	return auth.MeetsPermission(ident.Role, minRole, auth.RoleUser)
}
