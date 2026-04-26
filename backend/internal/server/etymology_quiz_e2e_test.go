package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
