// Package gutenberg converts a raw Project Gutenberg plain-text ebook into an
// in-memory "book" notebook bundle (index.yml + per-chapter YAML) that is
// byte-compatible with the existing story/book reader.
//
// The bundle reuses the exported notebook shapes from internal/ebook
// (NotebookIndex / NotebookEntry / NotebookScene) so the emitted YAML parses
// identically to notebooks produced by the EPUB pipeline.
//
// Parse is a pure function (no network, no filesystem) so it is fully unit
// testable offline. The network calls that feed it (Gutendex search + plain
// text fetch) live in client.go and are kept strictly separate.
package gutenberg

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/at-ishikawa/langner/internal/ebook"
	"gopkg.in/yaml.v3"
)

// Book is a catalog entry resolved from Gutendex. SourceID is the Gutenberg
// book id (as a string); TextURL points at the plain-text format to download.
type Book struct {
	SourceID string
	Title    string
	Author   string
	Language string
	TextURL  string
}

// BundleFile is a single emitted file: Path is relative (e.g. "index.yml",
// "001-chapter.yml") and Content is its marshalled YAML bytes.
type BundleFile struct {
	Path    string
	Content []byte
}

// Bundle is the full in-memory notebook directory for one book.
type Bundle struct {
	Files []BundleFile
}

// Defensive caps. A real book fits comfortably under these; the caps only
// guard against pathological input (e.g. a file with millions of "heading"
// lines) blowing up memory. The full book is still imported.
const (
	maxChapters            = 5000
	maxSentencesPerChapter = 200000
)

// Project Gutenberg wraps the actual text in boilerplate delimited by these
// markers. We tolerate both the current ("THE") and older ("THIS") phrasing.
var (
	pgStartMarker = regexp.MustCompile(`(?i)\*\*\*\s*START OF (?:THE|THIS) PROJECT GUTENBERG EBOOK.*`)
	pgEndMarker   = regexp.MustCompile(`(?i)\*\*\*\s*END OF (?:THE|THIS) PROJECT GUTENBERG EBOOK.*`)
)

// Chapter-heading heuristics. Lines starting with CHAPTER/BOOK/PART, bare
// roman numerals, or short all-caps lines are treated as chapter boundaries.
var (
	chapterWordPattern  = regexp.MustCompile(`(?i)^(?:chapter|book|part)\b`)
	romanNumeralPattern = regexp.MustCompile(`^[IVXLCDM]+\.?$`)
)

// whitespacePattern and the sentence splitter mirror the equivalents in
// internal/ebook (which are unexported there). They are intentionally kept
// simple: ebook additionally guards against abbreviations ("Mr.", "e.g."),
// but for plain-text import a straightforward punctuation split is sufficient
// and keeps the output deterministic.
var (
	whitespacePattern  = regexp.MustCompile(`\s+`)
	sentenceEndPattern = regexp.MustCompile(`[.!?]["']?(?:\s+|$)`)
)

// Parse converts a raw Project Gutenberg plain-text ebook into a notebook
// Bundle. It is pure: no network and no filesystem access. title/author are
// supplied by the caller (resolved from catalog metadata); author may be
// empty.
func Parse(raw, title, author string) (Bundle, error) {
	if strings.TrimSpace(title) == "" {
		return Bundle{}, fmt.Errorf("title is required")
	}

	body := stripBoilerplate(raw)
	chapters := splitChapters(body, title)

	var (
		files         []BundleFile
		notebookPaths []string
		chapterNo     int
	)
	for _, ch := range chapters {
		// Collect sentences across all paragraphs of the chapter into a single
		// scene (one scene per chapter, keeping segmentation simple).
		var statements []string
		for _, para := range ch.paras {
			for _, s := range splitSentences(para) {
				statements = append(statements, s)
				if len(statements) >= maxSentencesPerChapter {
					break
				}
			}
			if len(statements) >= maxSentencesPerChapter {
				break
			}
		}
		if len(statements) == 0 {
			// Skip empty chapters (e.g. a heading with no body text).
			continue
		}

		chapterNo++
		if chapterNo > maxChapters {
			break
		}

		entry := ebook.NotebookEntry{
			Event: ch.title,
			Scenes: []ebook.NotebookScene{
				{Scene: ch.title, Statements: statements},
			},
		}

		// The reader loads each notebook file as a LIST of entries, so marshal
		// a single-element list to match internal/ebook's output format.
		data, err := yaml.Marshal([]ebook.NotebookEntry{entry})
		if err != nil {
			return Bundle{}, fmt.Errorf("marshal chapter %d: %w", chapterNo, err)
		}

		path := fmt.Sprintf("%03d-%s.yml", chapterNo, slug(ch.title))
		files = append(files, BundleFile{Path: path, Content: data})
		notebookPaths = append(notebookPaths, path)
	}

	name := title
	if strings.TrimSpace(author) != "" {
		name = fmt.Sprintf("%s by %s", title, author)
	}

	index := ebook.NotebookIndex{
		ID:            slug(title),
		Name:          name,
		NotebookPaths: notebookPaths,
	}
	indexData, err := yaml.Marshal(index)
	if err != nil {
		return Bundle{}, fmt.Errorf("marshal index: %w", err)
	}

	// index.yml first so the bundle reads top-down like the on-disk layout.
	bundle := Bundle{Files: make([]BundleFile, 0, len(files)+1)}
	bundle.Files = append(bundle.Files, BundleFile{Path: "index.yml", Content: indexData})
	bundle.Files = append(bundle.Files, files...)
	return bundle, nil
}

