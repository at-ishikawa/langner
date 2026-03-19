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
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
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
	openaiClient     inference.Client
	noteRepository   notebook.NoteRepository
}

// NewNotebookHandler creates a new NotebookHandler.
// noteRepo is optional; pass nil when DB is not configured.
func NewNotebookHandler(notebooksConfig config.NotebooksConfig, templatesConfig config.TemplatesConfig, dictionaryMap map[string]rapidapi.Response, dictionaryReader *dictionary.Reader, openaiClient inference.Client, noteRepo notebook.NoteRepository) *NotebookHandler {
	return &NotebookHandler{
		notebooksConfig:  notebooksConfig,
		templatesConfig:  templatesConfig,
		dictionaryMap:    dictionaryMap,
		dictionaryReader: dictionaryReader,
		openaiClient:     openaiClient,
		noteRepository:   noteRepo,
	}
}

func (h *NotebookHandler) newReader() (*notebook.Reader, error) {
	return notebook.NewReader(
		h.notebooksConfig.StoriesDirectories,
		h.notebooksConfig.FlashcardsDirectories,
		h.notebooksConfig.BooksDirectories,
		h.notebooksConfig.DefinitionsDirectories,
		h.notebooksConfig.EtymologyDirectories,
		h.dictionaryMap,
	)
}


