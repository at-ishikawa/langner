package notebook

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testFlashcardEnv represents a test environment for flashcard tests.
type testFlashcardEnv struct {
	t       *testing.T
	tempDir string
}

// newTestFlashcardEnv creates a new test environment with a temporary directory.
// The directory is automatically cleaned up when the test completes.
func newTestFlashcardEnv(t *testing.T) *testFlashcardEnv {
	tempDir, err := os.MkdirTemp("", "flashcard_test")
	require.NoError(t, err)
	t.Cleanup(func() {
		os.RemoveAll(tempDir)
	})
	return &testFlashcardEnv{
		t:       t,
		tempDir: tempDir,
	}
}

// createFlashcardIndex creates a flashcard index.yml file with the given ID, name, and notebook paths.
func (env *testFlashcardEnv) createFlashcardIndex(indexID, indexName string, notebookPaths []string) string {
	flashcardDir := filepath.Join(env.tempDir, indexID)
	err := os.MkdirAll(flashcardDir, 0755)
	require.NoError(env.t, err)

	indexContent := "id: " + indexID + "\n"
	indexContent += "name: \"" + indexName + "\"\n"
	indexContent += "notebooks:\n"
	for _, path := range notebookPaths {
		indexContent += "  - " + path + "\n"
	}

	indexPath := filepath.Join(flashcardDir, "index.yml")
	err = os.WriteFile(indexPath, []byte(indexContent), 0644)
	require.NoError(env.t, err)

	return flashcardDir
}

// createCardFile creates a flashcard cards.yml file with the given content.
func (env *testFlashcardEnv) createCardFile(dir, filename, content string) {
	filePath := filepath.Join(dir, filename)
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(env.t, err)
}

// createStoryIndex creates a story index.yml file with the given ID, name, and notebook paths.
func (env *testFlashcardEnv) createStoryIndex(indexID, indexName string, notebookPaths []string) string {
	storyDir := filepath.Join(env.tempDir, indexID)
	err := os.MkdirAll(storyDir, 0755)
	require.NoError(env.t, err)

	indexContent := "id: " + indexID + "\n"
	indexContent += "name: \"" + indexName + "\"\n"
	indexContent += "notebooks:\n"
	for _, path := range notebookPaths {
		indexContent += "  - " + path + "\n"
	}

	indexPath := filepath.Join(storyDir, "index.yml")
	err = os.WriteFile(indexPath, []byte(indexContent), 0644)
	require.NoError(env.t, err)

	return storyDir
}

func TestNewReader_EmptyStoriesDirValidFlashcardDir(t *testing.T) {
	// This test covers the bug fix where passing "" for stories dir caused a crash
	env := newTestFlashcardEnv(t)

	// Create flashcard directory with index.yml
	flashcardDir := env.createFlashcardIndex("vocab", "Vocabulary", []string{"./cards.yml"})

	// Create a simple cards.yml
	cardsContent := `- title: "Test Cards"
  description: "Test flashcards"
  date: 2025-01-15T00:00:00Z
  cards:
    - expression: "test"
      meaning: "a test word"
`
	env.createCardFile(flashcardDir, "cards.yml", cardsContent)

	// Test with empty slice for stories directories
	reader, err := NewReader([]string{}, []string{env.tempDir}, nil, nil, nil)
	require.NoError(t, err)
	assert.NotNil(t, reader)

	// Verify flashcard indexes were loaded
	flashcardIndexes := reader.GetFlashcardIndexes()
	assert.Len(t, flashcardIndexes, 1)
	assert.Contains(t, flashcardIndexes, "vocab")
	assert.Equal(t, "Vocabulary", flashcardIndexes["vocab"].Name)

	// Verify story indexes are empty
	assert.Len(t, reader.indexes, 0)
}

