package server

import (
	"context"
	"errors"
	"net/http"

	"connectrpc.com/connect"

	"github.com/at-ishikawa/langner/internal/auth"
)

// NewAuthInterceptor rejects any RPC whose request context lacks a verified
// session (populated by AuthCookieMiddleware) with CodeUnauthenticated, and
// injects the user id for downstream handlers otherwise.
func NewAuthInterceptor() connect.UnaryInterceptorFunc {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			sess, ok := auth.SessionFromContext(ctx)
			if !ok {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
			}
			return next(auth.WithUserID(ctx, sess.UserID), req)
		}
	})
}

// AuthCookieMiddleware verifies the session cookie and stows the session in the
// request context for both the connect interceptor and /auth/me. It never
// rejects — /auth/* and CORS preflight must pass through unauthenticated.
func AuthCookieMiddleware(next http.Handler, sessions *auth.SessionSigner) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(SessionCookieName); err == nil {
			if sess, err := sessions.Verify(c.Value); err == nil {
				ctx := auth.WithSession(r.Context(), sess)
				ctx = auth.WithUserID(ctx, sess.UserID)
				r = r.WithContext(ctx)
			}
		}
		next.ServeHTTP(w, r)
	})
}
