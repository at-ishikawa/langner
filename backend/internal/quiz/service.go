package quiz

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// Service owns all quiz business logic shared between the CLI and RPC handler.
type Service struct {
	notebooksConfig    config.NotebooksConfig
	openaiClient       inference.Client
	dictionaryMap      map[string]rapidapi.Response
	learningRepository learning.LearningRepository
	historyStore       learning.HistoryStore
	originRepo         notebook.EtymologyOriginRepository
	skipFlagRepo       notebook.SkipFlagRepository
	noteRepo           notebook.NoteRepository
	calculator         notebook.IntervalCalculator
	disableShuffle     bool
}

// NewService creates a new Service.
// learningRepo / historyStore / originRepo / skipFlagRepo are optional in
// pure unit tests; the runtime always wires them up via the bootstrap.
// When historyStore is nil the legacy YAML loader is used as a fallback
// so the interactive CLI commands keep working without a DB.
func NewService(notebooksConfig config.NotebooksConfig, openaiClient inference.Client, dictionaryMap map[string]rapidapi.Response, learningRepo learning.LearningRepository, quizCfg config.QuizConfig) *Service {
	return &Service{
		notebooksConfig:    notebooksConfig,
		openaiClient:       openaiClient,
		dictionaryMap:      dictionaryMap,
		learningRepository: learningRepo,
		calculator:         notebook.NewIntervalCalculator(quizCfg.Algorithm, quizCfg.FixedIntervals),
		disableShuffle:     quizCfg.DisableShuffle,
	}
}

// WithDBState wires the DB-backed history store + supporting repos.
// Bootstrap calls it after constructing them. Service falls back to the
// YAML loader for any call where these aren't set.
func (s *Service) WithDBState(historyStore learning.HistoryStore, originRepo notebook.EtymologyOriginRepository, skipFlagRepo notebook.SkipFlagRepository, noteRepo notebook.NoteRepository) *Service {
	s.historyStore = historyStore
	s.originRepo = originRepo
	s.skipFlagRepo = skipFlagRepo
	s.noteRepo = noteRepo
	return s
}

// loadHistories returns the per-notebook LearningHistory map either from
// the DB-backed store (when wired) or from the legacy YAML directory.
// All in-package callers go through this helper so the cutover from
// YAML to DB is a single seam. Uses context.Background internally so
// existing Service methods don't have to grow a ctx parameter just for
// this — handlers that need real cancellation already pass ctx to
// SaveResult / GradeAnswer etc.
func (s *Service) loadHistories() (map[string][]notebook.LearningHistory, error) {
	if s.historyStore != nil {
		return s.historyStore.LoadAll(context.Background())
	}
	return notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
}

func (s *Service) newReader() (*notebook.Reader, error) {
	return notebook.NewReader(
		s.notebooksConfig.StoriesDirectories,
		s.notebooksConfig.FlashcardsDirectories,
		s.notebooksConfig.BooksDirectories,
		s.notebooksConfig.DefinitionsDirectories,
		s.notebooksConfig.EtymologyDirectories,
		s.dictionaryMap,
	)
}

// NewReader creates a new notebook reader. Exported for use by handlers
// that need to pass a reader to multiple service methods.
func (s *Service) NewReader() (*notebook.Reader, error) {
	return s.newReader()
}

// LoadNotebookSummaries returns all available notebooks with their review counts.
//
// includeUnstudied, when true, makes ReviewCount and ReverseReviewCount
// (per notebook and per section) reflect what the actual quiz will load
// when "Include unstudied words" is on — never-seen words AND words
// still within their SR interval. When false (default), counts are
// due-only, matching the conservative default the quiz uses without
// the toggle.
func (s *Service) LoadNotebookSummaries(includeUnstudied bool) ([]NotebookSummary, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize notebook reader: %w", err)
	}

	learningHistories, err := s.loadHistories()
	if err != nil {
		return nil, fmt.Errorf("failed to load learning histories: %w", err)
	}

	var summaries []NotebookSummary

	for id, index := range reader.GetStoryIndexes() {
		stories, err := reader.ReadStoryNotebooks(id)
		if err != nil {
			return nil, fmt.Errorf("failed to read story notebook %q: %w", id, err)
		}

		filtered, err := notebook.FilterStoryNotebooks(
			stories, learningHistories[id], s.dictionaryMap,
			false, includeUnstudied, true, false, notebook.QuizTypeNotebook,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to filter story notebook %q: %w", id, err)
		}

		var latestDate time.Time
		for _, s := range stories {
			if s.Date.After(latestDate) {
				latestDate = s.Date
			}
		}
		reverseCount := countReverseStoryDefinitions(stories, learningHistories[id], includeUnstudied)
		etymCount := countStoryEtymologyDefinitions(stories)
		summaries = append(summaries, NotebookSummary{
			NotebookID:           id,
			Name:                 index.Name,
			ReviewCount:          countStoryDefinitions(filtered),
			ReverseReviewCount:   reverseCount,
			EtymologyReviewCount: etymCount,
			LatestDate:           latestDate,
			Kind:                 kindFromIndex(index),
			HasContent:           storyHasContent(stories),
			Sections:             storySectionSummaries(stories, filtered, learningHistories[id], includeUnstudied),
		})
	}

	for id, index := range reader.GetFlashcardIndexes() {
		notebooks, err := reader.ReadFlashcardNotebooks(id)
		if err != nil {
			return nil, fmt.Errorf("failed to read flashcard notebook %q: %w", id, err)
		}

		// Summary counts pass `includeUnstudied` through to the filter so
		// the "due" number on the quiz start page matches what the
		// standard quiz will load when the user has the "Include
		// unstudied words" toggle on. The frontend re-fetches with
		// includeUnstudied=true when the toggle flips.
		filtered, err := notebook.FilterFlashcardNotebooks(
			notebooks, learningHistories[id], s.dictionaryMap, false, includeUnstudied, notebook.QuizTypeNotebook,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to filter flashcard notebook %q: %w", id, err)
		}

		reverseCount := countReverseFlashcardCards(notebooks, learningHistories[id], includeUnstudied)
		etymCount := countFlashcardEtymologyCards(notebooks)
		var latestDate time.Time
		for _, n := range notebooks {
			if n.Date.After(latestDate) {
				latestDate = n.Date
			}
		}
		summaries = append(summaries, NotebookSummary{
			NotebookID:            id,
			Name:                  index.Name,
			ReviewCount:           countFlashcardCards(filtered),
			ReverseReviewCount:    reverseCount,
			EtymologyReviewCount:  etymCount,
			LatestDate:            latestDate,
			Sections:              flashcardSectionSummaries(notebooks, filtered, learningHistories[id], includeUnstudied),
		})
	}

	// Add definitions-only books (not already in story or flashcard indexes)
	storyIndexes := reader.GetStoryIndexes()
	flashcardIndexes := reader.GetFlashcardIndexes()
	for _, nbID := range reader.GetDefinitionsBookIDs() {
		if _, isStory := storyIndexes[nbID]; isStory {
			continue
		}
		if _, isFlashcard := flashcardIndexes[nbID]; isFlashcard {
			continue
		}
		defs, ok := reader.GetDefinitionsNotesByTitle(nbID)
		if !ok {
			continue
		}
		conceptHeads := definitionConceptHeads(reader, nbID)
		reviewCount := countDefinitionNotes(defs, learningHistories[nbID], false, includeUnstudied, conceptHeads)
		reverseCount := countDefinitionNotes(defs, learningHistories[nbID], true, includeUnstudied, conceptHeads)
		if reviewCount == 0 && reverseCount == 0 {
			continue
		}
		summaries = append(summaries, NotebookSummary{
			NotebookID:         nbID,
			Name:               nbID,
			ReviewCount:        reviewCount,
			ReverseReviewCount: reverseCount,
			Kind:               "Books",
			LatestDate:         reader.GetDefinitionsLatestDate(nbID),
			Sections:           definitionsSectionSummaries(defs, learningHistories[nbID], includeUnstudied, conceptHeads),
		})
	}

	// Add etymology notebooks
	etymSummaries, err := s.LoadEtymologyNotebookSummaries(includeUnstudied)
	if err != nil {
		return nil, fmt.Errorf("failed to load etymology notebook summaries: %w", err)
	}
	summaries = append(summaries, etymSummaries...)

	return summaries, nil
}

// buildOriginMap builds a map of origin|language -> EtymologyOrigin from all etymology notebooks.
func buildOriginMap(reader *notebook.Reader) map[string]notebook.EtymologyOrigin {
	originMap := make(map[string]notebook.EtymologyOrigin)
	for id := range reader.GetEtymologyIndexes() {
		origins, err := reader.ReadEtymologyNotebook(id)
		if err != nil {
			continue
		}
		for _, o := range origins {
			key := strings.ToLower(o.Origin + "|" + o.Language)
			originMap[key] = o
		}
	}
	return originMap
}

// resolveOriginParts resolves OriginPartRef references to full WordOriginPart data.
func resolveOriginParts(refs []notebook.OriginPartRef, originMap map[string]notebook.EtymologyOrigin) []WordOriginPart {
	if len(refs) == 0 || len(originMap) == 0 {
		return nil
	}
	var parts []WordOriginPart
	for _, ref := range refs {
		key := strings.ToLower(ref.Origin + "|" + ref.Language)
		if o, ok := originMap[key]; ok {
			parts = append(parts, WordOriginPart{Origin: o.Origin, Type: o.Type, Language: o.Language, Meaning: o.Meaning})
		} else {
			// Try matching by origin only
			for k, o := range originMap {
				if strings.HasPrefix(k, strings.ToLower(ref.Origin)+"|") {
					parts = append(parts, WordOriginPart{Origin: o.Origin, Type: o.Type, Language: o.Language, Meaning: o.Meaning})
					break
				}
			}
		}
	}
	return parts
}

