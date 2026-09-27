package clicmd

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/gen-protos/api/v1/apiv1connect"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// bundleFile is one resolved file of a notebook bundle, keyed by its path
// relative to the bundle root (e.g. "index.yml", "cards.yml"). For a composite
// bundle RelPath is family-prefixed ("definitions/index.yml") and Family names
// the bucket, so the server can route one id across several families (§13.6).
type bundleFile struct {
	Family  string
	RelPath string
	Content []byte
}

// compositeBundleKind is the reserved PushNotebookRequest.kind the CLI sends for
// a multi-family bundle (must match server.compositeKind). Family is then read
// from each file's path prefix, not from this field.
const compositeBundleKind = "composite"

// clientKnownFamily reports whether name is a notebook family bucket dir.
func clientKnownFamily(name string) bool {
	switch name {
	case "stories", "journals", "flashcards", "books", "definitions", "etymology", "grammars":
		return true
	}
	return false
}

// kindForFamily returns the index.yml `kind:` a family's bundle validates as, so
// a family subdir (whose own index.yml may omit kind, e.g. definitions) is
// checked through the right reader.
func kindForFamily(family string) string {
	switch family {
	case "stories":
		return "story"
	case "books":
		return "book"
	case "definitions":
		return "definitions"
	case "etymology":
		return "etymology"
	default: // flashcards / journals / grammars validate via the flashcard/story readers
		return ""
	}
}

// bundleMeta is the index.yml metadata the CLI reads to describe a bundle.
type bundleMeta struct {
	ID        string   `yaml:"id"`
	Kind      string   `yaml:"kind"`
	Name      string   `yaml:"name"`
	Notebooks []string `yaml:"notebooks"`
}

// resolveBundle reads a notebook bundle from a directory (containing index.yml)
// or a single index.yml file. It returns index.yml plus every sibling file its
// `notebooks:` list references, each with a bundle-relative path — the exact
// set the server stores and re-parses. A directory without index.yml, or a
// single non-index file, is an error (a pushable notebook needs an index.yml).
func resolveBundle(path string) ([]bundleFile, bundleMeta, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, bundleMeta{}, fmt.Errorf("stat %s: %w", path, err)
	}

	var root, indexPath string
	if info.IsDir() {
		root = path
		indexPath = filepath.Join(path, "index.yml")
	} else if filepath.Base(path) == "index.yml" {
		root = filepath.Dir(path)
		indexPath = path
	} else {
		return nil, bundleMeta{}, fmt.Errorf("point at the notebook directory (or its index.yml); %s is not an index.yml", path)
	}

	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, bundleMeta{}, fmt.Errorf("read index.yml: %w", err)
	}
	var meta bundleMeta
	if err := yaml.Unmarshal(indexBytes, &meta); err != nil {
		return nil, bundleMeta{}, fmt.Errorf("parse index.yml: %w", err)
	}

	files := []bundleFile{{RelPath: "index.yml", Content: indexBytes}}
	seen := map[string]bool{"index.yml": true}
	for _, nb := range meta.Notebooks {
		rel := filepath.ToSlash(filepath.Clean(strings.TrimPrefix(nb, "./")))
		if seen[rel] {
			continue
		}
		content, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, bundleMeta{}, fmt.Errorf("read referenced file %s: %w", rel, err)
		}
		files = append(files, bundleFile{RelPath: rel, Content: content})
		seen[rel] = true
	}
	return files, meta, nil
}

// resolveNotebookBundle resolves either a single-family bundle (a dir with an
// index.yml, or an index.yml file) or a COMPOSITE bundle (a parent dir with no
// top-level index.yml whose subdirs are family buckets — definitions/,
// etymology/, … — that share one id). It is the entry point `push` uses so a
// user hands a composite notebook the same way it lives on disk (§13.6).
func resolveNotebookBundle(path string) ([]bundleFile, bundleMeta, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, bundleMeta{}, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.IsDir() {
		if _, e := os.Stat(filepath.Join(path, "index.yml")); e != nil {
			return resolveCompositeBundle(path) // no top-level index.yml → family subdirs
		}
	}
	return resolveBundle(path)
}

