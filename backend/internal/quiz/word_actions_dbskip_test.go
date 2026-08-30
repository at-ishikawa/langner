package quiz

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// fakeSkipFlags records skip-flag writes in memory (the DB skip-flag repo
// stand-in) and — like the real DBSkipFlagRepository — never touches the
// filesystem.
type fakeSkipFlags struct {
	skippedNotes  []string // "noteID:quizType"
	resumedNotes  []string
	skippedOrigin []string
	resumedOrigin []string
}

func (f *fakeSkipFlags) FindNoteFlags(context.Context, []int64) ([]notebook.NoteSkipFlagRecord, error) {
	return nil, nil
}
func (f *fakeSkipFlags) FindOriginFlags(context.Context, []int64) ([]notebook.OriginSkipFlagRecord, error) {
	return nil, nil
}
func (f *fakeSkipFlags) SkipNote(_ context.Context, _ int64, noteID int64, quizType string, _ time.Time) error {
	f.skippedNotes = append(f.skippedNotes, key(noteID, quizType))
	return nil
}
func (f *fakeSkipFlags) ResumeNote(_ context.Context, _ int64, noteID int64, quizType string) error {
	f.resumedNotes = append(f.resumedNotes, key(noteID, quizType))
	return nil
}
func (f *fakeSkipFlags) SkipOrigin(_ context.Context, _ int64, originID int64, quizType string, _ time.Time) error {
	f.skippedOrigin = append(f.skippedOrigin, key(originID, quizType))
	return nil
}
func (f *fakeSkipFlags) ResumeOrigin(_ context.Context, _ int64, originID int64, quizType string) error {
	f.resumedOrigin = append(f.resumedOrigin, key(originID, quizType))
	return nil
}

func key(id int64, quizType string) string {
	return strconv.FormatInt(id, 10) + ":" + quizType
}

// fakeNoteRepo returns a fixed set of notes; only FindAll is exercised by
// resolveSkipTarget.
type fakeNoteRepo struct{ notes []notebook.NoteRecord }

func (r *fakeNoteRepo) FindAll(context.Context) ([]notebook.NoteRecord, error) { return r.notes, nil }
func (r *fakeNoteRepo) FindByID(context.Context, int64) (*notebook.NoteRecord, error) {
	return nil, nil
}
func (r *fakeNoteRepo) BatchCreate(context.Context, []*notebook.NoteRecord) error { return nil }
func (r *fakeNoteRepo) BatchUpdate(context.Context, []*notebook.NoteRecord, []notebook.NotebookNote) error {
	return nil
}
func (r *fakeNoteRepo) Create(context.Context, *notebook.NoteRecord) error      { return nil }
func (r *fakeNoteRepo) Delete(context.Context, string, string) error            { return nil }
func (r *fakeNoteRepo) BatchDeleteNotes(context.Context, []int64) error         { return nil }
func (r *fakeNoteRepo) BatchDeleteNotebookNotes(context.Context, []int64) error { return nil }

func snapshotDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := make(map[string]string)
	require.NoError(t, filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		out[path] = string(b)
		return nil
	}))
	return out
}

// TestSkipWord_DBMode_WritesSkipFlagsNotYAML pins the DB-only-writes behavior
// of the deliberate Exclude action: in DB mode SkipWord/ResumeWord UPSERT/DELETE
// the DB skip-flag marker and MUST NOT write the on-disk learning_notes YAML.
// This is the PG-free before/after — on the pre-fix code SkipWord wrote
// learning_notes YAML (the reported "reputation" bug); after, the directory is
// byte-for-byte unchanged.
func TestSkipWord_DBMode_WritesSkipFlagsNotYAML(t *testing.T) {
	dir := t.TempDir()
	// A pre-existing learning_notes file that must stay untouched.
	seed := filepath.Join(dir, "roots-book.yml")
	require.NoError(t, os.WriteFile(seed, []byte("- metadata:\n    id: roots-book\n"), 0o644))
	before := snapshotDir(t, dir)

	skipRepo := &fakeSkipFlags{}
	svc := NewService(config.NotebooksConfig{LearningNotesDirectory: dir}, nil, nil, nil, config.QuizConfig{})
	svc.SetSkipStores(skipRepo, &fakeNoteRepo{}, nil)

	// Learn-page Exclude carries the DB note id directly (homograph-safe).
	info := CardInfo{NotebookName: "roots-book", Expression: "reputation", NoteID: 7}

	require.NoError(t, svc.SkipWord(0, info, "", []notebook.QuizType{notebook.QuizTypeNotebook}))
	assert.Equal(t, before, snapshotDir(t, dir),
		"SkipWord in DB mode MUST NOT create or modify any learning_notes YAML file")
	assert.Equal(t, []string{key(7, "notebook")}, skipRepo.skippedNotes,
		"SkipWord must UPSERT note_skip_flags(note_id, quiz_type)")

	require.NoError(t, svc.ResumeWord(0, info, []notebook.QuizType{notebook.QuizTypeNotebook}))
	assert.Equal(t, before, snapshotDir(t, dir),
		"ResumeWord in DB mode MUST NOT write learning_notes YAML either")
	assert.Equal(t, []string{key(7, "notebook")}, skipRepo.resumedNotes,
		"ResumeWord must DELETE the note_skip_flags row")
}

