package datasync

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDataTablesCoverAllSchema is the completeness GUARD for the table-dump
// exporter. It derives the set of tables from the source-of-truth schema
// (schemas/migrations/*.up.sql) and fails if DataTablesInDependencyOrder —
// which the exporter iterates — misses any of them (or lists a table no
// migration defines).
//
// This is what makes "export every table, no table left behind" enforceable:
// a future migration that adds a table but forgets to add it here breaks this
// test, so the export can never silently skip a table.
//
// To extend when a migration adds a table: add the new table to
// DataTablesInDependencyOrder in table_export.go, placing children (rows
// whose FK points at another table in the list) before their parents.
func TestDataTablesCoverAllSchema(t *testing.T) {
	migrationsDir := findMigrationsDir(t)
	expected, err := tablesFromMigrations(migrationsDir)
	require.NoError(t, err)
	delete(expected, "schema_migrations") // owned by the migration tool, not user data

	got := make(map[string]bool)
	for _, name := range DataTablesInDependencyOrder() {
		got[name] = true
	}

	var missing []string
	for table := range expected {
		if !got[table] {
			missing = append(missing, table)
		}
	}
	var extra []string
	for table := range got {
		if !expected[table] {
			extra = append(extra, table)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)

	assert.Empty(t, missing,
		"DataTablesInDependencyOrder is missing tables defined by schemas/migrations — "+
			"the table-dump export would silently skip them, losing data. Add each missing "+
			"table (children before parents in FK order).")
	assert.Empty(t, extra,
		"DataTablesInDependencyOrder lists tables no migration defines — either the migration "+
			"was reverted without updating the list, or the name is misspelled.")
}

// TestTableDumpSerializationRoundTrip proves the per-table serialisation
// layer is lossless WITHOUT a database: a row carrying every value kind the
// schema produces (bigint id, text, null, timestamp, JSONB bytes, bool,
// numeric) survives normalize -> YAML file -> read-back -> denormalize with
// its values intact. The live-Postgres round-trip test proves the same end
// to end against a real DB; this one keeps the core logic covered on every
// `go test` run.
func TestTableDumpSerializationRoundTrip(t *testing.T) {
	ts := time.Date(2026, 8, 17, 9, 30, 15, 123456000, time.UTC)
	original := map[string]any{
		"id":            int64(42),
		"usage":         "break the ice",
		"meaning":       "to relieve tension",
		"skipped_at":    nil,
		"created_at":    ts,
		"response":      []byte(`{"word":"break the ice"}`),
		"is_directed":   true,
		"interval_days": int64(7),
	}

	normalized := normalizeRow(original)
	// Timestamps and JSONB bytes must become plain scalars for YAML.
	assert.Equal(t, ts.Format(time.RFC3339Nano), normalized["created_at"])
	assert.Equal(t, `{"word":"break the ice"}`, normalized["response"])
	assert.Nil(t, normalized["skipped_at"])

	dir := t.TempDir()
	tablesDir := filepath.Join(dir, "tables")
	require.NoError(t, os.MkdirAll(tablesDir, 0o755))
	path := filepath.Join(tablesDir, "sample.yml")
	require.NoError(t, writeTableFile(path, []map[string]any{normalized}))

	readBack, err := readTableFile(path)
	require.NoError(t, err)
	require.Len(t, readBack, 1)
	row := readBack[0]

	// Timestamp column (isTimestamp=true) denormalises back to the same instant.
	gotTime, ok := denormalizeValue(row["created_at"], true).(time.Time)
	require.True(t, ok, "a timestamp column must denormalise to time.Time")
	assert.True(t, ts.Equal(gotTime), "timestamp round-trip: want %s got %s", ts, gotTime)

	// A NULL column round-trips as nil (not the empty string or a zero time).
	assert.Nil(t, denormalizeValue(row["skipped_at"], true))

	// Text, JSON, bool and integers survive unchanged.
	assert.Equal(t, "break the ice", row["usage"])
	assert.Equal(t, `{"word":"break the ice"}`, row["response"])
	assert.Equal(t, true, row["is_directed"])
	assert.Equal(t, int64(42), toInt64(row["id"]))
	assert.Equal(t, int64(7), toInt64(row["interval_days"]))
}

// TestTableDumpBinaryByteaRoundTrip proves bytea columns survive the
// dump/restore losslessly WITHOUT a database. AES-GCM ciphertext (auth's
// users.email_encrypted / users.name_encrypted / user_llm_credentials.
// api_key_encrypted) can hold bytes Postgres text/varchar cannot: string()-ing
// them and re-inserting produced `invalid byte sequence for encoding "UTF8":
// 0x00`. Crucially, a lone NUL (0x00) is VALID UTF-8 (U+0000) yet Postgres text
// still rejects it — so "valid UTF-8" is NOT enough to send as a text string;
// the value must also be NUL-free. Each case is driven through the real
// serialize -> yaml.Marshal(writeTableFile) -> yaml.Unmarshal(readTableFile) ->
// denormalizeValue path and asserted byte-for-byte, with a stable re-serialise.
func TestTableDumpBinaryByteaRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		// value is the original bytea. wantTagged is true when it must be
		// base64-tagged (binary OR contains a NUL) rather than a plain string.
		value      []byte
		wantTagged bool
	}{
		// Non-UTF-8 ciphertext (high bytes + NUL).
		{"non_utf8_ciphertext", []byte{0x00, 0x01, 0xDE, 0xAD, 0xBE, 0xEF, 0xFF}, true},
		// A lone NUL: the EXACT round-trip seed value ('\x00'). Valid UTF-8 but
		// Postgres text rejects it — the hole the first fix missed.
		{"lone_null_byte", []byte{0x00}, true},
		// Valid UTF-8 text with an embedded NUL — still cannot go through a text bind.
		{"utf8_text_with_embedded_null", []byte("ab\x00cd"), true},
		// Valid UTF-8, no NUL: JSONB / text-shaped bytea stays a plain string.
		{"utf8_json_no_null", []byte(`{"word":"break the ice"}`), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original := map[string]any{"id": int64(1), "email_encrypted": tc.value}

			normalized := normalizeRow(original)
			s, ok := normalized["email_encrypted"].(string)
			require.True(t, ok, "bytea must normalise to a string, got %T", normalized["email_encrypted"])
			if tc.wantTagged {
				assert.True(t, strings.HasPrefix(s, binaryValuePrefix),
					"a value Postgres text cannot hold (non-UTF-8 or containing NUL) must be base64-tagged; got %q", s)
				assert.NotContains(t, s, "\x00", "the serialised form must not embed a raw NUL byte")
			} else {
				assert.False(t, strings.HasPrefix(s, binaryValuePrefix),
					"plain UTF-8 text (no NUL) must stay an untagged string; got %q", s)
			}

			dir := t.TempDir()
			tablesDir := filepath.Join(dir, "tables")
			require.NoError(t, os.MkdirAll(tablesDir, 0o755))
			path := filepath.Join(tablesDir, "users.yml")
			require.NoError(t, writeTableFile(path, []map[string]any{normalized}))

			readBack, err := readTableFile(path)
			require.NoError(t, err)
			require.Len(t, readBack, 1)

			// email_encrypted is NOT a timestamp column -> isTimestamp=false.
			got := denormalizeValue(readBack[0]["email_encrypted"], false)
			if tc.wantTagged {
				b, ok := got.([]byte)
				require.True(t, ok, "a tagged bytea must denormalise back to []byte, got %T", got)
				assert.Equal(t, tc.value, b, "bytea round-trip must be byte-for-byte identical")
			} else {
				assert.Equal(t, string(tc.value), got, "plain UTF-8 text round-trips as the same string")
			}

			// Mirror the DB round trip export A -> import -> export B: re-normalise
			// the DENORMALISED value (what would be re-inserted) and assert the file
			// is byte-for-byte identical (the losslessness A==B proof).
			path2 := filepath.Join(tablesDir, "users2.yml")
			reNormalized := normalizeRow(map[string]any{"id": int64(1), "email_encrypted": got})
			require.NoError(t, writeTableFile(path2, []map[string]any{reNormalized}))
			a, err := os.ReadFile(path)
			require.NoError(t, err)
			b, err := os.ReadFile(path2)
			require.NoError(t, err)
			assert.Equal(t, string(a), string(b), "bytea must re-serialise stably")
		})
	}
}

// --- migration-schema parsing helpers (self-contained; mirrors the guard in
// cmd/langner/datasync_test.go so this package needs no cross-package deps) ---

func tablesFromMigrations(dir string) (map[string]bool, error) {
	createPattern := regexp.MustCompile(`(?i)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?` + "`?" + `(\w+)` + "`?")
	dropPattern := regexp.MustCompile(`(?i)DROP\s+TABLE\s+(?:IF\s+EXISTS\s+)?` + "`?" + `(\w+)` + "`?")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	out := make(map[string]bool)
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		for _, m := range createPattern.FindAllStringSubmatch(string(body), -1) {
			out[m[1]] = true
		}
		for _, m := range dropPattern.FindAllStringSubmatch(string(body), -1) {
			delete(out, m[1])
		}
	}
	return out, nil
}

func findMigrationsDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	for path := wd; path != "/" && path != "."; path = filepath.Dir(path) {
		candidate := filepath.Join(path, "schemas", "migrations")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	t.Fatalf("schemas/migrations directory not found above %s", wd)
	return ""
}