func buildWordDetail(note *notebook.Note, originMap map[string]notebook.EtymologyOrigin) WordDetail {
	return WordDetail{
		Origin:        note.Origin,
		Pronunciation: note.Pronunciation,
		PartOfSpeech:  note.PartOfSpeech,
		Synonyms:      note.Synonyms,
		Antonyms:      note.Antonyms,
		Memo:          note.Memo,
		OriginParts:   resolveOriginParts(note.OriginParts, originMap),
	}
}

// LoadCards returns filtered quiz cards for the given notebooks.
// Returns *NotFoundError if any notebook ID does not exist.
//
// When sectionTitlesByID has an entry for a notebook, only cards from
// sections (story events for stories, sub-notebook titles for flashcards)
// matching the listed titles are returned. A nil or empty list for a
// notebook means "all sections".
func (s *Service) LoadCards(notebookIDs []string, includeUnstudied bool, sectionTitlesByID map[string][]string) ([]Card, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize notebook reader: %w", err)
	}

	learningHistories, err := s.loadHistories()
	if err != nil {
		return nil, fmt.Errorf("failed to load learning histories: %w", err)
	}

	storyIndexes := reader.GetStoryIndexes()
	flashcardIndexes := reader.GetFlashcardIndexes()
	originMap := buildOriginMap(reader)

	var cards []Card

	for _, notebookID := range notebookIDs {
		_, isStory := storyIndexes[notebookID]
		_, isFlashcard := flashcardIndexes[notebookID]
		sectionFilter := sectionTitlesByID[notebookID]

		if !isStory && !isFlashcard {
			// Try definitions-only book as fallback. The notebook exists
			// if it's in the definitions index even when no cards are
			// currently due — return empty rather than NotFound so the
			// "Include unstudied" toggle controls visibility instead of
			// the very existence of the notebook.
			if _, ok := reader.GetDefinitionsNotes(notebookID); ok {
				defCards := loadDefinitionCards(reader, notebookID, learningHistories, originMap, sectionFilter, includeUnstudied)
				cards = append(cards, defCards...)
				continue
			}
			return nil, &NotFoundError{NotebookID: notebookID}
		}

		if isStory {
			storyCards, err := s.loadStoryCards(reader, notebookID, learningHistories, includeUnstudied, originMap, sectionFilter)
			if err != nil {
				return nil, fmt.Errorf("failed to load story cards for notebook %q: %w", notebookID, err)
			}
			cards = append(cards, storyCards...)
		}

		if isFlashcard {
			flashCards, err := s.loadFlashcardCards(reader, notebookID, learningHistories, includeUnstudied, originMap, sectionFilter)
			if err != nil {
				return nil, fmt.Errorf("failed to load flashcard cards for notebook %q: %w", notebookID, err)
			}
			cards = append(cards, flashCards...)
		}
	}

	cards = deduplicateCards(cards)
	// Collapse multi-member concept cards into a single representative
	// card per concept (with members listed). Run AFTER deduplicate so a
	// concept whose head happens to appear in multiple notebooks isn't
	// split. Run BEFORE shuffle so the concept-key index stays stable.
	cards = collapseConceptCards(cards, buildAllConceptIndexes(reader, notebookIDs))
	if !s.disableShuffle {
		rand.Shuffle(len(cards), func(i, j int) {
			cards[i], cards[j] = cards[j], cards[i]
		})
	}
	return cards, nil
}

// inSectionFilter reports whether title is allowed by filter. An empty
// filter means "no filter" (all sections allowed).
func inSectionFilter(filter []string, title string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, t := range filter {
		if t == title {
			return true
		}
	}
	return false
}

func deduplicateCards(cards []Card) []Card {
	seen := make(map[string]int) // entry -> index in result
	var result []Card
	for _, card := range cards {
		key := strings.ToLower(card.Entry)
		if idx, ok := seen[key]; ok {
			// Keep the card with more examples/contexts
			if len(card.Examples) > len(result[idx].Examples) {
				result[idx] = card
			}
		} else {
			seen[key] = len(result)
			result = append(result, card)
		}
	}
	return result
}

func (s *Service) loadStoryCards(
	reader *notebook.Reader,
	notebookID string,
	learningHistories map[string][]notebook.LearningHistory,
	includeUnstudied bool,
	originMap map[string]notebook.EtymologyOrigin,
	sectionFilter []string,
) ([]Card, error) {
	stories, err := reader.ReadStoryNotebooks(notebookID)
	if err != nil {
		return nil, fmt.Errorf("failed to read story notebook %q: %w", notebookID, err)
	}

	filtered, err := notebook.FilterStoryNotebooks(
		stories, learningHistories[notebookID], s.dictionaryMap,
		false, includeUnstudied, true, false, notebook.QuizTypeNotebook,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to filter story notebook %q: %w", notebookID, err)
	}

	var cards []Card
	for _, story := range filtered {
		if !inSectionFilter(sectionFilter, story.Event) {
			continue
		}
		for _, scene := range story.Scenes {
			for _, definition := range scene.Definitions {
				entry := definition.Definition
				originalEntry := ""
				if entry == "" {
					entry = definition.Expression
				} else {
					originalEntry = definition.Expression
				}

				examples, contexts := buildFromConversations(&scene, &definition)

				cards = append(cards, Card{
					NotebookName:  notebookID,
					StoryTitle:    story.Event,
					SceneTitle:    scene.Title,
					Entry:         entry,
					OriginalEntry: originalEntry,
					Meaning:       definition.Meaning,
					Examples:      examples,
					Contexts:      contexts,
					WordDetail:    buildWordDetail(&definition, originMap),
					Images:        definition.Images,
				})
			}
		}
	}

	return cards, nil
}

func (s *Service) loadFlashcardCards(
	reader *notebook.Reader,
	notebookID string,
	learningHistories map[string][]notebook.LearningHistory,
	includeUnstudied bool,
	originMap map[string]notebook.EtymologyOrigin,
	sectionFilter []string,
) ([]Card, error) {
	notebooks, err := reader.ReadFlashcardNotebooks(notebookID)
	if err != nil {
		return nil, fmt.Errorf("failed to read flashcard notebook %q: %w", notebookID, err)
	}

	filtered, err := notebook.FilterFlashcardNotebooks(
		notebooks, learningHistories[notebookID], s.dictionaryMap, false, includeUnstudied, notebook.QuizTypeNotebook,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to filter flashcard notebook %q: %w", notebookID, err)
	}

	var cards []Card
	for _, nb := range filtered {
		if !inSectionFilter(sectionFilter, nb.Title) {
			continue
		}
		for _, card := range nb.Cards {
			entry := card.Definition
			originalEntry := ""
			if entry == "" {
				entry = card.Expression
			} else {
				originalEntry = card.Expression
			}

			var examples []Example
			for _, ex := range card.Examples {
				examples = append(examples, Example{Text: ex})
			}

			cards = append(cards, Card{
				NotebookName:  notebookID,
				StoryTitle:    "flashcards",
				Entry:         entry,
				OriginalEntry: originalEntry,
				Meaning:       card.Meaning,
				Examples:      examples,
				WordDetail:    buildWordDetail(&card, originMap),
				Images:        card.Images,
			})
		}
	}

	return cards, nil
}

// GradeNotebookAnswer grades a meaning answer and returns the result.
func (s *Service) GradeNotebookAnswer(ctx context.Context, card Card, answer string, responseTimeMs int64) (GradeResult, error) {
	results, err := s.openaiClient.AnswerMeanings(ctx, inference.AnswerMeaningsRequest{
		Expressions: []inference.Expression{
			{
				Expression:        card.Entry,
				Meaning:           answer,
				Contexts:          card.Contexts,
				IsExpressionInput: false,
				ResponseTimeMs:    responseTimeMs,
			},
		},
	})
	if err != nil {
		return GradeResult{}, fmt.Errorf("failed to grade answer: %w", err)
	}
	if len(results.Answers) == 0 {
		return GradeResult{}, fmt.Errorf("no results returned from inference")
	}

	result := results.Answers[0]
	isCorrect, reason, quality := extractAnswerResult(result)

	return GradeResult{
		Correct: isCorrect,
		Reason:  reason,
		Quality: quality,
	}, nil
}

// SaveResult updates learning history via the repository.
//
// When the card represents a concept (card.ConceptHead != ""), the log
// is written under the head expression so the result lands on the same
// row MergeConcepts consolidated into. Without this, a card whose
// surviving label happens to be a non-head member (post-collapse) would
// be saved as a fresh per-member row — silently undoing the migration
// every time a member-named concept card is graded.
func (s *Service) SaveResult(ctx context.Context, card Card, result GradeResult, responseTimeMs int64) error {
	status := "misunderstood"
	if result.Correct {
		status = "understood"
	}
	expression := card.Entry
	originalExpression := card.OriginalEntry
	if card.ConceptHead != "" {
		expression = card.ConceptHead
		originalExpression = ""
	}
	log := &learning.LearningLog{
		Status: status, LearnedAt: time.Now(), Quality: result.Quality,
		ResponseTimeMs: int(responseTimeMs), QuizType: string(notebook.QuizTypeNotebook),
		SourceNotebookID: card.NotebookName, NotebookName: card.NotebookName,
		StoryTitle: card.StoryTitle, SceneTitle: card.SceneTitle,
		Expression: expression, OriginalExpression: originalExpression,
		IsCorrect: result.Correct, LearningNotesDir: s.notebooksConfig.LearningNotesDirectory,
	}
	s.applyNextInterval(card.NotebookName, expression, notebook.QuizTypeNotebook, log)
	if err := s.learningRepository.Create(ctx, log); err != nil {
		return fmt.Errorf("save learning log for %q: %w", card.NotebookName, err)
	}
	return nil
}

