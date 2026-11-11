package server

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
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

var whitespaceRegex = regexp.MustCompile(`[ \t\n\r\v\f]+`)

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

func contentVisibility(r *http.Request, ident *auth.Identity, alias string, a *sqlArgs) (string, bool) {
	rs := queryGet(r, "reviewStatus")
	includeMyPending := queryBool(r, "includeMyPending", false)
	if rs == "" || strings.EqualFold(rs, "Accepted") {
		if includeMyPending && ident != nil {
			return fmt.Sprintf(`(%s."ReviewStatus" = 1 OR (%s."ReviewStatus" = 0 AND %s."CreatorId" = %s))`, alias, alias, alias, a.add(ident.UserID)), true
		}
		return alias + `."ReviewStatus" = 1`, true
	}
	if !isModerator(ident) {
		return "", false
	}
	switch {
	case strings.EqualFold(rs, "Pending"):
		return alias + `."ReviewStatus" = 0`, true
	case strings.EqualFold(rs, "All"):
		return `TRUE`, true
	default:
		return "", false
	}
}

func lookupVisibility(r *http.Request, ident *auth.Identity, alias string, a *sqlArgs) (string, bool) {
	if isModerator(ident) {
		if queryGet(r, "reviewStatus") == "" {
			return "TRUE", true
		}
		return contentVisibility(r, ident, alias, a)
	}
	if ident != nil {
		return fmt.Sprintf(`(%s."ReviewStatus" = 1 OR (%s."ReviewStatus" = 0 AND %s."CreatorId" = %s))`, alias, alias, alias, a.add(ident.UserID)), true
	}
	return alias + `."ReviewStatus" = 1`, true
}

func validReviewStatusFilter(rs string) bool {
	return rs == "" || strings.EqualFold(rs, "Accepted") || strings.EqualFold(rs, "Pending") || strings.EqualFold(rs, "All")
}

const tagCountJoin = ` CROSS JOIN LATERAL (
		SELECT COUNT(*) AS cnt
		FROM "ImageTag" cit JOIN "Images" ci ON ci."Id" = cit."ImagesId" AND ci."ReviewStatus" = 1
		WHERE cit."TagsId" = t."Id"
	) tc`

const tagCountAggregateJoin = ` LEFT JOIN (
		SELECT cit."TagsId" AS tid, COUNT(*) AS cnt
		FROM "ImageTag" cit JOIN "Images" ci ON ci."Id" = cit."ImagesId" AND ci."ReviewStatus" = 1
		GROUP BY cit."TagsId"
	) tc ON tc.tid = t."Id"`

const tagUsageQuery = `
	SELECT cit."TagsId", COUNT(*)
	FROM "ImageTag" cit JOIN "Images" ci ON ci."Id" = cit."ImagesId" AND ci."ReviewStatus" = 1
	GROUP BY cit."TagsId"`

