package notebook

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDate_MarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name        string
		yamlInput   string
		expectError bool
		expectedDay string // YYYY-MM-DD format
	}{
		{
			name:        "old YYYY-MM-DD format (backward compatible)",
			yamlInput:   `learned_at: "2025-06-13"`,
			expectError: false,
			expectedDay: "2025-06-13",
		},
		{
			name:        "RFC3339 format",
			yamlInput:   `learned_at: 2025-05-02T00:00:00Z`,
			expectError: false,
			expectedDay: "2025-05-02",
		},
		{
			name:        "RFC3339Nano format with timezone",
			yamlInput:   `learned_at: 2025-06-04T20:05:49.744339678-07:00`,
			expectError: false,
			expectedDay: "2025-06-04",
		},
		{
			name:        "invalid format",
			yamlInput:   `learned_at: "invalid-date"`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var record struct {
				LearnedAt Date `yaml:"learned_at"`
			}

			err := yaml.Unmarshal([]byte(tt.yamlInput), &record)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedDay, record.LearnedAt.Format("2006-01-02"))

			// Test that marshaling produces RFC3339 format (with full timestamp)
			data, err := yaml.Marshal(record)
			require.NoError(t, err)

			// Should contain RFC3339 format
			assert.Contains(t, string(data), "learned_at:")
			// Verify the date part is preserved
			assert.Contains(t, string(data), tt.expectedDay)
		})
	}
}

func TestDate_NewDate(t *testing.T) {
	t.Run("without argument uses current time", func(t *testing.T) {
		date := NewDate()
		assert.False(t, date.IsZero())

		// Should be close to current time (within 1 second)
		now := time.Now()
		diff := now.Sub(date.Time)
		assert.True(t, diff < time.Second && diff > -time.Second)
	})

	t.Run("with argument uses provided time", func(t *testing.T) {
		testTime := time.Date(2025, 6, 13, 14, 30, 0, 0, time.UTC)
		date := NewDate(testTime)

		assert.Equal(t, testTime, date.Time)
		assert.Equal(t, "2025-06-13", date.Format("2006-01-02"))
	})
}
