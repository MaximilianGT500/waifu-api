package server

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"math/bits"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/waifu-im/waifu-api/backend/internal/auth"
	"github.com/waifu-im/waifu-api/backend/internal/imaging"
)

func (s *Server) RegisterImageRoutes() {
	s.Handle("GET /images", http.HandlerFunc(s.handleListImages))
	s.Handle("GET /images/{id}", http.HandlerFunc(s.handleGetImage))
	s.Handle("POST /images/upload", s.RequireAuth(http.HandlerFunc(s.handleUploadImage)))
	s.Handle("PATCH /images/{id}", s.RequireRole("Moderator")(http.HandlerFunc(s.handlePatchImage)))
	s.Handle("DELETE /images/{id}", s.RequireRole("Admin")(http.HandlerFunc(s.handleDeleteImage)))
}

func isModerator(ident *auth.Identity) bool {
	return ident != nil && ident.Role >= auth.RoleModerator
}

func isAdmin(ident *auth.Identity) bool {
	return ident != nil && ident.Role >= auth.RoleAdmin
}

func reverseByte(b byte) byte {
	var r byte
	for i := 0; i < 8; i++ {
		r = r<<1 | (b & 1)
		b >>= 1
	}
	return r
}

func reverseBytesPerByte(v uint64) uint64 {
	var out uint64
	for i := 0; i < 8; i++ {
		out = out<<8 | uint64(reverseByte(byte(v>>(8*i))))
	}
	return out
}

func bitsToHex(b pgtype.Bits) string {
	var v uint64
	for i := len(b.Bytes) - 1; i >= 0; i-- {
		v = v<<8 | uint64(b.Bytes[i])
	}
	return fmt.Sprintf("%016x", reverseBytesPerByte(v))
}

func hexToBits(hex string) pgtype.Bits {
	var v uint64
	_, _ = fmt.Sscanf(hex, "%x", &v)
	v = reverseBytesPerByte(v)
	bytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		bytes[i] = byte(v)
		v >>= 8
	}
	return pgtype.Bits{Bytes: bytes, Len: 64, Valid: true}
}

func storedHash(hash uint64) uint64 {
	v := reverseBytesPerByte(hash)
	var out uint64
	for i := 0; i < 8; i++ {
		out = out<<8 | uint64(byte(v>>(8*i)))
	}
	return out
}

func hammingDistance(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}

type imageFilter struct {
	isNsfw                   string
	isAnimated               string
	orderBy                  string
	orientation              string
	width, height, byteSize  string
	uploaderID               *int64
	reviewStatus             string
	includeMyPending         bool
	includeMyPendingChildren bool
	childrenReviewStatus     string
	includedTags             []string
	excludedTags             []string
	includedArtists          []string
	excludedArtists          []string
	includedIDs              []int64
	excludedIDs              []int64
}

type enumProblem struct {
	field string
	value string
}

func parseImageFilters(r *http.Request, ident *auth.Identity) (*imageFilter, []enumProblem) {
	q := r.URL.Query()
	f := &imageFilter{
		reviewStatus: "Accepted",
	}
	var problems []enumProblem

	if v := q.Get("isNsfw"); v != "" {
		if f.isNsfw, _ = enumValue(v, "False", "True", "All"); f.isNsfw == "" {
			problems = append(problems, enumProblem{"isNsfw", v})
		}
	} else {
		f.isNsfw = "False"
	}
	if v := q.Get("isAnimated"); v != "" {
		if f.isAnimated, _ = enumValue(v, "All", "True", "False"); f.isAnimated == "" {
			problems = append(problems, enumProblem{"isAnimated", v})
		}
	} else {
		f.isAnimated = "All"
	}
	if v := q.Get("orderBy"); v != "" {
		if f.orderBy, _ = enumValue(v, "Random", "UploadedAt", "Favorites", "AddedToAlbum"); f.orderBy == "" {
			problems = append(problems, enumProblem{"orderBy", v})
		}
	} else {
		f.orderBy = "Random"
	}
	if v := q.Get("orientation"); v != "" {
		if f.orientation, _ = enumValue(v, "All", "Landscape", "Portrait", "Square"); f.orientation == "" {
			problems = append(problems, enumProblem{"orientation", v})
		}
	} else {
		f.orientation = "All"
	}
	if v := q.Get("reviewStatus"); v != "" {
		if f.reviewStatus, _ = enumValue(v, "Accepted", "Pending", "All"); f.reviewStatus == "" {
			problems = append(problems, enumProblem{"reviewStatus", v})
		}
	}
	if v := q.Get("childrenReviewStatus"); v != "" {
		if f.childrenReviewStatus, _ = enumValue(v, "Accepted", "Pending", "All"); f.childrenReviewStatus == "" {
			problems = append(problems, enumProblem{"childrenReviewStatus", v})
		}
	} else {
		f.childrenReviewStatus = "Accepted"
	}

	f.includeMyPending = queryBool(r, "includeMyPending", false)
	f.includeMyPendingChildren = queryBool(r, "includeMyPendingChildren", false)

	f.width = q.Get("width")
	f.height = q.Get("height")
	f.byteSize = q.Get("byteSize")

	for _, t := range repeatedQuery(r, "includedTags") {
		f.includedTags = append(f.includedTags, strings.ToLower(strings.TrimSpace(t)))
	}
	for _, t := range repeatedQuery(r, "excludedTags") {
		f.excludedTags = append(f.excludedTags, strings.ToLower(strings.TrimSpace(t)))
	}
	for _, a := range repeatedQuery(r, "includedArtists") {
		f.includedArtists = append(f.includedArtists, strings.ToLower(strings.TrimSpace(a)))
	}
	for _, a := range repeatedQuery(r, "excludedArtists") {
		f.excludedArtists = append(f.excludedArtists, strings.ToLower(strings.TrimSpace(a)))
	}
	for _, v := range repeatedQuery(r, "includedIds") {
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			f.includedIDs = append(f.includedIDs, n)
		}
	}
	for _, v := range repeatedQuery(r, "excludedIds") {
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			f.excludedIDs = append(f.excludedIDs, n)
		}
	}

	if up := q.Get("uploaderId"); up != "" {
		if isModerator(ident) {
			if n, err := strconv.ParseInt(up, 10, 64); err == nil {
				f.uploaderID = &n
			}
		}

	}

	return f, problems
}

