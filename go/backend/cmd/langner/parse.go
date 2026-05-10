package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newParseCommand() *cobra.Command {
	parseCommand := &cobra.Command{
		Use: "parse",
	}
	parseCommand.AddCommand(&cobra.Command{
		Use: "friends <file>",
		RunE: func(cmd *cobra.Command, args []string) error {
			fileName := args[0]
			file, err := os.Open(fileName)
			if err != nil {
				return fmt.Errorf("os.Open() > %w", err)
			}
			defer func() {
				_ = file.Close()
			}()

			lines, err := io.ReadAll(file)
			if err != nil {
				return fmt.Errorf("io.ReadAll() > %w", err)
			}

			regexp, err := regexp.Compile(`^\[.*\]$`)
			if err != nil {
				return fmt.Errorf("regexp.Compile() > %w", err)
			}

			conversations := make([]notebook.Conversation, 0)
			for _, row := range strings.Split(string(lines), "\n") {
				if strings.TrimSpace(row) == "" {
					continue
				}
				if regexp.Match([]byte(row)) {
					continue
				}
				conversations = append(conversations, notebook.Conversation{
					Quote: row,
				})
			}
			storyNotebooks := make([]notebook.StoryNotebook, 0)
			storyNotebooks = append(storyNotebooks, notebook.StoryNotebook{
				Event: "Friends",
				Date:  time.Now(),
				Metadata: notebook.Metadata{
					Series:  "Friends",
					Season:  1,
					Episode: 1,
				},
				Scenes: []notebook.StoryScene{
					{
						Title:         "Opening",
						Conversations: conversations,
						Definitions: []notebook.Note{
							{
								Definition: "ran",
								Expression: "run",
								Meaning:    "to move quickly by foot",
								Synonyms:   []string{"race"},
							},
						},
					},
				},
			})

			notebook, err := yaml.Marshal(storyNotebooks)
			if err != nil {
				return fmt.Errorf("yaml.Marshal() > %w", err)
			}
			fmt.Printf("%s\n", string(notebook))

			return nil
		},
	})
	return parseCommand
}
