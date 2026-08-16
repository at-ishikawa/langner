package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference/mock"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
)

// newRelearnTestHandler builds a QuizHandler over a flashcard notebook whose
// learning history has words in several wrong/correct/old states so the pool
// selection can be exercised. Returns the handler and the learning-notes dir
// (for the "writes nothing" assertion). Uses the substring mock grader: any
// answer not starting with "wrong" is graded correct.
func newRelearnTestHandler(t *testing.T) (*QuizHandler, string) {
	t.Helper()
	flashcardsDir := t.TempDir()
	learningDir := t.TempDir()

	vocabDir := filepath.Join(flashcardsDir, "test-vocab")
	require.NoError(t, os.MkdirAll(vocabDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "index.yml"), []byte(`id: test-vocab
name: Test Vocabulary
notebooks:
  - ./cards.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "cards.yml"), []byte(`- title: "Basic Words"
  date: 2025-01-15T00:00:00Z
  cards:
    - expression: "alpha"
      meaning: "the first thing"
      examples:
        - "Alpha comes before beta."
    - expression: "beta"
      meaning: "the second thing"
    - expression: "gamma"
      meaning: "the third thing"
    - expression: "delta"
      meaning: "a change or difference"
      examples:
        - "The delta between the two readings was small."
    - expression: "epsilon"
      meaning: "the fifth thing"
`), 0644))

	now := time.Now()
	recent := now.Add(-2 * time.Hour).Format(time.RFC3339)        // in a 24h window, out of a 1h window
	veryRecent := now.Add(-30 * time.Minute).Format(time.RFC3339) // in both the 24h and 1h windows
	old := now.Add(-48 * time.Hour).Format(time.RFC3339)          // out of a 24h window

	// alpha: recently wrong (in pool). beta: recently correct (excluded).
	// gamma: wrong but too old (excluded). delta: recently wrong in REVERSE.
	history := fmt.Sprintf(`- metadata:
    notebook_id: test-vocab
    title: "Basic Words"
    type: "flashcard"
  expressions:
    - expression: "alpha"
      learned_logs:
        - status: "misunderstood"
          learned_at: %q
          quiz_type: "notebook"
    - expression: "beta"
      learned_logs:
        - status: "understood"
          learned_at: %q
          quiz_type: "notebook"
    - expression: "gamma"
      learned_logs:
        - status: "misunderstood"
          learned_at: %q
          quiz_type: "notebook"
    - expression: "delta"
      reverse_logs:
        - status: "misunderstood"
          learned_at: %q
          quiz_type: "reverse"
    - expression: "epsilon"
      learned_logs:
        - status: "misunderstood"
          learned_at: %q
          quiz_type: "notebook"
      reverse_logs:
        - status: "misunderstood"
          learned_at: %q
          quiz_type: "reverse"
`, recent, recent, old, veryRecent, recent, recent)
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-vocab.yml"), []byte(history), 0644))

	svc := quiz.NewService(config.NotebooksConfig{
		FlashcardsDirectories:  []string{flashcardsDir},
		LearningNotesDirectory: learningDir,
	}, mock.NewClient(), make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	return NewQuizHandler(svc), learningDir
}

func relearnEntries(cards []*apiv1.RelearnCard) map[string]*apiv1.RelearnCard {
	out := make(map[string]*apiv1.RelearnCard, len(cards))
	for _, c := range cards {
		out[c.GetEntry()] = c
	}
	return out
}

// newExampleRelearnHandler builds a QuizHandler over the repo's examples/ tree
// (the same directories config.example.yml wires), with the learning-notes dir
// redirected to a caller-owned temp dir. It uses the substring mock grader
// (mock.NewClient) — any answer NOT starting with "wrong" is graded CORRECT —
// which is exactly what makes the empty-answer test meaningful: a blank ("")
// would be graded correct if it reached the grader, so the test fails unless the
// empty→incorrect short-circuit is in place.
func newExampleRelearnHandler(t *testing.T, learningDir string) *QuizHandler {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	root := ""
	for dir != "/" {
		if _, statErr := os.Stat(filepath.Join(dir, "config.example.yml")); statErr == nil {
			root = dir
			break
		}
		dir = filepath.Dir(dir)
	}
	if root == "" {
		t.Skip("repo root (config.example.yml) not found")
	}
	ex := filepath.Join(root, "examples")
	svc := quiz.NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{filepath.Join(ex, "stories")},
		JournalsDirectories:    []string{filepath.Join(ex, "journals")},
		FlashcardsDirectories:  []string{filepath.Join(ex, "flashcards")},
		BooksDirectories:       []string{filepath.Join(ex, "books")},
		DefinitionsDirectories: []string{filepath.Join(ex, "definitions")},
		EtymologyDirectories:   []string{filepath.Join(ex, "etymology")},
		GrammarsDirectories:    []string{filepath.Join(ex, "grammars")},
		LearningNotesDirectory: learningDir,
	}, mock.NewClient(), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil),
		config.QuizConfig{Algorithm: "modified_sm2", FixedIntervals: []int{1, 7, 30, 90, 365, 1095, 1825}, DisableShuffle: true})
	return NewQuizHandler(svc)
}

// TestRelearn_OriginCardEmptyAnswerIsIncorrect pins the reported bug fix through
// the exact frontend "See answers" path: RelearnOriginPost sends an EMPTY answer
// for an un-typed word via SubmitRelearnAnswer, and it must grade INCORRECT — the
// "unanswered → incorrect" contract (quiz-ui-invariants U1). It drives the real
// example config + handler: a recognition miss of an origin-bearing word
// (deficient → facere) is recorded through the real quiz path, so the Relearn
// pool emits its origin family card; then an empty SubmitRelearnAnswer against
// that card is asserted wrong. With the substring mock grader an empty answer
// would be graded CORRECT if it reached the grader, so this fails without the
// empty→incorrect short-circuit in GradeNotebookAnswer.
func TestRelearn_OriginCardEmptyAnswerIsIncorrect(t *testing.T) {
	ctx := context.Background()
	learningDir := t.TempDir()
	h := newExampleRelearnHandler(t, learningDir)

	// Record a recognition miss of deficient through the real service path so it
	// surfaces as an origin family card in the pool.
	svc := h.svc
	cards, err := svc.LoadCards([]string{"roots-demo"}, true, nil)
	require.NoError(t, err)
	missed := false
	for i := range cards {
		if cards[i].Entry == "deficient" {
			require.NoError(t, svc.SaveResult(ctx, cards[i], quiz.GradeResult{Correct: false, Quality: 1}, 1000))
			missed = true
		}
	}
	require.True(t, missed, "standard quiz must serve deficient")

	pool := startRelearn(t, h, 24)
	var card *apiv1.RelearnCard
	for _, c := range pool {
		if c.GetEntry() == "deficient" {
			card = c
		}
	}
	require.NotNil(t, card, "the recognition-missed origin word must be in the pool")
	require.Equal(t, apiv1.QuizType_QUIZ_TYPE_ETYMOLOGY_ORIGIN, card.GetSourceQuizType(),
		"deficient folds into its origin family card")

	// "See answers" for the un-typed word → empty answer, is_skipped=false.
	resp, err := h.SubmitRelearnAnswer(ctx, connect.NewRequest(&apiv1.SubmitRelearnAnswerRequest{
		NoteId: card.GetNoteId(),
		Answer: "",
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.GetCorrect(),
		"an unanswered origin-card word must grade INCORRECT, not correct")
}

func startRelearn(t *testing.T, h *QuizHandler, windowHours int32) []*apiv1.RelearnCard {
	t.Helper()
	resp, err := h.StartRelearnQuiz(context.Background(),
		connect.NewRequest(&apiv1.StartRelearnQuizRequest{WindowHours: windowHours}))
	require.NoError(t, err)
	return resp.Msg.GetCards()
}

// relearnByEntryType keys cards by "entry/source_quiz_type" so a word failed in
// more than one quiz type (and thus present as several cards) stays distinct.
func relearnByEntryType(cards []*apiv1.RelearnCard) map[string]*apiv1.RelearnCard {
	out := make(map[string]*apiv1.RelearnCard, len(cards))
	for _, c := range cards {
		out[c.GetEntry()+"/"+c.GetSourceQuizType().String()] = c
	}
	return out
}

func TestRelearn_MirrorsEachSourceQuizType(t *testing.T) {
	h, _ := newRelearnTestHandler(t)
	byKey := relearnByEntryType(startRelearn(t, h, 24))

	// alpha was failed in the notebook (recognition) quiz.
	alpha := byKey["alpha/QUIZ_TYPE_STANDARD"]
	require.NotNil(t, alpha)
	assert.NotEmpty(t, alpha.GetExamples(), "recognition cards carry examples as a hint")

	// delta was failed in the reverse quiz: it must carry the meaning as the
	// prompt and masked contexts as the hint (not examples).
	delta := byKey["delta/QUIZ_TYPE_REVERSE"]
	require.NotNil(t, delta)
	assert.Equal(t, "a change or difference", delta.GetMeaning(), "reverse card prompts with the meaning")
}

// TestRelearn_VocabResponseHasNoLiteral pins that the etymology literal gloss
// is etymology-only: a plain vocabulary relearn card's response carries no
// literal (buildRelearnResponse sets it only inside the IsEtymology branch).
func TestRelearn_VocabResponseHasNoLiteral(t *testing.T) {
	h, _ := newRelearnTestHandler(t)
	id := relearnByEntryType(startRelearn(t, h, 24))["alpha/QUIZ_TYPE_STANDARD"].GetNoteId()
	resp, err := h.SubmitRelearnAnswer(context.Background(),
		connect.NewRequest(&apiv1.SubmitRelearnAnswerRequest{NoteId: id, Answer: "the first thing"}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.GetLiteral(), "a vocab relearn response carries no etymology literal")
}

// TestRelearn_WordFailedInTwoTypesYieldsOneCard pins the re-ask dedup
// (quiz-scheduling bug 1): a word missed in BOTH the standard and the reverse
// quiz is ONE failed word, so the end-of-session re-ask round holds it ONCE —
// drilled in reverse (producing the word is the stronger recall test, the same
// both-directions rule the origin family card uses). Before the dedup epsilon
// surfaced a recognition card AND a reverse card, inflating the round (K failed
// words could yield up to 2K cards).
func TestRelearn_WordFailedInTwoTypesYieldsOneCard(t *testing.T) {
	h, _ := newRelearnTestHandler(t)
	cards := startRelearn(t, h, 24)

	var epsilon []*apiv1.RelearnCard
	for _, c := range cards {
		if c.GetEntry() == "epsilon" {
			epsilon = append(epsilon, c)
		}
	}
	require.Len(t, epsilon, 1, "a word failed in two types is re-asked once (deduped by expression)")
	assert.Equal(t, apiv1.QuizType_QUIZ_TYPE_REVERSE, epsilon[0].GetSourceQuizType(),
		"a both-directions miss re-drills in reverse, once")
}

func TestRelearn_ReverseCardIsGradedByTheWordNotTheMeaning(t *testing.T) {
	// The mock reverse grader marks correct only when the answer matches the
	// expected WORD (same_word). Typing the meaning must be wrong — this is the
	// bug the mirror rework fixes. Each answer uses a fresh handler so the two
	// submissions stay independent.
	submitDelta := func(ans string) bool {
		h, _ := newRelearnTestHandler(t)
		id := relearnByEntryType(startRelearn(t, h, 24))["delta/QUIZ_TYPE_REVERSE"].GetNoteId()
		resp, err := h.SubmitRelearnAnswer(context.Background(),
			connect.NewRequest(&apiv1.SubmitRelearnAnswerRequest{NoteId: id, Answer: ans}))
		require.NoError(t, err)
		return resp.Msg.GetCorrect()
	}
	assert.True(t, submitDelta("delta"), "typing the WORD is correct in a reverse card")
	assert.False(t, submitDelta("a change or difference"), "typing the MEANING is wrong in a reverse card")
}

// TestRelearn_NoteIDsStableAcrossRestarts guards the card-store desync fix:
// note_id is a stable hash of the card, so two StartRelearn calls hand the
// same card the same id. The previous code assigned sequential ids in random
// map-iteration order, so a re-start silently repointed a note_id at a
// different card.
func TestRelearn_NoteIDsStableAcrossRestarts(t *testing.T) {
	h, _ := newRelearnTestHandler(t)

	first := relearnByEntryType(startRelearn(t, h, 24))
	second := relearnByEntryType(startRelearn(t, h, 24))

	require.NotEmpty(t, first)
	require.Len(t, second, len(first))
	for key, c1 := range first {
		c2, ok := second[key]
		require.True(t, ok, "card %q must appear in both starts", key)
		assert.Equal(t, c1.GetNoteId(), c2.GetNoteId(),
			"note_id for %q must be identical across StartRelearn calls", key)
	}
}

// TestRelearn_HeldNoteIDGradesTheCardItWasShownFor reproduces the reported bug:
// a learner starts Relearn, then a second StartRelearn happens within the
// window (another tab, or re-entering the session). Grading a note_id the
// client is still holding must resolve to the SAME card it was shown — the
// meaning returned matches the prompt and a correct answer is graded correct —
// instead of whatever card a reassigned sequential id happened to land on.
func TestRelearn_HeldNoteIDGradesTheCardItWasShownFor(t *testing.T) {
	h, _ := newRelearnTestHandler(t)

	held := relearnByEntryType(startRelearn(t, h, 24))["alpha/QUIZ_TYPE_STANDARD"]
	require.NotNil(t, held)

	// A second start replaces/extends the server store (the desync trigger).
	_ = startRelearn(t, h, 24)

	resp, err := h.SubmitRelearnAnswer(context.Background(),
		connect.NewRequest(&apiv1.SubmitRelearnAnswerRequest{NoteId: held.GetNoteId(), Answer: "the first thing"}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.GetCorrect(), "the held note_id still grades the card it was shown for")
	assert.Equal(t, held.GetMeaning(), resp.Msg.GetMeaning(),
		"the graded card's meaning matches the one the learner saw — no cross-card desync")
}

func TestRelearn_PoolSelectsRecentWrongWordsAcrossTypes(t *testing.T) {
	h, _ := newRelearnTestHandler(t)

	cards := startRelearn(t, h, 24)
	byEntry := relearnEntries(cards)

	assert.Contains(t, byEntry, "alpha", "recently-wrong word must be in the pool")
	assert.Contains(t, byEntry, "delta", "recently-wrong reverse word must be in the pool")
	assert.NotContains(t, byEntry, "beta", "recently-correct word must be excluded")
	assert.NotContains(t, byEntry, "gamma", "wrong-but-old word must be excluded (outside window)")

	assert.Equal(t, apiv1.QuizType_QUIZ_TYPE_STANDARD, byEntry["alpha"].GetSourceQuizType())
	assert.Equal(t, apiv1.QuizType_QUIZ_TYPE_REVERSE, byEntry["delta"].GetSourceQuizType())
}

func TestRelearn_WindowNarrowsPool(t *testing.T) {
	h, _ := newRelearnTestHandler(t)

	// A 1-hour window drops alpha (2h ago) but keeps delta (1h ago).
	byEntry := relearnEntries(startRelearn(t, h, 1))
	assert.NotContains(t, byEntry, "alpha")
	assert.Contains(t, byEntry, "delta")
}

func TestRelearn_ZeroWindowUsesDefault(t *testing.T) {
	h, _ := newRelearnTestHandler(t)

	zero := relearnEntries(startRelearn(t, h, 0))
	def := relearnEntries(startRelearn(t, h, 24))
	assert.Equal(t, len(def), len(zero), "window_hours=0 must behave like the 24h default")
	assert.Contains(t, zero, "alpha")
}

func TestRelearn_CorrectAnswerWritesNoHistoryAndWordIsRepeatable(t *testing.T) {
	h, learningDir := newRelearnTestHandler(t)
	historyPath := filepath.Join(learningDir, "test-vocab.yml")
	before, err := os.ReadFile(historyPath)
	require.NoError(t, err)

	cards := startRelearn(t, h, 24)
	byEntry := relearnEntries(cards)
	alpha := byEntry["alpha"]
	require.NotNil(t, alpha)

	resp, err := h.SubmitRelearnAnswer(context.Background(),
		connect.NewRequest(&apiv1.SubmitRelearnAnswerRequest{
			NoteId: alpha.GetNoteId(),
			Answer: "the first thing",
		}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.GetCorrect())
	assert.Equal(t, "the first thing", resp.Msg.GetMeaning())

	// The no-write guarantee: the learning-history YAML is byte-identical.
	after, err := os.ReadFile(historyPath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after),
		"a relearn answer must not write any learning history")

	// Relearn is repeatable: a correct answer persists no state, so the word is
	// still in the pool next session (it ages out of the window or is fixed in a
	// real quiz — not here).
	next := relearnEntries(startRelearn(t, h, 24))
	assert.Contains(t, next, "alpha", "a relearned word must reappear — relearn stores no clear state")
	assert.Contains(t, next, "delta")
}

func TestRelearn_WrongAndSkippedKeepWordInPool(t *testing.T) {
	h, _ := newRelearnTestHandler(t)

	first := relearnEntries(startRelearn(t, h, 24))
	require.Contains(t, first, "alpha")
	require.Contains(t, first, "delta")

	// Wrong answer for alpha.
	wrongResp, err := h.SubmitRelearnAnswer(context.Background(),
		connect.NewRequest(&apiv1.SubmitRelearnAnswerRequest{NoteId: first["alpha"].GetNoteId(), Answer: "wrong guess"}))
	require.NoError(t, err)
	assert.False(t, wrongResp.Msg.GetCorrect())

	// Skip delta.
	skipResp, err := h.SubmitRelearnAnswer(context.Background(),
		connect.NewRequest(&apiv1.SubmitRelearnAnswerRequest{NoteId: first["delta"].GetNoteId(), IsSkipped: true}))
	require.NoError(t, err)
	assert.False(t, skipResp.Msg.GetCorrect())

	// Relearn persists nothing, so both still appear next session.
	next := relearnEntries(startRelearn(t, h, 24))
	assert.Contains(t, next, "alpha", "a wrong answer leaves the word in the pool")
	assert.Contains(t, next, "delta", "a skip leaves the word in the pool")
}

func TestRelearn_BatchSubmit(t *testing.T) {
	h, _ := newRelearnTestHandler(t)
	byEntry := relearnEntries(startRelearn(t, h, 24))

	resp, err := h.BatchSubmitRelearnAnswers(context.Background(),
		connect.NewRequest(&apiv1.BatchSubmitRelearnAnswersRequest{
			Answers: []*apiv1.SubmitRelearnAnswerRequest{
				{NoteId: byEntry["alpha"].GetNoteId(), Answer: "the first thing"},
				{NoteId: byEntry["delta"].GetNoteId(), Answer: "wrong"},
			},
		}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetResponses(), 2)
	assert.True(t, resp.Msg.GetResponses()[0].GetCorrect())
	assert.False(t, resp.Msg.GetResponses()[1].GetCorrect())

	// A batch persists nothing either: both words remain in the pool.
	next := relearnEntries(startRelearn(t, h, 24))
	assert.Contains(t, next, "alpha")
	assert.Contains(t, next, "delta")
}

func TestRelearn_SubmitUnknownCardIsNotFound(t *testing.T) {
	h, _ := newRelearnTestHandler(t)
	_ = startRelearn(t, h, 24)
	_, err := h.SubmitRelearnAnswer(context.Background(),
		connect.NewRequest(&apiv1.SubmitRelearnAnswerRequest{NoteId: 99999, Answer: "x"}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

// TestRelearn_GrammarCardEndToEnd is the full-stack pin for Part A: a missed
// grammar correction reaches StartRelearnQuiz as a QUIZ_TYPE_GRAMMAR card
// carrying the entry's content and the mistaken span (the live grammar
// quiz's own inline-correction display data — no plain-text fallback), and
// SubmitRelearnAnswer grades it with the same GradeGrammarBlank the live
// grammar quiz uses, surfacing the reference fix + category on the response.
func TestRelearn_GrammarCardEndToEnd(t *testing.T) {
	h, _ := writeGrammarRelearnFixture(t)

	cards := startRelearn(t, h, 24)
	require.Len(t, cards, 1)
	card := cards[0]
	assert.Equal(t, apiv1.QuizType_QUIZ_TYPE_GRAMMAR, card.GetSourceQuizType())
	assert.Contains(t, card.GetContent(), "the John called me",
		"the card carries the whole entry, like the live grammar quiz — not a degraded plain-text fallback")
	assert.Equal(t, "the John", card.GetIncorrect())

	resp, err := h.SubmitRelearnAnswer(context.Background(), connect.NewRequest(&apiv1.SubmitRelearnAnswerRequest{
		NoteId: card.GetNoteId(),
		Answer: "John",
	}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.GetCorrect())
	assert.Equal(t, "John", resp.Msg.GetCorrectAnswer())
	assert.Equal(t, "article", resp.Msg.GetCategory())
}

// TestRelearnCardID_GrammarSpanDiscriminatesBlanks pins the note_id half of the
// grammar-relearn fix: two DISTINCT grammar corrections that share an Entry (the
// senseID) and an empty Meaning — which collided to ONE note_id before the fix —
// must now get DISTINCT note_ids because the mistaken span is folded in. A vocab
// pair with the same (Entry, Meaning) MUST still collide, so identical vocab
// words keep folding to one card (the discriminator is grammar-scoped).
func TestRelearnCardID_GrammarSpanDiscriminatesBlanks(t *testing.T) {
	grammarBlank := func(senseID, incorrect string) quiz.RelearnCard {
		// Entry == senseID and Meaning == "" for grammar cards (see relearn.go);
		// the mistaken span is what must break the tie.
		return quiz.RelearnCard{
			Format: notebook.QuizTypeGrammar, NotebookName: "journal",
			Entry: senseID, Content: "the post", Incorrect: incorrect,
		}
	}
	a := grammarBlank("dup-id", "the John")
	b := grammarBlank("dup-id", "go")
	assert.NotEqual(t, relearnCardID(a), relearnCardID(b),
		"two corrections sharing a senseID must get distinct note_ids via their span")

	// Stability: the same blank hashes the same across calls, so a client's held
	// id still resolves after another StartRelearn (the merge-store contract).
	assert.Equal(t, relearnCardID(a), relearnCardID(grammarBlank("dup-id", "the John")),
		"the same blank must hash to a stable note_id")

	// Vocab folding is unchanged: identical (Entry, Meaning) still collapses.
	v1 := quiz.RelearnCard{Format: notebook.QuizTypeNotebook, NotebookName: "vocab", Entry: "bank", Meaning: "a riverbank"}
	v2 := quiz.RelearnCard{Format: notebook.QuizTypeNotebook, NotebookName: "vocab", Entry: "bank", Meaning: "a riverbank"}
	assert.Equal(t, relearnCardID(v1), relearnCardID(v2),
		"vocab de-duplication must be unaffected: identical words fold to one card")
}

// TestRelearn_GrammarTwoBlanksIndependent is the full-stack pin: two distinct
// corrections in one post each keep their OWN learning-log series, reach
// StartRelearnQuiz as TWO blanks of one post, get DISTINCT note_ids, and grade
// INDEPENDENTLY — grading one leaves the other in the store as its own
// unaffected card (the frontend keys per-blank state by note_id).
//
// Two corrections can no longer share a senseID: ensureUniqueCorrectionIDs makes
// every correction's id unique at load time, so even a reused explicit `id:`
// resolves to two series (see internal/notebook grammars_test.go and
// internal/quiz TestService_GrammarQuiz_CollidingIDsStayIndependent). The
// note_id-level belt-and-suspenders (relearnCardID folds the mistaken span) is
// pinned separately by TestRelearnCardID_GrammarSpanDiscriminatesBlanks.
func TestRelearn_GrammarTwoBlanksIndependent(t *testing.T) {
	h := writeGrammarTwoBlanksFixture(t)

	cards := startRelearn(t, h, 24)
	require.Len(t, cards, 2, "both blanks of the post reach the client")
	assert.NotEqual(t, cards[0].GetNoteId(), cards[1].GetNoteId(),
		"the two blanks must carry distinct note_ids so their state never collapses")
	assert.Equal(t, cards[0].GetContent(), cards[1].GetContent(), "both blanks share the full post text")

	byIncorrect := map[string]*apiv1.RelearnCard{}
	for _, c := range cards {
		byIncorrect[c.GetIncorrect()] = c
	}
	require.Contains(t, byIncorrect, "the John")
	require.Contains(t, byIncorrect, "go")

	// Grade the first blank correct; the second must still resolve and grade on
	// its OWN reference — not be marked correct or dropped by the first answer.
	resp, err := h.SubmitRelearnAnswer(context.Background(), connect.NewRequest(&apiv1.SubmitRelearnAnswerRequest{
		NoteId: byIncorrect["the John"].GetNoteId(),
		Answer: "John",
	}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.GetCorrect())

	resp2, err := h.SubmitRelearnAnswer(context.Background(), connect.NewRequest(&apiv1.SubmitRelearnAnswerRequest{
		NoteId: byIncorrect["go"].GetNoteId(),
		Answer: "wrong",
	}))
	require.NoError(t, err)
	assert.False(t, resp2.Msg.GetCorrect(), "the second blank grades on its own reference, independent of the first")
	assert.Equal(t, "went", resp2.Msg.GetCorrectAnswer(), "the second blank keeps its own correction")
}

// writeGrammarTwoBlanksFixture writes a post whose single scene has two distinct
// corrections, each its own misunderstood series in-window, so both are due in
// the Relearn pool as separate blanks of one post.
func writeGrammarTwoBlanksFixture(t *testing.T) *QuizHandler {
	t.Helper()
	storiesDir := t.TempDir()
	grammarsDir := t.TempDir()
	learningDir := t.TempDir()

	storyDir := filepath.Join(storiesDir, "journal")
	require.NoError(t, os.MkdirAll(storyDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(
		"id: journal\nname: \"English Journal\"\nnotebooks:\n  - ./posts.yml\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "posts.yml"), []byte(
		"- event: \"Note 1\"\n  scenes:\n    - scene: \"\"\n      statements:\n        - \"Yesterday the John called me and then I go home.\"\n"), 0644))

	grammarNotebookDir := filepath.Join(grammarsDir, "journal")
	require.NoError(t, os.MkdirAll(grammarNotebookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(grammarNotebookDir, "index.yml"), []byte(
		"id: journal\nnotebooks:\n  - ./corr.yml\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(grammarNotebookDir, "corr.yml"), []byte(
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
`), 0644))

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
`, recent, recent)), 0644))

	svc := quiz.NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{storiesDir},
		GrammarsDirectories:    []string{grammarsDir},
		LearningNotesDirectory: learningDir,
	}, mock.NewClient(), make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})
	return NewQuizHandler(svc)
}

// writeGrammarRelearnFixture writes the minimal story + grammar + learning
// files for a single due grammar correction ("the John" → "John", status
// misunderstood in-window) and returns a handler over them plus the learning
// history path, so a test can drive Relearn against real YAML.
func writeGrammarRelearnFixture(t *testing.T) (*QuizHandler, string) {
	t.Helper()
	storiesDir := t.TempDir()
	grammarsDir := t.TempDir()
	learningDir := t.TempDir()

	storyDir := filepath.Join(storiesDir, "journal")
	require.NoError(t, os.MkdirAll(storyDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(
		"id: journal\nname: \"English Journal\"\nnotebooks:\n  - ./posts.yml\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "posts.yml"), []byte(
		"- event: \"Note 1\"\n  scenes:\n    - scene: \"\"\n      statements:\n        - \"Yesterday the John called me.\"\n"), 0644))

	grammarNotebookDir := filepath.Join(grammarsDir, "journal")
	require.NoError(t, os.MkdirAll(grammarNotebookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(grammarNotebookDir, "index.yml"), []byte(
		"id: journal\nnotebooks:\n  - ./corr.yml\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(grammarNotebookDir, "corr.yml"), []byte(
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
`), 0644))

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
`, recent)), 0644))

	svc := quiz.NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{storiesDir},
		GrammarsDirectories:    []string{grammarsDir},
		LearningNotesDirectory: learningDir,
	}, mock.NewClient(), make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})
	return NewQuizHandler(svc), learningDir
}

// TestRelearn_GrammarUnansweredIsIncorrectNotExcluded pins the final model: a
// grammar blank the learner never answered (revealed via "See answers", sent as
// an empty answer) is graded INCORRECT — a normal miss, never a skip and never
// an exclude. It must NOT write the exclude-from-quizzes marker (skipped_at)
// and the correction must stay due — reappearing in the next Relearn session.
// See .claude/rules/quiz-ui-invariants.md.
func TestRelearn_GrammarUnansweredIsIncorrectNotExcluded(t *testing.T) {
	h, learningDir := writeGrammarRelearnFixture(t)
	historyPath := filepath.Join(learningDir, "journal.yml")
	before, err := os.ReadFile(historyPath)
	require.NoError(t, err)

	cards := startRelearn(t, h, 24)
	require.Len(t, cards, 1, "the missed grammar correction is in the pool")
	card := cards[0]
	require.Equal(t, apiv1.QuizType_QUIZ_TYPE_GRAMMAR, card.GetSourceQuizType())

	// "See answers" for an unanswered blank sends an empty answer (is_skipped is
	// never set in grammar relearn) — it is graded incorrect deterministically.
	resp, err := h.SubmitRelearnAnswer(context.Background(), connect.NewRequest(&apiv1.SubmitRelearnAnswerRequest{
		NoteId: card.GetNoteId(),
		Answer: "",
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.GetCorrect(), "an unanswered blank grades as incorrect")

	// Real-state check 1: the learning-history YAML is byte-identical — no
	// skipped_at written, no log appended. A relearn answer persists nothing.
	after, err := os.ReadFile(historyPath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after),
		"an incorrect answer must not write learning history — and never the skipped_at exclude marker")
	assert.NotContains(t, string(after), "skipped_at",
		"an incorrect/unanswered blank must NOT be excluded from quizzes")

	// Real-state check 2: reload the pool — the correction is still due.
	next := startRelearn(t, h, 24)
	require.Len(t, next, 1, "an incorrect correction stays in the Relearn pool for a future session")
	assert.Equal(t, apiv1.QuizType_QUIZ_TYPE_GRAMMAR, next[0].GetSourceQuizType())
}

// TestRelearn_GrammarExcludeSetsSkippedAtAndRemovesFromPool pins the deliberate
// Exclude action: the per-blank Exclude button calls the same SkipWord RPC every
// other card uses. Excluding a grammar correction MUST write the skipped_at
// exclude marker on its (notebook, senseID) learning-history slot and remove it
// from the Relearn pool on reload — the opposite of an incorrect answer.
func TestRelearn_GrammarExcludeSetsSkippedAtAndRemovesFromPool(t *testing.T) {
	h, learningDir := writeGrammarRelearnFixture(t)
	historyPath := filepath.Join(learningDir, "journal.yml")

	cards := startRelearn(t, h, 24)
	require.Len(t, cards, 1, "the missed grammar correction is in the pool")
	card := cards[0]
	require.Equal(t, apiv1.QuizType_QUIZ_TYPE_GRAMMAR, card.GetSourceQuizType())

	// Exclude this blank: the same SkipWord RPC the vocab/etymology cards use,
	// resolved from the relearn store to the grammar correction's senseID.
	_, err := h.SkipWord(context.Background(), connect.NewRequest(&apiv1.SkipWordRequest{
		NoteId:    card.GetNoteId(),
		QuizTypes: []apiv1.QuizType{apiv1.QuizType_QUIZ_TYPE_GRAMMAR},
	}))
	require.NoError(t, err)

	// Real-state check 1: skipped_at IS now written for this correction.
	after, err := os.ReadFile(historyPath)
	require.NoError(t, err)
	assert.Contains(t, string(after), "skipped_at",
		"Exclude must write the skipped_at exclude marker")

	// Real-state check 2: reload the pool — the correction is gone.
	next := startRelearn(t, h, 24)
	assert.Empty(t, next, "an excluded correction must not appear in the Relearn pool")
}

// newRelearnOriginTestHandler builds a QuizHandler over a flashcard notebook
// whose words carry an etymology origin (resolved against an etymology book),
// each with an example sentence. The learning history controls, per word, the
// direction(s) it was missed in: `recognitionMissed` words have a misunderstood
// recognition (notebook) log, `reverseMissed` words a misunderstood reverse log.
// A word may be in both. This is the on-disk shape LoadRelearnPool folds into
// origin family cards, now for reverse misses too.
func newRelearnOriginTestHandler(t *testing.T, recognitionMissed, reverseMissed []string) *QuizHandler {
	t.Helper()
	flashcardsDir := t.TempDir()
	etymDir := t.TempDir()
	learningDir := t.TempDir()

	// Etymology book defining the shared origin, so the words' origin_parts
	// resolve and each missed word groups under it.
	etymBook := filepath.Join(etymDir, "roots")
	require.NoError(t, os.MkdirAll(etymBook, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(etymBook, "index.yml"), []byte(
		"id: roots\nkind: Etymology\nname: Roots\nnotebooks:\n  - ./s1.yml\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(etymBook, "s1.yml"), []byte(`metadata:
  title: "Session 1"
