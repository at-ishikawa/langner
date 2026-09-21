package clicmd

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/spf13/cobra"
)

// credsFromToken converts a server token response into stored credentials,
// computing the access-token expiry from expires_in.
func credsFromToken(tok *TokenResponse) ServerCredentials {
	return ServerCredentials{
		AccessToken:          tok.AccessToken,
		AccessTokenExpiresAt: time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second),
		RefreshToken:         tok.RefreshToken,
	}
}

// notLoggedInErr is the consistent "no entry for this server" error — there is
// no fallback across servers.
func notLoggedInErr(server string) error {
	return fmt.Errorf("not logged in to %s — run `langner login --server %s`", server, server)
}

// NewLoginCommand builds `langner login`: run the RFC 8628 device flow against a
// server, then store the issued tokens keyed by that server.
func NewLoginCommand() *cobra.Command {
	var server string
	var setDefault bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in to a langner server via the browser device flow",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := LoadCredentials()
			if err != nil {
				return err
			}
			target := store.ResolveServer(server)
			return runLogin(cmd.Context(), store, target, setDefault || store.DefaultServer == "")
		},
	}
	cmd.Flags().StringVar(&server, "server", "", "server base URL (default: LANGNER_SERVER, stored default, or "+DefaultServer+")")
	cmd.Flags().BoolVar(&setDefault, "set-default", false, "make this server the default for future commands")
	return cmd
}

func runLogin(ctx context.Context, store *CredentialStore, server string, makeDefault bool) error {
	client := NewAuthClient(server)
	dc, err := client.RequestDeviceCode(ctx)
	if err != nil {
		return fmt.Errorf("request device code from %s: %w", server, err)
	}

	fmt.Printf("To sign in, open:  %s\n", dc.VerificationURI)
	fmt.Printf("and enter code:    %s\n", dc.UserCode)
	if dc.VerificationURIComplete != "" {
		openBrowser(dc.VerificationURIComplete)
	}
	fmt.Println("(waiting for approval…)")

	interval := dc.Interval
	if interval <= 0 {
		interval = 5
	}
	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("login timed out — the code expired before it was approved")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(interval) * time.Second):
		}

		tok, err := client.PollToken(ctx, dc.DeviceCode)
		if err != nil {
			switch {
			case isOAuthErr(err, "authorization_pending"):
				continue
			case isOAuthErr(err, "slow_down"):
				interval += 5
				continue
			case isOAuthErr(err, "access_denied"):
				return fmt.Errorf("login denied in the browser")
			case isOAuthErr(err, "expired_token"):
				return fmt.Errorf("login timed out — the code expired before it was approved")
			default:
				return fmt.Errorf("poll for token: %w", err)
			}
		}

		store.Set(server, credsFromToken(tok), makeDefault)
		if err := store.Save(); err != nil {
			return err
		}
		if me, err := client.Me(ctx, tok.AccessToken); err == nil && me.Username != "" {
			fmt.Printf("Logged in as %s (%s)\n", me.Username, server)
		} else {
			fmt.Printf("Logged in to %s\n", server)
		}
		return nil
	}
}

// openBrowser makes a best-effort attempt to open a URL; failure is silent
// (the URL+code are always printed as the headless-safe fallback).
func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	default:
		cmd = "xdg-open"
	}
	args = append(args, url)
	_ = exec.Command(cmd, args...).Start()
}

// NewLogoutCommand builds `langner logout`: revoke the stored refresh token and
// delete the server's credentials.
func NewLogoutCommand() *cobra.Command {
	var server string
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Sign out of a langner server (revoke + delete stored credentials)",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := LoadCredentials()
			if err != nil {
				return err
			}
			target := store.ResolveServer(server)
			creds, ok := store.Get(target)
			if !ok {
				return notLoggedInErr(target)
			}
			// Best-effort revoke; delete local creds regardless of network result.
			if creds.RefreshToken != "" {
				if err := NewAuthClient(target).Revoke(cmd.Context(), creds.RefreshToken); err != nil {
					fmt.Printf("warning: could not revoke token server-side: %v\n", err)
				}
			}
			store.Delete(target)
			if err := store.Save(); err != nil {
				return err
			}
			fmt.Println("Logged out.")
			return nil
		},
	}
	cmd.Flags().StringVar(&server, "server", "", "server base URL")
	return cmd
}

// NewWhoamiCommand builds `langner whoami`: ensure a valid access token
// (refreshing if needed) and print the username from /auth/me.
func NewWhoamiCommand() *cobra.Command {
	var server string
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Show the signed-in identity for a langner server",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := LoadCredentials()
			if err != nil {
				return err
			}
			target := store.ResolveServer(server)
			accessToken, err := ensureAccessToken(cmd.Context(), store, target)
			if err != nil {
				return err
			}
			me, err := NewAuthClient(target).Me(cmd.Context(), accessToken)
			if err != nil {
				if isOAuthErr(err, "unauthenticated") {
					return notLoggedInErr(target)
				}
				return err
			}
			if !me.Authenticated {
				return notLoggedInErr(target)
			}
			fmt.Printf("%s (%s)\n", me.Username, target)
			return nil
		},
	}
	cmd.Flags().StringVar(&server, "server", "", "server base URL")
	return cmd
}

// ensureAccessToken returns a usable access token for the server, proactively
// refreshing (and persisting the rotated pair) when the stored token is within
// 60s of expiry. A missing entry is a hard error (no cross-server fallback); a
// failed refresh clears the entry and tells the user to log in again.
func ensureAccessToken(ctx context.Context, store *CredentialStore, server string) (string, error) {
	creds, ok := store.Get(server)
	if !ok {
		return "", notLoggedInErr(server)
	}
	needsRefresh := creds.AccessToken == "" || time.Until(creds.AccessTokenExpiresAt) < 60*time.Second
	if needsRefresh && creds.RefreshToken != "" {
		tok, err := NewAuthClient(server).Refresh(ctx, creds.RefreshToken)
		if err != nil {
			if isOAuthErr(err, "invalid_grant") {
				store.Delete(server)
				_ = store.Save()
				return "", fmt.Errorf("session expired — run `langner login --server %s`", server)
			}
			return "", fmt.Errorf("refresh access token: %w", err)
		}
		creds = credsFromToken(tok)
		store.Set(server, creds, false)
		if err := store.Save(); err != nil {
			return "", err
		}
	}
	if creds.AccessToken == "" {
		return "", notLoggedInErr(server)
	}
	return creds.AccessToken, nil
}
