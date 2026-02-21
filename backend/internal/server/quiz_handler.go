// Package server provides Connect RPC handlers for the quiz service.
package server

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/protobuf/proto"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/gen-protos/api/v1/apiv1connect"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// quizNote holds data needed to grade an answer and update learning history.
type quizNote struct {
	notebookName string
	storyTitle   string
	sceneTitle   string
	expression   string
	meaning      string
	contexts     []inference.Context
}

// QuizHandler implements the QuizServiceHandler interface.
type QuizHandler struct {
	apiv1connect.UnimplementedQuizServiceHandler

	cfg           *config.Config
	openaiClient  inference.Client
	dictionaryMap map[string]rapidapi.Response

	mu        sync.Mutex
	noteStore map[int64]*quizNote
	nextID    int64
}

// NewQuizHandler creates a new QuizHandler.
func NewQuizHandler(cfg *config.Config, openaiClient inference.Client, dictionaryMap map[string]rapidapi.Response) *QuizHandler {
	return &QuizHandler{
		cfg:           cfg,
		openaiClient:  openaiClient,
		dictionaryMap: dictionaryMap,
		noteStore:     make(map[int64]*quizNote),
		nextID:        1,
	}
}

// GetQuizOptions returns available notebooks with review counts.
func (h *QuizHandler) GetQuizOptions(
	ctx context.Context,
	req *connect.Request[apiv1.GetQuizOptionsRequest],
) (*connect.Response[apiv1.GetQuizOptionsResponse], error) {
	reader, err := h.newReader()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create reader: %w", err))
	}

	learningHistories, err := notebook.NewLearningHistories(h.cfg.Notebooks.LearningNotesDirectory)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load learning histories: %w", err))
	}

	var summaries []*apiv1.NotebookSummary

	for id, index := range reader.GetStoryIndexes() {
		stories, err := reader.ReadStoryNotebooks(id)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read story notebooks(%s): %w", id, err))
		}

		filtered, err := notebook.FilterStoryNotebooks(
			stories, learningHistories[id], h.dictionaryMap,
			false, true, true, false,
		)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("filter story notebooks(%s): %w", id, err))
		}

		summaries = append(summaries, &apiv1.NotebookSummary{
			NotebookId:  id,
			Name:        index.Name,
			ReviewCount: int32(countStoryDefinitions(filtered)),
		})
	}

	for id, index := range reader.GetFlashcardIndexes() {
		notebooks, err := reader.ReadFlashcardNotebooks(id)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read flashcard notebooks(%s): %w", id, err))
		}

		filtered, err := notebook.FilterFlashcardNotebooks(
			notebooks, learningHistories[id], h.dictionaryMap, false,
		)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("filter flashcard notebooks(%s): %w", id, err))
		}

		summaries = append(summaries, &apiv1.NotebookSummary{
			NotebookId:  id,
			Name:        index.Name,
			ReviewCount: int32(countFlashcardCards(filtered)),
		})
	}

	return connect.NewResponse(&apiv1.GetQuizOptionsResponse{
		Notebooks: summaries,
	}), nil
}

// StartQuiz starts a quiz session and returns flashcards for the selected notebooks.
func (h *QuizHandler) StartQuiz(
	ctx context.Context,
	req *connect.Request[apiv1.StartQuizRequest],
) (*connect.Response[apiv1.StartQuizResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}

	reader, err := h.newReader()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create reader: %w", err))
	}

	learningHistories, err := notebook.NewLearningHistories(h.cfg.Notebooks.LearningNotesDirectory)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load learning histories: %w", err))
	}

	storyIndexes := reader.GetStoryIndexes()
	flashcardIndexes := reader.GetFlashcardIndexes()
	includeUnstudied := req.Msg.GetIncludeUnstudied()

	h.mu.Lock()
	h.noteStore = make(map[int64]*quizNote)
	h.nextID = 1
	h.mu.Unlock()

	var flashcards []*apiv1.Flashcard

	for _, notebookID := range req.Msg.GetNotebookIds() {
		_, isStory := storyIndexes[notebookID]
		_, isFlashcard := flashcardIndexes[notebookID]

		if !isStory && !isFlashcard {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("notebook %q not found", notebookID))
		}

		if isStory {
			cards, err := h.loadStoryCards(reader, notebookID, learningHistories, includeUnstudied)
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load story cards(%s): %w", notebookID, err))
			}
			flashcards = append(flashcards, cards...)
		}

		if isFlashcard {
			cards, err := h.loadFlashcardCards(reader, notebookID, learningHistories)
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load flashcard cards(%s): %w", notebookID, err))
			}
			flashcards = append(flashcards, cards...)
		}
	}

	return connect.NewResponse(&apiv1.StartQuizResponse{
		Flashcards: flashcards,
	}), nil
}

