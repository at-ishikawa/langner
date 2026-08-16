package learning

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// The fakes below implement just enough of the four repository interfaces for
// DBHistoryStore.LoadAll. They stand in for the DB rows Postgres would return,
// letting us exercise the RECONSTRUCTION logic (rows → LearningHistory shape)
// deterministically without a live database. The transform under test is a
// pure mapping, so fake rows are the realistic input; only the actual SQL in
// the DB* repositories is left unexercised here (Postgres-only).

type fakeNoteRepo struct{ notes []notebook.NoteRecord }

func (f *fakeNoteRepo) FindAll(context.Context) ([]notebook.NoteRecord, error) {
	return f.notes, nil
}
func (f *fakeNoteRepo) FindByID(context.Context, int64) (*notebook.NoteRecord, error) {
	return nil, nil
}
func (f *fakeNoteRepo) BatchCreate(context.Context, []*notebook.NoteRecord) error { return nil }
func (f *fakeNoteRepo) BatchUpdate(context.Context, []*notebook.NoteRecord, []notebook.NotebookNote) error {
	return nil
}
func (f *fakeNoteRepo) Create(context.Context, *notebook.NoteRecord) error      { return nil }
func (f *fakeNoteRepo) Delete(context.Context, string, string) error            { return nil }
func (f *fakeNoteRepo) BatchDeleteNotes(context.Context, []int64) error         { return nil }
func (f *fakeNoteRepo) BatchDeleteNotebookNotes(context.Context, []int64) error { return nil }

type fakeLearningRepo struct{ logs []LearningLog }

func (f *fakeLearningRepo) FindAll(context.Context) ([]LearningLog, error)    { return f.logs, nil }
func (f *fakeLearningRepo) BatchCreate(context.Context, []*LearningLog) error { return nil }
func (f *fakeLearningRepo) Create(context.Context, *LearningLog) error        { return nil }
func (f *fakeLearningRepo) BatchDelete(context.Context, []int64) error        { return nil }
func (f *fakeLearningRepo) UpdateLog(context.Context, UpdateLogInput) (UpdateLogResult, error) {
	return UpdateLogResult{}, nil
}

type fakeOriginRepo struct {
	records []notebook.EtymologyOriginRecord
}

func (f *fakeOriginRepo) FindAll(context.Context) ([]notebook.EtymologyOriginRecord, error) {
	return f.records, nil
}
func (f *fakeOriginRepo) BatchCreate(context.Context, []*notebook.EtymologyOriginRecord) error {
	return nil
}

type fakeSkipFlagRepo struct {
	noteFlags   []notebook.NoteSkipFlagRecord
	originFlags []notebook.OriginSkipFlagRecord
}

