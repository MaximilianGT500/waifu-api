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

func (s *Server) RegisterArtistRoutes() {
	s.Handle("GET /artists", http.HandlerFunc(s.handleListArtists))
	s.Handle("GET /artists/by-name/{name}", http.HandlerFunc(s.handleGetArtistByName))
	s.Handle("GET /artists/{id}", http.HandlerFunc(s.handleGetArtist))
	s.Handle("POST /artists", s.RequireAuth(http.HandlerFunc(s.handleCreateArtist)))
	s.Handle("PATCH /artists/{id}", s.RequireRole("Moderator")(http.HandlerFunc(s.handlePatchArtist)))
	s.Handle("DELETE /artists/{id}", s.RequireRole("Admin")(http.HandlerFunc(s.handleDeleteArtist)))
}

func (s *Server) artistVisibility(r *http.Request, ident *auth.Identity) (string, bool) {
	rs := r.URL.Query().Get("reviewStatus")
	includeMyPending := queryBool(r, "includeMyPending", false)
	if rs == "" || strings.EqualFold(rs, "Accepted") {
		if includeMyPending && ident != nil {
			return fmt.Sprintf(`(a."ReviewStatus" = 1 OR (a."ReviewStatus" = 0 AND a."CreatorId" = %d))`, ident.UserID), true
		}
		return `a."ReviewStatus" = 1`, true
	}
	if !isModerator(ident) {
		return "", false
	}
	switch {
	case strings.EqualFold(rs, "Pending"):
		return `a."ReviewStatus" = 0`, true
	case strings.EqualFold(rs, "All"):
		return `TRUE`, true
	default:
		return "", false
	}
}