// resolveCompositeBundle reads a parent dir whose subdirs are family buckets
// (each a single-family bundle) sharing one id, validates each through its
// family's reader, and returns the union with family-prefixed paths + a
// compositeBundleKind meta. The server splits the prefix back into per-file
// families so the one id registers across several family maps (§13.2).
func resolveCompositeBundle(dir string) ([]bundleFile, bundleMeta, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, bundleMeta{}, fmt.Errorf("read %s: %w", dir, err)
	}
	var files []bundleFile
	var id, name string
	families := 0
	for _, e := range entries {
		family := e.Name()
		if !e.IsDir() || !clientKnownFamily(family) {
			continue
		}
		sub := filepath.Join(dir, family)
		if _, err := os.Stat(filepath.Join(sub, "index.yml")); err != nil {
			continue
		}
		subFiles, subMeta, err := resolveBundle(sub)
		if err != nil {
			return nil, bundleMeta{}, fmt.Errorf("family %s: %w", family, err)
		}
		// Validate each family through the reader its family expects.
		vMeta := subMeta
		if k := kindForFamily(family); k != "" {
			vMeta.Kind = k
		}
		if err := validateBundleLocally(subFiles, vMeta); err != nil {
			return nil, bundleMeta{}, fmt.Errorf("family %s: %w", family, err)
		}
		sid := strings.TrimSpace(subMeta.ID)
		if id == "" {
			id = sid
		} else if sid != id {
			return nil, bundleMeta{}, fmt.Errorf("composite families must share one id: %s declares %q but expected %q", family, sid, id)
		}
		if name == "" {
			name = subMeta.Name
		}
		for _, sf := range subFiles {
			files = append(files, bundleFile{Family: family, RelPath: family + "/" + sf.RelPath, Content: sf.Content})
		}
		families++
	}
	if families == 0 {
		return nil, bundleMeta{}, fmt.Errorf("%s has no top-level index.yml and no family subdirs (definitions/, etymology/, …) with an index.yml", dir)
	}
	if id == "" {
		return nil, bundleMeta{}, fmt.Errorf("composite notebook families must each declare the same id")
	}
	return files, bundleMeta{ID: id, Kind: compositeBundleKind, Name: name}, nil
}

// validateBundleLocally parses the bundle through the SAME notebook reader the
// server uses, catching structural errors before an upload. It materializes the
// files to a temp dir, builds a reader with the bundle slotted into the family
// its kind names, and confirms the notebook loads. Returns a descriptive error
// on failure so `push` can fail fast.
func validateBundleLocally(files []bundleFile, meta bundleMeta) error {
	root, err := os.MkdirTemp("", "langner-validate-*")
	if err != nil {
		return fmt.Errorf("temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(root) }()

	for _, f := range files {
		dest := filepath.Join(root, filepath.Clean("/"+f.RelPath))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("mkdir: %w", err)
		}
		if err := os.WriteFile(dest, f.Content, 0o644); err != nil {
			return fmt.Errorf("write: %w", err)
		}
	}

	var stories, flashcards, books, definitions, etymology []string
	switch strings.ToLower(strings.TrimSpace(meta.Kind)) {
	case "story":
		stories = []string{root}
	case "book":
		books = []string{root}
	case "definitions", "definition":
		definitions = []string{root}
	case "etymology":
		etymology = []string{root}
	default: // flashcard / journal / grammar all parse via story/flashcard readers
		flashcards = []string{root}
		stories = []string{root}
	}

	reader, err := notebook.NewReader(stories, flashcards, books, definitions, etymology, nil)
	if err != nil {
		return fmt.Errorf("parse notebook: %w", err)
	}

	id := meta.ID
	switch strings.ToLower(strings.TrimSpace(meta.Kind)) {
	case "definitions", "definition":
		if _, ok := reader.GetDefinitionsBook(id); !ok {
			return fmt.Errorf("definitions book %q has no readable sessions", id)
		}
	case "story":
		if _, err := reader.ReadStoryNotebooks(id); err != nil {
			return fmt.Errorf("read story %q: %w", id, err)
		}
	case "etymology":
		if _, err := reader.ReadEtymologyNotebook(id); err != nil {
			return fmt.Errorf("read etymology %q: %w", id, err)
		}
	default:
		if _, err := reader.ReadFlashcardNotebooks(id); err != nil {
			return fmt.Errorf("read flashcards %q: %w", id, err)
		}
	}
	return nil
}

// newNotebookClient builds a bearer-authenticated NotebookService client for a
// server, reusing the shared credential store + BearerInterceptor (#80).
func newNotebookClient(store *CredentialStore, server string) apiv1connect.NotebookServiceClient {
	httpClient := &http.Client{Timeout: 120 * time.Second}
	return apiv1connect.NewNotebookServiceClient(
		httpClient,
		strings.TrimRight(server, "/"),
		connect.WithInterceptors(NewBearerInterceptor(store, server)),
	)
}

// NewNotebooksCommand builds `langner notebooks` (push / pull / list).
func NewNotebooksCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notebooks",
		Short: "Manage your own notebooks on a langner server (push / pull / list)",
	}
	cmd.AddCommand(newNotebooksPushCommand(), newNotebooksPullCommand(), newNotebooksListCommand())
	return cmd
}

