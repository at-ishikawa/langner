package notebook

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// notebookKey groups notes by (NotebookType, NotebookID).
type notebookKey struct {
	notebookType string
	notebookID   string
}
// noteWithNN pairs a converted Note with its NotebookNote metadata.
type noteWithNN struct {
	note Note
	nn   NotebookNote
}

// YAMLNoteRepository reads notes from YAML files via a Reader and writes
// NoteRecord slices to YAML files in a directory structure matching the import format.
type YAMLNoteRepository struct {
	reader    *Reader
	outputDir string
}

// NewYAMLNoteRepository creates a new YAMLNoteRepository for reading.
func NewYAMLNoteRepository(reader *Reader) *YAMLNoteRepository {
	return &YAMLNoteRepository{reader: reader}
}

// NewYAMLNoteRepositoryWriter creates a new YAMLNoteRepository for writing.
func NewYAMLNoteRepositoryWriter(outputDir string) *YAMLNoteRepository {
	return &YAMLNoteRepository{outputDir: outputDir}
}

// FindAll reads all story and flashcard notebooks, converts each YAML Note
// to a NoteRecord, and deduplicates by (Usage, Entry) key.
func (r *YAMLNoteRepository) FindAll(ctx context.Context) ([]NoteRecord, error) {
	storyIndexes, err := r.reader.ReadAllStoryNotebooks()
	if err != nil {
		return nil, fmt.Errorf("read all story notebooks: %w", err)
	}

	flashcardIndexes, err := r.reader.ReadAllFlashcardNotebooks()
	if err != nil {
		return nil, fmt.Errorf("read all flashcard notebooks: %w", err)
	}

	type noteKey struct{ usage, entry string }
	noteMap := make(map[noteKey]*NoteRecord)
	var order []noteKey

	addNote := func(note Note, notebookType, notebookID, group, subgroup string) {
		rec := convertNoteToRecord(note, notebookType, notebookID, group, subgroup)
		key := noteKey{rec.Usage, rec.Entry}

		existing, ok := noteMap[key]
		if !ok {
			noteMap[key] = &rec
			order = append(order, key)
			return
		}

		existing.NotebookNotes = append(existing.NotebookNotes, rec.NotebookNotes...)
	}

	// Sort story index keys for deterministic ordering
	storyKeys := make([]string, 0, len(storyIndexes))
	for k := range storyIndexes {
		storyKeys = append(storyKeys, k)
	}
	sort.Strings(storyKeys)

	// Walk story/book indexes
	for _, indexID := range storyKeys {
		index := storyIndexes[indexID]
		notebookType := "story"
		if index.IsBook {
			notebookType = "book"
		}

		for _, storyNotebooks := range index.Notebooks {
			for _, sn := range storyNotebooks {
				for _, scene := range sn.Scenes {
					for _, def := range scene.Definitions {
						addNote(def, notebookType, indexID, sn.Event, scene.Title)
					}
				}
			}
		}
	}

	// Sort flashcard index keys for deterministic ordering
	flashcardKeys := make([]string, 0, len(flashcardIndexes))
	for k := range flashcardIndexes {
		flashcardKeys = append(flashcardKeys, k)
	}
	sort.Strings(flashcardKeys)

	// Walk flashcard indexes
	for _, flashcardID := range flashcardKeys {
		flashcardIndex := flashcardIndexes[flashcardID]
		for _, fn := range flashcardIndex.Notebooks {
			for _, card := range fn.Cards {
				addNote(card, "flashcard", flashcardID, fn.Title, "")
			}
		}
	}

	result := make([]NoteRecord, 0, len(order))
	for _, key := range order {
		result = append(result, *noteMap[key])
	}
	return result, nil
}