func (h *NotebookHandler) loadLearningHistory(notebookID string) ([]notebook.LearningHistory, error) {
	histories, err := notebook.NewLearningHistories(h.notebooksConfig.LearningNotesDirectory)
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

	reader, err := h.newReader()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create notebook reader: %w", err))
	}

	learningHistory, err := h.loadLearningHistory(notebookID)
	if err != nil {
		return nil, err
	}

	storyNotebooks, err := reader.ReadStoryNotebooks(notebookID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return h.getFlashcardNotebookDetail(notebookID, reader, learningHistory)
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read story notebooks: %w", err))
	}

	indexName := notebookID
	if idx, ok := reader.GetStoryIndexes()[notebookID]; ok {
		indexName = idx.Name
	}

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

				definitions = append(definitions, &apiv1.NotebookWord{
					Expression:     def.Expression,
					Definition:     def.Definition,
					Meaning:        def.Meaning,
					PartOfSpeech:   def.PartOfSpeech,
					Pronunciation:  def.Pronunciation,
					Examples:       def.Examples,
					Synonyms:       def.Synonyms,
					Antonyms:       def.Antonyms,
					LearningStatus: string(info.status),
					LearnedLogs:    convertLogsToProto(logs),
					EasinessFactor: info.easinessFactor,
					NextReviewDate: info.nextReviewDate,
					Origin:         def.Origin,
					IsSkipped:      info.isSkipped,
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
func (h *NotebookHandler) getFlashcardNotebookDetail(
	notebookID string,
	reader *notebook.Reader,
	learningHistory []notebook.LearningHistory,
) (*connect.Response[apiv1.GetNotebookDetailResponse], error) {
	flashcardNotebooks, err := reader.ReadFlashcardNotebooks(notebookID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("notebook %s not found", notebookID))
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

			definitions = append(definitions, &apiv1.NotebookWord{
				Expression:     card.Expression,
				Definition:     card.Definition,
				Meaning:        card.Meaning,
				PartOfSpeech:   card.PartOfSpeech,
				Pronunciation:  card.Pronunciation,
				Examples:       card.Examples,
				Synonyms:       card.Synonyms,
				Antonyms:       card.Antonyms,
				LearningStatus: string(info.status),
				LearnedLogs:    convertLogsToProto(logs),
				EasinessFactor: info.easinessFactor,
				NextReviewDate: info.nextReviewDate,
				Origin:         card.Origin,
				IsSkipped:      info.isSkipped,
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

// learningInfo holds learning status details for a definition.
type learningInfo struct {
	status         notebook.LearnedStatus
	easinessFactor float64
	nextReviewDate string
	isSkipped      bool
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
			easinessFactor: expr.EasinessFactor,
			nextReviewDate: nextReview,
			isSkipped:      expr.SkippedAt != "",
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

	learningHistory, err := h.loadLearningHistory(notebookID)
	if err != nil {
		return nil, err
	}

	preserveOrder := reader.IsBook(notebookID)
	filtered, err := notebook.FilterStoryNotebooks(storyNotebooks, learningHistory, h.dictionaryMap, false, true, false, preserveOrder)
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

	if h.openaiClient != nil {
		aiResp, err := h.openaiClient.LookupWord(ctx, inference.LookupWordRequest{
			Word:    word,
			Context: req.Msg.GetContext(),
		})
		if err != nil {
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

	// Build origin map for counting words per origin
	originWordCounts := make(map[string]int)

	// Find all definitions across all notebooks that have matching origin_parts
	var definitions []*apiv1.EtymologyDefinition
	originSet := make(map[string]bool)
	for _, o := range origins {
		originSet[strings.ToLower(o.Origin)] = true
	}

	// Search story notebooks
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
					if !hasMatchingOriginParts(def.OriginParts, originSet) {
						continue
					}
					var parts []*apiv1.EtymologyOriginPart
					for _, ref := range def.OriginParts {
						parts = append(parts, &apiv1.EtymologyOriginPart{
							Origin:   ref.Origin,
							Language: ref.Language,
						})
						originWordCounts[strings.ToLower(ref.Origin)]++
					}
					definitions = append(definitions, &apiv1.EtymologyDefinition{
						Expression:   def.Expression,
						Meaning:      def.Meaning,
						PartOfSpeech: def.PartOfSpeech,
						OriginParts:  parts,
						NotebookName: nbID,
					})
				}
			}
		}
	}

	// Search flashcard notebooks
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
				if !hasMatchingOriginParts(card.OriginParts, originSet) {
					continue
				}
				var parts []*apiv1.EtymologyOriginPart
				for _, ref := range card.OriginParts {
					parts = append(parts, &apiv1.EtymologyOriginPart{
						Origin:   ref.Origin,
						Language: ref.Language,
					})
					originWordCounts[strings.ToLower(ref.Origin)]++
				}
				definitions = append(definitions, &apiv1.EtymologyDefinition{
					Expression:   card.Expression,
					Meaning:      card.Meaning,
					PartOfSpeech: card.PartOfSpeech,
					OriginParts:  parts,
					NotebookName: nbID,
				})
			}
		}
	}

	// Build proto origins with word counts
	var protoOrigins []*apiv1.EtymologyOriginPart
	for _, o := range origins {
		protoOrigins = append(protoOrigins, &apiv1.EtymologyOriginPart{
			Origin:    o.Origin,
			Type:      o.Type,
			Language:  o.Language,
			Meaning:   o.Meaning,
			WordCount: int32(originWordCounts[strings.ToLower(o.Origin)]),
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
	}), nil
}

func hasMatchingOriginParts(refs []notebook.OriginPartRef, originSet map[string]bool) bool {
	for _, ref := range refs {
		if originSet[strings.ToLower(ref.Origin)] {
			return true
		}
	}
	return false
}

// RegisterDefinition adds a definition via the repository.
func (h *NotebookHandler) RegisterDefinition(
	ctx context.Context,
	req *connect.Request[apiv1.RegisterDefinitionRequest],
) (*connect.Response[apiv1.RegisterDefinitionResponse], error) {
	if err := validateRequest(req.Msg); err != nil { return nil, err }
	defsDir := "notebooks/definitions"
	if len(h.notebooksConfig.DefinitionsDirectories) > 0 && h.notebooksConfig.DefinitionsDirectories[0] != "" { defsDir = h.notebooksConfig.DefinitionsDirectories[0] }
	notebookIDRaw := req.Msg.GetNotebookId()
	checkPath := filepath.Join(defsDir, filepath.FromSlash(notebookIDRaw)+".yml")
	rel, err := filepath.Rel(defsDir, checkPath)
	if err != nil || strings.HasPrefix(rel, "..") { return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid notebook_id")) }
	expression := req.Msg.GetExpression()
	meaning := req.Msg.GetMeaning()
	note := &notebook.NoteRecord{
		Usage: expression, Entry: expression, Meaning: meaning,
		DefinitionsDir: defsDir, NotebookFile: req.Msg.GetNotebookFile(),
		SceneIndex: int(req.Msg.GetSceneIndex()),
		PartOfSpeech: req.Msg.GetPartOfSpeech(), Examples: req.Msg.GetExamples(),
		NotebookNotes: []notebook.NotebookNote{{NotebookType: "book", NotebookID: notebookIDRaw, Group: req.Msg.GetNotebookFile()}},
	}
	if err := h.noteRepository.Create(ctx, note); err != nil { return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create note: %w", err)) }
	return connect.NewResponse(&apiv1.RegisterDefinitionResponse{}), nil
}

// DeleteDefinition removes a definition via the repository.
func (h *NotebookHandler) DeleteDefinition(
	ctx context.Context,
	req *connect.Request[apiv1.DeleteDefinitionRequest],
) (*connect.Response[apiv1.DeleteDefinitionResponse], error) {
	if err := validateRequest(req.Msg); err != nil { return nil, err }
	if err := h.noteRepository.Delete(ctx, req.Msg.GetNotebookId(), req.Msg.GetExpression()); err != nil { return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("delete note: %w", err)) }
	return connect.NewResponse(&apiv1.DeleteDefinitionResponse{}), nil
}
