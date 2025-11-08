package converter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertMarkdownToPDF(t *testing.T) {
	tests := []struct {
		name          string
		markdownPath  string
		setupFile     func(t *testing.T) string
		wantErr       bool
		wantErrMsg    string
		validateAfter func(t *testing.T, pdfPath string)
	}{
		{
			name:         "invalid extension",
			markdownPath: "test.txt",
			wantErr:      true,
			wantErrMsg:   "input file must have .md extension",
		},
		{
			name:         "file not found",
			markdownPath: "nonexistent.md",
			wantErr:      true,
			wantErrMsg:   "os.ReadFile",
		},
		{
			name: "successful conversion",
			setupFile: func(t *testing.T) string {
				tmpDir := t.TempDir()
				mdPath := filepath.Join(tmpDir, "test.md")
				content := []byte("# Test Document\n\nThis is a test markdown file.\n")
				require.NoError(t, os.WriteFile(mdPath, content, 0644))
				return mdPath
			},
			wantErr: false,
			validateAfter: func(t *testing.T, pdfPath string) {
				_, err := os.Stat(pdfPath)
				assert.NoError(t, err, "PDF file should be created")
				assert.True(t, filepath.Ext(pdfPath) == ".pdf", "PDF file should have .pdf extension")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mdPath string
			if tt.setupFile != nil {
				mdPath = tt.setupFile(t)
			} else {
				mdPath = tt.markdownPath
			}

			pdfPath, err := ConvertMarkdownToPDF(mdPath)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, pdfPath)

			if tt.validateAfter != nil {
				tt.validateAfter(t, pdfPath)
			}

			if pdfPath != "" {
				defer os.Remove(pdfPath)
			}
		})
	}
}
