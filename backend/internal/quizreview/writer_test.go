package quizreview

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/analytics"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// stubSource lets the tests inject canned conversations / concepts for
// specific (notebookID, sessionTitle) pairs without spinning up a real
// notebook.Reader.
type stubSource struct {
	conversations map[string][]SourceScene
	concepts      map[string][]notebook.Concept
	relations     map[string][]notebook.Relation
	meanings      map[string]map[string]string
	bookIDs       map[string]bool
	vocabulary    map[string][]VocabularyPair
}

func (s *stubSource) IsBook(notebookID string) bool {
	return s.bookIDs[notebookID]
}

func (s *stubSource) VocabularyForSession(notebookID, sessionTitle string) []VocabularyPair {
	return s.vocabulary[s.key(notebookID, sessionTitle)]
}

func (s *stubSource) key(notebookID, sessionTitle string) string {
	return notebookID + "|" + sessionTitle
}
func (s *stubSource) StoryConversations(notebookID, sessionTitle string) []SourceScene {
	return s.conversations[s.key(notebookID, sessionTitle)]
}
func (s *stubSource) EtymologyConcepts(notebookID, sessionTitle string) ([]notebook.Concept, []notebook.Relation, map[string]string) {
	k := s.key(notebookID, sessionTitle)
	return s.concepts[k], s.relations[k], s.meanings[k]
}

// stubRepo lets the writer tests inject a fixed DayDetail without
// going through the YAML repository — keeps the tests focused on the
// per-notebook split and rendering rather than the YAML format.
type stubRepo struct {
	detail analytics.DayDetail
	err    error
}

func (s *stubRepo) DailySummaries(context.Context, int, analytics.Filters) ([]analytics.DailySummary, error) {
	return nil, nil
}
func (s *stubRepo) DayDetail(context.Context, time.Time, analytics.Filters) (analytics.DayDetail, error) {
	return s.detail, s.err
}
func (s *stubRepo) WordHistory(context.Context, analytics.WordRef) (analytics.WordHistory, error) {
	return analytics.WordHistory{}, nil
}

