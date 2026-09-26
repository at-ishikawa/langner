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

// NotebookFile is one raw YAML file of a user-pushed notebook bundle. Path is
// relative to the pushed directory root (e.g. "index.yml", "cards.yml") so the
// stored layout resolves index.yml's `notebooks:` references against sibling
// blobs exactly as the filesystem walk resolves them against a directory.
type NotebookFile struct {
	NotebookID    string `db:"notebook_id"`
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
// file's path + bytes folded into a sorted digest) so the same bundle hashes
// the same regardless of upload order. Used for content_hash on the notebooks
// row and to gate no-op re-pushes.
func HashBundle(files []NotebookFile) string {
	type ph struct{ path, hash string }
	phs := make([]ph, 0, len(files))
	for _, f := range files {
		sum := sha256.Sum256(f.Content)
		phs = append(phs, ph{f.Path, hex.EncodeToString(sum[:])})
	}
	sort.Slice(phs, func(i, j int) bool { return phs[i].path < phs[j].path })
	h := sha256.New()
	for _, p := range phs {
		h.Write([]byte(p.path))
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
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO notebook_files (notebook_id, path, content, content_sha256, byte_size)
			 VALUES ($1, $2, $3, $4, $5)`,
			nb.NotebookID, f.Path, f.Content, FileSHA256(f.Content), len(f.Content),
		); err != nil {
			return fmt.Errorf("insert notebook_file %s: %w", f.Path, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit push: %w", err)
	}
	return nil
}

// ListFiles returns every stored file for a notebook id, ordered by path.
func (r *NotebookFileRepository) ListFiles(ctx context.Context, notebookID string) ([]NotebookFile, error) {
	var files []NotebookFile
	if err := r.db.SelectContext(ctx, &files,
		`SELECT notebook_id, path, content, content_sha256, byte_size
		 FROM notebook_files WHERE notebook_id = $1 ORDER BY path`, notebookID); err != nil {
		return nil, fmt.Errorf("list notebook_files: %w", err)
	}
	return files, nil
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

// Fingerprint returns a stable digest of the current set of user notebooks
// (id + content_hash), so DBContentSource can cheaply detect when a re-push
// changed the materialized content and needs a refresh.
func (r *NotebookFileRepository) Fingerprint(ctx context.Context) (string, error) {
	rows, err := r.ListAllUserNotebooks(ctx)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, nb := range rows {
		b.WriteString(nb.NotebookID)
		b.WriteByte('\n')
		b.WriteString(nb.ContentHash)
		b.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:]), nil
}
