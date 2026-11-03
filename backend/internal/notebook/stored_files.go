package notebook

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// StoredFileBundles returns the notebooks stored as raw YAML files in
// notebook_files (how bundles were stored before migration 038) whose content
// rows must be (re)built from those files, grouped by notebook id:
//   - a notebook with no content rows yet;
//   - a notebook whose stored files are its current bundle (their hash is the
//     notebooks.content_hash) but whose rows were not recorded as built from
//     them — e.g. files re-uploaded by a server that wrote only notebook_files
//     and the hash after the rows were converted.
//
// Files whose hash differs from content_hash are an older copy (the notebook
// was pushed since as rows only), so rows that exist are kept.
//
// This repair only exists while notebook_files does; delete it with the table.
func StoredFileBundles(ctx context.Context, db *sqlx.DB) (map[string][]NotebookFile, error) {
	var exists bool
	if err := db.GetContext(ctx, &exists, `SELECT to_regclass('notebook_files') IS NOT NULL`); err != nil {
		return nil, fmt.Errorf("check notebook_files: %w", err)
	}
	if !exists {
		return nil, nil
	}
	var rows []struct {
		NotebookID  string `db:"notebook_id"`
		Family      string `db:"family"`
		Path        string `db:"path"`
		Content     []byte `db:"content"`
		ContentHash string `db:"content_hash"`
		BuiltHash   string `db:"content_built_hash"`
		HasRows     bool   `db:"has_rows"`
	}
	if err := db.SelectContext(ctx, &rows,
		`SELECT f.notebook_id, f.family, f.path, f.content,
		        COALESCE(n.content_hash, '') AS content_hash,
		        COALESCE(n.content_built_hash, '') AS content_built_hash,
		        EXISTS (SELECT 1 FROM content_indexes i WHERE i.notebook_id = f.notebook_id) AS has_rows
		 FROM notebook_files f LEFT JOIN notebooks n ON n.notebook_id = f.notebook_id
		 ORDER BY f.notebook_id, f.family, f.path`); err != nil {
		return nil, fmt.Errorf("read notebook_files: %w", err)
	}
	out := make(map[string][]NotebookFile)
	for _, r := range rows {
		out[r.NotebookID] = append(out[r.NotebookID], NotebookFile{
			NotebookID: r.NotebookID, Family: r.Family, Path: r.Path, Content: r.Content,
		})
	}
	for _, r := range rows {
		files, ok := out[r.NotebookID]
		if !ok || !r.HasRows {
			continue
		}
		hash := HashBundle(files)
		if hash != r.ContentHash || hash == r.BuiltHash {
			delete(out, r.NotebookID)
		}
	}
	return out, nil
}