func TestNewReader_ValidStoriesDirEmptyFlashcardDir(t *testing.T) {
	env := newTestFlashcardEnv(t)

	// Create story directory with index.yml
	storyDir := env.createStoryIndex("stories", "Story One", []string{"./notebook.yml"})

	// Test with empty slice for flashcard directories
	reader, err := NewReader([]string{storyDir}, []string{}, nil, nil, nil)
	require.NoError(t, err)
	assert.NotNil(t, reader)

	// Verify story indexes were loaded
	assert.Len(t, reader.indexes, 1)
	assert.Contains(t, reader.indexes, "stories")

	// Verify flashcard indexes are empty
	flashcardIndexes := reader.GetFlashcardIndexes()
	assert.Len(t, flashcardIndexes, 0)
}

func TestNewReader_BothDirectoriesEmpty(t *testing.T) {
	// Test with both directories as empty slices
	reader, err := NewReader([]string{}, []string{}, nil, nil, nil)
	require.NoError(t, err)
	assert.NotNil(t, reader)

	// Verify both indexes are empty
	assert.Len(t, reader.indexes, 0)
	flashcardIndexes := reader.GetFlashcardIndexes()
	assert.Len(t, flashcardIndexes, 0)
}

func TestNewReader_MultipleFlashcardIndexes(t *testing.T) {
	env := newTestFlashcardEnv(t)

	// Create first flashcard directory
	env.createFlashcardIndex("vocabulary", "Vocabulary Cards", []string{"./cards.yml"})

	// Create second flashcard directory
	env.createFlashcardIndex("idioms", "English Idioms", []string{"./idioms.yml"})

	// Test loading multiple indexes
	reader, err := NewReader([]string{}, []string{env.tempDir}, nil, nil, nil)
	require.NoError(t, err)
	assert.NotNil(t, reader)

	// Verify both indexes were loaded
	flashcardIndexes := reader.GetFlashcardIndexes()
	assert.Len(t, flashcardIndexes, 2)
	assert.Contains(t, flashcardIndexes, "vocabulary")
	assert.Contains(t, flashcardIndexes, "idioms")
	assert.Equal(t, "Vocabulary Cards", flashcardIndexes["vocabulary"].Name)
	assert.Equal(t, "English Idioms", flashcardIndexes["idioms"].Name)
}

func TestReadFlashcardNotebooks_Success(t *testing.T) {
	env := newTestFlashcardEnv(t)

	// Create flashcard directory with index.yml
	flashcardDir := env.createFlashcardIndex("vocab", "Vocabulary", []string{"./cards.yml", "./advanced.yml"})

	// Create cards.yml with multiple cards
	cardsContent := `- title: "Basic Vocabulary"
  description: "Basic English words"
  date: 2025-01-15T00:00:00Z
  cards:
    - expression: "serendipity"
      definition: "serendipity"
      meaning: "the occurrence and development of events by chance in a happy or beneficial way"
      part_of_speech: "noun"
      pronunciation: "/ˌserənˈdɪpəti/"
      examples:
        - "It was pure serendipity that we met at the coffee shop that day."
      synonyms:
        - "chance"
        - "luck"
      origin: "coined by Horace Walpole in 1754"
    - expression: "ephemeral"
      meaning: "lasting for a very short time; transitory"
`
	env.createCardFile(flashcardDir, "cards.yml", cardsContent)

	// Create advanced.yml
	advancedContent := `- title: "Advanced Vocabulary"
  date: 2025-01-16T00:00:00Z
  cards:
    - expression: "ubiquitous"
      meaning: "present, appearing, or found everywhere"
`
	env.createCardFile(flashcardDir, "advanced.yml", advancedContent)

	// Create reader and read flashcard notebooks
	reader, err := NewReader([]string{}, []string{env.tempDir}, nil, nil, nil)
	require.NoError(t, err)

	notebooks, err := reader.ReadFlashcardNotebooks("vocab")
	require.NoError(t, err)
	assert.Len(t, notebooks, 2)

	// Verify first notebook
	assert.Equal(t, "Basic Vocabulary", notebooks[0].Title)
	assert.Equal(t, "Basic English words", notebooks[0].Description)
	assert.Len(t, notebooks[0].Cards, 2)

	// Verify first card details
	card1 := notebooks[0].Cards[0]
	assert.Equal(t, "serendipity", card1.Expression)
	assert.Equal(t, "serendipity", card1.Definition)
	assert.Equal(t, "the occurrence and development of events by chance in a happy or beneficial way", card1.Meaning)
	assert.Equal(t, "noun", card1.PartOfSpeech)
	assert.Equal(t, "/ˌserənˈdɪpəti/", card1.Pronunciation)
	assert.Len(t, card1.Examples, 1)
	assert.Equal(t, "It was pure serendipity that we met at the coffee shop that day.", card1.Examples[0])
	assert.Len(t, card1.Synonyms, 2)
	assert.Contains(t, card1.Synonyms, "chance")
	assert.Contains(t, card1.Synonyms, "luck")
	assert.Equal(t, "coined by Horace Walpole in 1754", card1.Origin)

	// Verify second card (minimal fields)
	card2 := notebooks[0].Cards[1]
	assert.Equal(t, "ephemeral", card2.Expression)
	assert.Equal(t, "lasting for a very short time; transitory", card2.Meaning)

	// Verify second notebook
	assert.Equal(t, "Advanced Vocabulary", notebooks[1].Title)
	assert.Len(t, notebooks[1].Cards, 1)
	assert.Equal(t, "ubiquitous", notebooks[1].Cards[0].Expression)
}

