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

	// defaultTimeout is the per-ATTEMPT HTTP budget; getJSON/download retry a few
	// times on a transient failure, so a single slow attempt no longer sinks the
	// whole request (the reported "Client.Timeout exceeded" on a slow search).
	defaultTimeout = 20 * time.Second

	// gutendex can be slow or briefly throttle a bare Go user-agent; retry a
	// couple of times with backoff before giving up.
	maxAttempts = 3

	// userAgent identifies the app politely — a named UA is less likely to be
	// delayed/throttled than net/http's default "Go-http-client/1.1".
	userAgent = "langner/1.0 (+https://github.com/at-ishikawa/langner)"
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
	Results  []gutendexBook `json:"results"`
	Next     *string        `json:"next"`
	Previous *string        `json:"previous"`
}

// SearchResult is one page of catalog hits plus whether adjacent pages exist,
// so the UI can page the catalog (with or without a query).
type SearchResult struct {
	Books   []Book
	HasNext bool
	HasPrev bool
	Page    int
}

// Search returns one page of the Gutenberg catalog. An empty query BROWSES the
// popular catalog (Gutendex's default ordering, fast + cached); a non-empty
// query runs a free-text search. page is 1-based (<=0 means page 1). httpClient
// may be nil (a default client with a per-attempt timeout is used).
func Search(ctx context.Context, httpClient HTTPClient, query string, page int) (SearchResult, error) {
	if page <= 0 {
		page = 1
	}
	params := url.Values{}
	params.Set("page", fmt.Sprintf("%d", page))
	if q := query; q != "" {
		params.Set("search", q)
	}
	u := gutendexBaseURL + "?" + params.Encode()

	var resp gutendexSearchResponse
	if err := getJSON(ctx, resolveClient(httpClient), u, &resp); err != nil {
		return SearchResult{}, fmt.Errorf("gutendex search: %w", err)
	}

	books := make([]Book, 0, len(resp.Results))
	for _, r := range resp.Results {
		books = append(books, toBook(r))
	}
	return SearchResult{
		Books:   books,
		HasNext: resp.Next != nil && *resp.Next != "",
		HasPrev: resp.Previous != nil && *resp.Previous != "",
		Page:    page,
	}, nil
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
		CoverURL: g.Formats["image/jpeg"],
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
	body, err := fetchBytes(ctx, client, u)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}
	return nil
}

func download(ctx context.Context, client HTTPClient, u string) (string, error) {
	body, err := fetchBytes(ctx, client, u)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// fetchBytes GETs u and returns the (size-bounded) body, retrying transient
// failures (network/timeout, 5xx, 429) up to maxAttempts with linear backoff.
// Each attempt gets its own timeout so one slow response is cut and retried
// rather than hanging the whole call. A named User-Agent avoids the throttling
// a bare Go UA can hit. The whole body is read inside the attempt's context so
// a per-attempt deadline never truncates a mid-stream read.
func fetchBytes(ctx context.Context, client HTTPClient, u string) ([]byte, error) {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		body, retryable, err := fetchOnce(ctx, client, u)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
		if attempt < maxAttempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
			}
		}
	}
	return nil, lastErr
}

func fetchOnce(ctx context.Context, client HTTPClient, u string) (body []byte, retryable bool, err error) {
	attemptCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, u, nil)
	if err != nil {
		return nil, false, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, true, err // network / timeout — worth a retry
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		// 5xx / 429 are transient; 4xx (e.g. 404 for an unknown id) are not.
		return nil, resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests,
			fmt.Errorf("unexpected status %d for %s", resp.StatusCode, u)
	}
	body, err = io.ReadAll(io.LimitReader(resp.Body, maxTextBytes))
	if err != nil {
		return nil, true, fmt.Errorf("read body: %w", err)
	}
	return body, false, nil
}
