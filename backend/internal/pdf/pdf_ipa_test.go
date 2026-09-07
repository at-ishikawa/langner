package pdf

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConvertMarkdownToPDF_RendersUnicodeIPA drives the real render path with
// markdown containing IPA (stress marks + IPA letters), a heading, bold, and a
// code block, and asserts:
//   - it renders without error,
//   - the embedded DejaVu Unicode font is present in the output (so IPA renders
//     as glyphs, not boxes),
//   - the swap of every styler to the Unicode font didn't break heading / bold
//     / code rendering (they all produce a non-trivial PDF).
//
// A common word is used (never the user's real vocabulary). Visual confirmation
// of the glyphs is left to a human opening the PDF; this pins the mechanism.
func TestConvertMarkdownToPDF_RendersUnicodeIPA(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "ipa.md")

	// IPA stress mark U+02C8 (ˈ) + IPA letters (ɜ ə) — none of these are in
	// Latin-1, so a core font would render boxes. Also exercise a heading,
	// bold, inline code, and a fenced code block (all stylers were repointed).
	md := "# Pronunciation\n\n" +
		"**word** /ˈwɜːrd/: a unit of language.\n\n" +
		"Some **bold** text and `inline code`.\n\n" +
		"```\ncode block\n```\n"
	if err := os.WriteFile(mdPath, []byte(md), 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}

	pdfPath, err := ConvertMarkdownToPDF(mdPath)
	if err != nil {
		t.Fatalf("ConvertMarkdownToPDF returned error: %v", err)
	}

	out, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("read produced pdf: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatalf("output is not a PDF (prefix %q)", string(out[:min(8, len(out))]))
	}
	// A PDF with an embedded TTF subset is far larger than a core-font PDF.
	if len(out) < 20_000 {
		t.Fatalf("PDF unexpectedly small (%d bytes) — font likely not embedded", len(out))
	}
	// fpdf embeds the TrueType font; its name appears in the font descriptor.
	if !strings.Contains(strings.ToLower(string(out)), "dejavu") {
		t.Fatalf("embedded DejaVu font not found in PDF output (IPA would render as boxes)")
	}
}
