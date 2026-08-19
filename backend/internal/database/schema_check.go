package database

import (
	"fmt"
	"io/fs"
	"regexp"
	"strconv"

	"github.com/jmoiron/sqlx"
)

// migrationFilePattern matches the numeric prefix of a golang-migrate file
// name (e.g. "022_notes_homograph_unique.up.sql" -> 22).
var migrationFilePattern = regexp.MustCompile(`^(\d+)_.*\.up\.sql$`)

// expectedMigrationVersion returns the highest migration version number among
// the *.up.sql files in migrationsFS/dir. This is the version a fully-migrated
// database should report in schema_migrations.
func expectedMigrationVersion(migrationsFS fs.FS, dir string) (int, error) {
	entries, err := fs.ReadDir(migrationsFS, dir)
	if err != nil {
		return 0, fmt.Errorf("read migrations dir %q: %w", dir, err)
	}
	max := 0
	for _, e := range entries {
		m := migrationFilePattern.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		v, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if v > max {
			max = v
		}
	}
	if max == 0 {
		return 0, fmt.Errorf("no *.up.sql migrations found in %q", dir)
	}
	return max, nil
}

// currentMigrationVersion reads the applied version and dirty flag from the
// schema_migrations table golang-migrate maintains. exists is false when the
// table itself is absent — a database that has never had migrations run.
func currentMigrationVersion(db *sqlx.DB) (version int, dirty, exists bool, err error) {
	var present bool
	if err = db.Get(&present, `SELECT EXISTS (
		SELECT 1 FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_name = 'schema_migrations'
	)`); err != nil {
		return 0, false, false, fmt.Errorf("check schema_migrations existence: %w", err)
	}
	if !present {
		return 0, false, false, nil
	}
	var row struct {
		Version int  `db:"version"`
		Dirty   bool `db:"dirty"`
	}
	if err = db.Get(&row, `SELECT version, dirty FROM schema_migrations LIMIT 1`); err != nil {
		return 0, false, false, fmt.Errorf("read schema_migrations: %w", err)
	}
	return row.Version, row.Dirty, true, nil
}

// columnExists reports whether table has column in the current schema.
func columnExists(db *sqlx.DB, table, column string) (bool, error) {
	var present bool
	if err := db.Get(&present, `SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2
	)`, table, column); err != nil {
		return false, fmt.Errorf("check column %s.%s: %w", table, column, err)
	}
	return present, nil
}

// VerifySchema pre-flight-checks that db's schema matches what the embedded
// migrations expect, BEFORE a data import runs. It exists to turn the class of
// "schema drift" failures — a database built by an older / renumbered migration
// chain — into a single, actionable error instead of a cryptic error deep in a
// column scan (e.g. `missing destination name part_of_speech`,
// `column sense_id does not exist`).
//
// golang-migrate trusts the integer version in schema_migrations and does NOT
// verify the actual columns, so it happily reports "up to date" against a DB
// whose renumbered history skipped the migration that adds sense_id. The
// version check alone therefore can't catch drift; the explicit column checks
// (notes has sense_id, notes has no legacy part_of_speech) carry the weight.
//
// On any mismatch the returned error names the offending table/column and tells
// the user what to do: import into a fresh empty database.
func VerifySchema(db *sqlx.DB, migrationsFS fs.FS, dir string) error {
	expected, err := expectedMigrationVersion(migrationsFS, dir)
	if err != nil {
		return err
	}
	current, dirty, exists, err := currentMigrationVersion(db)
	if err != nil {
		return err
	}

	if !exists {
		return fmt.Errorf("schema drift: the target database has no schema_migrations table, "+
			"so no migrations have ever run against it. import-db requires a schema created by the "+
			"current migrations (expected version %d). Run the migrations first, or import into a "+
			"fresh empty database (createdb + import-db).", expected)
	}
	if dirty {
		return fmt.Errorf("schema drift: schema_migrations is marked DIRTY at version %d — a previous "+
			"migration failed partway. import-db will not run against a dirty schema. Fix the failed "+
			"migration (or rebuild into a fresh empty database) before importing.", current)
	}
	if current != expected {
		return fmt.Errorf("schema drift: the target database is at migration version %d but this binary "+
			"expects version %d. The DB was created by a different (older or renumbered) migration chain. "+
			"import-db requires a schema created by the current migrations. Import into a fresh empty "+
			"database (createdb + import-db), or migrate/rebuild this one.", current, expected)
	}

	// The version can match while the columns don't: a renumbered migration
	// chain lets golang-migrate believe a migration was applied when the
	// corresponding schema change never ran. Verify the columns that the
	// user's real-DB drift bugs turned on directly.
	hasSenseID, err := columnExists(db, "notes", "sense_id")
	if err != nil {
		return err
	}
	if !hasSenseID {
		return fmt.Errorf("schema drift: the notes table is missing column sense_id — this DB was created "+
			"by an older migration chain (schema_migrations reports version %d but the sense_id migration "+
			"never actually ran). import-db requires a schema created by the current migrations. Import "+
			"into a fresh empty database (createdb + import-db), or migrate/rebuild this one.", current)
	}
	hasPartOfSpeech, err := columnExists(db, "notes", "part_of_speech")
	if err != nil {
		return err
	}
	if hasPartOfSpeech {
		return fmt.Errorf("schema drift: the notes table has a legacy part_of_speech column that no current " +
			"migration creates (it is left over from an abandoned approach). import-db reads explicit " +
			"columns so it will not crash on it, but its presence means this DB predates the current " +
			"schema. Import into a fresh empty database (createdb + import-db) to be safe.")
	}
	return nil
}