// applyNextInterval mirrors what YAMLLearningRepository.Create did inside
// the in-memory updater: load the existing per-quiz-type log chain, ask
// the calculator for the new tentative record's interval, and stamp it
// onto the LearningLog before the repository write. Without this step
// the DB row's interval_days stays 0 and needsToLearn falls back to a
// count-based threshold that wrongly drops recently-correct cards.
func (s *Service) applyNextInterval(notebookName, expression string, quizType notebook.QuizType, log *learning.LearningLog) {
	if s.calculator == nil {
		return
	}
	existing := s.existingLogsForQuizType(notebookName, expression, quizType)
	tentative := notebook.LearningRecord{
		Status:         notebook.LearnedStatus(log.Status),
		LearnedAt:      notebook.NewDate(log.LearnedAt),
		Quality:        log.Quality,
		ResponseTimeMs: int64(log.ResponseTimeMs),
		QuizType:       log.QuizType,
	}
	interval, _ := s.calculator.NextIntervalForWrite(existing, tentative)
	log.IntervalDays = interval
}

// existingLogsForQuizType returns the chain of LearningRecords for
// the given expression in the given notebook, filtered to the requested
// quiz type. FindExpressionByName walks both flat (flashcard) and
// scene-based (story) histories, so this works regardless of notebook
// shape. Returns nil when no history exists yet.
func (s *Service) existingLogsForQuizType(notebookName, expression string, quizType notebook.QuizType) []notebook.LearningRecord {
	histories, err := s.loadHistories()
	if err != nil {
		return nil
	}
	updater := notebook.NewLearningHistoryUpdater(histories[notebookName], s.calculator)
	expr := updater.FindExpressionByName(expression)
	if expr == nil {
		return nil
	}
	return expr.GetLogsForQuizType(quizType)
}

// storyHasContent reports whether any scene carries prose or dialogue worth
// rendering in the content reader. Flashcards and definitions-only books
// return false because they have neither statements nor conversations.
// storySectionSummaries returns per-story counts in document order. The
// notebook-level filtered slice is regrouped by story event so we don't
// re-run FilterStoryNotebooks once per story; reverse and etymology counts
// are computed directly off the raw stories slice (those filters live in
// dedicated counters and don't share FilterStoryNotebooks' due-check).
func storySectionSummaries(
	stories []notebook.StoryNotebook,
	filtered []notebook.StoryNotebook,
	histories []notebook.LearningHistory,
	includeUnstudied bool,
) []NotebookSectionSummary {
	filteredByEvent := make(map[string][]notebook.StoryNotebook, len(filtered))
	for _, story := range filtered {
		filteredByEvent[story.Event] = append(filteredByEvent[story.Event], story)
	}

	seen := make(map[string]struct{}, len(stories))
	var sections []NotebookSectionSummary
	for _, story := range stories {
		if story.Event == "" {
			continue
		}
		if _, ok := seen[story.Event]; ok {
			continue
		}
		seen[story.Event] = struct{}{}
		one := []notebook.StoryNotebook{story}
		sections = append(sections, NotebookSectionSummary{
			Title:                story.Event,
			ReviewCount:          countStoryDefinitions(filteredByEvent[story.Event]),
			ReverseReviewCount:   countReverseStoryDefinitions(one, histories, includeUnstudied),
			EtymologyReviewCount: countStoryEtymologyDefinitions(one),
		})
	}
	return sections
}

// flashcardSectionSummaries returns per-flashcard-file counts. Each
// FlashcardNotebook's Title is the section, mirroring storySectionSummaries.
func flashcardSectionSummaries(
	notebooks []notebook.FlashcardNotebook,
	filtered []notebook.FlashcardNotebook,
	histories []notebook.LearningHistory,
	includeUnstudied bool,
) []NotebookSectionSummary {
	filteredByTitle := make(map[string][]notebook.FlashcardNotebook, len(filtered))
	for _, nb := range filtered {
		filteredByTitle[nb.Title] = append(filteredByTitle[nb.Title], nb)
	}

	seen := make(map[string]struct{}, len(notebooks))
	var sections []NotebookSectionSummary
	for _, nb := range notebooks {
		if nb.Title == "" {
			continue
		}
		if _, ok := seen[nb.Title]; ok {
			continue
		}
		seen[nb.Title] = struct{}{}
		one := []notebook.FlashcardNotebook{nb}
		sections = append(sections, NotebookSectionSummary{
			Title:                nb.Title,
			ReviewCount:          countFlashcardCards(filteredByTitle[nb.Title]),
			ReverseReviewCount:   countReverseFlashcardCards(one, histories, includeUnstudied),
			EtymologyReviewCount: countFlashcardEtymologyCards(one),
		})
	}
	return sections
}

func storyHasContent(stories []notebook.StoryNotebook) bool {
	for _, story := range stories {
		for _, scene := range story.Scenes {
			if len(scene.Statements) > 0 || len(scene.Conversations) > 0 {
				return true
			}
		}
	}
	return false
}

func countStoryDefinitions(stories []notebook.StoryNotebook) int {
	seen := make(map[string]struct{})
	for _, story := range stories {
		for _, scene := range story.Scenes {
			for _, def := range scene.Definitions {
				entry := def.Definition
				if entry == "" {
					entry = def.Expression
				}
				seen[strings.ToLower(entry)] = struct{}{}
			}
		}
	}
	return len(seen)
}

func countStoryEtymologyDefinitions(stories []notebook.StoryNotebook) int {
	count := 0
	for _, story := range stories {
		for _, scene := range story.Scenes {
			for _, def := range scene.Definitions {
				if len(def.OriginParts) > 0 {
					count++
				}
			}
		}
	}
	return count
}

func countFlashcardEtymologyCards(notebooks []notebook.FlashcardNotebook) int {
	count := 0
	for _, nb := range notebooks {
		for _, card := range nb.Cards {
			if len(card.OriginParts) > 0 {
				count++
			}
		}
	}
	return count
}

// isEligibleForReverseQuiz returns true if a note has the data needed for reverse quiz
// (shows meaning, asks for the word — requires a non-empty meaning and non-unusable level).
func isEligibleForReverseQuiz(note *notebook.Note) bool {
	return note.Meaning != "" && note.Level != notebook.ExpressionLevelUnusable
}

func countReverseStoryDefinitions(stories []notebook.StoryNotebook, histories []notebook.LearningHistory, includeUnstudied bool) int {
	seen := make(map[string]struct{})
	for _, story := range stories {
		for _, scene := range story.Scenes {
			for i := range scene.Definitions {
				def := &scene.Definitions[i]
				if !isEligibleForReverseQuiz(def) {
					continue
				}
				if needsReverseReview(histories, story.Event, scene.Title, def, includeUnstudied) {
					expr := def.Expression
					if def.Definition != "" {
						expr = def.Definition
					}
					seen[strings.ToLower(expr)] = struct{}{}
				}
			}
		}
	}
	return len(seen)
}

func countReverseFlashcardCards(notebooks []notebook.FlashcardNotebook, histories []notebook.LearningHistory, includeUnstudied bool) int {
	seen := make(map[string]struct{})
	for _, nb := range notebooks {
		for i := range nb.Cards {
			card := &nb.Cards[i]
			if !isEligibleForReverseQuiz(card) {
				continue
			}
			if needsReverseFlashcardReview(histories, nb.Title, card, includeUnstudied) {
				expr := card.Expression
				if card.Definition != "" {
					expr = card.Definition
				}
				seen[strings.ToLower(expr)] = struct{}{}
			}
		}
	}
	return len(seen)
}

func countFlashcardCards(notebooks []notebook.FlashcardNotebook) int {
	seen := make(map[string]struct{})
	for _, nb := range notebooks {
		for _, card := range nb.Cards {
			entry := card.Definition
			if entry == "" {
				entry = card.Expression
			}
			seen[strings.ToLower(entry)] = struct{}{}
		}
	}
	return len(seen)
}

func buildFromConversations(scene *notebook.StoryScene, definition *notebook.Note) ([]Example, []inference.Context) {
	var examples []Example
	var contexts []inference.Context
	for _, conv := range scene.Conversations {
		if conv.Quote == "" {
			continue
		}

		quoteLower := strings.ToLower(conv.Quote)
		if !containsExpression(quoteLower, definition.Expression, definition.Definition) {
			continue
		}

		cleaned := notebook.ConvertMarkersInText(conv.Quote, nil, notebook.ConversionStylePlain, "")
		examples = append(examples, Example{
			Text:    cleaned,
			Speaker: conv.Speaker,
		})
		contexts = append(contexts, inference.Context{
			Context:             cleaned,
			ReferenceDefinition: definition.Meaning,
		})
	}
	return examples, contexts
}

func containsExpression(textLower, expression, definition string) bool {
	if strings.Contains(textLower, strings.ToLower(expression)) {
		return true
	}
	if definition != "" && strings.Contains(textLower, strings.ToLower(definition)) {
		return true
	}
	return false
}

func extractAnswerResult(result inference.AnswerMeaning) (isCorrect bool, reason string, quality int) {
	if len(result.AnswersForContext) == 0 {
		return false, "", 1
	}

	first := result.AnswersForContext[0]
	isCorrect = first.Correct
	reason = first.Reason
	quality = first.Quality

	if quality == 0 {
		if isCorrect {
			quality = 4
		} else {
			quality = 1
		}
	}

	if !isCorrect && quality >= 3 {
		quality = 2
	}

	return isCorrect, reason, quality
}

