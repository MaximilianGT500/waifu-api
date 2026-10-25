package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

func (s *Server) withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			var buf [16]byte
			_, _ = rand.Read(buf[:])
			id = hex.EncodeToString(buf[:])
		}
		w.Header().Set("X-Request-Id", id)
		ctx := withRequestID(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")

		if !strings.HasPrefix(r.URL.Path, "/scalar") {
			h.Set("X-Frame-Options", "SAMEORIGIN")
		}
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	allowed := s.cfg.AllowedOrigins
	allowAll := s.cfg.CORSAllowAll
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			ok := allowAll
			if !ok {
				for _, o := range allowed {
					if strings.EqualFold(o, origin) {
						ok = true
						break
					}
				}
			}
			if ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, X-Api-Key, Content-Type")
				w.Header().Set("Access-Control-Max-Age", "600")
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic while handling request",
					"panic", rec,
					"path", r.URL.Path,
					"traceId", RequestID(r))
				InternalError(w, r, nil, nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type ipLimiter struct {
	mu       sync.Mutex
	lastSeen time.Time
	limiter  *rate.Limiter
}

type rateLimiter struct {
	mu      sync.Mutex
	clients map[string]*ipLimiter
	rate    rate.Limit
	burst   int
}

func newRateLimiter(rpm, burst int) *rateLimiter {
	if rpm <= 0 {
		rpm = 240
	}
	if burst <= 0 {
		burst = 40
	}
	return &rateLimiter{
		clients: make(map[string]*ipLimiter),
		rate:    rate.Limit(float64(rpm) / 60.0),
		burst:   burst,
	}
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	c, ok := rl.clients[ip]
	if !ok || now.Sub(c.lastSeen) > 10*time.Minute {
		rl.clients[ip] = &ipLimiter{lastSeen: now, limiter: rate.NewLimiter(rl.rate, rl.burst)}
		c = rl.clients[ip]
	}
	c.lastSeen = now
	return c.limiter.Allow()
}

func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	rl := newRateLimiter(s.cfg.RateLimit.RequestsPerMinute, s.cfg.RateLimit.Burst)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
			host = strings.TrimSpace(strings.Split(ip, ",")[0])
		}
		if !rl.allow(host) {
			w.Header().Set("Retry-After", "10")
			WriteProblem(w, r, http.StatusTooManyRequests,
				"https://tools.ietf.org/html/rfc6585#section-4",
				"Too Many Requests",
				"Rate limit exceeded. Please retry later.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.status = code
	rr.ResponseWriter.WriteHeader(code)
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		ident := IdentityFromContext(r.Context())
		isAPIKey := IsAPIKeyRequest(r.Context())
		userID := int64(0)
		if ident != nil {
			userID = ident.UserID
		}
		isJwt := ident != nil && !isAPIKey

		traceID := rec.Header().Get("X-Request-Id")
		if traceID == "" {
			traceID = RequestID(r)
		}

		s.log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start).Round(time.Millisecond).String(),
			"userId", userID,
			"auth", authLabel(isAPIKey, isJwt),
			"traceId", traceID,
			"ip", clientIP(r))

		if r.Method == http.MethodOptions {
			return
		}

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := s.recordStats(ctx, userID, isAPIKey, isJwt); err != nil {
				s.log.Warn("stats update failed", "error", err)
			}
		}()
	})
}

func authLabel(isAPIKey, isJwt bool) string {
	if isAPIKey {
		return "apikey"
	}
	if isJwt {
		return "jwt"
	}
	return "anonymous"
}

func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.TrimSpace(strings.Split(ip, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) recordStats(ctx context.Context, userID int64, isAPIKey, isJwt bool) error {
	today := time.Now().UTC().Format("2006-01-02")

	_, err := s.db.Exec(ctx,
		`INSERT INTO "DailyStats" ("Date","RequestCount") VALUES ($1::date, 1)
		 ON CONFLICT ("Date") DO UPDATE SET "RequestCount" = "DailyStats"."RequestCount" + 1`,
		today)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(ctx,
		`INSERT INTO "GlobalStats" ("Key","Value") VALUES ('TotalRequests', 1)
		 ON CONFLICT ("Key") DO UPDATE SET "Value" = "GlobalStats"."Value" + 1`)
	if err != nil {
		return err
	}

	if userID > 0 {
		_, err = s.db.Exec(ctx,
			`UPDATE "Users"
			 SET "RequestCount" = "RequestCount" + 1,
			     "ApiKeyRequestCount" = "ApiKeyRequestCount" + $2,
			     "JwtRequestCount" = "JwtRequestCount" + $3
			 WHERE "Id" = $1`,
			userID, boolToInt(isAPIKey), boolToInt(isJwt))
		if err != nil {
			return err
		}
	}
	return nil
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}
