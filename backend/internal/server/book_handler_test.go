package server

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/ebook"
	"gopkg.in/yaml.v3"
)

// A minimal generic Project Gutenberg plain-text fixture (public-domain style
// boilerplate + one chapter). Never real user data.
const gutenbergFixture = `The Project Gutenberg eBook of A Tiny Tale

*** START OF THE PROJECT GUTENBERG EBOOK A TINY TALE ***

CHAPTER I

The fox ran fast. The dog was lazy and slow.

*** END OF THE PROJECT GUTENBERG EBOOK A TINY TALE ***`

func TestBuildBookBundle_DeterministicIDAndBooksFamily(t *testing.T) {
	notebookID, files, displayName, err := buildBookBundle("42", "A Tiny Tale", "Jane Doe", gutenbergFixture)
	require.NoError(t, err)

	// Deterministic shared id (not the slugged title), so a re-import dedups.
	assert.Equal(t, "gutenberg-42", notebookID)
	assert.Equal(t, "A Tiny Tale by Jane Doe", displayName)

	require.NotEmpty(t, files)
	var indexContent []byte
	for _, f := range files {
		// Every file is stored under the "books" family so the reader classifies
		// the notebook as a readable book (walkIndexFiles(books, isBook=true)).
		assert.Equal(t, "books", f.Family, "file %q must be in books family", f.Path)
		assert.Equal(t, notebookID, f.NotebookID)
		if f.Path == "index.yml" {
			indexContent = f.Content
		}
	}
	require.NotNil(t, indexContent, "bundle must contain index.yml")

	// The index.yml id is overridden to the deterministic id (parser slugs the
	// title; identity is server-owned here).
	var idx ebook.NotebookIndex
	require.NoError(t, yaml.Unmarshal(indexContent, &idx))
	assert.Equal(t, "gutenberg-42", idx.ID)
	assert.Equal(t, "A Tiny Tale by Jane Doe", idx.Name)
	assert.NotEmpty(t, idx.NotebookPaths, "index must reference chapter files")

	// Boilerplate stripped: the PG markers must not survive into any chapter.
	for _, f := range files {
		assert.False(t, strings.Contains(string(f.Content), "PROJECT GUTENBERG EBOOK"),
			"boilerplate leaked into %q", f.Path)
	}
}