func convertNoteToRecord(note Note, notebookType, notebookID, group, subgroup string) NoteRecord {
	entry := note.Definition
	if entry == "" {
		entry = note.Expression
	}

	images := make([]NoteImage, len(note.Images))
	for i, img := range note.Images {
		images[i] = NoteImage{URL: img, SortOrder: i}
	}

	references := make([]NoteReference, len(note.References))
	for i, ref := range note.References {
		references[i] = NoteReference{Link: ref.URL, Description: ref.Description, SortOrder: i}
	}

	return NoteRecord{
		Usage:            note.Expression,
		Entry:            entry,
		Meaning:          note.Meaning,
		Level:            string(note.Level),
		DictionaryNumber: note.DictionaryNumber,
		Images:           images,
		References:       references,
		NotebookNotes: []NotebookNote{
			{NotebookType: notebookType, NotebookID: notebookID, Group: group, Subgroup: subgroup},
		},
	}
}

// WriteAll converts NoteRecords to YAML files grouped by notebook.
func (r *YAMLNoteRepository) WriteAll(notes []NoteRecord) error {
	grouped := make(map[notebookKey][]noteWithNN)
	var keys []notebookKey
	seenKeys := make(map[notebookKey]bool)

	for _, rec := range notes {
		note := convertRecordToNote(rec)
		for _, nn := range rec.NotebookNotes {
			key := notebookKey{notebookType: nn.NotebookType, notebookID: nn.NotebookID}
			if !seenKeys[key] {
				seenKeys[key] = true
				keys = append(keys, key)
			}
			grouped[key] = append(grouped[key], noteWithNN{note: note, nn: nn})
		}
	}

	// Sort keys for deterministic output
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].notebookType != keys[j].notebookType {
			return keys[i].notebookType < keys[j].notebookType
		}
		return keys[i].notebookID < keys[j].notebookID
	})

	for _, key := range keys {
		entries := grouped[key]
		switch key.notebookType {
		case "story", "book":
			if err := r.writeStoryNotebook(key, entries); err != nil {
				return fmt.Errorf("write story notebook %s: %w", key.notebookID, err)
			}
		case "flashcard":
			if err := r.writeFlashcardNotebook(key, entries); err != nil {
				return fmt.Errorf("write flashcard notebook %s: %w", key.notebookID, err)
			}
		}
	}

	return nil
}

