package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/waifu-im/waifu-api/backend/internal/auth"
)

func (s *Server) RegisterReviewRoutes() {
	s.Handle("POST /review/tasks", s.RequireAuth(http.HandlerFunc(s.handleCreateReviewTask)))
	s.Handle("GET /review/tasks", s.RequireAuth(http.HandlerFunc(s.handleListReviewTasks)))
	s.Handle("GET /review/tasks/{id}", s.RequireAuth(http.HandlerFunc(s.handleGetReviewTask)))
	s.Handle("PATCH /review/tasks/{id}", s.RequireAuth(http.HandlerFunc(s.handleEditReviewTask)))
	s.Handle("PATCH /review/tasks/{id}/approve", s.RequireRole("Moderator")(http.HandlerFunc(s.handleApproveReviewTask)))
	s.Handle("PATCH /review/tasks/{id}/reject", s.RequireRole("Moderator")(http.HandlerFunc(s.handleRejectReviewTask)))
	s.Handle("PATCH /review/tasks/{id}/cancel", s.RequireAuth(http.HandlerFunc(s.handleCancelReviewTask)))
}

func targetTypeInt(s string) (int, bool) {
	switch {
	case strings.EqualFold(s, "Image"):
		return 0, true
	case strings.EqualFold(s, "Tag"):
		return 1, true
	case strings.EqualFold(s, "Artist"):
		return 2, true
	}
	return 0, false
}

func kindInt(s string) (int, bool) {
	switch {
	case strings.EqualFold(s, "Edit"):
		return 1, true
	case strings.EqualFold(s, "NewContent"):
		return 0, true
	}
	return 0, false
}

func reviewTaskStatusInt(s string) (int, bool) {
	switch {
	case strings.EqualFold(s, "Pending"):
		return 0, true
	case strings.EqualFold(s, "Approved"):
		return 1, true
	case strings.EqualFold(s, "Rejected"):
		return 2, true
	case strings.EqualFold(s, "Cancelled"):
		return 3, true
	}
	return 0, false
}

func targetTypeName(tt int) string {
	switch tt {
	case 1:
		return "Tag"
	case 2:
		return "Artist"
	default:
		return "Image"
	}
}

func errForbidden(msg string) error {
	return &apiError{status: http.StatusForbidden, typ: typeForbidden, title: "Forbidden", detail: msg}
}

type reviewTaskRow struct {
	ID                int64
	Kind              int
	SubmitterID       *int64
	TargetType        int
	TargetID          int64
	Payload           []byte
	Reason            *string
	Status            int
	ReviewerID        *int64
	ReviewerNote      *string
	ModeratorEditedAt *time.Time
	CreatedAt         time.Time
	ResolvedAt        *time.Time
}

const reviewTaskColumns = `t."Id", t."Kind", t."SubmitterId", t."TargetType", t."TargetId", t."Payload", t."Reason", t."Status",
	t."ReviewerId", t."ReviewerNote", t."ModeratorEditedAt", t."CreatedAt", t."ResolvedAt"`