// ReverseCard represents a reverse quiz card.
type ReverseCard struct {
	NotebookName string
	StoryTitle   string
	SceneTitle   string
	Meaning      string
	Contexts     []ReverseContext
	Expression   string // original expression to guess
	AltForm      string // alternate inflected form (Note.Definition when set), used for masking
	WordDetail   WordDetail
	Images       []string

	// ConceptHead, ConceptMembers, ConceptMeaning carry concept context when
	// this card represents a definitions-side concept (see Card for details).
	// Reverse quizzes prompt with ConceptMeaning and accept ANY member as a
	// correct answer.
	ConceptHead    string
	ConceptMembers []string
	ConceptMeaning string
}

// ReverseContext represents a context sentence with masking info.
type ReverseContext struct {
	Context       string
	MaskedContext string
}

// LoadReverseCards loads reverse quiz cards for the given notebooks.
//
// sectionTitlesByID narrows results to the listed sections per notebook (see
// LoadCards). A nil/empty list for a notebook means "all sections".
func (s *Service) LoadReverseCards(notebookIDs []string, listMissingContext, includeUnstudied bool, sectionTitlesByID map[string][]string) ([]ReverseCard, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize notebook reader: %w", err)
	}

	learningHistories, err := s.loadHistories()
	if err != nil {
		return nil, fmt.Errorf("failed to load learning histories: %w", err)
	}

	storyIndexes := reader.GetStoryIndexes()
	flashcardIndexes := reader.GetFlashcardIndexes()
	originMap := buildOriginMap(reader)

	var cards []ReverseCard

	for _, notebookID := range notebookIDs {
		_, isStory := storyIndexes[notebookID]
		_, isFlashcard := flashcardIndexes[notebookID]
		sectionFilter := sectionTitlesByID[notebookID]

		if !isStory && !isFlashcard {
			// Try definitions-only book as fallback. Mirror LoadCards
			// behaviour: a notebook that exists in the definitions index
			// must return whatever cards qualify (possibly zero — e.g.
			// every word is skipped or unstudied) instead of NotFound,
			// which would otherwise abort the entire multi-notebook
			// session over an empty book.
			if _, ok := reader.GetDefinitionsNotes(notebookID); ok {
				defCards := loadDefinitionReverseCards(reader, notebookID, learningHistories, originMap, sectionFilter, includeUnstudied)
				cards = append(cards, defCards...)
				continue
			}
			return nil, &NotFoundError{NotebookID: notebookID}
		}

		if isStory {
			reverseCards, err := s.loadStoryReverseCards(reader, notebookID, learningHistories, listMissingContext, includeUnstudied, originMap, sectionFilter)
			if err != nil {
				return nil, fmt.Errorf("failed to load story reverse cards for notebook %q: %w", notebookID, err)
			}
			cards = append(cards, reverseCards...)
		}

		if isFlashcard {
			reverseCards, err := s.loadFlashcardReverseCards(reader, notebookID, learningHistories, listMissingContext, includeUnstudied, originMap, sectionFilter)
			if err != nil {
				return nil, fmt.Errorf("failed to load flashcard reverse cards for notebook %q: %w", notebookID, err)
			}
			cards = append(cards, reverseCards...)
		}
	}

	cards = deduplicateReverseCards(cards)
	// Collapse member rows into one concept card per concept, mirroring
	// LoadCards. Run before shuffle so concept ordering is stable.
	cards = collapseConceptReverseCards(cards, buildAllConceptIndexes(reader, notebookIDs))
	if !s.disableShuffle {
		rand.Shuffle(len(cards), func(i, j int) {
			cards[i], cards[j] = cards[j], cards[i]
		})
	}
	applyForwardMask(cards)
	return cards, nil
}

// applyForwardMask updates each card's MaskedContext to also hide the
// expressions of all cards that come AFTER it in the session order. This
// prevents shared example sentences from leaking the answers of upcoming
// cards. Words from cards that have already been asked remain visible.
//
// The current card's expression is masked with "______" (the standard quiz
// blank), while future cards' expressions use "[...]" so users can
// distinguish the blank they need to fill from words hidden for spoiler
// protection.
func applyForwardMask(cards []ReverseCard) {
	for i := range cards {
		for j := range cards[i].Contexts {
			ctx := cards[i].Contexts[j].Context
			// Mask the current card's expression with standard blank.
			ctx = maskOccurrences(ctx, cards[i].Expression)
			if cards[i].AltForm != "" {
				ctx = maskOccurrences(ctx, cards[i].AltForm)
			}
			// Mask future cards' expressions with a distinct marker.
			for k := i + 1; k < len(cards); k++ {
				ctx = maskOccurrencesAs(ctx, cards[k].Expression, "[...]")
				if cards[k].AltForm != "" {
					ctx = maskOccurrencesAs(ctx, cards[k].AltForm, "[...]")
				}
			}
			cards[i].Contexts[j].MaskedContext = ctx
		}
	}
}

func deduplicateReverseCards(cards []ReverseCard) []ReverseCard {
	seen := make(map[string]int) // expression -> index in result
	var result []ReverseCard
	for _, card := range cards {
		expr := strings.ToLower(card.Expression)
		if idx, ok := seen[expr]; ok {
			if len(card.Contexts) > len(result[idx].Contexts) {
				result[idx] = card
			}
		} else {
			seen[expr] = len(result)
			result = append(result, card)
		}
	}
	return result
}

func (s *Service) loadStoryReverseCards(
	reader *notebook.Reader,
	notebookID string,
	learningHistories map[string][]notebook.LearningHistory,
	listMissingContext bool,
	includeUnstudied bool,
	originMap map[string]notebook.EtymologyOrigin,
	sectionFilter []string,
) ([]ReverseCard, error) {
	stories, err := reader.ReadStoryNotebooks(notebookID)
	if err != nil {
		return nil, fmt.Errorf("failed to read story notebook %q: %w", notebookID, err)
	}

	var cards []ReverseCard
	for _, story := range stories {
		if !inSectionFilter(sectionFilter, story.Event) {
			continue
		}
		for _, scene := range story.Scenes {
			for _, definition := range scene.Definitions {
				if !isEligibleForReverseQuiz(&definition) {
					continue
				}

				expression := definition.Expression
				altForm := ""
				if definition.Definition != "" {
					expression = definition.Definition
					altForm = definition.Expression
				}

				// Skip words marked as skipped from reverse mode
				if isExpressionSkippedInHistory(learningHistories[notebookID], story.Event, scene.Title, &definition, notebook.QuizTypeReverse, nil) {
					continue
				}

				contexts := buildReverseContexts(&scene, &definition)

				if listMissingContext {
					if len(contexts) > 0 {
						continue
					}
				} else {
					needsReview := needsReverseReview(learningHistories[notebookID], story.Event, scene.Title, &definition, includeUnstudied)
					if !needsReview {
						continue
					}
				}

				cards = append(cards, ReverseCard{
					NotebookName: notebookID,
					StoryTitle:   story.Event,
					SceneTitle:   scene.Title,
					Meaning:      definition.Meaning,
					Contexts:     contexts,
					Expression:   expression,
					AltForm:      altForm,
					WordDetail:   buildWordDetail(&definition, originMap),
					Images:       definition.Images,
				})
			}
		}
	}

	return cards, nil
}

func (s *Service) loadFlashcardReverseCards(
	reader *notebook.Reader,
	notebookID string,
	learningHistories map[string][]notebook.LearningHistory,
	listMissingContext bool,
	includeUnstudied bool,
	originMap map[string]notebook.EtymologyOrigin,
	sectionFilter []string,
) ([]ReverseCard, error) {
	notebooks, err := reader.ReadFlashcardNotebooks(notebookID)
	if err != nil {
		return nil, fmt.Errorf("failed to read flashcard notebook %q: %w", notebookID, err)
	}

	var cards []ReverseCard
	for _, nb := range notebooks {
		if !inSectionFilter(sectionFilter, nb.Title) {
			continue
		}
		for _, card := range nb.Cards {
			if !isEligibleForReverseQuiz(&card) {
				continue
			}

			expression := card.Expression
			altForm := ""
			if card.Definition != "" {
				expression = card.Definition
				altForm = card.Expression
			}

			// Skip words marked as skipped from reverse mode
			if isExpressionSkippedInHistory(learningHistories[notebookID], nb.Title, "", &card, notebook.QuizTypeReverse, nil) {
				continue
			}

			var contexts []ReverseContext
			for _, ex := range card.Examples {
				if strings.Contains(strings.ToLower(ex), strings.ToLower(card.Expression)) {
					masked := maskWord(ex, card.Expression, card.Definition)
					contexts = append(contexts, ReverseContext{
						Context:       ex,
						MaskedContext: masked,
					})
				}
			}

			if listMissingContext {
				if len(contexts) > 0 {
					continue
				}
			} else if !s.disableShuffle {
				// When disableShuffle is set (test mode), bypass the
				// spaced-repetition due-check so every fixture card is
				// reachable regardless of accumulated learning history.
				needsReview := needsReverseFlashcardReview(learningHistories[notebookID], nb.Title, &card, includeUnstudied)
				if !needsReview {
					continue
				}
			}

			cards = append(cards, ReverseCard{
				NotebookName: notebookID,
				StoryTitle:   "flashcards",
				SceneTitle:   "",
				Meaning:      card.Meaning,
				Contexts:     contexts,
				Expression:   expression,
				AltForm:      altForm,
				WordDetail:   buildWordDetail(&card, originMap),
				Images:       card.Images,
			})
		}
	}

	return cards, nil
}

