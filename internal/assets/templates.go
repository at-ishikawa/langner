package assets

import (
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed templates/story-notebook.md.go.tmpl
var fallbackStoryNotebookTemplate string

func ParseStoryTemplate(templatePath string) (*template.Template, error) {
	return parseTemplateWithFallback(templatePath, fallbackStoryNotebookTemplate)
}

func parseTemplateWithFallback(templatePath string, fallbackTemplate string) (*template.Template, error) {
	funcMap := template.FuncMap{
		"join": strings.Join,
	}

	// First, try to read from the filesystem
	if _, err := os.Stat(templatePath); err == nil {
		// File exists on filesystem, try to parse it
		fileName := filepath.Base(templatePath)
		tmpl, err := template.New(fileName).
			Funcs(funcMap).
			ParseFiles(templatePath)
		if err == nil {
			return tmpl, nil
		}
		// TODO: replace a logger with an argument
		slog.Default().Warn("failed to parse a templatePath",
			slog.String("templatePath", templatePath),
			slog.Any("error", err),
		)
	}

	// Fall back to embedded assets - use the embedded template's name
	fileName := "story-notebook.md.go.tmpl"
	tmpl, err := template.New(fileName).
		Funcs(funcMap).
		Parse(string(fallbackTemplate))
	if err != nil {
		return nil, fmt.Errorf("failed to parse embedded template: %w", err)
	}

	return tmpl, nil
}
