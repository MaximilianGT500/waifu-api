package server

import (
	"context"
	"net/http"

	"github.com/waifu-im/waifu-api/backend/internal/auth"
)

type ctxKey int

const (
	ctxKeyRequestID ctxKey = iota
	ctxKeyIdentity
	ctxKeyIsAPIKey
	ctxKeyIsAPIKeyAttempt
)

func RequestID(r *http.Request) string {
	if v, ok := r.Context().Value(ctxKeyRequestID).(string); ok {
		return v
	}
	return ""
}

func withRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyRequestID, id)
}

func IdentityFromContext(ctx context.Context) *auth.Identity {
	v, _ := ctx.Value(ctxKeyIdentity).(*auth.Identity)
	return v
}

func withIdentity(ctx context.Context, id *auth.Identity) context.Context {
	return context.WithValue(ctx, ctxKeyIdentity, id)
}

func IsAPIKeyRequest(ctx context.Context) bool {
	v, _ := ctx.Value(ctxKeyIsAPIKey).(bool)
	return v
}
