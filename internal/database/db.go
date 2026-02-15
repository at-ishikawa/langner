// Package database provides database connection and migration management.
package database

import (
	"fmt"

	"github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	mysqlMigrate "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/schemas"
)

// Open opens a MySQL connection using the provided config.
func Open(cfg config.DatabaseConfig) (*sqlx.DB, error) {
	mysqlCfg := mysql.NewConfig()
	mysqlCfg.User = cfg.Username
	mysqlCfg.Passwd = cfg.Password
	mysqlCfg.Net = "tcp"
	mysqlCfg.Addr = fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	mysqlCfg.DBName = cfg.Database
	mysqlCfg.ParseTime = true
	mysqlCfg.MultiStatements = true

	db, err := sqlx.Open("mysql", mysqlCfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("sqlx.Open() > %w", err)
	}

	return db, nil
}

// RunMigrations runs all pending database migrations.
func RunMigrations(db *sqlx.DB) error {
	driver, err := mysqlMigrate.WithInstance(db.DB, &mysqlMigrate.Config{})
	if err != nil {
		return fmt.Errorf("mysqlMigrate.WithInstance() > %w", err)
	}

	source, err := iofs.New(schemas.Migrations, "migrations")
	if err != nil {
		return fmt.Errorf("iofs.New() > %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "mysql", driver)
	if err != nil {
		return fmt.Errorf("migrate.NewWithInstance() > %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("m.Up() > %w", err)
	}

	return nil
}