// stripBoilerplate removes the Project Gutenberg header (everything up to and
// including the START marker line) and footer (the END marker line onward). If
// a marker is missing the corresponding end of the text is kept as-is.
func stripBoilerplate(raw string) string {
	lines := strings.Split(raw, "\n")

	start := 0
	for i, line := range lines {
		if pgStartMarker.MatchString(line) {
			start = i + 1 // content begins after the START marker
			break
		}
	}

	end := len(lines)
	for i := start; i < len(lines); i++ {
		if pgEndMarker.MatchString(lines[i]) {
			end = i // content stops before the END marker
			break
		}
	}

	return strings.Join(lines[start:end], "\n")
}

// rawChapter is an intermediate holder: a title plus its cleaned paragraphs.
type rawChapter struct {
	title string
	paras []string
}

// splitChapters breaks the body into chapters on heading lines, grouping
// blank-line-separated paragraphs within each chapter. If no headings are
// detected the whole body becomes a single chapter titled after the book.
func splitChapters(body, title string) []rawChapter {
	lines := strings.Split(body, "\n")

	var chapters []rawChapter
	curIdx := -1
	var para strings.Builder

	flushPara := func() {
		text := cleanText(para.String())
		para.Reset()
		if text == "" {
			return
		}
		if curIdx == -1 {
			// Content before any heading → default chapter named after the book.
			chapters = append(chapters, rawChapter{title: title})
			curIdx = len(chapters) - 1
		}
		chapters[curIdx].paras = append(chapters[curIdx].paras, text)
	}

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			flushPara() // blank line ends the current paragraph
			continue
		}
		if isChapterHeading(line) {
			flushPara()
			chapters = append(chapters, rawChapter{title: cleanText(line)})
			curIdx = len(chapters) - 1
			continue
		}
		if para.Len() > 0 {
			para.WriteByte(' ')
		}
		para.WriteString(strings.TrimSpace(line))
	}
	flushPara()

	return chapters
}

// isChapterHeading reports whether a line looks like a chapter boundary.
func isChapterHeading(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" {
		return false
	}
	if chapterWordPattern.MatchString(t) {
		return true
	}
	if romanNumeralPattern.MatchString(t) {
		return true
	}
	// Short all-caps lines (headings like "THE BEGINNING") with at least one
	// letter and no lowercase letters.
	if len([]rune(t)) <= 60 && hasLetter(t) && isAllCaps(t) {
		return true
	}
	return false
}

func hasLetter(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

// isAllCaps reports whether s contains no lowercase ASCII letters.
func isAllCaps(s string) bool {
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			return false
		}
	}
	return true
}

// splitSentences splits a paragraph into sentences on sentence-ending
// punctuation. Mirrors internal/ebook's splitSentences but simplified (see the
// note on sentenceEndPattern above).
func splitSentences(text string) []string {
	text = cleanText(text)
	if text == "" {
		return nil
	}

	var sentences []string
	start := 0
	for _, loc := range sentenceEndPattern.FindAllStringIndex(text, -1) {
		s := strings.TrimSpace(text[start:loc[1]])
		if s != "" {
			sentences = append(sentences, s)
		}
		start = loc[1]
	}
	if start < len(text) {
		if s := strings.TrimSpace(text[start:]); s != "" {
			sentences = append(sentences, s)
		}
	}
	if len(sentences) == 0 {
		sentences = append(sentences, text)
	}
	return sentences
}

// cleanText normalizes all runs of whitespace to a single space and trims.
// Mirrors internal/ebook's cleanText.
func cleanText(s string) string {
	return strings.TrimSpace(whitespacePattern.ReplaceAllString(s, " "))
}

// slug builds a filesystem-safe, deterministic slug from a title for use in
// chapter filenames. Falls back to "chapter" when nothing usable remains.
func slug(s string) string {
	s = strings.ToLower(cleanText(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "chapter"
	}
	return out
}
