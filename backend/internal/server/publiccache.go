package server

import (
	"container/list"
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type cacheScope uint8

const (
	scopeContent cacheScope = 1 << iota
	scopeFavorites
)

type dataVersion struct {
	content   atomic.Uint64
	favorites atomic.Uint64
}

type versionStamp struct {
	content, favorites uint64
}

func (v *dataVersion) bump(scopes cacheScope) {
	if scopes&scopeContent != 0 {
		v.content.Add(1)
	}
	if scopes&scopeFavorites != 0 {
		v.favorites.Add(1)
	}
}

func (v *dataVersion) stamp(scopes cacheScope) versionStamp {
	var s versionStamp
	if scopes&scopeContent != 0 {
		s.content = v.content.Load()
	}
	if scopes&scopeFavorites != 0 {
		s.favorites = v.favorites.Load()
	}
	return s
}

type cacheEntry[V any] struct {
	key     string
	val     V
	stamp   versionStamp
	expires time.Time
	cost    int
	elem    *list.Element
}

type cacheFlight[V any] struct {
	done chan struct{}
	val  V
	err  error
}

type flightKey struct {
	key   string
	stamp versionStamp
}

type publicCache[V any] struct {
	ttl      time.Duration
	scopes   cacheScope
	versions *dataVersion
	maxCost  int
	maxItems int

	mu       sync.Mutex
	items    map[string]*cacheEntry[V]
	lru      *list.List
	cost     int
	inflight map[flightKey]*cacheFlight[V]
}

func newPublicCache[V any](ttl time.Duration, scopes cacheScope, versions *dataVersion, maxItems, maxCost int) *publicCache[V] {
	return &publicCache[V]{
		ttl:      ttl,
		scopes:   scopes,
		versions: versions,
		maxCost:  maxCost,
		maxItems: maxItems,
		items:    map[string]*cacheEntry[V]{},
		lru:      list.New(),
		inflight: map[flightKey]*cacheFlight[V]{},
	}
}

func (c *publicCache[V]) enabled() bool { return c != nil && c.ttl > 0 }

func (c *publicCache[V]) get(key string, scopes cacheScope) (V, bool) {
	var zero V
	if !c.enabled() {
		return zero, false
	}
	stamp := c.versions.stamp(c.scopes | scopes)
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[key]
	if !ok {
		return zero, false
	}
	if e.stamp != stamp || !time.Now().Before(e.expires) {
		c.removeLocked(e)
		return zero, false
	}
	c.lru.MoveToFront(e.elem)
	return e.val, true
}

func (c *publicCache[V]) load(ctx context.Context, key string, scopes cacheScope, build func(context.Context) (V, int, error)) (V, error) {
	if v, ok := c.get(key, scopes); ok {
		return v, nil
	}
	all := c.scopes | scopes
	started := time.Now()
	stamp := c.versions.stamp(all)
	fk := flightKey{key: key, stamp: stamp}
	c.mu.Lock()
	if f, ok := c.inflight[fk]; ok {
		c.mu.Unlock()
		select {
		case <-f.done:
			return f.val, f.err
		case <-ctx.Done():
			var zero V
			return zero, ctx.Err()
		}
	}
	f := &cacheFlight[V]{done: make(chan struct{})}
	c.inflight[fk] = f
	c.mu.Unlock()

	val, cost, err := build(context.WithoutCancel(ctx))
	f.val, f.err = val, err

	c.mu.Lock()
	delete(c.inflight, fk)
	if err == nil && c.versions.stamp(all) == stamp {
		c.storeLocked(key, val, stamp, cost, started.Add(c.ttl))
	}
	c.mu.Unlock()
	close(f.done)
	return val, err
}

func (c *publicCache[V]) invalidate(key string) {
	if !c.enabled() {
		return
	}
	c.mu.Lock()
	if e, ok := c.items[key]; ok {
		c.removeLocked(e)
	}
	c.mu.Unlock()
}

func (c *publicCache[V]) storeLocked(key string, val V, stamp versionStamp, cost int, expires time.Time) {
	if c.maxCost > 0 && cost > c.maxCost {
		return
	}
	if old, ok := c.items[key]; ok {
		c.removeLocked(old)
	}
	e := &cacheEntry[V]{key: key, val: val, stamp: stamp, expires: expires, cost: cost}
	e.elem = c.lru.PushFront(e)
	c.items[key] = e
	c.cost += cost
	for (c.maxItems > 0 && len(c.items) > c.maxItems) || (c.maxCost > 0 && c.cost > c.maxCost) {
		back := c.lru.Back()
		if back == nil {
			break
		}
		c.removeLocked(back.Value.(*cacheEntry[V]))
	}
}

func (c *publicCache[V]) removeLocked(e *cacheEntry[V]) {
	c.lru.Remove(e.elem)
	delete(c.items, e.key)
	c.cost -= e.cost
}

func (c *publicCache[V]) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

type usageSnapshot struct {
	ids    []int64
	counts []int64
}

func (s *Server) loadUsage(ctx context.Context, key, query string) (*usageSnapshot, error) {
	if !s.usageCounts.enabled() {
		return nil, nil
	}
	return s.usageCounts.load(ctx, key, 0, func(ctx context.Context) (*usageSnapshot, int, error) {
		rows, err := s.db.Query(ctx, query)
		if err != nil {
			return nil, 0, err
		}
		defer rows.Close()
		u := &usageSnapshot{ids: []int64{}, counts: []int64{}}
		for rows.Next() {
			var id, n int64
			if err := rows.Scan(&id, &n); err != nil {
				return nil, 0, err
			}
			u.ids = append(u.ids, id)
			u.counts = append(u.counts, n)
		}
		return u, 1, rows.Err()
	})
}

func usageJoin(alias, target, idsParam, countsParam string) string {
	return ` LEFT JOIN unnest(` + idsParam + `::bigint[], ` + countsParam + `::bigint[]) AS ` + alias + `(tid, cnt) ON ` + alias + `.tid = ` + target + `."Id"`
}

func (s *Server) writeScopes(r *http.Request) cacheScope {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return 0
	}
	path := r.URL.Path
	if s.base != "" {
		path = strings.TrimPrefix(path, "/"+s.base)
	}
	switch {
	case strings.HasPrefix(path, "/auth/"), strings.HasPrefix(path, "/banner"):
		return 0
	case strings.HasPrefix(path, "/users/") && strings.Contains(path, "/albums"):
		return scopeFavorites
	default:
		return scopeContent | scopeFavorites
	}
}

