package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/waifu-im/waifu-api/backend/internal/auth"
	"github.com/waifu-im/waifu-api/backend/internal/imaging"
)

func (s *Server) RegisterImageRoutes() {
	s.Handle("GET /images", http.HandlerFunc(s.handleListImages))
	s.Handle("GET /images/{id}", http.HandlerFunc(s.handleGetImage))
	s.Handle("POST /images/upload", s.RequireAuth(s.withLimit(s.uploadLimiter, http.HandlerFunc(s.handleUploadImage))))
	s.Handle("PATCH /images/{id}", s.RequireRole("Moderator")(http.HandlerFunc(s.handlePatchImage)))
	s.Handle("DELETE /images/{id}", s.RequireRole("Admin")(http.HandlerFunc(s.handleDeleteImage)))
}

const duplicateDistance = 4

func hammingDistance(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}

type sqlArgs struct {
	args   []any
	offset int
}

func (a *sqlArgs) add(v any) string {
	a.args = append(a.args, v)
	return "$" + strconv.Itoa(len(a.args)+a.offset)
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
	color                    *rgbColor
	colorTolerance           int
	uploadedAfter            *time.Time
	uploadedBefore           *time.Time
	mine                     bool
}

func (s *Server) parseImageFilters(r *http.Request, ident *auth.Identity) (*imageFilter, error) {
	f := &imageFilter{reviewStatus: "Accepted"}

	enum := func(key, def string, allowed ...string) (string, error) {
		v := queryGet(r, key)
		if v == "" {
			return def, nil
		}
		val, ok := enumValue(v, allowed...)
		if !ok {
			if runeLen(v) > 64 {
				v = string([]rune(v)[:64]) + "…"
			}
			return "", errFieldValidation(key, fmt.Sprintf("The value '%s' is not valid for %s.", v, key))
		}
		return val, nil
	}
	var err error
	if f.isNsfw, err = enum("isNsfw", "False", "False", "True", "All"); err != nil {
		return nil, err
	}
	if f.isAnimated, err = enum("isAnimated", "All", "All", "True", "False"); err != nil {
		return nil, err
	}
	if f.orderBy, err = enum("orderBy", "Random", "Random", "UploadedAt", "Favorites", "Trending", "AddedToAlbum"); err != nil {
		return nil, err
	}
	if f.orientation, err = enum("orientation", "All", "All", "Landscape", "Portrait", "Square"); err != nil {
		return nil, err
	}
	if f.reviewStatus, err = enum("reviewStatus", "Accepted", "Accepted", "Pending", "All"); err != nil {
		return nil, err
	}
	if f.childrenReviewStatus, err = enum("childrenReviewStatus", "Accepted", "Accepted", "Pending", "All"); err != nil {
		return nil, err
	}

	f.includeMyPending = queryBool(r, "includeMyPending", false)
	f.mine = queryBool(r, "mine", false)
	f.includeMyPendingChildren = queryBool(r, "includeMyPendingChildren", false)

	f.width = strings.TrimSpace(queryGet(r, "width"))
	f.height = strings.TrimSpace(queryGet(r, "height"))
	f.byteSize = strings.TrimSpace(queryGet(r, "byteSize"))

	toBad := func(err error) error { return errBadRequest(err.Error()) }
	if f.includedTags, err = s.listParam(r, "includedTags", true); err != nil {
		return nil, toBad(err)
	}
	if f.excludedTags, err = s.listParam(r, "excludedTags", true); err != nil {
		return nil, toBad(err)
	}
	if f.includedArtists, err = s.listParam(r, "includedArtists", true); err != nil {
		return nil, toBad(err)
	}
	if f.excludedArtists, err = s.listParam(r, "excludedArtists", true); err != nil {
		return nil, toBad(err)
	}
	if f.includedIDs, err = s.idListParam(r, "includedIds"); err != nil {
		return nil, toBad(err)
	}
	if f.excludedIDs, err = s.idListParam(r, "excludedIds"); err != nil {
		return nil, toBad(err)
	}

	if v := strings.TrimSpace(queryGet(r, "color")); v != "" {
		c, ok := parseHexColor(v)
		if !ok {
			return nil, errFieldValidation("color", "The color must be a hex value such as #F4A6C1.")
		}
		f.color = &c
		f.colorTolerance = defaultColorTolerance
		if t := strings.TrimSpace(queryGet(r, "colorTolerance")); t != "" {
			n, err := strconv.Atoi(t)
			if err != nil || n < 1 || n > 100 {
				return nil, errFieldValidation("colorTolerance", "The color tolerance must be a whole number between 1 and 100.")
			}
			f.colorTolerance = n
		}
	}
	for _, key := range []string{"uploadedAfter", "uploadedBefore"} {
		v := strings.TrimSpace(queryGet(r, key))
		if v == "" {
			continue
		}
		t, ok := parseTimeParam(v)
		if !ok {
			return nil, errFieldValidation(key, fmt.Sprintf("The value '%s' is not a valid date or ISO 8601 timestamp.", truncateRunes(v, 64)))
		}
		if key == "uploadedAfter" {
			f.uploadedAfter = &t
		} else {
			f.uploadedBefore = &t
		}
	}

	if up := queryGet(r, "uploaderId"); up != "" && isModerator(ident) {
		if n, err := strconv.ParseInt(strings.TrimSpace(up), 10, 64); err == nil {
			f.uploaderID = &n
		}
	}
	return f, nil
}