origins:
  - origin: "stare"
    type: root
    language: Latin
    meaning: to stand
    english_forms: [st, sta]
`), 0644))

	// Three flashcard words that all derive from "stare", each with one example
	// sentence so the feedback context scenes are non-empty.
	vocabDir := filepath.Join(flashcardsDir, "roots")
	require.NoError(t, os.MkdirAll(vocabDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "index.yml"), []byte(
		"id: roots\nname: Roots\nnotebooks:\n  - ./cards.yml\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "cards.yml"), []byte(`- title: "Roots"
  date: 2025-01-15T00:00:00Z
  cards:
    - expression: "constant"
      meaning: "steadfast and unchanging"
      examples:
        - "She stayed constant through every hardship."
      origin_parts:
        - origin: stare
    - expression: "obstinate"
      meaning: "stubbornly firm and unyielding"
      examples:
        - "He was obstinate and refused to move."
      origin_parts:
        - origin: stare
    - expression: "circumstance"
      meaning: "a condition or surrounding fact"
      examples:
        - "The circumstance changed everything."
      origin_parts:
        - origin: stare
`), 0644))

	recent := time.Now().Add(-30 * time.Minute).Format(time.RFC3339)
	inSet := func(set []string, w string) bool {
		for _, s := range set {
			if s == w {
				return true
			}
		}
		return false
	}
	exprs := ""
	for _, w := range []string{"constant", "obstinate", "circumstance"} {
		if !inSet(recognitionMissed, w) && !inSet(reverseMissed, w) {
			continue
		}
		exprs += fmt.Sprintf("    - expression: %q\n", w)
		if inSet(recognitionMissed, w) {
			exprs += fmt.Sprintf(`      learned_logs:
        - status: "misunderstood"
          learned_at: %q
          quiz_type: "notebook"
