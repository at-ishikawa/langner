package notebook

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/database"
)

// ContentRepository stores notebook content in the content_* tables
// (migration 038) and reads it back. It is the server's ContentLoader.
type ContentRepository struct {
	db *sqlx.DB
}

// NewContentRepository constructs the repository.
func NewContentRepository(db *sqlx.DB) *ContentRepository {
	return &ContentRepository{db: db}
}

type fileKey struct {
	notebook, family string
	file             int
}

type entryKey struct {
	fileKey
	entry int
}

type sceneKey struct {
	entryKey
	scene int
}

type itemKey struct {
	sceneKey
	item int
}

// contentRows collects the rows of one or more notebooks, table by table.
type contentRows map[string][][]any

func (rows contentRows) add(table string, values ...any) {
	rows[table] = append(rows[table], values)
}

var contentColumns = map[string][]string{
	"content_indexes":                {"notebook_id", "family", "kind", "name"},
	"content_files":                  {"notebook_id", "family", "file_no", "path", "shape"},
	"content_entries":                {"notebook_id", "family", "file_no", "entry_no", "title", "session_key", "description", "series", "season", "episode", "date"},
	"content_scenes":                 {"notebook_id", "family", "file_no", "entry_no", "scene_no", "title", "type", "scene_index", "scene_number"},
	"content_lines":                  {"notebook_id", "family", "file_no", "entry_no", "scene_no", "line_no", "speaker", "text"},
	"content_notes":                  {"notebook_id", "family", "file_no", "entry_no", "scene_no", "note_no", "sense_id", "expression", "definition", "level", "meaning", "origin", "part_of_speech", "pronunciation", "memo", "note", "reference", "youtube_url", "youtube_time_seconds", "dictionary_number", "not_used", "images", "synonyms", "antonyms", "links"},
	"content_note_examples":          {"notebook_id", "family", "file_no", "entry_no", "scene_no", "note_no", "item_no", "text", "highlight"},
	"content_note_origin_parts":      {"notebook_id", "family", "file_no", "entry_no", "scene_no", "note_no", "item_no", "origin", "language", "sense", "from_form"},
	"content_note_phrases":           {"notebook_id", "family", "file_no", "entry_no", "scene_no", "note_no", "item_no", "actor", "remarks"},
	"content_note_references":        {"notebook_id", "family", "file_no", "entry_no", "scene_no", "note_no", "item_no", "url", "description"},
	"content_origins":                {"notebook_id", "family", "file_no", "entry_no", "scene_no", "origin_no", "origin", "type", "language", "meaning", "sense", "note", "english_forms"},
	"content_origin_forms":           {"notebook_id", "family", "file_no", "entry_no", "scene_no", "origin_no", "item_no", "form", "role", "note"},
	"content_definition_concepts":    {"notebook_id", "family", "file_no", "entry_no", "concept_no", "head", "meaning", "kind", "expressions"},
	"content_origin_concepts":        {"notebook_id", "family", "file_no", "entry_no", "concept_no", "key", "meaning", "note"},
	"content_origin_concept_members": {"notebook_id", "family", "file_no", "entry_no", "concept_no", "item_no", "origin", "language"},
	"content_relations":              {"notebook_id", "family", "file_no", "entry_no", "relation_no", "type", "between_keys", "from_key", "to_key"},
	"content_corrections":            {"notebook_id", "family", "file_no", "entry_no", "scene_no", "correction_no", "correction_id", "incorrect", "correct", "category", "reason"},
}

