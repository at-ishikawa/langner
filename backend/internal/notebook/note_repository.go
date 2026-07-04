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
	FindByID(ctx context.Context, id int64) (*NoteRecord, error)
	BatchCreate(ctx context.Context, notes []*NoteRecord) error
	BatchUpdate(ctx context.Context, notes []*NoteRecord, newNotebookNotes []NotebookNote) error
	Create(ctx context.Context, note *NoteRecord) error
	Delete(ctx context.Context, notebookID string, expression string) error
	// BatchDeleteNotes removes notes whose IDs are in the slice along with
	// every dependent row (notebook_notes, learning_logs, note_origin_parts).
	// Used by the importer's reconcile pass.
	BatchDeleteNotes(ctx context.Context, ids []int64) error
	// BatchDeleteNotebookNotes removes specific notebook_notes rows.
	BatchDeleteNotebookNotes(ctx context.Context, ids []int64) error
}

// DBNoteRepository implements NoteRepository using PostgreSQL.
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

// FindByID returns a single note by ID with its notebook notes.
func (r *DBNoteRepository) FindByID(ctx context.Context, id int64) (*NoteRecord, error) {
	var note NoteRecord
	if err := r.db.GetContext(ctx, &note, `SELECT * FROM notes WHERE id = $1`, id); err != nil {
		return nil, fmt.Errorf("find note by id %d: %w", id, err)
	}
	notes := []NoteRecord{note}
	if err := r.loadRelations(ctx, notes); err != nil {
		return nil, err
	}
	return &notes[0], nil
}

