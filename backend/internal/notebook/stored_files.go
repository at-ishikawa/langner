package notebook

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// StoredFileBundles returns the notebooks stored as raw YAML files in
// notebook_files (how bundles were stored before migration 038) that have no
// content rows yet, grouped by notebook id.
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
		`SELECT notebook_id, family, path, content FROM notebook_files f
		 WHERE NOT EXISTS (SELECT 1 FROM content_indexes i WHERE i.notebook_id = f.notebook_id)
		 ORDER BY notebook_id, family, path`); err != nil {
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