func (s *Server) handleListArtists(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	rs := r.URL.Query().Get("reviewStatus")
	if rs != "" {
		valid := strings.EqualFold(rs, "Accepted") || strings.EqualFold(rs, "Pending") || strings.EqualFold(rs, "All")
		if !valid {
			invalidEnumProblem(w, r, "reviewStatus", rs)
			return
		}
	}
	vis, ok := s.artistVisibility(r, ident)
	if !ok {
		Forbidden(w, r, "Filtering by non-accepted review status is only available to moderators and admins.")
		return
	}
	page, pageSize := paginationParams(r, s.cfg.PageSizes.ArtistDefault, s.cfg.PageSizes.ArtistMax)
	name := r.URL.Query().Get("name")
	includedIDs, _ := parseLongList(repeatedQuery(r, "includedIds"))

	countExpr := `(SELECT COUNT(*) FROM "ArtistImage" ai JOIN "Images" i ON i."Id" = ai."ImagesId" AND i."ReviewStatus" = 1 WHERE ai."ArtistsId" = a."Id")`

	if len(includedIDs) > 0 {
		query := fmt.Sprintf(`
			SELECT a."Id", a."Name", a."Patreon", a."Pixiv", a."Twitter", a."DeviantArt", a."ReviewStatus", a."CreatorId", %s
			FROM "Artists" a WHERE a."Id" = ANY($1)`, countExpr)
		rows, err := s.db.Query(r.Context(), query, includedIDs)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		defer rows.Close()
		items := []artistDto{}
		for rows.Next() {
			var a artistDto
			var rs int
			var patreon, pixiv, twitter, deviantArt *string
			if err := rows.Scan(&a.ID, &a.Name, &patreon, &pixiv, &twitter, &deviantArt, &rs, &a.CreatorID, &a.ImageCount); err != nil {
				InternalError(w, r, s.log, err)
				return
			}
			a.ReviewStatus = reviewStatusString(rs)
			a.Patreon = orEmpty(patreon)
			a.Pixiv = orEmpty(pixiv)
			a.Twitter = orEmpty(twitter)
			a.DeviantArt = orEmpty(deviantArt)
			if !isModerator(ident) {
				a.CreatorID = nil
			}
			items = append(items, a)
		}
		writeJSON(w, http.StatusOK, newPaginated(items, 1, len(items), int64(len(items)), s.cfg.PageSizes.ArtistMax, s.cfg.PageSizes.ArtistDefault))
		return
	}

	where := " WHERE " + vis
	args := []any{}
	ph := func() string { return fmt.Sprintf("$%d", len(args)+1) }
	if name != "" {
		where += fmt.Sprintf(` AND lower(a."Name") LIKE %s`, ph())
		args = append(args, "%"+strings.ToLower(name)+"%")
	}

	var total int64
	if err := s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM "Artists" a`+where, args...).Scan(&total); err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	query := fmt.Sprintf(`
		SELECT a."Id", a."Name", a."Patreon", a."Pixiv", a."Twitter", a."DeviantArt", a."ReviewStatus", a."CreatorId", %s
		FROM "Artists" a%s
		ORDER BY %s DESC`, countExpr, where, countExpr)
	query += fmt.Sprintf(" LIMIT %d OFFSET %d", pageSize, (page-1)*pageSize)

	rows, err := s.db.Query(r.Context(), query, args...)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	defer rows.Close()

	items := []artistDto{}
	for rows.Next() {
		var a artistDto
		var rs int
		var patreon, pixiv, twitter, deviantArt *string
		if err := rows.Scan(&a.ID, &a.Name, &patreon, &pixiv, &twitter, &deviantArt, &rs, &a.CreatorID, &a.ImageCount); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		a.ReviewStatus = reviewStatusString(rs)
		a.Patreon = orEmpty(patreon)
		a.Pixiv = orEmpty(pixiv)
		a.Twitter = orEmpty(twitter)
		a.DeviantArt = orEmpty(deviantArt)
		if !isModerator(ident) {
			a.CreatorID = nil
		}
		items = append(items, a)
	}

	writeJSON(w, http.StatusOK, newPaginated(items, page, pageSize, total, s.cfg.PageSizes.ArtistMax, s.cfg.PageSizes.ArtistDefault))
}

func (s *Server) handleGetArtist(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	var a artistDto
	var rs int
	var patreon, pixiv, twitter, deviantArt *string
	err = s.db.QueryRow(r.Context(), `
		SELECT a."Id", a."Name", a."Patreon", a."Pixiv", a."Twitter", a."DeviantArt", a."ReviewStatus"
		FROM "Artists" a WHERE a."Id" = $1`, id).
		Scan(&a.ID, &a.Name, &patreon, &pixiv, &twitter, &deviantArt, &rs)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, fmt.Sprintf("Artist with ID %d not found.", id))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	a.ReviewStatus = reviewStatusString(rs)
	a.Patreon = orEmpty(patreon)
	a.Pixiv = orEmpty(pixiv)
	a.Twitter = orEmpty(twitter)
	a.DeviantArt = orEmpty(deviantArt)

	writeJSON(w, http.StatusOK, a)
}

func (s *Server) handleGetArtistByName(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var a artistEntity
	var rs int
	err := s.db.QueryRow(r.Context(), `
		SELECT "Id","Name","Patreon","Pixiv","Twitter","DeviantArt","ReviewStatus","CreatorId"
		FROM "Artists" WHERE "Name" = $1`, name).
		Scan(&a.ID, &a.Name, &a.Patreon, &a.Pixiv, &a.Twitter, &a.DeviantArt, &rs, &a.CreatorID)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, r, fmt.Sprintf("Artist with name '%s' not found.", name))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	a.ReviewStatus = reviewStatusString(rs)
	a.Images = []imageDto{}
	writeJSON(w, http.StatusOK, a)
}

type artistUpsertRequest struct {
	Name         string  `json:"name"`
	Patreon      *string `json:"patreon"`
	Pixiv        *string `json:"pixiv"`
	Twitter      *string `json:"twitter"`
	DeviantArt   *string `json:"deviantArt"`
	ReviewStatus string  `json:"reviewStatus"`
	CreatorID    *int64  `json:"creatorId"`
}

func validateArtistRequest(req *artistUpsertRequest) map[string][]string {
	errs := map[string][]string{}
	if strings.TrimSpace(req.Name) == "" {
		errs["Name"] = append(errs["Name"], "Name is required.")
	} else if len(req.Name) > 30 {
		errs["Name"] = append(errs["Name"], "Name must not exceed 30 characters.")
	}
	for field, v := range map[string]*string{
		"Patreon": req.Patreon, "Pixiv": req.Pixiv, "Twitter": req.Twitter, "DeviantArt": req.DeviantArt,
	} {
		if v == nil || *v == "" {
			continue
		}
		if len(*v) > 200 {
			errs[field] = append(errs[field], field+" link must not exceed 200 characters.")
		}
		if !isAbsoluteURL(*v) {
			errs[field] = append(errs[field], field+" link must be a valid URL.")
		}
	}
	return errs
}

func normalizeArtistLinks(req *artistUpsertRequest) (patreon, pixiv, twitter, deviantArt *string) {
	norm := func(v *string) *string {
		if v == nil {
			return nil
		}
		s := strings.TrimSpace(*v)
		if s == "" {
			return nil
		}
		return &s
	}
	return norm(req.Patreon), norm(req.Pixiv), norm(req.Twitter), norm(req.DeviantArt)
}

func (s *Server) handleCreateArtist(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	if !requireMinRole(ident, s.cfg.Permissions.ArtistCreationMinRole) {
		s.permissionRoleForbidden(w, r, s.cfg.Permissions.ArtistCreationMinRole, auth.RoleUser)
		return
	}
	var req artistUpsertRequest
	if err := decodeJSON(r, &req); err != nil {
		WriteValidationProblem(w, r, map[string][]string{
			"": {"The request could not be processed."},
		})
		return
	}
	if errs := validateArtistRequest(&req); len(errs) > 0 {
		WriteValidationProblem(w, r, errs)
		return
	}

	trimmedName := strings.TrimSpace(req.Name)
	patreon, pixiv, twitter, deviantArt := normalizeArtistLinks(&req)

	conflicts := []struct {
		field  string
		value  *string
		column string
	}{
		{"Name", strPtr(trimmedName), `lower("Name") = lower($1)`},
		{"Patreon", patreon, `"Patreon" = $1`},
		{"Pixiv", pixiv, `"Pixiv" = $1`},
		{"Twitter", twitter, `"Twitter" = $1`},
		{"DeviantArt", deviantArt, `"DeviantArt" = $1`},
	}
	for _, c := range conflicts {
		if c.value == nil || *c.value == "" {
			continue
		}
		var existing string
		query := fmt.Sprintf(`SELECT "Name" FROM "Artists" WHERE %s LIMIT 1`, c.column)
		if err := s.db.QueryRow(r.Context(), query, *c.value).Scan(&existing); err == nil && existing != "" {
			if c.field == "Name" {
				Conflict(w, r, fmt.Sprintf("Artist with name '%s' already exists.", trimmedName))
			} else {
				Conflict(w, r, fmt.Sprintf("Artist with %s '%s' already exists.", c.field, *c.value))
			}
			return
		}
	}

	reviewStatus := 0
	if !s.cfg.Review.RequireArtistReview || auth.ReviewBypassed(ident.Role, s.cfg.Review.ArtistReviewBypassMinRole) {
		reviewStatus = 1
	}

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	defer tx.Rollback(r.Context())

	var a artistEntity
	err = tx.QueryRow(r.Context(), `
		INSERT INTO "Artists" ("Name","Patreon","Pixiv","Twitter","DeviantArt","ReviewStatus","CreatorId")
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING "Id"`,
		trimmedName, patreon, pixiv, twitter, deviantArt, reviewStatus, ident.UserID).Scan(&a.ID)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	now := time.Now().UTC()
	if reviewStatus == 1 {
		_, err = tx.Exec(r.Context(), `
			INSERT INTO "ReviewTasks" ("Kind","TargetType","TargetId","SubmitterId","Status","ReviewerNote","CreatedAt","ResolvedAt")
			VALUES (0,2,$1,$2,1,'Auto-accepted (trusted role).',$3,$3)`, a.ID, ident.UserID, now)
	} else {
		_, err = tx.Exec(r.Context(), `
			INSERT INTO "ReviewTasks" ("Kind","TargetType","TargetId","SubmitterId","Status","CreatedAt")
			VALUES (0,2,$1,$2,0,$3)`, a.ID, ident.UserID, now)
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	a.Name = trimmedName
	a.Patreon = patreon
	a.Pixiv = pixiv
	a.Twitter = twitter
	a.DeviantArt = deviantArt
	a.ReviewStatus = reviewStatusString(reviewStatus)
	a.CreatorID = &ident.UserID
	a.Images = []imageDto{}
	w.Header().Set("Location", fmt.Sprintf("/artists?id=%d", a.ID))
	writeJSON(w, http.StatusCreated, a)
}

func strPtr(s string) *string { return &s }

func (s *Server) handlePatchArtist(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	var exists bool
	_ = s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM "Artists" WHERE "Id" = $1)`, id).Scan(&exists)
	if !exists {
		NotFound(w, r, fmt.Sprintf("Artist with ID %d not found.", id))
		return
	}

	var req artistUpsertRequest
	if err := decodeJSON(r, &req); err != nil {
		WriteValidationProblem(w, r, map[string][]string{
			"": {"The request could not be processed."},
		})
		return
	}
	if errs := validateArtistRequest(&req); len(errs) > 0 {
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
	patreon, pixiv, twitter, deviantArt := normalizeArtistLinks(&req)

	conflicts := []struct {
		field  string
		value  *string
		column string
	}{
		{"Name", strPtr(trimmedName), `lower("Name") = lower($1)`},
		{"Patreon", patreon, `"Patreon" = $1`},
		{"Pixiv", pixiv, `"Pixiv" = $1`},
		{"Twitter", twitter, `"Twitter" = $1`},
		{"DeviantArt", deviantArt, `"DeviantArt" = $1`},
	}
	for _, c := range conflicts {
		if c.value == nil || *c.value == "" {
			continue
		}
		var conflictID int64
		query := fmt.Sprintf(`SELECT "Id" FROM "Artists" WHERE %s AND "Id" <> $2 LIMIT 1`, c.column)
		if err := s.db.QueryRow(r.Context(), query, *c.value, id).Scan(&conflictID); err == nil && conflictID != 0 {
			if c.field == "Name" {
				Conflict(w, r, fmt.Sprintf("Artist with name '%s' already exists.", trimmedName))
			} else {
				Conflict(w, r, fmt.Sprintf("%s link is already used by another artist.", c.field))
			}
			return
		}
	}

	if _, err := s.db.Exec(r.Context(), `
		UPDATE "Artists" SET "Name" = $1, "Patreon" = $2, "Pixiv" = $3, "Twitter" = $4, "DeviantArt" = $5
		WHERE "Id" = $6`,
		trimmedName, patreon, pixiv, twitter, deviantArt, id); err != nil {
		InternalError(w, r, s.log, err)
		return
	}

	if reviewStatus != "" {
		rs := 0
		if reviewStatus == "Accepted" {
			rs = 1
		}
		if _, err := s.db.Exec(r.Context(), `UPDATE "Artists" SET "ReviewStatus" = $1 WHERE "Id" = $2`, rs, id); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		if reviewStatus == "Accepted" {

			_, _ = s.db.Exec(r.Context(), `
				UPDATE "ReviewTasks" SET "Status" = 1, "ResolvedAt" = now()
				WHERE "TargetType" = 2 AND "TargetId" = $1 AND "Kind" = 0 AND "Status" = 0`, id)
		}
	}
	if req.CreatorID != nil {
		if _, err := s.db.Exec(r.Context(), `UPDATE "Artists" SET "CreatorId" = $1 WHERE "Id" = $2`, *req.CreatorID, id); err != nil {
			InternalError(w, r, s.log, err)
			return
		}
	}

	var a artistEntity
	var rs int
	err = s.db.QueryRow(r.Context(), `
		SELECT "Id","Name","Patreon","Pixiv","Twitter","DeviantArt","ReviewStatus","CreatorId"
		FROM "Artists" WHERE "Id" = $1`, id).
		Scan(&a.ID, &a.Name, &a.Patreon, &a.Pixiv, &a.Twitter, &a.DeviantArt, &rs, &a.CreatorID)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	a.ReviewStatus = reviewStatusString(rs)
	a.Images = []imageDto{}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) handleDeleteArtist(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	var exists bool
	if err := s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM "Artists" WHERE "Id" = $1)`, id).Scan(&exists); err != nil || !exists {
		NotFound(w, r, fmt.Sprintf("Artist with ID %d not found.", id))
		return
	}
	_, err = s.db.Exec(r.Context(), `DELETE FROM "Artists" WHERE "Id" = $1`, id)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
