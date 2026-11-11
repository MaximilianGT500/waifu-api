package server

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/waifu-im/waifu-api/backend/internal/auth"
)

func (s *Server) RegisterDiscoveryRoutes() {
	s.Handle("GET /images/facets", http.HandlerFunc(s.handleImageFacets))
	s.Handle("GET /images/feed", http.HandlerFunc(s.handleImageFeed))
	s.Handle("GET /images/{id}/similar", http.HandlerFunc(s.handleSimilarImages))
}

type facetEntry struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Slug  string `json:"slug,omitempty"`
	Count int64  `json:"count"`
}

type facetResult struct {
	TotalCount int64        `json:"totalCount"`
	Tags       []facetEntry `json:"tags"`
	Artists    []facetEntry `json:"artists"`
}

func intParam(r *http.Request, key string, def, lo, hi int) (int, error) {
	v := strings.TrimSpace(queryGet(r, key))
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < lo || n > hi {
		return 0, errFieldValidation(key, fmt.Sprintf("The value must be a whole number between %d and %d.", lo, hi))
	}
	return n, nil
}

func sharedImageFilter(f *imageFilter, ident *auth.Identity) bool {
	return f.reviewStatus == "Accepted" && !(f.includeMyPending && ident != nil) && !f.mine && len(f.includedIDs) == 0 && len(f.excludedIDs) == 0
}

func (s *Server) parseVisibleImageFilters(w http.ResponseWriter, r *http.Request, ident *auth.Identity) (*imageFilter, bool) {
	f, err := s.parseImageFilters(r, ident)
	if err != nil {
		s.writeDomainError(w, r, err)
		return nil, false
	}
	if (f.reviewStatus != "Accepted" || f.childrenReviewStatus != "Accepted") && !isModerator(ident) {
		Forbidden(w, r, "Filtering by non-accepted review status is only available to moderators and admins.")
		return nil, false
	}
	return f, true
}

func andWhere(conds string, extra ...string) string {
	parts := make([]string, 0, len(extra)+1)
	if conds != "" {
		parts = append(parts, conds)
	}
	parts = append(parts, extra...)
	if len(parts) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(parts, " AND ")
}

