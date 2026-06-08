package notebook

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const legacyEtymologyYAML = `metadata:
  title: "Session A"
origins:
  - origin: spect
    type: root
    language: Latin
    meaning: to look or see
concepts:
  - key: looking
    meaning: things related to seeing
    members:
      - origin: spect
relations: []
`

const newShapeEtymologyYAML = `- event: "Session B"
  scenes:
    - scene: "ana (up, back)"
      origins:
        - origin: tele
          type: prefix
          language: Greek
          meaning: far
  concepts:
    - key: distance
      meaning: things related to distance
      members:
        - origin: tele
`

// writeEtymologyFixture lays out one etymology notebook with two session files
// — one legacy-shape (`session1.yml`) and one new-shape (`session2.yml`) — so
// the YAML* import sources have to handle both in the same run. The new-shape
// file is the exact regression case from `word-power-made-easy/session2.yml`.
func writeEtymologyFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	etymDir := filepath.Join(dir, "etymology", "book")
	require.NoError(t, os.MkdirAll(etymDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "index.yml"), []byte(`id: book
kind: Etymology
name: Book
notebooks:
  - ./session1.yml
  - ./session2.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "session1.yml"), []byte(legacyEtymologyYAML), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "session2.yml"), []byte(newShapeEtymologyYAML), 0o644))
	return dir
}

func newFixtureReader(t *testing.T) *Reader {
	t.Helper()
	dir := writeEtymologyFixture(t)
	reader, err := NewReader(nil, nil, nil, nil, []string{filepath.Join(dir, "etymology")}, nil)
	require.NoError(t, err)
	return reader
}

func TestYAMLEtymologyOriginSource_BothShapes(t *testing.T) {
	src := NewYAMLEtymologyOriginSource(newFixtureReader(t))
	rows, err := src.FindAll(context.Background())
	require.NoError(t, err)
	titles := map[string]bool{}
	for _, r := range rows {
		titles[r.SessionTitle] = true
	}
	assert.True(t, titles["Session A"], "legacy session not parsed: %v", titles)
	assert.True(t, titles["Session B"], "new-shape session not parsed: %v", titles)
}

func TestYAMLSemanticConceptSource_BothShapes(t *testing.T) {
	src := NewYAMLSemanticConceptSource(newFixtureReader(t))
	rows, err := src.FindAll(context.Background())
	require.NoError(t, err)
	got := map[string]string{}
	for _, r := range rows {
		got[r.Key] = r.SessionTitle
	}
	assert.Equal(t, "Session A", got["looking"])
	assert.Equal(t, "Session B", got["distance"])
}
