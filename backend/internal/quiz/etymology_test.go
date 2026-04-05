package quiz

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/config"
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

	// Create learning notes directory
	learningDir := filepath.Join(tmpDir, "learning")
	require.NoError(t, os.MkdirAll(learningDir, 0755))

	return tmpDir, learningDir
}

func TestService_LoadEtymologyOriginCards(t *testing.T) {
	tmpDir, learningDir := createEtymologyFixtures(t)

	svc := NewService(
		config.NotebooksConfig{
			EtymologyDirectories:   []string{filepath.Join(tmpDir, "etymology")},
			LearningNotesDirectory: learningDir,
		},
		nil, nil, nil,
		config.QuizConfig{},
	)

	cards, err := svc.LoadEtymologyOriginCards(
		[]string{"latin-roots"},
		true,
	)
	require.NoError(t, err)

	assert.Len(t, cards, 3)

	originMap := make(map[string]EtymologyOriginCard)
	for _, card := range cards {
		originMap[card.Origin] = card
	}

	spectCard, ok := originMap["spect"]
	require.True(t, ok, "should find 'spect' card")
	assert.Equal(t, "to look or see", spectCard.Meaning)
	assert.Equal(t, "root", spectCard.Type)
	assert.Equal(t, "Latin", spectCard.Language)
	assert.Equal(t, "Latin Roots", spectCard.NotebookTitle)

	preCard, ok := originMap["pre"]
	require.True(t, ok, "should find 'pre' card")
	assert.Equal(t, "before", preCard.Meaning)
	assert.Equal(t, "prefix", preCard.Type)
}

func TestService_LoadEtymologyOriginCards_Deduplicates(t *testing.T) {
	tmpDir := t.TempDir()

	etymDir1 := filepath.Join(tmpDir, "etymology", "roots-1")
	require.NoError(t, os.MkdirAll(etymDir1, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir1, "index.yml"), []byte(`id: roots-1
kind: Etymology
name: Roots 1
notebooks:
  - ./origins.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir1, "origins.yml"), []byte(`- origin: "spect"
  type: root
  language: Latin
  meaning: to look or see
`), 0644))

	etymDir2 := filepath.Join(tmpDir, "etymology", "roots-2")
	require.NoError(t, os.MkdirAll(etymDir2, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir2, "index.yml"), []byte(`id: roots-2
kind: Etymology
name: Roots 2
notebooks:
  - ./origins.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir2, "origins.yml"), []byte(`- origin: "spect"
  type: root
  language: Latin
  meaning: to see
`), 0644))

	learningDir := filepath.Join(tmpDir, "learning")
	require.NoError(t, os.MkdirAll(learningDir, 0755))

	svc := NewService(
		config.NotebooksConfig{
			EtymologyDirectories:   []string{filepath.Join(tmpDir, "etymology")},
			LearningNotesDirectory: learningDir,
		},
		nil, nil, nil,
		config.QuizConfig{},
	)

	cards, err := svc.LoadEtymologyOriginCards([]string{"roots-1", "roots-2"}, true)
	require.NoError(t, err)
	assert.Len(t, cards, 1, "duplicate origins should be deduplicated")
}
