package clicmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DefaultServer is the compiled-in server base URL used when neither --server,
// LANGNER_SERVER, nor a stored default_server selects one (design §10 Q6).
const DefaultServer = "http://localhost:8080"

// ServerCredentials holds the tokens issued by one server. The refresh token is
// the long-lived opaque secret; the access token is the short-lived bearer.
type ServerCredentials struct {
	AccessToken          string    `json:"access_token"`
	AccessTokenExpiresAt time.Time `json:"access_token_expires_at"`
	RefreshToken         string    `json:"refresh_token"`
}

// CredentialStore is the on-disk ~/.config/langner/credentials.json. It holds
// credentials for MULTIPLE servers keyed by server URL so a dev and a future
// production server never leak into each other; the active server is selected
// explicitly (flag/env/default) with NO fallback across servers.
type CredentialStore struct {
	DefaultServer string                       `json:"default_server,omitempty"`
	Servers       map[string]ServerCredentials `json:"servers"`
}

// credentialsPath returns the credentials.json path, honouring $XDG_CONFIG_HOME
// (falling back to ~/.config).
func credentialsPath() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "langner", "credentials.json"), nil
}

// LoadCredentials reads the store, returning an empty (non-nil) store when the
// file does not exist yet.
func LoadCredentials() (*CredentialStore, error) {
	path, err := credentialsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &CredentialStore{Servers: map[string]ServerCredentials{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}
	var store CredentialStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("parse credentials %s: %w", path, err)
	}
	if store.Servers == nil {
		store.Servers = map[string]ServerCredentials{}
	}
	return &store, nil
}

// Save writes the store atomically with 0600 perms under a 0700 dir.
func (s *CredentialStore) Save() error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	// Tighten the dir perms in case it pre-existed with looser bits.
	_ = os.Chmod(dir, 0o700)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace credentials: %w", err)
	}
	return nil
}

// Get returns the credentials for a server, if present.
func (s *CredentialStore) Get(server string) (ServerCredentials, bool) {
	c, ok := s.Servers[server]
	return c, ok
}

// Set stores credentials for a server. When makeDefault is true (or no default
// is set yet) it also becomes the default_server.
func (s *CredentialStore) Set(server string, creds ServerCredentials, makeDefault bool) {
	if s.Servers == nil {
		s.Servers = map[string]ServerCredentials{}
	}
	s.Servers[server] = creds
	if makeDefault || s.DefaultServer == "" {
		s.DefaultServer = server
	}
}

// Delete removes a server's entry and clears the default if it pointed there.
func (s *CredentialStore) Delete(server string) {
	delete(s.Servers, server)
	if s.DefaultServer == server {
		s.DefaultServer = ""
	}
}

// ResolveServer picks the active server: explicit flag → LANGNER_SERVER env →
// stored default_server → the compiled-in DefaultServer. There is no fallback
// ACROSS servers — this only chooses WHICH server; a missing credential entry
// for the chosen server is a separate, hard error (see requireCredentials).
func (s *CredentialStore) ResolveServer(flag string) string {
	if flag != "" {
		return flag
	}
	if env := os.Getenv("LANGNER_SERVER"); env != "" {
		return env
	}
	if s.DefaultServer != "" {
		return s.DefaultServer
	}
	return DefaultServer
}
