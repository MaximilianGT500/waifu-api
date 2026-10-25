package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/waifu-im/waifu-api/backend/internal/auth"
	"github.com/waifu-im/waifu-api/backend/internal/config"
	"github.com/waifu-im/waifu-api/backend/internal/db"
	"github.com/waifu-im/waifu-api/backend/internal/storage"
)

type Server struct {
	cfg     *config.Config
	db      *db.Pool
	s3      *storage.S3
	discord *auth.DiscordClient
	log     *slog.Logger
	mux     *http.ServeMux
	base    string
}

func New(cfg *config.Config, pool *db.Pool, s3Client *storage.S3, discord *auth.DiscordClient, log *slog.Logger) *Server {
	s := &Server{
		cfg:     cfg,
		db:      pool,
		s3:      s3Client,
		discord: discord,
		log:     log,
		mux:     http.NewServeMux(),
	}
	s.base = strings.Trim(cfg.APIBasePath, "/")
	return s
}

func (s *Server) Config() *config.Config { return s.cfg }

func (s *Server) DB() *db.Pool { return s.db }

func (s *Server) S3() *storage.S3 { return s.s3 }

func (s *Server) Discord() *auth.DiscordClient { return s.discord }

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

func (s *Server) authPass(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if IdentityFromContext(r.Context()) == nil {
			s.authenticate(r)
		}
		if ident := IdentityFromContext(r.Context()); ident != nil {
			var blacklisted bool
			var reason *string
			if err := s.db.QueryRow(r.Context(),
				`SELECT "IsBlacklisted", "BlacklistReason" FROM "Users" WHERE "Id" = $1`, ident.UserID).
				Scan(&blacklisted, &reason); err == nil && blacklisted {
				detail := "Your account has been blacklisted."
				if reason != nil && *reason != "" {
					detail = "Your account has been blacklisted. Reason: " + *reason
				}
				Forbidden(w, r, detail)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) Handler() http.Handler {

	notFound := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteProblem(w, r, http.StatusNotFound,
			"https://tools.ietf.org/html/rfc9110#section-15.5.5",
			"Not Found",
			"The requested endpoint was not found.")
	})
	s.mux.Handle("/", notFound)

	var h http.Handler = s.mux
	h = s.withRequestID(h)
	h = s.securityHeaders(h)
	h = s.recoveryMiddleware(h)
	h = s.corsMiddleware(h)
	h = s.rateLimitMiddleware(h)
	h = s.authPass(h)
	h = s.loggingMiddleware(h)
	return h
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decodeJSON(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}

func (s *Server) backgroundJob(name string, fn func(ctx context.Context)) {
	go func() {
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			fn(ctx)
			cancel()

			now := time.Now().UTC()
			next := time.Date(now.Year(), now.Month(), now.Day(), 4, 0, 0, 0, time.UTC)
			if !next.After(now) {
				next = next.Add(24 * time.Hour)
			}
			s.log.Info("background job scheduled", "job", name, "next", next)
			time.Sleep(time.Until(next))
		}
	}()
}
