package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Writes a YAML config, then sets env vars that should OVERRIDE it, and confirms
// env wins while un-overridden file values survive — the "run with env on top of
// (or instead of) a file" contract a serverless deploy relies on.
func TestLoad_EnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(path, []byte(`
server:
  port: 1234
  cors:
    allowed_origins:
      - http://localhost:3100
database:
  host: filehost
  port: 5432
  database: filedb
  username: fileuser
  tls: false
auth:
  redirect_url: http://file/callback
  frontend_url: http://file
  allowed_emails:
    - file@example.com
  cookie_secure: false
  cookie_samesite: lax
`), 0o600))

	t.Setenv("PORT", "9090")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://a.vercel.app, https://b.example.com")
	t.Setenv("DB_HOST", "pooler.example.com")
	t.Setenv("DB_PORT", "6543")
	t.Setenv("DB_USER", "postgres.project")
	t.Setenv("DB_TLS", "true")
	t.Setenv("DB_PARAMS", "sslmode=require, default_query_exec_mode=simple_protocol")
	t.Setenv("AUTH_REDIRECT_URL", "https://app.vercel.app/auth/google/callback")
	t.Setenv("AUTH_FRONTEND_URL", "https://app.vercel.app")
	t.Setenv("AUTH_ALLOWED_EMAILS", "a@x.com,b@y.com")
	t.Setenv("AUTH_COOKIE_SECURE", "true")
	t.Setenv("AUTH_COOKIE_SAMESITE", "none")

	loader, err := NewConfigLoader(path)
	require.NoError(t, err)
	cfg, err := loader.Load()
	require.NoError(t, err)

	// env wins
	assert.Equal(t, 9090, cfg.Server.Port)
	assert.Equal(t, []string{"https://a.vercel.app", "https://b.example.com"}, cfg.Server.CORS.AllowedOrigins)
	assert.Equal(t, "pooler.example.com", cfg.Database.Host)
	assert.Equal(t, 6543, cfg.Database.Port)
	assert.Equal(t, "postgres.project", cfg.Database.Username)
	assert.True(t, cfg.Database.TLS)
	assert.Equal(t, "require", cfg.Database.Params["sslmode"])
	assert.Equal(t, "simple_protocol", cfg.Database.Params["default_query_exec_mode"])
	assert.Equal(t, "https://app.vercel.app/auth/google/callback", cfg.Auth.RedirectURL)
	assert.Equal(t, "https://app.vercel.app", cfg.Auth.FrontendURL)
	assert.Equal(t, []string{"a@x.com", "b@y.com"}, cfg.Auth.AllowedEmails)
	assert.True(t, cfg.Auth.CookieSecure)
	assert.Equal(t, "none", cfg.Auth.CookieSameSite)

	// un-overridden file value survives
	assert.Equal(t, "filedb", cfg.Database.Database)
}

// With no config file present, env alone fully configures the server (the
// no-config.yml serverless case) on top of built-in defaults.
func TestLoad_EnvOnly_NoFile(t *testing.T) {
	// Point HOME at an empty dir and run from an empty CWD so the loader finds no
	// config.yml, then rely purely on defaults + env.
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DB_HOST", "envhost")
	t.Setenv("AUTH_COOKIE_SECURE", "true")

	loader, err := NewConfigLoader("")
	require.NoError(t, err)
	cfg, err := loader.Load()
	require.NoError(t, err)

	assert.Equal(t, "envhost", cfg.Database.Host)
	assert.True(t, cfg.Auth.CookieSecure)
	assert.Equal(t, 8080, cfg.Server.Port, "default survives when PORT unset")
}

func TestEnvExtraHelpers(t *testing.T) {
	t.Run("mergeKVInto merges into existing map, skips bad pairs", func(t *testing.T) {
		m := map[string]string{"keep": "1"}
		mergeKVInto(&m, "a=1, b = 2 ,bad")
		assert.Equal(t, "1", m["keep"])
		assert.Equal(t, "1", m["a"])
		assert.Equal(t, "2", m["b"])
		_, hasBad := m["bad"]
		assert.False(t, hasBad, "a param with no '=' is skipped")
	})
	t.Run("mergeKVInto creates the map when nil", func(t *testing.T) {
		var m map[string]string
		mergeKVInto(&m, "sslmode=require")
		assert.Equal(t, "require", m["sslmode"])
	})
	t.Run("mergeKVInto empty string is a no-op", func(t *testing.T) {
		var m map[string]string
		mergeKVInto(&m, "")
		assert.Nil(t, m)
	})
	t.Run("trimList trims and drops empties", func(t *testing.T) {
		assert.Equal(t, []string{"a", "b"}, trimList([]string{" a ", "", " b ", "  "}))
	})
}
