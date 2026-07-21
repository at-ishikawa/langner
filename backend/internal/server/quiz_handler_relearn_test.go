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
    - expression: "epsilon"
      meaning: "the fifth thing"
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
    - expression: "epsilon"
      learned_logs:
        - status: "misunderstood"
          learned_at: %q
          quiz_type: "notebook"
      reverse_logs:
        - status: "misunderstood"
          learned_at: %q
          quiz_type: "reverse"
`, recent, recent, old, veryRecent, recent, recent)
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

// relearnByEntryType keys cards by "entry/source_quiz_type" so a word failed in
// more than one quiz type (and thus present as several cards) stays distinct.
func relearnByEntryType(cards []*apiv1.RelearnCard) map[string]*apiv1.RelearnCard {
	out := make(map[string]*apiv1.RelearnCard, len(cards))
	for _, c := range cards {
		out[c.GetEntry()+"/"+c.GetSourceQuizType().String()] = c
	}
	return out
}

func TestRelearn_MirrorsEachSourceQuizType(t *testing.T) {
	h, _ := newRelearnTestHandler(t)
	byKey := relearnByEntryType(startRelearn(t, h, 24))

	// alpha was failed in the notebook (recognition) quiz.
	alpha := byKey["alpha/QUIZ_TYPE_STANDARD"]
	require.NotNil(t, alpha)
	assert.NotEmpty(t, alpha.GetExamples(), "recognition cards carry examples as a hint")

	// delta was failed in the reverse quiz: it must carry the meaning as the
	// prompt and masked contexts as the hint (not examples).
	delta := byKey["delta/QUIZ_TYPE_REVERSE"]
	require.NotNil(t, delta)
	assert.Equal(t, "a change or difference", delta.GetMeaning(), "reverse card prompts with the meaning")
}

func TestRelearn_WordFailedInTwoTypesYieldsTwoCards(t *testing.T) {
	h, _ := newRelearnTestHandler(t)
	byKey := relearnByEntryType(startRelearn(t, h, 24))

	assert.Contains(t, byKey, "epsilon/QUIZ_TYPE_STANDARD", "recognition card for the standard failure")
	assert.Contains(t, byKey, "epsilon/QUIZ_TYPE_REVERSE", "reverse card for the reverse failure")
}

func TestRelearn_ReverseCardIsGradedByTheWordNotTheMeaning(t *testing.T) {
	// The mock reverse grader marks correct only when the answer matches the
	// expected WORD (same_word). Typing the meaning must be wrong — this is the
	// bug the mirror rework fixes. Each answer uses a fresh handler so the two
	// submissions stay independent.
	submitDelta := func(ans string) bool {
		h, _ := newRelearnTestHandler(t)
		id := relearnByEntryType(startRelearn(t, h, 24))["delta/QUIZ_TYPE_REVERSE"].GetNoteId()
		resp, err := h.SubmitRelearnAnswer(context.Background(),
			connect.NewRequest(&apiv1.SubmitRelearnAnswerRequest{NoteId: id, Answer: ans}))
		require.NoError(t, err)
		return resp.Msg.GetCorrect()
	}
	assert.True(t, submitDelta("delta"), "typing the WORD is correct in a reverse card")
	assert.False(t, submitDelta("a change or difference"), "typing the MEANING is wrong in a reverse card")
}

// TestRelearn_NoteIDsStableAcrossRestarts guards the card-store desync fix:
// note_id is a stable hash of the card, so two StartRelearn calls hand the
// same card the same id. The previous code assigned sequential ids in random
// map-iteration order, so a re-start silently repointed a note_id at a
// different card.
func TestRelearn_NoteIDsStableAcrossRestarts(t *testing.T) {
	h, _ := newRelearnTestHandler(t)

	first := relearnByEntryType(startRelearn(t, h, 24))
	second := relearnByEntryType(startRelearn(t, h, 24))

	require.NotEmpty(t, first)
	require.Len(t, second, len(first))
	for key, c1 := range first {
		c2, ok := second[key]
		require.True(t, ok, "card %q must appear in both starts", key)
		assert.Equal(t, c1.GetNoteId(), c2.GetNoteId(),
			"note_id for %q must be identical across StartRelearn calls", key)
	}
}

// TestRelearn_HeldNoteIDGradesTheCardItWasShownFor reproduces the reported bug:
// a learner starts Relearn, then a second StartRelearn happens within the
// window (another tab, or re-entering the session). Grading a note_id the
// client is still holding must resolve to the SAME card it was shown — the
// meaning returned matches the prompt and a correct answer is graded correct —
// instead of whatever card a reassigned sequential id happened to land on.
func TestRelearn_HeldNoteIDGradesTheCardItWasShownFor(t *testing.T) {
	h, _ := newRelearnTestHandler(t)

	held := relearnByEntryType(startRelearn(t, h, 24))["alpha/QUIZ_TYPE_STANDARD"]
	require.NotNil(t, held)

	// A second start replaces/extends the server store (the desync trigger).
	_ = startRelearn(t, h, 24)

	resp, err := h.SubmitRelearnAnswer(context.Background(),
		connect.NewRequest(&apiv1.SubmitRelearnAnswerRequest{NoteId: held.GetNoteId(), Answer: "the first thing"}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.GetCorrect(), "the held note_id still grades the card it was shown for")
	assert.Equal(t, held.GetMeaning(), resp.Msg.GetMeaning(),
		"the graded card's meaning matches the one the learner saw — no cross-card desync")
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

func TestRelearn_CorrectAnswerWritesNoHistoryAndWordIsRepeatable(t *testing.T) {
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

	// Relearn is repeatable: a correct answer persists no state, so the word is
	// still in the pool next session (it ages out of the window or is fixed in a
	// real quiz — not here).
	next := relearnEntries(startRelearn(t, h, 24))
	assert.Contains(t, next, "alpha", "a relearned word must reappear — relearn stores no clear state")
	assert.Contains(t, next, "delta")
}

func TestRelearn_WrongAndSkippedKeepWordInPool(t *testing.T) {
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

	// Relearn persists nothing, so both still appear next session.
	next := relearnEntries(startRelearn(t, h, 24))
	assert.Contains(t, next, "alpha", "a wrong answer leaves the word in the pool")
	assert.Contains(t, next, "delta", "a skip leaves the word in the pool")
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

	// A batch persists nothing either: both words remain in the pool.
	next := relearnEntries(startRelearn(t, h, 24))
	assert.Contains(t, next, "alpha")
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
