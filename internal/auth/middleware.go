package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/yuddmm/meetgo-dating-server/internal/platform/httpx"
)

type ctxKey int

const userIDKey ctxKey = iota

// Middleware authenticates requests via the `Authorization: Bearer <jwt>` header.
// On success the user id is stored in the request context; otherwise it responds
// 401 UNAUTHORIZED and stops the chain.
func (s *TokenService) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, ok := bearerToken(r)
		if !ok {
			httpx.WriteError(w, errUnauthorized)
			return
		}
		userID, err := s.ParseAccess(raw)
		if err != nil {
			httpx.WriteError(w, errUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), userIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UserID returns the authenticated user id from the context. ok is false when
// the request was not authenticated (no Middleware in the chain).
func UserID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userIDKey).(uuid.UUID)
	return id, ok
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(h[len(prefix):])
	return token, token != ""
}