// BatchCreate inserts multiple notes with their images, references, and notebook notes in a single transaction.
func (r *DBNoteRepository) BatchCreate(ctx context.Context, notes []*NoteRecord) error {
	if len(notes) == 0 {
		return nil
	}

	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		// Insert notes one at a time so each auto-generated ID can be returned
		// via RETURNING id.
		for _, n := range notes {
			if err := tx.GetContext(ctx, &n.ID,
				`INSERT INTO notes ("usage", entry, meaning, level, dictionary_number, concept_key) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
				n.Usage, n.Entry, n.Meaning, n.Level, n.DictionaryNumber, n.ConceptKey); err != nil {
				return fmt.Errorf("insert note: %w", err)
			}
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
			q := database.BuildMultiRowInsert("note_images", []string{"note_id", "url", "sort_order"}, imgCount)
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
			q := database.BuildMultiRowInsert("note_references", []string{"note_id", "link", "description", "sort_order"}, refCount)
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
			// The DB unique constraint on notebook_notes is
			// (note_id, notebook_type, notebook_id, group) — four fields,
			// no subgroup. The importer's in-memory dedup keys by all
			// five, so a note appearing in multiple scenes of the same
			// story emits multiple candidate rows that collide on the
			// four-field DB key. ON CONFLICT DO NOTHING matches the
			// schema's intent (one row per notebook×group, subgroup is
			// the first scene that lands) without failing the whole
			// import.
			q := database.BuildMultiRowInsert("notebook_notes", []string{"note_id", "notebook_type", "notebook_id", `"group"`, "subgroup"}, nnCount) +
				` ON CONFLICT (note_id, notebook_type, notebook_id, "group") DO NOTHING`
			if _, err := tx.ExecContext(ctx, q, nnArgs...); err != nil {
				return fmt.Errorf("insert notebook notes: %w", err)
			}
		}

		return nil
	})
}

// Create inserts a single note with its notebook_notes in a transaction.
func (r *DBNoteRepository) Create(ctx context.Context, note *NoteRecord) error {
	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		if err := tx.GetContext(ctx, &note.ID,
			`INSERT INTO notes ("usage", entry, meaning, level, dictionary_number, concept_key) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			note.Usage, note.Entry, note.Meaning, note.Level, note.DictionaryNumber, note.ConceptKey); err != nil {
			return fmt.Errorf("insert note: %w", err)
		}

		for _, nn := range note.NotebookNotes {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO notebook_notes (note_id, notebook_type, notebook_id, "group", subgroup) VALUES ($1, $2, $3, $4, $5)`,
				note.ID, nn.NotebookType, nn.NotebookID, nn.Group, nn.Subgroup); err != nil {
				return fmt.Errorf("insert notebook note: %w", err)
			}
		}
		return nil
	})
}

// Delete removes the notebook_notes link for the given notebook and expression.
// If the note has no remaining notebook_notes links, the note itself and its
// related images, references, and learning_logs are also deleted.
func (r *DBNoteRepository) Delete(ctx context.Context, notebookID string, expression string) error {
	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		// Find note IDs linked to this notebook with matching expression
		var noteIDs []int64
		query := `SELECT n.id FROM notes n
			JOIN notebook_notes nn ON n.id = nn.note_id
			WHERE nn.notebook_id = $1 AND LOWER(n."usage") = LOWER($2)`
		if err := tx.SelectContext(ctx, &noteIDs, query, notebookID, expression); err != nil {
			return fmt.Errorf("find notes to delete: %w", err)
		}
		if len(noteIDs) == 0 {
			return nil
		}

		for _, noteID := range noteIDs {
			// Remove the specific notebook_notes link for this notebook
			if _, err := tx.ExecContext(ctx, `DELETE FROM notebook_notes WHERE note_id = $1 AND notebook_id = $2`, noteID, notebookID); err != nil {
				return fmt.Errorf("delete notebook note link: %w", err)
			}

			// Check if the note still has any remaining notebook_notes links
			var remaining int
			if err := tx.GetContext(ctx, &remaining, `SELECT COUNT(*) FROM notebook_notes WHERE note_id = $1`, noteID); err != nil {
				return fmt.Errorf("count remaining notebook notes: %w", err)
			}
			if remaining > 0 {
				continue
			}

			// No remaining links — delete the note and all related data
			if _, err := tx.ExecContext(ctx, `DELETE FROM learning_logs WHERE note_id = $1`, noteID); err != nil {
				return fmt.Errorf("delete learning logs: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM note_images WHERE note_id = $1`, noteID); err != nil {
				return fmt.Errorf("delete note images: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM note_references WHERE note_id = $1`, noteID); err != nil {
				return fmt.Errorf("delete note references: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM notes WHERE id = $1`, noteID); err != nil {
				return fmt.Errorf("delete note: %w", err)
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
				`UPDATE notes SET "usage" = $1, entry = $2, meaning = $3, level = $4, dictionary_number = $5, concept_key = $6 WHERE id = $7`,
				n.Usage, n.Entry, n.Meaning, n.Level, n.DictionaryNumber, n.ConceptKey, n.ID)
			if err != nil {
				return fmt.Errorf("update note: %w", err)
			}
		}

		if len(newNotebookNotes) > 0 {
			var nnArgs []interface{}
			for _, nn := range newNotebookNotes {
				nnArgs = append(nnArgs, nn.NoteID, nn.NotebookType, nn.NotebookID, nn.Group, nn.Subgroup)
			}
			// See BatchCreate for why we skip duplicates on the 4-field
			// unique key instead of failing.
			q := database.BuildMultiRowInsert("notebook_notes", []string{"note_id", "notebook_type", "notebook_id", `"group"`, "subgroup"}, len(newNotebookNotes)) +
				` ON CONFLICT (note_id, notebook_type, notebook_id, "group") DO NOTHING`
			if _, err := tx.ExecContext(ctx, q, nnArgs...); err != nil {
				return fmt.Errorf("insert notebook notes: %w", err)
			}
		}

		return nil
	})
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

// BatchDeleteNotes removes the given notes along with every dependent row
// in note_origin_parts, learning_logs, note_images, note_references, and
// notebook_notes. Used by the importer's reconcile pass to drop DB-only
// notes that no longer have a counterpart in the YAML source of truth.
func (r *DBNoteRepository) BatchDeleteNotes(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	const chunkSize = 5000
	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		for i := 0; i < len(ids); i += chunkSize {
			end := i + chunkSize
			if end > len(ids) {
				end = len(ids)
			}
			chunk := ids[i:end]
			tables := []string{
				"note_origin_parts",
				"learning_logs",
				"note_images",
				"note_references",
				"notebook_notes",
				"notes",
			}
			for _, t := range tables {
				column := "note_id"
				if t == "notes" {
					column = "id"
				}
				query, args, err := sqlx.In("DELETE FROM "+t+" WHERE "+column+" IN (?)", chunk)
				if err != nil {
					return fmt.Errorf("build delete query for %s: %w", t, err)
				}
				if _, err := tx.ExecContext(ctx, tx.Rebind(query), args...); err != nil {
					return fmt.Errorf("delete from %s: %w", t, err)
				}
			}
		}
		return nil
	})
}

// BatchDeleteNotebookNotes removes the specified notebook_notes rows.
// Used by the reconcile pass when a note still exists in YAML but its
// presence in a particular notebook does not.
func (r *DBNoteRepository) BatchDeleteNotebookNotes(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	const chunkSize = 5000
	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		for i := 0; i < len(ids); i += chunkSize {
			end := i + chunkSize
			if end > len(ids) {
				end = len(ids)
			}
			chunk := ids[i:end]
			query, args, err := sqlx.In("DELETE FROM notebook_notes WHERE id IN (?)", chunk)
			if err != nil {
				return fmt.Errorf("build delete query: %w", err)
			}
			if _, err := tx.ExecContext(ctx, tx.Rebind(query), args...); err != nil {
				return fmt.Errorf("delete notebook_notes: %w", err)
			}
		}
		return nil
	})
}
