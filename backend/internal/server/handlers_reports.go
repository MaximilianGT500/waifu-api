package server

import (
	"context"
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
	ctx := r.Context()
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
	req.Description = strings.TrimSpace(req.Description)
	errs := map[string][]string{}
	if req.ImageID <= 0 {
		errs["ImageId"] = append(errs["ImageId"], "Invalid Image ID.")
	}
	if req.Description == "" {
		errs["Description"] = append(errs["Description"], "Description is required.")
	} else if runeLen(req.Description) > 500 {
		errs["Description"] = append(errs["Description"], "Description must not exceed 500 characters.")
	}
	if len(errs) > 0 {
		WriteValidationProblem(w, r, errs)
		return
	}

	var rep reportDto
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var reviewStatus int
		var uploaderID *int64
		if err := tx.QueryRow(ctx, `SELECT "ReviewStatus", "UploaderId" FROM "Images" WHERE "Id" = $1`, req.ImageID).
			Scan(&reviewStatus, &uploaderID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return errNotFound(fmt.Sprintf("Image with ID %d not found.", req.ImageID))
			}
			return err
		}
		if !canSeeContent(ident, reviewStatus, uploaderID) {
			return errNotFound(fmt.Sprintf("Image with ID %d not found.", req.ImageID))
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('reports'), $1::int)`, int32(ident.UserID)); err != nil {
			return err
		}
		var existing bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM "Reports" WHERE "UserId" = $1 AND "ImageId" = $2 AND "Status" = 0)`,
			ident.UserID, req.ImageID).Scan(&existing); err != nil {
			return err
		}
		if existing {
			return errConflict("You already have a pending report for this image.")
		}
		if max := s.cfg.Limits.MaxPendingReportsPerUser; max > 0 && !isModerator(ident) {
			var open int
			if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM "Reports" WHERE "UserId" = $1 AND "Status" = 0`, ident.UserID).Scan(&open); err != nil {
				return err
			}
			if open >= max {
				return errConflict(fmt.Sprintf("You have too many pending reports (limit %d). Please wait for some to be handled.", max))
			}
		}
		var createdAt time.Time
		if err := tx.QueryRow(ctx, `
			INSERT INTO "Reports" ("UserId","ImageId","Description","CreatedAt","Status")
			VALUES ($1,$2,$3,now(),0) RETURNING "Id","CreatedAt"`,
			ident.UserID, req.ImageID, req.Description).Scan(&rep.ID, &createdAt); err != nil {
			return err
		}
		rep.CreatedAt = createdAt
		return nil
	})
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}

	rep.UserID = ident.UserID
	rep.ImageID = &req.ImageID
	rep.Description = req.Description
	rep.Status = "Pending"
	w.Header().Set("Location", fmt.Sprintf("/reports?id=%d", rep.ID))
	writeJSON(w, http.StatusCreated, rep)
}

const reportColumns = `r."Id", r."UserId", u."Name", u."AvatarUrl", r."ImageId", r."Description", r."Status", r."ReviewerNote", r."CreatedAt"`

func scanReport(row pgx.Row) (*reportDto, error) {
	var rep reportDto
	var st int
	var userName *string
	var userAvatar *string
	var desc *string
	if err := row.Scan(&rep.ID, &rep.UserID, &userName, &userAvatar, &rep.ImageID, &desc, &st, &rep.ReviewerNote, &rep.CreatedAt); err != nil {
		return nil, err
	}
	rep.Description = orEmpty(desc)
	rep.Status = reportStatusString(st)
	rep.User = &userMinimalDto{ID: rep.UserID, Name: orEmpty(userName), AvatarURL: userAvatar}
	return &rep, nil
}

func (s *Server) attachReportImages(ctx context.Context, ident *auth.Identity, reports []*reportDto) error {
	var ids []int64
	for _, rep := range reports {
		if rep.ImageID != nil {
			ids = append(ids, *rep.ImageID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	opts := hydrateOptions{ident: ident, childrenReviewStatus: "Accepted"}
	if isModerator(ident) {
		opts.childrenReviewStatus = "All"
	}
	var imageRows []imageRow
	b := &pgx.Batch{}
	b.Queue(`SELECT `+imageColumns+` FROM "Images" i WHERE i."Id" = ANY($1)`, ids).Query(func(rows pgx.Rows) error {
		for rows.Next() {
			row, err := scanImageRow(rows, false)
			if err != nil {
				return err
			}
			imageRows = append(imageRows, row)
		}
		return rows.Err()
	})
	children := s.queueImageChildren(b, ids, opts)
	if err := s.db.SendBatch(ctx, b).Close(); err != nil {
		return err
	}
	dtos := s.imageDtos(imageRows, opts, children)
	byID := make(map[int64]*imageDto, len(dtos))
	for i := range dtos {
		byID[dtos[i].ID] = &dtos[i]
	}
	for _, rep := range reports {
		if rep.ImageID != nil {
			if img, ok := byID[*rep.ImageID]; ok {
				cp := *img
				rep.Image = &cp
			}
		}
	}
	return nil
}

func (s *Server) handleListReports(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
	page, pageSize := s.paginationParams(r, s.cfg.PageSizes.ReportDefault, s.cfg.PageSizes.ReportMax)

	a := &sqlArgs{}
	where := " WHERE TRUE"
	if st := queryGet(r, "status"); st != "" {
		statusInt, ok := reportStatusInt(st)
		if !ok {
			invalidEnumProblem(w, r, "status", st)
			return
		}
		where += ` AND r."Status" = ` + a.add(statusInt)
	}
	if !isModerator(ident) || queryBool(r, "mine", false) {
		where += ` AND r."UserId" = ` + a.add(ident.UserID)
	}

	var total int64
	var reports []*reportDto
	query := `SELECT ` + reportColumns + ` FROM "Reports" r JOIN "Users" u ON u."Id" = r."UserId"` + where +
		fmt.Sprintf(` ORDER BY r."CreatedAt" DESC, r."Id" DESC LIMIT %d OFFSET %d`, pageSize, offsetOf(page, pageSize))
	b := &pgx.Batch{}
	b.Queue(`SELECT COUNT(*) FROM "Reports" r`+where, a.args...).QueryRow(func(row pgx.Row) error {
		return row.Scan(&total)
	})
	b.Queue(query, a.args...).Query(func(rows pgx.Rows) error {
		for rows.Next() {
			rep, err := scanReport(rows)
			if err != nil {
				return err
			}
			reports = append(reports, rep)
		}
		return rows.Err()
	})
	if err := s.db.SendBatch(ctx, b).Close(); err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if err := s.attachReportImages(ctx, ident, reports); err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	items := make([]reportDto, 0, len(reports))
	for _, rep := range reports {
		items = append(items, *rep)
	}
	writeJSON(w, http.StatusOK, s.newPaginated(items, page, pageSize, total, s.cfg.PageSizes.ReportMax, s.cfg.PageSizes.ReportDefault))
}

func (s *Server) handlePatchReport(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
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
		description = strings.TrimSpace(*req.Description)
	}
	if description == "" {
		WriteValidationProblem(w, r, map[string][]string{"Description": {"Description is required."}})
		return
	}
	if runeLen(description) > 500 {
		WriteValidationProblem(w, r, map[string][]string{"Description": {"Description must not exceed 500 characters."}})
		return
	}

	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var userID int64
		var st int
		if err := tx.QueryRow(ctx, `SELECT "UserId", "Status" FROM "Reports" WHERE "Id" = $1 FOR UPDATE`, id).Scan(&userID, &st); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return errNotFound(fmt.Sprintf("Report with ID %d not found.", id))
			}
			return err
		}
		if userID != ident.UserID && !isModerator(ident) {
			return errForbidden("You can only edit your own reports.")
		}
		if st != 0 {
			return errConflict("This report has already been handled and can no longer be edited.")
		}
		_, err := tx.Exec(ctx, `UPDATE "Reports" SET "Description" = $1 WHERE "Id" = $2`, description, id)
		return err
	})
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}

	rep, err := scanReport(s.db.QueryRow(ctx, `SELECT `+reportColumns+` FROM "Reports" r JOIN "Users" u ON u."Id" = r."UserId" WHERE r."Id" = $1`, id))
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if err := s.attachReportImages(ctx, ident, []*reportDto{rep}); err != nil {
		InternalError(w, r, s.log, err)
		return
	}
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
	var req struct {
		Reason string `json:"reason"`
	}
	_ = decodeJSON(r, &req)
	reason := strings.TrimSpace(req.Reason)
	if runeLen(reason) > 500 {
		WriteValidationProblem(w, r, map[string][]string{"Reason": {"Reason must not exceed 500 characters."}})
		return
	}

	res, err := s.db.Exec(r.Context(), `
		UPDATE "Reports" SET "Status" = $1, "ReviewerNote" = $2 WHERE "Id" = $3 AND "Status" = 0`,
		newStatus, orEmptyString(reason), id)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if res.RowsAffected() == 0 {
		var exists bool
		if err := s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM "Reports" WHERE "Id" = $1)`, id).Scan(&exists); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		if !exists {
			NotFound(w, r, fmt.Sprintf("Report with ID %d not found.", id))
			return
		}
		Conflict(w, r, "This report has already been handled.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCancelReport(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var userID int64
		var st int
		if err := tx.QueryRow(ctx, `SELECT "UserId", "Status" FROM "Reports" WHERE "Id" = $1 FOR UPDATE`, id).Scan(&userID, &st); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return errNotFound(fmt.Sprintf("Report with ID %d not found.", id))
			}
			return err
		}
		if userID != ident.UserID && !isModerator(ident) {
			return errForbidden("You can only cancel your own reports.")
		}
		if st != 0 {
			return errConflict("This report has already been handled.")
		}
		_, err := tx.Exec(ctx, `UPDATE "Reports" SET "Status" = 3 WHERE "Id" = $1`, id)
		return err
	})
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