func splitIDsAndNames(values []string) ([]int64, []string) {
	var ids []int64
	var names []string
	for _, v := range values {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			ids = append(ids, n)
		} else {
			names = append(names, v)
		}
	}
	return ids, names
}

type resolvedImageFilter struct {
	f           *imageFilter
	tagIDs      []int64
	exTagIDs    []int64
	artistSets  [][]int64
	exArtistIDs []int64
	impossible  bool
}

func (f *imageFilter) needsResolution() bool {
	for _, list := range [][]string{f.includedTags, f.excludedTags, f.includedArtists, f.excludedArtists} {
		if _, names := splitIDsAndNames(list); len(names) > 0 {
			return true
		}
	}
	return false
}

func appendUnique(dst []int64, vals ...int64) []int64 {
	for _, v := range vals {
		dup := false
		for _, d := range dst {
			if d == v {
				dup = true
				break
			}
		}
		if !dup {
			dst = append(dst, v)
		}
	}
	return dst
}

func (s *Server) resolveImageFilter(ctx context.Context, f *imageFilter) (*resolvedImageFilter, error) {
	rf := &resolvedImageFilter{f: f}
	tagIDs, tagSlugs := splitIDsAndNames(f.includedTags)
	exTagIDs, exTagSlugs := splitIDsAndNames(f.excludedTags)
	artistIDs, artistNames := splitIDsAndNames(f.includedArtists)
	exArtistIDs, exArtistNames := splitIDsAndNames(f.excludedArtists)

	slugs := append(append([]string{}, tagSlugs...), exTagSlugs...)
	names := append(append([]string{}, artistNames...), exArtistNames...)
	slugIDs := map[string]int64{}
	nameIDs := map[string][]int64{}
	if len(slugs) > 0 || len(names) > 0 {
		b := &pgx.Batch{}
		if len(slugs) > 0 {
			b.Queue(`SELECT "Id", "Slug" FROM "Tags" WHERE "Slug" = ANY($1)`, slugs).Query(func(rows pgx.Rows) error {
				for rows.Next() {
					var id int64
					var slug string
					if err := rows.Scan(&id, &slug); err != nil {
						return err
					}
					slugIDs[slug] = id
				}
				return rows.Err()
			})
		}
		if len(names) > 0 {
			b.Queue(`SELECT "Id", lower("Name") FROM "Artists" WHERE lower("Name") = ANY($1) ORDER BY "Id"`, names).Query(func(rows pgx.Rows) error {
				for rows.Next() {
					var id int64
					var name string
					if err := rows.Scan(&id, &name); err != nil {
						return err
					}
					nameIDs[name] = append(nameIDs[name], id)
				}
				return rows.Err()
			})
		}
		if err := s.db.SendBatch(ctx, b).Close(); err != nil {
			return nil, err
		}
	}

	rf.tagIDs = appendUnique(nil, tagIDs...)
	for _, slug := range tagSlugs {
		id, ok := slugIDs[slug]
		if !ok {
			rf.impossible = true
			continue
		}
		rf.tagIDs = appendUnique(rf.tagIDs, id)
	}
	rf.exTagIDs = appendUnique(nil, exTagIDs...)
	for _, slug := range exTagSlugs {
		if id, ok := slugIDs[slug]; ok {
			rf.exTagIDs = appendUnique(rf.exTagIDs, id)
		}
	}
	for _, id := range artistIDs {
		rf.artistSets = append(rf.artistSets, []int64{id})
	}
	for _, name := range artistNames {
		ids := nameIDs[name]
		if len(ids) == 0 {
			rf.impossible = true
			continue
		}
		rf.artistSets = append(rf.artistSets, ids)
	}
	rf.exArtistIDs = appendUnique(nil, exArtistIDs...)
	for _, name := range exArtistNames {
		rf.exArtistIDs = appendUnique(rf.exArtistIDs, nameIDs[name]...)
	}
	return rf, nil
}

