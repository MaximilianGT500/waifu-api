package server

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/waifu-im/waifu-api/backend/internal/auth"
	"golang.org/x/text/unicode/norm"
)

func (s *Server) RegisterTagRoutes() {
	s.Handle("GET /tags", http.HandlerFunc(s.handleListTags))
	s.Handle("GET /tags/by-slug/{slug}", http.HandlerFunc(s.handleGetTagBySlug))
	s.Handle("GET /tags/{id}", http.HandlerFunc(s.handleGetTag))
	s.Handle("POST /tags", s.RequireAuth(http.HandlerFunc(s.handleCreateTag)))
	s.Handle("PATCH /tags/{id}", s.RequireRole("Moderator")(http.HandlerFunc(s.handlePatchTag)))
	s.Handle("DELETE /tags/{id}", s.RequireRole("Admin")(http.HandlerFunc(s.handleDeleteTag)))
}

var slugRegex = regexp.MustCompile(`^[a-z0-9-]+$`)

func generateSlug(name string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(name) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	clean := strings.ToLower(norm.NFC.String(b.String()))

	var out strings.Builder
	for _, r := range clean {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			out.WriteRune(r)
		case r == '-':
			out.WriteRune(r)
		case r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f':

			out.WriteRune(' ')
		}
	}
	s := out.String()
	s = whitespaceRegex.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

var whitespaceRegex = regexp.MustCompile(`[ \t\n\r\v\f]+`)

