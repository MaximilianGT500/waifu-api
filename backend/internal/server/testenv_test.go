package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"log/slog"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/textproto"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/waifu-im/waifu-api/backend/internal/auth"
	"github.com/waifu-im/waifu-api/backend/internal/config"
	"github.com/waifu-im/waifu-api/backend/internal/db"
)

var (
	templateOnce sync.Once
	templateErr  error
	dbCounter    atomic.Int64
)

const templateDB = "waifu_test_template"

func adminURL(t *testing.T) string {
	t.Helper()
	u := os.Getenv("WAIFU_TEST_DATABASE_URL")
	if u == "" {
		t.Skip("WAIFU_TEST_DATABASE_URL not set; skipping integration test")
	}
	return u
}

func withDatabase(raw, name string) string {
	u, _ := url.Parse(raw)
	u.Path = "/" + name
	return u.String()
}

func ensureTemplate(t *testing.T, admin string) {
	templateOnce.Do(func() {
		ctx := context.Background()
		conn, err := pgx.Connect(ctx, admin)
		if err != nil {
			templateErr = err
			return
		}
		defer conn.Close(ctx)
		_, _ = conn.Exec(ctx, `DROP DATABASE IF EXISTS `+templateDB)
		if _, err := conn.Exec(ctx, `CREATE DATABASE `+templateDB); err != nil {
			templateErr = err
			return
		}
		schema, err := os.ReadFile("testdata/schema.sql")
		if err != nil {
			templateErr = err
			return
		}
		tc, err := pgx.Connect(ctx, withDatabase(admin, templateDB))
		if err != nil {
			templateErr = err
			return
		}
		defer tc.Close(ctx)
		if _, err := tc.Exec(ctx, string(schema)); err != nil {
			templateErr = err
		}
	})
	if templateErr != nil {
		t.Fatalf("preparing template database: %v", templateErr)
	}
}

type memObject struct {
	data        []byte
	contentType string
}

type memStore struct {
	mu             sync.Mutex
	objects        map[string]memObject
	failUploads    bool
	failAfterStore bool
	failDeletes    int
	uploadHook     func(key string)
}

func newMemStore() *memStore { return &memStore{objects: map[string]memObject{}} }

func (m *memStore) Upload(_ context.Context, key string, r io.Reader, _ int64, contentType string) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.mu.Lock()
	if m.failUploads {
		m.mu.Unlock()
		return fmt.Errorf("simulated storage outage")
	}
	m.objects[key] = memObject{data: data, contentType: contentType}
	hook, failAfter := m.uploadHook, m.failAfterStore
	m.mu.Unlock()
	if hook != nil {
		hook(key)
	}
	if failAfter {
		return fmt.Errorf("simulated timeout after the write")
	}
	return nil
}

func (m *memStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failDeletes > 0 {
		m.failDeletes--
		return fmt.Errorf("simulated storage outage")
	}
	delete(m.objects, key)
	return nil
}

func (m *memStore) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.objects)
}

func (m *memStore) Exists(_ context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.objects[key]
	return ok, nil
}

func (m *memStore) PublicURL(key string) string { return "https://cdn.test/" + key }

func (m *memStore) List(_ context.Context, fn func(string) bool) (bool, error) {
	m.mu.Lock()
	keys := make([]string, 0, len(m.objects))
	for k := range m.objects {
		keys = append(keys, k)
	}
	m.mu.Unlock()
	sort.Strings(keys)
	for _, k := range keys {
		if !fn(k) {
			return false, nil
		}
	}
	return true, nil
}

func (m *memStore) get(key string) (memObject, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.objects[key]
	return o, ok
}

type fakeDiscord struct {
	mu    sync.Mutex
	users map[string]*auth.DiscordUser
	calls int
}

func (f *fakeDiscord) ExchangeCode(_ context.Context, code string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if _, ok := f.users[code]; ok {
		return "token-" + code, nil
	}
	return "", &auth.DiscordOAuthError{StatusCode: http.StatusBadRequest}
}

func (f *fakeDiscord) GetUserProfile(_ context.Context, token string) (*auth.DiscordUser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[strings.TrimPrefix(token, "token-")]
	if !ok {
		return nil, auth.ErrDiscordProfile
	}
	cp := *u
	return &cp, nil
}

type testEnv struct {
	t       *testing.T
	cfg     *config.Config
	pool    *db.Pool
	dbURL   string
	srv     *Server
	h       http.Handler
	store   *memStore
	discord *fakeDiscord
	log     *slog.Logger
	nextIP  atomic.Int64
}

