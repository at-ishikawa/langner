package notebook

import "testing"

// note builds a NoteRecord living in one notebook with a given sense.
func note(nbID, expr, pos string) NoteRecord {
	return NoteRecord{
		Usage:        expr,
		Entry:        expr,
		PartOfSpeech: pos,
		NotebookNotes: []NotebookNote{
			{NotebookType: "flashcard", NotebookID: nbID, Group: "unit"},
		},
	}
}

func flashcardHistory(nbID string, exprs ...LearningHistoryExpression) LearningHistory {
	return LearningHistory{
		Metadata:    LearningHistoryMetadata{NotebookID: nbID, Title: "unit", Type: "flashcard"},
		Expressions: exprs,
	}
}

func TestBackfillSenses_SingleSenseStamped(t *testing.T) {
	histories := []LearningHistory{
		flashcardHistory("nb-1", LearningHistoryExpression{
			Expression:  "record",
			LearnedLogs: []LearningRecord{{Quality: 4}},
		}),
	}
	notes := []NoteRecord{note("nb-1", "record", "noun")}

	res := BackfillSenses(histories, notes)

	if got := histories[0].Expressions[0].PartOfSpeech; got != "noun" {
		t.Fatalf("expected entry stamped noun, got %q", got)
	}
	if res.Tagged != 1 || res.LeftLegacy != 0 {
		t.Fatalf("expected Tagged=1 LeftLegacy=0, got %+v", res)
	}
	// Logs untouched.
	if len(histories[0].Expressions[0].LearnedLogs) != 1 {
		t.Fatalf("logs must not move: got %d", len(histories[0].Expressions[0].LearnedLogs))
	}
}

func TestBackfillSenses_HomographLeftLegacy(t *testing.T) {
	histories := []LearningHistory{
		flashcardHistory("nb-1", LearningHistoryExpression{
			Expression:  "record",
			LearnedLogs: []LearningRecord{{Quality: 4}, {Quality: 1}},
		}),
	}
	// Two notes, same spelling, differing senses -> genuine homograph.
	notes := []NoteRecord{
		note("nb-1", "record", "noun"),
		note("nb-1", "record", "verb"),
	}

	res := BackfillSenses(histories, notes)

	if got := histories[0].Expressions[0].PartOfSpeech; got != "" {
		t.Fatalf("homograph entry must stay legacy, got %q", got)
	}
	if res.Tagged != 0 || res.LeftLegacy != 1 {
		t.Fatalf("expected Tagged=0 LeftLegacy=1, got %+v", res)
	}
	// No log was moved or lost.
	if len(histories[0].Expressions[0].LearnedLogs) != 2 {
		t.Fatalf("logs must not move: got %d", len(histories[0].Expressions[0].LearnedLogs))
	}
}

func TestBackfillSenses_AlreadyTaggedSkipped(t *testing.T) {
	histories := []LearningHistory{
		flashcardHistory("nb-1", LearningHistoryExpression{
			Expression:   "record",
			PartOfSpeech: "verb",
		}),
	}
	// Source says noun, but the entry is already tagged verb — leave it.
	notes := []NoteRecord{note("nb-1", "record", "noun")}

	res := BackfillSenses(histories, notes)

	if got := histories[0].Expressions[0].PartOfSpeech; got != "verb" {
		t.Fatalf("already-tagged entry must be preserved, got %q", got)
	}
	if res.Tagged != 0 || res.LeftLegacy != 0 {
		t.Fatalf("expected no changes, got %+v", res)
	}
}

func TestBackfillSenses_NoMatchingNoteSkipped(t *testing.T) {
	histories := []LearningHistory{
		flashcardHistory("nb-1", LearningHistoryExpression{Expression: "record"}),
	}
	// Note lives in a different notebook — must not match.
	notes := []NoteRecord{note("nb-2", "record", "noun")}

	res := BackfillSenses(histories, notes)

	if got := histories[0].Expressions[0].PartOfSpeech; got != "" {
		t.Fatalf("no matching note in notebook: entry must stay empty, got %q", got)
	}
	if res.Tagged != 0 || res.LeftLegacy != 0 {
		t.Fatalf("expected no changes, got %+v", res)
	}
}

func TestBackfillSenses_SingleUnspecifiedSenseSkipped(t *testing.T) {
	histories := []LearningHistory{
		flashcardHistory("nb-1", LearningHistoryExpression{Expression: "record"}),
	}
	// The lone matching note carries no sense — nothing to stamp.
	notes := []NoteRecord{note("nb-1", "record", "")}

	res := BackfillSenses(histories, notes)

	if got := histories[0].Expressions[0].PartOfSpeech; got != "" {
		t.Fatalf("unspecified sense: entry must stay empty, got %q", got)
	}
	if res.Tagged != 0 || res.LeftLegacy != 0 {
		t.Fatalf("expected no changes, got %+v", res)
	}
}

func TestBackfillSenses_SceneEntries(t *testing.T) {
	histories := []LearningHistory{
		{
			Metadata: LearningHistoryMetadata{NotebookID: "story-1", Title: "event"},
			Scenes: []LearningScene{
				{
					Metadata:    LearningSceneMetadata{Title: "scene"},
					Expressions: []LearningHistoryExpression{{Expression: "lead"}},
				},
			},
		},
	}
	notes := []NoteRecord{{
		Usage:        "lead",
		Entry:        "lead",
		PartOfSpeech: "verb",
		NotebookNotes: []NotebookNote{
			{NotebookType: "story", NotebookID: "story-1", Group: "event", Subgroup: "scene"},
		},
	}}

	res := BackfillSenses(histories, notes)

	if got := histories[0].Scenes[0].Expressions[0].PartOfSpeech; got != "verb" {
		t.Fatalf("scene entry should be stamped verb, got %q", got)
	}
	if res.Tagged != 1 {
		t.Fatalf("expected Tagged=1, got %+v", res)
	}
}
