package notebook

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/database"
)

// NoteRepository defines operations for managing notes.
type NoteRepository interface {
	FindAll(ctx context.Context) ([]NoteRecord, error)
	BatchCreate(ctx context.Context, notes []*NoteRecord) error
	BatchUpdate(ctx context.Context, notes []*NoteRecord, newNotebookNotes []NotebookNote) error
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

// BatchCreate inserts multiple notes with their images, references, and notebook notes in a single transaction.
func (r *DBNoteRepository) BatchCreate(ctx context.Context, notes []*NoteRecord) error {
	if len(notes) == 0 {
		return nil
	}

	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		// Build multi-row INSERT for notes
		query := buildMultiRowInsert(
			"notes",
			[]string{"`usage`", "entry", "meaning", "level", "dictionary_number"},
			len(notes),
		)
		var args []interface{}
		for _, n := range notes {
			args = append(args, n.Usage, n.Entry, n.Meaning, n.Level, n.DictionaryNumber)
		}
		result, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("insert notes: %w", err)
		}
		// MySQL guarantees consecutive auto-increment IDs for multi-row INSERT
		// when innodb_autoinc_lock_mode <= 1 (consecutive or traditional mode).
		// This is the default for MySQL 5.x and common for MySQL 8.0+.
		firstID, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("get notes insert ID: %w", err)
		}
		for i := range notes {
			notes[i].ID = firstID + int64(i)
		}

		// Collect all images
		var imgArgs []interface{}
		var imgCount int
		for _, n := range notes {
			for i := range n.Images {
				n.Images[i].NoteID = n.ID
				imgArgs = append(imgArgs, n.Images[i].NoteID, n.Images[i].URL, n.Images[i].SortOrder)
				imgCount++
			}
		}
		if imgCount > 0 {
			q := buildMultiRowInsert("note_images", []string{"note_id", "url", "sort_order"}, imgCount)
			if _, err := tx.ExecContext(ctx, q, imgArgs...); err != nil {
				return fmt.Errorf("insert note images: %w", err)
			}
		}

		// Collect all references
		var refArgs []interface{}
		var refCount int
		for _, n := range notes {
			for i := range n.References {
				n.References[i].NoteID = n.ID
				refArgs = append(refArgs, n.References[i].NoteID, n.References[i].Link, n.References[i].Description, n.References[i].SortOrder)
				refCount++
			}
		}
		if refCount > 0 {
			q := buildMultiRowInsert("note_references", []string{"note_id", "link", "description", "sort_order"}, refCount)
			if _, err := tx.ExecContext(ctx, q, refArgs...); err != nil {
				return fmt.Errorf("insert note references: %w", err)
			}
		}

		// Collect all notebook_notes
		var nnArgs []interface{}
		var nnCount int
		for _, n := range notes {
			for i := range n.NotebookNotes {
				n.NotebookNotes[i].NoteID = n.ID
				nn := n.NotebookNotes[i]
				nnArgs = append(nnArgs, nn.NoteID, nn.NotebookType, nn.NotebookID, nn.Group, nn.Subgroup)
				nnCount++
			}
		}
		if nnCount > 0 {
			q := buildMultiRowInsert("notebook_notes", []string{"note_id", "notebook_type", "notebook_id", "`group`", "subgroup"}, nnCount)
			if _, err := tx.ExecContext(ctx, q, nnArgs...); err != nil {
				return fmt.Errorf("insert notebook notes: %w", err)
			}
		}

		return nil
	})
}

// BatchUpdate updates note fields and inserts new notebook_note links in a single transaction.
func (r *DBNoteRepository) BatchUpdate(ctx context.Context, notes []*NoteRecord, newNotebookNotes []NotebookNote) error {
	if len(notes) == 0 && len(newNotebookNotes) == 0 {
		return nil
	}

	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		for _, n := range notes {
			_, err := tx.ExecContext(ctx,
				"UPDATE notes SET `usage` = ?, entry = ?, meaning = ?, level = ?, dictionary_number = ? WHERE id = ?",
				n.Usage, n.Entry, n.Meaning, n.Level, n.DictionaryNumber, n.ID)
			if err != nil {
				return fmt.Errorf("update note: %w", err)
			}
		}

		if len(newNotebookNotes) > 0 {
			var nnArgs []interface{}
			for _, nn := range newNotebookNotes {
				nnArgs = append(nnArgs, nn.NoteID, nn.NotebookType, nn.NotebookID, nn.Group, nn.Subgroup)
			}
			q := buildMultiRowInsert("notebook_notes", []string{"note_id", "notebook_type", "notebook_id", "`group`", "subgroup"}, len(newNotebookNotes))
			if _, err := tx.ExecContext(ctx, q, nnArgs...); err != nil {
				return fmt.Errorf("insert notebook notes: %w", err)
			}
		}

		return nil
	})
}

// buildMultiRowInsert builds a multi-row INSERT query.
func buildMultiRowInsert(table string, columns []string, rowCount int) string {
	placeholder := "(" + strings.Repeat("?, ", len(columns)-1) + "?)"
	values := strings.Repeat(placeholder+", ", rowCount-1) + placeholder
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", table, strings.Join(columns, ", "), values)
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
