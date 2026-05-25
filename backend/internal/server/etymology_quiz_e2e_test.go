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
	return newEtymologyTestHandlerWithDefinitions(t, openai, etymologyDir, learningDir, "")
}

// newEtymologyTestHandlerWithDefinitions is the variant used by tests that
// need a definitions notebook for the reader's SceneTitle projection.
func newEtymologyTestHandlerWithDefinitions(t *testing.T, openai inference.Client, etymologyDir, learningDir, definitionsDir string) *QuizHandler {
	t.Helper()
	cfg := config.NotebooksConfig{
		EtymologyDirectories:   []string{etymologyDir},
		LearningNotesDirectory: learningDir,
	}
	if definitionsDir != "" {
		cfg.DefinitionsDirectories = []string{definitionsDir}
	}
	svc := quiz.NewService(cfg, openai, make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})
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

// TestEtymologyFreeform_NextReviewDateAcrossModes reproduces the user-
// reported "Origin not found in notebooks" / "Found in notebooks +
// drillable" symptom for origins answered recently in any etymology mode.
//
// The trigger was a learning-history shape mismatch the post-migration
// data (history.metadata.title = SESSION title, e.g. "Session 2") didn't
// match the assumption baked into originNextReviewDate and
// originNeedsStudy (they compared against card.NotebookTitle = the BOOK
// name, e.g. "Word Power Made Easy"). When the comparison never matched,
// the function returned "" → frontend interpreted "no scheduled review"
// → showed "Found in notebooks" → user could redrill.
//
// Earlier server-level tests in this file seeded the LEGACY shape
// (history.metadata.title = book name + type: etymology) and happened to
// work because that shape coincidentally matched the buggy code's
// expectation. This test uses the POST-migration shape so the real bug
// path is exercised.
//
// Uses neutral Latin roots (none of which appear verbatim in any real
// user notebook).
func TestEtymologyFreeform_NextReviewDateAcrossModes(t *testing.T) {
	tmpDir := t.TempDir()
	etymologyDir := filepath.Join(tmpDir, "etymology")
	learningDir := filepath.Join(tmpDir, "learning")
	require.NoError(t, os.MkdirAll(learningDir, 0o755))

	nbDir := filepath.Join(etymologyDir, "demo-roots")
	require.NoError(t, os.MkdirAll(nbDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(nbDir, "index.yml"), []byte(`id: demo-roots
kind: Etymology
name: "Demo Roots"
notebooks:
  - ./session1.yml
`), 0o644))
	// Session whose metadata.title is "Session 1" (matches real-world
	// post-migration shape). Three origins:
	//   - alter-fake: drilled today in breakdown, interval=30 (NOT due)
	//   - other-fake: drilled today in assembly, interval=30 (NOT due)
	//   - third-fake: never touched (truly due)
	// Legacy flat-origin shape — same as the user's etymology source
	// files. SceneTitle gets populated by the reader's
	// pickBestSceneForOrigin from the definitions notebook below.
	require.NoError(t, os.WriteFile(filepath.Join(nbDir, "session1.yml"), []byte(`metadata:
  title: "Session 1"
origins:
  - origin: alter-fake
    type: root
    language: Latin
    meaning: other
  - origin: other-fake
    type: root
    language: Latin
    meaning: change
  - origin: third-fake
    type: root
    language: Latin
    meaning: third
`), 0o644))

	// Definitions notebook with scenes whose titles match the learning
	// history scenes below. The reader uses these to project SceneTitle
	// onto the legacy etymology origins.
	defDir := filepath.Join(tmpDir, "definitions", "demo-roots")
	require.NoError(t, os.MkdirAll(defDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(defDir, "index.yml"), []byte(`id: demo-roots
name: "Demo Roots"
notebooks:
  - ./session1.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(defDir, "session1.yml"), []byte(`- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        title: "alter-fake (other)"
      expressions:
        - expression: "demo-alter-word"
          meaning: "uses alter-fake"
          origin_parts:
            - origin: alter-fake
              language: Latin
    - metadata:
        title: "other-fake (change)"
      expressions:
        - expression: "demo-other-word"
          meaning: "uses other-fake"
          origin_parts:
            - origin: other-fake
              language: Latin
    - metadata:
        title: "third-fake (third)"
      expressions:
        - expression: "demo-third-word"
          meaning: "uses third-fake"
          origin_parts:
            - origin: third-fake
              language: Latin
`), 0o644))

	now := time.Now().UTC()
	today := now.Format(time.RFC3339)
	// CRITICAL: post-migration shape — top-level title is the SESSION
	// title, NOT the book name. Real user data looks exactly like this.
	historyYAML := `- metadata:
    id: demo-roots
    title: "Session 1"
  scenes:
    - metadata:
        title: "alter-fake (other)"
      expressions:
        - expression: alter-fake
          type: origin
          etymology_breakdown_logs:
            - status: usable
              learned_at: "` + today + `"
              quality: 4
              quiz_type: etymology_breakdown
              interval_days: 30
    - metadata:
        title: "other-fake (change)"
      expressions:
        - expression: other-fake
          type: origin
          etymology_assembly_logs:
            - status: usable
              learned_at: "` + today + `"
              quality: 4
              quiz_type: etymology_assembly
              interval_days: 30
`
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "demo-roots.yml"), []byte(historyYAML), 0o644))

	ctrl := gomock.NewController(t)
	openai := mock_inference.NewMockClient(ctrl)
	handler := newEtymologyTestHandlerWithDefinitions(t, openai, etymologyDir, learningDir, filepath.Join(tmpDir, "definitions"))
	ctx := context.Background()

	resp, err := handler.StartEtymologyFreeformQuiz(ctx, connect.NewRequest(&apiv1.StartEtymologyFreeformQuizRequest{
		EtymologyNotebookIds: []string{"demo-roots"},
	}))
	require.NoError(t, err)

	// All three origins are in the suggestion list (freeform doesn't filter).
	assert.ElementsMatch(t, []string{"alter-fake", "other-fake", "third-fake"}, resp.Msg.GetOrigins(),
		"freeform must list every origin so typed input resolves")

	// alter-fake and other-fake were both answered today with interval=30
	// in different modes. Both must report a future next-review date so
	// the frontend renders "Not until $date" and disables Submit.
	dates := resp.Msg.GetNextReviewDates()
	if assert.NotEmpty(t, dates["alter-fake"], "alter-fake answered today in breakdown — must have a future review date") {
		assert.True(t, dates["alter-fake"] > now.Format("2006-01-02"),
			"alter-fake's next review must be in the future; got %q", dates["alter-fake"])
	}
	if assert.NotEmpty(t, dates["other-fake"], "other-fake answered today in assembly — must have a future review date") {
		assert.True(t, dates["other-fake"] > now.Format("2006-01-02"),
			"other-fake's next review must be in the future; got %q", dates["other-fake"])
	}
	assert.Empty(t, dates["third-fake"],
		"third-fake has no logs — it's freely drillable, no nextReviewDate should be reported")
}

// TestSubmitEtymologyStandardAnswer_PreservesSkippedSiblings drives the user's
// suspected reproduction: in the SAME scene as the etymology origin being
// answered, two vocab-side stubs already exist with skipped_at set but no
// logs (the shape the notebook UI's per-type skip checkboxes leave behind).
// Submitting an etymology answer for the origin must NOT touch the
// SkippedAt of the sibling stubs, and must NOT delete the stubs.
//
// Reported symptom: "I skipped introvert and extrovert from my notebooks
// in the past and confirmed they were recorded appropriately, but they
// were deleted. When I checked the commit when skipped was deleted, it
// was just a normal update for vocabulary quiz or etymology's quiz's
// updates." This test exercises the etymology-side hypothesis end-to-end
// through the real SubmitEtymologyStandardAnswer RPC.
func TestSubmitEtymologyStandardAnswer_PreservesSkippedSiblings(t *testing.T) {
	tmpDir := t.TempDir()
	etymologyDir := filepath.Join(tmpDir, "etymology")
	learningDir := filepath.Join(tmpDir, "learning")
	require.NoError(t, os.MkdirAll(learningDir, 0o755))

	nbDir := filepath.Join(etymologyDir, "siblings")
	require.NoError(t, os.MkdirAll(nbDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(nbDir, "index.yml"), []byte(`id: siblings
kind: Etymology
name: Siblings
notebooks:
  - ./session1.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(nbDir, "session1.yml"), []byte(`metadata:
  title: "Session 1"
origins:
  - origin: turn-root
    language: Latin
    meaning: to turn
`), 0o644))

	// Learning history: Session 1 > "turn-root (to turn)" scene already
	// contains two SKIP-ONLY stubs ("inside-word" and "outside-word") plus
	// the etymology origin "turn-root" we're about to answer. Mirrors the
	// real scenario where the user skipped derived vocab words via the
	// notebook UI and the next quiz answer writes back to the same file.
	today := time.Now().UTC().Format(time.RFC3339)
	historyYAML := `- metadata:
    id: siblings
    title: "Session 1"
  scenes:
    - metadata:
        title: "turn-root (to turn)"
      expressions:
        - expression: inside-word
          learned_logs: []
          skipped_at:
            notebook: "` + today + `"
        - expression: outside-word
          learned_logs: []
          skipped_at:
            notebook: "` + today + `"
            reverse: "` + today + `"
        - expression: turn-root
          type: origin
          etymology_breakdown_logs:
            - status: understood
              learned_at: "2026-05-01T10:00:00Z"
              quiz_type: etymology_freeform
              interval_days: 1
`
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "siblings.yml"), []byte(historyYAML), 0o644))

	ctrl := gomock.NewController(t)
	openai := mock_inference.NewMockClient(ctrl)
	handler := newEtymologyTestHandler(t, openai, etymologyDir, learningDir)
	ctx := context.Background()

	startResp, err := handler.StartEtymologyQuiz(ctx, connect.NewRequest(&apiv1.StartEtymologyQuizRequest{
		EtymologyNotebookIds: []string{"siblings"},
		Mode:                 apiv1.EtymologyQuizMode_ETYMOLOGY_QUIZ_MODE_STANDARD,
		IncludeUnstudied:     true,
	}))
	require.NoError(t, err)
	require.Len(t, startResp.Msg.GetCards(), 1, "turn-root must surface as a standard-mode card")
	cardID := startResp.Msg.GetCards()[0].GetCardId()

	// Submit a correct answer for the origin — exact-match short-circuits OpenAI.
	_, err = handler.SubmitEtymologyStandardAnswer(ctx, connect.NewRequest(&apiv1.SubmitEtymologyStandardAnswerRequest{
		CardId:         cardID,
		Answer:         "to turn",
		ResponseTimeMs: 1234,
	}))
	require.NoError(t, err)

	// Read the file back and verify the sibling stubs still carry their skipped_at.
	raw, err := os.ReadFile(filepath.Join(learningDir, "siblings.yml"))
	require.NoError(t, err)
	var histories []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &histories))

	var insideExpr, outsideExpr *notebook.LearningHistoryExpression
	for hi := range histories {
		for si := range histories[hi].Scenes {
			for ei := range histories[hi].Scenes[si].Expressions {
				e := &histories[hi].Scenes[si].Expressions[ei]
				switch e.Expression {
				case "inside-word":
					insideExpr = e
				case "outside-word":
					outsideExpr = e
				}
			}
		}
	}

	require.NotNil(t, insideExpr, "inside-word must still exist after the etymology answer write")
	require.NotNil(t, outsideExpr, "outside-word must still exist after the etymology answer write")
	assert.True(t, insideExpr.SkippedAt.IsSkippedAny(),
		"inside-word's skipped_at must round-trip through the quiz save")
	assert.True(t, outsideExpr.SkippedAt.IsSkippedAny(),
		"outside-word's skipped_at must round-trip through the quiz save")
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

