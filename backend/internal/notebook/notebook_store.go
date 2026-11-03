package notebook

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/database"
)

// NotebookFile is one YAML file of a notebook bundle as pushed or read from
// disk. Path is relative to the family's index.yml (e.g. "index.yml",
// "cards.yml"); Family is the family folder the file belongs under — one
// composite notebook id carries files under several families.
type NotebookFile struct {
	NotebookID string
	Family     string
	Path       string
	Content    []byte
}

// UserNotebook is a row of the notebooks registry: a notebook's owner,
// visibility, provenance and display metadata.
type UserNotebook struct {
	NotebookID  string `db:"notebook_id"`
	OwnerUserID *int64 `db:"owner_user_id"`
	Visibility  string `db:"visibility"`
	// Source is 'user' (CLI push) or 'shipped' (catalog imported id-for-id).
	// PushBundle preserves it on update so re-pushing a shipped notebook
	// doesn't silently reclassify it. Empty defaults to 'user'.
	Source      string `db:"source"`
	Kind        string `db:"kind"`
	DisplayName string `db:"display_name"`
	ContentHash string `db:"content_hash"`
	// ByteSize is the size of the bundle as pushed.
	ByteSize  int   `db:"byte_size"`
	UpdatedAt int64 `db:"updated_at_unix"`
}

// HashBundle returns the sha256 of a bundle's files, order-independent (each
// file's family + path + bytes folded into a sorted digest) so the same bundle
// hashes the same regardless of upload order. Family is part of the key so a
// composite notebook (same path under two families) hashes distinctly. Used for
// content_hash on the notebooks row.
func HashBundle(files []NotebookFile) string {
	type ph struct{ key, hash string }
	phs := make([]ph, 0, len(files))
	for _, f := range files {
		sum := sha256.Sum256(f.Content)
		phs = append(phs, ph{f.Family + "/" + f.Path, hex.EncodeToString(sum[:])})
	}
	sort.Slice(phs, func(i, j int) bool { return phs[i].key < phs[j].key })
	h := sha256.New()
	for _, p := range phs {
		h.Write([]byte(p.key))
		h.Write([]byte{0})
		h.Write([]byte(p.hash))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// BundleBytes is the total size of a bundle's files.
func BundleBytes(files []NotebookFile) int {
	n := 0
	for _, f := range files {
		n += len(f.Content)
	}
	return n
}

// NotebookStore stores whole notebooks: the notebooks registry row (owner,
// visibility, kind, name, hash) and the notebook's content rows, written in one
// transaction so a notebook is never half-stored.
type NotebookStore struct {
	db      *sqlx.DB
	content *ContentRepository
}

// NewNotebookStore constructs the store.
func NewNotebookStore(db *sqlx.DB) *NotebookStore {
	return &NotebookStore{db: db, content: NewContentRepository(db)}
}

// PushBundle stores a learner's pushed notebook: it upserts the registry row
// (owner + visibility + source + kind + display_name + content_hash + size) and
// replaces the notebook's content.
func (r *NotebookStore) PushBundle(ctx context.Context, nb UserNotebook, content NotebookContent) error {
	source := strings.TrimSpace(nb.Source)
	if source == "" {
		source = "user"
	}
	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO notebooks (notebook_id, owner_user_id, visibility, source, kind, display_name, content_hash, content_built_hash, content_bytes)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $7, $8)
			 ON CONFLICT (notebook_id) DO UPDATE SET
			   owner_user_id      = EXCLUDED.owner_user_id,
			   visibility         = EXCLUDED.visibility,
			   source             = EXCLUDED.source,
			   kind               = EXCLUDED.kind,
			   display_name       = EXCLUDED.display_name,
			   content_hash       = EXCLUDED.content_hash,
			   content_built_hash = EXCLUDED.content_built_hash,
			   content_bytes      = EXCLUDED.content_bytes`,
			nb.NotebookID, nb.OwnerUserID, nb.Visibility, source, nb.Kind, nb.DisplayName, nb.ContentHash, nb.ByteSize,
		); err != nil {
			return fmt.Errorf("upsert notebooks row: %w", err)
		}
		return r.content.Store(ctx, tx, content)
	})
}

// ImportShippedBundle stores a catalog notebook under its EXISTING id so
// learning histories keyed by that id survive. It upserts the registry row with
// source='shipped' and the given kind/display_name/content_hash, but PRESERVES
// any existing owner_user_id/visibility (a prior `set-owner` is never
// clobbered); a new row is public. The content is replaced wholesale, so the
// caller passes every family of the notebook at once.
func (r *NotebookStore) ImportShippedBundle(ctx context.Context, kind, displayName, contentHash string, contentBytes int, content NotebookContent) error {
	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO notebooks (notebook_id, owner_user_id, visibility, source, kind, display_name, content_hash, content_built_hash, content_bytes)
			 VALUES ($1, NULL, 'public', 'shipped', $2, $3, $4, $4, $5)
			 ON CONFLICT (notebook_id) DO UPDATE SET
			   source             = 'shipped',
			   kind               = EXCLUDED.kind,
			   display_name       = EXCLUDED.display_name,
			   content_hash       = EXCLUDED.content_hash,
			   content_built_hash = EXCLUDED.content_built_hash,
			   content_bytes      = EXCLUDED.content_bytes`,
			content.ID, kind, displayName, contentHash, contentBytes,
		); err != nil {
			return fmt.Errorf("upsert shipped notebooks row: %w", err)
		}
		return r.content.Store(ctx, tx, content)
	})
}

