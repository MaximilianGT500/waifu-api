package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/waifu-im/waifu-api/backend/internal/auth"
)

func (s *Server) authenticate(r *http.Request) *auth.Identity {

	values, hasKeyHeader := r.Header[http.CanonicalHeaderKey("X-Api-Key")]
	if hasKeyHeader {
		key := ""
		if len(values) > 0 {
			key = strings.TrimSpace(values[0])
		}
		ctx := context.WithValue(r.Context(), ctxKeyIsAPIKeyAttempt, true)
		if key != "" {
			ident, err := s.lookupAPIKey(r.Context(), key)
			if err == nil {
				ctx = withIdentity(ctx, ident)
				ctx = context.WithValue(ctx, ctxKeyIsAPIKey, true)
				*r = *r.WithContext(ctx)
				return ident
			}
		}
		*r = *r.WithContext(ctx)
		return nil
	}

	header := r.Header.Get("Authorization")
	if strings.HasPrefix(header, "Bearer ") {
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		ident, err := auth.ValidateJWT(token, []byte(s.cfg.JWTKey), s.cfg.JWTIssuer, s.cfg.JWTAudience)
		if err != nil {
			return nil
		}
		ctx := withIdentity(r.Context(), ident)
		*r = *r.WithContext(ctx)
		return ident
	}
	return nil
}

func (s *Server) lookupAPIKey(ctx context.Context, rawKey string) (*auth.Identity, error) {
	hash := auth.HashAPIKey(rawKey)
	var (
		keyID   int64
		userID  int64
		name    string
		role    int
		expires *time.Time
	)
	err := s.db.QueryRow(ctx, `
		SELECT k."Id", k."UserId", u."Name", u."Role", k."ExpirationDate"
		FROM "ApiKeys" k
		JOIN "Users" u ON u."Id" = k."UserId"
		WHERE k."KeyHash" = $1`, hash).
		Scan(&keyID, &userID, &name, &role, &expires)
	if err != nil {
		return nil, err
	}
	if expires != nil && expires.Before(time.Now().UTC()) {
		return nil, errors.New("api key expired")
	}

	r, ok := auth.RoleFromInt(role)
	if !ok {
		return nil, errors.New("unknown role")
	}
	return &auth.Identity{UserID: userID, Name: name, Role: r, APIKeyID: &keyID}, nil
}

func apiKeyAttempted(r *http.Request) bool {
	v, _ := r.Context().Value(ctxKeyIsAPIKeyAttempt).(bool)
	return v
}

func (s *Server) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if IdentityFromContext(r.Context()) == nil {
			WriteUnauthorizedChallenge(w, apiKeyAttempted(r))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) RequireRole(min string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ident := IdentityFromContext(r.Context())
			if ident == nil {
				WriteUnauthorizedChallenge(w, apiKeyAttempted(r))
				return
			}
			if !auth.MeetsMinRole(ident.Role, min) {
				WriteForbiddenChallenge(w, apiKeyAttempted(r))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
