package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/waifu-im/waifu-api/backend/internal/auth"
)

func (s *Server) RegisterReportRoutes() {
	s.Handle("POST /reports", s.RequireAuth(http.HandlerFunc(s.handleCreateReport)))
	s.Handle("GET /reports", s.RequireAuth(http.HandlerFunc(s.handleListReports)))
	s.Handle("PATCH /reports/{id}", s.RequireAuth(http.HandlerFunc(s.handlePatchReport)))
	s.Handle("PATCH /reports/{id}/validate", s.RequireRole("Moderator")(http.HandlerFunc(s.handleValidateReport)))
	s.Handle("PATCH /reports/{id}/reject", s.RequireRole("Moderator")(http.HandlerFunc(s.handleRejectReport)))
	s.Handle("PATCH /reports/{id}/cancel", s.RequireAuth(http.HandlerFunc(s.handleCancelReport)))
}

func reportStatusInt(s string) (int, bool) {
	switch {
	case strings.EqualFold(s, "Pending"):
		return 0, true
	case strings.EqualFold(s, "Resolved"):
		return 1, true
	case strings.EqualFold(s, "Rejected"):
		return 2, true
	case strings.EqualFold(s, "Cancelled"):
		return 3, true
	}
	return 0, false
}

func (s *Server) handleCreateReport(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	var req struct {
		ImageID     int64  `json:"imageId"`
		Description string `json:"description"`
	}
	if err := decodeJSON(r, &req); err != nil {
		WriteValidationProblem(w, r, map[string][]string{
			"": {"The request could not be processed."},
		})
		return
	}
	errs := map[string][]string{}
	if req.ImageID <= 0 {
		errs["ImageId"] = append(errs["ImageId"], "Invalid Image ID.")
	}
	if req.Description == "" {
		errs["Description"] = append(errs["Description"], "Description is required.")
	} else if len(req.Description) > 500 {
		errs["Description"] = append(errs["Description"], "Description must not exceed 500 characters.")
	}
	if len(errs) > 0 {
		WriteValidationProblem(w, r, errs)
		return
	}

	var existing bool
	_ = s.db.QueryRow(r.Context(), `
		SELECT EXISTS(SELECT 1 FROM "Reports" WHERE "UserId" = $1 AND "ImageId" = $2 AND "Status" = 0)`,
		ident.UserID, req.ImageID).Scan(&existing)
	if existing {
		Conflict(w, r, "You already have a pending report for this image.")
		return
	}

	var repID int64
	var createdAt time.Time
	err := s.db.QueryRow(r.Context(), `
		INSERT INTO "Reports" ("UserId","ImageId","Description","CreatedAt","Status")
		VALUES ($1,$2,$3,now(),0) RETURNING "Id","CreatedAt"`,
		ident.UserID, req.ImageID, req.Description).Scan(&repID, &createdAt)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	rep := reportDto{
		ID:           repID,
		UserID:       ident.UserID,
		User:         nil,
		ImageID:      &req.ImageID,
		Image:        nil,
		Description:  req.Description,
		Status:       "Pending",
		ReviewerNote: nil,
		CreatedAt:    createdAt,
	}
	w.Header().Set("Location", fmt.Sprintf("/reports?id=%d", rep.ID))
	writeJSON(w, http.StatusCreated, rep)
}

