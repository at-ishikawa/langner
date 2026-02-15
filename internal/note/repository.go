// Package note provides note domain models and repository interfaces.
package note

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// Note represents a vocabulary word or phrase.
type Note struct {
	ID               int64           `db:"id" yaml:"id"`
	Usage            string          `db:"usage" yaml:"usage"`
	Entry            string          `db:"entry" yaml:"entry"`
	Meaning          string          `db:"meaning" yaml:"meaning"`
	Level            string          `db:"level" yaml:"level"`
	DictionaryNumber int             `db:"dictionary_number" yaml:"dictionary_number"`
	CreatedAt        time.Time       `db:"created_at" yaml:"created_at"`
	UpdatedAt        time.Time       `db:"updated_at" yaml:"updated_at"`
	Images           []NoteImage     `db:"-" yaml:"images,omitempty"`
	References       []NoteReference `db:"-" yaml:"references,omitempty"`
	NotebookNotes    []NotebookNote  `db:"-" yaml:"notebook_notes,omitempty"`
}

// NoteImage represents an image link for visual vocabulary learning.
type NoteImage struct {
	ID        int64     `db:"id" yaml:"id"`
	NoteID    int64     `db:"note_id" yaml:"note_id"`
	URL       string    `db:"url" yaml:"url"`
	SortOrder int       `db:"sort_order" yaml:"sort_order"`
	CreatedAt time.Time `db:"created_at" yaml:"created_at"`
	UpdatedAt time.Time `db:"updated_at" yaml:"updated_at"`
}

// NoteReference represents an external reference for a note.
type NoteReference struct {
	ID          int64     `db:"id" yaml:"id"`
	NoteID      int64     `db:"note_id" yaml:"note_id"`
	Link        string    `db:"link" yaml:"link"`
	Description string    `db:"description" yaml:"description"`
	SortOrder   int       `db:"sort_order" yaml:"sort_order"`
	CreatedAt   time.Time `db:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" yaml:"updated_at"`
}

// NotebookNote links a note to a source notebook.
type NotebookNote struct {
	ID           int64     `db:"id" yaml:"id"`
	NoteID       int64     `db:"note_id" yaml:"note_id"`
	NotebookType string    `db:"notebook_type" yaml:"notebook_type"`
	NotebookID   string    `db:"notebook_id" yaml:"notebook_id"`
	Group        string    `db:"group" yaml:"group"`
	Subgroup     string    `db:"subgroup" yaml:"subgroup"`
	CreatedAt    time.Time `db:"created_at" yaml:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" yaml:"updated_at"`
}

// NoteRepository defines operations for managing notes.
type NoteRepository interface {
	FindAll(ctx context.Context) ([]Note, error)
	FindByUsageAndEntry(ctx context.Context, usage, entry string) (*Note, error)
	FindByNotebook(ctx context.Context, notebookType, notebookID string) ([]Note, error)
	FindNotebookNote(ctx context.Context, noteID int64, notebookType, notebookID, group string) (*NotebookNote, error)
	Create(ctx context.Context, note *Note) error
	CreateNotebookNote(ctx context.Context, nn *NotebookNote) error
	Update(ctx context.Context, note *Note) error
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
func (r *DBNoteRepository) FindAll(ctx context.Context) ([]Note, error) {
	var notes []Note
	if err := r.db.SelectContext(ctx, &notes, "SELECT * FROM notes ORDER BY id"); err != nil {
		return nil, fmt.Errorf("db.SelectContext(notes) > %w", err)
	}
	if err := r.loadRelations(ctx, notes); err != nil {
		return nil, err
	}
	return notes, nil
}

// FindByUsageAndEntry returns a note matching the usage and entry, or nil if not found.
func (r *DBNoteRepository) FindByUsageAndEntry(ctx context.Context, usage, entry string) (*Note, error) {
	var n Note
	err := r.db.GetContext(ctx, &n, "SELECT * FROM notes WHERE `usage` = ? AND entry = ?", usage, entry)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db.GetContext(note) > %w", err)
	}
	notes := []Note{n}
	if err := r.loadRelations(ctx, notes); err != nil {
		return nil, err
	}
	return &notes[0], nil
}

// FindByNotebook returns notes linked to a specific notebook.
func (r *DBNoteRepository) FindByNotebook(ctx context.Context, notebookType, notebookID string) ([]Note, error) {
	var notes []Note
	query := `SELECT n.* FROM notes n
		JOIN notebook_notes nn ON n.id = nn.note_id
		WHERE nn.notebook_type = ? AND nn.notebook_id = ?
		ORDER BY n.id`
	if err := r.db.SelectContext(ctx, &notes, query, notebookType, notebookID); err != nil {
		return nil, fmt.Errorf("db.SelectContext(notes by notebook) > %w", err)
	}
	if err := r.loadRelations(ctx, notes); err != nil {
		return nil, err
	}
	return notes, nil
}

// FindNotebookNote returns a notebook_note matching the given criteria, or nil if not found.
func (r *DBNoteRepository) FindNotebookNote(ctx context.Context, noteID int64, notebookType, notebookID, group string) (*NotebookNote, error) {
	var nn NotebookNote
	err := r.db.GetContext(ctx, &nn, "SELECT * FROM notebook_notes WHERE note_id = ? AND notebook_type = ? AND notebook_id = ? AND `group` = ?",
		noteID, notebookType, notebookID, group)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db.GetContext(notebook_note) > %w", err)
	}
	return &nn, nil
}

