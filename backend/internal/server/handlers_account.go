package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Server) RegisterAccountRoutes() {
	s.Handle("GET /users/me/summary", s.RequireAuth(http.HandlerFunc(s.handleMySummary)))
	s.Handle("GET /users/me/export", s.RequireAuth(http.HandlerFunc(s.handleExportMe)))
}

type statusCounts struct {
	Accepted int64 `json:"accepted"`
	Pending  int64 `json:"pending"`
	Rejected int64 `json:"rejected"`
}

type openCounts struct {
	Pending int64 `json:"pending"`
	Total   int64 `json:"total"`
}

type requestCounts struct {
	Total  int64 `json:"total"`
	APIKey int64 `json:"apiKey"`
	JWT    int64 `json:"jwt"`
}

type accountSummary struct {
	Uploads     statusCounts  `json:"uploads"`
	Favorites   int64         `json:"favorites"`
	Albums      int64         `json:"albums"`
	AlbumImages int64         `json:"albumImages"`
	APIKeys     int64         `json:"apiKeys"`
	Reports     openCounts    `json:"reports"`
	Submissions openCounts    `json:"submissions"`
	Requests    requestCounts `json:"requests"`
}

func (s *Server) handleMySummary(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	var res accountSummary
	err := s.db.QueryRow(r.Context(), `
		SELECT
			(SELECT COUNT(*) FROM "Images" WHERE "UploaderId" = $1 AND "ReviewStatus" = 1),
			(SELECT COUNT(*) FROM "Images" WHERE "UploaderId" = $1 AND "ReviewStatus" = 0),
			(SELECT COUNT(*) FROM "Images" WHERE "UploaderId" = $1 AND "ReviewStatus" = 2),
			(SELECT COUNT(*) FROM "AlbumItems" ai JOIN "Albums" al ON al."Id" = ai."AlbumId" WHERE al."UserId" = $1 AND al."IsDefault"),
			(SELECT COUNT(*) FROM "Albums" WHERE "UserId" = $1),
			(SELECT COUNT(*) FROM "AlbumItems" ai JOIN "Albums" al ON al."Id" = ai."AlbumId" WHERE al."UserId" = $1),
			(SELECT COUNT(*) FROM "ApiKeys" WHERE "UserId" = $1),
			(SELECT COUNT(*) FROM "Reports" WHERE "UserId" = $1 AND "Status" = 0),
			(SELECT COUNT(*) FROM "Reports" WHERE "UserId" = $1),
			(SELECT COUNT(*) FROM "ReviewTasks" WHERE "SubmitterId" = $1 AND "Status" = 0),
			(SELECT COUNT(*) FROM "ReviewTasks" WHERE "SubmitterId" = $1),
			u."RequestCount", u."ApiKeyRequestCount", u."JwtRequestCount"
		FROM "Users" u WHERE u."Id" = $1`, ident.UserID).Scan(
		&res.Uploads.Accepted, &res.Uploads.Pending, &res.Uploads.Rejected,
		&res.Favorites, &res.Albums, &res.AlbumImages, &res.APIKeys,
		&res.Reports.Pending, &res.Reports.Total, &res.Submissions.Pending, &res.Submissions.Total,
		&res.Requests.Total, &res.Requests.APIKey, &res.Requests.JWT)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, fmt.Sprintf("User with ID %d not found.", ident.UserID))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type exportAlbumItem struct {
	ImageID int64     `json:"imageId"`
	AddedAt time.Time `json:"addedAt"`
}

type exportAlbum struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	IsDefault   bool              `json:"isDefault"`
	Images      []exportAlbumItem `json:"images"`
}

type exportAPIKey struct {
	ID             int64      `json:"id"`
	Description    string     `json:"description"`
	CreatedAt      time.Time  `json:"createdAt"`
	LastUsedAt     *time.Time `json:"lastUsedAt"`
	ExpirationDate *time.Time `json:"expirationDate"`
}

type exportUpload struct {
	ID           int64     `json:"id"`
	URL          string    `json:"url"`
	Source       *string   `json:"source"`
	IsNsfw       bool      `json:"isNsfw"`
	UploadedAt   time.Time `json:"uploadedAt"`
	ReviewStatus string    `json:"reviewStatus"`
}

