package pdf

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pdf "github.com/stephenafamo/goldmark-pdf"
	"github.com/yuin/goldmark"
)

// Fonts for the exported PDF. Charis SIL is an SIL typeface with full IPA
// coverage (ˈ ˌ ɛ ɪ ŋ ð ʃ …), so pronunciation fields and other Unicode render
// as real glyphs instead of boxes — the point of moving off the old Latin-1
// core-font renderer. goldmark-pdf fetches these from Google Fonts at render
// time and embeds a subset in the PDF; no font files are vendored.
const (
	bodyFontFamily = "Charis SIL"
	codeFontFamily = "Roboto Mono"
)

// ConvertMarkdownToPDF renders a markdown file to a PDF next to it (same path,
// .md -> .pdf) with goldmark + goldmark-pdf, returning the absolute PDF path.
//
// Unicode (IPA, smart punctuation, em-dashes) renders correctly because the
// body font is a UTF-8 font with IPA coverage. Remote images in the markdown
// are fetched by goldmark-pdf's built-in web filesystem, so no pre-download
// step is needed.
func ConvertMarkdownToPDF(markdownPath string) (string, error) {
	if !strings.HasSuffix(markdownPath, ".md") {
		return "", fmt.Errorf("input file must have .md extension: %s", markdownPath)
	}

	content, err := os.ReadFile(markdownPath)
	if err != nil {
		return "", fmt.Errorf("os.ReadFile(%s) > %w", markdownPath, err)
	}

	pdfPath := strings.TrimSuffix(markdownPath, ".md") + ".pdf"

	renderer := pdf.New(
		pdf.WithContext(context.Background()),
		// This is a PDF, not HTML: never turn `"` `<` `>` `&` into HTML entities
		// (goldmark-pdf otherwise renders a literal `&quot;` etc.).
		pdf.WithEscapeHTML(false),
		pdf.WithHeadingFont(pdf.GetTextFont(bodyFontFamily, pdf.FontCharisSIL)),
		pdf.WithBodyFont(pdf.GetTextFont(bodyFontFamily, pdf.FontCharisSIL)),
		pdf.WithCodeFont(pdf.GetCodeFont(codeFontFamily, pdf.FontRobotoMono)),
	)

	var buf bytes.Buffer
	md := goldmark.New(goldmark.WithRenderer(renderer))
	if err := md.Convert(content, &buf); err != nil {
		return "", fmt.Errorf("render markdown to pdf: %w", err)
	}

	// goldmark-pdf renders into a buffer, so we own the final write. os.WriteFile
	// is the write path that works on the Google Drive Stream / WSL mount (the
	// previous renderer opened the destination file directly, which failed there).
	if err := os.WriteFile(pdfPath, buf.Bytes(), 0o644); err != nil {
		return "", fmt.Errorf("write pdf to %s: %w", pdfPath, err)
	}

	absPath, err := filepath.Abs(pdfPath)
	if err != nil {
		return pdfPath, nil
	}
	return absPath, nil
}
