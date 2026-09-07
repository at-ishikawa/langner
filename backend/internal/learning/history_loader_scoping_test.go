package learning

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// Fakes scoped to the shared-note log-scoping tests. They return exactly the
// rows the caller wires up so LoadAll's reconstruction can be asserted directly.

type scopeNoteRepo struct{ notes []notebook.NoteRecord }

func (r *scopeNoteRepo) FindAll(context.Context) ([]notebook.NoteRecord, error) { return r.notes, nil }
func (r *scopeNoteRepo) FindByID(context.Context, int64) (*notebook.NoteRecord, error) {
	return nil, nil
}
func (r *scopeNoteRepo) BatchCreate(context.Context, []*notebook.NoteRecord) error { return nil }
func (r *scopeNoteRepo) BatchUpdate(context.Context, []*notebook.NoteRecord, []notebook.NotebookNote) error {
	return nil
}
func (r *scopeNoteRepo) Create(context.Context, *notebook.NoteRecord) error      { return nil }
func (r *scopeNoteRepo) Delete(context.Context, string, string) error            { return nil }
func (r *scopeNoteRepo) BatchDeleteNotes(context.Context, []int64) error         { return nil }
func (r *scopeNoteRepo) BatchDeleteNotebookNotes(context.Context, []int64) error { return nil }

type scopeLearnRepo struct{ logs []LearningLog }

func (r *scopeLearnRepo) FindAll(context.Context) ([]LearningLog, error)    { return r.logs, nil }
func (r *scopeLearnRepo) BatchCreate(context.Context, []*LearningLog) error { return nil }
func (r *scopeLearnRepo) Create(context.Context, *LearningLog) error        { return nil }
func (r *scopeLearnRepo) BatchDelete(context.Context, []int64) error        { return nil }
func (r *scopeLearnRepo) UpdateLog(context.Context, UpdateLogInput) (UpdateLogResult, error) {
	return UpdateLogResult{}, nil
}

type scopeSkipRepo struct{}

func (scopeSkipRepo) FindNoteFlags(context.Context, []int64) ([]notebook.NoteSkipFlagRecord, error) {
	return nil, nil
}
func (scopeSkipRepo) FindOriginFlags(context.Context, []int64) ([]notebook.OriginSkipFlagRecord, error) {
	return nil, nil
}
func (scopeSkipRepo) SkipNote(context.Context, int64, int64, string, time.Time) error   { return nil }
func (scopeSkipRepo) ResumeNote(context.Context, int64, int64, string) error            { return nil }
func (scopeSkipRepo) SkipOrigin(context.Context, int64, int64, string, time.Time) error { return nil }
func (scopeSkipRepo) ResumeOrigin(context.Context, int64, int64, string) error          { return nil }

func ts(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse time %q: %v", s, err)
	}
	return tm
}

// findFlashcardLogs returns the LearnedLogs (recognition/freeform slot) of the
// named expression inside a flashcard notebook's histories, or nil when absent.
func findFlashcardLogs(histories []notebook.LearningHistory, expression string) []notebook.LearningRecord {
	for _, h := range histories {
		for _, e := range h.Expressions {
			if e.Expression == expression {
				return e.LearnedLogs
			}
		}
		for _, sc := range h.Scenes {
			for _, e := range sc.Expressions {
				if e.Expression == expression {
					return e.LearnedLogs
				}
			}
		}
	}
	return nil
}

// statusDates flattens learning records to (status, YYYY-MM-DD) pairs so two
// reconstructions can be compared for equality regardless of Date internals.
func statusDates(records []notebook.LearningRecord) []string {
	out := make([]string, 0, len(records))
	for _, r := range records {
		out = append(out, string(r.Status)+"@"+r.LearnedAt.Time.Format("2006-01-02"))
	}
	return out
}

