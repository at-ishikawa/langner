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
				Port:     3306,
				Database: "testdb",
				Username: "testuser",
				Password: "testpass",
			},
		},
		{
			name: "creates connection with custom port",
			cfg: config.DatabaseConfig{
				Host:     "db.example.com",
				Port:     3307,
				Database: "langner",
				Username: "admin",
				Password: "secret",
			},
		},
		{
			name: "creates connection with pool settings",
			cfg: config.DatabaseConfig{
				Host:            "localhost",
				Port:            3306,
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
				Port:     3306,
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
				Port:     3306,
				Database: "testdb",
				Username: "testuser",
				Password: "testpass",
				Params:   map[string]string{"charset": "utf8mb4", "loc": "UTC"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Open(tt.cfg)
			require.NoError(t, err)
			require.NotNil(t, got)
			defer got.Close()

			assert.Equal(t, "mysql", got.DriverName())
		})
	}
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

			sqlxDB := sqlx.NewDb(db, "mysql")
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
