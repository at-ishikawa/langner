package clicmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// tgz builds an in-memory gzipped tar from name→content entries (a name ending
// in "/" is a directory).
func tgz(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range entries {
		if len(name) > 0 && name[len(name)-1] == '/' {
			if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractTarGz_WritesTree(t *testing.T) {
	dest := t.TempDir()
	data := tgz(t, map[string]string{
		"./":                   "",
		"stories/":             "",
		"stories/index.yml":    "kind: story\n",
		"flashcards/index.yml": "kind: flashcard\n",
	})
	if err := extractTarGz(bytes.NewReader(data), dest); err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "stories", "index.yml"))
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(got) != "kind: story\n" {
		t.Errorf("content = %q, want %q", got, "kind: story\n")
	}
	if _, err := os.Stat(filepath.Join(dest, "flashcards", "index.yml")); err != nil {
		t.Errorf("flashcards/index.yml not extracted: %v", err)
	}
}

func TestExtractTarGz_RejectsTraversal(t *testing.T) {
	dest := t.TempDir()
	for _, bad := range []string{"../escape.txt", "/etc/passwd", "a/../../escape.txt"} {
		data := tgz(t, map[string]string{bad: "x"})
		if err := extractTarGz(bytes.NewReader(data), dest); err == nil {
			t.Errorf("expected rejection for %q, got nil", bad)
		}
	}
}
