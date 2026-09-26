package clicmd

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
)

// BearerInterceptor attaches `Authorization: Bearer <access-token>` to every
// Connect RPC for a server, refreshing proactively (via ensureAccessToken)
// before the call and reactively once on a CodeUnauthenticated response. It is
// the shared plumbing future user-facing RPC commands opt into by constructing
// their client with connect.WithInterceptors(NewBearerInterceptor(store,
// server)). The DB-direct admin commands never hit the server and are
// unaffected.
type BearerInterceptor struct {
	store  *CredentialStore
	server string
}

// NewBearerInterceptor builds a BearerInterceptor for a resolved server.
func NewBearerInterceptor(store *CredentialStore, server string) *BearerInterceptor {
	return &BearerInterceptor{store: store, server: server}
}

// WrapUnary attaches the bearer and does proactive/reactive-401 refresh.
func (i *BearerInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		token, err := ensureAccessToken(ctx, i.store, i.server)
		if err != nil {
			return nil, connect.NewError(connect.CodeUnauthenticated, err)
		}
		req.Header().Set("Authorization", "Bearer "+token)
		resp, err := next(ctx, req)
		if err != nil && connect.CodeOf(err) == connect.CodeUnauthenticated {
			// Reactive refresh: rotate once and retry the original request.
			refreshed, rerr := i.forceRefresh(ctx)
			if rerr != nil {
				return nil, connect.NewError(connect.CodeUnauthenticated, rerr)
			}
			req.Header().Set("Authorization", "Bearer "+refreshed)
			return next(ctx, req)
		}
		return resp, err
	}
}

// WrapStreamingClient attaches the bearer to streaming clients (no reactive
// retry — a stream cannot be transparently replayed).
func (i *BearerInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		if token, err := ensureAccessToken(ctx, i.store, i.server); err == nil {
			conn.RequestHeader().Set("Authorization", "Bearer "+token)
		}
		return conn
	}
}

// WrapStreamingHandler is a no-op (this interceptor is client-side only).
func (i *BearerInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

// forceRefresh rotates the refresh token unconditionally and persists the pair.
func (i *BearerInterceptor) forceRefresh(ctx context.Context) (string, error) {
	creds, ok := i.store.Get(i.server)
	if !ok || creds.RefreshToken == "" {
		return "", notLoggedInErr(i.server)
	}
	tok, err := NewAuthClient(i.server).Refresh(ctx, creds.RefreshToken)
	if err != nil {
		if isOAuthErr(err, "invalid_grant") {
			i.store.Delete(i.server)
			_ = i.store.Save()
			return "", fmt.Errorf("session expired — run `langner login --server %s`", i.server)
		}
		return "", fmt.Errorf("refresh access token: %w", err)
	}
	i.store.Set(i.server, credsFromToken(tok), false)
	if err := i.store.Save(); err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}
