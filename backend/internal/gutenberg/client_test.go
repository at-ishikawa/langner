package gutenberg

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// fakeClient returns canned responses keyed by requested URL, so the feed and
// header mapping can be exercised without the real gutenberg.org network.
type fakeClient struct {
	responses map[string]string
}

func (f fakeClient) Do(req *http.Request) (*http.Response, error) {
	body, ok := f.responses[req.URL.String()]
	if !ok {
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func TestSearchMapping(t *testing.T) {
	client := fakeClient{responses: map[string]string{
		// url.Values.Encode sorts keys: page, search.
		"https://gutendex.com/books/?page=1&search=austen": readFixture(t, "search.json"),
	}}

	res, err := Search(context.Background(), client, "austen", 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Books) != 2 {
		t.Fatalf("got %d books, want 2: %+v", len(res.Books), res.Books)
	}
	b := res.Books[0]
	if b.SourceID != "1342" || b.Title != "Pride and Prejudice" || b.Author != "Austen, Jane" || b.Language != "en" {
		t.Errorf("unexpected book mapping: %+v", b)
	}
	if b.TextURL != "https://www.gutenberg.org/cache/epub/1342/pg1342.txt" {
		t.Errorf("TextURL = %q, want the cache plain-text URL", b.TextURL)
	}
	if b.CoverURL != "https://gutenberg.pglaf.org/cache/epub/1342/pg1342.cover.medium.jpg" {
		t.Errorf("CoverURL = %q, want the image/jpeg format on the mirror", b.CoverURL)
	}
	// "next" set and "previous" null on page 1 → hasNext true, hasPrev false.
	if !res.HasNext || res.HasPrev || res.Page != 1 {
		t.Errorf("pagination = (next %v, prev %v, page %d), want (true, false, 1)", res.HasNext, res.HasPrev, res.Page)
	}
}

// TestBrowseNoQuery: an empty query browses the catalog — the request carries
// the page but NO search param.
func TestBrowseNoQuery(t *testing.T) {
	page := `{"count": 1, "next": null, "previous": "https://gutendex.com/books/",
"results": [{"id": 84, "title": "Frankenstein", "authors": [{"name": "Shelley, Mary Wollstonecraft"}], "languages": ["en"], "formats": {}}]}`
	client := fakeClient{responses: map[string]string{
		"https://gutendex.com/books/?page=2": page,
	}}

	res, err := Search(context.Background(), client, "", 2)
	if err != nil {
		t.Fatalf("Search(browse): %v", err)
	}
	if len(res.Books) != 1 || res.Books[0].SourceID != "84" || res.Books[0].Author != "Shelley, Mary Wollstonecraft" {
		t.Fatalf("unexpected browse results: %+v", res.Books)
	}
	if res.HasNext || !res.HasPrev || res.Page != 2 {
		t.Errorf("pagination = (next %v, prev %v, page %d), want (false, true, 2)", res.HasNext, res.HasPrev, res.Page)
	}
}

func TestSearchRejectsMalformedResponse(t *testing.T) {
	client := fakeClient{responses: map[string]string{
		"https://gutendex.com/books/?page=1&search=x": "{",
	}}
	if _, err := Search(context.Background(), client, "x", 1); err == nil {
		t.Fatal("Search with a malformed response: want error, got nil")
	}
}

// TestFetch: one request to the mirror's plain-text cache; title/author come
// from the text's own Project Gutenberg header (CRLF line endings, BOM, as
// served).
func TestFetch(t *testing.T) {
	text := readFixture(t, "pg11.txt")
	client := fakeClient{responses: map[string]string{
		"https://gutenberg.pglaf.org/cache/epub/11/pg11.txt": text,
	}}

	title, author, raw, err := Fetch(context.Background(), client, "11")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if title != "Alice's Adventures in Wonderland" || author != "Lewis Carroll" {
		t.Errorf("Fetch metadata = (%q, %q), want (Alice's Adventures in Wonderland, Lewis Carroll)", title, author)
	}
	if raw != text {
		t.Errorf("Fetch raw differs from fixture text")
	}
	// The fetched text feeds Parse directly.
	if _, err := Parse(raw, title, author); err != nil {
		t.Errorf("Parse(fetched text): %v", err)
	}
}

func TestFetchUnknownBook(t *testing.T) {
	if _, _, _, err := Fetch(context.Background(), fakeClient{}, "999999"); err == nil {
		t.Fatal("Fetch of an unknown id: want error, got nil")
	}
}

func TestParseHeader(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantTitle  string
		wantAuthor string
	}{
		{
			name:       "wrapped title",
			raw:        "Title: Frankenstein; Or, The Modern\n       Prometheus\n\nAuthor: Mary Wollstonecraft Shelley\n\n*** START OF THE PROJECT GUTENBERG EBOOK FRANKENSTEIN ***\n",
			wantTitle:  "Frankenstein; Or, The Modern Prometheus",
			wantAuthor: "Mary Wollstonecraft Shelley",
		},
		{
			name:      "no author line",
			raw:       "Title: Beowulf\n\n*** START OF THE PROJECT GUTENBERG EBOOK BEOWULF ***\n",
			wantTitle: "Beowulf",
		},
		{
			// A "Title:" in the body (after the START marker) is not metadata.
			name: "ignores body lines",
			raw:  "*** START OF THE PROJECT GUTENBERG EBOOK X ***\nTitle: Not Metadata\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, author := parseHeader(tt.raw)
			if title != tt.wantTitle || author != tt.wantAuthor {
				t.Errorf("parseHeader = (%q, %q), want (%q, %q)", title, author, tt.wantTitle, tt.wantAuthor)
			}
		})
	}
}

// The mirror doesn't have book 5 here, so Fetch falls back to
// www.gutenberg.org; that text has no Title: line, so the id names the book.
func TestFetchFallsBackToIDTitle(t *testing.T) {
	client := fakeClient{responses: map[string]string{
		"https://www.gutenberg.org/cache/epub/5/pg5.txt": "*** START OF THE PROJECT GUTENBERG EBOOK ***\nBody.\n",
	}}
	title, _, _, err := Fetch(context.Background(), client, "5")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if title != "Project Gutenberg eBook #5" {
		t.Errorf("title = %q, want the id fallback", title)
	}
}

func TestFetchFailsWhenNoSourceHasTheText(t *testing.T) {
	if _, _, _, err := Fetch(context.Background(), fakeClient{}, "11"); err == nil {
		t.Fatal("Fetch with no source serving the text: want error, got nil")
	}
}
