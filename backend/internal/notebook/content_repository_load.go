package notebook

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// stringList scans a TEXT[] column selected as to_json(column).
type stringList []string

func (s *stringList) Scan(src any) error {
	var data []byte
	switch v := src.(type) {
	case nil:
		*s = nil
		return nil
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("stringList: unsupported %T", src)
	}
	var out []string
	if err := json.Unmarshal(data, &out); err != nil {
		return fmt.Errorf("stringList: %w", err)
	}
	if len(out) == 0 {
		out = nil
	}
	*s = out
	return nil
}

// contentLoad reads the content tables for a set of notebooks and assembles
// them into NotebookContent.
type contentLoad struct {
	ctx context.Context
	db  *sqlx.DB
	ids []string
}

type keyCols struct {
	NotebookID string `db:"notebook_id"`
	Family     string `db:"family"`
	FileNo     int    `db:"file_no"`
	EntryNo    int    `db:"entry_no"`
}

func (k keyCols) entry() entryKey { return entryKey{fileKey{k.NotebookID, k.Family, k.FileNo}, k.EntryNo} }

type sceneCols struct {
	keyCols
	SceneNo int `db:"scene_no"`
}

func (k sceneCols) scene() sceneKey { return sceneKey{k.entry(), k.SceneNo} }

type noteCols struct {
	sceneCols
	NoteNo int `db:"note_no"`
}

func (k noteCols) note() itemKey { return itemKey{k.scene(), k.NoteNo} }

// selectRows runs SELECT <columns> FROM table for the notebooks being loaded,
// in primary-key order.
func (l contentLoad) selectRows(dest any, table, columns, orderBy string) error {
	q := fmt.Sprintf(`SELECT %s FROM %s WHERE notebook_id = ANY($1) ORDER BY %s`, columns, table, orderBy)
	if err := l.db.SelectContext(l.ctx, dest, q, l.ids); err != nil {
		return fmt.Errorf("read %s: %w", table, err)
	}
	return nil
}

const (
	entryOrder = "notebook_id, family, file_no, entry_no"
	sceneOrder = entryOrder + ", scene_no"
	noteOrder  = sceneOrder + ", note_no"
)

