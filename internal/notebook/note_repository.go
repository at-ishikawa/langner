package notebook

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// NoteRepository defines operations for managing notes.
type NoteRepository interface {
	FindAll(ctx context.Context) ([]NoteRecord, error)
	FindByUsageAndEntry(ctx context.Context, usage, entry string) (*NoteRecord, error)
	FindByNotebook(ctx context.Context, notebookType, notebookID string) ([]NoteRecord, error)
	FindNotebookNote(ctx context.Context, noteID int64, notebookType, notebookID, group string) (*NotebookNote, error)
	Create(ctx context.Context, note *NoteRecord) error
	BatchCreate(ctx context.Context, notes []*NoteRecord) error
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
		return nil, fmt.Errorf("db.SelectContext(notes) > %w", err)
	}
	if err := r.loadRelations(ctx, notes); err != nil {
		return nil, err
	}
	return notes, nil
}

// FindByUsageAndEntry returns a note matching the usage and entry, or nil if not found.
func (r *DBNoteRepository) FindByUsageAndEntry(ctx context.Context, usage, entry string) (*NoteRecord, error) {
	var n NoteRecord
	err := r.db.GetContext(ctx, &n, "SELECT * FROM notes WHERE `usage` = ? AND entry = ?", usage, entry)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db.GetContext(note) > %w", err)
	}
	notes := []NoteRecord{n}
	if err := r.loadRelations(ctx, notes); err != nil {
		return nil, err
	}
	return &notes[0], nil
}

// FindByNotebook returns notes linked to a specific notebook.
func (r *DBNoteRepository) FindByNotebook(ctx context.Context, notebookType, notebookID string) ([]NoteRecord, error) {
	var notes []NoteRecord
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
func (r *DBNoteRepository) Create(ctx context.Context, note *NoteRecord) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db.BeginTxx() > %w", err)
	}
	defer func() { _ = tx.Rollback() }()

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

// BatchCreate inserts multiple simple notes (no images/references/notebook_notes).
func (r *DBNoteRepository) BatchCreate(ctx context.Context, notes []*NoteRecord) error {
	if len(notes) == 0 {
		return nil
	}
	const batchSize = 100
	for i := 0; i < len(notes); i += batchSize {
		end := i + batchSize
		if end > len(notes) {
			end = len(notes)
		}
		batch := notes[i:end]

		query := "INSERT INTO notes (`usage`, entry, meaning, level, dictionary_number) VALUES "
		args := make([]interface{}, 0, len(batch)*5)
		for j, n := range batch {
			if j > 0 {
				query += ", "
			}
			query += "(?, ?, ?, ?, ?)"
			args = append(args, n.Usage, n.Entry, n.Meaning, n.Level, n.DictionaryNumber)
		}
		if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("db.ExecContext(batch insert notes) > %w", err)
		}
	}
	return nil
}

// Update updates a note's fields (not related records).
func (r *DBNoteRepository) Update(ctx context.Context, note *NoteRecord) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE notes SET `usage` = ?, entry = ?, meaning = ?, level = ?, dictionary_number = ? WHERE id = ?",
		note.Usage, note.Entry, note.Meaning, note.Level, note.DictionaryNumber, note.ID)
	if err != nil {
		return fmt.Errorf("db.ExecContext(update note) > %w", err)
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