// TestSkipWord_DBMode_ResolvesNoteByExpression covers the in-quiz Exclude path
// where the request carries no DB note id: the target note is resolved by its
// surface (usage/entry) within the notebook — the SAME mapping the read side
// reconstructs (L2). It also asserts a homograph with no stable id fails loudly
// rather than skipping the wrong sense.
func TestSkipWord_DBMode_ResolvesNoteByExpression(t *testing.T) {
	dir := t.TempDir()
	skipRepo := &fakeSkipFlags{}
	notes := []notebook.NoteRecord{
		{ID: 3, Usage: "reputation", Entry: "reputation",
			NotebookNotes: []notebook.NotebookNote{{NotebookID: "roots-book"}}},
	}
	svc := NewService(config.NotebooksConfig{LearningNotesDirectory: dir}, nil, nil, nil, config.QuizConfig{})
	svc.SetSkipStores(skipRepo, &fakeNoteRepo{notes: notes}, nil)

	info := CardInfo{NotebookName: "roots-book", Expression: "reputation"} // no NoteID
	require.NoError(t, svc.SkipWord(0, info, "", []notebook.QuizType{notebook.QuizTypeReverse}))
	assert.Equal(t, []string{key(3, "reverse")}, skipRepo.skippedNotes,
		"resolve the note by (notebook, surface) exactly as the loaders reconstruct it")

	// Homograph: two notes share the spelling, no stable id -> error naming it.
	homographs := []notebook.NoteRecord{
		{ID: 4, Usage: "bank", Entry: "bank", SenseID: "bank-finance",
			NotebookNotes: []notebook.NotebookNote{{NotebookID: "roots-book"}}},
		{ID: 5, Usage: "bank", Entry: "bank", SenseID: "bank-river",
			NotebookNotes: []notebook.NotebookNote{{NotebookID: "roots-book"}}},
	}
	svc.SetSkipStores(skipRepo, &fakeNoteRepo{notes: homographs}, nil)
	err := svc.SkipWord(0, CardInfo{NotebookName: "roots-book", Expression: "bank"}, "",
		[]notebook.QuizType{notebook.QuizTypeNotebook})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bank")
}

// TestSkipWord_DBMode_GrammarIsNotPersisted asserts a grammar exclude neither
// writes YAML nor touches the note/origin skip tables in DB mode (grammar has
// no DB skip-flag table today — a reported follow-up), rather than erroring or
// mis-resolving a correction sense_id as a note.
func TestSkipWord_DBMode_GrammarIsNotPersisted(t *testing.T) {
	dir := t.TempDir()
	before := snapshotDir(t, dir)
	skipRepo := &fakeSkipFlags{}
	svc := NewService(config.NotebooksConfig{LearningNotesDirectory: dir}, nil, nil, nil, config.QuizConfig{})
	svc.SetSkipStores(skipRepo, &fakeNoteRepo{}, nil)

	info := CardInfo{NotebookName: "journal-nb", StoryTitle: notebook.JournalStoryTitle, Expression: "corr-1", ID: "corr-1"}
	require.NoError(t, svc.SkipWord(0, info, "", []notebook.QuizType{notebook.QuizTypeGrammar}))

	assert.Equal(t, before, snapshotDir(t, dir), "grammar exclude must not write YAML in DB mode")
	assert.Empty(t, skipRepo.skippedNotes, "grammar exclude must not write note_skip_flags")
	assert.Empty(t, skipRepo.skippedOrigin, "grammar exclude must not write origin_skip_flags")
}
