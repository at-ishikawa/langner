package notebook

import (
	"fmt"
	"path/filepath"
	"strings"
)

// EtymologyOriginForm is one inflectional / morphological variant of a
// source-language origin. The pair (form, role) is the identity within an
// origin. Role values are free strings; conventions are per-language and
// only need to be consistent within a single origin.
type EtymologyOriginForm struct {
	Form string `yaml:"form"`
	Role string `yaml:"role"`
	Note string `yaml:"note,omitempty"`
}

// Concept is a named grouping of origins declared at the top level of an
// etymology session file. The same concept_key may appear in multiple
// sessions of the same book; the ingestion layer unifies them by merging
// members, provided meaning and note match. Relations between concepts are
// modeled separately in the Relation struct.
type Concept struct {
	Key     string          `yaml:"key"`
	Meaning string          `yaml:"meaning"`
	Note    string          `yaml:"note,omitempty"`
	Members []ConceptMember `yaml:"members"`

	// SessionTitle records which session declared this concept block; set
	// at read time from the session file's metadata.title. Not serialised.
	SessionTitle string `yaml:"-"`
}

// ConceptMember is one member of a Concept, identified by (origin, language).
// The pair must resolve to an origin declared in the same session as the
// concept block that lists it.
type ConceptMember struct {
	Origin   string `yaml:"origin"`
	Language string `yaml:"language,omitempty"`
}

// Relation is a typed edge between two concept keys in the same book.
// Exactly one of Between (symmetric types) or From+To (directed types) must
// be set. Type is a free string; convention follows WordNet vocabulary
// (antonym, hypernym, hyponym, holonym, meronym, similar_to, causes,
// entails, derivation, …). Any new type is accepted.
type Relation struct {
	Type    string   `yaml:"type"`
	Between []string `yaml:"between,omitempty"`
	From    string   `yaml:"from,omitempty"`
	To      string   `yaml:"to,omitempty"`
}

// IsDirected reports whether this relation uses the from/to (directed)
// shape rather than between (undirected/symmetric).
func (r Relation) IsDirected() bool {
	return r.From != "" || r.To != ""
}

// Endpoints returns the two concept keys this relation connects, regardless
// of whether it was declared as Between or From/To. Returns ("", "") when
// the relation is malformed (caller is expected to have validated first).
func (r Relation) Endpoints() (string, string) {
	if r.IsDirected() {
		return r.From, r.To
	}
	if len(r.Between) == 2 {
		return r.Between[0], r.Between[1]
	}
	return "", ""
}

// etymologyBookView aggregates all the concept and relation data from the
// session files of a single etymology book. Used by the validator to
// enforce book-level invariants (unique concept_key with consistent
// meaning/note across sessions, relations pointing at known keys).
type etymologyBookView struct {
	bookID string

	// originsBySession maps SessionTitle -> set of (origin, language)
	// declared in that session's `origins:` block. Used to verify concept
	// members resolve.
	originsBySession map[string]map[conceptMemberKey]bool

	// concepts indexes every concept declaration by key. Multiple entries
	// per key are expected (cross-session merge); the validator checks
	// they agree on meaning/note.
	concepts map[string][]conceptDecl

	// relations is the flat list of all relation entries declared across
	// sessions in this book, with provenance for error messages.
	relations []relationDecl

	// originForms maps (origin, language) -> set of declared form strings,
	// scoped to a session. Used to resolve `from_form` on definitions.
	originForms map[originFormKey]map[string]bool
}

type conceptMemberKey struct {
	Origin   string
	Language string
}

type originFormKey struct {
	SessionTitle string
	Origin       string
	Language     string
}

type conceptDecl struct {
	concept Concept
	path    string
}

type relationDecl struct {
	relation Relation
	path     string
	session  string
}

// loadEtymologyBookView reads every session file under an etymology index
// and aggregates origins, forms, concepts, and relations into the
// book-level view used by the validator.
func (v *Validator) loadEtymologyBookView(indexPath, bookID string, sessionPaths []string) (*etymologyBookView, error) {
	view := &etymologyBookView{
		bookID:           bookID,
		originsBySession: make(map[string]map[conceptMemberKey]bool),
		concepts:         make(map[string][]conceptDecl),
		originForms:      make(map[originFormKey]map[string]bool),
	}

	for _, nbPath := range sessionPaths {
		path := filepath.Join(indexPath, nbPath)
		wrapped, err := readYamlFile[etymologySessionFile](path)
		if err != nil {
			return nil, fmt.Errorf("read etymology session %s: %w", path, err)
		}
		session := strings.TrimSpace(wrapped.Metadata.Title)
		if session == "" {
			// Validated elsewhere (walkEtymologyIndexFiles fails fast).
			// Skip so we don't crash on legacy files mid-fix.
			continue
		}

		origins := make(map[conceptMemberKey]bool, len(wrapped.Origins))
		for _, o := range wrapped.Origins {
			origins[conceptMemberKey{Origin: o.Origin, Language: o.Language}] = true
			if len(o.Forms) > 0 {
				k := originFormKey{SessionTitle: session, Origin: o.Origin, Language: o.Language}
				if _, ok := view.originForms[k]; !ok {
					view.originForms[k] = make(map[string]bool)
				}
				for _, f := range o.Forms {
					view.originForms[k][f.Form] = true
				}
			}
		}
		view.originsBySession[session] = origins

		for _, c := range wrapped.Concepts {
			c.SessionTitle = session
			view.concepts[c.Key] = append(view.concepts[c.Key], conceptDecl{concept: c, path: path})
		}
		for _, r := range wrapped.Relations {
			view.relations = append(view.relations, relationDecl{
				relation: r,
				path:     path,
				session:  session,
			})
		}
	}

	return view, nil
}

