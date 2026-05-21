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
				key := etymologyOriginKey(nbID, title, o.Sense, o.Origin, o.Language)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				rows = append(rows, EtymologyOriginRecord{
					NotebookID:   nbID,
					SessionTitle: title,
					Sense:        o.Sense,
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

// EtymologyOriginFormForImport is one form declared on an origin in the
// YAML source. The (NotebookID, SessionTitle, Sense, Origin, Language)
// tuple resolves to an etymology_origins row at import time; that origin's
// ID becomes OriginID before insert. Sense matches the parent origin's
// Sense (empty for single-sense origins).
type EtymologyOriginFormForImport struct {
	NotebookID   string
	SessionTitle string
	Sense        string
	Origin       string
	Language     string
	Form         string
	Role         string
	Note         string
	SortOrder    int
}

// YAMLEtymologyOriginFormSource enumerates every form declared on every
// origin across every etymology session. Output is ordered by appearance
// in the YAML — callers can use the index as SortOrder.
type YAMLEtymologyOriginFormSource struct {
	reader *Reader
}

// NewYAMLEtymologyOriginFormSource constructs the source.
func NewYAMLEtymologyOriginFormSource(reader *Reader) *YAMLEtymologyOriginFormSource {
	return &YAMLEtymologyOriginFormSource{reader: reader}
}

// FindAll walks every etymology session file and emits one form row per
// (notebook_id, session_title, origin, language, role, form). Within an
// origin, duplicate (role, form) pairs are collapsed to mirror the DB's
// unique constraint.
func (s *YAMLEtymologyOriginFormSource) FindAll(_ context.Context) ([]EtymologyOriginFormForImport, error) {
	indexes := s.reader.GetEtymologyIndexes()
	var rows []EtymologyOriginFormForImport
	seen := make(map[string]struct{})

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
				continue
			}
			for _, o := range sf.Origins {
				for i, f := range o.Forms {
					key := nbID + "\x00" + title + "\x00" + o.Sense + "\x00" + o.Origin + "\x00" + o.Language + "\x00" + f.Role + "\x00" + f.Form
					if _, ok := seen[key]; ok {
						continue
					}
					seen[key] = struct{}{}
					rows = append(rows, EtymologyOriginFormForImport{
						NotebookID:   nbID,
						SessionTitle: title,
						Sense:        o.Sense,
						Origin:       o.Origin,
						Language:     o.Language,
						Form:         f.Form,
						Role:         f.Role,
						Note:         f.Note,
						SortOrder:    i,
					})
				}
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

// SemanticConceptForImport is one concept declaration parsed from a session
// file. ConceptKey is the book-level identity; multiple declarations of the
// same key across sessions in a book merge to a single DB row.
type SemanticConceptForImport struct {
	NotebookID   string
	SessionTitle string
	Key          string
	Meaning      string
	Note         string
	Members      []ConceptMember
}

// YAMLSemanticConceptSource enumerates every concept block across every
// etymology session file. Order follows the etymology indexes' iteration,
// with sessions read in NotebookPaths order — ingestion uses the first
// declaration of a (notebook, concept_key) pair as authoritative.
type YAMLSemanticConceptSource struct {
	reader *Reader
}

// NewYAMLSemanticConceptSource constructs the source.
func NewYAMLSemanticConceptSource(reader *Reader) *YAMLSemanticConceptSource {
	return &YAMLSemanticConceptSource{reader: reader}
}

// FindAll walks every etymology session file and emits one entry per
// concept declaration with its session title attached.
func (s *YAMLSemanticConceptSource) FindAll(_ context.Context) ([]SemanticConceptForImport, error) {
	indexes := s.reader.GetEtymologyIndexes()
	var rows []SemanticConceptForImport

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
				continue
			}
			for _, c := range sf.Concepts {
				if strings.TrimSpace(c.Key) == "" {
					continue
				}
				rows = append(rows, SemanticConceptForImport{
					NotebookID:   nbID,
					SessionTitle: title,
					Key:          c.Key,
					Meaning:      c.Meaning,
					Note:         c.Note,
					Members:      c.Members,
				})
			}
		}
	}
	return rows, nil
}

// ConceptRelationForImport is one relation declaration parsed from a session
// file, normalised for ingestion. Symmetric relations keep both endpoints
// in (FromKey, ToKey); the ingestion layer expands them to two DB rows.
type ConceptRelationForImport struct {
	NotebookID   string
	SessionTitle string
	Type         string
	FromKey      string
	ToKey        string
	IsDirected   bool
}

// YAMLConceptRelationSource enumerates every relation across every
// etymology session file in the book. Malformed relations are skipped
// (the validator already warned about them).
type YAMLConceptRelationSource struct {
	reader *Reader
}

// NewYAMLConceptRelationSource constructs the source.
func NewYAMLConceptRelationSource(reader *Reader) *YAMLConceptRelationSource {
	return &YAMLConceptRelationSource{reader: reader}
}

// FindAll walks every etymology session file and emits one entry per well-
// formed relation. The Type plus endpoints determine identity at the YAML
// layer; the importer resolves keys to concept IDs.
func (s *YAMLConceptRelationSource) FindAll(_ context.Context) ([]ConceptRelationForImport, error) {
	indexes := s.reader.GetEtymologyIndexes()
	var rows []ConceptRelationForImport

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
			for _, r := range sf.Relations {
				if strings.TrimSpace(r.Type) == "" {
					continue
				}
				a, b := r.Endpoints()
				if a == "" || b == "" {
					continue
				}
				rows = append(rows, ConceptRelationForImport{
					NotebookID:   nbID,
					SessionTitle: title,
					Type:         r.Type,
					FromKey:      a,
					ToKey:        b,
					IsDirected:   r.IsDirected(),
				})
			}
		}
	}
	return rows, nil
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
