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
		got := r.Resolve(context.Background(), "vocab", "ephemeral", ExpressionTypeVocabulary, "notebook")
		assert.Equal(t, "lasting for a very short time", got.Meaning)
		assert.Equal(t, "Snow on the warm street was ephemeral.", got.ExampleSentence)
		assert.Equal(t, "flashcard", got.NotebookKind)
	})

	t.Run("origin", func(t *testing.T) {
		got := r.Resolve(context.Background(), "wpme", "tele", notebook.LearningExpressionTypeOrigin, "etymology_breakdown")
		assert.Equal(t, "far", got.Meaning)
		assert.Equal(t, "etymology", got.NotebookKind)
	})

	t.Run("miss returns empty", func(t *testing.T) {
		got := r.Resolve(context.Background(), "vocab", "nosuchword", ExpressionTypeVocabulary, "notebook")
		assert.Equal(t, WordMetadata{}, got)
	})

	t.Run("case insensitive vocab", func(t *testing.T) {
		// Learning-history records can drift from the canonical YAML in
		// case (e.g. "Ephemeral" vs "ephemeral"). The resolver must still
		// surface the meaning.
		got := r.Resolve(context.Background(), "vocab", "EPHEMERAL", ExpressionTypeVocabulary, "notebook")
		assert.Equal(t, "lasting for a very short time", got.Meaning)
	})

	t.Run("case insensitive origin", func(t *testing.T) {
		got := r.Resolve(context.Background(), "wpme", "TELE", notebook.LearningExpressionTypeOrigin, "etymology_breakdown")
		assert.Equal(t, "far", got.Meaning)
	})
}

