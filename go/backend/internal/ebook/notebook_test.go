package ebook

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateNotebooks(t *testing.T) {
	tests := []struct {
		name       string
		repo       Repository
		chapters   []Chapter
		setup      func(t *testing.T) string // returns booksDir; nil means use t.TempDir()
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "simple chapter",
			repo: Repository{
				ID:     "test-book",
				Title:  "Test Book",
				Author: "Test Author",
			},
			chapters: []Chapter{
				{
					Filename: "chapter-1.xhtml",
					Title:    "Chapter 1",
					Paragraphs: []Paragraph{
						{
							Sentences:    []string{"This is sentence one.", "This is sentence two."},
							InBlockquote: false,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "chapter with blockquote",
			repo: Repository{
				ID:     "test-book",
				Title:  "Test Book",
				Author: "Test Author",
			},
			chapters: []Chapter{
				{
					Filename: "letter-1.xhtml",
					Title:    "Letter I",
					Paragraphs: []Paragraph{
						{
							Sentences:    []string{"This is a quote."},
							InBlockquote: true,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "multiple chapters",
			repo: Repository{
				ID:     "test-book",
				Title:  "Test Book",
				Author: "Test Author",
			},
			chapters: []Chapter{
				{
					Filename: "intro.xhtml",
					Title:    "Introduction",
					Paragraphs: []Paragraph{
						{Sentences: []string{"Intro text."}},
					},
				},
				{
					Filename: "chapter-1.xhtml",
					Title:    "Chapter 1",
					Paragraphs: []Paragraph{
						{Sentences: []string{"Chapter text."}},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "MkdirAll error",
			repo: Repository{
				ID:     "test-book",
				Title:  "Test Book",
				Author: "Test Author",
			},
			chapters: []Chapter{
				{
					Filename: "ch1.xhtml",
					Title:    "Chapter 1",
					Paragraphs: []Paragraph{
						{Sentences: []string{"Hello world."}},
					},
				},
			},
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				filePath := filepath.Join(tempDir, "blocker")
				require.NoError(t, os.WriteFile(filePath, []byte("data"), 0644))
				return filePath
			},
			wantErr:    true,
			wantErrMsg: "create book directory",
		},
		{
			name: "write notebook error",
			repo: Repository{
				ID:     "test-book",
				Title:  "Test Book",
				Author: "Test Author",
			},
			chapters: []Chapter{
				{
					Filename: "ch1.xhtml",
					Title:    "Chapter 1",
					Paragraphs: []Paragraph{
						{Sentences: []string{"Hello world."}},
					},
				},
			},
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				bookDir := filepath.Join(tempDir, "test-book")
				require.NoError(t, os.MkdirAll(bookDir, 0755))
				notebookPath := filepath.Join(bookDir, "001-ch1.yml")
				require.NoError(t, os.MkdirAll(notebookPath, 0755))
				return tempDir
			},
			wantErr:    true,
			wantErrMsg: "write notebook",
		},
		{
			name: "write index error",
			repo: Repository{
				ID:     "test-book",
				Title:  "Test Book",
				Author: "Test Author",
			},
			chapters: []Chapter{},
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				bookDir := filepath.Join(tempDir, "test-book")
				require.NoError(t, os.MkdirAll(bookDir, 0755))
				indexPath := filepath.Join(bookDir, "index.yml")
				require.NoError(t, os.MkdirAll(indexPath, 0755))
				return tempDir
			},
			wantErr:    true,
			wantErrMsg: "write index",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tempDir string
			if tt.setup != nil {
				tempDir = tt.setup(t)
			} else {
				tempDir = t.TempDir()
			}

			err := GenerateNotebooks(tt.repo, tt.chapters, tempDir)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrMsg != "" {
					assert.Contains(t, err.Error(), tt.wantErrMsg)
				}
				return
			}
			require.NoError(t, err)

			// Verify book directory was created
			bookDir := filepath.Join(tempDir, tt.repo.ID)
			_, err = os.Stat(bookDir)
			require.NoError(t, err)

			// Verify index.yml was created
			indexPath := filepath.Join(bookDir, "index.yml")
			_, err = os.Stat(indexPath)
			require.NoError(t, err)

			// Verify notebook files were created
			for i, chapter := range tt.chapters {
				baseName := chapter.Filename[:len(chapter.Filename)-6] // remove .xhtml
				notebookFile := filepath.Join(bookDir, fmt.Sprintf("%03d-%s.yml", i+1, baseName))
				_, err = os.Stat(notebookFile)
				assert.NoError(t, err, "notebook file should exist: %s", notebookFile)
			}
		})
	}
}
