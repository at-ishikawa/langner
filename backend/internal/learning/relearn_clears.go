package learning

import (
	"context"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
)

// RelearnClearStore persists "relearn clears" — a lightweight marker that a
// word was recovered in a Relearn Quiz session. It is deliberately NOT a
// learning log: it carries no quiz type, status, quality, interval, or
// easiness factor, and is never read by SM-2 or analytics. Its only reader is
// the Relearn pool builder, which excludes a word when its clear time is newer
// than the word's most-recent in-window wrong log.
//
// Keys are opaque strings owned by the caller (the quiz package builds them
// from the notebook name and expression); this store never interprets them.
type RelearnClearStore interface {
	// AllClears returns every recorded clear keyed by clear_key. The Relearn
	// pool builder reads the whole set once per session start.
	AllClears(ctx context.Context) (map[string]time.Time, error)
	// MarkCleared upserts the latest clear time for a key.
	MarkCleared(ctx context.Context, clearKey string, at time.Time) error
}

// MemoryRelearnClearStore keeps clears in a process-local map. It is used when
// no database is configured (YAML-only storage). It is intentionally
// ephemeral: a restart drops the markers, at which point words age out of the
// Relearn pool on their own via the rolling look-back window.
type MemoryRelearnClearStore struct {
	mu     sync.RWMutex
	clears map[string]time.Time
}

// NewMemoryRelearnClearStore returns an empty in-memory clear store.
func NewMemoryRelearnClearStore() *MemoryRelearnClearStore {
	return &MemoryRelearnClearStore{clears: make(map[string]time.Time)}
}

func (s *MemoryRelearnClearStore) AllClears(_ context.Context) (map[string]time.Time, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]time.Time, len(s.clears))
	for k, v := range s.clears {
		out[k] = v
	}
	return out, nil
}

func (s *MemoryRelearnClearStore) MarkCleared(_ context.Context, clearKey string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.clears[clearKey]; !ok || at.After(existing) {
		s.clears[clearKey] = at
	}
	return nil
}

// DBRelearnClearStore persists clears in the relearn_clears table. It is used
// when a database is configured. The table has no foreign key and no
// learning-log columns — it is not part of the learning-history schema.
type DBRelearnClearStore struct {
	db *sqlx.DB
}

// NewDBRelearnClearStore returns a clear store backed by the relearn_clears table.
func NewDBRelearnClearStore(db *sqlx.DB) *DBRelearnClearStore {
	return &DBRelearnClearStore{db: db}
}

func (s *DBRelearnClearStore) AllClears(ctx context.Context) (map[string]time.Time, error) {
	var rows []struct {
		ClearKey  string    `db:"clear_key"`
		ClearedAt time.Time `db:"cleared_at"`
	}
	if err := s.db.SelectContext(ctx, &rows, `SELECT clear_key, cleared_at FROM relearn_clears`); err != nil {
		return nil, err
	}
	out := make(map[string]time.Time, len(rows))
	for _, r := range rows {
		out[r.ClearKey] = r.ClearedAt
	}
	return out, nil
}

func (s *DBRelearnClearStore) MarkCleared(ctx context.Context, clearKey string, at time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO relearn_clears (clear_key, cleared_at) VALUES ($1, $2)
		 ON CONFLICT (clear_key) DO UPDATE SET cleared_at = EXCLUDED.cleared_at`,
		clearKey, at)
	return err
}
