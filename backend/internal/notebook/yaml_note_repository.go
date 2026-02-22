package notebook

import (
	"context"
	"fmt"
	"sort"
)

// YAMLNoteRepository reads notes from YAML files via a Reader.
// It converts YAML Note models into NoteRecord structs, deduplicating
// by (Usage, Entry) and aggregating NotebookNotes from all occurrences.
type YAMLNoteRepository struct {
	reader *Reader
}

// NewYAMLNoteRepository creates a new YAMLNoteRepository.
func NewYAMLNoteRepository(reader *Reader) *YAMLNoteRepository {
	return &YAMLNoteRepository{reader: reader}
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
		if index.IsBook() {
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
