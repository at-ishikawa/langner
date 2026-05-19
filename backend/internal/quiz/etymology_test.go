package quiz

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/notebook"
)

func createEtymologyFixtures(t *testing.T) (string, string) {
	t.Helper()

	tmpDir := t.TempDir()

	// Create etymology notebook
	etymDir := filepath.Join(tmpDir, "etymology", "latin-roots")
	require.NoError(t, os.MkdirAll(etymDir, 0755))

	etymIndex := `id: latin-roots
kind: Etymology
name: Latin Roots
notebooks:
  - ./origins.yml
`
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "index.yml"), []byte(etymIndex), 0644))

	originsYAML := `metadata:
  title: "Latin Roots Lesson"
origins:
  - origin: "spect"
    type: root
    language: Latin
    meaning: to look or see
  - origin: "pre"
    type: prefix
    language: Latin
    meaning: before
  - origin: "tion"
    type: suffix
    language: Latin
    meaning: act or process of
`
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "origins.yml"), []byte(originsYAML), 0644))

	// Create learning notes directory with etymology_freeform history for
	// each origin so they pass the hard eligibility gate (must be freeformed
	// first AND have at least one correct etymology answer).
	learningDir := filepath.Join(tmpDir, "learning")
	require.NoError(t, os.MkdirAll(learningDir, 0755))
	learningHistory := `- metadata:
    notebook_id: latin-roots
    title: Latin Roots
  scenes:
    - metadata:
        title: "Latin Roots Lesson"
      expressions:
        - expression: spect
          etymology_breakdown_logs:
            - status: understood
              learned_at: "2025-01-01"
              quiz_type: etymology_freeform
        - expression: pre
          etymology_breakdown_logs:
            - status: understood
              learned_at: "2025-01-01"
              quiz_type: etymology_freeform
        - expression: tion
          etymology_breakdown_logs:
            - status: understood
              learned_at: "2025-01-01"
              quiz_type: etymology_freeform
`
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "latin-roots.yml"), []byte(learningHistory), 0644))

	return tmpDir, learningDir
}

func TestService_LoadEtymologyOriginCards(t *testing.T) {
	tmpDir, learningDir := createEtymologyFixtures(t)

	svc := NewService(
		config.NotebooksConfig{
			EtymologyDirectories:   []string{filepath.Join(tmpDir, "etymology")},
			LearningNotesDirectory: learningDir,
		},
		nil, nil, nil,
		config.QuizConfig{},
	)

	cards, err := svc.LoadEtymologyOriginCards(
		[]string{"latin-roots"},
		true,
		true,
		notebook.QuizTypeEtymologyStandard,
		nil,
	)
	require.NoError(t, err)

	assert.Len(t, cards, 3)

	originMap := make(map[string]EtymologyOriginCard)
	for _, card := range cards {
		originMap[card.Origin] = card
	}

	spectCard, ok := originMap["spect"]
	require.True(t, ok, "should find 'spect' card")
	assert.Equal(t, "to look or see", spectCard.Meaning)
	assert.Equal(t, "root", spectCard.Type)
	assert.Equal(t, "Latin", spectCard.Language)
	assert.Equal(t, "Latin Roots", spectCard.NotebookTitle)

	preCard, ok := originMap["pre"]
	require.True(t, ok, "should find 'pre' card")
	assert.Equal(t, "before", preCard.Meaning)
	assert.Equal(t, "prefix", preCard.Type)
}

