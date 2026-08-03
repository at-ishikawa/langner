package quiz

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/learning"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// etymologyFixture wires an etymology notebook and a matching definitions
// notebook (same book id + session title) so buildOriginFamilies can attach a
// word family to each origin. Returns a service with shuffle disabled.
func etymologyFixture(t *testing.T, etymYAML, defsYAML string) (*Service, string, string) {
	t.Helper()
	dir := t.TempDir()
	etymDir := filepath.Join(dir, "etymology")
	defsDir := filepath.Join(dir, "definitions")
	learningDir := filepath.Join(dir, "learning")
	const bookID = "roots"

	etymBook := filepath.Join(etymDir, bookID)
	require.NoError(t, os.MkdirAll(etymBook, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(etymBook, "index.yml"), []byte(`id: roots
kind: Etymology
name: Roots
notebooks:
  - ./s1.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(etymBook, "s1.yml"), []byte(etymYAML), 0644))

	defsBook := filepath.Join(defsDir, bookID)
	require.NoError(t, os.MkdirAll(defsBook, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(defsBook, "index.yml"), []byte(`id: roots
notebooks:
  - ./s1.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(defsBook, "s1.yml"), []byte(defsYAML), 0644))
	require.NoError(t, os.MkdirAll(learningDir, 0755))

	ctrl := gomock.NewController(t)
	svc := NewService(config.NotebooksConfig{
		EtymologyDirectories:   []string{etymDir},
		DefinitionsDirectories: []string{defsDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		nil, config.QuizConfig{DisableShuffle: true})
	return svc, bookID, learningDir
}

const singleSenseEtymYAML = `metadata:
  title: "Session 1"
origins:
  - origin: "scribo"
    type: root
    language: Latin
    meaning: to write
`

const singleSenseDefsYAML = `- metadata:
    title: "Session 1"
  scenes:
  - metadata:
      index: 0
      title: S1
    expressions:
    - expression: describe
      meaning: to represent in words
      origin_parts:
      - origin: scribo
    - expression: inscribe
      meaning: to write or carve on a surface
      origin_parts:
      - origin: scribo
`

// TestLoadEtymologyOriginCards_GroupsFamily verifies one card per origin
// carries the FULL session-scoped word family with meanings, and that the
// displayed origin text equals the source origin string (invariant L3).
func TestLoadEtymologyOriginCards_GroupsFamily(t *testing.T) {
	svc, bookID, _ := etymologyFixture(t, singleSenseEtymYAML, singleSenseDefsYAML)

	cards, err := svc.LoadEtymologyOriginCards([]string{bookID}, true, false, nil)
	require.NoError(t, err)
	require.Len(t, cards, 1)

	card := cards[0]
	assert.Equal(t, "scribo", card.Origin) // L3: display text == source origin
	assert.Equal(t, "Session 1", card.SessionTitle)
	assert.Equal(t, "to write", card.Meaning)

	gotWords := map[string]string{}
	for _, w := range card.Words {
		gotWords[w.Expression] = w.Meaning
	}
	assert.Equal(t, map[string]string{
		"describe": "to represent in words",
		"inscribe": "to write or carve on a surface",
	}, gotWords)
}

// TestLoadEtymologyOriginCards_CarriesStudyContext verifies the additive study
// context flows onto the card: origin-level english_forms + note, and per-word
// pronunciation, example sentences, and literal gloss (the latter read from the
// derived word's free-text note field). None of these are quizzed — only the
// word meaning is graded — but the card must carry them for the UI to render.
func TestLoadEtymologyOriginCards_CarriesStudyContext(t *testing.T) {
	const etymYAML = `metadata:
  title: "Session 1"
origins:
  - origin: "facere"
    type: root
    language: Latin
    meaning: "to make, to do"
    english_forms: [fac, fic, fect]
    note: "Watch for fic and fect."
`
	const defsYAML = `- metadata:
    title: "Session 1"
  scenes:
  - metadata:
      index: 0
      title: S1
    expressions:
    - expression: facsimile
      meaning: an exact copy
      pronunciation: fak-SIM-uh-lee
      note: 'fac "make" + simile "like" = "make similar"'
      examples:
        - The museum displayed a facsimile of the manuscript.
      origin_parts:
      - origin: facere
`
	svc, bookID, _ := etymologyFixture(t, etymYAML, defsYAML)

	cards, err := svc.LoadEtymologyOriginCards([]string{bookID}, true, false, nil)
	require.NoError(t, err)
	require.Len(t, cards, 1)

	card := cards[0]
	assert.Equal(t, []string{"fac", "fic", "fect"}, card.EnglishForms)
	assert.Equal(t, "Watch for fic and fect.", card.Note)

	require.Len(t, card.Words, 1)
	w := card.Words[0]
	assert.Equal(t, "facsimile", w.Expression)
	assert.Equal(t, "an exact copy", w.Meaning) // still the only graded field
	assert.Equal(t, "fak-SIM-uh-lee", w.Pronunciation)
	assert.Equal(t, `fac "make" + simile "like" = "make similar"`, w.Literal)
	assert.Equal(t, []string{"The museum displayed a facsimile of the manuscript."}, w.Examples)
}

// TestSaveEtymologyOriginResult_OneSeriesPerOrigin verifies L1/L4: two answers
// on the same origin append to ONE origin series (not one per family word),
// the read path sees them via the same canonical key (L2), and the derived
// vocabulary words' learning history is left untouched (L4).
func TestSaveEtymologyOriginResult_OneSeriesPerOrigin(t *testing.T) {
	svc, bookID, learningDir := etymologyFixture(t, singleSenseEtymYAML, singleSenseDefsYAML)

	cards, err := svc.LoadEtymologyOriginCards([]string{bookID}, true, false, nil)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	card := cards[0]

	require.NoError(t, svc.SaveEtymologyOriginResult(card, 5, true, 1000, true, nil))
	require.NoError(t, svc.SaveEtymologyOriginResult(card, 1, false, 1000, true, nil))

	// L2: the read path used by the submit response finds the just-written
	// series via the same canonical (session, origin, sense) key.
	learnedAt, _ := svc.GetLatestOriginLearnedInfo(card.NotebookName, card.SessionTitle, card.Origin, card.Sense)
	assert.NotEmpty(t, learnedAt, "L2: write must be visible to the read path")

	raw, err := os.ReadFile(filepath.Join(learningDir, bookID+".yml"))
	require.NoError(t, err)
	var histories []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &histories))

	// L1: exactly one origin entry for scribo, carrying BOTH answers as one
	// series — never one entry (or one series) per derived word.
	var originEntries int
	var logCount int
	for _, h := range histories {
		for _, e := range h.Expressions {
			if e.Type == notebook.LearningExpressionTypeOrigin && e.Expression == "scribo" {
				originEntries++
				logCount = len(e.EtymologyOriginLogs)
			}
			// L4: the derived words must NOT gain any etymology/vocab logs from
			// the origin quiz.
			if e.Expression == "describe" || e.Expression == "inscribe" {
				t.Errorf("L4 violation: derived word %q gained a learning entry from the origin quiz", e.Expression)
			}
		}
	}
	assert.Equal(t, 1, originEntries, "L1: one canonical entry per (origin, sense)")
	assert.Equal(t, 2, logCount, "L1/L4: both answers land in the single origin series")
}

// TestEtymologyOrigin_OverrideRoundTrip verifies L2 for the override path: a
// Mark-as-Correct override resolves the exact (session, origin, sense) series
// through the same canonical lookup the write path used, flips the log, and the
// read path reflects it.
func TestEtymologyOrigin_OverrideRoundTrip(t *testing.T) {
	svc, bookID, learningDir := etymologyFixture(t, singleSenseEtymYAML, singleSenseDefsYAML)
	svc.learningRepository = learning.NewYAMLLearningRepository(learningDir, nil)

	cards, err := svc.LoadEtymologyOriginCards([]string{bookID}, true, false, nil)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	card := cards[0]

	// Record a wrong answer, then override it to correct.
	require.NoError(t, svc.SaveEtymologyOriginResult(card, 1, false, 1000, true, nil))
	learnedAt, _ := svc.GetLatestOriginLearnedInfo(card.NotebookName, card.SessionTitle, card.Origin, card.Sense)
	require.NotEmpty(t, learnedAt)

	markCorrect := true
	_, err = svc.OverrideAnswer(CardInfo{
		NotebookName: card.NotebookName,
		StoryTitle:   card.SessionTitle,
		Expression:   card.Origin,
		Sense:        card.Sense,
		LearnedAt:    learnedAt,
		MarkCorrect:  &markCorrect,
	}, notebook.QuizTypeEtymologyOrigin)
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(learningDir, bookID+".yml"))
	require.NoError(t, err)
	var histories []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &histories))
	expr := notebook.FindOriginExpression(histories, card.SessionTitle, card.Origin, card.Sense)
	require.NotNil(t, expr)
	require.Len(t, expr.EtymologyOriginLogs, 1)
	assert.Equal(t, notebook.LearnedStatusUnderstood, expr.EtymologyOriginLogs[0].Status,
		"override must flip the origin series' log to a correct status")
}

