package database

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/jmoiron/sqlx"
)

// Gzip compresses b. Used for the compressed column copies (migration 037):
// Postgres sends BYTEA/JSON uncompressed, so reading a gzip copy cuts the
// bytes transferred from the database.
func Gzip(b []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	// Writes to a bytes.Buffer cannot fail.
	_, _ = w.Write(b)
	_ = w.Close()
	return buf.Bytes()
}

// Gunzip decompresses b.
func Gunzip(b []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("open gzip: %w", err)
	}
	defer func() { _ = r.Close() }()
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read gzip: %w", err)
	}
	return out, nil
}

// ColumnCheck reports, once per process, whether a column exists. The server
// does not migrate on boot, so code that uses a newly added column must keep
// working (on the old query) until the migration has been applied.
type ColumnCheck struct {
	table, column string
	once          sync.Once
	exists        bool
}

// NewColumnCheck returns a check for table.column.
func NewColumnCheck(table, column string) *ColumnCheck {
	return &ColumnCheck{table: table, column: column}
}

// Exists reports whether the column exists. The first call queries the
// catalog; a failed lookup is treated as "absent" (the safe, old behavior).
func (c *ColumnCheck) Exists(ctx context.Context, db *sqlx.DB) bool {
	c.once.Do(func() {
		var n int
		err := db.GetContext(ctx, &n,
			`SELECT count(*) FROM information_schema.columns
			 WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2`,
			c.table, c.column)
		c.exists = err == nil && n > 0
	})
	return c.exists
}
