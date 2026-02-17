package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunAnalyzeReport(t *testing.T) {
	tests := []struct {
		name       string
		setupFiles map[string]string
		year       int
		month      int
		wantErr    bool
	}{
		{
			name: "valid learning history",
			setupFiles: map[string]string{
				"notebook1.yml": `- metadata:
    id: test-id
    title: Test Notebook
  scenes:
    - metadata:
        title: Scene 1
      expressions:
        - expression: hello
          learned_logs:
            - status: understood
              learned_at: "2025-06-01"
              quality: 4
              interval_days: 3`,
			},
			year:    2025,
			month:   6,
			wantErr: false,
		},
		{
			name:       "empty directory",
			setupFiles: map[string]string{},
			year:       2025,
			month:      6,
			wantErr:    false,
		},
		{
			name:    "nonexistent directory",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "nonexistent directory" {
				err := RunAnalyzeReport("/nonexistent/directory", tt.year, tt.month)
				assert.Error(t, err)
				return
			}

			tempDir := t.TempDir()

			for filename, content := range tt.setupFiles {
				err := os.WriteFile(filepath.Join(tempDir, filename), []byte(content), 0644)
				require.NoError(t, err)
			}

			err := RunAnalyzeReport(tempDir, tt.year, tt.month)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
