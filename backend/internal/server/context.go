package server

import (
	"context"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"github.com/waifu-im/waifu-api/backend/internal/auth"
)

type ctxKey int

const (
	ctxKeyState ctxKey = iota
)

type requestState struct {
	requestID     string
	clientIP      netip.Addr
	viaTrustedHop bool
	https         bool

	ident         *auth.Identity
	apiKey        bool
	apiKeyAttempt bool

	query url.Values
}

func withState(ctx context.Context, st *requestState) context.Context {
	return context.WithValue(ctx, ctxKeyState, st)
}

func stateFrom(ctx context.Context) *requestState {
	st, _ := ctx.Value(ctxKeyState).(*requestState)
	return st
}

func RequestID(r *http.Request) string {
	if st := stateFrom(r.Context()); st != nil {
		return st.requestID
	}
	return ""
}

func IdentityFromContext(ctx context.Context) *auth.Identity {
	if st := stateFrom(ctx); st != nil {
		return st.ident
	}
	return nil
}

func IsAPIKeyRequest(ctx context.Context) bool {
	if st := stateFrom(ctx); st != nil {
		return st.apiKey
	}
	return false
}

func apiKeyAttempted(r *http.Request) bool {
	if st := stateFrom(r.Context()); st != nil {
		return st.apiKeyAttempt
	}
	return false
}

func clientIP(r *http.Request) string {
	if st := stateFrom(r.Context()); st != nil && st.clientIP.IsValid() {
		return st.clientIP.String()
	}
	return ""
}

func queryValues(r *http.Request) url.Values {
	st := stateFrom(r.Context())
	if st != nil && st.query != nil {
		return st.query
	}
	raw := r.URL.Query()
	q := make(url.Values, len(raw))
	for k, v := range raw {
		lk := strings.ToLower(k)
		q[lk] = append(q[lk], v...)
	}
	if st != nil {
		st.query = q
	}
	return q
}

func queryGet(r *http.Request, key string) string {
	vals := queryValues(r)[strings.ToLower(key)]
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

func queryAll(r *http.Request, key string) []string {
	return queryValues(r)[strings.ToLower(key)]
}