func buildImageWhere(f *imageFilter, ident *auth.Identity, albumID *int64, argOffset int) (string, []any) {
	var conds []string
	args := []any{}
	ph := func() string { return fmt.Sprintf("$%d", len(args)+1+argOffset) }
	add := func(cond string, a ...any) {
		conds = append(conds, cond)
		args = append(args, a...)
	}

	if albumID != nil {

		switch f.reviewStatus {
		case "Pending":
			add(`i."ReviewStatus" = 0`)
		case "Accepted":
			add(`i."ReviewStatus" = 1`)
		}
	} else {
		switch f.reviewStatus {
		case "All":

		case "Pending":
			add(`i."ReviewStatus" = 0`)
		default:
			if f.includeMyPending && ident != nil {
				add(fmt.Sprintf(`(i."ReviewStatus" = 1 OR (i."ReviewStatus" = 0 AND i."UploaderId" = %s))`, ph()), ident.UserID)
			} else {
				add(`i."ReviewStatus" = 1`)
			}
		}
	}

	switch f.isNsfw {
	case "True":
		add(`i."IsNsfw"`)
	case "False":
		add(`NOT i."IsNsfw"`)
	}
	switch f.isAnimated {
	case "True":
		add(`i."IsAnimated"`)
	case "False":
		add(`NOT i."IsAnimated"`)
	}
	switch f.orientation {
	case "Landscape":
		add(`i."Width" > i."Height"`)
	case "Portrait":
		add(`i."Width" < i."Height"`)
	case "Square":
		add(`i."Width" = i."Height"`)
	}

	for _, dim := range []struct {
		key, col string
	}{{f.width, `i."Width"`}, {f.height, `i."Height"`}, {f.byteSize, `i."ByteSize"`}} {
		if dim.key == "" {
			continue
		}
		if c, a, ok := dimensionCondition(dim.col, dim.key, ph); ok {
			conds = append(conds, c)
			args = append(args, a...)
		}
	}

	if albumID == nil && f.uploaderID != nil {
		add(fmt.Sprintf(`i."UploaderId" = %s`, ph()), *f.uploaderID)
	}
	for _, id := range f.includedIDs {

		add(fmt.Sprintf(`i."Id" = %s`, ph()), id)
	}
	if len(f.excludedIDs) > 0 {
		add(fmt.Sprintf(`i."Id" <> ALL(%s)`, ph()), f.excludedIDs)
	}
	for _, t := range f.includedTags {
		if n, err := strconv.ParseInt(t, 10, 64); err == nil {
			add(fmt.Sprintf(`EXISTS (SELECT 1 FROM "ImageTag" it JOIN "Tags" tg ON tg."Id" = it."TagsId" WHERE it."ImagesId" = i."Id" AND tg."Id" = %s)`, ph()), n)
		} else {
			add(fmt.Sprintf(`EXISTS (SELECT 1 FROM "ImageTag" it JOIN "Tags" tg ON tg."Id" = it."TagsId" WHERE it."ImagesId" = i."Id" AND tg."Slug" = %s)`, ph()), t)
		}
	}
	for _, t := range f.excludedTags {
		if n, err := strconv.ParseInt(t, 10, 64); err == nil {
			add(fmt.Sprintf(`NOT EXISTS (SELECT 1 FROM "ImageTag" it JOIN "Tags" tg ON tg."Id" = it."TagsId" WHERE it."ImagesId" = i."Id" AND tg."Id" = %s)`, ph()), n)
		} else {
			add(fmt.Sprintf(`NOT EXISTS (SELECT 1 FROM "ImageTag" it JOIN "Tags" tg ON tg."Id" = it."TagsId" WHERE it."ImagesId" = i."Id" AND tg."Slug" = %s)`, ph()), t)
		}
	}
	for _, a := range f.includedArtists {
		if n, err := strconv.ParseInt(a, 10, 64); err == nil {
			add(fmt.Sprintf(`EXISTS (SELECT 1 FROM "ArtistImage" ai JOIN "Artists" ar ON ar."Id" = ai."ArtistsId" WHERE ai."ImagesId" = i."Id" AND ar."Id" = %s)`, ph()), n)
		} else {
			add(fmt.Sprintf(`EXISTS (SELECT 1 FROM "ArtistImage" ai JOIN "Artists" ar ON ar."Id" = ai."ArtistsId" WHERE ai."ImagesId" = i."Id" AND lower(ar."Name") = %s)`, ph()), a)
		}
	}
	for _, a := range f.excludedArtists {
		if n, err := strconv.ParseInt(a, 10, 64); err == nil {
			add(fmt.Sprintf(`NOT EXISTS (SELECT 1 FROM "ArtistImage" ai JOIN "Artists" ar ON ar."Id" = ai."ArtistsId" WHERE ai."ImagesId" = i."Id" AND ar."Id" = %s)`, ph()), n)
		} else {
			add(fmt.Sprintf(`NOT EXISTS (SELECT 1 FROM "ArtistImage" ai JOIN "Artists" ar ON ar."Id" = ai."ArtistsId" WHERE ai."ImagesId" = i."Id" AND lower(ar."Name") = %s)`, ph()), a)
		}
	}

	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

func dimensionCondition(col, v string, ph func() string) (string, []any, bool) {
	parse := func(s string) (int64, bool) {
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		return n, err == nil
	}
	switch {
	case strings.HasPrefix(v, ">="):
		if n, ok := parse(v[2:]); ok {
			return fmt.Sprintf("%s >= %s", col, ph()), []any{n}, true
		}
	case strings.HasPrefix(v, ">"):
		if n, ok := parse(v[1:]); ok {
			return fmt.Sprintf("%s > %s", col, ph()), []any{n}, true
		}
	case strings.HasPrefix(v, "<="):
		if n, ok := parse(v[2:]); ok {
			return fmt.Sprintf("%s <= %s", col, ph()), []any{n}, true
		}
	case strings.HasPrefix(v, "<"):
		if n, ok := parse(v[1:]); ok {
			return fmt.Sprintf("%s < %s", col, ph()), []any{n}, true
		}
	case strings.Contains(v, ".."):
		parts := strings.SplitN(v, "..", 2)
		if lo, ok1 := parse(parts[0]); ok1 {
			if hi, ok2 := parse(parts[1]); ok2 {
				return fmt.Sprintf("%s BETWEEN %s AND %s", col, ph(), ph()), []any{lo, hi}, true
			}
		}
	default:
		if n, ok := parse(v); ok {
			return fmt.Sprintf("%s = %s", col, ph()), []any{n}, true
		}
	}
	return "", nil, false
}

func orderClause(f *imageFilter, albumID *int64) (string, error) {
	switch f.orderBy {
	case "Random":
		return " ORDER BY random()", nil
	case "UploadedAt":
		return ` ORDER BY i."UploadedAt" DESC`, nil
	case "Favorites":
		return ` ORDER BY (SELECT COUNT(*) FROM "AlbumItems" ai JOIN "Albums" al ON al."Id" = ai."AlbumId" AND al."IsDefault" WHERE ai."ImageId" = i."Id") DESC`, nil
	case "AddedToAlbum":
		if albumID == nil {
			return "", errors.New("AddedToAlbum ordering is only available when viewing an album.")
		}
		return ` ORDER BY ai."AddedAt" DESC`, nil
	default:
		return " ORDER BY random()", nil
	}
}

type imageRow struct {
	ID             int64
	PerceptualHash pgtype.Bits
	Extension      string
	DominantColor  string
	Source         *string
	UploaderID     *int64
	UploadedAt     time.Time
	IsNsfw         bool
	IsAnimated     bool
	Width          int64
	Height         int64
	ByteSize       int64
	ReviewStatus   int
	AddedAt        *time.Time
}

func (s *Server) handleListImages(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	f, problems := parseImageFilters(r, ident)
	if len(problems) > 0 {
		invalidEnumProblem(w, r, problems[0].field, problems[0].value)
		return
	}

	if f.reviewStatus != "Accepted" && !isModerator(ident) {
		Forbidden(w, r, "Filtering by non-accepted review status is only available to moderators and admins.")
		return
	}
	if f.childrenReviewStatus != "Accepted" && !isModerator(ident) {
		Forbidden(w, r, "Filtering by non-accepted review status is only available to moderators and admins.")
		return
	}

	page, pageSize := paginationParams(r, s.cfg.Image.DefaultPageSize, s.cfg.Image.MaxPageSize)
	where, args := buildImageWhere(f, ident, nil, 0)
	order, err := orderClause(f, nil)
	if err != nil {
		BadRequest(w, r, err.Error())
		return
	}

	var total int64
	if err := s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM "Images" i`+where, args...).Scan(&total); err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	query := `SELECT i."Id", i."PerceptualHash", i."Extension", i."DominantColor", i."Source",
		i."UploaderId", i."UploadedAt", i."IsNsfw", i."IsAnimated", i."Width", i."Height",
		i."ByteSize", i."ReviewStatus"
		FROM "Images" i` + where + order
	query += fmt.Sprintf(" LIMIT %d OFFSET %d", pageSize, (page-1)*pageSize)

	rows, err := s.db.Query(r.Context(), query, args...)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	defer rows.Close()

	items := []imageDto{}
	for rows.Next() {
		var row imageRow
		if err := rows.Scan(&row.ID, &row.PerceptualHash, &row.Extension, &row.DominantColor, &row.Source,
			&row.UploaderID, &row.UploadedAt, &row.IsNsfw, &row.IsAnimated, &row.Width, &row.Height,
			&row.ByteSize, &row.ReviewStatus); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		dto, err := s.assembleImageDto(r.Context(), row, ident, f.childrenReviewStatus, f.includeMyPendingChildren, nil, false)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		items = append(items, *dto)
	}

	writeJSON(w, http.StatusOK, newPaginated(items, page, pageSize, total, s.cfg.Image.MaxPageSize, s.cfg.Image.DefaultPageSize))
}

func (s *Server) assembleImageDto(ctx context.Context, row imageRow, ident *auth.Identity, childrenReviewStatus string, includeMyPendingChildren bool, albumID *int64, forceUploader bool) (*imageDto, error) {
	dto := &imageDto{
		ID:             row.ID,
		PerceptualHash: bitsToHex(row.PerceptualHash),
		Extension:      row.Extension,
		DominantColor:  row.DominantColor,
		Source:         row.Source,
		UploaderID:     nil,
		UploadedAt:     row.UploadedAt,
		IsNsfw:         row.IsNsfw,
		IsAnimated:     row.IsAnimated,
		Width:          row.Width,
		Height:         row.Height,
		ByteSize:       row.ByteSize,
		URL:            s.s3.PublicURL(fmt.Sprintf("%d%s", row.ID, row.Extension)),
		ReviewStatus:   reviewStatusString(row.ReviewStatus),
		Artists:        []artistDto{},
		Tags:           []tagDto{},
		Albums:         []imageAlbumRef{},
	}
	if isModerator(ident) || forceUploader {
		dto.UploaderID = row.UploaderID
	}

	childCondFor := func(alias string) string {
		switch {
		case childrenReviewStatus == "Pending":
			return fmt.Sprintf(`%s."ReviewStatus" = 0`, alias)
		case childrenReviewStatus == "All":
			return `TRUE`
		case includeMyPendingChildren && ident != nil:
			return fmt.Sprintf(`(%s."ReviewStatus" = 1 OR (%s."ReviewStatus" = 0 AND %s."CreatorId" = %d))`, alias, alias, alias, ident.UserID)
		default:
			return fmt.Sprintf(`%s."ReviewStatus" = 1`, alias)
		}
	}

	tagRows, err := s.db.Query(ctx, fmt.Sprintf(`
		SELECT tg."Id", tg."Name", tg."Slug", tg."Description", tg."ReviewStatus"
		FROM "ImageTag" it JOIN "Tags" tg ON tg."Id" = it."TagsId"
		WHERE it."ImagesId" = $1 AND %s`, childCondFor("tg")), row.ID)
	if err != nil {
		return nil, err
	}
	defer tagRows.Close()
	for tagRows.Next() {
		var t tagDto
		var rs int
		if err := tagRows.Scan(&t.ID, &t.Name, &t.Slug, &t.Description, &rs); err != nil {
			return nil, err
		}
		t.ReviewStatus = reviewStatusString(rs)
		dto.Tags = append(dto.Tags, t)
	}

	artistRows, err := s.db.Query(ctx, fmt.Sprintf(`
		SELECT ar."Id", ar."Name", ar."Patreon", ar."Pixiv", ar."Twitter", ar."DeviantArt", ar."ReviewStatus"
		FROM "ArtistImage" ai JOIN "Artists" ar ON ar."Id" = ai."ArtistsId"
		WHERE ai."ImagesId" = $1 AND %s`, childCondFor("ar")), row.ID)
	if err != nil {
		return nil, err
	}
	defer artistRows.Close()
	for artistRows.Next() {
		var a artistDto
		var rs int
		var patreon, pixiv, twitter, deviantArt *string
		if err := artistRows.Scan(&a.ID, &a.Name, &patreon, &pixiv, &twitter, &deviantArt, &rs); err != nil {
			return nil, err
		}
		a.ReviewStatus = reviewStatusString(rs)
		a.Patreon = orEmpty(patreon)
		a.Pixiv = orEmpty(pixiv)
		a.Twitter = orEmpty(twitter)
		a.DeviantArt = orEmpty(deviantArt)
		dto.Artists = append(dto.Artists, a)
	}

	if err := s.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM "AlbumItems" ai
		JOIN "Albums" al ON al."Id" = ai."AlbumId" AND al."IsDefault"
		WHERE ai."ImageId" = $1`, row.ID).Scan(&dto.Favorites); err != nil {
		return nil, err
	}

	if ident != nil {

		var likedAt *time.Time
		if err := s.db.QueryRow(ctx, `
			SELECT ai."AddedAt" FROM "AlbumItems" ai
			JOIN "Albums" al ON al."Id" = ai."AlbumId" AND al."IsDefault"
			WHERE ai."ImageId" = $1 AND al."UserId" = $2`, row.ID, ident.UserID).Scan(&likedAt); err == nil {
			dto.LikedAt = likedAt
		}
		albRows, err := s.db.Query(ctx, `
			SELECT al."Id", al."Name", al."UserId", al."IsDefault"
			FROM "AlbumItems" ai JOIN "Albums" al ON al."Id" = ai."AlbumId"
			WHERE ai."ImageId" = $1 AND al."UserId" = $2`, row.ID, ident.UserID)
		if err != nil {
			return nil, err
		}
		defer albRows.Close()
		for albRows.Next() {
			var ref imageAlbumRef
			if err := albRows.Scan(&ref.ID, &ref.Name, &ref.UserID, &ref.IsDefault); err != nil {
				return nil, err
			}
			dto.Albums = append(dto.Albums, ref)
		}
	}

	if row.AddedAt != nil {
		dto.AddedToAlbumAt = row.AddedAt
	}
	return dto, nil
}

func orEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (s *Server) handleGetImage(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	ident := IdentityFromContext(r.Context())
	childrenReviewStatus := "Accepted"
	if v := r.URL.Query().Get("childrenReviewStatus"); v != "" {
		var ok bool
		if childrenReviewStatus, ok = enumValue(v, "Accepted", "Pending", "All"); !ok {
			invalidEnumProblem(w, r, "childrenReviewStatus", v)
			return
		}
	}
	if childrenReviewStatus != "Accepted" && !isModerator(ident) {
		Forbidden(w, r, "Filtering by non-accepted review status is only available to moderators and admins.")
		return
	}
	includeMyPendingChildren := queryBool(r, "includeMyPendingChildren", false)

	var row imageRow
	err = s.db.QueryRow(r.Context(), `
		SELECT i."Id", i."PerceptualHash", i."Extension", i."DominantColor", i."Source",
		i."UploaderId", i."UploadedAt", i."IsNsfw", i."IsAnimated", i."Width", i."Height",
		i."ByteSize", i."ReviewStatus"
		FROM "Images" i WHERE i."Id" = $1`, id).
		Scan(&row.ID, &row.PerceptualHash, &row.Extension, &row.DominantColor, &row.Source,
			&row.UploaderID, &row.UploadedAt, &row.IsNsfw, &row.IsAnimated, &row.Width, &row.Height,
			&row.ByteSize, &row.ReviewStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, fmt.Sprintf("Image with ID %d not found.", id))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	dto, err := s.assembleImageDto(r.Context(), row, ident, childrenReviewStatus, includeMyPendingChildren, nil, false)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) handleUploadImage(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	if !requireMinRole(ident, s.cfg.Permissions.ImageUploadMinRole) {
		s.permissionRoleForbidden(w, r, s.cfg.Permissions.ImageUploadMinRole, auth.RoleUser)
		return
	}

	maxBytes := int64(s.cfg.MaxUploadMB) * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		WriteProblem(w, r, http.StatusRequestEntityTooLarge,
			"https://tools.ietf.org/html/rfc9110#section-15.5.14",
			"Request Entity Too Large",
			fmt.Sprintf("The request body exceeds the maximum size of %d MB.", s.cfg.MaxUploadMB))
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("file")
	if err != nil {
		WriteValidationProblem(w, r, map[string][]string{"File": {"The File field is required."}})
		return
	}
	defer file.Close()

	if strings.TrimSpace(header.Filename) == "" {
		WriteValidationProblem(w, r, map[string][]string{"FileName": {"File name is required."}})
		return
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		WriteValidationProblem(w, r, map[string][]string{"ContentType": {"Content type is required."}})
		return
	}
	switch strings.ToLower(contentType) {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
	default:
		WriteValidationProblem(w, r, map[string][]string{"ContentType": {"Only JPEG, PNG, GIF, and WEBP formats are supported."}})
		return
	}

	info, img, err := imaging.Decode(file)
	if err != nil {
		BadRequest(w, r, "The uploaded file is not a valid image.")
		return
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))

	if err := imaging.ValidateDimensions(info,
		s.cfg.Image.MinWidth, s.cfg.Image.MinHeight, s.cfg.Image.MaxWidth, s.cfg.Image.MaxHeight,
		s.cfg.Image.AnimatedMinWidth, s.cfg.Image.AnimatedMinHeight, s.cfg.Image.AnimatedMaxWidth, s.cfg.Image.AnimatedMaxHeight); err != nil {
		InvalidOperation(w, r, err.Error())
		return
	}

	source := strings.TrimSpace(r.FormValue("source"))
	if len(source) > 500 {
		WriteValidationProblem(w, r, map[string][]string{"Source": {"Source URL must not exceed 500 characters."}})
		return
	}
	if source != "" && !isAbsoluteURL(source) {
		WriteValidationProblem(w, r, map[string][]string{"Source": {"Source must be a valid URL."}})
		return
	}

	isNsfw := false
	if v := r.FormValue("isNsfw"); v != "" {
		isNsfw = strings.EqualFold(v, "true")
	}

	dominant := imaging.DominantColor(img)
	hash := imaging.PerceptualHash(img, info.Orientation)
	hashHex := fmt.Sprintf("%016x", hash)

	if dupID, err := s.findDuplicate(r.Context(), hash, 0); err == nil && dupID > 0 {
		Conflict(w, r, fmt.Sprintf("Duplicate image found. ID: %d", dupID))
		return
	}

	artistIDs := r.MultipartForm.Value["artists"]
	tagSlugs := r.MultipartForm.Value["tags"]

	artists, err := s.resolveArtistsByIDs(r.Context(), artistIDs)
	if err != nil {
		NotFound(w, r, err.Error())
		return
	}
	tags, err := s.resolveTagsBySlugs(r.Context(), tagSlugs)
	if err != nil {
		NotFound(w, r, err.Error())
		return
	}

	reviewStatus := 0
	requireReview := s.cfg.Review.RequireImageReview
	if !requireReview || auth.ReviewBypassed(ident.Role, s.cfg.Review.ImageReviewBypassMinRole) {
		reviewStatus = 1
	}

	byteSize := header.Size

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	defer tx.Rollback(r.Context())

	var imageID int64
	err = tx.QueryRow(r.Context(), `
		INSERT INTO "Images" ("PerceptualHash","Extension","DominantColor","Source","UploaderId",
			"UploadedAt","IsNsfw","IsAnimated","Width","Height","ByteSize","ReviewStatus")
		VALUES ($1,$2,$3,$4,$5,now(),$6,$7,$8,$9,$10,$11)
		RETURNING "Id"`,
		hexToBits(hashHex), ext, dominant, orEmptyString(source), ident.UserID,
		isNsfw, info.IsAnimated, info.Width, info.Height, byteSize, reviewStatus).Scan(&imageID)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	now := time.Now().UTC()
	if reviewStatus == 1 {
		_, err = tx.Exec(r.Context(), `
			INSERT INTO "ReviewTasks" ("Kind","TargetType","TargetId","SubmitterId","Status","ReviewerNote","CreatedAt","ResolvedAt")
			VALUES (0,0,$1,$2,1,'Auto-accepted (trusted role).',$3,$3)`, imageID, ident.UserID, now)
	} else {
		_, err = tx.Exec(r.Context(), `
			INSERT INTO "ReviewTasks" ("Kind","TargetType","TargetId","SubmitterId","Status","CreatedAt")
			VALUES (0,0,$1,$2,0,$3)`, imageID, ident.UserID, now)
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	for _, t := range tags {
		if _, err := tx.Exec(r.Context(), `INSERT INTO "ImageTag" ("ImagesId","TagsId") VALUES ($1,$2)`, imageID, t.ID); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
	}
	for _, a := range artists {
		if _, err := tx.Exec(r.Context(), `INSERT INTO "ArtistImage" ("ArtistsId","ImagesId") VALUES ($1,$2)`, a.ID, imageID); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	key := fmt.Sprintf("%d%s", imageID, ext)
	if err := s.s3.Upload(r.Context(), key, file, header.Size, contentType); err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	var row imageRow
	err = s.db.QueryRow(r.Context(), `
		SELECT "Id","PerceptualHash","Extension","DominantColor","Source","UploaderId","UploadedAt",
			"IsNsfw","IsAnimated","Width","Height","ByteSize","ReviewStatus"
		FROM "Images" WHERE "Id" = $1`, imageID).
		Scan(&row.ID, &row.PerceptualHash, &row.Extension, &row.DominantColor, &row.Source, &row.UploaderID,
			&row.UploadedAt, &row.IsNsfw, &row.IsAnimated, &row.Width, &row.Height, &row.ByteSize, &row.ReviewStatus)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	dto, err := s.assembleImageDto(r.Context(), row, ident, "All", false, nil, true)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	dto.Favorites = 0
	dto.LikedAt = nil
	w.Header().Set("Location", fmt.Sprintf("/images/%d", imageID))
	writeJSON(w, http.StatusCreated, dto)
}

func (s *Server) findDuplicate(ctx context.Context, hash uint64, excludeID int64) (int64, error) {
	wire := storedHash(hash)
	rows, err := s.db.Query(ctx, `SELECT "Id", "PerceptualHash" FROM "Images" WHERE "Id" <> $1`, excludeID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var b pgtype.Bits
		if err := rows.Scan(&id, &b); err != nil {
			return 0, err
		}
		var other uint64
		for _, by := range b.Bytes {
			other = other<<8 | uint64(by)
		}
		if hammingDistance(wire, other) <= 4 {
			return id, nil
		}
	}
	return 0, nil
}

func orEmptyString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func isAbsoluteURL(s string) bool {
	i := strings.Index(s, ":")
	if i <= 0 {
		return false
	}
	scheme := s[:i]
	if scheme == "" {
		return false
	}
	for _, c := range scheme {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

func (s *Server) handlePatchImage(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}

	var exists bool
	if err := s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM "Images" WHERE "Id" = $1)`, id).Scan(&exists); err != nil || !exists {
		NotFound(w, r, fmt.Sprintf("Image with ID %d not found.", id))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, int64(s.cfg.MaxUploadMB)*1024*1024+(1<<20))
	if err := r.ParseMultipartForm(int64(s.cfg.MaxUploadMB) * 1024 * 1024); err != nil {
		BadRequest(w, r, "Invalid multipart form.")
		return
	}
	defer r.MultipartForm.RemoveAll()

	isNsfw := false
	if v := r.FormValue("isNsfw"); v != "" {
		isNsfw = strings.EqualFold(v, "true")
	}

	reviewStatus := ""
	if v := r.FormValue("reviewStatus"); v != "" {
		var ok bool
		if reviewStatus, ok = enumValue(v, "Pending", "Accepted"); !ok {
			invalidEnumProblem(w, r, "reviewStatus", v)
			return
		}
	}

	var (
		newInfo        *imaging.Info
		newImg         image.Image
		newHashHex     string
		newExt         string
		newContentType string
		newByteSize    int64
		replacedFile   bool
		replFile       interface {
			io.Reader
			io.Seeker
			io.Closer
		}
	)
	if file, header, ferr := r.FormFile("file"); ferr == nil {
		replFile = file
		info, img, decErr := imaging.Decode(file)
		if decErr != nil {
			BadRequest(w, r, "The uploaded file is not a valid image.")
			return
		}
		if err := imaging.ValidateDimensions(info,
			s.cfg.Image.MinWidth, s.cfg.Image.MinHeight, s.cfg.Image.MaxWidth, s.cfg.Image.MaxHeight,
			s.cfg.Image.AnimatedMinWidth, s.cfg.Image.AnimatedMinHeight, s.cfg.Image.AnimatedMaxWidth, s.cfg.Image.AnimatedMaxHeight); err != nil {
			InvalidOperation(w, r, err.Error())
			return
		}
		hash := imaging.PerceptualHash(img, info.Orientation)
		if dupID, derr := s.findDuplicate(r.Context(), hash, id); derr == nil && dupID > 0 {
			Conflict(w, r, fmt.Sprintf("Duplicate image found. ID: %d", dupID))
			return
		}
		newInfo = info
		newImg = img
		newHashHex = fmt.Sprintf("%016x", hash)
		newExt = strings.ToLower(filepath.Ext(header.Filename))
		newContentType = header.Header.Get("Content-Type")
		newByteSize = header.Size
		replacedFile = true
	}
	if replFile != nil {
		defer replFile.Close()
	}

	source := orEmptyString(strings.TrimSpace(r.FormValue("source")))

	var newUploader *int64
	if v := r.FormValue("userId"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			var uExists bool
			if err := s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM "Users" WHERE "Id" = $1)`, n).Scan(&uExists); err != nil || !uExists {
				NotFound(w, r, fmt.Sprintf("User with ID %d not found.", n))
				return
			}
			newUploader = &n
		}

	}

	var newTags []tagDto
	var hasTags bool
	if vals, ok := r.MultipartForm.Value["tags"]; ok {
		hasTags = true
		trimmed := make([]string, 0, len(vals))
		for _, v := range vals {
			trimmed = append(trimmed, strings.TrimSpace(v))
		}
		if len(trimmed) > 0 {
			tags, terr := s.resolveTagsBySlugs(r.Context(), trimmed)
			if terr != nil {
				NotFound(w, r, terr.Error())
				return
			}
			newTags = tags
		}
	}
	var newArtists []artistDto
	var hasArtists bool
	if vals, ok := r.MultipartForm.Value["artists"]; ok {
		hasArtists = true
		rawIDs := make([]string, 0, len(vals))
		for _, v := range vals {
			rawIDs = append(rawIDs, strings.TrimSpace(v))
		}
		if len(rawIDs) > 0 {
			artists, aerr := s.resolveArtistsByIDs(r.Context(), rawIDs)
			if aerr != nil {
				NotFound(w, r, aerr.Error())
				return
			}
			newArtists = artists
		}
	}

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	defer tx.Rollback(r.Context())

	if _, err := tx.Exec(r.Context(), `UPDATE "Images" SET "Source" = $1, "IsNsfw" = $2, "UploaderId" = $3 WHERE "Id" = $4`,
		source, isNsfw, newUploader, id); err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	if hasTags {
		if _, err := tx.Exec(r.Context(), `DELETE FROM "ImageTag" WHERE "ImagesId" = $1`, id); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		for _, t := range newTags {
			if _, err := tx.Exec(r.Context(), `INSERT INTO "ImageTag" ("ImagesId","TagsId") VALUES ($1,$2)`, id, t.ID); err != nil {
				InternalError(w, r, s.log, err)
				return
			}
		}
	}
	if hasArtists {
		if _, err := tx.Exec(r.Context(), `DELETE FROM "ArtistImage" WHERE "ImagesId" = $1`, id); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		for _, a := range newArtists {
			if _, err := tx.Exec(r.Context(), `INSERT INTO "ArtistImage" ("ArtistsId","ImagesId") VALUES ($1,$2)`, a.ID, id); err != nil {
				InternalError(w, r, s.log, err)
				return
			}
		}
	}

	if reviewStatus == "Accepted" {
		if _, err := tx.Exec(r.Context(), `UPDATE "Images" SET "ReviewStatus" = 1 WHERE "Id" = $1`, id); err != nil {
			InternalError(w, r, s.log, err)
			return
		}

		if _, err := tx.Exec(r.Context(), `
			UPDATE "ReviewTasks" SET "Status" = 1, "ResolvedAt" = now()
			WHERE "TargetType" = 0 AND "TargetId" = $1 AND "Kind" = 0 AND "Status" = 0`, id); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
	} else if reviewStatus == "Pending" {
		if _, err := tx.Exec(r.Context(), `UPDATE "Images" SET "ReviewStatus" = 0 WHERE "Id" = $1`, id); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
	}

	var oldExt string
	if replacedFile {
		if err := tx.QueryRow(r.Context(), `SELECT "Extension" FROM "Images" WHERE "Id" = $1`, id).Scan(&oldExt); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		dominant := imaging.DominantColor(newImg)
		if _, err := tx.Exec(r.Context(), `
			UPDATE "Images" SET "Extension" = $1, "PerceptualHash" = $2, "DominantColor" = $3,
				"IsAnimated" = $4, "Width" = $5, "Height" = $6, "ByteSize" = $7
			WHERE "Id" = $8`,
			newExt, hexToBits(newHashHex), dominant, newInfo.IsAnimated,
			newInfo.Width, newInfo.Height, newByteSize, id); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	if replacedFile {
		if _, err := replFile.Seek(0, io.SeekStart); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		if err := s.s3.Upload(r.Context(), fmt.Sprintf("%d%s", id, newExt), replFile, newByteSize, newContentType); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		if oldExt != "" && oldExt != newExt {
			_ = s.s3.Delete(r.Context(), fmt.Sprintf("%d%s", id, oldExt))
		}
		s.purgeCDN([]string{fmt.Sprintf("%d%s", id, newExt), fmt.Sprintf("%d%s", id, oldExt)})
	}

	var row imageRow
	err = s.db.QueryRow(r.Context(), `
		SELECT "Id","PerceptualHash","Extension","DominantColor","Source","UploaderId","UploadedAt",
			"IsNsfw","IsAnimated","Width","Height","ByteSize","ReviewStatus"
		FROM "Images" WHERE "Id" = $1`, id).
		Scan(&row.ID, &row.PerceptualHash, &row.Extension, &row.DominantColor, &row.Source, &row.UploaderID,
			&row.UploadedAt, &row.IsNsfw, &row.IsAnimated, &row.Width, &row.Height, &row.ByteSize, &row.ReviewStatus)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	dto, err := s.assembleImageDto(r.Context(), row, ident, "All", false, nil, true)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	dto.Favorites = 0
	dto.LikedAt = nil
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) handleDeleteImage(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}

	var ext string
	err = s.db.QueryRow(r.Context(), `SELECT "Extension" FROM "Images" WHERE "Id" = $1`, id).Scan(&ext)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, fmt.Sprintf("Image with ID %d not found.", id))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	_, _ = s.db.Exec(r.Context(), `
		UPDATE "Reports" SET "Status" = 1, "ReviewerNote" = 'The reported image was deleted by a moderator.'
		WHERE "ImageId" = $1 AND "Status" = 0`, id)

	if err := s.s3.Delete(r.Context(), fmt.Sprintf("%d%s", id, ext)); err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	_, err = s.db.Exec(r.Context(), `DELETE FROM "Images" WHERE "Id" = $1`, id)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	s.purgeCDN([]string{fmt.Sprintf("%d%s", id, ext)})
	w.WriteHeader(http.StatusNoContent)
}
