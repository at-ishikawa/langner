package quiz_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/datasync"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
)

// These tests pin that a flashcard notebook keeps its learning history when its
// decks are regrouped under new titles. They load the example flashcard notebook
// examples/flashcards/idioms (decks "Level 1" / "Level 2") through
// config.example.yml and a live Postgres, and store an earlier version of it
// whose same cards sat in decks "Group A" / "Group B". Requires
// LANGNER_INTEGRATION_DB_URL (see setupEnsureFixture).

const idiomsID = "idioms"

// idiomsContent returns the example idioms notebook as an import or push stores
// it.
func (f ensureFixture) idiomsContent(t *testing.T) notebook.NotebookContent {
	t.Helper()
	contents, err := notebook.LoadDirectoryContent(notebook.ContentDirs{Flashcards: f.cfg.Notebooks.FlashcardsDirectories})
	require.NoError(t, err)
	for _, nc := range contents {
		if nc.ID == idiomsID {
			return nc
		}
	}
	t.Fatalf("example notebook %s not found", idiomsID)
	return notebook.NotebookContent{}
}

// earlierIdioms returns nc as it was before its decks were regrouped: the same
// cards, alternately in decks "Group A" and "Group B".
func earlierIdioms(nc notebook.NotebookContent) notebook.NotebookContent {
	var cards []notebook.Note
	fam := nc.Family(notebook.FamilyFlashcards)
	for _, f := range fam.Files {
		for _, deck := range f.Flashcards {
			cards = append(cards, deck.Cards...)
		}
	}
	decks := []notebook.FlashcardNotebook{{Title: "Group A"}, {Title: "Group B"}}
	for i, c := range cards {
		decks[i%2].Cards = append(decks[i%2].Cards, c)
	}
	files := append([]notebook.ContentFile{}, fam.Files...)
	files[0].Flashcards = decks
	out := notebook.NotebookContent{ID: nc.ID}
	out.Families = []notebook.FamilyContent{*fam}
	out.Families[0].Files = files
	return out
}

func (f ensureFixture) storeContent(t *testing.T, nc notebook.NotebookContent, withNotes bool) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, notebook.NewNotebookStore(f.db).ImportShippedBundle(ctx, "flashcard", nc.ID, "", 0, nc))
	if withNotes {
		_, err := datasync.EnsureContentNotes(ctx, f.db, nc)
		require.NoError(t, err)
	}
}

func (f ensureFixture) deckLinks(t *testing.T, notebookID string) []string {
	t.Helper()
	var out []string
	require.NoError(t, f.db.Select(&out,
		`SELECT n.entry || ' @ ' || nn."group" FROM notebook_notes nn JOIN notes n ON n.id = nn.note_id
		  WHERE nn.notebook_id = $1 ORDER BY 1`, notebookID))
	return out
}

func cardByEntry(t *testing.T, cards []quiz.Card, entry string) quiz.Card {
	t.Helper()
	for _, c := range cards {
		if c.Entry == entry {
			return c
		}
	}
	t.Fatalf("card %q not loaded", entry)
	return quiz.Card{}
}

func cardEntries(cards []quiz.Card) []string {
	out := make([]string, 0, len(cards))
	for _, c := range cards {
		out = append(out, c.Entry)
	}
	sort.Strings(out)
	return out
}

