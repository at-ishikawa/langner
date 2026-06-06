package quiz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/learning"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// createStoryFixtures creates story notebook fixtures in storiesDir with a learning file in learningDir.
func createStoryFixtures(t *testing.T, storiesDir, learningDir string) {
	t.Helper()
	storyDir := filepath.Join(storiesDir, "test-story")
	require.NoError(t, os.MkdirAll(storyDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(`id: test-story
name: Test Story
notebooks:
  - ./episodes.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "episodes.yml"), []byte(`- event: "Chapter One"
  date: 2025-01-15T00:00:00Z
  scenes:
    - scene: "Opening"
      conversations:
        - speaker: "Alice"
          quote: "That sounds preposterous to me."
      definitions:
        - expression: "preposterous"
          meaning: "contrary to reason or common sense"
`), 0644))
	// Write a learning history file so the notebook is recognised
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-story.yml"), []byte(`- metadata:
    notebook_id: test-story
    title: "Chapter One"
  scenes:
    - metadata:
        title: "Opening"
      expressions:
        - expression: "preposterous"
          learned_logs:
            - status: "misunderstood"
              learned_at: "2025-01-14"
              quiz_type: "freeform"
`), 0644))
}

// createFlashcardFixtures creates flashcard notebook fixtures in flashcardsDir with a learning file in learningDir.
func createFlashcardFixtures(t *testing.T, flashcardsDir, learningDir string) {
	t.Helper()
	vocabDir := filepath.Join(flashcardsDir, "test-vocab")
	require.NoError(t, os.MkdirAll(vocabDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "index.yml"), []byte(`id: test-vocab
name: Test Vocabulary
notebooks:
  - ./cards.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "cards.yml"), []byte(`- title: "Basic Words"
  date: 2025-01-15T00:00:00Z
  cards:
    - expression: "serendipity"
      meaning: "a fortunate discovery by accident"
      examples:
        - "It was pure serendipity that they met."
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-vocab.yml"), []byte(`- metadata:
    notebook_id: test-vocab
    title: "Basic Words"
    type: "flashcard"
  expressions:
    - expression: "serendipity"
      learned_logs:
        - status: "misunderstood"
          learned_at: "2025-01-14"
          quiz_type: "freeform"
`), 0644))
}

func newTestService(t *testing.T, openaiClient inference.Client) *Service {
	t.Helper()
	learningDir := t.TempDir()
	return NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{t.TempDir()},
		LearningNotesDirectory: learningDir,
	}, openaiClient, make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})
}

func newTestServiceWithFixtures(t *testing.T, openaiClient inference.Client) (*Service, string) {
	t.Helper()
	storiesDir := t.TempDir()
	flashcardsDir := t.TempDir()
	learningDir := t.TempDir()
	createStoryFixtures(t, storiesDir, learningDir)
	createFlashcardFixtures(t, flashcardsDir, learningDir)
	svc := NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{storiesDir},
		FlashcardsDirectories:  []string{flashcardsDir},
		LearningNotesDirectory: learningDir,
	}, openaiClient, make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})
	return svc, learningDir
}

// ---------- LoadNotebookSummaries ----------

func TestService_LoadNotebookSummaries_Empty(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := newTestService(t, mock_inference.NewMockClient(ctrl))

	summaries, err := svc.LoadNotebookSummaries(false)
	require.NoError(t, err)
	assert.Empty(t, summaries)
}

func TestService_LoadNotebookSummaries_WithFixtures(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc, _ := newTestServiceWithFixtures(t, mock_inference.NewMockClient(ctrl))

	summaries, err := svc.LoadNotebookSummaries(false)
	require.NoError(t, err)
	require.Len(t, summaries, 2)

	summaryMap := make(map[string]NotebookSummary)
	for _, s := range summaries {
		summaryMap[s.NotebookID] = s
	}

	storySummary, ok := summaryMap["test-story"]
	require.True(t, ok)
	assert.Equal(t, "Test Story", storySummary.Name)
	// Only misunderstood answers in fixture → no words with correct answers → ReviewCount=0
	assert.Equal(t, 0, storySummary.ReviewCount)
	// Only misunderstood answers in fixture → no words eligible for reverse quiz
	assert.Equal(t, 0, storySummary.ReverseReviewCount)

	vocabSummary, ok := summaryMap["test-vocab"]
	require.True(t, ok)
	assert.Equal(t, "Test Vocabulary", vocabSummary.Name)
	// Flashcard fixture has serendipity with only a misunderstood
	// freeform log → no correct answer → ReviewCount=0. Previously
	// FilterFlashcardNotebooks lacked the has-correct gate and this
	// asserted 1 (the bug the user reported); the fix aligns flashcards
	// with story notebooks.
	assert.Equal(t, 0, vocabSummary.ReviewCount)
	assert.Equal(t, 0, vocabSummary.ReverseReviewCount)
}

func TestService_LoadNotebookSummaries_ReverseReviewCount(t *testing.T) {
	ctrl := gomock.NewController(t)
	storiesDir := t.TempDir()
	flashcardsDir := t.TempDir()
	learningDir := t.TempDir()

	// Story with one word that has a correct answer (eligible for reverse)
	storyDir := filepath.Join(storiesDir, "my-story")
	require.NoError(t, os.MkdirAll(storyDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(`id: my-story
name: My Story
notebooks:
  - ./episodes.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "episodes.yml"), []byte(`- event: "Episode 1"
  date: 2025-01-15T00:00:00Z
  scenes:
    - scene: "Scene 1"
      conversations:
        - speaker: "Bob"
          quote: "He lost his temper completely."
      definitions:
        - expression: "lose one's temper"
          meaning: "to become very angry"
        - expression: "break the ice"
          meaning: "to initiate social interaction"
`), 0644))
	// Learning history: "lose one's temper" has a correct answer, "break the ice" does not
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "my-story.yml"), []byte(`- metadata:
    notebook_id: my-story
    title: "Episode 1"
  scenes:
    - metadata:
        title: "Scene 1"
      expressions:
        - expression: "lose one's temper"
          learned_logs:
            - status: "usable"
              learned_at: "2025-01-14"
              interval_days: 1
              quiz_type: "freeform"
        - expression: "break the ice"
          learned_logs:
            - status: "misunderstood"
              learned_at: "2025-01-14"
              quiz_type: "freeform"
`), 0644))

	// Flashcard with one word that has a correct answer
	vocabDir := filepath.Join(flashcardsDir, "my-vocab")
	require.NoError(t, os.MkdirAll(vocabDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "index.yml"), []byte(`id: my-vocab
name: My Vocab
notebooks:
  - ./cards.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "cards.yml"), []byte(`- title: "Common Phrases"
  date: 2025-01-15T00:00:00Z
  cards:
    - expression: "serendipity"
      meaning: "a fortunate discovery by accident"
    - expression: "ephemeral"
      meaning: "lasting for a very short time"
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "my-vocab.yml"), []byte(`- metadata:
    notebook_id: my-vocab
    title: "Common Phrases"
    type: "flashcard"
  expressions:
    - expression: "serendipity"
      learned_logs:
        - status: "usable"
          learned_at: "2025-01-14"
          interval_days: 1
          quiz_type: "freeform"
    - expression: "ephemeral"
      learned_logs:
        - status: "misunderstood"
          learned_at: "2025-01-14"
          quiz_type: "freeform"
`), 0644))

	svc := NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{storiesDir},
		FlashcardsDirectories:  []string{flashcardsDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	summaries, err := svc.LoadNotebookSummaries(false)
	require.NoError(t, err)

	summaryMap := make(map[string]NotebookSummary)
	for _, s := range summaries {
		summaryMap[s.NotebookID] = s
	}

	storySummary := summaryMap["my-story"]
	assert.Equal(t, 1, storySummary.ReviewCount, "only the word with a correct answer is counted")
	assert.Equal(t, 1, storySummary.ReverseReviewCount, "only the word with a correct answer is eligible for reverse")

	vocabSummary := summaryMap["my-vocab"]
	// Same gate applies to flashcards as stories: ephemeral has only a
	// misunderstood log → not counted. Only serendipity (correct, usable)
	// makes ReviewCount.
	assert.Equal(t, 1, vocabSummary.ReviewCount)
	assert.Equal(t, 1, vocabSummary.ReverseReviewCount, "only the word with a correct answer is eligible for reverse")
}

