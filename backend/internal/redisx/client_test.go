package redisx

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeServer struct {
	ln       net.Listener
	handle   func(cmd []string) string
	conns    atomic.Int32
	mu       sync.Mutex
	accepted []net.Conn
}

func newFakeServer(t *testing.T, handle func(cmd []string) string) *fakeServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fs := &fakeServer{ln: ln, handle: handle}
	go fs.serve()
	t.Cleanup(func() {
		_ = ln.Close()
		fs.dropAll()
	})
	return fs
}

func (fs *fakeServer) url() string { return "redis://" + fs.ln.Addr().String() }

func (fs *fakeServer) dropAll() {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	for _, c := range fs.accepted {
		_ = c.Close()
	}
	fs.accepted = nil
}

func (fs *fakeServer) serve() {
	for {
		c, err := fs.ln.Accept()
		if err != nil {
			return
		}
		fs.conns.Add(1)
		fs.mu.Lock()
		fs.accepted = append(fs.accepted, c)
		fs.mu.Unlock()
		go fs.serveConn(c)
	}
}

func (fs *fakeServer) serveConn(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		n, _ := strconv.Atoi(strings.TrimSpace(line[1:]))
		cmd := make([]string, 0, n)
		for i := 0; i < n; i++ {
			hdr, err := r.ReadString('\n')
			if err != nil {
				return
			}
			size, _ := strconv.Atoi(strings.TrimSpace(hdr[1:]))
			buf := make([]byte, size+2)
			if _, err := r.Read(buf); err != nil {
				return
			}
			cmd = append(cmd, string(buf[:size]))
		}
		resp := fs.handle(cmd)
		if resp == "" {
			return
		}
		if _, err := c.Write([]byte(resp)); err != nil {
			return
		}
	}
}