// TestOverrideEtymologyWordResult_FlipsOnlyThatWordWithinTheOneRecord verifies
// the bug-2 fix: overriding an individual derived family word's correctness
// (a) lands as inline data on the origin's SAME existing log entry — never a
// second entry/series for the word (L1); (b) leaves the sibling family word's
// stored result untouched; (c) leaves the origin's own aggregate
// Status/Quality/IntervalDays untouched — the origin-level override remains
// the only thing that can change those; and (d) never creates a vocabulary
// entry for the derived word, so its flashcard/story/other-quiz-mode history
// is unaffected (L4).
func TestOverrideEtymologyWordResult_FlipsOnlyThatWordWithinTheOneRecord(t *testing.T) {
	svc, bookID, learningDir := etymologyFixture(t, singleSenseEtymYAML, singleSenseDefsYAML)

	cards, err := svc.LoadEtymologyOriginCards([]string{bookID}, true, false, nil)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	card := cards[0]

	// Both family words graded incorrect (describe/inscribe from the fixture).
	wordResults := []notebook.EtymologyWordLog{
		{Expression: "describe", Correct: false},
		{Expression: "inscribe", Correct: false},
	}
	require.NoError(t, svc.SaveEtymologyOriginResult(card, 1, false, 1000, true, wordResults))
	learnedAt, _ := svc.GetLatestOriginLearnedInfo(card.NotebookName, card.SessionTitle, card.Origin, card.Sense)
	require.NotEmpty(t, learnedAt)

	// Override "describe" to correct; leave "inscribe" alone.
	correct := true
	require.NoError(t, svc.OverrideEtymologyWordResult(
		card.NotebookName, card.SessionTitle, card.Origin, card.Sense, learnedAt, "describe",
		&correct, nil,
	))

	raw, err := os.ReadFile(filepath.Join(learningDir, bookID+".yml"))
	require.NoError(t, err)
	var histories []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &histories))

	// L1: still exactly one origin entry with one log — the override must not
	// fork a second entry or a second log for the word.
	var originEntries int
	for _, h := range histories {
		for _, e := range h.Expressions {
			if e.Type == notebook.LearningExpressionTypeOrigin && e.Expression == "scribo" {
				originEntries++
			}
			// L4: the derived words must NOT gain their own learning entry
			// from a word-level etymology override.
			if e.Expression == "describe" || e.Expression == "inscribe" {
				t.Errorf("L4 violation: derived word %q gained a learning entry from the word override", e.Expression)
			}
		}
	}
	assert.Equal(t, 1, originEntries, "L1: word override must not fork a second origin entry")

	expr := notebook.FindOriginExpression(histories, card.SessionTitle, card.Origin, card.Sense)
	require.NotNil(t, expr)
	require.Len(t, expr.EtymologyOriginLogs, 1, "L1: word override must not append a second log")
	log := expr.EtymologyOriginLogs[0]

	// (b) and (c): only "describe" flips; "inscribe" and the aggregate
	// record's own status/quality are untouched.
	require.Len(t, log.WordResults, 2)
	byExpr := map[string]notebook.EtymologyWordLog{}
	for _, w := range log.WordResults {
		byExpr[w.Expression] = w
	}
	assert.True(t, byExpr["describe"].Correct, "the overridden word must flip to correct")
	assert.False(t, byExpr["inscribe"].Correct, "the sibling word must be untouched by the override")
	assert.Equal(t, notebook.LearnedStatusMisunderstood, log.Status,
		"a word-level override must not change the origin's own aggregate status")
	assert.Equal(t, 1, log.Quality,
		"a word-level override must not change the origin's own aggregate quality")
}