func TestReadFlashcardNotebooks_FlashcardIDNotFound(t *testing.T) {
	env := newTestFlashcardEnv(t)

	// Create flashcard directory with index.yml
	env.createFlashcardIndex("vocab", "Vocabulary", []string{"./cards.yml"})

	// Create reader
	reader, err := NewReader([]string{}, []string{env.tempDir}, nil, nil, nil)
	require.NoError(t, err)

	// Try to read flashcard notebooks with non-existent ID
	notebooks, err := reader.ReadFlashcardNotebooks("nonexistent")
	assert.Error(t, err)
	assert.Nil(t, notebooks)
	assert.Contains(t, err.Error(), "flashcard nonexistent not found")
}

func TestReadFlashcardNotebooks_InvalidYAMLFile(t *testing.T) {
	env := newTestFlashcardEnv(t)

	// Create flashcard directory with index.yml
	flashcardDir := env.createFlashcardIndex("vocab", "Vocabulary", []string{"./invalid.yml"})

	// Create invalid YAML file
	invalidContent := `- title: "Test"
  cards: [invalid yaml structure
`
	env.createCardFile(flashcardDir, "invalid.yml", invalidContent)

	// Create reader
	reader, err := NewReader([]string{}, []string{env.tempDir}, nil, nil, nil)
	require.NoError(t, err)

	// Try to read flashcard notebooks with invalid YAML
	notebooks, err := reader.ReadFlashcardNotebooks("vocab")
	assert.Error(t, err)
	assert.Nil(t, notebooks)
	assert.Contains(t, err.Error(), "readYamlFile")
}

func TestReadFlashcardNotebooks_MissingNotebookFile(t *testing.T) {
	env := newTestFlashcardEnv(t)

	// Create flashcard directory with index.yml (missing.yml is not created)
	env.createFlashcardIndex("vocab", "Vocabulary", []string{"./missing.yml"})

	// Create reader
	reader, err := NewReader([]string{}, []string{env.tempDir}, nil, nil, nil)
	require.NoError(t, err)

	// Try to read flashcard notebooks with missing notebook file
	notebooks, err := reader.ReadFlashcardNotebooks("vocab")
	assert.Error(t, err)
	assert.Nil(t, notebooks)
	assert.Contains(t, err.Error(), "readYamlFile")
}

