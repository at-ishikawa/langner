package quiz

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// The same word can legitimately live in several notebooks under ONE
// globally-unique sense_id (a shared user-supplied slug), yet carry a DIFFERENT
// meaning / example / part-of-speech in each notebook. The learning-LOG series
// is correctly one-per-sense_id (invariants L1/L4), but the DISPLAYED card on
// the reverse-quiz feedback page and in Relearn must stay faithful to the
// notebook the learner was actually quizzed from. These tests pin that the
// feedback/relearn resolution scopes to the originating notebook rather than
// returning whichever notebook's card the loader emits first (the reported
// identity-collision bug).
//
// Generic public idiom + a second plain sense of the same surface words; no
// user data (repo CLAUDE.md).
const (
	sharedSenseID       = "break-the-ice"
	sharedExpression    = "break the ice"
	meaningA            = "to do or say something that makes people feel relaxed"
	exampleA            = "A friendly question can break the ice with new people."
	partOfSpeechA       = "idiom"
	meaningB            = "to crack the frozen surface of water"
	exampleB            = "The ship had to break the ice to reach the harbor."
	partOfSpeechB       = "verb"
	sharedNotebookAID   = "shared-notebook-a"
	sharedNotebookBID   = "shared-notebook-b"
	sharedNotebookAName = "Notebook A"
	sharedNotebookBName = "Notebook B"
)

// sharedSenseIDFixture builds a real Service (the SAME NewService construction
// the server uses) over two on-disk notebooks that each define the SAME word
// under the SAME sense_id but with different meaning/example/part-of-speech:
//
//   - notebook A is a FLASHCARD book,
//   - notebook B is a DEFINITIONS book.
//
// The two notebook kinds are deliberate: LoadAllWords appends flashcard words
// before definitions words, so the id-keyed (last-write-wins) vocab index used
// by Relearn resolves the bare sense_id to B — the WRONG notebook for a word
// missed in A. That makes the pre-fix Relearn behavior deterministically wrong
// (not merely order-dependent), so the regression test cannot pass by luck.
//
// When seedReverseMissInA is true, a recent reverse miss of the word is written
// to notebook A's learning history (the real YAML write shape), so the word
// enters the Relearn pool as a reverse-direction candidate owned by notebook A.
func sharedSenseIDFixture(t *testing.T, seedReverseMissInA bool) *Service {
	t.Helper()
	dir := t.TempDir()
	flashcardsDir := filepath.Join(dir, "flashcards")
	defsDir := filepath.Join(dir, "definitions")
	learningDir := filepath.Join(dir, "learning")
	require.NoError(t, os.MkdirAll(learningDir, 0755))

	// Notebook A — flashcard book, meaning/example/pos "A".
	aBook := filepath.Join(flashcardsDir, sharedNotebookAID)
	require.NoError(t, os.MkdirAll(aBook, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(aBook, "index.yml"), []byte(fmt.Sprintf(`id: %s
name: %s
notebooks:
  - ./cards.yml
`, sharedNotebookAID, sharedNotebookAName)), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(aBook, "cards.yml"), []byte(fmt.Sprintf(`- title: "Idioms"
  date: 2025-01-15T00:00:00Z
  cards:
    - expression: %q
      id: %s
      meaning: %q
      part_of_speech: %s
      examples:
        - %q
`, sharedExpression, sharedSenseID, meaningA, partOfSpeechA, exampleA)), 0644))

	// Notebook B — definitions book, SAME sense_id, meaning/example/pos "B".
	bBook := filepath.Join(defsDir, sharedNotebookBID)
	require.NoError(t, os.MkdirAll(bBook, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(bBook, "index.yml"), []byte(fmt.Sprintf(`id: %s
name: %s
notebooks:
  - ./s1.yml
`, sharedNotebookBID, sharedNotebookBName)), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(bBook, "s1.yml"), []byte(fmt.Sprintf(`- metadata:
    title: "Water"
  scenes:
    - metadata:
        index: 0
        title: "ice"
      expressions:
        - expression: %q
          id: %s
          meaning: %q
          part_of_speech: %s
          examples:
            - %q
`, sharedExpression, sharedSenseID, meaningB, partOfSpeechB, exampleB)), 0644))

	if seedReverseMissInA {
		recent := time.Now().Add(-time.Hour).Format(time.RFC3339)
		learnYAML := fmt.Sprintf(`- metadata:
    id: %s
    title: "Idioms"
    type: flashcard
  expressions:
    - expression: %q
      id: %s
      type: vocabulary
      reverse_logs:
        - status: misunderstood
          learned_at: %q
`, sharedNotebookAID, sharedExpression, sharedSenseID, recent)
		require.NoError(t, os.WriteFile(filepath.Join(learningDir, sharedNotebookAID+".yml"), []byte(learnYAML), 0644))
	}

	ctrl := gomock.NewController(t)
	return NewService(config.NotebooksConfig{
		FlashcardsDirectories:  []string{flashcardsDir},
		DefinitionsDirectories: []string{defsDir},
		LearningNotesDirectory: learningDir,
	}, inference.StaticResolver(mock_inference.NewMockClient(ctrl)), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil),
		config.QuizConfig{Algorithm: "modified_sm2", FixedIntervals: []int{1, 7, 30, 90, 365, 1095, 1825}, DisableShuffle: true})
}