func scanReviewTask(row pgx.Row) (*reviewTaskRow, error) {
	var t reviewTaskRow
	err := row.Scan(&t.ID, &t.Kind, &t.SubmitterID, &t.TargetType, &t.TargetID, &t.Payload, &t.Reason,
		&t.Status, &t.ReviewerID, &t.ReviewerNote, &t.ModeratorEditedAt, &t.CreatedAt, &t.ResolvedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Server) loadReviewTaskRow(ctx context.Context, q querier, id int64, forUpdate bool) (*reviewTaskRow, error) {
	query := `SELECT ` + reviewTaskColumns + ` FROM "ReviewTasks" t WHERE t."Id" = $1`
	if forUpdate {
		query += " FOR UPDATE"
	}
	t, err := scanReviewTask(q.QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errNotFound(fmt.Sprintf("Review task with ID %d not found.", id))
	}
	return t, err
}

func (s *Server) hydrateReviewTasks(ctx context.Context, ident *auth.Identity, tasks []*reviewTaskRow) ([]reviewTaskDto, error) {
	out := make([]reviewTaskDto, len(tasks))
	if len(tasks) == 0 {
		return out, nil
	}

	userSet := map[int64]bool{}
	var imageIDs, tagIDs, artistIDs []int64
	for _, t := range tasks {
		if t.SubmitterID != nil {
			userSet[*t.SubmitterID] = true
		}
		if t.ReviewerID != nil {
			userSet[*t.ReviewerID] = true
		}
		switch t.TargetType {
		case 1:
			tagIDs = append(tagIDs, t.TargetID)
		case 2:
			artistIDs = append(artistIDs, t.TargetID)
		default:
			imageIDs = append(imageIDs, t.TargetID)
		}
	}

	b := &pgx.Batch{}
	users := map[int64]userMinimalDto{}
	if len(userSet) > 0 {
		ids := make([]int64, 0, len(userSet))
		for id := range userSet {
			ids = append(ids, id)
		}
		b.Queue(`SELECT "Id","Name","AvatarUrl" FROM "Users" WHERE "Id" = ANY($1)`, ids).Query(func(rows pgx.Rows) error {
			for rows.Next() {
				var u userMinimalDto
				if err := rows.Scan(&u.ID, &u.Name, &u.AvatarURL); err != nil {
					return err
				}
				users[u.ID] = u
			}
			return rows.Err()
		})
	}

	var imageRows []imageRow
	var children *imageChildren
	opts := hydrateOptions{ident: ident, showUploader: true}
	if isModerator(ident) {
		opts.childrenReviewStatus = "All"
	} else {
		opts.childrenReviewStatus = "Accepted"
		opts.includeMyPendingChildren = true
	}
	if len(imageIDs) > 0 {
		b.Queue(`SELECT `+imageColumns+` FROM "Images" i WHERE i."Id" = ANY($1)`, imageIDs).Query(func(rows pgx.Rows) error {
			for rows.Next() {
				row, err := scanImageRow(rows, false)
				if err != nil {
					return err
				}
				imageRows = append(imageRows, row)
			}
			return rows.Err()
		})
		children = queueImageChildren(b, imageIDs, opts)
	}

	tags := map[int64]tagDto{}
	if len(tagIDs) > 0 {
		b.Queue(`SELECT "Id","Name","Slug","Description","ReviewStatus" FROM "Tags" WHERE "Id" = ANY($1)`, tagIDs).Query(func(rows pgx.Rows) error {
			for rows.Next() {
				var t tagDto
				var rs int
				var desc *string
				if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &desc, &rs); err != nil {
					return err
				}
				t.Description = orEmpty(desc)
				t.ReviewStatus = reviewStatusString(rs)
				tags[t.ID] = t
			}
			return rows.Err()
		})
	}

	artists := map[int64]artistDto{}
	if len(artistIDs) > 0 {
		b.Queue(`
			SELECT "Id","Name","Patreon","Pixiv","Twitter","DeviantArt","ReviewStatus"
			FROM "Artists" WHERE "Id" = ANY($1)`, artistIDs).Query(func(rows pgx.Rows) error {
			for rows.Next() {
				var a artistDto
				var rs int
				var patreon, pixiv, twitter, deviantArt *string
				if err := rows.Scan(&a.ID, &a.Name, &patreon, &pixiv, &twitter, &deviantArt, &rs); err != nil {
					return err
				}
				a.ReviewStatus = reviewStatusString(rs)
				a.Patreon = orEmpty(patreon)
				a.Pixiv = orEmpty(pixiv)
				a.Twitter = orEmpty(twitter)
				a.DeviantArt = orEmpty(deviantArt)
				artists[a.ID] = a
			}
			return rows.Err()
		})
	}

	if err := s.db.SendBatch(ctx, b).Close(); err != nil {
		return nil, err
	}
	images := map[int64]imageDto{}
	if len(imageRows) > 0 {
		for _, d := range s.imageDtos(imageRows, opts, children) {
			images[d.ID] = d
		}
	}

	for i, t := range tasks {
		dto := reviewTaskDto{
			ID:                t.ID,
			Kind:              reviewTaskKindString(t.Kind),
			SubmitterID:       t.SubmitterID,
			TargetType:        targetTypeString(t.TargetType),
			TargetID:          t.TargetID,
			Reason:            t.Reason,
			Status:            reviewTaskStatusString(t.Status),
			ReviewerID:        t.ReviewerID,
			ReviewerNote:      t.ReviewerNote,
			ModeratorEditedAt: t.ModeratorEditedAt,
			CreatedAt:         t.CreatedAt,
			ResolvedAt:        t.ResolvedAt,
		}
		if t.Payload != nil {
			var payload map[string]any
			if json.Unmarshal(t.Payload, &payload) == nil {
				dto.Payload = payload
			}
		}
		if t.SubmitterID != nil {
			if u, ok := users[*t.SubmitterID]; ok {
				u := u
				dto.Submitter = &u
			}
		}
		if t.ReviewerID != nil {
			if u, ok := users[*t.ReviewerID]; ok {
				u := u
				dto.Reviewer = &u
			}
		}
		switch t.TargetType {
		case 1:
			if tag, ok := tags[t.TargetID]; ok {
				dto.Target = map[string]any{"tag": tag}
			}
		case 2:
			if a, ok := artists[t.TargetID]; ok {
				dto.Target = map[string]any{"artist": a}
			}
		default:
			if img, ok := images[t.TargetID]; ok {
				img := img
				dto.Target = map[string]any{"image": &img}
			}
		}
		out[i] = dto
	}
	return out, nil
}

func (s *Server) hydrateReviewTask(ctx context.Context, ident *auth.Identity, t *reviewTaskRow) (*reviewTaskDto, error) {
	dtos, err := s.hydrateReviewTasks(ctx, ident, []*reviewTaskRow{t})
	if err != nil {
		return nil, err
	}
	return &dtos[0], nil
}

func (s *Server) handleListReviewTasks(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
	page, pageSize := s.paginationParams(r, s.cfg.Review.DefaultPageSize, s.cfg.Review.MaxPageSize)

	a := &sqlArgs{}
	where := " WHERE TRUE"
	if st := queryGet(r, "status"); st != "" {
		v, ok := reviewTaskStatusInt(st)
		if !ok {
			invalidEnumProblem(w, r, "status", st)
			return
		}
		where += ` AND t."Status" = ` + a.add(v)
	}
	if k := queryGet(r, "kind"); k != "" {
		v, ok := kindInt(k)
		if !ok {
			invalidEnumProblem(w, r, "kind", k)
			return
		}
		where += ` AND t."Kind" = ` + a.add(v)
	}
	if tt := queryGet(r, "targetType"); tt != "" {
		v, ok := targetTypeInt(tt)
		if !ok {
			invalidEnumProblem(w, r, "targetType", tt)
			return
		}
		where += ` AND t."TargetType" = ` + a.add(v)
	}
	if tid := queryGet(r, "targetId"); tid != "" {
		n, err := strconv.ParseInt(strings.TrimSpace(tid), 10, 64)
		if err != nil {
			invalidEnumProblem(w, r, "targetId", tid)
			return
		}
		where += ` AND t."TargetId" = ` + a.add(n)
	}
	if !isModerator(ident) || queryBool(r, "mine", false) {
		where += ` AND t."SubmitterId" = ` + a.add(ident.UserID)
	}

	var total int64
	var tasks []*reviewTaskRow
	b := &pgx.Batch{}
	b.Queue(`SELECT COUNT(*) FROM "ReviewTasks" t`+where, a.args...).QueryRow(func(row pgx.Row) error {
		return row.Scan(&total)
	})
	b.Queue(`SELECT `+reviewTaskColumns+` FROM "ReviewTasks" t`+where+
		fmt.Sprintf(` ORDER BY t."CreatedAt" DESC, t."Id" DESC LIMIT %d OFFSET %d`, pageSize, offsetOf(page, pageSize)), a.args...).Query(func(rows pgx.Rows) error {
		for rows.Next() {
			t, err := scanReviewTask(rows)
			if err != nil {
				return err
			}
			tasks = append(tasks, t)
		}
		return rows.Err()
	})
	if err := s.db.SendBatch(ctx, b).Close(); err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	items, err := s.hydrateReviewTasks(ctx, ident, tasks)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, s.newPaginated(items, page, pageSize, total, s.cfg.Review.MaxPageSize, s.cfg.Review.DefaultPageSize))
}