func (r *YAMLNoteRepository) writeStoryNotebook(key notebookKey, entries []noteWithNN) error {
	// Group by Group (Event) then Subgroup (Scene Title)
	type sceneKey struct {
		event string
		scene string
	}

	eventOrder := make(map[string]int)
	sceneOrder := make(map[sceneKey]int)
	var eventCounter, sceneCounter int

	notebookMap := make(map[string]map[string][]Note)
	for _, e := range entries {
		event := e.nn.Group
		scene := e.nn.Subgroup

		if _, ok := eventOrder[event]; !ok {
			eventOrder[event] = eventCounter
			eventCounter++
		}
		sk := sceneKey{event, scene}
		if _, ok := sceneOrder[sk]; !ok {
			sceneOrder[sk] = sceneCounter
			sceneCounter++
		}

		if notebookMap[event] == nil {
			notebookMap[event] = make(map[string][]Note)
		}
		notebookMap[event][scene] = append(notebookMap[event][scene], e.note)
	}

	// Sort events deterministically by insertion order
	events := make([]string, 0, len(notebookMap))
	for event := range notebookMap {
		events = append(events, event)
	}
	sort.Slice(events, func(i, j int) bool {
		return eventOrder[events[i]] < eventOrder[events[j]]
	})

	var storyNotebooks []StoryNotebook
	for _, event := range events {
		scenes := notebookMap[event]

		// Sort scenes deterministically by insertion order
		sceneNames := make([]string, 0, len(scenes))
		for scene := range scenes {
			sceneNames = append(sceneNames, scene)
		}
		sort.Slice(sceneNames, func(i, j int) bool {
			return sceneOrder[sceneKey{event, sceneNames[i]}] < sceneOrder[sceneKey{event, sceneNames[j]}]
		})

		var storyScenes []StoryScene
		for _, sceneName := range sceneNames {
			storyScenes = append(storyScenes, StoryScene{
				Title:       sceneName,
				Definitions: scenes[sceneName],
			})
		}

		storyNotebooks = append(storyNotebooks, StoryNotebook{
			Event:  event,
			Scenes: storyScenes,
		})
	}

	// Determine directory name based on type
	dirName := "stories"
	if key.notebookType == "book" {
		dirName = "books"
	}
	dir := filepath.Join(r.outputDir, dirName, key.notebookID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	// Write index.yml
	index := Index{
		ID:            key.notebookID,
		Kind:          key.notebookType,
		Name:          key.notebookID,
		NotebookPaths: []string{"./notebooks.yml"},
	}
	if err := WriteYamlFile(filepath.Join(dir, "index.yml"), index); err != nil {
		return fmt.Errorf("write index.yml: %w", err)
	}

	// Write notebooks.yml
	if err := WriteYamlFile(filepath.Join(dir, "notebooks.yml"), storyNotebooks); err != nil {
		return fmt.Errorf("write notebooks.yml: %w", err)
	}

	return nil
}

func (r *YAMLNoteRepository) writeFlashcardNotebook(key notebookKey, entries []noteWithNN) error {
	// Group by Group (Title)
	titleOrder := make(map[string]int)
	var titleCounter int
	notebookMap := make(map[string][]Note)

	for _, e := range entries {
		title := e.nn.Group
		if _, ok := titleOrder[title]; !ok {
			titleOrder[title] = titleCounter
			titleCounter++
		}
		notebookMap[title] = append(notebookMap[title], e.note)
	}

	// Sort titles deterministically by insertion order
	titles := make([]string, 0, len(notebookMap))
	for title := range notebookMap {
		titles = append(titles, title)
	}
	sort.Slice(titles, func(i, j int) bool {
		return titleOrder[titles[i]] < titleOrder[titles[j]]
	})

	var flashcardNotebooks []FlashcardNotebook
	for _, title := range titles {
		flashcardNotebooks = append(flashcardNotebooks, FlashcardNotebook{
			Title: title,
			Cards: notebookMap[title],
		})
	}

	dir := filepath.Join(r.outputDir, "flashcards", key.notebookID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	// Write index.yml
	index := FlashcardIndex{
		ID:            key.notebookID,
		Name:          key.notebookID,
		NotebookPaths: []string{"./cards.yml"},
	}
	if err := WriteYamlFile(filepath.Join(dir, "index.yml"), index); err != nil {
		return fmt.Errorf("write index.yml: %w", err)
	}

	// Write cards.yml
	if err := WriteYamlFile(filepath.Join(dir, "cards.yml"), flashcardNotebooks); err != nil {
		return fmt.Errorf("write cards.yml: %w", err)
	}

	return nil
}

func convertRecordToNote(rec NoteRecord) Note {
	definition := rec.Entry
	if definition == rec.Usage {
		definition = ""
	}

	var images []string
	if len(rec.Images) > 0 {
		sorted := make([]NoteImage, len(rec.Images))
		copy(sorted, rec.Images)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].SortOrder < sorted[j].SortOrder
		})
		images = make([]string, len(sorted))
		for i, img := range sorted {
			images[i] = img.URL
		}
	}

	var references []Reference
	if len(rec.References) > 0 {
		sorted := make([]NoteReference, len(rec.References))
		copy(sorted, rec.References)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].SortOrder < sorted[j].SortOrder
		})
		references = make([]Reference, len(sorted))
		for i, ref := range sorted {
			references[i] = Reference{URL: ref.Link, Description: ref.Description}
		}
	}

	return Note{
		Expression:       rec.Usage,
		Definition:       definition,
		Meaning:          rec.Meaning,
		Level:            ExpressionLevel(rec.Level),
		DictionaryNumber: rec.DictionaryNumber,
		Images:           images,
		References:       references,
	}
}
