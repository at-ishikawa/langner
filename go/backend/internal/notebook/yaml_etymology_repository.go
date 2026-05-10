package notebook

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// YAMLEtymologyOriginSource reads etymology origins from YAML session files
// and returns them as EtymologyOriginRecord values ready to compare against
// the DB's etymology_origins table. The session_title disambiguator comes
// from each session file's metadata.title (Phase 1 schema).
type YAMLEtymologyOriginSource struct {
	reader *Reader
}

// NewYAMLEtymologyOriginSource constructs the source.
func NewYAMLEtymologyOriginSource(reader *Reader) *YAMLEtymologyOriginSource {
	return &YAMLEtymologyOriginSource{reader: reader}
}

// FindAll walks every etymology notebook + session file and returns one
// EtymologyOriginRecord per (notebook_id, session_title, origin, language).
// IDs are zero — the importer fills them in after a successful insert.
//
// Within a single session, duplicate (origin, language) pairs collapse via
// originRecordKey so the YAML side mirrors the DB's unique constraint.
func (s *YAMLEtymologyOriginSource) FindAll(_ context.Context) ([]EtymologyOriginRecord, error) {
	indexes := s.reader.GetEtymologyIndexes()
	seen := make(map[string]struct{})
	var rows []EtymologyOriginRecord

	for nbID, idx := range indexes {
		for _, nbPath := range idx.NotebookPaths {
			path := filepath.Join(idx.Path, nbPath)
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("read etymology session %s: %w", path, err)
			}
			var sf etymologySessionFile
			if err := yaml.Unmarshal(data, &sf); err != nil {
				return nil, fmt.Errorf("parse etymology session %s: %w", path, err)
			}
			title := strings.TrimSpace(sf.Metadata.Title)
			if title == "" {
				return nil, fmt.Errorf("etymology session %s missing required metadata.title", path)
			}
			for _, o := range sf.Origins {
				key := etymologyOriginKey(nbID, title, o.Origin, o.Language)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				rows = append(rows, EtymologyOriginRecord{
					NotebookID:   nbID,
					SessionTitle: title,
					Origin:       o.Origin,
					Type:         o.Type,
					Language:     o.Language,
					Meaning:      o.Meaning,
				})
			}
		}
	}
	return rows, nil
}

// YAMLEtymologyDefinitionSource emits every etymology-bearing definition
// (story, flashcard, definitions-only, and etymology-session-embedded)
// with its parent session title attached for origin-part binding.
type YAMLEtymologyDefinitionSource struct {
	reader *Reader
}

// NewYAMLEtymologyDefinitionSource constructs the source.
func NewYAMLEtymologyDefinitionSource(reader *Reader) *YAMLEtymologyDefinitionSource {
	return &YAMLEtymologyDefinitionSource{reader: reader}
}

// EtymologyDefinitionForImport carries everything ImportNoteOriginParts
// needs to resolve a definition's origin_parts to (note_id, origin_id, sort).
type EtymologyDefinitionForImport struct {
	NotebookID   string          // ID of the *definitions* notebook (matches DB notebook_id for join lookups)
	SessionTitle string          // metadata.title used for origin sense binding
	Expression   string          // Note.Expression (for note_id lookup)
	OriginParts  []OriginPartRef // ordered origin references
}

// FindAll returns every definition in the user's data that carries
// origin_parts. SessionTitle is filled from the parent metadata.title;
// rows without it are skipped because they can't be sense-bound.
func (s *YAMLEtymologyDefinitionSource) FindAll(_ context.Context) ([]EtymologyDefinitionForImport, error) {
	var out []EtymologyDefinitionForImport

	// Definitions notebook (the canonical home for vocabulary that
	// references etymology origins). Its YAML is parsed via Reader's
	// raw definitions cache because that's the only path that preserves
	// the parent metadata.title needed for sense binding.
	for _, bookID := range s.reader.GetDefinitionsBookIDs() {
		raw, ok := s.reader.GetDefinitionsBook(bookID)
		if !ok {
			continue
		}
		for _, def := range raw {
			title := strings.TrimSpace(def.Metadata.Title)
			if title == "" {
				continue
			}
			for _, scene := range def.Scenes {
				for _, note := range scene.Expressions {
					if len(note.OriginParts) == 0 {
						continue
					}
					expr := note.Expression
					if expr == "" {
						expr = note.Definition
					}
					if expr == "" {
						continue
					}
					out = append(out, EtymologyDefinitionForImport{
						NotebookID:   bookID,
						SessionTitle: title,
						Expression:   expr,
						OriginParts:  note.OriginParts,
					})
				}
			}
		}
	}

	// Etymology-session-embedded definitions carry their session via
	// SessionTitle (set by Phase 1's reader).
	for _, def := range s.reader.ReadAllEtymologyDefinitions() {
		if len(def.OriginParts) == 0 {
			continue
		}
		title := strings.TrimSpace(def.SessionTitle)
		if title == "" {
			continue
		}
		out = append(out, EtymologyDefinitionForImport{
			NotebookID:   def.NotebookName,
			SessionTitle: title,
			Expression:   def.GetExpression(),
			OriginParts:  def.OriginParts,
		})
	}

	return out, nil
}
