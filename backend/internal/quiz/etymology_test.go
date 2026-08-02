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

	require.NoError(t, svc.SaveEtymologyOriginResult(card, 5, true, 1000, true))
	require.NoError(t, svc.SaveEtymologyOriginResult(card, 1, false, 1000, true))

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
		require.NoError(t, svc.SaveEtymologyOriginResult(c, 5, true, 1000, true))
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
