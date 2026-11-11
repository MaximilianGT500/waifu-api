package server

import (
	"context"
	"strconv"
	"strings"
	"time"
)

const (
	defaultColorTolerance = 15
	maxColorDistance      = 765.0
)

type rgbColor struct {
	r, g, b int
}

func parseHexColor(v string) (rgbColor, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "#")
	if len(v) != 6 {
		return rgbColor{}, false
	}
	n, err := strconv.ParseUint(v, 16, 32)
	if err != nil {
		return rgbColor{}, false
	}
	return rgbColor{r: int(n >> 16 & 255), g: int(n >> 8 & 255), b: int(n & 255)}, true
}

func (c rgbColor) hex() string {
	const digits = "0123456789ABCDEF"
	out := []byte{'#', 0, 0, 0, 0, 0, 0}
	for i, v := range []int{c.r, c.g, c.b} {
		out[1+i*2] = digits[v>>4]
		out[2+i*2] = digits[v&15]
	}
	return string(out)
}

func colorLimitSq(tolerance int) float64 {
	limit := float64(tolerance) * maxColorDistance / 100
	return limit * limit
}

func colorDistanceSq(a, b rgbColor) float64 {
	rm := float64(a.r+b.r) / 2
	dr, dg, db := float64(a.r-b.r), float64(a.g-b.g), float64(a.b-b.b)
	return (2+rm/256)*dr*dr + 4*dg*dg + (2+(255-rm)/256)*db*db
}

func colorCondition(c rgbColor, tolerance int, a *sqlArgs) string {
	v := `('x' || substr(i."DominantColor", 2, 6))::bit(24)::int`
	r := `(((` + v + `) >> 16) & 255)`
	g := `(((` + v + `) >> 8) & 255)`
	b := `((` + v + `) & 255)`
	R, G, B := a.add(c.r), a.add(c.g), a.add(c.b)
	sum := `(` + r + ` + ` + R + `::int)::float8`
	dist := `(2 + ` + sum + ` / 512) * power(` + r + ` - ` + R + `::int, 2)` +
		` + 4 * power(` + g + ` - ` + G + `::int, 2)` +
		` + (2 + (255 - ` + sum + ` / 2) / 256) * power(` + b + ` - ` + B + `::int, 2)`
	return `(CASE WHEN i."DominantColor" ~ '^#[0-9A-Fa-f]{6}$' THEN ` + dist + ` <= ` + a.add(colorLimitSq(tolerance)) + `::float8 ELSE false END)`
}

type colorSnapshot struct {
	rgb map[int64]rgbColor
}

const colorSnapshotQuery = `SELECT "Id", "DominantColor" FROM "Images" WHERE "ReviewStatus" = 1`

func (s *Server) loadColors(ctx context.Context) (*colorSnapshot, error) {
	build := func(ctx context.Context) (*colorSnapshot, int, error) {
		rows, err := s.db.Query(ctx, colorSnapshotQuery)
		if err != nil {
			return nil, 0, err
		}
		defer rows.Close()
		snap := &colorSnapshot{rgb: map[int64]rgbColor{}}
		for rows.Next() {
			var id int64
			var hex string
			if err := rows.Scan(&id, &hex); err != nil {
				return nil, 0, err
			}
			if c, ok := parseHexColor(hex); ok {
				snap.rgb[id] = c
			}
		}
		return snap, 1, rows.Err()
	}
	if !s.colorIndex.enabled() {
		snap, _, err := build(ctx)
		return snap, err
	}
	return s.colorIndex.load(ctx, "colors", 0, build)
}

func filterByColor(ids []int64, snap *colorSnapshot, target rgbColor, tolerance int) []int64 {
	limit := colorLimitSq(tolerance)
	out := make([]int64, 0, len(ids)/8)
	for _, id := range ids {
		if c, ok := snap.rgb[id]; ok && colorDistanceSq(c, target) <= limit {
			out = append(out, id)
		}
	}
	return out
}

func parseTimeParam(v string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

func truncateRunes(v string, n int) string {
	if runeLen(v) <= n {
		return v
	}
	return string([]rune(v)[:n]) + "…"
}