func TestService_LoadNotebookSummaries_LearningHistoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	learningDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "broken.yml"), []byte("{{invalid yaml"), 0644))

	svc := NewService(config.NotebooksConfig{
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	_, err := svc.LoadNotebookSummaries(false)
	require.Error(t, err)
}

// ---------- LoadCards ----------

func TestService_LoadCards_StoryNotebook(t *testing.T) {
	ctrl := gomock.NewController(t)
	storiesDir := t.TempDir()
	learningDir := t.TempDir()
	createStoryFixtures(t, storiesDir, learningDir)

	svc := NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{storiesDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	cards, err := svc.LoadCards([]string{"test-story"}, true, nil)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, "preposterous", cards[0].Entry)
	assert.Equal(t, "contrary to reason or common sense", cards[0].Meaning)
	assert.Equal(t, "test-story", cards[0].NotebookName)
	assert.Equal(t, "Chapter One", cards[0].StoryTitle)
	assert.Equal(t, "Opening", cards[0].SceneTitle)
	assert.NotEmpty(t, cards[0].Examples)
}

func TestService_LoadCards_FlashcardNotebook(t *testing.T) {
	ctrl := gomock.NewController(t)
	flashcardsDir := t.TempDir()
	learningDir := t.TempDir()
	createFlashcardFixtures(t, flashcardsDir, learningDir)

	svc := NewService(config.NotebooksConfig{
		FlashcardsDirectories:  []string{flashcardsDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	cards, err := svc.LoadCards([]string{"test-vocab"}, true, nil)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, "serendipity", cards[0].Entry)
	assert.Equal(t, "a fortunate discovery by accident", cards[0].Meaning)
	assert.Equal(t, "test-vocab", cards[0].NotebookName)
	assert.Equal(t, "flashcards", cards[0].StoryTitle)
	assert.Empty(t, cards[0].SceneTitle)
	require.Len(t, cards[0].Examples, 1)
	assert.Equal(t, "It was pure serendipity that they met.", cards[0].Examples[0].Text)
}

// TestService_LoadCards_FlashcardNotebook_HidesFreeformWrongWhenNotIncludingUnstudied
// pins the gate: a flashcard the user only ever freeform-failed (no
// correct answer anywhere) must NOT appear in the standard quiz unless
// the user enables "Include unstudied words". The previous filter
// always served misunderstood words, contradicting the rule that
// applies to story notebooks.
func TestService_LoadCards_FlashcardNotebook_HidesFreeformWrongWhenNotIncludingUnstudied(t *testing.T) {
	ctrl := gomock.NewController(t)
	flashcardsDir := t.TempDir()
	learningDir := t.TempDir()

	vocabDir := filepath.Join(flashcardsDir, "fail-vocab")
	require.NoError(t, os.MkdirAll(vocabDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "index.yml"), []byte(`id: fail-vocab
name: Fail Vocab
notebooks:
  - ./cards.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "cards.yml"), []byte(`- title: "Basic Words"
  date: 2025-01-15T00:00:00Z
  cards:
    - expression: "serendipity"
      meaning: "a fortunate discovery by accident"
`), 0644))
	// Freeform answered — but with status=misunderstood. No correct
	// answer recorded anywhere.
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "fail-vocab.yml"), []byte(`- metadata:
    notebook_id: fail-vocab
    title: "Basic Words"
    type: "flashcard"
  expressions:
    - expression: "serendipity"
      learned_logs:
        - status: "misunderstood"
          learned_at: "2025-01-14"
          quiz_type: "freeform"
`), 0644))

	svc := NewService(config.NotebooksConfig{
		FlashcardsDirectories:  []string{flashcardsDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	// includeUnstudied=false: must NOT appear.
	cards, err := svc.LoadCards([]string{"fail-vocab"}, false, nil)
	require.NoError(t, err)
	assert.Empty(t, cards, "flashcard with only freeform-failed history must not appear when includeUnstudied=false")

	// includeUnstudied=true: appears as expected.
	cards, err = svc.LoadCards([]string{"fail-vocab"}, true, nil)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, "serendipity", cards[0].Entry)
}

// TestService_LoadCards_DefinitionsBook_HidesFreeformWrongWhenNotIncludingUnstudied
// is the definitions-only counterpart of the flashcard test. The
// previous needsDefinitionReview gate only checked HasFreeformAnswer;
// a freeform-failed word would still leak into the standard quiz.
func TestService_LoadCards_DefinitionsBook_HidesFreeformWrongWhenNotIncludingUnstudied(t *testing.T) {
	ctrl := gomock.NewController(t)
	defsDir := t.TempDir()
	learningDir := t.TempDir()

	bookDir := filepath.Join(defsDir, "fail-defs")
	require.NoError(t, os.MkdirAll(bookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "index.yml"), []byte(`id: fail-defs
notebooks:
  - ./session1.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "session1.yml"), []byte(`- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        index: 0
        title: "common idioms"
      expressions:
        - expression: "break the ice"
          meaning: "to start social interaction"
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "fail-defs.yml"), []byte(`- metadata:
    notebook_id: fail-defs
    title: "Session 1"
  scenes:
    - metadata:
        title: "common idioms"
      expressions:
        - expression: "break the ice"
          learned_logs:
            - status: "misunderstood"
              learned_at: "2025-01-14"
              quiz_type: "freeform"
`), 0644))

	svc := NewService(config.NotebooksConfig{
		DefinitionsDirectories: []string{defsDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	cards, err := svc.LoadCards([]string{"fail-defs"}, false, nil)
	require.NoError(t, err)
	assert.Empty(t, cards, "definitions-only word with only freeform-failed history must not appear when includeUnstudied=false")
}

// TestService_LoadCards_DefinitionsBook_StudiedWordRespectsSREvenWithoutFreeform
// pins the egotist-style bug. A word with correct standard/reverse
// history but no freeform answer must still be gated by its SR
// interval, regardless of the includeUnstudied toggle. The previous
// needsDefinitionReview gate short-circuited to includeUnstudied
// whenever HasFreeformAnswer was false, which meant any word the user
// had answered correctly only in standard or reverse mode was re-asked
// on every quiz session as long as the toggle was on — bypassing SR
// entirely. The fix: studied words (any correct answer in any
// direction) defer to NeedsForwardReview / NeedsReverseReview; the
// toggle only gates pristine words.
func TestService_LoadCards_DefinitionsBook_StudiedWordRespectsSREvenWithoutFreeform(t *testing.T) {
	ctrl := gomock.NewController(t)
	defsDir := t.TempDir()
	learningDir := t.TempDir()

	bookDir := filepath.Join(defsDir, "studied-defs")
	require.NoError(t, os.MkdirAll(bookDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "index.yml"), []byte(`id: studied-defs
notebooks:
  - ./session1.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "session1.yml"), []byte(`- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        index: 0
        title: "common idioms"
      expressions:
        - expression: "break the ice"
          meaning: "to start social interaction"
`), 0o644))
	// Word has a correct standard-quiz answer with interval_days=7, dated
	// just one day ago. No freeform answer, no reverse answer. SR says
	// next review is 6 days from now — the quiz must NOT include it,
	// even with includeUnstudied=true.
	yesterday := time.Now().Add(-24 * time.Hour).Format("2006-01-02T15:04:05Z07:00")
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "studied-defs.yml"), []byte(`- metadata:
    notebook_id: studied-defs
    title: "Session 1"
  scenes:
    - metadata:
        title: "common idioms"
      expressions:
        - expression: "break the ice"
          learned_logs:
            - status: "understood"
              learned_at: "`+yesterday+`"
              quality: 4
              quiz_type: "notebook"
              interval_days: 7
`), 0o644))

	svc := NewService(config.NotebooksConfig{
		DefinitionsDirectories: []string{defsDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	cards, err := svc.LoadCards([]string{"studied-defs"}, false, nil)
	require.NoError(t, err)
	assert.Empty(t, cards, "studied word inside its SR interval must not appear with includeUnstudied=false")

	cards, err = svc.LoadCards([]string{"studied-defs"}, true, nil)
	require.NoError(t, err)
	assert.Empty(t, cards, "includeUnstudied=true must NOT override SR for studied words — only pristine words are gated by the toggle")

	reverseCards, err := svc.LoadReverseCards([]string{"studied-defs"}, false, true, nil)
	require.NoError(t, err)
	// Reverse quiz: ReverseLogs is empty, so the word IS due in reverse
	// (NeedsReverseReview returns true on empty logs). Studied path
	// applies because the standard correct answer makes it studied.
	assert.Len(t, reverseCards, 1, "studied word with empty reverse logs is due in reverse regardless of toggle")
}

// TestService_LoadCards_DefinitionsBook_IncludeUnstudiedLoadsUnstudiedWords
// pins the fix for the user-reported "only 30 words in Word Power Made
// Easy even with Include unstudied on" bug. Definitions-only books
// ignored includeUnstudied entirely: loadDefinitionCards gated every
// word behind needsDefinitionReview, which excludes words that have no
// history or haven't cleared the freeform/correct gate. So the standard
// quiz only ever loaded the freeform-cleared, due subset regardless of
// the toggle. With includeUnstudied=true, never-seen words and
// freeform-failed words must now load.
func TestService_LoadCards_DefinitionsBook_IncludeUnstudiedLoadsUnstudiedWords(t *testing.T) {
	ctrl := gomock.NewController(t)
	defsDir := t.TempDir()
	learningDir := t.TempDir()

	bookDir := filepath.Join(defsDir, "mixed-defs")
	require.NoError(t, os.MkdirAll(bookDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "index.yml"), []byte(`id: mixed-defs
notebooks:
  - ./session1.yml
`), 0o644))
	// Two words: one never studied (no history), one freeform-failed.
	// Neither is eligible without includeUnstudied.
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "session1.yml"), []byte(`- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        index: 0
        title: "common idioms"
      expressions:
        - expression: "break the ice"
          meaning: "to start social interaction"
        - expression: "lose your temper"
          meaning: "to become angry"
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "mixed-defs.yml"), []byte(`- metadata:
    notebook_id: mixed-defs
    title: "Session 1"
  scenes:
    - metadata:
        title: "common idioms"
      expressions:
        - expression: "lose your temper"
          learned_logs:
            - status: "misunderstood"
              learned_at: "2025-01-14"
              quiz_type: "freeform"
`), 0o644))

	svc := NewService(config.NotebooksConfig{
		DefinitionsDirectories: []string{defsDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	// Without the toggle: neither word is eligible.
	due, err := svc.LoadCards([]string{"mixed-defs"}, false, nil)
	require.NoError(t, err)
	assert.Empty(t, due, "no word is eligible without includeUnstudied")

	// With the toggle: both the never-studied and freeform-failed word
	// must load.
	all, err := svc.LoadCards([]string{"mixed-defs"}, true, nil)
	require.NoError(t, err)
	assert.Len(t, all, 2,
		"includeUnstudied must load both the never-seen and the freeform-failed word")

	// The summary count must agree with the loaded card count so the
	// quiz start page badge isn't misleading.
	summaries, err := svc.LoadNotebookSummaries(true)
	require.NoError(t, err)
	var book *NotebookSummary
	for i := range summaries {
		if summaries[i].NotebookID == "mixed-defs" {
			book = &summaries[i]
			break
		}
	}
	require.NotNil(t, book, "mixed-defs must appear in summaries when includeUnstudied=true")
	assert.Equal(t, 2, book.ReviewCount,
		"summary ReviewCount with includeUnstudied must match the 2 cards LoadCards returns")
}

// makeDefinitionsBookSkipFixture writes a definitions-only book with
// one expression "break the ice" plus a learning_history that marks
// the expression skipped from one or more quiz types. The eligibility
// gates (HasFreeformAnswer, HasAnyCorrectAnswer) are satisfied by a
// `usable` freeform log so we can be sure the skip check is what
// filters the word out, not the eligibility prerequisites.
func makeDefinitionsBookSkipFixture(t *testing.T, skippedAt string) (defsDir, learningDir string) {
	t.Helper()
	defsDir = t.TempDir()
	learningDir = t.TempDir()

	bookDir := filepath.Join(defsDir, "skip-defs")
	require.NoError(t, os.MkdirAll(bookDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "index.yml"), []byte(`id: skip-defs
notebooks:
  - ./session1.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "session1.yml"), []byte(`- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        index: 0
        title: "common idioms"
      expressions:
        - expression: "break the ice"
          meaning: "to start social interaction"
`), 0o644))

	// The learning-history scene title matches the HUMAN scene title
	// ("common idioms") because the quiz now reads definitions through
	// GetDefinitionsNotesByTitle — the same human-title keying the
	// notebook-detail page and the skip-write path use. (Pre-fix the
	// loader keyed by "__index_N"; that split skips and logs across two
	// scene entries, which is the bug this convergence resolves.)
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "skip-defs.yml"), []byte(`- metadata:
    notebook_id: skip-defs
    title: "Session 1"
  scenes:
    - metadata:
        title: "common idioms"
      expressions:
        - expression: "break the ice"
          learned_logs:
            - status: "usable"
              learned_at: "2025-01-14T10:00:00Z"
              quality: 4
              interval_days: 7
              quiz_type: "freeform"
          reverse_logs:
            - status: "understood"
              learned_at: "2025-01-15T10:00:00Z"
              quality: 4
              interval_days: 1
              quiz_type: "reverse"
          skipped_at:
`+skippedAt), 0o644))
	return defsDir, learningDir
}

