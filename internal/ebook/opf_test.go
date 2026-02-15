package ebook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOPF(t *testing.T) {
	// Create a temporary directory structure
	tempDir := t.TempDir()
	opfDir := filepath.Join(tempDir, "src", "epub")
	require.NoError(t, os.MkdirAll(opfDir, 0755))

	// Create a sample content.opf file
	opfContent := `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" dir="ltr" prefix="se: https://standardebooks.org/vocab/1.0" unique-identifier="uid" version="3.0" xml:lang="en-US">
    <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
        <dc:title id="title">Frankenstein</dc:title>
        <dc:creator id="author">Mary Shelley</dc:creator>
    </metadata>
</package>`

	opfPath := filepath.Join(opfDir, "content.opf")
	require.NoError(t, os.WriteFile(opfPath, []byte(opfContent), 0644))

	// Test parsing
	title, author, err := parseOPF(tempDir)
	require.NoError(t, err)
	assert.Equal(t, "Frankenstein", title)
	assert.Equal(t, "Mary Shelley", author)
}

func TestParseOPF_FileNotFound(t *testing.T) {
	tempDir := t.TempDir()

	_, _, err := parseOPF(tempDir)
	require.Error(t, err)
}