// sharedSenseIDNote builds the NoteRecord the stateless submit path resolves
// from: one note (the globally-unique sense_id) linked to BOTH notebooks. This
// is identity only — the card CONTENT is rebuilt by the real loader inside
// ResolveReverseCardByNote, so the buggy content-resolution path runs for real.
// Notebook B is listed first to prove scoping wins over list order (a naive
// first-match would return B).
func sharedSenseIDNote() *notebook.NoteRecord {
	return &notebook.NoteRecord{
		SenseID: sharedSenseID,
		NotebookNotes: []notebook.NotebookNote{
			{NotebookID: sharedNotebookBID, NotebookType: "definition"},
			{NotebookID: sharedNotebookAID, NotebookType: "flashcard"},
		},
	}
}

// TestResolveReverseCardByNote_SharedSenseID_ScopesToOriginatingNotebook pins
// that the reverse-quiz feedback card resolves to the notebook the learner was
// quizzed from — not whichever notebook's card the loader lists first. Asserting
// BOTH directions (scope→A gives A, scope→B gives B) makes this a reliable
// regression: the pre-fix first-match-by-sense_id returns ONE fixed card for
// both scopes, so at least one direction is guaranteed to fail without the fix.
func TestResolveReverseCardByNote_SharedSenseID_ScopesToOriginatingNotebook(t *testing.T) {
	svc := sharedSenseIDFixture(t, false)
	note := sharedSenseIDNote()

	t.Run("scoped to A", func(t *testing.T) {
		card, found, err := svc.ResolveReverseCardByNote(0, note, sharedNotebookAID)
		require.NoError(t, err)
		require.True(t, found, "the shared word must resolve within its originating notebook")
		assert.Equal(t, sharedNotebookAID, card.NotebookName)
		assert.Equal(t, meaningA, card.Meaning, "must show notebook A's meaning, not B's")
		assert.Equal(t, partOfSpeechA, card.WordDetail.PartOfSpeech)
		// Notebook B's distinctive content must NOT leak into A's feedback card.
		assert.NotEqual(t, meaningB, card.Meaning)
		assert.NotContains(t, strings.ToLower(contextsText(card)), "harbor",
			"notebook B's example must not appear on notebook A's feedback card")
	})

	t.Run("scoped to B", func(t *testing.T) {
		card, found, err := svc.ResolveReverseCardByNote(0, note, sharedNotebookBID)
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, sharedNotebookBID, card.NotebookName)
		assert.Equal(t, meaningB, card.Meaning, "must show notebook B's meaning, not A's")
		assert.Equal(t, partOfSpeechB, card.WordDetail.PartOfSpeech)
		assert.NotEqual(t, meaningA, card.Meaning)
	})

	// Legacy clients that don't echo the notebook still resolve SOME card
	// (unchanged fallback), so the empty-origin path must not regress to a miss.
	t.Run("legacy empty origin still resolves", func(t *testing.T) {
		_, found, err := svc.ResolveReverseCardByNote(0, note, "")
		require.NoError(t, err)
		assert.True(t, found)
	})
}