// TestOverrideEtymologyWordResult_BlankGradedIncorrect verifies that a word
// the learner left blank (recorded incorrect — there is no "skipped" state)
// can be flipped to correct on the feedback screen via the per-word override,
// landing on the origin's ONE stored record (invariant L1).
func TestOverrideEtymologyWordResult_BlankGradedIncorrect(t *testing.T) {
	svc, bookID, learningDir := etymologyFixture(t, singleSenseEtymYAML, singleSenseDefsYAML)

	cards, err := svc.LoadEtymologyOriginCards([]string{bookID}, true, false, nil)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	card := cards[0]

	// "describe" was answered correctly; "inscribe" was left blank, so it is
	// recorded as a normal miss (Correct=false) — not a distinct skipped state.
	wordResults := []notebook.EtymologyWordLog{
		{Expression: "describe", Correct: true},
		{Expression: "inscribe", Correct: false},
	}
	require.NoError(t, svc.SaveEtymologyOriginResult(card, 1, false, 1000, true, wordResults))
	learnedAt, _ := svc.GetLatestOriginLearnedInfo(card.NotebookName, card.SessionTitle, card.Origin, card.Sense)
	require.NotEmpty(t, learnedAt)

	// The learner later reconsiders and marks the missed word correct.
	correct := true
	require.NoError(t, svc.OverrideEtymologyWordResult(
		card.NotebookName, card.SessionTitle, card.Origin, card.Sense, learnedAt, "inscribe",
		&correct, nil,
	))

	raw, err := os.ReadFile(filepath.Join(learningDir, bookID+".yml"))
	require.NoError(t, err)
	var histories []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &histories))

	expr := notebook.FindOriginExpression(histories, card.SessionTitle, card.Origin, card.Sense)
	require.NotNil(t, expr)
	require.Len(t, expr.EtymologyOriginLogs, 1)
	byExpr := map[string]notebook.EtymologyWordLog{}
	for _, w := range expr.EtymologyOriginLogs[0].WordResults {
		byExpr[w.Expression] = w
	}
	assert.True(t, byExpr["inscribe"].Correct, "override must flip the word to correct")
}