func TestReadFlashcardNotebooks_EmptyNotebooksList(t *testing.T) {
	env := newTestFlashcardEnv(t)

	// Create flashcard directory with index.yml with empty notebooks list
	env.createFlashcardIndex("vocab", "Vocabulary", []string{})

	// Create reader
	reader, err := NewReader([]string{}, []string{env.tempDir}, nil, nil, nil)
	require.NoError(t, err)

	// Read flashcard notebooks with empty notebooks list
	notebooks, err := reader.ReadFlashcardNotebooks("vocab")
	require.NoError(t, err)
	assert.Len(t, notebooks, 0)
}

func TestReadFlashcardNotebooks_WithImages(t *testing.T) {
	env := newTestFlashcardEnv(t)

	// Create flashcard directory with index.yml
	flashcardDir := env.createFlashcardIndex("visual", "Visual Vocabulary", []string{"./cards.yml"})

	// Create cards.yml with image field
	cardsContent := `- title: "Visual Vocabulary"
  date: 2025-01-15T00:00:00Z
  cards:
    - expression: "juxtapose"
      definition: "juxtapose"
      meaning: "to place or deal with close together for contrasting effect"
      part_of_speech: "verb"
      images:
        - "https://example.com/juxtapose-art-example.jpg"
`
	env.createCardFile(flashcardDir, "cards.yml", cardsContent)

	// Create reader and read flashcard notebooks
	reader, err := NewReader([]string{}, []string{env.tempDir}, nil, nil, nil)
	require.NoError(t, err)

	notebooks, err := reader.ReadFlashcardNotebooks("visual")
	require.NoError(t, err)
	assert.Len(t, notebooks, 1)
	assert.Len(t, notebooks[0].Cards, 1)

	// Verify image field
	card := notebooks[0].Cards[0]
	assert.Equal(t, "juxtapose", card.Expression)
	assert.Len(t, card.Images, 1)
	assert.Equal(t, "https://example.com/juxtapose-art-example.jpg", card.Images[0])
}

func TestReadAllFlashcardNotebooks(t *testing.T) {
	env := newTestFlashcardEnv(t)

	// Create first flashcard directory
	flashcardDir1 := env.createFlashcardIndex("vocabulary", "Vocabulary Cards", []string{"./cards.yml"})

	cards1Content := `- title: "Basic Words"
  date: 2025-01-15T00:00:00Z
  cards:
    - expression: "test"
      meaning: "a test word"
`
	env.createCardFile(flashcardDir1, "cards.yml", cards1Content)

	// Create second flashcard directory
	flashcardDir2 := env.createFlashcardIndex("idioms", "English Idioms", []string{"./idioms.yml"})

	idioms2Content := `- title: "Common Idioms"
  date: 2025-01-16T00:00:00Z
  cards:
    - expression: "break the ice"
      meaning: "to initiate social interaction"
`
	env.createCardFile(flashcardDir2, "idioms.yml", idioms2Content)

	// Create reader
	reader, err := NewReader([]string{}, []string{env.tempDir}, nil, nil, nil)
	require.NoError(t, err)

	// Read all flashcard notebooks
	result, err := reader.ReadAllFlashcardNotebooks()
	require.NoError(t, err)
	assert.Len(t, result, 2)

	// Verify both indexes have loaded notebooks
	assert.Contains(t, result, "vocabulary")
	assert.Contains(t, result, "idioms")
	assert.Len(t, result["vocabulary"].Notebooks, 1)
	assert.Len(t, result["idioms"].Notebooks, 1)
	assert.Equal(t, "Basic Words", result["vocabulary"].Notebooks[0].Title)
	assert.Equal(t, "Common Idioms", result["idioms"].Notebooks[0].Title)
}