// TestResolveCardByNote_SharedSenseID_ScopesToOriginatingNotebook is the
// standard (recognition) quiz analogue: the feedback card after a standard
// answer must resolve to the notebook the learner was quizzed from. Same
// identity-collision, same fix (ResolveCardByNote scoped by originNotebookID).
func TestResolveCardByNote_SharedSenseID_ScopesToOriginatingNotebook(t *testing.T) {
	svc := sharedSenseIDFixture(t, false)
	note := sharedSenseIDNote()

	t.Run("scoped to A", func(t *testing.T) {
		card, found, err := svc.ResolveCardByNote(0, note, sharedNotebookAID)
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, sharedNotebookAID, card.NotebookName)
		assert.Equal(t, meaningA, card.Meaning, "must show notebook A's meaning, not B's")
		assert.Equal(t, partOfSpeechA, card.WordDetail.PartOfSpeech)
		assert.NotEqual(t, meaningB, card.Meaning)
	})

	t.Run("scoped to B", func(t *testing.T) {
		card, found, err := svc.ResolveCardByNote(0, note, sharedNotebookBID)
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, sharedNotebookBID, card.NotebookName)
		assert.Equal(t, meaningB, card.Meaning, "must show notebook B's meaning, not A's")
		assert.Equal(t, partOfSpeechB, card.WordDetail.PartOfSpeech)
		assert.NotEqual(t, meaningA, card.Meaning)
	})
}

// TestLoadRelearnPool_SharedSenseID_ResolvesMissToOriginatingNotebook pins that
// a reverse miss of a shared word folds into a Relearn card showing the meaning
// of the notebook it was missed in (A), not the other notebook that happens to
// win the bare-sense_id vocab index (B). Pre-fix, resolveWord keyed by bare id
// and returned B's content while labelling the card as notebook A — the exact
// wrong-notebook drift reported.
func TestLoadRelearnPool_SharedSenseID_ResolvesMissToOriginatingNotebook(t *testing.T) {
	svc := sharedSenseIDFixture(t, true)

	// Both notebooks contribute a card for the word (LoadAllWords does not dedup
	// plain flashcard/definitions books across notebooks), so the vocab index
	// genuinely holds two same-sense_id entries to disambiguate between.
	words, err := svc.LoadAllWords(0)
	require.NoError(t, err)
	count := 0
	for _, w := range words {
		if w.ID == sharedSenseID {
			count++
		}
	}
	require.Equal(t, 2, count, "the shared word must appear once per notebook in the vocab index")

	pool, err := svc.LoadRelearnPool(0, time.Now().Add(-24*time.Hour))
	require.NoError(t, err)

	var card *RelearnCard
	for i := range pool {
		if strings.EqualFold(pool[i].Entry, sharedExpression) {
			card = &pool[i]
		}
	}
	require.NotNil(t, card, "the missed word must be in the relearn pool")
	assert.Equal(t, sharedNotebookAID, card.NotebookName, "the miss belongs to notebook A")
	assert.Equal(t, meaningA, card.Meaning, "relearn must show notebook A's meaning, not B's")
	assert.Equal(t, meaningA, card.ReverseCard().Meaning,
		"the reverse grader card must also carry notebook A's content")
	assert.NotEqual(t, meaningB, card.Meaning, "notebook B's meaning must not drift onto the card")
}

// contextsText joins a reverse card's context sentences for substring checks.
func contextsText(c ReverseCard) string {
	var b strings.Builder
	for _, ctx := range c.Contexts {
		b.WriteString(ctx.Context)
		b.WriteString(" ")
	}
	return b.String()
}
