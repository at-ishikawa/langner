package server

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"connectrpc.com/connect"
	"gopkg.in/yaml.v3"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/auth"
	"github.com/at-ishikawa/langner/internal/datasync"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// requireUserID returns the authenticated caller's id, or a CodeUnauthenticated
// error. All per-user-notebook RPCs are owner-scoped, so a real userID > 0 is
// mandatory (the auth interceptor already guarantees it in a gated deployment;
// this is defense in depth for the RPC boundary).
func requireUserID(ctx context.Context) (int64, error) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok || userID <= 0 {
		return 0, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("authentication required"))
	}
	return userID, nil
}

// indexMeta is the subset of an index.yml the push handler reads to record
// provenance on the notebooks row and to rewrite the notebook's identity.
type indexMeta struct {
	ID   string `yaml:"id"`
	Kind string `yaml:"kind"`
	Name string `yaml:"name"`
}

// findIndexFile returns the bundle's index.yml content, or ok=false.
func findIndexFile(files []notebook.NotebookFile) (notebook.NotebookFile, bool) {
	for _, f := range files {
		if filepath.Base(f.Path) == "index.yml" {
			return f, true
		}
	}
	return notebook.NotebookFile{}, false
}

// rewriteTopLevelID replaces the first top-level `id:` line in an index.yml with
// the server-minted nb_ id, preserving the rest of the bytes so pull stays a
// faithful round-trip of what the server stored. The app owns notebook identity
// (design §6): whatever `id:` the author wrote is ignored for identity.
func rewriteTopLevelID(content []byte, newID string) []byte {
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		// top-level (unindented) id: key only
		if line == trimmed && (strings.HasPrefix(trimmed, "id:") || strings.HasPrefix(trimmed, "id :")) {
			lines[i] = "id: " + newID
			return []byte(strings.Join(lines, "\n"))
		}
	}
	// No id: line — prepend one so the notebook has an identity.
	return []byte("id: " + newID + "\n" + string(content))
}

