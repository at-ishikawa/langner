package notebook

import "time"

// NoteRecord represents a vocabulary word or phrase in the database.
type NoteRecord struct {
	ID int64 `db:"id"`
	// SenseID is the stable source-entry identity (Note.ID). It is the
	// canonical key for the note's identity; ID above is the DB serial.
	SenseID          string `db:"sense_id"`
	Usage            string `db:"usage"`
	Entry            string `db:"entry"`
	Meaning          string `db:"meaning"`
	Level            string `db:"level"`
	DictionaryNumber int    `db:"dictionary_number"`
	// ConceptKey is the head expression of the definitions concept this
	// note belongs to, or "" when it doesn't belong to a concept. Set at
	// ingestion time from the parsed concepts: block; populated only for
	// definitions-side notes.
	ConceptKey    string          `db:"concept_key"`
	CreatedAt     time.Time       `db:"created_at"`
	UpdatedAt     time.Time       `db:"updated_at"`
	SkippedAt     *time.Time      `db:"skipped_at"`
	Images        []NoteImage     `db:"-"`
	References    []NoteReference `db:"-"`
	NotebookNotes []NotebookNote  `db:"-"`

	DefinitionsDir string   `db:"-"`
	NotebookFile   string   `db:"-"`
	SceneIndex     int      `db:"-"`
	PartOfSpeech   string   `db:"-"`
	Examples       []string `db:"-"`
}

// NoteImage represents an image link for visual vocabulary learning.
type NoteImage struct {
	ID        int64     `db:"id"`
	NoteID    int64     `db:"note_id"`
	URL       string    `db:"url"`
	SortOrder int       `db:"sort_order"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// NoteReference represents an external reference for a note.
type NoteReference struct {
	ID          int64     `db:"id"`
	NoteID      int64     `db:"note_id"`
	Link        string    `db:"link"`
	Description string    `db:"description"`
	SortOrder   int       `db:"sort_order"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

// NotebookNote links a note to a source notebook.
type NotebookNote struct {
	ID           int64     `db:"id"`
	NoteID       int64     `db:"note_id"`
	NotebookType string    `db:"notebook_type"`
	NotebookID   string    `db:"notebook_id"`
	Group        string    `db:"group"`
	Subgroup     string    `db:"subgroup"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}
