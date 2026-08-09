package quiz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/learning"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
)

func TestRelearnRecognitionContexts_GuaranteesOneAnchoredContext(t *testing.T) {
	// A word with no example sentences must still yield one context carrying the
	// meaning as reference_definition — otherwise the meaning grader returns no
	// answer and the card is always marked incorrect.
	got := relearnRecognitionContexts(FreeformCard{Meaning: "all-knowing"})
	require.Len(t, got, 1)
	assert.Equal(t, "all-knowing", got[0].ReferenceDefinition)
}

func TestRelearnRecognitionContexts_SetsReferenceOnEveryContext(t *testing.T) {
	got := relearnRecognitionContexts(FreeformCard{
		Meaning: "abstaining from worldly pleasures",
		Contexts: []inference.Context{
			{Context: "an ascetic monk"},
			{Context: "an ascetic life"},
		},
	})
	require.Len(t, got, 2)
	for _, c := range got {
		assert.Equal(t, "abstaining from worldly pleasures", c.ReferenceDefinition,
			"the known meaning is the grader's authoritative ground truth")
	}
	assert.Equal(t, "an ascetic monk", got[0].Context, "existing context sentences are preserved")
}

// TestLoadRelearnPool_ResolvesHomographByID pins invariant L2 (symmetric read
// and write) for the Relearn pool: two vocab entries that share the SAME
// expression AND part_of_speech but carry distinct stable ids and distinct
// meanings must each resolve to their OWN card by id — never collapse into one
// last-write-wins entry showing the other sense's meaning.
//
// Before the id-keyed resolution, both senses collided: the candidate map keyed
// only by (format, notebook, expression) collapsed the pair into a single
// candidate, and the vocab index keyed only by expression returned whichever
// card was written last — so the surviving card displayed the WRONG meaning.
func TestLoadRelearnPool_ResolvesHomographByID(t *testing.T) {
	const (
		notebookID  = "vocab"
		expr        = "bank"
		firstID     = "bank-river"
		secondID    = "bank-money"
		firstMean   = "the land alongside a river"
		secondMean  = "a financial institution"
		partOfSpeak = "noun"
	)

	flashcardsDir := t.TempDir()
	learningDir := t.TempDir()

	notebookDir := filepath.Join(flashcardsDir, notebookID)
	require.NoError(t, os.MkdirAll(notebookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(notebookDir, "index.yml"), []byte(
		"id: "+notebookID+"\nname: \"Vocabulary\"\nnotebooks:\n  - ./cards.yml\n"), 0644))
	// Two senses of "bank" — same spelling, same part_of_speech, distinct id +
	// meaning — in ONE notebook.
	require.NoError(t, os.WriteFile(filepath.Join(notebookDir, "cards.yml"), []byte(
		`- title: "Flashcards"
  date: 2025-01-15T00:00:00Z
  cards:
    - id: `+firstID+`
      expression: `+expr+`
      part_of_speech: `+partOfSpeak+`
      meaning: `+firstMean+`
    - id: `+secondID+`
      expression: `+expr+`
      part_of_speech: `+partOfSpeak+`
      meaning: `+secondMean+`
`), 0644))

	// Learning history: both senses failed (misunderstood) in-window; the first
	// (bank-river) is the most-recent wrong, so a bug that keeps the newest
	// candidate but resolves by expression would surface the second sense.
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, notebookID+".yml"), []byte(
		`- metadata:
    id: `+notebookID+`
    title: "Flashcards"
    type: flashcard
  expressions:
    - id: `+firstID+`
      expression: `+expr+`
      type: vocabulary
      learned_logs:
        - status: misunderstood
          learned_at: "2026-07-20T00:00:00Z"
    - id: `+secondID+`
      expression: `+expr+`
      type: vocabulary
      learned_logs:
        - status: misunderstood
          learned_at: "2026-07-19T00:00:00Z"
`), 0644))

	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl) // never called: LoadRelearnPool does no grading
	svc := NewService(config.NotebooksConfig{
		FlashcardsDirectories:  []string{flashcardsDir},
		LearningNotesDirectory: learningDir,
	}, mockClient, make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil),
		config.QuizConfig{})

	windowStart := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	cards, err := svc.LoadRelearnPool(windowStart)
	require.NoError(t, err)

	// Each id resolves to its own card: two cards, each carrying its own sense's
	// meaning. A collision would produce one card (or two cards both showing the
	// second sense).
	require.Len(t, cards, 2, "each distinct id must yield its own relearn card")
	meanings := map[string]bool{}
	for _, c := range cards {
		assert.Equal(t, expr, c.Entry)
		meanings[c.Meaning] = true
	}
	assert.True(t, meanings[firstMean],
		"the first sense (%s) must resolve to its own meaning by id, not the other sense's", firstID)
	assert.True(t, meanings[secondMean],
		"the second sense (%s) must resolve to its own meaning by id", secondID)
}

