package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"connectrpc.com/connect"

	"github.com/at-ishikawa/langner/internal/auth"
)

// NewAuthInterceptor rejects any RPC whose request context lacks a verified
// session (populated by AuthBearerMiddleware) with CodeUnauthenticated, and
// injects the user id for downstream handlers otherwise. It is the always-on
// auth gate: main wires it unconditionally.
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

// AuthBearerMiddleware is the ONLY authentication path: it reads the
// `Authorization: Bearer <access-jwt>` header, and on a valid token populates
// the request context with the session/user id that both the connect
// interceptor and /auth/me read (auth.WithSession / auth.WithUserID). It NEVER
// rejects — an absent/expired/invalid bearer just leaves the context empty and
// the interceptor does the rejecting, so /auth/* and CORS preflight still pass
// through unauthenticated. There is no session cookie anymore; web and CLI both
// present a bearer.
func AuthBearerMiddleware(next http.Handler, tokens *auth.TokenSigner) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token, ok := bearerToken(r); ok {
			if claims, err := tokens.Verify(token); err == nil {
				sess := auth.Session{UserID: claims.UserID, ExpiresAt: claims.ExpiresAt}
				ctx := auth.WithSession(r.Context(), sess)
				ctx = auth.WithUserID(ctx, sess.UserID)
				r = r.WithContext(ctx)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// bearerToken extracts the token from an `Authorization: Bearer <token>`
// header, case-insensitively on the scheme.
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", false
	}
	const prefix = "bearer "
	if len(h) < len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(h[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}
