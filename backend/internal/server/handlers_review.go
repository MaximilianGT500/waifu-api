package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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

type notFoundError struct{ msg string }

func (e *notFoundError) Error() string { return e.msg }

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

func (s *Server) loadReviewTaskRow(ctx context.Context, id int64) (*reviewTaskRow, error) {
	var t reviewTaskRow
	err := s.db.QueryRow(ctx, `
		SELECT "Id","Kind","SubmitterId","TargetType","TargetId","Payload","Reason","Status",
			"ReviewerId","ReviewerNote","ModeratorEditedAt","CreatedAt","ResolvedAt"
		FROM "ReviewTasks" WHERE "Id" = $1`, id).
		Scan(&t.ID, &t.Kind, &t.SubmitterID, &t.TargetType, &t.TargetID, &t.Payload, &t.Reason,
			&t.Status, &t.ReviewerID, &t.ReviewerNote, &t.ModeratorEditedAt, &t.CreatedAt, &t.ResolvedAt)
	return &t, err
}

func (s *Server) hydrateReviewTask(r *http.Request, t *reviewTaskRow) *reviewTaskDto {
	ident := IdentityFromContext(r.Context())
	dto := &reviewTaskDto{
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
		var u userMinimalDto
		if err := s.db.QueryRow(r.Context(), `SELECT "Id","Name","AvatarUrl" FROM "Users" WHERE "Id" = $1`, *t.SubmitterID).
			Scan(&u.ID, &u.Name, &u.AvatarURL); err == nil {
			dto.Submitter = &u
		}
	}
	if t.ReviewerID != nil {
		var u userMinimalDto
		if err := s.db.QueryRow(r.Context(), `SELECT "Id","Name","AvatarUrl" FROM "Users" WHERE "Id" = $1`, *t.ReviewerID).
			Scan(&u.ID, &u.Name, &u.AvatarURL); err == nil {
			dto.Reviewer = &u
		}
	}

	var target map[string]any
	switch t.TargetType {
	case 0:
		var row imageRow
		err := s.db.QueryRow(r.Context(), `
			SELECT "Id","PerceptualHash","Extension","DominantColor","Source","UploaderId","UploadedAt",
				"IsNsfw","IsAnimated","Width","Height","ByteSize","ReviewStatus"
			FROM "Images" WHERE "Id" = $1`, t.TargetID).
			Scan(&row.ID, &row.PerceptualHash, &row.Extension, &row.DominantColor, &row.Source, &row.UploaderID,
				&row.UploadedAt, &row.IsNsfw, &row.IsAnimated, &row.Width, &row.Height, &row.ByteSize, &row.ReviewStatus)
		if err == nil {
			if img, err := s.assembleImageDto(r.Context(), row, ident, "All", false, nil, true); err == nil {
				img.Favorites = 0
				img.LikedAt = nil
				img.Albums = []imageAlbumRef{}
				target = map[string]any{"image": img}
			}
		}
	case 1:
		var tag tagDto
		var rs int
		err := s.db.QueryRow(r.Context(), `
			SELECT t."Id", t."Name", t."Slug", t."Description", t."ReviewStatus"
			FROM "Tags" t WHERE t."Id" = $1`, t.TargetID).
			Scan(&tag.ID, &tag.Name, &tag.Slug, &tag.Description, &rs)
		if err == nil {
			tag.ReviewStatus = reviewStatusString(rs)
			target = map[string]any{"tag": tag}
		}
	case 2:
		var a artistDto
		var rs int
		var patreon, pixiv, twitter, deviantArt *string
		err := s.db.QueryRow(r.Context(), `
			SELECT "Id","Name","Patreon","Pixiv","Twitter","DeviantArt","ReviewStatus"
			FROM "Artists" WHERE "Id" = $1`, t.TargetID).
			Scan(&a.ID, &a.Name, &patreon, &pixiv, &twitter, &deviantArt, &rs)
		if err == nil {
			a.ReviewStatus = reviewStatusString(rs)
			a.Patreon = orEmpty(patreon)
			a.Pixiv = orEmpty(pixiv)
			a.Twitter = orEmpty(twitter)
			a.DeviantArt = orEmpty(deviantArt)
			target = map[string]any{"artist": a}
		}
	}
	dto.Target = target
	return dto
}

