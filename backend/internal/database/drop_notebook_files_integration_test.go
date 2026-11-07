package database

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/schemas"
)

// bundleHash is notebook.HashBundle for one file (that package imports this
// one, so it can't be called here).
func bundleHash(key string, content []byte) string {
	sum := sha256.Sum256(content)
	h := sha256.New()
	h.Write([]byte(key))
	h.Write([]byte{0})
	h.Write([]byte(hex.EncodeToString(sum[:])))
	h.Write([]byte{0})
	return hex.EncodeToString(h.Sum(nil))
}

// Migration 040 drops notebook_files only when no stored notebook still needs
// its files: every one has content rows, built from those files when they are
// its current bundle. Otherwise it keeps the table and fails.
func TestDropNotebookFiles_KeepsFilesStillNeeded_LivePostgres_Integration(t *testing.T) {
	content := []byte("id: phrasal-verbs")
	filesHash := bundleHash("flashcards/index.yml", content)

	cases := []struct {
		name      string
		hasRows   bool
		hash      string // notebooks.content_hash
		builtHash string // notebooks.content_built_hash
		dropped   bool
	}{
		{name: "rows built from the files", hasRows: true, hash: filesHash, builtHash: filesHash, dropped: true},
		{name: "files older than the rows", hasRows: true, hash: "newer-push", builtHash: "newer-push", dropped: true},
		{name: "never converted", hasRows: false, hash: filesHash, builtHash: "", dropped: false},
		{name: "rows behind the current files", hasRows: true, hash: filesHash, builtHash: "", dropped: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openDriftTestDB(t)
			require.NoError(t, MigrateTo(db, schemas.Migrations, "migrations", 39))
			_, err := db.Exec(`INSERT INTO notebooks (notebook_id, content_hash, content_built_hash) VALUES ('phrasal-verbs', $1, NULLIF($2, ''))`, tc.hash, tc.builtHash)
			require.NoError(t, err)
			_, err = db.Exec(`INSERT INTO notebook_files (notebook_id, family, path, content, content_sha256, byte_size)
				VALUES ('phrasal-verbs', 'flashcards', 'index.yml', $1, repeat('0', 64), $2)`, content, len(content))
			require.NoError(t, err)
			if tc.hasRows {
				_, err = db.Exec(`INSERT INTO content_indexes (notebook_id, family, kind, name) VALUES ('phrasal-verbs', 'flashcards', '', 'Phrasal verbs')`)
				require.NoError(t, err)
			}

			err = Migrate(db, schemas.Migrations, "migrations")
			var exists bool
			require.NoError(t, db.Get(&exists, `SELECT to_regclass('notebook_files') IS NOT NULL`))
			assert.Equal(t, !tc.dropped, exists)
			if tc.dropped {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, "never built from them")
			}
		})
	}
}
