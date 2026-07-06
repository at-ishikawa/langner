package notebook

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// DefinitionsSessionRecord mirrors a row of definitions_sessions. One row
// per session in a definitions notebook.
type DefinitionsSessionRecord struct {
	ID           int64      `db:"id"`
	NotebookID   string     `db:"notebook_id"`
	Title        string     `db:"title"`
	NotebookFile string     `db:"notebook_file"`
	Date         *time.Time `db:"date"`
	SortOrder    int        `db:"sort_order"`
	CreatedAt    time.Time  `db:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at"`
}

// DefinitionsSceneRecord mirrors a row of definitions_scenes.
type DefinitionsSceneRecord struct {
	ID         int64     `db:"id"`
	SessionID  int64     `db:"session_id"`
	Title      string    `db:"title"`
	SceneIndex int       `db:"scene_index"`
	SortOrder  int       `db:"sort_order"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

// DefinitionsRepository owns the per-notebook session/scene structure
// that used to live in definitions YAML. The notes themselves keep
// living in `notes` + `notebook_notes` — only the structural metadata
// (session titles, scene titles, ordering, dates) is in these tables.
type DefinitionsRepository interface {
	// ListSessions returns every session for a notebook, ordered by
	// (date, sort_order, id) — same order the YAML reader produced.
	ListSessions(ctx context.Context, notebookID string) ([]DefinitionsSessionRecord, error)
	// ListScenes returns every scene for the given session IDs.
	ListScenes(ctx context.Context, sessionIDs []int64) ([]DefinitionsSceneRecord, error)
	// FindOrCreateSession returns the session matching
	// (notebook_id, title, notebook_file), inserting one when missing.
	// Called by RegisterDefinition so adding a definition through the UI
	// can land in a brand-new session.
	FindOrCreateSession(ctx context.Context, notebookID, title, notebookFile string, date *time.Time) (*DefinitionsSessionRecord, error)
	// FindOrCreateScene returns the scene matching (session_id, scene_index),
	// inserting one when missing. Scene titles are updated when the caller
	// supplies a non-empty title and the existing row has an empty title.
	FindOrCreateScene(ctx context.Context, sessionID int64, sceneIndex int, title string) (*DefinitionsSceneRecord, error)
}

// DBDefinitionsRepository is the PostgreSQL-backed implementation.
type DBDefinitionsRepository struct {
	db *sqlx.DB
}

// NewDBDefinitionsRepository constructs the repository.
func NewDBDefinitionsRepository(db *sqlx.DB) *DBDefinitionsRepository {
	return &DBDefinitionsRepository{db: db}
}

const definitionsSessionColumns = `id, notebook_id, title, notebook_file, "date", sort_order, created_at, updated_at`
const definitionsSceneColumns = `id, session_id, title, scene_index, sort_order, created_at, updated_at`

// ListSessions returns every session for notebookID.
func (r *DBDefinitionsRepository) ListSessions(ctx context.Context, notebookID string) ([]DefinitionsSessionRecord, error) {
	var rows []DefinitionsSessionRecord
	query := `SELECT ` + definitionsSessionColumns + `
		FROM definitions_sessions
		WHERE notebook_id = $1
		ORDER BY "date" NULLS LAST, sort_order, id`
	if err := r.db.SelectContext(ctx, &rows, query, notebookID); err != nil {
		return nil, fmt.Errorf("list definitions sessions: %w", err)
	}
	return rows, nil
}

// ListScenes returns every scene under the given session IDs.
func (r *DBDefinitionsRepository) ListScenes(ctx context.Context, sessionIDs []int64) ([]DefinitionsSceneRecord, error) {
	if len(sessionIDs) == 0 {
		return nil, nil
	}
	query, args, err := sqlx.In(
		`SELECT `+definitionsSceneColumns+`
		 FROM definitions_scenes
		 WHERE session_id IN (?)
		 ORDER BY session_id, sort_order, scene_index, id`,
		sessionIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("build list definitions scenes: %w", err)
	}
	var rows []DefinitionsSceneRecord
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("list definitions scenes: %w", err)
	}
	return rows, nil
}

// FindOrCreateSession inserts when the (notebook_id, title, notebook_file)
// tuple is missing and returns the resulting row either way.
func (r *DBDefinitionsRepository) FindOrCreateSession(ctx context.Context, notebookID, title, notebookFile string, date *time.Time) (*DefinitionsSessionRecord, error) {
	var session DefinitionsSessionRecord
	err := r.db.GetContext(ctx, &session,
		`SELECT `+definitionsSessionColumns+`
		 FROM definitions_sessions
		 WHERE notebook_id = $1 AND title = $2 AND notebook_file = $3`,
		notebookID, title, notebookFile,
	)
	if err == nil {
		return &session, nil
	}

	if err := r.db.GetContext(ctx, &session,
		`INSERT INTO definitions_sessions (notebook_id, title, notebook_file, "date") VALUES ($1, $2, $3, $4)
		 RETURNING `+definitionsSessionColumns,
		notebookID, title, notebookFile, date,
	); err != nil {
		return nil, fmt.Errorf("insert definitions session: %w", err)
	}
	return &session, nil
}

// FindOrCreateScene inserts when (session_id, scene_index) is missing.
// When the existing row has an empty title and the caller supplies one,
// the title is filled in — saves a follow-up update path.
func (r *DBDefinitionsRepository) FindOrCreateScene(ctx context.Context, sessionID int64, sceneIndex int, title string) (*DefinitionsSceneRecord, error) {
	var scene DefinitionsSceneRecord
	err := r.db.GetContext(ctx, &scene,
		`SELECT `+definitionsSceneColumns+`
		 FROM definitions_scenes
		 WHERE session_id = $1 AND scene_index = $2`,
		sessionID, sceneIndex,
	)
	if err == nil {
		if scene.Title == "" && title != "" {
			if _, uerr := r.db.ExecContext(ctx,
				`UPDATE definitions_scenes SET title = $1 WHERE id = $2`, title, scene.ID,
			); uerr != nil {
				return nil, fmt.Errorf("update scene title: %w", uerr)
			}
			scene.Title = title
		}
		return &scene, nil
	}

	if err := r.db.GetContext(ctx, &scene,
		`INSERT INTO definitions_scenes (session_id, title, scene_index) VALUES ($1, $2, $3)
		 RETURNING `+definitionsSceneColumns,
		sessionID, title, sceneIndex,
	); err != nil {
		return nil, fmt.Errorf("insert definitions scene: %w", err)
	}
	return &scene, nil
}
