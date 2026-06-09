package analytics

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// TestNotebookMetadataResolver_FlashcardAndEtymology covers the two
// branches the WrongWord card relies on: a vocab lookup (flashcard
// notebook → meaning + first example) and an origin lookup
// (etymology notebook → meaning).
func TestNotebookMetadataResolver_FlashcardAndEtymology(t *testing.T) {
	root := t.TempDir()

	// Flashcards: one card "ephemeral" with meaning + example.
	flashDir := filepath.Join(root, "flashcards", "vocab")
	require.NoError(t, os.MkdirAll(flashDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(flashDir, "index.yml"), []byte(`id: vocab
name: Vocab
notebooks:
  - ./cards.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(flashDir, "cards.yml"), []byte(`- title: Common Words
  date: 2025-01-01T00:00:00Z
  cards:
    - expression: ephemeral
      meaning: lasting for a very short time
      examples:
        - "Snow on the warm street was ephemeral."
`), 0o644))

	// Etymology: one origin "tele" with meaning.
	etymDir := filepath.Join(root, "etymology", "wpme")
	require.NoError(t, os.MkdirAll(etymDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "index.yml"), []byte(`id: wpme
kind: Etymology
name: Word Power Made Easy
notebooks:
  - ./session1.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "session1.yml"), []byte(`metadata:
  title: "Session 1"
origins:
  - origin: tele
    type: prefix
    language: Greek
    meaning: far
`), 0o644))

	reader, err := notebook.NewReader(
		nil,
		[]string{filepath.Join(root, "flashcards")},
		nil,
		nil,
		[]string{filepath.Join(root, "etymology")},
		nil,
	)
	require.NoError(t, err)

	r := NewNotebookMetadataResolver(reader)

	t.Run("vocab", func(t *testing.T) {
		got := r.Resolve(context.Background(), "vocab", "ephemeral", ExpressionTypeVocabulary)
		assert.Equal(t, "lasting for a very short time", got.Meaning)
		assert.Equal(t, "Snow on the warm street was ephemeral.", got.ExampleSentence)
		assert.Equal(t, "flashcard", got.NotebookKind)
	})

	t.Run("origin", func(t *testing.T) {
		got := r.Resolve(context.Background(), "wpme", "tele", notebook.LearningExpressionTypeOrigin)
		assert.Equal(t, "far", got.Meaning)
		assert.Equal(t, "etymology", got.NotebookKind)
	})

	t.Run("miss returns empty", func(t *testing.T) {
		got := r.Resolve(context.Background(), "vocab", "nosuchword", ExpressionTypeVocabulary)
		assert.Equal(t, WordMetadata{}, got)
	})
}