// SubmitAnswer grades a user's answer and updates learning history.
func (h *QuizHandler) SubmitAnswer(
	ctx context.Context,
	req *connect.Request[apiv1.SubmitAnswerRequest],
) (*connect.Response[apiv1.SubmitAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}

	noteID := req.Msg.GetNoteId()
	answer := req.Msg.GetAnswer()

	h.mu.Lock()
	note, ok := h.noteStore[noteID]
	h.mu.Unlock()

	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d not found in active quiz session", noteID))
	}

	results, err := h.openaiClient.AnswerMeanings(ctx, inference.AnswerMeaningsRequest{
		Expressions: []inference.Expression{
			{
				Expression:        note.expression,
				Meaning:           answer,
				Contexts:          note.contexts,
				IsExpressionInput: false,
				ResponseTimeMs:    req.Msg.GetResponseTimeMs(),
			},
		},
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade answer: %w", err))
	}
	if len(results.Answers) == 0 {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("no results returned from inference"))
	}

	result := results.Answers[0]
	isCorrect, reason, quality := extractAnswerResult(result)

	h.mu.Lock()
	defer h.mu.Unlock()

	if err := h.updateLearningHistory(note, isCorrect, quality, req.Msg.GetResponseTimeMs()); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update learning history: %w", err))
	}

	return connect.NewResponse(&apiv1.SubmitAnswerResponse{
		Correct: isCorrect,
		Meaning: note.meaning,
		Reason:  reason,
	}), nil
}

func (h *QuizHandler) newReader() (*notebook.Reader, error) {
	return notebook.NewReader(
		h.cfg.Notebooks.StoriesDirectories,
		h.cfg.Notebooks.FlashcardsDirectories,
		h.cfg.Notebooks.BooksDirectories,
		h.cfg.Notebooks.DefinitionsDirectories,
		h.dictionaryMap,
	)
}

func (h *QuizHandler) loadStoryCards(
	reader *notebook.Reader,
	notebookID string,
	learningHistories map[string][]notebook.LearningHistory,
	includeUnstudied bool,
) ([]*apiv1.Flashcard, error) {
	stories, err := reader.ReadStoryNotebooks(notebookID)
	if err != nil {
		return nil, fmt.Errorf("read story notebooks(%s): %w", notebookID, err)
	}

	filtered, err := notebook.FilterStoryNotebooks(
		stories, learningHistories[notebookID], h.dictionaryMap,
		false, includeUnstudied, true, false,
	)
	if err != nil {
		return nil, fmt.Errorf("filter story notebooks(%s): %w", notebookID, err)
	}

	var flashcards []*apiv1.Flashcard
	for _, story := range filtered {
		for _, scene := range story.Scenes {
			for _, definition := range scene.Definitions {
				expression := definition.Definition
				if expression == "" {
					expression = definition.Expression
				}

				examples, contexts := buildFromConversations(&scene, &definition)

				h.mu.Lock()
				noteID := h.nextID
				h.nextID++
				h.noteStore[noteID] = &quizNote{
					notebookName: notebookID,
					storyTitle:   story.Event,
					sceneTitle:   scene.Title,
					expression:   expression,
					meaning:      definition.Meaning,
					contexts:     contexts,
				}
				h.mu.Unlock()

				flashcards = append(flashcards, &apiv1.Flashcard{
					NoteId:   noteID,
					Entry:    expression,
					Examples: examples,
				})
			}
		}
	}

	return flashcards, nil
}