func buildResolvedImageConds(rf *resolvedImageFilter, ident *auth.Identity, albumID *int64, argOffset int) (string, []any) {
	f := rf.f
	var conds []string
	a := &sqlArgs{offset: argOffset}

	if rf.impossible || (f.mine && ident == nil) {
		return "FALSE", nil
	}

	if albumID != nil {
		switch f.reviewStatus {
		case "Pending":
			conds = append(conds, `i."ReviewStatus" = 0`)
		case "Accepted":
			conds = append(conds, `i."ReviewStatus" = 1`)
		}
	} else {
		switch f.reviewStatus {
		case "All":
		case "Pending":
			conds = append(conds, `i."ReviewStatus" = 0`)
		default:
			if f.includeMyPending && ident != nil {
				conds = append(conds, fmt.Sprintf(`(i."ReviewStatus" = 1 OR (i."ReviewStatus" = 0 AND i."UploaderId" = %s))`, a.add(ident.UserID)))
			} else {
				conds = append(conds, `i."ReviewStatus" = 1`)
			}
		}
	}

	switch f.isNsfw {
	case "True":
		conds = append(conds, `i."IsNsfw"`)
	case "False":
		conds = append(conds, `NOT i."IsNsfw"`)
	}
	switch f.isAnimated {
	case "True":
		conds = append(conds, `i."IsAnimated"`)
	case "False":
		conds = append(conds, `NOT i."IsAnimated"`)
	}
	switch f.orientation {
	case "Landscape":
		conds = append(conds, `i."Width" > i."Height"`)
	case "Portrait":
		conds = append(conds, `i."Width" < i."Height"`)
	case "Square":
		conds = append(conds, `i."Width" = i."Height"`)
	}

	for _, dim := range []struct {
		key, col string
		lo, hi   int64
	}{
		{f.width, `i."Width"`, math.MinInt32, math.MaxInt32},
		{f.height, `i."Height"`, math.MinInt32, math.MaxInt32},
		{f.byteSize, `i."ByteSize"`, math.MinInt64, math.MaxInt64},
	} {
		if dim.key == "" {
			continue
		}
		if c, ok := dimensionCondition(dim.col, dim.key, dim.lo, dim.hi, a); ok {
			conds = append(conds, c)
		}
	}

	if albumID == nil && f.uploaderID != nil {
		conds = append(conds, fmt.Sprintf(`i."UploaderId" = %s`, a.add(*f.uploaderID)))
	}
	if f.mine {
		conds = append(conds, fmt.Sprintf(`i."UploaderId" = %s`, a.add(ident.UserID)))
	}
	if f.uploadedAfter != nil {
		conds = append(conds, fmt.Sprintf(`i."UploadedAt" > %s`, a.add(*f.uploadedAfter)))
	}
	if f.uploadedBefore != nil {
		conds = append(conds, fmt.Sprintf(`i."UploadedAt" < %s`, a.add(*f.uploadedBefore)))
	}
	if f.color != nil {
		conds = append(conds, colorCondition(*f.color, f.colorTolerance, a))
	}

	if len(f.includedIDs) == 1 {
		conds = append(conds, fmt.Sprintf(`i."Id" = %s`, a.add(f.includedIDs[0])))
	} else if len(f.includedIDs) > 1 {
		conds = append(conds, "FALSE")
	}
	if len(f.excludedIDs) > 0 {
		conds = append(conds, fmt.Sprintf(`i."Id" <> ALL(%s)`, a.add(f.excludedIDs)))
	}

	for _, id := range rf.tagIDs {
		conds = append(conds, fmt.Sprintf(`EXISTS (SELECT 1 FROM "ImageTag" xit WHERE xit."ImagesId" = i."Id" AND xit."TagsId" = %s)`, a.add(id)))
	}
	if len(rf.exTagIDs) > 0 {
		conds = append(conds, fmt.Sprintf(`NOT EXISTS (SELECT 1 FROM "ImageTag" xit WHERE xit."ImagesId" = i."Id" AND xit."TagsId" = ANY(%s))`, a.add(rf.exTagIDs)))
	}
	for _, set := range rf.artistSets {
		if len(set) == 1 {
			conds = append(conds, fmt.Sprintf(`EXISTS (SELECT 1 FROM "ArtistImage" xai WHERE xai."ImagesId" = i."Id" AND xai."ArtistsId" = %s)`, a.add(set[0])))
		} else {
			conds = append(conds, fmt.Sprintf(`EXISTS (SELECT 1 FROM "ArtistImage" xai WHERE xai."ImagesId" = i."Id" AND xai."ArtistsId" = ANY(%s))`, a.add(set)))
		}
	}
	if len(rf.exArtistIDs) > 0 {
		conds = append(conds, fmt.Sprintf(`NOT EXISTS (SELECT 1 FROM "ArtistImage" xai WHERE xai."ImagesId" = i."Id" AND xai."ArtistsId" = ANY(%s))`, a.add(rf.exArtistIDs)))
	}
	return strings.Join(conds, " AND "), a.args
}

func whereClause(conds string) string {
	if conds == "" {
		return ""
	}
	return " WHERE " + conds
}

func dimensionCondition(col, v string, lo, hi int64, a *sqlArgs) (string, bool) {
	parse := func(s string) (int64, bool) {
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil {
			var ne *strconv.NumError
			if !errors.As(err, &ne) || ne.Err != strconv.ErrRange {
				return 0, false
			}
		}
		return min(max(n, lo), hi), true
	}
	switch {
	case strings.HasPrefix(v, ">="):
		if n, ok := parse(v[2:]); ok {
			return fmt.Sprintf("%s >= %s", col, a.add(n)), true
		}
	case strings.HasPrefix(v, ">"):
		if n, ok := parse(v[1:]); ok {
			return fmt.Sprintf("%s > %s", col, a.add(n)), true
		}
	case strings.HasPrefix(v, "<="):
		if n, ok := parse(v[2:]); ok {
			return fmt.Sprintf("%s <= %s", col, a.add(n)), true
		}
	case strings.HasPrefix(v, "<"):
		if n, ok := parse(v[1:]); ok {
			return fmt.Sprintf("%s < %s", col, a.add(n)), true
		}
	case strings.Contains(v, ".."):
		parts := strings.SplitN(v, "..", 2)
		if lo, ok1 := parse(parts[0]); ok1 {
			if hi, ok2 := parse(parts[1]); ok2 {
				return fmt.Sprintf("%s BETWEEN %s AND %s", col, a.add(lo), a.add(hi)), true
			}
		}
	default:
		if n, ok := parse(v); ok {
			return fmt.Sprintf("%s = %s", col, a.add(n)), true
		}
	}
	return "", false
}

