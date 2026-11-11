package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

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

const artistCountJoin = ` CROSS JOIN LATERAL (
		SELECT COUNT(*) AS cnt
		FROM "ArtistImage" cai JOIN "Images" ci ON ci."Id" = cai."ImagesId" AND ci."ReviewStatus" = 1
		WHERE cai."ArtistsId" = a."Id"
	) ac`

const artistCountAggregateJoin = ` LEFT JOIN (
		SELECT cai."ArtistsId" AS aid, COUNT(*) AS cnt
		FROM "ArtistImage" cai JOIN "Images" ci ON ci."Id" = cai."ImagesId" AND ci."ReviewStatus" = 1
		GROUP BY cai."ArtistsId"
	) ac ON ac.aid = a."Id"`

const artistUsageQuery = `
	SELECT cai."ArtistsId", COUNT(*)
	FROM "ArtistImage" cai JOIN "Images" ci ON ci."Id" = cai."ImagesId" AND ci."ReviewStatus" = 1
	GROUP BY cai."ArtistsId"`

const artistListColumns = `a."Id", a."Name", a."Patreon", a."Pixiv", a."Twitter", a."DeviantArt", a."ReviewStatus", a."CreatorId", COALESCE(ac.cnt, 0)`

const artistRowColumns = `a."Id", a."Name", a."Patreon", a."Pixiv", a."Twitter", a."DeviantArt", a."ReviewStatus", a."CreatorId", 0::bigint`

func scanArtistRows(rows pgx.Rows, ident *auth.Identity) ([]artistDto, error) {
	defer rows.Close()
	items := []artistDto{}
	for rows.Next() {
		var a artistDto
		var rs int
		var patreon, pixiv, twitter, deviantArt *string
		if err := rows.Scan(&a.ID, &a.Name, &patreon, &pixiv, &twitter, &deviantArt, &rs, &a.CreatorID, &a.ImageCount); err != nil {
			return nil, err
		}
		a.ReviewStatus = reviewStatusString(rs)
		a.Patreon = orEmpty(patreon)
		a.Pixiv = orEmpty(pixiv)
		a.Twitter = orEmpty(twitter)
		a.DeviantArt = orEmpty(deviantArt)
		if !isModerator(ident) && !(ident != nil && a.CreatorID != nil && *a.CreatorID == ident.UserID) {
			a.CreatorID = nil
		}
		items = append(items, a)
	}
	return items, rows.Err()
}

func (s *Server) handleListArtists(w http.ResponseWriter, r *http.Request) {
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

	if len(includedIDs) > 0 {
		if _, ok := lookupVisibility(r, ident, "a", &sqlArgs{}); !ok {
			Forbidden(w, r, "Filtering by non-accepted review status is only available to moderators and admins.")
			return
		}
		usage, err := s.loadUsage(ctx, "artists", artistUsageQuery)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		a := &sqlArgs{}
		join := artistCountJoin
		columns := artistListColumns
		if usage != nil {
			join = ""
			columns = artistRowColumns
		}
		vis, _ := lookupVisibility(r, ident, "a", a)
		rows, err := s.db.Query(ctx, `SELECT `+columns+` FROM "Artists" a`+join+
			` WHERE a."Id" = ANY(`+a.add(includedIDs)+`) AND `+vis+` ORDER BY a."Id"`, a.args...)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		items, err := scanArtistRows(rows, ident)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		if usage != nil {
			for i := range items {
				items[i].ImageCount = usage.byID[items[i].ID]
			}
		}
		writePublicJSON(w, r, s.newPaginated(items, 1, len(items), int64(len(items)), s.cfg.PageSizes.ArtistMax, s.cfg.PageSizes.ArtistDefault))
		return
	}

	if _, ok := contentVisibility(r, ident, "a", &sqlArgs{}); !ok {
		Forbidden(w, r, "Filtering by non-accepted review status is only available to moderators and admins.")
		return
	}
	page, pageSize := s.paginationParams(r, s.cfg.PageSizes.ArtistDefault, s.cfg.PageSizes.ArtistMax)
	name := strings.TrimSpace(queryGet(r, "name"))
	filter := func(a *sqlArgs) string {
		vis, _ := contentVisibility(r, ident, "a", a)
		where := " WHERE " + vis
		if name != "" {
			where += ` AND lower(a."Name") LIKE ` + a.add(likePattern(strings.ToLower(name)))
		}
		return where
	}

	usage, err := s.loadUsage(ctx, "artists", artistUsageQuery)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if usage != nil {
		idArgs := &sqlArgs{}
		where := filter(idArgs)
		cacheKey := ""
		if name == "" && len(idArgs.args) == 0 {
			cacheKey = "artists" + where
		}
		pageIDs, total, err := s.rankedPage(ctx, cacheKey, `SELECT a."Id" FROM "Artists" a`+where, idArgs.args, usage, page, pageSize)
		if err != nil {
			InternalError(w, r, s.log, err)
			return
		}
		items := []artistDto{}
		if len(pageIDs) > 0 {
			rows, err := s.db.Query(ctx, `SELECT `+artistRowColumns+` FROM "Artists" a WHERE a."Id" = ANY($1)`, pageIDs)
			if err != nil {
				InternalError(w, r, s.log, err)
				return
			}
			scanned, err := scanArtistRows(rows, ident)
			if err != nil {
				InternalError(w, r, s.log, err)
				return
			}
			items = orderByIDs(pageIDs, scanned, func(a artistDto) int64 { return a.ID })
			for i := range items {
				items[i].ImageCount = usage.byID[items[i].ID]
			}
		}
		writePublicJSON(w, r, s.newPaginated(items, page, pageSize, total, s.cfg.PageSizes.ArtistMax, s.cfg.PageSizes.ArtistDefault))
		return
	}

	countArgs := &sqlArgs{}
	countWhere := filter(countArgs)
	listArgs := &sqlArgs{}
	join := artistCountAggregateJoin
	listWhere := filter(listArgs)

	var total int64
	var items []artistDto
	b := &pgx.Batch{}
	b.Queue(`SELECT COUNT(*) FROM "Artists" a`+countWhere, countArgs.args...).QueryRow(func(row pgx.Row) error {
		return row.Scan(&total)
	})
	b.Queue(`SELECT `+artistListColumns+` FROM "Artists" a`+join+listWhere+
		` ORDER BY COALESCE(ac.cnt, 0) DESC, a."Id" ASC`+
		fmt.Sprintf(" LIMIT %d OFFSET %d", pageSize, offsetOf(page, pageSize)), listArgs.args...).Query(func(rows pgx.Rows) error {
		var err error
		items, err = scanArtistRows(rows, ident)
		return err
	})
	if err := s.db.SendBatch(ctx, b).Close(); err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	writePublicJSON(w, r, s.newPaginated(items, page, pageSize, total, s.cfg.PageSizes.ArtistMax, s.cfg.PageSizes.ArtistDefault))
}

