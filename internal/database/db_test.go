package database

import (
	"testing"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
