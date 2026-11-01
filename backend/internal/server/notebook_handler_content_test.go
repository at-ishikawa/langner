package server

import (
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// newDirNotebookHandler is NewNotebookHandler serving the notebooks under the
// config's directories, parsed into content the way an import stores them.
func newDirNotebookHandler(notebooksConfig config.NotebooksConfig, templatesConfig config.TemplatesConfig, dictionaryMap map[string]rapidapi.Response, dictionaryReader *dictionary.Reader, clientResolver inference.ClientResolver, noteRepo notebook.NoteRepository) *NotebookHandler {
	h := NewNotebookHandler(notebooksConfig, templatesConfig, dictionaryReader, clientResolver, noteRepo)
	contents, err := notebook.LoadDirectoryContent(notebook.ContentDirs{
		Stories:     notebooksConfig.StoriesDirectories,
		Flashcards:  notebooksConfig.FlashcardsDirectories,
		Books:       notebooksConfig.BooksDirectories,
		Definitions: notebooksConfig.DefinitionsDirectories,
		Etymology:   notebooksConfig.EtymologyDirectories,
		Journals:    notebooksConfig.JournalsDirectories,
		Grammars:    notebooksConfig.GrammarsDirectories,
	})
	if err != nil {
		panic(err)
	}
	h.SetContent(notebook.NewMemoryContent(contents), notebook.MapDictionary(dictionaryMap))
	return h
}
