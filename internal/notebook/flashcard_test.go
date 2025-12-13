package notebook

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewReader_EmptyStoriesDirValidFlashcardDir(t *testing.T) {
	// This test covers the bug fix where passing "" for stories dir caused a crash
	tempDir, err := os.MkdirTemp("", "flashcard_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create flashcard directory with index.yml
	flashcardDir := filepath.Join(tempDir, "flashcards")
	err = os.MkdirAll(flashcardDir, 0755)
	require.NoError(t, err)

	indexContent := `id: vocab
name: "Vocabulary"
notebooks:
  - ./cards.yml
`
	indexPath := filepath.Join(flashcardDir, "index.yml")
	err = os.WriteFile(indexPath, []byte(indexContent), 0644)
	require.NoError(t, err)

	// Create a simple cards.yml
	cardsContent := `- title: "Test Cards"
  description: "Test flashcards"
  date: 2025-01-15T00:00:00Z
  cards:
    - expression: "test"
      meaning: "a test word"
`
	cardsPath := filepath.Join(flashcardDir, "cards.yml")
	err = os.WriteFile(cardsPath, []byte(cardsContent), 0644)
	require.NoError(t, err)

	// Test with empty string for stories directory
	reader, err := NewReader("", flashcardDir, nil)
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
	tempDir, err := os.MkdirTemp("", "flashcard_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create story directory with index.yml
	storyDir := filepath.Join(tempDir, "stories")
	err = os.MkdirAll(storyDir, 0755)
	require.NoError(t, err)

	indexContent := `id: story1
name: "Story One"
notebooks:
  - ./notebook.yml
`
	indexPath := filepath.Join(storyDir, "index.yml")
	err = os.WriteFile(indexPath, []byte(indexContent), 0644)
	require.NoError(t, err)

	// Test with empty string for flashcard directory
	reader, err := NewReader(storyDir, "", nil)
	require.NoError(t, err)
	assert.NotNil(t, reader)

	// Verify story indexes were loaded
	assert.Len(t, reader.indexes, 1)
	assert.Contains(t, reader.indexes, "story1")

	// Verify flashcard indexes are empty
	flashcardIndexes := reader.GetFlashcardIndexes()
	assert.Len(t, flashcardIndexes, 0)
}

func TestNewReader_BothDirectoriesEmpty(t *testing.T) {
	// Test with both directories as empty strings
	reader, err := NewReader("", "", nil)
	require.NoError(t, err)
	assert.NotNil(t, reader)

	// Verify both indexes are empty
	assert.Len(t, reader.indexes, 0)
	flashcardIndexes := reader.GetFlashcardIndexes()
	assert.Len(t, flashcardIndexes, 0)
}

func TestNewReader_MultipleFlashcardIndexes(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "flashcard_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create first flashcard directory
	flashcardDir1 := filepath.Join(tempDir, "vocab")
	err = os.MkdirAll(flashcardDir1, 0755)
	require.NoError(t, err)

	index1Content := `id: vocabulary
name: "Vocabulary Cards"
notebooks:
  - ./cards.yml
`
	err = os.WriteFile(filepath.Join(flashcardDir1, "index.yml"), []byte(index1Content), 0644)
	require.NoError(t, err)

	// Create second flashcard directory
	flashcardDir2 := filepath.Join(tempDir, "idioms")
	err = os.MkdirAll(flashcardDir2, 0755)
	require.NoError(t, err)

	index2Content := `id: idioms
name: "English Idioms"
notebooks:
  - ./idioms.yml
`
	err = os.WriteFile(filepath.Join(flashcardDir2, "index.yml"), []byte(index2Content), 0644)
	require.NoError(t, err)

	// Test loading multiple indexes
	reader, err := NewReader("", tempDir, nil)
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
	tempDir, err := os.MkdirTemp("", "flashcard_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create flashcard directory with index.yml
	flashcardDir := filepath.Join(tempDir, "flashcards")
	err = os.MkdirAll(flashcardDir, 0755)
	require.NoError(t, err)

	indexContent := `id: vocab
name: "Vocabulary"
notebooks:
  - ./cards.yml
  - ./advanced.yml
`
	indexPath := filepath.Join(flashcardDir, "index.yml")
	err = os.WriteFile(indexPath, []byte(indexContent), 0644)
	require.NoError(t, err)

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
	cardsPath := filepath.Join(flashcardDir, "cards.yml")
	err = os.WriteFile(cardsPath, []byte(cardsContent), 0644)
	require.NoError(t, err)

	// Create advanced.yml
	advancedContent := `- title: "Advanced Vocabulary"
  date: 2025-01-16T00:00:00Z
  cards:
    - expression: "ubiquitous"
      meaning: "present, appearing, or found everywhere"
`
	advancedPath := filepath.Join(flashcardDir, "advanced.yml")
	err = os.WriteFile(advancedPath, []byte(advancedContent), 0644)
	require.NoError(t, err)

	// Create reader and read flashcard notebooks
	reader, err := NewReader("", flashcardDir, nil)
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
	tempDir, err := os.MkdirTemp("", "flashcard_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create flashcard directory with index.yml
	flashcardDir := filepath.Join(tempDir, "flashcards")
	err = os.MkdirAll(flashcardDir, 0755)
	require.NoError(t, err)

	indexContent := `id: vocab
name: "Vocabulary"
notebooks:
  - ./cards.yml
`
	indexPath := filepath.Join(flashcardDir, "index.yml")
	err = os.WriteFile(indexPath, []byte(indexContent), 0644)
	require.NoError(t, err)

	// Create reader
	reader, err := NewReader("", flashcardDir, nil)
	require.NoError(t, err)

	// Try to read flashcard notebooks with non-existent ID
	notebooks, err := reader.ReadFlashcardNotebooks("nonexistent")
	assert.Error(t, err)
	assert.Nil(t, notebooks)
	assert.Contains(t, err.Error(), "flashcard nonexistent not found")
}

func TestReadFlashcardNotebooks_InvalidYAMLFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "flashcard_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create flashcard directory with index.yml
	flashcardDir := filepath.Join(tempDir, "flashcards")
	err = os.MkdirAll(flashcardDir, 0755)
	require.NoError(t, err)

	indexContent := `id: vocab
name: "Vocabulary"
notebooks:
  - ./invalid.yml
`
	indexPath := filepath.Join(flashcardDir, "index.yml")
	err = os.WriteFile(indexPath, []byte(indexContent), 0644)
	require.NoError(t, err)

	// Create invalid YAML file
	invalidContent := `- title: "Test"
  cards: [invalid yaml structure
`
	invalidPath := filepath.Join(flashcardDir, "invalid.yml")
	err = os.WriteFile(invalidPath, []byte(invalidContent), 0644)
	require.NoError(t, err)

	// Create reader
	reader, err := NewReader("", flashcardDir, nil)
	require.NoError(t, err)

	// Try to read flashcard notebooks with invalid YAML
	notebooks, err := reader.ReadFlashcardNotebooks("vocab")
	assert.Error(t, err)
	assert.Nil(t, notebooks)
	assert.Contains(t, err.Error(), "readYamlFile")
}

func TestReadFlashcardNotebooks_MissingNotebookFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "flashcard_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create flashcard directory with index.yml
	flashcardDir := filepath.Join(tempDir, "flashcards")
	err = os.MkdirAll(flashcardDir, 0755)
	require.NoError(t, err)

	indexContent := `id: vocab
name: "Vocabulary"
notebooks:
  - ./missing.yml
`
	indexPath := filepath.Join(flashcardDir, "index.yml")
	err = os.WriteFile(indexPath, []byte(indexContent), 0644)
	require.NoError(t, err)

	// Create reader (missing.yml is not created)
	reader, err := NewReader("", flashcardDir, nil)
	require.NoError(t, err)

	// Try to read flashcard notebooks with missing notebook file
	notebooks, err := reader.ReadFlashcardNotebooks("vocab")
	assert.Error(t, err)
	assert.Nil(t, notebooks)
	assert.Contains(t, err.Error(), "readYamlFile")
}

func TestReadFlashcardNotebooks_EmptyNotebooksList(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "flashcard_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create flashcard directory with index.yml
	flashcardDir := filepath.Join(tempDir, "flashcards")
	err = os.MkdirAll(flashcardDir, 0755)
	require.NoError(t, err)

	indexContent := `id: vocab
name: "Vocabulary"
notebooks: []
`
	indexPath := filepath.Join(flashcardDir, "index.yml")
	err = os.WriteFile(indexPath, []byte(indexContent), 0644)
	require.NoError(t, err)

	// Create reader
	reader, err := NewReader("", flashcardDir, nil)
	require.NoError(t, err)

	// Read flashcard notebooks with empty notebooks list
	notebooks, err := reader.ReadFlashcardNotebooks("vocab")
	require.NoError(t, err)
	assert.Len(t, notebooks, 0)
}

func TestReadFlashcardNotebooks_WithImages(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "flashcard_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create flashcard directory with index.yml
	flashcardDir := filepath.Join(tempDir, "flashcards")
	err = os.MkdirAll(flashcardDir, 0755)
	require.NoError(t, err)

	indexContent := `id: visual
name: "Visual Vocabulary"
notebooks:
  - ./cards.yml
`
	indexPath := filepath.Join(flashcardDir, "index.yml")
	err = os.WriteFile(indexPath, []byte(indexContent), 0644)
	require.NoError(t, err)

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
	cardsPath := filepath.Join(flashcardDir, "cards.yml")
	err = os.WriteFile(cardsPath, []byte(cardsContent), 0644)
	require.NoError(t, err)

	// Create reader and read flashcard notebooks
	reader, err := NewReader("", flashcardDir, nil)
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
	tempDir, err := os.MkdirTemp("", "flashcard_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create first flashcard directory
	flashcardDir1 := filepath.Join(tempDir, "vocab")
	err = os.MkdirAll(flashcardDir1, 0755)
	require.NoError(t, err)

	index1Content := `id: vocabulary
name: "Vocabulary Cards"
notebooks:
  - ./cards.yml
`
	err = os.WriteFile(filepath.Join(flashcardDir1, "index.yml"), []byte(index1Content), 0644)
	require.NoError(t, err)

	cards1Content := `- title: "Basic Words"
  date: 2025-01-15T00:00:00Z
  cards:
    - expression: "test"
      meaning: "a test word"
`
	err = os.WriteFile(filepath.Join(flashcardDir1, "cards.yml"), []byte(cards1Content), 0644)
	require.NoError(t, err)

	// Create second flashcard directory
	flashcardDir2 := filepath.Join(tempDir, "idioms")
	err = os.MkdirAll(flashcardDir2, 0755)
	require.NoError(t, err)

	index2Content := `id: idioms
name: "English Idioms"
notebooks:
  - ./idioms.yml
`
	err = os.WriteFile(filepath.Join(flashcardDir2, "index.yml"), []byte(index2Content), 0644)
	require.NoError(t, err)

	idioms2Content := `- title: "Common Idioms"
  date: 2025-01-16T00:00:00Z
  cards:
    - expression: "break the ice"
      meaning: "to initiate social interaction"
`
	err = os.WriteFile(filepath.Join(flashcardDir2, "idioms.yml"), []byte(idioms2Content), 0644)
	require.NoError(t, err)

	// Create reader
	reader, err := NewReader("", tempDir, nil)
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
	tempDir, err := os.MkdirTemp("", "flashcard_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create first flashcard directory (valid)
	flashcardDir1 := filepath.Join(tempDir, "vocab")
	err = os.MkdirAll(flashcardDir1, 0755)
	require.NoError(t, err)

	index1Content := `id: vocabulary
name: "Vocabulary Cards"
notebooks:
  - ./cards.yml
`
	err = os.WriteFile(filepath.Join(flashcardDir1, "index.yml"), []byte(index1Content), 0644)
	require.NoError(t, err)

	cards1Content := `- title: "Basic Words"
  date: 2025-01-15T00:00:00Z
  cards:
    - expression: "test"
      meaning: "a test word"
`
	err = os.WriteFile(filepath.Join(flashcardDir1, "cards.yml"), []byte(cards1Content), 0644)
	require.NoError(t, err)

	// Create second flashcard directory (invalid - missing file)
	flashcardDir2 := filepath.Join(tempDir, "idioms")
	err = os.MkdirAll(flashcardDir2, 0755)
	require.NoError(t, err)

	index2Content := `id: idioms
name: "English Idioms"
notebooks:
  - ./missing.yml
`
	err = os.WriteFile(filepath.Join(flashcardDir2, "index.yml"), []byte(index2Content), 0644)
	require.NoError(t, err)

	// Create reader
	reader, err := NewReader("", tempDir, nil)
	require.NoError(t, err)

	// Read all flashcard notebooks - should fail on the invalid one
	result, err := reader.ReadAllFlashcardNotebooks()
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "ReadFlashcardNotebooks")
}

func TestFlashcardNotebook_DateParsing(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "flashcard_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create flashcard directory
	flashcardDir := filepath.Join(tempDir, "flashcards")
	err = os.MkdirAll(flashcardDir, 0755)
	require.NoError(t, err)

	indexContent := `id: vocab
name: "Vocabulary"
notebooks:
  - ./cards.yml
`
	err = os.WriteFile(filepath.Join(flashcardDir, "index.yml"), []byte(indexContent), 0644)
	require.NoError(t, err)

	// Create cards.yml with specific date
	cardsContent := `- title: "Test Cards"
  date: 2025-01-15T14:30:00Z
  cards:
    - expression: "test"
      meaning: "a test word"
`
	err = os.WriteFile(filepath.Join(flashcardDir, "cards.yml"), []byte(cardsContent), 0644)
	require.NoError(t, err)

	// Create reader and read notebooks
	reader, err := NewReader("", flashcardDir, nil)
	require.NoError(t, err)

	notebooks, err := reader.ReadFlashcardNotebooks("vocab")
	require.NoError(t, err)
	assert.Len(t, notebooks, 1)

	// Verify date was parsed correctly
	expectedDate := time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC)
	assert.Equal(t, expectedDate, notebooks[0].Date)
}

func TestGetFlashcardIndexes(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "flashcard_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create flashcard directory
	flashcardDir := filepath.Join(tempDir, "flashcards")
	err = os.MkdirAll(flashcardDir, 0755)
	require.NoError(t, err)

	indexContent := `id: vocab
name: "Vocabulary"
notebooks:
  - ./cards.yml
`
	err = os.WriteFile(filepath.Join(flashcardDir, "index.yml"), []byte(indexContent), 0644)
	require.NoError(t, err)

	// Create reader
	reader, err := NewReader("", flashcardDir, nil)
	require.NoError(t, err)

	// Get flashcard indexes
	indexes := reader.GetFlashcardIndexes()
	assert.Len(t, indexes, 1)
	assert.Contains(t, indexes, "vocab")
	assert.Equal(t, "Vocabulary", indexes["vocab"].Name)
	assert.Equal(t, []string{"./cards.yml"}, indexes["vocab"].NotebookPaths)
}
