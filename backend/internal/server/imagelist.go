package server

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/waifu-im/waifu-api/backend/internal/auth"
)

const favoritesCountExpr = `(SELECT COUNT(*) FROM "AlbumItems" fai JOIN "Albums" fal ON fal."Id" = fai."AlbumId" AND fal."IsDefault" WHERE fai."ImageId" = i."Id")`

const favoritesCountJoin = ` LEFT JOIN (SELECT fai."ImageId" AS iid, COUNT(*) AS cnt FROM "AlbumItems" fai JOIN "Albums" fal ON fal."Id" = fai."AlbumId" AND fal."IsDefault" GROUP BY fai."ImageId") fav ON fav.iid = i."Id"`

const favoritesJoinThreshold = 5000

const (
	imageListMaxEntries = 512
	imageListMaxIDs     = 2_000_000
)

type imageList struct {
	ids   []int64
	conds string
	args  []any
}

func cacheableImageFilter(f *imageFilter, ident *auth.Identity) bool {
	if f.reviewStatus != "Accepted" || (f.includeMyPending && ident != nil) {
		return false
	}
	if len(f.includedIDs) > 0 || len(f.excludedIDs) > 0 {
		return false
	}
	switch f.orderBy {
	case "Random", "UploadedAt", "Favorites":
		return true
	}
	return false
}

func imageListKey(f *imageFilter, favorites bool) string {
	var b strings.Builder
	for _, v := range []string{f.isNsfw, f.isAnimated, f.orientation, f.width, f.height, f.byteSize} {
		b.WriteString(v)
		b.WriteByte(0)
	}
	if f.uploaderID != nil {
		b.WriteString(strconv.FormatInt(*f.uploaderID, 10))
	}
	b.WriteByte(0)
	for _, list := range [][]string{f.includedTags, f.excludedTags, f.includedArtists, f.excludedArtists} {
		b.WriteString(strings.Join(list, "\x1f"))
		b.WriteByte(0)
	}
	if favorites {
		b.WriteString("favorites")
	}
	return b.String()
}

func (s *Server) resolveIfNeeded(ctx context.Context, f *imageFilter) (*resolvedImageFilter, error) {
	if !f.needsResolution() {
		rf := &resolvedImageFilter{f: f}
		tagIDs, _ := splitIDsAndNames(f.includedTags)
		exTagIDs, _ := splitIDsAndNames(f.excludedTags)
		artistIDs, _ := splitIDsAndNames(f.includedArtists)
		exArtistIDs, _ := splitIDsAndNames(f.excludedArtists)
		rf.tagIDs = appendUnique(nil, tagIDs...)
		rf.exTagIDs = appendUnique(nil, exTagIDs...)
		for _, id := range artistIDs {
			rf.artistSets = append(rf.artistSets, []int64{id})
		}
		rf.exArtistIDs = appendUnique(nil, exArtistIDs...)
		return rf, nil
	}
	return s.resolveImageFilter(ctx, f)
}