func TestReadAllFlashcardNotebooks_OneInvalidNotebook(t *testing.T) {
	env := newTestFlashcardEnv(t)

	// Create first flashcard directory (valid)
	flashcardDir1 := env.createFlashcardIndex("vocabulary", "Vocabulary Cards", []string{"./cards.yml"})

	cards1Content := `- title: "Basic Words"
  date: 2025-01-15T00:00:00Z
  cards:
    - expression: "test"
      meaning: "a test word"
`
	env.createCardFile(flashcardDir1, "cards.yml", cards1Content)

	// Create second flashcard directory (invalid - missing file)
	env.createFlashcardIndex("idioms", "English Idioms", []string{"./missing.yml"})

	// Create reader
	reader, err := NewReader([]string{}, []string{env.tempDir}, nil, nil, nil)
	require.NoError(t, err)

	// Read all flashcard notebooks - should fail on the invalid one
	result, err := reader.ReadAllFlashcardNotebooks()
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "ReadFlashcardNotebooks")
}

func TestFlashcardNotebook_DateParsing(t *testing.T) {
	env := newTestFlashcardEnv(t)

	// Create flashcard directory
	flashcardDir := env.createFlashcardIndex("vocab", "Vocabulary", []string{"./cards.yml"})

	// Create cards.yml with specific date
	cardsContent := `- title: "Test Cards"
  date: 2025-01-15T14:30:00Z
  cards:
    - expression: "test"
      meaning: "a test word"
`
	env.createCardFile(flashcardDir, "cards.yml", cardsContent)

	// Create reader and read notebooks
	reader, err := NewReader([]string{}, []string{env.tempDir}, nil, nil, nil)
	require.NoError(t, err)

	notebooks, err := reader.ReadFlashcardNotebooks("vocab")
	require.NoError(t, err)
	assert.Len(t, notebooks, 1)

	// Verify date was parsed correctly
	expectedDate := time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC)
	assert.Equal(t, expectedDate, notebooks[0].Date)
}

func TestGetFlashcardIndexes(t *testing.T) {
	env := newTestFlashcardEnv(t)

	// Create flashcard directory
	env.createFlashcardIndex("vocab", "Vocabulary", []string{"./cards.yml"})

	// Create reader
	reader, err := NewReader([]string{}, []string{env.tempDir}, nil, nil, nil)
	require.NoError(t, err)

	// Get flashcard indexes
	indexes := reader.GetFlashcardIndexes()
	assert.Len(t, indexes, 1)
	assert.Contains(t, indexes, "vocab")
	assert.Equal(t, "Vocabulary", indexes["vocab"].Name)
	assert.Equal(t, []string{"./cards.yml"}, indexes["vocab"].NotebookPaths)
}

func TestReader_GetStoryIndexes(t *testing.T) {
	env := newTestFlashcardEnv(t)

	// Create story directory with index.yml
	storyDir := env.createStoryIndex("my-stories", "My Stories", []string{"./notebook.yml"})

	reader, err := NewReader([]string{storyDir}, []string{}, nil, nil, nil)
	require.NoError(t, err)

	indexes := reader.GetStoryIndexes()
	assert.Len(t, indexes, 1)
	assert.Contains(t, indexes, "my-stories")
	assert.Equal(t, "My Stories", indexes["my-stories"].Name)
}

func TestReader_GetStoryIndexes_Empty(t *testing.T) {
	reader, err := NewReader([]string{}, []string{}, nil, nil, nil)
	require.NoError(t, err)

	indexes := reader.GetStoryIndexes()
	assert.Len(t, indexes, 0)
}

func TestReader_ReadAllStoryNotebooks(t *testing.T) {
	env := newTestFlashcardEnv(t)

	// Create story directory
	storyDir := env.createStoryIndex("test-story", "Test Story", []string{"./episodes.yml"})

	// Create episode YAML
	episodesContent := `- event: "Episode 1"
  date: 2025-01-15T00:00:00Z
  scenes:
    - scene: "Opening"
      conversations:
        - speaker: "A"
          quote: "Hello there"
      definitions:
        - expression: "greeting"
          meaning: "a word of welcome"
`
	env.createCardFile(storyDir, "episodes.yml", episodesContent)

	reader, err := NewReader([]string{env.tempDir}, []string{}, nil, nil, nil)
	require.NoError(t, err)

	result, err := reader.ReadAllStoryNotebooks()
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Contains(t, result, "test-story")
}

