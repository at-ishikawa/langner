// Package database provides database connection management.
package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	migrate "github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/config"
)

// Open opens a MySQL/TiDB connection using the provided config.
//
// Every connection produced from the pool is best-effort-asked to set
// tidb_skip_isolation_level_check=1. golang-migrate begins each
// migration in a SERIALIZABLE transaction; TiDB rejects that isolation
// level unless this session variable is set, and adding it to
// DatabaseConfig.Params would break MySQL (which doesn't recognize the
// variable). Issuing the SET per-connection and ignoring its error
// lets the same binary speak to both engines without forcing the
// operator to maintain two configs.
func Open(cfg config.DatabaseConfig) (*sqlx.DB, error) {
	mysqlCfg := mysql.NewConfig()
	mysqlCfg.User = cfg.Username
	mysqlCfg.Passwd = cfg.Password
	mysqlCfg.Net = "tcp"
	mysqlCfg.Addr = fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	mysqlCfg.DBName = cfg.Database
	mysqlCfg.ParseTime = true
	mysqlCfg.MultiStatements = true
	if cfg.TLS {
		mysqlCfg.TLSConfig = "true"
	}
	if len(cfg.Params) > 0 {
		mysqlCfg.Params = cfg.Params
	}

	// Round-trip the DSN so the driver's parser normalises special keys
	// (charset → Collation, tls → TLSConfig, …) the same way it does
	// for sql.Open. mysql.NewConnector(mysqlCfg) skips that step and
	// would then issue a literal `SET @@charset = utf8mb4` per
	// connection, which TiDB rejects with `Unknown system variable
	// 'charset'`.
	dsn := mysqlCfg.FormatDSN()
	normalizedCfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse mysql DSN: %w", err)
	}
	baseConnector, err := mysql.NewConnector(normalizedCfg)
	if err != nil {
		return nil, fmt.Errorf("build mysql connector: %w", err)
	}
	db := sqlx.NewDb(sql.OpenDB(&tidbAwareConnector{inner: baseConnector}), "mysql")

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

// tidbAwareConnector wraps the standard MySQL driver.Connector so that
// each connection it produces tries to enable
// tidb_skip_isolation_level_check. On TiDB the SET succeeds and lets
// golang-migrate run its SERIALIZABLE transactions. On MySQL the
// variable doesn't exist, the SET errors, and we discard the error so
// the connection stays usable.
type tidbAwareConnector struct {
	inner driver.Connector
}

func (c *tidbAwareConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	if execer, ok := conn.(driver.ExecerContext); ok {
		_, _ = execer.ExecContext(ctx, "SET @@SESSION.tidb_skip_isolation_level_check = 1", nil)
	}
	return conn, nil
}

func (c *tidbAwareConnector) Driver() driver.Driver {
	return c.inner.Driver()
}

// ExecWithRetry runs query/args on the pool, retrying up to three
// times on transient connection-level failures. Used by chunked
// importers (learning_logs, dictionary_entries) to absorb TiDB
// Cloud's occasional mid-statement `tls: bad record MAC`, where the
// connection silently rots inside the pool and the next checkout
// finds it unusable. Persistent errors (duplicate-key, constraint,
// parse) bubble up immediately so callers see the real cause.
//
// Safe only for autocommit ExecContext calls — a query inside an
// open *sqlx.Tx must not be retried this way because the surrounding
// transaction will already be rolled back. Wrap individual chunks,
// not whole batches.
func ExecWithRetry(ctx context.Context, db *sqlx.DB, query string, args ...interface{}) error {
	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		_, err := db.ExecContext(ctx, query, args...)
		if err == nil {
			return nil
		}
		if !isTransientConnError(err) {
			return err
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * 200 * time.Millisecond):
		}
	}
	return fmt.Errorf("retry exhausted after %d attempts: %w", maxAttempts, lastErr)
}

// isTransientConnError reports whether err describes a connection-
// level fault we can retry. database/sql.ErrBadConn covers the
// driver-signalled case; TLS framing errors against TiDB Cloud
// surface as plain strings rather than typed errors, so we also
// match by substring.
func isTransientConnError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, driver.ErrBadConn) {
		return true
	}
	msg := err.Error()
	if strings.Contains(msg, "invalid connection") ||
		strings.Contains(msg, "bad record MAC") ||
		strings.Contains(msg, "tls:") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset by peer") {
		return true
	}
	return false
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

	driver, err := migratemysql.WithInstance(db.DB, &migratemysql.Config{})
	if err != nil {
		return fmt.Errorf("init migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "mysql", driver)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// BuildMultiRowInsert builds a multi-row INSERT query.
func BuildMultiRowInsert(table string, columns []string, rowCount int) string {
	placeholder := "(" + strings.Repeat("?, ", len(columns)-1) + "?)"
	values := strings.Repeat(placeholder+", ", rowCount-1) + placeholder
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", table, strings.Join(columns, ", "), values)
}