// TestDBHistoryStore_LoadAll_ScopesSharedNoteLogsPerNotebook pins the fix: ONE
// note linked to two notebooks, with logs sourced to only one of them, must
// reconstruct each notebook's history from ONLY its own-sourced logs — a miss
// recorded in roots-book must not replay into other-book (invariant L4). The
// per-slot ordering stays newest-first.
func TestDBHistoryStore_LoadAll_ScopesSharedNoteLogsPerNotebook(t *testing.T) {
	notes := []notebook.NoteRecord{{
		ID: 1, SenseID: "break-the-ice", Entry: "break the ice", Usage: "break the ice",
		NotebookNotes: []notebook.NotebookNote{
			{NoteID: 1, NotebookType: "flashcard", NotebookID: "roots-book", Group: "Roots"},
			{NoteID: 1, NotebookType: "flashcard", NotebookID: "other-book", Group: "Other"},
		},
	}}
	// roots-book owns two logs (latest a miss); other-book owns one (a pass).
	logs := []LearningLog{
		{ID: 1, NoteID: 1, Status: "understood", LearnedAt: ts(t, "2025-01-01T00:00:00Z"), QuizType: "notebook", SourceNotebookID: "roots-book"},
		{ID: 2, NoteID: 1, Status: "understood", LearnedAt: ts(t, "2025-02-01T00:00:00Z"), QuizType: "notebook", SourceNotebookID: "other-book"},
		{ID: 3, NoteID: 1, Status: "misunderstood", LearnedAt: ts(t, "2025-03-01T00:00:00Z"), QuizType: "notebook", SourceNotebookID: "roots-book"},
	}
	store := NewDBHistoryStore(&scopeNoteRepo{notes: notes}, &scopeLearnRepo{logs: logs}, nil, scopeSkipRepo{}, nil)

	histories, err := store.LoadAll(context.Background(), 0)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	roots := statusDates(findFlashcardLogs(histories["roots-book"], "break the ice"))
	other := statusDates(findFlashcardLogs(histories["other-book"], "break the ice"))

	wantRoots := []string{"misunderstood@2025-03-01", "understood@2025-01-01"} // newest-first, own-sourced only
	wantOther := []string{"understood@2025-02-01"}                             // own-sourced only, NOT the roots miss

	if got := roots; !equalStrings(got, wantRoots) {
		t.Errorf("roots-book logs = %v, want %v", got, wantRoots)
	}
	if got := other; !equalStrings(got, wantOther) {
		t.Errorf("other-book logs = %v, want %v (roots-book's miss must NOT bleed here)", got, wantOther)
	}
}

// TestDBHistoryStore_LoadAll_EmptySourceFallback pins the empty/unmatched-source
// rule for legacy imports: a source-less log on a note with exactly one link
// attributes to that link; on a multi-link note it attributes to the
// first-declared notebook and nowhere else (never fanned out to every link).
func TestDBHistoryStore_LoadAll_EmptySourceFallback(t *testing.T) {
	notes := []notebook.NoteRecord{
		{
			ID: 1, SenseID: "s1", Entry: "single", Usage: "single",
			NotebookNotes: []notebook.NotebookNote{
				{NoteID: 1, NotebookType: "flashcard", NotebookID: "solo-book", Group: "G"},
			},
		},
		{
			ID: 2, SenseID: "s2", Entry: "shared", Usage: "shared",
			NotebookNotes: []notebook.NotebookNote{
				{NoteID: 2, NotebookType: "flashcard", NotebookID: "first-book", Group: "G"},
				{NoteID: 2, NotebookType: "flashcard", NotebookID: "second-book", Group: "G"},
			},
		},
	}
	// Both logs carry an EMPTY source_notebook_id (legacy pre-source import).
	logs := []LearningLog{
		{ID: 1, NoteID: 1, Status: "misunderstood", LearnedAt: ts(t, "2025-01-01T00:00:00Z"), QuizType: "notebook", SourceNotebookID: ""},
		{ID: 2, NoteID: 2, Status: "misunderstood", LearnedAt: ts(t, "2025-01-01T00:00:00Z"), QuizType: "notebook", SourceNotebookID: ""},
	}
	store := NewDBHistoryStore(&scopeNoteRepo{notes: notes}, &scopeLearnRepo{logs: logs}, nil, scopeSkipRepo{}, nil)

	histories, err := store.LoadAll(context.Background(), 0)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	// Single-link note: source-less log attributed to its sole notebook.
	if got := findFlashcardLogs(histories["solo-book"], "single"); len(got) != 1 {
		t.Errorf("solo-book \"single\" logs = %d, want 1 (source-less log on single-link note)", len(got))
	}
	// Multi-link note: attributed to first-declared only, not fanned out.
	if got := findFlashcardLogs(histories["first-book"], "shared"); len(got) != 1 {
		t.Errorf("first-book \"shared\" logs = %d, want 1 (first-declared fallback)", len(got))
	}
	if got := findFlashcardLogs(histories["second-book"], "shared"); len(got) != 0 {
		t.Errorf("second-book \"shared\" logs = %d, want 0 (source-less log must NOT fan out to every link)", len(got))
	}
}