// CreateNotebookNote inserts a single notebook_note record.
func (r *DBNoteRepository) CreateNotebookNote(ctx context.Context, nn *NotebookNote) error {
	result, err := r.db.ExecContext(ctx,
		"INSERT INTO notebook_notes (note_id, notebook_type, notebook_id, `group`, subgroup) VALUES (?, ?, ?, ?, ?)",
		nn.NoteID, nn.NotebookType, nn.NotebookID, nn.Group, nn.Subgroup)
	if err != nil {
		return fmt.Errorf("db.ExecContext(insert notebook_note) > %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("result.LastInsertId() > %w", err)
	}
	nn.ID = id
	return nil
}

// Create inserts a note with its images, references, and notebook notes in a transaction.
func (r *DBNoteRepository) Create(ctx context.Context, note *Note) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db.BeginTxx() > %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx,
		"INSERT INTO notes (`usage`, entry, meaning, level, dictionary_number) VALUES (?, ?, ?, ?, ?)",
		note.Usage, note.Entry, note.Meaning, note.Level, note.DictionaryNumber)
	if err != nil {
		return fmt.Errorf("tx.ExecContext(insert note) > %w", err)
	}
	noteID, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("result.LastInsertId() > %w", err)
	}
	note.ID = noteID

	for i := range note.Images {
		note.Images[i].NoteID = noteID
		_, err := tx.ExecContext(ctx,
			"INSERT INTO note_images (note_id, url, sort_order) VALUES (?, ?, ?)",
			noteID, note.Images[i].URL, note.Images[i].SortOrder)
		if err != nil {
			return fmt.Errorf("tx.ExecContext(insert note_image) > %w", err)
		}
	}

	for i := range note.References {
		note.References[i].NoteID = noteID
		_, err := tx.ExecContext(ctx,
			"INSERT INTO note_references (note_id, link, description, sort_order) VALUES (?, ?, ?, ?)",
			noteID, note.References[i].Link, note.References[i].Description, note.References[i].SortOrder)
		if err != nil {
			return fmt.Errorf("tx.ExecContext(insert note_reference) > %w", err)
		}
	}

	for i := range note.NotebookNotes {
		note.NotebookNotes[i].NoteID = noteID
		_, err := tx.ExecContext(ctx,
			"INSERT INTO notebook_notes (note_id, notebook_type, notebook_id, `group`, subgroup) VALUES (?, ?, ?, ?, ?)",
			noteID, note.NotebookNotes[i].NotebookType, note.NotebookNotes[i].NotebookID,
			note.NotebookNotes[i].Group, note.NotebookNotes[i].Subgroup)
		if err != nil {
			return fmt.Errorf("tx.ExecContext(insert notebook_note) > %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("tx.Commit() > %w", err)
	}
	return nil
}

// Update updates a note's fields (not related records).
func (r *DBNoteRepository) Update(ctx context.Context, note *Note) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE notes SET `usage` = ?, entry = ?, meaning = ?, level = ?, dictionary_number = ? WHERE id = ?",
		note.Usage, note.Entry, note.Meaning, note.Level, note.DictionaryNumber, note.ID)
	if err != nil {
		return fmt.Errorf("db.ExecContext(update note) > %w", err)
	}
	return nil
}

func (r *DBNoteRepository) loadRelations(ctx context.Context, notes []Note) error {
	if len(notes) == 0 {
		return nil
	}

	noteIDs := make([]int64, len(notes))
	noteMap := make(map[int64]*Note, len(notes))
	for i := range notes {
		noteIDs[i] = notes[i].ID
		noteMap[notes[i].ID] = &notes[i]
	}

	query, args, err := sqlx.In("SELECT * FROM note_images WHERE note_id IN (?) ORDER BY sort_order", noteIDs)
	if err != nil {
		return fmt.Errorf("sqlx.In(note_images) > %w", err)
	}
	var images []NoteImage
	if err := r.db.SelectContext(ctx, &images, r.db.Rebind(query), args...); err != nil {
		return fmt.Errorf("db.SelectContext(note_images) > %w", err)
	}
	for _, img := range images {
		n := noteMap[img.NoteID]
		n.Images = append(n.Images, img)
	}

	query, args, err = sqlx.In("SELECT * FROM note_references WHERE note_id IN (?) ORDER BY sort_order", noteIDs)
	if err != nil {
		return fmt.Errorf("sqlx.In(note_references) > %w", err)
	}
	var refs []NoteReference
	if err := r.db.SelectContext(ctx, &refs, r.db.Rebind(query), args...); err != nil {
		return fmt.Errorf("db.SelectContext(note_references) > %w", err)
	}
	for _, ref := range refs {
		n := noteMap[ref.NoteID]
		n.References = append(n.References, ref)
	}

	query, args, err = sqlx.In("SELECT * FROM notebook_notes WHERE note_id IN (?) ORDER BY id", noteIDs)
	if err != nil {
		return fmt.Errorf("sqlx.In(notebook_notes) > %w", err)
	}
	var nns []NotebookNote
	if err := r.db.SelectContext(ctx, &nns, r.db.Rebind(query), args...); err != nil {
		return fmt.Errorf("db.SelectContext(notebook_notes) > %w", err)
	}
	for _, nn := range nns {
		n := noteMap[nn.NoteID]
		n.NotebookNotes = append(n.NotebookNotes, nn)
	}

	return nil
}