func TestWriter_SingleFileWithEveryNotebook(t *testing.T) {
	day, _ := time.Parse("2006-01-02", "2026-06-16")
	repo := &stubRepo{
		detail: analytics.DayDetail{
			WrongWords: []analytics.WrongWord{
				{
					NotebookID:    "word-power-made-easy",
					NotebookTitle: "Session 3",
					Expression:    "gauche",
					QuizType:      "notebook",
					Meaning:       "clumsy, tactless, especially in social situations",
					NotebookKind:  "story",
					RelatedGroups: []analytics.RelatedGroup{
						{Kind: "concept", Label: "clumsy in social situations", Members: []string{"gaucherie"}},
						{Kind: "antonym", Label: "rightness — right", Members: []string{"dexter (Latin) — right hand", "droit (French) — right hand"}},
					},
				},
				{
					NotebookID:    "word-power-made-easy",
					NotebookTitle: "Session 3",
					Expression:    "logos",
					QuizType:      "etymology_breakdown",
					Meaning:       "science, study",
					NotebookKind:  "etymology",
				},
				{
					NotebookID:    "word-power-made-easy",
					NotebookTitle: "Session 5",
					Expression:    "obstetrics",
					QuizType:      "notebook",
					Meaning:       "the medical specialty dealing with childbirth",
					NotebookKind:  "story",
				},
				{
					NotebookID:      "more-speak-english-like-an-american",
					NotebookTitle:   "LESSON 7: CINDY ASKS MARK TO GET BACK TOGETHER",
					Expression:      "stuffed shirt",
					QuizType:        "notebook",
					Meaning:         "a self-important formal person",
					ExampleSentence: "I've dated a lot of losers lately: stuffed shirts, two-timers — you get the picture.",
					NotebookKind:    "story",
				},
			},
		},
	}
	writer := NewWriter(repo)
	tmpDir := t.TempDir()
	written, err := writer.Output(context.Background(), day, tmpDir, false)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmpDir, "quiz-review-2026-06-16.md"), written,
		"single combined file lives directly under the output directory, named with the date")

	body, err := os.ReadFile(written)
	require.NoError(t, err)
	out := string(body)

	// Top-of-file summary covers every notebook.
	assert.Contains(t, out, "# Quiz review — 2026-06-16")
	assert.Contains(t, out, "4 wrong attempts across 2 notebooks.",
		"top summary counts every wrong attempt across every notebook on the day")

	// Each notebook is a top-level section, in first-appearance order.
	assert.Contains(t, out, "## word-power-made-easy")
	assert.Contains(t, out, "## more-speak-english-like-an-american")
	assert.Less(t, indexOf(out, "## word-power-made-easy"), indexOf(out, "## more-speak-english-like-an-american"),
		"notebooks render in first-appearance order — WPME comes first because its first wrong attempt was first in the day detail")

	// Per-notebook summary nested under each H2.
	assert.Contains(t, out, "3 wrong attempts across 2 sessions.",
		"per-notebook summary covers the entries inside that notebook only")
	assert.Contains(t, out, "1 wrong attempt across 1 session.",
		"the second notebook's summary exercises the singular pluralisation path")

	// Sessions sit one level deeper (### instead of ##).
	assert.Contains(t, out, "### Session 3")
	assert.Contains(t, out, "### Session 5")
	assert.Contains(t, out, "### LESSON 7: CINDY ASKS MARK TO GET BACK TOGETHER")

	// Failed-origins and failed-vocabularies blocks pushed to ####.
	assert.Contains(t, out, "#### Failed origins")
	assert.Contains(t, out, "#### Failed vocabularies")
	assert.Contains(t, out, "- **logos** [etymology breakdown]: science, study")
	assert.Contains(t, out, "- **gauche** [vocab]: clumsy, tactless, especially in social situations")
	assert.Contains(t, out, "    - Same sense (clumsy in social situations): gaucherie")
	assert.Contains(t, out, "    - Antonym (rightness — right): dexter (Latin) — right hand, droit (French) — right hand")

	// Notebook sections are separated by a horizontal rule so a reader
	// scrolling through the file gets a clear cut between notebooks.
	assert.Contains(t, out, "\n---\n", "horizontal rule separates each notebook section")

	// Stuffed-shirt example renders italic in the speak-english section.
	assert.Contains(t, out,
		"    - Example: *I've dated a lot of losers lately: stuffed shirts, two-timers — you get the picture.*",
		"example sentence still renders inside the entry body as italic")
}