func collectIDs(rows pgx.Rows) ([]int64, error) {
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Server) buildImageList(ctx context.Context, f *imageFilter, favorites bool) (*imageList, int, error) {
	var base *imageList
	if favorites {
		var err error
		base, err = s.imageLists.load(ctx, imageListKey(f, false), 0, func(ctx context.Context) (*imageList, int, error) {
			return s.buildImageList(ctx, f, false)
		})
		if err != nil {
			return nil, 0, err
		}
		if len(base.ids) == 0 {
			return base, 64, nil
		}
	}
	rf, err := s.resolveIfNeeded(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	conds, args := buildResolvedImageConds(rf, nil, nil, 0)
	recheck, recheckArgs := buildResolvedImageConds(rf, nil, nil, 1)
	query := `SELECT i."Id" FROM "Images" i` + whereClause(conds) + ` ORDER BY i."UploadedAt" DESC, i."Id" DESC`
	if favorites {
		if len(base.ids) > favoritesJoinThreshold {
			query = `SELECT i."Id" FROM "Images" i` + favoritesCountJoin + whereClause(conds) + ` ORDER BY COALESCE(fav.cnt, 0) DESC, i."Id" DESC`
		} else {
			query = `SELECT i."Id" FROM "Images" i` + whereClause(conds) + ` ORDER BY ` + favoritesCountExpr + ` DESC, i."Id" DESC`
		}
	}
	rows, err := s.db.Query(ctx, query, append([]any{pgx.QueryExecModeCacheDescribe}, args...)...)
	if err != nil {
		return nil, 0, err
	}
	ids, err := collectIDs(rows)
	if err != nil {
		return nil, 0, err
	}
	return &imageList{ids: ids, conds: recheck, args: recheckArgs}, len(ids) + 64, nil
}

func randomSample(ids []int64, n int) []int64 {
	if n <= 0 {
		return nil
	}
	if n >= len(ids)/4 {
		out := make([]int64, len(ids))
		copy(out, ids)
		for i := 0; i < n; i++ {
			j := i + rand.IntN(len(out)-i)
			out[i], out[j] = out[j], out[i]
		}
		return out[:n]
	}
	picked := make(map[int]struct{}, n)
	out := make([]int64, 0, n)
	for len(out) < n {
		i := rand.IntN(len(ids))
		if _, dup := picked[i]; dup {
			continue
		}
		picked[i] = struct{}{}
		out = append(out, ids[i])
	}
	return out
}

func (s *Server) listImagesFromCache(ctx context.Context, f *imageFilter, page, pageSize int, opts hydrateOptions) ([]imageDto, int64, bool, error) {
	favorites := f.orderBy == "Favorites"
	key := imageListKey(f, favorites)
	var scopes cacheScope
	if favorites {
		scopes = scopeFavorites
	}
	list, err := s.imageLists.load(ctx, key, scopes, func(ctx context.Context) (*imageList, int, error) {
		return s.buildImageList(ctx, f, favorites)
	})
	if err != nil {
		return nil, 0, false, err
	}
	total := len(list.ids)
	offset := offsetOf(page, pageSize)
	if total == 0 || offset >= total {
		return []imageDto{}, int64(total), true, nil
	}
	var pick []int64
	if f.orderBy == "Random" {
		pick = randomSample(list.ids, min(pageSize, total-offset))
	} else {
		pick = list.ids[offset:min(offset+pageSize, total)]
	}

	b := &pgx.Batch{}
	found := make(map[int64]imageRow, len(pick))
	b.Queue(`SELECT `+imageColumns+` FROM "Images" i WHERE i."Id" = ANY($1) AND `+list.conds,
		append([]any{pick}, list.args...)...).Query(func(rows pgx.Rows) error {
		for rows.Next() {
			row, err := scanImageRow(rows, false)
			if err != nil {
				return err
			}
			found[row.ID] = row
		}
		return rows.Err()
	})
	children := queueImageChildren(b, pick, opts)
	if err := s.db.SendBatch(ctx, b).Close(); err != nil {
		return nil, 0, false, err
	}
	if len(found) != len(pick) {
		s.imageLists.invalidate(key)
		return nil, 0, false, nil
	}
	rows := make([]imageRow, len(pick))
	for i, id := range pick {
		rows[i] = found[id]
	}
	return s.imageDtos(rows, opts, children), int64(total), true, nil
}

func (s *Server) queryImagePage(ctx context.Context, f *imageFilter, ident *auth.Identity, albumID *int64, page, pageSize int) ([]imageRow, int64, error) {
	rf, err := s.resolveIfNeeded(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	argOffset := 0
	from := ` FROM "Images" i`
	if albumID != nil {
		argOffset = 1
		from = ` FROM "Images" i JOIN "AlbumItems" ai ON ai."ImageId" = i."Id" AND ai."AlbumId" = $1`
	}
	conds, args := buildResolvedImageConds(rf, ident, albumID, argOffset)
	if albumID != nil {
		args = append([]any{*albumID}, args...)
	}
	where := whereClause(conds)
	mode := []any{pgx.QueryExecModeCacheDescribe}

	var total int64
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*)`+from+where, append(mode, args...)...).Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := offsetOf(page, pageSize)
	if total == 0 || int64(offset) >= total {
		return nil, total, nil
	}

	cols := imageColumns
	if albumID != nil {
		cols += `, ai."AddedAt"`
	}
	var order string
	switch f.orderBy {
	case "UploadedAt":
		order = ` ORDER BY i."UploadedAt" DESC, i."Id" DESC`
	case "Favorites":
		if total > favoritesJoinThreshold {
			from += favoritesCountJoin
			order = ` ORDER BY COALESCE(fav.cnt, 0) DESC, i."Id" DESC`
		} else {
			order = ` ORDER BY ` + favoritesCountExpr + ` DESC, i."Id" DESC`
		}
	case "AddedToAlbum":
		order = ` ORDER BY ai."AddedAt" DESC, i."Id" DESC`
	default:
		order = ` ORDER BY random()`
	}
	query := `SELECT ` + cols + from + where + order + fmt.Sprintf(" LIMIT %d OFFSET %d", pageSize, offset)
	rows, err := s.db.Query(ctx, query, append(mode, args...)...)
	if err != nil {
		return nil, 0, err
	}
	imageRows, err := collectImageRows(rows, albumID != nil)
	if err != nil {
		return nil, 0, err
	}
	return imageRows, total, nil
}