func testConfig() *config.Config {
	return &config.Config{
		APIBasePath: "/",
		APIBaseURL:  "https://api.test",
		FrontendURL: "https://app.test",
		AppName:     "Waifu API",
		CDNBaseURL:  "https://cdn.test",
		Environment: "production",
		JWTKey:      "integration-test-signing-key-0123456789",
		JWTIssuer:   "waifu-api",
		JWTAudience: "waifu-api-users",
		JWTLifetime: time.Hour,
		Image: config.ImageConfig{
			MinWidth: 64, MinHeight: 64, MaxWidth: 4000, MaxHeight: 4000,
			AnimatedMinWidth: 32, AnimatedMinHeight: 32, AnimatedMaxWidth: 2000, AnimatedMaxHeight: 2000,
			DefaultPageSize: 1, MaxPageSize: -1,
		},
		Review: config.ReviewConfig{
			RequireImageReview: true, RequireArtistReview: true, RequireTagReview: true, RequireEditRequestReview: true,
			ImageReviewBypassMinRole: "TrustedUser", TagReviewBypassMinRole: "TrustedUser", ArtistReviewBypassMinRole: "TrustedUser",
			ImageEditReviewBypassMinRole: "TrustedUser", TagEditReviewBypassMinRole: "TrustedUser", ArtistEditReviewBypassMinRole: "TrustedUser",
			DefaultPageSize: 30, MaxPageSize: -1, MaxOpenEditsPerUser: 20, TaskRetentionDays: 90,
		},
		Permissions: config.PermissionsConfig{
			TagCreationMinRole: "User", ArtistCreationMinRole: "User", ImageUploadMinRole: "User",
			SubmitEditMinRole: "User", BannerManagementMinRole: "Moderator",
		},
		PageSizes: config.PageSizesConfig{
			TagDefault: 30, TagMax: -1, ArtistDefault: 30, ArtistMax: -1, AlbumDefault: 30, AlbumMax: -1,
			UserDefault: 30, UserMax: -1, ReportDefault: 30, ReportMax: -1,
		},
		RateLimit: config.RateLimitConfig{
			RequestsPerMinute: 100000, Burst: 100000,
			AuthRequestsPerMinute: 100000, AuthBurst: 100000,
			UploadRequestsPerMinute: 100000, UploadBurst: 100000,
			MaxTrackedClients: 1000,
		},
		StorageDiff: config.StorageDiffConfig{DefaultPageSize: 30, MaxPageSize: 100},
		Limits: config.LimitsConfig{
			MaxPageSize: 500, MaxFilterValues: 50, MaxTagsPerImage: 100, MaxArtistsPerImage: 25,
			MaxAPIKeysPerUser: 3, MaxAlbumsPerUser: 5, MaxPendingReportsPerUser: 50,
			ImageProcessingConcurrency: 2, MaxImagePixels: 16_000_000,
		},
		Network: config.NetworkConfig{
			TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")},
			HSTSMaxAge:     31536000,
		},
		MaxUploadMB:    5,
		AllowedOrigins: []string{"https://app.test"},
		Cache:          config.CacheConfig{PublicTTL: time.Minute},
	}
}

func newTestEnv(t *testing.T, mutate ...func(*config.Config)) *testEnv {
	t.Helper()
	admin := adminURL(t)
	ensureTemplate(t, admin)

	ctx := context.Background()
	name := fmt.Sprintf("waifu_test_%d_%d", os.Getpid(), dbCounter.Add(1))
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `CREATE DATABASE `+name+` TEMPLATE `+templateDB); err != nil {
		conn.Close(ctx)
		t.Fatal(err)
	}
	conn.Close(ctx)

	cfg := testConfig()
	for _, m := range mutate {
		m(cfg)
	}
	dbURL := withDatabase(admin, name)
	pool, err := db.Connect(ctx, dbURL, db.Options{MaxConns: 8, StatementTimeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		c, err := pgx.Connect(context.Background(), admin)
		if err == nil {
			_, _ = c.Exec(context.Background(), `DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`)
			c.Close(context.Background())
		}
	})

	store := newMemStore()
	discord := &fakeDiscord{users: map[string]*auth.DiscordUser{}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if os.Getenv("WAIFU_TEST_LOG") != "" {
		log = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	env := &testEnv{t: t, cfg: cfg, pool: pool, dbURL: dbURL, store: store, discord: discord, log: log}
	env.srv = env.newServer()
	env.h = env.srv.Handler()
	return env
}

func (e *testEnv) newServer(opts ...Option) *Server {
	e.t.Helper()
	lockPool, err := db.Connect(context.Background(), e.dbURL, db.Options{MaxConns: 4, MinConns: 1, KeepAlive: true})
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(lockPool.Close)
	srv := New(e.cfg, e.pool, e.store, e.discord, e.log, append([]Option{WithLockPool(lockPool)}, opts...)...)
	srv.RegisterAll()
	return srv
}

func (e *testEnv) peer(opts ...Option) *testEnv {
	e.t.Helper()
	p := &testEnv{t: e.t, cfg: e.cfg, pool: e.pool, dbURL: e.dbURL, store: e.store, discord: e.discord, log: e.log}
	p.srv = p.newServer(opts...)
	p.h = p.srv.Handler()
	p.nextIP.Store(e.nextIP.Load() + 1<<20)
	return p
}

type reqOpt func(*http.Request)

func withToken(tok string) reqOpt {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+tok) }
}

func withAPIKey(key string) reqOpt {
	return func(r *http.Request) { r.Header.Set("X-Api-Key", key) }
}