// TestWriter_RendersStoryConversations pins the story-side context
// block. The writer must:
//
//   - mark the line containing the failed expression with a ✗ prefix
//     and bold the matched span (root + conjugation: "scrimp" inside
//     "scrimping" lights up the whole word, not just the root);
//   - drop scenes that don't contain any failure so a 19-line lesson
//     with one wrong word doesn't render all 19 lines;
//   - leave non-matching lines inside a matched scene as plain
//     dialogue — context for the bolded line;
//   - suppress the Example: line in the per-failure entry because the
//     conversation block already carries the same quote.
func TestWriter_RendersStoryConversations(t *testing.T) {
	day, _ := time.Parse("2006-01-02", "2026-06-17")
	repo := &stubRepo{
		detail: analytics.DayDetail{
			WrongWords: []analytics.WrongWord{
				{
					NotebookID:      "speak-english",
					NotebookTitle:   "LESSON 18",
					Expression:      "scrimp",
					QuizType:        "notebook",
					Meaning:         "to economize",
					ExampleSentence: "They're probably scrimping on the plastic.",
					NotebookKind:    "story",
				},
			},
		},
	}
	src := &stubSource{
		conversations: map[string][]SourceScene{
			"speak-english|LESSON 18": {
				{
					Lines: []SourceLine{
						{Speaker: "Meg", Quote: "What's going on with the CD cases? They keep cracking."},
						{Speaker: "Josh", Quote: "Our supplier must be cutting corners."},
						{Speaker: "Meg", Quote: "They're probably scrimping on the plastic. I'll have a word with them."},
						{Speaker: "Josh", Quote: "Good idea."},
					},
				},
				{
					// Unrelated scene — every line is just context, no
					// failed expression mentioned. Must be omitted.
					Lines: []SourceLine{
						{Speaker: "Meg", Quote: "Hi Gary, the cases you sent keep cracking."},
						{Speaker: "Gary", Quote: "That's strange. Try storing them differently."},
					},
				},
			},
		},
	}
	writer := NewWriterWithSource(repo, src)
	tmpDir := t.TempDir()
	written, err := writer.Output(context.Background(), day, tmpDir, false)
	require.NoError(t, err)
	body, err := os.ReadFile(written)
	require.NoError(t, err)
	out := string(body)

	assert.Contains(t, out, "#### Conversations")
	assert.Contains(t, out, "- ✗ _Meg_: They're probably **scrimping** on the plastic. I'll have a word with them.",
		"failed line is a bullet item with italic speaker (matching the existing story-notebook template) and bolds the whole word containing the root")
	assert.Contains(t, out, "- _Josh_: Our supplier must be cutting corners.",
		"non-failed lines render as plain bullet items with italic speaker")
	assert.NotContains(t, out, "> _Meg_",
		"conversation lines do not use the blockquote `> ` prefix — the PDF preprocessor strips **bold** from blockquote lines, so the bullet+italic shape from story-notebook.md.go.tmpl is reused instead")
	// The second scene mentions cracking + storing — neither matches
	// "scrimp" or any significant token of it, so it must be dropped.
	assert.NotContains(t, out, "Hi Gary",
		"scenes with zero failure matches must be omitted entirely so the file stays scannable")

	// Example: line in the entry suppressed because the conversation
	// block already carried the same quote.
	assert.NotContains(t, out, "- Example:",
		"Example: line is redundant when the conversation block already shows the quote — it must be suppressed")

	// The conversation block precedes the failed-vocabularies block.
	assert.Less(t,
		indexOf(out, "#### Conversations"),
		indexOf(out, "#### Failed vocabularies"),
		"conversation context must appear before the failure list so the reader has the dialogue framing the failed word")
}

// TestWriter_BoldingHandlesTokenFalsePositives pins the regression where
// "take" — a content token of "take the plunge" — was matched inside
// "mistake" and falsely marked an unrelated dialogue line. Word-start
// anchoring on the token fallback prevents the substring-inside-word
// hit.
func TestWriter_BoldingHandlesTokenFalsePositives(t *testing.T) {
	day, _ := time.Parse("2006-01-02", "2026-06-17")
	repo := &stubRepo{
		detail: analytics.DayDetail{
			WrongWords: []analytics.WrongWord{
				{
					NotebookID:    "speak-english",
					NotebookTitle: "LESSON 10",
					Expression:    "take the plunge",
					QuizType:      "notebook",
					Meaning:       "to start something risky",
					NotebookKind:  "story",
				},
			},
		},
	}
	src := &stubSource{
		conversations: map[string][]SourceScene{
			"speak-english|LESSON 10": {{
				Lines: []SourceLine{
					{Speaker: "Susan", Quote: "I'm ready to take the plunge and join a start-up."},
					{Speaker: "Craig", Quote: "I still think you're making a mistake leaving King."},
				},
			}},
		},
	}
	writer := NewWriterWithSource(repo, src)
	tmpDir := t.TempDir()
	written, err := writer.Output(context.Background(), day, tmpDir, false)
	require.NoError(t, err)
	body, err := os.ReadFile(written)
	require.NoError(t, err)
	out := string(body)
	assert.Contains(t, out, "- ✗ _Susan_: I'm ready to **take the plunge** and join a start-up.",
		"exact-substring match bolds the full expression and marks the line")
	assert.NotContains(t, out, "✗ **Craig:**",
		"the second line must NOT be marked — 'take' inside 'mistake' must not light up as a token match")
	assert.NotContains(t, out, "**mistake**",
		"'mistake' must not be bolded by accident from the 'take' token of 'take the plunge'")
}