func scanTagRows(rows pgx.Rows, ident *auth.Identity) ([]tagDto, error) {
	defer rows.Close()
	items := []tagDto{}
	for rows.Next() {
		var t tagDto
		var rs int
		var desc *string
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &desc, &rs, &t.CreatorID, &t.ImageCount); err != nil {
			return nil, err
		}
		t.Description = orEmpty(desc)
		t.ReviewStatus = reviewStatusString(rs)
		if !isModerator(ident) && !(ident != nil && t.CreatorID != nil && *t.CreatorID == ident.UserID) {
			t.CreatorID = nil
		}
		items = append(items, t)
	}
	return items, rows.Err()
}

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
	rs := queryGet(r, "reviewStatus")
	if !validReviewStatusFilter(rs) {
		invalidEnumProblem(w, r, "reviewStatus", rs)
		return
	}
	includedIDs, err := s.idListParam(r, "includedIds")
	if err != nil {
		BadRequest(w, r, err.Error())
		return
	}
	includedSlugs, err := s.listParam(r, "includedSlugs", false)
	if err != nil {
		BadRequest(w, r, err.Error())
		return
	}

	if len(includedIDs) > 0 || len(includedSlugs) > 0 {
		if _, ok := lookupVisibility(r, ident, "t", &sqlArgs{}); !ok {
			Forbidden(w, r, "Filtering by non-accepted review status is only available to moderators and admins.")
			return
		}
		usage, err := s.loadUsage(ctx, "tags", tagUsageQuery)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		a := &sqlArgs{}
		join := tagCountJoin
		count := "tc.cnt"
		if usage != nil {
			join = ""
			count = "0::bigint"
		}
		vis, _ := lookupVisibility(r, ident, "t", a)
		var where string
		if len(includedIDs) > 0 {
			where = ` WHERE t."Id" = ANY(` + a.add(includedIDs) + `)`
		} else {
			where = ` WHERE t."Slug" = ANY(` + a.add(includedSlugs) + `)`
		}
		rows, err := s.db.Query(ctx, `
			SELECT t."Id", t."Name", t."Slug", t."Description", t."ReviewStatus", t."CreatorId", `+count+`
			FROM "Tags" t`+join+where+` AND `+vis+` ORDER BY t."Id"`, a.args...)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		items, err := scanTagRows(rows, ident)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		if usage != nil {
			for i := range items {
				items[i].ImageCount = usage.byID[items[i].ID]
			}
		}
		writePublicJSON(w, r, s.newPaginated(items, 1, len(items), int64(len(items)), s.cfg.PageSizes.TagMax, s.cfg.PageSizes.TagDefault))
		return
	}

	if _, ok := contentVisibility(r, ident, "t", &sqlArgs{}); !ok {
		Forbidden(w, r, "Filtering by non-accepted review status is only available to moderators and admins.")
		return
	}
	page, pageSize := s.paginationParams(r, s.cfg.PageSizes.TagDefault, s.cfg.PageSizes.TagMax)
	name := strings.TrimSpace(queryGet(r, "name"))
	filter := func(a *sqlArgs) string {
		vis, _ := contentVisibility(r, ident, "t", a)
		where := " WHERE " + vis
		if name != "" {
			where += ` AND lower(t."Name") LIKE ` + a.add(likePattern(strings.ToLower(name)))
		}
		return where
	}

	usage, err := s.loadUsage(ctx, "tags", tagUsageQuery)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if usage != nil {
		idArgs := &sqlArgs{}
		where := filter(idArgs)
		cacheKey := ""
		if name == "" && len(idArgs.args) == 0 {
			cacheKey = "tags" + where
		}
		pageIDs, total, err := s.rankedPage(ctx, cacheKey, `SELECT t."Id" FROM "Tags" t`+where, idArgs.args, usage, page, pageSize)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		items := []tagDto{}
		if len(pageIDs) > 0 {
			rows, err := s.db.Query(ctx, `
				SELECT t."Id", t."Name", t."Slug", t."Description", t."ReviewStatus", t."CreatorId", 0::bigint
				FROM "Tags" t WHERE t."Id" = ANY($1)`, pageIDs)
			if err != nil {
				InternalError(w, r, s.log, err)
				return
			}
			scanned, err := scanTagRows(rows, ident)
			if err != nil {
				InternalError(w, r, s.log, err)
				return
			}
			items = orderByIDs(pageIDs, scanned, func(t tagDto) int64 { return t.ID })
			for i := range items {
				items[i].ImageCount = usage.byID[items[i].ID]
			}
		}
		writePublicJSON(w, r, s.newPaginated(items, page, pageSize, total, s.cfg.PageSizes.TagMax, s.cfg.PageSizes.TagDefault))
		return
	}

	countArgs := &sqlArgs{}
	countWhere := filter(countArgs)
	listArgs := &sqlArgs{}
	join := tagCountAggregateJoin
	listWhere := filter(listArgs)

	var total int64
	var items []tagDto
	b := &pgx.Batch{}
	b.Queue(`SELECT COUNT(*) FROM "Tags" t`+countWhere, countArgs.args...).QueryRow(func(row pgx.Row) error {
		return row.Scan(&total)
	})
	b.Queue(`SELECT t."Id", t."Name", t."Slug", t."Description", t."ReviewStatus", t."CreatorId", COALESCE(tc.cnt, 0)
		FROM "Tags" t`+join+listWhere+
		` ORDER BY COALESCE(tc.cnt, 0) DESC, t."Id" ASC`+
		fmt.Sprintf(" LIMIT %d OFFSET %d", pageSize, offsetOf(page, pageSize)), listArgs.args...).Query(func(rows pgx.Rows) error {
		var err error
		items, err = scanTagRows(rows, ident)
		return err
	})
	if err := s.db.SendBatch(ctx, b).Close(); err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	writePublicJSON(w, r, s.newPaginated(items, page, pageSize, total, s.cfg.PageSizes.TagMax, s.cfg.PageSizes.TagDefault))
}

