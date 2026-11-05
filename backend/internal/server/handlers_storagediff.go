package server

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Server) RegisterStorageDiffRoutes() {
	s.Handle("GET /storage-diff", s.RequireRole("Admin")(http.HandlerFunc(s.handleGetStorageDiff)))
	s.Handle("DELETE /storage-diff/s3", s.RequireRole("Admin")(s.requireSession(adminSessionDetail, http.HandlerFunc(s.handleDeleteStorageDiffS3))))
	s.Handle("DELETE /storage-diff/db", s.RequireRole("Admin")(s.requireSession(adminSessionDetail, http.HandlerFunc(s.handleDeleteStorageDiffDB))))
}

const maxStorageDiffBatch = 1000

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
		key := imageKey(id, ext)
		out = append(out, key)
		byKey[key] = id
	}
	return out, byKey, rows.Err()
}

func (s *Server) handleGetStorageDiff(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rawMode := queryGet(r, "mode")
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
	page, pageSize := s.paginationParams(r, s.cfg.StorageDiff.DefaultPageSize, s.cfg.StorageDiff.MaxPageSize)
	ident := IdentityFromContext(ctx)

	keys, err := s.s3Keys(ctx)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	dbKeys, byKey, err := s.dbImageKeys(ctx)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	items := []storageDiffDto{}
	if mode == "OrphansInS3" {
		for _, k := range keys {
			if strings.HasPrefix(k, "healthcheck/") {
				continue
			}
			if _, ok := byKey[k]; !ok {
				items = append(items, storageDiffDto{FileName: k})
			}
		}
	} else {
		set := make(map[string]bool, len(keys))
		for _, k := range keys {
			set[k] = true
		}
		for _, k := range dbKeys {
			if !set[k] {
				id := byKey[k]
				items = append(items, storageDiffDto{ID: &id, FileName: k})
			}
		}
	}

	total := int64(len(items))
	start := offsetOf(page, pageSize)
	if start > len(items) {
		start = len(items)
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	pageItems := items[start:end]

	if mode == "MissingFromS3" && len(pageItems) > 0 {
		ids := make([]int64, 0, len(pageItems))
		for _, it := range pageItems {
			ids = append(ids, *it.ID)
		}
		rows, err := s.db.Query(ctx, `SELECT `+imageColumns+` FROM "Images" i WHERE i."Id" = ANY($1)`, ids)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		imageRows, err := collectImageRows(rows, false)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		dtos, err := s.hydrateImages(ctx, s.db, imageRows, hydrateOptions{ident: ident, childrenReviewStatus: "All", showUploader: true})
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		byID := map[int64]*imageDto{}
		for i := range dtos {
			byID[dtos[i].ID] = &dtos[i]
		}
		for i := range pageItems {
			if img, ok := byID[*pageItems[i].ID]; ok {
				pageItems[i].Image = img
			}
		}
	}

	writeJSON(w, http.StatusOK, s.newPaginated(pageItems, page, pageSize, total, s.cfg.StorageDiff.MaxPageSize, s.cfg.StorageDiff.DefaultPageSize))
}

func validStorageKey(k string) bool {
	if k == "" || len(k) > 512 || strings.HasPrefix(k, "/") || strings.Contains(k, "..") {
		return false
	}
	for _, c := range k {
		if c < 0x20 || c == 0x7f {
			return false
		}
	}
	return true
}

func (s *Server) handleDeleteStorageDiffS3(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
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
	names := *req.FileNames
	if len(names) > maxStorageDiffBatch {
		WriteValidationProblem(w, r, map[string][]string{
			"FileNames": {fmt.Sprintf("At most %d files can be deleted per request.", maxStorageDiffBatch)},
		})
		return
	}
	for _, name := range names {
		if !validStorageKey(name) {
			WriteValidationProblem(w, r, map[string][]string{
				"FileNames": {fmt.Sprintf("Invalid file name '%s'.", name)},
			})
			return
		}
	}

	_, byKey, err := s.dbImageKeys(ctx)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	for _, name := range names {
		if id, ok := byKey[name]; ok {
			Conflict(w, r, fmt.Sprintf("File '%s' is still used by image %d and cannot be deleted here.", name, id))
			return
		}
	}

	deleted := make([]string, 0, len(names))
	defer func() { s.cdn.purge(deleted) }()
	for _, name := range names {
		release, inUse, err := s.guardStoredObject(ctx, name)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		if inUse {
			release()
			Conflict(w, r, fmt.Sprintf("File '%s' is still used by an image and cannot be deleted here.", name))
			return
		}
		err = s.s3.Delete(ctx, name)
		release()
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		deleted = append(deleted, name)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteStorageDiffDB(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
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
	ids := *req.ImageIDs
	if len(ids) > maxStorageDiffBatch {
		WriteValidationProblem(w, r, map[string][]string{
			"ImageIds": {fmt.Sprintf("At most %d images can be deleted per request.", maxStorageDiffBatch)},
		})
		return
	}
	if len(ids) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	rows, err := s.db.Query(ctx, `SELECT "Id", "Extension" FROM "Images" WHERE "Id" = ANY($1)`, ids)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	existing := map[int64]string{}
	for rows.Next() {
		var id int64
		var ext string
		if err := rows.Scan(&id, &ext); err != nil {
			rows.Close()
			InternalError(w, r, s.log, err)
			return
		}
		existing[id] = ext
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	toDelete := make([]int64, 0, len(existing))
	for id, ext := range existing {
		present, err := s.s3.Exists(ctx, imageKey(id, ext))
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		if present {
			Conflict(w, r, "Image "+strconv.FormatInt(id, 10)+" still has a file in storage and cannot be deleted here.")
			return
		}
		toDelete = append(toDelete, id)
	}

	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE "Reports" SET "Status" = 1, "ReviewerNote" = 'The reported image was deleted by a moderator.'
			WHERE "ImageId" = ANY($1) AND "Status" = 0`, toDelete); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM "Images" WHERE "Id" = ANY($1)`, toDelete)
		return err
	})
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