func maskWord(context, expression, definition string) string {
	context = maskOccurrences(context, expression)
	if definition != "" {
		context = maskOccurrences(context, definition)
	}
	return context
}

// maskOccurrences replaces every case-insensitive occurrence of target in
// context with "______". It uses \b for word-character boundaries (so partial
// matches like "questioning" don't match "question") and falls back to a
// non-word/start-of-string boundary on either side when target itself starts
// or ends with a non-word character (e.g. "#1 fan").
func maskOccurrences(context, target string) string {
	return maskOccurrencesAs(context, target, "______")
}

func maskOccurrencesAs(context, target, replacement string) string {
	if target == "" {
		return context
	}
	runes := []rune(target)
	left := `\b`
	if !isWordChar(runes[0]) {
		left = `(?:^|[^\w])`
	}
	right := `\b`
	if !isWordChar(runes[len(runes)-1]) {
		right = `(?:[^\w]|$)`
	}
	re := regexp.MustCompile(`(?i)` + left + regexp.QuoteMeta(target) + right)
	targetLower := strings.ToLower(target)
	return re.ReplaceAllStringFunc(context, func(match string) string {
		idx := strings.Index(strings.ToLower(match), targetLower)
		if idx < 0 {
			return replacement
		}
		return match[:idx] + replacement + match[idx+len(target):]
	})
}

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// containsExpressionWord reports whether the text contains the expression or
// a close English inflection of it, case-insensitively. It is used to
// safety-net the grading model: self-definition reasons must never fire when
// the expression word (or its stem) is absent from the user's answer.
//
// For single-word expressions of length >= 5 it drops the last character to
// form a stem, so "happy" (stem "happ") matches both "happy" and "happiness".
// Multi-word expressions and short words are matched literally.
func containsExpressionWord(text, expression string) bool {
	text = strings.ToLower(text)
	expr := strings.ToLower(strings.TrimSpace(expression))
	if expr == "" {
		return false
	}
	stem := expr
	if !strings.Contains(expr, " ") && len([]rune(expr)) >= 5 {
		r := []rune(expr)
		stem = string(r[:len(r)-1])
	}
	return strings.Contains(text, stem)
}

func buildReverseContexts(scene *notebook.StoryScene, definition *notebook.Note) []ReverseContext {
	var contexts []ReverseContext
	for _, conv := range scene.Conversations {
		if conv.Quote == "" {
			continue
		}

		quoteLower := strings.ToLower(conv.Quote)
		if !containsExpression(quoteLower, definition.Expression, definition.Definition) {
			continue
		}

		cleaned := notebook.ConvertMarkersInText(conv.Quote, nil, notebook.ConversionStylePlain, "")
		masked := maskWord(cleaned, definition.Expression, definition.Definition)
		contexts = append(contexts, ReverseContext{
			Context:       cleaned,
			MaskedContext: masked,
		})
	}
	return contexts
}

// needsReverseReview reports whether a story word should appear in the
// reverse quiz. includeUnstudied mirrors the standard-quiz toggle: words
// that haven't cleared the freeform/correct prerequisite (or have no
// history at all) are included when it's true; studied words still
// respect their reverse SR interval either way.
func needsReverseReview(
	learningHistories []notebook.LearningHistory,
	storyTitle, sceneTitle string,
	definition *notebook.Note,
	includeUnstudied bool,
) bool {
	for _, h := range learningHistories {
		if h.Metadata.Title != storyTitle {
			continue
		}

		for _, scene := range h.Scenes {
			if scene.Metadata.Title != sceneTitle {
				continue
			}

			for _, expr := range scene.Expressions {
				if expr.Expression != definition.Expression && expr.Expression != definition.Definition {
					continue
				}

				// Words must be answered in freeform first AND have at
				// least one correct answer before becoming eligible for
				// reverse quiz — unless the user opted into unstudied words.
				if !expr.HasFreeformAnswer() || !expr.HasAnyCorrectAnswer() {
					return includeUnstudied
				}

				if len(expr.ReverseLogs) > 0 && !expr.NeedsReverseReview() {
					return false
				}
				return true
			}
		}
	}
	return includeUnstudied
}

func needsReverseFlashcardReview(
	learningHistories []notebook.LearningHistory,
	flashcardTitle string,
	card *notebook.Note,
	includeUnstudied bool,
) bool {
	for _, h := range learningHistories {
		if h.Metadata.Title != flashcardTitle {
			continue
		}

		for _, expr := range h.Expressions {
			if expr.Expression != card.Expression && expr.Expression != card.Definition {
				continue
			}

			// Words must be answered in freeform first AND have at
			// least one correct answer before becoming eligible for
			// reverse quiz — unless the user opted into unstudied words.
			if !expr.HasFreeformAnswer() || !expr.HasAnyCorrectAnswer() {
				return includeUnstudied
			}

			if len(expr.ReverseLogs) > 0 && !expr.NeedsReverseReview() {
				return false
			}
			return true
		}
	}
	return includeUnstudied
}

// GradeReverseAnswer grades a reverse quiz answer (user guesses the word from meaning/context).
func (s *Service) GradeReverseAnswer(ctx context.Context, card ReverseCard, answer string, responseTimeMs int64) (GradeResult, error) {
	var contextStr string
	if len(card.Contexts) > 0 {
		contextStr = card.Contexts[0].Context
	}

	validation, err := s.openaiClient.ValidateWordForm(ctx, inference.ValidateWordFormRequest{
		Expected:       card.Expression,
		UserAnswer:     answer,
		Meaning:        card.Meaning,
		Context:        contextStr,
		ResponseTimeMs: responseTimeMs,
	})
	if err != nil {
		return GradeResult{}, fmt.Errorf("failed to validate word: %w", err)
	}

	isCorrect := validation.Classification == inference.ClassificationSameWord
	quality := 1
	if isCorrect {
		if responseTimeMs < 3000 {
			quality = 5
		} else if responseTimeMs < 10000 {
			quality = 4
		} else {
			quality = 3
		}
	}

	return GradeResult{
		Correct:        isCorrect,
		Reason:         validation.Reason,
		Quality:        quality,
		Classification: string(validation.Classification),
	}, nil
}

// SaveReverseResult updates learning history via the repository.
// Same head-redirection as SaveResult; see SaveResult for details.
func (s *Service) SaveReverseResult(ctx context.Context, card ReverseCard, result GradeResult, responseTimeMs int64) error {
	status := "misunderstood"
	if result.Correct { status = "understood" }
	expression := card.Expression
	if card.ConceptHead != "" {
		expression = card.ConceptHead
	}
	log := &learning.LearningLog{
		Status: status, LearnedAt: time.Now(), Quality: result.Quality,
		ResponseTimeMs: int(responseTimeMs), QuizType: string(notebook.QuizTypeReverse),
		SourceNotebookID: card.NotebookName, NotebookName: card.NotebookName,
		StoryTitle: card.StoryTitle, SceneTitle: card.SceneTitle,
		Expression: expression, OriginalExpression: expression,
		IsCorrect: result.Correct, LearningNotesDir: s.notebooksConfig.LearningNotesDirectory,
	}
	s.applyNextInterval(card.NotebookName, expression, notebook.QuizTypeReverse, log)
	if err := s.learningRepository.Create(ctx, log); err != nil {
		return fmt.Errorf("save learning log for %q: %w", card.NotebookName, err)
	}
	return nil
}

// FreeformCard represents a freeform quiz card (user inputs word + meaning).
type FreeformCard struct {
	NotebookName       string
	StoryTitle         string
	SceneTitle         string
	Expression         string // canonical form (Definition if set, otherwise Expression)
	OriginalExpression string // text form as it appears in the story (Note.Expression)
	Meaning            string
	Contexts           []inference.Context
	WordDetail         WordDetail
	Images             []string
	// ConceptHead, when non-empty, names the concept head this card maps
	// to. SaveFreeformResult writes the log under the head so per-member
	// freeform answers (e.g. typing "misanthropist") consolidate into
	// the head's history row that MergeConcepts established.
	ConceptHead string
}

// LoadAllWords loads all words from all notebooks for freeform quiz.
func (s *Service) LoadAllWords() ([]FreeformCard, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize notebook reader: %w", err)
	}

	storyIndexes := reader.GetStoryIndexes()
	flashcardIndexes := reader.GetFlashcardIndexes()
	originMap := buildOriginMap(reader)

	var cards []FreeformCard

	for notebookID := range storyIndexes {
		words, err := s.loadStoryWords(reader, notebookID, originMap)
		if err != nil {
			continue
		}
		cards = append(cards, words...)
	}

	for notebookID := range flashcardIndexes {
		words, err := s.loadFlashcardWords(reader, notebookID, originMap)
		if err != nil {
			continue
		}
		cards = append(cards, words...)
	}

	// Also load from definitions-only books
	learningHistories, _ := s.loadHistories()
	for _, nbID := range reader.GetDefinitionsBookIDs() {
		if _, isStory := storyIndexes[nbID]; isStory {
			continue
		}
		if _, isFlashcard := flashcardIndexes[nbID]; isFlashcard {
			continue
		}
		defWords := loadDefinitionWords(reader, nbID, originMap, learningHistories)
		cards = append(cards, defWords...)
	}

	return cards, nil
}

