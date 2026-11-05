package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http/httptest"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func mustPrefixes(t *testing.T, s ...string) []netip.Prefix {
	t.Helper()
	var out []netip.Prefix
	for _, p := range s {
		out = append(out, netip.MustParsePrefix(p))
	}
	return out
}

func TestClientIPIgnoresForwardedHeadersFromUntrustedPeers(t *testing.T) {
	ir := newIPResolver(mustPrefixes(t, "10.0.0.0/8"), "")
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.7:4321"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	ip, trusted := ir.resolve(r)
	if trusted || ip.String() != "203.0.113.7" {
		t.Fatalf("spoofed XFF must be ignored, got %s trusted=%v", ip, trusted)
	}
}

func TestClientIPRightmostUntrusted(t *testing.T) {
	ir := newIPResolver(mustPrefixes(t, "10.0.0.0/8", "127.0.0.0/8"), "")
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.2:1234"
	r.Header.Set("X-Forwarded-For", "6.6.6.6, 198.51.100.9, 10.0.0.5")
	ip, trusted := ir.resolve(r)
	if !trusted || ip.String() != "198.51.100.9" {
		t.Fatalf("expected the rightmost untrusted address, got %s", ip)
	}
}

func TestClientIPRealIPHeader(t *testing.T) {
	ir := newIPResolver(mustPrefixes(t, "10.0.0.0/8"), "CF-Connecting-IP")
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.1.2.3:80"
	r.Header.Set("CF-Connecting-IP", "2001:db8::1")
	ip, _ := ir.resolve(r)
	if ip.String() != "2001:db8::1" {
		t.Fatalf("expected header address, got %s", ip)
	}
	if k := rateLimitKey(ip); k != "2001:db8::/64" {
		t.Fatalf("IPv6 clients must be bucketed per /64, got %s", k)
	}
}

func TestClientIPRealIPHeaderNeedsTrustedHop(t *testing.T) {
	ir := newIPResolver(mustPrefixes(t, "10.0.0.0/8", "173.245.48.0/20"), "CF-Connecting-IP")

	direct := httptest.NewRequest("GET", "/", nil)
	direct.RemoteAddr = "10.0.0.2:1234"
	direct.Header.Set("X-Forwarded-For", "203.0.113.7")
	direct.Header.Set("CF-Connecting-IP", "198.51.100.1")
	if ip, _ := ir.resolve(direct); ip.String() != "203.0.113.7" {
		t.Fatalf("a header that a client sent past the edge must be ignored, got %s", ip)
	}

	viaEdge := httptest.NewRequest("GET", "/", nil)
	viaEdge.RemoteAddr = "10.0.0.2:1234"
	viaEdge.Header.Set("X-Forwarded-For", "6.6.6.6, 173.245.48.9")
	viaEdge.Header.Set("CF-Connecting-IP", "198.51.100.1")
	if ip, _ := ir.resolve(viaEdge); ip.String() != "198.51.100.1" {
		t.Fatalf("the header of a trusted edge must be used, got %s", ip)
	}
}

func TestLimiterStoreBoundsMemory(t *testing.T) {
	ls := newLimiterStore("t", 60, 2, 100)
	now := time.Now()
	for i := 0; i < 1000; i++ {
		ls.take(netip.AddrFrom4([4]byte{1, 2, byte(i >> 8), byte(i)}).String(), now)
	}
	if ls.size() > 100 {
		t.Fatalf("limiter store grew beyond its cap: %d", ls.size())
	}
	ls.sweep(now.Add(time.Hour))
	if ls.size() != 0 {
		t.Fatalf("idle entries should be evicted, %d left", ls.size())
	}
}

func TestLimiterStoreRejectsAfterBurst(t *testing.T) {
	ls := newLimiterStore("t", 60, 3, 10)
	now := time.Now()
	for i := 0; i < 3; i++ {
		if !ls.take("a", now).allowed {
			t.Fatalf("request %d should pass", i)
		}
	}
	res := ls.take("a", now)
	if res.allowed || res.retryAfter <= 0 {
		t.Fatalf("expected throttling with retry-after, got %+v", res)
	}
	if !ls.take("b", now).allowed {
		t.Fatal("other clients must not be affected")
	}
}

