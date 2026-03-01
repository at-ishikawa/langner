package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v3"

	"github.com/at-ishikawa/langner/internal/ebook"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/spf13/cobra"
)

func newEbookCommand() *cobra.Command {
	ebookCmd := &cobra.Command{
		Use:   "ebook",
		Short: "Manage ebook repositories",
	}

	ebookCmd.AddCommand(newEbookCloneCommand())
	ebookCmd.AddCommand(newEbookListCommand())
	ebookCmd.AddCommand(newEbookRemoveCommand())
	ebookCmd.AddCommand(newEbookScenesCommand())
	ebookCmd.AddCommand(newEbookSearchCommand())
	ebookCmd.AddCommand(newEbookFixDefinitionsCommand())

	return ebookCmd
}

func newEbookManager() (*ebook.Manager, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	var booksDir string
	if len(cfg.Notebooks.BooksDirectories) > 0 {
		booksDir = cfg.Notebooks.BooksDirectories[0]
	}
	return ebook.NewManager(cfg.Books.RepoDirectory, cfg.Books.RepositoriesFile, booksDir), nil
}

// truncate returns s truncated to n characters with "..." appended if longer
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func newEbookCloneCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "clone <url>",
		Short: "Clone a Standard Ebooks repository",
		Long:  "Clone a Standard Ebooks repository from GitHub or standardebooks.org URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := newEbookManager()
			if err != nil {
				return err
			}
			if err := manager.Clone(args[0]); err != nil {
				return fmt.Errorf("failed to clone ebook: %w", err)
			}

			return nil
		},
	}
}

func newEbookListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List cloned ebook repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := newEbookManager()
			if err != nil {
				return err
			}
			repos, err := manager.List()
			if err != nil {
				return fmt.Errorf("failed to list ebooks: %w", err)
			}

			if len(repos) == 0 {
				fmt.Println("No ebooks cloned yet. Use 'langner ebook clone <url>' to clone an ebook.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "ID\tTITLE\tAUTHOR\tPATH")
			for _, repo := range repos {
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", repo.ID, repo.Title, repo.Author, repo.RepoPath)
			}
			_ = w.Flush()

			return nil
		},
	}
}

func newEbookRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove a cloned ebook repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := newEbookManager()
			if err != nil {
				return err
			}
			if err := manager.Remove(args[0]); err != nil {
				return fmt.Errorf("failed to remove ebook: %w", err)
			}

			return nil
		},
	}
}

func newEbookScenesCommand() *cobra.Command {
	var notebookFilter string

	cmd := &cobra.Command{
		Use:   "scenes <book-id>",
		Short: "List scenes with their indices for a book",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := newEbookManager()
			if err != nil {
				return err
			}

			bookNotebooks, err := manager.GetBookNotebooks(args[0])
			if err != nil {
				return fmt.Errorf("get book notebooks: %w", err)
			}

			for _, nb := range bookNotebooks {
				if notebookFilter != "" && nb.Path != notebookFilter {
					continue
				}
				for _, entry := range nb.Entries {
					fmt.Printf("\n%s (event: %s)\n", nb.Path, entry.Event)
					for i, scene := range entry.Scenes {
						snippet := ""
						if len(scene.Statements) > 0 {
							snippet = truncate(scene.Statements[0], 80)
						}
						fmt.Printf("  [%d] %s\n", i, snippet)
					}
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&notebookFilter, "notebook", "", "Show only this notebook file (e.g. 005-letter-1.yml)")
	return cmd
}

func newEbookSearchCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "search <book-id> <word>",
		Short: "Search for a word in a book to find its notebook file and scene index",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := newEbookManager()
			if err != nil {
				return err
			}

			bookID, word := args[0], args[1]
			bookNotebooks, err := manager.GetBookNotebooks(bookID)
			if err != nil {
				return fmt.Errorf("get book notebooks: %w", err)
			}

			lowerWord := strings.ToLower(word)
			found := false
			for _, nb := range bookNotebooks {
				for _, entry := range nb.Entries {
					for i, scene := range entry.Scenes {
						for _, stmt := range scene.Statements {
							if !strings.Contains(strings.ToLower(stmt), lowerWord) {
								continue
							}
							found = true
							fmt.Printf("notebook: %s, scene: %d\n  %s\n", nb.Path, i, truncate(stmt, 120))
						}
					}
				}
			}

			if !found {
				fmt.Printf("Word %q not found in book %q\n", word, bookID)
			}
			return nil
		},
	}
}

func newEbookFixDefinitionsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "fix-definitions <book-id>",
		Short: "Auto-update scene indices in definitions files by searching for expressions in the book text",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			bookID := args[0]

			manager, err := newEbookManager()
			if err != nil {
				return err
			}

			bookNotebooks, err := manager.GetBookNotebooks(bookID)
			if err != nil {
				return fmt.Errorf("get book notebooks: %w", err)
			}

			// Build map: notebook path -> scenes (use first entry per file)
			notebookScenesMap := make(map[string][]ebook.NotebookScene)
			for _, nb := range bookNotebooks {
				if len(nb.Entries) > 0 {
					notebookScenesMap[nb.Path] = nb.Entries[0].Scenes
				}
			}

			if len(cfg.Notebooks.DefinitionsDirectories) == 0 {
				return fmt.Errorf("no definitions directories configured")
			}

			definitionsPath := filepath.Join(cfg.Notebooks.DefinitionsDirectories[0], bookID+".yml")
			data, err := os.ReadFile(definitionsPath)
			if err != nil {
				return fmt.Errorf("read definitions file: %w", err)
			}

			var definitions []notebook.Definitions
			if err := yaml.Unmarshal(data, &definitions); err != nil {
				return fmt.Errorf("parse definitions file: %w", err)
			}

			changed := 0
			for di := range definitions {
				notebookPath := definitions[di].Metadata.Notebook
				if notebookPath == "" {
					continue
				}

				scenes, ok := notebookScenesMap[notebookPath]
				if !ok {
					fmt.Printf("Warning: notebook %q not found in book %q\n", notebookPath, bookID)
					continue
				}

				for si := range definitions[di].Scenes {
					defScene := &definitions[di].Scenes[si]
					if len(defScene.Expressions) == 0 {
						continue
					}

					expression := strings.ToLower(strings.TrimSpace(defScene.Expressions[0].Expression))
					foundIndex := -1
					for sceneIdx, scene := range scenes {
						for _, stmt := range scene.Statements {
							if strings.Contains(strings.ToLower(stmt), expression) {
								foundIndex = sceneIdx
								break
							}
						}
						if foundIndex >= 0 {
							break
						}
					}

					if foundIndex < 0 {
						fmt.Printf("Warning: expression %q not found in notebook %q\n", defScene.Expressions[0].Expression, notebookPath)
						continue
					}

					currentIndex := defScene.Metadata.GetIndex()
					if currentIndex == foundIndex {
						continue
					}

					fmt.Printf("Updating %q in %q: index %d -> %d\n", defScene.Expressions[0].Expression, notebookPath, currentIndex, foundIndex)
					defScene.Metadata.Index = foundIndex
					defScene.Metadata.Scene = nil
					changed++
				}
			}

			if changed == 0 {
				fmt.Println("No changes needed.")
				return nil
			}

			updatedData, err := yaml.Marshal(definitions)
			if err != nil {
				return fmt.Errorf("marshal definitions: %w", err)
			}

			if err := os.WriteFile(definitionsPath, updatedData, 0644); err != nil {
				return fmt.Errorf("write definitions file: %w", err)
			}

			fmt.Printf("Updated %d scene index(es) in %s\n", changed, definitionsPath)
			return nil
		},
	}
}