// TestWriter_TokenFallbackBoldsContentWord pins the multi-word case
// where the exact expression doesn't substring-match the quote ("drum
// up business" vs "drum up a lot of business"): the writer should fall
// back to the longest content token and bold the matching word.
func TestWriter_TokenFallbackBoldsContentWord(t *testing.T) {
	day, _ := time.Parse("2006-01-02", "2026-06-17")
	repo := &stubRepo{
		detail: analytics.DayDetail{
			WrongWords: []analytics.WrongWord{
				{
					NotebookID:    "speak-business",
					NotebookTitle: "LESSON 6",
					Expression:    "drum up business",
					QuizType:      "notebook",
					Meaning:       "to create new business",
					NotebookKind:  "story",
				},
			},
		},
	}
	src := &stubSource{
		conversations: map[string][]SourceScene{
			"speak-business|LESSON 6": {{
				Lines: []SourceLine{
					{Speaker: "Linda", Quote: "Linda, your campaign helped us drum up a lot of business."},
				},
			}},
		},
	}
	writer := NewWriterWithSource(repo, src)
	tmpDir := t.TempDir()
	written, err := writer.Output(context.Background(), day, tmpDir, false)
	require.NoError(t, err)
	body, err := os.ReadFile(written)
	require.NoError(t, err)
	out := string(body)
	// The exact phrase doesn't appear in the quote — the token
	// fallback bolds the longest content word (here "business",
	// 8 chars > "drum", 4 chars).
	assert.Contains(t, out, "- ✗ _Linda_:",
		"line carries the ✗ marker even when the exact phrase isn't a substring")
	assert.Contains(t, out, "**business**",
		"the longest content token is bolded as a whole word")
}

// TestWriter_BoldsConjugatedFormViaSourceVocabulary pins the regression
// where the failure was recorded under the dictionary form ("give
// someone the runaround") but the dialogue uses the conjugated form
// ("giving me the runaround"). The YAML stores `expression: giving me
// the runaround` + `definition: give someone the runaround`; the
// writer must look the pair up via SourceContent.VocabularyForSession
// and bold the CONJUGATED form so the user can spot the phrase in the
// dialogue, not just one isolated token of it.
func TestWriter_BoldsConjugatedFormViaSourceVocabulary(t *testing.T) {
	day, _ := time.Parse("2006-01-02", "2026-06-17")
	repo := &stubRepo{
		detail: analytics.DayDetail{
			WrongWords: []analytics.WrongWord{
				{
					NotebookID:    "speak-english",
					NotebookTitle: "LESSON 8",
					Expression:    "give someone the runaround",
					QuizType:      "notebook",
					Meaning:       "to lead someone along without giving them what they want",
					NotebookKind:  "story",
				},
			},
		},
	}
	src := &stubSource{
		conversations: map[string][]SourceScene{
			"speak-english|LESSON 8": {{
				Lines: []SourceLine{
					{Speaker: "Mark", Quote: "I feel like you're giving me the runaround."},
				},
			}},
		},
		vocabulary: map[string][]VocabularyPair{
			"speak-english|LESSON 8": {
				{
					Expression: "giving me the runaround",
					Definition: "give someone the runaround",
				},
			},
		},
	}
	writer := NewWriterWithSource(repo, src)
	tmpDir := t.TempDir()
	written, err := writer.Output(context.Background(), day, tmpDir, false)
	require.NoError(t, err)
	body, err := os.ReadFile(written)
	require.NoError(t, err)
	out := string(body)
	assert.Contains(t, out, "**giving me the runaround**",
		"the YAML's expression field carries the conjugated form; matching against it lets the writer bold the whole phrase, not just one token")
	assert.Contains(t, out, "- ✗ _Mark_:",
		"line still gets the ✗ marker")
}

