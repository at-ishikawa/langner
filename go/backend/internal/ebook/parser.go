package ebook

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var whitespacePattern = regexp.MustCompile(`\s+`)
var sentencePattern = regexp.MustCompile(`([.!?]["']?\s+)`)

// Paragraph represents a paragraph with its sentences
type Paragraph struct {
	Text         string   // Full paragraph text
	Sentences    []string // Sentences split from the paragraph
	InBlockquote bool     // Whether this paragraph is inside a blockquote
}

// Chapter represents a parsed chapter from an ebook
type Chapter struct {
	Filename   string
	Title      string
	Paragraphs []Paragraph
}

// ParseChapters parses all chapter XHTML files from an ebook repository
// Uses spine from content.opf for ordering and toc.xhtml for titles
func ParseChapters(repoPath string) ([]Chapter, error) {
	textDir := filepath.Join(repoPath, "src", "epub", "text")

	// Get spine order from content.opf
	spineOrder, err := parseSpineOrder(repoPath)
	if err != nil {
		return nil, fmt.Errorf("parse spine order: %w", err)
	}

	// Get titles from toc.xhtml
	tocTitles, err := parseToc(repoPath)
	if err != nil {
		// Not fatal, we can fall back to extracting from content
		tocTitles = make(map[string]string)
	}

	var chapters []Chapter
	for _, filename := range spineOrder {
		// Skip metadata files
		if isMetadataFile(filename) {
			continue
		}

		filePath := filepath.Join(textDir, filename)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			continue
		}

		chapter, err := parseChapterFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("parse chapter %s: %w", filename, err)
		}

		// Use toc title if available, otherwise use extracted title
		if tocTitle, ok := tocTitles[filename]; ok {
			chapter.Title = tocTitle
		}

		if len(chapter.Paragraphs) > 0 {
			chapters = append(chapters, chapter)
		}
	}

	return chapters, nil
}

func isMetadataFile(name string) bool {
	// Only skip files that don't have readable content
	metadataFiles := []string{
		"colophon.xhtml",     // Publishing details
		"imprint.xhtml",      // Publisher info
		"titlepage.xhtml",    // Title page image
		"toc.xhtml",          // Table of contents
		"halftitlepage.xhtml", // Half title page
		"loi.xhtml",          // List of illustrations
		"uncopyright.xhtml",  // Copyright info
	}
	for _, f := range metadataFiles {
		if name == f {
			return true
		}
	}
	return false
}

func parseChapterFile(path string) (Chapter, error) {
	file, err := os.Open(path)
	if err != nil {
		return Chapter{}, fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = file.Close() }()

	doc, err := html.Parse(file)
	if err != nil {
		return Chapter{}, fmt.Errorf("parse HTML: %w", err)
	}

	chapter := Chapter{
		Filename: filepath.Base(path),
	}

	// Extract title from h1 or h2 (fallback if toc doesn't have it)
	chapter.Title = cleanText(extractTitle(doc))
	if chapter.Title == "" {
		// Use filename as fallback
		chapter.Title = strings.TrimSuffix(filepath.Base(path), ".xhtml")
	}

	// Extract paragraphs
	chapter.Paragraphs = extractParagraphs(doc)

	return chapter, nil
}

func extractTitle(n *html.Node) string {
	// First try to find <title> element in head (cleanest source)
	if title := extractTitleElement(n); title != "" {
		return title
	}
	// Fall back to h1 or h2
	return extractHeading(n)
}

