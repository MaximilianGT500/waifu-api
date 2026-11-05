package server

import (
	"context"
	"net/http"
	"time"
)

func (s *Server) RegisterHealthRoutes() {
	s.Handle("GET /readyz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if s.draining.Load() {
			WriteProblem(w, r, http.StatusServiceUnavailable,
				"https://tools.ietf.org/html/rfc9110#section-15.6.4",
				"Service Unavailable", "The instance is shutting down.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := s.db.Ping(ctx); err != nil {
			WriteProblem(w, r, http.StatusServiceUnavailable,
				"https://tools.ietf.org/html/rfc9110#section-15.6.4",
				"Service Unavailable", "Database unavailable.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}))
	s.Handle("GET /healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		w.Header().Set("Cache-Control", "no-store")
		if err := s.db.Ping(ctx); err != nil {
			WriteProblem(w, r, http.StatusServiceUnavailable,
				"https://tools.ietf.org/html/rfc9110#section-15.6.4",
				"Service Unavailable", "Database unavailable.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}))
}

func (s *Server) cleanupStats(ctx context.Context) {
	cutoff := time.Now().UTC().AddDate(-1, 0, 0).Format("2006-01-02")
	if _, err := s.db.Exec(ctx, `DELETE FROM "DailyStats" WHERE "Date" < $1::date`, cutoff); err != nil {
		s.log.Warn("stats cleanup failed", "error", err)
	}
}

func (s *Server) cleanupReviewTasks(ctx context.Context) {
	retention := s.cfg.Review.TaskRetentionDays
	if retention <= 0 {
		return
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -retention)
	res, err := s.db.Exec(ctx, `
		DELETE FROM "ReviewTasks"
		WHERE "Status" IN (1,2,3) AND "ResolvedAt" IS NOT NULL AND "ResolvedAt" < $1`,
		cutoff)
	if err != nil {
		s.log.Warn("review task cleanup failed", "error", err)
	} else if res.RowsAffected() > 0 {
		s.log.Info("review tasks pruned", "count", res.RowsAffected())
	}
}