// TestService_LoadEtymologyOriginCards_FreeformFirstGate verifies that the
// hard eligibility gate is ALWAYS enforced, even when includeUnstudied=true.
// Origins must have been attempted in etymology freeform mode AND have at
// least one correct etymology answer before they show up in standard or
// reverse quizzes.
func TestService_LoadEtymologyOriginCards_FreeformFirstGate(t *testing.T) {
	tmpDir := t.TempDir()
	etymDir := filepath.Join(tmpDir, "etymology", "sample-roots")
	require.NoError(t, os.MkdirAll(etymDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "index.yml"), []byte(`id: sample-roots
kind: Etymology
name: Sample Roots
notebooks:
  - ./origins.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "origins.yml"), []byte(`metadata:
  title: "Sample Lesson"
origins:
  - origin: "root-a"
    type: root
    language: Latin
    meaning: first sample meaning
  - origin: "root-b"
    type: root
    language: Latin
    meaning: second sample meaning
  - origin: "root-c"
    type: root
    language: Latin
    meaning: third sample meaning
  - origin: "root-d"
    type: root
    language: Latin
    meaning: fourth sample meaning
`), 0644))

	learningDir := filepath.Join(tmpDir, "learning")
	require.NoError(t, os.MkdirAll(learningDir, 0755))
	// root-a: freeformed but never answered correctly → NOT eligible.
	// root-b: freeformed and answered correctly → eligible.
	// root-c: answered correctly in etymology standard (not freeform) → NOT eligible (freeform-first).
	// root-d: no history at all → NOT eligible.
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "sample-roots.yml"), []byte(`- metadata:
    notebook_id: sample-roots
    title: Sample Roots
  scenes:
    - metadata:
        title: "Sample Lesson"
      expressions:
        - expression: root-a
          etymology_breakdown_logs:
            - status: misunderstood
              learned_at: "2025-01-01"
              quiz_type: etymology_freeform
        - expression: root-b
          etymology_breakdown_logs:
            - status: understood
              learned_at: "2025-01-01"
              quiz_type: etymology_freeform
        - expression: root-c
          etymology_breakdown_logs:
            - status: understood
              learned_at: "2025-01-01"
              quiz_type: etymology_breakdown
`), 0644))

	svc := NewService(
		config.NotebooksConfig{
			EtymologyDirectories:   []string{filepath.Join(tmpDir, "etymology")},
			LearningNotesDirectory: learningDir,
		},
		nil, nil, nil,
		config.QuizConfig{},
	)

	// When skipEligibility is false, only root-b should be eligible (standard/reverse gate).
	cards, err := svc.LoadEtymologyOriginCards([]string{"sample-roots"}, true, false, notebook.QuizTypeEtymologyStandard, nil)
	require.NoError(t, err)
	require.Len(t, cards, 1, "only origins that were freeformed AND answered correctly should be eligible")
	assert.Equal(t, "root-b", cards[0].Origin)

	// When skipEligibility is true (freeform mode), ALL origins are returned.
	freeformCards, err := svc.LoadEtymologyOriginCards([]string{"sample-roots"}, true, true, notebook.QuizTypeEtymologyFreeform, nil)
	require.NoError(t, err)
	require.Len(t, freeformCards, 4, "freeform quiz should see all origins regardless of eligibility")
}

func TestService_LoadEtymologyOriginCards_Deduplicates(t *testing.T) {
	tmpDir := t.TempDir()

	etymDir1 := filepath.Join(tmpDir, "etymology", "roots-1")
	require.NoError(t, os.MkdirAll(etymDir1, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir1, "index.yml"), []byte(`id: roots-1
kind: Etymology
name: Roots 1
notebooks:
  - ./origins.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir1, "origins.yml"), []byte(`metadata:
  title: "Roots 1 Lesson"
origins:
  - origin: "spect"
    type: root
    language: Latin
    meaning: to look or see
`), 0644))

	etymDir2 := filepath.Join(tmpDir, "etymology", "roots-2")
	require.NoError(t, os.MkdirAll(etymDir2, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir2, "index.yml"), []byte(`id: roots-2
kind: Etymology
name: Roots 2
notebooks:
  - ./origins.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir2, "origins.yml"), []byte(`metadata:
  title: "Roots 2 Lesson"
origins:
  - origin: "spect"
    type: root
    language: Latin
    meaning: to see
`), 0644))

	learningDir := filepath.Join(tmpDir, "learning")
	require.NoError(t, os.MkdirAll(learningDir, 0755))
	// Each notebook records the origin under its own session-titled scene,
	// matching the per-(notebook, session, origin) keying used by the loader.
	freeformHistory := `- metadata:
    notebook_id: %s
    title: %s
  scenes:
    - metadata:
        title: "%s"
      expressions:
        - expression: spect
          etymology_breakdown_logs:
            - status: understood
              learned_at: "2025-01-01"
              quiz_type: etymology_freeform
`
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "roots-1.yml"),
		[]byte(fmt.Sprintf(freeformHistory, "roots-1", "Roots 1", "Roots 1 Lesson")), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "roots-2.yml"),
		[]byte(fmt.Sprintf(freeformHistory, "roots-2", "Roots 2", "Roots 2 Lesson")), 0644))

	svc := NewService(
		config.NotebooksConfig{
			EtymologyDirectories:   []string{filepath.Join(tmpDir, "etymology")},
			LearningNotesDirectory: learningDir,
		},
		nil, nil, nil,
		config.QuizConfig{},
	)

	// Two notebooks record "spect" as separate origins (different notebooks
	// = different sources of truth, distinct cards under the new keying).
	// Within a single notebook + session, duplicates would still collapse;
	// here we have two notebooks, so two cards survive.
	cards, err := svc.LoadEtymologyOriginCards([]string{"roots-1", "roots-2"}, true, true, notebook.QuizTypeEtymologyFreeform, nil)
	require.NoError(t, err)
	assert.Len(t, cards, 2, "the same origin in two notebooks remains as separate cards because each notebook is independently tracked")
}

func TestService_LoadEtymologyOriginCards_SectionFilter(t *testing.T) {
	tmpDir := t.TempDir()
	etymDir := filepath.Join(tmpDir, "etymology", "two-sessions")
	require.NoError(t, os.MkdirAll(etymDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "index.yml"), []byte(`id: two-sessions
kind: Etymology
name: Two Sessions
notebooks:
  - ./session-1.yml
  - ./session-2.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "session-1.yml"), []byte(`metadata:
  title: "Session One"
origins:
  - origin: "alpha"
    type: root
    language: Latin
    meaning: alpha meaning
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "session-2.yml"), []byte(`metadata:
  title: "Session Two"
origins:
  - origin: "beta"
    type: root
    language: Latin
    meaning: beta meaning
`), 0644))

	learningDir := filepath.Join(tmpDir, "learning")
	require.NoError(t, os.MkdirAll(learningDir, 0755))

	svc := NewService(
		config.NotebooksConfig{
			EtymologyDirectories:   []string{filepath.Join(tmpDir, "etymology")},
			LearningNotesDirectory: learningDir,
		},
		nil, nil, nil,
		config.QuizConfig{},
	)

	all, err := svc.LoadEtymologyOriginCards(
		[]string{"two-sessions"}, true, true, notebook.QuizTypeEtymologyFreeform, nil,
	)
	require.NoError(t, err)
	require.Len(t, all, 2, "no filter returns both sessions")

	filtered, err := svc.LoadEtymologyOriginCards(
		[]string{"two-sessions"}, true, true, notebook.QuizTypeEtymologyFreeform,
		map[string][]string{"two-sessions": {"Session Two"}},
	)
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, "Session Two", filtered[0].SessionTitle)
	assert.Equal(t, "beta", filtered[0].Origin)
}

// TestService_LoadEtymologyOriginCards_DedupesAcrossLanguageMetadata pins the
// fix for a bug where the same origin recorded with inconsistent language
// metadata (e.g. case differences, whitespace, or empty language) bypassed the
// dedup, causing the same word to appear multiple times in the standard quiz
// and learning history records to be appended multiple times per session.
func TestService_LoadEtymologyOriginCards_DedupesAcrossLanguageMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	etymDir := filepath.Join(tmpDir, "etymology", "messy-roots")
	require.NoError(t, os.MkdirAll(etymDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "index.yml"), []byte(`id: messy-roots
kind: Etymology
name: Messy Roots
notebooks:
  - ./origins.yml
`), 0644))
	// Three "spect" entries with inconsistent language fields and one with a
	// trailing space in the origin. All four refer to the same origin.
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "origins.yml"), []byte(`metadata:
  title: "Messy Lesson"
origins:
  - origin: "spect"
    type: root
    language: Latin
    meaning: to look or see
  - origin: "spect"
    type: root
    language: latin
    meaning: to look or see
  - origin: "spect"
    type: root
    language: ""
    meaning: to look or see
  - origin: "spect "
    type: root
    language: Latin
    meaning: to look or see
`), 0644))

	learningDir := filepath.Join(tmpDir, "learning")
	require.NoError(t, os.MkdirAll(learningDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "messy-roots.yml"), []byte(`- metadata:
    notebook_id: messy-roots
    title: Messy Roots
  scenes:
    - metadata:
        title: "Messy Lesson"
      expressions:
        - expression: spect
          etymology_breakdown_logs:
            - status: understood
              learned_at: "2025-01-01"
              quiz_type: etymology_freeform
`), 0644))

	svc := NewService(
		config.NotebooksConfig{
			EtymologyDirectories:   []string{filepath.Join(tmpDir, "etymology")},
			LearningNotesDirectory: learningDir,
		},
		nil, nil, nil,
		config.QuizConfig{},
	)

	cards, err := svc.LoadEtymologyOriginCards([]string{"messy-roots"}, true, false, notebook.QuizTypeEtymologyStandard, nil)
	require.NoError(t, err)
	require.Len(t, cards, 1, "the same origin must collapse to one card regardless of language/whitespace differences")
	assert.Equal(t, "spect", cards[0].Origin)

	summaries, err := svc.LoadEtymologyNotebookSummaries()
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, 1, summaries[0].EtymologyReviewCount,
		"the due count shown on the start page must equal the number of cards in the quiz")
}

// TestService_LoadEtymologyOriginCards_FreeformRespectsCrossModeReview pins
// the behaviour the StartEtymologyFreeformQuiz handler now relies on: when
// includeUnstudied=false (which the handler sends), an origin recently
// answered in standard or reverse mode (interval 30) must not appear in
// freeform the same day, even though it has no freeform-mode log. Per-mode
// needsOriginReview alone would let it through.
func TestService_LoadEtymologyOriginCards_FreeformRespectsCrossModeReview(t *testing.T) {
	tmpDir := t.TempDir()
	etymDir := filepath.Join(tmpDir, "etymology", "cross-mode")
	require.NoError(t, os.MkdirAll(etymDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "index.yml"), []byte(`id: cross-mode
kind: Etymology
name: Cross Mode
notebooks:
  - ./origins.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "origins.yml"), []byte(`metadata:
  title: "Cross Mode Lesson"
origins:
  - origin: "root-x"
    type: root
    language: Latin
    meaning: drilled standard today
  - origin: "root-y"
    type: root
    language: Latin
    meaning: drilled reverse today
  - origin: "root-z"
    type: root
    language: Latin
    meaning: never touched
`), 0644))

	// root-x: today's etymology_breakdown log, interval=30 → freeform should skip.
	// root-y: today's etymology_assembly log, interval=30 → freeform should skip.
	// root-z: no logs anywhere → freeform should include (first encounter).
	learningDir := filepath.Join(tmpDir, "learning")
	require.NoError(t, os.MkdirAll(learningDir, 0755))
	today := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "cross-mode.yml"), []byte(fmt.Sprintf(`- metadata:
    notebook_id: cross-mode
    title: Cross Mode
  scenes:
    - metadata:
        title: "Cross Mode Lesson"
      expressions:
        - expression: root-x
          etymology_breakdown_logs:
            - status: understood
              learned_at: %q
              quiz_type: etymology_breakdown
              interval_days: 30
        - expression: root-y
          etymology_assembly_logs:
            - status: understood
              learned_at: %q
              quiz_type: etymology_assembly
              interval_days: 30
`, today, today)), 0644))

	svc := NewService(
		config.NotebooksConfig{
			EtymologyDirectories:   []string{filepath.Join(tmpDir, "etymology")},
			LearningNotesDirectory: learningDir,
		},
		nil, nil, nil,
		config.QuizConfig{},
	)
	svc.disableShuffle = true

	// Freeform with includeUnstudied=false (the production path): only
	// root-z should show up. root-x and root-y were just answered in
	// other modes and shouldn't reappear in freeform the same day.
	cards, err := svc.LoadEtymologyOriginCards(
		[]string{"cross-mode"}, false, true,
		notebook.QuizTypeEtymologyFreeform, nil,
	)
	require.NoError(t, err)
	got := make([]string, 0, len(cards))
	for _, c := range cards {
		got = append(got, c.Origin)
	}
	assert.Equal(t, []string{"root-z"}, got,
		"freeform must skip origins recently answered in any etymology mode; "+
			"only the never-touched root-z should appear")
}
