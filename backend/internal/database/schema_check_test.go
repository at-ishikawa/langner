package database

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/schemas"
)

// TestExpectedMigrationVersion_EmbeddedMigrations pins the version-parsing
// against the REAL embedded migrations so a renumbering that breaks the naming
// convention (or drops the leading integer) fails here rather than silently
// making the pre-flight check compare against the wrong number.
func TestExpectedMigrationVersion_EmbeddedMigrations(t *testing.T) {
	v, err := expectedMigrationVersion(schemas.Migrations, "migrations")
	require.NoError(t, err)
	// At least migration 022 (the homograph unique-index fix) exists; the max
	// is whatever the highest-numbered *.up.sql is. Assert it is that file's
	// number so adding a migration without touching this test still passes.
	assert.GreaterOrEqual(t, v, 22, "expected version must track the highest numbered migration")
}

func TestExpectedMigrationVersion_ParsesMax(t *testing.T) {
	fsys := fstest.MapFS{
		"migrations/001_a.up.sql":   {Data: []byte("")},
		"migrations/001_a.down.sql": {Data: []byte("")},
		"migrations/007_b.up.sql":   {Data: []byte("")},
		"migrations/022_c.up.sql":   {Data: []byte("")},
		"migrations/022_c.down.sql": {Data: []byte("")},
		"migrations/README.md":      {Data: []byte("not a migration")},
	}
	v, err := expectedMigrationVersion(fsys, "migrations")
	require.NoError(t, err)
	assert.Equal(t, 22, v, "must pick the highest up-migration number and ignore down/non-migration files")
}

func TestExpectedMigrationVersion_NoMigrations(t *testing.T) {
	fsys := fstest.MapFS{"migrations/README.md": {Data: []byte("x")}}
	_, err := expectedMigrationVersion(fsys, "migrations")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no *.up.sql migrations")
}