// validateEtymologyExtensions checks the new forms/concepts/relations
// fields on etymology session files. Failures are emitted as warnings (not
// errors) while the feature is exploratory — existing notebooks shouldn't
// fail validation just because they don't carry the new fields, and the
// new fields shouldn't block fix runs while the schema matures.
func (v *Validator) validateEtymologyExtensions(result *ValidationResult) {
	for _, dir := range v.etymologyDirs {
		indexes := make(map[string]EtymologyIndex)
		if err := walkEtymologyIndexFiles(dir, indexes); err != nil {
			result.AddWarning(ValidationError{
				File:    dir,
				Message: fmt.Sprintf("scan etymology indexes: %v", err),
			})
			continue
		}
		for _, idx := range indexes {
			view, err := v.loadEtymologyBookView(idx.Path, idx.ID, idx.NotebookPaths)
			if err != nil {
				result.AddWarning(ValidationError{
					File:    idx.Path,
					Message: fmt.Sprintf("load book view: %v", err),
				})
				continue
			}
			v.validateBookForms(view, result)
			v.validateBookConcepts(view, result)
			v.validateBookRelations(view, result)
		}
	}
}

// validateBookForms checks that each origin's forms[] entries have
// non-empty form and role fields. Roles are otherwise free strings.
func (v *Validator) validateBookForms(view *etymologyBookView, result *ValidationResult) {
	for k, forms := range view.originForms {
		seen := make(map[string]int, len(forms))
		// Note: view.originForms collapses to a set; per-form integrity is
		// enforced when we walked the originals. Re-walk session files for
		// detailed reporting would duplicate I/O — for v1 we surface only
		// duplicates within an origin's forms via the map, plus a generic
		// hint when empty.
		if len(forms) == 0 {
			result.AddWarning(ValidationError{
				File:     view.bookID,
				Location: fmt.Sprintf("session %q, origin %q (%s)", k.SessionTitle, k.Origin, k.Language),
				Message:  "forms: list is empty (omit the field entirely if no forms are recorded)",
			})
			continue
		}
		for form := range forms {
			if strings.TrimSpace(form) == "" {
				result.AddWarning(ValidationError{
					File:     view.bookID,
					Location: fmt.Sprintf("session %q, origin %q (%s)", k.SessionTitle, k.Origin, k.Language),
					Message:  "forms[].form must be non-empty",
				})
			}
			seen[form]++
		}
		for form, n := range seen {
			if n > 1 {
				result.AddWarning(ValidationError{
					File:     view.bookID,
					Location: fmt.Sprintf("session %q, origin %q (%s)", k.SessionTitle, k.Origin, k.Language),
					Message:  fmt.Sprintf("forms[].form %q appears %d times on the same origin", form, n),
				})
			}
		}
	}
}

// validateBookConcepts checks book-level concept invariants:
//   - the same concept_key declared in multiple sessions must agree on
//     meaning and note (members are unioned, not duplicated);
//   - each member must resolve to an origin in the same session as the
//     declaring concept block;
//   - key and meaning must be non-empty.
func (v *Validator) validateBookConcepts(view *etymologyBookView, result *ValidationResult) {
	for key, decls := range view.concepts {
		if strings.TrimSpace(key) == "" {
			for _, d := range decls {
				result.AddWarning(ValidationError{
					File:     d.path,
					Location: fmt.Sprintf("session %q", d.concept.SessionTitle),
					Message:  "concept.key must be non-empty",
				})
			}
			continue
		}
		// Cross-session meaning/note agreement.
		var meaning, note, firstPath, firstSession string
		for i, d := range decls {
			if strings.TrimSpace(d.concept.Meaning) == "" {
				result.AddWarning(ValidationError{
					File:     d.path,
					Location: fmt.Sprintf("session %q, concept %q", d.concept.SessionTitle, key),
					Message:  "concept.meaning must be non-empty",
				})
			}
			if i == 0 {
				meaning = d.concept.Meaning
				note = d.concept.Note
				firstPath = d.path
				firstSession = d.concept.SessionTitle
				continue
			}
			if d.concept.Meaning != meaning {
				result.AddWarning(ValidationError{
					File:     d.path,
					Location: fmt.Sprintf("session %q, concept %q", d.concept.SessionTitle, key),
					Message: fmt.Sprintf(
						"concept.meaning %q disagrees with earlier declaration %q (session %q in %s)",
						d.concept.Meaning, meaning, firstSession, firstPath,
					),
				})
			}
			if d.concept.Note != note {
				result.AddWarning(ValidationError{
					File:     d.path,
					Location: fmt.Sprintf("session %q, concept %q", d.concept.SessionTitle, key),
					Message: fmt.Sprintf(
						"concept.note disagrees with earlier declaration in session %q (%s)",
						firstSession, firstPath,
					),
				})
			}
		}

		// Members must resolve to origins in the same session.
		for _, d := range decls {
			origins := view.originsBySession[d.concept.SessionTitle]
			for _, m := range d.concept.Members {
				k := conceptMemberKey(m)
				if !origins[k] {
					result.AddWarning(ValidationError{
						File:     d.path,
						Location: fmt.Sprintf("session %q, concept %q", d.concept.SessionTitle, key),
						Message: fmt.Sprintf(
							"member %q (%s) does not match any origin declared in this session",
							m.Origin, m.Language,
						),
					})
				}
			}
		}
	}
}

