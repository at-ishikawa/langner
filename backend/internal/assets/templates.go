package assets

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

func parseTemplateWithFallback(templatePath string, fallbackTemplate string) (*template.Template, error) {
	funcMap := template.FuncMap{
		"join": strings.Join,
	}

	// If template path is empty, use fallback directly
	if templatePath == "" {
		fileName := "story-notebook.md.go.tmpl"
		tmpl, err := template.New(fileName).
			Funcs(funcMap).
			Parse(string(fallbackTemplate))
		if err != nil {
			return nil, fmt.Errorf("failed to parse embedded template: %w", err)
		}
		return tmpl, nil
	}

	// If template path is provided, it must be valid.
	if _, err := os.Stat(templatePath); err != nil {
		return nil, fmt.Errorf("template file not found or accessible: %w", err)
	}

	fileName := filepath.Base(templatePath)
	tmpl, err := template.New(fileName).
		Funcs(funcMap).
		ParseFiles(templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template file %s: %w", templatePath, err)
	}
	return tmpl, nil
}
