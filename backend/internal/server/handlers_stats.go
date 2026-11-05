package server

import (
	"context"
	"net/http"
	"sync"
	"time"
)

func (s *Server) RegisterStatsRoutes() {
	s.Handle("GET /stats/public", http.HandlerFunc(s.handlePublicStats))
	s.Handle("GET /stats/admin", s.RequireRole("Admin")(http.HandlerFunc(s.handleAdminStats)))
}

type publicStatsCache struct {
	mu      sync.Mutex
	value   map[string]any
	expires time.Time
}

func (s *Server) handlePublicStats(w http.ResponseWriter, r *http.Request) {
	c := &s.publicStats
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.value == nil || time.Now().After(c.expires) {
		v, err := s.loadPublicStats(r.Context())
		if err != nil {
			if c.value == nil {
				InternalError(w, r, s.log, err)
				return
			}
			s.log.Warn("public stats refresh failed, serving cached values", "error", err)
		} else {
			c.value = v
			c.expires = time.Now().Add(time.Minute)
		}
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, http.StatusOK, c.value)
}

func (s *Server) loadPublicStats(ctx context.Context) (map[string]any, error) {
	var totalRequests, totalImages, totalTags, totalArtists int64
	err := s.db.QueryRow(ctx, `
		SELECT
			COALESCE((SELECT "Value" FROM "GlobalStats" WHERE "Key" = 'TotalRequests'), 0),
			(SELECT COUNT(*) FROM "Images" WHERE "ReviewStatus" = 1),
			(SELECT COUNT(*) FROM "Tags" WHERE "ReviewStatus" = 1),
			(SELECT COUNT(*) FROM "Artists" WHERE "ReviewStatus" = 1)`).
		Scan(&totalRequests, &totalImages, &totalTags, &totalArtists)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"totalRequests": totalRequests,
		"totalImages":   totalImages,
		"totalTags":     totalTags,
		"totalArtists":  totalArtists,
	}, nil
}

type userStatDto struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	RequestCount       int64  `json:"requestCount"`
	APIKeyRequestCount int64  `json:"apiKeyRequestCount"`
	JWTRequestCount    int64  `json:"jwtRequestCount"`
}

type uploaderStatDto struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	UploadedImageCount int64  `json:"uploadedImageCount"`
}

type albumUserStatDto struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	AlbumImageCount int64  `json:"albumImageCount"`
}

type dayStat struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