// TestWriter_EscapesStrayAsterisksInQuotes pins the regression where a
// stray `*` in the source text (footnote marker like "losers*") opened
// an italic span that swallowed the trailing `**bold**` for the failed
// expression. The escape pass turns the lone `*` into `\*` so the
// bold survives the markdown parser end to end.
func TestWriter_EscapesStrayAsterisksInQuotes(t *testing.T) {
	day, _ := time.Parse("2006-01-02", "2026-06-17")
	repo := &stubRepo{
		detail: analytics.DayDetail{
			WrongWords: []analytics.WrongWord{
				{
					NotebookID:    "speak-english",
					NotebookTitle: "LESSON 7",
					Expression:    "stuffed shirt",
					QuizType:      "notebook",
					Meaning:       "a self-important formal person",
					NotebookKind:  "story",
				},
			},
		},
	}
	src := &stubSource{
		conversations: map[string][]SourceScene{
			"speak-english|LESSON 7": {{
				Lines: []SourceLine{
					{Speaker: "Cindy", Quote: "I've dated a lot of losers* lately: stuffed shirts, two-timers."},
				},
			}},
		},
	}
	writer := NewWriterWithSource(repo, src)
	tmpDir := t.TempDir()
	written, err := writer.Output(context.Background(), day, tmpDir, false)
	require.NoError(t, err)
	body, err := os.ReadFile(written)
	require.NoError(t, err)
	out := string(body)
	assert.Contains(t, out, `losers\*`,
		"stray `*` in the source (footnote marker) must be backslash-escaped so a markdown parser doesn't read it as an italic-open")
	assert.Contains(t, out, "**stuffed shirts**",
		"the **bold** for the failed expression must survive — without the escape it gets consumed by the italic the stray `*` opens")
}

// TestWriter_PreservesExampleWhenNoConversation pins the WPME-style
// fallback: when the source has no conversations for the session, the
// Example: line stays because there's no other place the quote
// surfaces.
func TestWriter_PreservesExampleWhenNoConversation(t *testing.T) {
	day, _ := time.Parse("2006-01-02", "2026-06-17")
	repo := &stubRepo{
		detail: analytics.DayDetail{
			WrongWords: []analytics.WrongWord{
				{
					NotebookID:      "wpme",
					NotebookTitle:   "Session 5",
					Expression:      "geriatrics",
					QuizType:        "notebook",
					Meaning:         "medicine of the elderly",
					ExampleSentence: "The clinic specializes in geriatrics.",
				},
			},
		},
	}
	src := &stubSource{} // no conversations
	writer := NewWriterWithSource(repo, src)
	tmpDir := t.TempDir()
	written, err := writer.Output(context.Background(), day, tmpDir, false)
	require.NoError(t, err)
	body, err := os.ReadFile(written)
	require.NoError(t, err)
	out := string(body)
	assert.NotContains(t, out, "#### Conversations")
	assert.Contains(t, out, "- Example: *The clinic specializes in geriatrics.*",
		"Example: line stays when the source has no conversations — it's the only carrier of the usage")
}

