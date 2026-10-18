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
	json := `{"next":"https://gutendex.com/books?page=2&search=tale","previous":null,"results":[
		{"id":42,"title":"A Tale","authors":[{"name":"Jane Placeholder"}],"languages":["en"],
		 "formats":{"text/html":"http://x/h","text/plain; charset=utf-8":"http://x/utf8","text/plain":"http://x/plain","image/jpeg":"http://x/cover.jpg"}}
	]}`
	client := fakeClient{responses: map[string]string{
		// url.Values.Encode sorts keys: page then search.
		"https://gutendex.com/books?page=1&search=tale": json,
	}}

	res, err := Search(context.Background(), client, "tale", 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Books) != 1 {
		t.Fatalf("got %d books, want 1", len(res.Books))
	}
	b := res.Books[0]
	if b.SourceID != "42" || b.Title != "A Tale" || b.Author != "Jane Placeholder" || b.Language != "en" {
		t.Errorf("unexpected book mapping: %+v", b)
	}
	// Prefer the UTF-8 plain-text format; carry the cover thumbnail.
	if b.TextURL != "http://x/utf8" {
		t.Errorf("TextURL = %q, want UTF-8 variant", b.TextURL)
	}
	if b.CoverURL != "http://x/cover.jpg" {
		t.Errorf("CoverURL = %q, want the image/jpeg format", b.CoverURL)
	}
	// next set, previous null → hasNext true, hasPrev false.
	if !res.HasNext || res.HasPrev {
		t.Errorf("pagination = (next %v, prev %v), want (true, false)", res.HasNext, res.HasPrev)
	}
}

// TestBrowseNoQuery: an empty query browses the catalog — the request carries a
// page but NO search param.
func TestBrowseNoQuery(t *testing.T) {
	json := `{"next":null,"previous":"https://gutendex.com/books?page=1","results":[
		{"id":1,"title":"Popular","authors":[],"languages":["en"],"formats":{"text/plain; charset=utf-8":"http://x/1"}}
	]}`
	client := fakeClient{responses: map[string]string{
		"https://gutendex.com/books?page=2": json,
	}}

	res, err := Search(context.Background(), client, "", 2)
	if err != nil {
		t.Fatalf("Search(browse): %v", err)
	}
	if len(res.Books) != 1 || res.Books[0].Title != "Popular" {
		t.Fatalf("unexpected browse results: %+v", res.Books)
	}
	if res.HasNext || !res.HasPrev || res.Page != 2 {
		t.Errorf("pagination = (next %v, prev %v, page %d), want (false, true, 2)", res.HasNext, res.HasPrev, res.Page)
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
