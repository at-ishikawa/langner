package gutenberg

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// fakeClient returns canned responses keyed by requested URL, so the JSON
// mapping can be exercised without the real Gutendex network.
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

func TestSearchMapping(t *testing.T) {
	json := `{"results":[
		{"id":42,"title":"A Tale","authors":[{"name":"Jane Placeholder"}],"languages":["en"],
		 "formats":{"text/html":"http://x/h","text/plain; charset=utf-8":"http://x/utf8","text/plain":"http://x/plain"}}
	]}`
	client := fakeClient{responses: map[string]string{
		"https://gutendex.com/books?search=tale": json,
	}}

	books, err := Search(context.Background(), client, "tale")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(books) != 1 {
		t.Fatalf("got %d books, want 1", len(books))
	}
	b := books[0]
	if b.SourceID != "42" || b.Title != "A Tale" || b.Author != "Jane Placeholder" || b.Language != "en" {
		t.Errorf("unexpected book mapping: %+v", b)
	}
	// Prefer the UTF-8 plain-text format.
	if b.TextURL != "http://x/utf8" {
		t.Errorf("TextURL = %q, want UTF-8 variant", b.TextURL)
	}
}

func TestPickTextURLFallback(t *testing.T) {
	// No UTF-8 variant → fall back to any text/plain format.
	got := pickTextURL(gutendexFormats{
		"text/html":                    "http://x/h",
		"text/plain; charset=us-ascii": "http://x/ascii",
	})
	if got != "http://x/ascii" {
		t.Errorf("pickTextURL fallback = %q, want ascii variant", got)
	}

	if got := pickTextURL(gutendexFormats{"application/epub+zip": "http://x/e"}); got != "" {
		t.Errorf("pickTextURL with no plain text = %q, want empty", got)
	}
}

func TestFetch(t *testing.T) {
	meta := `{"id":7,"title":"Flat Book","authors":[{"name":"A. Author"}],"languages":["en"],
		"formats":{"text/plain; charset=utf-8":"http://files/7.txt"}}`
	text := "*** START OF THE PROJECT GUTENBERG EBOOK FLAT ***\n\nBody.\n\n*** END OF THE PROJECT GUTENBERG EBOOK FLAT ***\n"
	client := fakeClient{responses: map[string]string{
		"https://gutendex.com/books/7": meta,
		"http://files/7.txt":           text,
	}}

	title, author, raw, err := Fetch(context.Background(), client, "7")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if title != "Flat Book" || author != "A. Author" {
		t.Errorf("Fetch metadata = (%q, %q), want (Flat Book, A. Author)", title, author)
	}
	if raw != text {
		t.Errorf("Fetch raw = %q, want fixture text", raw)
	}
}
