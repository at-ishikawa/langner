package notebook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
)

// NotebookFile is one raw YAML file of a stored notebook bundle. Path is
// relative to the notebook's directory root within its family (e.g. "index.yml",
// "cards.yml") so the stored layout resolves index.yml's `notebooks:` references
// against sibling blobs exactly as the filesystem walk resolves them against a
// directory. Family is the ContentDirs bucket the file belongs under
// (stories|flashcards|books|definitions|etymology|journals|grammars) — a single
// composite notebook id carries files under several families (design §13).
type NotebookFile struct {
	NotebookID    string `db:"notebook_id"`
	Family        string `db:"family"`
	Path          string `db:"path"`
	Content       []byte `db:"content"`
	ContentSHA256 string `db:"content_sha256"`
	ByteSize      int    `db:"byte_size"`
}

// UserNotebook is a row of the notebooks overlay whose source='user' — a
// notebook a learner pushed via the CLI. It is the metadata the reader and the
// `notebooks list` command need without parsing every blob.
type UserNotebook struct {
	NotebookID  string `db:"notebook_id"`
	OwnerUserID *int64 `db:"owner_user_id"`
	Visibility  string `db:"visibility"`
	Kind        string `db:"kind"`
	DisplayName string `db:"display_name"`
	ContentHash string `db:"content_hash"`
	ByteSize    int    `db:"byte_size"`
	UpdatedAt   int64  `db:"updated_at_unix"`
}

// HashBundle returns the sha256 of a bundle's files, order-independent (each
// file's family + path + bytes folded into a sorted digest) so the same bundle
// hashes the same regardless of upload order. Family is part of the key so a
// composite notebook (same path under two families) hashes distinctly. Used for
// content_hash on the notebooks row and to gate no-op re-pushes.
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

// FileSHA256 returns the hex sha256 of a single file's bytes.
func FileSHA256(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// NotebookFileRepository is the DB store for user-notebook content blobs plus
// the source='user' registry rows on the notebooks overlay. It is the write
// side of PushNotebook and the read side DBContentSource / PullNotebook /
// ListMyNotebooks consult.
type NotebookFileRepository struct {
	db *sqlx.DB
}

// NewNotebookFileRepository constructs the repository.
func NewNotebookFileRepository(db *sqlx.DB) *NotebookFileRepository {
	return &NotebookFileRepository{db: db}
}

// PushBundle writes a whole user-notebook bundle transactionally: it upserts
// the source='user' notebooks registry row (owner + visibility + kind +
// display_name + content_hash) and replaces the notebook_files blobs for the
// id. Replacing (delete-then-insert) makes a re-push idempotent and keeps the
// stored files exactly matching the pushed set. The registry row is written in
// the SAME transaction so a notebook is never half-registered.
func (r *NotebookFileRepository) PushBundle(ctx context.Context, nb UserNotebook, files []NotebookFile) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO notebooks (notebook_id, owner_user_id, visibility, source, kind, display_name, content_hash)
		 VALUES ($1, $2, $3, 'user', $4, $5, $6)
		 ON CONFLICT (notebook_id) DO UPDATE SET
		   owner_user_id = EXCLUDED.owner_user_id,
		   visibility    = EXCLUDED.visibility,
		   source        = 'user',
		   kind          = EXCLUDED.kind,
		   display_name  = EXCLUDED.display_name,
		   content_hash  = EXCLUDED.content_hash`,
		nb.NotebookID, nb.OwnerUserID, nb.Visibility, nb.Kind, nb.DisplayName, nb.ContentHash,
	); err != nil {
		return fmt.Errorf("upsert notebooks row: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM notebook_files WHERE notebook_id = $1`, nb.NotebookID); err != nil {
		return fmt.Errorf("clear old notebook_files: %w", err)
	}
	for _, f := range files {
		family := f.Family
		if strings.TrimSpace(family) == "" {
			family = familyDir(nb.Kind) // single-kind push: every file in the notebook's one family
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO notebook_files (notebook_id, family, path, content, content_sha256, byte_size)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			nb.NotebookID, family, f.Path, f.Content, FileSHA256(f.Content), len(f.Content),
		); err != nil {
			return fmt.Errorf("insert notebook_file %s: %w", f.Path, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit push: %w", err)
	}
	return nil
}

// ImportShippedBundle stores a filesystem-catalog notebook under its EXISTING
// id (no nb_ minting) so learning histories keyed by that id survive (design
// §13.3). It upserts the notebooks registry row with source='shipped' and the
// given primary kind/display_name/content_hash, but PRESERVES any existing
// owner_user_id/visibility — a prior `set-owner` (e.g. a private row) is never
// clobbered by an import; a brand-new row defaults to public. Blobs for the id
// are replaced wholesale (all families at once), so the caller MUST pass every
// family's files for the id in one call.
func (r *NotebookFileRepository) ImportShippedBundle(ctx context.Context, notebookID, kind, displayName, contentHash string, files []NotebookFile) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO notebooks (notebook_id, owner_user_id, visibility, source, kind, display_name, content_hash)
		 VALUES ($1, NULL, 'public', 'shipped', $2, $3, $4)
		 ON CONFLICT (notebook_id) DO UPDATE SET
		   source        = 'shipped',
		   kind          = EXCLUDED.kind,
		   display_name  = EXCLUDED.display_name,
		   content_hash  = EXCLUDED.content_hash`,
		notebookID, kind, displayName, contentHash,
	); err != nil {
		return fmt.Errorf("upsert shipped notebooks row: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM notebook_files WHERE notebook_id = $1`, notebookID); err != nil {
		return fmt.Errorf("clear old notebook_files: %w", err)
	}
	for _, f := range files {
		family := f.Family
		if strings.TrimSpace(family) == "" {
			family = familyDir(kind)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO notebook_files (notebook_id, family, path, content, content_sha256, byte_size)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			notebookID, family, f.Path, f.Content, FileSHA256(f.Content), len(f.Content),
		); err != nil {
			return fmt.Errorf("insert notebook_file %s/%s: %w", family, f.Path, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit import: %w", err)
	}
	return nil
}

// ListFiles returns every stored file for a notebook id (across all its
// families), ordered by family then path.
func (r *NotebookFileRepository) ListFiles(ctx context.Context, notebookID string) ([]NotebookFile, error) {
	var files []NotebookFile
	if err := r.db.SelectContext(ctx, &files,
		`SELECT notebook_id, family, path, content, content_sha256, byte_size
		 FROM notebook_files WHERE notebook_id = $1 ORDER BY family, path`, notebookID); err != nil {
		return nil, fmt.Errorf("list notebook_files: %w", err)
	}
	return files, nil
}

// ListContentNotebookIDs returns every notebook id that has stored blobs,
// regardless of source ('shipped' imported catalog + 'user' pushes). This is
// the enumeration DBContentSource materializes — presence of blobs, not the
// notebooks.kind, decides what the DB serves (design §13.4).
func (r *NotebookFileRepository) ListContentNotebookIDs(ctx context.Context) ([]string, error) {
	var ids []string
	if err := r.db.SelectContext(ctx, &ids,
		`SELECT DISTINCT notebook_id FROM notebook_files ORDER BY notebook_id`); err != nil {
		return nil, fmt.Errorf("list content notebook ids: %w", err)
	}
	return ids, nil
}

// ListUserNotebooks returns every source='user' notebook owned by ownerUserID,
// with a byte_size summed from its files and an updated_at unix timestamp.
func (r *NotebookFileRepository) ListUserNotebooks(ctx context.Context, ownerUserID int64) ([]UserNotebook, error) {
	var rows []UserNotebook
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT n.notebook_id, n.owner_user_id, n.visibility,
		        COALESCE(n.kind, '')         AS kind,
		        COALESCE(n.display_name, '') AS display_name,
		        COALESCE(n.content_hash, '') AS content_hash,
		        COALESCE((SELECT SUM(byte_size) FROM notebook_files f WHERE f.notebook_id = n.notebook_id), 0) AS byte_size,
		        CAST(EXTRACT(EPOCH FROM n.updated_at) AS BIGINT) AS updated_at_unix
		 FROM notebooks n
		 WHERE n.source = 'user' AND n.owner_user_id = $1
		 ORDER BY n.updated_at DESC`, ownerUserID); err != nil {
		return nil, fmt.Errorf("list user notebooks: %w", err)
	}
	return rows, nil
}

// ListAllUserNotebooks returns every source='user' notebook regardless of
// owner. DBContentSource uses it to enumerate all user notebooks for the
// process-wide reader; visibility is enforced downstream by the same
// VisibleNotebookIDs predicate shipped notebooks use (defense in depth).
func (r *NotebookFileRepository) ListAllUserNotebooks(ctx context.Context) ([]UserNotebook, error) {
	var rows []UserNotebook
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT n.notebook_id, n.owner_user_id, n.visibility,
		        COALESCE(n.kind, '')         AS kind,
		        COALESCE(n.display_name, '') AS display_name,
		        COALESCE(n.content_hash, '') AS content_hash,
		        0 AS byte_size,
		        CAST(EXTRACT(EPOCH FROM n.updated_at) AS BIGINT) AS updated_at_unix
		 FROM notebooks n
		 WHERE n.source = 'user'
		 ORDER BY n.notebook_id`); err != nil {
		return nil, fmt.Errorf("list all user notebooks: %w", err)
	}
	return rows, nil
}