type invalidatingWriter struct {
	http.ResponseWriter
	once   sync.Once
	status int
	notify func()
}

func clientError(code int) bool { return code >= 400 && code < 500 }

func (iw *invalidatingWriter) WriteHeader(code int) {
	if iw.status == 0 {
		iw.status = code
	}
	if !clientError(code) {
		iw.once.Do(iw.notify)
	}
	iw.ResponseWriter.WriteHeader(code)
}

func (iw *invalidatingWriter) Write(b []byte) (int, error) {
	if iw.status == 0 {
		iw.status = http.StatusOK
	}
	if !clientError(iw.status) {
		iw.once.Do(iw.notify)
	}
	return iw.ResponseWriter.Write(b)
}

func (iw *invalidatingWriter) Unwrap() http.ResponseWriter { return iw.ResponseWriter }

func (s *Server) invalidationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scopes := s.writeScopes(r)
		if scopes == 0 {
			next.ServeHTTP(w, r)
			return
		}
		iw := &invalidatingWriter{ResponseWriter: w, notify: func() { s.versions.bump(scopes) }}
		defer func() {
			if clientError(iw.status) {
				return
			}
			s.versions.bump(scopes)
			s.publishVersions(r.Context(), scopes)
		}()
		next.ServeHTTP(iw, r)
	})
}

var syncedScopes = []cacheScope{scopeContent, scopeFavorites}

type versionSync struct {
	mu   sync.Mutex
	seen map[cacheScope]string
	seq  atomic.Uint64
}

func (v *versionSync) swap(scope cacheScope, token string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.seen == nil {
		v.seen = map[cacheScope]string{}
	}
	changed := v.seen[scope] != token
	v.seen[scope] = token
	return changed
}

func (s *Server) versionKey(scope cacheScope) string {
	name := "content"
	if scope == scopeFavorites {
		name = "favorites"
	}
	return s.cfg.Cluster.RedisPrefix + "cache:" + name
}

func (s *Server) redisTimeout() time.Duration {
	if t := s.cfg.Cluster.RedisTimeout; t > 0 {
		return t
	}
	return 150 * time.Millisecond
}

func (s *Server) versionSyncEnabled() bool {
	return s.redis != nil && (s.imageLists.enabled() || s.usageCounts.enabled())
}

func (s *Server) publishVersions(ctx context.Context, scopes cacheScope) {
	if !s.versionSyncEnabled() {
		return
	}
	token := s.instanceID + "-" + strconv.FormatInt(time.Now().UnixNano(), 36) + "-" + strconv.FormatUint(s.versionSync.seq.Add(1), 36)
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.redisTimeout())
	defer cancel()
	for _, scope := range syncedScopes {
		if scopes&scope == 0 {
			continue
		}
		if _, err := s.redis.Do(ctx, "SET", s.versionKey(scope), token, "PX", "86400000"); err != nil {
			s.log.Debug("cache version publish failed", "err", err)
			return
		}
		s.versionSync.swap(scope, token)
	}
}

func (s *Server) pollVersions(ctx context.Context) {
	for _, scope := range syncedScopes {
		rctx, cancel := context.WithTimeout(ctx, s.redisTimeout())
		reply, err := s.redis.Do(rctx, "GET", s.versionKey(scope))
		cancel()
		if err != nil {
			s.log.Debug("cache version poll failed", "err", err)
			return
		}
		if s.versionSync.swap(scope, reply.Str) {
			s.versions.bump(scope)
		}
	}
}

func (s *Server) runVersionSync(ctx context.Context, interval time.Duration) {
	s.pollVersions(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pollVersions(ctx)
		}
	}
}
