package notebook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReader_ReadEtymologyNotebook(t *testing.T) {
	// Create temp directory structure for etymology notebook
	tmpDir := t.TempDir()
	etymDir := filepath.Join(tmpDir, "etymology", "latin-roots")
	require.NoError(t, os.MkdirAll(etymDir, 0755))

	// Write index.yml
	indexYAML := `id: latin-roots
kind: Etymology
name: Latin Roots
notebooks:
  - ./origins.yml
`
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "index.yml"), []byte(indexYAML), 0644))

	// Write origins.yml
	originsYAML := `- origin: "spect"
  type: root
  language: Latin
  meaning: to look or see
- origin: "pre"
  type: prefix
  language: Latin
  meaning: before
- origin: "graph"
  type: root
  language: Greek
  meaning: to write
`
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "origins.yml"), []byte(originsYAML), 0644))

	reader, err := NewReader(
		[]string{filepath.Join(tmpDir, "etymology")},
		nil,
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)

	// Verify etymology index was loaded
	etymIndexes := reader.GetEtymologyIndexes()
	assert.Len(t, etymIndexes, 1)
	assert.Contains(t, etymIndexes, "latin-roots")
	assert.Equal(t, "Latin Roots", etymIndexes["latin-roots"].Name)

	// Read the notebook
	origins, err := reader.ReadEtymologyNotebook("latin-roots")
	require.NoError(t, err)
	assert.Len(t, origins, 3)

	assert.Equal(t, "spect", origins[0].Origin)
	assert.Equal(t, "root", origins[0].Type)
	assert.Equal(t, "Latin", origins[0].Language)
	assert.Equal(t, "to look or see", origins[0].Meaning)

	assert.Equal(t, "pre", origins[1].Origin)
	assert.Equal(t, "prefix", origins[1].Type)

	assert.Equal(t, "graph", origins[2].Origin)
	assert.Equal(t, "Greek", origins[2].Language)
}

func TestReader_ReadEtymologyNotebook_NotFound(t *testing.T) {
	reader, err := NewReader(nil, nil, nil, nil, nil)
	require.NoError(t, err)

	_, err = reader.ReadEtymologyNotebook("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestReader_EtymologyNotSeparatedFromStory(t *testing.T) {
	// Verify that etymology indexes with kind "Etymology" are NOT loaded as story indexes
	tmpDir := t.TempDir()
	mixedDir := filepath.Join(tmpDir, "mixed")
	require.NoError(t, os.MkdirAll(filepath.Join(mixedDir, "etymology-nb"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(mixedDir, "story-nb"), 0755))

	// Etymology index
	etymIndex := `id: etym-test
kind: Etymology
name: Test Etymology
notebooks:
  - ./origins.yml
`
	require.NoError(t, os.WriteFile(filepath.Join(mixedDir, "etymology-nb", "index.yml"), []byte(etymIndex), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mixedDir, "etymology-nb", "origins.yml"), []byte("[]"), 0644))

	// Story index
	storyIndex := `id: story-test
kind: TVShows
name: Test Story
notebooks:
  - ./episodes.yml
`
	require.NoError(t, os.WriteFile(filepath.Join(mixedDir, "story-nb", "index.yml"), []byte(storyIndex), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mixedDir, "story-nb", "episodes.yml"), []byte("[]"), 0644))

	reader, err := NewReader(
		[]string{mixedDir},
		nil,
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)

	// Etymology should be in etymology indexes
	assert.Len(t, reader.GetEtymologyIndexes(), 1)
	assert.Contains(t, reader.GetEtymologyIndexes(), "etym-test")

	// Story should be in story indexes, but etymology should NOT be
	storyIndexes := reader.GetStoryIndexes()
	assert.Contains(t, storyIndexes, "story-test")
	assert.NotContains(t, storyIndexes, "etym-test")
}
