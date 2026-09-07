// Package server provides Connect RPC handlers for the quiz service.
package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"log/slog"

	"connectrpc.com/connect"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/gen-protos/api/v1/apiv1connect"
	"github.com/at-ishikawa/langner/internal/assets"
	"github.com/at-ishikawa/langner/internal/auth"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/pdf"
)

// NotebookHandler implements the NotebookServiceHandler interface.
type NotebookHandler struct {
	apiv1connect.UnimplementedNotebookServiceHandler
	notebooksConfig  config.NotebooksConfig
	templatesConfig  config.TemplatesConfig
	dictionaryMap    map[string]rapidapi.Response
	dictionaryReader *dictionary.Reader
	clientResolver   inference.ClientResolver
	noteRepository   notebook.NoteRepository
	// historyStore, when set, is the DB-backed READ side for learning history.
	// The Learn page's status / next-review / exclusion badges then resolve
	// from the database instead of re-parsing the on-disk learning_notes YAML,
	// which is frozen at runtime once a database is configured. Nil in
	// YAML-only mode, in which case loadHistories reads the YAML files.
	historyStore learning.HistoryStore
	// aclRepo, when set (DB mode), resolves notebook public/private visibility
	// (auth Phase 3). GetNotebookDetail / GetEtymologyNotebook / ExportNotebookPDF
	// hard-reject a notebook the requesting user can't see with CodeNotFound (a
	// 404 that doesn't disclose the notebook exists). Nil in YAML-only / no-DB
	// dev, where every notebook is visible.
	aclRepo notebook.NotebookVisibility
}

// SetNotebookACL installs the notebook visibility resolver (auth Phase 3).
// Called from bootstrap once the database is connected; nil keeps every
// notebook visible (YAML-only / no-DB dev).
func (h *NotebookHandler) SetNotebookACL(aclRepo notebook.NotebookVisibility) {
	h.aclRepo = aclRepo
}

// ensureNotebookVisible returns CodeNotFound when the notebook is hidden from
// userID (a private notebook they don't own). A hidden notebook is treated
// exactly like a non-existent one so the response never discloses its
// existence. No ACL repo (YAML-only / no-DB dev) means every notebook is
// visible.
func (h *NotebookHandler) ensureNotebookVisible(ctx context.Context, userID int64, notebookID string) error {
	if h.aclRepo == nil {
		return nil
	}
	visible, err := h.aclRepo.VisibleNotebookIDs(ctx, userID)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("resolve notebook visibility: %w", err))
	}
	if !visible(notebookID) {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("notebook %s not found", notebookID))
	}
	return nil
}

// SetHistoryStore installs the DB-backed learning-history read store so the
// Learn page reads from the database. Called from bootstrap once the database
// is connected; nil keeps the on-disk YAML fallback.
func (h *NotebookHandler) SetHistoryStore(store learning.HistoryStore) {
	h.historyStore = store
}

// loadHistories returns every notebook's learning history keyed by notebook
// ID, from the DB store when one is installed and otherwise from the on-disk
// learning_notes YAML. Both sources return the identical map shape (the
// DBHistoryStore mirrors notebook.NewLearningHistories), so callers never
// branch on the source — mirroring quiz.Service.loadHistories, which keeps
// the Learn page's reads symmetric with the quiz service's.
func (h *NotebookHandler) loadHistories(userID int64) (map[string][]notebook.LearningHistory, error) {
	if h.historyStore != nil {
		return h.historyStore.LoadAll(context.Background(), userID)
	}
	return notebook.NewLearningHistories(h.notebooksConfig.LearningNotesDirectory)
}

// NewNotebookHandler creates a new NotebookHandler.
// noteRepo is optional; pass nil when DB is not configured.
func NewNotebookHandler(notebooksConfig config.NotebooksConfig, templatesConfig config.TemplatesConfig, dictionaryMap map[string]rapidapi.Response, dictionaryReader *dictionary.Reader, clientResolver inference.ClientResolver, noteRepo notebook.NoteRepository) *NotebookHandler {
	return &NotebookHandler{
		notebooksConfig:  notebooksConfig,
		templatesConfig:  templatesConfig,
		dictionaryMap:    dictionaryMap,
		dictionaryReader: dictionaryReader,
		clientResolver:   clientResolver,
		noteRepository:   noteRepo,
	}
}

func (h *NotebookHandler) newReader() (*notebook.Reader, error) {
	reader, err := notebook.NewReader(
		h.notebooksConfig.StoriesDirectories,
		h.notebooksConfig.FlashcardsDirectories,
		h.notebooksConfig.BooksDirectories,
		h.notebooksConfig.DefinitionsDirectories,
		h.notebooksConfig.EtymologyDirectories,
		h.dictionaryMap,
	)
	if err != nil {
		return nil, err
	}
	// Register journals (stored in the story format) so GetNotebookDetail can
	// load a journal's prose, mirroring the quiz service reader.
	if err := reader.LoadJournals(h.notebooksConfig.JournalsDirectories); err != nil {
		return nil, fmt.Errorf("load journals: %w", err)
	}
	return reader, nil
}

func (h *NotebookHandler) loadLearningHistory(userID int64, notebookID string) ([]notebook.LearningHistory, error) {
	histories, err := h.loadHistories(userID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load learning histories: %w", err))
	}
	return histories[notebookID], nil
}