func (s *Server) handleGetArtist(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
		return
	}
	var a artistDto
	var rs int
	var patreon, pixiv, twitter, deviantArt *string
	var creatorID *int64
	err = s.db.QueryRow(r.Context(), `
		SELECT a."Id", a."Name", a."Patreon", a."Pixiv", a."Twitter", a."DeviantArt", a."ReviewStatus", a."CreatorId"
		FROM "Artists" a WHERE a."Id" = $1`, id).
		Scan(&a.ID, &a.Name, &patreon, &pixiv, &twitter, &deviantArt, &rs, &creatorID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !canSeeContent(ident, rs, creatorID)) {
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
	writePublicJSON(w, r, a)
}

func (s *Server) handleGetArtistByName(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	name := r.PathValue("name")
	if runeLen(name) > 100 {
		NotFound(w, r, "Artist not found.")
		return
	}
	var a artistEntity
	var rs int
	err := s.db.QueryRow(r.Context(), `
		SELECT "Id","Name","Patreon","Pixiv","Twitter","DeviantArt","ReviewStatus","CreatorId"
		FROM "Artists" WHERE "Name" = $1 ORDER BY "Id" LIMIT 1`, name).
		Scan(&a.ID, &a.Name, &a.Patreon, &a.Pixiv, &a.Twitter, &a.DeviantArt, &rs, &a.CreatorID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !canSeeContent(ident, rs, a.CreatorID)) {
		NotFound(w, r, fmt.Sprintf("Artist with name '%s' not found.", name))
		return
	}
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if !isModerator(ident) && !(ident != nil && a.CreatorID != nil && *a.CreatorID == ident.UserID) {
		a.CreatorID = nil
	}
	a.ReviewStatus = reviewStatusString(rs)
	a.Images = []imageDto{}
	writePublicJSON(w, r, a)
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
	} else if runeLen(strings.TrimSpace(req.Name)) > 30 {
		errs["Name"] = append(errs["Name"], "Name must not exceed 30 characters.")
	}
	for _, f := range []struct {
		field string
		v     *string
	}{{"Patreon", req.Patreon}, {"Pixiv", req.Pixiv}, {"Twitter", req.Twitter}, {"DeviantArt", req.DeviantArt}} {
		if f.v == nil || strings.TrimSpace(*f.v) == "" {
			continue
		}
		v := strings.TrimSpace(*f.v)
		if runeLen(v) > 200 {
			errs[f.field] = append(errs[f.field], f.field+" link must not exceed 200 characters.")
		}
		if !isValidWebURL(v) {
			errs[f.field] = append(errs[f.field], f.field+" link must be a valid http(s) URL.")
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

type artistConflictCheck struct {
	field  string
	value  *string
	column string
}

func artistConflicts(name string, patreon, pixiv, twitter, deviantArt *string) []artistConflictCheck {
	return []artistConflictCheck{
		{"Name", strPtr(name), `lower("Name") = lower($1)`},
		{"Patreon", patreon, `"Patreon" = $1`},
		{"Pixiv", pixiv, `"Pixiv" = $1`},
		{"Twitter", twitter, `"Twitter" = $1`},
		{"DeviantArt", deviantArt, `"DeviantArt" = $1`},
	}
}

func checkArtistConflicts(ctx context.Context, q querier, excludeID int64, name string, patreon, pixiv, twitter, deviantArt *string) error {
	for _, c := range artistConflicts(name, patreon, pixiv, twitter, deviantArt) {
		if c.value == nil || *c.value == "" {
			continue
		}
		var exists bool
		query := fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM "Artists" WHERE %s AND "Id" <> $2)`, c.column)
		if err := q.QueryRow(ctx, query, *c.value, excludeID).Scan(&exists); err != nil {
			return err
		}
		if exists {
			if c.field == "Name" {
				return errConflict(fmt.Sprintf("Artist with name '%s' already exists.", name))
			}
			if excludeID == 0 {
				return errConflict(fmt.Sprintf("Artist with %s '%s' already exists.", c.field, *c.value))
			}
			return errConflict(fmt.Sprintf("%s link is already used by another artist.", c.field))
		}
	}
	return nil
}

func (s *Server) handleCreateArtist(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
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

	autoAccept := !s.cfg.Review.RequireArtistReview || auth.ReviewBypassed(ident.Role, s.cfg.Review.ArtistReviewBypassMinRole)
	reviewStatus := 0
	if autoAccept {
		reviewStatus = 1
	}

	var a artistEntity
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if !autoAccept {
			if err := s.checkPendingSubmissions(ctx, tx, ident.UserID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('artists'))`); err != nil {
			return err
		}
		if err := checkArtistConflicts(ctx, tx, 0, trimmedName, patreon, pixiv, twitter, deviantArt); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO "Artists" ("Name","Patreon","Pixiv","Twitter","DeviantArt","ReviewStatus","CreatorId")
			VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING "Id"`,
			trimmedName, patreon, pixiv, twitter, deviantArt, reviewStatus, ident.UserID).Scan(&a.ID); err != nil {
			if isUniqueViolation(err) {
				return errConflict(fmt.Sprintf("Artist with name '%s' already exists.", trimmedName))
			}
			return err
		}
		if autoAccept {
			_, err := tx.Exec(ctx, `
				INSERT INTO "ReviewTasks" ("Kind","TargetType","TargetId","SubmitterId","Status","ReviewerNote","CreatedAt","ResolvedAt")
				VALUES (0,2,$1,$2,1,'Auto-accepted (trusted role).',now(),now())`, a.ID, ident.UserID)
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO "ReviewTasks" ("Kind","TargetType","TargetId","SubmitterId","Status","CreatedAt")
			VALUES (0,2,$1,$2,0,now())`, a.ID, ident.UserID)
		return err
	})
	if err != nil {
		s.writeDomainError(w, r, err)
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

func (s *Server) handlePatchArtist(w http.ResponseWriter, r *http.Request) {
	ident := IdentityFromContext(r.Context())
	ctx := r.Context()
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		RouteNotFound(w, r)
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

	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := lockPendingTasks(ctx, tx, 2, id); err != nil {
			return err
		}
		var lockedID int64
		if err := tx.QueryRow(ctx, `SELECT "Id" FROM "Artists" WHERE "Id" = $1 FOR UPDATE`, id).Scan(&lockedID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return errNotFound(fmt.Sprintf("Artist with ID %d not found.", id))
			}
			return err
		}
		if err := checkArtistConflicts(ctx, tx, id, trimmedName, patreon, pixiv, twitter, deviantArt); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE "Artists" SET "Name" = $1, "Patreon" = $2, "Pixiv" = $3, "Twitter" = $4, "DeviantArt" = $5
			WHERE "Id" = $6`,
			trimmedName, patreon, pixiv, twitter, deviantArt, id); err != nil {
			return err
		}
		if reviewStatus != "" {
			rs := 0
			if reviewStatus == "Accepted" {
				rs = 1
			}
			if _, err := tx.Exec(ctx, `UPDATE "Artists" SET "ReviewStatus" = $1 WHERE "Id" = $2`, rs, id); err != nil {
				return err
			}
			if rs == 1 {
				if _, err := tx.Exec(ctx, `
					UPDATE "ReviewTasks" SET "Status" = 1, "ReviewerId" = $2, "ResolvedAt" = now()
					WHERE "TargetType" = 2 AND "TargetId" = $1 AND "Kind" = 0 AND "Status" = 0`, id, ident.UserID); err != nil {
					return err
				}
			}
		}
		if req.CreatorID != nil {
			if _, err := tx.Exec(ctx, `UPDATE "Artists" SET "CreatorId" = $1 WHERE "Id" = $2`, *req.CreatorID, id); err != nil {
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
			Conflict(w, r, "An artist with this name or link already exists.")
			return
		}
		s.writeDomainError(w, r, err)
		return
	}

	var a artistEntity
	var rs int
	err = s.db.QueryRow(ctx, `
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
	res, err := s.db.Exec(r.Context(), `DELETE FROM "Artists" WHERE "Id" = $1`, id)
	if err != nil {
		InternalError(w, r, s.log, err)
		return
	}
	if res.RowsAffected() == 0 {
		NotFound(w, r, fmt.Sprintf("Artist with ID %d not found.", id))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