// TestNotebookMetadataResolver_StoryNotebookPullsExampleFromConversations
// pins the behavior the user expects on the analytics card for a
// story-style notebook (Speak English Like an American, Friends, etc.).
// These notebooks declare definitions in a separate file with no
// `examples:` field; the natural usage example is the conversation
// quote that contains the idiom. Three sub-cases the test covers:
//
//   - canonical expression appears verbatim in a conversation quote
//   - the quote uses a conjugated / plural form (definition alias)
//   - statements are scanned when conversations don't carry the idiom
//
// Without this fallback the analytics card showed only the long scene
// summary in the breadcrumb and no usage line at all — the user read
// the breadcrumb as the example and reported it as "wrong and useless".
func TestNotebookMetadataResolver_StoryNotebookPullsExampleFromConversations(t *testing.T) {
	root := t.TempDir()

	storyDir := filepath.Join(root, "stories", "idioms")
	require.NoError(t, os.MkdirAll(storyDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(`id: idioms
kind: Book
name: Idioms Book
notebooks:
  - ./book.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "book.yml"), []byte(`- event: 'LESSON 1: TEAM TALK'
  date: 2026-02-04T00:00:00Z
  scenes:
    - scene: |
        Alex and Sam discuss a sudden change in plans.
      conversations:
        - speaker: Alex
          quote: We were getting ready to break the ice with the new client when the deal fell through.
        - speaker: Sam
          quote: Don't worry about it.
- event: 'LESSON 2: REGRETS'
  date: 2026-02-05T00:00:00Z
  scenes:
    - scene: |
        Alex regrets earlier choices.
      conversations:
        - speaker: Alex
          quote: I keep losing my temper at meetings — it's been a rough week.
- event: 'LESSON 3: STATEMENTS'
  date: 2026-02-06T00:00:00Z
  scenes:
    - scene: |
        A narration-only lesson.
      statements:
        - The trick was to spill the beans without anyone catching on.
`), 0o644))

	defsDir := filepath.Join(root, "definitions", "stories", "idioms")
	require.NoError(t, os.MkdirAll(defsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(defsDir, "index.yml"), []byte(`id: idioms
notebooks:
  - ./definitions.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(defsDir, "definitions.yml"), []byte(`- metadata:
    title: 'LESSON 1: TEAM TALK'
  scenes:
    - metadata:
        index: 0
      expressions:
        - expression: break the ice
          meaning: to start a conversation in a social setting
- metadata:
    title: 'LESSON 2: REGRETS'
  scenes:
    - metadata:
        index: 0
      expressions:
        - expression: lose one's temper
          definition: lose one's temper
          meaning: to suddenly become very angry
- metadata:
    title: 'LESSON 3: STATEMENTS'
  scenes:
    - metadata:
        index: 0
      expressions:
        - expression: spill the beans
          meaning: to reveal a secret
`), 0o644))

	reader, err := notebook.NewReader(
		[]string{filepath.Join(root, "stories")},
		nil,
		nil,
		[]string{filepath.Join(root, "definitions")},
		nil,
		nil,
	)
	require.NoError(t, err)
	r := NewNotebookMetadataResolver(reader)

	t.Run("conversation quote with verbatim expression", func(t *testing.T) {
		got := r.Resolve(context.Background(), "idioms", "break the ice", ExpressionTypeVocabulary, "notebook")
		assert.Equal(t, "to start a conversation in a social setting", got.Meaning)
		assert.Equal(t, "story", got.NotebookKind)
		assert.Equal(t, "We were getting ready to break the ice with the new client when the deal fell through.", got.ExampleSentence,
			"story notebooks without `examples:` should fall back to a conversation quote that uses the expression")
	})

	t.Run("conversation quote uses conjugated form via definition alias", func(t *testing.T) {
		// Learning history may record "losing my temper" or "lose my temper"
		// — both should resolve to a quote that uses the same idiom in any
		// conjugated form. We exercise the definition-alias path by querying
		// the canonical "lose one's temper" form; the quote uses "losing my
		// temper" so plain expression-substring would miss.
		got := r.Resolve(context.Background(), "idioms", "lose one's temper", ExpressionTypeVocabulary, "notebook")
		assert.Equal(t, "to suddenly become very angry", got.Meaning)
		assert.Equal(t, "story", got.NotebookKind)
		assert.Contains(t, got.ExampleSentence, "losing my temper",
			"alias-form quotes must surface — without this the definition's plural / conjugated quote would be invisible to the resolver")
	})

	t.Run("falls back to statements when conversations don't carry the idiom", func(t *testing.T) {
		got := r.Resolve(context.Background(), "idioms", "spill the beans", ExpressionTypeVocabulary, "notebook")
		assert.Equal(t, "to reveal a secret", got.Meaning)
		assert.Equal(t, "The trick was to spill the beans without anyone catching on.", got.ExampleSentence,
			"narration-only scenes must surface the matching statement as the usage example")
	})
}

// TestNotebookMetadataResolver_QuizTypeOverridesExpressionType pins the
// gauche-class regression: a word that lives on BOTH the etymology side
// (origin meaning) and the vocabulary side (English-adjective meaning)
// must resolve based on which quiz produced the wrong attempt, not the
// learning-history record's static `type:` field.
//
// The reported case: the user failed a vocabulary `notebook` quiz on the
// word "gauche" (English meaning "clumsy, especially in social
// situations") but the analytics card showed "left hand" — the meaning
// of the French origin. The learning-history record carried `type:
// origin` because the same string is also tracked as an etymology
// origin in the same notebook; with the prior dispatch logic the
// resolver picked the origin path and surfaced the wrong meaning.
func TestNotebookMetadataResolver_QuizTypeOverridesExpressionType(t *testing.T) {
	root := t.TempDir()

	// Definitions side: the English vocabulary entry.
	defsDir := filepath.Join(root, "definitions", "wpme")
	require.NoError(t, os.MkdirAll(defsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(defsDir, "index.yml"), []byte(`id: wpme
notebooks:
  - ./session3.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(defsDir, "session3.yml"), []byte(`- metadata:
    title: "Session 3"
  scenes:
    - metadata:
        index: 0
        title: "social adjectives"
      expressions:
        - expression: gauche
          meaning: clumsy, tactless, especially in social situations
          part_of_speech: adjective
`), 0o644))

	// Etymology side: the French origin.
	etymDir := filepath.Join(root, "etymology", "wpme")
	require.NoError(t, os.MkdirAll(etymDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "index.yml"), []byte(`id: wpme
kind: Etymology
name: Word Power Made Easy
notebooks:
  - ./session3.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "session3.yml"), []byte(`metadata:
  title: "Session 3"
origins:
  - origin: gauche
    language: French
    meaning: left
`), 0o644))

	reader, err := notebook.NewReader(nil, nil, nil,
		[]string{filepath.Join(root, "definitions")},
		[]string{filepath.Join(root, "etymology")}, nil)
	require.NoError(t, err)
	r := NewNotebookMetadataResolver(reader)

	t.Run("vocab quiz returns the English meaning even when expressionType says origin", func(t *testing.T) {
		got := r.Resolve(context.Background(), "wpme", "gauche",
			notebook.LearningExpressionTypeOrigin, "notebook")
		assert.Equal(t, "clumsy, tactless, especially in social situations", got.Meaning,
			"a vocabulary quiz failure must surface the vocabulary meaning regardless of how the learning-history record was tagged")
	})

	t.Run("etymology quiz returns the origin meaning", func(t *testing.T) {
		got := r.Resolve(context.Background(), "wpme", "gauche",
			notebook.LearningExpressionTypeOrigin, "etymology_breakdown")
		assert.Equal(t, "left", got.Meaning,
			"etymology_* quizzes must continue to return the origin meaning")
	})

	t.Run("reverse vocab quiz also returns the English meaning", func(t *testing.T) {
		got := r.Resolve(context.Background(), "wpme", "gauche",
			notebook.LearningExpressionTypeOrigin, "reverse")
		assert.Equal(t, "clumsy, tactless, especially in social situations", got.Meaning)
	})
}

// TestNotebookMetadataResolver_DefinitionsBookCovered targets the Word-Power-
// Made-Easy class of bug: vocab definitions live in definitions_directories
// (not stories / flashcards), so the resolver has to walk GetDefinitionsNotes
// or the meaning silently disappears from the analytics card.
func TestNotebookMetadataResolver_DefinitionsBookCovered(t *testing.T) {
	root := t.TempDir()

	defsDir := filepath.Join(root, "definitions", "wpme")
	require.NoError(t, os.MkdirAll(defsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(defsDir, "index.yml"), []byte(`id: wpme
name: WPME
notebooks:
  - ./session1.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(defsDir, "session1.yml"), []byte(`- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        index: 0
        title: "Old age"
      expressions:
        - expression: geriatrics
          meaning: the medicine of the elderly
          examples:
            - "The clinic specializes in geriatrics."
`), 0o644))

	reader, err := notebook.NewReader(nil, nil, nil, []string{filepath.Join(root, "definitions")}, nil, nil)
	require.NoError(t, err)
	r := NewNotebookMetadataResolver(reader)

	got := r.Resolve(context.Background(), "wpme", "geriatrics", ExpressionTypeVocabulary, "notebook")
	assert.Equal(t, "the medicine of the elderly", got.Meaning)
	assert.Equal(t, "The clinic specializes in geriatrics.", got.ExampleSentence)
	assert.Equal(t, "story", got.NotebookKind, "definitions-only notebooks deep-link via the story reader")
}
