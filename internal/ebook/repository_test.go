package ebook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	m := NewManager("/repos", "/repos.yml", "/books")
	assert.Equal(t, "/repos", m.repoDirectory)
	assert.Equal(t, "/repos.yml", m.repositoriesFile)
	assert.Equal(t, "/books", m.booksDirectory)
}

func TestManager_List(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, dir string) string
		want    []Repository
		wantErr bool
	}{
		{
			name: "returns empty when file does not exist",
			setup: func(t *testing.T, dir string) string {
				return filepath.Join(dir, "nonexistent.yml")
			},
			want: []Repository{},
		},
		{
			name: "returns repositories from file",
			setup: func(t *testing.T, dir string) string {
				filePath := filepath.Join(dir, "repos.yml")
				content := `repositories:
  - id: test-book
    repo_path: /repos/test-book
    source_url: https://github.com/standardebooks/test-book
    web_url: https://standardebooks.org/ebooks/test/book
    title: Test Book
    author: Test Author
`
				require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))
				return filePath
			},
			want: []Repository{
				{
					ID:        "test-book",
					RepoPath:  "/repos/test-book",
					SourceURL: "https://github.com/standardebooks/test-book",
					WebURL:    "https://standardebooks.org/ebooks/test/book",
					Title:     "Test Book",
					Author:    "Test Author",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			reposFile := tt.setup(t, tmpDir)
			m := NewManager(tmpDir, reposFile, tmpDir)

			got, err := m.List()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestManager_Remove(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		setup   func(t *testing.T, dir string) (reposFile string)
		wantErr bool
	}{
		{
			name: "removes existing repository",
			id:   "test-book",
			setup: func(t *testing.T, dir string) string {
				// Create repo directory
				repoPath := filepath.Join(dir, "repos", "test-book")
				require.NoError(t, os.MkdirAll(repoPath, 0755))

				// Create books directory
				booksPath := filepath.Join(dir, "books", "test-book")
				require.NoError(t, os.MkdirAll(booksPath, 0755))

				// Create repos config
				filePath := filepath.Join(dir, "repos.yml")
				content := `repositories:
  - id: test-book
    repo_path: ` + repoPath + `
    title: Test Book
    author: Test Author
  - id: other-book
    repo_path: /other
    title: Other Book
    author: Other Author
`
				require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))
				return filePath
			},
		},
		{
			name: "returns error when ID not found",
			id:   "nonexistent",
			setup: func(t *testing.T, dir string) string {
				filePath := filepath.Join(dir, "repos.yml")
				content := `repositories:
  - id: test-book
    title: Test Book
`
				require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))
				return filePath
			},
			wantErr: true,
		},
		{
			name: "returns error when no config file",
			id:   "test",
			setup: func(t *testing.T, dir string) string {
				return filepath.Join(dir, "nonexistent.yml")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			reposFile := tt.setup(t, tmpDir)
			booksDir := filepath.Join(tmpDir, "books")
			m := NewManager(filepath.Join(tmpDir, "repos"), reposFile, booksDir)

			err := m.Remove(tt.id)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Verify repository was removed from config
			repos, err := m.List()
			require.NoError(t, err)
			for _, r := range repos {
				assert.NotEqual(t, tt.id, r.ID)
			}
		})
	}
}

func TestManager_addRepository(t *testing.T) {
	tests := []struct {
		name    string
		repo    Repository
		setup   func(t *testing.T, dir string) string
		wantErr bool
	}{
		{
			name: "adds to empty config",
			repo: Repository{ID: "new-book", Title: "New Book"},
			setup: func(t *testing.T, dir string) string {
				return filepath.Join(dir, "repos.yml")
			},
		},
		{
			name: "adds to existing config",
			repo: Repository{ID: "second-book", Title: "Second Book"},
			setup: func(t *testing.T, dir string) string {
				filePath := filepath.Join(dir, "repos.yml")
				content := `repositories:
  - id: first-book
    title: First Book
`
				require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))
				return filePath
			},
		},
		{
			name: "returns error for duplicate ID",
			repo: Repository{ID: "existing", Title: "Duplicate"},
			setup: func(t *testing.T, dir string) string {
				filePath := filepath.Join(dir, "repos.yml")
				content := `repositories:
  - id: existing
    title: Existing Book
`
				require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))
				return filePath
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			reposFile := tt.setup(t, tmpDir)
			m := NewManager(tmpDir, reposFile, tmpDir)

			err := m.addRepository(tt.repo)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Verify the repo was added
			repos, err := m.List()
			require.NoError(t, err)
			found := false
			for _, r := range repos {
				if r.ID == tt.repo.ID {
					found = true
					break
				}
			}
			assert.True(t, found, "added repository should be in list")
		})
	}
}
