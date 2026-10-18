package gutenberg

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Gutendex (https://gutendex.com) is a JSON API over the Project Gutenberg
// catalog. These calls run only in prod/preview (no network in CI/containers);
// they are structured around an injectable HTTPClient so the JSON mapping is
// testable without the real network.
const (
	gutendexBaseURL = "https://gutendex.com/books"

	// maxTextBytes bounds the downloaded plain-text body so a hostile or
	// mislabeled URL can't stream an unbounded amount into memory.
	maxTextBytes = 8 << 20 // 8 MiB

	defaultTimeout = 30 * time.Second
)

// HTTPClient is the subset of *http.Client used here, so tests can inject a
// fake that returns canned responses.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// defaultHTTPClient returns a client with a sane timeout when the caller
// passes nil.
func resolveClient(c HTTPClient) HTTPClient {
	if c != nil {
		return c
	}
	return &http.Client{Timeout: defaultTimeout}
}

// gutendexFormats maps MIME type -> download URL for one catalog entry.
type gutendexFormats map[string]string

// gutendexBook mirrors the fields we consume from a Gutendex book object.
type gutendexBook struct {
	ID        int              `json:"id"`
	Title     string           `json:"title"`
	Authors   []gutendexAuthor `json:"authors"`
	Languages []string         `json:"languages"`
	Formats   gutendexFormats  `json:"formats"`
}

type gutendexAuthor struct {
	Name string `json:"name"`
}

type gutendexSearchResponse struct {
	Results []gutendexBook `json:"results"`
}

// Search queries Gutendex by free text and maps results to Book. httpClient
// may be nil (a default client with a timeout is used).
func Search(ctx context.Context, httpClient HTTPClient, query string) ([]Book, error) {
	u := fmt.Sprintf("%s?search=%s", gutendexBaseURL, url.QueryEscape(query))

	var resp gutendexSearchResponse
	if err := getJSON(ctx, resolveClient(httpClient), u, &resp); err != nil {
		return nil, fmt.Errorf("gutendex search: %w", err)
	}

	books := make([]Book, 0, len(resp.Results))
	for _, r := range resp.Results {
		books = append(books, toBook(r))
	}
	return books, nil
}

// Fetch resolves a book's metadata and plain-text URL via Gutendex, then
// downloads the plain text. The returned raw text is ready for Parse.
func Fetch(ctx context.Context, httpClient HTTPClient, sourceID string) (title, author, raw string, err error) {
	client := resolveClient(httpClient)

	u := fmt.Sprintf("%s/%s", gutendexBaseURL, url.PathEscape(sourceID))
	var meta gutendexBook
	if err := getJSON(ctx, client, u, &meta); err != nil {
		return "", "", "", fmt.Errorf("gutendex fetch metadata: %w", err)
	}

	book := toBook(meta)
	if book.TextURL == "" {
		return "", "", "", fmt.Errorf("no plain-text format for book %s", sourceID)
	}

	raw, err = download(ctx, client, book.TextURL)
	if err != nil {
		return "", "", "", fmt.Errorf("download text: %w", err)
	}
	return book.Title, book.Author, raw, nil
}

// toBook maps a Gutendex book object to our Book, picking the best plain-text
// format: prefer UTF-8, then any "text/plain" variant.
func toBook(g gutendexBook) Book {
	b := Book{
		SourceID: fmt.Sprintf("%d", g.ID),
		Title:    g.Title,
		TextURL:  pickTextURL(g.Formats),
	}
	if len(g.Authors) > 0 {
		b.Author = g.Authors[0].Name
	}
	if len(g.Languages) > 0 {
		b.Language = g.Languages[0]
	}
	return b
}

// pickTextURL selects a plain-text download URL, preferring the UTF-8 variant
// and falling back to any text/plain format.
func pickTextURL(formats gutendexFormats) string {
	if u, ok := formats["text/plain; charset=utf-8"]; ok {
		return u
	}
	for mime, u := range formats {
		// Match "text/plain", "text/plain; charset=us-ascii", etc.
		if len(mime) >= len("text/plain") && mime[:len("text/plain")] == "text/plain" {
			return u
		}
	}
	return ""
}

func getJSON(ctx context.Context, client HTTPClient, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d for %s", resp.StatusCode, u)
	}
	// Bound the metadata body too; Gutendex responses are small.
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxTextBytes)).Decode(out); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}
	return nil
}

func download(ctx context.Context, client HTTPClient, u string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d for %s", resp.StatusCode, u)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTextBytes))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	return string(body), nil
}
