package server

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/waifu-im/waifu-api/backend/internal/auth"
)

type createdEntity struct {
	ID   int64  `json:"id"`
	Slug string `json:"slug"`
}

func uploadMany(t *testing.T, e *testEnv, tok string, seed int64, n int, fields map[string][]string) []int64 {
	t.Helper()
	ids := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		w, img := e.upload(tok, seed+int64(i), fields)
		expectStatus(t, w, http.StatusCreated)
		ids = append(ids, img.ID)
	}
	return ids
}

func createTag(t *testing.T, e *testEnv, tok, name string) createdEntity {
	t.Helper()
	w := e.do("POST", "/tags", map[string]string{"name": name, "description": name + " description"}, withToken(tok))
	expectStatus(t, w, http.StatusCreated)
	return decode[createdEntity](t, w)
}

func createArtist(t *testing.T, e *testEnv, tok, name string) createdEntity {
	t.Helper()
	w := e.do("POST", "/artists", map[string]any{"name": name}, withToken(tok))
	expectStatus(t, w, http.StatusCreated)
	return decode[createdEntity](t, w)
}

func imageFavorites(t *testing.T, e *testEnv, id int64, opts ...reqOpt) int64 {
	t.Helper()
	w := e.do("GET", fmt.Sprintf("/images/%d", id), nil, opts...)
	expectStatus(t, w, http.StatusOK)
	return decode[imageDto](t, w).Favorites
}

func TestFavoriteCountsAreCachedAndFollowWrites(t *testing.T) {
	e := newTestEnv(t)
	tok := trustedUploader(t, e)
	ids := uploadMany(t, e, tok, 1100, 2, nil)
	fanTok, _ := e.login("100000000000001101", "fan-one", "")
	otherTok, otherID := e.login("100000000000001102", "fan-two", "")

	expectStatus(t, e.do("POST", fmt.Sprintf("/users/me/albums/favorites/images/%d", ids[0]), nil, withToken(fanTok)), http.StatusNoContent)
	if n := imageFavorites(t, e, ids[0]); n != 1 {
		t.Fatalf("a favorite must count at once, got %d", n)
	}

	var otherAlbum int64
	if err := e.pool.QueryRow(t.Context(), `SELECT "Id" FROM "Albums" WHERE "UserId" = $1 AND "IsDefault"`, otherID).Scan(&otherAlbum); err != nil {
		w := e.do("GET", "/users/me/albums/favorites", nil, withToken(otherTok))
		expectStatus(t, w, http.StatusOK)
		otherAlbum = e.scalar(`SELECT "Id" FROM "Albums" WHERE "UserId" = $1 AND "IsDefault"`, otherID)
	}
	e.exec(`INSERT INTO "AlbumItems" ("AlbumId","ImageId","AddedAt") VALUES ($1,$2,now())`, otherAlbum, ids[0])
	if n := imageFavorites(t, e, ids[0]); n != 1 {
		t.Fatalf("counts must come from the cache between writes, got %d", n)
	}

	expectStatus(t, e.do("POST", fmt.Sprintf("/users/me/albums/favorites/images/%d", ids[1]), nil, withToken(fanTok)), http.StatusNoContent)
	if n := imageFavorites(t, e, ids[0]); n != 2 {
		t.Fatalf("any favorite write must refresh the cached counts, got %d", n)
	}

	expectStatus(t, e.do("DELETE", fmt.Sprintf("/users/me/albums/favorites/images/%d", ids[0]), nil, withToken(fanTok)), http.StatusNoContent)
	w := e.do("GET", "/images?orderBy=Favorites&pageSize=10", nil)
	expectStatus(t, w, http.StatusOK)
	page := decode[struct{ Items []imageDto }](t, w)
	counts := map[int64]int64{}
	for _, img := range page.Items {
		counts[img.ID] = img.Favorites
	}
	if counts[ids[0]] != 1 || counts[ids[1]] != 1 {
		t.Fatalf("list counts must be current: %v", counts)
	}
}

