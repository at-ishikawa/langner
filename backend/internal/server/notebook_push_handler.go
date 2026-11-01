package server

import (
	"context"
	"fmt"
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
// the server-minted nb_ id, preserving the rest of the bytes (the content hash
// covers the bundle as stored). The app owns notebook identity
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
// nb_ id (create) or confirms ownership (update), parses the bundle, stores its
// content rows + a private ownership row transactionally, then creates the
// notes its words need so the notebook is immediately quizzable by the caller.
// Private by default.
func (h *NotebookHandler) PushNotebook(
	ctx context.Context,
	req *connect.Request[apiv1.PushNotebookRequest],
) (*connect.Response[apiv1.PushNotebookResponse], error) {
	userID, err := requireUserID(ctx)
	if err != nil {
		return nil, err
	}
	if h.db == nil || h.store == nil {
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
	// new notebooks). It UPDATES an existing notebook in place — preserving its id
	// (so learning history stays attached) and source ('shipped' stays 'shipped')
	// — in three cases, in order: (1) an explicit --id the caller owns; (2) a
	// declared id that already exists and the caller owns (a claimed shipped
	// notebook); (3) the caller already owns exactly one notebook with the SAME
	// display name (the natural "edit and re-push" workflow — re-pushing an
	// updated bundle updates that notebook instead of creating a same-named
	// duplicate). Only when none of these match does push mint a fresh id.
	declaredID := strings.TrimSpace(meta.ID)
	notebookID := strings.TrimSpace(msg.NotebookId)
	storedSource := "user"
	visibility := notebook.VisibilityPrivate

	// Display name (used both as the stored name and as the name-match key).
	name := meta.Name
	if strings.TrimSpace(name) == "" {
		name = strings.TrimSpace(msg.Name)
	}

	// claimExisting resolves id as an update target: (claimed=true) when it
	// exists and the caller owns it (adopting its source/visibility), an error
	// when it exists but belongs to someone else, (claimed=false) when absent.
	claimExisting := func(id string) (bool, error) {
		owner, source, vis, found, gerr := h.store.GetNotebookOwnership(ctx, id)
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

	// claimByName resolves an update target by display name among the caller's
	// OWN notebooks. 0 matches → (claimed=false, mint a fresh id). Exactly 1 →
	// update it in place. >1 → the name is already ambiguous (a pre-existing
	// duplicate), so refuse and point at --id / delete rather than guess which to
	// overwrite. Name-matching is what makes an edited bundle re-push update the
	// same notebook instead of creating a second one with the same name.
	claimByName := func(displayName string) (bool, error) {
		if strings.TrimSpace(displayName) == "" {
			return false, nil
		}
		owned, lerr := h.store.ListUserNotebooks(ctx, userID)
		if lerr != nil {
			return false, connect.NewError(connect.CodeInternal, lerr)
		}
		var matched []notebook.UserNotebook
		for _, n := range owned {
			if n.DisplayName == displayName {
				matched = append(matched, n)
			}
		}
		switch len(matched) {
		case 0:
			return false, nil
		case 1:
			notebookID = matched[0].NotebookID
			if s := strings.TrimSpace(matched[0].Source); s != "" {
				storedSource = s
			}
			visibility = matched[0].Visibility
			return true, nil
		default:
			return false, connect.NewError(connect.CodeFailedPrecondition,
				fmt.Errorf("you have %d notebooks named %q; re-push with --id <nb_id> to pick one, or delete the duplicate with `langner notebooks delete <nb_id>`", len(matched), displayName))
		}
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
	default:
		// No explicit --id: try a declared id the caller owns (claimed shipped
		// notebook), else match by display name, else mint a fresh id.
		claimed := false
		if declaredID != "" {
			c, cerr := claimExisting(declaredID)
			if cerr != nil {
				return nil, cerr
			}
			claimed = c
		}
		if !claimed {
			c, cerr := claimByName(name)
			if cerr != nil {
				return nil, cerr
			}
			claimed = c
		}
		if !claimed {
			if notebookID, err = notebook.MintNotebookID(); err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
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

	// A single-family push's files live in the family its kind names.
	for i := range files {
		if strings.TrimSpace(files[i].Family) == "" {
			files[i].Family = notebook.FamilyForKind(kind)
		}
	}
	content, err := notebook.ParseBundle(notebookID, files)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("parse notebook: %w", err))
	}

	nb := notebook.UserNotebook{
		NotebookID:  notebookID,
		OwnerUserID: &userID,
		Visibility:  visibility,   // private on create; preserved on update
		Source:      storedSource, // 'user' on create; preserved ('shipped') on update
		Kind:        kind,
		DisplayName: name,
		ContentHash: notebook.HashBundle(files),
		ByteSize:    notebook.BundleBytes(files),
	}
	if err := h.store.PushBundle(ctx, nb, content); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("store notebook: %w", err))
	}

	// Create the notes the notebook's words need so it is quizzable and its
	// learning logs resolve. Additive: existing notes are kept.
	importedNotes, ierr := datasync.EnsureContentNotes(ctx, h.db, content)
	if ierr != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("import notebook: %w", ierr))
	}

	return connect.NewResponse(&apiv1.PushNotebookResponse{
		NotebookId:   notebookID,
		ImportedRows: map[string]int32{"notes": int32(importedNotes)},
	}), nil
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