func (s *Server) handleListReviewTasks(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	page, pageSize := paginationParams(r, s.cfg.Review.DefaultPageSize, s.cfg.Review.MaxPageSize)

	where := " WHERE TRUE"
	args := []any{}
	ph := func() string { return fmt.Sprintf("$%d", len(args)+1) }
	if st := r.URL.Query().Get("status"); st != "" {
		v, ok := reviewTaskStatusInt(st)
		if !ok {
			invalidEnumProblem(w, r, "status", st)
			return
		}
		where += fmt.Sprintf(` AND t."Status" = %s`, ph())
		args = append(args, v)
	}
	if k := r.URL.Query().Get("kind"); k != "" {
		v, ok := kindInt(k)
		if !ok {
			invalidEnumProblem(w, r, "kind", k)
			return
		}
		where += fmt.Sprintf(` AND t."Kind" = %s`, ph())
		args = append(args, v)
	}
	if tt := r.URL.Query().Get("targetType"); tt != "" {
		v, ok := targetTypeInt(tt)
		if !ok {
			invalidEnumProblem(w, r, "targetType", tt)
			return
		}
		where += fmt.Sprintf(` AND t."TargetType" = %s`, ph())
		args = append(args, v)
	}
	if tid := r.URL.Query().Get("targetId"); tid != "" {
		n, err := strconv.ParseInt(tid, 10, 64)
		if err != nil {
			invalidEnumProblem(w, r, "targetId", tid)
			return
		}
		where += fmt.Sprintf(` AND t."TargetId" = %s`, ph())
		args = append(args, n)
	}
	mine := queryBool(r, "mine", false)
	if isModerator(ident) && mine {
		where += fmt.Sprintf(` AND t."SubmitterId" = %s`, ph())
		args = append(args, ident.UserID)
	} else if !isModerator(ident) {
		where += fmt.Sprintf(` AND t."SubmitterId" = %s`, ph())
		args = append(args, ident.UserID)
	}

	var total int64
	_ = s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM "ReviewTasks" t`+where, args...).Scan(&total)

	rows, err := s.db.Query(r.Context(), `
		SELECT "Id","Kind","SubmitterId","TargetType","TargetId","Payload","Reason","Status",
			"ReviewerId","ReviewerNote","ModeratorEditedAt","CreatedAt","ResolvedAt"
		FROM "ReviewTasks" t`+where+fmt.Sprintf(` ORDER BY t."CreatedAt" DESC LIMIT %d OFFSET %d`, pageSize, (page-1)*pageSize), args...)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	defer rows.Close()

	items := []reviewTaskDto{}
	for rows.Next() {
		var t reviewTaskRow
		if err := rows.Scan(&t.ID, &t.Kind, &t.SubmitterID, &t.TargetType, &t.TargetID, &t.Payload, &t.Reason,
			&t.Status, &t.ReviewerID, &t.ReviewerNote, &t.ModeratorEditedAt, &t.CreatedAt, &t.ResolvedAt); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		items = append(items, *s.hydrateReviewTask(r, &t))
	}
	writeJSON(w, http.StatusOK, newPaginated(items, page, pageSize, total, s.cfg.Review.MaxPageSize, s.cfg.Review.DefaultPageSize))
}

func (s *Server) handleGetReviewTask(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	t, err := s.loadReviewTaskRow(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, fmt.Sprintf("Review task with ID %d not found.", id))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if !isModerator(ident) && (t.SubmitterID == nil || *t.SubmitterID != ident.UserID) {
		NotFound(w, r, fmt.Sprintf("Review task with ID %d not found.", id))
		return
	}
	writeJSON(w, http.StatusOK, s.hydrateReviewTask(r, t))
}

func (s *Server) handleCreateReviewTask(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
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
		WriteValidationProblem(w, r, map[string][]string{
			"TargetType": {"Invalid target type."},
		})
		return
	}
	if req.TargetID <= 0 {
		WriteValidationProblem(w, r, map[string][]string{
			"TargetId": {"Invalid target ID."},
		})
		return
	}
	if req.Payload == nil {
		WriteValidationProblem(w, r, map[string][]string{
			"PayloadJson": {"Proposed changes are required."},
		})
		return
	}
	if len(req.Reason) > 500 {
		WriteValidationProblem(w, r, map[string][]string{
			"Reason": {"Reason must not exceed 500 characters."},
		})
		return
	}

	if !s.targetExistsAccepted(r.Context(), tt, req.TargetID) {
		NotFound(w, r, fmt.Sprintf("%s with ID %d not found.", targetTypeName(tt), req.TargetID))
		return
	}

	payload, err := s.normalizeEditPayload(r.Context(), tt, req.TargetID, req.Payload)
	if err != nil {
		var nf *notFoundError
		if errors.As(err, &nf) {
			NotFound(w, r, nf.msg)
			return
		}
		BadRequest(w, r, err.Error())
		return
	}

	if s.cfg.Review.MaxOpenEditsPerUser > 0 {
		var open int64
		_ = s.db.QueryRow(r.Context(), `
			SELECT COUNT(*) FROM "ReviewTasks" WHERE "SubmitterId" = $1 AND "Kind" = 1 AND "Status" = 0`,
			ident.UserID).Scan(&open)
		if open >= int64(s.cfg.Review.MaxOpenEditsPerUser) {
			Conflict(w, r, fmt.Sprintf("You have too many pending edits (limit %d). Please wait for some to be reviewed.", s.cfg.Review.MaxOpenEditsPerUser))
			return
		}
	}

	payloadJSON, _ := json.Marshal(payload)

	autoApply := !s.cfg.Review.RequireEditRequestReview || auth.ReviewBypassed(ident.Role, s.editBypassMinRole(tt))

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	defer tx.Rollback(r.Context())

	now := time.Now().UTC()
	status := 0
	var reviewerNote *string
	var resolvedAt *time.Time
	if autoApply {
		status = 1
		note := "Auto-applied (trusted role)."
		reviewerNote = &note
		resolvedAt = &now
	}

	var taskID int64
	err = tx.QueryRow(r.Context(), `
		INSERT INTO "ReviewTasks" ("Kind","TargetType","TargetId","SubmitterId","Payload","Reason","Status","ReviewerNote","CreatedAt","ResolvedAt")
		VALUES (1,$1,$2,$3,$4::jsonb,$5,$6,$7,$8,$9)
		RETURNING "Id"`,
		tt, req.TargetID, ident.UserID, payloadJSON, orEmptyString(strings.TrimSpace(req.Reason)), status, reviewerNote, now, resolvedAt).Scan(&taskID)
	if err != nil {
		if isUniqueViolation(err) {
			Conflict(w, r, "You already have a pending edit for this item. Edit or cancel it first.")
			return
		}
		InternalError(w, r, s.log, err)
		return
	}

	if autoApply {
		if err := s.applyEditPayload(tx, r.Context(), tt, req.TargetID, payload); err != nil {
			var nf *notFoundError
			if errors.As(err, &nf) {
				NotFound(w, r, nf.msg)
				return
			}
			var cf *conflictError
			if errors.As(err, &cf) {
				Conflict(w, r, cf.msg)
				return
			}
			InternalError(w, r, s.log, err)
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	t, err := s.loadReviewTaskRow(r.Context(), taskID)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/review/tasks/%d", taskID))
	writeJSON(w, http.StatusCreated, s.hydrateReviewTask(r, t))
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

type conflictError struct{ msg string }

func (e *conflictError) Error() string { return e.msg }

func (s *Server) targetExistsAccepted(ctx context.Context, tt int, id int64) bool {
	var ok bool
	switch tt {
	case 0:
		_ = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "Images" WHERE "Id" = $1 AND "ReviewStatus" = 1)`, id).Scan(&ok)
	case 1:
		_ = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "Tags" WHERE "Id" = $1 AND "ReviewStatus" = 1)`, id).Scan(&ok)
	case 2:
		_ = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "Artists" WHERE "Id" = $1 AND "ReviewStatus" = 1)`, id).Scan(&ok)
	}
	return ok
}

