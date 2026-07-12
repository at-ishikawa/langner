package database

import (
	"context"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/config"
)

func TestOpen(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.DatabaseConfig
	}{
		{
			name: "creates connection with valid config",
			cfg: config.DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				Database: "testdb",
				Username: "testuser",
				Password: "testpass",
			},
		},
		{
			name: "creates connection with custom port",
			cfg: config.DatabaseConfig{
				Host:     "db.example.com",
				Port:     5433,
				Database: "langner",
				Username: "admin",
				Password: "secret",
			},
		},
		{
			name: "creates connection with pool settings",
			cfg: config.DatabaseConfig{
				Host:            "localhost",
				Port:            5432,
				Database:        "testdb",
				Username:        "testuser",
				Password:        "testpass",
				MaxOpenConns:    25,
				MaxIdleConns:    5,
				ConnMaxLifetime: 300,
			},
		},
		{
			name: "creates connection with TLS enabled",
			cfg: config.DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				Database: "testdb",
				Username: "testuser",
				Password: "testpass",
				TLS:      true,
			},
		},
		{
			name: "creates connection with custom params",
			cfg: config.DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				Database: "testdb",
				Username: "testuser",
				Password: "testpass",
				Params:   map[string]string{"application_name": "langner", "sslmode": "prefer"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Open(tt.cfg)
			require.NoError(t, err)
			require.NotNil(t, got)
			defer got.Close()

			assert.Equal(t, "pgx", got.DriverName())
		})
	}
}

func TestBuildDSN(t *testing.T) {
	t.Run("defaults to sslmode=disable", func(t *testing.T) {
		dsn := buildDSN(config.DatabaseConfig{
			Host: "localhost", Port: 5432, Database: "db", Username: "u", Password: "p",
		})
		assert.Contains(t, dsn, "postgres://u:p@localhost:5432/db")
		assert.Contains(t, dsn, "sslmode=disable")
	})
	t.Run("TLS enables sslmode=require", func(t *testing.T) {
		dsn := buildDSN(config.DatabaseConfig{
			Host: "h", Port: 5432, Database: "db", Username: "u", Password: "p", TLS: true,
		})
		assert.Contains(t, dsn, "sslmode=require")
	})
	t.Run("Params override sslmode", func(t *testing.T) {
		dsn := buildDSN(config.DatabaseConfig{
			Host: "h", Port: 5432, Database: "db", Username: "u", Password: "p",
			Params: map[string]string{"sslmode": "verify-full"},
		})
		assert.Contains(t, dsn, "sslmode=verify-full")
		assert.NotContains(t, dsn, "sslmode=disable")
	})
	t.Run("defaults to pgx exec mode to stay pooler-safe", func(t *testing.T) {
		dsn := buildDSN(config.DatabaseConfig{
			Host: "h", Port: 5432, Database: "db", Username: "u", Password: "p",
		})
		assert.Contains(t, dsn, "default_query_exec_mode=exec")
	})
	t.Run("Params override the exec mode", func(t *testing.T) {
		dsn := buildDSN(config.DatabaseConfig{
			Host: "h", Port: 5432, Database: "db", Username: "u", Password: "p",
			Params: map[string]string{"default_query_exec_mode": "simple_protocol"},
		})
		assert.Contains(t, dsn, "default_query_exec_mode=simple_protocol")
		assert.NotContains(t, dsn, "default_query_exec_mode=exec")
	})
}

func TestBuildMultiRowInsert(t *testing.T) {
	t.Run("single row", func(t *testing.T) {
		got := BuildMultiRowInsert("notes", []string{"a", "b"}, 1)
		assert.Equal(t, "INSERT INTO notes (a, b) VALUES ($1, $2)", got)
	})
	t.Run("multi row", func(t *testing.T) {
		got := BuildMultiRowInsert("notes", []string{"a", "b", "c"}, 3)
		assert.Equal(t, "INSERT INTO notes (a, b, c) VALUES ($1, $2, $3), ($4, $5, $6), ($7, $8, $9)", got)
	})
}

func TestRunInTx(t *testing.T) {
	tests := []struct {
		name      string
		fn        func(ctx context.Context, tx *sqlx.Tx) error
		setupMock func(mock sqlmock.Sqlmock)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "commits on success",
			fn: func(ctx context.Context, tx *sqlx.Tx) error {
				return nil
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectCommit()
			},
		},
		{
			name: "rolls back on error",
			fn: func(ctx context.Context, tx *sqlx.Tx) error {
				return fmt.Errorf("something failed")
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectRollback()
			},
			wantErr: true,
			errMsg:  "something failed",
		},
		{
			name: "begin error",
			fn: func(ctx context.Context, tx *sqlx.Tx) error {
				return nil
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin().WillReturnError(fmt.Errorf("begin failed"))
			},
			wantErr: true,
			errMsg:  "begin transaction",
		},
		{
			name: "commit error",
			fn: func(ctx context.Context, tx *sqlx.Tx) error {
				return nil
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectCommit().WillReturnError(fmt.Errorf("commit failed"))
			},
			wantErr: true,
			errMsg:  "commit transaction",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			sqlxDB := sqlx.NewDb(db, "pgx")
			tt.setupMock(mock)

			err = RunInTx(context.Background(), sqlxDB, tt.fn)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
