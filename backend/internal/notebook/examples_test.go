package notebook_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// examplesDir returns the absolute path to the examples directory.
// Tests are skipped if the directory doesn't exist (e.g., in CI without checkout root).
func examplesDir(t *testing.T) string {
	t.Helper()
	// Walk up from the test file to find the repo root
	dir, err := os.Getwd()
	require.NoError(t, err)
	for dir != "/" {
		if _, err := os.Stat(filepath.Join(dir, "config.example.yml")); err == nil {
			return filepath.Join(dir, "examples")
		}
		dir = filepath.Dir(dir)
	}
	// Try relative from backend/
	candidate := filepath.Join("..", "examples")
	if _, err := os.Stat(candidate); err == nil {
		abs, _ := filepath.Abs(candidate)
		return abs
	}
	t.Skip("examples directory not found")
	return ""
}

func TestExamples_ReadStoryNotebooks(t *testing.T) {
	examples := examplesDir(t)

	reader, err := notebook.NewReader(
		[]string{filepath.Join(examples, "stories")},
		nil,
		[]string{filepath.Join(examples, "books")},
		[]string{filepath.Join(examples, "definitions")},
		nil, nil,
	)
	require.NoError(t, err)

	t.Run("friends story notebook", func(t *testing.T) {
		notebooks, err := reader.ReadStoryNotebooks("friends")
		require.NoError(t, err)
		require.NotEmpty(t, notebooks)

		// Verify first notebook has scenes with definitions
		assert.NotEmpty(t, notebooks[0].Event)
		assert.NotEmpty(t, notebooks[0].Scenes)
		assert.NotEmpty(t, notebooks[0].Scenes[0].Definitions)
		assert.NotEmpty(t, notebooks[0].Scenes[0].Conversations)
	})

	t.Run("frankenstein book with merged definitions", func(t *testing.T) {
		notebooks, err := reader.ReadStoryNotebooks("frankenstein")
		require.NoError(t, err)
		require.NotEmpty(t, notebooks)

		// Verify definitions were merged from the definitions directory
		totalDefs := 0
		for _, nb := range notebooks {
			for _, scene := range nb.Scenes {
				totalDefs += len(scene.Definitions)
			}
		}
		assert.Greater(t, totalDefs, 0, "definitions should be merged from definitions directory")
	})
}

func TestExamples_ReadFlashcardNotebooks(t *testing.T) {
	examples := examplesDir(t)

	reader, err := notebook.NewReader(
		nil,
		[]string{filepath.Join(examples, "flashcards")},
		nil, nil, nil, nil,
	)
	require.NoError(t, err)

	notebooks, err := reader.ReadFlashcardNotebooks("vocabulary")
	require.NoError(t, err)
	require.NotEmpty(t, notebooks)
	assert.NotEmpty(t, notebooks[0].Cards)
	assert.NotEmpty(t, notebooks[0].Cards[0].Expression)
	assert.NotEmpty(t, notebooks[0].Cards[0].Meaning)
}

func TestExamples_ReadEtymologyNotebook(t *testing.T) {
	examples := examplesDir(t)

	reader, err := notebook.NewReader(
		nil, nil, nil, nil,
		[]string{filepath.Join(examples, "etymology")},
		nil,
	)
	require.NoError(t, err)

	origins, err := reader.ReadEtymologyNotebook("common-roots")
	require.NoError(t, err)
	require.NotEmpty(t, origins)

	// Verify origins have required fields
	for _, o := range origins {
		assert.NotEmpty(t, o.Origin, "origin name should not be empty")
		assert.NotEmpty(t, o.Meaning, "origin meaning should not be empty")
	}

	// Verify we have different types
	types := make(map[string]bool)
	for _, o := range origins {
		if o.Type != "" {
			types[o.Type] = true
		}
	}
	assert.Contains(t, types, "root", "should have root origins")
	assert.Contains(t, types, "prefix", "should have prefix origins")
	assert.Contains(t, types, "suffix", "should have suffix origins")
}

func TestExamples_LearningHistories(t *testing.T) {
	examples := examplesDir(t)

	histories, err := notebook.NewLearningHistories(filepath.Join(examples, "learning_notes"))
	require.NoError(t, err)

	t.Run("vocabulary flashcard history", func(t *testing.T) {
		vh, ok := histories["vocabulary"]
		require.True(t, ok, "vocabulary learning history should exist")
		require.NotEmpty(t, vh)
	})

	t.Run("friends story history", func(t *testing.T) {
		fh, ok := histories["friends"]
		require.True(t, ok, "friends learning history should exist")
		require.NotEmpty(t, fh)
	})

	t.Run("frankenstein definitions history", func(t *testing.T) {
		fh, ok := histories["frankenstein"]
		require.True(t, ok, "frankenstein learning history should exist")
		require.NotEmpty(t, fh)
	})

	t.Run("etymology history", func(t *testing.T) {
		eh, ok := histories["common-roots"]
		require.True(t, ok, "etymology learning history should exist")
		require.NotEmpty(t, eh)
	})
}

func TestExamples_FilterStoryNotebooks_IncludesReverseMisunderstood(t *testing.T) {
	examples := examplesDir(t)

	reader, err := notebook.NewReader(
		[]string{filepath.Join(examples, "stories")},
		nil,
		[]string{filepath.Join(examples, "books")},
		[]string{filepath.Join(examples, "definitions")},
		nil, nil,
	)
	require.NoError(t, err)

	histories, err := notebook.NewLearningHistories(filepath.Join(examples, "learning_notes"))
	require.NoError(t, err)

	// The frankenstein learning history has "forebodings" with a misunderstood
	// reverse log. FilterStoryNotebooks should include it in the PDF filter.
	notebooks, err := reader.ReadStoryNotebooks("frankenstein")
	require.NoError(t, err)

	lh := histories["frankenstein"]
	filtered, err := notebook.FilterStoryNotebooks(notebooks, lh, nil, false, true, false, true, notebook.QuizTypeNotebook)
	require.NoError(t, err)

	// Find forebodings in filtered result
	found := false
	for _, nb := range filtered {
		for _, scene := range nb.Scenes {
			for _, def := range scene.Definitions {
				if def.Expression == "forebodings" || def.Expression == "foreboding" {
					found = true
					assert.NotEmpty(t, def.ReverseLogs, "ReverseLogs should be populated")
				}
			}
		}
	}
	assert.True(t, found, "forebodings should be included due to reverse-misunderstood status")
}

func TestExamples_Validate(t *testing.T) {
	examples := examplesDir(t)

	calculator := &notebook.SM2Calculator{}
	validator := notebook.NewValidator(
		filepath.Join(examples, "learning_notes"),
		[]string{filepath.Join(examples, "stories")},
		[]string{filepath.Join(examples, "flashcards")},
		[]string{filepath.Join(examples, "definitions")},
		[]string{filepath.Join(examples, "etymology")},
		filepath.Join(examples, "dictionaries", "rapidapi"),
		calculator,
	)

	result, err := validator.Validate()
	require.NoError(t, err)
	assert.False(t, result.HasErrors(), "example data should pass validation without errors")
}
