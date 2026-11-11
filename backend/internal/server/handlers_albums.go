package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/waifu-im/waifu-api/backend/internal/auth"
)

func (s *Server) RegisterAlbumRoutes() {
	s.Handle("GET /users/{userId}/albums", s.RequireAuth(http.HandlerFunc(s.handleListAlbums)))
	s.Handle("POST /users/{userId}/albums", s.RequireAuth(http.HandlerFunc(s.handleCreateAlbum)))
	s.Handle("GET /users/{userId}/albums/{albumId}", s.RequireAuth(http.HandlerFunc(s.handleGetAlbum)))
	s.Handle("PATCH /users/{userId}/albums/{albumId}", s.RequireAuth(http.HandlerFunc(s.handlePatchAlbum)))
	s.Handle("DELETE /users/{userId}/albums/{albumId}", s.RequireAuth(http.HandlerFunc(s.handleDeleteAlbum)))
	s.Handle("GET /users/{userId}/albums/{albumId}/images", s.RequireAuth(http.HandlerFunc(s.handleListAlbumImages)))
	s.Handle("POST /users/{userId}/albums/{albumId}/images/{imageId}", s.RequireAuth(http.HandlerFunc(s.handleAddAlbumImage)))
	s.Handle("DELETE /users/{userId}/albums/{albumId}/images/{imageId}", s.RequireAuth(http.HandlerFunc(s.handleRemoveAlbumImage)))
	s.Handle("PATCH /users/{userId}/albums/{albumId}/images", s.RequireAuth(http.HandlerFunc(s.handleBatchAlbumImages)))
}

const albumBatchMax = 100

type albumBatchRequest struct {
	Add    []int64 `json:"add"`
	Remove []int64 `json:"remove"`
}

type albumBatchResult struct {
	Added      int64   `json:"added"`
	Removed    int64   `json:"removed"`
	Skipped    []int64 `json:"skipped"`
	ImageCount int64   `json:"imageCount"`
}

func uniquePositive(ids []int64) ([]int64, bool) {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, false
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, true
}

func (s *Server) handleBatchAlbumImages(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
	userID, ok := s.resolveAlbumUser(w, r, ident)
	if !ok {
		return
	}
	albumID, ok := s.resolveAlbumID(w, r, userID)
	if !ok {
		return
	}
	var req albumBatchRequest
	if err := decodeJSON(r, &req); err != nil {
		WriteValidationProblem(w, r, map[string][]string{"": {"The request could not be processed."}})
		return
	}
	add, okAdd := uniquePositive(req.Add)
	remove, okRemove := uniquePositive(req.Remove)
	errs := map[string][]string{}
	if !okAdd || len(add) > albumBatchMax {
		errs["Add"] = []string{fmt.Sprintf("Add must contain at most %d positive image IDs.", albumBatchMax)}
	}
	if !okRemove || len(remove) > albumBatchMax {
		errs["Remove"] = []string{fmt.Sprintf("Remove must contain at most %d positive image IDs.", albumBatchMax)}
	}
	if len(errs) == 0 && len(add) == 0 && len(remove) == 0 {
		errs[""] = []string{"Add or remove at least one image."}
	}
	if len(errs) > 0 {
		WriteValidationProblem(w, r, errs)
		return
	}

	owned, err := s.albumOwned(ctx, albumID, userID)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if !owned {
		NotFound(w, r, fmt.Sprintf("Album with ID %d not found.", albumID))
		return
	}

	res := albumBatchResult{Skipped: []int64{}}
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if len(add) > 0 {
			rows, err := tx.Query(ctx, `
				SELECT i."Id" FROM "Images" i
				WHERE i."Id" = ANY($1) AND (i."ReviewStatus" = 1 OR $2 OR i."UploaderId" = $3)`, add, isModerator(ident), ident.UserID)
			if err != nil {
				return err
			}
			visible, err := collectIDs(rows)
			if err != nil {
				return err
			}
			known := make(map[int64]struct{}, len(visible))
			for _, id := range visible {
				known[id] = struct{}{}
			}
			for _, id := range add {
				if _, ok := known[id]; !ok {
					res.Skipped = append(res.Skipped, id)
				}
			}
			if len(visible) > 0 {
				tag, err := tx.Exec(ctx, `
					INSERT INTO "AlbumItems" ("AlbumId","ImageId","AddedAt")
					SELECT $1, v.id, now() FROM unnest($2::bigint[]) AS v(id)
					ON CONFLICT DO NOTHING`, albumID, visible)
				if err != nil {
					return err
				}
				res.Added = tag.RowsAffected()
			}
		}
		if len(remove) > 0 {
			tag, err := tx.Exec(ctx, `DELETE FROM "AlbumItems" WHERE "AlbumId" = $1 AND "ImageId" = ANY($2)`, albumID, remove)
			if err != nil {
				return err
			}
			res.Removed = tag.RowsAffected()
		}
		return tx.QueryRow(ctx, `SELECT COUNT(*) FROM "AlbumItems" WHERE "AlbumId" = $1`, albumID).Scan(&res.ImageCount)
	})
	if isForeignKeyViolation(err) {
		Conflict(w, r, "One of the images was removed meanwhile. Please try again.")
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) resolveAlbumUser(w http.ResponseWriter, r *http.Request, ident *auth.Identity) (int64, bool) {
	raw := r.PathValue("userId")
	if strings.EqualFold(raw, "me") {
		return ident.UserID, true
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		BadRequest(w, r, "Invalid User ID format.")
		return 0, false
	}
	if id != ident.UserID && !isAdmin(ident) {
		EmptyNotFound(w)
		return 0, false
	}
	return id, true
}