// TestOverrideEtymologyWordResult_UnknownWord_NotFound verifies the override
// is a hard no-op error (not a silent write) when the word isn't part of the
// stored record — it must never fall back to appending a guessed entry.
func TestOverrideEtymologyWordResult_UnknownWord_NotFound(t *testing.T) {
	svc, bookID, _ := etymologyFixture(t, singleSenseEtymYAML, singleSenseDefsYAML)

	cards, err := svc.LoadEtymologyOriginCards([]string{bookID}, true, false, nil)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	card := cards[0]

	wordResults := []notebook.EtymologyWordLog{{Expression: "describe", Correct: true}}
	require.NoError(t, svc.SaveEtymologyOriginResult(card, 5, true, 1000, true, wordResults))
	learnedAt, _ := svc.GetLatestOriginLearnedInfo(card.NotebookName, card.SessionTitle, card.Origin, card.Sense)

	correct := false
	err = svc.OverrideEtymologyWordResult(
		card.NotebookName, card.SessionTitle, card.Origin, card.Sense, learnedAt, "not-a-family-word",
		&correct, nil,
	)
	assert.Error(t, err)
}

const multiSenseEtymYAML = `metadata:
  title: "Session 1"
origins:
  - origin: "pathos"
    type: root
    language: Greek
    meaning: feeling
    sense: feeling
  - origin: "pathos"
    type: root
    language: Greek
    meaning: disease, suffering
    sense: disease
`

const multiSenseDefsYAML = `- metadata:
    title: "Session 1"
  scenes:
  - metadata:
      index: 0
      title: S1
    expressions:
    - expression: sympathy
      meaning: shared feeling
      origin_parts:
      - origin: pathos
        sense: feeling
    - expression: pathology
      meaning: the study of disease
      origin_parts:
      - origin: pathos
        sense: disease
`