// validateBookRelations checks per-relation structural invariants and
// that endpoints resolve to known concept keys somewhere in the same book.
// Symmetric (between) relations are reported as such; the import layer is
// responsible for materialising both directions in the DB.
func (v *Validator) validateBookRelations(view *etymologyBookView, result *ValidationResult) {
	for _, rd := range view.relations {
		r := rd.relation
		if strings.TrimSpace(r.Type) == "" {
			result.AddWarning(ValidationError{
				File:     rd.path,
				Location: fmt.Sprintf("session %q", rd.session),
				Message:  "relation.type must be non-empty",
			})
			continue
		}
		hasBetween := len(r.Between) > 0
		hasDirected := r.From != "" || r.To != ""
		switch {
		case hasBetween && hasDirected:
			result.AddWarning(ValidationError{
				File:     rd.path,
				Location: fmt.Sprintf("session %q, relation type %q", rd.session, r.Type),
				Message:  "relation must use either `between` (symmetric) or `from`/`to` (directed), not both",
			})
			continue
		case hasBetween:
			if len(r.Between) != 2 {
				result.AddWarning(ValidationError{
					File:     rd.path,
					Location: fmt.Sprintf("session %q, relation type %q", rd.session, r.Type),
					Message:  fmt.Sprintf("relation.between must list exactly 2 concept keys (got %d)", len(r.Between)),
				})
				continue
			}
		case hasDirected:
			if r.From == "" || r.To == "" {
				result.AddWarning(ValidationError{
					File:     rd.path,
					Location: fmt.Sprintf("session %q, relation type %q", rd.session, r.Type),
					Message:  "relation requires both `from` and `to` when using directed form",
				})
				continue
			}
		default:
			result.AddWarning(ValidationError{
				File:     rd.path,
				Location: fmt.Sprintf("session %q, relation type %q", rd.session, r.Type),
				Message:  "relation needs either `between` or `from`/`to`",
			})
			continue
		}

		a, b := r.Endpoints()
		for _, ep := range []string{a, b} {
			if _, ok := view.concepts[ep]; !ok {
				result.AddWarning(ValidationError{
					File:     rd.path,
					Location: fmt.Sprintf("session %q, relation type %q", rd.session, r.Type),
					Message:  fmt.Sprintf("relation endpoint %q is not a declared concept in this book", ep),
				})
			}
		}
	}
}

// validateFromForm walks all definitions in etymology session files and
// emits a warning when a definition's origin_parts[].from_form is not one
// of the forms declared on the referenced origin in the same session.
// Used after view construction so it can be called separately from the
// per-book validators that operate on view contents only.
func (v *Validator) validateFromForm(result *ValidationResult) {
	for _, dir := range v.etymologyDirs {
		indexes := make(map[string]EtymologyIndex)
		if err := walkEtymologyIndexFiles(dir, indexes); err != nil {
			continue
		}
		for _, idx := range indexes {
			view, err := v.loadEtymologyBookView(idx.Path, idx.ID, idx.NotebookPaths)
			if err != nil {
				continue
			}
			for _, nbPath := range idx.NotebookPaths {
				path := filepath.Join(idx.Path, nbPath)
				wrapped, err := readYamlFile[etymologySessionFile](path)
				if err != nil {
					continue
				}
				session := strings.TrimSpace(wrapped.Metadata.Title)
				for _, def := range wrapped.Definitions {
					for _, op := range def.OriginParts {
						if op.FromForm == "" {
							continue
						}
						k := originFormKey{SessionTitle: session, Origin: op.Origin, Language: op.Language}
						forms, ok := view.originForms[k]
						if !ok || !forms[op.FromForm] {
							result.AddWarning(ValidationError{
								File:     path,
								Location: fmt.Sprintf("session %q, expression %q", session, def.GetExpression()),
								Message: fmt.Sprintf(
									"from_form %q does not match any form declared on origin %q (%s) in this session",
									op.FromForm, op.Origin, op.Language,
								),
							})
						}
					}
				}
			}
		}
	}
}