// GetUserNotebook returns a single source='user' row, or ok=false when absent.
func (r *NotebookFileRepository) GetUserNotebook(ctx context.Context, notebookID string) (UserNotebook, bool, error) {
	var rows []UserNotebook
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT n.notebook_id, n.owner_user_id, n.visibility,
		        COALESCE(n.kind, '')         AS kind,
		        COALESCE(n.display_name, '') AS display_name,
		        COALESCE(n.content_hash, '') AS content_hash,
		        0 AS byte_size,
		        CAST(EXTRACT(EPOCH FROM n.updated_at) AS BIGINT) AS updated_at_unix
		 FROM notebooks n WHERE n.notebook_id = $1 AND n.source = 'user'`, notebookID); err != nil {
		return UserNotebook{}, false, fmt.Errorf("get user notebook: %w", err)
	}
	if len(rows) == 0 {
		return UserNotebook{}, false, nil
	}
	return rows[0], true, nil
}

// Fingerprint returns a stable digest of ALL stored notebook content (every
// blob's id + family + path + per-file hash), so DBContentSource can cheaply
// detect when a push OR a filesystem re-import changed the materialized content
// and needs a refresh. Computed straight from notebook_files so it covers both
// 'shipped' and 'user' notebooks.
func (r *NotebookFileRepository) Fingerprint(ctx context.Context) (string, error) {
	type fp struct {
		NotebookID    string `db:"notebook_id"`
		Family        string `db:"family"`
		Path          string `db:"path"`
		ContentSHA256 string `db:"content_sha256"`
	}
	var rows []fp
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT notebook_id, family, path, content_sha256
		 FROM notebook_files ORDER BY notebook_id, family, path`); err != nil {
		return "", fmt.Errorf("fingerprint notebook_files: %w", err)
	}
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(r.NotebookID)
		b.WriteByte('\n')
		b.WriteString(r.Family)
		b.WriteByte('\n')
		b.WriteString(r.Path)
		b.WriteByte('\n')
		b.WriteString(r.ContentSHA256)
		b.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:]), nil
}