func TestTrendingOrder(t *testing.T) {
	e := newTestEnv(t)
	tok := trustedUploader(t, e)
	ids := uploadMany(t, e, tok, 1200, 3, nil)
	for i := 0; i < 2; i++ {
		fan, _ := e.login(fmt.Sprintf("10000000000000121%d", i), fmt.Sprintf("trend-%d", i), "")
		expectStatus(t, e.do("POST", fmt.Sprintf("/users/me/albums/favorites/images/%d", ids[1]), nil, withToken(fan)), http.StatusNoContent)
		expectStatus(t, e.do("POST", fmt.Sprintf("/users/me/albums/favorites/images/%d", ids[0]), nil, withToken(fan)), http.StatusNoContent)
	}
	fan, _ := e.login("100000000000001220", "trend-late", "")
	expectStatus(t, e.do("POST", fmt.Sprintf("/users/me/albums/favorites/images/%d", ids[1]), nil, withToken(fan)), http.StatusNoContent)
	e.exec(`UPDATE "AlbumItems" SET "AddedAt" = now() - interval '30 days' WHERE "ImageId" = $1`, ids[0])
	e.exec(`UPDATE "Images" SET "UploadedAt" = now() - interval '2 days' WHERE "Id" = $1`, ids[2])

	got, total := listIDs(t, e, "orderBy=Trending&pageSize=10")
	want := []int64{ids[1], ids[0], ids[2]}
	if total != 3 || !slices.Equal(got, want) {
		t.Fatalf("trending order %v, want %v", got, want)
	}
	got, _ = listIDs(t, e, "orderBy=Trending&pageSize=10&includeMyPending=true", withToken(tok))
	if !slices.Equal(got, want) {
		t.Fatalf("uncached trending order %v, want %v", got, want)
	}
	expectStatus(t, e.do("GET", "/images?orderBy=Sideways", nil), http.StatusBadRequest)
}

func TestColorAndDateFilters(t *testing.T) {
	e := newTestEnv(t)
	tok := trustedUploader(t, e)
	ids := uploadMany(t, e, tok, 1300, 4, nil)
	colors := []string{"#FF0000", "#F80A0A", "#00FF00", "invalid"}
	for i, c := range colors {
		e.exec(`UPDATE "Images" SET "DominantColor" = $1 WHERE "Id" = $2`, c, ids[i])
	}
	e.exec(`UPDATE "Images" SET "UploadedAt" = '2026-01-10T12:00:00Z' WHERE "Id" = $1`, ids[0])
	e.exec(`UPDATE "Images" SET "UploadedAt" = '2026-03-10T12:00:00Z' WHERE "Id" = $1`, ids[1])

	reds := []int64{ids[0], ids[1]}
	slices.Sort(reds)
	for _, opts := range [][]reqOpt{nil, {withToken(tok)}} {
		query := "color=ff0000&orderBy=UploadedAt&pageSize=10"
		if opts != nil {
			query += "&includeMyPending=true"
		}
		got, total := listIDs(t, e, query, opts...)
		slices.Sort(got)
		if total != 2 || !slices.Equal(got, reds) {
			t.Fatalf("color filter (%s) returned %v (total %d), want %v", query, got, total, reds)
		}
	}
	if _, total := listIDs(t, e, "color=%2300FF00&colorTolerance=100&pageSize=10"); total != 3 {
		t.Fatalf("full tolerance must match every valid color, got %d", total)
	}
	expectStatus(t, e.do("GET", "/images?color=red", nil), http.StatusBadRequest)
	expectStatus(t, e.do("GET", "/images?color=ff0000&colorTolerance=0", nil), http.StatusBadRequest)

	got, _ := listIDs(t, e, "uploadedBefore=2026-02-01&pageSize=10")
	if !slices.Equal(got, []int64{ids[0]}) {
		t.Fatalf("uploadedBefore returned %v", got)
	}
	_, total := listIDs(t, e, "uploadedAfter=2026-02-01T00:00:00Z&uploadedBefore=2026-04-01&pageSize=10")
	if total != 1 {
		t.Fatalf("date range returned %d images", total)
	}
	expectStatus(t, e.do("GET", "/images?uploadedAfter=yesterday", nil), http.StatusBadRequest)
}

