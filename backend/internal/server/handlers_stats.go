package server

import (
	"net/http"
	"time"
)

func (s *Server) RegisterStatsRoutes() {
	s.Handle("GET /stats/public", http.HandlerFunc(s.handlePublicStats))
	s.Handle("GET /stats/admin", s.RequireRole("Admin")(http.HandlerFunc(s.handleAdminStats)))
}

func (s *Server) handlePublicStats(w http.ResponseWriter, r *http.Request) {
	var totalRequests int64
	var totalImages, totalTags, totalArtists int64
	_ = s.db.QueryRow(r.Context(), `SELECT COALESCE("Value",0) FROM "GlobalStats" WHERE "Key" = 'TotalRequests'`).Scan(&totalRequests)
	_ = s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM "Images"`).Scan(&totalImages)
	_ = s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM "Tags"`).Scan(&totalTags)
	_ = s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM "Artists"`).Scan(&totalArtists)

	writeJSON(w, http.StatusOK, map[string]any{
		"totalRequests": totalRequests,
		"totalImages":   totalImages,
		"totalTags":     totalTags,
		"totalArtists":  totalArtists,
	})
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

func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	today := time.Now().UTC().Format("2006-01-02")

	var requestsToday int64
	_ = s.db.QueryRow(r.Context(), `SELECT COALESCE("RequestCount",0) FROM "DailyStats" WHERE "Date" = $1::date`, today).Scan(&requestsToday)

	cutoff := time.Now().UTC().AddDate(-1, 0, 0).Format("2006-01-02")
	history := []dayStat{}
	rows, err := s.db.Query(r.Context(), `
		SELECT to_char("Date",'YYYY-MM-DD'), "RequestCount"
		FROM "DailyStats"
		WHERE "Date" >= $1::date
		ORDER BY "Date" ASC`, cutoff)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var d dayStat
			if rows.Scan(&d.Date, &d.Count) == nil {
				history = append(history, d)
			}
		}
	}

	topUsers := []userStatDto{}
	rows2, err := s.db.Query(r.Context(), `
		SELECT "Id","Name","RequestCount","ApiKeyRequestCount","JwtRequestCount"
		FROM "Users" ORDER BY "RequestCount" DESC LIMIT 10`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var u userStatDto
			if rows2.Scan(&u.ID, &u.Name, &u.RequestCount, &u.APIKeyRequestCount, &u.JWTRequestCount) == nil {
				topUsers = append(topUsers, u)
			}
		}
	}

	topAPIKeyUsers := []userStatDto{}
	rows3, err := s.db.Query(r.Context(), `
		SELECT "Id","Name","RequestCount","ApiKeyRequestCount","JwtRequestCount"
		FROM "Users" ORDER BY "ApiKeyRequestCount" DESC LIMIT 5`)
	if err == nil {
		defer rows3.Close()
		for rows3.Next() {
			var u userStatDto
			if rows3.Scan(&u.ID, &u.Name, &u.RequestCount, &u.APIKeyRequestCount, &u.JWTRequestCount) == nil {
				topAPIKeyUsers = append(topAPIKeyUsers, u)
			}
		}
	}

	topJWTUsers := []userStatDto{}
	rows4, err := s.db.Query(r.Context(), `
		SELECT "Id","Name","RequestCount","ApiKeyRequestCount","JwtRequestCount"
		FROM "Users" ORDER BY "JwtRequestCount" DESC LIMIT 5`)
	if err == nil {
		defer rows4.Close()
		for rows4.Next() {
			var u userStatDto
			if rows4.Scan(&u.ID, &u.Name, &u.RequestCount, &u.APIKeyRequestCount, &u.JWTRequestCount) == nil {
				topJWTUsers = append(topJWTUsers, u)
			}
		}
	}

	topUploaders := []uploaderStatDto{}
	rows5, err := s.db.Query(r.Context(), `
		SELECT u."Id", u."Name", COUNT(i."Id") AS uploaded
		FROM "Images" i JOIN "Users" u ON u."Id" = i."UploaderId"
		WHERE i."ReviewStatus" = 1
		GROUP BY u."Id", u."Name"
		ORDER BY uploaded DESC LIMIT 10`)
	if err == nil {
		defer rows5.Close()
		for rows5.Next() {
			var u uploaderStatDto
			if rows5.Scan(&u.ID, &u.Name, &u.UploadedImageCount) == nil {
				topUploaders = append(topUploaders, u)
			}
		}
	}

	topAlbumUsers := []albumUserStatDto{}
	rows6, err := s.db.Query(r.Context(), `
		SELECT u."Id", u."Name", COUNT(ai."ImageId") AS cnt
		FROM "Albums" a
		JOIN "Users" u ON u."Id" = a."UserId"
		LEFT JOIN "AlbumItems" ai ON ai."AlbumId" = a."Id"
		GROUP BY u."Id", u."Name"
		HAVING COUNT(ai."ImageId") > 0
		ORDER BY cnt DESC LIMIT 10`)
	if err == nil {
		defer rows6.Close()
		for rows6.Next() {
			var u albumUserStatDto
			if rows6.Scan(&u.ID, &u.Name, &u.AlbumImageCount) == nil {
				topAlbumUsers = append(topAlbumUsers, u)
			}
		}
	}

	var totalRequests, totalAuth, totalAPIKey, totalJWT int64
	_ = s.db.QueryRow(r.Context(), `SELECT COALESCE("Value",0) FROM "GlobalStats" WHERE "Key" = 'TotalRequests'`).Scan(&totalRequests)
	_ = s.db.QueryRow(r.Context(), `SELECT COALESCE(SUM("RequestCount"),0) FROM "Users"`).Scan(&totalAuth)
	_ = s.db.QueryRow(r.Context(), `SELECT COALESCE(SUM("ApiKeyRequestCount"),0) FROM "Users"`).Scan(&totalAPIKey)
	_ = s.db.QueryRow(r.Context(), `SELECT COALESCE(SUM("JwtRequestCount"),0) FROM "Users"`).Scan(&totalJWT)

	var totalImages, totalTags, totalArtists int64
	_ = s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM "Images"`).Scan(&totalImages)
	_ = s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM "Tags"`).Scan(&totalTags)
	_ = s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM "Artists"`).Scan(&totalArtists)

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