func TestIsValidWebURL(t *testing.T) {
	ok := []string{"https://pixiv.net/artworks/1", "http://example.com", "https://例え.jp/パス?q=ü", "https://example.com:8443/x",
		"https://[2001:db8::1]/x"}
	bad := []string{"javascript:alert(1)", "JaVaScRiPt:alert(1)", "data:text/html,x", "//evil.com", "/relative",
		"https://", "https://user:pw@example.com", "https://exa mple.com", "vbscript:x", "ftp://example.com",
		"https://example.com/\u00a0x", "https://example.com/\u200bx", "\ufeffhttps://example.com",
		"https://example.com/\u202egpj.exe", "https://example.com/\x7f", "https://example.com/\u0085", "https://example.com/\xff",
		"http://exa\u3164mple.com/", "https://example.com/\u115fpath", "https://example.com/a\u034fb", "https://example.com/\ufe0f",
		"http://evil.com\\.example.com/", "https://example.com:99999/", "https://example.com:0/",
		"https://:80/", "http://[evil.com]/", "http://[]/"}
	for _, u := range ok {
		if !isValidWebURL(u) {
			t.Errorf("%q should be valid", u)
		}
	}
	for _, u := range bad {
		if isValidWebURL(u) {
			t.Errorf("%q must be rejected", u)
		}
	}
}