func (f *fakeSkipFlagRepo) FindNoteFlags(_ context.Context, noteIDs []int64) ([]notebook.NoteSkipFlagRecord, error) {
	want := make(map[int64]bool, len(noteIDs))
	for _, id := range noteIDs {
		want[id] = true
	}
	var out []notebook.NoteSkipFlagRecord
	for _, r := range f.noteFlags {
		if want[r.NoteID] {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeSkipFlagRepo) FindOriginFlags(_ context.Context, originIDs []int64) ([]notebook.OriginSkipFlagRecord, error) {
	want := make(map[int64]bool, len(originIDs))
	for _, id := range originIDs {
		want[id] = true
	}
	var out []notebook.OriginSkipFlagRecord
	for _, r := range f.originFlags {
		if want[r.OriginID] {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeSkipFlagRepo) SkipNote(context.Context, int64, string, time.Time) error   { return nil }
func (f *fakeSkipFlagRepo) ResumeNote(context.Context, int64, string) error            { return nil }
func (f *fakeSkipFlagRepo) SkipOrigin(context.Context, int64, string, time.Time) error { return nil }
func (f *fakeSkipFlagRepo) ResumeOrigin(context.Context, int64, string) error          { return nil }

// findExpr locates a reconstructed expression by notebook id + spelling,
// searching both the flat .Expressions slice (flashcards) and nested scenes
// (stories / origins).
func findExpr(t *testing.T, histories map[string][]notebook.LearningHistory, notebookID, expression string) notebook.LearningHistoryExpression {
	t.Helper()
	for _, h := range histories[notebookID] {
		for _, e := range h.Expressions {
			if e.Expression == expression {
				return e
			}
		}
		for _, sc := range h.Scenes {
			for _, e := range sc.Expressions {
				if e.Expression == expression {
					return e
				}
			}
		}
	}
	t.Fatalf("expression %q not found in notebook %q", expression, notebookID)
	return notebook.LearningHistoryExpression{}
}

// TestDBHistoryStore_LoadAll_RoutesLogsAndSkipFlags asserts the read side is
// symmetric with the write side (learning-history invariant L2): each log is
// bucketed into the SAME slot the writer keys it under —
//   - quiz_type=reverse          → ReverseLogs
//   - quiz_type=etymology_origin  → EtymologyOriginLogs (keyed by origin_id)
//   - everything else             → LearnedLogs
//
// and skip flags land on the matching per-quiz-type SkippedAt entry. This is
// the exact routing GetLogsForQuizType / SetLogsForQuizType use on the write
// side, so a card's logs read back under the same key they were written to.
func TestDBHistoryStore_LoadAll_RoutesLogsAndSkipFlags(t *testing.T) {
	baseTime := time.Date(2025, 4, 2, 8, 0, 0, 0, time.UTC)

	notes := []notebook.NoteRecord{
		{
			ID:    1,
			Entry: "break the ice",
			NotebookNotes: []notebook.NotebookNote{
				{NoteID: 1, NotebookType: "story", NotebookID: "demo-story", Group: "Episode 1", Subgroup: "Opening"},
			},
		},
		{
			ID:    2,
			Entry: "lose one's temper",
			NotebookNotes: []notebook.NotebookNote{
				{NoteID: 2, NotebookType: "flashcard", NotebookID: "demo-cards", Group: "Common Idioms"},
			},
		},
	}

	logs := []LearningLog{
		// Vocab note 1: one recognition + one reverse attempt.
		{NoteID: 1, Status: "understood", LearnedAt: baseTime, Quality: 4, QuizType: string(notebook.QuizTypeNotebook), IntervalDays: 7},
		{NoteID: 1, Status: "misunderstood", LearnedAt: baseTime.Add(time.Hour), Quality: 1, QuizType: string(notebook.QuizTypeReverse), IntervalDays: 1},
		// Flashcard note 2: a freeform attempt (non-reverse) stays in LearnedLogs.
		{NoteID: 2, Status: "understood", LearnedAt: baseTime, Quality: 5, QuizType: "freeform", IntervalDays: 30},
		// Origin 500: an etymology_origin attempt keyed by origin_id, NOT any note.
		{OriginID: 500, Status: "understood", LearnedAt: baseTime, Quality: 3, QuizType: string(notebook.QuizTypeEtymologyOrigin), IntervalDays: 7},
	}

	origins := []notebook.EtymologyOriginRecord{
		{ID: 500, NotebookID: "demo-story", SessionTitle: "Episode 1", Origin: "alter", Meaning: "other"},
	}

	skipFlags := &fakeSkipFlagRepo{
		noteFlags: []notebook.NoteSkipFlagRecord{
			{NoteID: 2, QuizType: string(notebook.QuizTypeNotebook), SkippedAt: baseTime},
			// Reverse-track exclusion on note 1: the DB read must surface it
			// per quiz type so the reverse count/loader skip-filter can drop it
			// (mirrors the YAML path — see the reverse-skip count filter shared
			// by the story/flashcard reverse counters).
			{NoteID: 1, QuizType: string(notebook.QuizTypeReverse), SkippedAt: baseTime},
		},
		originFlags: []notebook.OriginSkipFlagRecord{
			{OriginID: 500, QuizType: string(notebook.QuizTypeEtymologyOrigin), SkippedAt: baseTime},
		},
	}

	store := NewDBHistoryStore(
		&fakeNoteRepo{notes: notes},
		&fakeLearningRepo{logs: logs},
		&fakeOriginRepo{records: origins},
		skipFlags,
	)

	histories, err := store.LoadAll(context.Background())
	require.NoError(t, err)

	// Vocab note 1 (story): recognition → LearnedLogs, reverse → ReverseLogs.
	ice := findExpr(t, histories, "demo-story", "break the ice")
	assert.Equal(t, notebook.LearningExpressionTypeVocabulary, ice.Type)
	require.Len(t, ice.LearnedLogs, 1, "notebook attempt routes to LearnedLogs")
	assert.Equal(t, string(notebook.QuizTypeNotebook), ice.LearnedLogs[0].QuizType)
	require.Len(t, ice.ReverseLogs, 1, "reverse attempt routes to ReverseLogs")
	assert.Equal(t, string(notebook.QuizTypeReverse), ice.ReverseLogs[0].QuizType)
	assert.Empty(t, ice.EtymologyOriginLogs)
	// The reverse-track skip flag reconstructs onto SkippedAt[reverse] — the
	// exact predicate the reverse count/loader skip-filter reads, so an
	// excluded reverse word drops on the DB path just as on the YAML path.
	assert.True(t, ice.SkippedAt.IsSkipped(notebook.QuizTypeReverse), "reverse skip flag must reconstruct onto SkippedAt[reverse]")
	assert.False(t, ice.SkippedAt.IsSkipped(notebook.QuizTypeNotebook), "reverse-only skip must not spill onto the forward track")

	// Flashcard note 2: freeform stays in LearnedLogs; skip flag maps through.
	temper := findExpr(t, histories, "demo-cards", "lose one's temper")
	require.Len(t, temper.LearnedLogs, 1)
	assert.Equal(t, "freeform", temper.LearnedLogs[0].QuizType, "freeform record keeps its own quiz type in LearnedLogs")
	assert.True(t, temper.SkippedAt.IsSkipped(notebook.QuizTypeNotebook), "note skip flag must reconstruct onto SkippedAt")

	// Origin 500: its etymology_origin log is keyed by origin, on an
	// origin-typed expression — never conflated onto a vocab note.
	alter := findExpr(t, histories, "demo-story", "alter")
	assert.Equal(t, notebook.LearningExpressionTypeOrigin, alter.Type)
	require.Len(t, alter.EtymologyOriginLogs, 1, "origin_id log routes to EtymologyOriginLogs")
	assert.Equal(t, string(notebook.QuizTypeEtymologyOrigin), alter.EtymologyOriginLogs[0].QuizType)
	assert.Empty(t, alter.LearnedLogs)
	assert.Empty(t, alter.ReverseLogs)
	assert.True(t, alter.SkippedAt.IsSkipped(notebook.QuizTypeEtymologyOrigin), "origin skip flag must reconstruct onto SkippedAt")
}
