package notebook

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// FlashcardDeckRecord mirrors a row of flashcard_decks. Title matches
// `notebook_notes.group` for the deck's cards, so joining the two yields
// "all cards in this deck" without a separate junction table.
type FlashcardDeckRecord struct {
	ID          int64      `db:"id"`
	NotebookID  string     `db:"notebook_id"`
	Title       string     `db:"title"`
	Description string     `db:"description"`
	Date        *time.Time `db:"date"`
	SortOrder   int        `db:"sort_order"`
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"`
}

// FlashcardDeckRepository owns flashcard deck metadata (title,
// description, date) for a flashcard index. The cards themselves stay
// in notes + notebook_notes.
type FlashcardDeckRepository interface {
	// ListByNotebook returns every deck for an index, ordered for stable
	// rendering.
	ListByNotebook(ctx context.Context, notebookID string) ([]FlashcardDeckRecord, error)
	// FindOrCreate returns the deck matching (notebook_id, title),
	// inserting one when missing.
	FindOrCreate(ctx context.Context, notebookID, title, description string, date *time.Time) (*FlashcardDeckRecord, error)
}

// DBFlashcardDeckRepository is the PostgreSQL-backed implementation.
type DBFlashcardDeckRepository struct {
	db *sqlx.DB
}

// NewDBFlashcardDeckRepository constructs the repository.
func NewDBFlashcardDeckRepository(db *sqlx.DB) *DBFlashcardDeckRepository {
	return &DBFlashcardDeckRepository{db: db}
}

const flashcardDeckColumns = `id, notebook_id, title, description, "date", sort_order, created_at, updated_at`

// ListByNotebook returns every deck for a notebook ordered by
// (date, sort_order, id) so the rendering matches the YAML order.
// PostgreSQL sorts NULLs last for ASC by default; NULLS LAST is
// spelled out for clarity.
func (r *DBFlashcardDeckRepository) ListByNotebook(ctx context.Context, notebookID string) ([]FlashcardDeckRecord, error) {
	var rows []FlashcardDeckRecord
	query := `SELECT ` + flashcardDeckColumns + `
		FROM flashcard_decks
		WHERE notebook_id = $1
		ORDER BY "date" NULLS LAST, sort_order, id`
	if err := r.db.SelectContext(ctx, &rows, query, notebookID); err != nil {
		return nil, fmt.Errorf("list flashcard decks: %w", err)
	}
	return rows, nil
}

// FindOrCreate inserts when (notebook_id, title) is missing.
func (r *DBFlashcardDeckRepository) FindOrCreate(ctx context.Context, notebookID, title, description string, date *time.Time) (*FlashcardDeckRecord, error) {
	var deck FlashcardDeckRecord
	err := r.db.GetContext(ctx, &deck,
		`SELECT `+flashcardDeckColumns+`
		 FROM flashcard_decks
		 WHERE notebook_id = $1 AND title = $2`,
		notebookID, title,
	)
	if err == nil {
		return &deck, nil
	}

	if err := r.db.GetContext(ctx, &deck,
		`INSERT INTO flashcard_decks (notebook_id, title, description, "date") VALUES ($1, $2, $3, $4)
		 RETURNING `+flashcardDeckColumns,
		notebookID, title, description, date,
	); err != nil {
		return nil, fmt.Errorf("insert flashcard deck: %w", err)
	}
	return &deck, nil
}
