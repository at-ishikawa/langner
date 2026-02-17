package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/ebook"
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

	return ebookCmd
}

func newEbookManager() (*ebook.Manager, error) {
	loader, err := config.NewConfigLoader(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create config loader: %w", err)
	}
	cfg, err := loader.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}
	var booksDir string
	if len(cfg.Notebooks.BooksDirectories) > 0 {
		booksDir = cfg.Notebooks.BooksDirectories[0]
	}
	return ebook.NewManager(cfg.Books.RepoDirectory, cfg.Books.RepositoriesFile, booksDir), nil
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
