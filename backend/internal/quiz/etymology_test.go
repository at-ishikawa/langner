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
		nil, config.QuizConfig{Algorithm: "modified_sm2", FixedIntervals: []int{1, 7, 30, 90, 365, 1095, 1825}, DisableShuffle: true})
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

// answerCard grades every family word on the card with the given correctness
// and persists a per-word etymology-origin result (mirrors what the handler
// does), returning the aggregate learned_at.
func answerCard(t *testing.T, svc *Service, card EtymologyOriginCard, correct bool) string {
	t.Helper()
	quality := 5
	if !correct {
		quality = 1
	}
	grades := make([]EtymologyWordGrade, 0, len(card.Words))
	for _, w := range card.Words {
		grades = append(grades, EtymologyWordGrade{Word: w, Correct: correct, Quality: quality})
	}
	learnedAt, _, err := svc.SaveEtymologyWordResults(card, grades, 1000)
	require.NoError(t, err)
	return learnedAt
}

// wordEntry finds a derived word's learning-history entry across scenes.
func wordEntry(histories []notebook.LearningHistory, expr string) *notebook.LearningHistoryExpression {
	for hi := range histories {
		for ei := range histories[hi].Expressions {
			if histories[hi].Expressions[ei].Expression == expr {
				return &histories[hi].Expressions[ei]
			}
		}
		for si := range histories[hi].Scenes {
			for ei := range histories[hi].Scenes[si].Expressions {
				if histories[hi].Scenes[si].Expressions[ei].Expression == expr {
					return &histories[hi].Scenes[si].Expressions[ei]
				}
			}
		}
	}
	return nil
}

// TestSaveEtymologyWordResults_OneSeriesPerWord verifies L1/L4: answering a card
// writes ONE etymology-origin series per DERIVED WORD (not per origin), two
// answers append to that word's single series, and no per-origin entry is ever
// created.
func TestSaveEtymologyWordResults_OneSeriesPerWord(t *testing.T) {
	svc, bookID, learningDir := etymologyFixture(t, singleSenseEtymYAML, singleSenseDefsYAML)

	cards, err := svc.LoadEtymologyOriginCards([]string{bookID}, true, false, nil)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	card := cards[0]

	learnedAt := answerCard(t, svc, card, true)
	assert.NotEmpty(t, learnedAt, "L2: write must be visible via the returned learned_at")
	answerCard(t, svc, card, false)

	raw, err := os.ReadFile(filepath.Join(learningDir, bookID+".yml"))
	require.NoError(t, err)
	var histories []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &histories))

	// L1: no per-origin entry exists — the origin is presentation only.
	for _, h := range histories {
		for _, e := range h.Expressions {
			if e.Type == notebook.LearningExpressionTypeOrigin {
				t.Errorf("L1 violation: a per-origin entry %q was created; the schedule is per-word now", e.Expression)
			}
		}
	}

	// L1/L4: each derived word carries its OWN etymology series with BOTH
	// answers; the standard series is untouched.
	for _, expr := range []string{"describe", "inscribe"} {
		e := wordEntry(histories, expr)
		require.NotNilf(t, e, "derived word %q must own an etymology-origin series", expr)
		assert.Lenf(t, e.EtymologyOriginLogs, 2, "both answers land in %q's single per-word series", expr)
		assert.Emptyf(t, e.LearnedLogs, "the origin quiz must not touch %q's standard series (L4)", expr)
	}
}

// TestEtymologyOrigin_WordOverrideRoundTrip verifies L2 for the per-word override
// path: a Mark-as-Correct override resolves the WORD's own etymology series by
// expression (the same key the exclude path uses), flips its log to a correct
// status, and never forks a second series.
func TestEtymologyOrigin_WordOverrideRoundTrip(t *testing.T) {
	svc, bookID, learningDir := etymologyFixture(t, singleSenseEtymYAML, singleSenseDefsYAML)

	cards, err := svc.LoadEtymologyOriginCards([]string{bookID}, true, false, nil)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	card := cards[0]

	learnedAt := answerCard(t, svc, card, false) // both words wrong

	// Override only "describe" to correct; "inscribe" stays wrong.
	correct := true
	require.NoError(t, svc.OverrideEtymologyWordResult(card.NotebookName, learnedAt, "describe", &correct))

	raw, err := os.ReadFile(filepath.Join(learningDir, bookID+".yml"))
	require.NoError(t, err)
	var histories []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &histories))

	describe := wordEntry(histories, "describe")
	require.NotNil(t, describe)
	require.Len(t, describe.EtymologyOriginLogs, 1, "L1: override must not fork a second log")
	assert.Equal(t, notebook.LearnedStatusUnderstood, describe.EtymologyOriginLogs[0].Status,
		"override must flip the word's own series to a correct status")

	inscribe := wordEntry(histories, "inscribe")
	require.NotNil(t, inscribe)
	require.Len(t, inscribe.EtymologyOriginLogs, 1)
	assert.Equal(t, notebook.LearnedStatusMisunderstood, inscribe.EtymologyOriginLogs[0].Status,
		"the sibling word must be untouched by the override")
}

