package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"hash/crc64"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/waifu-im/waifu-api/backend/internal/config"
	"github.com/waifu-im/waifu-api/backend/internal/db"
	"github.com/waifu-im/waifu-api/backend/internal/redisx"
)

type Server struct {
	cfg     *config.Config
	db      *db.Pool
	s3      ObjectStore
	discord DiscordAuthenticator
	log     *slog.Logger
	mux     *http.ServeMux
	base    string

	ipResolver    *ipResolver
	globalLimiter *rateLimiter
	authLimiter   *rateLimiter
	uploadLimiter *rateLimiter
	stats         *statsCollector
	cdn           *cdnPurger
	objects       *objectJanitor
	cluster       *cluster
	redis         *redisx.Client
	instanceID    string
	draining      atomic.Bool
	imageSlots    chan struct{}

	publicStats publicStatsCache
	openapi     openAPICache

	versions    dataVersion
	versionSync versionSync
	imageLists  *publicCache[*imageList]
	usageCounts *publicCache[*usageSnapshot]
	rankedLists *publicCache[*rankedIDs]
	colorIndex  *publicCache[*colorSnapshot]
	facets      *publicCache[*facetResult]
	similar     *publicCache[*rankedIDs]
	favCounts   *favoriteCounts

	bgCancel context.CancelFunc
	bgWG     sync.WaitGroup
	detached atomic.Int64
}

type Option func(*Server)

func WithRedis(c *redisx.Client) Option { return func(s *Server) { s.redis = c } }

func WithLockPool(p *db.Pool) Option { return func(s *Server) { s.cluster.locks = p } }

func WithInstanceID(id string) Option { return func(s *Server) { s.instanceID = id } }

func New(cfg *config.Config, pool *db.Pool, store ObjectStore, discord DiscordAuthenticator, log *slog.Logger, opts ...Option) *Server {
	s := &Server{
		cfg:     cfg,
		db:      pool,
		s3:      store,
		discord: discord,
		log:     log,
		mux:     http.NewServeMux(),
		cluster: newCluster(nil, log),
	}
	for _, opt := range opts {
		opt(s)
	}
	s.base = strings.Trim(cfg.APIBasePath, "/")
	s.ipResolver = newIPResolver(cfg.Network.TrustedProxies, cfg.Network.RealIPHeader)
	s.ipResolver.log = log
	rl := cfg.RateLimit
	prefix, timeout := cfg.Cluster.RedisPrefix, cfg.Cluster.RedisTimeout
	s.globalLimiter = newRateLimiter("global", rl.RequestsPerMinute, rl.Burst, rl.MaxTrackedClients, s.redis, prefix, timeout, log)
	s.authLimiter = newRateLimiter("auth", rl.AuthRequestsPerMinute, rl.AuthBurst, rl.MaxTrackedClients, s.redis, prefix, timeout, log)
	s.uploadLimiter = newRateLimiter("upload", rl.UploadRequestsPerMinute, rl.UploadBurst, rl.MaxTrackedClients, s.redis, prefix, timeout, log)
	s.stats = newStatsCollector(pool, log)
	s.cdn = newCDNPurger(cfg, store, log)
	s.objects = newObjectJanitor(store, log)
	s.objects.guard = s.guardStoredObject
	s.objects.deleted = func(key string) { s.cdn.purge([]string{key}) }
	slots := cfg.Limits.ImageProcessingConcurrency
	if slots <= 0 {
		slots = 2
	}
	s.imageSlots = make(chan struct{}, slots)
	ttl := cfg.Cache.PublicTTL
	s.imageLists = newPublicCache[*imageList](ttl, scopeContent, &s.versions, imageListMaxEntries, imageListMaxIDs)
	s.usageCounts = newPublicCache[*usageSnapshot](ttl, scopeContent, &s.versions, 4, 0)
	s.rankedLists = newPublicCache[*rankedIDs](ttl, scopeContent, &s.versions, 64, 4_000_000)
	s.colorIndex = newPublicCache[*colorSnapshot](ttl, scopeContent, &s.versions, 1, 0)
	s.facets = newPublicCache[*facetResult](ttl, scopeContent, &s.versions, 256, 0)
	s.similar = newPublicCache[*rankedIDs](ttl, scopeContent, &s.versions, 2048, 0)
	s.favCounts = newFavoriteCounts(ttl, &s.versions, favoriteCountsMaxItems)
	return s
}