// TestLoadRelearnPool_ConceptMemberUsesHeadMeaning pins the consummate bug:
// a word folded into a definitions family-concept must be shown in relearn
// under the concept HEAD and its umbrella meaning — the same as the standard
// quiz — not resolved to its own raw sense by last-write-wins. "bank" here has
// two senses and is a concept member; relearn must show the concept meaning.
func TestLoadRelearnPool_ConceptMemberUsesHeadMeaning(t *testing.T) {
	ctrl := gomock.NewController(t)
	defsDir := t.TempDir()
	learningDir := t.TempDir()

	bookDir := filepath.Join(defsDir, "test-book")
	require.NoError(t, os.MkdirAll(bookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "index.yml"), []byte(`id: test-book
notebooks:
  - ./ch.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "ch.yml"), []byte(`- metadata:
    title: Chapter 1
  scenes:
  - metadata:
      index: 0
      title: S1
    expressions:
    - expression: bank
      id: bank
      meaning: the land beside a river
    - expression: bank
      id: bank-2
      meaning: a place to keep money
  concepts:
  - head: bank
    meaning: umbrella meaning for the bank family
    expressions:
    - bank
`), 0644))

	recent := time.Now().Add(-30 * time.Minute).Format(time.RFC3339)
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-book.yml"), []byte(fmt.Sprintf(`- metadata:
    notebook_id: test-book
    title: Chapter 1
  scenes:
  - metadata:
      title: S1
    expressions:
    - expression: bank
      learned_logs:
      - status: misunderstood
        learned_at: %q
        quiz_type: notebook
`, recent)), 0644))

	svc := NewService(config.NotebooksConfig{
		DefinitionsDirectories: []string{defsDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	cards, err := svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
	require.NoError(t, err)

	var found bool
	for _, c := range cards {
		if c.Entry == "bank" {
			found = true
			assert.Equal(t, "umbrella meaning for the bank family", c.Meaning,
				"a concept member must show the concept head's meaning, not a raw sense")
		}
	}
	require.True(t, found, "the failed concept member must appear in the relearn pool")
}

// TestLoadRelearnPool_GrammarMiss pins Part A of the grammar/Relearn fix: a
// missed grammar correction (a "misunderstood" log under the flat "grammar"
// learning-history bucket) must resurface in the Relearn pool as a
// QuizTypeGrammar card carrying the entry's full content and the mistaken
// span — not be silently dropped because relearnSeries mislabeled it
// QuizTypeNotebook and it then failed vocab resolution (the pre-fix bug).
func TestLoadRelearnPool_GrammarMiss(t *testing.T) {
	storiesDir, grammarsDir := writeGrammarNotebook(t)
	learningDir := t.TempDir()

	// A learning history entry for the correction, written the same way
	// SaveGrammarBlank writes it: flat "grammar" bucket, Expression == ID ==
	// the correction's stable senseID, status misunderstood.
	recent := time.Now().Add(-30 * time.Minute).Format(time.RFC3339)
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "journal.yml"), []byte(fmt.Sprintf(`- metadata:
    id: journal
    title: journal
    type: grammar
  expressions:
    - id: note-the-john
      expression: note-the-john
      learned_logs:
        - status: misunderstood
          learned_at: %q
          quiz_type: grammar
`, recent)), 0o644))

	svc := newGrammarService(t, storiesDir, grammarsDir, learningDir)

	cards, err := svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
	require.NoError(t, err)
	require.Len(t, cards, 1, "the missed correction must yield exactly one relearn card")

	card := cards[0]
	assert.Equal(t, notebook.QuizTypeGrammar, card.Format)
	assert.Contains(t, card.Content, "the John called me", "the card carries the whole entry, like the live grammar quiz")
	assert.Equal(t, "the John", card.Incorrect)
	// NotebookName carries the notebook ID (not the display name) so a
	// deliberate Exclude (SkipWord) resolves to the correct <id>.yml.
	assert.Equal(t, "journal", card.NotebookName)

	// The card must grade like the live quiz: GradeGrammarBlank against the
	// card's own grading inputs (GrammarCard(), never sent to the client).
	result, err := svc.GradeGrammarBlank(context.Background(), card.Content, card.GrammarCard(), "John", 1200)
	require.NoError(t, err)
	assert.True(t, result.Correct)
}

// TestLoadRelearnPool_GrammarSamePostMultipleBlanks pins the contract the
// frontend's post-grouping relies on: when a single journal post has MORE THAN
// ONE due correction, the pool emits one card per correction, every card
// carries the SAME full post text (so the client can group them into one
// progressive post view), each card keeps its OWN mistaken span and grades
// individually — and a correction of the same post that is NOT due (its latest
// log is not "misunderstood") is not emitted at all, so relearn only ever asks
// the due blanks while still rendering the whole post for context.
func TestLoadRelearnPool_GrammarSamePostMultipleBlanks(t *testing.T) {
	base := t.TempDir()
	learningDir := t.TempDir()

	storyDir := filepath.Join(base, "stories", "journal")
	require.NoError(t, os.MkdirAll(storyDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(
		"id: journal\nname: \"English Journal\"\nnotebooks:\n  - ./posts.yml\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "posts.yml"), []byte(
		"- event: \"Note 1\"\n  scenes:\n    - scene: \"\"\n      statements:\n        - \"Yesterday the John called me and then I go home.\"\n"), 0o644))

	grammarsDir := filepath.Join(base, "grammars", "journal")
	require.NoError(t, os.MkdirAll(grammarsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(grammarsDir, "index.yml"), []byte(
		"id: journal\nnotebooks:\n  - ./corr.yml\n"), 0o644))
	// One post, three corrections: two will be due, one will not be.
	require.NoError(t, os.WriteFile(filepath.Join(grammarsDir, "corr.yml"), []byte(
		`- metadata:
    title: "Note 1"
  scenes:
    - metadata:
        index: 0
      corrections:
        - id: note-the-john
          incorrect: "the John"
          correct: "John"
          category: article
          reason: "No article before a personal name."
        - id: note-go
          incorrect: "go"
          correct: "went"
          category: tense
          reason: "Use past tense for a past event."
        - id: note-me
          incorrect: "called me"
          correct: "phoned me"
          category: word-choice
          reason: "Prefer a more precise verb."
`), 0o644))

	// Two corrections failed (misunderstood) in-window; the third is understood,
	// so it is NOT due and must not enter the pool.
	recent := time.Now().Add(-30 * time.Minute).Format(time.RFC3339)
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "journal.yml"), []byte(fmt.Sprintf(`- metadata:
    id: journal
    title: journal
    type: grammar
  expressions:
    - id: note-the-john
      expression: note-the-john
      learned_logs:
        - status: misunderstood
          learned_at: %q
          quiz_type: grammar
    - id: note-go
      expression: note-go
      learned_logs:
        - status: misunderstood
          learned_at: %q
          quiz_type: grammar
    - id: note-me
      expression: note-me
      learned_logs:
        - status: understood
          learned_at: %q
          quiz_type: grammar
`, recent, recent, recent)), 0o644))

	svc := newGrammarService(t, filepath.Join(base, "stories"), filepath.Join(base, "grammars"), learningDir)

	cards, err := svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
	require.NoError(t, err)

	var grammar []RelearnCard
	for _, c := range cards {
		if c.Format == notebook.QuizTypeGrammar {
			grammar = append(grammar, c)
		}
	}
	require.Len(t, grammar, 2, "only the two DUE corrections are emitted; the understood one is excluded")

	incorrects := map[string]bool{}
	for _, c := range grammar {
		incorrects[c.Incorrect] = true
		// Every due blank carries the SAME whole post text — the group key the
		// client folds them by into one progressive post view.
		assert.Equal(t, grammar[0].Content, c.Content, "all blanks of one post share the full post text")
		assert.Contains(t, c.Content, "the John called me and then I go home")
		// Each blank grades on its OWN correction, individually (no collapsing).
		want := "John"
		if c.Incorrect == "go" {
			want = "went"
		}
		result, err := svc.GradeGrammarBlank(context.Background(), c.Content, c.GrammarCard(), want, 1200)
		require.NoError(t, err)
		assert.True(t, result.Correct, "blank %q must grade against its own reference", c.Incorrect)
	}
	assert.True(t, incorrects["the John"], "the first due mistake is asked")
	assert.True(t, incorrects["go"], "the second due mistake is asked")
	assert.False(t, incorrects["called me"], "a not-due correction of the same post is not asked")
}

// TestLoadRelearnPool_GrammarTwoBlanksKeepsBothBlanks pins the pool half of the
// "one correct blank marks the others correct and drops them" fix: two distinct
// corrections in one scene, each its OWN misunderstood series, must each survive
// as its own gradeable blank in the pool. Two corrections can no longer share a
// senseID (ensureUniqueCorrectionIDs makes every correction id unique at load,
// even a reused explicit id: — see notebook grammars_test.go and
// TestService_GrammarQuiz_CollidingIDsStayIndependent), so a single post always
// emits one card per due blank, each carrying its own span and grading
// independently.
func TestLoadRelearnPool_GrammarTwoBlanksKeepsBothBlanks(t *testing.T) {
	base := t.TempDir()
	learningDir := t.TempDir()

	storyDir := filepath.Join(base, "stories", "journal")
	require.NoError(t, os.MkdirAll(storyDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(
		"id: journal\nname: \"English Journal\"\nnotebooks:\n  - ./posts.yml\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "posts.yml"), []byte(
		"- event: \"Note 1\"\n  scenes:\n    - scene: \"\"\n      statements:\n        - \"Yesterday the John called me and then I go home.\"\n"), 0o644))

	grammarsDir := filepath.Join(base, "grammars", "journal")
	require.NoError(t, os.MkdirAll(grammarsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(grammarsDir, "index.yml"), []byte(
		"id: journal\nnotebooks:\n  - ./corr.yml\n"), 0o644))
	// Two DISTINCT corrections in one scene, each with its own id.
	require.NoError(t, os.WriteFile(filepath.Join(grammarsDir, "corr.yml"), []byte(
		`- metadata:
    title: "Note 1"
  scenes:
    - metadata:
        index: 0
      corrections:
        - id: note-the-john
          incorrect: "the John"
          correct: "John"
          category: article
          reason: "No article before a personal name."
        - id: note-i-go
          incorrect: "go"
          correct: "went"
          category: tense
          reason: "Use past tense for a past event."
`), 0o644))

	// Each correction has its own misunderstood series, so both blanks are due.
	recent := time.Now().Add(-30 * time.Minute).Format(time.RFC3339)
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "journal.yml"), []byte(fmt.Sprintf(`- metadata:
    id: journal
    title: journal
    type: grammar
  expressions:
    - id: note-the-john
      expression: note-the-john
      learned_logs:
        - status: misunderstood
          learned_at: %q
          quiz_type: grammar
    - id: note-i-go
      expression: note-i-go
      learned_logs:
        - status: misunderstood
          learned_at: %q
          quiz_type: grammar
`, recent, recent)), 0o644))

	svc := newGrammarService(t, filepath.Join(base, "stories"), filepath.Join(base, "grammars"), learningDir)

	cards, err := svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
	require.NoError(t, err)

	var grammar []RelearnCard
	for _, c := range cards {
		if c.Format == notebook.QuizTypeGrammar {
			grammar = append(grammar, c)
		}
	}
	require.Len(t, grammar, 2, "both corrections sharing one senseID must each keep their own blank")

	incorrects := map[string]bool{}
	for _, c := range grammar {
		incorrects[c.Incorrect] = true
		assert.Equal(t, grammar[0].Content, c.Content, "both blanks of one post share the full post text")
		want := "John"
		if c.Incorrect == "go" {
			want = "went"
		}
		// Each blank grades against its OWN correction — grading one never
		// resolves the other.
		result, err := svc.GradeGrammarBlank(context.Background(), c.Content, c.GrammarCard(), want, 1200)
		require.NoError(t, err)
		assert.True(t, result.Correct, "blank %q must grade against its own reference", c.Incorrect)
	}
	assert.True(t, incorrects["the John"], "the first span survives")
	assert.True(t, incorrects["go"], "the second span survives (was dropped before the fix)")
}

// etymologyRelearnFixture wires an etymology notebook (an origin with its
// gloss) and a matching definitions notebook whose words carry origin_parts
// referencing that origin, plus a learning history in which those words were
// recently missed as normal vocabulary (a "misunderstood" recognition log).
// This is the on-disk shape LoadRelearnPool reads to build origin-family cards
// from missed vocabulary words — etymology is no longer a standalone quiz.
func etymologyRelearnFixture(t *testing.T, missed ...string) *Service {
	t.Helper()
	dir := t.TempDir()
	etymDir := filepath.Join(dir, "etymology")
	defsDir := filepath.Join(dir, "definitions")
	learningDir := filepath.Join(dir, "learning")
	const bookID = "roots"

	etymBook := filepath.Join(etymDir, bookID)
	require.NoError(t, os.MkdirAll(etymBook, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(etymBook, "index.yml"), []byte(
		"id: roots\nkind: Etymology\nname: Roots\nnotebooks:\n  - ./s1.yml\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(etymBook, "s1.yml"), []byte(`metadata:
  title: "Session 1"
origins:
  - origin: "scribo"
    type: root
    language: Latin
    meaning: to write
    english_forms: [scrib, script]
`), 0644))

	defsBook := filepath.Join(defsDir, bookID)
	require.NoError(t, os.MkdirAll(defsBook, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(defsBook, "index.yml"), []byte(
		"id: roots\nnotebooks:\n  - ./s1.yml\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(defsBook, "s1.yml"), []byte(`- metadata:
    title: "Session 1"
  scenes:
  - metadata:
      index: 0
      title: S1
    expressions:
    - expression: describe
      meaning: to represent in words
      note: 'de "down" + scribo "write" = "write down"'
      origin_parts:
      - origin: scribo
    - expression: inscribe
      meaning: to write or carve on a surface
      origin_parts:
      - origin: scribo
`), 0644))

	require.NoError(t, os.MkdirAll(learningDir, 0755))
	recent := time.Now().Add(-time.Hour).Format(time.RFC3339)
	exprs := ""
	for _, w := range missed {
		exprs += fmt.Sprintf(`    - expression: %s
      type: vocabulary
      learned_logs:
        - status: misunderstood
          learned_at: "%s"
`, w, recent)
	}
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, bookID+".yml"), []byte(
		"- metadata:\n    id: roots\n    title: \"Session 1\"\n    type: definition\n  expressions:\n"+exprs), 0644))

	ctrl := gomock.NewController(t)
	return NewService(config.NotebooksConfig{
		EtymologyDirectories:   []string{etymDir},
		DefinitionsDirectories: []string{defsDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil),
		config.QuizConfig{Algorithm: "modified_sm2", FixedIntervals: []int{1, 7, 30, 90, 365, 1095, 1825}, DisableShuffle: true})
}

func etymRelearnByEntry(cards []RelearnCard) map[string]RelearnCard {
	out := map[string]RelearnCard{}
	for _, c := range cards {
		if c.Format == notebook.QuizTypeEtymologyOrigin {
			out[c.Entry] = c
		}
	}
	return out
}

// TestLoadRelearnPool_EtymologyOriginFamilyCard pins the new model: a missed
// vocabulary word that carries an etymology origin surfaces in the Relearn pool
// as a QUIZ_TYPE_ETYMOLOGY_ORIGIN card carrying the origin header
// (OriginText/OriginMeaning) so the frontend groups every card sharing an origin
// into one family card, while it is still graded against the word's own meaning.
func TestLoadRelearnPool_EtymologyOriginFamilyCard(t *testing.T) {
	svc := etymologyRelearnFixture(t, "describe", "inscribe")

	pool, err := svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
	require.NoError(t, err)
	got := etymRelearnByEntry(pool)
	require.Contains(t, got, "describe", "a missed origin-bearing word must enter the pool as a family card")
	require.Contains(t, got, "inscribe")
	// Origin header shared by the family; grading reference is the word's meaning.
	assert.Equal(t, "scribo", got["describe"].OriginText)
	assert.Equal(t, "to write", got["describe"].OriginMeaning)
	assert.Equal(t, "to represent in words", got["describe"].Meaning)
	assert.True(t, got["describe"].IsEtymology())

	// Exclude "describe" from its origin family (the same SkipWord path every
	// card uses). It leaves the origin grouping — but a normal miss must never
	// drop a word from Relearn (quiz-ui-invariants U1), so it reappears as a
	// plain recognition card. "inscribe" is untouched.
	require.NoError(t, svc.SkipWord(
		CardInfo{NotebookName: "roots", Expression: "describe"},
		"", []notebook.QuizType{notebook.QuizTypeEtymologyOrigin},
	))
	pool, err = svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
	require.NoError(t, err)
	assert.NotContains(t, etymRelearnByEntry(pool), "describe",
		"excluding from the family drops the origin grouping")
	assert.Contains(t, etymRelearnByEntry(pool), "inscribe", "excluding one word must not drop the other")
	stillDue := false
	for _, c := range pool {
		if c.Entry == "describe" && c.Format == notebook.QuizTypeNotebook {
			stillDue = true
		}
	}
	assert.True(t, stillDue, "a normal miss must keep the word due as a recognition card (U1)")
}

// TestLoadRelearnPool_EtymologyCardCarriesOriginDetails pins the Relearn
// etymology feedback contract: a pooled origin-family card must carry the origin
// roots WITH their meanings (WordDetail.OriginParts) and the literal gloss
// (sourced from the word's definitions note) so the feedback shows the origin
// breakdown + literal like every other vocabulary card's feedback.
func TestLoadRelearnPool_EtymologyCardCarriesOriginDetails(t *testing.T) {
	svc := etymologyRelearnFixture(t, "describe")

	pool, err := svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
	require.NoError(t, err)

	var describe *RelearnCard
	for i := range pool {
		if pool[i].Format == notebook.QuizTypeEtymologyOrigin && pool[i].Entry == "describe" {
			describe = &pool[i]
		}
	}
	require.NotNil(t, describe, "the missed origin-bearing word must be pooled as a family card")

	require.Len(t, describe.WordDetail.OriginParts, 1)
	assert.Equal(t, "scribo", describe.WordDetail.OriginParts[0].Origin)
	assert.Equal(t, "to write", describe.WordDetail.OriginParts[0].Meaning)
	// The literal gloss flows from the word's definitions note field.
	assert.Equal(t, `de "down" + scribo "write" = "write down"`, describe.Literal)
}

// TestLoadRelearnPool_EtymologyCardCarriesEnglishForms pins that an origin-family
// card carries the origin's english_forms (the English combining-form spellings)
// resolved from the full EtymologyOrigin definition — the word's own origin_parts
// do not carry them, so the relearn builder must look them up via the origin map.
func TestLoadRelearnPool_EtymologyCardCarriesEnglishForms(t *testing.T) {
	svc := etymologyRelearnFixture(t, "describe")

	pool, err := svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
	require.NoError(t, err)

	got := etymRelearnByEntry(pool)
	require.Contains(t, got, "describe")
	assert.Equal(t, []string{"scrib", "script"}, got["describe"].EnglishForms,
		"the origin's english_forms must be threaded onto the relearn family card")
}