// ConvertBundle stores the content of a notebook that was stored as raw files,
// keeping its registry row (owner, visibility, source, kind, name, hash) as it
// is and recording builtHash (the stored files' hash) as the hash its rows were
// built from. A missing row is created as a public catalog notebook.
func (r *NotebookStore) ConvertBundle(ctx context.Context, builtHash string, contentBytes int, content NotebookContent) error {
	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO notebooks (notebook_id, content_built_hash, content_bytes) VALUES ($1, $2, $3)
			 ON CONFLICT (notebook_id) DO UPDATE SET
			   content_built_hash = EXCLUDED.content_built_hash,
			   content_bytes      = EXCLUDED.content_bytes`,
			content.ID, builtHash, contentBytes,
		); err != nil {
			return fmt.Errorf("upsert notebooks row: %w", err)
		}
		return r.content.Store(ctx, tx, content)
	})
}

// ContentCurrent reports whether a notebook is stored at bundle hash `hash`
// with content rows built from that same hash, i.e. importing the bundle again
// would change nothing.
func (r *NotebookStore) ContentCurrent(ctx context.Context, notebookID, hash string) (bool, error) {
	var current bool
	err := r.db.GetContext(ctx, &current,
		`SELECT COALESCE(n.content_hash = $2 AND n.content_built_hash = $2, false)
		        AND EXISTS (SELECT 1 FROM content_indexes i WHERE i.notebook_id = n.notebook_id)
		 FROM notebooks n WHERE n.notebook_id = $1`, notebookID, hash)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check content of %s: %w", notebookID, err)
	}
	return current, nil
}

// LoadContent returns a notebook's stored content, or ok=false when it has
// none.
func (r *NotebookStore) LoadContent(ctx context.Context, notebookID string) (NotebookContent, bool, error) {
	contents, err := r.content.Load(ctx, []string{notebookID})
	if err != nil {
		return NotebookContent{}, false, err
	}
	if len(contents) == 0 {
		return NotebookContent{}, false, nil
	}
	return contents[0], true, nil
}

// DeleteUserNotebook removes a notebook: its per-notebook membership links
// (notebook_notes) and its registry row, which deletes its content. It does NOT
// delete `notes` rows — those are shared across notebooks, and an unreferenced
// note is harmless. The caller must have authorized the delete.
func (r *NotebookStore) DeleteUserNotebook(ctx context.Context, notebookID string) error {
	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM notebook_notes WHERE notebook_id = $1`, notebookID); err != nil {
			return fmt.Errorf("delete notebook_notes: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM notebooks WHERE notebook_id = $1`, notebookID); err != nil {
			return fmt.Errorf("delete notebooks row: %w", err)
		}
		return nil
	})
}

const userNotebookColumns = `n.notebook_id, n.owner_user_id, n.visibility,
		        COALESCE(n.kind, '')         AS kind,
		        COALESCE(n.display_name, '') AS display_name,
		        COALESCE(n.content_hash, '') AS content_hash,
		        n.content_bytes              AS byte_size,
		        CAST(EXTRACT(EPOCH FROM n.updated_at) AS BIGINT) AS updated_at_unix`

// ListUserNotebooks returns every notebook owned by ownerUserID — regardless of
// source ('user' pushes AND 'shipped' catalog notebooks the owner claimed).
func (r *NotebookStore) ListUserNotebooks(ctx context.Context, ownerUserID int64) ([]UserNotebook, error) {
	var rows []UserNotebook
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT `+userNotebookColumns+`
		 FROM notebooks n
		 WHERE n.owner_user_id = $1
		 ORDER BY n.updated_at DESC`, ownerUserID); err != nil {
		return nil, fmt.Errorf("list user notebooks: %w", err)
	}
	return rows, nil
}

// GetUserNotebook returns a single source='user' row, or ok=false when absent.
func (r *NotebookStore) GetUserNotebook(ctx context.Context, notebookID string) (UserNotebook, bool, error) {
	var rows []UserNotebook
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT `+userNotebookColumns+`
		 FROM notebooks n WHERE n.notebook_id = $1 AND n.source = 'user'`, notebookID); err != nil {
		return UserNotebook{}, false, fmt.Errorf("get user notebook: %w", err)
	}
	if len(rows) == 0 {
		return UserNotebook{}, false, nil
	}
	return rows[0], true, nil
}

// GetNotebookOwnership returns a notebook's owner, source, and visibility
// regardless of source ('shipped' or 'user'), or found=false when the id has no
// registry row. The push create-or-update path uses it to decide whether a
// declared id is an existing notebook the caller owns (→ update in place,
// preserving source) versus absent / someone else's (→ mint a new id).
func (r *NotebookStore) GetNotebookOwnership(ctx context.Context, notebookID string) (owner *int64, source, visibility string, found bool, err error) {
	var row struct {
		OwnerUserID *int64 `db:"owner_user_id"`
		Source      string `db:"source"`
		Visibility  string `db:"visibility"`
	}
	if gerr := r.db.GetContext(ctx, &row,
		`SELECT owner_user_id, COALESCE(source, '') AS source, visibility
		 FROM notebooks WHERE notebook_id = $1`, notebookID); gerr != nil {
		if errors.Is(gerr, sql.ErrNoRows) {
			return nil, "", "", false, nil
		}
		return nil, "", "", false, fmt.Errorf("get notebook ownership: %w", gerr)
	}
	return row.OwnerUserID, row.Source, row.Visibility, true, nil
}

// FamilyForKind maps a notebook `kind:` to its family; an unknown or empty kind
// falls back to flashcards.
func FamilyForKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "story":
		return FamilyStories
	case "journal":
		return FamilyJournals
	case "grammar":
		return FamilyGrammars
	case "book":
		return FamilyBooks
	case "definitions", "definition":
		return FamilyDefinitions
	case "etymology":
		return FamilyEtymology
	default:
		return FamilyFlashcards
	}
}
