package pdf_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/at-ishikawa/langner/internal/pdf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConvertMarkdownToPDF_RendersUnicodeIPA drives the real render path with
// markdown containing IPA (stress marks + IPA letters) inside a bullet list —
// the exact shape (pronunciation in a list item) that broke on the old
// core-font renderer — and asserts the output is a valid, non-trivial PDF with
// the IPA-capable UTF-8 font embedded. A common word is used (never the user's
// real vocabulary). Visual confirmation of the glyphs is a human step; this
// pins that the Unicode font is actually embedded.
//
// It renders through goldmark-pdf, which fetches the font from Google Fonts, so
// it needs network; on a fetch failure (offline) it skips rather than failing.
func TestConvertMarkdownToPDF_RendersUnicodeIPA(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "ipa.md")

	// U+02C8 stress mark (ˈ) + IPA letters (ɜ ə) — none in Latin-1 — inside a
	// bullet list, plus a heading and inline code.
	md := "# Pronunciation\n\n" +
		"- **word** /ˈwɜːrd/: a unit of language.\n" +
		"- Some `inline code` and **bold**.\n"
	require.NoError(t, os.WriteFile(mdPath, []byte(md), 0o644))

	pdfPath, err := pdf.ConvertMarkdownToPDF(mdPath)
	if err != nil {
		// The renderer fetches the font over the network; don't fail the suite
		// when the environment has no egress to Google Fonts.
		t.Skipf("skipping: could not render (likely no network to Google Fonts): %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(pdfPath) })

	out, err := os.ReadFile(pdfPath)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(out, []byte("%PDF")), "output must be a PDF")
	// A PDF with an embedded TTF subset is far larger than a core-font PDF.
	assert.Greater(t, len(out), 20_000, "PDF should embed a font subset (IPA would render as boxes otherwise)")
	// The embedded UTF-8 font's name appears in the PDF font descriptor.
	assert.Contains(t, strings.ToLower(string(out)), "charis", "the IPA-capable UTF-8 font must be embedded")
}