func (s *Server) normalizeEditPayload(ctx context.Context, tt int, id int64, in map[string]any) (map[string]any, error) {
	if in == nil {
		return nil, errors.New("Invalid payload: payload is required.")
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
		_ = s.db.QueryRow(ctx, `SELECT "Source","IsNsfw" FROM "Images" WHERE "Id" = $1`, id).Scan(&source, &isNsfw)

		if v, ok := fieldString(in, "source"); ok {
			proposed := strings.TrimSpace(v)
			if proposed != "" {
				if len(proposed) > 500 {
					return nil, errors.New("Source URL must not exceed 500 characters.")
				}
				if !isValidWebURL(proposed) {
					return nil, errors.New("Source must be a valid http(s) URL.")
				}
				if proposed != orEmpty(source) {
					out["source"] = proposed
				}
			}

		}
		if v, ok := fieldBool(in, "isNsfw"); ok && v != isNsfw {
			out["isNsfw"] = v
		}

		currentTagSlugs, err := s.imageTagSlugs(ctx, id)
		if err != nil {
			return nil, err
		}
		currentArtistIDs, err := s.imageArtistIDs(ctx, id)
		if err != nil {
			return nil, err
		}

		addTags, err := cleanStringList(in, "addTagSlugs")
		if err != nil {
			return nil, err
		}
		removeTags, err := cleanStringList(in, "removeTagSlugs")
		if err != nil {
			return nil, err
		}
		if len(addTags) > 0 {
			var found []string
			frows, err := s.db.Query(ctx, `SELECT "Slug" FROM "Tags" WHERE "Slug" = ANY($1)`, addTags)
			if err != nil {
				return nil, err
			}
			for frows.Next() {
				var slug string
				if err := frows.Scan(&slug); err != nil {
					frows.Close()
					return nil, err
				}
				found = append(found, slug)
			}
			frows.Close()
			foundSet := map[string]bool{}
			for _, f := range found {
				foundSet[f] = true
			}
			var missing []string
			for _, slug := range addTags {
				if !foundSet[slug] {
					missing = append(missing, slug)
				}
			}
			if len(missing) > 0 {
				return nil, &notFoundError{fmt.Sprintf("Tags with slugs %s not found.", strings.Join(missing, ", "))}
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

		addArtists, err := cleanIDList(in, "addArtistIds")
		if err != nil {
			return nil, err
		}
		removeArtists, err := cleanIDList(in, "removeArtistIds")
		if err != nil {
			return nil, err
		}
		if len(addArtists) > 0 {
			if err := s.artistsExist(ctx, addArtists); err != nil {
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
		var name, desc string
		_ = s.db.QueryRow(ctx, `SELECT "Name","Description" FROM "Tags" WHERE "Id" = $1`, id).Scan(&name, &desc)
		if v, ok := fieldString(in, "name"); ok {
			proposed := strings.TrimSpace(v)
			if proposed != "" {
				if len(proposed) > 30 {
					return nil, errors.New("Name must not exceed 30 characters.")
				}
				if proposed != name {
					out["name"] = proposed
				}
			}
		}
		if v, ok := fieldString(in, "description"); ok {
			proposed := strings.TrimSpace(v)
			if proposed != "" {
				if len(proposed) > 150 {
					return nil, errors.New("Description must not exceed 150 characters.")
				}
				if proposed != desc {
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
		_ = s.db.QueryRow(ctx, `SELECT "Name","Patreon","Pixiv","Twitter","DeviantArt" FROM "Artists" WHERE "Id" = $1`, id).
			Scan(&name, &patreon, &pixiv, &twitter, &deviantArt)
		if v, ok := fieldString(in, "name"); ok {
			proposed := strings.TrimSpace(v)
			if proposed != "" {
				if len(proposed) > 30 {
					return nil, errors.New("Name must not exceed 30 characters.")
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
					if len(proposed) > 200 {
						return nil, fmt.Errorf("%s link must not exceed 200 characters.", strings.ToUpper(f.key[:1])+f.key[1:])
					}
					if !isValidWebURL(proposed) {
						return nil, fmt.Errorf("%s link must be a valid http(s) URL.", strings.ToUpper(f.key[:1])+f.key[1:])
					}
					if proposed != orEmpty(f.cur) {
						out[f.key] = proposed
					}
				}

			}
		}
	default:
		return nil, errors.New("Invalid payload: unknown target type.")
	}
	if len(out) == 0 {
		return nil, errors.New("The proposed changes are empty or identical to the current values.")
	}
	return out, nil
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
			return fmt.Errorf("Invalid payload: The JSON property '%s' is not allowed.", key)
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

func cleanStringList(in map[string]any, key string) ([]string, error) {
	for k, v := range in {
		if !strings.EqualFold(k, key) {
			continue
		}
		if v == nil {
			return nil, nil
		}
		arr, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("Invalid payload: '%s' must be an array of strings.", key)
		}
		seen := map[string]bool{}
		var out []string
		for _, item := range arr {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("Invalid payload: '%s' must be an array of strings.", key)
			}
			s = strings.TrimSpace(s)
			if s != "" && !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
		return out, nil
	}
	return nil, nil
}

func cleanIDList(in map[string]any, key string) ([]int64, error) {
	for k, v := range in {
		if !strings.EqualFold(k, key) {
			continue
		}
		if v == nil {
			return nil, nil
		}
		arr, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("Invalid payload: '%s' must be an array of numbers.", key)
		}
		seen := map[int64]bool{}
		var out []int64
		for _, item := range arr {
			n, ok := toInt64(item)
			if !ok || n <= 0 {
				return nil, fmt.Errorf("Invalid payload: '%s' must be an array of numbers.", key)
			}
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
		return out, nil
	}
	return nil, nil
}

func (s *Server) imageTagSlugs(ctx context.Context, id int64) (map[string]bool, error) {
	out := map[string]bool{}
	rows, err := s.db.Query(ctx, `
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
	return out, nil
}

func (s *Server) imageArtistIDs(ctx context.Context, id int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	rows, err := s.db.Query(ctx, `SELECT "ArtistsId" FROM "ArtistImage" WHERE "ImagesId" = $1`, id)
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
	return out, nil
}

func (s *Server) artistsExist(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	var found []int64
	rows, err := s.db.Query(ctx, `SELECT "Id" FROM "Artists" WHERE "Id" = ANY($1)`, ids)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		found = append(found, id)
	}
	rows.Close()
	foundSet := map[int64]bool{}
	for _, f := range found {
		foundSet[f] = true
	}
	var missing []string
	for _, id := range ids {
		if !foundSet[id] {
			missing = append(missing, strconv.FormatInt(id, 10))
		}
	}
	if len(missing) > 0 {
		return &notFoundError{fmt.Sprintf("Artists with IDs %s not found.", strings.Join(missing, ", "))}
	}
	return nil
}

func isValidWebURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

func (s *Server) applyEditPayload(tx pgx.Tx, ctx context.Context, tt int, id int64, payload map[string]any) error {
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
		_, hasAdd := payload["addTagSlugs"].([]any)
		_, hasRemove := payload["removeTagSlugs"].([]any)
		if hasAdd || hasRemove {

			current, err := s.imageTagSlugs(ctx, id)
			if err != nil {
				return err
			}
			merged := []string{}
			for slug := range current {
				merged = append(merged, slug)
			}
			if v, ok := payload["addTagSlugs"].([]any); ok {
				for _, x := range v {
					merged = append(merged, x.(string))
				}
			}
			if v, ok := payload["removeTagSlugs"].([]any); ok {
				rm := map[string]bool{}
				for _, x := range v {
					rm[x.(string)] = true
				}
				var kept []string
				for _, slug := range merged {
					if !rm[slug] {
						kept = append(kept, slug)
					}
				}
				merged = kept
			}
			tags, err := s.resolveTagsBySlugs(ctx, merged)
			if err != nil {
				return &notFoundError{err.Error()}
			}
			if _, err := tx.Exec(ctx, `DELETE FROM "ImageTag" WHERE "ImagesId" = $1`, id); err != nil {
				return err
			}
			for _, t := range tags {
				if _, err := tx.Exec(ctx, `INSERT INTO "ImageTag" ("ImagesId","TagsId") VALUES ($1,$2)`, id, t.ID); err != nil {
					return err
				}
			}
		}
		_, hasAddArtists := payload["addArtistIds"].([]any)
		_, hasRemoveArtists := payload["removeArtistIds"].([]any)
		if hasAddArtists || hasRemoveArtists {
			current, err := s.imageArtistIDs(ctx, id)
			if err != nil {
				return err
			}
			merged := []int64{}
			for aid := range current {
				merged = append(merged, aid)
			}
			if v, ok := payload["addArtistIds"].([]any); ok {
				for _, x := range v {
					if n, ok := toInt64(x); ok {
						merged = append(merged, n)
					}
				}
			}
			if v, ok := payload["removeArtistIds"].([]any); ok {
				rm := map[int64]bool{}
				for _, x := range v {
					if n, ok := toInt64(x); ok {
						rm[n] = true
					}
				}
				var kept []int64
				for _, aid := range merged {
					if !rm[aid] {
						kept = append(kept, aid)
					}
				}
				merged = kept
			}
			if _, err := tx.Exec(ctx, `DELETE FROM "ArtistImage" WHERE "ImagesId" = $1`, id); err != nil {
				return err
			}
			for _, aid := range merged {
				if _, err := tx.Exec(ctx, `INSERT INTO "ArtistImage" ("ArtistsId","ImagesId") VALUES ($1,$2)`, aid, id); err != nil {
					return err
				}
			}
		}
	case 1:
		if v, ok := payload["name"].(string); ok {
			var conflictID int64
			if err := tx.QueryRow(ctx, `SELECT "Id" FROM "Tags" WHERE lower("Name") = lower($1) AND "Id" <> $2 LIMIT 1`, v, id).Scan(&conflictID); err == nil && conflictID != 0 {
				return &conflictError{fmt.Sprintf("Tag with name '%s' already exists.", v)}
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
			var conflictID int64
			if err := tx.QueryRow(ctx, `SELECT "Id" FROM "Artists" WHERE lower("Name") = lower($1) AND "Id" <> $2 LIMIT 1`, v, id).Scan(&conflictID); err == nil && conflictID != 0 {
				return &conflictError{fmt.Sprintf("Artist with name '%s' already exists.", v)}
			}
			if _, err := tx.Exec(ctx, `UPDATE "Artists" SET "Name" = $1 WHERE "Id" = $2`, v, id); err != nil {
				return err
			}
		}
		for _, f := range []string{"patreon", "pixiv", "twitter", "deviantArt"} {
			if v, ok := payload[f].(string); ok {
				col := map[string]string{"patreon": `"Patreon"`, "pixiv": `"Pixiv"`, "twitter": `"Twitter"`, "deviantArt": `"DeviantArt"`}[f]
				var conflictID int64
				if err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT "Id" FROM "Artists" WHERE %s = $1 AND "Id" <> $2 LIMIT 1`, col), v, id).Scan(&conflictID); err == nil && conflictID != 0 {
					return &conflictError{fmt.Sprintf("%s link is already used by another artist.", strings.ToUpper(f[:1])+f[1:])}
				}
				if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE "Artists" SET %s = $1 WHERE "Id" = $2`, col), orEmptyString(v), id); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
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

func (s *Server) handleEditReviewTask(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	t, err := s.loadReviewTaskRow(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, fmt.Sprintf("Review task with ID %d not found.", id))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if t.SubmitterID == nil || (*t.SubmitterID != ident.UserID && !isModerator(ident)) {
		Forbidden(w, r, "You can only edit your own submissions.")
		return
	}
	if t.Status != 0 {
		Conflict(w, r, "This review task has already been resolved.")
		return
	}

	var req struct {
		Content map[string]any `json:"content"`
	}
	if err := decodeJSON(r, &req); err != nil {
		BadRequest(w, r, "Invalid request body.")
		return
	}

	moderatorEditedAt := t.ModeratorEditedAt
	if isModerator(ident) && t.SubmitterID != nil && *t.SubmitterID != ident.UserID {
		now := time.Now().UTC()
		moderatorEditedAt = &now
	}

	if t.Kind == 1 {
		payload, err := s.normalizeEditPayload(r.Context(), t.TargetType, t.TargetID, req.Content)
		if err != nil {
			var nf *notFoundError
			if errors.As(err, &nf) {
				NotFound(w, r, nf.msg)
				return
			}
			BadRequest(w, r, err.Error())
			return
		}
		payloadJSON, _ := json.Marshal(payload)
		_, err = s.db.Exec(r.Context(), `
			UPDATE "ReviewTasks" SET "Payload" = $1::jsonb, "ModeratorEditedAt" = $2 WHERE "Id" = $3`,
			payloadJSON, moderatorEditedAt, id)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
	} else {
		if err := s.updateNewContentFromPayload(r.Context(), t, req.Content); err != nil {
			var nf *notFoundError
			if errors.As(err, &nf) {
				NotFound(w, r, nf.msg)
				return
			}
			BadRequest(w, r, err.Error())
			return
		}
		_, _ = s.db.Exec(r.Context(), `UPDATE "ReviewTasks" SET "ModeratorEditedAt" = $1 WHERE "Id" = $2`, moderatorEditedAt, id)
	}

	t, err = s.loadReviewTaskRow(r.Context(), id)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, s.hydrateReviewTask(r, t))
}

func (s *Server) updateNewContentFromPayload(ctx context.Context, t *reviewTaskRow, content map[string]any) error {
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
					return errors.New("Invalid content: source must be a string.")
				}
				src = strings.TrimSpace(src)
				if src != "" {
					if len(src) > 500 {
						return errors.New("Invalid content: source must not exceed 500 characters.")
					}
					if !isValidWebURL(src) {
						return errors.New("Invalid content: source must be a valid http(s) URL.")
					}
				}
				if _, err := s.db.Exec(ctx, `UPDATE "Images" SET "Source" = $1 WHERE "Id" = $2`, orEmptyString(src), t.TargetID); err != nil {
					return err
				}
			case strings.EqualFold(k, "isNsfw"):
				if v == nil {
					continue
				}
				b, ok := v.(bool)
				if !ok {
					return errors.New("Invalid content: isNsfw must be a boolean.")
				}
				if _, err := s.db.Exec(ctx, `UPDATE "Images" SET "IsNsfw" = $1 WHERE "Id" = $2`, b, t.TargetID); err != nil {
					return err
				}
			case strings.EqualFold(k, "tags"):
				slugs, err := cleanStringList(content, "tags")
				if err != nil {
					return errors.New("Invalid content: tags must be an array of strings.")
				}
				tags, err := s.resolveTagsBySlugs(ctx, slugs)
				if err != nil {
					return &notFoundError{err.Error()}
				}
				if _, err := s.db.Exec(ctx, `DELETE FROM "ImageTag" WHERE "ImagesId" = $1`, t.TargetID); err != nil {
					return err
				}
				for _, tag := range tags {
					if _, err := s.db.Exec(ctx, `INSERT INTO "ImageTag" ("ImagesId","TagsId") VALUES ($1,$2)`, t.TargetID, tag.ID); err != nil {
						return err
					}
				}
			case strings.EqualFold(k, "artists"):
				raw := []string{}
				arr, ok := v.([]any)
				if !ok {
					return errors.New("Invalid content: artists must be an array of numbers.")
				}
				for _, x := range arr {
					if n, ok := toInt64(x); ok && n > 0 {
						raw = append(raw, strconv.FormatInt(n, 10))
					} else {
						return errors.New("Invalid content: artists must be an array of numbers.")
					}
				}
				artists, err := s.resolveArtistsByIDs(ctx, raw)
				if err != nil {
					return &notFoundError{err.Error()}
				}
				if _, err := s.db.Exec(ctx, `DELETE FROM "ArtistImage" WHERE "ImagesId" = $1`, t.TargetID); err != nil {
					return err
				}
				for _, a := range artists {
					if _, err := s.db.Exec(ctx, `INSERT INTO "ArtistImage" ("ArtistsId","ImagesId") VALUES ($1,$2)`, a.ID, t.TargetID); err != nil {
						return err
					}
				}
			default:
				return fmt.Errorf("Invalid content: unknown field '%s'.", k)
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
					return errors.New("Invalid content: name must be a string.")
				}
				name = strings.TrimSpace(name)
				if name == "" || len(name) > 30 {
					return errors.New("Invalid content: name must not exceed 30 characters.")
				}
				if _, err := s.db.Exec(ctx, `UPDATE "Tags" SET "Name" = $1 WHERE "Id" = $2`, name, t.TargetID); err != nil {
					return err
				}
			case strings.EqualFold(k, "description"):
				if v == nil {
					continue
				}
				desc, ok := v.(string)
				if !ok {
					return errors.New("Invalid content: description must be a string.")
				}
				desc = strings.TrimSpace(desc)
				if len(desc) > 150 {
					return errors.New("Invalid content: description must not exceed 150 characters.")
				}
				if _, err := s.db.Exec(ctx, `UPDATE "Tags" SET "Description" = $1 WHERE "Id" = $2`, desc, t.TargetID); err != nil {
					return err
				}
			default:
				return fmt.Errorf("Invalid content: unknown field '%s'.", k)
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
					return errors.New("Invalid content: name must be a string.")
				}
				name = strings.TrimSpace(name)
				if name == "" || len(name) > 30 {
					return errors.New("Invalid content: name must not exceed 30 characters.")
				}
				if _, err := s.db.Exec(ctx, `UPDATE "Artists" SET "Name" = $1 WHERE "Id" = $2`, name, t.TargetID); err != nil {
					return err
				}
				continue
			}
			for _, f := range []string{"patreon", "pixiv", "twitter", "deviantArt"} {
				if !strings.EqualFold(k, f) {
					continue
				}
				if v == nil {
					continue
				}
				link, ok := v.(string)
				if !ok {
					return fmt.Errorf("Invalid content: %s must be a string.", f)
				}
				link = strings.TrimSpace(link)
				if link != "" {
					if len(link) > 200 {
						return fmt.Errorf("Invalid content: %s link must not exceed 200 characters.", f)
					}
					if !isValidWebURL(link) {
						return fmt.Errorf("Invalid content: %s link must be a valid http(s) URL.", f)
					}
				}
				col := map[string]string{"patreon": `"Patreon"`, "pixiv": `"Pixiv"`, "twitter": `"Twitter"`, "deviantArt": `"DeviantArt"`}[f]
				if _, err := s.db.Exec(ctx, fmt.Sprintf(`UPDATE "Artists" SET %s = $1 WHERE "Id" = $2`, col), orEmptyString(link), t.TargetID); err != nil {
					return err
				}
				goto nextField
			}
			return fmt.Errorf("Invalid content: unknown field '%s'.", k)
		nextField:
		}
	}
	return nil
}

func (s *Server) handleApproveReviewTask(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	t, err := s.loadReviewTaskRow(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, fmt.Sprintf("Review task with ID %d not found.", id))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if t.Status != 0 {
		Conflict(w, r, "This review task has already been resolved.")
		return
	}

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	defer tx.Rollback(r.Context())

	if t.Kind == 1 {
		if !s.targetExists(r.Context(), t.TargetType, t.TargetID) {
			NotFound(w, r, fmt.Sprintf("%s with ID %d not found.", targetTypeName(t.TargetType), t.TargetID))
			return
		}
		var payload map[string]any
		if t.Payload != nil {
			_ = json.Unmarshal(t.Payload, &payload)
		}
		if err := s.applyEditPayload(tx, r.Context(), t.TargetType, t.TargetID, payload); err != nil {
			var nf *notFoundError
			if errors.As(err, &nf) {
				NotFound(w, r, nf.msg)
				return
			}
			var cf *conflictError
			if errors.As(err, &cf) {
				Conflict(w, r, cf.msg)
				return
			}
			InternalError(w, r, s.log, err)
			return
		}
	} else {

		if !s.targetExists(r.Context(), t.TargetType, t.TargetID) {
			NotFound(w, r, fmt.Sprintf("%s with ID %d not found.", targetTypeName(t.TargetType), t.TargetID))
			return
		}
		switch t.TargetType {
		case 0:
			_, err = tx.Exec(r.Context(), `UPDATE "Images" SET "ReviewStatus" = 1 WHERE "Id" = $1`, t.TargetID)
		case 1:
			_, err = tx.Exec(r.Context(), `UPDATE "Tags" SET "ReviewStatus" = 1 WHERE "Id" = $1`, t.TargetID)
		case 2:
			_, err = tx.Exec(r.Context(), `UPDATE "Artists" SET "ReviewStatus" = 1 WHERE "Id" = $1`, t.TargetID)
		}
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
	}
	_, err = tx.Exec(r.Context(), `
		UPDATE "ReviewTasks" SET "Status" = 1, "ReviewerId" = $1, "ResolvedAt" = now() WHERE "Id" = $2`,
		ident.UserID, id)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) targetExists(ctx context.Context, tt int, id int64) bool {
	var ok bool
	switch tt {
	case 0:
		_ = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "Images" WHERE "Id" = $1)`, id).Scan(&ok)
	case 1:
		_ = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "Tags" WHERE "Id" = $1)`, id).Scan(&ok)
	case 2:
		_ = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "Artists" WHERE "Id" = $1)`, id).Scan(&ok)
	}
	return ok
}

func (s *Server) handleRejectReviewTask(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	t, err := s.loadReviewTaskRow(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, fmt.Sprintf("Review task with ID %d not found.", id))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if t.Status != 0 {
		Conflict(w, r, "This review task has already been resolved.")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = decodeJSON(r, &req)

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	defer tx.Rollback(r.Context())

	if t.Kind == 0 {
		if !s.targetExists(r.Context(), t.TargetType, t.TargetID) {
			NotFound(w, r, fmt.Sprintf("%s with ID %d not found.", targetTypeName(t.TargetType), t.TargetID))
			return
		}
		switch t.TargetType {
		case 0:
			var ext string
			if err := tx.QueryRow(r.Context(), `SELECT "Extension" FROM "Images" WHERE "Id" = $1`, t.TargetID).Scan(&ext); err == nil {
				_ = s.s3.Delete(r.Context(), fmt.Sprintf("%d%s", t.TargetID, ext))
			}
			_, err = tx.Exec(r.Context(), `DELETE FROM "Images" WHERE "Id" = $1`, t.TargetID)
		case 1:
			_, err = tx.Exec(r.Context(), `DELETE FROM "Tags" WHERE "Id" = $1`, t.TargetID)
		case 2:
			_, err = tx.Exec(r.Context(), `DELETE FROM "Artists" WHERE "Id" = $1`, t.TargetID)
		}
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
	}
	_, err = tx.Exec(r.Context(), `
		UPDATE "ReviewTasks" SET "Status" = 2, "ReviewerId" = $1, "ReviewerNote" = $2, "ResolvedAt" = now()
		WHERE "Id" = $3`, ident.UserID, orEmptyString(strings.TrimSpace(req.Reason)), id)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCancelReviewTask(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	t, err := s.loadReviewTaskRow(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, fmt.Sprintf("Review task with ID %d not found.", id))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if t.SubmitterID == nil || (*t.SubmitterID != ident.UserID && !isModerator(ident)) {
		Forbidden(w, r, "You can only cancel your own submissions.")
		return
	}
	if t.Status != 0 {
		Conflict(w, r, "This review task has already been resolved.")
		return
	}

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	defer tx.Rollback(r.Context())

	if t.Kind == 0 {
		if !s.targetExists(r.Context(), t.TargetType, t.TargetID) {
			NotFound(w, r, fmt.Sprintf("%s with ID %d not found.", targetTypeName(t.TargetType), t.TargetID))
			return
		}
		switch t.TargetType {
		case 0:
			var ext string
			if err := tx.QueryRow(r.Context(), `SELECT "Extension" FROM "Images" WHERE "Id" = $1`, t.TargetID).Scan(&ext); err == nil {
				_ = s.s3.Delete(r.Context(), fmt.Sprintf("%d%s", t.TargetID, ext))
			}
			_, err = tx.Exec(r.Context(), `DELETE FROM "Images" WHERE "Id" = $1`, t.TargetID)
		case 1:
			_, err = tx.Exec(r.Context(), `DELETE FROM "Tags" WHERE "Id" = $1`, t.TargetID)
		case 2:
			_, err = tx.Exec(r.Context(), `DELETE FROM "Artists" WHERE "Id" = $1`, t.TargetID)
		}
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
	}

	query := `UPDATE "ReviewTasks" SET "Status" = 3, "ResolvedAt" = now() WHERE "Id" = $1`
	args := []any{id}
	if isModerator(ident) && t.SubmitterID != nil && *t.SubmitterID != ident.UserID {
		query = `UPDATE "ReviewTasks" SET "Status" = 3, "ResolvedAt" = now(), "ReviewerId" = $2 WHERE "Id" = $1`
		args = append(args, ident.UserID)
	}
	if _, err := tx.Exec(r.Context(), query, args...); err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
