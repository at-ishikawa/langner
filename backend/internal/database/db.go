// Package database provides database connection management.
package database

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // register the pgx driver under the name "pgx"
	migrate "github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/config"
)

// Open opens a PostgreSQL connection using the provided config.
func Open(cfg config.DatabaseConfig) (*sqlx.DB, error) {
	dsn := buildDSN(cfg)

	db, err := sqlx.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database connection: %w", err)
	}

	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)
	}

	return db, nil
}

// buildDSN constructs a PostgreSQL connection URL from the configuration.
// Defaults sslmode=disable so local Docker deployments connect without
// extra setup; cfg.TLS upgrades that to sslmode=require, and any value
// in cfg.Params takes precedence so callers can override per environment.
func buildDSN(cfg config.DatabaseConfig) string {
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.Username, cfg.Password),
		Host:   fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Path:   "/" + cfg.Database,
	}
	q := u.Query()
	q.Set("sslmode", "disable")
	if cfg.TLS {
		q.Set("sslmode", "require")
	}
	// Default to pgx's unnamed-prepared-statement exec mode instead of its
	// named statement cache. The cache breaks behind transaction-pooling
	// connection poolers (e.g. PgBouncer) and after pooled server-connection
	// reuse, surfacing as `prepared statement "stmtcache_..." already exists`
	// (SQLSTATE 42P05). The exec mode keeps the extended protocol (proper
	// typing / binary results) but uses no persistent server-side statements,
	// so there is nothing to collide. Overridable via
	// database.params.default_query_exec_mode (e.g. "simple_protocol").
	q.Set("default_query_exec_mode", "exec")
	for k, v := range cfg.Params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// RunInTx runs fn within a database transaction.
// If fn returns an error, the transaction is rolled back; otherwise, it is committed.
func RunInTx(ctx context.Context, db *sqlx.DB, fn func(ctx context.Context, tx *sqlx.Tx) error) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	if err := fn(ctx, tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("rollback transaction: %w (original error: %v)", rbErr, err)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// Migrate runs all pending up migrations against db, sourced from the given
// embedded filesystem. dir is the path within migrationsFS that contains the
// *.up.sql / *.down.sql files (e.g. "migrations" when the embed pattern is
// "migrations/*.sql"). Returns nil when the schema is already at the latest
// version. Idempotent.
func Migrate(db *sqlx.DB, migrationsFS fs.FS, dir string) error {
	src, err := iofs.New(migrationsFS, dir)
	if err != nil {
		return fmt.Errorf("init migration source: %w", err)
	}

	driver, err := migratepgx.WithInstance(db.DB, &migratepgx.Config{})
	if err != nil {
		return fmt.Errorf("init migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "pgx5", driver)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// BuildMultiRowInsert builds a multi-row INSERT query using PostgreSQL's
// numbered placeholder syntax ($1, $2, ...).
func BuildMultiRowInsert(table string, columns []string, rowCount int) string {
	var b strings.Builder
	b.WriteString("INSERT INTO ")
	b.WriteString(table)
	b.WriteString(" (")
	b.WriteString(strings.Join(columns, ", "))
	b.WriteString(") VALUES ")
	n := 0
	for r := 0; r < rowCount; r++ {
		if r > 0 {
			b.WriteString(", ")
		}
		b.WriteByte('(')
		for c := 0; c < len(columns); c++ {
			if c > 0 {
				b.WriteString(", ")
			}
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
		}
		b.WriteByte(')')
	}
	return b.String()
}
