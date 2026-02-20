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
		name     string
		repo     Repository
		chapters []Chapter
		wantErr  bool
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tempDir := t.TempDir()

			err := GenerateNotebooks(tt.repo, tt.chapters, tempDir)
			if tt.wantErr {
				require.Error(t, err)
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
