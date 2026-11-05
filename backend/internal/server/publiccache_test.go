package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/waifu-im/waifu-api/backend/internal/auth"
	"github.com/waifu-im/waifu-api/backend/internal/config"
)

func listIDs(t *testing.T, e *testEnv, query string, opts ...reqOpt) ([]int64, int64) {
	t.Helper()
	w := e.do("GET", "/images?"+query, nil, opts...)
	expectStatus(t, w, http.StatusOK)
	page := decode[struct {
		Items      []imageDto
		TotalCount int64
	}](t, w)
	ids := make([]int64, len(page.Items))
	for i, it := range page.Items {
		ids[i] = it.ID
	}
	return ids, page.TotalCount
}

func trustedUploader(t *testing.T, e *testEnv) string {
	t.Helper()
	tok, uid := e.login("100000000000000900", "cache-uploader", "")
	e.setRole(uid, auth.RoleTrustedUser)
	return tok
}

func TestPublicCacheSingleFlightAndVersions(t *testing.T) {
	var versions dataVersion
	c := newPublicCache[int](time.Minute, scopeContent, &versions, 8, 0)
	var builds atomic.Int32
	release := make(chan struct{})
	build := func(context.Context) (int, int, error) {
		builds.Add(1)
		<-release
		return 7, 1, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v, err := c.load(context.Background(), "k", 0, build); err != nil || v != 7 {
				t.Errorf("load = %d, %v", v, err)
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if n := builds.Load(); n != 1 {
		t.Fatalf("concurrent misses must share one build, got %d", n)
	}
	if _, ok := c.get("k", 0); !ok {
		t.Fatal("value must be cached")
	}
	versions.bump(scopeFavorites)
	if _, ok := c.get("k", 0); !ok {
		t.Fatal("a favorites write must not drop content entries")
	}
	versions.bump(scopeContent)
	if _, ok := c.get("k", 0); ok {
		t.Fatal("a content write must drop content entries")
	}

	failing := func(context.Context) (int, int, error) { return 0, 0, errors.New("boom") }
	if _, err := c.load(context.Background(), "e", 0, failing); err == nil {
		t.Fatal("build errors must be returned")
	}
	if _, ok := c.get("e", 0); ok {
		t.Fatal("failed builds must not be cached")
	}

	stale := func(context.Context) (int, int, error) {
		versions.bump(scopeContent)
		return 1, 1, nil
	}
	if _, err := c.load(context.Background(), "s", 0, stale); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.get("s", 0); ok {
		t.Fatal("a value built while the data changed must not be stored")
	}

	expiring := newPublicCache[int](20*time.Millisecond, scopeContent, &versions, 8, 0)
	if _, err := expiring.load(context.Background(), "x", 0, func(context.Context) (int, int, error) { return 1, 1, nil }); err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	if _, ok := expiring.get("x", 0); ok {
		t.Fatal("entries must expire after the TTL")
	}

	bounded := newPublicCache[int](time.Minute, scopeContent, &versions, 2, 0)
	for _, k := range []string{"a", "b", "c"} {
		if _, err := bounded.load(context.Background(), k, 0, func(context.Context) (int, int, error) { return 1, 1, nil }); err != nil {
			t.Fatal(err)
		}
	}
	if n := bounded.len(); n != 2 {
		t.Fatalf("cache must stay within its entry limit, has %d", n)
	}
	if _, ok := bounded.get("a", 0); ok {
		t.Fatal("the least recently used entry must be evicted first")
	}
}

func TestImageListCacheFollowsWrites(t *testing.T) {
	e := newTestEnv(t)
	tok := trustedUploader(t, e)
	var uploaded []int64
	for i := int64(0); i < 4; i++ {
		w, img := e.upload(tok, 900+i, nil)
		expectStatus(t, w, http.StatusCreated)
		uploaded = append(uploaded, img.ID)
	}

	ids, total := listIDs(t, e, "orderBy=UploadedAt&pageSize=10")
	if total != 4 || len(ids) != 4 || ids[0] != uploaded[3] {
		t.Fatalf("unexpected first listing %v (total %d)", ids, total)
	}

	w, img := e.upload(tok, 950, nil)
	expectStatus(t, w, http.StatusCreated)
	ids, total = listIDs(t, e, "orderBy=UploadedAt&pageSize=10")
	if total != 5 || ids[0] != img.ID {
		t.Fatalf("a new upload must show up at once on the same instance: %v (total %d)", ids, total)
	}

	adminTok, adminID := e.login("100000000000000901", "cache-admin", "")
	e.setRole(adminID, auth.RoleAdmin)
	expectStatus(t, e.do("DELETE", fmt.Sprintf("/images/%d", uploaded[0]), nil, withToken(adminTok)), http.StatusNoContent)
	ids, total = listIDs(t, e, "orderBy=UploadedAt&pageSize=10")
	if total != 4 {
		t.Fatalf("deleted image still counted: %v (total %d)", ids, total)
	}
	for _, id := range ids {
		if id == uploaded[0] {
			t.Fatalf("deleted image %d still listed", id)
		}
	}

	e.exec(`UPDATE "Images" SET "IsNsfw" = true WHERE "Id" = $1`, uploaded[1])
	ids, _ = listIDs(t, e, "orderBy=UploadedAt&pageSize=10&isNsfw=False")
	for _, id := range ids {
		if id == uploaded[1] {
			t.Fatal("rows changed behind the cache must be rechecked before they are served")
		}
	}
	ids, _ = listIDs(t, e, "orderBy=Random&pageSize=10&isNsfw=False")
	for _, id := range ids {
		if id == uploaded[1] {
			t.Fatal("random pages must recheck cached rows as well")
		}
	}
}

func TestRandomOrderSamplesTheWholeFilter(t *testing.T) {
	e := newTestEnv(t)
	tok := trustedUploader(t, e)
	matching := map[int64]bool{}
	w := e.do("POST", "/tags", map[string]string{"name": "Sampled", "description": "only these"}, withToken(tok))
	expectStatus(t, w, http.StatusCreated)
	for i := int64(0); i < 9; i++ {
		fields := map[string][]string{}
		if i%3 != 0 {
			fields["tags"] = []string{"sampled"}
		}
		w, img := e.upload(tok, 960+i, fields)
		expectStatus(t, w, http.StatusCreated)
		if i%3 != 0 {
			matching[img.ID] = true
		}
	}
	firsts := map[int64]bool{}
	for i := 0; i < 40; i++ {
		ids, total := listIDs(t, e, "orderBy=Random&pageSize=3&includedTags=sampled")
		if total != int64(len(matching)) || len(ids) != 3 {
			t.Fatalf("unexpected random page %v (total %d)", ids, total)
		}
		seen := map[int64]bool{}
		for _, id := range ids {
			if !matching[id] || seen[id] {
				t.Fatalf("random page %v contains a non-matching or repeated image", ids)
			}
			seen[id] = true
		}
		firsts[ids[0]] = true
	}
	if len(firsts) < 3 {
		t.Fatalf("random order must vary between requests, saw only %d different first images", len(firsts))
	}
	if w := e.do("GET", "/images?orderBy=Random&pageSize=3", nil); w.Header().Get("ETag") != "" {
		t.Fatal("random pages must not carry an ETag")
	}
}

func TestPublicResponsesAnswerConditionalRequests(t *testing.T) {
	e := newTestEnv(t)
	tok := trustedUploader(t, e)
	expectStatus(t, e.do("POST", "/tags", map[string]string{"name": "Etag", "description": "x"}, withToken(tok)), http.StatusCreated)

	w := e.do("GET", "/tags", nil)
	expectStatus(t, w, http.StatusOK)
	etag := w.Header().Get("ETag")
	if etag == "" || w.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("anonymous list must be revalidatable: etag=%q cache-control=%q", etag, w.Header().Get("Cache-Control"))
	}
	w = e.do("GET", "/tags", nil, withHeader("If-None-Match", etag))
	if w.Code != http.StatusNotModified || w.Body.Len() != 0 {
		t.Fatalf("matching If-None-Match must return 304 without a body, got %d (%d bytes)", w.Code, w.Body.Len())
	}

	expectStatus(t, e.do("POST", "/tags", map[string]string{"name": "Etag two", "description": "y"}, withToken(tok)), http.StatusCreated)
	w = e.do("GET", "/tags", nil, withHeader("If-None-Match", etag))
	if w.Code != http.StatusOK || w.Header().Get("ETag") == etag {
		t.Fatalf("changed data must not be answered with 304: %d", w.Code)
	}

	w = e.do("GET", "/tags", nil, withToken(tok), withHeader("If-None-Match", etag))
	if w.Code != http.StatusOK || w.Header().Get("ETag") != "" {
		t.Fatalf("personalised responses must not use ETags: %d etag=%q", w.Code, w.Header().Get("ETag"))
	}
}

func TestClientErrorsDoNotInvalidateCaches(t *testing.T) {
	e := newTestEnv(t)
	tok := trustedUploader(t, e)
	before := e.srv.versions.stamp(scopeContent | scopeFavorites)
	expectStatus(t, e.do("POST", "/tags", map[string]string{"name": ""}, withToken(tok)), http.StatusBadRequest)
	expectStatus(t, e.do("POST", "/tags", map[string]string{"name": "x"}), http.StatusUnauthorized)
	if after := e.srv.versions.stamp(scopeContent | scopeFavorites); after != before {
		t.Fatal("rejected requests must not invalidate the caches")
	}
	expectStatus(t, e.do("POST", "/tags", map[string]string{"name": "Valid", "description": "ok"}, withToken(tok)), http.StatusCreated)
	if after := e.srv.versions.stamp(scopeContent); after == before {
		t.Fatal("successful writes must invalidate the caches")
	}
}

func TestCacheInvalidationReachesOtherInstances(t *testing.T) {
	rc := testRedis(t)
	prefix := uniquePrefix()
	e := newTestEnv(t, func(c *config.Config) { c.Cluster.RedisPrefix = prefix })
	a := e.peer(WithRedis(rc), WithInstanceID("api-a"))
	b := e.peer(WithRedis(rc), WithInstanceID("api-b"))
	tok := trustedUploader(t, a)
	w, first := a.upload(tok, 980, nil)
	expectStatus(t, w, http.StatusCreated)

	b.srv.pollVersions(context.Background())
	ids, total := listIDs(t, b, "orderBy=UploadedAt&pageSize=10")
	if total != 1 || ids[0] != first.ID {
		t.Fatalf("unexpected listing on b: %v", ids)
	}

	w, second := a.upload(tok, 981, nil)
	expectStatus(t, w, http.StatusCreated)
	ids, _ = listIDs(t, b, "orderBy=UploadedAt&pageSize=10")
	if len(ids) != 1 {
		t.Fatalf("b serves its cached list until it hears about the write, got %v", ids)
	}
	b.srv.pollVersions(context.Background())
	ids, total = listIDs(t, b, "orderBy=UploadedAt&pageSize=10")
	if total != 2 || ids[0] != second.ID {
		t.Fatalf("after the version sync b must see the new upload: %v (total %d)", ids, total)
	}

	before := b.srv.versions.stamp(scopeContent)
	b.srv.pollVersions(context.Background())
	if b.srv.versions.stamp(scopeContent) != before {
		t.Fatal("polling without new writes must keep the caches")
	}
}
