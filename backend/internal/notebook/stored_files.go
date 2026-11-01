package notebook

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// StoredFileBundles returns the notebooks still stored as raw YAML files in
// notebook_files (how bundles were stored before migration 038), grouped by
// notebook id. It returns nothing once the table is empty or gone.
func StoredFileBundles(ctx context.Context, db *sqlx.DB) (map[string][]NotebookFile, error) {
	var exists bool
	if err := db.GetContext(ctx, &exists, `SELECT to_regclass('notebook_files') IS NOT NULL`); err != nil {
		return nil, fmt.Errorf("check notebook_files: %w", err)
	}
	if !exists {
		return nil, nil
	}
	var rows []struct {
		NotebookID string `db:"notebook_id"`
		Family     string `db:"family"`
		Path       string `db:"path"`
		Content    []byte `db:"content"`
	}
	if err := db.SelectContext(ctx, &rows,
		`SELECT notebook_id, family, path, content FROM notebook_files ORDER BY notebook_id, family, path`); err != nil {
		return nil, fmt.Errorf("read notebook_files: %w", err)
	}
	out := make(map[string][]NotebookFile)
	for _, r := range rows {
		out[r.NotebookID] = append(out[r.NotebookID], NotebookFile{
			NotebookID: r.NotebookID, Family: r.Family, Path: r.Path, Content: r.Content,
		})
	}
	return out, nil
}

// DeleteStoredFiles removes a notebook's notebook_files rows once its content
// has been stored as rows.
func DeleteStoredFiles(ctx context.Context, db *sqlx.DB, notebookID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM notebook_files WHERE notebook_id = $1`, notebookID); err != nil {
		return fmt.Errorf("delete notebook_files of %s: %w", notebookID, err)
	}
	return nil
}