// PullNotebook returns an owned notebook as YAML files rendered from its stored
// content (owner-only, gated by the same VisibleNotebookIDs predicate). A
// notebook with several families returns each file under its family folder,
// the layout a composite push takes.
func (h *NotebookHandler) PullNotebook(
	ctx context.Context,
	req *connect.Request[apiv1.PullNotebookRequest],
) (*connect.Response[apiv1.PullNotebookResponse], error) {
	userID, err := requireUserID(ctx)
	if err != nil {
		return nil, err
	}
	if h.store == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("per-user notebooks require a configured database"))
	}
	notebookID := req.Msg.NotebookId
	if err := h.ensureNotebookVisible(ctx, userID, notebookID); err != nil {
		return nil, err
	}
	nb, found, err := h.store.GetUserNotebook(ctx, notebookID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if !found {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("notebook %s not found", notebookID))
	}
	content, _, err := h.store.LoadContent(ctx, notebookID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	files, err := content.Bundle()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	out := make([]*apiv1.NotebookFile, 0, len(files))
	for _, f := range files {
		p := f.Path
		if len(content.Families) > 1 {
			p = f.Family + "/" + p
		}
		out = append(out, &apiv1.NotebookFile{Path: p, Content: f.Content})
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
	if h.store == nil {
		return connect.NewResponse(&apiv1.ListMyNotebooksResponse{}), nil
	}
	rows, err := h.store.ListUserNotebooks(ctx, userID)
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

// DeleteNotebook permanently removes a notebook the caller owns — its content,
// per-notebook memberships, and ownership row (shared notes are left
// intact). Used to clean up duplicates. NotFound when the id doesn't exist;
// PermissionDenied when it exists but belongs to someone else.
func (h *NotebookHandler) DeleteNotebook(
	ctx context.Context,
	req *connect.Request[apiv1.DeleteNotebookRequest],
) (*connect.Response[apiv1.DeleteNotebookResponse], error) {
	userID, err := requireUserID(ctx)
	if err != nil {
		return nil, err
	}
	if h.db == nil || h.store == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("per-user notebooks require a configured database"))
	}
	id := strings.TrimSpace(req.Msg.GetNotebookId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("notebook_id is required"))
	}
	owner, _, _, found, gerr := h.store.GetNotebookOwnership(ctx, id)
	if gerr != nil {
		return nil, connect.NewError(connect.CodeInternal, gerr)
	}
	if !found {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("notebook %s not found", id))
	}
	if owner == nil || *owner != userID {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("not the owner of %s", id))
	}
	if err := h.store.DeleteUserNotebook(ctx, id); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("delete notebook: %w", err))
	}
	return connect.NewResponse(&apiv1.DeleteNotebookResponse{NotebookId: id}), nil
}