func (s *Server) tagVisibility(r *http.Request, ident *auth.Identity) (string, bool) {
	rs := r.URL.Query().Get("reviewStatus")
	includeMyPending := queryBool(r, "includeMyPending", false)
	if rs == "" || strings.EqualFold(rs, "Accepted") {
		if includeMyPending && ident != nil {
			return fmt.Sprintf(`(t."ReviewStatus" = 1 OR (t."ReviewStatus" = 0 AND t."CreatorId" = %d))`, ident.UserID), true
		}
		return `t."ReviewStatus" = 1`, true
	}
	if !isModerator(ident) {
		return "", false
	}
	switch {
	case strings.EqualFold(rs, "Pending"):
		return `t."ReviewStatus" = 0`, true
	case strings.EqualFold(rs, "All"):
		return `TRUE`, true
	default:
		return "", false
	}
}

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	rs := r.URL.Query().Get("reviewStatus")
	if rs != "" {
		valid := strings.EqualFold(rs, "Accepted") || strings.EqualFold(rs, "Pending") || strings.EqualFold(rs, "All")
		if !valid {
			invalidEnumProblem(w, r, "reviewStatus", rs)
			return
		}
	}
	vis, ok := s.tagVisibility(r, ident)
	if !ok {
		Forbidden(w, r, "Filtering by non-accepted review status is only available to moderators and admins.")
		return
	}
	page, pageSize := paginationParams(r, s.cfg.PageSizes.TagDefault, s.cfg.PageSizes.TagMax)

	name := r.URL.Query().Get("name")
	includedIDs, _ := parseLongList(repeatedQuery(r, "includedIds"))
	includedSlugs := repeatedQuery(r, "includedSlugs")

	countExpr := `(SELECT COUNT(*) FROM "ImageTag" it JOIN "Images" i ON i."Id" = it."ImagesId" AND i."ReviewStatus" = 1 WHERE it."TagsId" = t."Id")`

	if len(includedIDs) > 0 || len(includedSlugs) > 0 {
		where := ""
		args := []any{}
		if len(includedIDs) > 0 {
			where = ` WHERE t."Id" = ANY($1)`
			args = append(args, includedIDs)
		} else {
			where = ` WHERE t."Slug" = ANY($1)`
			args = append(args, includedSlugs)
		}
		query := fmt.Sprintf(`
			SELECT t."Id", t."Name", t."Slug", t."Description", t."ReviewStatus", t."CreatorId", %s
			FROM "Tags" t%s`, countExpr, where)
		rows, err := s.db.Query(r.Context(), query, args...)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		defer rows.Close()
		items := []tagDto{}
		for rows.Next() {
			var t tagDto
			var rs int
			if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.Description, &rs, &t.CreatorID, &t.ImageCount); err != nil {
				InternalError(w, r, s.log, err)
				return
			}
			t.ReviewStatus = reviewStatusString(rs)
			if !isModerator(ident) {
				t.CreatorID = nil
			}
			items = append(items, t)
		}
		writeJSON(w, http.StatusOK, newPaginated(items, 1, len(items), int64(len(items)), s.cfg.PageSizes.TagMax, s.cfg.PageSizes.TagDefault))
		return
	}

	where := " WHERE " + vis
	args := []any{}
	ph := func() string { return fmt.Sprintf("$%d", len(args)+1) }
	if name != "" {
		where += fmt.Sprintf(` AND lower(t."Name") LIKE %s`, ph())
		args = append(args, "%"+strings.ToLower(name)+"%")
	}

	var total int64
	if err := s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM "Tags" t`+where, args...).Scan(&total); err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	query := fmt.Sprintf(`
		SELECT t."Id", t."Name", t."Slug", t."Description", t."ReviewStatus", t."CreatorId", %s
		FROM "Tags" t%s
		ORDER BY %s DESC`, countExpr, where, countExpr)
	query += fmt.Sprintf(" LIMIT %d OFFSET %d", pageSize, (page-1)*pageSize)

	rows, err := s.db.Query(r.Context(), query, args...)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	defer rows.Close()

	items := []tagDto{}
	for rows.Next() {
		var t tagDto
		var rs int
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.Description, &rs, &t.CreatorID, &t.ImageCount); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		t.ReviewStatus = reviewStatusString(rs)
		if !isModerator(ident) {
			t.CreatorID = nil
		}
		items = append(items, t)
	}

	writeJSON(w, http.StatusOK, newPaginated(items, page, pageSize, total, s.cfg.PageSizes.TagMax, s.cfg.PageSizes.TagDefault))
}

func (s *Server) handleGetTag(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	var t tagDto
	var rs int
	err = s.db.QueryRow(r.Context(), `
		SELECT t."Id", t."Name", t."Slug", t."Description", t."ReviewStatus"
		FROM "Tags" t WHERE t."Id" = $1`, id).
		Scan(&t.ID, &t.Name, &t.Slug, &t.Description, &rs)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, fmt.Sprintf("Tag with ID %d not found.", id))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	t.ReviewStatus = reviewStatusString(rs)

	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleGetTagBySlug(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	var t tagDto
	var rs int
	err := s.db.QueryRow(r.Context(), `
		SELECT t."Id", t."Name", t."Slug", t."Description", t."ReviewStatus"
		FROM "Tags" t WHERE t."Slug" = $1`, slug).
		Scan(&t.ID, &t.Name, &t.Slug, &t.Description, &rs)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, fmt.Sprintf("Tag with slug '%s' not found.", slug))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	t.ReviewStatus = reviewStatusString(rs)
	writeJSON(w, http.StatusOK, t)
}

type tagUpsertRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Slug         string `json:"slug"`
	ReviewStatus string `json:"reviewStatus"`
	CreatorID    *int64 `json:"creatorId"`
}

func validateTagCreate(req *tagUpsertRequest) map[string][]string {
	errs := map[string][]string{}
	if strings.TrimSpace(req.Name) == "" {
		errs["Name"] = append(errs["Name"], "Name is required.")
	} else if len(req.Name) > 30 {
		errs["Name"] = append(errs["Name"], "Name must not exceed 30 characters.")
	}
	if strings.TrimSpace(req.Description) == "" {
		errs["Description"] = append(errs["Description"], "Description is required.")
	} else if len(req.Description) > 150 {
		errs["Description"] = append(errs["Description"], "Description must not exceed 150 characters.")
	}
	if req.Slug != "" {
		if len(req.Slug) > 50 {
			errs["Slug"] = append(errs["Slug"], "Slug must not exceed 50 characters.")
		} else if !slugRegex.MatchString(req.Slug) {
			errs["Slug"] = append(errs["Slug"], "Slug can only contain lowercase letters, numbers, and hyphens.")
		}
	}
	return errs
}

func validateTagUpdate(req *tagUpsertRequest) map[string][]string {
	errs := map[string][]string{}
	if strings.TrimSpace(req.Name) == "" {
		errs["Name"] = append(errs["Name"], "Name is required.")
	} else if len(req.Name) > 30 {
		errs["Name"] = append(errs["Name"], "Name must not exceed 30 characters.")
	}
	if strings.TrimSpace(req.Description) == "" {
		errs["Description"] = append(errs["Description"], "Description is required.")
	} else if len(req.Description) > 150 {
		errs["Description"] = append(errs["Description"], "Description must not exceed 150 characters.")
	}
	return errs
}

func (s *Server) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	if !requireMinRole(ident, s.cfg.Permissions.TagCreationMinRole) {
		s.permissionRoleForbidden(w, r, s.cfg.Permissions.TagCreationMinRole, auth.RoleUser)
		return
	}
	var req tagUpsertRequest
	if err := decodeJSON(r, &req); err != nil {
		WriteValidationProblem(w, r, map[string][]string{
			"": {"The request could not be processed."},
		})
		return
	}
	if errs := validateTagCreate(&req); len(errs) > 0 {
		WriteValidationProblem(w, r, errs)
		return
	}

	trimmedName := strings.TrimSpace(req.Name)
	slug := generateSlug(trimmedName)
	if strings.TrimSpace(req.Slug) != "" {
		slug = generateSlug(strings.TrimSpace(req.Slug))
	}

	var conflict string
	_ = s.db.QueryRow(r.Context(), `SELECT "Name" FROM "Tags" WHERE "Slug" = $1`, slug).Scan(&conflict)
	if conflict != "" {
		Conflict(w, r, fmt.Sprintf("Tag with slug '%s' already exists.", slug))
		return
	}
	_ = s.db.QueryRow(r.Context(), `SELECT "Slug" FROM "Tags" WHERE lower("Name") = lower($1)`, trimmedName).Scan(&conflict)
	if conflict != "" {
		Conflict(w, r, fmt.Sprintf("Tag with name '%s' already exists.", trimmedName))
		return
	}

	reviewStatus := 0
	if !s.cfg.Review.RequireTagReview || auth.ReviewBypassed(ident.Role, s.cfg.Review.TagReviewBypassMinRole) {
		reviewStatus = 1
	}

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	defer tx.Rollback(r.Context())

	var t tagDto
	err = tx.QueryRow(r.Context(), `
		INSERT INTO "Tags" ("Name","Slug","Description","ReviewStatus","CreatorId")
		VALUES ($1,$2,$3,$4,$5) RETURNING "Id"`,
		trimmedName, slug, strings.TrimSpace(req.Description), reviewStatus, ident.UserID).Scan(&t.ID)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	now := time.Now().UTC()
	if reviewStatus == 1 {
		_, err = tx.Exec(r.Context(), `
			INSERT INTO "ReviewTasks" ("Kind","TargetType","TargetId","SubmitterId","Status","ReviewerNote","CreatedAt","ResolvedAt")
			VALUES (0,1,$1,$2,1,'Auto-accepted (trusted role).',$3,$3)`, t.ID, ident.UserID, now)
	} else {
		_, err = tx.Exec(r.Context(), `
			INSERT INTO "ReviewTasks" ("Kind","TargetType","TargetId","SubmitterId","Status","CreatedAt")
			VALUES (0,1,$1,$2,0,$3)`, t.ID, ident.UserID, now)
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	t.Name = trimmedName
	t.Slug = slug
	t.Description = strings.TrimSpace(req.Description)
	t.ReviewStatus = reviewStatusString(reviewStatus)
	t.CreatorID = &ident.UserID
	t.ImageCount = 0
	w.Header().Set("Location", fmt.Sprintf("/tags?id=%d", t.ID))
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handlePatchTag(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	var exists bool
	_ = s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM "Tags" WHERE "Id" = $1)`, id).Scan(&exists)
	if !exists {
		NotFound(w, r, fmt.Sprintf("Tag with ID %d not found.", id))
		return
	}

	var req tagUpsertRequest
	if err := decodeJSON(r, &req); err != nil {
		WriteValidationProblem(w, r, map[string][]string{
			"": {"The request could not be processed."},
		})
		return
	}
	if errs := validateTagUpdate(&req); len(errs) > 0 {
		WriteValidationProblem(w, r, errs)
		return
	}

	reviewStatus := ""
	if req.ReviewStatus != "" {
		var ok bool
		if reviewStatus, ok = enumValue(req.ReviewStatus, "Pending", "Accepted"); !ok {
			invalidEnumProblem(w, r, "reviewStatus", req.ReviewStatus)
			return
		}
	}

	trimmedName := strings.TrimSpace(req.Name)

	var conflictID int64
	_ = s.db.QueryRow(r.Context(), `SELECT "Id" FROM "Tags" WHERE lower("Name") = lower($1) AND "Id" <> $2`, trimmedName, id).Scan(&conflictID)
	if conflictID != 0 {
		Conflict(w, r, fmt.Sprintf("Tag with name '%s' already exists.", trimmedName))
		return
	}
	if req.Slug != "" {
		newSlug := generateSlug(strings.TrimSpace(req.Slug))
		var existingID int64
		_ = s.db.QueryRow(r.Context(), `SELECT "Id" FROM "Tags" WHERE "Slug" = $1`, newSlug).Scan(&existingID)
		if existingID != 0 {
			Conflict(w, r, fmt.Sprintf("Tag with slug '%s' already exists.", newSlug))
			return
		}
		_, _ = s.db.Exec(r.Context(), `UPDATE "Tags" SET "Slug" = $1 WHERE "Id" = $2`, newSlug, id)
	}

	if _, err := s.db.Exec(r.Context(), `UPDATE "Tags" SET "Name" = $1, "Description" = $2 WHERE "Id" = $3`,
		trimmedName, strings.TrimSpace(req.Description), id); err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	if reviewStatus != "" {
		rs := 0
		if reviewStatus == "Accepted" {
			rs = 1
		}
		if _, err := s.db.Exec(r.Context(), `UPDATE "Tags" SET "ReviewStatus" = $1 WHERE "Id" = $2`, rs, id); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		if reviewStatus == "Accepted" {

			_, _ = s.db.Exec(r.Context(), `
				UPDATE "ReviewTasks" SET "Status" = 1, "ResolvedAt" = now()
				WHERE "TargetType" = 1 AND "TargetId" = $1 AND "Kind" = 0 AND "Status" = 0`, id)
		}
	}
	if req.CreatorID != nil {
		if _, err := s.db.Exec(r.Context(), `UPDATE "Tags" SET "CreatorId" = $1 WHERE "Id" = $2`, *req.CreatorID, id); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
	}

	var t tagDto
	var rs int
	err = s.db.QueryRow(r.Context(), `
		SELECT t."Id", t."Name", t."Slug", t."Description", t."ReviewStatus", t."CreatorId"
		FROM "Tags" t WHERE t."Id" = $1`, id).
		Scan(&t.ID, &t.Name, &t.Slug, &t.Description, &rs, &t.CreatorID)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	t.ReviewStatus = reviewStatusString(rs)
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	var exists bool
	if err := s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM "Tags" WHERE "Id" = $1)`, id).Scan(&exists); err != nil || !exists {
		NotFound(w, r, fmt.Sprintf("Tag with ID %d not found.", id))
		return
	}
	_, err = s.db.Exec(r.Context(), `DELETE FROM "Tags" WHERE "Id" = $1`, id)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
