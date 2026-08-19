package datasync

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// fakeGrammarRepoRepro records FindOrCreate calls and hands out incrementing
// ids, standing in for the DB grammar_corrections table.
type fakeGrammarRepoRepro struct {
	next     int64
	upserted map[string]int64 // notebookID\x00senseID -> id
}

func (f *fakeGrammarRepoRepro) FindAll(context.Context) ([]notebook.GrammarCorrectionRecord, error) {
	return nil, nil
}
func (f *fakeGrammarRepoRepro) FindOrCreate(_ context.Context, notebookID, senseID string) (notebook.GrammarCorrectionRecord, error) {
	if f.upserted == nil {
		f.upserted = map[string]int64{}
	}
	key := notebookID + "\x00" + senseID
	id, ok := f.upserted[key]
	if !ok {
		id = atomic.AddInt64(&f.next, 1)
		f.upserted[key] = id
	}
	// created_at==updated_at signals a fresh insert (the seeder counts it).
	return notebook.GrammarCorrectionRecord{ID: id, NotebookID: notebookID, SenseID: senseID}, nil
}

// TestStateSeeder_SeedGrammarCorrections_FromE2EFixtures drives the real
// StateSeeder grammar phase over the e2e learning_notes fixtures (practice.yml
// carries a `type: grammar` block with one seeded misunderstood correction) and
// asserts it upserts a grammar_corrections row and writes its learning_logs
// keyed on correction_id with quiz_type=grammar — the DB-only-state parallel of
// the etymology-origin seed. The true Postgres round-trip is Postgres-only and
// NOT exercised here.
func TestStateSeeder_SeedGrammarCorrections_FromE2EFixtures(t *testing.T) {
	repoRoot, _ := filepath.Abs("../../..")
	learningNotes := filepath.Join(repoRoot, "frontend", "e2e", "fixtures", "learning_notes")

	learnRepo := &fakeLearningRepoRepro{}
	grammarRepo := &fakeGrammarRepoRepro{}
	seeder := &StateSeeder{
		grammarRepo:      grammarRepo,
		learningRepo:     learnRepo,
		learningNotesDir: learningNotes,
	}

	result := &StateSeedResult{}
	if err := seeder.seedGrammarCorrections(context.Background(), result); err != nil {
		t.Fatalf("seedGrammarCorrections: %v", err)
	}

	if result.GrammarCorrectionsCreated == 0 {
		t.Fatalf("no grammar corrections seeded from the fixtures")
	}
	if result.GrammarLogsCreated == 0 {
		t.Fatalf("no grammar logs seeded from the fixtures")
	}
	// The practice.yml correction must have been upserted under its notebook.
	if _, ok := grammarRepo.upserted["practice\x00party-suggested"]; !ok {
		t.Fatalf("expected grammar correction (practice, party-suggested) to be upserted, got %v", grammarRepo.upserted)
	}
	// Every seeded grammar log keys on a correction (never a note/origin) and
	// keeps quiz_type=grammar so Analytics labels it correctly.
	found := false
	for _, l := range learnRepo.created {
		if l.SourceNotebookID != "practice" {
			continue
		}
		found = true
		if l.CorrectionID == 0 {
			t.Errorf("grammar log must key on correction_id, got 0")
		}
		if l.NoteID != 0 || l.OriginID != 0 {
			t.Errorf("grammar log must not set note_id/origin_id, got note=%d origin=%d", l.NoteID, l.OriginID)
		}
		if l.QuizType != string(notebook.QuizTypeGrammar) {
			t.Errorf("grammar log quiz_type = %q, want grammar", l.QuizType)
		}
		if l.Status != "misunderstood" {
			t.Errorf("seeded party-suggested log status = %q, want misunderstood", l.Status)
		}
	}
	if !found {
		t.Fatalf("no grammar log written for the practice notebook")
	}
}