func withHeader(k, v string) reqOpt {
	return func(r *http.Request) { r.Header.Set(k, v) }
}

func withRemote(addr string) reqOpt {
	return func(r *http.Request) { r.RemoteAddr = addr }
}

func (e *testEnv) request(method, path string, body io.Reader, contentType string, opts ...reqOpt) *httptest.ResponseRecorder {
	e.t.Helper()
	r := httptest.NewRequest(method, path, body)
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	n := e.nextIP.Add(1)
	r.RemoteAddr = fmt.Sprintf("198.18.%d.%d:5555", (n>>8)&0xff, n&0xff)
	for _, o := range opts {
		o(r)
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

func (e *testEnv) do(method, path string, body any, opts ...reqOpt) *httptest.ResponseRecorder {
	e.t.Helper()
	var rd io.Reader
	ct := ""
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatal(err)
		}
		rd = bytes.NewReader(b)
		ct = "application/json"
	}
	return e.request(method, path, rd, ct, opts...)
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("decoding %q: %v", w.Body.String(), err)
	}
	return v
}

func expectStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("expected status %d, got %d: %s", want, w.Code, w.Body.String())
	}
}

func (e *testEnv) login(discordID, name, avatar string) (string, int64) {
	e.t.Helper()
	code := "code" + discordID
	e.discord.mu.Lock()
	e.discord.users[code] = &auth.DiscordUser{ID: discordID, Username: name, Avatar: avatar}
	e.discord.mu.Unlock()
	w := e.do("POST", "/auth/discord", map[string]string{"code": code})
	expectStatus(e.t, w, http.StatusOK)
	tok := decode[map[string]string](e.t, w)["token"]
	claims, err := auth.ValidateJWT(tok, []byte(e.cfg.JWTKey), e.cfg.JWTIssuer, e.cfg.JWTAudience)
	if err != nil {
		e.t.Fatal(err)
	}
	return tok, claims.UserID
}

func (e *testEnv) setRole(userID int64, role auth.Role) {
	e.t.Helper()
	if _, err := e.pool.Exec(context.Background(), `UPDATE "Users" SET "Role" = $1 WHERE "Id" = $2`, int(role), userID); err != nil {
		e.t.Fatal(err)
	}
}

func (e *testEnv) exec(sql string, args ...any) {
	e.t.Helper()
	if _, err := e.pool.Exec(context.Background(), sql, args...); err != nil {
		e.t.Fatal(err)
	}
}

func (e *testEnv) scalar(sql string, args ...any) int64 {
	e.t.Helper()
	var n int64
	if err := e.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func noisePNG(t *testing.T, seed int64, w, h int) []byte {
	t.Helper()
	rnd := rand.New(rand.NewSource(seed))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	block := 8
	for by := 0; by < h; by += block {
		for bx := 0; bx < w; bx += block {
			c := color.RGBA{uint8(rnd.Intn(256)), uint8(rnd.Intn(256)), uint8(rnd.Intn(256)), 255}
			for y := by; y < by+block && y < h; y++ {
				for x := bx; x < bx+block && x < w; x++ {
					img.Set(x, y, c)
				}
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type uploadSpec struct {
	file        []byte
	filename    string
	contentType string
	fields      map[string][]string
}

func (e *testEnv) multipart(method, path string, spec uploadSpec, opts ...reqOpt) *httptest.ResponseRecorder {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if spec.file != nil {
		h := make(textproto.MIMEHeader)
		fn := spec.filename
		if fn == "" {
			fn = "image.png"
		}
		ct := spec.contentType
		if ct == "" {
			ct = "image/png"
		}
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, fn))
		h.Set("Content-Type", ct)
		part, err := mw.CreatePart(h)
		if err != nil {
			e.t.Fatal(err)
		}
		_, _ = part.Write(spec.file)
	}
	for k, vals := range spec.fields {
		for _, v := range vals {
			_ = mw.WriteField(k, v)
		}
	}
	_ = mw.Close()
	return e.request(method, path, &buf, mw.FormDataContentType(), opts...)
}

func (e *testEnv) upload(token string, seed int64, fields map[string][]string) (*httptest.ResponseRecorder, imageDto) {
	e.t.Helper()
	w := e.multipart("POST", "/images/upload", uploadSpec{file: noisePNG(e.t, seed, 160, 200), fields: fields}, withToken(token))
	var dto imageDto
	if w.Code == http.StatusCreated {
		dto = decode[imageDto](e.t, w)
	}
	return w, dto
}

func gifBytes(t *testing.T) []byte {
	t.Helper()
	pal := color.Palette{color.Black, color.White, color.RGBA{255, 0, 0, 255}}
	g := &gif.GIF{}
	for i := 0; i < 3; i++ {
		frame := image.NewPaletted(image.Rect(0, 0, 96, 96), pal)
		for y := 0; y < 96; y++ {
			for x := 0; x < 96; x++ {
				frame.SetColorIndex(x, y, uint8((x/8+y/8+i)%3))
			}
		}
		g.Image = append(g.Image, frame)
		g.Delay = append(g.Delay, 5)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
