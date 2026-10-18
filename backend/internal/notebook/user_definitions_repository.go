package notebook

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// UserDefinition is one learner's PRIVATE highlight registration on a notebook:
// which word they chose to study, their own authoritative meaning, and where in
// the book (event = notebook_file-or-title key, scene_index). Expression +
// SenseID come from the joined global notes row (note_id), which serves only as
// a stable word identity for per-user learning_logs binding — the meaning here
// is authoritative, NOT notes.meaning (two users may save different meanings for
// the same word). Examples is the decoded []string (stored JSON-encoded).
type UserDefinition struct {
	NoteID       int64
	Event        string
	SceneIndex   int
	Expression   string
	SenseID      string
	Meaning      string
	PartOfSpeech string
	Examples     []string
	// CreatedAt is when THIS user registered the word — the per-user recency
	// signal the quiz-hub list uses to order a shared book (a shared book has no
	// per-user date of its own, so it would otherwise have none).
	CreatedAt time.Time
}

// MeaningSuggestion is a DISTINCT meaning other users privately saved for the
// same (notebook, expression), with how many users saved it. LookupWord returns
// these as prefill suggestions; accepting one still stores the caller's OWN copy.
type MeaningSuggestion struct {
	Meaning      string
	SavedByCount int
}

// UserDefinitionsRepository is the per-user private registration store backing
// RegisterDefinition / DeleteDefinition, the serve-time overlay (reader + quiz),
// and LookupWord's shared suggestions. It is the write+read side of
// user_notebook_definitions (migration 036).
type UserDefinitionsRepository struct {
	db *sqlx.DB
}

// NewUserDefinitionsRepository constructs the repository.
func NewUserDefinitionsRepository(db *sqlx.DB) *UserDefinitionsRepository {
	return &UserDefinitionsRepository{db: db}
}

// Upsert records (or updates in place) one user's private registration of a
// word in a notebook. noteID is the pre-resolved global notes identity row.
// Re-registering the same (user, notebook, note) overwrites the user's own copy
// — never duplicates it (UNIQUE (user_id, notebook_id, note_id)).
func (r *UserDefinitionsRepository) Upsert(ctx context.Context, userID int64, notebookID string, noteID int64, event string, sceneIndex int, meaning, partOfSpeech string, examples []string) error {
	encoded, err := json.Marshal(examples)
	if err != nil {
		return fmt.Errorf("encode examples: %w", err)
	}
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO user_notebook_definitions
		   (user_id, notebook_id, note_id, event, scene_index, meaning, part_of_speech, examples)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (user_id, notebook_id, note_id) DO UPDATE SET
		   event          = EXCLUDED.event,
		   scene_index    = EXCLUDED.scene_index,
		   meaning        = EXCLUDED.meaning,
		   part_of_speech = EXCLUDED.part_of_speech,
		   examples       = EXCLUDED.examples`,
		userID, notebookID, noteID, event, sceneIndex, meaning, partOfSpeech, string(encoded),
	); err != nil {
		return fmt.Errorf("upsert user definition: %w", err)
	}
	return nil
}

// DeleteForUserByExpression removes ONLY the caller's own registration of
// expression in notebookID (scoped by user_id). It resolves the note identity
// from the global notes table by usage/entry, so a caller who passes the
// surface word deletes their private row without needing the note id. Another
// user's registration of the same word is untouched.
func (r *UserDefinitionsRepository) DeleteForUserByExpression(ctx context.Context, userID int64, notebookID, expression string) error {
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM user_notebook_definitions
		 WHERE user_id = $1 AND notebook_id = $2
		   AND note_id IN (
		     SELECT id FROM notes
		     WHERE LOWER("usage") = LOWER($3) OR LOWER(entry) = LOWER($3)
		   )`,
		userID, notebookID, expression,
	); err != nil {
		return fmt.Errorf("delete user definition: %w", err)
	}
	return nil
}

