package notebook

import (
	"context"
	"log/slog"
)

type MultiNoteRepository struct {
	primary   NoteRepository
	secondary NoteRepository
}

func NewMultiNoteRepository(primary, secondary NoteRepository) *MultiNoteRepository {
	return &MultiNoteRepository{primary: primary, secondary: secondary}
}

func (m *MultiNoteRepository) Create(ctx context.Context, note *NoteRecord) error {
	if err := m.primary.Create(ctx, note); err != nil { return err }
	if err := m.secondary.Create(ctx, note); err != nil { slog.Warn("secondary note storage write failed", "error", err) }
	return nil
}

func (m *MultiNoteRepository) Delete(ctx context.Context, notebookID string, expression string) error {
	if err := m.primary.Delete(ctx, notebookID, expression); err != nil { return err }
	if err := m.secondary.Delete(ctx, notebookID, expression); err != nil { slog.Warn("secondary note storage delete failed", "error", err) }
	return nil
}

func (m *MultiNoteRepository) FindAll(ctx context.Context) ([]NoteRecord, error) { return m.primary.FindAll(ctx) }
func (m *MultiNoteRepository) FindByID(ctx context.Context, id int64) (*NoteRecord, error) { return m.primary.FindByID(ctx, id) }

func (m *MultiNoteRepository) BatchCreate(ctx context.Context, notes []*NoteRecord) error {
	if err := m.primary.BatchCreate(ctx, notes); err != nil { return err }
	if err := m.secondary.BatchCreate(ctx, notes); err != nil { slog.Warn("secondary note batch write failed", "error", err) }
	return nil
}

func (m *MultiNoteRepository) BatchUpdate(ctx context.Context, notes []*NoteRecord, newNotebookNotes []NotebookNote) error {
	if err := m.primary.BatchUpdate(ctx, notes, newNotebookNotes); err != nil { return err }
	if err := m.secondary.BatchUpdate(ctx, notes, newNotebookNotes); err != nil { slog.Warn("secondary note batch update failed", "error", err) }
	return nil
}
