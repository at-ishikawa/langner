package pdf

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mandolyte/mdtopdf"
)

// boldPattern matches **bold** text in markdown
var boldPattern = regexp.MustCompile(`\*\*([^*]+)\*\*`)

// ConvertMarkdownToPDF converts a markdown file to PDF using mdtopdf package
// The PDF file will be created in the same directory as the markdown file
func ConvertMarkdownToPDF(markdownPath string) (string, error) {
	if !strings.HasSuffix(markdownPath, ".md") {
		return "", fmt.Errorf("input file must have .md extension: %s", markdownPath)
	}

	content, err := os.ReadFile(markdownPath)
	if err != nil {
		return "", fmt.Errorf("os.ReadFile(%s) > %w", markdownPath, err)
	}

	// Preprocess: normalize smart punctuation to ASCII (default PDF fonts don't support Unicode)
	content = normalizeSmartPunctuation(content)

	// Preprocess: remove bold markers in blockquotes (mdtopdf doesn't handle them well)
	content = convertBoldToItalicInBlockquotes(content)

	// Preprocess: download images once and replace URLs with local paths
	// mdtopdf re-downloads the same URL every time it appears
	content, cleanupFn := preDownloadImages(content)
	defer cleanupFn()

	pdfPath := strings.TrimSuffix(markdownPath, ".md") + ".pdf"

	renderer := mdtopdf.NewPdfRenderer("P", "A4", pdfPath, "", nil, mdtopdf.LIGHT)
	renderer.UpdateBlockquoteStyler()
	if err := renderer.Process(content); err != nil {
		return "", fmt.Errorf("renderer.Process() > %w", err)
	}

	absPath, err := filepath.Abs(pdfPath)
	if err != nil {
		return pdfPath, nil
	}

	return absPath, nil
}

// normalizeSmartPunctuation replaces Unicode smart quotes/punctuation with ASCII equivalents.
// The default PDF fonts (Latin-1) don't support these characters, causing garbled output.
func normalizeSmartPunctuation(content []byte) []byte {
	replacer := strings.NewReplacer(
		"\u2018", "'", // LEFT SINGLE QUOTATION MARK
		"\u2019", "'", // RIGHT SINGLE QUOTATION MARK
		"\u201C", "\"", // LEFT DOUBLE QUOTATION MARK
		"\u201D", "\"", // RIGHT DOUBLE QUOTATION MARK
		"\u2013", "-", // EN DASH
		"\u2014", "--", // EM DASH
		"\u2026", "...", // HORIZONTAL ELLIPSIS
	)
	return []byte(replacer.Replace(string(content)))
}

// imageURLPattern matches markdown image URLs: ![alt](https://...)
var imageURLPattern = regexp.MustCompile(`!\[([^\]]*)\]\((https?://[^)]+)\)`)

// preDownloadImages finds all image URLs in the markdown, downloads each unique
// URL once to a temp directory, and replaces the URLs with local file paths.
// Returns the modified content and a cleanup function to remove the temp dir.
func preDownloadImages(content []byte) ([]byte, func()) {
	text := string(content)
	matches := imageURLPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return content, func() {}
	}

	tmpDir, err := os.MkdirTemp("", "langner-images-*")
	if err != nil {
		return content, func() {}
	}

	// Download each unique URL once
	downloaded := make(map[string]string) // URL -> local path
	for _, match := range matches {
		url := match[2]
		if _, ok := downloaded[url]; ok {
			continue
		}

		localPath := filepath.Join(tmpDir, filepath.Base(url))
		// Deduplicate filenames that differ only by URL path
		if _, exists := downloaded[url]; exists {
			continue
		}

		if err := downloadImage(url, localPath); err != nil {
			fmt.Printf("Warning: failed to download image %s: %v\n", url, err)
			continue
		}
		downloaded[url] = localPath
	}

	// Replace URLs with local paths
	for url, localPath := range downloaded {
		text = strings.ReplaceAll(text, url, localPath)
	}

	cleanup := func() {
		_ = os.RemoveAll(tmpDir)
	}

	return []byte(text), cleanup
}

// downloadImage downloads a URL to a local file path.
func downloadImage(url, destPath string) error {
	resp, err := http.Get(url) //nolint:gosec // URLs come from user's notebook data
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	_, err = io.Copy(f, resp.Body)
	return err
}

// convertBoldToItalicInBlockquotes removes **bold** markers in blockquote lines
// mdtopdf's blockquote multiCell doesn't handle inline bold properly
// Blockquotes are already rendered in italic, so the text remains styled
func convertBoldToItalicInBlockquotes(content []byte) []byte {
	lines := strings.Split(string(content), "\n")

	for i, line := range lines {
		if strings.HasPrefix(line, "> ") {
			// Remove **bold** markers - blockquote text is already italic
			lines[i] = boldPattern.ReplaceAllString(line, "$1")
		}
	}
	return []byte(strings.Join(lines, "\n"))
}
