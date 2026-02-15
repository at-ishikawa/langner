package pdf

import (
	"fmt"
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

	// Preprocess: remove bold markers in blockquotes (mdtopdf doesn't handle them well)
	content = convertBoldToItalicInBlockquotes(content)

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
