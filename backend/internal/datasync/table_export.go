package datasync

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"gopkg.in/yaml.v3"
)

// DataTablesInDependencyOrder lists every persisted-data table with child
// tables (whose foreign key points at another table in the list) BEFORE
// their parents. Deleting/truncating in this order is FK-safe; inserting in
// the reverse order is FK-safe.
//
// This is the single source of truth for:
//   - the table-dump exporter/importer below (which MUST cover every table
//     so nothing is silently lost when the app moves to DB-only state), and
//   - the destructive clear step in sync-db/validate-db.
//
// schema_migrations is intentionally excluded — it is owned by the migration
// tool, not by user data. A completeness guard test
// (TestDataTablesCoverAllSchema) derives the table set from
// schemas/migrations/*.up.sql and fails if this list drifts, so a future
// migration that adds a table but forgets its export is caught immediately.
func DataTablesInDependencyOrder() []string {
	return []string{
		// Skip-flag / DB-only-state children (migrations 020, 021).
		"note_skip_flags",        // -> notes
		"origin_skip_flags",      // -> etymology_origins
		"definitions_scenes",     // -> definitions_sessions
		"definitions_sessions",   // leaf parent
		"flashcard_decks",        // leaf parent
		"note_origin_parts",      // -> notes, etymology_origins, etymology_origin_forms
		"notebook_notes",         // -> notes
		"note_images",            // -> notes
		"note_references",        // -> notes
		"learning_logs",          // -> notes, etymology_origins, grammar_corrections
		"grammar_corrections",    // leaf parent (learning_logs.correction_id -> here)
		"etymology_origin_forms", // -> etymology_origins
		"semantic_concept_members",
		"concept_relations",
		"definition_concept_members",
		"notes",
		"etymology_origins",
		"semantic_concepts",
		"definition_concepts",
		"dictionary_entries",
		"notebooks", // -> users (migration 027 notebook ownership); child of users
		"users",     // leaf parent (migration 024); notebooks + learning_logs.user_id reference it
	}
}

// TableDumpResult reports how many rows were written (or restored) per table.
type TableDumpResult struct {
	RowsByTable map[string]int
}

// TableDumpExporter writes a faithful, per-table YAML snapshot of every
// persisted-data table to <outputDir>/tables/<table>.yml.
//
// Unlike the notebook-shaped Exporter — which reconstructs the app's YAML
// notebook formats and therefore cannot represent DB-only columns (note ids,
// sense_id, skipped_at, the etymology junction tables) and drops note-body
// fields the DB never stores — this dump captures every row of every column
// exactly as stored. It is the lossless, diffable, recoverable snapshot the
// DB-only migration needs. Rows are sorted by primary key so the files are
// stable and git-diffable.
type TableDumpExporter struct {
	db        *sqlx.DB
	outputDir string
	writer    io.Writer
}

// NewTableDumpExporter constructs a TableDumpExporter.
func NewTableDumpExporter(db *sqlx.DB, outputDir string, writer io.Writer) *TableDumpExporter {
	return &TableDumpExporter{db: db, outputDir: outputDir, writer: writer}
}

// ExportTables writes one YAML file per table under <outputDir>/tables/.
// Empty tables produce an empty-list file so the snapshot is complete and a
// table that unexpectedly emptied out is visible in a diff.
func (e *TableDumpExporter) ExportTables(ctx context.Context) (*TableDumpResult, error) {
	dir := filepath.Join(e.outputDir, "tables")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create tables dir %s: %w", dir, err)
	}
	res := &TableDumpResult{RowsByTable: make(map[string]int)}
	for _, table := range DataTablesInDependencyOrder() {
		rows, err := e.dumpTable(ctx, table)
		if err != nil {
			return nil, err
		}
		if err := writeTableFile(filepath.Join(dir, table+".yml"), rows); err != nil {
			return nil, err
		}
		res.RowsByTable[table] = len(rows)
		_, _ = fmt.Fprintf(e.writer, "  Exported %d row(s) from %s\n", len(rows), table)
	}
	return res, nil
}

