package auth

import (
	"context"
	"time"
)

// Session is the authenticated identity carried in the request context. It is
// populated from a verified bearer access token (AuthBearerMiddleware) and
// carries only the user id and the token's expiry — NO email or other PII.
type Session struct {
	UserID    int64
	ExpiresAt time.Time
}

type contextKey int

const (
	userIDContextKey contextKey = iota
	sessionContextKey
)

// WithUserID stores the authenticated user id in the context.
func WithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, userIDContextKey, userID)
}

// UserIDFromContext returns the authenticated user id, if present.
func UserIDFromContext(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(userIDContextKey).(int64)
	return userID, ok
}

// WithSession stores the verified session in the context.
func WithSession(ctx context.Context, sess Session) context.Context {
	return context.WithValue(ctx, sessionContextKey, sess)
}

// SessionFromContext returns the verified session, if present.
func SessionFromContext(ctx context.Context) (Session, bool) {
	sess, ok := ctx.Value(sessionContextKey).(Session)
	return sess, ok
}