func convertLogsToProto(logs []notebook.LearningRecord) []*apiv1.LearningLogEntry {
	entries := make([]*apiv1.LearningLogEntry, 0, len(logs))
	for _, log := range logs {
		entries = append(entries, &apiv1.LearningLogEntry{
			Status:         string(log.Status),
			LearnedAt:      log.LearnedAt.Format("2006-01-02"),
			Quality:        int32(log.Quality),
			ResponseTimeMs: log.ResponseTimeMs,
			QuizType:       log.QuizType,
			IntervalDays:   int32(log.IntervalDays),
		})
	}
	return entries
}

// GetNotebookDetail returns the detailed contents of a notebook.
func (h *NotebookHandler) GetNotebookDetail(
	ctx context.Context,
	req *connect.Request[apiv1.GetNotebookDetailRequest],
) (*connect.Response[apiv1.GetNotebookDetailResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}

	notebookID := req.Msg.GetNotebookId()
	userID, _ := auth.UserIDFromContext(ctx)

	if err := h.ensureNotebookVisible(ctx, userID, notebookID); err != nil {
		return nil, err
	}

	reader, err := h.newReader()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create notebook reader: %w", err))
	}

	learningHistory, err := h.loadLearningHistory(userID, notebookID)
	if err != nil {
		return nil, err
	}

	noteIDByExpr := h.loadNoteIDsForNotebook(ctx, notebookID)

	storyNotebooks, err := reader.ReadStoryNotebooks(notebookID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return h.getFlashcardNotebookDetail(notebookID, reader, learningHistory, noteIDByExpr)
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read story notebooks: %w", err))
	}

	indexName := notebookID
	if idx, ok := reader.GetStoryIndexes()[notebookID]; ok {
		indexName = idx.Name
	}

	// Concept lookup for this notebook: empty when the book has no
	// concepts: declarations. byExpression maps each member to its head;
	// byHead carries the umbrella meaning + ordered member list so the
	// frontend can render one card per concept with shared skip controls.
	conceptByExpression, conceptByHead := reader.GetDefinitionsBookConceptInfo(notebookID)

	var totalWordCount int32
	var stories []*apiv1.StoryEntry
	for _, nb := range storyNotebooks {
		var scenes []*apiv1.StoryScene
		for _, scene := range nb.Scenes {
			var definitions []*apiv1.NotebookWord
			for _, def := range scene.Definitions {
				var logs []notebook.LearningRecord
				for _, hist := range learningHistory {
					if l := hist.GetLogs(nb.Event, scene.Title, def); len(l) > 0 {
						logs = l
					}
				}

				_ = def.SetDetails(h.dictionaryMap, "")
				info := h.findLearningInfoFull(learningHistory, nb.Event, scene.Title, def)
				info.noteID = noteIDForDef(noteIDByExpr, def)

				conceptHead, conceptMembers, conceptMeaning := lookupConceptForWord(def, conceptByExpression, conceptByHead)

				definitions = append(definitions, &apiv1.NotebookWord{
					Expression:       def.Expression,
					Definition:       def.Definition,
					Meaning:          def.Meaning,
					PartOfSpeech:     def.PartOfSpeech,
					Pronunciation:    def.Pronunciation,
					Examples:         def.Examples.Texts(),
					Synonyms:         def.Synonyms,
					Antonyms:         def.Antonyms,
					LearningStatus:   string(info.status),
					LearnedLogs:      convertLogsToProto(logs),
					NextReviewDate:   info.nextReviewDate,
					Origin:           def.Origin,
					IsSkipped:        info.isSkipped,
					SkippedQuizTypes: info.skippedTypes,
					NoteId:           info.noteID,
					ConceptHead:      conceptHead,
					ConceptMembers:   conceptMembers,
					ConceptMeaning:   conceptMeaning,
				})
				totalWordCount++
			}

			var conversations []*apiv1.Conversation
			for _, conv := range scene.Conversations {
				conversations = append(conversations, &apiv1.Conversation{
					Speaker: conv.Speaker,
					Quote:   conv.Quote,
				})
			}

			scenes = append(scenes, &apiv1.StoryScene{
				Title:         scene.Title,
				Conversations: conversations,
				Definitions:   definitions,
				Statements:    scene.Statements,
			})
		}

		stories = append(stories, &apiv1.StoryEntry{
			Event: nb.Event,
			Metadata: &apiv1.StoryMetadata{
				Series:  nb.Metadata.Series,
				Season:  int32(nb.Metadata.Season),
				Episode: int32(nb.Metadata.Episode),
			},
			Date:   nb.Date.Format("2006-01-02"),
			Scenes: scenes,
		})
	}

	return connect.NewResponse(&apiv1.GetNotebookDetailResponse{
		NotebookId:     notebookID,
		Name:           indexName,
		Stories:        stories,
		TotalWordCount: totalWordCount,
	}), nil
}