func (s *Server) Config() *config.Config { return s.cfg }

func (s *Server) DB() *db.Pool { return s.db }

func (s *Server) Log() *slog.Logger { return s.log }

func (s *Server) Handle(pattern string, h http.Handler) {
	if s.base != "" {
		if idx := strings.Index(pattern, " "); idx >= 0 {
			pattern = pattern[:idx+1] + "/" + s.base + pattern[idx+1:]
		} else {
			pattern = "/" + s.base + pattern
		}
	}
	s.mux.Handle(pattern, h)
}

func (s *Server) RegisterAll() {
	s.RegisterAuthRoutes()
	s.RegisterHealthRoutes()
	s.RegisterOpenAPIRoutes()
	s.RegisterImageRoutes()
	s.RegisterDiscoveryRoutes()
	s.RegisterAccountRoutes()
	s.RegisterTagRoutes()
	s.RegisterArtistRoutes()
	s.RegisterUserRoutes()
	s.RegisterAlbumRoutes()
	s.RegisterReviewRoutes()
	s.RegisterReportRoutes()
	s.RegisterBannerRoutes()
	s.RegisterStatsRoutes()
	s.RegisterStorageDiffRoutes()
}

func (s *Server) Handler() http.Handler {
	notFound := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		RouteNotFound(w, r)
	})
	s.mux.Handle("/", notFound)

	var h http.Handler = s.mux
	h = s.invalidationMiddleware(h)
	h = s.authMiddleware(h)
	h = s.rateLimitMiddleware(h)
	h = s.corsMiddleware(h)
	h = s.loggingMiddleware(h)
	h = s.securityHeaders(h)
	h = s.baseMiddleware(h)
	h = s.recoveryMiddleware(h)
	return h
}

var jsonBuffers = sync.Pool{New: func() any { return new(bytes.Buffer) }}

func encodeJSON(v any) *bytes.Buffer {
	buf := jsonBuffers.Get().(*bytes.Buffer)
	buf.Reset()
	_ = json.NewEncoder(buf).Encode(v)
	return buf
}

func releaseJSON(buf *bytes.Buffer) {
	if buf.Cap() <= 4<<20 {
		jsonBuffers.Put(buf)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	buf := encodeJSON(v)
	defer releaseJSON(buf)
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Content-Length", strconv.Itoa(buf.Len()))
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

var etagTable = crc64.MakeTable(crc64.ECMA)

func etagFor(body []byte) string {
	return `"` + strconv.FormatUint(crc64.Checksum(body, etagTable), 36) + "-" + strconv.FormatInt(int64(len(body)), 36) + `"`
}

func etagMatches(header, etag string) bool {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "*" || strings.TrimPrefix(part, "W/") == etag {
			return true
		}
	}
	return false
}

func writePublicJSON(w http.ResponseWriter, r *http.Request, v any) {
	if IdentityFromContext(r.Context()) != nil || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		writeJSON(w, http.StatusOK, v)
		return
	}
	buf := encodeJSON(v)
	defer releaseJSON(buf)
	h := w.Header()
	etag := etagFor(buf.Bytes())
	h.Set("ETag", etag)
	h.Add("Vary", "Authorization")
	h.Add("Vary", "X-Api-Key")
	if h.Get("Cache-Control") == "" {
		h.Set("Cache-Control", "no-cache")
	}
	if inm := r.Header.Get("If-None-Match"); inm != "" && etagMatches(inm, etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Type", "application/json")
	h.Set("Content-Length", strconv.Itoa(buf.Len()))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

func decodeJSON(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 1<<10))
	return nil
}