func TestParseURL(t *testing.T) {
	cases := []struct {
		in         string
		addr, user string
		pass       string
		db         int
		tls        bool
	}{
		{"redis://localhost", "localhost:6379", "", "", 0, false},
		{"redis://:secret@cache:6380/2", "cache:6380", "", "secret", 2, false},
		{"rediss://waifu:pw@cache.example.com", "cache.example.com:6379", "waifu", "pw", 0, true},
		{"valkey://onlypass@cache", "cache:6379", "", "onlypass", 0, false},
	}
	for _, tc := range cases {
		c, err := New(tc.in, Options{})
		if err != nil {
			t.Fatalf("%s: %v", tc.in, err)
		}
		if c.addr != tc.addr || c.username != tc.user || c.password != tc.pass || c.db != tc.db || (c.tls != nil) != tc.tls {
			t.Errorf("%s: got addr=%s user=%q pass=%q db=%d tls=%v", tc.in, c.addr, c.username, c.password, c.db, c.tls != nil)
		}
	}
	for _, bad := range []string{"http://x", "redis://", "redis://h/99", "redis://h/abc", "::"} {
		if _, err := New(bad, Options{}); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

func TestAuthAndCommands(t *testing.T) {
	var authed sync.Map
	fs := newFakeServer(t, func(cmd []string) string {
		switch strings.ToUpper(cmd[0]) {
		case "AUTH":
			if len(cmd) == 3 && cmd[1] == "waifu" && cmd[2] == "pw" {
				authed.Store(true, true)
				return "+OK\r\n"
			}
			return "-WRONGPASS invalid username-password pair\r\n"
		case "SELECT":
			return "+OK\r\n"
		case "PING":
			return "+PONG\r\n"
		case "GET":
			return "$5\r\nvalue\r\n"
		case "MISSING":
			return "$-1\r\n"
		case "BOOM":
			return "-ERR something failed\r\n"
		case "ARR":
			return "*3\r\n:1\r\n$2\r\nok\r\n*-1\r\n"
		}
		return "-ERR unknown\r\n"
	})
	c, err := New("redis://waifu:pw@"+fs.ln.Addr().String()+"/1", Options{PoolSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	if err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	r, err := c.Do(ctx, "GET", "k")
	if err != nil || r.Str != "value" {
		t.Fatalf("GET: %+v %v", r, err)
	}
	r, err = c.Do(ctx, "MISSING")
	if err != nil || !r.Nil {
		t.Fatalf("nil bulk: %+v %v", r, err)
	}
	_, err = c.Do(ctx, "BOOM")
	var se Error
	if !errors.As(err, &se) || !strings.HasPrefix(string(se), "ERR") {
		t.Fatalf("server error expected, got %v", err)
	}
	r, err = c.Do(ctx, "ARR")
	if err != nil || len(r.Array) != 3 || r.Array[0].Int != 1 || r.Array[1].Str != "ok" || !r.Array[2].Nil {
		t.Fatalf("array: %+v %v", r, err)
	}
	if n := fs.conns.Load(); n != 1 {
		t.Fatalf("server errors must not discard the connection, %d connections opened", n)
	}

	bad, err := New("redis://waifu:wrong@"+fs.ln.Addr().String(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Close()
	if err := bad.Ping(ctx); err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("wrong password must fail, got %v", err)
	}
}

func TestOversizedRepliesAreRejected(t *testing.T) {
	fs := newFakeServer(t, func(cmd []string) string {
		switch cmd[0] {
		case "BIG":
			return fmt.Sprintf("$%d\r\n", maxBulkLen+1)
		case "WIDE":
			return fmt.Sprintf("*%d\r\n", maxArrayLen+1)
		case "DEEP":
			return strings.Repeat("*1\r\n", maxReplyNest+2) + ":1\r\n"
		case "MANY":
			inner := "*32\r\n" + strings.Repeat(":1\r\n", 32)
			return "*40\r\n" + strings.Repeat(inner, 40)
		case "HEAVY":
			bulk := fmt.Sprintf("$%d\r\n%s\r\n", maxBulkLen-1, strings.Repeat("x", maxBulkLen-1))
			return "*3\r\n" + strings.Repeat(bulk, 3)
		}
		return "+PONG\r\n"
	})
	c, _ := New(fs.url(), Options{})
	defer c.Close()
	for _, cmd := range []string{"BIG", "WIDE", "DEEP", "MANY", "HEAVY"} {
		if _, err := c.Do(context.Background(), cmd); err == nil {
			t.Errorf("%s: oversized reply must fail", cmd)
		}
	}
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("client must recover after a protocol error: %v", err)
	}
}

func TestBrokenPooledConnectionIsRetried(t *testing.T) {
	fs := newFakeServer(t, func(cmd []string) string { return "+PONG\r\n" })
	c, _ := New(fs.url(), Options{})
	defer c.Close()
	if err := c.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	fs.dropAll()
	time.Sleep(20 * time.Millisecond)
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("a dropped idle connection must be replaced transparently: %v", err)
	}
}

func TestPoolLimitAndTimeout(t *testing.T) {
	release := make(chan struct{})
	fs := newFakeServer(t, func(cmd []string) string {
		if cmd[0] == "SLOW" {
			<-release
		}
		return "+PONG\r\n"
	})
	c, _ := New(fs.url(), Options{PoolSize: 1})
	defer c.Close()
	go func() { _, _ = c.Do(context.Background(), "SLOW") }()
	time.Sleep(50 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := c.Ping(ctx); err == nil {
		t.Fatal("pool exhaustion must respect the context")
	}
	close(release)
}

func TestScriptFallsBackToEval(t *testing.T) {
	var loaded atomic.Bool
	fs := newFakeServer(t, func(cmd []string) string {
		switch cmd[0] {
		case "EVALSHA":
			if !loaded.Load() {
				return "-NOSCRIPT No matching script\r\n"
			}
			return ":2\r\n"
		case "EVAL":
			loaded.Store(true)
			return ":1\r\n"
		}
		return "-ERR\r\n"
	})
	c, _ := New(fs.url(), Options{})
	defer c.Close()
	s := NewScript("return 1")
	r, err := s.Run(context.Background(), c, []string{"k"}, "a")
	if err != nil || r.Int != 1 {
		t.Fatalf("first run: %+v %v", r, err)
	}
	r, err = s.Run(context.Background(), c, []string{"k"}, "a")
	if err != nil || r.Int != 2 {
		t.Fatalf("second run must use EVALSHA: %+v %v", r, err)
	}
}

func TestRealServer(t *testing.T) {
	url := os.Getenv("WAIFU_TEST_REDIS_URL")
	if url == "" {
		t.Skip("WAIFU_TEST_REDIS_URL not set")
	}
	c, err := New(url, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	if err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	s := NewScript("redis.call('SET', KEYS[1], ARGV[1], 'PX', 5000) return redis.call('GET', KEYS[1])")
	r, err := s.Run(ctx, c, []string{"waifu:test:redisx"}, "hello")
	if err != nil || r.Str != "hello" {
		t.Fatalf("script: %+v %v", r, err)
	}
}