func TestImageFacets(t *testing.T) {
	e := newTestEnv(t)
	tok := trustedUploader(t, e)
	maid := createTag(t, e, tok, "Maid")
	night := createTag(t, e, tok, "Night")
	rain := createTag(t, e, tok, "Rain")
	artist := createArtist(t, e, tok, "Facet Artist")
	uploadMany(t, e, tok, 1400, 3, map[string][]string{"tags": {maid.Slug, night.Slug}, "artists": {strconv.FormatInt(artist.ID, 10)}})
	uploadMany(t, e, tok, 1410, 1, map[string][]string{"tags": {maid.Slug, rain.Slug}})
	uploadMany(t, e, tok, 1420, 2, map[string][]string{"tags": {rain.Slug}})

	w := e.do("GET", "/images/facets?includedTags="+maid.Slug, nil)
	expectStatus(t, w, http.StatusOK)
	res := decode[facetResult](t, w)
	if res.TotalCount != 4 {
		t.Fatalf("facet total %d, want 4", res.TotalCount)
	}
	if len(res.Tags) != 2 || res.Tags[0].ID != night.ID || res.Tags[0].Count != 3 || res.Tags[1].ID != rain.ID || res.Tags[1].Count != 1 {
		t.Fatalf("unexpected tag facets %+v", res.Tags)
	}
	if len(res.Artists) != 1 || res.Artists[0].ID != artist.ID || res.Artists[0].Count != 3 {
		t.Fatalf("unexpected artist facets %+v", res.Artists)
	}
	w = e.do("GET", "/images/facets?limit=1", nil)
	expectStatus(t, w, http.StatusOK)
	res = decode[facetResult](t, w)
	if res.TotalCount != 6 || len(res.Tags) != 1 || res.Tags[0].Count != 4 {
		t.Fatalf("unexpected global facets %+v", res)
	}
	expectStatus(t, e.do("GET", "/images/facets?limit=99", nil), http.StatusBadRequest)
	expectStatus(t, e.do("GET", "/images/facets?reviewStatus=Pending", nil), http.StatusForbidden)
}

func TestSimilarImages(t *testing.T) {
	e := newTestEnv(t)
	tok := trustedUploader(t, e)
	ids := uploadMany(t, e, tok, 1500, 4, nil)
	hashes := []string{
		"0000000000000000000000000000000000000000000000000000000000000000",
		"0000000000000000000000000000000000000000000000000000000000000111",
		"0000000000000000000000000000000000000000000000000000000000000001",
		"1111111111111111111111111111111111111111111111111111111111111111",
	}
	for i, h := range hashes {
		e.exec(`UPDATE "Images" SET "PerceptualHash" = $1::bit(64) WHERE "Id" = $2`, h, ids[i])
	}
	w := e.do("GET", fmt.Sprintf("/images/%d/similar?limit=5", ids[0]), nil)
	expectStatus(t, w, http.StatusOK)
	page := decode[struct{ Items []imageDto }](t, w)
	var got []int64
	for _, img := range page.Items {
		got = append(got, img.ID)
	}
	if !slices.Equal(got, []int64{ids[2], ids[1], ids[3]}) {
		t.Fatalf("similar order %v", got)
	}
	w = e.do("GET", fmt.Sprintf("/images/%d/similar?maxDistance=2", ids[0]), nil)
	if n := len(decode[struct{ Items []imageDto }](t, w).Items); n != 1 {
		t.Fatalf("maxDistance must cut off far images, got %d", n)
	}
	expectStatus(t, e.do("GET", "/images/999999/similar", nil), http.StatusNotFound)
	e.exec(`UPDATE "Images" SET "ReviewStatus" = 0 WHERE "Id" = $1`, ids[3])
	expectStatus(t, e.do("GET", fmt.Sprintf("/images/%d/similar", ids[3]), nil), http.StatusNotFound)
	expectStatus(t, e.do("GET", fmt.Sprintf("/images/%d/similar", ids[3]), nil, withToken(tok)), http.StatusOK)
}

