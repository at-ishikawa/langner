package notebook

import "time"

// NoteRecord represents a vocabulary word or phrase in the database.
type NoteRecord struct {
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