func newNotebooksPushCommand() *cobra.Command {
	var server, notebookID string
	cmd := &cobra.Command{
		Use:   "push <dir|index.yml>",
		Short: "Validate and upload a notebook bundle (private by default)",
		Long: `Validate and upload a notebook bundle, private by default.

Point at a single-family notebook dir (containing index.yml), or at a COMPOSITE
notebook: a parent dir whose subdirs are family buckets (definitions/, etymology/,
stories/, …) that share the same id: — the same on-disk shape a composite notebook
uses in the catalog. All families upload as ONE notebook under one server-minted id.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			files, meta, err := resolveNotebookBundle(args[0])
			if err != nil {
				return err
			}
			// Fail fast: validate locally before touching the server. A composite
			// bundle is already validated per-family inside resolveCompositeBundle.
			if meta.Kind != compositeBundleKind {
				if err := validateBundleLocally(files, meta); err != nil {
					return fmt.Errorf("bundle validation failed: %w", err)
				}
			}

			store, err := LoadCredentials()
			if err != nil {
				return err
			}
			target := store.ResolveServer(server)
			client := newNotebookClient(store, target)

			protoFiles := make([]*apiv1.NotebookFile, 0, len(files))
			for _, f := range files {
				protoFiles = append(protoFiles, &apiv1.NotebookFile{Path: f.RelPath, Content: f.Content})
			}
			resp, err := client.PushNotebook(cmd.Context(), connect.NewRequest(&apiv1.PushNotebookRequest{
				NotebookId:  notebookID,
				Kind:        meta.Kind,
				Name:        meta.Name,
				Files:       protoFiles,
				ContentHash: bundleContentHash(files),
			}))
			if err != nil {
				return fmt.Errorf("push notebook: %w", err)
			}
			fmt.Printf("Pushed notebook %s\n", resp.Msg.NotebookId)
			for table, n := range resp.Msg.ImportedRows {
				fmt.Printf("  imported %d %s\n", n, table)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&server, "server", "", "server base URL")
	cmd.Flags().StringVar(&notebookID, "id", "", "existing nb_ id to update (default: create a new notebook)")
	return cmd
}

func newNotebooksPullCommand() *cobra.Command {
	var server, outDir string
	cmd := &cobra.Command{
		Use:   "pull <nb_id>",
		Short: "Download a notebook's files to a directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := LoadCredentials()
			if err != nil {
				return err
			}
			target := store.ResolveServer(server)
			client := newNotebookClient(store, target)
			resp, err := client.PullNotebook(cmd.Context(), connect.NewRequest(&apiv1.PullNotebookRequest{
				NotebookId: args[0],
			}))
			if err != nil {
				return fmt.Errorf("pull notebook: %w", err)
			}
			dest := outDir
			if dest == "" {
				dest = args[0]
			}
			for _, f := range resp.Msg.Files {
				p := filepath.Join(dest, filepath.Clean("/"+f.Path))
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					return fmt.Errorf("mkdir %s: %w", p, err)
				}
				if err := os.WriteFile(p, f.Content, 0o644); err != nil {
					return fmt.Errorf("write %s: %w", p, err)
				}
			}
			fmt.Printf("Pulled %d file(s) to %s\n", len(resp.Msg.Files), dest)
			return nil
		},
	}
	cmd.Flags().StringVar(&server, "server", "", "server base URL")
	cmd.Flags().StringVar(&outDir, "out", "", "output directory (default: the notebook id)")
	return cmd
}

func newNotebooksListCommand() *cobra.Command {
	var server string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List your own notebooks on a server",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := LoadCredentials()
			if err != nil {
				return err
			}
			target := store.ResolveServer(server)
			client := newNotebookClient(store, target)
			resp, err := client.ListMyNotebooks(cmd.Context(), connect.NewRequest(&apiv1.ListMyNotebooksRequest{}))
			if err != nil {
				return fmt.Errorf("list notebooks: %w", err)
			}
			if len(resp.Msg.Notebooks) == 0 {
				fmt.Println("No notebooks yet. Push one with `langner notebooks push <dir>`.")
				return nil
			}
			entries := resp.Msg.Notebooks
			sort.Slice(entries, func(i, j int) bool { return entries[i].NotebookId < entries[j].NotebookId })
			for _, e := range entries {
				fmt.Printf("%s  %-12s  %-8s  %5dB  %s\n", e.NotebookId, e.Kind, e.Visibility, e.ByteSize, e.Name)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&server, "server", "", "server base URL")
	return cmd
}

// bundleContentHash mirrors notebook.HashBundle for the client-side hash the
// server re-verifies.
func bundleContentHash(files []bundleFile) string {
	nbFiles := make([]notebook.NotebookFile, 0, len(files))
	for _, f := range files {
		nbFiles = append(nbFiles, notebook.NotebookFile{Path: f.RelPath, Content: f.Content})
	}
	return notebook.HashBundle(nbFiles)
}