// getFlashcardNotebookDetail handles GetNotebookDetail for flashcard notebooks.
// If no flashcard notebook matches, it falls through to definitions-only books
// (e.g. vocabulary books under definitions/books/<id>/) before returning 404.
func (h *NotebookHandler) getFlashcardNotebookDetail(
	notebookID string,
	reader *notebook.Reader,
	learningHistory []notebook.LearningHistory,
	noteIDByExpr map[string]int64,
) (*connect.Response[apiv1.GetNotebookDetailResponse], error) {
	flashcardNotebooks, err := reader.ReadFlashcardNotebooks(notebookID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return h.getDefinitionsBookDetail(notebookID, reader, learningHistory, noteIDByExpr)
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read flashcard notebooks: %w", err))
	}

	indexName := notebookID
	if idx, ok := reader.GetFlashcardIndexes()[notebookID]; ok {
		indexName = idx.Name
	}

	var totalWordCount int32
	var stories []*apiv1.StoryEntry
	for _, nb := range flashcardNotebooks {
		var definitions []*apiv1.NotebookWord
		for _, card := range nb.Cards {
			var logs []notebook.LearningRecord
			for _, hist := range learningHistory {
				if l := hist.GetLogs(nb.Title, "", card); len(l) > 0 {
					logs = l
				}
			}

			_ = card.SetDetails(h.dictionaryMap, "")
			info := h.findLearningInfoFull(learningHistory, nb.Title, "", card)
			info.noteID = noteIDForDef(noteIDByExpr, card)

			definitions = append(definitions, &apiv1.NotebookWord{
				Expression:       card.Expression,
				Definition:       card.Definition,
				Meaning:          card.Meaning,
				PartOfSpeech:     card.PartOfSpeech,
				Pronunciation:    card.Pronunciation,
				Examples:         card.Examples.Texts(),
				Synonyms:         card.Synonyms,
				Antonyms:         card.Antonyms,
				LearningStatus:   string(info.status),
				LearnedLogs:      convertLogsToProto(logs),
				NextReviewDate:   info.nextReviewDate,
				Origin:           card.Origin,
				IsSkipped:        info.isSkipped,
				SkippedQuizTypes: info.skippedTypes,
				NoteId:           info.noteID,
			})
			totalWordCount++
		}

		stories = append(stories, &apiv1.StoryEntry{
			Event: nb.Title,
			Scenes: []*apiv1.StoryScene{
				{
					Definitions: definitions,
				},
			},
		})
	}

	return connect.NewResponse(&apiv1.GetNotebookDetailResponse{
		NotebookId:     notebookID,
		Name:           indexName,
		Stories:        stories,
		TotalWordCount: totalWordCount,
	}), nil
}

// getDefinitionsBookDetail handles GetNotebookDetail for definitions-only
// vocabulary books (e.g. Word Power Made Easy) that aren't loaded as story
// or flashcard notebooks. The book's source YAML preserves session titles
// and per-scene titles, so each Definitions entry surfaces as a StoryEntry
// (one per session/title) with nested scenes carrying the original scene
// titles (e.g. "tele (far)") and their expressions.
func (h *NotebookHandler) getDefinitionsBookDetail(
	notebookID string,
	reader *notebook.Reader,
	learningHistory []notebook.LearningHistory,
	noteIDByExpr map[string]int64,
) (*connect.Response[apiv1.GetNotebookDetailResponse], error) {
	bookDefs, ok := reader.GetDefinitionsBook(notebookID)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("notebook %s not found", notebookID))
	}

	conceptByExpression, conceptByHead := reader.GetDefinitionsBookConceptInfo(notebookID)

	var totalWordCount int32
	var stories []*apiv1.StoryEntry
	for _, def := range bookDefs {
		event := def.Metadata.Title
		if event == "" {
			event = def.Metadata.Notebook
		}
		if event == "" {
			continue
		}

		var scenes []*apiv1.StoryScene
		for _, scene := range def.Scenes {
			var definitions []*apiv1.NotebookWord
			for i := range scene.Expressions {
				note := scene.Expressions[i]
				_ = note.SetDetails(h.dictionaryMap, "")
				info := h.findLearningInfoFull(learningHistory, event, scene.Metadata.Title, note)
				info.noteID = noteIDForDef(noteIDByExpr, note)

				var logs []notebook.LearningRecord
				for _, hist := range learningHistory {
					if l := hist.GetLogs(event, scene.Metadata.Title, note); len(l) > 0 {
						logs = l
					}
				}

				conceptHead, conceptMembers, conceptMeaning := lookupConceptForWord(note, conceptByExpression, conceptByHead)

				definitions = append(definitions, &apiv1.NotebookWord{
					Expression:       note.Expression,
					Definition:       note.Definition,
					Meaning:          note.Meaning,
					PartOfSpeech:     note.PartOfSpeech,
					Pronunciation:    note.Pronunciation,
					Examples:         note.Examples.Texts(),
					Synonyms:         note.Synonyms,
					Antonyms:         note.Antonyms,
					LearningStatus:   string(info.status),
					LearnedLogs:      convertLogsToProto(logs),
					NextReviewDate:   info.nextReviewDate,
					Origin:           note.Origin,
					IsSkipped:        info.isSkipped,
					SkippedQuizTypes: info.skippedTypes,
					NoteId:           info.noteID,
					ConceptHead:      conceptHead,
					ConceptMembers:   conceptMembers,
					ConceptMeaning:   conceptMeaning,
				})
				totalWordCount++
			}
			scenes = append(scenes, &apiv1.StoryScene{
				Title:       scene.Metadata.Title,
				Definitions: definitions,
			})
		}

		stories = append(stories, &apiv1.StoryEntry{
			Event:  event,
			Date:   def.Metadata.Date.Format("2006-01-02"),
			Scenes: scenes,
		})
	}

	return connect.NewResponse(&apiv1.GetNotebookDetailResponse{
		NotebookId:     notebookID,
		Name:           notebookID,
		Stories:        stories,
		TotalWordCount: totalWordCount,
	}), nil
}