func checkImageOrder(f *imageFilter, albumID *int64) error {
	if f.orderBy == "AddedToAlbum" && albumID == nil {
		return errors.New("AddedToAlbum ordering is only available when viewing an album.")
	}
	return nil
}

func (s *Server) handleListImages(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	f, err := s.parseImageFilters(r, ident)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	if (f.reviewStatus != "Accepted" || f.childrenReviewStatus != "Accepted") && !isModerator(ident) {
		Forbidden(w, r, "Filtering by non-accepted review status is only available to moderators and admins.")
		return
	}

	page, pageSize := s.paginationParams(r, s.cfg.Image.DefaultPageSize, s.cfg.Image.MaxPageSize)
	if err := checkImageOrder(f, nil); err != nil {
		BadRequest(w, r, err.Error())
		return
	}

	ctx := r.Context()
	opts := hydrateOptions{
		ident:                    ident,
		childrenReviewStatus:     f.childrenReviewStatus,
		includeMyPendingChildren: f.includeMyPendingChildren,
		withUserData:             true,
	}
	var items []imageDto
	var total int64
	served := false
	if s.imageLists.enabled() && cacheableImageFilter(f, ident) {
		items, total, served, err = s.listImagesFromCache(ctx, f, page, pageSize, opts)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
	}
	if !served {
		rows, n, err := s.queryImagePage(ctx, f, ident, nil, page, pageSize)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		if items, err = s.hydrateImages(ctx, s.db, rows, opts); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		total = n
	}

	body := s.newPaginated(items, page, pageSize, total, s.cfg.Image.MaxPageSize, s.cfg.Image.DefaultPageSize)
	if f.orderBy == "Random" {
		writeJSON(w, http.StatusOK, body)
		return
	}
	writePublicJSON(w, r, body)
}

func (s *Server) handleGetImage(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	ident := IdentityFromContext(r.Context())
	childrenReviewStatus := "Accepted"
	if v := queryGet(r, "childrenReviewStatus"); v != "" {
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

	opts := hydrateOptions{
		ident:                    ident,
		childrenReviewStatus:     childrenReviewStatus,
		includeMyPendingChildren: queryBool(r, "includeMyPendingChildren", false),
		showUploader:             true,
		withUserData:             true,
	}
	var row imageRow
	var rowErr error
	b := &pgx.Batch{}
	b.Queue(`SELECT `+imageColumns+` FROM "Images" i WHERE i."Id" = $1`, id).QueryRow(func(r pgx.Row) error {
		row, rowErr = scanImageRow(r, false)
		if errors.Is(rowErr, pgx.ErrNoRows) {
			return nil
		}
		return rowErr
	})
	children := s.queueImageChildren(b, []int64{id}, opts)
	err = s.db.SendBatch(r.Context(), b).Close()
	if err == nil {
		err = rowErr
	}
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !canSeeContent(ident, row.ReviewStatus, row.UploaderID)) {
		NotFound(w, r, fmt.Sprintf("Image with ID %d not found.", id))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	dto := s.imageDtos([]imageRow{row}, opts, children)[0]
	writePublicJSON(w, r, &dto)
}

type processedImage struct {
	info        *imaging.Info
	data        []byte
	ext         string
	contentType string
	hash        uint64
	dominant    string
}

var allowedUploadContentTypes = map[string]bool{
	"image/jpeg": true, "image/png": true, "image/gif": true, "image/webp": true,
}

func (s *Server) maxUploadBytes() int64 { return int64(s.cfg.MaxUploadMB) * 1024 * 1024 }

func (s *Server) parseMultipart(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, s.maxUploadBytes()+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return errPayloadTooLarge(fmt.Sprintf("The request body exceeds the maximum size of %d MB.", s.cfg.MaxUploadMB))
		}
		return errBadRequest("Invalid multipart form.")
	}
	return nil
}