func (s *Server) writeTagByQuery(w http.ResponseWriter, r *http.Request, where string, arg any, notFoundMsg string) {
	ident := IdentityFromContext(r.Context())
	var t tagDto
	var rs int
	var desc *string
	var creatorID *int64
	err := s.db.QueryRow(r.Context(), `
		SELECT t."Id", t."Name", t."Slug", t."Description", t."ReviewStatus", t."CreatorId"
		FROM "Tags" t WHERE `+where, arg).
		Scan(&t.ID, &t.Name, &t.Slug, &desc, &rs, &creatorID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !canSeeContent(ident, rs, creatorID)) {
		NotFound(w, r, notFoundMsg)
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	t.Description = orEmpty(desc)
	t.ReviewStatus = reviewStatusString(rs)
	writePublicJSON(w, r, t)
}

func (s *Server) handleGetTag(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	s.writeTagByQuery(w, r, `t."Id" = $1`, id, fmt.Sprintf("Tag with ID %d not found.", id))
}

func (s *Server) handleGetTagBySlug(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if len(slug) > 100 {
		NotFound(w, r, "Tag not found.")
		return
	}
	s.writeTagByQuery(w, r, `t."Slug" = $1`, slug, fmt.Sprintf("Tag with slug '%s' not found.", slug))
}

type tagUpsertRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Slug         string `json:"slug"`
	ReviewStatus string `json:"reviewStatus"`
	CreatorID    *int64 `json:"creatorId"`
}

func validateTagFields(req *tagUpsertRequest, withSlug bool) map[string][]string {
	errs := map[string][]string{}
	if strings.TrimSpace(req.Name) == "" {
		errs["Name"] = append(errs["Name"], "Name is required.")
	} else if runeLen(strings.TrimSpace(req.Name)) > 30 {
		errs["Name"] = append(errs["Name"], "Name must not exceed 30 characters.")
	}
	if strings.TrimSpace(req.Description) == "" {
		errs["Description"] = append(errs["Description"], "Description is required.")
	} else if runeLen(strings.TrimSpace(req.Description)) > 150 {
		errs["Description"] = append(errs["Description"], "Description must not exceed 150 characters.")
	}
	if withSlug && req.Slug != "" {
		if len(req.Slug) > 50 {
			errs["Slug"] = append(errs["Slug"], "Slug must not exceed 50 characters.")
		} else if !slugRegex.MatchString(req.Slug) {
			errs["Slug"] = append(errs["Slug"], "Slug can only contain lowercase letters, numbers, and hyphens.")
		}
	}
	return errs
}