func (s *Server) handleGetReviewTask(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	t, err := s.loadReviewTaskRow(r.Context(), s.db, id, false)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	if !isModerator(ident) && (t.SubmitterID == nil || *t.SubmitterID != ident.UserID) {
		NotFound(w, r, fmt.Sprintf("Review task with ID %d not found.", id))
		return
	}
	dto, err := s.hydrateReviewTask(r.Context(), ident, t)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) editBypassMinRole(tt int) string {
	switch tt {
	case 0:
		return s.cfg.Review.ImageEditReviewBypassMinRole
	case 1:
		return s.cfg.Review.TagEditReviewBypassMinRole
	default:
		return s.cfg.Review.ArtistEditReviewBypassMinRole
	}
}

func (s *Server) checkPendingSubmissions(ctx context.Context, tx pgx.Tx, userID int64) error {
	limit := s.cfg.Review.MaxPendingSubmissionsPerUser
	if limit <= 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('review-submissions'), $1::int)`, int32(userID)); err != nil {
		return err
	}
	var pending int64
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM "ReviewTasks" WHERE "SubmitterId" = $1 AND "Kind" = 0 AND "Status" = 0`,
		userID).Scan(&pending); err != nil {
		return err
	}
	if pending >= int64(limit) {
		return errConflict(fmt.Sprintf("You have too many submissions waiting for review (limit %d). Please wait for some to be reviewed.", limit))
	}
	return nil
}

func (s *Server) handleCreateReviewTask(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
	if !requireMinRole(ident, s.cfg.Permissions.SubmitEditMinRole) {
		s.permissionRoleForbidden(w, r, s.cfg.Permissions.SubmitEditMinRole, auth.RoleUser)
		return
	}
	var req struct {
		TargetType string         `json:"targetType"`
		TargetID   int64          `json:"targetId"`
		Reason     string         `json:"reason"`
		Payload    map[string]any `json:"payload"`
	}
	if err := decodeJSON(r, &req); err != nil {
		WriteValidationProblem(w, r, map[string][]string{
			"": {"The request could not be processed."},
		})
		return
	}

	tt, ok := targetTypeInt(req.TargetType)
	if !ok {
		WriteValidationProblem(w, r, map[string][]string{"TargetType": {"Invalid target type."}})
		return
	}
	if req.TargetID <= 0 {
		WriteValidationProblem(w, r, map[string][]string{"TargetId": {"Invalid target ID."}})
		return
	}
	if req.Payload == nil {
		WriteValidationProblem(w, r, map[string][]string{"PayloadJson": {"Proposed changes are required."}})
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if runeLen(reason) > 500 {
		WriteValidationProblem(w, r, map[string][]string{"Reason": {"Reason must not exceed 500 characters."}})
		return
	}

	exists, err := s.targetExistsAccepted(ctx, tt, req.TargetID)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if !exists {
		NotFound(w, r, fmt.Sprintf("%s with ID %d not found.", targetTypeName(tt), req.TargetID))
		return
	}

	payload, err := s.normalizeEditPayload(ctx, s.db, tt, req.TargetID, req.Payload, ident)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	autoApply := !s.cfg.Review.RequireEditRequestReview || auth.ReviewBypassed(ident.Role, s.editBypassMinRole(tt))

	var taskID int64
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('review-edits'), $1::int)`, int32(ident.UserID)); err != nil {
			return err
		}
		if s.cfg.Review.MaxOpenEditsPerUser > 0 && !autoApply {
			var open int64
			if err := tx.QueryRow(ctx, `
				SELECT COUNT(*) FROM "ReviewTasks" WHERE "SubmitterId" = $1 AND "Kind" = 1 AND "Status" = 0`,
				ident.UserID).Scan(&open); err != nil {
				return err
			}
			if open >= int64(s.cfg.Review.MaxOpenEditsPerUser) {
				return errConflict(fmt.Sprintf("You have too many pending edits (limit %d). Please wait for some to be reviewed.", s.cfg.Review.MaxOpenEditsPerUser))
			}
		}
		if !autoApply {
			var dup bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS(SELECT 1 FROM "ReviewTasks"
				WHERE "SubmitterId" = $1 AND "Kind" = 1 AND "Status" = 0 AND "TargetType" = $2 AND "TargetId" = $3)`,
				ident.UserID, tt, req.TargetID).Scan(&dup); err != nil {
				return err
			}
			if dup {
				return errConflict("You already have a pending edit for this item. Edit or cancel it first.")
			}
		}

		status := 0
		var reviewerNote *string
		var resolvedAt any
		if autoApply {
			status = 1
			reviewerNote = strPtr("Auto-applied (trusted role).")
			resolvedAt = time.Now().UTC()
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO "ReviewTasks" ("Kind","TargetType","TargetId","SubmitterId","Payload","Reason","Status","ReviewerNote","CreatedAt","ResolvedAt")
			VALUES (1,$1,$2,$3,$4::jsonb,$5,$6,$7,now(),$8)
			RETURNING "Id"`,
			tt, req.TargetID, ident.UserID, payloadJSON, orEmptyString(reason), status, reviewerNote, resolvedAt).Scan(&taskID)
		if err != nil {
			if isUniqueViolation(err) {
				return errConflict("You already have a pending edit for this item. Edit or cancel it first.")
			}
			return err
		}
		if autoApply {
			var stored map[string]any
			if err := json.Unmarshal(payloadJSON, &stored); err != nil {
				return err
			}
			if err := s.applyEditPayload(ctx, tx, tt, req.TargetID, stored, ident); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}

	t, err := s.loadReviewTaskRow(ctx, s.db, taskID, false)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	dto, err := s.hydrateReviewTask(ctx, ident, t)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/review/tasks/%d", taskID))
	writeJSON(w, http.StatusCreated, dto)
}

func (s *Server) targetExistsAccepted(ctx context.Context, tt int, id int64) (bool, error) {
	var ok bool
	var err error
	switch tt {
	case 0:
		err = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "Images" WHERE "Id" = $1 AND "ReviewStatus" = 1)`, id).Scan(&ok)
	case 1:
		err = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "Tags" WHERE "Id" = $1 AND "ReviewStatus" = 1)`, id).Scan(&ok)
	case 2:
		err = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "Artists" WHERE "Id" = $1 AND "ReviewStatus" = 1)`, id).Scan(&ok)
	}
	return ok, err
}