func (s *Server) processImageFile(ctx context.Context, file multipart.File, header *multipart.FileHeader) (*processedImage, error) {
	maxBytes := s.maxUploadBytes()
	if header.Size > maxBytes {
		return nil, errPayloadTooLarge(fmt.Sprintf("The request body exceeds the maximum size of %d MB.", s.cfg.MaxUploadMB))
	}
	if strings.TrimSpace(header.Filename) == "" {
		return nil, errFieldValidation("FileName", "File name is required.")
	}
	contentType := strings.ToLower(strings.TrimSpace(header.Header.Get("Content-Type")))
	if contentType == "" {
		return nil, errFieldValidation("ContentType", "Content type is required.")
	}
	if i := strings.Index(contentType, ";"); i >= 0 {
		contentType = strings.TrimSpace(contentType[:i])
	}
	if !allowedUploadContentTypes[contentType] {
		return nil, errFieldValidation("ContentType", "Only JPEG, PNG, GIF, and WEBP formats are supported.")
	}

	data, err := imaging.ReadAll(file, maxBytes)
	if errors.Is(err, imaging.ErrTooLarge) {
		return nil, errPayloadTooLarge(fmt.Sprintf("The request body exceeds the maximum size of %d MB.", s.cfg.MaxUploadMB))
	}
	if err != nil {
		return nil, errBadRequest("The uploaded file could not be read.")
	}

	info, err := imaging.Probe(data)
	if errors.Is(err, imaging.ErrUnsupportedFormat) {
		return nil, errFieldValidation("ContentType", "Only JPEG, PNG, GIF, and WEBP formats are supported.")
	}
	if errors.Is(err, imaging.ErrTooComplex) {
		return nil, errInvalidOperation(fmt.Sprintf(
			"The JPEG file contains too many scans (maximum %d). Please save it again with a standard encoder.", imaging.MaxJPEGScans))
	}
	if err != nil {
		return nil, errBadRequest("The uploaded file is not a valid image.")
	}
	if err := imaging.ValidateDimensions(info,
		s.cfg.Image.MinWidth, s.cfg.Image.MinHeight, s.cfg.Image.MaxWidth, s.cfg.Image.MaxHeight,
		s.cfg.Image.AnimatedMinWidth, s.cfg.Image.AnimatedMinHeight, s.cfg.Image.AnimatedMaxWidth, s.cfg.Image.AnimatedMaxHeight); err != nil {
		return nil, errInvalidOperation(err.Error())
	}
	if limit := s.cfg.Limits.MaxDecodeBytes; limit > 0 && info.DecodeBytes > limit {
		return nil, errInvalidOperation(fmt.Sprintf(
			"The image needs too much memory to process (about %d MB, maximum %d MB). Please upload a smaller image or save it as a baseline JPEG or 8-bit PNG.",
			(info.DecodeBytes+(1<<20)-1)>>20, limit>>20))
	}

	release, err := s.acquireImageSlot(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	stored, stripped := data, false
	if clean, err := imaging.StripMetadata(data, info); err == nil {
		stored, stripped = clean, true
	}
	img, err := imaging.Decode(stored, info, s.cfg.Limits.MaxImagePixels)
	if err != nil && stripped && !errors.Is(err, imaging.ErrTooManyPixels) {
		if original, oerr := imaging.Decode(data, info, s.cfg.Limits.MaxImagePixels); oerr == nil {
			s.log.Warn("removing image metadata made the file unreadable, keeping the original", "format", info.Format)
			img, err, stored = original, nil, data
		}
	}
	if errors.Is(err, imaging.ErrTooManyPixels) {
		return nil, errInvalidOperation(fmt.Sprintf("Image resolution is too high (%dx%d).", info.Width, info.Height))
	}
	if err != nil {
		return nil, errBadRequest("The uploaded file is not a valid image.")
	}

	return &processedImage{
		info:        info,
		data:        stored,
		ext:         imaging.Extension(info.Format, strings.ToLower(filepath.Ext(header.Filename))),
		contentType: imaging.ContentType(info.Format),
		hash:        imaging.PerceptualHash(img, info.Orientation),
		dominant:    imaging.DominantColor(img),
	}, nil
}

type duplicateImage struct {
	id           int64
	reviewStatus int
	uploaderID   *int64
}

func (d duplicateImage) conflictMessage(ident *auth.Identity) string {
	if canSeeContent(ident, d.reviewStatus, d.uploaderID) {
		return fmt.Sprintf("Duplicate image found. ID: %d", d.id)
	}
	return "Duplicate image found."
}

func (s *Server) findDuplicate(ctx context.Context, q querier, hash uint64, excludeID int64) (duplicateImage, error) {
	var d duplicateImage
	err := q.QueryRow(ctx, `
		SELECT "Id", "ReviewStatus", "UploaderId" FROM "Images"
		WHERE "Id" <> $1 AND bit_count("PerceptualHash" # $2::bit(64)) <= $3
		ORDER BY "Id" LIMIT 1`, excludeID, hashToBits(hash), duplicateDistance).Scan(&d.id, &d.reviewStatus, &d.uploaderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return duplicateImage{}, nil
	}
	return d, err
}

const uploadLockKey int64 = 0x5761696675

func (s *Server) handleUploadImage(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	if !requireMinRole(ident, s.cfg.Permissions.ImageUploadMinRole) {
		s.permissionRoleForbidden(w, r, s.cfg.Permissions.ImageUploadMinRole, auth.RoleUser)
		return
	}
	ctx := r.Context()

	if err := s.parseMultipart(w, r); err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("file")
	if err != nil {
		WriteValidationProblem(w, r, map[string][]string{"File": {"The File field is required."}})
		return
	}
	defer file.Close()

	img, err := s.processImageFile(ctx, file, header)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}

	source := strings.TrimSpace(r.FormValue("source"))
	if runeLen(source) > 500 {
		WriteValidationProblem(w, r, map[string][]string{"Source": {"Source URL must not exceed 500 characters."}})
		return
	}
	if source != "" && !isValidWebURL(source) {
		WriteValidationProblem(w, r, map[string][]string{"Source": {"Source must be a valid http(s) URL."}})
		return
	}
	isNsfw := strings.EqualFold(strings.TrimSpace(r.FormValue("isNsfw")), "true")

	tagSlugs, err := cleanSlugList(r.MultipartForm.Value["tags"], s.cfg.Limits.MaxTagsPerImage, "tags")
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	artistIDs, err := cleanIDFormList(r.MultipartForm.Value["artists"], s.cfg.Limits.MaxArtistsPerImage, "artists")
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	artists, err := s.resolveArtistsByIDs(ctx, s.db, artistIDs, ident)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	tags, err := s.resolveTagsBySlugs(ctx, s.db, tagSlugs, ident)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}

	autoAccept := !s.cfg.Review.RequireImageReview || auth.ReviewBypassed(ident.Role, s.cfg.Review.ImageReviewBypassMinRole)

	var imageID, taskID int64
	slot, err := s.cluster.reserve(ctx)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	defer slot.release()
	unlock := func() {}
	defer func() { unlock() }()
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if !autoAccept {
			if err := s.checkPendingSubmissions(ctx, tx, ident.UserID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, uploadLockKey); err != nil {
			return err
		}
		dup, err := s.findDuplicate(ctx, tx, img.hash, 0)
		if err != nil {
			return err
		}
		if dup.id > 0 {
			return errConflict(dup.conflictMessage(ident))
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO "Images" ("PerceptualHash","Extension","DominantColor","Source","UploaderId",
				"UploadedAt","IsNsfw","IsAnimated","Width","Height","ByteSize","ReviewStatus")
			VALUES ($1,$2,$3,$4,$5,now(),$6,$7,$8,$9,$10,0)
			RETURNING "Id"`,
			hashToBits(img.hash), img.ext, img.dominant, orEmptyString(source), ident.UserID,
			isNsfw, img.info.IsAnimated, img.info.Width, img.info.Height, int64(len(img.data))).Scan(&imageID); err != nil {
			return err
		}
		lockCtx, cancelLock := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancelLock()
		var lerr error
		if unlock, lerr = slot.lockNew(lockCtx, imageID); lerr != nil {
			return lerr
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO "ReviewTasks" ("Kind","TargetType","TargetId","SubmitterId","Status","CreatedAt")
			VALUES (0,0,$1,$2,0,now()) RETURNING "Id"`, imageID, ident.UserID).Scan(&taskID); err != nil {
			return err
		}
		for _, t := range tags {
			if _, err := tx.Exec(ctx, `INSERT INTO "ImageTag" ("ImagesId","TagsId") VALUES ($1,$2)`, imageID, t.ID); err != nil {
				return err
			}
		}
		for _, a := range artists {
			if _, err := tx.Exec(ctx, `INSERT INTO "ArtistImage" ("ArtistsId","ImagesId") VALUES ($1,$2)`, a.ID, imageID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}

	finishDetached := s.beginDetached()
	defer finishDetached()
	key := imageKey(imageID, img.ext)
	uploadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	err = s.s3.Upload(uploadCtx, key, bytes.NewReader(img.data), int64(len(img.data)), img.contentType)
	cancel()
	if err != nil {
		s.log.Error("storage upload failed, rolling back image", "imageId", imageID, "error", err)
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		_ = pgx.BeginFunc(cleanupCtx, s.db, func(tx pgx.Tx) error {
			if _, err := tx.Exec(cleanupCtx, `DELETE FROM "ReviewTasks" WHERE "Id" = $1`, taskID); err != nil {
				return err
			}
			_, err := tx.Exec(cleanupCtx, `DELETE FROM "Images" WHERE "Id" = $1`, imageID)
			return err
		})
		s.objects.remove(cleanupCtx, key)
		cancel()
		InternalError(w, r, s.log, err)
		return
	}

	finishCtx, cancelFinish := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancelFinish()
	removed := false
	err = pgx.BeginFunc(finishCtx, s.db, func(tx pgx.Tx) error {
		taskStatus := -1
		if err := tx.QueryRow(finishCtx, `SELECT "Status" FROM "ReviewTasks" WHERE "Id" = $1 FOR UPDATE`, taskID).
			Scan(&taskStatus); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var reviewStatus int
		err := tx.QueryRow(finishCtx, `SELECT "ReviewStatus" FROM "Images" WHERE "Id" = $1 FOR UPDATE`, imageID).Scan(&reviewStatus)
		if errors.Is(err, pgx.ErrNoRows) {
			removed = true
			return nil
		}
		if err != nil {
			return err
		}
		if !autoAccept || taskStatus != 0 || reviewStatus != 0 {
			return nil
		}
		if _, err := tx.Exec(finishCtx, `UPDATE "Images" SET "ReviewStatus" = 1 WHERE "Id" = $1 AND "ReviewStatus" = 0`, imageID); err != nil {
			return err
		}
		_, err = tx.Exec(finishCtx, `
			UPDATE "ReviewTasks" SET "Status" = 1, "ReviewerNote" = 'Auto-accepted (trusted role).', "ResolvedAt" = now()
			WHERE "Id" = $1 AND "Status" = 0`, taskID)
		return err
	})
	if err != nil {
		s.log.Error("finishing upload failed", "imageId", imageID, "error", err)
		checkCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		var exists bool
		if qerr := s.db.QueryRow(checkCtx, `SELECT EXISTS(SELECT 1 FROM "Images" WHERE "Id" = $1)`, imageID).Scan(&exists); qerr == nil && !exists {
			removed = true
		}
		cancel()
	}
	if removed {
		s.removeStoredImage(finishCtx, key)
		unlock()
		finishDetached()
		s.writeDomainError(w, r, errConflict("The upload was cancelled or removed while it was being processed."))
		return
	}
	unlock()
	finishDetached()

	row, err := s.loadImageRow(ctx, s.db, imageID)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	dto, err := s.hydrateImage(ctx, s.db, row, hydrateOptions{ident: ident, childrenReviewStatus: "All", showUploader: true})
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/images/%d", imageID))
	writeJSON(w, http.StatusCreated, dto)
}

func (s *Server) handlePatchImage(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}

	var exists bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "Images" WHERE "Id" = $1)`, id).Scan(&exists); err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if !exists {
		NotFound(w, r, fmt.Sprintf("Image with ID %d not found.", id))
		return
	}

	if err := s.parseMultipart(w, r); err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	defer r.MultipartForm.RemoveAll()

	isNsfw := strings.EqualFold(strings.TrimSpace(r.FormValue("isNsfw")), "true")

	reviewStatus := ""
	if v := r.FormValue("reviewStatus"); v != "" {
		var ok bool
		if reviewStatus, ok = enumValue(v, "Pending", "Accepted"); !ok {
			invalidEnumProblem(w, r, "reviewStatus", v)
			return
		}
	}

	sourceRaw := strings.TrimSpace(r.FormValue("source"))
	if runeLen(sourceRaw) > 500 {
		WriteValidationProblem(w, r, map[string][]string{"Source": {"Source URL must not exceed 500 characters."}})
		return
	}
	if sourceRaw != "" && !isValidWebURL(sourceRaw) {
		WriteValidationProblem(w, r, map[string][]string{"Source": {"Source must be a valid http(s) URL."}})
		return
	}
	source := orEmptyString(sourceRaw)

	var replacement *processedImage
	if file, header, ferr := r.FormFile("file"); ferr == nil {
		replacement, err = s.processImageFile(ctx, file, header)
		file.Close()
		if err != nil {
			s.writeDomainError(w, r, err)
			return
		}
		dup, err := s.findDuplicate(ctx, s.db, replacement.hash, id)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		if dup.id > 0 {
			Conflict(w, r, dup.conflictMessage(IdentityFromContext(ctx)))
			return
		}
	}

	var newUploader *int64
	if v := strings.TrimSpace(r.FormValue("userId")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			var uExists bool
			if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "Users" WHERE "Id" = $1)`, n).Scan(&uExists); err != nil {
				InternalError(w, r, s.log, err)
				return
			}
			if !uExists {
				NotFound(w, r, fmt.Sprintf("User with ID %d not found.", n))
				return
			}
			newUploader = &n
		}
	}

	var newTags []tagDto
	_, hasTags := r.MultipartForm.Value["tags"]
	if hasTags {
		slugs, err := cleanSlugList(r.MultipartForm.Value["tags"], s.cfg.Limits.MaxTagsPerImage, "tags")
		if err != nil {
			s.writeDomainError(w, r, err)
			return
		}
		if newTags, err = s.resolveTagsBySlugs(ctx, s.db, slugs, ident); err != nil {
			s.writeDomainError(w, r, err)
			return
		}
	}
	var newArtists []artistDto
	_, hasArtists := r.MultipartForm.Value["artists"]
	if hasArtists {
		ids, err := cleanIDFormList(r.MultipartForm.Value["artists"], s.cfg.Limits.MaxArtistsPerImage, "artists")
		if err != nil {
			s.writeDomainError(w, r, err)
			return
		}
		if newArtists, err = s.resolveArtistsByIDs(ctx, s.db, ids, ident); err != nil {
			s.writeDomainError(w, r, err)
			return
		}
	}

	var rowExt, newKey string
	unlock, finishDetached := func() {}, func() {}
	defer func() {
		unlock()
		finishDetached()
	}()
	if replacement != nil {
		var lerr error
		if unlock, lerr = s.lockImage(ctx, id); lerr != nil {
			s.writeDomainError(w, r, lerr)
			return
		}
		finishDetached = s.beginDetached()
		if err := s.db.QueryRow(ctx, `SELECT "Extension" FROM "Images" WHERE "Id" = $1`, id).Scan(&rowExt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				NotFound(w, r, fmt.Sprintf("Image with ID %d not found.", id))
				return
			}
			InternalError(w, r, s.log, err)
			return
		}
		newKey = imageKey(id, replacement.ext)
		uploadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		err := s.s3.Upload(uploadCtx, newKey, bytes.NewReader(replacement.data), int64(len(replacement.data)), replacement.contentType)
		cancel()
		if err != nil {
			if newKey != imageKey(id, rowExt) {
				s.objects.remove(ctx, newKey)
			} else {
				s.log.Error("storing the replacement failed; the file may have been replaced anyway, repeat the edit",
					"imageId", id, "error", err)
			}
			InternalError(w, r, s.log, err)
			return
		}
	}

	txCtx := ctx
	if replacement != nil {
		var cancelTx context.CancelFunc
		txCtx, cancelTx = context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancelTx()
	}
	imageGone := false
	err = pgx.BeginFunc(txCtx, s.db, func(tx pgx.Tx) error {
		if err := lockPendingTasks(txCtx, tx, 0, id); err != nil {
			return err
		}
		if err := tx.QueryRow(txCtx, `SELECT "Extension" FROM "Images" WHERE "Id" = $1 FOR UPDATE`, id).Scan(&rowExt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				imageGone = true
				return errNotFound(fmt.Sprintf("Image with ID %d not found.", id))
			}
			return err
		}
		if _, err := tx.Exec(txCtx, `UPDATE "Images" SET "Source" = $1, "IsNsfw" = $2, "UploaderId" = $3 WHERE "Id" = $4`,
			source, isNsfw, newUploader, id); err != nil {
			return err
		}
		if hasTags {
			if _, err := tx.Exec(txCtx, `DELETE FROM "ImageTag" WHERE "ImagesId" = $1`, id); err != nil {
				return err
			}
			for _, t := range newTags {
				if _, err := tx.Exec(txCtx, `INSERT INTO "ImageTag" ("ImagesId","TagsId") VALUES ($1,$2)`, id, t.ID); err != nil {
					return err
				}
			}
		}
		if hasArtists {
			if _, err := tx.Exec(txCtx, `DELETE FROM "ArtistImage" WHERE "ImagesId" = $1`, id); err != nil {
				return err
			}
			for _, a := range newArtists {
				if _, err := tx.Exec(txCtx, `INSERT INTO "ArtistImage" ("ArtistsId","ImagesId") VALUES ($1,$2)`, a.ID, id); err != nil {
					return err
				}
			}
		}
		switch reviewStatus {
		case "Accepted":
			if _, err := tx.Exec(txCtx, `UPDATE "Images" SET "ReviewStatus" = 1 WHERE "Id" = $1`, id); err != nil {
				return err
			}
			if _, err := tx.Exec(txCtx, `
				UPDATE "ReviewTasks" SET "Status" = 1, "ReviewerId" = $2, "ResolvedAt" = now()
				WHERE "TargetType" = 0 AND "TargetId" = $1 AND "Kind" = 0 AND "Status" = 0`, id, ident.UserID); err != nil {
				return err
			}
		case "Pending":
			if _, err := tx.Exec(txCtx, `UPDATE "Images" SET "ReviewStatus" = 0 WHERE "Id" = $1`, id); err != nil {
				return err
			}
		}
		if replacement != nil {
			if _, err := tx.Exec(txCtx, `
				UPDATE "Images" SET "Extension" = $1, "PerceptualHash" = $2, "DominantColor" = $3,
					"IsAnimated" = $4, "Width" = $5, "Height" = $6, "ByteSize" = $7
				WHERE "Id" = $8`,
				replacement.ext, hashToBits(replacement.hash), replacement.dominant, replacement.info.IsAnimated,
				replacement.info.Width, replacement.info.Height, int64(len(replacement.data)), id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if replacement != nil {
			if imageGone || newKey != imageKey(id, rowExt) {
				s.objects.remove(ctx, newKey)
			} else {
				s.log.Error("image file replaced but the metadata update failed; repeat the edit to fix it", "imageId", id, "error", err)
			}
		}
		s.writeDomainError(w, r, err)
		return
	}

	if replacement != nil {
		s.cdn.purge([]string{newKey})
		if oldKey := imageKey(id, rowExt); oldKey != newKey {
			s.objects.remove(ctx, oldKey)
		}
	}
	unlock()
	finishDetached()

	row, err := s.loadImageRow(ctx, s.db, id)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	dto, err := s.hydrateImage(ctx, s.db, row, hydrateOptions{ident: ident, childrenReviewStatus: "All", showUploader: true})
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) handleDeleteImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}

	var ext string
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT "Extension" FROM "Images" WHERE "Id" = $1 FOR UPDATE`, id).Scan(&ext); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return errNotFound(fmt.Sprintf("Image with ID %d not found.", id))
			}
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE "Reports" SET "Status" = 1, "ReviewerNote" = 'The reported image was deleted by a moderator.'
			WHERE "ImageId" = $1 AND "Status" = 0`, id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM "Images" WHERE "Id" = $1`, id)
		return err
	})
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}

	s.removeStoredImage(ctx, imageKey(id, ext))
	w.WriteHeader(http.StatusNoContent)
}