func (s *Service) loadStoryWords(reader *notebook.Reader, notebookID string, originMap map[string]notebook.EtymologyOrigin) ([]FreeformCard, error) {
	stories, err := reader.ReadStoryNotebooks(notebookID)
	if err != nil {
		return nil, err
	}

	learningHistories, err := s.loadHistories()
	if err != nil {
		return nil, fmt.Errorf("failed to load learning histories: %w", err)
	}

	var cards []FreeformCard
	for _, story := range stories {
		for _, scene := range story.Scenes {
			for _, definition := range scene.Definitions {
				// Skip words marked as skipped from freeform mode
				if isExpressionSkippedInHistory(learningHistories[notebookID], story.Event, scene.Title, &definition, notebook.QuizTypeFreeform, nil) {
					continue
				}

				expression := definition.Expression
				if definition.Definition != "" {
					expression = definition.Definition
				}

				_, contexts := buildFromConversations(&scene, &definition)

				cards = append(cards, FreeformCard{
					NotebookName:       notebookID,
					StoryTitle:         story.Event,
					SceneTitle:         scene.Title,
					Expression:         expression,
					OriginalExpression: definition.Expression,
					Meaning:            definition.Meaning,
					Contexts:           contexts,
					WordDetail:         buildWordDetail(&definition, originMap),
					Images:             definition.Images,
				})
			}
		}
	}

	return cards, nil
}

func (s *Service) loadFlashcardWords(reader *notebook.Reader, notebookID string, originMap map[string]notebook.EtymologyOrigin) ([]FreeformCard, error) {
	notebooks, err := reader.ReadFlashcardNotebooks(notebookID)
	if err != nil {
		return nil, err
	}

	learningHistories, err := s.loadHistories()
	if err != nil {
		return nil, fmt.Errorf("failed to load learning histories: %w", err)
	}

	var cards []FreeformCard
	for _, nb := range notebooks {
		for _, card := range nb.Cards {
			// Skip words marked as skipped from freeform mode
			if isExpressionSkippedInHistory(learningHistories[notebookID], nb.Title, "", &card, notebook.QuizTypeFreeform, nil) {
				continue
			}

			expression := card.Expression
			if card.Definition != "" {
				expression = card.Definition
			}

			var contexts []inference.Context
			for _, ex := range card.Examples {
				contexts = append(contexts, inference.Context{
					Context:             ex,
					ReferenceDefinition: card.Meaning,
				})
			}

			cards = append(cards, FreeformCard{
				NotebookName: notebookID,
				StoryTitle:   "flashcards",
				SceneTitle:   "",
				Expression:   expression,
				Meaning:      card.Meaning,
				Contexts:     contexts,
				WordDetail:   buildWordDetail(&card, originMap),
				Images:       card.Images,
			})
		}
	}

	return cards, nil
}

// GradeFreeformAnswer grades a freeform quiz answer (user provides word + meaning).
func (s *Service) GradeFreeformAnswer(ctx context.Context, word, meaning string, responseTimeMs int64, cards []FreeformCard) (FreeformGradeResult, error) {
	matchingCards := findMatchingCards(cards, word)

	if len(matchingCards) == 0 {
		return FreeformGradeResult{
			Correct: false,
			Word:    word,
			Meaning: meaning,
			Reason:  fmt.Sprintf("Word '%s' not found in any notebook", word),
		}, nil
	}

	results, err := s.openaiClient.AnswerMeanings(ctx, inference.AnswerMeaningsRequest{
		Expressions: []inference.Expression{
			{
				Expression:        word,
				Meaning:           meaning,
				Contexts:          matchingCards[0].Contexts,
				IsExpressionInput: true,
				ResponseTimeMs:    responseTimeMs,
			},
		},
	})
	if err != nil {
		return FreeformGradeResult{}, fmt.Errorf("failed to grade answer: %w", err)
	}

	if len(results.Answers) == 0 {
		return FreeformGradeResult{}, fmt.Errorf("no results returned from inference")
	}

	result := results.Answers[0]
	isCorrect, reason, quality := extractAnswerResult(result)

	// Safety net: the grading model occasionally flags an answer as
	// "self-definition" even though the user's meaning does not contain
	// the expression word at all. Self-definition by definition requires
	// the user to reuse the expression word itself, so if the expression
	// word is absent from the meaning, override the model's verdict.
	if !isCorrect && strings.Contains(strings.ToLower(reason), "self-definition") &&
		!containsExpressionWord(meaning, word) {
		isCorrect = true
		reason = "matches the expected meaning (self-definition reason was overridden: your answer does not contain the expression word)"
		if quality < 3 {
			quality = 3
		}
	}

	var context string
	var notebookName string
	if len(matchingCards) > 0 {
		if len(matchingCards[0].Contexts) > 0 {
			context = matchingCards[0].Contexts[0].Context
		}
		notebookName = matchingCards[0].NotebookName
	}

	// Prefer the notebook's reference meaning over OpenAI's canonical
	// meaning. The notebook is what the user studied, and showing the
	// model's context-derived meaning led to cases where "Expected
	// meaning" and "Reason" described different interpretations.
	expectedMeaning := matchingCards[0].Meaning
	if expectedMeaning == "" {
		expectedMeaning = result.Meaning
	}

	return FreeformGradeResult{
		Correct:      isCorrect,
		Word:         result.Expression,
		Meaning:      expectedMeaning,
		Reason:       reason,
		Context:      context,
		NotebookName: notebookName,
		Quality:      quality,
		MatchedCard:  &matchingCards[0],
	}, nil
}

// FreeformGradeResult holds the outcome of grading a freeform answer.
type FreeformGradeResult struct {
	Correct      bool
	Word         string
	Meaning      string
	Reason       string
	Context      string
	NotebookName string
	Quality      int
	MatchedCard  *FreeformCard
}

// SaveFreeformResult updates learning history via the repository.
// Same head-redirection as SaveResult; see SaveResult for details.
func (s *Service) SaveFreeformResult(ctx context.Context, card FreeformCard, result FreeformGradeResult, responseTimeMs int64) error {
	status := "misunderstood"
	if result.Correct { status = "understood" }
	expression := card.Expression
	if card.ConceptHead != "" {
		expression = card.ConceptHead
	}
	log := &learning.LearningLog{
		Status: status, LearnedAt: time.Now(), Quality: result.Quality,
		ResponseTimeMs: int(responseTimeMs), QuizType: string(notebook.QuizTypeFreeform),
		SourceNotebookID: card.NotebookName, NotebookName: card.NotebookName,
		StoryTitle: card.StoryTitle, SceneTitle: card.SceneTitle,
		Expression: expression, OriginalExpression: expression,
		IsCorrect: result.Correct, LearningNotesDir: s.notebooksConfig.LearningNotesDirectory,
	}
	s.applyNextInterval(card.NotebookName, expression, notebook.QuizTypeFreeform, log)
	if err := s.learningRepository.Create(ctx, log); err != nil {
		return fmt.Errorf("save learning log for %q: %w", card.NotebookName, err)
	}
	return nil
}


// kindFromIndex returns the kind string for a notebook index.
// Books loaded from books_directories have IsBook=true but an empty Kind field,
// so we fall back to "Books" when IsBook is set.
func kindFromIndex(index notebook.Index) string {
	if index.Kind != "" {
		return index.Kind
	}
	if index.IsBook {
		return "Books"
	}
	return ""
}

// GetFreeformNextReviewDates returns a map of lowercase expression -> next review date ("YYYY-MM-DD").
// Only expressions that are NOT yet due are included; due or never-studied expressions are omitted.
func (s *Service) GetFreeformNextReviewDates(cards []FreeformCard) (map[string]string, error) {
	// In test mode, never gate the freeform Submit button on a future review
	// date — the test suite submits the same expression repeatedly across scenarios.
	if s.disableShuffle {
		return map[string]string{}, nil
	}
	learningHistories, err := s.loadHistories()
	if err != nil {
		return nil, fmt.Errorf("failed to load learning histories: %w", err)
	}

	// Track which expressions have at least one due (or never-studied) card.
	// If any card for an expression is due, the expression should be available.
	due := make(map[string]bool)
	result := make(map[string]string)
	for _, card := range cards {
		exprKey := strings.ToLower(card.Expression)
		origKey := strings.ToLower(card.OriginalExpression)

		nextDate := freeformNextReviewDate(learningHistories[card.NotebookName], card)
		if nextDate == "" {
			// This card is due or never studied — mark the expression as due
			due[exprKey] = true
			if origKey != "" {
				due[origKey] = true
			}
		} else if !due[exprKey] {
			// Only record the not-due date if no card for this expression is due
			existing, ok := result[exprKey]
			if !ok || nextDate > existing {
				result[exprKey] = nextDate
			}
			if origKey != "" && !due[origKey] {
				existingOrig, okOrig := result[origKey]
				if !okOrig || nextDate > existingOrig {
					result[origKey] = nextDate
				}
			}
		}
	}
	// Remove any not-due dates for expressions that have at least one due card
	for key := range due {
		delete(result, key)
	}
	return result, nil
}

func freeformNextReviewDate(histories []notebook.LearningHistory, card FreeformCard) string {
	for _, hist := range histories {
		if hist.Metadata.Title != card.StoryTitle {
			continue
		}
		if hist.Metadata.Type == "flashcard" {
			for _, expr := range hist.Expressions {
				if strings.EqualFold(expr.Expression, card.Expression) {
					return computeNextReviewDate(expr.LearnedLogs)
				}
			}
			continue
		}
		for _, scene := range hist.Scenes {
			if scene.Metadata.Title != card.SceneTitle {
				continue
			}
			for _, expr := range scene.Expressions {
				if strings.EqualFold(expr.Expression, card.Expression) {
					return computeNextReviewDate(expr.LearnedLogs)
				}
			}
		}
	}
	return ""
}