func lockPendingTasks(ctx context.Context, tx pgx.Tx, targetType int, targetID int64) error {
	_, err := tx.Exec(ctx, `
		SELECT 1 FROM "ReviewTasks" WHERE "TargetType" = $1 AND "TargetId" = $2 AND "Status" = 0
		ORDER BY "Id" FOR UPDATE`, targetType, targetID)
	return err
}

type targetInfo struct {
	exists       bool
	reviewStatus int
	extension    string
}

func targetState(ctx context.Context, q querier, tt int, id int64, lock bool) (targetInfo, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	var ti targetInfo
	var err error
	switch tt {
	case 0:
		err = q.QueryRow(ctx, `SELECT "ReviewStatus", "Extension" FROM "Images" WHERE "Id" = $1`+suffix, id).Scan(&ti.reviewStatus, &ti.extension)
	case 1:
		err = q.QueryRow(ctx, `SELECT "ReviewStatus" FROM "Tags" WHERE "Id" = $1`+suffix, id).Scan(&ti.reviewStatus)
	case 2:
		err = q.QueryRow(ctx, `SELECT "ReviewStatus" FROM "Artists" WHERE "Id" = $1`+suffix, id).Scan(&ti.reviewStatus)
	default:
		return ti, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ti, nil
	}
	if err != nil {
		return ti, err
	}
	ti.exists = true
	return ti, nil
}

func rejectUnknownFields(in map[string]any, allowed map[string]bool) error {
	for key := range in {
		found := false
		for a := range allowed {
			if strings.EqualFold(key, a) {
				found = true
				break
			}
		}
		if !found {
			return errValidation(fmt.Sprintf("Invalid payload: The JSON property '%s' is not allowed.", key))
		}
	}
	return nil
}

func fieldString(in map[string]any, key string) (string, bool) {
	for k, v := range in {
		if strings.EqualFold(k, key) {
			if v == nil {
				return "", false
			}
			s, ok := v.(string)
			if !ok {
				return "", false
			}
			return s, true
		}
	}
	return "", false
}

func fieldBool(in map[string]any, key string) (bool, bool) {
	for k, v := range in {
		if strings.EqualFold(k, key) {
			if v == nil {
				return false, false
			}
			b, ok := v.(bool)
			if !ok {
				return false, false
			}
			return b, true
		}
	}
	return false, false
}

