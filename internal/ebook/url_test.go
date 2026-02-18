package ebook

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeriveURLs(t *testing.T) {
	tests := []struct {
		name          string
		inputURL      string
		wantRepoName  string
		wantSourceURL string
		wantWebURL    string
		wantErr       bool
	}{
		{
			name:          "Standard Ebooks URL",
			inputURL:      "https://standardebooks.org/ebooks/mary-shelley/frankenstein",
			wantRepoName:  "mary-shelley_frankenstein",
			wantSourceURL: "https://github.com/standardebooks/mary-shelley_frankenstein",
			wantWebURL:    "https://standardebooks.org/ebooks/mary-shelley/frankenstein",
		},
		{
			name:          "Standard Ebooks URL with trailing slash",
			inputURL:      "https://standardebooks.org/ebooks/mary-shelley/frankenstein/",
			wantRepoName:  "mary-shelley_frankenstein",
			wantSourceURL: "https://github.com/standardebooks/mary-shelley_frankenstein",
			wantWebURL:    "https://standardebooks.org/ebooks/mary-shelley/frankenstein",
		},
		{
			name:          "GitHub URL",
			inputURL:      "https://github.com/standardebooks/mary-shelley_frankenstein",
			wantRepoName:  "mary-shelley_frankenstein",
			wantSourceURL: "https://github.com/standardebooks/mary-shelley_frankenstein",
			wantWebURL:    "https://standardebooks.org/ebooks/mary-shelley/frankenstein",
		},
		{
			name:          "GitHub URL with .git suffix",
			inputURL:      "https://github.com/standardebooks/mary-shelley_frankenstein.git",
			wantRepoName:  "mary-shelley_frankenstein",
			wantSourceURL: "https://github.com/standardebooks/mary-shelley_frankenstein",
			wantWebURL:    "https://standardebooks.org/ebooks/mary-shelley/frankenstein",
		},
		{
			name:          "Three-part Standard Ebooks URL (with translator)",
			inputURL:      "https://standardebooks.org/ebooks/fyodor-dostoevsky/crime-and-punishment/constance-garnett",
			wantRepoName:  "fyodor-dostoevsky_crime-and-punishment_constance-garnett",
			wantSourceURL: "https://github.com/standardebooks/fyodor-dostoevsky_crime-and-punishment_constance-garnett",
			wantWebURL:    "https://standardebooks.org/ebooks/fyodor-dostoevsky/crime-and-punishment/constance-garnett",
		},
		{
			name:     "Invalid host",
			inputURL: "https://example.com/ebooks/test",
			wantErr:  true,
		},
		{
			name:     "Invalid Standard Ebooks path",
			inputURL: "https://standardebooks.org/ebooks/only-author",
			wantErr:  true,
		},
		{
			name:     "Invalid GitHub URL with subpath",
			inputURL: "https://github.com/standardebooks/org/extra-path",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoName, sourceURL, webURL, err := deriveURLs(tt.inputURL)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantRepoName, repoName)
			assert.Equal(t, tt.wantSourceURL, sourceURL)
			assert.Equal(t, tt.wantWebURL, webURL)
		})
	}
}

func TestDeriveID(t *testing.T) {
	tests := []struct {
		name     string
		repoName string
		wantID   string
	}{
		{
			name:     "Two-part repo name",
			repoName: "mary-shelley_frankenstein",
			wantID:   "frankenstein",
		},
		{
			name:     "Three-part repo name",
			repoName: "fyodor-dostoevsky_crime-and-punishment_constance-garnett",
			wantID:   "constance-garnett",
		},
		{
			name:     "Single-part repo name",
			repoName: "single-part",
			wantID:   "single-part",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveID(tt.repoName)
			assert.Equal(t, tt.wantID, got)
		})
	}
}
