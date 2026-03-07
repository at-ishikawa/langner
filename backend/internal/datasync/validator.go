package datasync

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// ValidationError describes a single mismatch found during validation.
type ValidationError struct {
	NoteKey string
	Field   string
	Source  string
	Export  string
}

// ValidationResult holds the outcome of a validate-datasync run.
type ValidationResult struct {
	SourceNoteCount   int
	ExportedNoteCount int
	Errors            []ValidationError
}

// Validator compares source notes against exported notes to verify round-trip integrity.
type Validator struct {
	writer io.Writer
}

// NewValidator creates a new Validator.
func NewValidator(writer io.Writer) *Validator {
	return &Validator{writer: writer}
}

// ValidateNotes compares source notes with exported notes read back from the export directory.
func (v *Validator) ValidateNotes(ctx context.Context, source NoteSource, exportedSource NoteSource) (*ValidationResult, error) {
	sourceNotes, err := source.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("read source notes: %w", err)
	}

	exportedNotes, err := exportedSource.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("read exported notes: %w", err)
	}

	result := &ValidationResult{
		SourceNoteCount:   len(sourceNotes),
		ExportedNoteCount: len(exportedNotes),
	}

	// Build maps keyed by (usage, entry) for comparison
	type noteKey struct{ usage, entry string }
	sourceMap := make(map[noteKey]notebook.NoteRecord, len(sourceNotes))
	for _, n := range sourceNotes {
		sourceMap[noteKey{n.Usage, n.Entry}] = n
	}

	exportedMap := make(map[noteKey]notebook.NoteRecord, len(exportedNotes))
	for _, n := range exportedNotes {
		exportedMap[noteKey{n.Usage, n.Entry}] = n
	}

	// Check for missing notes in export
	for key := range sourceMap {
		if _, ok := exportedMap[key]; !ok {
			result.Errors = append(result.Errors, ValidationError{
				NoteKey: fmt.Sprintf("%s (%s)", key.usage, key.entry),
				Field:   "presence",
				Source:  "exists",
				Export:  "missing",
			})
		}
	}

	// Check for extra notes in export
	for key := range exportedMap {
		if _, ok := sourceMap[key]; !ok {
			result.Errors = append(result.Errors, ValidationError{
				NoteKey: fmt.Sprintf("%s (%s)", key.usage, key.entry),
				Field:   "presence",
				Source:  "missing",
				Export:  "exists",
			})
		}
	}

	// Compare matching notes
	for key, src := range sourceMap {
		exp, ok := exportedMap[key]
		if !ok {
			continue
		}
		keyStr := fmt.Sprintf("%s (%s)", key.usage, key.entry)
		v.compareNote(keyStr, src, exp, result)
	}

	// Sort errors for deterministic output
	sort.Slice(result.Errors, func(i, j int) bool {
		if result.Errors[i].NoteKey != result.Errors[j].NoteKey {
			return result.Errors[i].NoteKey < result.Errors[j].NoteKey
		}
		return result.Errors[i].Field < result.Errors[j].Field
	})

	return result, nil
}

func (v *Validator) compareNote(keyStr string, src, exp notebook.NoteRecord, result *ValidationResult) {
	if src.Meaning != exp.Meaning {
		result.Errors = append(result.Errors, ValidationError{
			NoteKey: keyStr, Field: "meaning",
			Source: src.Meaning, Export: exp.Meaning,
		})
	}
	if src.Level != exp.Level {
		result.Errors = append(result.Errors, ValidationError{
			NoteKey: keyStr, Field: "level",
			Source: src.Level, Export: exp.Level,
		})
	}
	if src.DictionaryNumber != exp.DictionaryNumber {
		result.Errors = append(result.Errors, ValidationError{
			NoteKey: keyStr, Field: "dictionary_number",
			Source: fmt.Sprintf("%d", src.DictionaryNumber), Export: fmt.Sprintf("%d", exp.DictionaryNumber),
		})
	}

	// Compare notebook notes (by type+id+group+subgroup)
	type nnKey struct{ notebookType, notebookID, group, subgroup string }
	srcNNs := make(map[nnKey]bool, len(src.NotebookNotes))
	for _, nn := range src.NotebookNotes {
		srcNNs[nnKey{nn.NotebookType, nn.NotebookID, nn.Group, nn.Subgroup}] = true
	}
	expNNs := make(map[nnKey]bool, len(exp.NotebookNotes))
	for _, nn := range exp.NotebookNotes {
		expNNs[nnKey{nn.NotebookType, nn.NotebookID, nn.Group, nn.Subgroup}] = true
	}

	for nn := range srcNNs {
		if !expNNs[nn] {
			result.Errors = append(result.Errors, ValidationError{
				NoteKey: keyStr, Field: "notebook_note",
				Source: fmt.Sprintf("%s/%s/%s/%s", nn.notebookType, nn.notebookID, nn.group, nn.subgroup),
				Export: "missing",
			})
		}
	}
	for nn := range expNNs {
		if !srcNNs[nn] {
			result.Errors = append(result.Errors, ValidationError{
				NoteKey: keyStr, Field: "notebook_note",
				Source: "missing",
				Export: fmt.Sprintf("%s/%s/%s/%s", nn.notebookType, nn.notebookID, nn.group, nn.subgroup),
			})
		}
	}
}