// TestWriter_RendersEtymologyConceptsWithHighlight pins the etymology
// context block: when a session has concepts/relations, they render as
// a table next to the failure list, and members whose origin name
// matches any failed expression in that session (origin OR vocab) get
// a ✗ marker so the reader's eye lands on the wrong card.
func TestWriter_RendersEtymologyConceptsWithHighlight(t *testing.T) {
	day, _ := time.Parse("2006-01-02", "2026-06-17")
	repo := &stubRepo{
		detail: analytics.DayDetail{
			WrongWords: []analytics.WrongWord{
				{
					NotebookID:    "wpme",
					NotebookTitle: "Session 3",
					Expression:    "gauche",
					QuizType:      "notebook",
					Meaning:       "clumsy, tactless, especially in social situations",
					NotebookKind:  "story",
				},
			},
		},
	}
	src := &stubSource{
		concepts: map[string][]notebook.Concept{
			"wpme|Session 3": {
				{
					Key:     "leftness",
					Meaning: "left",
					Note:    "historically pejorative",
					Members: []notebook.ConceptMember{
						{Origin: "sinister", Language: "Latin"},
						{Origin: "gauche", Language: "French"},
					},
				},
				{
					Key:     "rightness",
					Meaning: "right",
					Members: []notebook.ConceptMember{
						{Origin: "dexter", Language: "Latin"},
						{Origin: "droit", Language: "French"},
					},
				},
			},
		},
		relations: map[string][]notebook.Relation{
			"wpme|Session 3": {
				{Type: "antonym", Between: []string{"leftness", "rightness"}},
			},
		},
		meanings: map[string]map[string]string{
			"wpme|Session 3": {
				"sinister": "left hand",
				"gauche":   "left hand",
				"dexter":   "right hand",
				"droit":    "right hand",
			},
		},
	}
	writer := NewWriterWithSource(repo, src)
	tmpDir := t.TempDir()
	written, err := writer.Output(context.Background(), day, tmpDir, false)
	require.NoError(t, err)
	body, err := os.ReadFile(written)
	require.NoError(t, err)
	out := string(body)

	assert.Contains(t, out, "#### Concepts")
	assert.Contains(t, out, "**leftness — left**", "concept header carries the key + umbrella meaning")
	assert.Contains(t, out, "_historically pejorative_", "concept note renders as italic")
	assert.Contains(t, out, "| Member | Language | Meaning |", "concept members render as a markdown table")
	assert.Contains(t, out, "| ✗ gauche | French | left hand |",
		"the failed expression's origin row gets a ✗ marker so the reader sees the failure inside the concept block")
	assert.Contains(t, out, "| sinister | Latin | left hand |",
		"non-failed members render without the marker")
	assert.Contains(t, out, "Relations: antonym ↔ rightness",
		"symmetric relations surface on both endpoints")
	// The concept block precedes the failure list for the same session.
	assert.Less(t,
		indexOf(out, "#### Concepts"),
		indexOf(out, "#### Failed vocabularies"),
		"concept context must precede the failure list within a session")
}

// TestWriter_ConceptsFilteredToFailuresOnly pins the noise-reduction
// rule the user asked for: a session can declare many concepts (Session
// 9 of WPME has nine), but quiz-review must render only the ones whose
// members touch a failure on this day. Concepts unrelated to the
// failure are dropped entirely — the user reading the file shouldn't
// scroll past a wall of irrelevant tables to find the one concept the
// failed origin belongs to.
func TestWriter_ConceptsFilteredToFailuresOnly(t *testing.T) {
	day, _ := time.Parse("2006-01-02", "2026-06-17")
	repo := &stubRepo{
		detail: analytics.DayDetail{
			WrongWords: []analytics.WrongWord{
				{
					NotebookID:    "wpme",
					NotebookTitle: "Session 9",
					Expression:    "orthos",
					QuizType:      "etymology_breakdown",
					Meaning:       "straight, correct",
					NotebookKind:  "etymology",
				},
			},
		},
	}
	src := &stubSource{
		concepts: map[string][]notebook.Concept{
			"wpme|Session 9": {
				// Concept the failed origin belongs to — MUST render.
				{
					Key:     "straightness",
					Meaning: "straight",
					Members: []notebook.ConceptMember{
						{Origin: "orthos", Language: "Greek"},
					},
				},
				// Concept on a different sense — MUST drop.
				{
					Key:     "body-part",
					Meaning: "parts of the body",
					Members: []notebook.ConceptMember{
						{Origin: "osteon", Language: "Greek"},
						{Origin: "cheir", Language: "Greek"},
					},
				},
				// Concept on a third sense — MUST drop.
				{
					Key:     "measured-quantity",
					Meaning: "physical quantities subject to measurement",
					Members: []notebook.ConceptMember{
						{Origin: "therme", Language: "Greek"},
						{Origin: "baros", Language: "Greek"},
					},
				},
			},
		},
	}
	writer := NewWriterWithSource(repo, src)
	tmpDir := t.TempDir()
	written, err := writer.Output(context.Background(), day, tmpDir, false)
	require.NoError(t, err)
	body, err := os.ReadFile(written)
	require.NoError(t, err)
	out := string(body)
	assert.Contains(t, out, "**straightness — straight**",
		"the concept whose member matches the failed origin must render with the ✗ marker")
	assert.Contains(t, out, "| ✗ orthos | Greek |",
		"failed origin is highlighted inside the rendered concept's members table")
	assert.NotContains(t, out, "body-part",
		"concept unrelated to the failure must be dropped entirely — quiz-review focuses on what the user missed")
	assert.NotContains(t, out, "measured-quantity",
		"every concept without a failed-member must be dropped, not just some")
}

