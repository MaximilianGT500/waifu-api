package server

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/waifu-im/waifu-api/backend/internal/auth"
)

const favoritesCountExpr = `(SELECT COUNT(*) FROM "AlbumItems" fai JOIN "Albums" fal ON fal."Id" = fai."AlbumId" AND fal."IsDefault" WHERE fai."ImageId" = i."Id")`

const favoritesCountJoin = ` LEFT JOIN (SELECT fai."ImageId" AS iid, COUNT(*) AS cnt FROM "AlbumItems" fai JOIN "Albums" fal ON fal."Id" = fai."AlbumId" AND fal."IsDefault" GROUP BY fai."ImageId") fav ON fav.iid = i."Id"`

const favoritesJoinThreshold = 5000

const trendingJoin = ` LEFT JOIN (SELECT tai."ImageId" AS iid, COUNT(*) AS cnt FROM "AlbumItems" tai JOIN "Albums" tal ON tal."Id" = tai."AlbumId" AND tal."IsDefault" WHERE tai."AddedAt" > now() - interval '7 days' GROUP BY tai."ImageId") tr ON tr.iid = i."Id"`

const trendingOrder = ` ORDER BY COALESCE(tr.cnt, 0) DESC, i."UploadedAt" DESC, i."Id" DESC`

type listOrder uint8

const (
	orderNewest listOrder = iota
	orderFavorites
	orderTrending
)

func listOrderOf(f *imageFilter) listOrder {
	switch f.orderBy {
	case "Favorites":
		return orderFavorites
	case "Trending":
		return orderTrending
	}
	return orderNewest
}

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
	if f.reviewStatus != "Accepted" || (f.includeMyPending && ident != nil) || f.mine {
		return false
	}
	if len(f.includedIDs) > 0 || len(f.excludedIDs) > 0 {
		return false
	}
	switch f.orderBy {
	case "Random", "UploadedAt", "Favorites", "Trending":
		return true
	}
	return false
}

func imageListKey(f *imageFilter, order listOrder) string {
	var b strings.Builder
	for _, v := range []string{f.isNsfw, f.isAnimated, f.orientation, f.width, f.height, f.byteSize} {
		b.WriteString(v)
		b.WriteByte(0)
	}
	for _, t := range []*time.Time{f.uploadedAfter, f.uploadedBefore} {
		if t != nil {
			b.WriteString(strconv.FormatInt(t.UnixMicro(), 36))
		}
		b.WriteByte(0)
	}
	if f.color != nil {
		b.WriteString(f.color.hex())
		b.WriteByte('~')
		b.WriteString(strconv.Itoa(f.colorTolerance))
	}
	b.WriteByte(0)
	if f.uploaderID != nil {
		b.WriteString(strconv.FormatInt(*f.uploaderID, 10))
	}
	b.WriteByte(0)
	for _, list := range [][]string{f.includedTags, f.excludedTags, f.includedArtists, f.excludedArtists} {
		b.WriteString(strings.Join(list, "\x1f"))
		b.WriteByte(0)
	}
	switch order {
	case orderFavorites:
		b.WriteString("favorites")
	case orderTrending:
		b.WriteString("trending")
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

func (s *Server) buildImageList(ctx context.Context, f *imageFilter, order listOrder) (*imageList, int, error) {
	if f.color != nil {
		plain := *f
		plain.color = nil
		base, err := s.imageLists.load(ctx, imageListKey(&plain, order), 0, func(ctx context.Context) (*imageList, int, error) {
			return s.buildImageList(ctx, &plain, order)
		})
		if err != nil {
			return nil, 0, err
		}
		colors, err := s.loadColors(ctx)
		if err != nil {
			return nil, 0, err
		}
		ids := filterByColor(base.ids, colors, *f.color, f.colorTolerance)
		return &imageList{ids: ids, conds: base.conds, args: base.args}, len(ids) + 64, nil
	}
	favorites := order == orderFavorites
	var base *imageList
	if favorites {
		var err error
		base, err = s.imageLists.load(ctx, imageListKey(f, orderNewest), 0, func(ctx context.Context) (*imageList, int, error) {
			return s.buildImageList(ctx, f, orderNewest)
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
	if order == orderTrending {
		query = `SELECT i."Id" FROM "Images" i` + trendingJoin + whereClause(conds) + trendingOrder
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
	order := listOrderOf(f)
	key := imageListKey(f, order)
	list, err := s.imageLists.load(ctx, key, 0, func(ctx context.Context) (*imageList, int, error) {
		return s.buildImageList(ctx, f, order)
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
	children := s.queueImageChildren(b, pick, opts)
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
	case "Trending":
		from += trendingJoin
		order = trendingOrder
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