func (s *Server) handleImageFacets(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
	f, ok := s.parseVisibleImageFilters(w, r, ident)
	if !ok {
		return
	}
	limit, err := intParam(r, "limit", 12, 1, 50)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	compute := func(ctx context.Context) (*facetResult, int, error) {
		res, err := s.computeFacets(ctx, f, ident, limit)
		return res, 1, err
	}
	var res *facetResult
	if s.facets.enabled() && sharedImageFilter(f, ident) {
		res, err = s.facets.load(ctx, imageListKey(f, orderNewest)+"|"+strconv.Itoa(limit), 0, compute)
	} else {
		res, _, err = compute(ctx)
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	writePublicJSON(w, r, res)
}

func (s *Server) computeFacets(ctx context.Context, f *imageFilter, ident *auth.Identity, limit int) (*facetResult, error) {
	res := &facetResult{Tags: []facetEntry{}, Artists: []facetEntry{}}
	rf, err := s.resolveIfNeeded(ctx, f)
	if err != nil {
		return nil, err
	}
	if rf.impossible {
		return res, nil
	}
	conds, args := buildResolvedImageConds(rf, ident, nil, 0)
	n := len(args)
	includedArtists := []int64{}
	for _, set := range rf.artistSets {
		includedArtists = append(includedArtists, set...)
	}
	tagArgs := append(append([]any{}, args...), append([]int64{}, rf.tagIDs...), limit)
	artistArgs := append(append([]any{}, args...), includedArtists, limit)
	b := &pgx.Batch{}
	b.Queue(`SELECT COUNT(*) FROM "Images" i`+whereClause(conds), args...).QueryRow(func(row pgx.Row) error {
		return row.Scan(&res.TotalCount)
	})
	b.Queue(`
		SELECT t."Id", t."Name", t."Slug", COUNT(*) AS cnt
		FROM "Images" i
		JOIN "ImageTag" it ON it."ImagesId" = i."Id"
		JOIN "Tags" t ON t."Id" = it."TagsId" AND t."ReviewStatus" = 1`+
		andWhere(conds, fmt.Sprintf(`it."TagsId" <> ALL($%d::bigint[])`, n+1))+`
		GROUP BY t."Id", t."Name", t."Slug"
		ORDER BY cnt DESC, t."Id" ASC
		LIMIT $`+strconv.Itoa(n+2), tagArgs...).Query(func(rows pgx.Rows) error {
		for rows.Next() {
			var e facetEntry
			if err := rows.Scan(&e.ID, &e.Name, &e.Slug, &e.Count); err != nil {
				return err
			}
			res.Tags = append(res.Tags, e)
		}
		return rows.Err()
	})
	b.Queue(`
		SELECT a."Id", a."Name", COUNT(*) AS cnt
		FROM "Images" i
		JOIN "ArtistImage" ar ON ar."ImagesId" = i."Id"
		JOIN "Artists" a ON a."Id" = ar."ArtistsId" AND a."ReviewStatus" = 1`+
		andWhere(conds, fmt.Sprintf(`ar."ArtistsId" <> ALL($%d::bigint[])`, n+1))+`
		GROUP BY a."Id", a."Name"
		ORDER BY cnt DESC, a."Id" ASC
		LIMIT $`+strconv.Itoa(n+2), artistArgs...).Query(func(rows pgx.Rows) error {
		for rows.Next() {
			var e facetEntry
			if err := rows.Scan(&e.ID, &e.Name, &e.Count); err != nil {
				return err
			}
			res.Artists = append(res.Artists, e)
		}
		return rows.Err()
	})
	if err := s.db.SendBatch(ctx, b).Close(); err != nil {
		return nil, err
	}
	return res, nil
}

func (s *Server) handleSimilarImages(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
	limit, err := intParam(r, "limit", 12, 1, 30)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	maxDistance, err := intParam(r, "maxDistance", 64, 0, 64)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	isNsfw := "False"
	if v := queryGet(r, "isNsfw"); v != "" {
		var ok bool
		if isNsfw, ok = enumValue(v, "False", "True", "All"); !ok {
			invalidEnumProblem(w, r, "isNsfw", v)
			return
		}
	}

	var hash pgtype.Bits
	var reviewStatus int
	var uploaderID *int64
	err = s.db.QueryRow(ctx, `SELECT "PerceptualHash", "ReviewStatus", "UploaderId" FROM "Images" WHERE "Id" = $1`, id).
		Scan(&hash, &reviewStatus, &uploaderID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !canSeeContent(ident, reviewStatus, uploaderID)) {
		NotFound(w, r, fmt.Sprintf("Image with ID %d not found.", id))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	nsfwCond := "TRUE"
	switch isNsfw {
	case "False":
		nsfwCond = `NOT i."IsNsfw"`
	case "True":
		nsfwCond = `i."IsNsfw"`
	}
	compute := func(ctx context.Context) (*rankedIDs, int, error) {
		rows, err := s.db.Query(ctx, `
			SELECT i."Id" FROM "Images" i
			WHERE i."Id" <> $1 AND i."ReviewStatus" = 1 AND `+nsfwCond+`
				AND bit_count(i."PerceptualHash" # $2::bit(64)) <= $3
			ORDER BY bit_count(i."PerceptualHash" # $2::bit(64)) ASC, i."Id" DESC
			LIMIT $4`, id, hash, maxDistance, limit)
		if err != nil {
			return nil, 0, err
		}
		ids, err := collectIDs(rows)
		return &rankedIDs{ids: ids}, len(ids) + 1, err
	}
	var ranked *rankedIDs
	if reviewStatus == 1 && s.similar.enabled() {
		ranked, err = s.similar.load(ctx, fmt.Sprintf("%d|%s|%d|%d", id, isNsfw, maxDistance, limit), 0, compute)
	} else {
		ranked, _, err = compute(ctx)
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	items := []imageDto{}
	if len(ranked.ids) > 0 {
		rows, err := s.db.Query(ctx, `SELECT `+imageColumns+` FROM "Images" i WHERE i."Id" = ANY($1) AND i."ReviewStatus" = 1`, ranked.ids)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		found, err := collectImageRows(rows, false)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		ordered := orderByIDs(ranked.ids, found, func(row imageRow) int64 { return row.ID })
		items, err = s.hydrateImages(ctx, s.db, ordered, hydrateOptions{ident: ident, withUserData: true})
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
	}
	writePublicJSON(w, r, s.newPaginated(items, 1, max(len(items), 1), int64(len(items)), s.cfg.Image.MaxPageSize, s.cfg.Image.DefaultPageSize))
}

type atomLink struct {
	Rel    string `xml:"rel,attr,omitempty"`
	Type   string `xml:"type,attr,omitempty"`
	Href   string `xml:"href,attr"`
	Length int64  `xml:"length,attr,omitempty"`
}

type atomPerson struct {
	Name string `xml:"name"`
	URI  string `xml:"uri,omitempty"`
}

type atomCategory struct {
	Term  string `xml:"term,attr"`
	Label string `xml:"label,attr,omitempty"`
}

type atomText struct {
	Type string `xml:"type,attr"`
	Body string `xml:",chardata"`
}

type atomEntry struct {
	Title      string         `xml:"title"`
	ID         string         `xml:"id"`
	Updated    string         `xml:"updated"`
	Published  string         `xml:"published"`
	Links      []atomLink     `xml:"link"`
	Authors    []atomPerson   `xml:"author"`
	Categories []atomCategory `xml:"category"`
	Content    atomText       `xml:"content"`
}

type atomFeed struct {
	XMLName   xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
	Title     string      `xml:"title"`
	Subtitle  string      `xml:"subtitle,omitempty"`
	ID        string      `xml:"id"`
	Updated   string      `xml:"updated"`
	Links     []atomLink  `xml:"link"`
	Author    atomPerson  `xml:"author"`
	Generator string      `xml:"generator"`
	Entries   []atomEntry `xml:"entry"`
}

var feedFilterParams = []string{
	"isNsfw", "isAnimated", "orientation", "width", "height", "byteSize", "includedTags", "excludedTags",
	"includedArtists", "excludedArtists", "color", "colorTolerance", "uploadedAfter", "uploadedBefore",
}

func feedQuery(r *http.Request) url.Values {
	out := url.Values{}
	for _, key := range feedFilterParams {
		for k, values := range r.URL.Query() {
			if strings.EqualFold(k, key) {
				out[key] = append(out[key], values...)
			}
		}
	}
	return out
}

func contentTypeForExtension(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

func (s *Server) handleImageFeed(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
	f, ok := s.parseVisibleImageFilters(w, r, ident)
	if !ok {
		return
	}
	limit, err := intParam(r, "pageSize", 30, 1, 50)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	f.orderBy = "UploadedAt"
	opts := hydrateOptions{ident: ident, childrenReviewStatus: f.childrenReviewStatus, includeMyPendingChildren: f.includeMyPendingChildren}
	var items []imageDto
	served := false
	if s.imageLists.enabled() && cacheableImageFilter(f, ident) {
		items, _, served, err = s.listImagesFromCache(ctx, f, 1, limit, opts)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
	}
	if !served {
		rows, _, err := s.queryImagePage(ctx, f, ident, nil, 1, limit)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		if items, err = s.hydrateImages(ctx, s.db, rows, opts); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
	}

	frontend := strings.TrimRight(s.cfg.FrontendURL, "/")
	query := feedQuery(r)
	self := strings.TrimRight(s.cfg.APIBaseURL, "/") + r.URL.Path
	if encoded := query.Encode(); encoded != "" {
		self += "?" + encoded
	}
	gallery := frontend + "/gallery"
	if encoded := query.Encode(); encoded != "" {
		gallery += "?" + encoded
	}
	title := s.cfg.AppName + " – newest images"
	if len(f.includedTags) > 0 {
		title += " tagged " + strings.Join(f.includedTags, ", ")
	}
	updated := time.Now().UTC()
	if len(items) > 0 {
		updated = items[0].UploadedAt.UTC()
	}
	feed := atomFeed{
		Title:     title,
		Subtitle:  "The newest accepted images matching this filter.",
		ID:        self,
		Updated:   updated.Format(time.RFC3339),
		Links:     []atomLink{{Rel: "self", Type: "application/atom+xml", Href: self}, {Rel: "alternate", Type: "text/html", Href: gallery}},
		Author:    atomPerson{Name: s.cfg.AppName, URI: frontend},
		Generator: s.cfg.AppName,
		Entries:   make([]atomEntry, 0, len(items)),
	}
	for _, img := range items {
		page := fmt.Sprintf("%s/images/%d", frontend, img.ID)
		entry := atomEntry{
			Title:     fmt.Sprintf("Image #%d", img.ID),
			ID:        page,
			Updated:   img.UploadedAt.UTC().Format(time.RFC3339),
			Published: img.UploadedAt.UTC().Format(time.RFC3339),
			Links: []atomLink{
				{Rel: "alternate", Type: "text/html", Href: page},
				{Rel: "enclosure", Type: contentTypeForExtension(img.Extension), Href: img.URL, Length: img.ByteSize},
			},
		}
		names := make([]string, 0, len(img.Artists))
		for _, a := range img.Artists {
			entry.Authors = append(entry.Authors, atomPerson{Name: a.Name, URI: fmt.Sprintf("%s/artists/%d", frontend, a.ID)})
			names = append(names, a.Name)
		}
		if len(entry.Authors) == 0 {
			entry.Authors = []atomPerson{{Name: s.cfg.AppName}}
		} else {
			entry.Title += " by " + strings.Join(names, ", ")
		}
		tagNames := make([]string, 0, len(img.Tags))
		for _, t := range img.Tags {
			entry.Categories = append(entry.Categories, atomCategory{Term: t.Slug, Label: t.Name})
			tagNames = append(tagNames, t.Name)
		}
		var body strings.Builder
		fmt.Fprintf(&body, `<p><a href="%s"><img src="%s" alt="Image #%d" width="%d" height="%d"/></a></p>`,
			xmlAttr(page), xmlAttr(img.URL), img.ID, img.Width, img.Height)
		fmt.Fprintf(&body, `<p>%d × %d`, img.Width, img.Height)
		if img.IsNsfw {
			body.WriteString(` · NSFW`)
		}
		if len(tagNames) > 0 {
			body.WriteString(` · ` + xmlAttr(strings.Join(tagNames, ", ")))
		}
		body.WriteString(`</p>`)
		entry.Content = atomText{Type: "html", Body: body.String()}
		feed.Entries = append(feed.Entries, entry)
	}

	out, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	body := append([]byte(xml.Header), out...)
	h := w.Header()
	if ident == nil {
		etag := etagFor(body)
		h.Set("ETag", etag)
		h.Set("Cache-Control", "public, max-age=300")
		if inm := r.Header.Get("If-None-Match"); inm != "" && etagMatches(inm, etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	h.Set("Content-Type", "application/atom+xml; charset=utf-8")
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func xmlAttr(v string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(v))
	return b.String()
}