func (s *Server) handleListReports(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	page, pageSize := paginationParams(r, s.cfg.PageSizes.ReportDefault, s.cfg.PageSizes.ReportMax)

	where := " WHERE TRUE"
	args := []any{}
	ph := func() string { return fmt.Sprintf("$%d", len(args)+1) }
	if st := r.URL.Query().Get("status"); st != "" {
		statusInt, ok := reportStatusInt(st)
		if !ok {
			invalidEnumProblem(w, r, "status", st)
			return
		}
		where += fmt.Sprintf(` AND r."Status" = %s`, ph())
		args = append(args, statusInt)
	}
	mine := queryBool(r, "mine", false)
	if isModerator(ident) && mine {
		where += fmt.Sprintf(` AND r."UserId" = %s`, ph())
		args = append(args, ident.UserID)
	} else if !isModerator(ident) {
		where += fmt.Sprintf(` AND r."UserId" = %s`, ph())
		args = append(args, ident.UserID)
	}

	var total int64
	_ = s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM "Reports" r`+where, args...).Scan(&total)

	query := `SELECT r."Id", r."UserId", u."Name", u."AvatarUrl", r."ImageId", r."Description", r."Status", r."ReviewerNote", r."CreatedAt"
		FROM "Reports" r JOIN "Users" u ON u."Id" = r."UserId"` + where + ` ORDER BY r."CreatedAt" DESC`
	query += fmt.Sprintf(" LIMIT %d OFFSET %d", pageSize, (page-1)*pageSize)

	rows, err := s.db.Query(r.Context(), query, args...)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	defer rows.Close()

	items := []reportDto{}
	for rows.Next() {
		var rep reportDto
		var st int
		var userName string
		var userAvatar *string
		var reviewerNote *string
		if err := rows.Scan(&rep.ID, &rep.UserID, &userName, &userAvatar, &rep.ImageID,
			&rep.Description, &st, &reviewerNote, &rep.CreatedAt); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		rep.Status = reportStatusString(st)
		rep.ReviewerNote = reviewerNote
		rep.User = &userMinimalDto{ID: rep.UserID, Name: userName, AvatarURL: userAvatar}
		if rep.ImageID != nil {
			rep.Image = s.loadReportImage(r, ident, *rep.ImageID)
		}
		items = append(items, rep)
	}
	writeJSON(w, http.StatusOK, newPaginated(items, page, pageSize, total, s.cfg.PageSizes.ReportMax, s.cfg.PageSizes.ReportDefault))
}

func (s *Server) loadReportImage(r *http.Request, ident *auth.Identity, imageID int64) *imageDto {
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
	dto, err := s.assembleImageDto(r.Context(), row, ident, "All", false, nil, false)
	if err != nil {
		return nil
	}
	dto.Favorites = 0
	dto.LikedAt = nil
	dto.Albums = []imageAlbumRef{}
	return dto
}

func (s *Server) loadReport(w http.ResponseWriter, r *http.Request, ident *auth.Identity, id int64) (*reportDto, bool) {
	var rep reportDto
	var st int
	var userName string
	var userAvatar *string
	var reviewerNote *string
	err := s.db.QueryRow(r.Context(), `
		SELECT r."Id", r."UserId", u."Name", u."AvatarUrl", r."ImageId", r."Description", r."Status", r."ReviewerNote", r."CreatedAt"
		FROM "Reports" r JOIN "Users" u ON u."Id" = r."UserId"
		WHERE r."Id" = $1`, id).
		Scan(&rep.ID, &rep.UserID, &userName, &userAvatar, &rep.ImageID, &rep.Description, &st, &reviewerNote, &rep.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, fmt.Sprintf("Report with ID %d not found.", id))
		return nil, false
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return nil, false
	}
	rep.Status = reportStatusString(st)
	rep.ReviewerNote = reviewerNote
	rep.User = &userMinimalDto{ID: rep.UserID, Name: userName, AvatarURL: userAvatar}
	if rep.ImageID != nil {
		rep.Image = s.loadReportImage(r, ident, *rep.ImageID)
	}
	return &rep, true
}

func (s *Server) handlePatchReport(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}

	var req struct {
		Description *string `json:"description"`
	}
	if err := decodeJSON(r, &req); err != nil {
		WriteValidationProblem(w, r, map[string][]string{
			"": {"The request could not be processed."},
		})
		return
	}
	description := ""
	if req.Description != nil {
		description = *req.Description
	}
	if description == "" {
		WriteValidationProblem(w, r, map[string][]string{
			"Description": {"Description is required."},
		})
		return
	}
	if len(description) > 500 {
		WriteValidationProblem(w, r, map[string][]string{
			"Description": {"Description must not exceed 500 characters."},
		})
		return
	}

	rep, ok := s.loadReport(w, r, ident, id)
	if !ok {
		return
	}
	if rep.UserID != ident.UserID && !isModerator(ident) {
		Forbidden(w, r, "You can only edit your own reports.")
		return
	}
	if rep.Status != "Pending" {
		Conflict(w, r, "This report has already been handled and can no longer be edited.")
		return
	}
	trimmed := strings.TrimSpace(description)
	_, err = s.db.Exec(r.Context(), `UPDATE "Reports" SET "Description" = $1 WHERE "Id" = $2`, trimmed, id)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	rep.Description = trimmed
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) handleValidateReport(w http.ResponseWriter, r *http.Request) {
	s.resolveReport(w, r, 1)
}

func (s *Server) handleRejectReport(w http.ResponseWriter, r *http.Request) {
	s.resolveReport(w, r, 2)
}

func (s *Server) resolveReport(w http.ResponseWriter, r *http.Request, newStatus int) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	var st int
	err = s.db.QueryRow(r.Context(), `SELECT "Status" FROM "Reports" WHERE "Id" = $1`, id).Scan(&st)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, fmt.Sprintf("Report with ID %d not found.", id))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if st != 0 {
		Conflict(w, r, "This report has already been handled.")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = decodeJSON(r, &req)
	_, err = s.db.Exec(r.Context(), `
		UPDATE "Reports" SET "Status" = $1, "ReviewerNote" = $2 WHERE "Id" = $3`,
		newStatus, orEmptyString(strings.TrimSpace(req.Reason)), id)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCancelReport(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	var userID int64
	var st int
	err = s.db.QueryRow(r.Context(), `SELECT "UserId", "Status" FROM "Reports" WHERE "Id" = $1`, id).Scan(&userID, &st)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, fmt.Sprintf("Report with ID %d not found.", id))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	if st != 0 {
		Conflict(w, r, "This report has already been handled.")
		return
	}
	if userID != ident.UserID && !isModerator(ident) {
		Forbidden(w, r, "You can only cancel your own reports.")
		return
	}
	_, err = s.db.Exec(r.Context(), `UPDATE "Reports" SET "Status" = 3 WHERE "Id" = $1`, id)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
