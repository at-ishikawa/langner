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
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/learning"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
)

// newEtymologyTestHandler builds a real QuizHandler backed by a real Service
// reading from on-disk YAML fixtures in tmpDir. Only the OpenAI client is a
// gomock stub — every other layer (notebook reader, learning history updater,
// service business logic, RPC handler) runs production code.
func newEtymologyTestHandler(t *testing.T, openai inference.Client, etymologyDir, learningDir string) *QuizHandler {
	t.Helper()
	svc := quiz.NewService(config.NotebooksConfig{
		EtymologyDirectories:   []string{etymologyDir},
		LearningNotesDirectory: learningDir,
	}, openai, make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})
	return NewQuizHandler(svc)
}

// TestEtymologyStandardQuiz_DuplicatesInSeedYieldOneCardAndOneSavePerOrigin
// reproduces the user-reported bug end-to-end through the real RPC layer.
//
// Symptom (from a real session): "I only answered 2 origins, but the feedback
// screen showed 7 entries (4 of one origin + 3 of another), and the progress
// bar advanced by 3 instead of 2." The hypothesis was that when the etymology
// notebook lists the same origin multiple times across session files, those
// duplicates leaked through StartEtymologyQuiz into the cards array and then
// into the per-card save loop in BatchSubmitEtymologyStandardAnswers,
// producing one save (and one feedback entry) per duplicate.
//
// This test seeds a notebook that has the same two origins repeated 4 and 3
// times respectively (mimicking the user's "4 derma, 3 ophthalmos" symptom
// with generic Greek-root data) and asserts:
//
//  1. StartEtymologyQuiz returns exactly 2 cards (not 7).
//  2. Submitting answers for the 2 returned cards produces exactly 2
//     responses and writes exactly 1 etymology_breakdown_logs entry per
//     origin to the learning history file (not 4 + 3 = 7).
func TestEtymologyStandardQuiz_DuplicatesInSeedYieldOneCardAndOneSavePerOrigin(t *testing.T) {
	tmpDir := t.TempDir()
	etymologyDir := filepath.Join(tmpDir, "etymology")
	learningDir := filepath.Join(tmpDir, "learning")
	require.NoError(t, os.MkdirAll(learningDir, 0o755))

	// Create an etymology notebook whose session files repeat the same two
	// origins 4 and 3 times respectively. This mirrors how a user's data
	// drifts when they record the same root in multiple daily session files.
	nbDir := filepath.Join(etymologyDir, "greek-roots")
	require.NoError(t, os.MkdirAll(nbDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(nbDir, "index.yml"), []byte(`id: greek-roots
kind: Etymology
name: Greek Roots
notebooks:
  - ./session1.yml
  - ./session2.yml
`), 0o644))
	// 4× tele + 1× graph in session1, 2× graph in session2 → 4 tele + 3 graph total.
	require.NoError(t, os.WriteFile(filepath.Join(nbDir, "session1.yml"), []byte(`origins:
  - origin: tele
    type: root
    language: Greek
    meaning: distant, far off
  - origin: tele
    type: root
    language: Greek
    meaning: distant, far off
  - origin: tele
    type: root
    language: Greek
    meaning: distant, far off
  - origin: tele
    type: root
    language: Greek
    meaning: distant, far off
  - origin: graph
    type: root
    language: Greek
    meaning: to write
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(nbDir, "session2.yml"), []byte(`origins:
  - origin: graph
    type: root
    language: Greek
    meaning: to write
  - origin: graph
    type: root
    language: Greek
    meaning: to write
`), 0o644))

	// Both origins must have a freeform-mode answer + at least one correct
	// etymology answer to pass the standard-quiz eligibility gate.
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "greek-roots.yml"), []byte(`- metadata:
    id: greek-roots
    title: Greek Roots
    type: etymology
  expressions:
    - expression: tele
      learned_logs: []
      etymology_breakdown_logs:
        - status: understood
          learned_at: "2025-01-01"
          quiz_type: etymology_freeform
          interval_days: 7
    - expression: graph
      learned_logs: []
      etymology_breakdown_logs:
        - status: understood
          learned_at: "2025-01-01"
          quiz_type: etymology_freeform
          interval_days: 7
`), 0o644))

	ctrl := gomock.NewController(t)
	openai := mock_inference.NewMockClient(ctrl)
	// Both origins will be due for review (last review > 7 days ago).
	openai.EXPECT().ValidateWordForm(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req inference.ValidateWordFormRequest) (inference.ValidateWordFormResponse, error) {
			return inference.ValidateWordFormResponse{
				Classification: inference.ClassificationSameWord,
				Reason:         "ok",
				Quality:        4,
			}, nil
		}).AnyTimes()

	handler := newEtymologyTestHandler(t, openai, etymologyDir, learningDir)
	ctx := context.Background()

	// 1. Start the quiz and assert the cards array has exactly 2 entries.
	startResp, err := handler.StartEtymologyQuiz(ctx, connect.NewRequest(&apiv1.StartEtymologyQuizRequest{
		EtymologyNotebookIds: []string{"greek-roots"},
		Mode:                 apiv1.EtymologyQuizMode_ETYMOLOGY_QUIZ_MODE_STANDARD,
		IncludeUnstudied:     true,
	}))
	require.NoError(t, err)
	cards := startResp.Msg.GetCards()

	assert.Len(t, cards, 2,
		"start quiz must return one card per unique origin; got %d (means duplicates leaked into the quiz)", len(cards))

	// 2. Submit answers for whatever the handler returned (could be 2 or, if
	//    the bug is present, 7) and assert the per-origin save count.
	answers := make([]*apiv1.SubmitEtymologyStandardAnswerRequest, len(cards))
	for i, c := range cards {
		answers[i] = &apiv1.SubmitEtymologyStandardAnswerRequest{
			CardId:         c.CardId,
			Answer:         "stand-in answer",
			ResponseTimeMs: 1000,
		}
	}
	batchResp, err := handler.BatchSubmitEtymologyStandardAnswers(ctx,
		connect.NewRequest(&apiv1.BatchSubmitEtymologyStandardAnswersRequest{Answers: answers}))
	require.NoError(t, err)
	assert.Len(t, batchResp.Msg.GetResponses(), len(cards),
		"the response count must match the request count")

	// 3. Read the learning-history file directly and verify exactly one new
	//    breakdown_log per origin was appended (not one per duplicate).
	teleNew, graphNew := countNewBreakdownLogs(t, filepath.Join(learningDir, "greek-roots.yml"))
	assert.Equal(t, 1, teleNew, "exactly one new etymology_breakdown_logs entry should be saved for 'tele'")
	assert.Equal(t, 1, graphNew, "exactly one new etymology_breakdown_logs entry should be saved for 'graph'")
}

// TestVocabularyFreeformQuiz_LoadsWordsFromDefinitionsOnlyBook reproduces a
// user-reported bug: an expression registered in a definitions-only book
// (notebooks/definitions/books/<id>/session*.yml indexed by index.yml) was
// not selectable in the vocabulary freeform quiz, even though it had a
// `meaning` field set. The expected behavior is that StartFreeformQuiz
// surfaces every expression with a meaning from every kind of notebook the
// reader knows about — story, flashcard, AND definitions-only book — so the
// frontend's expression list shows it. This test pins that contract end-to-
// end through the real RPC handler with a generic seeded book.
func TestVocabularyFreeformQuiz_LoadsWordsFromDefinitionsOnlyBook(t *testing.T) {
	tmpDir := t.TempDir()
	booksDir := filepath.Join(tmpDir, "definitions", "books")
	bookDir := filepath.Join(booksDir, "test-vocab")
	require.NoError(t, os.MkdirAll(bookDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "index.yml"), []byte(`id: test-vocab
notebooks:
  - ./session1.yml
`), 0o644))
	// One expression has a meaning AND origin_parts (mirrors the real-world
	// shape that triggered the report); a sibling expression in the same
	// scene has only origin_parts (no meaning). We assert only the one with a
	// meaning surfaces — the meaning-less one is intentionally excluded
	// because freeform grading needs a reference meaning to compare against.
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "session1.yml"), []byte(`- metadata:
    title: Lesson One
  scenes:
    - metadata:
        index: 0
        scene: null
        title: stargazers
      expressions:
        - expression: stargazer-with-meaning
          meaning: a person who studies the stars
          origin_parts:
            - origin: star
              language: English
            - origin: gaze
        - expression: stargazer-without-meaning
          origin_parts:
            - origin: star
              language: English
`), 0o644))

	ctrl := gomock.NewController(t)
	openai := mock_inference.NewMockClient(ctrl)
	openai.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ inference.AnswerMeaningsRequest) (inference.AnswerMeaningsResponse, error) {
			return inference.AnswerMeaningsResponse{
				Answers: []inference.AnswerMeaning{{
					Expression: "stargazer-with-meaning",
					Meaning:    "a person who studies the stars",
					AnswersForContext: []inference.AnswersForContext{{
						Correct: true, Reason: "matches", Quality: 4,
					}},
				}},
			}, nil
		}).AnyTimes()

	learningDir := t.TempDir()
	svc := quiz.NewService(config.NotebooksConfig{
		DefinitionsDirectories: []string{filepath.Join(tmpDir, "definitions")},
		LearningNotesDirectory: learningDir,
	}, openai, make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})
	handler := NewQuizHandler(svc)

	startResp, err := handler.StartFreeformQuiz(context.Background(),
		connect.NewRequest(&apiv1.StartFreeformQuizRequest{}))
	require.NoError(t, err)

	expressions := startResp.Msg.GetExpressions()
	assert.Contains(t, expressions, "stargazer-with-meaning",
		"a definitions-book expression with a meaning must be selectable in freeform")
	assert.NotContains(t, expressions, "stargazer-without-meaning",
		"expressions without a meaning have no reference for grading and must be excluded")

	submitResp, err := handler.SubmitFreeformAnswer(context.Background(),
		connect.NewRequest(&apiv1.SubmitFreeformAnswerRequest{
			Word:           "stargazer-with-meaning",
			Meaning:        "a person who studies the stars",
			ResponseTimeMs: 2000,
		}))
	require.NoError(t, err)
	assert.True(t, submitResp.Msg.GetCorrect(),
		"submitting the expected meaning must grade as correct, not get rejected with 'not found in any notebook'")
	assert.NotContains(t, submitResp.Msg.GetReason(), "not found",
		"the answer must not be rejected as 'not found in any notebook'")
}

// TestVocabularyFreeformQuiz_MisunderstoodWordIsImmediatelyRetryable
// reproduces a user report: a vocabulary word in a definitions-only book
// could not be re-attempted in freeform after a single wrong answer. The
// SR calculator wrote interval_days=1 for the wrong answer (status:
// misunderstood, quality: 1) — a sensible "show again tomorrow" interval
// for the spaced-repetition schedule — but freeform's UI keys off
// nextReviewDate to disable the Submit button, so the user was locked
// out of the word for a full day with no way to retry.
//
// Other quiz modes special-case misunderstood (NeedsForwardReview,
// NeedsReverseReview, NeedsEtymologyReview all return true on
// misunderstood regardless of stored interval). Freeform's
// NextReviewDates path was missing that override.
//
// Expected: GetFreeformNextReviewDates does NOT include a future date
// for an expression whose latest log is "misunderstood", regardless of
// the stored interval_days. The frontend then treats the word as due
// and the Submit button stays enabled.
func TestVocabularyFreeformQuiz_MisunderstoodWordIsImmediatelyRetryable(t *testing.T) {
	tmpDir := t.TempDir()
	booksDir := filepath.Join(tmpDir, "definitions", "books")
	bookDir := filepath.Join(booksDir, "test-vocab")
	require.NoError(t, os.MkdirAll(bookDir, 0o755))
	learningDir := filepath.Join(tmpDir, "learning_notes")
	require.NoError(t, os.MkdirAll(learningDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "index.yml"), []byte(`id: test-vocab
notebooks:
  - ./session1.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "session1.yml"), []byte(`- metadata:
    title: Lesson One
  scenes:
    - metadata:
        index: 0
        scene: null
        title: stargazers
      expressions:
        - expression: stargazer
          meaning: a person who studies the stars
`), 0o644))

	// Seed a freeform "wrong answer" entry — the stored interval is 1 day
	// (today → tomorrow) but status is misunderstood, so the word is *not*
	// truly mastered for tomorrow; the user just got it wrong and should
	// be able to retry now.
	today := time.Now().Format("2006-01-02T15:04:05-07:00")
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-vocab.yml"), []byte(fmt.Sprintf(`- metadata:
    id: test-vocab
    title: Lesson One
  scenes:
    - metadata:
        title: __index_0
      expressions:
        - expression: stargazer
          learned_logs:
            - status: misunderstood
              learned_at: %q
              quality: 1
              quiz_type: freeform
              interval_days: 1
`, today)), 0o644))

	svc := quiz.NewService(config.NotebooksConfig{
		DefinitionsDirectories: []string{filepath.Join(tmpDir, "definitions")},
		LearningNotesDirectory: learningDir,
	}, nil, make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})
	handler := NewQuizHandler(svc)

	resp, err := handler.StartFreeformQuiz(context.Background(),
		connect.NewRequest(&apiv1.StartFreeformQuizRequest{}))
	require.NoError(t, err)

	expressions := resp.Msg.GetExpressions()
	assert.Contains(t, expressions, "stargazer")

	nextDates := resp.Msg.GetExpressionNextReviewDate()
	_, hasFutureDate := nextDates["stargazer"]
	assert.False(t, hasFutureDate,
		"a misunderstood word must NOT carry a future next-review date — the user just got it wrong and should be able to retry immediately, not be locked out of the freeform Submit button until tomorrow; got %v",
		nextDates["stargazer"])
}

// countNewBreakdownLogs reads a learning-history YAML and returns the number
// of breakdown log entries with quiz_type=etymology_breakdown (i.e. saves
// from a standard quiz, not the seed freeform entries).
func countNewBreakdownLogs(t *testing.T, path string) (tele, graph int) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var histories []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(data, &histories))
	for _, h := range histories {
		for _, e := range h.Expressions {
			for _, log := range e.EtymologyBreakdownLogs {
				if log.QuizType != string(notebook.QuizTypeEtymologyStandard) {
					continue
				}
				switch e.Expression {
				case "tele":
					tele++
				case "graph":
					graph++
				}
			}
		}
	}
	return tele, graph
}
