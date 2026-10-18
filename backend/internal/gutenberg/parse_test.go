package gutenberg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/at-ishikawa/langner/internal/ebook"
	"gopkg.in/yaml.v3"
)

// findFile returns the content of the bundle file at path, or fails the test.
func findFile(t *testing.T, b Bundle, path string) []byte {
	t.Helper()
	for _, f := range b.Files {
		if f.Path == path {
			return f.Content
		}
	}
	t.Fatalf("bundle missing file %q; have %v", path, filePaths(b))
	return nil
}

func filePaths(b Bundle) []string {
	paths := make([]string, len(b.Files))
	for i, f := range b.Files {
		paths[i] = f.Path
	}
	return paths
}

func TestParse(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "sample.txt"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	tests := []struct {
		name          string
		raw           string
		title         string
		author        string
		wantName      string
		wantChapters  int
		wantEvents    []string
		wantPaths     []string
		wantFirstStmt string
	}{
		{
			name:          "two chapters from PG fixture",
			raw:           string(raw),
			title:         "A Small Invented Tale",
			author:        "Jane Placeholder",
			wantName:      "A Small Invented Tale by Jane Placeholder",
			wantChapters:  2,
			wantEvents:    []string{"CHAPTER I", "CHAPTER II"},
			wantPaths:     []string{"001-chapter-i.yml", "002-chapter-ii.yml"},
			wantFirstStmt: "The morning was bright and the river ran slow.",
		},
		{
			name: "no headings yields single chapter",
			raw: "*** START OF THE PROJECT GUTENBERG EBOOK PLAIN ***\n\n" +
				"Just one flat paragraph. It has two sentences.\n\n" +
				"*** END OF THE PROJECT GUTENBERG EBOOK PLAIN ***\n",
			title:         "Plain",
			author:        "",
			wantName:      "Plain",
			wantChapters:  1,
			wantEvents:    []string{"Plain"},
			wantPaths:     []string{"001-plain.yml"},
			wantFirstStmt: "Just one flat paragraph.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bundle, err := Parse(tt.raw, tt.title, tt.author)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			// Boilerplate must be gone from every emitted file.
			for _, f := range bundle.Files {
				if strings.Contains(strings.ToUpper(string(f.Content)), "PROJECT GUTENBERG") {
					t.Errorf("file %q still contains Project Gutenberg boilerplate:\n%s", f.Path, f.Content)
				}
			}

			// index.yml round-trips into the reader's Index shape.
			indexData := findFile(t, bundle, "index.yml")
			var index ebook.NotebookIndex
			if err := yaml.Unmarshal(indexData, &index); err != nil {
				t.Fatalf("unmarshal index.yml: %v", err)
			}
			if index.Name != tt.wantName {
				t.Errorf("index Name = %q, want %q", index.Name, tt.wantName)
			}
			if len(index.NotebookPaths) != tt.wantChapters {
				t.Fatalf("index lists %d notebooks, want %d: %v", len(index.NotebookPaths), tt.wantChapters, index.NotebookPaths)
			}
			for i, p := range tt.wantPaths {
				if index.NotebookPaths[i] != p {
					t.Errorf("notebook path[%d] = %q, want %q", i, index.NotebookPaths[i], p)
				}
			}

			// Each listed chapter file must exist, round-trip into the reader's
			// StoryNotebook shape, and carry the expected Event + statements.
			for i, p := range index.NotebookPaths {
				data := findFile(t, bundle, p)
				var entries []ebook.NotebookEntry
				if err := yaml.Unmarshal(data, &entries); err != nil {
					t.Fatalf("unmarshal %s: %v", p, err)
				}
				if len(entries) != 1 {
					t.Fatalf("%s: got %d entries, want 1", p, len(entries))
				}
				entry := entries[0]
				if entry.Event != tt.wantEvents[i] {
					t.Errorf("%s: Event = %q, want %q", p, entry.Event, tt.wantEvents[i])
				}
				if len(entry.Scenes) != 1 {
					t.Fatalf("%s: got %d scenes, want 1", p, len(entry.Scenes))
				}
				scene := entry.Scenes[0]
				if scene.Scene != tt.wantEvents[i] {
					t.Errorf("%s: scene.Scene = %q, want %q", p, scene.Scene, tt.wantEvents[i])
				}
				if len(scene.Statements) == 0 {
					t.Errorf("%s: scene has no statements", p)
				}
			}

			// Spot-check the first statement of the first chapter.
			firstData := findFile(t, bundle, tt.wantPaths[0])
			var firstEntries []ebook.NotebookEntry
			if err := yaml.Unmarshal(firstData, &firstEntries); err != nil {
				t.Fatalf("unmarshal first chapter: %v", err)
			}
			got := firstEntries[0].Scenes[0].Statements[0]
			if got != tt.wantFirstStmt {
				t.Errorf("first statement = %q, want %q", got, tt.wantFirstStmt)
			}
		})
	}
}

func TestParseRequiresTitle(t *testing.T) {
	if _, err := Parse("some text", "  ", ""); err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestSplitSentences(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"One. Two. Three.", []string{"One.", "Two.", "Three."}},
		{"Hello!  How are you?", []string{"Hello!", "How are you?"}},
		{"No terminator here", []string{"No terminator here"}},
		{"   ", nil},
	}
	for _, tt := range tests {
		got := splitSentences(tt.in)
		if len(got) != len(tt.want) {
			t.Fatalf("splitSentences(%q) = %v, want %v", tt.in, got, tt.want)
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitSentences(%q)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
			}
		}
	}
}

func TestIsChapterHeading(t *testing.T) {
	headings := []string{"CHAPTER I", "Chapter 12", "IV.", "XVII", "THE BEGINNING", "PART ONE"}
	for _, h := range headings {
		if !isChapterHeading(h) {
			t.Errorf("isChapterHeading(%q) = false, want true", h)
		}
	}
	nonHeadings := []string{"", "The morning was bright.", "a short line", "He said hello."}
	for _, h := range nonHeadings {
		if isChapterHeading(h) {
			t.Errorf("isChapterHeading(%q) = true, want false", h)
		}
	}
}