func computeNextReviewDate(logs []notebook.LearningRecord) string {
	if len(logs) == 0 || logs[0].IntervalDays == 0 {
		return ""
	}
	nextDate := logs[0].LearnedAt.AddDate(0, 0, logs[0].IntervalDays)
	if !time.Now().Before(nextDate) {
		return ""
	}
	return nextDate.Format("2006-01-02")
}

func findMatchingCards(cards []FreeformCard, word string) []FreeformCard {
	var matches []FreeformCard
	for _, card := range cards {
		if strings.EqualFold(card.Expression, word) || strings.EqualFold(card.OriginalExpression, word) {
			matches = append(matches, card)
		}
	}
	return matches
}

// GetLatestLearnedInfo returns the learned_at and next_review_date for the latest log
// of a given expression in a specific notebook.
func (s *Service) GetLatestLearnedInfo(notebookName, expression string, quizType notebook.QuizType) (learnedAt string, nextReviewDate string) {
	learningHistories, err := s.loadHistories()
	if err != nil {
		return "", ""
	}

	updater := notebook.NewLearningHistoryUpdater(learningHistories[notebookName], s.calculator)
	expr := updater.FindExpressionByName(expression)
	if expr == nil {
		return "", ""
	}

	logs := expr.GetLogsForQuizType(quizType)
	if len(logs) == 0 {
		return "", ""
	}

	latest := logs[0]
	learnedAt = latest.LearnedAt.Format("2006-01-02")
	if latest.IntervalDays > 0 {
		nextReviewDate = latest.LearnedAt.AddDate(0, 0, latest.IntervalDays).Format("2006-01-02")
	}
	return learnedAt, nextReviewDate
}

// isExpressionSkippedInHistory checks whether a note is excluded from the
// given quiz type. Per-type skipping replaced the global skip flag.
// When the note is a concept member, its expression/definition are
// resolved to the head before matching history — the migration folded
// member skip timestamps into the head, so a direct lookup would miss
// the skip flag and re-surface the word.
func isExpressionSkippedInHistory(histories []notebook.LearningHistory, event, sceneTitle string, def *notebook.Note, quizType notebook.QuizType, conceptHeads map[string]string) bool {
	expr := canonicalDefinitionExpression(def.Expression, conceptHeads)
	defn := canonicalDefinitionExpression(def.Definition, conceptHeads)
	return notebook.IsExpressionSkipped(histories, event, sceneTitle, expr, defn, quizType)
}

// definitionsSectionSummaries returns per-session counts for a
// definitions-only book (e.g. Word Power Made Easy). Without this, the
// vocabulary quiz options page showed the book as a single un-expandable
// row even though etymology mode listed every session of the same book.
// Sessions are ordered by the trailing integer in their title
// ("Session 1", "Session 10", "Session 2" → 1, 2, 10) so users see them
// in document order; titles without a trailing integer fall back to
// alphabetical ordering after numbered ones.
func definitionsSectionSummaries(
	defs map[string]map[string][]notebook.Note,
	histories []notebook.LearningHistory,
	includeUnstudied bool,
	conceptHeads map[string]string,
) []NotebookSectionSummary {
	if len(defs) == 0 {
		return nil
	}
	titles := make([]string, 0, len(defs))
	for title := range defs {
		titles = append(titles, title)
	}
	sort.Slice(titles, func(i, j int) bool {
		ni, oki := trailingInt(titles[i])
		nj, okj := trailingInt(titles[j])
		if oki && okj {
			if ni != nj {
				return ni < nj
			}
		} else if oki != okj {
			return oki
		}
		return titles[i] < titles[j]
	})
	var sections []NotebookSectionSummary
	for _, title := range titles {
		one := map[string]map[string][]notebook.Note{title: defs[title]}
		sections = append(sections, NotebookSectionSummary{
			Title:              title,
			ReviewCount:        countDefinitionNotes(one, histories, false, includeUnstudied, conceptHeads),
			ReverseReviewCount: countDefinitionNotes(one, histories, true, includeUnstudied, conceptHeads),
		})
	}
	return sections
}

// trailingInt extracts a trailing integer from s. "Session 12" → (12,
// true); "intro" → (0, false). Used by definitionsSectionSummaries to
// order session titles numerically rather than lexically.
func trailingInt(s string) (int, bool) {
	i := len(s)
	for i > 0 {
		c := s[i-1]
		if c < '0' || c > '9' {
			break
		}
		i--
	}
	if i == len(s) {
		return 0, false
	}
	n := 0
	for _, c := range s[i:] {
		n = n*10 + int(c-'0')
	}
	return n, true
}

// countDefinitionNotes counts notes in definitions-only books that need
// review for the given direction. Both standard and reverse honour
// includeUnstudied (the start-page toggle passes through to whichever
// quiz the user is about to start). The count goes through
// shouldIncludeDefinition so the badge stays in lockstep with what
// loadDefinitionCards / loadDefinitionReverseCards actually return —
// previously this function skipped the per-type SkippedAt gate the
// loaders apply, so the badge over-counted any word the user had
// excluded from that quiz mode.
func countDefinitionNotes(defs map[string]map[string][]notebook.Note, histories []notebook.LearningHistory, isReverse, includeUnstudied bool, conceptHeads map[string]string) int {
	quizType := notebook.QuizTypeNotebook
	if isReverse {
		quizType = notebook.QuizTypeReverse
	}
	count := 0
	// Dedupe by concept so the badge reflects what the quiz actually
	// surfaces (one card per concept) rather than counting head+members
	// independently. The first head-or-member encountered for a concept
	// triggers shouldIncludeDefinition; subsequent ones are skipped.
	seenConcept := make(map[string]bool)
	for storyTitle, sceneDefs := range defs {
		for sceneTitle, notes := range sceneDefs {
			for i := range notes {
				note := &notes[i]
				if note.Meaning == "" {
					continue
				}
				if _, isConceptExpr := conceptHeads[note.Expression]; isConceptExpr {
					canonical := canonicalDefinitionExpression(note.Expression, conceptHeads)
					conceptKey := storyTitle + "|" + sceneTitle + "|" + canonical
					if seenConcept[conceptKey] {
						continue
					}
					seenConcept[conceptKey] = true
				}
				if shouldIncludeDefinition(histories, storyTitle, sceneTitle, note, includeUnstudied, quizType, conceptHeads) {
					count++
				}
			}
		}
	}
	return count
}

// shouldIncludeDefinition is the single source of truth for whether a
// definitions-only-book note appears in the standard or reverse vocab
// quiz (and is counted in the start-page badge). Returning true means
// the note appears in BOTH the badge count and the corresponding
// LoadCards / LoadReverseCards result; false means it appears in
// neither. Freeform has its own simpler rule (no SR gate) and stays in
// loadDefinitionWords.
//
// quizType picks the per-type SkippedAt slot and the SR direction:
//
//	QuizTypeNotebook → standard (needsDefinitionReview)
//	QuizTypeReverse  → reverse  (needsDefinitionReverseReview)
//
// includeUnstudied threads through to the SR helpers exactly as the
// loaders pass it — never-seen and not-yet-cleared words become
// eligible when the toggle is on, but words still inside their SR
// interval stay excluded.
func shouldIncludeDefinition(
	histories []notebook.LearningHistory,
	storyTitle, sceneTitle string,
	note *notebook.Note,
	includeUnstudied bool,
	quizType notebook.QuizType,
	conceptHeads map[string]string,
) bool {
	if isExpressionSkippedInHistory(histories, storyTitle, sceneTitle, note, quizType, conceptHeads) {
		return false
	}
	if quizType == notebook.QuizTypeReverse {
		return needsDefinitionReverseReview(histories, storyTitle, sceneTitle, note, includeUnstudied, conceptHeads)
	}
	return needsDefinitionReview(histories, storyTitle, sceneTitle, note, includeUnstudied, conceptHeads)
}

// loadDefinitionCards loads standard quiz cards from definitions-only books.
//
// Per-kind concept handling:
//
//   family       — one card per concept, sourced from the head's own
//                  note row (head expression + head's meaning). Non-head
//                  member notes are skipped entirely so the prompt and
//                  the answer can never refer to different forms with
//                  different meanings (cardiology / "the medical
//                  specialty …" rather than the cardiologist row's
//                  meaning slipping in).
//   synonym /    — display-only groupings: each member's note becomes
//   antonym /     its own card with its own meaning. ConceptMembers is
//   visualization populated for the Family-chip UI, but ConceptHead is
//                  left empty so SaveResult writes under the member
//                  (no SR consolidation under the head).
func loadDefinitionCards(reader *notebook.Reader, bookID string, learningHistories map[string][]notebook.LearningHistory, originMap map[string]notebook.EtymologyOrigin, sectionFilter []string, includeUnstudied bool) []Card {
	defs, ok := reader.GetDefinitionsNotesByTitle(bookID)
	if !ok {
		return nil
	}
	conceptHeads, byHead := reader.GetDefinitionsBookConceptInfo(bookID)
	byMember := reader.GetDefinitionsBookConceptByMember(bookID)

	var cards []Card
	seenFamily := make(map[string]bool)
	for storyTitle, sceneDefs := range defs {
		if !inSectionFilter(sectionFilter, storyTitle) {
			continue
		}
		for sceneTitle, notes := range sceneDefs {
			for _, note := range notes {
				if note.Meaning == "" {
					continue
				}
				// Family member that isn't the head: skip — the head's
				// own note carries the canonical (expression, meaning)
				// pair this concept should surface.
				if head, ok := conceptHeads[note.Expression]; ok && head != note.Expression {
					continue
				}
				// Family head: dedupe (multiple sessions could redeclare
				// the same head; the loader keeps the first occurrence).
				if info, ok := byHead[note.Expression]; ok && info.ConsolidatesSR() {
					key := bookID + "|" + note.Expression
					if seenFamily[key] {
						continue
					}
					seenFamily[key] = true
				}
				if !shouldIncludeDefinition(learningHistories[bookID], storyTitle, sceneTitle, &note, includeUnstudied, notebook.QuizTypeNotebook, conceptHeads) {
					continue
				}
				entry := note.Definition
				originalEntry := ""
				if entry == "" {
					entry = note.Expression
				} else {
					originalEntry = note.Expression
				}
				card := Card{
					NotebookName:  bookID,
					StoryTitle:    storyTitle,
					SceneTitle:    sceneTitle,
					Entry:         entry,
					OriginalEntry: originalEntry,
					Meaning:       note.Meaning,
					WordDetail:    buildWordDetail(&note, originMap),
				}
				// Decorate via byMember so non-head members of non-family
				// concepts still get ConceptMembers / ConceptMeaning for
				// the Family-chip display. Only family concepts set
				// ConceptHead, which is what gates SR consolidation.
				if info, ok := byMember[note.Expression]; ok {
					card.ConceptMeaning = info.Meaning
					card.ConceptMembers = info.Members
					if info.ConsolidatesSR() {
						card.ConceptHead = info.Head
					}
				}
				cards = append(cards, card)
			}
		}
	}
	return cards
}

