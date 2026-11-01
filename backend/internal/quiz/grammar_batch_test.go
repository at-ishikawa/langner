package quiz

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// catalogCounter counts content catalog reads: one per Reader built.
type catalogCounter struct {
	notebook.ContentLoader
	catalogs int
}

func (c *catalogCounter) Catalog(ctx context.Context) ([]notebook.CatalogEntry, error) {
	c.catalogs++
	return c.ContentLoader.Catalog(ctx)
}

// Loading the grammar posts or mistakes of every journal reads the journals
// together, with one Reader however many journals there are, and gives what
// loading each journal on its own gives. The second journal is the example
// journal stored under another id.
func TestService_GrammarLoadsReadJournalsTogether(t *testing.T) {
	cfg := exampleNotebooksConfig(t, t.TempDir())
	contents, err := notebook.LoadDirectoryContent(notebook.ContentDirs{
		Stories: cfg.StoriesDirectories, Flashcards: cfg.FlashcardsDirectories, Books: cfg.BooksDirectories,
		Definitions: cfg.DefinitionsDirectories, Etymology: cfg.EtymologyDirectories,
		Journals: cfg.JournalsDirectories, Grammars: cfg.GrammarsDirectories,
	})
	require.NoError(t, err)
	var journal *notebook.NotebookContent
	for i := range contents {
		if contents[i].Family(notebook.FamilyGrammars) != nil {
			journal = &contents[i]
		}
	}
	require.NotNil(t, journal, "the examples must have a journal with grammar corrections")
	second := *journal
	second.ID = journal.ID + "-second"
	contents = append(contents, second)

	svc := newExampleServiceWithShuffle(t, cfg.LearningNotesDirectory, true)
	loader := &catalogCounter{ContentLoader: notebook.NewMemoryContent(contents)}
	svc.SetContent(loader, nil)
	ids := []string{journal.ID, second.ID}

	var wantPosts []GrammarPost
	var wantRefs []GrammarMistakeRef
	for _, id := range ids {
		posts, err := svc.LoadGrammarPosts(0, id, nil)
		require.NoError(t, err)
		require.NotEmpty(t, posts, id)
		wantPosts = append(wantPosts, posts...)
		mistakes, err := svc.LoadGrammarMistakes(0, id, nil)
		require.NoError(t, err)
		for _, m := range mistakes {
			wantRefs = append(wantRefs, GrammarMistakeRef{NotebookID: id, EntryID: m.EntryID, SenseID: m.SenseID, Incorrect: m.Incorrect})
		}
	}

	loader.catalogs = 0
	gotPosts, err := svc.LoadAllGrammarPosts(0)
	require.NoError(t, err)
	assert.Equal(t, 1, loader.catalogs, "one Reader for every journal's posts")
	assert.Equal(t, sortedPosts(wantPosts), sortedPosts(gotPosts))

	loader.catalogs = 0
	gotRefs, err := svc.AllGrammarMistakeRefs(0)
	require.NoError(t, err)
	assert.Equal(t, 1, loader.catalogs, "one Reader for every journal's mistakes")
	assert.ElementsMatch(t, wantRefs, gotRefs)
}

func sortedPosts(posts []GrammarPost) []GrammarPost {
	out := append([]GrammarPost(nil), posts...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].NotebookID != out[j].NotebookID {
			return out[i].NotebookID < out[j].NotebookID
		}
		return out[i].EntryID < out[j].EntryID
	})
	return out
}
