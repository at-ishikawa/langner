package quiz

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/notebook"
)

func createEtymologyFixtures(t *testing.T) (string, string) {
	t.Helper()

	tmpDir := t.TempDir()

	// Create etymology notebook
	etymDir := filepath.Join(tmpDir, "etymology", "latin-roots")
	require.NoError(t, os.MkdirAll(etymDir, 0755))

	etymIndex := `id: latin-roots
kind: Etymology
name: Latin Roots
notebooks:
  - ./origins.yml
`
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "index.yml"), []byte(etymIndex), 0644))

	originsYAML := `- origin: "spect"
  type: root
  language: Latin
  meaning: to look or see
- origin: "pre"
  type: prefix
  language: Latin
  meaning: before
- origin: "tion"
  type: suffix
  language: Latin
  meaning: act or process of
`
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "origins.yml"), []byte(originsYAML), 0644))

	// Create flashcard notebook with origin_parts
	flashDir := filepath.Join(tmpDir, "flashcards", "vocab")
	require.NoError(t, os.MkdirAll(flashDir, 0755))

	flashIndex := `id: vocab
name: Vocabulary
notebooks:
  - ./cards.yml
`
	require.NoError(t, os.WriteFile(filepath.Join(flashDir, "index.yml"), []byte(flashIndex), 0644))

	cardsYAML := `- title: "Unit 1"
  cards:
    - expression: "inspect"
      meaning: "to look at closely"
      origin_parts:
        - origin: "spect"
          language: "Latin"
    - expression: "prediction"
      meaning: "a statement about the future"
      origin_parts:
        - origin: "pre"
          language: "Latin"
        - origin: "tion"
          language: "Latin"
    - expression: "happy"
      meaning: "feeling pleasure"
`
	require.NoError(t, os.WriteFile(filepath.Join(flashDir, "cards.yml"), []byte(cardsYAML), 0644))

	// Create learning notes directory
	learningDir := filepath.Join(tmpDir, "learning")
	require.NoError(t, os.MkdirAll(learningDir, 0755))

	return tmpDir, learningDir
}

func TestService_LoadEtymologyCards(t *testing.T) {
	tmpDir, learningDir := createEtymologyFixtures(t)

	svc := NewService(
		config.NotebooksConfig{
			StoriesDirectories:     []string{filepath.Join(tmpDir, "etymology")},
			FlashcardsDirectories:  []string{filepath.Join(tmpDir, "flashcards")},
			LearningNotesDirectory: learningDir,
		},
		nil, // openaiClient not needed for loading
		nil, // dictionaryMap not needed
	)

	cards, err := svc.LoadEtymologyCards(
		[]string{"latin-roots"},
		[]string{"vocab"},
		true, // include unstudied
	)
	require.NoError(t, err)

	// Should find 2 cards (inspect and prediction) which have origin_parts
	// "happy" should be excluded because it has no origin_parts
	assert.Len(t, cards, 2)

	// Verify cards have resolved origin parts
	expressionMap := make(map[string]EtymologyCard)
	for _, card := range cards {
		expressionMap[card.Expression] = card
	}

	inspectCard, ok := expressionMap["inspect"]
	require.True(t, ok, "should find 'inspect' card")
	assert.Equal(t, "to look at closely", inspectCard.Meaning)
	assert.Len(t, inspectCard.OriginParts, 1)
	assert.Equal(t, "spect", inspectCard.OriginParts[0].Origin)
	assert.Equal(t, "to look or see", inspectCard.OriginParts[0].Meaning)

	predictionCard, ok := expressionMap["prediction"]
	require.True(t, ok, "should find 'prediction' card")
	assert.Len(t, predictionCard.OriginParts, 2)
}

func TestService_LoadEtymologyCards_NoOriginParts(t *testing.T) {
	tmpDir := t.TempDir()

	// Create etymology notebook
	etymDir := filepath.Join(tmpDir, "etymology", "test-roots")
	require.NoError(t, os.MkdirAll(etymDir, 0755))

	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "index.yml"), []byte(`id: test-roots
kind: Etymology
name: Test Roots
notebooks:
  - ./origins.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "origins.yml"), []byte("[]"), 0644))

	// Create flashcard without origin_parts
	flashDir := filepath.Join(tmpDir, "flashcards", "simple")
	require.NoError(t, os.MkdirAll(flashDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(flashDir, "index.yml"), []byte(`id: simple
name: Simple
notebooks:
  - ./cards.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(flashDir, "cards.yml"), []byte(`- title: "Words"
  cards:
    - expression: "house"
      meaning: "a building for living in"
`), 0644))

	learningDir := filepath.Join(tmpDir, "learning")
	require.NoError(t, os.MkdirAll(learningDir, 0755))

	svc := NewService(
		config.NotebooksConfig{
			StoriesDirectories:     []string{filepath.Join(tmpDir, "etymology")},
			FlashcardsDirectories:  []string{filepath.Join(tmpDir, "flashcards")},
			LearningNotesDirectory: learningDir,
		},
		nil, nil,
	)

	cards, err := svc.LoadEtymologyCards([]string{"test-roots"}, []string{"simple"}, true)
	require.NoError(t, err)
	assert.Len(t, cards, 0, "should find no cards without origin_parts")
}

func TestDeduplicateEtymologyCards(t *testing.T) {
	cards := []EtymologyCard{
		{Expression: "inspect", Meaning: "to look at"},
		{Expression: "Inspect", Meaning: "to look at closely"},
		{Expression: "preview", Meaning: "to see before"},
	}

	result := deduplicateEtymologyCards(cards)
	assert.Len(t, result, 2)
}

func TestResolveOriginParts(t *testing.T) {
	originMap := map[string]EtymologyOriginPart{
		"spect|latin": {Origin: "spect", Type: "root", Language: "Latin", Meaning: "to look"},
		"pre|latin":   {Origin: "pre", Type: "prefix", Language: "Latin", Meaning: "before"},
	}

	tests := []struct {
		name string
		refs []notebook.OriginPartRef
		want int
	}{
		{
			name: "exact match",
			refs: []notebook.OriginPartRef{
				{Origin: "spect", Language: "Latin"},
			},
			want: 1,
		},
		{
			name: "match by origin only",
			refs: []notebook.OriginPartRef{
				{Origin: "spect"},
			},
			want: 1,
		},
		{
			name: "no match",
			refs: []notebook.OriginPartRef{
				{Origin: "unknown", Language: "Latin"},
			},
			want: 0,
		},
		{
			name: "multiple matches",
			refs: []notebook.OriginPartRef{
				{Origin: "spect", Language: "Latin"},
				{Origin: "pre", Language: "Latin"},
			},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveOriginParts(tt.refs, originMap)
			assert.Len(t, got, tt.want)
		})
	}
}

func TestHasMatchingOrigin(t *testing.T) {
	originSet := map[string]bool{
		"spect": true,
		"pre":   true,
	}

	tests := []struct {
		name string
		refs []notebook.OriginPartRef
		want bool
	}{
		{
			name: "has matching origin",
			refs: []notebook.OriginPartRef{{Origin: "spect"}},
			want: true,
		},
		{
			name: "no matching origin",
			refs: []notebook.OriginPartRef{{Origin: "unknown"}},
			want: false,
		},
		{
			name: "empty refs",
			refs: nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasMatchingOrigin(tt.refs, originSet)
			assert.Equal(t, tt.want, got)
		})
	}
}
