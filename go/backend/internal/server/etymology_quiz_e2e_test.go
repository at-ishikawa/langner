package server

import (
	"context"
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
// notebook lists the same origin multiple times within a session file, those
// duplicates leaked through StartEtymologyQuiz into the cards array and then
// into the per-card save loop in BatchSubmitEtymologyStandardAnswers,
// producing one save (and one feedback entry) per duplicate.
//
// This test seeds a notebook session that has the same two origins repeated
// 4 and 3 times respectively (mimicking the user's "4 derma, 3 ophthalmos"
// symptom with generic Greek-root data) and asserts:
//
//  1. StartEtymologyQuiz returns exactly 2 cards (not 7).
//  2. Submitting answers for the 2 returned cards produces exactly 2
//     responses and writes exactly 1 etymology_breakdown_logs entry per
//     origin to the learning history file (not 4 + 3 = 7).
//
// Cross-session "duplicates" (same origin string across two sessions) are
// intentionally NOT collapsed — those are multi-sense origins. The within-
// session dedup that this test pins down still applies under the new keying.
func TestEtymologyStandardQuiz_DuplicatesInSeedYieldOneCardAndOneSavePerOrigin(t *testing.T) {
	tmpDir := t.TempDir()
	etymologyDir := filepath.Join(tmpDir, "etymology")
	learningDir := filepath.Join(tmpDir, "learning")
	require.NoError(t, os.MkdirAll(learningDir, 0o755))

	// Create an etymology notebook whose session file repeats the same two
	// origins 4 and 3 times respectively. This mirrors how a user's data
	// drifts when they accidentally record the same root multiple times in
	// the same session file.
	nbDir := filepath.Join(etymologyDir, "greek-roots")
	require.NoError(t, os.MkdirAll(nbDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(nbDir, "index.yml"), []byte(`id: greek-roots
kind: Etymology
name: Greek Roots
notebooks:
  - ./session1.yml
`), 0o644))
	// 4× tele + 3× graph all in one session → still 2 unique origins.
	require.NoError(t, os.WriteFile(filepath.Join(nbDir, "session1.yml"), []byte(`metadata:
  title: "Session 1"
origins:
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
  scenes:
    - metadata:
        title: "Session 1"
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

// TestEtymologyReverseQuiz_DoesNotRepeatJustAnsweredOrigin reproduces, end
// to end via the real RPC, the case where an origin's standard-direction SR
// interval is overdue but its reverse-direction interval is not. The bug:
// StartEtymologyQuiz used etymology_breakdown_logs for the SR-due check
// regardless of mode, so reverse mode kept returning origins the user had
// just answered correctly in reverse the same day.
//
// Setup: one origin "spectro", same session.
//   - etymology_breakdown_logs[0]: 10 days ago, interval=7  → standard track is overdue
//   - etymology_assembly_logs[0]:  today,       interval=30 → reverse track is fresh
//
// Expectation:
//   - StartEtymologyQuiz(STANDARD) returns 1 card (standard is overdue).
//   - StartEtymologyQuiz(REVERSE)  returns 0 cards (reverse just answered).
func TestEtymologyReverseQuiz_DoesNotRepeatJustAnsweredOrigin(t *testing.T) {
	tmpDir := t.TempDir()
	etymologyDir := filepath.Join(tmpDir, "etymology")
	learningDir := filepath.Join(tmpDir, "learning")
	require.NoError(t, os.MkdirAll(learningDir, 0o755))

	nbDir := filepath.Join(etymologyDir, "test-roots")
	require.NoError(t, os.MkdirAll(nbDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(nbDir, "index.yml"), []byte(`id: test-roots
kind: Etymology
name: Test Roots
notebooks:
  - ./session1.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(nbDir, "session1.yml"), []byte(`metadata:
  title: "Session 1"
origins:
  - origin: spectro
    type: root
    language: Latin
    meaning: to look closely
`), 0o644))

	now := time.Now().UTC()
	tenDaysAgo := now.AddDate(0, 0, -10).Format(time.RFC3339)
	today := now.Format(time.RFC3339)
	historyYAML := `- metadata:
    id: test-roots
    title: Test Roots
    type: etymology
  scenes:
    - metadata:
        title: "Session 1"
      expressions:
        - expression: spectro
          learned_logs: []
          etymology_breakdown_logs:
            - status: usable
              learned_at: "` + tenDaysAgo + `"
              quality: 4
              quiz_type: etymology_breakdown
              interval_days: 7
            - status: understood
              learned_at: "` + tenDaysAgo + `"
              quality: 4
              quiz_type: etymology_freeform
              interval_days: 7
          etymology_assembly_logs:
            - status: understood
              learned_at: "` + today + `"
              quality: 5
              quiz_type: etymology_assembly
              interval_days: 30
`
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-roots.yml"), []byte(historyYAML), 0o644))

	ctrl := gomock.NewController(t)
	openai := mock_inference.NewMockClient(ctrl)
	handler := newEtymologyTestHandler(t, openai, etymologyDir, learningDir)
	ctx := context.Background()

	standardResp, err := handler.StartEtymologyQuiz(ctx, connect.NewRequest(&apiv1.StartEtymologyQuizRequest{
		EtymologyNotebookIds: []string{"test-roots"},
		Mode:                 apiv1.EtymologyQuizMode_ETYMOLOGY_QUIZ_MODE_STANDARD,
	}))
	require.NoError(t, err)
	assert.Len(t, standardResp.Msg.GetCards(), 1,
		"standard track is overdue (10 days ago, interval=7) — quiz must surface the card")

	reverseResp, err := handler.StartEtymologyQuiz(ctx, connect.NewRequest(&apiv1.StartEtymologyQuizRequest{
		EtymologyNotebookIds: []string{"test-roots"},
		Mode:                 apiv1.EtymologyQuizMode_ETYMOLOGY_QUIZ_MODE_REVERSE,
	}))
	require.NoError(t, err)
	assert.Empty(t, reverseResp.Msg.GetCards(),
		"reverse track was answered today with interval=30 — quiz must NOT surface the card again")
}

// countNewBreakdownLogs reads a learning-history YAML and returns the number
// of breakdown log entries with quiz_type=etymology_breakdown (i.e. saves
// from a standard quiz, not the seed freeform entries). The new etymology
// learning-history shape is nested: scenes[].expressions[], so we walk both.
func countNewBreakdownLogs(t *testing.T, path string) (tele, graph int) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var histories []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(data, &histories))
	count := func(e notebook.LearningHistoryExpression) {
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
	for _, h := range histories {
		for _, e := range h.Expressions {
			count(e)
		}
		for _, scene := range h.Scenes {
			for _, e := range scene.Expressions {
				count(e)
			}
		}
	}
	return tele, graph
}