func (s *Server) resolveAlbumID(w http.ResponseWriter, r *http.Request, userID int64) (int64, bool) {
	raw := r.PathValue("albumId")
	if strings.EqualFold(raw, "favorites") {
		id, err := s.ensureDefaultAlbum(r.Context(), userID)
		if err != nil {
			InternalError(w, r, s.log, err)
			return 0, false
		}
		return id, true
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		BadRequest(w, r, "Invalid Album ID format.")
		return 0, false
	}
	return id, true
}

func (s *Server) albumOwned(ctx context.Context, albumID, userID int64) (bool, error) {
	var owned bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "Albums" WHERE "Id" = $1 AND "UserId" = $2)`, albumID, userID).Scan(&owned)
	return owned, err
}

func (s *Server) ensureDefaultAlbum(ctx context.Context, userID int64) (int64, error) {
	var id int64
	err := s.db.QueryRow(ctx, `SELECT "Id" FROM "Albums" WHERE "UserId" = $1 AND "IsDefault" ORDER BY "Id" LIMIT 1`, userID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('default-album'), $1::int)`, int32(userID)); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT "Id" FROM "Albums" WHERE "UserId" = $1 AND "IsDefault" ORDER BY "Id" LIMIT 1`, userID).Scan(&id)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		return tx.QueryRow(ctx, `
			INSERT INTO "Albums" ("Name","Description","IsDefault","UserId")
			VALUES ('Favorites','',true,$1)
			RETURNING "Id"`, userID).Scan(&id)
	})
	return id, err
}

func (s *Server) handleListAlbums(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	userID, ok := s.resolveAlbumUser(w, r, ident)
	if !ok {
		return
	}
	page, pageSize := s.paginationParams(r, s.cfg.PageSizes.AlbumDefault, s.cfg.PageSizes.AlbumMax)

	var total int64
	if err := s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM "Albums" WHERE "UserId" = $1`, userID).Scan(&total); err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	rows, err := s.db.Query(r.Context(), `
		SELECT a."Id", a."Name", a."Description", a."IsDefault", a."UserId",
			(SELECT COUNT(*) FROM "AlbumItems" ai WHERE ai."AlbumId" = a."Id")
		FROM "Albums" a WHERE a."UserId" = $1
		ORDER BY a."IsDefault" DESC, a."Name" ASC, a."Id" ASC
		LIMIT $2 OFFSET $3`, userID, pageSize, offsetOf(page, pageSize))
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	defer rows.Close()

	items := []albumDto{}
	for rows.Next() {
		var a albumDto
		var desc *string
		if err := rows.Scan(&a.ID, &a.Name, &desc, &a.IsDefault, &a.UserID, &a.ImageCount); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		a.Description = orEmpty(desc)
		items = append(items, a)
	}
	if err := rows.Err(); err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, s.newPaginated(items, page, pageSize, total, s.cfg.PageSizes.AlbumMax, s.cfg.PageSizes.AlbumDefault))
}