func TestImageFeed(t *testing.T) {
	e := newTestEnv(t)
	tok := trustedUploader(t, e)
	tag := createTag(t, e, tok, "Feed Tag")
	ids := uploadMany(t, e, tok, 1600, 2, map[string][]string{"tags": {tag.Slug}})
	uploadMany(t, e, tok, 1610, 1, map[string][]string{"isNsfw": {"true"}})

	w := e.do("GET", "/images/feed?includedTags="+tag.Slug, nil)
	expectStatus(t, w, http.StatusOK)
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/atom+xml") {
		t.Fatalf("content type %q", ct)
	}
	var feed atomFeed
	if err := xml.Unmarshal(w.Body.Bytes(), &feed); err != nil {
		t.Fatal(err)
	}
	if len(feed.Entries) != 2 || feed.Entries[0].ID != fmt.Sprintf("https://app.test/images/%d", ids[1]) {
		t.Fatalf("unexpected entries %+v", feed.Entries)
	}
	if !strings.Contains(feed.Entries[0].Content.Body, "<img src=") || len(feed.Entries[0].Categories) != 1 {
		t.Fatalf("entry content missing: %+v", feed.Entries[0])
	}
	etag := w.Header().Get("ETag")
	expectStatus(t, e.do("GET", "/images/feed?includedTags="+tag.Slug, nil, withHeader("If-None-Match", etag)), http.StatusNotModified)

	w = e.do("GET", "/images/feed", nil)
	var all atomFeed
	if err := xml.Unmarshal(w.Body.Bytes(), &all); err != nil || len(all.Entries) != 2 {
		t.Fatalf("the feed must hide NSFW images by default (%d entries, %v)", len(all.Entries), err)
	}
	expectStatus(t, e.do("GET", "/images/feed?pageSize=0", nil), http.StatusBadRequest)
}

func TestAlbumBatchUpdate(t *testing.T) {
	e := newTestEnv(t)
	tok := trustedUploader(t, e)
	ids := uploadMany(t, e, tok, 1700, 3, nil)
	userTok, _ := e.login("100000000000001701", "collector", "")
	otherTok, _ := e.login("100000000000001702", "stranger", "")
	w, pending := e.upload(otherTok, 1750, nil)
	expectStatus(t, w, http.StatusCreated)

	w = e.do("POST", "/users/me/albums", map[string]string{"name": "Batch", "description": ""}, withToken(userTok))
	expectStatus(t, w, http.StatusCreated)
	album := decode[albumDto](t, w)
	path := fmt.Sprintf("/users/me/albums/%d/images", album.ID)

	w = e.do("PATCH", path, map[string]any{"add": []int64{ids[0], ids[1], ids[1], pending.ID, 999999}}, withToken(userTok))
	expectStatus(t, w, http.StatusOK)
	res := decode[albumBatchResult](t, w)
	slices.Sort(res.Skipped)
	if res.Added != 2 || res.ImageCount != 2 || !slices.Equal(res.Skipped, []int64{pending.ID, 999999}) {
		t.Fatalf("unexpected add result %+v", res)
	}
	w = e.do("PATCH", path, map[string]any{"add": []int64{ids[2]}, "remove": []int64{ids[0]}}, withToken(userTok))
	expectStatus(t, w, http.StatusOK)
	res = decode[albumBatchResult](t, w)
	if res.Added != 1 || res.Removed != 1 || res.ImageCount != 2 {
		t.Fatalf("unexpected mixed result %+v", res)
	}
	expectStatus(t, e.do("PATCH", fmt.Sprintf("/users/me/albums/%d/images", album.ID), map[string]any{"add": []int64{ids[0]}}, withToken(otherTok)), http.StatusNotFound)
	expectStatus(t, e.do("PATCH", path, map[string]any{}, withToken(userTok)), http.StatusBadRequest)
	many := make([]int64, albumBatchMax+1)
	for i := range many {
		many[i] = int64(i + 1)
	}
	expectStatus(t, e.do("PATCH", path, map[string]any{"add": many}, withToken(userTok)), http.StatusBadRequest)
	expectStatus(t, e.do("PATCH", path, map[string]any{"remove": []int64{-4}}, withToken(userTok)), http.StatusBadRequest)
	expectStatus(t, e.do("PATCH", path, map[string]any{"add": []int64{ids[0]}}), http.StatusUnauthorized)
}