// TestWriter_ConceptsRenderForVocabFailureViaOriginParts pins the
// origin-parts expansion: a vocab failure like "gynecology" doesn't
// share its name with any concept member, but its origin_parts list
// includes "gyne" — and "gyne" IS a concept member. The filter looks
// up VocabularyForSession and treats every origin_part of a failed
// vocab as a touched concept member, so the relevant etymology concept
// still surfaces.
func TestWriter_ConceptsRenderForVocabFailureViaOriginParts(t *testing.T) {
	day, _ := time.Parse("2006-01-02", "2026-06-17")
	repo := &stubRepo{
		detail: analytics.DayDetail{
			WrongWords: []analytics.WrongWord{
				{
					NotebookID:    "wpme",
					NotebookTitle: "Session 5",
					Expression:    "gynecology",
					QuizType:      "notebook",
					Meaning:       "the medical science of women",
					NotebookKind:  "story",
				},
			},
		},
	}
	src := &stubSource{
		concepts: map[string][]notebook.Concept{
			"wpme|Session 5": {
				{
					Key:     "woman",
					Meaning: "woman",
					Members: []notebook.ConceptMember{
						{Origin: "gyne", Language: "Greek"},
					},
				},
				{
					Key:     "eye",
					Meaning: "eye",
					Members: []notebook.ConceptMember{
						{Origin: "ophthalmos", Language: "Greek"},
					},
				},
			},
		},
		vocabulary: map[string][]VocabularyPair{
			"wpme|Session 5": {
				{
					Expression:  "gynecology",
					OriginNames: []string{"gyne", "logos"},
				},
			},
		},
	}
	writer := NewWriterWithSource(repo, src)
	tmpDir := t.TempDir()
	written, err := writer.Output(context.Background(), day, tmpDir, false)
	require.NoError(t, err)
	body, err := os.ReadFile(written)
	require.NoError(t, err)
	out := string(body)
	assert.Contains(t, out, "**woman — woman**",
		"the concept containing 'gyne' (an origin_part of the failed 'gynecology') must surface")
	assert.Contains(t, out, "| ✗ gyne | Greek |",
		"the origin_part itself is marked because the vocab that depends on it was failed")
	assert.NotContains(t, out, "**eye — eye**",
		"the unrelated eye concept must be dropped — the failed gynecology doesn't touch ophthalmos")
}

// TestWriter_NoSourceContent_RendersLegacyFailureOnlyOutput pins the
// no-context fallback: when the writer is constructed via NewWriter
// (no source), the markdown reverts to the failure-list-only layout —
// no Conversations block, no Concepts block — so existing callers and
// tests that don't wire a SourceContent still produce stable output.
func TestWriter_NoSourceContent_RendersLegacyFailureOnlyOutput(t *testing.T) {
	day, _ := time.Parse("2006-01-02", "2026-06-17")
	repo := &stubRepo{
		detail: analytics.DayDetail{
			WrongWords: []analytics.WrongWord{
				{
					NotebookID:    "wpme",
					NotebookTitle: "Session 3",
					Expression:    "gauche",
					QuizType:      "notebook",
					Meaning:       "clumsy",
				},
			},
		},
	}
	writer := NewWriter(repo)
	tmpDir := t.TempDir()
	written, err := writer.Output(context.Background(), day, tmpDir, false)
	require.NoError(t, err)
	body, err := os.ReadFile(written)
	require.NoError(t, err)
	out := string(body)
	assert.NotContains(t, out, "#### Conversations")
	assert.NotContains(t, out, "#### Concepts")
	assert.Contains(t, out, "#### Failed vocabularies", "failure list still renders")
}

