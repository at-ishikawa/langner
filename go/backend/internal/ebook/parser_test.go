package ebook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

func TestSplitSentences(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:  "Single letter initial M. Waldman",
			input: "In M. Waldman I found a true friend.",
			expected: []string{
				"In M. Waldman I found a true friend.",
			},
		},
		{
			name:  "Multiple single letter initials",
			input: "Mr. J. Smith met Dr. A. Brown at the office.",
			expected: []string{
				"Mr. J. Smith met Dr. A. Brown at the office.",
			},
		},
		{
			name:  "Simple two sentences",
			input: "Hello world. How are you?",
			expected: []string{
				"Hello world.",
				"How are you?",
			},
		},
		{
			name:  "Sentence with Mr. abbreviation",
			input: "Mr. Smith went to the store. He bought milk.",
			expected: []string{
				"Mr. Smith went to the store.",
				"He bought milk.",
			},
		},
		{
			name:  "Sentence ending with abbreviation-like word",
			input: "The meeting ended at 5 PM. Everyone left.",
			expected: []string{
				"The meeting ended at 5 PM.",
				"Everyone left.",
			},
		},
		{
			name:  "Single letter initial at start",
			input: "M. Krempe was a professor. He taught chemistry.",
			expected: []string{
				"M. Krempe was a professor.",
				"He taught chemistry.",
			},
		},
		{
			name:  "Date line with month abbreviation",
			input: "St. Petersburgh, Dec. 11th, 17—.",
			expected: []string{
				"St. Petersburgh, Dec. 11th, 17—.",
			},
		},
		{
			name:  "Repeated exclamation with lowercase continuation",
			input: `"Oh, save me! save me!" I imagined that the monster seized me.`,
			expected: []string{
				// Split at !" I because I is uppercase (narrator continues after dialogue)
				`"Oh, save me! save me!"`,
				"I imagined that the monster seized me.",
			},
		},
		{
			name:  "Lowercase after exclamation keeps together",
			input: `Oh, save me! save me! Please help!`,
			expected: []string{
				// "save" is lowercase so stays with previous exclamation
				"Oh, save me! save me!",
				"Please help!",
			},
		},
		{
			name:  "Exclamation followed by capital letter starts new sentence",
			input: "Stop! Don't move.",
			expected: []string{
				"Stop!",
				"Don't move.",
			},
		},
		{
			name:  "Question with lowercase continuation in dialogue",
			input: `"What is it? he asked nervously.`,
			expected: []string{
				`"What is it? he asked nervously.`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitSentences(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsAbbreviation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"Mr.", "Hello Mr.", true},
		{"Dr.", "Visit Dr.", true},
		{"Single letter M.", "In M.", true},
		{"Single letter J.", "Meet J.", true},
		{"Single letter at start", "M.", true},
		{"Not abbreviation AM.", "10 AM.", false}, // AM is not preceded by space
		{"Regular word ending in period", "Hello.", false},
		{"etc.", "and so on etc.", true},
		{"Dec. month abbreviation", "On Dec.", true},
		{"Jan. month abbreviation", "In Jan.", true},
		{"St. abbreviation", "Visit St.", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAbbreviation(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsMetadataFile(t *testing.T) {
	tests := []struct {
		name string
		file string
		want bool
	}{
		{name: "colophon", file: "colophon.xhtml", want: true},
		{name: "imprint", file: "imprint.xhtml", want: true},
		{name: "titlepage", file: "titlepage.xhtml", want: true},
		{name: "toc", file: "toc.xhtml", want: true},
		{name: "halftitlepage", file: "halftitlepage.xhtml", want: true},
		{name: "loi", file: "loi.xhtml", want: true},
		{name: "uncopyright", file: "uncopyright.xhtml", want: true},
		{name: "chapter file", file: "chapter-1.xhtml", want: false},
		{name: "preface", file: "preface.xhtml", want: false},
		{name: "introduction", file: "introduction.xhtml", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isMetadataFile(tt.file)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCleanText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "no change", input: "hello world", want: "hello world"},
		{name: "trim whitespace", input: "  hello  ", want: "hello"},
		{name: "collapse multiple spaces", input: "hello   world", want: "hello world"},
		{name: "collapse newlines and tabs", input: "hello\n\tworld", want: "hello world"},
		{name: "empty string", input: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanText(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		name     string
		htmlStr  string
		want     string
	}{
		{
			name:    "extracts title element",
			htmlStr: `<html><head><title>Chapter One</title></head><body><p>Text</p></body></html>`,
			want:    "Chapter One",
		},
		{
			name:    "falls back to h1",
			htmlStr: `<html><head></head><body><h1>Introduction</h1><p>Text</p></body></html>`,
			want:    "Introduction",
		},
		{
			name:    "falls back to h2",
			htmlStr: `<html><head></head><body><h2>Preface</h2><p>Text</p></body></html>`,
			want:    "Preface",
		},
		{
			name:    "prefers title over heading",
			htmlStr: `<html><head><title>From Title</title></head><body><h1>From Heading</h1></body></html>`,
			want:    "From Title",
		},
		{
			name:    "returns empty for no title or heading",
			htmlStr: `<html><head></head><body><p>Just text.</p></body></html>`,
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := html.Parse(strings.NewReader(tt.htmlStr))
			require.NoError(t, err)
			got := extractTitle(doc)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractParagraphs(t *testing.T) {
	tests := []struct {
		name    string
		htmlStr string
		want    []Paragraph
	}{
		{
			name:    "single paragraph",
			htmlStr: `<html><body><p>Hello world.</p></body></html>`,
			want: []Paragraph{
				{Text: "Hello world.", Sentences: []string{"Hello world."}, InBlockquote: false},
			},
		},
		{
			name:    "multiple paragraphs",
			htmlStr: `<html><body><p>First paragraph.</p><p>Second paragraph.</p></body></html>`,
			want: []Paragraph{
				{Text: "First paragraph.", Sentences: []string{"First paragraph."}, InBlockquote: false},
				{Text: "Second paragraph.", Sentences: []string{"Second paragraph."}, InBlockquote: false},
			},
		},
		{
			name:    "paragraph in blockquote",
			htmlStr: `<html><body><blockquote><p>Quoted text.</p></blockquote></body></html>`,
			want: []Paragraph{
				{Text: "Quoted text.", Sentences: []string{"Quoted text."}, InBlockquote: true},
			},
		},
		{
			name:    "empty paragraph is skipped",
			htmlStr: `<html><body><p>  </p><p>Real text.</p></body></html>`,
			want: []Paragraph{
				{Text: "Real text.", Sentences: []string{"Real text."}, InBlockquote: false},
			},
		},
		{
			name:    "no paragraphs",
			htmlStr: `<html><body><div>No p tags here.</div></body></html>`,
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := html.Parse(strings.NewReader(tt.htmlStr))
			require.NoError(t, err)
			got := extractParagraphs(doc)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseChapterFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    Chapter
		wantErr bool
	}{
		{
			name: "basic chapter",
			content: `<?xml version="1.0" encoding="utf-8"?>
<html><head><title>Chapter I</title></head><body>
<h1>Chapter I</h1>
<p>It was a dark and stormy night.</p>
<p>The rain fell in torrents.</p>
</body></html>`,
			want: Chapter{
				Title: "Chapter I",
				Paragraphs: []Paragraph{
					{Text: "It was a dark and stormy night.", Sentences: []string{"It was a dark and stormy night."}, InBlockquote: false},
					{Text: "The rain fell in torrents.", Sentences: []string{"The rain fell in torrents."}, InBlockquote: false},
				},
			},
		},
		{
			name: "chapter with blockquote",
			content: `<?xml version="1.0" encoding="utf-8"?>
<html><head><title>Letter I</title></head><body>
<blockquote><p>A quoted passage from the letter.</p></blockquote>
</body></html>`,
			want: Chapter{
				Title: "Letter I",
				Paragraphs: []Paragraph{
					{Text: "A quoted passage from the letter.", Sentences: []string{"A quoted passage from the letter."}, InBlockquote: true},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			filePath := filepath.Join(tmpDir, "chapter.xhtml")
			require.NoError(t, os.WriteFile(filePath, []byte(tt.content), 0644))

			got, err := parseChapterFile(filePath)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want.Title, got.Title)
			assert.Equal(t, tt.want.Paragraphs, got.Paragraphs)
		})
	}
}

func TestParseSpineOrder(t *testing.T) {
	tests := []struct {
		name       string
		opfContent string
		want       []string
		wantErr    bool
	}{
		{
			name: "basic spine order",
			opfContent: `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf">
    <spine>
        <itemref idref="titlepage.xhtml"/>
        <itemref idref="chapter-1.xhtml"/>
        <itemref idref="chapter-2.xhtml"/>
        <itemref idref="colophon.xhtml"/>
    </spine>
</package>`,
			want: []string{"titlepage.xhtml", "chapter-1.xhtml", "chapter-2.xhtml", "colophon.xhtml"},
		},
		{
			name: "spine without xhtml extension",
			opfContent: `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf">
    <spine>
        <itemref idref="chapter-1"/>
    </spine>
</package>`,
			want: []string{"chapter-1.xhtml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			opfDir := filepath.Join(tmpDir, "src", "epub")
			require.NoError(t, os.MkdirAll(opfDir, 0755))
			require.NoError(t, os.WriteFile(filepath.Join(opfDir, "content.opf"), []byte(tt.opfContent), 0644))

			got, err := parseSpineOrder(tmpDir)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseToc(t *testing.T) {
	tests := []struct {
		name       string
		tocContent string
		want       map[string]string
		wantErr    bool
	}{
		{
			name: "basic toc with nav",
			tocContent: `<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<body>
<nav id="toc">
    <ol>
        <li><a href="text/chapter-1.xhtml">Chapter I</a></li>
        <li><a href="text/chapter-2.xhtml">Chapter II</a></li>
    </ol>
</nav>
</body>
</html>`,
			want: map[string]string{
				"chapter-1.xhtml": "Chapter I",
				"chapter-2.xhtml": "Chapter II",
			},
		},
		{
			name: "toc with anchor links",
			tocContent: `<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<body>
<nav id="toc">
    <ol>
        <li><a href="text/chapter-24.xhtml#walton-in-continuation">Walton, in Continuation</a></li>
    </ol>
</nav>
</body>
</html>`,
			want: map[string]string{
				"chapter-24.xhtml": "Walton, in Continuation",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tocDir := filepath.Join(tmpDir, "src", "epub")
			require.NoError(t, os.MkdirAll(tocDir, 0755))
			require.NoError(t, os.WriteFile(filepath.Join(tocDir, "toc.xhtml"), []byte(tt.tocContent), 0644))

			got, err := parseToc(tmpDir)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseChapterFile_FileNotFound(t *testing.T) {
	_, err := parseChapterFile("/nonexistent/path/chapter.xhtml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open file")
}

func TestParseChapterFile_TitleFallbackToFilename(t *testing.T) {
	tmpDir := t.TempDir()
	// XHTML with no title element and no heading - should fall back to filename
	content := `<?xml version="1.0" encoding="utf-8"?>
<html><head></head><body>
<p>Some text without a title.</p>
</body></html>`
	filePath := filepath.Join(tmpDir, "epilogue.xhtml")
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))

	got, err := parseChapterFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, "epilogue", got.Title)
}

func TestParseChapters_SpineOrderError(t *testing.T) {
	// No content.opf exists, so parseSpineOrder should fail
	tmpDir := t.TempDir()
	_, err := ParseChapters(tmpDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse spine order")
}

func TestParseChapters_MissingToc(t *testing.T) {
	// Set up ebook with content.opf and chapter file but no toc.xhtml
	tmpDir := t.TempDir()
	textDir := filepath.Join(tmpDir, "src", "epub", "text")
	epubDir := filepath.Join(tmpDir, "src", "epub")
	require.NoError(t, os.MkdirAll(textDir, 0755))

	opfContent := `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf">
    <spine><itemref idref="chapter-1.xhtml"/></spine>
</package>`
	require.NoError(t, os.WriteFile(filepath.Join(epubDir, "content.opf"), []byte(opfContent), 0644))

	chapterContent := `<html><head><title>Chapter 1</title></head><body>
<p>Some text here.</p></body></html>`
	require.NoError(t, os.WriteFile(filepath.Join(textDir, "chapter-1.xhtml"), []byte(chapterContent), 0644))

	chapters, err := ParseChapters(tmpDir)
	require.NoError(t, err)
	require.Len(t, chapters, 1)
	assert.Equal(t, "Chapter 1", chapters[0].Title)
}

func TestParseChapters_NonexistentSpineFile(t *testing.T) {
	// Spine references a file that doesn't exist in the text directory
	tmpDir := t.TempDir()
	textDir := filepath.Join(tmpDir, "src", "epub", "text")
	epubDir := filepath.Join(tmpDir, "src", "epub")
	require.NoError(t, os.MkdirAll(textDir, 0755))

	opfContent := `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf">
    <spine>
        <itemref idref="chapter-1.xhtml"/>
        <itemref idref="nonexistent.xhtml"/>
    </spine>
</package>`
	require.NoError(t, os.WriteFile(filepath.Join(epubDir, "content.opf"), []byte(opfContent), 0644))

	tocContent := `<?xml version="1.0" encoding="utf-8"?>
<html><body><nav id="toc"><ol>
    <li><a href="text/chapter-1.xhtml">Chapter I</a></li>
</ol></nav></body></html>`
	require.NoError(t, os.WriteFile(filepath.Join(epubDir, "toc.xhtml"), []byte(tocContent), 0644))

	chapterContent := `<html><head><title>Chapter 1</title></head><body>
<p>Some text here.</p></body></html>`
	require.NoError(t, os.WriteFile(filepath.Join(textDir, "chapter-1.xhtml"), []byte(chapterContent), 0644))

	chapters, err := ParseChapters(tmpDir)
	require.NoError(t, err)
	require.Len(t, chapters, 1)
	assert.Equal(t, "Chapter I", chapters[0].Title)
}

func TestParseChapters(t *testing.T) {
	// Set up a minimal ebook directory structure
	tmpDir := t.TempDir()
	textDir := filepath.Join(tmpDir, "src", "epub", "text")
	epubDir := filepath.Join(tmpDir, "src", "epub")
	require.NoError(t, os.MkdirAll(textDir, 0755))

	// content.opf with spine
	opfContent := `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf">
    <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
        <dc:title>Test Book</dc:title>
        <dc:creator>Test Author</dc:creator>
    </metadata>
    <spine>
        <itemref idref="titlepage.xhtml"/>
        <itemref idref="chapter-1.xhtml"/>
    </spine>
</package>`
	require.NoError(t, os.WriteFile(filepath.Join(epubDir, "content.opf"), []byte(opfContent), 0644))

	// toc.xhtml
	tocContent := `<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<body>
<nav id="toc">
    <ol>
        <li><a href="text/chapter-1.xhtml">The Beginning</a></li>
    </ol>
</nav>
</body>
</html>`
	require.NoError(t, os.WriteFile(filepath.Join(epubDir, "toc.xhtml"), []byte(tocContent), 0644))

	// chapter-1.xhtml (titlepage.xhtml is metadata, will be skipped)
	chapterContent := `<?xml version="1.0" encoding="utf-8"?>
<html><head><title>Chapter 1</title></head><body>
<p>Once upon a time there was a story.</p>
</body></html>`
	require.NoError(t, os.WriteFile(filepath.Join(textDir, "chapter-1.xhtml"), []byte(chapterContent), 0644))

	chapters, err := ParseChapters(tmpDir)
	require.NoError(t, err)
	require.Len(t, chapters, 1)
	assert.Equal(t, "The Beginning", chapters[0].Title) // from toc
	assert.Len(t, chapters[0].Paragraphs, 1)
}
