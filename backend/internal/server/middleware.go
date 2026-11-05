package server

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

var requestIDRe = regexp.MustCompile(`^[A-Za-z0-9._:\-]{1,128}$`)

func (s *Server) baseMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st := &requestState{}
		st.clientIP, st.viaTrustedHop = s.ipResolver.resolve(r)
		st.https = forwardedHTTPS(r, st.viaTrustedHop)

		id := r.Header.Get("X-Request-Id")
		if !requestIDRe.MatchString(id) {
			var buf [16]byte
			_, _ = rand.Read(buf[:])
			id = hex.EncodeToString(buf[:])
		}
		st.requestID = id
		w.Header().Set("X-Request-Id", id)

		next.ServeHTTP(w, r.WithContext(withState(r.Context(), st)))
	})
}

func (s *Server) isDocsPath(path string) bool {
	if s.base != "" {
		rest, ok := strings.CutPrefix(path, "/"+s.base)
		if !ok {
			return false
		}
		path = rest
	}
	return path == "/scalar" || strings.HasPrefix(path, "/scalar/")
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	hsts := ""
	if s.cfg.Network.HSTSMaxAge > 0 {
		hsts = "max-age=" + strconv.Itoa(s.cfg.Network.HSTSMaxAge) + "; includeSubDomains"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		if s.isDocsPath(r.URL.Path) {
			h.Set("Content-Security-Policy", scalarCSP)
		} else {
			h.Set("X-Frame-Options", "DENY")
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		}
		if hsts != "" {
			if st := stateFrom(r.Context()); st != nil && st.https {
				h.Set("Strict-Transport-Security", hsts)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	allowed := make(map[string]bool, len(s.cfg.AllowedOrigins))
	for _, o := range s.cfg.AllowedOrigins {
		allowed[strings.ToLower(o)] = true
	}
	allowAll := s.cfg.CORSAllowAll
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Add("Vary", "Origin")
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		if allowAll || allowed[strings.ToLower(strings.TrimRight(origin, "/"))] {
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Expose-Headers", "Location, Retry-After, X-Request-Id, X-RateLimit-Limit, X-RateLimit-Remaining")
			if r.Method == http.MethodOptions {
				h.Add("Vary", "Access-Control-Request-Method")
				h.Add("Vary", "Access-Control-Request-Headers")
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Authorization, X-Api-Key, Content-Type, X-Request-Id")
				h.Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				s.log.Error("panic while handling request",
					"panic", rec,
					"path", r.URL.Path,
					"traceId", RequestID(r),
					"stack", string(debug.Stack()))
				InternalError(w, r, nil, nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type responseRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int64
	wroteHeader bool
}

func (rr *responseRecorder) WriteHeader(code int) {
	if rr.wroteHeader {
		return
	}
	rr.wroteHeader = true
	rr.status = code
	rr.ResponseWriter.WriteHeader(code)
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	if !rr.wroteHeader {
		rr.WriteHeader(http.StatusOK)
	}
	n, err := rr.ResponseWriter.Write(b)
	rr.bytes += int64(n)
	return n, err
}

func (rr *responseRecorder) Unwrap() http.ResponseWriter { return rr.ResponseWriter }

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	prefix := ""
	if s.base != "" {
		prefix = "/" + s.base
	}
	health, ready := prefix+"/healthz", prefix+"/readyz"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		st := stateFrom(r.Context())
		userID := int64(0)
		var apiKeyID *int64
		if st.ident != nil {
			userID = st.ident.UserID
			apiKeyID = st.ident.APIKeyID
		}
		isJwt := st.ident != nil && !st.apiKey

		isHealth := r.URL.Path == health || r.URL.Path == ready
		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"bytes", rec.bytes,
			"duration", time.Since(start).Round(time.Millisecond).String(),
			"userId", userID,
			"auth", authLabel(st.apiKey, isJwt),
			"traceId", st.requestID,
			"ip", clientIP(r),
		}
		if isHealth {
			s.log.Debug("request", attrs...)
		} else {
			s.log.Info("request", attrs...)
		}

		if r.Method == http.MethodOptions || isHealth || rec.status == http.StatusTooManyRequests {
			return
		}
		s.stats.record(userID, st.apiKey, isJwt, apiKeyID)
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
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return n
	}
	return def
}
