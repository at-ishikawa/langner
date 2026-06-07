package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupEtymologyMigrationFixture builds a two-scene book with one
// multi-scene combinator origin (used as the structural analogue of
// the "logos" pattern), to verify the earliest-rule places it in the
// scene that introduces it first.
func setupEtymologyMigrationFixture(t *testing.T) (etymologyDir, definitionsDir, bookID string) {
	t.Helper()
	root := t.TempDir()
	bookID = "demo-book"

	etymologyDir = filepath.Join(root, "etymology")
	etymBook := filepath.Join(etymologyDir, bookID)
	require.NoError(t, os.MkdirAll(etymBook, 0o755))
	writeFile(t, filepath.Join(etymBook, "index.yml"), `id: demo-book
kind: Etymology
name: Demo Book
notebooks:
  - ./sessionA.yml
`)
	writeFile(t, filepath.Join(etymBook, "sessionA.yml"), `metadata:
  title: "Session A"
origins:
  - origin: alpha
    language: Greek
    meaning: first
  - origin: combinator
    language: Greek
    meaning: "study, science"
  - origin: beta
    language: Greek
    meaning: second
  - origin: orphan
    language: Latin
    meaning: lonely
concepts:
  - key: cluster
    meaning: a grouping
    members:
      - { origin: alpha, language: Greek }
      - { origin: beta,  language: Greek }
relations:
  - { type: antonym, between: [cluster, cluster] }
`)

	definitionsDir = filepath.Join(root, "definitions")
	defsBook := filepath.Join(definitionsDir, bookID)
	require.NoError(t, os.MkdirAll(defsBook, 0o755))
	writeFile(t, filepath.Join(defsBook, "index.yml"), `id: demo-book
notebooks:
  - ./sessionA.yml
`)
	writeFile(t, filepath.Join(defsBook, "sessionA.yml"), `- metadata:
    title: "Session A"
  scenes:
    - metadata:
        index: 0
        title: "alpha-scene"
      expressions:
        - expression: alphalogy
          meaning: the study of firstness
          origin_parts:
            - origin: alpha
              language: Greek
            - origin: combinator
              language: Greek
    - metadata:
        index: 1
        title: "beta-scene"
      expressions:
        - expression: betalogy
          meaning: the study of secondness
          origin_parts:
            - origin: beta
              language: Greek
            - origin: combinator
              language: Greek
`)
	return etymologyDir, definitionsDir, bookID
}

func TestMigrateEtymologyToScenes_PlacesCombinatorInEarliestScene(t *testing.T) {
	etymDir, defsDir, _ := setupEtymologyMigrationFixture(t)

	err := MigrateEtymologyToScenes([]string{etymDir}, []string{defsDir}, false)
	require.NoError(t, err)

	migrated, err := os.ReadFile(filepath.Join(etymDir, "demo-book", "sessionA.yml"))
	require.NoError(t, err)
	entries, err := notebook.ReadEtymologyFromBytes(migrated)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	entry := entries[0]

	assert.Equal(t, "Session A", entry.Event)
	// alpha-scene: alpha + combinator (combinator first appears here).
	// beta-scene: beta (combinator already placed; not duplicated here).
	// orphan synthetic scene: orphan (not referenced by any vocab).
	require.Len(t, entry.Scenes, 3, "alpha-scene, beta-scene, and an orphan-synthetic scene")

	assert.Equal(t, "alpha-scene", entry.Scenes[0].Scene)
	gotOrigins0 := originStrings(entry.Scenes[0].Origins)
	assert.ElementsMatch(t, []string{"alpha", "combinator"}, gotOrigins0,
		"combinator must land in alpha-scene (earliest scene that references it)")

	assert.Equal(t, "beta-scene", entry.Scenes[1].Scene)
	gotOrigins1 := originStrings(entry.Scenes[1].Origins)
	assert.Equal(t, []string{"beta"}, gotOrigins1,
		"beta-scene must hold ONLY beta — combinator already placed in alpha-scene")

	assert.Equal(t, "orphan", entry.Scenes[2].Scene)
	gotOrigins2 := originStrings(entry.Scenes[2].Origins)
	assert.Equal(t, []string{"orphan"}, gotOrigins2,
		"origin with no vocab reference falls into a synthetic scene named after itself")

	// Concepts and relations must round-trip into the new shape (they're
	// session-scoped, not scene-scoped).
	require.Len(t, entry.Concepts, 1)
	assert.Equal(t, "cluster", entry.Concepts[0].Key)
	require.Len(t, entry.Relations, 1)
	assert.Equal(t, "antonym", entry.Relations[0].Type)
}

func TestMigrateEtymologyToScenes_IsIdempotent(t *testing.T) {
	etymDir, defsDir, _ := setupEtymologyMigrationFixture(t)

	// First run migrates.
	require.NoError(t, MigrateEtymologyToScenes([]string{etymDir}, []string{defsDir}, false))
	firstPass, err := os.ReadFile(filepath.Join(etymDir, "demo-book", "sessionA.yml"))
	require.NoError(t, err)

	// Second run is a no-op — file is already new-shape.
	require.NoError(t, MigrateEtymologyToScenes([]string{etymDir}, []string{defsDir}, false))
	secondPass, err := os.ReadFile(filepath.Join(etymDir, "demo-book", "sessionA.yml"))
	require.NoError(t, err)

	assert.Equal(t, string(firstPass), string(secondPass), "re-running the migrator must not change an already-migrated file")
}

func TestMigrateEtymologyToScenes_DryRunDoesNotWrite(t *testing.T) {
	etymDir, defsDir, _ := setupEtymologyMigrationFixture(t)
	path := filepath.Join(etymDir, "demo-book", "sessionA.yml")

	before, err := os.ReadFile(path)
	require.NoError(t, err)

	require.NoError(t, MigrateEtymologyToScenes([]string{etymDir}, []string{defsDir}, true))

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "dry-run must not modify any file")
}

func originStrings(origins []notebook.EtymologyOrigin) []string {
	out := make([]string, len(origins))
	for i, o := range origins {
		out[i] = o.Origin
	}
	return out
}
