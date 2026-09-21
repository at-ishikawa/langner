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

// AuthBearerMiddleware accepts an `Authorization: Bearer <access-jwt>` token
// ALONGSIDE the session cookie (additive; the cookie path is untouched). It
// only acts when the cookie middleware has NOT already established a session,
// so a cookie'd request keeps its cookie identity; a request with no valid
// cookie but a valid bearer authenticates via the same context the interceptor
// and /auth/me read (auth.WithSession / auth.WithUserID). Like the cookie
// middleware it NEVER rejects — a bad/expired/absent bearer just leaves the
// context empty and the interceptor does the rejecting.
//
// To make the cookie win when both are present, compose this INSIDE the cookie
// middleware (AuthCookieMiddleware(AuthBearerMiddleware(root))).
func AuthBearerMiddleware(next http.Handler, tokens *auth.TokenSigner) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.SessionFromContext(r.Context()); !ok {
			if token, ok := bearerToken(r); ok {
				if claims, err := tokens.Verify(token); err == nil {
					sess := auth.Session{UserID: claims.UserID, ExpiresAt: claims.ExpiresAt}
					ctx := auth.WithSession(r.Context(), sess)
					ctx = auth.WithUserID(ctx, sess.UserID)
					r = r.WithContext(ctx)
				}
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