// learningInfo holds learning status details for a definition.
type learningInfo struct {
	status         notebook.LearnedStatus
	nextReviewDate string
	// isSkipped is true when the expression is excluded from at least one
	// quiz type. Used for the simple "is anything skipped" badge.
	isSkipped bool
	// skippedTypes lists the quiz type strings the expression is excluded
	// from, for per-type badge rendering on the notebook detail page.
	skippedTypes []string
	// noteID is the DB primary key of this expression's note row, used by
	// the notebook detail page to call SkipWord/ResumeWord. Zero when the
	// DB hasn't been populated.
	noteID int64
}

// loadNoteIDsForNotebook returns a map of lowercase expression -> note ID
// for every note linked to notebookID via notebook_notes. Returns an empty
// map if the DB is not configured or the lookup fails — the caller then
// emits zero note IDs and the frontend hides the skip controls.
func (h *NotebookHandler) loadNoteIDsForNotebook(ctx context.Context, notebookID string) map[string]int64 {
	out := make(map[string]int64)
	if h.noteRepository == nil {
		return out
	}
	notes, err := h.noteRepository.FindAll(ctx)
	if err != nil {
		return out
	}
	for _, n := range notes {
		linked := false
		for _, nn := range n.NotebookNotes {
			if nn.NotebookID == notebookID {
				linked = true
				break
			}
		}
		if !linked {
			continue
		}
		if k := strings.ToLower(strings.TrimSpace(n.Usage)); k != "" {
			out[k] = n.ID
		}
		if k := strings.ToLower(strings.TrimSpace(n.Entry)); k != "" {
			out[k] = n.ID
		}
	}
	return out
}

// noteIDForDef looks up the DB note ID for a definition. Tries Expression
// first, then Definition (some entries are keyed by definition text). Zero
// when not found.
func noteIDForDef(byExpr map[string]int64, def notebook.Note) int64 {
	if id, ok := byExpr[strings.ToLower(strings.TrimSpace(def.Expression))]; ok {
		return id
	}
	if id, ok := byExpr[strings.ToLower(strings.TrimSpace(def.Definition))]; ok {
		return id
	}
	return 0
}

// lookupConceptForWord returns the concept context for a definition, or
// empty values when the word isn't a concept member. byExpression maps each
// member expression to its head; byHead carries the umbrella meaning + the
// ordered member list. Falls back to Definition when Expression doesn't
// match (some entries use the dictionary form as the canonical name).
func lookupConceptForWord(
	def notebook.Note,
	byExpression map[string]string,
	byHead map[string]notebook.DefinitionConceptInfo,
) (head string, members []string, meaning string) {
	if len(byExpression) == 0 {
		return "", nil, ""
	}
	h, ok := byExpression[def.Expression]
	if !ok && def.Definition != "" {
		h, ok = byExpression[def.Definition]
	}
	if !ok {
		return "", nil, ""
	}
	info := byHead[h]
	return h, append([]string(nil), info.Members...), info.Meaning
}

// findLearningInfoFull returns full learning info including skip status.
func (h *NotebookHandler) findLearningInfoFull(
	learningHistory []notebook.LearningHistory,
	event, sceneTitle string,
	def notebook.Note,
) learningInfo {
	matchesExpr := func(expr notebook.LearningHistoryExpression) bool {
		return expr.Expression == def.Expression || expr.Expression == def.Definition
	}
	extractInfo := func(expr notebook.LearningHistoryExpression) learningInfo {
		var nextReview string
		if len(expr.LearnedLogs) > 0 {
			if last := expr.LearnedLogs[0]; last.IntervalDays > 0 {
				nextReview = last.LearnedAt.AddDate(0, 0, last.IntervalDays).Format("2006-01-02")
			}
		}
		return learningInfo{
			status:         expr.GetLatestStatus(),
			nextReviewDate: nextReview,
			isSkipped:      expr.SkippedAt.IsSkippedAny(),
			skippedTypes:   expr.SkippedAt.SkippedTypes(),
		}
	}

	for _, hist := range learningHistory {
		if hist.Metadata.Title != event {
			continue
		}
		for _, scene := range hist.Scenes {
			if scene.Metadata.Title != sceneTitle {
				continue
			}
			for _, expr := range scene.Expressions {
				if matchesExpr(expr) {
					return extractInfo(expr)
				}
			}
		}
		if hist.Metadata.Type == "flashcard" {
			for _, expr := range hist.Expressions {
				if matchesExpr(expr) {
					return extractInfo(expr)
				}
			}
		}
	}
	return learningInfo{status: notebook.LearnedStatusLearning}
}

