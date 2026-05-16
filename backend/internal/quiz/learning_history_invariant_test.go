package quiz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/learning"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// TestLearningHistory_OneLocationPerExpression_AcrossAllWriters pins the
// invariant that every code path that mutates a learning_notes YAML places
// the same (notebook_id, expression) at the SAME (story_title, scene_title)
// location. When two writers disagree on the location, updates from one
// path are invisible to the other and the YAML accumulates duplicate,
// fragmented records — exactly the bug the user reported in
// word-power-made-easy.yml.
//
// SKIPPED today because the etymology quiz writer diverges from
// standard/reverse/freeform: etymology writes (notebook_name, session_title)
// while the others write (story_event, scene_title). Re-enable this test
// once the canonical shape is unified — see PR description for the deferred
// migration plan. Removing t.Skip is the first step of that migration.
//
// Writer matrix — every "live" code path that mutates a learning_notes YAML
// for a single notebook ID. Adding a new writer must add a row below or
// the matrix silently misses it; the comment is an explicit instruction
// to the person extending the code.
//
//	Writer                              | Code path
//	------------------------------------|----------------------------------
//	standard quiz answer                | Service.SaveResult
//	reverse quiz answer                 | Service.SaveReverseResult
//	freeform quiz answer                | Service.SaveFreeformResult
//	etymology answer (any quiz mode)    | Service.SaveEtymologyOriginResult
//	per-type skip                       | Service.SkipWord
//	per-type resume                     | Service.ResumeWord
//	override answer toggle              | Service.OverrideAnswer
//	normalisation pass                  | Validator.Fix
//
// One-shot migrations like cli/migrate_learning_history.go are excluded —
// they intentionally rewrite shapes and aren't subject to the invariant.
func TestLearningHistory_OneLocationPerExpression_AcrossAllWriters(t *testing.T) {
	dir := t.TempDir()
	storiesDir := filepath.Join(dir, "stories")
	etymDir := filepath.Join(dir, "etymology")
	learningDir := filepath.Join(dir, "learning")
	require.NoError(t, os.MkdirAll(storiesDir, 0755))
	require.NoError(t, os.MkdirAll(etymDir, 0755))
	require.NoError(t, os.MkdirAll(learningDir, 0755))

	// One notebook id ("dual"), simultaneously a story-style notebook AND
	// an etymology source — the configuration in which the divergence
	// originally appeared. Vocabulary writes target "introvert" (a full
	// word in a story scene); etymology writes target "intro" (a Latin
	// prefix). In real data these never collide — origins are
	// prefixes/suffixes/roots, not full words.
	const notebookID = "dual"
	const vocabExpr = "introvert"
	const etymExpr = "intro"

	storyNotebookDir := filepath.Join(storiesDir, notebookID)
	require.NoError(t, os.MkdirAll(storyNotebookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(storyNotebookDir, "index.yml"), []byte(`id: dual
name: Dual Notebook
notebooks:
  - ./session8.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(storyNotebookDir, "session8.yml"), []byte(`- event: "Session 8"
  date: 2025-01-15T00:00:00Z
  scenes:
    - scene: "psyche + intro"
      conversations:
        - speaker: "A"
          quote: "She is an introvert."
      definitions:
        - expression: "introvert"
          meaning: "a quiet, inwardly-focused person"
          origin_parts:
            - origin: "intro"
              language: Latin
`), 0644))

	etymNotebookDir := filepath.Join(etymDir, notebookID)
	require.NoError(t, os.MkdirAll(etymNotebookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(etymNotebookDir, "index.yml"), []byte(`id: dual
kind: Etymology
name: Dual Notebook
notebooks:
  - ./session8.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(etymNotebookDir, "session8.yml"), []byte(`metadata:
  title: "Session 8"
origins:
  - origin: "intro"
    type: prefix
    language: Latin
    meaning: into, within
`), 0644))

	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	// Grading is irrelevant here — the test exercises Save* and friends
	// directly with synthesised GradeResults. Stub generously.
	mockClient.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).
		Return(inference.AnswerMeaningsResponse{
			Answers: []inference.AnswerMeaning{{
				Expression: vocabExpr,
				AnswersForContext: []inference.AnswersForContext{
					{Correct: true, Reason: "stub", Quality: 4},
				},
			}},
		}, nil).
		AnyTimes()

	svc := NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{storiesDir},
		EtymologyDirectories:   []string{etymDir},
		LearningNotesDirectory: learningDir,
	}, mockClient, make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil),
		config.QuizConfig{})

	ctx := context.Background()

	// 1. standard quiz answer — Service.SaveResult
	require.NoError(t, svc.SaveResult(ctx, Card{
		NotebookName: notebookID, StoryTitle: "Session 8", SceneTitle: "psyche + intro",
		Entry: vocabExpr, Meaning: "a quiet person",
	}, GradeResult{Correct: true, Quality: 4}, 1000))

	// 2. reverse quiz answer — Service.SaveReverseResult
	require.NoError(t, svc.SaveReverseResult(ctx, ReverseCard{
		NotebookName: notebookID, StoryTitle: "Session 8", SceneTitle: "psyche + intro",
		Expression: vocabExpr, Meaning: "a quiet person",
	}, GradeResult{Correct: true, Quality: 4}, 1000))

	// 3. freeform quiz answer — Service.SaveFreeformResult
	require.NoError(t, svc.SaveFreeformResult(ctx, FreeformCard{
		NotebookName: notebookID, StoryTitle: "Session 8", SceneTitle: "psyche + intro",
		Expression: vocabExpr, Meaning: "a quiet person",
	}, FreeformGradeResult{Correct: true, Quality: 4}, 1000))

	// 4. etymology answer — Service.SaveEtymologyOriginResult writes
	// canonical Shape B: top-level title=session, scene from the
	// EtymologyOriginCard's SceneTitle.
	require.NoError(t, svc.SaveEtymologyOriginResult(EtymologyOriginCard{
		NotebookName: notebookID, NotebookTitle: "Dual Notebook",
		SessionTitle: "Session 8", SceneTitle: "psyche + intro",
		Origin: etymExpr, Meaning: "into",
	}, 4, true, 1000, notebook.QuizTypeEtymologyStandard, true))

	// 5. per-type skip — Service.SkipWord (vocab side)
	require.NoError(t, svc.SkipWord(CardInfo{
		NotebookName: notebookID, StoryTitle: "Session 8", SceneTitle: "psyche + intro",
		Expression: vocabExpr,
	}, "", []notebook.QuizType{notebook.QuizTypeReverse}))

	// 6. per-type resume — Service.ResumeWord (vocab side)
	require.NoError(t, svc.ResumeWord(CardInfo{
		NotebookName: notebookID, StoryTitle: "Session 8", SceneTitle: "psyche + intro",
		Expression: vocabExpr,
	}, []notebook.QuizType{notebook.QuizTypeReverse}))

	// 7. override answer — Service.OverrideAnswer (vocab side)
	_, err := svc.OverrideAnswer(CardInfo{
		NotebookName: notebookID, StoryTitle: "Session 8", SceneTitle: "psyche + intro",
		Expression: vocabExpr,
	}, notebook.QuizTypeNotebook)
	require.NoError(t, err)

	// 8. normalisation pass — Validator.Fix
	v := notebook.NewValidator(learningDir, []string{storiesDir}, nil, nil, []string{etymDir}, "", nil)
	_, err = v.Fix()
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(learningDir, notebookID+".yml"))
	require.NoError(t, err)
	var got []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &got))

	type loc struct{ histTitle, sceneTitle string }
	locationsOf := func(needle string) []loc {
		var out []loc
		for _, h := range got {
			for _, expr := range h.Expressions {
				if expr.Expression == needle {
					out = append(out, loc{histTitle: h.Metadata.Title, sceneTitle: "(top-level)"})
				}
			}
			for _, scene := range h.Scenes {
				for _, expr := range scene.Expressions {
					if expr.Expression == needle {
						out = append(out, loc{histTitle: h.Metadata.Title, sceneTitle: scene.Metadata.Title})
					}
				}
			}
		}
		return out
	}

	// Locator: each expression — vocab and etymology — exists at exactly
	// one on-disk location after every writer runs.
	require.Lenf(t, locationsOf(vocabExpr), 1,
		"vocab writers must converge on one location for %q — found: %v",
		vocabExpr, locationsOf(vocabExpr),
	)
	require.Lenf(t, locationsOf(etymExpr), 1,
		"etymology writer must produce exactly one location for %q — found: %v",
		etymExpr, locationsOf(etymExpr),
	)

	// Shape fingerprint: no top-level block carries the legacy
	// metadata.type=etymology shape.
	for _, h := range got {
		assert.NotEqualf(t, "etymology", h.Metadata.Type,
			"legacy etymology-shape block (title=%q) survived Validator.Fix", h.Metadata.Title)
	}
}

// TestLearningHistory_ReadWriteRoundtrip_AcrossAllWriters is the third
// invariant: for every (writer, reader) pair, the writer's effect must be
// observable by the reader.
//
// This is a "future PR" placeholder. Unlike the locator and shape tests
// above (which would pass once the canonical shape is unified), the
// read-write matrix needs a deliberate fixture that lets every reader run
// against every writer's output. It's worth landing once the shape is
// unified, because at that point any drift between read and write paths
// becomes a visible regression rather than expected divergence.
//
// Reader candidates:
//   - Service.LoadCards excludes recently-answered words
//   - Service.LoadReverseCards excludes recently-answered words
//   - Service.LoadEtymologyOriginCards excludes recently-answered origins
//   - Service.LoadNotebookSummaries decrements review counts
//   - server.NotebookHandler.GetNotebookDetail surfaces the new log
//   - Validator.Fix is a no-op on a freshly-written file
func TestLearningHistory_ReadWriteRoundtrip_AcrossAllWriters(t *testing.T) {
	t.Skip("future work: enable after the locator + fingerprint invariants " +
		"are restored. See the writer/reader matrix in the comment above.")

	_ = fmt.Sprintf // silence imports until the test body lands
	_ = assert.True
}