// TestService_LoadCards_DefinitionsBook_RespectsNotebookSkip pins the
// behaviour that motivated the loadDefinitionCards skip-gate fix. A
// word skipped from the `notebook` quiz type must not appear in
// LoadCards results for a definitions-only book — the gate was missing
// before, so a user-skipped word kept reappearing in standard quizzes.
func TestService_LoadCards_DefinitionsBook_RespectsNotebookSkip(t *testing.T) {
	ctrl := gomock.NewController(t)
	defsDir, learningDir := makeDefinitionsBookSkipFixture(t,
		`            notebook: "2025-01-20T10:00:00Z"`+"\n")

	svc := NewService(config.NotebooksConfig{
		DefinitionsDirectories: []string{defsDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	cards, err := svc.LoadCards([]string{"skip-defs"}, true, nil)
	require.NoError(t, err)
	assert.Empty(t, cards, "definitions-only word with notebook skip must not appear in LoadCards")
}

// TestService_LoadReverseCards_DefinitionsBook_RespectsReverseSkip is
// the regression test for the bug user-reported on 2026-05-24: "verb"
// skipped from reverse, yet the reverse quiz still served it. The
// loadDefinitionReverseCards function used to ignore SkippedAt entirely.
func TestService_LoadReverseCards_DefinitionsBook_RespectsReverseSkip(t *testing.T) {
	ctrl := gomock.NewController(t)
	defsDir, learningDir := makeDefinitionsBookSkipFixture(t,
		`            reverse: "2025-01-20T10:00:00Z"`+"\n")

	svc := NewService(config.NotebooksConfig{
		DefinitionsDirectories: []string{defsDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	cards, err := svc.LoadReverseCards([]string{"skip-defs"}, false, false, nil)
	require.NoError(t, err)
	assert.Empty(t, cards, "definitions-only word with reverse skip must not appear in LoadReverseCards")
}

// TestService_LoadReverseCards_DefinitionsBook_IncludeUnstudied is the
// reverse-quiz companion to the standard-quiz fix: definitions-only
// books ignored includeUnstudied in reverse mode too, so the reverse
// quiz never surfaced never-studied or not-yet-freeform-cleared words
// even with the toggle on. With includeUnstudied=true both must load;
// a reverse-skipped word stays excluded regardless of the toggle.
func TestService_LoadReverseCards_DefinitionsBook_IncludeUnstudied(t *testing.T) {
	ctrl := gomock.NewController(t)
	defsDir := t.TempDir()
	learningDir := t.TempDir()

	bookDir := filepath.Join(defsDir, "mixed-rev")
	require.NoError(t, os.MkdirAll(bookDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "index.yml"), []byte(`id: mixed-rev
notebooks:
  - ./session1.yml
`), 0o644))
	// Two never-studied words (no history) — neither eligible without
	// the toggle, both eligible with it.
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "session1.yml"), []byte(`- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        index: 0
        title: "common idioms"
      expressions:
        - expression: "break the ice"
          meaning: "to start social interaction"
        - expression: "lose your temper"
          meaning: "to become angry"
`), 0o644))

	svc := NewService(config.NotebooksConfig{
		DefinitionsDirectories: []string{defsDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	due, err := svc.LoadReverseCards([]string{"mixed-rev"}, false, false, nil)
	require.NoError(t, err)
	assert.Empty(t, due, "no reverse card is eligible without includeUnstudied")

	all, err := svc.LoadReverseCards([]string{"mixed-rev"}, false, true, nil)
	require.NoError(t, err)
	assert.Len(t, all, 2,
		"includeUnstudied must load both never-studied words into the reverse quiz")

	// Summary reverse count must agree with the loaded reverse-card count.
	summaries, err := svc.LoadNotebookSummaries(true)
	require.NoError(t, err)
	var book *NotebookSummary
	for i := range summaries {
		if summaries[i].NotebookID == "mixed-rev" {
			book = &summaries[i]
			break
		}
	}
	require.NotNil(t, book)
	assert.Equal(t, 2, book.ReverseReviewCount,
		"summary ReverseReviewCount with includeUnstudied must match the 2 reverse cards loaded")
}

// TestService_LoadDefinitionWords_RespectsFreeformSkip exercises the
// freeform side of the same fix. The free-form loader received no
// learning histories and never consulted SkippedAt before this PR.
func TestService_LoadDefinitionWords_RespectsFreeformSkip(t *testing.T) {
	defsDir, learningDir := makeDefinitionsBookSkipFixture(t,
		`            freeform: "2025-01-20T10:00:00Z"`+"\n")

	reader, err := notebook.NewReader(nil, nil, nil, []string{defsDir}, nil, nil)
	require.NoError(t, err)
	histories, err := notebook.NewLearningHistories(learningDir)
	require.NoError(t, err)

	cards := loadDefinitionWords(reader, "skip-defs", nil, histories)
	assert.Empty(t, cards, "definitions-only word with freeform skip must not appear in freeform cards")
}

func TestService_LoadCards_MultipleNotebooks(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc, _ := newTestServiceWithFixtures(t, mock_inference.NewMockClient(ctrl))

	cards, err := svc.LoadCards([]string{"test-story", "test-vocab"}, true, nil)
	require.NoError(t, err)
	assert.Len(t, cards, 2)
}

func TestService_LoadCards_NotFoundError(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := newTestService(t, mock_inference.NewMockClient(ctrl))

	_, err := svc.LoadCards([]string{"non-existent"}, true, nil)
	require.Error(t, err)
	var notFoundErr *NotFoundError
	require.ErrorAs(t, err, &notFoundErr)
	assert.Equal(t, "non-existent", notFoundErr.NotebookID)
}

func TestService_LoadCards_DefinitionFieldUsedAsEntry(t *testing.T) {
	ctrl := gomock.NewController(t)
	storiesDir := t.TempDir()
	learningDir := t.TempDir()

	storyDir := filepath.Join(storiesDir, "test-story")
	require.NoError(t, os.MkdirAll(storyDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(`id: test-story
name: Test Story
notebooks:
  - ./episodes.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "episodes.yml"), []byte(`- event: "Chapter One"
  date: 2025-01-15T00:00:00Z
  scenes:
    - scene: "Opening"
      conversations:
        - speaker: "Alice"
          quote: "He ran away quickly."
      definitions:
        - expression: "run"
          definition: "ran"
          meaning: "to move swiftly on foot"
`), 0644))
	// Word must have a freeform answer to be eligible for standard quiz.
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-story.yml"), []byte(`- metadata:
    notebook_id: test-story
    title: "Chapter One"
  scenes:
    - metadata:
        title: "Opening"
      expressions:
        - expression: "run"
          learned_logs:
            - status: "misunderstood"
              learned_at: "2025-01-14"
              quiz_type: "freeform"
`), 0644))

	svc := NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{storiesDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	cards, err := svc.LoadCards([]string{"test-story"}, true, nil)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, "ran", cards[0].Entry)
}

func TestService_LoadNotebookSummaries_SectionCounts(t *testing.T) {
	ctrl := gomock.NewController(t)
	storiesDir := t.TempDir()
	learningDir := t.TempDir()

	storyDir := filepath.Join(storiesDir, "two-eps")
	require.NoError(t, os.MkdirAll(storyDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(`id: two-eps
name: Two Episodes
notebooks:
  - ./episodes.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "episodes.yml"), []byte(`- event: "Episode 1"
  date: 2025-01-15T00:00:00Z
  scenes:
    - scene: "Scene A"
      conversations:
        - speaker: "Alice"
          quote: "Time to break the ice."
      definitions:
        - expression: "break the ice"
          meaning: "to start social interaction"
- event: "Episode 2"
  date: 2025-01-16T00:00:00Z
  scenes:
    - scene: "Scene B"
      conversations:
        - speaker: "Bob"
          quote: "Don't lose your temper now."
        - speaker: "Bob"
          quote: "It's a piece of cake."
      definitions:
        - expression: "lose temper"
          meaning: "to become angry"
        - expression: "piece of cake"
          meaning: "something easy"
`), 0644))
	// Both expressions in episode 2 are eligible (have correct freeform
	// answers); episode 1's expression is not yet learned.
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "two-eps.yml"), []byte(`- metadata:
    notebook_id: two-eps
    title: "Episode 1"
  scenes:
    - metadata:
        title: "Scene A"
      expressions:
        - expression: "break the ice"
          learned_logs:
            - status: "misunderstood"
              learned_at: "2025-01-14"
              quiz_type: "freeform"
- metadata:
    notebook_id: two-eps
    title: "Episode 2"
  scenes:
    - metadata:
        title: "Scene B"
      expressions:
        - expression: "lose temper"
          learned_logs:
            - status: "understood"
              learned_at: "2025-01-14"
              quiz_type: "freeform"
        - expression: "piece of cake"
          learned_logs:
            - status: "understood"
              learned_at: "2025-01-14"
              quiz_type: "freeform"
`), 0644))

	svc := NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{storiesDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	summaries, err := svc.LoadNotebookSummaries(false)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	require.Len(t, summaries[0].Sections, 2)

	// Sections must be in document order.
	assert.Equal(t, "Episode 1", summaries[0].Sections[0].Title)
	assert.Equal(t, "Episode 2", summaries[0].Sections[1].Title)

	// Episode 1 has no eligible (freeform-correct) words yet.
	assert.Equal(t, 0, summaries[0].Sections[0].ReviewCount, "ep1 word still misunderstood")
	// Episode 2 has two eligible words.
	assert.Equal(t, 2, summaries[0].Sections[1].ReviewCount, "ep2 has two eligible words")

	// Reverse counts likewise reflect per-section eligibility.
	assert.Equal(t, 0, summaries[0].Sections[0].ReverseReviewCount)
	assert.Equal(t, 2, summaries[0].Sections[1].ReverseReviewCount)

	// Per-section counts add up to the notebook-level total.
	totalReview := summaries[0].Sections[0].ReviewCount + summaries[0].Sections[1].ReviewCount
	assert.Equal(t, summaries[0].ReviewCount, totalReview)
}

func TestService_LoadCards_SectionFilter(t *testing.T) {
	ctrl := gomock.NewController(t)
	storiesDir := t.TempDir()
	learningDir := t.TempDir()

	storyDir := filepath.Join(storiesDir, "two-chapters")
	require.NoError(t, os.MkdirAll(storyDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(`id: two-chapters
name: Two Chapters
notebooks:
  - ./episodes.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "episodes.yml"), []byte(`- event: "Chapter One"
  date: 2025-01-15T00:00:00Z
  scenes:
    - scene: "Opening"
      conversations:
        - speaker: "Alice"
          quote: "We need to break the ice."
      definitions:
        - expression: "break the ice"
          meaning: "to start social interaction"
- event: "Chapter Two"
  date: 2025-01-16T00:00:00Z
  scenes:
    - scene: "Closing"
      conversations:
        - speaker: "Bob"
          quote: "Don't lose your temper."
      definitions:
        - expression: "lose temper"
          meaning: "to become angry"
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "two-chapters.yml"), []byte(`- metadata:
    notebook_id: two-chapters
    title: "Chapter One"
  scenes:
    - metadata:
        title: "Opening"
      expressions:
        - expression: "break the ice"
          learned_logs:
            - status: "misunderstood"
              learned_at: "2025-01-14"
              quiz_type: "freeform"
- metadata:
    notebook_id: two-chapters
    title: "Chapter Two"
  scenes:
    - metadata:
        title: "Closing"
      expressions:
        - expression: "lose temper"
          learned_logs:
            - status: "misunderstood"
              learned_at: "2025-01-14"
              quiz_type: "freeform"
`), 0644))

	svc := NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{storiesDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	all, err := svc.LoadCards([]string{"two-chapters"}, true, nil)
	require.NoError(t, err)
	require.Len(t, all, 2, "no filter returns both chapters")

	filtered, err := svc.LoadCards([]string{"two-chapters"}, true, map[string][]string{
		"two-chapters": {"Chapter Two"},
	})
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, "Chapter Two", filtered[0].StoryTitle)
	assert.Equal(t, "lose temper", filtered[0].Entry)
}

func TestService_LoadReverseCards_SectionFilter(t *testing.T) {
	ctrl := gomock.NewController(t)
	storiesDir := t.TempDir()
	learningDir := t.TempDir()

	storyDir := filepath.Join(storiesDir, "rev-chapters")
	require.NoError(t, os.MkdirAll(storyDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(`id: rev-chapters
name: Reverse Chapters
notebooks:
  - ./episodes.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "episodes.yml"), []byte(`- event: "Chapter A"
  date: 2025-01-15T00:00:00Z
  scenes:
    - scene: "S1"
      conversations:
        - speaker: "X"
          quote: "Time to break the ice."
      definitions:
        - expression: "break the ice"
          meaning: "to start social interaction"
- event: "Chapter B"
  date: 2025-01-16T00:00:00Z
  scenes:
    - scene: "S2"
      conversations:
        - speaker: "Y"
          quote: "Try not to lose your temper."
      definitions:
        - expression: "lose temper"
          meaning: "to become angry"
`), 0644))
	// Both expressions need a correct freeform log to be reverse-eligible.
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "rev-chapters.yml"), []byte(`- metadata:
    notebook_id: rev-chapters
    title: "Chapter A"
  scenes:
    - metadata:
        title: "S1"
      expressions:
        - expression: "break the ice"
          learned_logs:
            - status: "understood"
              learned_at: "2025-01-14"
              quiz_type: "freeform"
- metadata:
    notebook_id: rev-chapters
    title: "Chapter B"
  scenes:
    - metadata:
        title: "S2"
      expressions:
        - expression: "lose temper"
          learned_logs:
            - status: "understood"
              learned_at: "2025-01-14"
              quiz_type: "freeform"
`), 0644))

	svc := NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{storiesDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	filtered, err := svc.LoadReverseCards([]string{"rev-chapters"}, false, false, map[string][]string{
		"rev-chapters": {"Chapter A"},
	})
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, "Chapter A", filtered[0].StoryTitle)
	assert.Equal(t, "break the ice", filtered[0].Expression)
}

// ---------- GradeNotebookAnswer ----------

func TestService_GradeNotebookAnswer_Correct(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	svc := newTestService(t, mockClient)

	card := Card{
		Entry:   "preposterous",
		Meaning: "contrary to reason",
	}

	mockClient.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).Return(
		inference.AnswerMeaningsResponse{
			Answers: []inference.AnswerMeaning{
				{
					Expression: "preposterous",
					Meaning:    "contrary to reason",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: true, Reason: "Good answer", Quality: 4},
					},
				},
			},
		}, nil,
	)

	result, err := svc.GradeNotebookAnswer(context.Background(), card, "contrary to reason", 1000)
	require.NoError(t, err)
	assert.True(t, result.Correct)
	assert.Equal(t, "Good answer", result.Reason)
	assert.Equal(t, 4, result.Quality)
}

func TestService_GradeNotebookAnswer_Incorrect(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	svc := newTestService(t, mockClient)

	card := Card{
		Entry:   "serendipity",
		Meaning: "a fortunate discovery by accident",
	}

	mockClient.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).Return(
		inference.AnswerMeaningsResponse{
			Answers: []inference.AnswerMeaning{
				{
					Expression: "serendipity",
					Meaning:    "a fortunate discovery by accident",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: false, Reason: "Wrong meaning", Quality: 1},
					},
				},
			},
		}, nil,
	)

	result, err := svc.GradeNotebookAnswer(context.Background(), card, "wrong answer", 1000)
	require.NoError(t, err)
	assert.False(t, result.Correct)
	assert.Equal(t, "Wrong meaning", result.Reason)
	assert.Equal(t, 1, result.Quality)
}

func TestService_GradeNotebookAnswer_InferenceError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	svc := newTestService(t, mockClient)

	card := Card{Entry: "preposterous"}

	mockClient.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).Return(
		inference.AnswerMeaningsResponse{}, assert.AnError,
	)

	_, err := svc.GradeNotebookAnswer(context.Background(), card, "some answer", 1000)
	require.Error(t, err)
}

func TestService_GradeNotebookAnswer_EmptyAnswers(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	svc := newTestService(t, mockClient)

	card := Card{Entry: "preposterous"}

	mockClient.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).Return(
		inference.AnswerMeaningsResponse{Answers: nil}, nil,
	)

	_, err := svc.GradeNotebookAnswer(context.Background(), card, "some answer", 1000)
	require.Error(t, err)
}

// ---------- SaveResult ----------

func TestService_SaveResult_WritesFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	learningDir := t.TempDir()
	svc := NewService(config.NotebooksConfig{
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	card := Card{
		NotebookName: "test-vocab",
		StoryTitle:   "flashcards",
		SceneTitle:   "",
		Entry:        "serendipity",
		Meaning:      "a fortunate discovery by accident",
	}

	err := svc.SaveResult(context.Background(), card, GradeResult{Correct: true, Quality: 4}, 1000)
	require.NoError(t, err)

	historyPath := filepath.Join(learningDir, "test-vocab.yml")
	_, statErr := os.Stat(historyPath)
	assert.NoError(t, statErr)
}

func TestService_SaveResult_MalformedYAMLError(t *testing.T) {
	ctrl := gomock.NewController(t)
	learningDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-notebook.yml"), []byte("{{invalid yaml"), 0644))

	svc := NewService(config.NotebooksConfig{
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	card := Card{
		NotebookName: "test-notebook",
		StoryTitle:   "flashcards",
		Entry:        "preposterous",
	}

	err := svc.SaveResult(context.Background(), card, GradeResult{Correct: true, Quality: 4}, 1000)
	require.Error(t, err)
}

// makeConceptBookFixture writes a definitions-only book with one
// concept whose head ("smile" – stand-in for an actual entry) groups
// three derived forms. The head carries a correct freeform log so its
// SR-interval gate is the only thing controlling badge/loader output;
// the three member rows are absent — matching the post-MergeConcepts
// shape MergeConcepts produces in a real notebook.
func makeConceptBookFixture(t *testing.T, recentInterval int) (defsDir, learningDir string) {
	t.Helper()
	defsDir = t.TempDir()
	learningDir = t.TempDir()

	bookDir := filepath.Join(defsDir, "concept-book")
	require.NoError(t, os.MkdirAll(bookDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "index.yml"), []byte(`id: concept-book
notebooks:
  - ./session1.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "session1.yml"), []byte(`- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        index: 0
        title: "common idioms"
      expressions:
        - expression: "break the ice"
          meaning: "to start social interaction"
        - expression: "ice breaker"
          meaning: "something that starts social interaction"
        - expression: "ice-breaking"
          meaning: "starting social interaction"
  concepts:
    - head: "break the ice"
      meaning: "to start social interaction"
      expressions:
        - "break the ice"
        - "ice breaker"
        - "ice-breaking"
`), 0o644))

	tomorrow := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "concept-book.yml"), []byte(`- metadata:
    notebook_id: concept-book
    title: "Session 1"
  scenes:
    - metadata:
        title: "common idioms"
      expressions:
        - expression: "break the ice"
          learned_logs:
            - status: "understood"
              learned_at: "`+tomorrow+`"
              quality: 4
              interval_days: `+fmt.Sprint(recentInterval)+`
              quiz_type: "notebook"
          reverse_logs:
            - status: "understood"
              learned_at: "`+tomorrow+`"
              quality: 4
              interval_days: `+fmt.Sprint(recentInterval)+`
              quiz_type: "reverse"
`), 0o644))
	return defsDir, learningDir
}