// ExportNotebookPDF generates a PDF for a notebook and returns its content.
func (h *NotebookHandler) ExportNotebookPDF(
	ctx context.Context,
	req *connect.Request[apiv1.ExportNotebookPDFRequest],
) (*connect.Response[apiv1.ExportNotebookPDFResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}

	notebookID := req.Msg.GetNotebookId()
	userID, _ := auth.UserIDFromContext(ctx)

	if err := h.ensureNotebookVisible(ctx, userID, notebookID); err != nil {
		return nil, err
	}

	reader, err := h.newReader()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create notebook reader: %w", err))
	}

	storyNotebooks, err := reader.ReadStoryNotebooks(notebookID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read story notebooks: %w", err))
	}

	learningHistory, err := h.loadLearningHistory(userID, notebookID)
	if err != nil {
		return nil, err
	}

	preserveOrder := reader.IsBook(notebookID)
	filtered, err := notebook.FilterStoryNotebooks(storyNotebooks, learningHistory, h.dictionaryMap, false, true, false, preserveOrder, notebook.QuizTypeNotebook)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("filter story notebooks: %w", err))
	}

	tmpDir, err := os.MkdirTemp("", "langner-pdf-*")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create temp directory: %w", err))
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	mdPath := filepath.Join(tmpDir, notebookID+".md")
	mdFile, err := os.Create(mdPath)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create temp markdown file: %w", err))
	}
	defer func() { _ = mdFile.Close() }()

	templateData := notebook.ConvertToAssetsStoryTemplate(filtered)
	if err := assets.WriteStoryNotebook(mdFile, h.templatesConfig.StoryNotebookTemplate, templateData); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("write story notebook: %w", err))
	}
	if err := mdFile.Close(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("close markdown file: %w", err))
	}

	pdfPath, err := pdf.ConvertMarkdownToPDF(mdPath)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("convert to PDF: %w", err))
	}

	pdfContent, err := os.ReadFile(pdfPath)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read PDF file: %w", err))
	}

	return connect.NewResponse(&apiv1.ExportNotebookPDFResponse{
		PdfContent: pdfContent,
		Filename:   notebookID + ".pdf",
	}), nil
}

// LookupWord looks up a word definition using dictionary cache, RapidAPI, or OpenAI.
func (h *NotebookHandler) LookupWord(
	ctx context.Context,
	req *connect.Request[apiv1.LookupWordRequest],
) (*connect.Response[apiv1.LookupWordResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}

	word := req.Msg.GetWord()

	if resp, ok := h.dictionaryMap[word]; ok {
		return connect.NewResponse(rapidAPIToLookupResponse(word, resp, "dictionary")), nil
	}

	if h.dictionaryReader != nil {
		resp, err := h.dictionaryReader.Lookup(ctx, word)
		if err != nil {
			slog.Warn("dictionary lookup failed", "word", word, "error", err)
		} else if len(resp.Results) > 0 {
			return connect.NewResponse(rapidAPIToLookupResponse(word, resp, "dictionary")), nil
		}
	}

	if h.clientResolver != nil {
		userID, _ := auth.UserIDFromContext(ctx)
		// Resolve the AI client from the user's credential. Word lookup is a
		// convenience, not a graded action, so a missing/invalid key degrades
		// gracefully to the dictionary-only response instead of erroring.
		client, resolveErr := h.clientResolver.ResolveClient(ctx, userID)
		if resolveErr != nil {
			slog.Warn("openai word lookup skipped — client unavailable", "word", word, "error", resolveErr)
		} else if aiResp, err := client.LookupWord(ctx, inference.LookupWordRequest{
			Word:    word,
			Context: req.Msg.GetContext(),
		}); err != nil {
			slog.Warn("openai word lookup failed", "word", word, "error", err)
		} else if len(aiResp.Definitions) > 0 {
			var defs []*apiv1.WordDefinition
			for _, d := range aiResp.Definitions {
				defs = append(defs, &apiv1.WordDefinition{
					PartOfSpeech:  d.PartOfSpeech,
					Definition:    d.Definition,
					Pronunciation: d.Pronunciation,
					Examples:      d.Examples,
					Synonyms:      d.Synonyms,
					Antonyms:      d.Antonyms,
					Origin:        d.Origin,
				})
			}
			return connect.NewResponse(&apiv1.LookupWordResponse{
				Word:        word,
				Definitions: defs,
				Source:      "openai",
			}), nil
		}
	}

	return connect.NewResponse(&apiv1.LookupWordResponse{Word: word}), nil
}

func rapidAPIToLookupResponse(word string, resp rapidapi.Response, source string) *apiv1.LookupWordResponse {
	var defs []*apiv1.WordDefinition
	for _, r := range resp.Results {
		defs = append(defs, &apiv1.WordDefinition{
			PartOfSpeech:  r.PartOfSpeech,
			Definition:    r.Definition,
			Examples:      r.Examples,
			Synonyms:      r.Synonyms,
			Pronunciation: resp.Pronunciation.All,
			Origin:        strings.Join(r.Derivation, ", "),
		})
	}
	return &apiv1.LookupWordResponse{
		Word:        word,
		Definitions: defs,
		Source:      source,
	}
}

