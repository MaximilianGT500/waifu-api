package server

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/waifu-im/waifu-api/backend/internal/auth"
)

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

const imageColumns = `i."Id", i."PerceptualHash", i."Extension", i."DominantColor", i."Source",
	i."UploaderId", i."UploadedAt", i."IsNsfw", i."IsAnimated", i."Width", i."Height",
	i."ByteSize", i."ReviewStatus"`

func scanImageRow(row pgx.Row, withAddedAt bool) (imageRow, error) {
	var r imageRow
	dest := []any{&r.ID, &r.PerceptualHash, &r.Extension, &r.DominantColor, &r.Source,
		&r.UploaderID, &r.UploadedAt, &r.IsNsfw, &r.IsAnimated, &r.Width, &r.Height,
		&r.ByteSize, &r.ReviewStatus}
	if withAddedAt {
		dest = append(dest, &r.AddedAt)
	}
	err := row.Scan(dest...)
	return r, err
}

func collectImageRows(rows pgx.Rows, withAddedAt bool) ([]imageRow, error) {
	defer rows.Close()
	out := []imageRow{}
	for rows.Next() {
		r, err := scanImageRow(rows, withAddedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Server) loadImageRow(ctx context.Context, q querier, id int64) (imageRow, error) {
	return scanImageRow(q.QueryRow(ctx, `SELECT `+imageColumns+` FROM "Images" i WHERE i."Id" = $1`, id), false)
}

type hydrateOptions struct {
	ident                    *auth.Identity
	childrenReviewStatus     string
	includeMyPendingChildren bool
	showUploader             bool
	withUserData             bool
}

func childCondition(alias string, opts hydrateOptions, args *[]any) string {
	switch {
	case opts.childrenReviewStatus == "Pending":
		return alias + `."ReviewStatus" = 0`
	case opts.childrenReviewStatus == "All":
		return "TRUE"
	case opts.includeMyPendingChildren && opts.ident != nil:
		*args = append(*args, opts.ident.UserID)
		return fmt.Sprintf(`(%s."ReviewStatus" = 1 OR (%s."ReviewStatus" = 0 AND %s."CreatorId" = $%d))`, alias, alias, alias, len(*args))
	default:
		return alias + `."ReviewStatus" = 1`
	}
}

type imageChildren struct {
	tags      map[int64][]tagDto
	artists   map[int64][]artistDto
	favorites map[int64]int64
	albums    map[int64][]imageAlbumRef
	likedAt   map[int64]*time.Time
}

func (s *Server) queueImageChildren(b *pgx.Batch, ids []int64, opts hydrateOptions) *imageChildren {
	c := &imageChildren{
		tags:      map[int64][]tagDto{},
		artists:   map[int64][]artistDto{},
		favorites: map[int64]int64{},
		albums:    map[int64][]imageAlbumRef{},
		likedAt:   map[int64]*time.Time{},
	}
	if len(ids) == 0 {
		return c
	}

	tagArgs := []any{ids}
	tagCond := childCondition("tg", opts, &tagArgs)
	b.Queue(`
		SELECT it."ImagesId", tg."Id", tg."Name", tg."Slug", tg."Description", tg."ReviewStatus"
		FROM "ImageTag" it JOIN "Tags" tg ON tg."Id" = it."TagsId"
		WHERE it."ImagesId" = ANY($1) AND `+tagCond+`
		ORDER BY it."ImagesId", tg."Id"`, tagArgs...).Query(func(rows pgx.Rows) error {
		for rows.Next() {
			var imageID int64
			var t tagDto
			var rs int
			if err := rows.Scan(&imageID, &t.ID, &t.Name, &t.Slug, &t.Description, &rs); err != nil {
				return err
			}
			t.ReviewStatus = reviewStatusString(rs)
			c.tags[imageID] = append(c.tags[imageID], t)
		}
		return rows.Err()
	})

	artistArgs := []any{ids}
	artistCond := childCondition("ar", opts, &artistArgs)
	b.Queue(`
		SELECT ai."ImagesId", ar."Id", ar."Name", ar."Patreon", ar."Pixiv", ar."Twitter", ar."DeviantArt", ar."ReviewStatus"
		FROM "ArtistImage" ai JOIN "Artists" ar ON ar."Id" = ai."ArtistsId"
		WHERE ai."ImagesId" = ANY($1) AND `+artistCond+`
		ORDER BY ai."ImagesId", ar."Id"`, artistArgs...).Query(func(rows pgx.Rows) error {
		for rows.Next() {
			var imageID int64
			var a artistDto
			var rs int
			var patreon, pixiv, twitter, deviantArt *string
			if err := rows.Scan(&imageID, &a.ID, &a.Name, &patreon, &pixiv, &twitter, &deviantArt, &rs); err != nil {
				return err
			}
			a.ReviewStatus = reviewStatusString(rs)
			a.Patreon = orEmpty(patreon)
			a.Pixiv = orEmpty(pixiv)
			a.Twitter = orEmpty(twitter)
			a.DeviantArt = orEmpty(deviantArt)
			c.artists[imageID] = append(c.artists[imageID], a)
		}
		return rows.Err()
	})

	if !opts.withUserData {
		return c
	}

	known, missing, stamp := s.favCounts.lookup(ids)
	for id, n := range known {
		c.favorites[id] = n
	}
	if len(missing) > 0 {
		b.Queue(`
			SELECT ai."ImageId", COUNT(*)
			FROM "AlbumItems" ai JOIN "Albums" al ON al."Id" = ai."AlbumId" AND al."IsDefault"
			WHERE ai."ImageId" = ANY($1)
			GROUP BY ai."ImageId"`, missing).Query(func(rows pgx.Rows) error {
			fetched := make(map[int64]int64, len(missing))
			for rows.Next() {
				var imageID, n int64
				if err := rows.Scan(&imageID, &n); err != nil {
					return err
				}
				c.favorites[imageID] = n
				fetched[imageID] = n
			}
			if err := rows.Err(); err != nil {
				return err
			}
			s.favCounts.store(stamp, missing, fetched)
			return nil
		})
	}

	if opts.ident != nil {
		b.Queue(`
			SELECT ai."ImageId", al."Id", al."Name", al."UserId", al."IsDefault", ai."AddedAt"
			FROM "AlbumItems" ai JOIN "Albums" al ON al."Id" = ai."AlbumId"
			WHERE ai."ImageId" = ANY($1) AND al."UserId" = $2
			ORDER BY ai."ImageId", al."IsDefault" DESC, al."Id"`, ids, opts.ident.UserID).Query(func(rows pgx.Rows) error {
			for rows.Next() {
				var imageID int64
				var ref imageAlbumRef
				var addedAt *time.Time
				if err := rows.Scan(&imageID, &ref.ID, &ref.Name, &ref.UserID, &ref.IsDefault, &addedAt); err != nil {
					return err
				}
				c.albums[imageID] = append(c.albums[imageID], ref)
				if ref.IsDefault && c.likedAt[imageID] == nil {
					c.likedAt[imageID] = addedAt
				}
			}
			return rows.Err()
		})
	}
	return c
}

func (s *Server) imageDtos(rows []imageRow, opts hydrateOptions, c *imageChildren) []imageDto {
	out := make([]imageDto, len(rows))
	last := make(map[int64]int, len(rows))
	for i, row := range rows {
		last[row.ID] = i
	}
	for i, row := range rows {
		dto := imageDto{
			ID:             row.ID,
			PerceptualHash: bitsToHex(row.PerceptualHash),
			Extension:      row.Extension,
			DominantColor:  row.DominantColor,
			Source:         row.Source,
			UploadedAt:     row.UploadedAt,
			IsNsfw:         row.IsNsfw,
			IsAnimated:     row.IsAnimated,
			Width:          row.Width,
			Height:         row.Height,
			ByteSize:       row.ByteSize,
			URL:            s.s3.PublicURL(imageKey(row.ID, row.Extension)),
			ReviewStatus:   reviewStatusString(row.ReviewStatus),
			Artists:        []artistDto{},
			Tags:           []tagDto{},
			Albums:         []imageAlbumRef{},
			AddedToAlbumAt: row.AddedAt,
		}
		if isModerator(opts.ident) ||
			(opts.showUploader && opts.ident != nil && row.UploaderID != nil && *row.UploaderID == opts.ident.UserID) {
			dto.UploaderID = row.UploaderID
		}
		if last[row.ID] == i {
			if t := c.tags[row.ID]; t != nil {
				dto.Tags = t
			}
			if a := c.artists[row.ID]; a != nil {
				dto.Artists = a
			}
			dto.Favorites = c.favorites[row.ID]
			if al := c.albums[row.ID]; al != nil {
				dto.Albums = al
			}
			dto.LikedAt = c.likedAt[row.ID]
		}
		out[i] = dto
	}
	return out
}

func imageRowIDs(rows []imageRow) []int64 {
	ids := make([]int64, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	return ids
}

func (s *Server) hydrateImages(ctx context.Context, q querier, rows []imageRow, opts hydrateOptions) ([]imageDto, error) {
	if len(rows) == 0 {
		return []imageDto{}, nil
	}
	b := &pgx.Batch{}
	children := s.queueImageChildren(b, imageRowIDs(rows), opts)
	if err := q.SendBatch(ctx, b).Close(); err != nil {
		return nil, err
	}
	return s.imageDtos(rows, opts, children), nil
}

func (s *Server) hydrateImage(ctx context.Context, q querier, row imageRow, opts hydrateOptions) (*imageDto, error) {
	dtos, err := s.hydrateImages(ctx, q, []imageRow{row}, opts)
	if err != nil {
		return nil, err
	}
	return &dtos[0], nil
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

func hashToBits(hash uint64) pgtype.Bits {
	v := reverseBytesPerByte(hash)
	bytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		bytes[i] = byte(v)
		v >>= 8
	}
	return pgtype.Bits{Bytes: bytes, Len: 64, Valid: true}
}
