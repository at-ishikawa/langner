package quiz

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

// TestService_LoadEtymologyOriginCards_EligibilityGate verifies the gate
// for standard/reverse: any origin with a learning-history entry is driven
// by the SR interval (a `misunderstood` last log triggers retry); origins
// with no history fall back to the `includeUnstudied` toggle. The earlier
// "must have a prior correct etymology answer" requirement is gone —
// standard/reverse already show the correct answer on the feedback screen,
// so blocking SR-due retries on tried-failed origins (e.g. algos with one
// misunderstood assembly log) just hid them.
func TestService_LoadEtymologyOriginCards_EligibilityGate(t *testing.T) {
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
	// root-a: only misunderstood logs → SR says retry → included.
	// root-b: freeformed and answered correctly → included when due.
	// root-c: answered correctly in etymology standard → included when due.
	// root-d: no history at all → only included when includeUnstudied=true.
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

	// With includeUnstudied=true on standard quiz:
	//   - root-b, root-c: have logs, latest 2025-01-01 → past any interval →
	//     SR-due → included.
	//   - root-a: has logs but only misunderstood → SR retries → included
	//     (the feedback screen will show the correct answer).
	//   - root-d: never seen (no logs at all) → included because the user
	//     opted in via "Include unstudied".
	cards, err := svc.LoadEtymologyOriginCards([]string{"sample-roots"}, true, false, notebook.QuizTypeEtymologyStandard, nil)
	require.NoError(t, err)
	gotOrigins := make([]string, 0, len(cards))
	for _, c := range cards {
		gotOrigins = append(gotOrigins, c.Origin)
	}
	sort.Strings(gotOrigins)
	assert.Equal(t, []string{"root-a", "root-b", "root-c", "root-d"}, gotOrigins,
		"includeUnstudied=true: all studied origins are SR-driven (incl. tried-failed), plus never-seen origins")

	// With includeUnstudied=false: never-seen origins drop out, but
	// tried-failed (root-a) stays because SR says retry.
	noUnstudied, err := svc.LoadEtymologyOriginCards([]string{"sample-roots"}, false, false, notebook.QuizTypeEtymologyStandard, nil)
	require.NoError(t, err)
	gotOrigins = gotOrigins[:0]
	for _, c := range noUnstudied {
		gotOrigins = append(gotOrigins, c.Origin)
	}
	sort.Strings(gotOrigins)
	assert.Equal(t, []string{"root-a", "root-b", "root-c"}, gotOrigins,
		"includeUnstudied=false leaves all studied origins (SR-driven), drops only the never-seen one")

	// When skipEligibility is true (freeform mode), ALL origins are returned.
	freeformCards, err := svc.LoadEtymologyOriginCards([]string{"sample-roots"}, true, true, notebook.QuizTypeEtymologyFreeform, nil)
	require.NoError(t, err)
	require.Len(t, freeformCards, 4, "freeform quiz should see all origins regardless of the SR gate")
}

