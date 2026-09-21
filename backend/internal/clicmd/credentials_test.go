package clicmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withTempConfigHome points XDG_CONFIG_HOME at a temp dir for the test.
func withTempConfigHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("LANGNER_SERVER", "") // don't inherit a real env value
	return dir
}

func TestCredentialStore_RoundTripAndPerms(t *testing.T) {
	dir := withTempConfigHome(t)

	store, err := LoadCredentials()
	require.NoError(t, err)
	assert.Empty(t, store.Servers, "fresh store is empty")

	store.Set("http://localhost:8080", ServerCredentials{
		AccessToken:          "jwt-a",
		AccessTokenExpiresAt: time.Now().Add(time.Hour),
		RefreshToken:         "refresh-a",
	}, false)
	require.NoError(t, store.Save())

	// The first Set with no prior default becomes the default.
	assert.Equal(t, "http://localhost:8080", store.DefaultServer)

	// File + dir perms.
	path := filepath.Join(dir, "langner", "credentials.json")
	fi, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm())
	di, err := os.Stat(filepath.Join(dir, "langner"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), di.Mode().Perm())

	// Reload sees the same data.
	reloaded, err := LoadCredentials()
	require.NoError(t, err)
	got, ok := reloaded.Get("http://localhost:8080")
	require.True(t, ok)
	assert.Equal(t, "jwt-a", got.AccessToken)
	assert.Equal(t, "refresh-a", got.RefreshToken)
}

func TestCredentialStore_MultiServerNoLeak(t *testing.T) {
	withTempConfigHome(t)
	store, err := LoadCredentials()
	require.NoError(t, err)

	store.Set("http://localhost:8080", ServerCredentials{AccessToken: "dev"}, true)
	store.Set("https://api.langner.app", ServerCredentials{AccessToken: "prod"}, false)

	dev, ok := store.Get("http://localhost:8080")
	require.True(t, ok)
	assert.Equal(t, "dev", dev.AccessToken)
	prod, ok := store.Get("https://api.langner.app")
	require.True(t, ok)
	assert.Equal(t, "prod", prod.AccessToken)

	// A server with no entry is not resolved to another server's creds.
	_, ok = store.Get("https://other.example")
	assert.False(t, ok)
}

func TestCredentialStore_ResolveServerOrder(t *testing.T) {
	withTempConfigHome(t)
	store, err := LoadCredentials()
	require.NoError(t, err)

	// No flag, no env, no default → compiled-in default.
	assert.Equal(t, DefaultServer, store.ResolveServer(""))

	// Stored default wins over compiled-in.
	store.DefaultServer = "https://stored.example"
	assert.Equal(t, "https://stored.example", store.ResolveServer(""))

	// Env wins over stored default.
	t.Setenv("LANGNER_SERVER", "https://env.example")
	assert.Equal(t, "https://env.example", store.ResolveServer(""))

	// Explicit flag wins over everything.
	assert.Equal(t, "https://flag.example", store.ResolveServer("https://flag.example"))
}

func TestCredentialStore_DeleteClearsDefault(t *testing.T) {
	withTempConfigHome(t)
	store, err := LoadCredentials()
	require.NoError(t, err)
	store.Set("http://localhost:8080", ServerCredentials{AccessToken: "x"}, true)
	store.Delete("http://localhost:8080")
	_, ok := store.Get("http://localhost:8080")
	assert.False(t, ok)
	assert.Empty(t, store.DefaultServer)
}
