package gutenberg

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// These calls talk to Project Gutenberg (gutenberg.org) directly: its OPDS
// search feed for the catalog and its plain-text cache for a book. They run
// only in prod/preview (no network in CI/containers); they are structured
// around an injectable HTTPClient so the feed/header mapping is testable
// without the real network.
const (
	gutenbergBaseURL = "https://www.gutenberg.org"
	// mirrorBaseURL is an official Project Gutenberg mirror. Gutenberg asks
	// automated clients to download texts from mirrors, and the mirror stays up
	// while www.gutenberg.org is down for maintenance.
	mirrorBaseURL = "https://gutenberg.pglaf.org"

	// searchPageSize is how many books one OPDS search page holds; start_index
	// (1-based) pages through the results in steps of this size.
	searchPageSize = 25

	// maxTextBytes bounds the downloaded plain-text body so a hostile or
	// mislabeled URL can't stream an unbounded amount into memory.
	maxTextBytes = 8 << 20 // 8 MiB

	// defaultTimeout is the per-ATTEMPT HTTP budget; fetchBytes retries a few
	// times on a transient failure, so a single slow attempt no longer sinks the
	// whole request (the reported "Client.Timeout exceeded" on a slow search).
	defaultTimeout = 20 * time.Second

	// gutenberg.org can be slow or briefly throttle a bare Go user-agent; retry
	// a couple of times with backoff before giving up.
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

// opdsFeed mirrors the fields we consume from a Gutenberg OPDS (Atom) search
// feed. Besides one entry per book, the feed carries navigation entries (e.g.
// "Sort Alphabetically by Title"); those have no book id and are skipped.
type opdsFeed struct {
	Links   []opdsLink  `xml:"link"`
	Entries []opdsEntry `xml:"entry"`
}

type opdsEntry struct {
	ID      string     `xml:"id"`
	Title   string     `xml:"title"`
	Authors []string   `xml:"author>name"`
	Content string     `xml:"content"`
	Links   []opdsLink `xml:"link"`
}

type opdsLink struct {
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
	Href string `xml:"href,attr"`
}

// opdsBookID matches a book entry id such as
// "http://www.gutenberg.org/ebooks/11.opds" and captures the numeric id.
var opdsBookID = regexp.MustCompile(`/ebooks/(\d+)(?:\.opds)?$`)

// SearchResult is one page of catalog hits plus whether adjacent pages exist,
// so the UI can page the catalog (with or without a query).
type SearchResult struct {
	Books   []Book
	HasNext bool
	HasPrev bool
	Page    int
}

// Search returns one page of the Gutenberg catalog, most-downloaded first. An
// empty query BROWSES the popular catalog; a non-empty query runs a free-text
// search. page is 1-based (<=0 means page 1). httpClient may be nil (a default
// client with a per-attempt timeout is used).
func Search(ctx context.Context, httpClient HTTPClient, query string, page int) (SearchResult, error) {
	if page <= 0 {
		page = 1
	}
	params := url.Values{}
	params.Set("sort_order", "downloads")
	params.Set("start_index", fmt.Sprintf("%d", (page-1)*searchPageSize+1))
	if query != "" {
		params.Set("query", query)
	}
	u := gutenbergBaseURL + "/ebooks/search.opds/?" + params.Encode()

	body, err := fetchBytes(ctx, resolveClient(httpClient), u)
	if err != nil {
		return SearchResult{}, fmt.Errorf("gutenberg search: %w", err)
	}
	var feed opdsFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return SearchResult{}, fmt.Errorf("gutenberg search: decode feed: %w", err)
	}

	books := make([]Book, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		if b, ok := toBook(e); ok {
			books = append(books, b)
		}
	}
	hasNext := false
	for _, l := range feed.Links {
		if l.Rel == "next" {
			hasNext = true
		}
	}
	return SearchResult{
		Books:   books,
		HasNext: hasNext,
		HasPrev: page > 1,
		Page:    page,
	}, nil
}

// Fetch downloads a book's UTF-8 plain text from the Gutenberg cache (the
// mirror first, then www.gutenberg.org) and reads its title/author from the
// text's own header, so an import needs ONE successful request. The returned
// raw text is ready for Parse.
func Fetch(ctx context.Context, httpClient HTTPClient, sourceID string) (title, author, raw string, err error) {
	client := resolveClient(httpClient)
	var body []byte
	for _, u := range textURLs(sourceID) {
		if body, err = fetchBytes(ctx, client, u); err == nil {
			break
		}
	}
	if err != nil {
		return "", "", "", fmt.Errorf("download text: %w", err)
	}
	raw = string(body)
	title, author = parseHeader(raw)
	if title == "" {
		title = "Project Gutenberg eBook #" + sourceID
	}
	return title, author, raw, nil
}

// textURL is the plain-text (UTF-8) download URL for a Gutenberg book id on
// www.gutenberg.org.
func textURL(sourceID string) string {
	return cacheTextURL(gutenbergBaseURL, sourceID)
}

// textURLs lists where Fetch looks for a book's text, in order.
func textURLs(sourceID string) []string {
	return []string{cacheTextURL(mirrorBaseURL, sourceID), textURL(sourceID)}
}

func cacheTextURL(base, sourceID string) string {
	id := url.PathEscape(sourceID)
	return fmt.Sprintf("%s/cache/epub/%s/pg%s.txt", base, id, id)
}

// toBook maps an OPDS search entry to our Book; ok is false for the feed's
// navigation entries, which carry no book id.
func toBook(e opdsEntry) (Book, bool) {
	m := opdsBookID.FindStringSubmatch(strings.TrimSpace(e.ID))
	if m == nil {
		return Book{}, false
	}
	b := Book{
		SourceID: m[1],
		Title:    strings.TrimSpace(e.Title),
		TextURL:  textURL(m[1]),
	}
	// Search entries put the author in <content>; acquisition-style entries
	// use <author><name>.
	if len(e.Authors) > 0 {
		b.Author = strings.TrimSpace(e.Authors[0])
	} else {
		b.Author = strings.TrimSpace(e.Content)
	}
	for _, l := range e.Links {
		if strings.HasPrefix(l.Type, "image/") && strings.HasSuffix(l.Rel, "/thumbnail") {
			b.CoverURL = absoluteURL(l.Href)
		}
	}
	return b, true
}

// absoluteURL resolves a feed-relative href ("/cache/...") against gutenberg.org.
func absoluteURL(href string) string {
	if strings.HasPrefix(href, "/") {
		return gutenbergBaseURL + href
	}
	return href
}

// parseHeader reads the "Title:" and "Author:" lines of the Project Gutenberg
// header (everything before the START marker). A value may wrap onto indented
// continuation lines, which are joined with a space.
func parseHeader(raw string) (title, author string) {
	header := raw
	if loc := pgStartMarker.FindStringIndex(raw); loc != nil {
		header = raw[:loc[0]]
	}
	var current *string
	for _, line := range strings.Split(header, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "Title:"):
			title = strings.TrimSpace(strings.TrimPrefix(line, "Title:"))
			current = &title
		case strings.HasPrefix(line, "Author:"):
			author = strings.TrimSpace(strings.TrimPrefix(line, "Author:"))
			current = &author
		case current != nil && strings.TrimSpace(line) != "" && strings.TrimLeft(line, " \t") != line:
			*current += " " + strings.TrimSpace(line)
		default:
			current = nil
		}
	}
	return title, author
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