func TestAccountSummaryExportAndMine(t *testing.T) {
	e := newTestEnv(t)
	tok, uid := e.login("100000000000001801", "exporter", "")
	w, pending := e.upload(tok, 1800, nil)
	expectStatus(t, w, http.StatusCreated)
	e.setRole(uid, auth.RoleTrustedUser)
	w, accepted := e.upload(tok, 1801, nil)
	expectStatus(t, w, http.StatusCreated)
	expectStatus(t, e.do("POST", fmt.Sprintf("/users/me/albums/favorites/images/%d", accepted.ID), nil, withToken(tok)), http.StatusNoContent)
	expectStatus(t, e.do("POST", "/auth/api-keys", map[string]string{"description": "export key"}, withToken(tok)), http.StatusCreated)

	w = e.do("GET", "/users/me/summary", nil, withToken(tok))
	expectStatus(t, w, http.StatusOK)
	sum := decode[accountSummary](t, w)
	if sum.Uploads.Accepted != 1 || sum.Uploads.Pending != 1 || sum.Favorites != 1 || sum.APIKeys != 1 || sum.Albums != 1 {
		t.Fatalf("unexpected summary %+v", sum)
	}

	w = e.do("GET", "/users/me/export", nil, withToken(tok))
	expectStatus(t, w, http.StatusOK)
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Fatalf("export must download as a file, got %q", cd)
	}
	body := w.Body.String()
	if strings.Contains(body, "keyHash") || strings.Contains(body, "KeyHash") {
		t.Fatal("the export must not contain key hashes")
	}
	exp := decode[accountExport](t, w)
	if exp.User == nil || exp.User.ID != uid || len(exp.Uploads) != 2 || len(exp.APIKeys) != 1 || len(exp.Albums) != 1 || len(exp.Albums[0].Images) != 1 {
		t.Fatalf("unexpected export %+v", exp)
	}
	expectStatus(t, e.do("GET", "/users/me/export", nil), http.StatusUnauthorized)

	got, _ := listIDs(t, e, "mine=true&includeMyPending=true&orderBy=UploadedAt&pageSize=10", withToken(tok))
	if !slices.Equal(got, []int64{accepted.ID, pending.ID}) {
		t.Fatalf("mine returned %v", got)
	}
	if _, total := listIDs(t, e, "mine=true&pageSize=10"); total != 0 {
		t.Fatalf("mine without an account must be empty, got %d", total)
	}
}

