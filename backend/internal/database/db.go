// Package database provides database connection management.
package database

import (
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/config"
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
	if cfg.TLS {
		mysqlCfg.TLSConfig = "true"
	}
	if len(cfg.Params) > 0 {
		mysqlCfg.Params = cfg.Params
	}

	db, err := sqlx.Open("mysql", mysqlCfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("sqlx.Open() > %w", err)
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