// ListForUser returns one user's registrations for a notebook, joined to the
// global notes row for the word's surface (usage, falling back to entry) and
// sense id. Ordered for deterministic overlay output.
func (r *UserDefinitionsRepository) ListForUser(ctx context.Context, userID int64, notebookID string) ([]UserDefinition, error) {
	type row struct {
		NoteID       int64     `db:"note_id"`
		Event        string    `db:"event"`
		SceneIndex   int       `db:"scene_index"`
		Usage        string    `db:"usage"`
		Entry        string    `db:"entry"`
		SenseID      string    `db:"sense_id"`
		Meaning      string    `db:"meaning"`
		PartOfSpeech string    `db:"part_of_speech"`
		Examples     string    `db:"examples"`
		CreatedAt    time.Time `db:"created_at"`
	}
	var rows []row
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT d.note_id, d.event, d.scene_index,
		        n."usage" AS "usage", n.entry AS entry, COALESCE(n.sense_id, '') AS sense_id,
		        d.meaning, d.part_of_speech, d.examples, d.created_at
		 FROM user_notebook_definitions d
		 JOIN notes n ON n.id = d.note_id
		 WHERE d.user_id = $1 AND d.notebook_id = $2
		 ORDER BY d.event, d.scene_index, d.id`,
		userID, notebookID,
	); err != nil {
		return nil, fmt.Errorf("list user definitions: %w", err)
	}

	out := make([]UserDefinition, 0, len(rows))
	for _, rr := range rows {
		expr := rr.Usage
		if expr == "" {
			expr = rr.Entry
		}
		var examples []string
		if rr.Examples != "" {
			// Best-effort decode; a malformed blob just yields no examples rather
			// than failing the whole serve path.
			_ = json.Unmarshal([]byte(rr.Examples), &examples)
		}
		out = append(out, UserDefinition{
			NoteID:       rr.NoteID,
			Event:        rr.Event,
			SceneIndex:   rr.SceneIndex,
			Expression:   expr,
			SenseID:      rr.SenseID,
			Meaning:      rr.Meaning,
			PartOfSpeech: rr.PartOfSpeech,
			Examples:     examples,
			CreatedAt:    rr.CreatedAt,
		})
	}
	return out, nil
}

// Suggestions returns the DISTINCT meanings saved for expression in notebookID
// across ALL users, each with the number of users who saved it, most-saved
// first. The word identity is resolved through the notes table (usage/entry) so
// a suggestion spans every user who registered the same surface word in the
// book. A wrong suggestion is advisory only — it never enters another user's
// quiz (accepting one still writes the caller's own row).
func (r *UserDefinitionsRepository) Suggestions(ctx context.Context, notebookID, expression string) ([]MeaningSuggestion, error) {
	type row struct {
		Meaning string `db:"meaning"`
		Count   int    `db:"saved_by_count"`
	}
	var rows []row
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT d.meaning AS meaning, COUNT(DISTINCT d.user_id) AS saved_by_count
		 FROM user_notebook_definitions d
		 JOIN notes n ON n.id = d.note_id
		 WHERE d.notebook_id = $1
		   AND (LOWER(n."usage") = LOWER($2) OR LOWER(n.entry) = LOWER($2))
		   AND d.meaning <> ''
		 GROUP BY d.meaning
		 ORDER BY saved_by_count DESC, d.meaning ASC`,
		notebookID, expression,
	); err != nil {
		return nil, fmt.Errorf("query suggestions: %w", err)
	}
	out := make([]MeaningSuggestion, 0, len(rows))
	for _, rr := range rows {
		out = append(out, MeaningSuggestion{Meaning: rr.Meaning, SavedByCount: rr.Count})
	}
	return out, nil
}

// BuildUserDefinitionsMap turns a user's private registrations into a
// DefinitionsMap keyed exactly like the content definitions map
// (notebookID -> event-key -> __index_N -> []Note), so it merges through the
// SAME MergeDefinitionsIntoNotebooks seam, ADDITIVELY on top of any
// content-declared scene.Definitions. Pure (no DB) so the bridge is unit
// testable offline.
func BuildUserDefinitionsMap(notebookID string, rows []UserDefinition) DefinitionsMap {
	if len(rows) == 0 {
		return nil
	}
	m := DefinitionsMap{notebookID: {}}
	for _, row := range rows {
		if row.Event == "" {
			continue
		}
		if m[notebookID][row.Event] == nil {
			m[notebookID][row.Event] = make(map[string][]Note)
		}
		sceneKey := fmt.Sprintf("__index_%d", row.SceneIndex)
		m[notebookID][row.Event][sceneKey] = append(
			m[notebookID][row.Event][sceneKey],
			Note{
				ID:           row.SenseID,
				Expression:   row.Expression,
				Meaning:      row.Meaning,
				PartOfSpeech: row.PartOfSpeech,
				Examples:     ExamplesFromStrings(row.Examples),
			},
		)
	}
	return m
}

// ApplyUserDefinitionsOverlay merges a user's private registrations onto
// already-read story notebooks, additively, through MergeDefinitionsIntoNotebooks.
// notebookPaths is the per-notebook file path slice from
// ReadStoryNotebooksWithPaths; the overlay keys (stored `event`) match either a
// file path (primary) or the event title (fallback), so a registration surfaces
// on the right scene regardless of which the caller stored. Returns notebooks
// unchanged when there are no rows.
func ApplyUserDefinitionsOverlay(notebookID string, notebooks []StoryNotebook, notebookPaths []string, rows []UserDefinition) []StoryNotebook {
	overlay := BuildUserDefinitionsMap(notebookID, rows)
	if overlay == nil {
		return notebooks
	}
	merged := MergeDefinitionsIntoNotebooks(notebookID, notebooks, notebookPaths, overlay)

	// A shared book has no per-user date of its own, so it would sort to the
	// bottom of the quiz-hub list (which orders by latest content date). Give
	// such date-less notebooks THIS user's latest registration time, so a book
	// is ordered by when the user themselves last engaged with it — a per-user
	// signal, not a shared import timestamp. Notebooks that carry an authored
	// date (shipped stories/journals) keep it untouched.
	var latest time.Time
	for _, row := range rows {
		if row.CreatedAt.After(latest) {
			latest = row.CreatedAt
		}
	}
	if !latest.IsZero() {
		for i := range merged {
			if merged[i].Date.IsZero() {
				merged[i].Date = latest
			}
		}
	}
	return merged
}