func TestGenerateSlug(t *testing.T) {
	cases := map[string]string{
		"Catgirl Maid":  "catgirl-maid",
		"  Ümlaut  Été": "umlaut-ete",
		"猫":             "",
		"a--b":          "a--b",
	}
	for in, want := range cases {
		if got := generateSlug(in); got != want {
			t.Errorf("generateSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLikePatternEscapesWildcards(t *testing.T) {
	if got := likePattern(`50%_off\`); got != `%50\%\_off\\%` {
		t.Fatalf("unexpected pattern %q", got)
	}
}

func TestHashRoundTripMatchesLegacyEncoding(t *testing.T) {
	for _, h := range []uint64{0, 1, 0xdeadbeefcafebabe, ^uint64(0)} {
		if got := bitsToHex(hashToBits(h)); got != hexOf(h) {
			t.Fatalf("round trip mismatch for %x: %s", h, got)
		}
	}
}

func hexOf(h uint64) string {
	const digits = "0123456789abcdef"
	b := make([]byte, 16)
	for i := 15; i >= 0; i-- {
		b[i] = digits[h&0xf]
		h >>= 4
	}
	return string(b)
}

func TestLimiterStoreReusesRefilledBuckets(t *testing.T) {
	ls := newLimiterStore("t", 60, 2, 10)
	now := time.Now()
	for i := 0; i < 10; i++ {
		ls.take(fmt.Sprintf("k%d", i), now)
	}
	later := now.Add(5 * time.Second)
	for i := 0; i < 10; i++ {
		if !ls.take(fmt.Sprintf("new%d", i), later).allowed {
			t.Fatalf("new client %d should be allowed", i)
		}
	}
	if ls.size() != 10 {
		t.Fatalf("store should stay at its cap, has %d", ls.size())
	}
	for i := 0; i < 10; i++ {
		if _, ok := ls.entries[fmt.Sprintf("new%d", i)]; !ok {
			t.Fatalf("new client %d should have its own bucket", i)
		}
	}
}

func TestLimiterStoreFullOfActiveClientsUsesOverflow(t *testing.T) {
	ls := newLimiterStore("t", 60, 1, 10)
	now := time.Now()
	for i := 0; i < 10; i++ {
		ls.take(fmt.Sprintf("k%d", i), now)
	}
	ls.take("x", now)
	if _, ok := ls.entries["x"]; ok {
		t.Fatal("no slot should have been freed while every client is actively limited")
	}
	sweptAt := ls.lastSweep
	ls.take("y", now)
	if !ls.lastSweep.Equal(sweptAt) {
		t.Fatal("full sweeps must run at most once per second")
	}
}

func TestLimiterStoreGroupsIPv6UnderPressure(t *testing.T) {
	ls := newLimiterStore("t", 60, 1, 20)
	now := time.Now()
	for i := 0; i < 18; i++ {
		ls.take(fmt.Sprintf("v4-%d", i), now)
	}
	a := netip.MustParseAddr("2001:db8:1:2::1")
	b := netip.MustParseAddr("2001:db8:1:3::1")
	if !ls.takeAddr(a, now).allowed {
		t.Fatal("first request should pass")
	}
	if ls.takeAddr(b, now).allowed {
		t.Fatal("under pressure, /64 networks of the same /48 share one bucket")
	}
	if _, ok := ls.entries["2001:db8:1::/48"]; !ok {
		t.Fatalf("expected a /48 bucket, have %v", ls.entries)
	}
}

func TestObjectJanitorRetriesFailedDeletes(t *testing.T) {
	store := newMemStore()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, k := range []string{"1.png", "2.png", "3.png"} {
		if err := store.Upload(ctx, k, strings.NewReader("x"), 1, "image/png"); err != nil {
			t.Fatal(err)
		}
	}
	j := newObjectJanitor(store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	j.backoff = []time.Duration{time.Millisecond, 5 * time.Millisecond, 5 * time.Millisecond}
	var purgedMu sync.Mutex
	var purged []string
	j.deleted = func(key string) {
		purgedMu.Lock()
		purged = append(purged, key)
		purgedMu.Unlock()
	}
	done := make(chan struct{})
	go func() {
		j.run(ctx)
		close(done)
	}()

	store.mu.Lock()
	store.failDeletes = 3
	store.mu.Unlock()
	j.remove(ctx, "1.png")
	j.remove(ctx, "2.png")

	deadline := time.Now().Add(5 * time.Second)
	for {
		_, ok1 := store.get("1.png")
		_, ok2 := store.get("2.png")
		if !ok1 && !ok2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("failed deletes were not retried")
		}
		time.Sleep(2 * time.Millisecond)
	}
	if _, ok := store.get("3.png"); !ok {
		t.Fatal("unrelated file deleted")
	}
	var gotPurged []string
	for wait := time.Now().Add(5 * time.Second); ; {
		purgedMu.Lock()
		gotPurged = append(gotPurged[:0], purged...)
		purgedMu.Unlock()
		if len(gotPurged) >= 2 || time.Now().After(wait) {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	sort.Strings(gotPurged)
	if strings.Join(gotPurged, ",") != "1.png,2.png" {
		t.Fatalf("the CDN must be purged once per successful delete, got %v", gotPurged)
	}

	store.mu.Lock()
	store.failDeletes = 1000
	store.mu.Unlock()
	j.remove(ctx, "3.png")
	time.Sleep(50 * time.Millisecond)
	store.mu.Lock()
	remaining := store.failDeletes
	store.mu.Unlock()
	if attempts := 1000 - remaining; attempts != 1+len(j.backoff) {
		t.Fatalf("expected %d delete attempts, got %d", 1+len(j.backoff), attempts)
	}
	purgedMu.Lock()
	if len(purged) != 2 {
		t.Fatalf("a delete that never succeeded must not purge the CDN, got %v", purged)
	}
	purgedMu.Unlock()
	cancel()
	<-done
}

func TestCloudflareHintIsLoggedOnce(t *testing.T) {
	var buf bytes.Buffer
	ir := newIPResolver(mustPrefixes(t, "10.0.0.0/8"), "")
	ir.log = slog.New(slog.NewTextHandler(&buf, nil))
	for i := 0; i < 3; i++ {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "10.0.0.2:1234"
		r.Header.Set("CF-Connecting-IP", "198.51.100.7")
		ir.resolve(r)
	}
	if n := strings.Count(buf.String(), "level=WARN"); n != 1 {
		t.Fatalf("expected exactly one hint, got %d: %s", n, buf.String())
	}

	buf.Reset()
	ir = newIPResolver(mustPrefixes(t, "10.0.0.0/8"), "CF-Connecting-IP")
	ir.log = slog.New(slog.NewTextHandler(&buf, nil))
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.2:1234"
	r.Header.Set("CF-Connecting-IP", "198.51.100.7")
	if ip, _ := ir.resolve(r); ip.String() != "198.51.100.7" || buf.Len() != 0 {
		t.Fatalf("unexpected result %s, log %q", ip, buf.String())
	}
}

func TestDimensionConditionClampsPerColumn(t *testing.T) {
	a := &sqlArgs{}
	if c, ok := dimensionCondition(`i."Width"`, ">99999999999", math.MinInt32, math.MaxInt32, a); !ok || c != `i."Width" > $1` {
		t.Fatalf("unexpected condition %q", c)
	}
	if a.args[0] != int64(math.MaxInt32) {
		t.Fatalf("width must be clamped to int4, got %v", a.args[0])
	}
	b := &sqlArgs{}
	if _, ok := dimensionCondition(`i."ByteSize"`, "<=3000000000", math.MinInt64, math.MaxInt64, b); !ok || b.args[0] != int64(3000000000) {
		t.Fatalf("byte sizes are bigint and must not be clamped, got %v", b.args)
	}
	for _, bad := range []string{">=", "1..", "1.5", "0x10", "abc"} {
		if _, ok := dimensionCondition(`i."Width"`, bad, math.MinInt32, math.MaxInt32, &sqlArgs{}); ok {
			t.Errorf("%q must be ignored", bad)
		}
	}
}

func TestParseImageKey(t *testing.T) {
	if id, ext, ok := parseImageKey("123.png"); !ok || id != 123 || ext != ".png" {
		t.Fatalf("got %d %q %v", id, ext, ok)
	}
	for _, bad := range []string{"", ".png", "abc.png", "-1.png", "0.gif", "png"} {
		if _, _, ok := parseImageKey(bad); ok {
			t.Errorf("%q must not parse", bad)
		}
	}
}

func TestKeyedMutex(t *testing.T) {
	var k keyedMutex
	unlock, err := k.lock(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	other, err := k.lock(context.Background(), 8)
	if err != nil {
		t.Fatal(err)
	}
	other()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := k.lock(ctx, 7); err == nil {
		t.Fatal("second lock of the same ID must wait")
	}

	got := make(chan struct{})
	go func() {
		u, err := k.lock(context.Background(), 7)
		if err == nil {
			u()
		}
		close(got)
	}()
	select {
	case <-got:
		t.Fatal("waiter acquired a held lock")
	case <-time.After(30 * time.Millisecond):
	}
	unlock()
	unlock()
	select {
	case <-got:
	case <-time.After(time.Second):
		t.Fatal("waiter not woken")
	}
	if n := k.size(); n != 0 {
		t.Fatalf("lock entries leak: %d", n)
	}
}

func TestObjectJanitorPostponesWhileTheDatabaseIsDown(t *testing.T) {
	store := newMemStore()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = store.Upload(ctx, "5.png", strings.NewReader("x"), 1, "image/png")
	j := newObjectJanitor(store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	j.backoff = []time.Duration{time.Millisecond, 2 * time.Millisecond}
	var guardCalls atomic.Int32
	j.guard = func(context.Context, string) (func(), bool, error) {
		if guardCalls.Add(1) <= 5 {
			return nil, false, errors.New("database unavailable")
		}
		return func() {}, false, nil
	}
	done := make(chan struct{})
	go func() {
		j.run(ctx)
		close(done)
	}()
	store.mu.Lock()
	store.failDeletes = 1
	store.mu.Unlock()
	j.remove(ctx, "5.png")

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, ok := store.get("5.png"); !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("delete was given up during the database outage (guard calls: %d)", guardCalls.Load())
		}
		time.Sleep(2 * time.Millisecond)
	}
	cancel()
	<-done
}

func TestShutdownWaitsForDetachedWork(t *testing.T) {
	s := &Server{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	done := s.beginDetached()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	if err := s.Shutdown(ctx); err == nil {
		t.Fatal("shutdown must report storage work that is still running")
	}
	done()
	done()
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	if n := s.detached.Load(); n != 0 {
		t.Fatalf("counter out of balance: %d", n)
	}
}