func (l contentLoad) run() ([]NotebookContent, error) {
	var indexes []struct {
		NotebookID string `db:"notebook_id"`
		Family     string `db:"family"`
		Kind       string `db:"kind"`
		Name       string `db:"name"`
	}
	if err := l.selectRows(&indexes, "content_indexes", "notebook_id, family, kind, name", "notebook_id, family"); err != nil {
		return nil, err
	}
	var files []struct {
		NotebookID string `db:"notebook_id"`
		Family     string `db:"family"`
		FileNo     int    `db:"file_no"`
		Path       string `db:"path"`
		Shape      string `db:"shape"`
	}
	if err := l.selectRows(&files, "content_files", "notebook_id, family, file_no, path, shape", "notebook_id, family, file_no"); err != nil {
		return nil, err
	}
	var entries []struct {
		keyCols
		Title       string       `db:"title"`
		SessionKey  string       `db:"session_key"`
		Description string       `db:"description"`
		Series      string       `db:"series"`
		Season      int          `db:"season"`
		Episode     int          `db:"episode"`
		Date        sql.NullTime `db:"date"`
	}
	if err := l.selectRows(&entries, "content_entries",
		"notebook_id, family, file_no, entry_no, title, session_key, description, series, season, episode, date", entryOrder); err != nil {
		return nil, err
	}
	var scenes []struct {
		sceneCols
		Title       string        `db:"title"`
		Type        string        `db:"type"`
		SceneIndex  int           `db:"scene_index"`
		SceneNumber sql.NullInt64 `db:"scene_number"`
	}
	if err := l.selectRows(&scenes, "content_scenes",
		"notebook_id, family, file_no, entry_no, scene_no, title, type, scene_index, scene_number", sceneOrder); err != nil {
		return nil, err
	}
	var lines []struct {
		sceneCols
		Speaker sql.NullString `db:"speaker"`
		Text    string         `db:"text"`
	}
	if err := l.selectRows(&lines, "content_lines",
		"notebook_id, family, file_no, entry_no, scene_no, speaker, text", sceneOrder+", line_no"); err != nil {
		return nil, err
	}
	notes, err := l.loadNotes()
	if err != nil {
		return nil, err
	}
	origins, err := l.loadOrigins()
	if err != nil {
		return nil, err
	}
	defConcepts, originConcepts, relations, err := l.loadGraph()
	if err != nil {
		return nil, err
	}
	var corrections []struct {
		sceneCols
		CorrectionID string `db:"correction_id"`
		Incorrect    string `db:"incorrect"`
		Correct      string `db:"correct"`
		Category     string `db:"category"`
		Reason       string `db:"reason"`
	}
	if err := l.selectRows(&corrections, "content_corrections",
		"notebook_id, family, file_no, entry_no, scene_no, correction_id, incorrect, correct, category, reason",
		sceneOrder+", correction_no"); err != nil {
		return nil, err
	}

	// Group the child rows by their parent.
	type sceneRows struct {
		title, typ    string
		index         int
		number        *int
		conversations []Conversation
		statements    []string
		notes         []Note
		origins       []EtymologyOrigin
		corrections   []Correction
	}
	scenesByEntry := make(map[entryKey][]*sceneRows)
	sceneByKey := make(map[sceneKey]*sceneRows)
	for _, s := range scenes {
		sr := &sceneRows{title: s.Title, typ: s.Type, index: s.SceneIndex}
		if s.SceneNumber.Valid {
			n := int(s.SceneNumber.Int64)
			sr.number = &n
		}
		scenesByEntry[s.entry()] = append(scenesByEntry[s.entry()], sr)
		sceneByKey[s.scene()] = sr
	}
	for _, ln := range lines {
		sr := sceneByKey[ln.scene()]
		if sr == nil {
			continue
		}
		if ln.Speaker.Valid {
			sr.conversations = append(sr.conversations, Conversation{Speaker: ln.Speaker.String, Quote: ln.Text})
		} else {
			sr.statements = append(sr.statements, ln.Text)
		}
	}
	for _, n := range notes {
		if sr := sceneByKey[n.key]; sr != nil {
			sr.notes = append(sr.notes, n.note)
		}
	}
	for _, o := range origins {
		if sr := sceneByKey[o.key]; sr != nil {
			sr.origins = append(sr.origins, o.origin)
		}
	}
	for _, c := range corrections {
		if sr := sceneByKey[c.scene()]; sr != nil {
			sr.corrections = append(sr.corrections, Correction{
				ID: c.CorrectionID, Incorrect: c.Incorrect, Correct: c.Correct, Category: c.Category, Reason: c.Reason,
			})
		}
	}
	type entryRows struct {
		title, sessionKey, description string
		meta                           Metadata
		date                           time.Time
		scenes                         []*sceneRows
	}
	entriesByFile := make(map[fileKey][]*entryRows)
	for _, e := range entries {
		er := &entryRows{
			title: e.Title, sessionKey: e.SessionKey, description: e.Description,
			meta:   Metadata{Series: e.Series, Season: e.Season, Episode: e.Episode},
			scenes: scenesByEntry[e.entry()],
		}
		if e.Date.Valid {
			er.date = e.Date.Time.UTC()
		}
		entriesByFile[e.entry().fileKey] = append(entriesByFile[e.entry().fileKey], er)
	}

	buildFile := func(fk fileKey, p, shape string) ContentFile {
		f := ContentFile{Path: p, Shape: shape}
		ents := entriesByFile[fk]
		entryKeyAt := func(i int) entryKey { return entryKey{fk, i} }
		switch shape {
		case ShapeStory:
			for _, e := range ents {
				nb := StoryNotebook{Event: e.title, Metadata: e.meta, Date: e.date}
				for _, s := range e.scenes {
					nb.Scenes = append(nb.Scenes, StoryScene{
						Title: s.title, Type: s.typ, Conversations: s.conversations,
						Statements: s.statements, Definitions: s.notes,
					})
				}
				f.Stories = append(f.Stories, nb)
			}
		case ShapeFlashcard:
			for _, e := range ents {
				deck := FlashcardNotebook{Title: e.title, Description: e.description, Date: e.date}
				for _, s := range e.scenes {
					deck.Cards = append(deck.Cards, s.notes...)
				}
				f.Flashcards = append(f.Flashcards, deck)
			}
		case ShapeDefinitions:
			for i, e := range ents {
				def := Definitions{
					Metadata: DefinitionsMetadata{Notebook: e.sessionKey, Title: e.title, Date: e.date},
					Concepts: defConcepts[entryKeyAt(i)],
				}
				for _, s := range e.scenes {
					def.Scenes = append(def.Scenes, DefinitionsScene{
						Metadata:    DefinitionsSceneMetadata{Index: s.index, Scene: s.number, Title: s.title},
						Expressions: s.notes,
					})
				}
				f.Definitions = append(f.Definitions, def)
			}
		case ShapeEtymology:
			for i, e := range ents {
				entry := EtymologyNotebookEntry{
					Event: e.title, Date: e.date,
					Concepts: originConcepts[entryKeyAt(i)], Relations: relations[entryKeyAt(i)],
				}
				for _, s := range e.scenes {
					entry.Scenes = append(entry.Scenes, EtymologyNotebookScene{Scene: s.title, Origins: s.origins})
				}
				f.Etymology = append(f.Etymology, entry)
			}
		case ShapeEtymologyLegacy:
			legacy := etymologySessionFile{}
			if len(ents) > 0 {
				e := ents[0]
				legacy.Metadata.Title = e.title
				legacy.Date = e.date
				legacy.Concepts = originConcepts[entryKeyAt(0)]
				legacy.Relations = relations[entryKeyAt(0)]
				for _, s := range e.scenes {
					legacy.Origins = append(legacy.Origins, s.origins...)
					for _, n := range s.notes {
						legacy.Definitions = append(legacy.Definitions, EtymologyDefinitionEntry{
							ID: n.ID, Definition: n.Definition, Expression: n.Expression, Meaning: n.Meaning,
							PartOfSpeech: n.PartOfSpeech, Note: n.Note, Examples: n.Examples, OriginParts: n.OriginParts,
						})
					}
				}
			}
			f.EtymologyLegacy = &legacy
		case ShapeGrammar:
			for _, e := range ents {
				g := GrammarEntry{Metadata: GrammarMetadata{Title: e.title}}
				for _, s := range e.scenes {
					g.Scenes = append(g.Scenes, GrammarScene{
						Metadata:    GrammarSceneMetadata{Index: s.index, Scene: s.number, Title: s.title},
						Corrections: s.corrections,
					})
				}
				f.Grammar = append(f.Grammar, g)
			}
		}
		return f
	}

	byID := make(map[string]*NotebookContent)
	var order []string
	for _, ix := range indexes {
		nc := byID[ix.NotebookID]
		if nc == nil {
			nc = &NotebookContent{ID: ix.NotebookID}
			byID[ix.NotebookID] = nc
			order = append(order, ix.NotebookID)
		}
		nc.Families = append(nc.Families, FamilyContent{Family: ix.Family, Kind: ix.Kind, Name: ix.Name})
	}
	for _, f := range files {
		nc := byID[f.NotebookID]
		if nc == nil {
			continue
		}
		fc := nc.Family(f.Family)
		if fc == nil {
			continue
		}
		fc.Files = append(fc.Files, buildFile(fileKey{f.NotebookID, f.Family, f.FileNo}, f.Path, f.Shape))
	}
	out := make([]NotebookContent, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}

type loadedNote struct {
	key  sceneKey
	note Note
}

func (l contentLoad) loadNotes() ([]loadedNote, error) {
	var rows []struct {
		noteCols
		SenseID            string     `db:"sense_id"`
		Expression         string     `db:"expression"`
		Definition         string     `db:"definition"`
		Level              string     `db:"level"`
		Meaning            string     `db:"meaning"`
		Origin             string     `db:"origin"`
		PartOfSpeech       string     `db:"part_of_speech"`
		Pronunciation      string     `db:"pronunciation"`
		Memo               string     `db:"memo"`
		Note               string     `db:"note"`
		Reference          string     `db:"reference"`
		YoutubeURL         string     `db:"youtube_url"`
		YoutubeTimeSeconds int        `db:"youtube_time_seconds"`
		DictionaryNumber   int        `db:"dictionary_number"`
		NotUsed            bool       `db:"not_used"`
		Images             stringList `db:"images"`
		Synonyms           stringList `db:"synonyms"`
		Antonyms           stringList `db:"antonyms"`
		Links              stringList `db:"links"`
	}
	if err := l.selectRows(&rows, "content_notes", strings.Join([]string{
		"notebook_id, family, file_no, entry_no, scene_no, note_no, sense_id, expression, definition, level",
		"meaning, origin, part_of_speech, pronunciation, memo, note, reference, youtube_url, youtube_time_seconds",
		"dictionary_number, not_used, to_json(images) AS images, to_json(synonyms) AS synonyms",
		"to_json(antonyms) AS antonyms, to_json(links) AS links",
	}, ", "), noteOrder); err != nil {
		return nil, err
	}

	var examples []struct {
		noteCols
		Text      string `db:"text"`
		Highlight string `db:"highlight"`
	}
	if err := l.selectRows(&examples, "content_note_examples",
		"notebook_id, family, file_no, entry_no, scene_no, note_no, text, highlight", noteOrder+", item_no"); err != nil {
		return nil, err
	}
	var originParts []struct {
		noteCols
		Origin   string `db:"origin"`
		Language string `db:"language"`
		Sense    string `db:"sense"`
		FromForm string `db:"from_form"`
	}
	if err := l.selectRows(&originParts, "content_note_origin_parts",
		"notebook_id, family, file_no, entry_no, scene_no, note_no, origin, language, sense, from_form", noteOrder+", item_no"); err != nil {
		return nil, err
	}
	var phrases []struct {
		noteCols
		Actor   string `db:"actor"`
		Remarks string `db:"remarks"`
	}
	if err := l.selectRows(&phrases, "content_note_phrases",
		"notebook_id, family, file_no, entry_no, scene_no, note_no, actor, remarks", noteOrder+", item_no"); err != nil {
		return nil, err
	}
	var references []struct {
		noteCols
		URL         string `db:"url"`
		Description string `db:"description"`
	}
	if err := l.selectRows(&references, "content_note_references",
		"notebook_id, family, file_no, entry_no, scene_no, note_no, url, description", noteOrder+", item_no"); err != nil {
		return nil, err
	}

	out := make([]loadedNote, len(rows))
	byKey := make(map[itemKey]*Note, len(rows))
	for i, r := range rows {
		out[i] = loadedNote{key: r.scene(), note: Note{
			ID: r.SenseID, Expression: r.Expression, Definition: r.Definition, Level: ExpressionLevel(r.Level),
			Meaning: r.Meaning, Origin: r.Origin, PartOfSpeech: r.PartOfSpeech, Pronunciation: r.Pronunciation,
			Memo: r.Memo, Note: r.Note, Reference: r.Reference, YoutubeURL: r.YoutubeURL,
			YouTubeTimeSeconds: r.YoutubeTimeSeconds, DictionaryNumber: r.DictionaryNumber, NotUsed: r.NotUsed,
			Images: r.Images, Synonyms: r.Synonyms, Antonyms: r.Antonyms, Links: r.Links,
		}}
		byKey[r.note()] = &out[i].note
	}
	for _, e := range examples {
		if n := byKey[e.note()]; n != nil {
			n.Examples = append(n.Examples, Example{Text: e.Text, Highlight: e.Highlight})
		}
	}
	for _, o := range originParts {
		if n := byKey[o.note()]; n != nil {
			n.OriginParts = append(n.OriginParts, OriginPartRef{Origin: o.Origin, Language: o.Language, Sense: o.Sense, FromForm: o.FromForm})
		}
	}
	for _, p := range phrases {
		if n := byKey[p.note()]; n != nil {
			n.Statements = append(n.Statements, Phrase{Actor: p.Actor, Remarks: p.Remarks})
		}
	}
	for _, ref := range references {
		if n := byKey[ref.note()]; n != nil {
			n.References = append(n.References, Reference{URL: ref.URL, Description: ref.Description})
		}
	}
	return out, nil
}

type loadedOrigin struct {
	key    sceneKey
	origin EtymologyOrigin
}

func (l contentLoad) loadOrigins() ([]loadedOrigin, error) {
	type originCols struct {
		sceneCols
		OriginNo int `db:"origin_no"`
	}
	var rows []struct {
		originCols
		Origin       string     `db:"origin"`
		Type         string     `db:"type"`
		Language     string     `db:"language"`
		Meaning      string     `db:"meaning"`
		Sense        string     `db:"sense"`
		Note         string     `db:"note"`
		EnglishForms stringList `db:"english_forms"`
	}
	if err := l.selectRows(&rows, "content_origins",
		"notebook_id, family, file_no, entry_no, scene_no, origin_no, origin, type, language, meaning, sense, note, to_json(english_forms) AS english_forms",
		sceneOrder+", origin_no"); err != nil {
		return nil, err
	}
	var forms []struct {
		originCols
		Form string `db:"form"`
		Role string `db:"role"`
		Note string `db:"note"`
	}
	if err := l.selectRows(&forms, "content_origin_forms",
		"notebook_id, family, file_no, entry_no, scene_no, origin_no, form, role, note", sceneOrder+", origin_no, item_no"); err != nil {
		return nil, err
	}
	out := make([]loadedOrigin, len(rows))
	byKey := make(map[itemKey]*EtymologyOrigin, len(rows))
	for i, r := range rows {
		out[i] = loadedOrigin{key: r.scene(), origin: EtymologyOrigin{
			Origin: r.Origin, Type: r.Type, Language: r.Language, Meaning: r.Meaning,
			Sense: r.Sense, Note: r.Note, EnglishForms: r.EnglishForms,
		}}
		byKey[itemKey{r.scene(), r.OriginNo}] = &out[i].origin
	}
	for _, f := range forms {
		if o := byKey[itemKey{f.scene(), f.OriginNo}]; o != nil {
			o.Forms = append(o.Forms, EtymologyOriginForm{Form: f.Form, Role: f.Role, Note: f.Note})
		}
	}
	return out, nil
}

func (l contentLoad) loadGraph() (map[entryKey][]DefinitionConcept, map[entryKey][]Concept, map[entryKey][]Relation, error) {
	var defRows []struct {
		keyCols
		Head        string     `db:"head"`
		Meaning     string     `db:"meaning"`
		Kind        string     `db:"kind"`
		Expressions stringList `db:"expressions"`
	}
	if err := l.selectRows(&defRows, "content_definition_concepts",
		"notebook_id, family, file_no, entry_no, head, meaning, kind, to_json(expressions) AS expressions", entryOrder+", concept_no"); err != nil {
		return nil, nil, nil, err
	}
	defConcepts := make(map[entryKey][]DefinitionConcept)
	for _, r := range defRows {
		defConcepts[r.entry()] = append(defConcepts[r.entry()], DefinitionConcept{
			Head: r.Head, Meaning: r.Meaning, Kind: ConceptKind(r.Kind), Expressions: r.Expressions,
		})
	}

	type conceptCols struct {
		keyCols
		ConceptNo int `db:"concept_no"`
	}
	var conceptRows []struct {
		conceptCols
		Key     string `db:"key"`
		Meaning string `db:"meaning"`
		Note    string `db:"note"`
	}
	if err := l.selectRows(&conceptRows, "content_origin_concepts",
		"notebook_id, family, file_no, entry_no, concept_no, key, meaning, note", entryOrder+", concept_no"); err != nil {
		return nil, nil, nil, err
	}
	var memberRows []struct {
		conceptCols
		Origin   string `db:"origin"`
		Language string `db:"language"`
	}
	if err := l.selectRows(&memberRows, "content_origin_concept_members",
		"notebook_id, family, file_no, entry_no, concept_no, origin, language", entryOrder+", concept_no, item_no"); err != nil {
		return nil, nil, nil, err
	}
	originConcepts := make(map[entryKey][]Concept)
	for _, r := range conceptRows {
		originConcepts[r.entry()] = append(originConcepts[r.entry()], Concept{Key: r.Key, Meaning: r.Meaning, Note: r.Note})
	}
	for _, m := range memberRows {
		concepts := originConcepts[m.entry()]
		if m.ConceptNo < len(concepts) {
			concepts[m.ConceptNo].Members = append(concepts[m.ConceptNo].Members, ConceptMember{Origin: m.Origin, Language: m.Language})
		}
	}

	var relRows []struct {
		keyCols
		Type        string     `db:"type"`
		BetweenKeys stringList `db:"between_keys"`
		FromKey     string     `db:"from_key"`
		ToKey       string     `db:"to_key"`
	}
	if err := l.selectRows(&relRows, "content_relations",
		"notebook_id, family, file_no, entry_no, type, to_json(between_keys) AS between_keys, from_key, to_key",
		entryOrder+", relation_no"); err != nil {
		return nil, nil, nil, err
	}
	relations := make(map[entryKey][]Relation)
	for _, r := range relRows {
		relations[r.entry()] = append(relations[r.entry()], Relation{Type: r.Type, Between: r.BetweenKeys, From: r.FromKey, To: r.ToKey})
	}
	return defConcepts, originConcepts, relations, nil
}
