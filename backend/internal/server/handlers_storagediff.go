package server

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/waifu-im/waifu-api/backend/internal/auth"
)

func (s *Server) RegisterStorageDiffRoutes() {
	s.Handle("GET /storage-diff", s.RequireRole("Admin")(http.HandlerFunc(s.handleGetStorageDiff)))
	s.Handle("DELETE /storage-diff/s3", s.RequireRole("Admin")(http.HandlerFunc(s.handleDeleteStorageDiffS3)))
	s.Handle("DELETE /storage-diff/db", s.RequireRole("Admin")(http.HandlerFunc(s.handleDeleteStorageDiffDB)))
}

func (s *Server) s3Keys(ctx context.Context) ([]string, error) {
	set := map[string]bool{}
	_, err := s.s3.List(ctx, func(key string) bool {
		set[key] = true
		return true
	})
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *Server) dbImageKeys(ctx context.Context) ([]string, map[string]int64, error) {
	out := []string{}
	byKey := map[string]int64{}
	rows, err := s.db.Query(ctx, `SELECT "Id", "Extension" FROM "Images" ORDER BY "Id"`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var ext string
		if err := rows.Scan(&id, &ext); err != nil {
			return nil, nil, err
		}
		key := fmt.Sprintf("%d%s", id, ext)
		out = append(out, key)
		byKey[key] = id
	}
	return out, byKey, nil
}

func (s *Server) handleGetStorageDiff(w http.ResponseWriter, r *http.Request) {
	rawMode := r.URL.Query().Get("mode")
	mode := ""
	switch {
	case strings.EqualFold(rawMode, "OrphansInS3"):
		mode = "OrphansInS3"
	case strings.EqualFold(rawMode, "MissingFromS3"):
		mode = "MissingFromS3"
	default:
		if rawMode == "" {
			WriteValidationProblem(w, r, map[string][]string{
				"Mode": {"The Mode field is required."},
			})
			return
		}
		invalidEnumProblem(w, r, "mode", rawMode)
		return
	}
	page, pageSize := paginationParams(r, s.cfg.StorageDiff.DefaultPageSize, s.cfg.StorageDiff.MaxPageSize)

	ident := IdentityFromContext(r.Context())
	items := []storageDiffDto{}

	if mode == "OrphansInS3" {
		keys, err := s.s3Keys(r.Context())
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		_, dbKeys, err := s.dbImageKeys(r.Context())
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		for _, k := range keys {
			if strings.HasPrefix(k, "healthcheck/") {
				continue
			}
			if _, ok := dbKeys[k]; !ok {
				items = append(items, storageDiffDto{FileName: k})
			}
		}
	} else {
		s3Set, err := s.s3Keys(r.Context())
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		set := map[string]bool{}
		for _, k := range s3Set {
			set[k] = true
		}
		dbKeys, _, err := s.dbImageKeys(r.Context())
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		for _, k := range dbKeys {
			if !set[k] {
				id := idFromKey(k)
				img := s.loadStorageDiffImage(r, ident, id)
				items = append(items, storageDiffDto{ID: &id, FileName: k, Image: img})
			}
		}
	}

	total := int64(len(items))
	start := (page - 1) * pageSize
	if start > len(items) {
		start = len(items)
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	writeJSON(w, http.StatusOK, newPaginated(items[start:end], page, pageSize, total, s.cfg.StorageDiff.MaxPageSize, s.cfg.StorageDiff.DefaultPageSize))
}

func idFromKey(key string) int64 {
	var n int64
	for _, c := range key {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int64(c-'0')
	}
	return n
}

func (s *Server) loadStorageDiffImage(r *http.Request, ident *auth.Identity, imageID int64) *imageDto {
	var row imageRow
	err := s.db.QueryRow(r.Context(), `
		SELECT "Id","PerceptualHash","Extension","DominantColor","Source","UploaderId","UploadedAt",
			"IsNsfw","IsAnimated","Width","Height","ByteSize","ReviewStatus"
		FROM "Images" WHERE "Id" = $1`, imageID).
		Scan(&row.ID, &row.PerceptualHash, &row.Extension, &row.DominantColor, &row.Source, &row.UploaderID,
			&row.UploadedAt, &row.IsNsfw, &row.IsAnimated, &row.Width, &row.Height, &row.ByteSize, &row.ReviewStatus)
	if err != nil {
		return nil
	}
	dto, err := s.assembleImageDto(r.Context(), row, ident, "All", false, nil, true)
	if err != nil {
		return nil
	}
	dto.Favorites = 0
	dto.LikedAt = nil
	dto.Albums = []imageAlbumRef{}
	return dto
}

func (s *Server) handleDeleteStorageDiffS3(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FileNames *[]string `json:"fileNames"`
	}
	if err := decodeJSON(r, &req); err != nil {
		WriteValidationProblem(w, r, map[string][]string{
			"": {"The request could not be processed."},
		})
		return
	}
	if req.FileNames == nil {
		WriteValidationProblem(w, r, map[string][]string{
			"FileNames": {"The FileNames field is required."},
		})
		return
	}
	for _, name := range *req.FileNames {
		if err := s.s3.Delete(r.Context(), name); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
	}
	s.purgeCDN(*req.FileNames)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteStorageDiffDB(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ImageIDs *[]int64 `json:"imageIds"`
	}
	if err := decodeJSON(r, &req); err != nil {
		WriteValidationProblem(w, r, map[string][]string{
			"": {"The request could not be processed."},
		})
		return
	}
	if req.ImageIDs == nil {
		WriteValidationProblem(w, r, map[string][]string{
			"ImageIds": {"The ImageIds field is required."},
		})
		return
	}
	for _, id := range *req.ImageIDs {

		_, _ = s.db.Exec(r.Context(), `
			UPDATE "Reports" SET "Status" = 1, "ReviewerNote" = 'The reported image was deleted by a moderator.'
			WHERE "ImageId" = $1 AND "Status" = 0`, id)

		_, err := s.db.Exec(r.Context(), `DELETE FROM "Images" WHERE "Id" = $1`, id)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