func TestReader_ReadAllNotes(t *testing.T) {
	env := newTestFlashcardEnv(t)

	// Create story directory
	storyDir := env.createStoryIndex("test-story", "Test Story", []string{"./episodes.yml"})

	// Create episode YAML with a definition that needs to learn
	episodesContent := `- event: "Episode 1"
  date: 2025-01-15T00:00:00Z
  scenes:
    - scene: "Opening"
      conversations:
        - speaker: "A"
          quote: "The {{ arduous }} task continued"
      definitions:
        - expression: "arduous"
          meaning: "involving great effort"
`
	env.createCardFile(storyDir, "episodes.yml", episodesContent)

	reader, err := NewReader([]string{env.tempDir}, []string{}, nil, nil, nil)
	require.NoError(t, err)

	// Test with learning history that marks expression as needing learn
	learningHistories := map[string][]LearningHistory{
		"test-story": {
			{
				Metadata: LearningHistoryMetadata{Title: "Episode 1"},
				Scenes: []LearningScene{
					{
						Metadata: LearningSceneMetadata{Title: "Opening"},
						Expressions: []LearningHistoryExpression{
							{
								Expression: "arduous",
								LearnedLogs: []LearningRecord{
									{
										Status:    LearnedStatusMisunderstood,
										LearnedAt: NewDate(time.Now().Add(-24 * time.Hour)),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	notes, err := reader.ReadAllNotes("test-story", learningHistories)
	require.NoError(t, err)
	assert.Greater(t, len(notes), 0)
}

func TestIndex_GetNotebookPath(t *testing.T) {
	index := Index{
		path:          "/base/path",
		NotebookPaths: []string{"chapter1.yml", "chapter2.yml"},
	}

	got := index.GetNotebookPath(0)
	assert.Equal(t, "/base/path/chapter1.yml", got)

	got = index.GetNotebookPath(1)
	assert.Equal(t, "/base/path/chapter2.yml", got)
}

func TestReader_IsBook(t *testing.T) {
	env := newTestFlashcardEnv(t)

	// Create a stories directory
	storiesDir := filepath.Join(env.tempDir, "stories")
	err := os.MkdirAll(storiesDir, 0755)
	require.NoError(t, err)

	// Create a books directory
	booksDir := filepath.Join(env.tempDir, "books")
	err = os.MkdirAll(booksDir, 0755)
	require.NoError(t, err)

	// Create story index
	storyDir := filepath.Join(storiesDir, "friends")
	err = os.MkdirAll(storyDir, 0755)
	require.NoError(t, err)
	storyIndex := `id: friends
name: "Friends Episodes"
notebooks:
  - episode1.yml
`
	err = os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(storyIndex), 0644)
	require.NoError(t, err)

	// Create book index
	bookDir := filepath.Join(booksDir, "frankenstein")
	err = os.MkdirAll(bookDir, 0755)
	require.NoError(t, err)
	bookIndex := `id: frankenstein
name: "Frankenstein"
notebooks:
  - chapter1.yml
`
	err = os.WriteFile(filepath.Join(bookDir, "index.yml"), []byte(bookIndex), 0644)
	require.NoError(t, err)

	// Create reader with both directories
	reader, err := NewReader([]string{storiesDir}, nil, []string{booksDir}, nil, nil)
	require.NoError(t, err)

	// Test IsBook
	assert.False(t, reader.IsBook("friends"), "friends should not be a book")
	assert.True(t, reader.IsBook("frankenstein"), "frankenstein should be a book")
	assert.False(t, reader.IsBook("unknown"), "unknown should return false")
}