// PushNotebook validates + stores a user-authored notebook bundle: it mints an
// nb_ id (create) or confirms ownership (update), rewrites the index.yml id,
// writes the raw blobs + a private ownership row transactionally, then runs the
// existing additive importer over the blobs so the notebook is immediately
// quizzable by the caller. Private by default.
func (h *NotebookHandler) PushNotebook(
	ctx context.Context,
	req *connect.Request[apiv1.PushNotebookRequest],
) (*connect.Response[apiv1.PushNotebookResponse], error) {
	userID, err := requireUserID(ctx)
	if err != nil {
		return nil, err
	}
	if h.db == nil || h.fileRepo == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("per-user notebooks require a configured database"))
	}
	msg := req.Msg
	if len(msg.Files) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("no files in push"))
	}

	// A composite push (kind == compositeKind) carries files whose path is
	// prefixed with their family bucket ("definitions/index.yml",
	// "etymology/origins.yml"), so one notebook id can span several families
	// exactly like the filesystem catalog (design §13). A single-family push
	// leaves paths unprefixed and derives the family from kind at storage time.
	composite := strings.TrimSpace(msg.Kind) == compositeKind
	files := make([]notebook.NotebookFile, 0, len(msg.Files))
	for _, f := range msg.Files {
		p := filepath.ToSlash(f.Path)
		nf := notebook.NotebookFile{Path: p, Content: f.Content}
		if composite {
			seg, rest, ok := strings.Cut(p, "/")
			if !ok || !knownFamily(seg) {
				return nil, connect.NewError(connect.CodeInvalidArgument,
					fmt.Errorf("composite push file %q must live under a family dir (definitions/, etymology/, stories/, …)", p))
			}
			nf.Family = seg
			nf.Path = rest
		}
		files = append(files, nf)
	}
	indexFile, ok := findIndexFile(files)
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("bundle has no index.yml"))
	}
	var meta indexMeta
	if err := yaml.Unmarshal(indexFile.Content, &meta); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("parse index.yml: %w", err))
	}

	// Resolve the notebook id (create-or-update). Policy: a fresh push MINTS a
	// new nb_ id and ignores the author's declared id (ids are server-owned for
	// new notebooks). The ONE exception is an UPDATE — when the id already
	// exists AND the caller owns it, push updates it in place, preserving the id
	// (so learning history stays attached) and its source ('shipped' stays
	// 'shipped'). An explicit --id must be an existing owned notebook.
	declaredID := strings.TrimSpace(meta.ID)
	notebookID := strings.TrimSpace(msg.NotebookId)
	storedSource := "user"
	visibility := notebook.VisibilityPrivate

	// claimExisting resolves id as an update target: (claimed=true) when it
	// exists and the caller owns it (adopting its source/visibility), an error
	// when it exists but belongs to someone else, (claimed=false) when absent.
	claimExisting := func(id string) (bool, error) {
		owner, source, vis, found, gerr := h.fileRepo.GetNotebookOwnership(ctx, id)
		if gerr != nil {
			return false, connect.NewError(connect.CodeInternal, gerr)
		}
		if !found {
			return false, nil
		}
		if owner == nil || *owner != userID {
			return false, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("not the owner of %s", id))
		}
		notebookID = id
		if s := strings.TrimSpace(source); s != "" {
			storedSource = s
		}
		visibility = vis
		return true, nil
	}

	switch {
	case notebookID != "":
		claimed, cerr := claimExisting(notebookID)
		if cerr != nil {
			return nil, cerr
		}
		if !claimed {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("notebook %s not found", notebookID))
		}
	case declaredID != "":
		claimed, cerr := claimExisting(declaredID)
		if cerr != nil {
			return nil, cerr
		}
		if !claimed {
			if notebookID, err = notebook.MintNotebookID(); err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
		}
	default:
		if notebookID, err = notebook.MintNotebookID(); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}

	// Rewrite the index.yml id to the minted/confirmed nb_ id so display,
	// storage, and lookup stay symmetric (learning-history invariant L3).
	for i := range files {
		if filepath.Base(files[i].Path) == "index.yml" {
			files[i].Content = rewriteTopLevelID(files[i].Content, notebookID)
		}
	}

	kind := meta.Kind
	if strings.TrimSpace(kind) == "" {
		kind = strings.TrimSpace(msg.Kind)
	}
	if composite {
		// Family placement is per-file for a composite notebook; the single
		// `kind` column would mislabel it, so leave it empty (display only).
		kind = ""
	}
	name := meta.Name
	if strings.TrimSpace(name) == "" {
		name = strings.TrimSpace(msg.Name)
	}

	nb := notebook.UserNotebook{
		NotebookID:  notebookID,
		OwnerUserID: &userID,
		Visibility:  visibility,   // private on create; preserved on update
		Source:      storedSource, // 'user' on create; preserved ('shipped') on update
		Kind:        kind,
		DisplayName: name,
		ContentHash: notebook.HashBundle(files),
	}
	if err := h.fileRepo.PushBundle(ctx, nb, files); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("store notebook: %w", err))
	}

	// Import the stored blobs so notes/notebook_notes exist (the notebook is
	// quizzable and its learning logs resolvable). Reuses the SAME additive,
	// no-reconcile importer path ensure-on-serve uses — no new ingestion path.
	importedNotes, ierr := h.importPushedNotebook(ctx, notebookID, kind, files)
	if ierr != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("import notebook: %w", ierr))
	}

	return connect.NewResponse(&apiv1.PushNotebookResponse{
		NotebookId:   notebookID,
		ImportedRows: map[string]int32{"notes": int32(importedNotes)},
	}), nil
}