// TestEtymologyOrigin_MultiSense_SeparateSeries verifies a same-session
// multi-sense origin yields one card and one log series PER sense (invariant
// L4: sense selects the series, it does not create a parallel series), each
// with its own family word.
func TestEtymologyOrigin_MultiSense_SeparateSeries(t *testing.T) {
	svc, bookID, learningDir := etymologyFixture(t, multiSenseEtymYAML, multiSenseDefsYAML)

	cards, err := svc.LoadEtymologyOriginCards([]string{bookID}, true, false, nil)
	require.NoError(t, err)
	require.Len(t, cards, 2, "two senses of the same origin become two cards")

	familyBySense := map[string][]string{}
	for _, c := range cards {
		require.NoError(t, svc.SaveEtymologyOriginResult(c, 5, true, 1000, true, nil))
		for _, w := range c.Words {
			familyBySense[c.Sense] = append(familyBySense[c.Sense], w.Expression)
		}
	}
	assert.Equal(t, []string{"sympathy"}, familyBySense["feeling"])
	assert.Equal(t, []string{"pathology"}, familyBySense["disease"])

	raw, err := os.ReadFile(filepath.Join(learningDir, bookID+".yml"))
	require.NoError(t, err)
	var histories []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &histories))

	feeling := notebook.FindOriginExpression(histories, "Session 1", "pathos", "feeling")
	disease := notebook.FindOriginExpression(histories, "Session 1", "pathos", "disease")
	require.NotNil(t, feeling)
	require.NotNil(t, disease)
	assert.Len(t, feeling.EtymologyOriginLogs, 1)
	assert.Len(t, disease.EtymologyOriginLogs, 1)
	assert.NotSame(t, feeling, disease, "each sense keeps an independent series")
}

// familyExpressions returns the derived-word expressions of the one card the
// loader emits for the single-sense scribo fixture (or nil when the origin is
// no longer offered because its whole family was excluded).
func familyExpressions(t *testing.T, svc *Service, bookID string) []string {
	t.Helper()
	cards, err := svc.LoadEtymologyOriginCards([]string{bookID}, true, false, nil)
	require.NoError(t, err)
	if len(cards) == 0 {
		return nil
	}
	require.Len(t, cards, 1)
	var out []string
	for _, w := range cards[0].Words {
		out = append(out, w.Expression)
	}
	return out
}

// TestEtymologyOrigin_PerWordExclude verifies FIX 2: excluding ONE family word
// (a) persists a per-word etymology-origin skipped_at, (b) drops only that word
// from the origin's family, (c) drops the whole origin once every word is
// excluded, and (d) is reversible with ResumeWord. Exclusion uses the SAME
// SkipWord/skipped_at path every other card uses, and the loader reads it back
// via the same key (invariants L1/L2, U1/U2). It also asserts the browse
// payload's exclusion read (IsExpressionExcludedForQuizType) agrees.
func TestEtymologyOrigin_PerWordExclude(t *testing.T) {
	svc, bookID, learningDir := etymologyFixture(t, singleSenseEtymYAML, singleSenseDefsYAML)

	// Baseline: the origin carries both family words.
	assert.ElementsMatch(t, []string{"describe", "inscribe"}, familyExpressions(t, svc, bookID))

	excludeWord := func(expr string) {
		require.NoError(t, svc.SkipWord(
			CardInfo{NotebookName: bookID, StoryTitle: "Session 1", SceneTitle: "S1", Expression: expr},
			"", []notebook.QuizType{notebook.QuizTypeEtymologyOrigin},
		))
	}
	readHistories := func() []notebook.LearningHistory {
		raw, err := os.ReadFile(filepath.Join(learningDir, bookID+".yml"))
		require.NoError(t, err)
		var histories []notebook.LearningHistory
		require.NoError(t, yaml.Unmarshal(raw, &histories))
		return histories
	}

	// Exclude "describe": only that word drops; the origin stays quizzable.
	excludeWord("describe")
	assert.Equal(t, []string{"inscribe"}, familyExpressions(t, svc, bookID))
	assert.True(t, notebook.IsExpressionExcludedForQuizType(
		readHistories(), "", notebook.QuizTypeEtymologyOrigin, "describe"),
		"the browse payload must report the excluded word as skipped")
	assert.False(t, notebook.IsExpressionExcludedForQuizType(
		readHistories(), "", notebook.QuizTypeEtymologyOrigin, "inscribe"))

	// Exclude "inscribe" too: the origin now has no family and is not offered.
	excludeWord("inscribe")
	assert.Nil(t, familyExpressions(t, svc, bookID), "an origin with all words excluded is not offered")

	// Resume "describe": it reappears; "inscribe" stays excluded.
	require.NoError(t, svc.ResumeWord(
		CardInfo{NotebookName: bookID, StoryTitle: "Session 1", SceneTitle: "S1", Expression: "describe"},
		[]notebook.QuizType{notebook.QuizTypeEtymologyOrigin},
	))
	assert.Equal(t, []string{"describe"}, familyExpressions(t, svc, bookID))
}
