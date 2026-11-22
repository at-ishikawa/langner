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
		// url.Values.Encode sorts keys: query, sort_order, start_index.
		"https://www.gutenberg.org/ebooks/search.opds/?query=alice&sort_order=downloads&start_index=1": readFixture(t, "search.opds"),
	}}

	res, err := Search(context.Background(), client, "alice", 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// The "Sort Alphabetically" navigation entry is not a book.
	if len(res.Books) != 2 {
		t.Fatalf("got %d books, want 2: %+v", len(res.Books), res.Books)
	}
	b := res.Books[0]
	if b.SourceID != "11" || b.Title != "Alice's Adventures in Wonderland" || b.Author != "Lewis Carroll" {
		t.Errorf("unexpected book mapping: %+v", b)
	}
	if b.TextURL != "https://www.gutenberg.org/cache/epub/11/pg11.txt" {
		t.Errorf("TextURL = %q, want the cache plain-text URL", b.TextURL)
	}
	if b.CoverURL != "https://www.gutenberg.org/cache/epub/11/pg11.cover.small.jpg" {
		t.Errorf("CoverURL = %q, want the thumbnail link", b.CoverURL)
	}
	// A feed-relative thumbnail href is made absolute.
	if got := res.Books[1].CoverURL; got != "https://www.gutenberg.org/cache/epub/12/pg12.cover.small.jpg" {
		t.Errorf("relative CoverURL = %q, want absolute", got)
	}
	// rel="next" link present on page 1 → hasNext true, hasPrev false.
	if !res.HasNext || res.HasPrev || res.Page != 1 {
		t.Errorf("pagination = (next %v, prev %v, page %d), want (true, false, 1)", res.HasNext, res.HasPrev, res.Page)
	}
}

// TestBrowseNoQuery: an empty query browses the catalog — the request carries
// the page's start_index but NO query param.
func TestBrowseNoQuery(t *testing.T) {
	feed := `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
<link rel="self" type="application/atom+xml;profile=opds-catalog" href="/ebooks/search.opds/?sort_order=downloads&amp;start_index=26"/>
<entry>
<title>Pride and Prejudice</title>
<content type="text">Jane Austen</content>
<id>http://www.gutenberg.org/ebooks/1342.opds</id>
</entry>
</feed>`
	client := fakeClient{responses: map[string]string{
		"https://www.gutenberg.org/ebooks/search.opds/?sort_order=downloads&start_index=26": feed,
	}}

	res, err := Search(context.Background(), client, "", 2)
	if err != nil {
		t.Fatalf("Search(browse): %v", err)
	}
	if len(res.Books) != 1 || res.Books[0].SourceID != "1342" || res.Books[0].Author != "Jane Austen" {
		t.Fatalf("unexpected browse results: %+v", res.Books)
	}
	if res.HasNext || !res.HasPrev || res.Page != 2 {
		t.Errorf("pagination = (next %v, prev %v, page %d), want (false, true, 2)", res.HasNext, res.HasPrev, res.Page)
	}
}

func TestSearchRejectsMalformedFeed(t *testing.T) {
	client := fakeClient{responses: map[string]string{
		"https://www.gutenberg.org/ebooks/search.opds/?query=x&sort_order=downloads&start_index=1": "<feed><entry>",
	}}
	if _, err := Search(context.Background(), client, "x", 1); err == nil {
		t.Fatal("Search with a malformed feed: want error, got nil")
	}
}

// TestFetch: one request to the plain-text cache; title/author come from the
// text's own Project Gutenberg header (CRLF line endings, BOM, as served).
func TestFetch(t *testing.T) {
	text := readFixture(t, "pg11.txt")
	client := fakeClient{responses: map[string]string{
		"https://www.gutenberg.org/cache/epub/11/pg11.txt": text,
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
