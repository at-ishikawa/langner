package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference/mock"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/quiz"
)

// newRelearnTestHandler builds a QuizHandler over a flashcard notebook whose
// learning history has words in several wrong/correct/old states so the pool
// selection can be exercised. Returns the handler and the learning-notes dir
// (for the "writes nothing" assertion). Uses the substring mock grader: any
// answer not starting with "wrong" is graded correct.
func newRelearnTestHandler(t *testing.T) (*QuizHandler, string) {
	t.Helper()
	flashcardsDir := t.TempDir()
	learningDir := t.TempDir()

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
    - expression: "alpha"
      meaning: "the first thing"
      examples:
        - "Alpha comes before beta."
    - expression: "beta"
      meaning: "the second thing"
    - expression: "gamma"
      meaning: "the third thing"
    - expression: "delta"
      meaning: "a change or difference"
`), 0644))

	now := time.Now()
	recent := now.Add(-2 * time.Hour).Format(time.RFC3339)        // in a 24h window, out of a 1h window
	veryRecent := now.Add(-30 * time.Minute).Format(time.RFC3339) // in both the 24h and 1h windows
	old := now.Add(-48 * time.Hour).Format(time.RFC3339)          // out of a 24h window

	// alpha: recently wrong (in pool). beta: recently correct (excluded).
	// gamma: wrong but too old (excluded). delta: recently wrong in REVERSE.
	history := fmt.Sprintf(`- metadata:
    notebook_id: test-vocab
    title: "Basic Words"
    type: "flashcard"
  expressions:
    - expression: "alpha"
      learned_logs:
        - status: "misunderstood"
          learned_at: %q
          quiz_type: "notebook"
    - expression: "beta"
      learned_logs:
        - status: "understood"
          learned_at: %q
          quiz_type: "notebook"
    - expression: "gamma"
      learned_logs:
        - status: "misunderstood"
          learned_at: %q
          quiz_type: "notebook"
    - expression: "delta"
      reverse_logs:
        - status: "misunderstood"
          learned_at: %q
          quiz_type: "reverse"
`, recent, recent, old, veryRecent)
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-vocab.yml"), []byte(history), 0644))

	svc := quiz.NewService(config.NotebooksConfig{
		FlashcardsDirectories:  []string{flashcardsDir},
		LearningNotesDirectory: learningDir,
	}, mock.NewClient(), make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	return NewQuizHandler(svc), learningDir
}

func relearnEntries(cards []*apiv1.RelearnCard) map[string]*apiv1.RelearnCard {
	out := make(map[string]*apiv1.RelearnCard, len(cards))
	for _, c := range cards {
		out[c.GetEntry()] = c
	}
	return out
}

func startRelearn(t *testing.T, h *QuizHandler, windowHours int32) []*apiv1.RelearnCard {
	t.Helper()
	resp, err := h.StartRelearnQuiz(context.Background(),
		connect.NewRequest(&apiv1.StartRelearnQuizRequest{WindowHours: windowHours}))
	require.NoError(t, err)
	return resp.Msg.GetCards()
}

func TestRelearn_PoolSelectsRecentWrongWordsAcrossTypes(t *testing.T) {
	h, _ := newRelearnTestHandler(t)

	cards := startRelearn(t, h, 24)
	byEntry := relearnEntries(cards)

	assert.Contains(t, byEntry, "alpha", "recently-wrong word must be in the pool")
	assert.Contains(t, byEntry, "delta", "recently-wrong reverse word must be in the pool")
	assert.NotContains(t, byEntry, "beta", "recently-correct word must be excluded")
	assert.NotContains(t, byEntry, "gamma", "wrong-but-old word must be excluded (outside window)")

	assert.Equal(t, apiv1.QuizType_QUIZ_TYPE_STANDARD, byEntry["alpha"].GetSourceQuizType())
	assert.Equal(t, apiv1.QuizType_QUIZ_TYPE_REVERSE, byEntry["delta"].GetSourceQuizType())
}

func TestRelearn_WindowNarrowsPool(t *testing.T) {
	h, _ := newRelearnTestHandler(t)

	// A 1-hour window drops alpha (2h ago) but keeps delta (1h ago).
	byEntry := relearnEntries(startRelearn(t, h, 1))
	assert.NotContains(t, byEntry, "alpha")
	assert.Contains(t, byEntry, "delta")
}

func TestRelearn_ZeroWindowUsesDefault(t *testing.T) {
	h, _ := newRelearnTestHandler(t)

	zero := relearnEntries(startRelearn(t, h, 0))
	def := relearnEntries(startRelearn(t, h, 24))
	assert.Equal(t, len(def), len(zero), "window_hours=0 must behave like the 24h default")
	assert.Contains(t, zero, "alpha")
}

func TestRelearn_CorrectAnswerClearsWordAndWritesNoHistory(t *testing.T) {
	h, learningDir := newRelearnTestHandler(t)
	historyPath := filepath.Join(learningDir, "test-vocab.yml")
	before, err := os.ReadFile(historyPath)
	require.NoError(t, err)

	cards := startRelearn(t, h, 24)
	byEntry := relearnEntries(cards)
	alpha := byEntry["alpha"]
	require.NotNil(t, alpha)

	resp, err := h.SubmitRelearnAnswer(context.Background(),
		connect.NewRequest(&apiv1.SubmitRelearnAnswerRequest{
			NoteId: alpha.GetNoteId(),
			Answer: "the first thing",
		}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.GetCorrect())
	assert.Equal(t, "the first thing", resp.Msg.GetMeaning())

	// The no-write guarantee: the learning-history YAML is byte-identical.
	after, err := os.ReadFile(historyPath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after),
		"a relearn answer must not write any learning history")

	// The cleared word drops out of the next session; delta remains.
	next := relearnEntries(startRelearn(t, h, 24))
	assert.NotContains(t, next, "alpha", "a correctly relearned word is cleared from the next session")
	assert.Contains(t, next, "delta")
}

func TestRelearn_WrongAndSkippedDoNotClear(t *testing.T) {
	h, _ := newRelearnTestHandler(t)

	first := relearnEntries(startRelearn(t, h, 24))
	require.Contains(t, first, "alpha")
	require.Contains(t, first, "delta")

	// Wrong answer for alpha.
	wrongResp, err := h.SubmitRelearnAnswer(context.Background(),
		connect.NewRequest(&apiv1.SubmitRelearnAnswerRequest{NoteId: first["alpha"].GetNoteId(), Answer: "wrong guess"}))
	require.NoError(t, err)
	assert.False(t, wrongResp.Msg.GetCorrect())

	// Skip delta.
	skipResp, err := h.SubmitRelearnAnswer(context.Background(),
		connect.NewRequest(&apiv1.SubmitRelearnAnswerRequest{NoteId: first["delta"].GetNoteId(), IsSkipped: true}))
	require.NoError(t, err)
	assert.False(t, skipResp.Msg.GetCorrect())

	// Neither is cleared: both still appear next session.
	next := relearnEntries(startRelearn(t, h, 24))
	assert.Contains(t, next, "alpha", "a wrong answer must not clear the word")
	assert.Contains(t, next, "delta", "a skip must not clear the word")
}

func TestRelearn_BatchSubmit(t *testing.T) {
	h, _ := newRelearnTestHandler(t)
	byEntry := relearnEntries(startRelearn(t, h, 24))

	resp, err := h.BatchSubmitRelearnAnswers(context.Background(),
		connect.NewRequest(&apiv1.BatchSubmitRelearnAnswersRequest{
			Answers: []*apiv1.SubmitRelearnAnswerRequest{
				{NoteId: byEntry["alpha"].GetNoteId(), Answer: "the first thing"},
				{NoteId: byEntry["delta"].GetNoteId(), Answer: "wrong"},
			},
		}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetResponses(), 2)
	assert.True(t, resp.Msg.GetResponses()[0].GetCorrect())
	assert.False(t, resp.Msg.GetResponses()[1].GetCorrect())

	// alpha cleared (correct), delta not (wrong).
	next := relearnEntries(startRelearn(t, h, 24))
	assert.NotContains(t, next, "alpha")
	assert.Contains(t, next, "delta")
}

func TestRelearn_SubmitUnknownCardIsNotFound(t *testing.T) {
	h, _ := newRelearnTestHandler(t)
	_ = startRelearn(t, h, 24)
	_, err := h.SubmitRelearnAnswer(context.Background(),
		connect.NewRequest(&apiv1.SubmitRelearnAnswerRequest{NoteId: 99999, Answer: "x"}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}