func (s *Server) topUsers(ctx context.Context, orderColumn string, limit int) ([]userStatDto, error) {
	out := []userStatDto{}
	rows, err := s.db.Query(ctx, `
		SELECT "Id", COALESCE("Name", ''), "RequestCount", "ApiKeyRequestCount", "JwtRequestCount"
		FROM "Users" ORDER BY `+orderColumn+` DESC, "Id" ASC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var u userStatDto
		if err := rows.Scan(&u.ID, &u.Name, &u.RequestCount, &u.APIKeyRequestCount, &u.JWTRequestCount); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fail := func(err error) { InternalError(w, r, s.log, err) }
	today := time.Now().UTC().Format("2006-01-02")

	var requestsToday int64
	if err := s.db.QueryRow(ctx, `SELECT COALESCE((SELECT "RequestCount" FROM "DailyStats" WHERE "Date" = $1::date), 0)`, today).Scan(&requestsToday); err != nil {
		fail(err)
		return
	}

	cutoff := time.Now().UTC().AddDate(-1, 0, 0).Format("2006-01-02")
	history := []dayStat{}
	rows, err := s.db.Query(ctx, `
		SELECT to_char("Date",'YYYY-MM-DD'), "RequestCount"
		FROM "DailyStats" WHERE "Date" >= $1::date ORDER BY "Date" ASC`, cutoff)
	if err != nil {
		fail(err)
		return
	}
	for rows.Next() {
		var d dayStat
		if err := rows.Scan(&d.Date, &d.Count); err != nil {
			rows.Close()
			fail(err)
			return
		}
		history = append(history, d)
	}
	rows.Close()

	topUsers, err := s.topUsers(ctx, `"RequestCount"`, 10)
	if err != nil {
		fail(err)
		return
	}
	topAPIKeyUsers, err := s.topUsers(ctx, `"ApiKeyRequestCount"`, 5)
	if err != nil {
		fail(err)
		return
	}
	topJWTUsers, err := s.topUsers(ctx, `"JwtRequestCount"`, 5)
	if err != nil {
		fail(err)
		return
	}

	topUploaders := []uploaderStatDto{}
	rows, err = s.db.Query(ctx, `
		SELECT u."Id", COALESCE(u."Name", ''), COUNT(i."Id") AS uploaded
		FROM "Images" i JOIN "Users" u ON u."Id" = i."UploaderId"
		WHERE i."ReviewStatus" = 1
		GROUP BY u."Id", u."Name"
		ORDER BY uploaded DESC, u."Id" ASC LIMIT 10`)
	if err != nil {
		fail(err)
		return
	}
	for rows.Next() {
		var u uploaderStatDto
		if err := rows.Scan(&u.ID, &u.Name, &u.UploadedImageCount); err != nil {
			rows.Close()
			fail(err)
			return
		}
		topUploaders = append(topUploaders, u)
	}
	rows.Close()

	topAlbumUsers := []albumUserStatDto{}
	rows, err = s.db.Query(ctx, `
		SELECT u."Id", COALESCE(u."Name", ''), COUNT(ai."ImageId") AS cnt
		FROM "Albums" a
		JOIN "Users" u ON u."Id" = a."UserId"
		LEFT JOIN "AlbumItems" ai ON ai."AlbumId" = a."Id"
		GROUP BY u."Id", u."Name"
		HAVING COUNT(ai."ImageId") > 0
		ORDER BY cnt DESC, u."Id" ASC LIMIT 10`)
	if err != nil {
		fail(err)
		return
	}
	for rows.Next() {
		var u albumUserStatDto
		if err := rows.Scan(&u.ID, &u.Name, &u.AlbumImageCount); err != nil {
			rows.Close()
			fail(err)
			return
		}
		topAlbumUsers = append(topAlbumUsers, u)
	}
	rows.Close()

	var totalRequests, totalAuth, totalAPIKey, totalJWT, totalImages, totalTags, totalArtists int64
	if err := s.db.QueryRow(ctx, `
		SELECT
			COALESCE((SELECT "Value" FROM "GlobalStats" WHERE "Key" = 'TotalRequests'), 0),
			COALESCE(SUM("RequestCount"), 0), COALESCE(SUM("ApiKeyRequestCount"), 0), COALESCE(SUM("JwtRequestCount"), 0),
			(SELECT COUNT(*) FROM "Images"), (SELECT COUNT(*) FROM "Tags"), (SELECT COUNT(*) FROM "Artists")
		FROM "Users"`).
		Scan(&totalRequests, &totalAuth, &totalAPIKey, &totalJWT, &totalImages, &totalTags, &totalArtists); err != nil {
		fail(err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"requestsToday":                requestsToday,
		"history":                      history,
		"topUsers":                     topUsers,
		"topApiKeyUsers":               topAPIKeyUsers,
		"topJwtUsers":                  topJWTUsers,
		"topUploaders":                 topUploaders,
		"topAlbumUsers":                topAlbumUsers,
		"totalRequests":                totalRequests,
		"totalAuthenticatedRequests":   totalAuth,
		"totalUnauthenticatedRequests": totalRequests - totalAuth,
		"totalApiKeyRequests":          totalAPIKey,
		"totalJwtRequests":             totalJWT,
		"totalImages":                  totalImages,
		"totalTags":                    totalTags,
		"totalArtists":                 totalArtists,
	})
}