func TestRankedTagAndArtistLists(t *testing.T) {
	e := newTestEnv(t)
	tok := trustedUploader(t, e)
	a := createTag(t, e, tok, "Alpha")
	b := createTag(t, e, tok, "Beta")
	c := createTag(t, e, tok, "Gamma")
	uploadMany(t, e, tok, 1900, 2, map[string][]string{"tags": {b.Slug}})
	uploadMany(t, e, tok, 1910, 1, map[string][]string{"tags": {c.Slug, b.Slug}})
	userTok, _ := e.login("100000000000001901", "pending-creator", "")
	w := e.do("POST", "/tags", map[string]string{"name": "Delta", "description": "pending"}, withToken(userTok))
	expectStatus(t, w, http.StatusCreated)
	pendingTag := decode[createdEntity](t, w)

	for round := 0; round < 2; round++ {
		w = e.do("GET", "/tags?pageSize=2", nil)
		expectStatus(t, w, http.StatusOK)
		page := decode[struct {
			Items      []tagDto
			TotalCount int64
			TotalPages int
		}](t, w)
		if page.TotalCount != 3 || page.TotalPages != 2 || len(page.Items) != 2 || page.Items[0].ID != b.ID || page.Items[0].ImageCount != 3 || page.Items[1].ID != c.ID {
			t.Fatalf("round %d: unexpected first page %+v", round, page)
		}
		w = e.do("GET", "/tags?pageSize=2&page=2", nil)
		second := decode[struct{ Items []tagDto }](t, w)
		if len(second.Items) != 1 || second.Items[0].ID != a.ID || second.Items[0].ImageCount != 0 {
			t.Fatalf("round %d: unexpected second page %+v", round, second.Items)
		}
	}
	w = e.do("GET", "/tags?name=amm", nil)
	if items := decode[struct{ Items []tagDto }](t, w).Items; len(items) != 1 || items[0].ID != c.ID {
		t.Fatalf("name filter returned %+v", items)
	}
	w = e.do("GET", "/tags?includeMyPending=true&pageSize=10", nil, withToken(userTok))
	if items := decode[struct{ Items []tagDto }](t, w).Items; len(items) != 4 || items[3].ID != pendingTag.ID {
		t.Fatalf("own pending tags must be listed for their creator: %+v", items)
	}
	w = e.do("GET", fmt.Sprintf("/tags?includedIds=%d&includedIds=%d", b.ID, a.ID), nil)
	if items := decode[struct{ Items []tagDto }](t, w).Items; len(items) != 2 || items[0].ID != a.ID || items[1].ImageCount != 3 {
		t.Fatalf("lookups must carry usage counts: %+v", items)
	}

	art := createArtist(t, e, tok, "Ranked Artist")
	uploadMany(t, e, tok, 1950, 2, map[string][]string{"artists": {strconv.FormatInt(art.ID, 10)}})
	createArtist(t, e, tok, "Unused Artist")
	w = e.do("GET", "/artists?pageSize=1", nil)
	page := decode[struct {
		Items      []artistDto
		TotalCount int64
	}](t, w)
	if page.TotalCount != 2 || len(page.Items) != 1 || page.Items[0].ID != art.ID || page.Items[0].ImageCount != 2 {
		t.Fatalf("unexpected artist ranking %+v", page)
	}
}

func TestReportsAndRoleChangesKeepPublicCaches(t *testing.T) {
	e := newTestEnv(t)
	tok := trustedUploader(t, e)
	ids := uploadMany(t, e, tok, 2000, 1, nil)
	userTok, userID := e.login("100000000000002001", "reporter", "")
	adminTok, adminID := e.login("100000000000002002", "report-admin", "")
	e.setRole(adminID, auth.RoleAdmin)

	before := e.srv.versions.content.Load()
	expectStatus(t, e.do("POST", "/reports", map[string]any{"imageId": ids[0], "description": "wrong tags"}, withToken(userTok)), http.StatusCreated)
	expectStatus(t, e.do("PUT", fmt.Sprintf("/users/%d/role", userID), map[string]string{"role": "TrustedUser"}, withToken(adminTok)), http.StatusOK)
	if after := e.srv.versions.content.Load(); after != before {
		t.Fatalf("reports and role changes must not drop the public caches (%d -> %d)", before, after)
	}
	expectStatus(t, e.do("DELETE", fmt.Sprintf("/images/%d", ids[0]), nil, withToken(adminTok)), http.StatusNoContent)
	if e.srv.versions.content.Load() == before {
		t.Fatal("content writes must still invalidate")
	}
}