func (s *Server) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
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
	if errs := validateTagFields(&req, true); len(errs) > 0 {
		WriteValidationProblem(w, r, errs)
		return
	}

	trimmedName := strings.TrimSpace(req.Name)
	description := strings.TrimSpace(req.Description)
	slug := generateSlug(trimmedName)
	if strings.TrimSpace(req.Slug) != "" {
		slug = generateSlug(strings.TrimSpace(req.Slug))
	}
	if slug == "" {
		WriteValidationProblem(w, r, map[string][]string{
			"Slug": {"A slug could not be generated from the name. Please provide a slug (lowercase letters, numbers and hyphens)."},
		})
		return
	}
	if len(slug) > 50 {
		slug = strings.Trim(slug[:50], "-")
	}

	autoAccept := !s.cfg.Review.RequireTagReview || auth.ReviewBypassed(ident.Role, s.cfg.Review.TagReviewBypassMinRole)
	reviewStatus := 0
	if autoAccept {
		reviewStatus = 1
	}

	var t tagDto
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if !autoAccept {
			if err := s.checkPendingSubmissions(ctx, tx, ident.UserID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('tags'))`); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "Tags" WHERE "Slug" = $1)`, slug).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return errConflict(fmt.Sprintf("Tag with slug '%s' already exists.", slug))
		}
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "Tags" WHERE lower("Name") = lower($1))`, trimmedName).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return errConflict(fmt.Sprintf("Tag with name '%s' already exists.", trimmedName))
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO "Tags" ("Name","Slug","Description","ReviewStatus","CreatorId")
			VALUES ($1,$2,$3,$4,$5) RETURNING "Id"`,
			trimmedName, slug, description, reviewStatus, ident.UserID).Scan(&t.ID); err != nil {
			if isUniqueViolation(err) {
				return errConflict(fmt.Sprintf("Tag with slug '%s' already exists.", slug))
			}
			return err
		}
		if autoAccept {
			_, err := tx.Exec(ctx, `
				INSERT INTO "ReviewTasks" ("Kind","TargetType","TargetId","SubmitterId","Status","ReviewerNote","CreatedAt","ResolvedAt")
				VALUES (0,1,$1,$2,1,'Auto-accepted (trusted role).',now(),now())`, t.ID, ident.UserID)
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO "ReviewTasks" ("Kind","TargetType","TargetId","SubmitterId","Status","CreatedAt")
			VALUES (0,1,$1,$2,0,now())`, t.ID, ident.UserID)
		return err
	})
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}

	t.Name = trimmedName
	t.Slug = slug
	t.Description = description
	t.ReviewStatus = reviewStatusString(reviewStatus)
	t.CreatorID = &ident.UserID
	t.ImageCount = 0
	w.Header().Set("Location", fmt.Sprintf("/tags?id=%d", t.ID))
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handlePatchTag(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}

	var req tagUpsertRequest
	if err := decodeJSON(r, &req); err != nil {
		WriteValidationProblem(w, r, map[string][]string{
			"": {"The request could not be processed."},
		})
		return
	}
	if errs := validateTagFields(&req, false); len(errs) > 0 {
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
	description := strings.TrimSpace(req.Description)
	newSlug := ""
	if strings.TrimSpace(req.Slug) != "" {
		newSlug = generateSlug(strings.TrimSpace(req.Slug))
		if newSlug == "" || len(newSlug) > 50 {
			WriteValidationProblem(w, r, map[string][]string{
				"Slug": {"Slug can only contain lowercase letters, numbers, and hyphens."},
			})
			return
		}
	}

	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := lockPendingTasks(ctx, tx, 1, id); err != nil {
			return err
		}
		var currentSlug string
		if err := tx.QueryRow(ctx, `SELECT "Slug" FROM "Tags" WHERE "Id" = $1 FOR UPDATE`, id).Scan(&currentSlug); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return errNotFound(fmt.Sprintf("Tag with ID %d not found.", id))
			}
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "Tags" WHERE lower("Name") = lower($1) AND "Id" <> $2)`, trimmedName, id).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return errConflict(fmt.Sprintf("Tag with name '%s' already exists.", trimmedName))
		}
		if newSlug != "" && newSlug != currentSlug {
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "Tags" WHERE "Slug" = $1 AND "Id" <> $2)`, newSlug, id).Scan(&exists); err != nil {
				return err
			}
			if exists {
				return errConflict(fmt.Sprintf("Tag with slug '%s' already exists.", newSlug))
			}
			if _, err := tx.Exec(ctx, `UPDATE "Tags" SET "Slug" = $1 WHERE "Id" = $2`, newSlug, id); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE "Tags" SET "Name" = $1, "Description" = $2 WHERE "Id" = $3`,
			trimmedName, description, id); err != nil {
			return err
		}
		if reviewStatus != "" {
			rs := 0
			if reviewStatus == "Accepted" {
				rs = 1
			}
			if _, err := tx.Exec(ctx, `UPDATE "Tags" SET "ReviewStatus" = $1 WHERE "Id" = $2`, rs, id); err != nil {
				return err
			}
			if rs == 1 {
				if _, err := tx.Exec(ctx, `
					UPDATE "ReviewTasks" SET "Status" = 1, "ReviewerId" = $2, "ResolvedAt" = now()
					WHERE "TargetType" = 1 AND "TargetId" = $1 AND "Kind" = 0 AND "Status" = 0`, id, ident.UserID); err != nil {
					return err
				}
			}
		}
		if req.CreatorID != nil {
			if _, err := tx.Exec(ctx, `UPDATE "Tags" SET "CreatorId" = $1 WHERE "Id" = $2`, *req.CreatorID, id); err != nil {
				if isForeignKeyViolation(err) {
					return errNotFound(fmt.Sprintf("User with ID %d not found.", *req.CreatorID))
				}
				return err
			}
		}
		return nil
	})
	if err != nil {
		if isUniqueViolation(err) {
			Conflict(w, r, "A tag with this name or slug already exists.")
			return
		}
		s.writeDomainError(w, r, err)
		return
	}

	var t tagDto
	var rs int
	var desc *string
	err = s.db.QueryRow(ctx, `
		SELECT t."Id", t."Name", t."Slug", t."Description", t."ReviewStatus", t."CreatorId"
		FROM "Tags" t WHERE t."Id" = $1`, id).
		Scan(&t.ID, &t.Name, &t.Slug, &desc, &rs, &t.CreatorID)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	t.Description = orEmpty(desc)
	t.ReviewStatus = reviewStatusString(rs)
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	res, err := s.db.Exec(r.Context(), `DELETE FROM "Tags" WHERE "Id" = $1`, id)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if res.RowsAffected() == 0 {
		NotFound(w, r, fmt.Sprintf("Tag with ID %d not found.", id))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