// TestEtymologyReverseQuiz_ClusterGraphCarriesMemberMeaning pins the
// behavior the user asked for: when the reverse quiz renders a CLUSTER
// graph (two members of one concept), every non-blank ORIGIN node
// carries its meaning prose from the etymology YAML — that's how
// inter-member relationships like "X (past participle of Y)" surface
// to the learner without a new graph shape or schema field.
//
// The fixture uses a made-up Latin verb / past-participle pair (writus /
// scripta — not in langner's actual data) so it doesn't reference the
// user's real notebook contents. The reverse quiz blanks one member;
// the OTHER member's node must include its meaning text.
func TestEtymologyReverseQuiz_ClusterGraphCarriesMemberMeaning(t *testing.T) {
	tmpDir := t.TempDir()
	etymologyDir := filepath.Join(tmpDir, "etymology")
	learningDir := filepath.Join(tmpDir, "learning")
	require.NoError(t, os.MkdirAll(learningDir, 0o755))

	nbDir := filepath.Join(etymologyDir, "demo-roots")
	require.NoError(t, os.MkdirAll(nbDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(nbDir, "index.yml"), []byte(`id: demo-roots
kind: Etymology
name: Demo Roots
notebooks:
  - ./session1.yml
`), 0o644))
	// Two members of one concept; one carries a relationship-bearing
	// meaning that references the other ("past participle of writus").
	require.NoError(t, os.WriteFile(filepath.Join(nbDir, "session1.yml"), []byte(`metadata:
  title: "Session 1"
origins:
  - origin: writus
    language: Latin
    meaning: to write
  - origin: scripta
    language: Latin
    meaning: "written (past participle of writus)"
concepts:
  - key: writing
    meaning: to write
    members:
      - { origin: writus,  language: Latin }
      - { origin: scripta, language: Latin }
`), 0o644))
	// Both origins need a freeform pass + a correct answer to clear the
	// standard-quiz eligibility gate so the reverse quiz can also pick
	// them up.
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "demo-roots.yml"), []byte(`- metadata:
    id: demo-roots
    title: Demo Roots
  scenes:
    - metadata:
        title: "Session 1"
      expressions:
        - expression: writus
          learned_logs: []
          etymology_breakdown_logs:
            - status: understood
              learned_at: "2025-01-01"
              quiz_type: etymology_freeform
              interval_days: 7
        - expression: scripta
          learned_logs: []
          etymology_breakdown_logs:
            - status: understood
              learned_at: "2025-01-01"
              quiz_type: etymology_freeform
              interval_days: 7
`), 0o644))

	ctrl := gomock.NewController(t)
	openai := mock_inference.NewMockClient(ctrl)
	openai.EXPECT().ValidateWordForm(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ inference.ValidateWordFormRequest) (inference.ValidateWordFormResponse, error) {
			return inference.ValidateWordFormResponse{Classification: inference.ClassificationSameWord, Reason: "ok", Quality: 4}, nil
		}).AnyTimes()

	handler := newEtymologyTestHandler(t, openai, etymologyDir, learningDir)
	ctx := context.Background()

	startResp, err := handler.StartEtymologyQuiz(ctx, connect.NewRequest(&apiv1.StartEtymologyQuizRequest{
		EtymologyNotebookIds: []string{"demo-roots"},
		Mode:                 apiv1.EtymologyQuizMode_ETYMOLOGY_QUIZ_MODE_REVERSE,
		IncludeUnstudied:     true,
	}))
	require.NoError(t, err)
	cards := startResp.Msg.GetCards()
	require.NotEmpty(t, cards, "reverse quiz must produce at least one card")

	// Find the card whose graph is a CLUSTER (the only shape that
	// renders sibling members for this fixture — there's no antonym
	// relation and only one origin per member key).
	var clusterPrompt *apiv1.GraphPrompt
	for _, c := range cards {
		gp := c.GetGraphPrompt()
		if gp != nil && gp.GetShape() == apiv1.GraphPrompt_CLUSTER {
			clusterPrompt = gp
			break
		}
	}
	require.NotNil(t, clusterPrompt, "expected at least one CLUSTER graph prompt across the cards")

	// One member is the blank (its label is empty, meaning is also
	// empty so the answer doesn't leak). The OTHER member must carry
	// its meaning prose.
	var foundNonBlankMeaning string
	for _, n := range clusterPrompt.GetNodes() {
		if n.GetKind() != apiv1.GraphNode_ORIGIN {
			continue
		}
		if n.GetId() == clusterPrompt.GetBlankNodeId() {
			assert.Empty(t, n.GetMeaning(),
				"blank node must NOT carry meaning (would leak the answer)")
			continue
		}
		assert.NotEmpty(t, n.GetMeaning(),
			"non-blank ORIGIN node must carry its YAML meaning prose")
		foundNonBlankMeaning = n.GetMeaning()
	}
	// Whichever side was the blank, the meaning surfaced should be one
	// of the two YAML glosses. The "past participle of writus" gloss is
	// the one that proves the relationship info reaches the graph; we
	// accept either since which side gets blanked is a card-selection
	// choice.
	require.NotEmpty(t, foundNonBlankMeaning)
	assert.Contains(t,
		[]string{"to write", "written (past participle of writus)"},
		foundNonBlankMeaning,
		"non-blank meaning should match one of the YAML origins' glosses")
}
