package datasync

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
)

type fakeOriginRepoRepro struct {
	recs []notebook.EtymologyOriginRecord
}

func (f *fakeOriginRepoRepro) FindAll(context.Context) ([]notebook.EtymologyOriginRecord, error) {
	return f.recs, nil
}
func (f *fakeOriginRepoRepro) BatchCreate(context.Context, []*notebook.EtymologyOriginRecord) error {
	return nil
}

type fakeNoteRepoRepro struct{ notes []notebook.NoteRecord }

func (f *fakeNoteRepoRepro) FindAll(context.Context) ([]notebook.NoteRecord, error) {
	return f.notes, nil
}
func (f *fakeNoteRepoRepro) FindByID(context.Context, int64) (*notebook.NoteRecord, error) {
	return nil, nil
}
func (f *fakeNoteRepoRepro) BatchCreate(context.Context, []*notebook.NoteRecord) error { return nil }
func (f *fakeNoteRepoRepro) BatchUpdate(context.Context, []*notebook.NoteRecord, []notebook.NotebookNote) error {
	return nil
}
func (f *fakeNoteRepoRepro) Create(context.Context, *notebook.NoteRecord) error      { return nil }
func (f *fakeNoteRepoRepro) Delete(context.Context, string, string) error            { return nil }
func (f *fakeNoteRepoRepro) BatchDeleteNotes(context.Context, []int64) error         { return nil }
func (f *fakeNoteRepoRepro) BatchDeleteNotebookNotes(context.Context, []int64) error { return nil }

type fakeLearningRepoRepro struct{ created []*learning.LearningLog }

func (f *fakeLearningRepoRepro) FindAll(context.Context) ([]learning.LearningLog, error) {
	return nil, nil
}
func (f *fakeLearningRepoRepro) BatchCreate(context.Context, []*learning.LearningLog) error {
	return nil
}
func (f *fakeLearningRepoRepro) Create(_ context.Context, l *learning.LearningLog) error {
	cp := *l
	f.created = append(f.created, &cp)
	return nil
}
func (f *fakeLearningRepoRepro) BatchDelete(context.Context, []int64) error { return nil }
func (f *fakeLearningRepoRepro) UpdateLog(context.Context, learning.UpdateLogInput) (learning.UpdateLogResult, error) {
	return learning.UpdateLogResult{}, nil
}

type fakeSkipRepoRepro struct{}

func (fakeSkipRepoRepro) FindNoteFlags(context.Context, []int64) ([]notebook.NoteSkipFlagRecord, error) {
	return nil, nil
}
func (fakeSkipRepoRepro) FindOriginFlags(context.Context, []int64) ([]notebook.OriginSkipFlagRecord, error) {
	return nil, nil
}
func (fakeSkipRepoRepro) SkipNote(context.Context, int64, string, time.Time) error   { return nil }
func (fakeSkipRepoRepro) ResumeNote(context.Context, int64, string) error            { return nil }
func (fakeSkipRepoRepro) SkipOrigin(context.Context, int64, string, time.Time) error { return nil }
func (fakeSkipRepoRepro) ResumeOrigin(context.Context, int64, string) error          { return nil }

// TestReproSeeder_EtymologyLogsFromE2EFixtures drives the StateSeeder's
// etymology-log seeding against the real e2e fixtures (etymology notebooks +
// learning_notes) through the real reader + real YAML learning source, with
// the origin repo populated exactly as import-db would (via the real YAML
// origin source). It reproduces the "Etymology logs: 0 new" symptom without a
// live DB and shows which origins the seeder resolves.
func TestReproSeeder_EtymologyLogsFromE2EFixtures(t *testing.T) {
	repoRoot, _ := filepath.Abs("../../..")
	fx := filepath.Join(repoRoot, "frontend", "e2e", "fixtures")

	reader, err := notebook.NewReader(
		[]string{filepath.Join(fx, "stories")},
		[]string{filepath.Join(fx, "flashcards")},
		nil,
		[]string{filepath.Join(fx, "definitions")},
		[]string{filepath.Join(fx, "etymology")},
		nil,
	)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	// Origins exactly as import-db writes them (real source), with ids.
	originSrc := notebook.NewYAMLEtymologyOriginSource(reader)
	origins, err := originSrc.FindAll(context.Background())
	if err != nil {
		t.Fatalf("origin source FindAll: %v", err)
	}
	for i := range origins {
		origins[i].ID = int64(i + 1)
		t.Logf("ORIGIN[%d] notebook=%q session=%q origin=%q", origins[i].ID, origins[i].NotebookID, origins[i].SessionTitle, origins[i].Origin)
	}

	// The learning source must surface the flat etymology-origin blocks
	// (word-roots.yml / word-stems.yml): their expressions sit at the top
	// level with `type: origin`, no scenes, and no `type: flashcard` on the
	// metadata. Before the FindByNotebookID fix this returned zero, so the
	// seeder wrote no etymology logs ("Etymology logs: 0 new").
	learningSrc := learning.NewYAMLLearningRepository(filepath.Join(fx, "learning_notes"), nil)
	for _, id := range []string{"word-roots", "word-stems"} {
		exprs, ferr := learningSrc.FindByNotebookID(id)
		if ferr != nil {
			t.Fatalf("FindByNotebookID(%s): %v", id, ferr)
		}
		if len(exprs) == 0 {
			t.Fatalf("FindByNotebookID(%s) returned 0 flat etymology expressions", id)
		}
	}

	learnRepo := &fakeLearningRepoRepro{}
	seeder := &StateSeeder{
		reader:       reader,
		noteRepo:     &fakeNoteRepoRepro{},
		originRepo:   &fakeOriginRepoRepro{recs: origins},
		skipFlagRepo: fakeSkipRepoRepro{},
		learningRepo: learnRepo,
		learningSrc:  learningSrc,
	}

	result := &StateSeedResult{}
	if err := seeder.seedSkipFlagsAndEtymologyLogs(context.Background(), result); err != nil {
		t.Fatalf("seedSkipFlagsAndEtymologyLogs: %v", err)
	}

	// graph, tele (word-roots) + scribo, dico (word-stems) each carry one
	// misunderstood etymology_origin log → 4 rows, each keyed by origin_id
	// (never a note) with quiz_type=etymology_origin.
	if result.EtymologyLogsCreated != 4 {
		t.Fatalf("EtymologyLogsCreated = %d, want 4 (etymology-origin logs not seeded into DB)", result.EtymologyLogsCreated)
	}
	for _, l := range learnRepo.created {
		if l.OriginID == 0 {
			t.Errorf("etymology log must key by origin_id, got NoteID=%d OriginID=0", l.NoteID)
		}
		if l.NoteID != 0 {
			t.Errorf("etymology log must NOT attach to a note, got NoteID=%d", l.NoteID)
		}
		if l.QuizType != string(notebook.QuizTypeEtymologyOrigin) {
			t.Errorf("etymology log quiz_type = %q, want etymology_origin", l.QuizType)
		}
	}
}