func (h *QuizHandler) loadFlashcardCards(
	reader *notebook.Reader,
	notebookID string,
	learningHistories map[string][]notebook.LearningHistory,
) ([]*apiv1.Flashcard, error) {
	notebooks, err := reader.ReadFlashcardNotebooks(notebookID)
	if err != nil {
		return nil, fmt.Errorf("read flashcard notebooks(%s): %w", notebookID, err)
	}

	filtered, err := notebook.FilterFlashcardNotebooks(
		notebooks, learningHistories[notebookID], h.dictionaryMap, false,
	)
	if err != nil {
		return nil, fmt.Errorf("filter flashcard notebooks(%s): %w", notebookID, err)
	}

	var flashcards []*apiv1.Flashcard
	for _, nb := range filtered {
		for _, card := range nb.Cards {
			expression := card.Definition
			if expression == "" {
				expression = card.Expression
			}

			var examples []*apiv1.Example
			for _, example := range card.Examples {
				examples = append(examples, &apiv1.Example{
					Text: example,
				})
			}

			h.mu.Lock()
			noteID := h.nextID
			h.nextID++
			h.noteStore[noteID] = &quizNote{
				notebookName: notebookID,
				storyTitle:   "flashcards",
				expression:   expression,
				meaning:      card.Meaning,
			}
			h.mu.Unlock()

			flashcards = append(flashcards, &apiv1.Flashcard{
				NoteId:   noteID,
				Entry:    expression,
				Examples: examples,
			})
		}
	}

	return flashcards, nil
}

func (h *QuizHandler) updateLearningHistory(note *quizNote, isCorrect bool, quality int, responseTimeMs int64) error {
	learningHistories, err := notebook.NewLearningHistories(h.cfg.Notebooks.LearningNotesDirectory)
	if err != nil {
		return fmt.Errorf("load learning histories: %w", err)
	}

	updater := notebook.NewLearningHistoryUpdater(learningHistories[note.notebookName])
	updater.UpdateOrCreateExpressionWithQuality(
		note.notebookName,
		note.storyTitle,
		note.sceneTitle,
		note.expression,
		isCorrect,
		true,
		quality,
		responseTimeMs,
		notebook.QuizTypeNotebook,
	)

	notePath := filepath.Join(h.cfg.Notebooks.LearningNotesDirectory, note.notebookName+".yml")
	if err := notebook.WriteYamlFile(notePath, updater.GetHistory()); err != nil {
		return fmt.Errorf("write learning history(%s): %w", notePath, err)
	}

	return nil
}

func countStoryDefinitions(stories []notebook.StoryNotebook) int {
	var count int
	for _, story := range stories {
		for _, scene := range story.Scenes {
			count += len(scene.Definitions)
		}
	}
	return count
}

func countFlashcardCards(notebooks []notebook.FlashcardNotebook) int {
	var count int
	for _, nb := range notebooks {
		count += len(nb.Cards)
	}
	return count
}

func buildFromConversations(scene *notebook.StoryScene, definition *notebook.Note) ([]*apiv1.Example, []inference.Context) {
	var examples []*apiv1.Example
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
		examples = append(examples, &apiv1.Example{
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

	return isCorrect, reason, quality
}

func validateRequest(msg proto.Message) *connect.Error {
	if err := protovalidate.Validate(msg); err != nil {
		connectErr := connect.NewError(connect.CodeInvalidArgument, err)
		var valErr *protovalidate.ValidationError
		if errors.As(err, &valErr) {
			var fieldViolations []*errdetails.BadRequest_FieldViolation
			for _, v := range valErr.Violations {
				fieldViolations = append(fieldViolations, &errdetails.BadRequest_FieldViolation{
					Field:       protovalidate.FieldPathString(v.Proto.GetField()),
					Description: v.Proto.GetMessage(),
				})
			}
			if detail, detailErr := connect.NewErrorDetail(&errdetails.BadRequest{
				FieldViolations: fieldViolations,
			}); detailErr == nil {
				connectErr.AddDetail(detail)
			}
		}
		return connectErr
	}
	return nil
}
