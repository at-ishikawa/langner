package quiz

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// Fakes for DBHistoryStore, mirroring the rows import-db + a runtime quiz write
// leave in Postgres for the e2e idioms flashcard. FindAll returns logs in
// id-ASC order (the real DBLearningRepository.FindAll ordering), so a
// runtime-written miss — which gets the HIGHEST id — lands LAST in the slice.

type rlNoteRepo struct{ notes []notebook.NoteRecord }

func (r *rlNoteRepo) FindAll(context.Context) ([]notebook.NoteRecord, error) { return r.notes, nil }
func (r *rlNoteRepo) FindByID(context.Context, int64) (*notebook.NoteRecord, error) {
	return nil, nil
}
func (r *rlNoteRepo) BatchCreate(context.Context, []*notebook.NoteRecord) error { return nil }
func (r *rlNoteRepo) BatchUpdate(context.Context, []*notebook.NoteRecord, []notebook.NotebookNote) error {
	return nil
}
func (r *rlNoteRepo) Create(context.Context, *notebook.NoteRecord) error      { return nil }
func (r *rlNoteRepo) Delete(context.Context, string, string) error            { return nil }
func (r *rlNoteRepo) BatchDeleteNotes(context.Context, []int64) error         { return nil }
func (r *rlNoteRepo) BatchDeleteNotebookNotes(context.Context, []int64) error { return nil }

type rlLearnRepo struct{ logs []learning.LearningLog }

func (r *rlLearnRepo) FindAll(context.Context) ([]learning.LearningLog, error)    { return r.logs, nil }
func (r *rlLearnRepo) BatchCreate(context.Context, []*learning.LearningLog) error { return nil }
func (r *rlLearnRepo) Create(context.Context, *learning.LearningLog) error        { return nil }
func (r *rlLearnRepo) BatchDelete(context.Context, []int64) error                 { return nil }
func (r *rlLearnRepo) UpdateLog(context.Context, learning.UpdateLogInput) (learning.UpdateLogResult, error) {
	return learning.UpdateLogResult{}, nil
}

type rlOriginRepo struct{}

func (rlOriginRepo) FindAll(context.Context) ([]notebook.EtymologyOriginRecord, error) {
	return nil, nil
}
func (rlOriginRepo) BatchCreate(context.Context, []*notebook.EtymologyOriginRecord) error {
	return nil
}

type rlSkipRepo struct{}

func (rlSkipRepo) FindNoteFlags(context.Context, []int64) ([]notebook.NoteSkipFlagRecord, error) {
	return nil, nil
}
func (rlSkipRepo) FindOriginFlags(context.Context, []int64) ([]notebook.OriginSkipFlagRecord, error) {
	return nil, nil
}
func (rlSkipRepo) SkipNote(context.Context, int64, string, time.Time) error   { return nil }
func (rlSkipRepo) ResumeNote(context.Context, int64, string) error            { return nil }
func (rlSkipRepo) SkipOrigin(context.Context, int64, string, time.Time) error { return nil }
func (rlSkipRepo) ResumeOrigin(context.Context, int64, string) error          { return nil }

func TestRepro_RelearnPool_DBOrderDropsRuntimeMiss(t *testing.T) {
	repoRoot, _ := filepath.Abs("../../..")
	fx := filepath.Join(repoRoot, "frontend", "e2e", "fixtures")

	cfg := config.NotebooksConfig{
		StoriesDirectories:     []string{filepath.Join(fx, "stories")},
		JournalsDirectories:    []string{filepath.Join(fx, "journals")},
		FlashcardsDirectories:  []string{filepath.Join(fx, "flashcards")},
		DefinitionsDirectories: []string{filepath.Join(fx, "definitions")},
		EtymologyDirectories:   []string{filepath.Join(fx, "etymology")},
		GrammarsDirectories:    []string{filepath.Join(fx, "grammars")},
		LearningNotesDirectory: filepath.Join(fx, "learning_notes"),
	}

	ts := func(s string) time.Time { tm, _ := time.Parse(time.RFC3339, s); return tm }
	notes := []notebook.NoteRecord{{
		ID: 1, Entry: "break the ice", Usage: "break the ice",
		NotebookNotes: []notebook.NotebookNote{{NoteID: 1, NotebookType: "flashcard", NotebookID: "idioms", Group: "Common Idioms"}},
	}}
	// id-ASC as FindAll returns: the two baseline logs (dated 2025), then the
	// runtime miss the Standard quiz wrote "moments ago" (highest id → LAST).
	now := time.Now().UTC()
	mk := func(id int64, status, at, qt string) learning.LearningLog {
		return learning.LearningLog{ID: id, NoteID: 1, Status: status, LearnedAt: ts(at), QuizType: qt, SourceNotebookID: "idioms"}
	}
	logs := []learning.LearningLog{
		mk(1, "understood", "2025-01-01T00:00:00Z", "freeform"),
		mk(2, "misunderstood", "2025-01-02T00:00:00Z", "notebook"),
		{ID: 3, NoteID: 1, Status: "misunderstood", LearnedAt: now.Add(-1 * time.Minute), QuizType: "notebook", SourceNotebookID: "idioms"}, // fresh miss
	}
	store := learning.NewDBHistoryStore(&rlNoteRepo{notes: notes}, &rlLearnRepo{logs: logs}, rlOriginRepo{}, rlSkipRepo{}).
		WithGrammarYAMLDir(cfg.LearningNotesDirectory)

	svc := NewService(cfg, nil, nil, nil, config.QuizConfig{DisableShuffle: true})
	svc.SetHistoryStore(store)

	cards, err := svc.LoadRelearnPool(now.Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("LoadRelearnPool: %v", err)
	}

	// The learner missed "break the ice" a minute ago in the Standard quiz, so
	// it MUST be in the 24h relearn pool. If DB-order reconstruction leaves the
	// runtime miss out of logs[0], the recency check misses it and the pool is
	// empty — the frontend Start button then stays disabled.
	if len(cards) != 1 {
		t.Fatalf("relearn pool has %d cards, want 1 (runtime Standard miss must be pooled in DB mode)", len(cards))
	}
	if cards[0].Entry != "break the ice" {
		t.Fatalf("pooled card entry = %q, want \"break the ice\"", cards[0].Entry)
	}
}