// importPushedNotebook materializes the pushed blobs to a temp dir, builds a
// reader over just that notebook, and runs EnsureNotesForNotebook — the
// additive (never-deletes) importer path — so the notebook's notes +
// notebook_notes exist in the DB. Returns the number of notes created.
func (h *NotebookHandler) importPushedNotebook(ctx context.Context, notebookID, kind string, files []notebook.NotebookFile) (int, error) {
	root, err := os.MkdirTemp("", "langner-push-*")
	if err != nil {
		return 0, fmt.Errorf("temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(root) }()

	// Materialize each file under <root>/<family>/<notebookID>/<path>, keyed by
	// the file's OWN family (falling back to the notebook kind for a
	// single-family push), so a composite notebook's several families each land
	// under their own root — the same layout DBContentSource uses, so the reader
	// registers one id across several family maps with no divergence (§13.2).
	dirs := notebook.ContentDirs{}
	used := map[string]bool{}
	for _, f := range files {
		family := strings.TrimSpace(f.Family)
		if family == "" {
			family = familyForKind(kind)
		}
		dest := filepath.Join(root, family, notebookID, filepath.Clean("/"+f.Path))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return 0, fmt.Errorf("mkdir: %w", err)
		}
		if err := os.WriteFile(dest, f.Content, 0o644); err != nil {
			return 0, fmt.Errorf("write: %w", err)
		}
		used[family] = true
	}
	add := func(family string, target *[]string) {
		if used[family] {
			*target = append(*target, filepath.Join(root, family))
		}
	}
	add("stories", &dirs.Stories)
	add("journals", &dirs.Journals)
	add("flashcards", &dirs.Flashcards)
	add("books", &dirs.Books)
	add("definitions", &dirs.Definitions)
	add("etymology", &dirs.Etymology)

	reader, err := notebook.NewReader(dirs.Stories, dirs.Flashcards, dirs.Books, dirs.Definitions, dirs.Etymology, nil)
	if err != nil {
		return 0, fmt.Errorf("reader: %w", err)
	}
	noteSource := notebook.NewYAMLNoteRepository(reader)
	noteRepo := notebook.NewDBNoteRepository(h.db)
	importer := datasync.NewImporter(noteRepo, nil, noteSource, nil, nil, nil, io.Discard)
	return importer.EnsureNotesForNotebook(ctx, notebookID)
}

// compositeKind is the reserved PushNotebookRequest.kind sentinel signaling a
// multi-family bundle whose file paths are family-prefixed (design §13.6). It is
// not a real notebook kind.
const compositeKind = "composite"

// knownFamily reports whether seg is a ContentDirs family bucket name — the set
// a composite push's path prefix must name.
func knownFamily(seg string) bool {
	switch seg {
	case "stories", "journals", "flashcards", "books", "definitions", "etymology", "grammars":
		return true
	}
	return false
}

// familyForKind mirrors notebook.familyDir for the push-time reader assembly.
func familyForKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "story":
		return "stories"
	case "journal":
		return "journals"
	case "book":
		return "books"
	case "definitions", "definition":
		return "definitions"
	case "etymology":
		return "etymology"
	default:
		return "flashcards"
	}
}

// PullNotebook streams the stored blobs of an owned notebook back to the caller
// (owner-only, gated by the same VisibleNotebookIDs predicate).
func (h *NotebookHandler) PullNotebook(
	ctx context.Context,
	req *connect.Request[apiv1.PullNotebookRequest],
) (*connect.Response[apiv1.PullNotebookResponse], error) {
	userID, err := requireUserID(ctx)
	if err != nil {
		return nil, err
	}
	if h.fileRepo == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("per-user notebooks require a configured database"))
	}
	notebookID := req.Msg.NotebookId
	if err := h.ensureNotebookVisible(ctx, userID, notebookID); err != nil {
		return nil, err
	}
	nb, found, err := h.fileRepo.GetUserNotebook(ctx, notebookID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if !found {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("notebook %s not found", notebookID))
	}
	files, err := h.fileRepo.ListFiles(ctx, notebookID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	out := make([]*apiv1.NotebookFile, 0, len(files))
	for _, f := range files {
		out = append(out, &apiv1.NotebookFile{Path: f.Path, Content: f.Content})
	}
	return connect.NewResponse(&apiv1.PullNotebookResponse{
		Files:       out,
		ContentHash: nb.ContentHash,
	}), nil
}

// ListMyNotebooks returns the caller's own user notebooks.
func (h *NotebookHandler) ListMyNotebooks(
	ctx context.Context,
	req *connect.Request[apiv1.ListMyNotebooksRequest],
) (*connect.Response[apiv1.ListMyNotebooksResponse], error) {
	userID, err := requireUserID(ctx)
	if err != nil {
		return nil, err
	}
	if h.fileRepo == nil {
		return connect.NewResponse(&apiv1.ListMyNotebooksResponse{}), nil
	}
	rows, err := h.fileRepo.ListUserNotebooks(ctx, userID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	entries := make([]*apiv1.ListMyNotebooksResponse_Entry, 0, len(rows))
	for _, r := range rows {
		entries = append(entries, &apiv1.ListMyNotebooksResponse_Entry{
			NotebookId: r.NotebookID,
			Name:       r.DisplayName,
			Kind:       r.Kind,
			Visibility: r.Visibility,
			ByteSize:   int32(r.ByteSize),
			UpdatedAt:  r.UpdatedAt,
		})
	}
	return connect.NewResponse(&apiv1.ListMyNotebooksResponse{Notebooks: entries}), nil
}