type exportNamed struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	Slug         *string `json:"slug,omitempty"`
	ReviewStatus string  `json:"reviewStatus"`
}

type exportReport struct {
	ID           int64     `json:"id"`
	ImageID      *int64    `json:"imageId"`
	Description  string    `json:"description"`
	Status       string    `json:"status"`
	ReviewerNote *string   `json:"reviewerNote"`
	CreatedAt    time.Time `json:"createdAt"`
}

type exportReviewTask struct {
	ID           int64           `json:"id"`
	Kind         string          `json:"kind"`
	TargetType   string          `json:"targetType"`
	TargetID     int64           `json:"targetId"`
	Status       string          `json:"status"`
	Reason       *string         `json:"reason"`
	ReviewerNote *string         `json:"reviewerNote"`
	Payload      json.RawMessage `json:"payload"`
	CreatedAt    time.Time       `json:"createdAt"`
	ResolvedAt   *time.Time      `json:"resolvedAt"`
}

type accountExport struct {
	ExportedAt     time.Time          `json:"exportedAt"`
	Service        string             `json:"service"`
	User           *userDto           `json:"user"`
	Albums         []*exportAlbum     `json:"albums"`
	APIKeys        []exportAPIKey     `json:"apiKeys"`
	Uploads        []exportUpload     `json:"uploads"`
	TagsCreated    []exportNamed      `json:"tagsCreated"`
	ArtistsCreated []exportNamed      `json:"artistsCreated"`
	Reports        []exportReport     `json:"reports"`
	ReviewTasks    []exportReviewTask `json:"reviewTasks"`
}