`, recent)
		}
		if inSet(reverseMissed, w) {
			exprs += fmt.Sprintf(`      reverse_logs:
        - status: "misunderstood"
          learned_at: %q
          quiz_type: "reverse"
`, recent)
		}
	}
	history := "- metadata:\n    notebook_id: roots\n    title: \"Roots\"\n    type: \"flashcard\"\n  expressions:\n" + exprs
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "roots.yml"), []byte(history), 0644))

	svc := quiz.NewService(config.NotebooksConfig{
		FlashcardsDirectories:  []string{flashcardsDir},
		EtymologyDirectories:   []string{etymDir},
		LearningNotesDirectory: learningDir,
	}, mock.NewClient(), make(map[string]rapidapi.Response), learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})
	return NewQuizHandler(svc)
}

// TestRelearn_ReverseOriginMissGroupsWithDirection pins the core of this change:
// an origin-bearing word missed in REVERSE folds into the SAME origin family
// card as a recognition-missed sibling, but each keeps the direction it was
// missed in. Recognition → origin_direction STANDARD (ask the meaning); reverse
// → origin_direction REVERSE (ask the word), both under one origin header.
func TestRelearn_ReverseOriginMissGroupsWithDirection(t *testing.T) {
	h := newRelearnOriginTestHandler(t, []string{"obstinate"}, []string{"constant"})
	byEntry := relearnEntries(startRelearn(t, h, 24))

	constant := byEntry["constant"]
	obstinate := byEntry["obstinate"]
	require.NotNil(t, constant, "a reverse-missed origin word must enter the pool")
	require.NotNil(t, obstinate, "a recognition-missed origin word must enter the pool")

	// Both are etymology-origin family cards under the SAME origin.
	assert.Equal(t, apiv1.QuizType_QUIZ_TYPE_ETYMOLOGY_ORIGIN, constant.GetSourceQuizType())
	assert.Equal(t, apiv1.QuizType_QUIZ_TYPE_ETYMOLOGY_ORIGIN, obstinate.GetSourceQuizType())
	assert.Equal(t, "stare", constant.GetOriginText())
	assert.Equal(t, constant.GetOriginText(), obstinate.GetOriginText(), "both fold under one origin")
	assert.Equal(t, constant.GetOriginMeaning(), obstinate.GetOriginMeaning())

	// Direction is carried per word.
	assert.Equal(t, apiv1.QuizType_QUIZ_TYPE_REVERSE, constant.GetOriginDirection(),
		"a reverse-missed word is drilled in the reverse direction inside the family card")
	assert.Equal(t, apiv1.QuizType_QUIZ_TYPE_STANDARD, obstinate.GetOriginDirection(),
		"a recognition-missed word is drilled in the recognition direction")

	// The reverse word prompts with the meaning and hints with masked contexts;
	// the recognition word carries examples.
	assert.Equal(t, "steadfast and unchanging", constant.GetMeaning())
	assert.NotEmpty(t, constant.GetContexts(), "reverse family word carries masked contexts as its hint")
	assert.NotEmpty(t, obstinate.GetExamples(), "recognition family word carries examples as its hint")
}

// TestRelearn_ReverseOriginWordGradedByTheWord pins that a reverse-direction
// family word is graded produce-the-word (typed word vs expression), NOT by the
// recognition meaning grader. Typing the WORD is correct; typing the MEANING is
// wrong — end to end through SubmitRelearnAnswer.
func TestRelearn_ReverseOriginWordGradedByTheWord(t *testing.T) {
	submit := func(answer string) *apiv1.SubmitRelearnAnswerResponse {
		h := newRelearnOriginTestHandler(t, nil, []string{"constant"})
		id := relearnEntries(startRelearn(t, h, 24))["constant"].GetNoteId()
		resp, err := h.SubmitRelearnAnswer(context.Background(),
			connect.NewRequest(&apiv1.SubmitRelearnAnswerRequest{NoteId: id, Answer: answer}))
		require.NoError(t, err)
		return resp.Msg
	}
	assert.True(t, submit("constant").GetCorrect(), "typing the WORD is correct for a reverse family word")
	assert.False(t, submit("steadfast and unchanging").GetCorrect(),
		"typing the MEANING must be wrong for a reverse family word (not the recognition grader)")
}

// TestRelearn_ReverseOriginWordFeedbackHasExampleScenes pins the reverse-examples
// parity fix: a reverse-direction family word's feedback carries its example
// statement(s) in context_scenes, exactly like a recognition word — so the
// "Where it appears" example is shown for reverse misses too.
func TestRelearn_ReverseOriginWordFeedbackHasExampleScenes(t *testing.T) {
	h := newRelearnOriginTestHandler(t, nil, []string{"constant"})
	id := relearnEntries(startRelearn(t, h, 24))["constant"].GetNoteId()
	resp, err := h.SubmitRelearnAnswer(context.Background(),
		connect.NewRequest(&apiv1.SubmitRelearnAnswerRequest{NoteId: id, Answer: "constant"}))
	require.NoError(t, err)

	scenes := resp.Msg.GetContextScenes()
	require.NotEmpty(t, scenes, "a reverse family word's feedback must carry its example scenes")
	var statements []string
	for _, s := range scenes {
		statements = append(statements, s.GetStatements()...)
	}
	assert.Contains(t, statements, "She stayed constant through every hardship.",
		"the reverse word's example statement must appear in feedback like a recognition word's")
}

// TestRelearn_OriginMissBothDirectionsDrilledOnce pins the both-directions dedup
// choice: a word missed in BOTH recognition and reverse appears ONCE in its
// family card, drilled in the reverse direction (produce-the-word is the
// stronger recall test), never twice.
func TestRelearn_OriginMissBothDirectionsDrilledOnce(t *testing.T) {
	h := newRelearnOriginTestHandler(t, []string{"circumstance"}, []string{"circumstance"})
	cards := startRelearn(t, h, 24)

	var circ []*apiv1.RelearnCard
	for _, c := range cards {
		if c.GetEntry() == "circumstance" {
			circ = append(circ, c)
		}
	}
	require.Len(t, circ, 1, "a word missed both ways must be drilled once, not shown twice")
	assert.Equal(t, apiv1.QuizType_QUIZ_TYPE_ETYMOLOGY_ORIGIN, circ[0].GetSourceQuizType())
	assert.Equal(t, apiv1.QuizType_QUIZ_TYPE_REVERSE, circ[0].GetOriginDirection(),
		"a word missed both ways is drilled in the reverse direction")
}

// TestRelearn_ReverseStandaloneCardFeedbackHasExampleScenes pins that a reverse
// miss that stays a plain standalone card (no origin) still shows its example
// statement(s) in feedback via context_scenes — parity with recognition. The
// example flows from the word's contexts into relearnScenesFromCard for reverse
// too; only the answering-screen hint differs (masked contexts vs examples).
func TestRelearn_ReverseStandaloneCardFeedbackHasExampleScenes(t *testing.T) {
	h, _ := newRelearnTestHandler(t)
	delta := relearnByEntryType(startRelearn(t, h, 24))["delta/QUIZ_TYPE_REVERSE"]
	require.NotNil(t, delta)

	resp, err := h.SubmitRelearnAnswer(context.Background(),
		connect.NewRequest(&apiv1.SubmitRelearnAnswerRequest{NoteId: delta.GetNoteId(), Answer: "delta"}))
	require.NoError(t, err)

	var statements []string
	for _, s := range resp.Msg.GetContextScenes() {
		statements = append(statements, s.GetStatements()...)
	}
	assert.Contains(t, statements, "The delta between the two readings was small.",
		"a standalone reverse card must surface its example statement in feedback like recognition")
}
