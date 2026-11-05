package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/waifu-im/waifu-api/backend/internal/auth"
	"github.com/waifu-im/waifu-api/backend/internal/config"
	"github.com/waifu-im/waifu-api/backend/internal/db"
	"github.com/waifu-im/waifu-api/backend/internal/redisx"
)

func testRedis(t *testing.T) *redisx.Client {
	t.Helper()
	url := os.Getenv("WAIFU_TEST_REDIS_URL")
	if url == "" {
		t.Skip("WAIFU_TEST_REDIS_URL not set; skipping shared rate limit test")
	}
	c, err := redisx.New(url, redisx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	if err := c.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	return c
}

func uniquePrefix() string {
	return fmt.Sprintf("waifu:test:%d:%d:", os.Getpid(), time.Now().UnixNano())
}

func TestSharedRateLimitAcrossInstances(t *testing.T) {
	rc := testRedis(t)
	prefix := uniquePrefix()
	e := newTestEnv(t, func(c *config.Config) {
		c.RateLimit.RequestsPerMinute = 60
		c.RateLimit.Burst = 4
		c.Cluster.RedisPrefix = prefix
	})
	a := e.peer(WithRedis(rc))
	b := e.peer(WithRedis(rc))
	allowed := 0
	for i := 0; i < 10; i++ {
		inst := a
		if i%2 == 1 {
			inst = b
		}
		w := inst.do("GET", "/banner", nil, withRemote("203.0.113.9:1000"))
		switch w.Code {
		case http.StatusTooManyRequests:
			if w.Header().Get("Retry-After") == "" {
				t.Fatal("429 without Retry-After")
			}
		default:
			allowed++
		}
	}
	if allowed != 4 {
		t.Fatalf("two instances must share one limit: %d of 10 requests allowed, want 4", allowed)
	}
	w := a.do("GET", "/banner", nil, withRemote("203.0.113.10:1000"))
	if w.Code == http.StatusTooManyRequests || w.Header().Get("X-RateLimit-Remaining") != "3" {
		t.Fatalf("another client has its own bucket: %d remaining=%s", w.Code, w.Header().Get("X-RateLimit-Remaining"))
	}
}

func TestRateLimitFallsBackWhenTheSharedStoreIsDown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	rc, err := redisx.New("redis://"+addr, redisx.Options{DialTimeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	e := newTestEnv(t, func(c *config.Config) {
		c.RateLimit.RequestsPerMinute = 60
		c.RateLimit.Burst = 3
		c.Cluster.RedisTimeout = 100 * time.Millisecond
	})
	inst := e.peer(WithRedis(rc))
	start := time.Now()
	allowed := 0
	for i := 0; i < 6; i++ {
		w := inst.do("GET", "/banner", nil, withRemote("203.0.113.20:1000"))
		if w.Code >= 500 {
			t.Fatalf("an unavailable rate limit store must not break requests: %d", w.Code)
		}
		if w.Code != http.StatusTooManyRequests {
			allowed++
		}
	}
	if allowed != 3 {
		t.Fatalf("local limits must still apply: %d allowed", allowed)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("requests must not wait for the unavailable store each time (took %s)", d)
	}
}

func TestImageLockIsClusterWide(t *testing.T) {
	e := newTestEnv(t)
	a, b := e.peer(), e.peer()
	unlock, err := a.srv.lockImage(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := b.srv.lockImage(ctx, 42); err == nil {
		t.Fatal("another instance must wait for the lock")
	}
	other, err := b.srv.lockImage(context.Background(), 43)
	if err != nil {
		t.Fatalf("unrelated images must not wait: %v", err)
	}
	other()
	unlock()
	got, err := b.srv.lockImage(context.Background(), 42)
	if err != nil {
		t.Fatalf("lock not released across instances: %v", err)
	}
	got()
}

func TestLeaderElectionFailsOver(t *testing.T) {
	e := newTestEnv(t)
	pools := make([]*db.Pool, 2)
	for i := range pools {
		p, err := db.Connect(context.Background(), e.dbURL, db.Options{MaxConns: 2, MinConns: 1, KeepAlive: true})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.Close)
		pools[i] = p
	}
	var leaders atomic.Int32
	var maxLeaders atomic.Int32
	var runs [2]atomic.Int32
	clusters := make([]*cluster, 2)
	cancels := make([]context.CancelFunc, 2)
	dones := make([]chan struct{}, 2)
	for i := range clusters {
		c := newCluster(pools[i], e.log)
		c.retryLeader = 30 * time.Millisecond
		c.checkLeader = 20 * time.Millisecond
		clusters[i] = c
		ctx, cancel := context.WithCancel(context.Background())
		cancels[i] = cancel
		dones[i] = make(chan struct{})
		i := i
		go func() {
			defer close(dones[i])
			c.runLeader(ctx, func(lctx context.Context) {
				runs[i].Add(1)
				n := leaders.Add(1)
				for {
					m := maxLeaders.Load()
					if n <= m || maxLeaders.CompareAndSwap(m, n) {
						break
					}
				}
				<-lctx.Done()
				leaders.Add(-1)
			})
		}()
	}
	waitFor(t, func() bool { return leaders.Load() == 1 })
	first := 0
	if clusters[1].isLeader() {
		first = 1
	}
	cancels[first]()
	<-dones[first]
	second := 1 - first
	waitFor(t, func() bool { return clusters[second].isLeader() && runs[second].Load() == 1 })
	if maxLeaders.Load() != 1 {
		t.Fatalf("at most one instance may lead at a time, saw %d", maxLeaders.Load())
	}
	cancels[second]()
	<-dones[second]
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached in time")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestReadyzReportsDraining(t *testing.T) {
	e := newTestEnv(t)
	w := e.do("GET", "/readyz", nil)
	expectStatus(t, w, http.StatusOK)
	e.srv.BeginDrain()
	w = e.do("GET", "/readyz", nil)
	expectStatus(t, w, http.StatusServiceUnavailable)
	expectStatus(t, e.do("GET", "/healthz", nil), http.StatusOK)
}

func TestReplacementOnAnotherInstanceWaitsForUpload(t *testing.T) {
	e := newTestEnv(t)
	tok, uid := e.login("100000000000000048", "mod5", "")
	e.setRole(uid, auth.RoleModerator)
	other := e.peer()

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
			w := other.multipart("PATCH", fmt.Sprintf("/images/%d", id),
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

	w, img := e.upload(tok, 7401, nil)
	expectStatus(t, w, http.StatusCreated)
	if <-finishedEarly {
		t.Fatal("a replacement on another instance finished while the upload was still being stored")
	}
	if code := <-patchDone; code != http.StatusOK {
		t.Fatalf("replacement failed with %d", code)
	}
	e.store.mu.Lock()
	e.store.uploadHook = nil
	e.store.mu.Unlock()
	if _, ok := e.store.get(fmt.Sprintf("%d.png", img.ID)); ok {
		t.Fatal("the original file was left behind")
	}
	if n := e.store.count(); n != 1 {
		t.Fatalf("expected exactly one stored file, got %d", n)
	}
}