func extractTitleElement(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "title" {
		return getTextContent(n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if title := extractTitleElement(c); title != "" {
			return title
		}
	}
	return ""
}

func extractHeading(n *html.Node) string {
	if n.Type == html.ElementNode && (n.Data == "h1" || n.Data == "h2") {
		return getTextContent(n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if title := extractHeading(c); title != "" {
			return title
		}
	}
	return ""
}

func extractParagraphs(n *html.Node) []Paragraph {
	var paragraphs []Paragraph
	extractParagraphsRecursive(n, &paragraphs, false)
	return paragraphs
}

func extractParagraphsRecursive(n *html.Node, paragraphs *[]Paragraph, inBlockquote bool) {
	// Track if we're entering a blockquote
	if n.Type == html.ElementNode && n.Data == "blockquote" {
		inBlockquote = true
	}

	if n.Type == html.ElementNode && n.Data == "p" {
		text := cleanText(getTextContent(n))
		if text != "" {
			sentences := splitSentences(text)
			*paragraphs = append(*paragraphs, Paragraph{
				Text:         text,
				Sentences:    sentences,
				InBlockquote: inBlockquote,
			})
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractParagraphsRecursive(c, paragraphs, inBlockquote)
	}
}

// splitSentences splits text into sentences based on punctuation
func splitSentences(text string) []string {
	// Pattern to split on sentence-ending punctuation followed by space or end
	// Handles: . ! ? and also handles abbreviations like Mr. Mrs. Dr. etc.
	var sentences []string

	parts := sentencePattern.Split(text, -1)
	delimiters := sentencePattern.FindAllString(text, -1)

	var current strings.Builder
	for i, part := range parts {
		current.WriteString(part)
		if i < len(delimiters) {
			current.WriteString(strings.TrimSpace(delimiters[i]))
			// Check if this looks like end of sentence (not abbreviation)
			sentence := strings.TrimSpace(current.String())
			// Also check if next part starts with lowercase (continuation, not new sentence)
			nextStartsLower := false
			if i+1 < len(parts) && len(parts[i+1]) > 0 {
				firstChar := rune(parts[i+1][0])
				// If next part starts with lowercase letter, it's likely a continuation
				// Exception: don't apply this check if it looks like a quote continuation
				nextStartsLower = firstChar >= 'a' && firstChar <= 'z'
			}
			if sentence != "" && !isAbbreviation(sentence) && !nextStartsLower {
				sentences = append(sentences, sentence)
				current.Reset()
			} else {
				current.WriteString(" ")
			}
		}
	}

	// Add any remaining text
	remaining := strings.TrimSpace(current.String())
	if remaining != "" {
		sentences = append(sentences, remaining)
	}

	// If no sentences were split, return the whole text as one sentence
	if len(sentences) == 0 && text != "" {
		sentences = append(sentences, text)
	}

	return sentences
}

// isAbbreviation checks if the text ends with a common abbreviation
func isAbbreviation(text string) bool {
	abbreviations := []string{
		// Titles
		"Mr.", "Mrs.", "Ms.", "Dr.", "Prof.", "Rev.", "Fr.",
		"Jr.", "Sr.", "St.", "Mt.", "Ft.",
		// Latin abbreviations
		"vs.", "etc.", "i.e.", "e.g.", "cf.",
		// Publishing
		"Vol.", "No.", "Ed.", "Trans.",
		// Months
		"Jan.", "Feb.", "Mar.", "Apr.", "Jun.", "Jul.", "Aug.", "Sep.", "Sept.", "Oct.", "Nov.", "Dec.",
	}
	for _, abbr := range abbreviations {
		if strings.HasSuffix(text, abbr) {
			return true
		}
	}

	// Check for single uppercase letter abbreviations (initials like "M.", "J.", "A.")
	// Common in classic literature for names like "M. Waldman", "J. Smith"
	if len(text) >= 2 {
		lastTwo := text[len(text)-2:]
		if len(lastTwo) == 2 && lastTwo[1] == '.' && lastTwo[0] >= 'A' && lastTwo[0] <= 'Z' {
			// Check if preceded by space or start of text (to avoid matching "AM.")
			if len(text) == 2 || text[len(text)-3] == ' ' {
				return true
			}
		}
	}

	return false
}

func getTextContent(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var result strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		result.WriteString(getTextContent(c))
	}
	return result.String()
}

func cleanText(s string) string {
	// Normalize whitespace
	s = whitespacePattern.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}