func cleanStringList(in map[string]any, key string, max int) ([]string, error) {
	for k, v := range in {
		if !strings.EqualFold(k, key) {
			continue
		}
		if v == nil {
			return nil, nil
		}
		arr, ok := v.([]any)
		if !ok {
			return nil, errValidation(fmt.Sprintf("Invalid payload: '%s' must be an array of strings.", key))
		}
		seen := map[string]bool{}
		var out []string
		for _, item := range arr {
			s, ok := item.(string)
			if !ok {
				return nil, errValidation(fmt.Sprintf("Invalid payload: '%s' must be an array of strings.", key))
			}
			s = strings.TrimSpace(s)
			if len(s) > 100 {
				return nil, errValidation(fmt.Sprintf("Invalid payload: '%s' contains an invalid value.", key))
			}
			if s != "" && !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
		if max > 0 && len(out) > max {
			return nil, errValidation(fmt.Sprintf("Invalid payload: '%s' must not contain more than %d entries.", key, max))
		}
		return out, nil
	}
	return nil, nil
}

func cleanIDList(in map[string]any, key string, max int) ([]int64, error) {
	for k, v := range in {
		if !strings.EqualFold(k, key) {
			continue
		}
		if v == nil {
			return nil, nil
		}
		arr, ok := v.([]any)
		if !ok {
			return nil, errValidation(fmt.Sprintf("Invalid payload: '%s' must be an array of numbers.", key))
		}
		seen := map[int64]bool{}
		var out []int64
		for _, item := range arr {
			n, ok := toInt64(item)
			if !ok || n <= 0 {
				return nil, errValidation(fmt.Sprintf("Invalid payload: '%s' must be an array of numbers.", key))
			}
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
		if max > 0 && len(out) > max {
			return nil, errValidation(fmt.Sprintf("Invalid payload: '%s' must not contain more than %d entries.", key, max))
		}
		return out, nil
	}
	return nil, nil
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		if n != float64(int64(n)) {
			return 0, false
		}
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	}
	return 0, false
}

func imageTagSlugs(ctx context.Context, q querier, id int64) (map[string]bool, error) {
	out := map[string]bool{}
	rows, err := q.Query(ctx, `
		SELECT tg."Slug" FROM "ImageTag" it JOIN "Tags" tg ON tg."Id" = it."TagsId"
		WHERE it."ImagesId" = $1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		out[slug] = true
	}
	return out, rows.Err()
}

func imageArtistIDs(ctx context.Context, q querier, id int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	rows, err := q.Query(ctx, `SELECT "ArtistsId" FROM "ArtistImage" WHERE "ImagesId" = $1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var aid int64
		if err := rows.Scan(&aid); err != nil {
			return nil, err
		}
		out[aid] = true
	}
	return out, rows.Err()
}

func (s *Server) normalizeEditPayload(ctx context.Context, q querier, tt int, id int64, in map[string]any, ident *auth.Identity) (map[string]any, error) {
	if in == nil {
		return nil, errValidation("Invalid payload: payload is required.")
	}
	out := map[string]any{}
	switch tt {
	case 0:
		allowed := map[string]bool{"source": true, "isNsfw": true, "addTagSlugs": true, "removeTagSlugs": true, "addArtistIds": true, "removeArtistIds": true}
		if err := rejectUnknownFields(in, allowed); err != nil {
			return nil, err
		}
		var source *string
		var isNsfw bool
		if err := q.QueryRow(ctx, `SELECT "Source","IsNsfw" FROM "Images" WHERE "Id" = $1`, id).Scan(&source, &isNsfw); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, errNotFound(fmt.Sprintf("Image with ID %d not found.", id))
			}
			return nil, err
		}

		if v, ok := fieldString(in, "source"); ok {
			proposed := strings.TrimSpace(v)
			if proposed != "" {
				if runeLen(proposed) > 500 {
					return nil, errValidation("Source URL must not exceed 500 characters.")
				}
				if !isValidWebURL(proposed) {
					return nil, errValidation("Source must be a valid http(s) URL.")
				}
				if proposed != orEmpty(source) {
					out["source"] = proposed
				}
			}
		}
		if v, ok := fieldBool(in, "isNsfw"); ok && v != isNsfw {
			out["isNsfw"] = v
		}

		currentTagSlugs, err := imageTagSlugs(ctx, q, id)
		if err != nil {
			return nil, err
		}
		currentArtistIDs, err := imageArtistIDs(ctx, q, id)
		if err != nil {
			return nil, err
		}

		maxTags := s.cfg.Limits.MaxTagsPerImage
		maxArtists := s.cfg.Limits.MaxArtistsPerImage
		addTags, err := cleanStringList(in, "addTagSlugs", maxTags)
		if err != nil {
			return nil, err
		}
		removeTags, err := cleanStringList(in, "removeTagSlugs", maxTags)
		if err != nil {
			return nil, err
		}
		if len(addTags) > 0 {
			if _, err := s.resolveTagsBySlugs(ctx, q, addTags, ident); err != nil {
				return nil, err
			}
			var effective []string
			for _, slug := range addTags {
				if !currentTagSlugs[slug] {
					effective = append(effective, slug)
				}
			}
			if len(effective) > 0 {
				out["addTagSlugs"] = effective
			}
		}
		if len(removeTags) > 0 {
			var effective []string
			for _, slug := range removeTags {
				if currentTagSlugs[slug] {
					effective = append(effective, slug)
				}
			}
			if len(effective) > 0 {
				out["removeTagSlugs"] = effective
			}
		}

		addArtists, err := cleanIDList(in, "addArtistIds", maxArtists)
		if err != nil {
			return nil, err
		}
		removeArtists, err := cleanIDList(in, "removeArtistIds", maxArtists)
		if err != nil {
			return nil, err
		}
		if len(addArtists) > 0 {
			if _, err := s.resolveArtistsByIDs(ctx, q, addArtists, ident); err != nil {
				return nil, err
			}
			var effective []int64
			for _, aid := range addArtists {
				if !currentArtistIDs[aid] {
					effective = append(effective, aid)
				}
			}
			if len(effective) > 0 {
				out["addArtistIds"] = effective
			}
		}
		if len(removeArtists) > 0 {
			var effective []int64
			for _, aid := range removeArtists {
				if currentArtistIDs[aid] {
					effective = append(effective, aid)
				}
			}
			if len(effective) > 0 {
				out["removeArtistIds"] = effective
			}
		}
	case 1:
		allowed := map[string]bool{"name": true, "description": true}
		if err := rejectUnknownFields(in, allowed); err != nil {
			return nil, err
		}
		var name string
		var desc *string
		if err := q.QueryRow(ctx, `SELECT "Name","Description" FROM "Tags" WHERE "Id" = $1`, id).Scan(&name, &desc); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, errNotFound(fmt.Sprintf("Tag with ID %d not found.", id))
			}
			return nil, err
		}
		if v, ok := fieldString(in, "name"); ok {
			proposed := strings.TrimSpace(v)
			if proposed != "" {
				if runeLen(proposed) > 30 {
					return nil, errValidation("Name must not exceed 30 characters.")
				}
				if proposed != name {
					out["name"] = proposed
				}
			}
		}
		if v, ok := fieldString(in, "description"); ok {
			proposed := strings.TrimSpace(v)
			if proposed != "" {
				if runeLen(proposed) > 150 {
					return nil, errValidation("Description must not exceed 150 characters.")
				}
				if proposed != orEmpty(desc) {
					out["description"] = proposed
				}
			}
		}
	case 2:
		allowed := map[string]bool{"name": true, "patreon": true, "pixiv": true, "twitter": true, "deviantArt": true}
		if err := rejectUnknownFields(in, allowed); err != nil {
			return nil, err
		}
		var name string
		var patreon, pixiv, twitter, deviantArt *string
		if err := q.QueryRow(ctx, `SELECT "Name","Patreon","Pixiv","Twitter","DeviantArt" FROM "Artists" WHERE "Id" = $1`, id).
			Scan(&name, &patreon, &pixiv, &twitter, &deviantArt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, errNotFound(fmt.Sprintf("Artist with ID %d not found.", id))
			}
			return nil, err
		}
		if v, ok := fieldString(in, "name"); ok {
			proposed := strings.TrimSpace(v)
			if proposed != "" {
				if runeLen(proposed) > 30 {
					return nil, errValidation("Name must not exceed 30 characters.")
				}
				if proposed != name {
					out["name"] = proposed
				}
			}
		}
		for _, f := range []struct {
			key string
			cur *string
		}{{"patreon", patreon}, {"pixiv", pixiv}, {"twitter", twitter}, {"deviantArt", deviantArt}} {
			if v, ok := fieldString(in, f.key); ok {
				proposed := strings.TrimSpace(v)
				if proposed != "" {
					label := TitleOrEmpty(f.key)
					if runeLen(proposed) > 200 {
						return nil, errValidation(fmt.Sprintf("%s link must not exceed 200 characters.", label))
					}
					if !isValidWebURL(proposed) {
						return nil, errValidation(fmt.Sprintf("%s link must be a valid http(s) URL.", label))
					}
					if proposed != orEmpty(f.cur) {
						out[f.key] = proposed
					}
				}
			}
		}
	default:
		return nil, errValidation("Invalid payload: unknown target type.")
	}
	if len(out) == 0 {
		return nil, errValidation("The proposed changes are empty or identical to the current values.")
	}
	return out, nil
}

