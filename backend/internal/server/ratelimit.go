package server

import (
	"context"
	"math"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const evictionSample = 64

type limiterEntry struct {
	lim  *rate.Limiter
	last time.Time
}

type limiterStore struct {
	name       string
	rpm        int
	rate       rate.Limit
	burst      int
	maxEntries int
	pressure   int
	idle       time.Duration

	mu        sync.Mutex
	entries   map[string]*limiterEntry
	overflow  *rate.Limiter
	lastSweep time.Time
}

func newLimiterStore(name string, rpm, burst, maxEntries int) *limiterStore {
	if rpm <= 0 {
		rpm = 240
	}
	if burst <= 0 {
		burst = 1
	}
	if maxEntries <= 0 {
		maxEntries = 100000
	}
	r := rate.Limit(float64(rpm) / 60.0)
	return &limiterStore{
		name:       name,
		rpm:        rpm,
		rate:       r,
		burst:      burst,
		maxEntries: maxEntries,
		pressure:   maxEntries * 9 / 10,
		idle:       10 * time.Minute,
		entries:    make(map[string]*limiterEntry),
		overflow:   rate.NewLimiter(r*10, burst*10),
	}
}

type limitResult struct {
	allowed    bool
	remaining  int
	retryAfter time.Duration
}

func (ls *limiterStore) takeAddr(a netip.Addr, now time.Time) limitResult {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	key := rateLimitKey(a)
	if _, ok := ls.entries[key]; !ok && a.Is6() && len(ls.entries) >= ls.pressure {
		key = rateLimitKeyPrefix(a, 48)
	}
	return ls.takeLocked(key, now)
}

func (ls *limiterStore) take(key string, now time.Time) limitResult {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.takeLocked(key, now)
}

func (ls *limiterStore) takeLocked(key string, now time.Time) limitResult {
	e, ok := ls.entries[key]
	var lim *rate.Limiter
	switch {
	case ok:
		e.last = now
		lim = e.lim
	case len(ls.entries) < ls.maxEntries || ls.makeRoomLocked(now):
		e = &limiterEntry{lim: rate.NewLimiter(ls.rate, ls.burst), last: now}
		ls.entries[key] = e
		lim = e.lim
	default:
		lim = ls.overflow
	}

	if lim.AllowN(now, 1) {
		rem := int(math.Floor(lim.TokensAt(now)))
		if rem < 0 {
			rem = 0
		}
		return limitResult{allowed: true, remaining: rem}
	}
	res := lim.ReserveN(now, 1)
	delay := res.DelayFrom(now)
	res.CancelAt(now)
	if delay <= 0 || delay == rate.InfDuration {
		delay = time.Second
	}
	return limitResult{allowed: false, retryAfter: delay}
}

func (ls *limiterStore) reusable(e *limiterEntry, now time.Time) bool {
	return now.Sub(e.last) > ls.idle || e.lim.TokensAt(now) >= float64(ls.burst)
}

func (ls *limiterStore) makeRoomLocked(now time.Time) bool {
	n := 0
	for k, e := range ls.entries {
		if ls.reusable(e, now) {
			delete(ls.entries, k)
			return true
		}
		if n++; n >= evictionSample {
			break
		}
	}
	if now.Sub(ls.lastSweep) >= time.Second {
		ls.sweepLocked(now)
		return len(ls.entries) < ls.maxEntries
	}
	return false
}

func (ls *limiterStore) sweepLocked(now time.Time) {
	ls.lastSweep = now
	for k, e := range ls.entries {
		if ls.reusable(e, now) {
			delete(ls.entries, k)
		}
	}
}

func (ls *limiterStore) sweep(now time.Time) {
	ls.mu.Lock()
	ls.sweepLocked(now)
	ls.mu.Unlock()
}

func (ls *limiterStore) size() int {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return len(ls.entries)
}

func (ls *limiterStore) janitor(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			ls.sweep(now)
		}
	}
}

func writeRateLimited(w http.ResponseWriter, r *http.Request, res limitResult) {
	secs := int(math.Ceil(res.retryAfter.Seconds()))
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	WriteProblem(w, r, http.StatusTooManyRequests,
		"https://tools.ietf.org/html/rfc6585#section-4",
		"Too Many Requests",
		"Rate limit exceeded. Please retry later.")
}

func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	l := s.globalLimiter
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st := stateFrom(r.Context())
		res := l.take(r.Context(), st.clientIP)
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(l.limit()))
		if !res.allowed {
			w.Header().Set("X-RateLimit-Remaining", "0")
			writeRateLimited(w, r, res)
			return
		}
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(res.remaining))
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withLimit(l *rateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st := stateFrom(r.Context())
		res := l.take(r.Context(), st.clientIP)
		if !res.allowed {
			writeRateLimited(w, r, res)
			return
		}
		next.ServeHTTP(w, r)
	})
}