// TestWriteAllLoadAll_YAMLParity_SharedNote proves the DBHistoryStore
// reconstructs the IDENTICAL per-notebook view the YAML reader produces for a
// note shared across notebooks: WriteAll → read YAML, and DBHistoryStore.LoadAll
// over the same rows, must agree on each notebook's own-sourced logs.
func TestWriteAllLoadAll_YAMLParity_SharedNote(t *testing.T) {
	notes := []notebook.NoteRecord{{
		ID: 1, SenseID: "break-the-ice", Entry: "break the ice", Usage: "break the ice",
		NotebookNotes: []notebook.NotebookNote{
			{NoteID: 1, NotebookType: "flashcard", NotebookID: "roots-book", Group: "Roots"},
			{NoteID: 1, NotebookType: "flashcard", NotebookID: "other-book", Group: "Other"},
		},
	}}
	logs := []LearningLog{
		{ID: 1, NoteID: 1, Status: "understood", LearnedAt: ts(t, "2025-01-01T00:00:00Z"), QuizType: "notebook", SourceNotebookID: "roots-book"},
		{ID: 2, NoteID: 1, Status: "understood", LearnedAt: ts(t, "2025-02-01T00:00:00Z"), QuizType: "notebook", SourceNotebookID: "other-book"},
		{ID: 3, NoteID: 1, Status: "misunderstood", LearnedAt: ts(t, "2025-03-01T00:00:00Z"), QuizType: "notebook", SourceNotebookID: "roots-book"},
	}

	// YAML side: write, then read back through the same reader the app uses.
	outDir := t.TempDir()
	writer := NewYAMLLearningRepositoryWriter(outDir)
	if err := writer.WriteAll(notes, logs, nil); err != nil {
		t.Fatalf("WriteAll: %v", err)
	}
	yamlHistories, err := notebook.NewLearningHistories(filepath.Join(outDir, "learning_notes"))
	if err != nil {
		t.Fatalf("read written YAML: %v", err)
	}

	// DB side: reconstruct from the same rows.
	store := NewDBHistoryStore(&scopeNoteRepo{notes: notes}, &scopeLearnRepo{logs: logs}, nil, scopeSkipRepo{}, nil)
	dbHistories, err := store.LoadAll(context.Background(), 0)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	for _, nbID := range []string{"roots-book", "other-book"} {
		yamlLogs := statusDates(findFlashcardLogs(yamlHistories[nbID], "break the ice"))
		dbLogs := statusDates(findFlashcardLogs(dbHistories[nbID], "break the ice"))
		if !equalStrings(yamlLogs, dbLogs) {
			t.Errorf("notebook %q: YAML logs %v != DB logs %v (reconstruction must match the YAML reader)", nbID, yamlLogs, dbLogs)
		}
	}
	// Sanity: the shared miss lives only in roots-book on both sides.
	if got := statusDates(findFlashcardLogs(dbHistories["roots-book"], "break the ice")); !equalStrings(got, []string{"misunderstood@2025-03-01", "understood@2025-01-01"}) {
		t.Errorf("roots-book DB logs = %v", got)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
