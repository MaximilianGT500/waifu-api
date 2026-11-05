package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/waifu-im/waifu-api/backend/internal/auth"
)

const maxAPIKeyLength = 256

type authResult struct {
	ident           *auth.Identity
	blacklisted     bool
	blacklistReason *string
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st := stateFrom(r.Context())
		res, err := s.authenticate(r.Context(), r, st)
		if err != nil {
			s.log.Error("authentication lookup failed", "error", err, "traceId", st.requestID)
			w.Header().Set("Retry-After", "5")
			WriteProblem(w, r, http.StatusServiceUnavailable,
				"https://tools.ietf.org/html/rfc9110#section-15.6.4",
				"Service Unavailable",
				"Authentication is temporarily unavailable. Please retry later.")
			return
		}
		if res.ident != nil {
			w.Header().Set("Cache-Control", "no-store")
			if res.blacklisted {
				detail := "Your account has been blacklisted."
				if res.blacklistReason != nil && *res.blacklistReason != "" {
					detail = "Your account has been blacklisted. Reason: " + *res.blacklistReason
				}
				Forbidden(w, r, detail)
				return
			}
			st.ident = res.ident
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authenticate(ctx context.Context, r *http.Request, st *requestState) (authResult, error) {
	if values, hasKeyHeader := r.Header[http.CanonicalHeaderKey("X-Api-Key")]; hasKeyHeader {
		st.apiKeyAttempt = true
		key := ""
		if len(values) > 0 {
			key = strings.TrimSpace(values[0])
		}
		if key == "" || len(key) > maxAPIKeyLength {
			return authResult{}, nil
		}
		res, err := s.lookupAPIKey(ctx, key)
		if err != nil {
			return authResult{}, err
		}
		if res.ident != nil {
			st.apiKey = true
		}
		return res, nil
	}

	header := r.Header.Get("Authorization")
	if len(header) > 7 && strings.EqualFold(header[:7], "Bearer ") {
		token := strings.TrimSpace(header[7:])
		claims, err := auth.ValidateJWT(token, []byte(s.cfg.JWTKey), s.cfg.JWTIssuer, s.cfg.JWTAudience)
		if err != nil {
			return authResult{}, nil
		}
		return s.lookupUser(ctx, claims)
	}
	return authResult{}, nil
}

func (s *Server) lookupUser(ctx context.Context, claims *auth.TokenClaims) (authResult, error) {
	var (
		name      *string
		role      int
		discordID *string
		res       authResult
	)
	err := s.db.QueryRow(ctx, `
		SELECT "Name", "Role", "DiscordId", "IsBlacklisted", "BlacklistReason"
		FROM "Users" WHERE "Id" = $1`, claims.UserID).
		Scan(&name, &role, &discordID, &res.blacklisted, &res.blacklistReason)
	if errors.Is(err, pgx.ErrNoRows) {
		return authResult{}, nil
	}
	if err != nil {
		return authResult{}, err
	}
	rRole, ok := auth.RoleFromInt(role)
	if !ok {
		rRole = auth.RoleUser
	}
	res.ident = &auth.Identity{UserID: claims.UserID, Name: orEmpty(name), Role: rRole, DiscordID: orEmpty(discordID)}
	return res, nil
}

func (s *Server) lookupAPIKey(ctx context.Context, rawKey string) (authResult, error) {
	hash := auth.HashAPIKey(rawKey)
	var (
		keyID     int64
		userID    int64
		name      *string
		role      int
		discordID *string
		expires   *time.Time
		res       authResult
	)
	err := s.db.QueryRow(ctx, `
		SELECT k."Id", k."UserId", u."Name", u."Role", u."DiscordId", k."ExpirationDate",
			u."IsBlacklisted", u."BlacklistReason"
		FROM "ApiKeys" k
		JOIN "Users" u ON u."Id" = k."UserId"
		WHERE k."KeyHash" = $1`, hash).
		Scan(&keyID, &userID, &name, &role, &discordID, &expires, &res.blacklisted, &res.blacklistReason)
	if errors.Is(err, pgx.ErrNoRows) {
		return authResult{}, nil
	}
	if err != nil {
		return authResult{}, err
	}
	if expires != nil && expires.Before(time.Now().UTC()) {
		return authResult{}, nil
	}
	rRole, ok := auth.RoleFromInt(role)
	if !ok {
		return authResult{}, nil
	}
	res.ident = &auth.Identity{UserID: userID, Name: orEmpty(name), Role: rRole, DiscordID: orEmpty(discordID), APIKeyID: &keyID}
	return res, nil
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

const adminSessionDetail = "Changing roles, bans or stored files requires a website session; API keys cannot do this."

func (s *Server) RequireSession(next http.Handler) http.Handler {
	return s.requireSession("API keys cannot be used to manage API keys. Please sign in on the website.", next)
}

func (s *Server) requireSession(detail string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if IdentityFromContext(r.Context()) == nil {
			WriteUnauthorizedChallenge(w, apiKeyAttempted(r))
			return
		}
		if IsAPIKeyRequest(r.Context()) {
			pascalCaseChallenge(w, "Forbidden", http.StatusForbidden, detail,
				"https://tools.ietf.org/html/rfc9110#section-15.5.4")
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
