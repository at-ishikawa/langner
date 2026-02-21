package notebook

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/database"
)

// NoteRepository defines operations for managing notes.
type NoteRepository interface {
	FindAll(ctx context.Context) ([]NoteRecord, error)
	Create(ctx context.Context, note *NoteRecord) error
	CreateNotebookNote(ctx context.Context, nn *NotebookNote) error
	Update(ctx context.Context, note *NoteRecord) error
}

// DBNoteRepository implements NoteRepository using MySQL.
type DBNoteRepository struct {
	db *sqlx.DB
}

// NewDBNoteRepository creates a new DBNoteRepository.
func NewDBNoteRepository(db *sqlx.DB) *DBNoteRepository {
	return &DBNoteRepository{db: db}
}

// FindAll returns all notes with their images, references, and notebook notes.
func (r *DBNoteRepository) FindAll(ctx context.Context) ([]NoteRecord, error) {
	var notes []NoteRecord
	if err := r.db.SelectContext(ctx, &notes, "SELECT * FROM notes ORDER BY id"); err != nil {
		return nil, fmt.Errorf("load all notes: %w", err)
	}
	if err := r.loadRelations(ctx, notes); err != nil {
		return nil, err
	}
	return notes, nil
}

// CreateNotebookNote inserts a single notebook_note record.
func (r *DBNoteRepository) CreateNotebookNote(ctx context.Context, nn *NotebookNote) error {
	result, err := r.db.NamedExecContext(ctx,
		"INSERT INTO notebook_notes (note_id, notebook_type, notebook_id, `group`, subgroup) VALUES (:note_id, :notebook_type, :notebook_id, :group, :subgroup)",
		nn)
	if err != nil {
		return fmt.Errorf("insert notebook note: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get notebook note insert ID: %w", err)
	}
	nn.ID = id
	return nil
}

// Create inserts a note with its images, references, and notebook notes in a transaction.
func (r *DBNoteRepository) Create(ctx context.Context, note *NoteRecord) error {
	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		result, err := tx.NamedExecContext(ctx,
			"INSERT INTO notes (`usage`, entry, meaning, level, dictionary_number) VALUES (:usage, :entry, :meaning, :level, :dictionary_number)",
			note)
		if err != nil {
			return fmt.Errorf("insert note: %w", err)
		}
		noteID, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("get note insert ID: %w", err)
		}
		note.ID = noteID

		for i := range note.Images {
			note.Images[i].NoteID = noteID
			_, err := tx.NamedExecContext(ctx,
				"INSERT INTO note_images (note_id, url, sort_order) VALUES (:note_id, :url, :sort_order)",
				note.Images[i])
			if err != nil {
				return fmt.Errorf("insert note image: %w", err)
			}
		}

		for i := range note.References {
			note.References[i].NoteID = noteID
			_, err := tx.NamedExecContext(ctx,
				"INSERT INTO note_references (note_id, link, description, sort_order) VALUES (:note_id, :link, :description, :sort_order)",
				note.References[i])
			if err != nil {
				return fmt.Errorf("insert note reference: %w", err)
			}
		}

		for i := range note.NotebookNotes {
			note.NotebookNotes[i].NoteID = noteID
			_, err := tx.NamedExecContext(ctx,
				"INSERT INTO notebook_notes (note_id, notebook_type, notebook_id, `group`, subgroup) VALUES (:note_id, :notebook_type, :notebook_id, :group, :subgroup)",
				note.NotebookNotes[i])
			if err != nil {
				return fmt.Errorf("insert notebook note: %w", err)
			}
		}

		return nil
	})
}

// Update updates a note's fields (not related records).
func (r *DBNoteRepository) Update(ctx context.Context, note *NoteRecord) error {
	_, err := r.db.NamedExecContext(ctx,
		"UPDATE notes SET `usage` = :usage, entry = :entry, meaning = :meaning, level = :level, dictionary_number = :dictionary_number WHERE id = :id",
		note)
	if err != nil {
		return fmt.Errorf("update note: %w", err)
	}
	return nil
}

func (r *DBNoteRepository) loadRelations(ctx context.Context, notes []NoteRecord) error {
	if len(notes) == 0 {
		return nil
	}

	noteIDs := make([]int64, len(notes))
	noteMap := make(map[int64]*NoteRecord, len(notes))
	for i := range notes {
		noteIDs[i] = notes[i].ID
		noteMap[notes[i].ID] = &notes[i]
	}

	query, args, err := sqlx.In("SELECT * FROM note_images WHERE note_id IN (?) ORDER BY sort_order", noteIDs)
	if err != nil {
		return fmt.Errorf("build note images query: %w", err)
	}
	var images []NoteImage
	if err := r.db.SelectContext(ctx, &images, r.db.Rebind(query), args...); err != nil {
		return fmt.Errorf("load note images: %w", err)
	}
	for _, img := range images {
		n := noteMap[img.NoteID]
		n.Images = append(n.Images, img)
	}

	query, args, err = sqlx.In("SELECT * FROM note_references WHERE note_id IN (?) ORDER BY sort_order", noteIDs)
	if err != nil {
		return fmt.Errorf("build note references query: %w", err)
	}
	var refs []NoteReference
	if err := r.db.SelectContext(ctx, &refs, r.db.Rebind(query), args...); err != nil {
		return fmt.Errorf("load note references: %w", err)
	}
	for _, ref := range refs {
		n := noteMap[ref.NoteID]
		n.References = append(n.References, ref)
	}

	query, args, err = sqlx.In("SELECT * FROM notebook_notes WHERE note_id IN (?) ORDER BY id", noteIDs)
	if err != nil {
		return fmt.Errorf("build notebook notes query: %w", err)
	}
	var nns []NotebookNote
	if err := r.db.SelectContext(ctx, &nns, r.db.Rebind(query), args...); err != nil {
		return fmt.Errorf("load notebook notes: %w", err)
	}
	for _, nn := range nns {
		n := noteMap[nn.NoteID]
		n.NotebookNotes = append(n.NotebookNotes, nn)
	}

	return nil
}