// TestWriter_FiltersOutBookNotebooks pins the book-exclusion rule:
// quiz-review is a study sheet for the failed vocabulary / origin
// quizzes, so failures on full-book sources (Gatsby, John Tenniel,
// loaded from books_directories) must not appear in the output. A day
// with failures on a study notebook AND a book renders only the study
// notebook; a day where every failure was on a book produces no file.
func TestWriter_FiltersOutBookNotebooks(t *testing.T) {
	day, _ := time.Parse("2006-01-02", "2026-06-17")
	repo := &stubRepo{
		detail: analytics.DayDetail{
			WrongWords: []analytics.WrongWord{
				{
					NotebookID:    "the-great-gatsby",
					NotebookTitle: "Chapter 1",
					Expression:    "bond business",
					QuizType:      "notebook",
					Meaning:       "the bond market",
				},
				{
					NotebookID:    "word-power-made-easy",
					NotebookTitle: "Session 3",
					Expression:    "gauche",
					QuizType:      "notebook",
					Meaning:       "clumsy",
				},
			},
		},
	}
	src := &stubSource{
		bookIDs: map[string]bool{"the-great-gatsby": true},
	}
	writer := NewWriterWithSource(repo, src)
	tmpDir := t.TempDir()
	written, err := writer.Output(context.Background(), day, tmpDir, false)
	require.NoError(t, err)
	body, err := os.ReadFile(written)
	require.NoError(t, err)
	out := string(body)
	assert.NotContains(t, out, "the-great-gatsby",
		"book notebooks (loaded from books_directories) must not appear in quiz-review")
	assert.NotContains(t, out, "bond business",
		"failures on book notebooks must be dropped end-to-end")
	assert.Contains(t, out, "word-power-made-easy",
		"study notebooks on the same day still render normally")
	assert.Contains(t, out, "gauche")
	assert.Contains(t, out, "1 wrong attempt across 1 notebook.",
		"top summary reflects only the surviving (non-book) notebooks")

	// All-book day → no file at all (mirrors the no-failures path).
	bookOnlyRepo := &stubRepo{
		detail: analytics.DayDetail{
			WrongWords: []analytics.WrongWord{
				{NotebookID: "the-great-gatsby", Expression: "x", QuizType: "notebook"},
			},
		},
	}
	bookOnlyWriter := NewWriterWithSource(bookOnlyRepo, src)
	written2, err := bookOnlyWriter.Output(context.Background(), day, t.TempDir(), false)
	require.NoError(t, err)
	assert.Empty(t, written2,
		"a day where every failure was on a book produces no quiz-review file")
}

// TestWriter_NoWrongAttemptsReturnsEmpty pins the no-op for days with no
// activity: no file is written, no error is raised. The CLI surfaces a
// friendly "nothing to write" line off the empty result.
func TestWriter_NoWrongAttemptsReturnsEmpty(t *testing.T) {
	repo := &stubRepo{detail: analytics.DayDetail{}}
	writer := NewWriter(repo)
	tmpDir := t.TempDir()
	written, err := writer.Output(context.Background(), time.Now(), tmpDir, false)
	require.NoError(t, err)
	assert.Empty(t, written)
	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "no file should be created when there is nothing to write")
}

// TestWriter_RejectsEmptyOutputDirectory guards against silently
// writing to the working directory when the config omits the output
// path.
func TestWriter_RejectsEmptyOutputDirectory(t *testing.T) {
	writer := NewWriter(&stubRepo{detail: analytics.DayDetail{WrongWords: []analytics.WrongWord{{NotebookID: "x"}}}})
	_, err := writer.Output(context.Background(), time.Now(), "", false)
	require.Error(t, err)
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
