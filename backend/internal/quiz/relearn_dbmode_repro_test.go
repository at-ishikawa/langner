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
	store := learning.NewDBHistoryStore(&rlNoteRepo{notes: notes}, &rlLearnRepo{logs: logs}, rlOriginRepo{}, rlSkipRepo{}, nil)

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

// TestRepro_GetLatestLearnedInfo_NewestByDate pins the F1 fix: in DB mode a
// note's logs are reconstructed in id order, so a fresh runtime answer (highest
// id) is LAST, not at [0]. GetLatestLearnedInfo must return the newest attempt
// by date — the override RPCs key on that date to resolve the learning_logs
// row, so returning the stale baseline date made Mark/Undo operate on the wrong
// (old, correct) attempt and a wrong answer would resurface as "correct".
func TestRepro_GetLatestLearnedInfo_NewestByDate(t *testing.T) {
	repoRoot, _ := filepath.Abs("../../..")
	fx := filepath.Join(repoRoot, "frontend", "e2e", "fixtures")
	cfg := config.NotebooksConfig{
		FlashcardsDirectories:  []string{filepath.Join(fx, "flashcards")},
		LearningNotesDirectory: filepath.Join(fx, "learning_notes"),
	}

	ts := func(s string) time.Time { tm, _ := time.Parse(time.RFC3339, s); return tm }
	notes := []notebook.NoteRecord{{
		ID: 1, Entry: "break the ice", Usage: "break the ice",
		NotebookNotes: []notebook.NotebookNote{{NoteID: 1, NotebookType: "flashcard", NotebookID: "idioms", Group: "Common Idioms"}},
	}}
	today := time.Now().UTC()
	// id-ASC as FindAll returns: baseline first, the fresh runtime answer LAST.
	logs := []learning.LearningLog{
		{ID: 1, NoteID: 1, Status: "understood", LearnedAt: ts("2025-01-01T00:00:00Z"), QuizType: "notebook", SourceNotebookID: "idioms"},
		{ID: 2, NoteID: 1, Status: "misunderstood", LearnedAt: today, QuizType: "notebook", SourceNotebookID: "idioms"},
	}
	store := learning.NewDBHistoryStore(&rlNoteRepo{notes: notes}, &rlLearnRepo{logs: logs}, rlOriginRepo{}, rlSkipRepo{}, nil)
	svc := NewService(cfg, nil, nil, nil, config.QuizConfig{})
	svc.SetHistoryStore(store)

	learnedAt, _ := svc.GetLatestLearnedInfo("idioms", "", "break the ice", notebook.QuizTypeNotebook)
	if learnedAt != today.Format("2006-01-02") {
		t.Fatalf("GetLatestLearnedInfo learnedAt = %q, want today %q (must pick the newest attempt by date, not logs[0])",
			learnedAt, today.Format("2006-01-02"))
	}
}

// sharedNoteConfig points the Service at the real fixture notebooks that BOTH
// contain "break the ice" (id break-the-ice): the "idioms" flashcard book and
// the "short-tales" story book, with DIFFERENT meanings. Driving LoadRelearnPool
// through this real construction is what exercises the true reader/originMap the
// server builds (per verify-data-features-with-example-notebooks).
func sharedNoteConfig() config.NotebooksConfig {
	repoRoot, _ := filepath.Abs("../../..")
	fx := filepath.Join(repoRoot, "frontend", "e2e", "fixtures")
	return config.NotebooksConfig{
		StoriesDirectories:     []string{filepath.Join(fx, "stories")},
		JournalsDirectories:    []string{filepath.Join(fx, "journals")},
		FlashcardsDirectories:  []string{filepath.Join(fx, "flashcards")},
		DefinitionsDirectories: []string{filepath.Join(fx, "definitions")},
		EtymologyDirectories:   []string{filepath.Join(fx, "etymology")},
		GrammarsDirectories:    []string{filepath.Join(fx, "grammars")},
		LearningNotesDirectory: filepath.Join(fx, "learning_notes"),
	}
}

