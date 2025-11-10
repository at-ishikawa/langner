package pdf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mandolyte/mdtopdf"
)

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

	pdfPath := strings.TrimSuffix(markdownPath, ".md") + ".pdf"

	renderer := mdtopdf.NewPdfRenderer("P", "A4", pdfPath, "", nil, mdtopdf.LIGHT)
	if err := renderer.Process(content); err != nil {
		return "", fmt.Errorf("renderer.Process() > %w", err)
	}

	absPath, err := filepath.Abs(pdfPath)
	if err != nil {
		return pdfPath, nil
	}

	return absPath, nil
}