func (s *Server) loadAlbum(ctx context.Context, albumID, userID int64) (*albumDto, error) {
	var a albumDto
	var desc *string
	err := s.db.QueryRow(ctx, `
		SELECT a."Id", a."Name", a."Description", a."IsDefault", a."UserId",
			(SELECT COUNT(*) FROM "AlbumItems" ai WHERE ai."AlbumId" = a."Id")
		FROM "Albums" a WHERE a."Id" = $1 AND a."UserId" = $2`, albumID, userID).
		Scan(&a.ID, &a.Name, &desc, &a.IsDefault, &a.UserID, &a.ImageCount)
	if err != nil {
		return nil, err
	}
	a.Description = orEmpty(desc)
	return &a, nil
}

func (s *Server) handleGetAlbum(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	userID, ok := s.resolveAlbumUser(w, r, ident)
	if !ok {
		return
	}
	albumID, ok := s.resolveAlbumID(w, r, userID)
	if !ok {
		return
	}
	a, err := s.loadAlbum(r.Context(), albumID, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, fmt.Sprintf("Album with ID %d not found.", albumID))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

type albumUpsertRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func validateAlbumCreate(req *albumUpsertRequest) map[string][]string {
	errs := map[string][]string{}
	if strings.TrimSpace(req.Name) == "" {
		errs["Name"] = append(errs["Name"], "Name is required.")
	} else if runeLen(req.Name) > 30 {
		errs["Name"] = append(errs["Name"], "Name must not exceed 30 characters.")
	}
	if runeLen(req.Description) > 150 {
		errs["Description"] = append(errs["Description"], "Description must not exceed 150 characters.")
	}
	return errs
}

func validateAlbumUpdate(req *albumUpsertRequest) map[string][]string {
	errs := map[string][]string{}
	if strings.TrimSpace(req.Name) == "" {
		errs["Name"] = append(errs["Name"], "Name is required.")
	} else if runeLen(req.Name) > 100 {
		errs["Name"] = append(errs["Name"], "Name must not exceed 100 characters.")
	}
	if runeLen(req.Description) > 150 {
		errs["Description"] = append(errs["Description"], "Description must not exceed 150 characters.")
	}
	return errs
}

func (s *Server) handleCreateAlbum(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	userID, ok := s.resolveAlbumUser(w, r, ident)
	if !ok {
		return
	}
	var req albumUpsertRequest
	if err := decodeJSON(r, &req); err != nil {
		WriteValidationProblem(w, r, map[string][]string{
			"": {"The request could not be processed."},
		})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if errs := validateAlbumCreate(&req); len(errs) > 0 {
		WriteValidationProblem(w, r, errs)
		return
	}

	ctx := r.Context()
	var a albumDto
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('albums'), $1::int)`, int32(userID)); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM "Albums" WHERE "UserId" = $1`, userID).Scan(&count); err != nil {
			return err
		}
		if count >= s.cfg.Limits.MaxAlbumsPerUser {
			return errConflict(fmt.Sprintf("Album limit reached (maximum %d albums).", s.cfg.Limits.MaxAlbumsPerUser))
		}
		return tx.QueryRow(ctx, `
			INSERT INTO "Albums" ("Name","Description","IsDefault","UserId")
			VALUES ($1,$2,false,$3) RETURNING "Id"`,
			req.Name, req.Description, userID).Scan(&a.ID)
	})
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	a.Name = req.Name
	a.Description = req.Description
	a.UserID = userID
	a.ImageCount = 0
	w.Header().Set("Location", fmt.Sprintf("/users/%d/albums/%d", userID, a.ID))
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) handlePatchAlbum(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	userID, ok := s.resolveAlbumUser(w, r, ident)
	if !ok {
		return
	}
	albumID, ok := s.resolveAlbumID(w, r, userID)
	if !ok {
		return
	}
	var req albumUpsertRequest
	if err := decodeJSON(r, &req); err != nil {
		WriteValidationProblem(w, r, map[string][]string{
			"": {"The request could not be processed."},
		})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if errs := validateAlbumUpdate(&req); len(errs) > 0 {
		WriteValidationProblem(w, r, errs)
		return
	}
	res, err := s.db.Exec(r.Context(), `
		UPDATE "Albums" SET "Name" = $1, "Description" = $2 WHERE "Id" = $3 AND "UserId" = $4`,
		req.Name, req.Description, albumID, userID)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if res.RowsAffected() == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("null"))
		return
	}
	a, err := s.loadAlbum(r.Context(), albumID, userID)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) handleDeleteAlbum(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	userID, ok := s.resolveAlbumUser(w, r, ident)
	if !ok {
		return
	}
	albumID, ok := s.resolveAlbumID(w, r, userID)
	if !ok {
		return
	}
	var isDefault bool
	err := s.db.QueryRow(r.Context(), `SELECT "IsDefault" FROM "Albums" WHERE "Id" = $1 AND "UserId" = $2`, albumID, userID).Scan(&isDefault)
	if errors.Is(err, pgx.ErrNoRows) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if isDefault {
		InvalidOperation(w, r, "Cannot delete the default album.")
		return
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM "Albums" WHERE "Id" = $1 AND "UserId" = $2 AND NOT "IsDefault"`, albumID, userID); err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListAlbumImages(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
	userID, ok := s.resolveAlbumUser(w, r, ident)
	if !ok {
		return
	}
	albumID, ok := s.resolveAlbumID(w, r, userID)
	if !ok {
		return
	}
	if !strings.EqualFold(r.PathValue("albumId"), "favorites") {
		owned, err := s.albumOwned(ctx, albumID, userID)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		if !owned {
			NotFound(w, r, fmt.Sprintf("Album with ID %d not found.", albumID))
			return
		}
	}

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
	if err := checkImageOrder(f, &albumID); err != nil {
		BadRequest(w, r, err.Error())
		return
	}

	rows, total, err := s.queryImagePage(ctx, f, ident, &albumID, page, pageSize)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	items, err := s.hydrateImages(ctx, s.db, rows, hydrateOptions{
		ident:                    ident,
		childrenReviewStatus:     f.childrenReviewStatus,
		includeMyPendingChildren: f.includeMyPendingChildren,
		withUserData:             true,
	})
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, s.newPaginated(items, page, pageSize, total, s.cfg.Image.MaxPageSize, s.cfg.Image.DefaultPageSize))
}

func (s *Server) handleAddAlbumImage(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
	userID, ok := s.resolveAlbumUser(w, r, ident)
	if !ok {
		return
	}
	albumID, ok := s.resolveAlbumID(w, r, userID)
	if !ok {
		return
	}
	imageID, err := parseID(r.PathValue("imageId"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}

	owned, err := s.albumOwned(ctx, albumID, userID)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if !owned {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var reviewStatus int
	var uploaderID *int64
	err = s.db.QueryRow(ctx, `SELECT "ReviewStatus", "UploaderId" FROM "Images" WHERE "Id" = $1`, imageID).Scan(&reviewStatus, &uploaderID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !canSeeContent(ident, reviewStatus, uploaderID)) {
		NotFound(w, r, fmt.Sprintf("Image with ID %d not found.", imageID))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO "AlbumItems" ("AlbumId","ImageId","AddedAt") VALUES ($1,$2,now())
		ON CONFLICT DO NOTHING`, albumID, imageID)
	if isForeignKeyViolation(err) {
		NotFound(w, r, fmt.Sprintf("Image with ID %d not found.", imageID))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRemoveAlbumImage(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
	userID, ok := s.resolveAlbumUser(w, r, ident)
	if !ok {
		return
	}
	albumID, ok := s.resolveAlbumID(w, r, userID)
	if !ok {
		return
	}
	imageID, err := parseID(r.PathValue("imageId"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	if _, err := s.db.Exec(ctx, `
		DELETE FROM "AlbumItems" ai USING "Albums" al
		WHERE ai."AlbumId" = al."Id" AND al."Id" = $1 AND al."UserId" = $2 AND ai."ImageId" = $3`,
		albumID, userID, imageID); err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