func anyStrings(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func anyIDs(v any) []int64 {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]int64, 0, len(arr))
	for _, x := range arr {
		if n, ok := toInt64(x); ok && n > 0 {
			out = append(out, n)
		}
	}
	return out
}

var artistLinkColumns = map[string]string{
	"patreon": `"Patreon"`, "pixiv": `"Pixiv"`, "twitter": `"Twitter"`, "deviantArt": `"DeviantArt"`,
}

func (s *Server) applyEditPayload(ctx context.Context, tx pgx.Tx, tt int, id int64, payload map[string]any, ident *auth.Identity) error {
	switch tt {
	case 0:
		if v, ok := payload["source"].(string); ok {
			if _, err := tx.Exec(ctx, `UPDATE "Images" SET "Source" = $1 WHERE "Id" = $2`, orEmptyString(v), id); err != nil {
				return err
			}
		}
		if v, ok := payload["isNsfw"].(bool); ok {
			if _, err := tx.Exec(ctx, `UPDATE "Images" SET "IsNsfw" = $1 WHERE "Id" = $2`, v, id); err != nil {
				return err
			}
		}
		if remove := anyStrings(payload["removeTagSlugs"]); len(remove) > 0 {
			if _, err := tx.Exec(ctx, `
				DELETE FROM "ImageTag" WHERE "ImagesId" = $1
				AND "TagsId" IN (SELECT "Id" FROM "Tags" WHERE "Slug" = ANY($2))`, id, remove); err != nil {
				return err
			}
		}
		if add := anyStrings(payload["addTagSlugs"]); len(add) > 0 {
			tags, err := s.resolveTagsBySlugs(ctx, tx, add, ident)
			if err != nil {
				return err
			}
			for _, t := range tags {
				if _, err := tx.Exec(ctx, `
					INSERT INTO "ImageTag" ("ImagesId","TagsId") SELECT $1, $2
					WHERE NOT EXISTS (SELECT 1 FROM "ImageTag" WHERE "ImagesId" = $1 AND "TagsId" = $2)`, id, t.ID); err != nil {
					return err
				}
			}
		}
		if remove := anyIDs(payload["removeArtistIds"]); len(remove) > 0 {
			if _, err := tx.Exec(ctx, `DELETE FROM "ArtistImage" WHERE "ImagesId" = $1 AND "ArtistsId" = ANY($2)`, id, remove); err != nil {
				return err
			}
		}
		if add := anyIDs(payload["addArtistIds"]); len(add) > 0 {
			artists, err := s.resolveArtistsByIDs(ctx, tx, add, ident)
			if err != nil {
				return err
			}
			for _, a := range artists {
				if _, err := tx.Exec(ctx, `
					INSERT INTO "ArtistImage" ("ArtistsId","ImagesId") SELECT $1, $2
					WHERE NOT EXISTS (SELECT 1 FROM "ArtistImage" WHERE "ArtistsId" = $1 AND "ImagesId" = $2)`, a.ID, id); err != nil {
					return err
				}
			}
		}
	case 1:
		if v, ok := payload["name"].(string); ok {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "Tags" WHERE lower("Name") = lower($1) AND "Id" <> $2)`, v, id).Scan(&exists); err != nil {
				return err
			}
			if exists {
				return errConflict(fmt.Sprintf("Tag with name '%s' already exists.", v))
			}
			if _, err := tx.Exec(ctx, `UPDATE "Tags" SET "Name" = $1 WHERE "Id" = $2`, v, id); err != nil {
				return err
			}
		}
		if v, ok := payload["description"].(string); ok {
			if _, err := tx.Exec(ctx, `UPDATE "Tags" SET "Description" = $1 WHERE "Id" = $2`, v, id); err != nil {
				return err
			}
		}
	case 2:
		if v, ok := payload["name"].(string); ok {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "Artists" WHERE lower("Name") = lower($1) AND "Id" <> $2)`, v, id).Scan(&exists); err != nil {
				return err
			}
			if exists {
				return errConflict(fmt.Sprintf("Artist with name '%s' already exists.", v))
			}
			if _, err := tx.Exec(ctx, `UPDATE "Artists" SET "Name" = $1 WHERE "Id" = $2`, v, id); err != nil {
				return err
			}
		}
		for _, f := range []string{"patreon", "pixiv", "twitter", "deviantArt"} {
			v, ok := payload[f].(string)
			if !ok {
				continue
			}
			col := artistLinkColumns[f]
			var exists bool
			if err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM "Artists" WHERE %s = $1 AND "Id" <> $2)`, col), v, id).Scan(&exists); err != nil {
				return err
			}
			if exists {
				return errConflict(fmt.Sprintf("%s link is already used by another artist.", TitleOrEmpty(f)))
			}
			if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE "Artists" SET %s = $1 WHERE "Id" = $2`, col), orEmptyString(v), id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Server) handleEditReviewTask(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	var req struct {
		Content map[string]any `json:"content"`
	}
	if err := decodeJSON(r, &req); err != nil {
		BadRequest(w, r, "Invalid request body.")
		return
	}

	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		t, err := s.loadReviewTaskRow(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if t.SubmitterID == nil || (*t.SubmitterID != ident.UserID && !isModerator(ident)) {
			return errForbidden("You can only edit your own submissions.")
		}
		if t.Status != 0 {
			return errConflict("This review task has already been resolved.")
		}

		moderatorEditedAt := t.ModeratorEditedAt
		if isModerator(ident) && *t.SubmitterID != ident.UserID {
			now := time.Now().UTC()
			moderatorEditedAt = &now
		}

		if t.Kind == 1 {
			payload, err := s.normalizeEditPayload(ctx, tx, t.TargetType, t.TargetID, req.Content, ident)
			if err != nil {
				return err
			}
			payloadJSON, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `
				UPDATE "ReviewTasks" SET "Payload" = $1::jsonb, "ModeratorEditedAt" = $2 WHERE "Id" = $3`,
				payloadJSON, moderatorEditedAt, id)
			return err
		}

		target, err := targetState(ctx, tx, t.TargetType, t.TargetID, true)
		if err != nil {
			return err
		}
		if !target.exists {
			return errNotFound(fmt.Sprintf("%s with ID %d not found.", targetTypeName(t.TargetType), t.TargetID))
		}
		if target.reviewStatus != 0 {
			return errConflict("This submission has already been reviewed and can no longer be edited here.")
		}
		if err := s.updateNewContentFromPayload(ctx, tx, t, req.Content, ident); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE "ReviewTasks" SET "ModeratorEditedAt" = $1 WHERE "Id" = $2`, moderatorEditedAt, id)
		return err
	})
	if err != nil {
		if isUniqueViolation(err) {
			Conflict(w, r, "The new value is already used by another entry.")
			return
		}
		s.writeDomainError(w, r, err)
		return
	}

	t, err := s.loadReviewTaskRow(ctx, s.db, id, false)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	dto, err := s.hydrateReviewTask(ctx, ident, t)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) updateNewContentFromPayload(ctx context.Context, tx pgx.Tx, t *reviewTaskRow, content map[string]any, ident *auth.Identity) error {
	switch t.TargetType {
	case 0:
		for k, v := range content {
			switch {
			case strings.EqualFold(k, "source"):
				if v == nil {
					continue
				}
				src, ok := v.(string)
				if !ok {
					return errValidation("Invalid content: source must be a string.")
				}
				src = strings.TrimSpace(src)
				if src != "" {
					if runeLen(src) > 500 {
						return errValidation("Invalid content: source must not exceed 500 characters.")
					}
					if !isValidWebURL(src) {
						return errValidation("Invalid content: source must be a valid http(s) URL.")
					}
				}
				if _, err := tx.Exec(ctx, `UPDATE "Images" SET "Source" = $1 WHERE "Id" = $2`, orEmptyString(src), t.TargetID); err != nil {
					return err
				}
			case strings.EqualFold(k, "isNsfw"):
				if v == nil {
					continue
				}
				b, ok := v.(bool)
				if !ok {
					return errValidation("Invalid content: isNsfw must be a boolean.")
				}
				if _, err := tx.Exec(ctx, `UPDATE "Images" SET "IsNsfw" = $1 WHERE "Id" = $2`, b, t.TargetID); err != nil {
					return err
				}
			case strings.EqualFold(k, "tags"):
				slugs, err := cleanStringList(content, "tags", s.cfg.Limits.MaxTagsPerImage)
				if err != nil {
					return errValidation("Invalid content: tags must be an array of strings.")
				}
				tags, err := s.resolveTagsBySlugs(ctx, tx, slugs, ident)
				if err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `DELETE FROM "ImageTag" WHERE "ImagesId" = $1`, t.TargetID); err != nil {
					return err
				}
				for _, tag := range tags {
					if _, err := tx.Exec(ctx, `INSERT INTO "ImageTag" ("ImagesId","TagsId") VALUES ($1,$2)`, t.TargetID, tag.ID); err != nil {
						return err
					}
				}
			case strings.EqualFold(k, "artists"):
				ids, err := cleanIDList(content, "artists", s.cfg.Limits.MaxArtistsPerImage)
				if err != nil {
					return errValidation("Invalid content: artists must be an array of numbers.")
				}
				artists, err := s.resolveArtistsByIDs(ctx, tx, ids, ident)
				if err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `DELETE FROM "ArtistImage" WHERE "ImagesId" = $1`, t.TargetID); err != nil {
					return err
				}
				for _, a := range artists {
					if _, err := tx.Exec(ctx, `INSERT INTO "ArtistImage" ("ArtistsId","ImagesId") VALUES ($1,$2)`, a.ID, t.TargetID); err != nil {
						return err
					}
				}
			default:
				return errValidation(fmt.Sprintf("Invalid content: unknown field '%s'.", k))
			}
		}
	case 1:
		for k, v := range content {
			switch {
			case strings.EqualFold(k, "name"):
				if v == nil {
					continue
				}
				name, ok := v.(string)
				if !ok {
					return errValidation("Invalid content: name must be a string.")
				}
				name = strings.TrimSpace(name)
				if name == "" || runeLen(name) > 30 {
					return errValidation("Invalid content: name must not exceed 30 characters.")
				}
				var exists bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "Tags" WHERE lower("Name") = lower($1) AND "Id" <> $2)`, name, t.TargetID).Scan(&exists); err != nil {
					return err
				}
				if exists {
					return errConflict(fmt.Sprintf("Tag with name '%s' already exists.", name))
				}
				if _, err := tx.Exec(ctx, `UPDATE "Tags" SET "Name" = $1 WHERE "Id" = $2`, name, t.TargetID); err != nil {
					return err
				}
			case strings.EqualFold(k, "description"):
				if v == nil {
					continue
				}
				desc, ok := v.(string)
				if !ok {
					return errValidation("Invalid content: description must be a string.")
				}
				desc = strings.TrimSpace(desc)
				if runeLen(desc) > 150 {
					return errValidation("Invalid content: description must not exceed 150 characters.")
				}
				if _, err := tx.Exec(ctx, `UPDATE "Tags" SET "Description" = $1 WHERE "Id" = $2`, desc, t.TargetID); err != nil {
					return err
				}
			default:
				return errValidation(fmt.Sprintf("Invalid content: unknown field '%s'.", k))
			}
		}
	case 2:
		for k, v := range content {
			if strings.EqualFold(k, "name") {
				if v == nil {
					continue
				}
				name, ok := v.(string)
				if !ok {
					return errValidation("Invalid content: name must be a string.")
				}
				name = strings.TrimSpace(name)
				if name == "" || runeLen(name) > 30 {
					return errValidation("Invalid content: name must not exceed 30 characters.")
				}
				var exists bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "Artists" WHERE lower("Name") = lower($1) AND "Id" <> $2)`, name, t.TargetID).Scan(&exists); err != nil {
					return err
				}
				if exists {
					return errConflict(fmt.Sprintf("Artist with name '%s' already exists.", name))
				}
				if _, err := tx.Exec(ctx, `UPDATE "Artists" SET "Name" = $1 WHERE "Id" = $2`, name, t.TargetID); err != nil {
					return err
				}
				continue
			}
			matched := false
			for _, f := range []string{"patreon", "pixiv", "twitter", "deviantArt"} {
				if !strings.EqualFold(k, f) {
					continue
				}
				matched = true
				if v == nil {
					break
				}
				link, ok := v.(string)
				if !ok {
					return errValidation(fmt.Sprintf("Invalid content: %s must be a string.", f))
				}
				link = strings.TrimSpace(link)
				if link != "" {
					if runeLen(link) > 200 {
						return errValidation(fmt.Sprintf("Invalid content: %s link must not exceed 200 characters.", f))
					}
					if !isValidWebURL(link) {
						return errValidation(fmt.Sprintf("Invalid content: %s link must be a valid http(s) URL.", f))
					}
					col := artistLinkColumns[f]
					var exists bool
					if err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM "Artists" WHERE %s = $1 AND "Id" <> $2)`, col), link, t.TargetID).Scan(&exists); err != nil {
						return err
					}
					if exists {
						return errConflict(fmt.Sprintf("%s link is already used by another artist.", TitleOrEmpty(f)))
					}
				}
				col := artistLinkColumns[f]
				if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE "Artists" SET %s = $1 WHERE "Id" = $2`, col), orEmptyString(link), t.TargetID); err != nil {
					return err
				}
				break
			}
			if !matched {
				return errValidation(fmt.Sprintf("Invalid content: unknown field '%s'.", k))
			}
		}
	}
	return nil
}

func (s *Server) handleApproveReviewTask(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}

	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		t, err := s.loadReviewTaskRow(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if t.Status != 0 {
			return errConflict("This review task has already been resolved.")
		}
		target, err := targetState(ctx, tx, t.TargetType, t.TargetID, true)
		if err != nil {
			return err
		}
		if !target.exists {
			return errNotFound(fmt.Sprintf("%s with ID %d not found.", targetTypeName(t.TargetType), t.TargetID))
		}

		if t.Kind == 1 {
			var payload map[string]any
			if t.Payload != nil {
				if err := json.Unmarshal(t.Payload, &payload); err != nil {
					return err
				}
			}
			if err := s.applyEditPayload(ctx, tx, t.TargetType, t.TargetID, payload, ident); err != nil {
				return err
			}
		} else if target.reviewStatus == 0 {
			var err error
			switch t.TargetType {
			case 0:
				_, err = tx.Exec(ctx, `UPDATE "Images" SET "ReviewStatus" = 1 WHERE "Id" = $1`, t.TargetID)
			case 1:
				_, err = tx.Exec(ctx, `UPDATE "Tags" SET "ReviewStatus" = 1 WHERE "Id" = $1`, t.TargetID)
			case 2:
				_, err = tx.Exec(ctx, `UPDATE "Artists" SET "ReviewStatus" = 1 WHERE "Id" = $1`, t.TargetID)
			}
			if err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `
			UPDATE "ReviewTasks" SET "Status" = 1, "ReviewerId" = $1, "ResolvedAt" = now() WHERE "Id" = $2`,
			ident.UserID, id)
		return err
	})
	if err != nil {
		if isUniqueViolation(err) {
			Conflict(w, r, "The proposed value is already used by another entry.")
			return
		}
		s.writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func discardPendingTarget(ctx context.Context, tx pgx.Tx, t *reviewTaskRow) (string, error) {
	if t.Kind != 0 {
		return "", nil
	}
	target, err := targetState(ctx, tx, t.TargetType, t.TargetID, true)
	if err != nil {
		return "", err
	}
	if !target.exists || target.reviewStatus != 0 {
		return "", nil
	}
	switch t.TargetType {
	case 0:
		if _, err := tx.Exec(ctx, `DELETE FROM "Images" WHERE "Id" = $1`, t.TargetID); err != nil {
			return "", err
		}
		return imageKey(t.TargetID, target.extension), nil
	case 1:
		_, err = tx.Exec(ctx, `DELETE FROM "Tags" WHERE "Id" = $1`, t.TargetID)
	case 2:
		_, err = tx.Exec(ctx, `DELETE FROM "Artists" WHERE "Id" = $1`, t.TargetID)
	}
	return "", err
}

func (s *Server) removeStoredImage(ctx context.Context, key string) {
	if key == "" {
		return
	}
	s.objects.remove(ctx, key)
}

func (s *Server) handleRejectReviewTask(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
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

	var deletedKey string
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		t, err := s.loadReviewTaskRow(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if t.Status != 0 {
			return errConflict("This review task has already been resolved.")
		}
		if deletedKey, err = discardPendingTarget(ctx, tx, t); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE "ReviewTasks" SET "Status" = 2, "ReviewerId" = $1, "ReviewerNote" = $2, "ResolvedAt" = now()
			WHERE "Id" = $3`, ident.UserID, orEmptyString(reason), id)
		return err
	})
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	s.removeStoredImage(ctx, deletedKey)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCancelReviewTask(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}

	var deletedKey string
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		t, err := s.loadReviewTaskRow(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if t.SubmitterID == nil || (*t.SubmitterID != ident.UserID && !isModerator(ident)) {
			return errForbidden("You can only cancel your own submissions.")
		}
		if t.Status != 0 {
			return errConflict("This review task has already been resolved.")
		}
		if deletedKey, err = discardPendingTarget(ctx, tx, t); err != nil {
			return err
		}
		if isModerator(ident) && *t.SubmitterID != ident.UserID {
			_, err = tx.Exec(ctx, `UPDATE "ReviewTasks" SET "Status" = 3, "ResolvedAt" = now(), "ReviewerId" = $2 WHERE "Id" = $1`, id, ident.UserID)
		} else {
			_, err = tx.Exec(ctx, `UPDATE "ReviewTasks" SET "Status" = 3, "ResolvedAt" = now() WHERE "Id" = $1`, id)
		}
		return err
	})
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	s.removeStoredImage(ctx, deletedKey)
	w.WriteHeader(http.StatusNoContent)
}