// GetEtymologyNotebook returns the contents of an etymology notebook.
func (h *NotebookHandler) GetEtymologyNotebook(
	ctx context.Context,
	req *connect.Request[apiv1.GetEtymologyNotebookRequest],
) (*connect.Response[apiv1.GetEtymologyNotebookResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}

	notebookID := req.Msg.GetNotebookId()
	userID, _ := auth.UserIDFromContext(ctx)

	if err := h.ensureNotebookVisible(ctx, userID, notebookID); err != nil {
		return nil, err
	}

	reader, err := h.newReader()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create notebook reader: %w", err))
	}

	origins, err := reader.ReadEtymologyNotebook(notebookID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("etymology notebook %s not found", notebookID))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read etymology notebook: %w", err))
	}

	// Learning histories (for the per-word etymology-origin exclusion badge) and
	// a lazily-filled DB note-id cache (so the browse page can call
	// SkipWord/ResumeWord with a stable note_id, exactly like the vocabulary
	// notebook-detail page does off NotebookWord). Both are keyed per notebook
	// because a definition can come from any book. Skip exclusion is read via
	// the SAME key SkipWord wrote (invariant L2).
	learningHistories, _ := h.loadHistories(userID)
	noteIDCache := make(map[string]map[string]int64)
	noteIDsFor := func(nbName string) map[string]int64 {
		if m, ok := noteIDCache[nbName]; ok {
			return m
		}
		m := h.loadNoteIDsForNotebook(ctx, nbName)
		noteIDCache[nbName] = m
		return m
	}

	// originKey returns the (origin, sessionTitle) tuple used for strict
	// per-sense binding. Two senses of the same origin (e.g. "ana" in Session
	// 13 vs Session 16) have different keys and accumulate separate word
	// counts.
	originKey := func(origin, sessionTitle string) string {
		return strings.ToLower(strings.TrimSpace(origin)) + "\x00" + sessionTitle
	}

	originWordCounts := make(map[string]int)
	originSet := make(map[string]bool)
	originFormsByKey := make(map[string][]notebook.EtymologyOriginForm)
	for _, o := range origins {
		k := originKey(o.Origin, o.SessionTitle)
		originSet[k] = true
		if len(o.Forms) > 0 {
			originFormsByKey[k] = o.Forms
		}
	}

	// addDefinition deduplicates and appends a definition. sessionTitle is
	// the definition's parent metadata.title; under strict mode an empty
	// sessionTitle means the definition cannot bind to any origin's sense
	// and is silently skipped.
	var definitions []*apiv1.EtymologyDefinition
	seen := make(map[string]bool)
	addDefinition := func(expr, meaning, partOfSpeech, note string, examples, contexts []string, originParts []notebook.OriginPartRef, nbName, sessionTitle, id string) {
		key := strings.ToLower(expr) + "|" + nbName + "|" + sessionTitle
		if seen[key] {
			return
		}
		seen[key] = true
		if sessionTitle == "" {
			return
		}
		matched := false
		for _, ref := range originParts {
			if originSet[originKey(ref.Origin, sessionTitle)] {
				matched = true
				break
			}
		}
		if !matched {
			return
		}
		var parts []*apiv1.EtymologyOriginPart
		for _, ref := range originParts {
			k := originKey(ref.Origin, sessionTitle)
			var forms []*apiv1.EtymologyOriginForm
			for _, f := range originFormsByKey[k] {
				forms = append(forms, &apiv1.EtymologyOriginForm{
					Form: f.Form,
					Role: f.Role,
					Note: f.Note,
				})
			}
			parts = append(parts, &apiv1.EtymologyOriginPart{
				Origin:   ref.Origin,
				Language: ref.Language,
				FromForm: ref.FromForm,
				Forms:    forms,
			})
			if originSet[k] {
				originWordCounts[k]++
			}
		}
		// Per-word etymology-origin exclusion state for the browse page's
		// Exclude/Resume control. is_skipped is specifically the
		// etymology-origin exclusion; skipped_quiz_types lists every mode the
		// word is excluded from; note_id lets the frontend call
		// SkipWord/ResumeWord directly (zero when the DB isn't populated).
		histories := learningHistories[nbName]
		isSkipped := notebook.IsExpressionExcludedForQuizType(histories, id, notebook.QuizTypeEtymologyOrigin, expr)
		var skippedTypes []string
		if e := notebook.FindExpressionInHistories(histories, id, expr); e != nil {
			skippedTypes = e.SkippedAt.SkippedTypes()
		}
		var noteID int64
		if ids := noteIDsFor(nbName); len(ids) > 0 {
			noteID = ids[strings.ToLower(strings.TrimSpace(expr))]
		}
		definitions = append(definitions, &apiv1.EtymologyDefinition{
			Expression:       expr,
			Meaning:          meaning,
			PartOfSpeech:     partOfSpeech,
			Note:             note,
			Examples:         examples,
			Contexts:         contexts,
			OriginParts:      parts,
			NotebookName:     nbName,
			IsSkipped:        isSkipped,
			SkippedQuizTypes: skippedTypes,
			NoteId:           noteID,
		})
	}

	// Story and flashcard definitions don't carry session metadata, so they
	// can't bind to a specific sense under strict mode. They're scanned for
	// future when stories/flashcards adopt session titles.
	for nbID := range reader.GetStoryIndexes() {
		stories, err := reader.ReadStoryNotebooks(nbID)
		if err != nil {
			continue
		}
		for _, story := range stories {
			for _, scene := range story.Scenes {
				for _, def := range scene.Definitions {
					if len(def.OriginParts) == 0 {
						continue
					}
					var contexts []string
					contexts = append(contexts, scene.Statements...)
					for _, conv := range scene.Conversations {
						contexts = append(contexts, conv.Speaker+": "+conv.Quote)
					}
					addDefinition(def.Expression, def.Meaning, def.PartOfSpeech, def.Memo, def.Examples.Texts(), contexts, def.OriginParts, nbID, "", def.ID)
				}
			}
		}
	}

	// Etymology session-embedded definitions carry their parent session's
	// title via SessionTitle (set in Phase 1's reader).
	for _, def := range reader.ReadAllEtymologyDefinitions() {
		if len(def.OriginParts) == 0 {
			continue
		}
		addDefinition(def.GetExpression(), def.Meaning, def.PartOfSpeech, def.Note, nil, nil, def.OriginParts, def.NotebookName, def.SessionTitle, "")
	}

	for nbID := range reader.GetFlashcardIndexes() {
		notebooks, err := reader.ReadFlashcardNotebooks(nbID)
		if err != nil {
			continue
		}
		for _, nb := range notebooks {
			for _, card := range nb.Cards {
				if len(card.OriginParts) == 0 {
					continue
				}
				addDefinition(card.Expression, card.Meaning, card.PartOfSpeech, card.Memo, card.Examples.Texts(), nil, card.OriginParts, nbID, "", card.ID)
			}
		}
	}

	// Standalone definitions books: the outer map key is the parent
	// metadata.title — the binding key for matching origin senses to
	// the definition's source session.
	storyIndexes := reader.GetStoryIndexes()
	flashcardIndexes := reader.GetFlashcardIndexes()
	for _, nbID := range reader.GetDefinitionsBookIDs() {
		if _, isStory := storyIndexes[nbID]; isStory {
			continue
		}
		if _, isFlashcard := flashcardIndexes[nbID]; isFlashcard {
			continue
		}
		defs, ok := reader.GetDefinitionsNotes(nbID)
		if !ok {
			continue
		}
		for sessionTitle, sceneDefs := range defs {
			for _, notes := range sceneDefs {
				for _, note := range notes {
					if len(note.OriginParts) == 0 {
						continue
					}
					addDefinition(note.Expression, note.Meaning, note.PartOfSpeech, note.Memo, note.Examples.Texts(), nil, note.OriginParts, nbID, sessionTitle, note.ID)
				}
			}
		}
	}

	// Load book-scoped semantic concepts and relations directly from YAML
	// (the same source ingestion uses). Concepts with the same concept_key
	// across sessions merge into one entry, with members unioned. Relations
	// reference concept keys; the response carries them as keys so the
	// frontend doesn't need to map IDs.
	concepts, conceptKeysByOrigin, err := h.loadEtymologyConcepts(ctx, reader, notebookID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load semantic concepts: %w", err))
	}

	// Build proto origins with per-sense word counts
	var protoOrigins []*apiv1.EtymologyOriginPart
	for _, o := range origins {
		var forms []*apiv1.EtymologyOriginForm
		for _, f := range o.Forms {
			forms = append(forms, &apiv1.EtymologyOriginForm{
				Form: f.Form,
				Role: f.Role,
				Note: f.Note,
			})
		}
		// Concept membership for this origin sense — empty when the origin
		// participates in no concept on this notebook.
		var conceptKeys []string
		if keys, ok := conceptKeysByOrigin[originKey(o.Origin, o.SessionTitle)]; ok {
			conceptKeys = keys
		}
		protoOrigins = append(protoOrigins, &apiv1.EtymologyOriginPart{
			Origin:       o.Origin,
			Type:         o.Type,
			Language:     o.Language,
			Meaning:      o.Meaning,
			WordCount:    int32(originWordCounts[originKey(o.Origin, o.SessionTitle)]),
			Forms:        forms,
			ConceptKeys:  conceptKeys,
			SessionTitle: o.SessionTitle,
		})
	}

	// Group origins by meaning for the "By Meaning" tab
	meaningMap := make(map[string][]*apiv1.EtymologyOriginPart)
	var meaningOrder []string
	for _, o := range protoOrigins {
		meaning := o.Meaning
		if _, exists := meaningMap[meaning]; !exists {
			meaningOrder = append(meaningOrder, meaning)
		}
		meaningMap[meaning] = append(meaningMap[meaning], o)
	}

	var meaningGroups []*apiv1.EtymologyMeaningGroup
	for _, meaning := range meaningOrder {
		meaningGroups = append(meaningGroups, &apiv1.EtymologyMeaningGroup{
			Meaning: meaning,
			Origins: meaningMap[meaning],
		})
	}

	return connect.NewResponse(&apiv1.GetEtymologyNotebookResponse{
		Origins:         protoOrigins,
		Definitions:     definitions,
		MeaningGroups:   meaningGroups,
		OriginCount:     int32(len(origins)),
		DefinitionCount: int32(len(definitions)),
		Concepts:        concepts,
	}), nil
}