// sharedNoteStore builds a DBHistoryStore for ONE note (break-the-ice) linked to
// BOTH the idioms flashcard notebook and the short-tales story notebook, with all
// logs sourced to "idioms" only and the latest a fresh in-window miss. This is the
// DB-mode shape of a single notes row shared across notebooks (notebook_notes),
// which is where the log-bleed bug lived.
func sharedNoteStore(now time.Time) *learning.DBHistoryStore {
	ts := func(s string) time.Time { tm, _ := time.Parse(time.RFC3339, s); return tm }
	notes := []notebook.NoteRecord{{
		ID: 1, SenseID: "break-the-ice", Entry: "break the ice", Usage: "break the ice",
		NotebookNotes: []notebook.NotebookNote{
			{NoteID: 1, NotebookType: "flashcard", NotebookID: "idioms", Group: "Common Idioms"},
			{NoteID: 1, NotebookType: "story", NotebookID: "short-tales", Group: "Chapter 1 - First Meeting", Subgroup: "An awkward introduction"},
		},
	}}
	logs := []learning.LearningLog{
		{ID: 1, NoteID: 1, Status: "understood", LearnedAt: ts("2025-01-01T00:00:00Z"), QuizType: "notebook", SourceNotebookID: "idioms"},
		{ID: 2, NoteID: 1, Status: "misunderstood", LearnedAt: now.Add(-1 * time.Minute), QuizType: "notebook", SourceNotebookID: "idioms"},
	}
	return learning.NewDBHistoryStore(&rlNoteRepo{notes: notes}, &rlLearnRepo{logs: logs}, rlOriginRepo{}, rlSkipRepo{}, nil)
}

// TestRepro_RelearnPool_SharedNoteScopesLogsPerNotebook is the regression for the
// DB-mode shared-note log-bleed bug. The single note "break the ice" is missed
// while studying the idioms book (source_notebook_id=idioms only). The Relearn
// pool MUST surface EXACTLY ONE card, for the idioms book — NOT a second, phantom
// card for short-tales, which shares the note but was never missed there.
//
// Pre-fix (DBHistoryStore bucketed by note_id alone) this returned TWO cards.
func TestRepro_RelearnPool_SharedNoteScopesLogsPerNotebook(t *testing.T) {
	now := time.Now().UTC()
	svc := NewService(sharedNoteConfig(), nil, nil, nil, config.QuizConfig{DisableShuffle: true})
	svc.SetHistoryStore(sharedNoteStore(now))

	cards, err := svc.LoadRelearnPool(now.Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("LoadRelearnPool: %v", err)
	}
	if len(cards) != 1 {
		names := make([]string, len(cards))
		for i, c := range cards {
			names[i] = c.NotebookName
		}
		t.Fatalf("relearn pool has %d cards %v, want 1 (a miss in idioms must NOT replay into short-tales)", len(cards), names)
	}
	if cards[0].NotebookName != "idioms" {
		t.Fatalf("pooled card NotebookName = %q, want \"idioms\" (the notebook the miss was sourced to)", cards[0].NotebookName)
	}
	if cards[0].Entry != "break the ice" {
		t.Fatalf("pooled card entry = %q, want \"break the ice\"", cards[0].Entry)
	}
}

// TestRepro_RelearnPool_SharedNoteCardCarriesSourceMeaning pins that the single
// scoped card carries the meaning of the notebook the miss was sourced to. The
// two fixture books gloss "break the ice" differently; because only the idioms
// miss survives scoping, the card must show the idioms gloss, not short-tales'.
func TestRepro_RelearnPool_SharedNoteCardCarriesSourceMeaning(t *testing.T) {
	now := time.Now().UTC()
	svc := NewService(sharedNoteConfig(), nil, nil, nil, config.QuizConfig{DisableShuffle: true})
	svc.SetHistoryStore(sharedNoteStore(now))

	cards, err := svc.LoadRelearnPool(now.Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("LoadRelearnPool: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("relearn pool has %d cards, want 1", len(cards))
	}
	const idiomsMeaning = "a way to start a conversation in a social setting"
	const storyMeaning = "To initiate conversation or relieve tension in a social situation"
	if cards[0].Meaning != idiomsMeaning {
		t.Fatalf("card meaning = %q, want the idioms gloss %q (not the short-tales gloss %q)",
			cards[0].Meaning, idiomsMeaning, storyMeaning)
	}
}