// TestFlashcardRegroupedDecks_KeepHistory: the notes were linked while the
// cards sat in "Group A" / "Group B", then the notebook was stored with decks
// "Level 1" / "Level 2" and its links were not rewritten (the state notebooks
// already in the DB are in). A card answered correctly must stay out of the
// standard quiz and an excluded card must stay excluded.
func TestFlashcardRegroupedDecks_KeepHistory(t *testing.T) {
	f := setupEnsureFixture(t)
	ctx := context.Background()
	current := f.idiomsContent(t)
	f.storeContent(t, earlierIdioms(current), true)
	f.storeContent(t, current, false)
	for _, link := range f.deckLinks(t, idiomsID) {
		require.NotContains(t, link, "Level", "precondition: links still carry the earlier deck titles")
	}

	svc := f.newService(t)
	cards, err := svc.LoadCards(f.userID, []string{idiomsID}, true, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"bite the bullet", "call it a day", "cost an arm and a leg", "once in a blue moon"}, cardEntries(cards))

	// A correct answer due again in 30 days, written through the learning repository.
	answered := cardByEntry(t, cards, "call it a day")
	require.NoError(t, f.repos.Learning.Create(ctx, &learning.LearningLog{
		UserID: f.userID, Status: "understood", LearnedAt: time.Now(), Quality: 5,
		QuizType: string(notebook.QuizTypeNotebook), IntervalDays: 30,
		SourceNotebookID: idiomsID, NotebookName: idiomsID, StoryTitle: answered.StoryTitle,
		Expression: answered.Entry, IsCorrect: true,
	}))
	// A correct answer through the quiz's own save path.
	require.NoError(t, svc.SaveResult(ctx, f.userID, cardByEntry(t, cards, "bite the bullet"), quiz.GradeResult{Correct: true, Quality: 5}, 1000))
	// Exclude from quizzes.
	require.NoError(t, svc.SkipWord(f.userID, quiz.CardInfoFromCard(cardByEntry(t, cards, "once in a blue moon")), "", []notebook.QuizType{notebook.QuizTypeNotebook}))

	cards, err = svc.LoadCards(f.userID, []string{idiomsID}, true, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"cost an arm and a leg"}, cardEntries(cards), "answered and excluded cards are not served again")

	summaries, err := svc.LoadNotebookSummaries(f.userID, true)
	require.NoError(t, err)
	for _, s := range summaries {
		if s.NotebookID == idiomsID {
			assert.Equal(t, 1, s.ReviewCount, "the due count matches the quiz")
		}
	}
}

// TestFlashcardSharedNote_OwnLogsOnly: a card in two notebooks is one note;
// each notebook still reads only the logs recorded under it.
func TestFlashcardSharedNote_OwnLogsOnly(t *testing.T) {
	f := setupEnsureFixture(t)
	ctx := context.Background()
	current := f.idiomsContent(t)
	f.storeContent(t, current, true)
	copyContent := earlierIdioms(current)
	copyContent.ID = "idioms-copy"
	f.storeContent(t, copyContent, true)

	// Answer in the notebook linked second, so a log falling back to the
	// note's first notebook would show up in the wrong one.
	svc := f.newService(t)
	cards, err := svc.LoadCards(f.userID, []string{"idioms-copy"}, true, nil)
	require.NoError(t, err)
	require.NoError(t, svc.SaveResult(ctx, f.userID, cardByEntry(t, cards, "call it a day"), quiz.GradeResult{Correct: true, Quality: 5}, 1000))

	cards, err = svc.LoadCards(f.userID, []string{"idioms-copy"}, true, nil)
	require.NoError(t, err)
	assert.NotContains(t, cardEntries(cards), "call it a day", "answered in this notebook")

	cards, err = svc.LoadCards(f.userID, []string{idiomsID}, true, nil)
	require.NoError(t, err)
	assert.Contains(t, cardEntries(cards), "call it a day", "the other notebook's log must not apply here")
}

// TestFlashcardRegroupedDecks_ReimportMovesLinks: storing the regrouped
// notebook again moves each card's link to its new deck and leaves no link to
// a removed deck, while the cards' logs and exclusions stay.
func TestFlashcardRegroupedDecks_ReimportMovesLinks(t *testing.T) {
	f := setupEnsureFixture(t)
	ctx := context.Background()
	current := f.idiomsContent(t)
	f.storeContent(t, earlierIdioms(current), true)
	require.Equal(t, []string{
		"bite the bullet @ Group B", "call it a day @ Group A", "cost an arm and a leg @ Group B", "once in a blue moon @ Group A",
	}, f.deckLinks(t, idiomsID))

	svc := f.newService(t)
	cards, err := svc.LoadCards(f.userID, []string{idiomsID}, true, nil)
	require.NoError(t, err)
	require.NoError(t, svc.SaveResult(ctx, f.userID, cardByEntry(t, cards, "call it a day"), quiz.GradeResult{Correct: true, Quality: 5}, 1000))
	require.NoError(t, svc.SkipWord(f.userID, quiz.CardInfoFromCard(cardByEntry(t, cards, "once in a blue moon")), "", []notebook.QuizType{notebook.QuizTypeNotebook}))

	f.storeContent(t, current, true)
	assert.Equal(t, []string{
		"bite the bullet @ Level 1", "call it a day @ Level 1", "cost an arm and a leg @ Level 2", "once in a blue moon @ Level 2",
	}, f.deckLinks(t, idiomsID))

	var logs int
	require.NoError(t, f.db.Get(&logs, `SELECT count(*) FROM learning_logs WHERE source_notebook_id = $1`, idiomsID))
	assert.Equal(t, 1, logs)
	assert.Equal(t, 1, f.countSkipFlags(t, idiomsID))
}