// loadEtymologyConcepts merges per-session concept declarations into one
// proto entry per (notebook_id, concept_key) and attaches each concept's
// outgoing relations. Returns the proto list plus a map keyed by the same
// (origin, sessionTitle) tuple GetEtymologyNotebook uses for word counts,
// containing the concept keys whose membership includes that origin sense.
//
// Members are still session-scoped at the YAML layer (per the schema), but
// across sessions of the same book they fold into one concept whose
// members[] is the union. session_title on each member preserves
// provenance for the frontend to bucket "this session vs other" rendering.
func (h *NotebookHandler) loadEtymologyConcepts(
	ctx context.Context,
	reader *notebook.Reader,
	notebookID string,
) ([]*apiv1.SemanticConcept, map[string][]string, error) {
	conceptSrc := notebook.NewYAMLSemanticConceptSource(reader)
	conceptRows, err := conceptSrc.FindAll(ctx)
	if err != nil {
		return nil, nil, err
	}
	relSrc := notebook.NewYAMLConceptRelationSource(reader)
	relRows, err := relSrc.FindAll(ctx)
	if err != nil {
		return nil, nil, err
	}

	type conceptAcc struct {
		proto   *apiv1.SemanticConcept
		members map[string]bool // (origin, language, sessionTitle) dedupe key
	}
	concepts := make(map[string]*conceptAcc)
	var order []string
	originKey := func(origin, sessionTitle string) string {
		return strings.ToLower(strings.TrimSpace(origin)) + "\x00" + sessionTitle
	}
	conceptKeysByOrigin := make(map[string][]string)
	seenOriginConcept := make(map[string]bool) // (originKey, conceptKey) -> already tagged

	for _, row := range conceptRows {
		if row.NotebookID != notebookID {
			continue
		}
		acc, ok := concepts[row.Key]
		if !ok {
			acc = &conceptAcc{
				proto: &apiv1.SemanticConcept{
					NotebookId: row.NotebookID,
					ConceptKey: row.Key,
					Meaning:    row.Meaning,
					Note:       row.Note,
				},
				members: make(map[string]bool),
			}
			concepts[row.Key] = acc
			order = append(order, row.Key)
		}
		for _, m := range row.Members {
			dedup := strings.ToLower(strings.TrimSpace(m.Origin)) + "|" + m.Language + "|" + row.SessionTitle
			if acc.members[dedup] {
				continue
			}
			acc.members[dedup] = true
			acc.proto.Members = append(acc.proto.Members, &apiv1.SemanticConceptMember{
				Origin: &apiv1.EtymologyOriginPart{
					Origin:   m.Origin,
					Language: m.Language,
				},
				SessionTitle: row.SessionTitle,
			})
			// Tag this origin sense with the concept key so the per-
			// origin response can render its concept memberships.
			tagKey := originKey(m.Origin, row.SessionTitle) + "\x01" + row.Key
			if !seenOriginConcept[tagKey] {
				seenOriginConcept[tagKey] = true
				ok := originKey(m.Origin, row.SessionTitle)
				conceptKeysByOrigin[ok] = append(conceptKeysByOrigin[ok], row.Key)
			}
		}
	}

	for _, rel := range relRows {
		if rel.NotebookID != notebookID {
			continue
		}
		acc, ok := concepts[rel.FromKey]
		if !ok {
			// Endpoint not declared in this book — skip rather than emit
			// a half-built concept; the validator already warned.
			continue
		}
		acc.proto.OutgoingRelations = append(acc.proto.OutgoingRelations, &apiv1.ConceptRelation{
			Type:           rel.Type,
			IsDirected:     rel.IsDirected,
			FromConceptKey: rel.FromKey,
			ToConceptKey:   rel.ToKey,
		})
		// Materialise the reverse direction for symmetric relations so
		// either endpoint's outgoing_relations carries the edge — frontend
		// then doesn't need to scan every concept to find inbound links.
		if !rel.IsDirected {
			revAcc, ok := concepts[rel.ToKey]
			if ok {
				revAcc.proto.OutgoingRelations = append(revAcc.proto.OutgoingRelations, &apiv1.ConceptRelation{
					Type:           rel.Type,
					IsDirected:     false,
					FromConceptKey: rel.ToKey,
					ToConceptKey:   rel.FromKey,
				})
			}
		}
	}

	out := make([]*apiv1.SemanticConcept, 0, len(order))
	for _, k := range order {
		out = append(out, concepts[k].proto)
	}
	return out, conceptKeysByOrigin, nil
}

