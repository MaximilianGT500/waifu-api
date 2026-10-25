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

func (s *Server) ensureDefaultAlbum(ctx context.Context, userID int64) (int64, error) {
	var id int64
	err := s.db.QueryRow(ctx, `SELECT "Id" FROM "Albums" WHERE "UserId" = $1 AND "IsDefault"`, userID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	err = s.db.QueryRow(ctx, `
		INSERT INTO "Albums" ("Name","Description","IsDefault","UserId")
		VALUES ('Favorites','',true,$1)
		ON CONFLICT DO NOTHING
		RETURNING "Id"`, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {

		err = s.db.QueryRow(ctx, `SELECT "Id" FROM "Albums" WHERE "UserId" = $1 AND "IsDefault"`, userID).Scan(&id)
	}
	return id, err
}

func (s *Server) handleListAlbums(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	userID, ok := s.resolveAlbumUser(w, r, ident)
	if !ok {
		return
	}
	page, pageSize := paginationParams(r, s.cfg.PageSizes.AlbumDefault, s.cfg.PageSizes.AlbumMax)

	var total int64
	_ = s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM "Albums" WHERE "UserId" = $1`, userID).Scan(&total)

	rows, err := s.db.Query(r.Context(), `
		SELECT a."Id", a."Name", a."Description", a."IsDefault", a."UserId",
			(SELECT COUNT(*) FROM "AlbumItems" ai WHERE ai."AlbumId" = a."Id")
		FROM "Albums" a WHERE a."UserId" = $1
		ORDER BY a."IsDefault" DESC, a."Name" ASC
		LIMIT $2 OFFSET $3`, userID, pageSize, (page-1)*pageSize)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	defer rows.Close()

	items := []albumDto{}
	for rows.Next() {
		var a albumDto
		if err := rows.Scan(&a.ID, &a.Name, &a.Description, &a.IsDefault, &a.UserID, &a.ImageCount); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		items = append(items, a)
	}
	writeJSON(w, http.StatusOK, newPaginated(items, page, pageSize, total, s.cfg.PageSizes.AlbumMax, s.cfg.PageSizes.AlbumDefault))
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
	var a albumDto
	err := s.db.QueryRow(r.Context(), `
		SELECT a."Id", a."Name", a."Description", a."IsDefault", a."UserId",
			(SELECT COUNT(*) FROM "AlbumItems" ai WHERE ai."AlbumId" = a."Id")
		FROM "Albums" a WHERE a."Id" = $1`, albumID).
		Scan(&a.ID, &a.Name, &a.Description, &a.IsDefault, &a.UserID, &a.ImageCount)
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
	} else if len(req.Name) > 30 {
		errs["Name"] = append(errs["Name"], "Name must not exceed 30 characters.")
	}
	if len(req.Description) > 150 {
		errs["Description"] = append(errs["Description"], "Description must not exceed 150 characters.")
	}
	return errs
}

func validateAlbumUpdate(req *albumUpsertRequest) map[string][]string {
	errs := map[string][]string{}
	if strings.TrimSpace(req.Name) == "" {
		errs["Name"] = append(errs["Name"], "Name is required.")
	} else if len(req.Name) > 100 {
		errs["Name"] = append(errs["Name"], "Name must not exceed 100 characters.")
	}
	if len(req.Description) > 60 {
		errs["Description"] = append(errs["Description"], "Description must not exceed 60 characters.")
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
	if errs := validateAlbumCreate(&req); len(errs) > 0 {
		WriteValidationProblem(w, r, errs)
		return
	}
	var a albumDto
	err := s.db.QueryRow(r.Context(), `
		INSERT INTO "Albums" ("Name","Description","IsDefault","UserId")
		VALUES ($1,$2,false,$3) RETURNING "Id"`,
		req.Name, req.Description, userID).Scan(&a.ID)
	if err != nil {
		InternalError(w, r, s.log, err)
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
	var a albumDto
	err = s.db.QueryRow(r.Context(), `
		SELECT a."Id", a."Name", a."Description", a."IsDefault", a."UserId",
			(SELECT COUNT(*) FROM "AlbumItems" ai WHERE ai."AlbumId" = a."Id")
		FROM "Albums" a WHERE a."Id" = $1`, albumID).
		Scan(&a.ID, &a.Name, &a.Description, &a.IsDefault, &a.UserID, &a.ImageCount)
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
	_, err = s.db.Exec(r.Context(), `DELETE FROM "Albums" WHERE "Id" = $1 AND "UserId" = $2`, albumID, userID)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListAlbumImages(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	userID, ok := s.resolveAlbumUser(w, r, ident)
	if !ok {
		return
	}
	albumID, ok := s.resolveAlbumID(w, r, userID)
	if !ok {
		return
	}
	f, problems := parseImageFilters(r, ident)
	if len(problems) > 0 {
		invalidEnumProblem(w, r, problems[0].field, problems[0].value)
		return
	}
	if f.reviewStatus != "Accepted" && !isModerator(ident) {
		Forbidden(w, r, "Filtering by non-accepted review status is only available to moderators and admins.")
		return
	}
	if f.childrenReviewStatus != "Accepted" && !isModerator(ident) {
		Forbidden(w, r, "Filtering by non-accepted review status is only available to moderators and admins.")
		return
	}
	page, pageSize := paginationParams(r, s.cfg.Image.DefaultPageSize, s.cfg.Image.MaxPageSize)

	where, whereArgs := buildImageWhere(f, ident, &albumID, 1)
	args := append([]any{albumID}, whereArgs...)

	order, err := orderClause(f, &albumID)
	if err != nil {
		BadRequest(w, r, err.Error())
		return
	}

	join := ` FROM "Images" i JOIN "AlbumItems" ai ON ai."ImageId" = i."Id" AND ai."AlbumId" = $1`
	var total int64
	if err := s.db.QueryRow(r.Context(), `SELECT COUNT(*)`+join+where, args...).Scan(&total); err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	query := `SELECT i."Id", i."PerceptualHash", i."Extension", i."DominantColor", i."Source",
		i."UploaderId", i."UploadedAt", i."IsNsfw", i."IsAnimated", i."Width", i."Height",
		i."ByteSize", i."ReviewStatus", ai."AddedAt"` + join + where + order
	query += fmt.Sprintf(" LIMIT %d OFFSET %d", pageSize, (page-1)*pageSize)

	rows, err := s.db.Query(r.Context(), query, args...)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	defer rows.Close()

	items := []imageDto{}
	for rows.Next() {
		var row imageRow
		if err := rows.Scan(&row.ID, &row.PerceptualHash, &row.Extension, &row.DominantColor, &row.Source,
			&row.UploaderID, &row.UploadedAt, &row.IsNsfw, &row.IsAnimated, &row.Width, &row.Height,
			&row.ByteSize, &row.ReviewStatus, &row.AddedAt); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		dto, err := s.assembleImageDto(r.Context(), row, ident, f.childrenReviewStatus, f.includeMyPendingChildren, &albumID, false)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		items = append(items, *dto)
	}
	writeJSON(w, http.StatusOK, newPaginated(items, page, pageSize, total, s.cfg.Image.MaxPageSize, s.cfg.Image.DefaultPageSize))
}

func (s *Server) handleAddAlbumImage(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
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

	var owned bool
	if err := s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM "Albums" WHERE "Id" = $1 AND "UserId" = $2)`, albumID, userID).Scan(&owned); err != nil || !owned {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	_, err = s.db.Exec(r.Context(), `
		INSERT INTO "AlbumItems" ("AlbumId","ImageId","AddedAt") VALUES ($1,$2,now())
		ON CONFLICT DO NOTHING`, albumID, imageID)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRemoveAlbumImage(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
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

	var owned bool
	if err := s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM "Albums" WHERE "Id" = $1 AND "UserId" = $2)`, albumID, userID).Scan(&owned); err != nil || !owned {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	_, _ = s.db.Exec(r.Context(), `DELETE FROM "AlbumItems" WHERE "AlbumId" = $1 AND "ImageId" = $2`, albumID, imageID)
	w.WriteHeader(http.StatusNoContent)
}