func (e *TableDumpExporter) dumpTable(ctx context.Context, table string) ([]map[string]any, error) {
	// table comes only from the hard-coded DataTablesInDependencyOrder
	// allowlist, never from user input, so string-building the query is safe.
	rows, err := e.db.QueryxContext(ctx, "SELECT * FROM "+table) //nolint:gosec // table is an internal allowlist constant
	if err != nil {
		return nil, fmt.Errorf("select %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	var out []map[string]any
	for rows.Next() {
		raw := map[string]any{}
		if err := rows.MapScan(raw); err != nil {
			return nil, fmt.Errorf("scan %s: %w", table, err)
		}
		out = append(out, normalizeRow(raw))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s: %w", table, err)
	}
	sort.Slice(out, func(i, j int) bool { return rowSortKey(out[i]) < rowSortKey(out[j]) })
	return out, nil
}

// TableDumpImporter restores a TableDumpExporter snapshot into a database,
// inserting every row verbatim (primary keys, timestamps and all) in FK-safe
// parent-before-child order. It is the recovery half of the dump: clear the
// DB, ImportTables, and the DB is exactly what it was — which is also what
// proves the export lossless (see the round-trip integration test).
//
// It assumes the target tables are empty (e.g. after TRUNCATE ... RESTART
// IDENTITY CASCADE) so the explicit id values don't collide.
type TableDumpImporter struct {
	db       *sqlx.DB
	inputDir string
	writer   io.Writer
}

// NewTableDumpImporter constructs a TableDumpImporter.
func NewTableDumpImporter(db *sqlx.DB, inputDir string, writer io.Writer) *TableDumpImporter {
	return &TableDumpImporter{db: db, inputDir: inputDir, writer: writer}
}

// ImportTables reads <inputDir>/tables/<table>.yml for every table and
// inserts its rows. Parents are inserted before children (the reverse of the
// child-first dependency order) so foreign keys always resolve.
func (imp *TableDumpImporter) ImportTables(ctx context.Context) (*TableDumpResult, error) {
	res := &TableDumpResult{RowsByTable: make(map[string]int)}
	tables := DataTablesInDependencyOrder()
	for i := len(tables) - 1; i >= 0; i-- {
		table := tables[i]
		rows, err := readTableFile(filepath.Join(imp.inputDir, "tables", table+".yml"))
		if err != nil {
			return nil, err
		}
		tsCols, err := imp.timestampColumns(ctx, table)
		if err != nil {
			return nil, err
		}
		if err := imp.insertRows(ctx, table, rows, tsCols); err != nil {
			return nil, err
		}
		res.RowsByTable[table] = len(rows)
		_, _ = fmt.Fprintf(imp.writer, "  Restored %d row(s) into %s\n", len(rows), table)
	}
	return res, nil
}

// timestampColumns returns the set of columns on the table whose type is a
// timestamp, sourced from the live schema rather than a column-name heuristic.
// normalizeValue renders every timestamp as an RFC3339 string for YAML, so on
// restore those columns must be parsed back to time.Time before insert — and
// the schema is the only reliable way to know which columns those are (the
// `date`-named TIMESTAMP columns on definitions_sessions / flashcard_decks do
// not end in "_at").
func (imp *TableDumpImporter) timestampColumns(ctx context.Context, table string) (map[string]bool, error) {
	var cols []string
	if err := imp.db.SelectContext(ctx, &cols,
		`SELECT column_name FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = $1 AND data_type LIKE 'timestamp%'`,
		table,
	); err != nil {
		return nil, fmt.Errorf("read timestamp columns for %s: %w", table, err)
	}
	set := make(map[string]bool, len(cols))
	for _, c := range cols {
		set[c] = true
	}
	return set, nil
}

func (imp *TableDumpImporter) insertRows(ctx context.Context, table string, rows []map[string]any, tsCols map[string]bool) error {
	for _, row := range rows {
		cols := make([]string, 0, len(row))
		for k := range row {
			cols = append(cols, k)
		}
		sort.Strings(cols)

		quoted := make([]string, len(cols))
		placeholders := make([]string, len(cols))
		args := make([]any, len(cols))
		for i, c := range cols {
			quoted[i] = `"` + c + `"`
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = denormalizeValue(row[c], tsCols[c])
		}
		// table + column names are internal allowlist/identifier values, not
		// user input; values are parameterised.
		query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, //nolint:gosec // table/columns are internal identifiers
			strings.Join(quoted, ", "), strings.Join(placeholders, ", "))
		if _, err := imp.db.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("insert into %s: %w", table, err)
		}
	}
	return nil
}

// normalizeRow converts a scanned DB row into YAML-friendly, faithful
// scalar values.
func normalizeRow(raw map[string]any) map[string]any {
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		out[k] = normalizeValue(v)
	}
	return out
}

// normalizeValue renders one DB value as a YAML-serialisable scalar without
// losing information:
//   - time.Time  -> RFC3339 with nanoseconds, in UTC (stable, comparable)
//   - []byte     -> string (covers JSONB text and any bytea)
//   - everything else (int64, float64, bool, string, nil) passes through.
func normalizeValue(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case time.Time:
		return t.UTC().Format(time.RFC3339Nano)
	case []byte:
		return string(t)
	default:
		return v
	}
}

// denormalizeValue converts a YAML-decoded value back into a form the DB
// driver accepts for INSERT. Timestamp columns (identified from the live
// schema, not a name heuristic) are parsed back into time.Time; every other
// value inserts as-is (pgx coerces int/float/bool/string, and a JSON string
// into a jsonb column).
func denormalizeValue(v any, isTimestamp bool) any {
	if v == nil {
		return nil
	}
	if isTimestamp {
		if s, ok := v.(string); ok {
			if parsed, err := time.Parse(time.RFC3339Nano, s); err == nil {
				return parsed
			}
		}
	}
	return v
}

// rowSortKey produces a deterministic ordering key for a row so the dumped
// file is stable. Rows with an integer id sort by it; the id-less
// dictionary_entries (and any future keyless table) sort by their full
// content.
func rowSortKey(row map[string]any) string {
	if id, ok := row["id"]; ok {
		return fmt.Sprintf("id:%020d", toInt64(id))
	}
	keys := make([]string, 0, len(row))
	for k := range row {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		_, _ = fmt.Fprintf(&b, "%s=%v\x00", k, row[k])
	}
	return b.String()
}

func toInt64(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	default:
		return 0
	}
}

func writeTableFile(path string, rows []map[string]any) error {
	if rows == nil {
		rows = []map[string]any{}
	}
	data, err := yaml.Marshal(rows)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func readTableFile(path string) ([]map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var rows []map[string]any
	if err := yaml.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", path, err)
	}
	return rows, nil
}