// RegisterDefinition adds a definition via the repository.
func (h *NotebookHandler) RegisterDefinition(
	ctx context.Context,
	req *connect.Request[apiv1.RegisterDefinitionRequest],
) (*connect.Response[apiv1.RegisterDefinitionResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	userID, _ := auth.UserIDFromContext(ctx)
	if err := h.ensureNotebookVisible(ctx, userID, req.Msg.GetNotebookId()); err != nil {
		return nil, err
	}
	defsDir := "notebooks/definitions"
	if len(h.notebooksConfig.DefinitionsDirectories) > 0 && h.notebooksConfig.DefinitionsDirectories[0] != "" {
		defsDir = h.notebooksConfig.DefinitionsDirectories[0]
	}
	notebookIDRaw := req.Msg.GetNotebookId()
	checkPath := filepath.Join(defsDir, filepath.FromSlash(notebookIDRaw)+".yml")
	rel, err := filepath.Rel(defsDir, checkPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid notebook_id"))
	}
	expression := req.Msg.GetExpression()
	meaning := req.Msg.GetMeaning()
	note := &notebook.NoteRecord{
		Usage: expression, Entry: expression, Meaning: meaning,
		DefinitionsDir: defsDir, NotebookFile: req.Msg.GetNotebookFile(),
		SceneIndex:   int(req.Msg.GetSceneIndex()),
		PartOfSpeech: req.Msg.GetPartOfSpeech(), Examples: req.Msg.GetExamples(),
		NotebookNotes: []notebook.NotebookNote{{NotebookType: "book", NotebookID: notebookIDRaw, Group: req.Msg.GetNotebookFile()}},
	}
	if err := h.noteRepository.Create(ctx, note); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create note: %w", err))
	}
	return connect.NewResponse(&apiv1.RegisterDefinitionResponse{}), nil
}

// DeleteDefinition removes a definition via the repository.
func (h *NotebookHandler) DeleteDefinition(
	ctx context.Context,
	req *connect.Request[apiv1.DeleteDefinitionRequest],
) (*connect.Response[apiv1.DeleteDefinitionResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	userID, _ := auth.UserIDFromContext(ctx)
	if err := h.ensureNotebookVisible(ctx, userID, req.Msg.GetNotebookId()); err != nil {
		return nil, err
	}
	if err := h.noteRepository.Delete(ctx, req.Msg.GetNotebookId(), req.Msg.GetExpression()); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("delete note: %w", err))
	}
	return connect.NewResponse(&apiv1.DeleteDefinitionResponse{}), nil
}