// TestService_LoadEtymologyOriginCards_IncludeUnstudiedDoesntBypassSR pins
// the bug fix for the user-reported case: a studied origin (latest etymology
// log understood, interval 90) was re-asked the same day the schedule
// pushed it 90 days out, just because the user toggled "Include unstudied".
// includeUnstudied must NOT bypass the SR gate for origins that already
// have a correct answer.
func TestService_LoadEtymologyOriginCards_IncludeUnstudiedDoesntBypassSR(t *testing.T) {
	tmpDir := t.TempDir()
	etymDir := filepath.Join(tmpDir, "etymology", "sr-roots")
	require.NoError(t, os.MkdirAll(etymDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "index.yml"), []byte(`id: sr-roots
kind: Etymology
name: SR Roots
notebooks:
  - ./origins.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "origins.yml"), []byte(`metadata:
  title: "SR Lesson"
origins:
  - origin: just-scheduled
    type: root
    language: Latin
    meaning: studied today on a 90-day interval
  - origin: never-seen
    type: root
    language: Latin
    meaning: no learning history at all
`), 0o644))

	learningDir := filepath.Join(tmpDir, "learning")
	require.NoError(t, os.MkdirAll(learningDir, 0o755))
	// just-scheduled: latest etymology_breakdown_log is "understood" today
	// with interval_days 90. needsOriginReview should return false. Even
	// with includeUnstudied=true, the SR gate must keep it out.
	today := time.Now().Format("2006-01-02")
	// just-scheduled has fresh correct logs in BOTH the breakdown and
	// assembly tracks with interval_days=90. needsOriginReview consults
	// the track that matches the active quiz type — standard reads
	// breakdown, reverse reads assembly — so the test covers both
	// directions.
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "sr-roots.yml"), []byte(`- metadata:
    notebook_id: sr-roots
    title: SR Roots
  scenes:
    - metadata:
        title: "SR Lesson"
      expressions:
        - expression: just-scheduled
          etymology_breakdown_logs:
            - status: understood
              learned_at: "`+today+`"
              quality: 4
              interval_days: 90
              quiz_type: etymology_breakdown
          etymology_assembly_logs:
            - status: understood
              learned_at: "`+today+`"
              quality: 4
              interval_days: 90
              quiz_type: etymology_assembly
`), 0o644))

	svc := NewService(
		config.NotebooksConfig{
			EtymologyDirectories:   []string{filepath.Join(tmpDir, "etymology")},
			LearningNotesDirectory: learningDir,
		},
		nil, nil, nil,
		config.QuizConfig{},
	)

	cards, err := svc.LoadEtymologyOriginCards([]string{"sr-roots"}, true, false, notebook.QuizTypeEtymologyStandard, nil)
	require.NoError(t, err)
	gotOrigins := make([]string, 0, len(cards))
	for _, c := range cards {
		gotOrigins = append(gotOrigins, c.Origin)
	}
	sort.Strings(gotOrigins)
	assert.Equal(t, []string{"never-seen"}, gotOrigins,
		"includeUnstudied=true must surface never-seen origins but never re-serve a studied origin still within its SR interval")

	// Same assertion for reverse — the symmetric branch in LoadEtymologyOriginCards.
	reverseCards, err := svc.LoadEtymologyOriginCards([]string{"sr-roots"}, true, false, notebook.QuizTypeEtymologyReverse, nil)
	require.NoError(t, err)
	gotOrigins = gotOrigins[:0]
	for _, c := range reverseCards {
		gotOrigins = append(gotOrigins, c.Origin)
	}
	sort.Strings(gotOrigins)
	assert.Equal(t, []string{"never-seen"}, gotOrigins,
		"the SR gate applies equally to etymology reverse — includeUnstudied=true must not re-ask a studied origin still within interval")
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

	summaries, err := svc.LoadEtymologyNotebookSummaries(false)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, 1, summaries[0].EtymologyReviewCount,
		"the due count shown on the start page must equal the number of cards in the quiz")
}

// TestService_GetEtymologyOriginNextReviewDates_CoversAllModes pins the
// fix for the freeform "Origin not found in notebooks" symptom on words
// recently answered in standard or reverse. originNextReviewDate must
// consult both EtymologyBreakdownLogs and EtymologyAssemblyLogs (freeform
// writes to both, so two fields cover all three modes); reading only
// breakdown let assembly-only answers slip through and the freeform
// frontend rendered them as "Found in notebooks" / drillable.
func TestService_GetEtymologyOriginNextReviewDates_CoversAllModes(t *testing.T) {
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

	learningDir := filepath.Join(tmpDir, "learning")
	require.NoError(t, os.MkdirAll(learningDir, 0755))
	today := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	// Post-migration shape: history.metadata.title is the SESSION
	// title (matches the etymology source's metadata.title above),
	// not the book name. SceneTitle is the session title too because
	// the etymology source uses the flat (legacy) origin shape without
	// scene structure and there's no definitions notebook here, so the
	// reader's pickBestSceneForOrigin falls back to the session title.
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "cross-mode.yml"), []byte(fmt.Sprintf(`- metadata:
    notebook_id: cross-mode
    title: "Cross Mode Lesson"
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

	cards, err := svc.LoadEtymologyOriginCards(
		[]string{"cross-mode"}, true, true,
		notebook.QuizTypeEtymologyFreeform, nil,
	)
	require.NoError(t, err)
	require.Len(t, cards, 3, "freeform card list should include every origin")

	dates, err := svc.GetEtymologyOriginNextReviewDates(cards)
	require.NoError(t, err)

	_, xFuture := dates["root-x"]
	_, yFuture := dates["root-y"]
	_, zFuture := dates["root-z"]
	assert.True(t, xFuture, "root-x answered today in breakdown should be reported as not-due")
	assert.True(t, yFuture, "root-y answered today in assembly should be reported as not-due")
	assert.False(t, zFuture, "root-z has no logs and should remain freely drillable")
}

// TestService_EtymologyReverseQueueCountsOnlyAssemblyHistory reproduces
// the start-page count blow-up the user reported after sync-db: every
// origin they had ever touched in *any* etymology mode (freeform or
// breakdown) showed up in the reverse queue, not just the ones they
// had actually drilled in reverse.
//
// The underlying cause is in shouldIncludeOrigin → needsOriginReview:
// for an origin with a history entry but no logs in the requested
// quiz_type slot, needsOriginReview returns true ("never studied this
// mode, so due now"). That semantic means an origin studied only via
// etymology_breakdown / freeform gets pulled into the reverse queue
// unconditionally — and the more import-side fixes routed origin logs
// onto etymology_origins.id, the more origins qualified as
// "studied in some mode," so the reverse queue grew with every
// well-routed import.
//
// What the user expects (and what this test asserts):
//   - Origins with assembly logs (reverse history) → counted in reverse.
//   - Origins with only breakdown / freeform logs → counted ONLY when
//     includeUnstudied is on. With includeUnstudied=false the reverse
//     queue should NOT pull them in.
//
// The test is intentionally written to fail under the current logic
// so the next commit's fix to shouldIncludeOrigin is visible in the
// diff. The companion test TestService_EtymologySummaryMatchesQuizLoad
// below still pins quiz-load and start-page parity for whatever the
// gate ends up being.
func TestService_EtymologyReverseQueueCountsOnlyAssemblyHistory(t *testing.T) {
	tmpDir := t.TempDir()
	etymDir := filepath.Join(tmpDir, "etymology", "demo-roots")
	require.NoError(t, os.MkdirAll(etymDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "index.yml"), []byte(`id: demo-roots
kind: Etymology
name: Demo Roots
notebooks:
  - ./origins.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "origins.yml"), []byte(`metadata:
  title: "Demo Lesson"
origins:
  - origin: reverse-studied
    type: root
    language: Latin
    meaning: drilled in reverse before (assembly logs present)
  - origin: only-breakdown
    type: root
    language: Latin
    meaning: drilled in standard only, never in reverse
  - origin: only-freeform
    type: root
    language: Latin
    meaning: typed in freeform only, never in reverse
`), 0o644))

	today := time.Now().Format("2006-01-02")
	learningDir := filepath.Join(tmpDir, "learning")
	require.NoError(t, os.MkdirAll(learningDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "demo-roots.yml"), []byte(`- metadata:
    notebook_id: demo-roots
    title: Demo Roots
  scenes:
    - metadata:
        title: "Demo Lesson"
      expressions:
        - expression: reverse-studied
          etymology_assembly_logs:
            - status: understood
              learned_at: "2024-01-01"
              quality: 4
              interval_days: 7
              quiz_type: etymology_assembly
        - expression: only-breakdown
          etymology_breakdown_logs:
            - status: understood
              learned_at: "`+today+`"
              quality: 4
              interval_days: 90
              quiz_type: etymology_breakdown
        - expression: only-freeform
          etymology_breakdown_logs:
            - status: understood
              learned_at: "`+today+`"
              quality: 4
              interval_days: 90
              quiz_type: etymology_freeform
`), 0o644))

	svc := NewService(
		config.NotebooksConfig{
			EtymologyDirectories:   []string{filepath.Join(tmpDir, "etymology")},
			LearningNotesDirectory: learningDir,
		},
		nil, nil, nil,
		config.QuizConfig{DisableShuffle: true},
	)

	t.Run("reverse with includeUnstudied=false counts only assembly-history origins", func(t *testing.T) {
		cards, err := svc.LoadEtymologyOriginCards(
			[]string{"demo-roots"}, false, false, notebook.QuizTypeEtymologyReverse, nil,
		)
		require.NoError(t, err)
		gotOrigins := make([]string, 0, len(cards))
		for _, c := range cards {
			gotOrigins = append(gotOrigins, c.Origin)
		}
		sort.Strings(gotOrigins)
		// reverse-studied has an old (overdue) assembly log → should be due.
		// only-breakdown and only-freeform have no assembly logs at all;
		// with includeUnstudied=false they should be filtered out.
		assert.Equal(t, []string{"reverse-studied"}, gotOrigins,
			"the reverse queue with includeUnstudied=false should NOT include "+
				"origins whose history exists in other etymology modes but not "+
				"in assembly — that's what blows up the start-page count after sync-db")
	})

	t.Run("reverse with includeUnstudied=true counts everything", func(t *testing.T) {
		cards, err := svc.LoadEtymologyOriginCards(
			[]string{"demo-roots"}, true, false, notebook.QuizTypeEtymologyReverse, nil,
		)
		require.NoError(t, err)
		gotOrigins := make([]string, 0, len(cards))
		for _, c := range cards {
			gotOrigins = append(gotOrigins, c.Origin)
		}
		sort.Strings(gotOrigins)
		assert.Equal(t, []string{"only-breakdown", "only-freeform", "reverse-studied"}, gotOrigins,
			"includeUnstudied=true should bring in cross-mode-only origins as 'unstudied for reverse'")
	})

	t.Run("start-page reverse count matches the quiz queue", func(t *testing.T) {
		for _, includeUnstudied := range []bool{false, true} {
			cards, err := svc.LoadEtymologyOriginCards(
				[]string{"demo-roots"}, includeUnstudied, false, notebook.QuizTypeEtymologyReverse, nil,
			)
			require.NoError(t, err)

			summaries, err := svc.LoadEtymologyNotebookSummaries(includeUnstudied)
			require.NoError(t, err)
			require.Len(t, summaries, 1)
			assert.Equal(t, len(cards), summaries[0].EtymologyReverseReviewCount,
				"includeUnstudied=%v: summary reverse count must match quiz queue size", includeUnstudied)
		}
	})
}

// TestService_EtymologySummaryMatchesQuizLoad pins the fix for the
// start-page-vs-quiz count mismatch: the per-notebook and per-section
// EtymologyReviewCount on the start page must equal the number of cards
// the quiz actually loads, for both includeUnstudied toggle states and
// for both standard and reverse modes.
//
// Fixture mix per session:
//
//	studied-due       — correct etymology log with a lapsed SR interval.
//	                    In BOTH summary and quiz, regardless of toggle.
//	studied-not-due   — fresh 90-day interval. SR says not yet → excluded
//	                    everywhere; includeUnstudied does NOT bypass SR.
//	never-seen        — no learning history. Included iff includeUnstudied.
//	tried-failed      — has logs but only misunderstood. SR retries the
//	                    same-track misunderstood (standard reads breakdown);
//	                    cross-track (reverse reading assembly) has no logs
//	                    so the "never-seen in this mode" SR branch returns
//	                    true too. Included in both modes.
func TestService_EtymologySummaryMatchesQuizLoad(t *testing.T) {
	tmpDir := t.TempDir()
	etymDir := filepath.Join(tmpDir, "etymology", "demo-roots")
	require.NoError(t, os.MkdirAll(etymDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "index.yml"), []byte(`id: demo-roots
kind: Etymology
name: Demo Roots
notebooks:
  - ./origins.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(etymDir, "origins.yml"), []byte(`metadata:
  title: "Demo Lesson"
origins:
  - origin: studied-due
    type: root
    language: Latin
    meaning: learned long ago, interval lapsed
  - origin: studied-not-due
    type: root
    language: Latin
    meaning: learned today, still inside 90-day interval
  - origin: never-seen-1
    type: root
    language: Latin
    meaning: no logs at all
  - origin: never-seen-2
    type: root
    language: Latin
    meaning: no logs at all either
  - origin: tried-failed
    type: root
    language: Latin
    meaning: tried but only misunderstood
`), 0o644))

	today := time.Now().Format("2006-01-02")
	learningDir := filepath.Join(tmpDir, "learning")
	require.NoError(t, os.MkdirAll(learningDir, 0o755))
	// Both breakdown and assembly logs are present for studied-due so the
	// fixture exercises BOTH quiz modes — needsOriginReview reads the
	// breakdown track for standard and the assembly track for reverse.
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "demo-roots.yml"), []byte(`- metadata:
    notebook_id: demo-roots
    title: Demo Roots
  scenes:
    - metadata:
        title: "Demo Lesson"
      expressions:
        - expression: studied-due
          etymology_breakdown_logs:
            - status: understood
              learned_at: "2024-01-01"
              quality: 4
              interval_days: 7
              quiz_type: etymology_breakdown
          etymology_assembly_logs:
            - status: understood
              learned_at: "2024-01-01"
              quality: 4
              interval_days: 7
              quiz_type: etymology_assembly
        - expression: studied-not-due
          etymology_breakdown_logs:
            - status: understood
              learned_at: "`+today+`"
              quality: 4
              interval_days: 90
              quiz_type: etymology_breakdown
          etymology_assembly_logs:
            - status: understood
              learned_at: "`+today+`"
              quality: 4
              interval_days: 90
              quiz_type: etymology_assembly
        - expression: tried-failed
          etymology_breakdown_logs:
            - status: misunderstood
              learned_at: "2024-01-01"
              quiz_type: etymology_breakdown
`), 0o644))

	svc := NewService(
		config.NotebooksConfig{
			EtymologyDirectories:   []string{filepath.Join(tmpDir, "etymology")},
			LearningNotesDirectory: learningDir,
		},
		nil, nil, nil,
		config.QuizConfig{DisableShuffle: true},
	)

	cases := []struct {
		name             string
		includeUnstudied bool
		quizType         notebook.QuizType
		wantOrigins      []string
	}{
		{
			name:             "standard, includeUnstudied=false",
			includeUnstudied: false,
			quizType:         notebook.QuizTypeEtymologyStandard,
			wantOrigins:      []string{"studied-due", "tried-failed"},
		},
		{
			name:             "standard, includeUnstudied=true",
			includeUnstudied: true,
			quizType:         notebook.QuizTypeEtymologyStandard,
			wantOrigins:      []string{"studied-due", "tried-failed", "never-seen-1", "never-seen-2"},
		},
		{
			// tried-failed has logs only in etymology_breakdown_logs, not
			// in etymology_assembly_logs. Per the updated shouldIncludeOrigin
			// semantic, "no logs in the requested mode's slot" falls under
			// the includeUnstudied gate; with includeUnstudied=false the
			// reverse queue should drop it. This is the fix that stops the
			// start-page reverse count from exploding after sync-db.
			name:             "reverse, includeUnstudied=false",
			includeUnstudied: false,
			quizType:         notebook.QuizTypeEtymologyReverse,
			wantOrigins:      []string{"studied-due"},
		},
		{
			name:             "reverse, includeUnstudied=true",
			includeUnstudied: true,
			quizType:         notebook.QuizTypeEtymologyReverse,
			wantOrigins:      []string{"studied-due", "tried-failed", "never-seen-1", "never-seen-2"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cards, err := svc.LoadEtymologyOriginCards(
				[]string{"demo-roots"}, tc.includeUnstudied, false, tc.quizType, nil,
			)
			require.NoError(t, err)
			gotOrigins := make([]string, 0, len(cards))
			for _, c := range cards {
				gotOrigins = append(gotOrigins, c.Origin)
			}
			sort.Strings(gotOrigins)
			wantOrigins := append([]string{}, tc.wantOrigins...)
			sort.Strings(wantOrigins)
			assert.Equal(t, wantOrigins, gotOrigins,
				"quiz loader should include exactly the expected set of origins")

			summaries, err := svc.LoadEtymologyNotebookSummaries(tc.includeUnstudied)
			require.NoError(t, err)
			require.Len(t, summaries, 1)
			summary := summaries[0]
			gotCount := summary.EtymologyReviewCount
			if tc.quizType == notebook.QuizTypeEtymologyReverse {
				gotCount = summary.EtymologyReverseReviewCount
			}
			assert.Equal(t, len(cards), gotCount,
				"start page count for the active mode must equal what the quiz loads")

			require.Len(t, summary.Sections, 1)
			gotSectionCount := summary.Sections[0].EtymologyReviewCount
			if tc.quizType == notebook.QuizTypeEtymologyReverse {
				gotSectionCount = summary.Sections[0].EtymologyReverseReviewCount
			}
			assert.Equal(t, len(cards), gotSectionCount,
				"start page section count for the active mode must equal what the quiz loads")
		})
	}
}