// TestOverrideEtymologyWordResult_UnknownWord_NotFound verifies the per-word
// override is a hard no-op error (not a silent write) when no series exists for
// that word — it must never fabricate an entry.
func TestOverrideEtymologyWordResult_UnknownWord_NotFound(t *testing.T) {
	svc, bookID, _ := etymologyFixture(t, singleSenseEtymYAML, singleSenseDefsYAML)

	cards, err := svc.LoadEtymologyOriginCards([]string{bookID}, true, false, nil)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	learnedAt := answerCard(t, svc, cards[0], true)

	correct := false
	err = svc.OverrideEtymologyWordResult(cards[0].NotebookName, learnedAt, "not-a-family-word", &correct)
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

// TestEtymologyOrigin_MultiSense_SeparateWords verifies a same-session
// multi-sense origin yields one card per sense, each carrying its own derived
// word, and that answering advances each WORD's own per-word series (the words
// differ per sense, so the series never collide).
func TestEtymologyOrigin_MultiSense_SeparateWords(t *testing.T) {
	svc, bookID, learningDir := etymologyFixture(t, multiSenseEtymYAML, multiSenseDefsYAML)

	cards, err := svc.LoadEtymologyOriginCards([]string{bookID}, true, false, nil)
	require.NoError(t, err)
	require.Len(t, cards, 2, "two senses of the same origin become two cards")

	familyBySense := map[string][]string{}
	for _, c := range cards {
		answerCard(t, svc, c, true)
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

	sympathy := wordEntry(histories, "sympathy")
	pathology := wordEntry(histories, "pathology")
	require.NotNil(t, sympathy)
	require.NotNil(t, pathology)
	assert.Len(t, sympathy.EtymologyOriginLogs, 1)
	assert.Len(t, pathology.EtymologyOriginLogs, 1)
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

// A multi-origin fixture: "deficient" derives from BOTH "de" and "facere", so
// under a per-origin schedule it lived in two families and was asked twice per
// session. "defer" derives from "de" only. Generic Latin examples — no personal
// data.
const multiOriginEtymYAML = `metadata:
  title: "Session 1"
origins:
  - origin: "de"
    type: prefix
    language: Latin
    meaning: down, away
  - origin: "facere"
    type: root
    language: Latin
    meaning: to make, to do
`

const multiOriginDefsYAML = `- metadata:
    title: "Session 1"
  scenes:
  - metadata:
      index: 0
      title: S1
    expressions:
    - expression: deficient
      meaning: lacking something
      origin_parts:
      - origin: de
      - origin: facere
    - expression: defer
      meaning: to put off
      origin_parts:
      - origin: de
`

// cardExpressions returns the derived-word expressions on a card.
func cardExpressions(c EtymologyOriginCard) []string {
	var out []string
	for _, w := range c.Words {
		out = append(out, w.Expression)
	}
	return out
}

// TestLoadEtymologyOriginCards_WithinSessionDedup verifies FIX 3: a word whose
// origin_parts span two origins is placed on exactly ONE card per session, and a
// card whose only word was already placed on an earlier card is dropped.
func TestLoadEtymologyOriginCards_WithinSessionDedup(t *testing.T) {
	svc, bookID, _ := etymologyFixture(t, multiOriginEtymYAML, multiOriginDefsYAML)

	cards, err := svc.LoadEtymologyOriginCards([]string{bookID}, true, false, nil)
	require.NoError(t, err)

	// "deficient" appears on exactly one card across the whole deck.
	count := 0
	for _, c := range cards {
		for _, w := range c.Words {
			if w.Expression == "deficient" {
				count++
			}
		}
	}
	assert.Equal(t, 1, count, "a multi-origin word must be asked exactly once per session")

	// The "facere" card's only word was "deficient" (placed on the earlier "de"
	// card), so that card is dropped and only "de" survives with both its words.
	require.Len(t, cards, 1)
	assert.Equal(t, "de", cards[0].Origin)
	assert.ElementsMatch(t, []string{"deficient", "defer"}, cardExpressions(cards[0]))
}

// TestLoadEtymologyNotebookSummaries_CountsDistinctDueWords verifies the review
// badge counts DISTINCT due words: "deficient" (in two origins) + "defer" = 2,
// not 3 (which the old per-origin double-count would report).
func TestLoadEtymologyNotebookSummaries_CountsDistinctDueWords(t *testing.T) {
	svc, _, _ := etymologyFixture(t, multiOriginEtymYAML, multiOriginDefsYAML)

	summaries, err := svc.LoadEtymologyNotebookSummaries(true)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, 2, summaries[0].EtymologyReviewCount,
		"distinct due words are counted once, not once per origin")
	require.Len(t, summaries[0].Sections, 1)
	assert.Equal(t, 2, summaries[0].Sections[0].EtymologyReviewCount)
}

// TestEtymologyOrigin_PerWordSchedule_NoCrossOriginRepeat verifies FIX 1: after
// the multi-origin word is answered correctly ONCE, its per-word schedule is no
// longer due, so it never resurfaces under EITHER origin in a later session (the
// same-day / pre-interval case).
func TestEtymologyOrigin_PerWordSchedule_NoCrossOriginRepeat(t *testing.T) {
	svc, bookID, _ := etymologyFixture(t, multiOriginEtymYAML, multiOriginDefsYAML)

	cards, err := svc.LoadEtymologyOriginCards([]string{bookID}, true, false, nil)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	answerCard(t, svc, cards[0], true) // answers deficient + defer correctly

	// Next session, before the interval lapses: no word is due, and "deficient"
	// in particular is absent under every origin.
	again, err := svc.LoadEtymologyOriginCards([]string{bookID}, false, false, nil)
	require.NoError(t, err)
	for _, c := range again {
		assert.NotContains(t, cardExpressions(c), "deficient",
			"a word answered once must not recur under any origin the same session")
	}
}