// contentTables lists the tables parents first, the order rows are inserted.
var contentTables = []string{
	"content_indexes", "content_files", "content_entries", "content_scenes",
	"content_lines", "content_notes", "content_note_examples", "content_note_origin_parts",
	"content_note_phrases", "content_note_references", "content_origins", "content_origin_forms",
	"content_definition_concepts", "content_origin_concepts", "content_origin_concept_members",
	"content_relations", "content_corrections",
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func nullInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func textArray(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// Store replaces a notebook's content with nc, inside tx. The notebooks row
// must exist.
func (r *ContentRepository) Store(ctx context.Context, tx *sqlx.Tx, nc NotebookContent) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM content_indexes WHERE notebook_id = $1`, nc.ID); err != nil {
		return fmt.Errorf("clear content of %s: %w", nc.ID, err)
	}
	rows := make(contentRows)
	nb := nc.ID
	for _, fc := range nc.Families {
		fam := fc.Family
		rows.add("content_indexes", nb, fam, fc.Kind, fc.Name)
		for fi, f := range fc.Files {
			rows.add("content_files", nb, fam, fi, f.Path, f.Shape)
			addFileRows(rows, fileKey{nb, fam, fi}, f)
		}
	}
	for _, table := range contentTables {
		if err := insertRows(ctx, tx, table, contentColumns[table], rows[table]); err != nil {
			return fmt.Errorf("store %s rows of %s: %w", table, nb, err)
		}
	}
	return nil
}

func insertRows(ctx context.Context, tx *sqlx.Tx, table string, columns []string, rows [][]any) error {
	perBatch := 65535 / len(columns)
	for start := 0; start < len(rows); start += perBatch {
		end := min(start+perBatch, len(rows))
		args := make([]any, 0, (end-start)*len(columns))
		for _, row := range rows[start:end] {
			args = append(args, row...)
		}
		if _, err := tx.ExecContext(ctx, database.BuildMultiRowInsert(table, columns, end-start), args...); err != nil {
			return err
		}
	}
	return nil
}

func addFileRows(rows contentRows, fk fileKey, f ContentFile) {
	entry := func(i int, title, sessionKey, description string, meta Metadata, date time.Time) entryKey {
		rows.add("content_entries", fk.notebook, fk.family, fk.file, i, title, sessionKey, description,
			meta.Series, meta.Season, meta.Episode, nullTime(date))
		return entryKey{fk, i}
	}
	scene := func(ek entryKey, i int, title, typ string, index int, number *int) sceneKey {
		rows.add("content_scenes", ek.notebook, ek.family, ek.file, ek.entry, i, title, typ, index, nullInt(number))
		return sceneKey{ek, i}
	}
	switch f.Shape {
	case ShapeStory:
		for ei, nb := range f.Stories {
			ek := entry(ei, nb.Event, "", "", nb.Metadata, nb.Date)
			for si, s := range nb.Scenes {
				sk := scene(ek, si, s.Title, s.Type, 0, nil)
				line := 0
				for _, c := range s.Conversations {
					addLine(rows, sk, line, c.Speaker, c.Quote)
					line++
				}
				for _, st := range s.Statements {
					addLine(rows, sk, line, nil, st)
					line++
				}
				for ni, n := range s.Definitions {
					addNoteRows(rows, itemKey{sk, ni}, n)
				}
			}
		}
	case ShapeFlashcard:
		for ei, deck := range f.Flashcards {
			ek := entry(ei, deck.Title, "", deck.Description, Metadata{}, deck.Date)
			sk := scene(ek, 0, "", "", 0, nil)
			for ni, n := range deck.Cards {
				addNoteRows(rows, itemKey{sk, ni}, n)
			}
		}
	case ShapeDefinitions:
		for ei, def := range f.Definitions {
			ek := entry(ei, def.Metadata.Title, def.Metadata.Notebook, "", Metadata{}, def.Metadata.Date)
			for si, s := range def.Scenes {
				sk := scene(ek, si, s.Metadata.Title, "", s.Metadata.Index, s.Metadata.Scene)
				for ni, n := range s.Expressions {
					addNoteRows(rows, itemKey{sk, ni}, n)
				}
			}
			for ci, c := range def.Concepts {
				rows.add("content_definition_concepts", ek.notebook, ek.family, ek.file, ek.entry, ci,
					c.Head, c.Meaning, string(c.Kind), textArray(c.Expressions))
			}
		}
	case ShapeEtymology:
		for ei, e := range f.Etymology {
			ek := entry(ei, e.Event, "", "", Metadata{}, e.Date)
			for si, s := range e.Scenes {
				sk := scene(ek, si, s.Scene, "", 0, nil)
				for oi, o := range s.Origins {
					addOriginRows(rows, itemKey{sk, oi}, o)
				}
			}
			addEtymologyGraphRows(rows, ek, e.Concepts, e.Relations)
		}
	case ShapeEtymologyLegacy:
		l := f.EtymologyLegacy
		if l == nil {
			return
		}
		ek := entry(0, l.Metadata.Title, "", "", Metadata{}, l.Date)
		sk := scene(ek, 0, "", "", 0, nil)
		for oi, o := range l.Origins {
			addOriginRows(rows, itemKey{sk, oi}, o)
		}
		for ni, d := range l.Definitions {
			addNoteRows(rows, itemKey{sk, ni}, Note{
				ID:           d.ID,
				Definition:   d.Definition,
				Expression:   d.Expression,
				Meaning:      d.Meaning,
				PartOfSpeech: d.PartOfSpeech,
				Note:         d.Note,
				Examples:     d.Examples,
				OriginParts:  d.OriginParts,
			})
		}
		addEtymologyGraphRows(rows, ek, l.Concepts, l.Relations)
	case ShapeGrammar:
		for ei, g := range f.Grammar {
			ek := entry(ei, g.Metadata.Title, "", "", Metadata{}, time.Time{})
			for si, s := range g.Scenes {
				sk := scene(ek, si, s.Metadata.Title, "", s.Metadata.Index, s.Metadata.Scene)
				for ci, c := range s.Corrections {
					rows.add("content_corrections", sk.notebook, sk.family, sk.file, sk.entry, sk.scene, ci,
						c.ID, c.Incorrect, c.Correct, c.Category, c.Reason)
				}
			}
		}
	}
}

func addLine(rows contentRows, sk sceneKey, i int, speaker any, text string) {
	rows.add("content_lines", sk.notebook, sk.family, sk.file, sk.entry, sk.scene, i, speaker, text)
}

func addNoteRows(rows contentRows, k itemKey, n Note) {
	prefix := []any{k.notebook, k.family, k.file, k.entry, k.scene, k.item}
	rows.add("content_notes", append(prefix,
		n.ID, n.Expression, n.Definition, string(n.Level), n.Meaning, n.Origin, n.PartOfSpeech,
		n.Pronunciation, n.Memo, n.Note, n.Reference, n.YoutubeURL, n.YouTubeTimeSeconds,
		n.DictionaryNumber, n.NotUsed, textArray(n.Images), textArray(n.Synonyms),
		textArray(n.Antonyms), textArray(n.Links))...)
	child := func(table string, i int, values ...any) {
		rows.add(table, append(append(append([]any{}, prefix...), i), values...)...)
	}
	for i, e := range n.Examples {
		child("content_note_examples", i, e.Text, e.Highlight)
	}
	for i, o := range n.OriginParts {
		child("content_note_origin_parts", i, o.Origin, o.Language, o.Sense, o.FromForm)
	}
	for i, p := range n.Statements {
		child("content_note_phrases", i, p.Actor, p.Remarks)
	}
	for i, ref := range n.References {
		child("content_note_references", i, ref.URL, ref.Description)
	}
}

func addOriginRows(rows contentRows, k itemKey, o EtymologyOrigin) {
	rows.add("content_origins", k.notebook, k.family, k.file, k.entry, k.scene, k.item,
		o.Origin, o.Type, o.Language, o.Meaning, o.Sense, o.Note, textArray(o.EnglishForms))
	for i, f := range o.Forms {
		rows.add("content_origin_forms", k.notebook, k.family, k.file, k.entry, k.scene, k.item, i, f.Form, f.Role, f.Note)
	}
}

func addEtymologyGraphRows(rows contentRows, ek entryKey, concepts []Concept, relations []Relation) {
	for ci, c := range concepts {
		rows.add("content_origin_concepts", ek.notebook, ek.family, ek.file, ek.entry, ci, c.Key, c.Meaning, c.Note)
		for mi, m := range c.Members {
			rows.add("content_origin_concept_members", ek.notebook, ek.family, ek.file, ek.entry, ci, mi, m.Origin, m.Language)
		}
	}
	for ri, rel := range relations {
		rows.add("content_relations", ek.notebook, ek.family, ek.file, ek.entry, ri,
			rel.Type, textArray(rel.Between), rel.From, rel.To)
	}
}

// Catalog implements ContentLoader: every stored notebook family with its
// file paths, without content.
func (r *ContentRepository) Catalog(ctx context.Context) ([]CatalogEntry, error) {
	var indexes []struct {
		NotebookID string `db:"notebook_id"`
		Family     string `db:"family"`
		Kind       string `db:"kind"`
		Name       string `db:"name"`
		Version    string `db:"version"`
	}
	if err := r.db.SelectContext(ctx, &indexes,
		`SELECT i.notebook_id, i.family, i.kind, i.name,
		        to_char(n.updated_at, 'YYYYMMDDHH24MISSUS') || ':' || COALESCE(n.content_hash, '') AS version
		 FROM content_indexes i JOIN notebooks n ON n.notebook_id = i.notebook_id
		 ORDER BY i.notebook_id, i.family`); err != nil {
		return nil, fmt.Errorf("list content indexes: %w", err)
	}
	var files []struct {
		NotebookID string       `db:"notebook_id"`
		Family     string       `db:"family"`
		Path       string       `db:"path"`
		Date       sql.NullTime `db:"date"`
	}
	// An etymology file's date is its first dated entry (the reader sorts
	// etymology notebooks by their newest session).
	if err := r.db.SelectContext(ctx, &files,
		`SELECT f.notebook_id, f.family, f.path,
		        (SELECT e.date FROM content_entries e
		         WHERE f.family = 'etymology' AND e.notebook_id = f.notebook_id AND e.family = f.family
		           AND e.file_no = f.file_no AND e.date IS NOT NULL
		         ORDER BY e.entry_no LIMIT 1) AS date
		 FROM content_files f ORDER BY f.notebook_id, f.family, f.file_no`); err != nil {
		return nil, fmt.Errorf("list content files: %w", err)
	}
	type key struct{ id, family string }
	pos := make(map[key]int, len(indexes))
	out := make([]CatalogEntry, len(indexes))
	for i, ix := range indexes {
		out[i] = CatalogEntry{NotebookID: ix.NotebookID, Family: ix.Family, Kind: ix.Kind, Name: ix.Name, Version: ix.Version}
		pos[key{ix.NotebookID, ix.Family}] = i
	}
	for _, f := range files {
		i, ok := pos[key{f.NotebookID, f.Family}]
		if !ok {
			continue
		}
		out[i].Paths = append(out[i].Paths, f.Path)
		if f.Date.Valid && f.Date.Time.After(out[i].LatestDate) {
			out[i].LatestDate = f.Date.Time.UTC()
		}
	}
	return out, nil
}

// Load implements ContentLoader: the full content of the given notebooks.
func (r *ContentRepository) Load(ctx context.Context, ids []string) ([]NotebookContent, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	l := contentLoad{ctx: ctx, db: r.db, ids: ids}
	return l.run()
}