// TestService_LoadNotebookSummaries_ConceptMembersFollowHead pins the
// post-MergeConcepts behaviour: when the head has a recent correct
// answer that's still inside its SR interval, the badge must show 0
// even though the source YAML still lists the two non-head members.
// Pre-fix, needsDefinitionReview missed the head's history when asked
// about a member's expression and returned includeUnstudied=true,
// inflating the badge by the member count for every concept.
func TestService_LoadNotebookSummaries_ConceptMembersFollowHead(t *testing.T) {
	ctrl := gomock.NewController(t)
	defsDir, learningDir := makeConceptBookFixture(t, 30)

	svc := NewService(config.NotebooksConfig{
		DefinitionsDirectories: []string{defsDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	summaries, err := svc.LoadNotebookSummaries(true)
	require.NoError(t, err)
	var book *NotebookSummary
	for i := range summaries {
		if summaries[i].NotebookID == "concept-book" {
			book = &summaries[i]
			break
		}
	}
	require.Nil(t, book,
		"head is studied and inside SR interval; concept must contribute 0 — book should not appear")
}

// TestService_LoadCards_DefinitionsBook_FamilyConceptUsesHeadRow pins the
// kind=family loader contract: exactly one card per concept, sourced
// from the head's OWN note row so the displayed (word, meaning) pair
// always agrees. Pre-fix, whichever member iterated first contributed
// its meaning while the answer was forced to the head, so the user
// saw e.g. cardiology paired with cardiologist's meaning.
func TestService_LoadCards_DefinitionsBook_FamilyConceptUsesHeadRow(t *testing.T) {
	ctrl := gomock.NewController(t)
	defsDir := t.TempDir()
	learningDir := t.TempDir()

	bookDir := filepath.Join(defsDir, "concept-book")
	require.NoError(t, os.MkdirAll(bookDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "index.yml"), []byte(`id: concept-book
notebooks:
  - ./session1.yml
`), 0o644))
	// Head and members have DIFFERENT meanings so the test fails loudly
	// if the loader picks a member's meaning instead of the head's.
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "session1.yml"), []byte(`- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        index: 0
        title: "common idioms"
      expressions:
        - expression: "break the ice"
          meaning: "to start a conversation in a social setting"
        - expression: "ice breaker"
          meaning: "a person or thing that initiates social interaction"
        - expression: "ice-breaking"
          meaning: "the act of initiating social interaction"
  concepts:
    - head: "break the ice"
      kind: family
      meaning: "starting social interaction"
      expressions:
        - "break the ice"
        - "ice breaker"
        - "ice-breaking"
`), 0o644))

	svc := NewService(config.NotebooksConfig{
		DefinitionsDirectories: []string{defsDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	cards, err := svc.LoadCards([]string{"concept-book"}, true, nil)
	require.NoError(t, err)
	require.Len(t, cards, 1, "family concept must collapse to one card at load time")
	assert.Equal(t, "break the ice", cards[0].Entry,
		"surviving card must be named for the head; saves under it land on the consolidated row")
	assert.Equal(t, "break the ice", cards[0].ConceptHead,
		"ConceptHead must be set so SaveResult redirects under the head")
	assert.Equal(t, "to start a conversation in a social setting", cards[0].Meaning,
		"Meaning must come from the head's own note row, not whichever member iterated first")

	reverse, err := svc.LoadReverseCards([]string{"concept-book"}, false, true, nil)
	require.NoError(t, err)
	require.Len(t, reverse, 1, "family concept reverse quiz must also surface one card")
	assert.Equal(t, "break the ice", reverse[0].Expression,
		"reverse card answer must be the head expression")
	assert.Equal(t, "to start a conversation in a social setting", reverse[0].Meaning,
		"reverse prompt must be the head's own meaning so prompt and answer match")
}

// TestService_LoadCards_DefinitionsBook_SynonymConceptKeepsMembers
// pins the kind=synonym contract: the concepts block groups expressions
// for the Family-chip display, but each member keeps its own card with
// its own meaning, and ConceptHead stays empty so SaveResult writes
// under the member (independent SR row per word). Identical contract
// applies to kind=antonym and kind=visualization.
func TestService_LoadCards_DefinitionsBook_SynonymConceptKeepsMembers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defsDir := t.TempDir()
	learningDir := t.TempDir()

	bookDir := filepath.Join(defsDir, "synonym-book")
	require.NoError(t, os.MkdirAll(bookDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "index.yml"), []byte(`id: synonym-book
notebooks:
  - ./session1.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "session1.yml"), []byte(`- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        index: 0
        title: "feelings"
      expressions:
        - expression: "happy"
          meaning: "feeling pleasure or contentment"
        - expression: "joyful"
          meaning: "expressing great happiness"
        - expression: "cheerful"
          meaning: "noticeably pleasant in mood"
  concepts:
    - head: "happy"
      kind: synonym
      meaning: "shared meaning: experiencing positive emotion"
      expressions:
        - "happy"
        - "joyful"
        - "cheerful"
`), 0o644))

	svc := NewService(config.NotebooksConfig{
		DefinitionsDirectories: []string{defsDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	cards, err := svc.LoadCards([]string{"synonym-book"}, true, nil)
	require.NoError(t, err)
	assert.Len(t, cards, 3, "synonym kind must keep one card per member, not collapse")
	gotByEntry := map[string]Card{}
	for _, c := range cards {
		gotByEntry[c.Entry] = c
	}
	for _, name := range []string{"happy", "joyful", "cheerful"} {
		c, ok := gotByEntry[name]
		require.True(t, ok, "expected a card for %q", name)
		assert.Empty(t, c.ConceptHead,
			"synonym member must not carry ConceptHead — saves would otherwise consolidate under happy")
		assert.Equal(t, []string{"happy", "joyful", "cheerful"}, c.ConceptMembers,
			"ConceptMembers stays populated for the Family chip even when SR isn't consolidated")
	}
	assert.Equal(t, "feeling pleasure or contentment", gotByEntry["happy"].Meaning,
		"each synonym card keeps its own row's meaning")
	assert.Equal(t, "expressing great happiness", gotByEntry["joyful"].Meaning)
}

// TestService_SaveResult_SynonymMemberWritesUnderMember pins the
// save-side contract for non-family concepts: ConceptHead is empty on
// these cards, so the log lands under the member's own expression.
func TestService_SaveResult_SynonymMemberWritesUnderMember(t *testing.T) {
	ctrl := gomock.NewController(t)
	learningDir := t.TempDir()

	svc := NewService(config.NotebooksConfig{
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	card := Card{
		NotebookName: "synonym-book",
		StoryTitle:   "Session 1",
		SceneTitle:   "feelings",
		Entry:        "joyful",
		// ConceptHead deliberately unset — synonym/antonym/visualization
		// concepts emit cards without it.
		Meaning: "expressing great happiness",
	}
	require.NoError(t, svc.SaveResult(context.Background(), card,
		GradeResult{Correct: true, Quality: 4}, 1000))

	yamlBytes, err := os.ReadFile(filepath.Join(learningDir, "synonym-book.yml"))
	require.NoError(t, err)
	got := string(yamlBytes)
	assert.Contains(t, got, "expression: joyful",
		"log for a non-family-member card must land under the member itself")
	assert.NotContains(t, got, "expression: happy",
		"non-family concepts must not consolidate under the concept head")
}

// TestService_SaveResult_ConceptHeadRedirectsLog pins the save-side
// fix: when a Card carries ConceptHead, the log must land under the
// head expression in the YAML, not under card.Entry. Without this,
// every quiz answer for a member-named card recreated the per-member
// row that MergeConcepts purged.
func TestService_SaveResult_ConceptHeadRedirectsLog(t *testing.T) {
	ctrl := gomock.NewController(t)
	learningDir := t.TempDir()

	svc := NewService(config.NotebooksConfig{
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	card := Card{
		NotebookName: "concept-book",
		StoryTitle:   "Session 1",
		SceneTitle:   "common idioms",
		Entry:        "ice breaker", // member name (legacy code path)
		ConceptHead:  "break the ice",
		Meaning:      "to start social interaction",
	}
	require.NoError(t, svc.SaveResult(context.Background(), card,
		GradeResult{Correct: true, Quality: 4}, 1000))

	yamlBytes, err := os.ReadFile(filepath.Join(learningDir, "concept-book.yml"))
	require.NoError(t, err)
	got := string(yamlBytes)
	assert.Contains(t, got, "expression: break the ice",
		"log must land under the concept head, not the member name")
	assert.NotContains(t, got, "expression: ice breaker",
		"member name must not appear as a separate row")
}

// ---------- helper functions (package-internal) ----------

func TestExtractAnswerResult(t *testing.T) {
	tests := []struct {
		name        string
		result      inference.AnswerMeaning
		wantCorrect bool
		wantReason  string
		wantQuality int
	}{
		{
			name:        "empty answers returns incorrect with quality 1",
			result:      inference.AnswerMeaning{AnswersForContext: nil},
			wantCorrect: false,
			wantReason:  "",
			wantQuality: 1,
		},
		{
			name: "correct answer extracts fields",
			result: inference.AnswerMeaning{
				AnswersForContext: []inference.AnswersForContext{
					{Correct: true, Reason: "Good answer", Quality: 5},
				},
			},
			wantCorrect: true,
			wantReason:  "Good answer",
			wantQuality: 5,
		},
		{
			name: "quality zero defaults to 4 for correct",
			result: inference.AnswerMeaning{
				AnswersForContext: []inference.AnswersForContext{
					{Correct: true, Reason: "OK", Quality: 0},
				},
			},
			wantCorrect: true,
			wantReason:  "OK",
			wantQuality: 4,
		},
		{
			name: "quality zero defaults to 1 for incorrect",
			result: inference.AnswerMeaning{
				AnswersForContext: []inference.AnswersForContext{
					{Correct: false, Reason: "Wrong", Quality: 0},
				},
			},
			wantCorrect: false,
			wantReason:  "Wrong",
			wantQuality: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCorrect, gotReason, gotQuality := extractAnswerResult(tt.result)
			assert.Equal(t, tt.wantCorrect, gotCorrect)
			assert.Equal(t, tt.wantReason, gotReason)
			assert.Equal(t, tt.wantQuality, gotQuality)
		})
	}
}

func TestCountStoryDefinitions(t *testing.T) {
	tests := []struct {
		name    string
		stories []notebook.StoryNotebook
		want    int
	}{
		{
			name:    "empty stories",
			stories: nil,
			want:    0,
		},
		{
			name: "counts definitions across stories and scenes",
			stories: []notebook.StoryNotebook{
				{
					Scenes: []notebook.StoryScene{
						{Definitions: []notebook.Note{{Expression: "a"}, {Expression: "b"}}},
						{Definitions: []notebook.Note{{Expression: "c"}}},
					},
				},
				{
					Scenes: []notebook.StoryScene{
						{Definitions: []notebook.Note{{Expression: "d"}}},
					},
				},
			},
			want: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countStoryDefinitions(tt.stories)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCountFlashcardCards(t *testing.T) {
	tests := []struct {
		name      string
		notebooks []notebook.FlashcardNotebook
		want      int
	}{
		{
			name:      "empty notebooks",
			notebooks: nil,
			want:      0,
		},
		{
			name: "counts cards across notebooks",
			notebooks: []notebook.FlashcardNotebook{
				{Cards: []notebook.Note{{Expression: "a"}, {Expression: "b"}}},
				{Cards: []notebook.Note{{Expression: "c"}}},
			},
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countFlashcardCards(tt.notebooks)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildFromConversations(t *testing.T) {
	tests := []struct {
		name         string
		scene        notebook.StoryScene
		definition   notebook.Note
		wantExamples int
	}{
		{
			name: "skips empty quotes",
			scene: notebook.StoryScene{
				Conversations: []notebook.Conversation{
					{Speaker: "Alice", Quote: ""},
					{Speaker: "Bob", Quote: "This is absolutely preposterous."},
				},
			},
			definition:   notebook.Note{Expression: "preposterous", Meaning: "absurd"},
			wantExamples: 1,
		},
		{
			name: "skips non-matching quotes",
			scene: notebook.StoryScene{
				Conversations: []notebook.Conversation{
					{Speaker: "Alice", Quote: "Hello there."},
					{Speaker: "Bob", Quote: "Good morning."},
				},
			},
			definition:   notebook.Note{Expression: "preposterous", Meaning: "absurd"},
			wantExamples: 0,
		},
		{
			name: "matches multiple quotes containing expression",
			scene: notebook.StoryScene{
				Conversations: []notebook.Conversation{
					{Speaker: "Alice", Quote: "That is preposterous!"},
					{Speaker: "Bob", Quote: "I agree, totally preposterous."},
				},
			},
			definition:   notebook.Note{Expression: "preposterous", Meaning: "absurd"},
			wantExamples: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			examples, contexts := buildFromConversations(&tt.scene, &tt.definition)
			assert.Len(t, examples, tt.wantExamples)
			assert.Len(t, contexts, tt.wantExamples)
		})
	}
}

func TestContainsExpression(t *testing.T) {
	tests := []struct {
		name       string
		textLower  string
		expression string
		definition string
		want       bool
	}{
		{
			name:       "matches expression",
			textLower:  "i need to comprehend this",
			expression: "comprehend",
			definition: "",
			want:       true,
		},
		{
			name:       "matches definition",
			textLower:  "he ran away quickly",
			expression: "run",
			definition: "ran",
			want:       true,
		},
		{
			name:       "no match",
			textLower:  "hello world",
			expression: "comprehend",
			definition: "",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsExpression(tt.textLower, tt.expression, tt.definition)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFindMatchingCards(t *testing.T) {
	cards := []FreeformCard{
		{
			Expression:         "jump down someone's throat",
			OriginalExpression: "jumped down my throat",
			Meaning:            "to speak sharply to someone",
		},
		{
			Expression:         "break the ice",
			OriginalExpression: "",
			Meaning:            "to initiate social interaction",
		},
		{
			Expression:         "lose one's temper",
			OriginalExpression: "lost her temper",
			Meaning:            "to become very angry",
		},
	}

	tests := []struct {
		name      string
		word      string
		wantCount int
		wantExpr  string
	}{
		{
			name:      "matches canonical expression",
			word:      "break the ice",
			wantCount: 1,
			wantExpr:  "break the ice",
		},
		{
			name:      "matches original expression from story text",
			word:      "jumped down my throat",
			wantCount: 1,
			wantExpr:  "jump down someone's throat",
		},
		{
			name:      "case insensitive match on canonical",
			word:      "Break The Ice",
			wantCount: 1,
			wantExpr:  "break the ice",
		},
		{
			name:      "case insensitive match on original",
			word:      "Lost Her Temper",
			wantCount: 1,
			wantExpr:  "lose one's temper",
		},
		{
			name:      "no match",
			word:      "spill the beans",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findMatchingCards(cards, tt.word)
			assert.Len(t, got, tt.wantCount)
			if tt.wantCount > 0 {
				assert.Equal(t, tt.wantExpr, got[0].Expression)
			}
		})
	}
}

func TestDeduplicateReverseCards(t *testing.T) {
	tests := []struct {
		name      string
		cards     []ReverseCard
		wantCount int
		validate  func(t *testing.T, result []ReverseCard)
	}{
		{
			name:      "empty input",
			cards:     []ReverseCard{},
			wantCount: 0,
		},
		{
			name: "no duplicates",
			cards: []ReverseCard{
				{Expression: "break the ice", Meaning: "to initiate conversation"},
				{Expression: "lose one's temper", Meaning: "to become very angry"},
			},
			wantCount: 2,
		},
		{
			name: "duplicates - keeps card with more contexts",
			cards: []ReverseCard{
				{Expression: "break the ice", Meaning: "to initiate conversation", Contexts: []ReverseContext{{Context: "She broke the ice.", MaskedContext: "She ______ the ice."}}},
				{Expression: "break the ice", Meaning: "to initiate conversation", Contexts: []ReverseContext{}},
			},
			wantCount: 1,
			validate: func(t *testing.T, result []ReverseCard) {
				assert.Equal(t, 1, len(result[0].Contexts), "should keep card with more contexts")
			},
		},
		{
			name: "case insensitive dedup",
			cards: []ReverseCard{
				{Expression: "Break the Ice", Meaning: "to initiate conversation"},
				{Expression: "break the ice", Meaning: "to initiate conversation", Contexts: []ReverseContext{{Context: "She broke the ice.", MaskedContext: "She ______ the ice."}}},
			},
			wantCount: 1,
			validate: func(t *testing.T, result []ReverseCard) {
				assert.Equal(t, 1, len(result[0].Contexts), "should keep card with more contexts")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deduplicateReverseCards(tt.cards)
			assert.Equal(t, tt.wantCount, len(result))
			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestDeduplicateCards(t *testing.T) {
	tests := []struct {
		name      string
		cards     []Card
		wantCount int
		validate  func(t *testing.T, result []Card)
	}{
		{
			name:      "empty input",
			cards:     []Card{},
			wantCount: 0,
		},
		{
			name: "no duplicates",
			cards: []Card{
				{Entry: "break the ice", Meaning: "to initiate conversation"},
				{Entry: "lose one's temper", Meaning: "to become very angry"},
			},
			wantCount: 2,
		},
		{
			name: "duplicates - keeps card with more examples",
			cards: []Card{
				{Entry: "break the ice", Meaning: "to initiate conversation", Examples: []Example{{Text: "She broke the ice.", Speaker: "Alice"}}},
				{Entry: "break the ice", Meaning: "to initiate conversation"},
			},
			wantCount: 1,
			validate: func(t *testing.T, result []Card) {
				assert.Equal(t, 1, len(result[0].Examples), "should keep card with more examples")
			},
		},
		{
			name: "case insensitive dedup",
			cards: []Card{
				{Entry: "Break the Ice", Meaning: "to initiate conversation"},
				{Entry: "break the ice", Meaning: "to initiate conversation", Examples: []Example{{Text: "She broke the ice.", Speaker: "Alice"}}},
			},
			wantCount: 1,
			validate: func(t *testing.T, result []Card) {
				assert.Equal(t, 1, len(result[0].Examples), "should keep card with more examples")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deduplicateCards(tt.cards)
			assert.Equal(t, tt.wantCount, len(result))
			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestApplyForwardMask(t *testing.T) {
	// Two reverse-quiz cards whose contexts contain BOTH expressions. The
	// first card (index 0) shows its context to the user; that context must
	// hide the second card's expression too. The second card sees its own
	// context with only its own expression masked (since the first was
	// already revealed by then).
	cards := []ReverseCard{
		{
			Expression: "alpha",
			Contexts: []ReverseContext{
				{Context: "alpha and beta are friends.", MaskedContext: "alpha and beta are friends."},
			},
		},
		{
			Expression: "beta",
			Contexts: []ReverseContext{
				{Context: "alpha and beta are friends.", MaskedContext: "alpha and beta are friends."},
			},
		},
	}

	applyForwardMask(cards)

	assert.Equal(t, "______ and [...] are friends.", cards[0].Contexts[0].MaskedContext, "first card masks itself with ______ and future card with [...]")
	assert.Equal(t, "alpha and ______ are friends.", cards[1].Contexts[0].MaskedContext, "second card masks only itself; the previously-asked first card is visible")
}

func TestApplyForwardMask_AltForm(t *testing.T) {
	// When a card has an inflected form (AltForm), both the primary expression
	// and the inflected form must be masked in future cards' contexts.
	cards := []ReverseCard{
		{
			Expression: "running",
			AltForm:    "run",
			Contexts:   []ReverseContext{{Context: "She is running fast.", MaskedContext: "She is running fast."}},
		},
		{
			Expression: "fast",
			Contexts:   []ReverseContext{{Context: "She likes to run fast.", MaskedContext: "She likes to run fast."}},
		},
	}

	applyForwardMask(cards)

	assert.Equal(t, "She is ______ [...].", cards[0].Contexts[0].MaskedContext, "first card masks itself with ______ and future card 'fast' with [...]")
	// Second card only masks itself; the past card's forms ('run', 'running') are now revealed.
	assert.Equal(t, "She likes to run ______.", cards[1].Contexts[0].MaskedContext, "second card masks only itself; the past card's forms are revealed")
}

func TestContainsExpressionWord(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		expression string
		want       bool
	}{
		{
			name:       "exact match",
			text:       "happy",
			expression: "happy",
			want:       true,
		},
		{
			name:       "trivial inflection",
			text:       "happiness and joy",
			expression: "happy",
			want:       true,
		},
		{
			name:       "does not match words that merely appear in the expected meaning",
			text:       "feeling of great joy",
			expression: "happy",
			want:       false,
		},
		{
			name:       "does not match partial substring",
			text:       "of large size",
			expression: "huge",
			want:       false,
		},
		{
			name:       "case insensitive",
			text:       "Happy feelings",
			expression: "happy",
			want:       true,
		},
		{
			name:       "multi-word expression",
			text:       "she decided to break the ice at the party",
			expression: "break the ice",
			want:       true,
		},
		{
			name:       "expression starting with non-word character",
			text:       "she is the #1 fan of that band",
			expression: "#1 fan",
			want:       true,
		},
		{
			name:       "empty expression",
			text:       "anything",
			expression: "",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsExpressionWord(tt.text, tt.expression)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMaskWord(t *testing.T) {
	tests := []struct {
		name       string
		context    string
		expression string
		definition string
		want       string
	}{
		{
			name:       "simple word with surrounding spaces",
			context:    "the question is hard",
			expression: "question",
			want:       "the ______ is hard",
		},
		{
			name:       "case insensitive match",
			context:    "Question time",
			expression: "question",
			want:       "______ time",
		},
		{
			name:       "does not mask partial word",
			context:    "the questioning continues",
			expression: "question",
			want:       "the questioning continues",
		},
		{
			name:       "expression starting with non-word character",
			context:    "She is the #1 fan of that band.",
			expression: "#1 fan",
			want:       "She is the ______ of that band.",
		},
		{
			name:       "multiple consecutive occurrences",
			context:    "fast and fast and fast",
			expression: "fast",
			want:       "______ and ______ and ______",
		},
		{
			name:       "expression at end of context",
			context:    "an idiom for break the ice",
			expression: "break the ice",
			want:       "an idiom for ______",
		},
		{
			name:       "definition also masked",
			context:    "She used the term break the ice during the meeting",
			expression: "break ice",
			definition: "break the ice",
			want:       "She used the term ______ during the meeting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskWord(tt.context, tt.expression, tt.definition)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestDefinitionsSectionSummaries_NaturalSessionOrdering pins the
// ordering rule that drove this helper's addition: session titles with
// a trailing integer ("Session 2", "Session 10") sort numerically, not
// lexically. Without this, the per-session list on the vocabulary quiz
// start page would put "Session 10" before "Session 2" — visible
// regression vs how etymology orders the same book.
//
// Per-section ReviewCount semantics are owned by countDefinitionNotes
// (notes without a history don't "need review" yet) and tested
// elsewhere; this test stays focused on ordering.
func TestDefinitionsSectionSummaries_NaturalSessionOrdering(t *testing.T) {
	defs := map[string]map[string][]notebook.Note{
		"Session 10": {"__index_0": {{Expression: "hesitate", Meaning: "to pause"}}},
		"Session 2":  {"__index_0": {{Expression: "stall", Meaning: "to delay"}}},
		"Intro":      {"__index_0": {{Expression: "warm up", Meaning: "to ease in"}}},
	}

	got := definitionsSectionSummaries(defs, nil, false, nil)

	require.Len(t, got, 3)
	// Numbered sessions sort numerically before non-numbered ones; among
	// non-numbered, alphabetical. So: Session 2, Session 10, Intro.
	assert.Equal(t, "Session 2", got[0].Title)
	assert.Equal(t, "Session 10", got[1].Title)
	assert.Equal(t, "Intro", got[2].Title)
}

// TestService_DefinitionsBookSummaryMatchesLoad pins the contract that
// the vocab start-page badge count equals what the standard/reverse
// quiz actually loads for a definitions-only book.
//
// The mismatch this guards against: countDefinitionNotes used to skip
// the per-type SkippedAt gate that loadDefinitionCards /
// loadDefinitionReverseCards apply, so the badge over-counted any
// note the user had excluded from that quiz mode (and per-section
// counts diverged the same way via definitionsSectionSummaries).
//
// Per-direction cases — each fixture marks the single note skipped
// from that direction's quiz and asserts both directions agree with
// their loader.
func TestService_DefinitionsBookSummaryMatchesLoad(t *testing.T) {
	cases := []struct {
		name      string
		skippedAt string
	}{
		{
			name:      "notebook skip",
			skippedAt: `            notebook: "2025-01-20T10:00:00Z"` + "\n",
		},
		{
			name:      "reverse skip",
			skippedAt: `            reverse: "2025-01-20T10:00:00Z"` + "\n",
		},
		{
			name:      "no skip",
			skippedAt: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defsDir, learningDir := makeDefinitionsBookSkipFixture(t, tc.skippedAt)

			svc := NewService(config.NotebooksConfig{
				DefinitionsDirectories: []string{defsDir},
				LearningNotesDirectory: learningDir,
			}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
				learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

			summaries, err := svc.LoadNotebookSummaries(true)
			require.NoError(t, err)
			var book *NotebookSummary
			for i := range summaries {
				if summaries[i].NotebookID == "skip-defs" {
					book = &summaries[i]
					break
				}
			}
			require.NotNil(t, book, "skip-defs must appear in summaries")
			require.Len(t, book.Sections, 1, "fixture has exactly one session")

			cards, err := svc.LoadCards([]string{"skip-defs"}, true, nil)
			require.NoError(t, err)
			assert.Equal(t, len(cards), book.ReviewCount,
				"notebook ReviewCount must equal len(LoadCards)")
			assert.Equal(t, len(cards), book.Sections[0].ReviewCount,
				"section ReviewCount must equal len(LoadCards)")

			reverseCards, err := svc.LoadReverseCards([]string{"skip-defs"}, false, true, nil)
			require.NoError(t, err)
			assert.Equal(t, len(reverseCards), book.ReverseReviewCount,
				"notebook ReverseReviewCount must equal len(LoadReverseCards)")
			assert.Equal(t, len(reverseCards), book.Sections[0].ReverseReviewCount,
				"section ReverseReviewCount must equal len(LoadReverseCards)")
		})
	}
}
