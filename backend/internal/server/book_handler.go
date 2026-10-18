package server

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"gopkg.in/yaml.v3"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/ebook"
	"github.com/at-ishikawa/langner/internal/gutenberg"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// SearchBooks queries the public Project Gutenberg catalog (keyless Gutendex)
// by free text and returns catalog hits. Discovery only — no DB and no auth
// gating beyond the always-on interceptor. Outbound HTTP runs only in
// preview/prod (the dev container/CI has no egress), so this surfaces an error
// there rather than failing the build.
func (h *NotebookHandler) SearchBooks(
	ctx context.Context,
	req *connect.Request[apiv1.SearchBooksRequest],
) (*connect.Response[apiv1.SearchBooksResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	res, err := gutenberg.Search(ctx, h.bookHTTPClient, req.Msg.GetQuery(), int(req.Msg.GetPage()))
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("search books: %w", err))
	}
	results := make([]*apiv1.BookResult, 0, len(res.Books))
	for _, b := range res.Books {
		results = append(results, &apiv1.BookResult{
			SourceId: b.SourceID,
			Title:    b.Title,
			Author:   b.Author,
			Language: b.Language,
			CoverUrl: b.CoverURL,
		})
	}
	return connect.NewResponse(&apiv1.SearchBooksResponse{
		Books:   results,
		HasNext: res.HasNext,
		HasPrev: res.HasPrev,
		Page:    int32(res.Page),
	}), nil
}

// ImportBook downloads + parses a Project Gutenberg book and stores it ONCE as
// a shared, public, owner-less notebook under the deterministic id
// `gutenberg-<source_id>` via ImportShippedBundle (owner_user_id=NULL,
// visibility='public', upsert by id). A re-import by anyone is an idempotent
// no-op — one shared copy every user can read and quiz; private highlights stay
// per-user (user_notebook_definitions). Requires a signed-in user and a
// configured database (mirrors PushNotebook's Unimplemented guard).
func (h *NotebookHandler) ImportBook(
	ctx context.Context,
	req *connect.Request[apiv1.ImportBookRequest],
) (*connect.Response[apiv1.ImportBookResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	if _, err := requireUserID(ctx); err != nil {
		return nil, err
	}
	if h.db == nil || h.fileRepo == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("importing books requires a configured database"))
	}

	sourceID := req.Msg.GetSourceId()
	title, author, raw, err := gutenberg.Fetch(ctx, h.bookHTTPClient, sourceID)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("fetch book %s: %w", sourceID, err))
	}

	// Stamp the chapters with the import time so the book sorts by import
	// recency in the quiz-hub list (which orders by latest content date); a
	// freshly imported book otherwise has a zero date and sinks to the bottom.
	notebookID, files, displayName, err := buildBookBundle(sourceID, title, author, raw, time.Now())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("parse book %s: %w", sourceID, err))
	}

	// kind "book" → familyDir "books", so blobs are stored under the books
	// family and the reader classifies the notebook as a readable book
	// (walkIndexFiles(books, …, isBook=true)); preserveOrder then keeps chapter
	// order instead of sorting by the (zero) entry date.
	if err := h.fileRepo.ImportShippedBundle(ctx, notebookID, "book", displayName, notebook.HashBundle(files), files); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("store book: %w", err))
	}
	return connect.NewResponse(&apiv1.ImportBookResponse{NotebookId: notebookID}), nil
}

// buildBookBundle parses a raw Project Gutenberg plain-text book into stored
// notebook files, overriding the index id with the DETERMINISTIC shared id
// `gutenberg-<source_id>` (the parser slugs the title; identity is server-owned
// here so a re-import of the same book dedups by id) and tagging every file
// with the "books" family. Pure (no network, no DB) so the classification +
// id-minting wiring is unit testable offline. Returns the id, the files, and
// the display name from the parsed index.
func buildBookBundle(sourceID, title, author, raw string, importedAt time.Time) (notebookID string, files []notebook.NotebookFile, displayName string, err error) {
	bundle, err := gutenberg.Parse(raw, title, author)
	if err != nil {
		return "", nil, "", err
	}
	notebookID = "gutenberg-" + sourceID

	files = make([]notebook.NotebookFile, 0, len(bundle.Files))
	for _, f := range bundle.Files {
		content := f.Content
		if f.Path == "index.yml" {
			rewritten, name, rerr := rewriteBookIndex(f.Content, notebookID)
			if rerr != nil {
				return "", nil, "", fmt.Errorf("rewrite index: %w", rerr)
			}
			content = rewritten
			displayName = name
		} else {
			// Chapter file: stamp the import time as the entry date so the book
			// sorts by import recency in the quiz-hub list.
			stamped, serr := stampBookDate(f.Content, importedAt)
			if serr != nil {
				return "", nil, "", fmt.Errorf("stamp date for %s: %w", f.Path, serr)
			}
			content = stamped
		}
		files = append(files, notebook.NotebookFile{
			NotebookID: notebookID,
			Family:     "books",
			Path:       f.Path,
			Content:    content,
		})
	}
	return notebookID, files, displayName, nil
}

// stampBookDate sets the entry date on every chapter in a book file (the parser
// emits a single-element []NotebookEntry per chapter with a zero date). The date
// is read by the quiz-hub summary to order notebooks by recency; keeping it off
// the pure parser means Parse stays deterministic and this import-time concern
// lives with the importer.
func stampBookDate(content []byte, when time.Time) ([]byte, error) {
	var entries []ebook.NotebookEntry
	if err := yaml.Unmarshal(content, &entries); err != nil {
		return nil, err
	}
	for i := range entries {
		entries[i].Date = when
	}
	return yaml.Marshal(entries)
}

// rewriteBookIndex sets the index.yml `id:` to the deterministic shared id,
// returning the re-marshalled bytes and the index's display name. Round-tripping
// through ebook.NotebookIndex (the exact shape Parse emits) keeps the stored
// index byte-compatible with the reader.
func rewriteBookIndex(content []byte, notebookID string) (rewritten []byte, displayName string, err error) {
	var idx ebook.NotebookIndex
	if err := yaml.Unmarshal(content, &idx); err != nil {
		return nil, "", err
	}
	idx.ID = notebookID
	out, err := yaml.Marshal(idx)
	if err != nil {
		return nil, "", err
	}
	return out, idx.Name, nil
}
