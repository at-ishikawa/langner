package notebook_test

import (
	"context"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// exampleDirs returns the example catalog's directories, as config.example.yml
// wires them.
func exampleDirs(t *testing.T) notebook.ContentDirs {
	examples := examplesDir(t)
	return notebook.ContentDirs{
		Stories:     []string{filepath.Join(examples, "stories")},
		Journals:    []string{filepath.Join(examples, "journals")},
		Flashcards:  []string{filepath.Join(examples, "flashcards")},
		Books:       []string{filepath.Join(examples, "books")},
		Definitions: []string{filepath.Join(examples, "definitions")},
		Etymology:   []string{filepath.Join(examples, "etymology")},
		Grammars:    []string{filepath.Join(examples, "grammars")},
	}
}

func newDirectoryReader(t *testing.T, dirs notebook.ContentDirs) *notebook.Reader {
	t.Helper()
	r, err := notebook.NewReader(dirs.Stories, dirs.Flashcards, dirs.Books, dirs.Definitions, dirs.Etymology, nil)
	require.NoError(t, err)
	require.NoError(t, r.LoadJournals(dirs.Journals))
	require.NoError(t, r.LoadGrammars(dirs.Grammars))
	return r
}

func newMemoryContentReader(t *testing.T, dirs notebook.ContentDirs) *notebook.Reader {
	t.Helper()
	contents, err := notebook.LoadDirectoryContent(dirs)
	require.NoError(t, err)
	r, err := notebook.NewContentReader(context.Background(), notebook.NewMemoryContent(contents), nil)
	require.NoError(t, err)
	return r
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sorted(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

// A Reader over parsed content must read the example catalog exactly as the
// directory Reader does, for every read the server makes.
func TestContentReader_MatchesDirectoryReader(t *testing.T) {
	dirs := exampleDirs(t)
	fromDisk := newDirectoryReader(t, dirs)
	fromContent := newMemoryContentReader(t, dirs)

	require.Equal(t, sortedKeys(fromDisk.GetStoryIndexes()), sortedKeys(fromContent.GetStoryIndexes()))
	require.Equal(t, sortedKeys(fromDisk.GetFlashcardIndexes()), sortedKeys(fromContent.GetFlashcardIndexes()))
	require.Equal(t, sortedKeys(fromDisk.GetEtymologyIndexes()), sortedKeys(fromContent.GetEtymologyIndexes()))
	require.Equal(t, sorted(fromDisk.GetDefinitionsBookIDs()), sorted(fromContent.GetDefinitionsBookIDs()))
	require.Equal(t, sorted(fromDisk.GrammarStoryIDs()), sorted(fromContent.GrammarStoryIDs()))
	require.NotEmpty(t, fromDisk.GetStoryIndexes())

	for _, id := range sortedKeys(fromDisk.GetStoryIndexes()) {
		t.Run("story "+id, func(t *testing.T) {
			want := fromDisk.GetStoryIndexes()[id]
			got := fromContent.GetStoryIndexes()[id]
			assert.Equal(t, want.Kind, got.Kind)
			assert.Equal(t, want.Name, got.Name)
			assert.Equal(t, want.IsBook, got.IsBook)
			assert.Equal(t, want.NotebookPaths, got.NotebookPaths)
			assert.Equal(t, fromDisk.IsJournal(id), fromContent.IsJournal(id))

			wantNBs, wantPaths, err := fromDisk.ReadStoryNotebooksWithPaths(id)
			require.NoError(t, err)
			gotNBs, gotPaths, err := fromContent.ReadStoryNotebooksWithPaths(id)
			require.NoError(t, err)
			assert.Equal(t, wantNBs, gotNBs)
			assert.Equal(t, wantPaths, gotPaths)

			for _, nb := range wantNBs {
				for i := range nb.Scenes {
					assert.Equal(t,
						fromDisk.CorrectionsForScene(id, nb.Event, i),
						fromContent.CorrectionsForScene(id, nb.Event, i))
				}
			}
		})
	}
	for _, id := range sortedKeys(fromDisk.GetFlashcardIndexes()) {
		t.Run("flashcards "+id, func(t *testing.T) {
			assert.Equal(t, fromDisk.GetFlashcardIndexes()[id].Name, fromContent.GetFlashcardIndexes()[id].Name)
			want, err := fromDisk.ReadFlashcardNotebooks(id)
			require.NoError(t, err)
			got, err := fromContent.ReadFlashcardNotebooks(id)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
	for _, id := range sorted(fromDisk.GetDefinitionsBookIDs()) {
		t.Run("definitions "+id, func(t *testing.T) {
			wantBook, wantOK := fromDisk.GetDefinitionsBook(id)
			gotBook, gotOK := fromContent.GetDefinitionsBook(id)
			assert.Equal(t, wantOK, gotOK)
			assert.Equal(t, wantBook, gotBook)
			wantNotes, _ := fromDisk.GetDefinitionsNotes(id)
			gotNotes, _ := fromContent.GetDefinitionsNotes(id)
			assert.Equal(t, wantNotes, gotNotes)
			wantByTitle, _ := fromDisk.GetDefinitionsNotesByTitle(id)
			gotByTitle, _ := fromContent.GetDefinitionsNotesByTitle(id)
			assert.Equal(t, wantByTitle, gotByTitle)
			assert.Equal(t, fromDisk.GetDefinitionsSessionOrder(id), fromContent.GetDefinitionsSessionOrder(id))
			assert.Equal(t, fromDisk.GetDefinitionsLatestDate(id), fromContent.GetDefinitionsLatestDate(id))
			assert.Equal(t, fromDisk.GetDefinitionsBookConceptByMember(id), fromContent.GetDefinitionsBookConceptByMember(id))
		})
	}
	for _, id := range sortedKeys(fromDisk.GetEtymologyIndexes()) {
		t.Run("etymology "+id, func(t *testing.T) {
			want := fromDisk.GetEtymologyIndexes()[id]
			got := fromContent.GetEtymologyIndexes()[id]
			assert.Equal(t, want.Name, got.Name)
			assert.Equal(t, want.NotebookPaths, got.NotebookPaths)
			assert.Equal(t, want.LatestDate, got.LatestDate)

			wantOrigins, err := fromDisk.ReadEtymologyNotebook(id)
			require.NoError(t, err)
			gotOrigins, err := fromContent.ReadEtymologyNotebook(id)
			require.NoError(t, err)
			assert.Equal(t, wantOrigins, gotOrigins)

			wantConcepts, wantRelations := fromDisk.GetEtymologyConceptsAndRelations(id)
			gotConcepts, gotRelations := fromContent.GetEtymologyConceptsAndRelations(id)
			assert.Equal(t, wantConcepts, gotConcepts)
			assert.Equal(t, wantRelations, gotRelations)

			wantBySession, wantRelBySession := fromDisk.GetEtymologyConceptsBySession(id)
			gotBySession, gotRelBySession := fromContent.GetEtymologyConceptsBySession(id)
			assert.Equal(t, wantBySession, gotBySession)
			assert.Equal(t, wantRelBySession, gotRelBySession)
		})
	}

	byKey := func(defs []notebook.EtymologyDefinitionEntry) map[string]notebook.EtymologyDefinitionEntry {
		out := make(map[string]notebook.EtymologyDefinitionEntry)
		for _, d := range defs {
			out[d.NotebookName+"|"+d.SessionTitle+"|"+d.NoteID()] = d
		}
		return out
	}
	assert.Equal(t, byKey(fromDisk.ReadAllEtymologyDefinitions()), byKey(fromContent.ReadAllEtymologyDefinitions()))
	assert.Equal(t, byKey(fromDisk.ReadEtymologyNotebookDefinitions()), byKey(fromContent.ReadEtymologyNotebookDefinitions()))
	assert.NotEmpty(t, fromDisk.ReadEtymologyNotebookDefinitions())
}

// A content Reader reads only the notebooks a caller touches.
func TestContentReader_LoadsOnlyTheNotebooksRead(t *testing.T) {
	contents, err := notebook.LoadDirectoryContent(exampleDirs(t))
	require.NoError(t, err)
	loader := &countingLoader{inner: notebook.NewMemoryContent(contents)}
	r, err := notebook.NewContentReader(context.Background(), loader, nil)
	require.NoError(t, err)
	assert.Empty(t, loader.loaded, "building the reader reads only the catalog")

	var storyID string
	for id, idx := range r.GetStoryIndexes() {
		if !idx.IsBook && !r.IsJournal(id) {
			storyID = id
			break
		}
	}
	require.NotEmpty(t, storyID)
	_, err = r.ReadStoryNotebooks(storyID)
	require.NoError(t, err)
	_, err = r.ReadStoryNotebooks(storyID)
	require.NoError(t, err)
	assert.Equal(t, []string{storyID}, loader.loaded, "one load, for the notebook read")
}

type countingLoader struct {
	inner  notebook.ContentLoader
	loaded []string
}

func (c *countingLoader) Catalog(ctx context.Context) ([]notebook.CatalogEntry, error) {
	return c.inner.Catalog(ctx)
}

func (c *countingLoader) Load(ctx context.Context, ids []string) ([]notebook.NotebookContent, error) {
	c.loaded = append(c.loaded, ids...)
	return c.inner.Load(ctx, ids)
}

// Bundle renders content back to YAML that parses to the same content (the
// same YAML again).
func TestNotebookContent_BundleRoundTrip(t *testing.T) {
	contents, err := notebook.LoadDirectoryContent(exampleDirs(t))
	require.NoError(t, err)
	for _, nc := range contents {
		files, err := nc.Bundle()
		require.NoError(t, err)
		back, err := notebook.ParseBundle(nc.ID, files)
		require.NoError(t, err, nc.ID)
		again, err := back.Bundle()
		require.NoError(t, err)
		assert.Equal(t, files, again, nc.ID)
	}
}

func mustYAML(t *testing.T, v any) string {
	t.Helper()
	data, err := yaml.Marshal(v)
	require.NoError(t, err)
	return string(data)
}

// Readers sharing loaded content (as the server's cache does) don't see each
// other's changes: reading a story twice, through two readers, gives the
// directory reader's result both times.
func TestContentReader_SharedContentIsNotModified(t *testing.T) {
	dirs := exampleDirs(t)
	fromDisk := newDirectoryReader(t, dirs)
	contents, err := notebook.LoadDirectoryContent(dirs)
	require.NoError(t, err)
	shared := notebook.NewCachedContent(notebook.NewMemoryContent(contents))
	for _, id := range sortedKeys(fromDisk.GetStoryIndexes()) {
		want, err := fromDisk.ReadStoryNotebooks(id)
		require.NoError(t, err)
		for i := 0; i < 2; i++ {
			r, err := notebook.NewContentReader(context.Background(), shared, nil)
			require.NoError(t, err)
			got, err := r.ReadStoryNotebooks(id)
			require.NoError(t, err)
			assert.Equal(t, want, got, "%s read %d", id, i)
		}
	}
}

// versionedLoader serves fixed content with a version the test can bump.
type versionedLoader struct {
	countingLoader
	version string
}

func (v *versionedLoader) Catalog(ctx context.Context) ([]notebook.CatalogEntry, error) {
	catalog, err := v.inner.Catalog(ctx)
	for i := range catalog {
		catalog[i].Version = v.version
	}
	return catalog, err
}

// The cache loads a notebook once, and again only after its version changes.
func TestCachedContent_ReloadsOnlyChangedNotebooks(t *testing.T) {
	contents, err := notebook.LoadDirectoryContent(exampleDirs(t))
	require.NoError(t, err)
	inner := &versionedLoader{countingLoader: countingLoader{inner: notebook.NewMemoryContent(contents)}, version: "v1"}
	cache := notebook.NewCachedContent(inner)
	ctx := context.Background()
	id := contents[0].ID

	for i := 0; i < 3; i++ {
		_, err := cache.Catalog(ctx)
		require.NoError(t, err)
		got, err := cache.Load(ctx, []string{id})
		require.NoError(t, err)
		require.Len(t, got, 1)
	}
	assert.Equal(t, []string{id}, inner.loaded, "loaded once while the version is unchanged")

	inner.version = "v2"
	_, err = cache.Catalog(ctx)
	require.NoError(t, err)
	_, err = cache.Load(ctx, []string{id})
	require.NoError(t, err)
	assert.Equal(t, []string{id, id}, inner.loaded, "reloaded after the notebook changed")
}