func (s *Server) handleExportMe(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
	uid := ident.UserID
	out := accountExport{
		ExportedAt:     time.Now().UTC(),
		Service:        s.cfg.AppName,
		Albums:         []*exportAlbum{},
		APIKeys:        []exportAPIKey{},
		Uploads:        []exportUpload{},
		TagsCreated:    []exportNamed{},
		ArtistsCreated: []exportNamed{},
		Reports:        []exportReport{},
		ReviewTasks:    []exportReviewTask{},
	}
	albums := map[int64]*exportAlbum{}
	var userErr error
	b := &pgx.Batch{}
	b.Queue(`SELECT `+userColumns+` FROM "Users" u WHERE u."Id" = $1`, uid).QueryRow(func(row pgx.Row) error {
		out.User, userErr = scanUser(row)
		if errors.Is(userErr, pgx.ErrNoRows) {
			return nil
		}
		return userErr
	})
	b.Queue(`SELECT "Id", "Name", "Description", "IsDefault" FROM "Albums" WHERE "UserId" = $1 ORDER BY "IsDefault" DESC, "Id"`, uid).Query(func(rows pgx.Rows) error {
		for rows.Next() {
			a := &exportAlbum{Images: []exportAlbumItem{}}
			var desc *string
			if err := rows.Scan(&a.ID, &a.Name, &desc, &a.IsDefault); err != nil {
				return err
			}
			a.Description = orEmpty(desc)
			albums[a.ID] = a
			out.Albums = append(out.Albums, a)
		}
		return rows.Err()
	})
	b.Queue(`
		SELECT ai."AlbumId", ai."ImageId", ai."AddedAt"
		FROM "AlbumItems" ai JOIN "Albums" al ON al."Id" = ai."AlbumId"
		WHERE al."UserId" = $1 ORDER BY ai."AlbumId", ai."AddedAt" DESC`, uid).Query(func(rows pgx.Rows) error {
		for rows.Next() {
			var albumID int64
			var item exportAlbumItem
			if err := rows.Scan(&albumID, &item.ImageID, &item.AddedAt); err != nil {
				return err
			}
			if a := albums[albumID]; a != nil {
				a.Images = append(a.Images, item)
			}
		}
		return rows.Err()
	})
	b.Queue(`SELECT "Id", "Description", "CreatedAt", "LastUsedAt", "ExpirationDate" FROM "ApiKeys" WHERE "UserId" = $1 ORDER BY "Id"`, uid).Query(func(rows pgx.Rows) error {
		for rows.Next() {
			var k exportAPIKey
			var desc *string
			if err := rows.Scan(&k.ID, &desc, &k.CreatedAt, &k.LastUsedAt, &k.ExpirationDate); err != nil {
				return err
			}
			k.Description = orEmpty(desc)
			out.APIKeys = append(out.APIKeys, k)
		}
		return rows.Err()
	})
	b.Queue(`SELECT "Id", "Extension", "Source", "IsNsfw", "UploadedAt", "ReviewStatus" FROM "Images" WHERE "UploaderId" = $1 ORDER BY "Id"`, uid).Query(func(rows pgx.Rows) error {
		for rows.Next() {
			var u exportUpload
			var ext string
			var rs int
			if err := rows.Scan(&u.ID, &ext, &u.Source, &u.IsNsfw, &u.UploadedAt, &rs); err != nil {
				return err
			}
			u.URL = s.s3.PublicURL(imageKey(u.ID, ext))
			u.ReviewStatus = reviewStatusString(rs)
			out.Uploads = append(out.Uploads, u)
		}
		return rows.Err()
	})
	b.Queue(`SELECT "Id", "Name", "Slug", "ReviewStatus" FROM "Tags" WHERE "CreatorId" = $1 ORDER BY "Id"`, uid).Query(func(rows pgx.Rows) error {
		for rows.Next() {
			var t exportNamed
			var rs int
			if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &rs); err != nil {
				return err
			}
			t.ReviewStatus = reviewStatusString(rs)
			out.TagsCreated = append(out.TagsCreated, t)
		}
		return rows.Err()
	})
	b.Queue(`SELECT "Id", "Name", "ReviewStatus" FROM "Artists" WHERE "CreatorId" = $1 ORDER BY "Id"`, uid).Query(func(rows pgx.Rows) error {
		for rows.Next() {
			var a exportNamed
			var rs int
			if err := rows.Scan(&a.ID, &a.Name, &rs); err != nil {
				return err
			}
			a.ReviewStatus = reviewStatusString(rs)
			out.ArtistsCreated = append(out.ArtistsCreated, a)
		}
		return rows.Err()
	})
	b.Queue(`SELECT "Id", "ImageId", "Description", "Status", "ReviewerNote", "CreatedAt" FROM "Reports" WHERE "UserId" = $1 ORDER BY "Id"`, uid).Query(func(rows pgx.Rows) error {
		for rows.Next() {
			var rep exportReport
			var desc *string
			var st int
			if err := rows.Scan(&rep.ID, &rep.ImageID, &desc, &st, &rep.ReviewerNote, &rep.CreatedAt); err != nil {
				return err
			}
			rep.Description = orEmpty(desc)
			rep.Status = reportStatusString(st)
			out.Reports = append(out.Reports, rep)
		}
		return rows.Err()
	})
	b.Queue(`
		SELECT "Id", "Kind", "TargetType", "TargetId", "Status", "Reason", "ReviewerNote", "Payload", "CreatedAt", "ResolvedAt"
		FROM "ReviewTasks" WHERE "SubmitterId" = $1 ORDER BY "Id"`, uid).Query(func(rows pgx.Rows) error {
		for rows.Next() {
			var t exportReviewTask
			var kind, target, st int
			var payload []byte
			if err := rows.Scan(&t.ID, &kind, &target, &t.TargetID, &st, &t.Reason, &t.ReviewerNote, &payload, &t.CreatedAt, &t.ResolvedAt); err != nil {
				return err
			}
			t.Kind = reviewTaskKindString(kind)
			t.TargetType = targetTypeString(target)
			t.Status = reviewTaskStatusString(st)
			if len(payload) > 0 {
				t.Payload = json.RawMessage(payload)
			} else {
				t.Payload = json.RawMessage("null")
			}
			out.ReviewTasks = append(out.ReviewTasks, t)
		}
		return rows.Err()
	})
	if err := s.db.SendBatch(ctx, b).Close(); err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if out.User == nil {
		NotFound(w, r, fmt.Sprintf("User with ID %d not found.", uid))
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="account-export-`+strconv.FormatInt(uid, 10)+`.json"`)
	writeJSON(w, http.StatusOK, out)
}