// loadDefinitionReverseCards loads reverse quiz cards from definitions-only books.
// Per-kind handling mirrors loadDefinitionCards: family concepts emit
// one card sourced from the head's own note (so the prompt-meaning and
// the expected expression always match); non-family concepts emit
// per-member cards with the member's own meaning, ConceptMembers
// populated for display, and ConceptHead left empty so saves stay on
// the member.
func loadDefinitionReverseCards(reader *notebook.Reader, bookID string, learningHistories map[string][]notebook.LearningHistory, originMap map[string]notebook.EtymologyOrigin, sectionFilter []string, includeUnstudied bool) []ReverseCard {
	defs, ok := reader.GetDefinitionsNotesByTitle(bookID)
	if !ok {
		return nil
	}
	conceptHeads, byHead := reader.GetDefinitionsBookConceptInfo(bookID)
	byMember := reader.GetDefinitionsBookConceptByMember(bookID)

	var cards []ReverseCard
	seenFamily := make(map[string]bool)
	for storyTitle, sceneDefs := range defs {
		if !inSectionFilter(sectionFilter, storyTitle) {
			continue
		}
		for sceneTitle, notes := range sceneDefs {
			for _, note := range notes {
				if note.Meaning == "" {
					continue
				}
				if head, ok := conceptHeads[note.Expression]; ok && head != note.Expression {
					continue
				}
				if info, ok := byHead[note.Expression]; ok && info.ConsolidatesSR() {
					key := bookID + "|" + note.Expression
					if seenFamily[key] {
						continue
					}
					seenFamily[key] = true
				}
				if !shouldIncludeDefinition(learningHistories[bookID], storyTitle, sceneTitle, &note, includeUnstudied, notebook.QuizTypeReverse, conceptHeads) {
					continue
				}
				expression := note.Expression
				altForm := ""
				if note.Definition != "" {
					expression = note.Definition
					altForm = note.Expression
				}
				card := ReverseCard{
					NotebookName: bookID,
					StoryTitle:   storyTitle,
					SceneTitle:   sceneTitle,
					Meaning:      note.Meaning,
					Expression:   expression,
					AltForm:      altForm,
					WordDetail:   buildWordDetail(&note, originMap),
				}
				if info, ok := byMember[note.Expression]; ok {
					card.ConceptMeaning = info.Meaning
					card.ConceptMembers = info.Members
					if info.ConsolidatesSR() {
						card.ConceptHead = info.Head
					}
				}
				cards = append(cards, card)
			}
		}
	}
	return cards
}

// loadDefinitionWords loads freeform cards from definitions-only books.
// Words skipped from freeform mode are excluded; the gate matches the
// story-side path's behaviour at line 1317.
//
// Concept members keep their own card here (freeform asks the user to
// type a specific form, so all members remain answerable), but the
// resulting cards carry ConceptHead so SaveFreeformResult records the
// outcome under the head.
func loadDefinitionWords(reader *notebook.Reader, bookID string, originMap map[string]notebook.EtymologyOrigin, learningHistories map[string][]notebook.LearningHistory) []FreeformCard {
	defs, ok := reader.GetDefinitionsNotesByTitle(bookID)
	if !ok {
		return nil
	}
	conceptHeads, _ := reader.GetDefinitionsBookConceptInfo(bookID)

	histories := learningHistories[bookID]
	var cards []FreeformCard
	for storyTitle, sceneDefs := range defs {
		for sceneTitle, notes := range sceneDefs {
			for _, note := range notes {
				if note.Meaning == "" {
					continue
				}
				if isExpressionSkippedInHistory(histories, storyTitle, sceneTitle, &note, notebook.QuizTypeFreeform, conceptHeads) {
					continue
				}
				expression := note.Expression
				if note.Definition != "" {
					expression = note.Definition
				}
				card := FreeformCard{
					NotebookName:       bookID,
					StoryTitle:         storyTitle,
					SceneTitle:         sceneTitle,
					Expression:         expression,
					OriginalExpression: note.Expression,
					Meaning:            note.Meaning,
					WordDetail:         buildWordDetail(&note, originMap),
				}
				if head, ok := conceptHeads[note.Expression]; ok && head != "" {
					card.ConceptHead = head
				}
				cards = append(cards, card)
			}
		}
	}
	return cards
}

// needsDefinitionReview checks if a definition note needs forward quiz review.
// Mirrors needsReverseReview's eligibility gate: a word must have been
// freeform-answered first AND have at least one correct answer before it
// appears in standard quiz. Without the has-correct-answer gate, a word
// the user only freeform-failed would still be served by the standard
// quiz even with "Include unstudied" off.
// needsDefinitionReview reports whether a definitions-book word should
// appear in the standard quiz. Without includeUnstudied, a word is
// eligible only after it's been freeform-answered, has a correct answer,
// AND its SR interval has elapsed — never-seen words are excluded.
//
// includeUnstudied mirrors the story/flashcard "Include unstudied words"
// behaviour for definitions books: never-seen words (no matching history)
// and words that haven't cleared the freeform/correct gate are included.
// Words that ARE studied still respect their SR interval (so the toggle
// adds unstudied words without re-surfacing words you've recently
// answered). Before this, definitions books ignored the toggle entirely
// and the standard quiz only ever loaded the freeform-cleared, due
// subset — e.g. ~30 words for a book with hundreds of unstudied entries.
func needsDefinitionReview(
	histories []notebook.LearningHistory,
	storyTitle, sceneTitle string,
	note *notebook.Note,
	includeUnstudied bool,
	conceptHeads map[string]string,
) bool {
	// Resolve concept members to their head. MergeConcepts folded
	// per-member learning history rows into the head, so a lookup keyed
	// by the member's own expression would miss the head's row and the
	// function would fall through to `return includeUnstudied` — making
	// the member look pristine forever even after the head has been
	// answered correctly.
	primary := canonicalDefinitionExpression(note.Expression, conceptHeads)
	secondary := canonicalDefinitionExpression(note.Definition, conceptHeads)
	for _, h := range histories {
		if h.Metadata.Title != storyTitle {
			continue
		}
		for _, scene := range h.Scenes {
			if scene.Metadata.Title != sceneTitle {
				continue
			}
			for _, expr := range scene.Expressions {
				if expr.Expression != primary && (secondary == "" || expr.Expression != secondary) {
					continue
				}
				// Once a word has any correct answer (in any direction),
				// it counts as "studied" and the SR interval alone decides
				// whether to show it. The includeUnstudied toggle only
				// gates pristine words — it must NOT bypass SR for words
				// the user has already gotten right, even when they got
				// them right in a different mode (e.g. egotist answered
				// correctly in standard/reverse but never in freeform).
				if expr.HasAnyCorrectAnswerInAnyDirection() {
					return expr.NeedsForwardReview()
				}
				return includeUnstudied
			}
		}
	}
	// No history for this word at all → pristine; include only when the
	// toggle is on.
	return includeUnstudied
}

// needsDefinitionReverseReview checks if a definition note needs reverse quiz review.
func needsDefinitionReverseReview(
	histories []notebook.LearningHistory,
	storyTitle, sceneTitle string,
	note *notebook.Note,
	includeUnstudied bool,
	conceptHeads map[string]string,
) bool {
	primary := canonicalDefinitionExpression(note.Expression, conceptHeads)
	secondary := canonicalDefinitionExpression(note.Definition, conceptHeads)
	for _, h := range histories {
		if h.Metadata.Title != storyTitle {
			continue
		}
		for _, scene := range h.Scenes {
			if scene.Metadata.Title != sceneTitle {
				continue
			}
			for _, expr := range scene.Expressions {
				if expr.Expression != primary && (secondary == "" || expr.Expression != secondary) {
					continue
				}
				// Same studied/unstudied split as needsDefinitionReview:
				// a correct answer in ANY direction means SR governs.
				// includeUnstudied only gates pristine words.
				if expr.HasAnyCorrectAnswerInAnyDirection() {
					return expr.NeedsReverseReview()
				}
				return includeUnstudied
			}
		}
	}
	return includeUnstudied
}