func (s *Server) StartBackgroundJobs() {
	ctx, cancel := context.WithCancel(context.Background())
	s.bgCancel = cancel

	s.goBackground(func() { s.stats.run(ctx, 10*time.Second) })
	s.goBackground(func() { s.cdn.run(ctx) })
	s.goBackground(func() { s.objects.run(ctx) })
	for _, l := range []*rateLimiter{s.globalLimiter, s.authLimiter, s.uploadLimiter} {
		ls := l.local
		s.goBackground(func() { ls.janitor(ctx) })
	}
	s.goBackground(func() { s.cluster.runLeader(ctx, s.runScheduledJobs) })
	if s.versionSyncEnabled() {
		interval := min(time.Second, s.cfg.Cache.PublicTTL)
		s.goBackground(func() { s.runVersionSync(ctx, interval) })
	}
}

func (s *Server) goBackground(fn func()) {
	s.bgWG.Add(1)
	go func() {
		defer s.bgWG.Done()
		fn()
	}()
}

type scheduledJob struct {
	name string
	run  func(context.Context)
}

func (s *Server) runScheduledJobs(ctx context.Context) {
	jobs := []scheduledJob{{"stats-cleanup", s.cleanupStats}, {"review-task-cleanup", s.cleanupReviewTasks}}
	for {
		for _, job := range jobs {
			if ctx.Err() != nil {
				return
			}
			jobCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						s.log.Error("background job panicked", "job", job.name, "panic", rec)
					}
				}()
				job.run(jobCtx)
			}()
			cancel()
		}
		now := time.Now().UTC()
		next := time.Date(now.Year(), now.Month(), now.Day(), 4, 0, 0, 0, time.UTC)
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		s.log.Info("scheduled jobs done", "next", next)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *Server) BeginDrain() { s.draining.Store(true) }

func (s *Server) Shutdown(ctx context.Context) error {
	detachedCtx := ctx
	if deadline, ok := ctx.Deadline(); ok {
		var cancel context.CancelFunc
		detachedCtx, cancel = context.WithDeadline(ctx, time.Now().Add(time.Until(deadline)*2/3))
		defer cancel()
	}
	detachedErr := s.waitDetached(detachedCtx)
	if detachedErr != nil {
		s.log.Warn("uploads or file replacements still running at shutdown")
	}
	if s.bgCancel != nil {
		s.bgCancel()
	}
	if err := waitGroupCtx(ctx, &s.bgWG); err != nil {
		return errors.New("background jobs did not stop in time")
	}
	return detachedErr
}

func waitGroupCtx(ctx context.Context, wg *sync.WaitGroup) error {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) beginDetached() func() {
	s.detached.Add(1)
	var once sync.Once
	return func() { once.Do(func() { s.detached.Add(-1) }) }
}

func (s *Server) waitDetached(ctx context.Context) error {
	t := time.NewTicker(25 * time.Millisecond)
	defer t.Stop()
	for s.detached.Load() > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
	return nil
}

func (s *Server) acquireImageSlot(ctx context.Context) (func(), error) {
	select {
	case s.imageSlots <- struct{}{}:
		return func() { <-s.imageSlots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *Server) guardStoredObject(ctx context.Context, key string) (func(), bool, error) {
	id, ext, ok := parseImageKey(key)
	if !ok {
		return func() {}, false, nil
	}
	unlock, err := s.lockImage(ctx, id)
	if err != nil {
		return nil, false, err
	}
	var inUse bool
	err = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "Images" WHERE "Id" = $1 AND "Extension" = $2)`, id, ext).Scan(&inUse)
	if err != nil {
		unlock()
		return nil, false, err
	}
	return unlock, inUse, nil
}

func (s *Server) lockImage(ctx context.Context, id int64) (func(), error) {
	return s.cluster.lockImage(ctx, id)
}
