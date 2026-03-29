// Package server provides Connect RPC handlers for the quiz service.
package server

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/protobuf/proto"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/gen-protos/api/v1/apiv1connect"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
)

// QuizHandler implements the QuizServiceHandler interface.
type QuizHandler struct {
	apiv1connect.UnimplementedQuizServiceHandler

	svc            *quiz.Service
	noteRepository notebook.NoteRepository
	mu             sync.Mutex
	noteStore      map[int64]quiz.Card
	reverseStore   map[int64]quiz.ReverseCard
	freeformCards  []quiz.FreeformCard
	freeformStore  map[int64]quiz.FreeformCard
	etymologyStore map[int64]quiz.EtymologyCard
	etymologyCards []quiz.EtymologyCard // for freeform
	nextID         int64
}

// NewQuizHandler creates a new QuizHandler.
func NewQuizHandler(svc *quiz.Service) *QuizHandler {
	return &QuizHandler{
		svc:            svc,
		noteStore:      make(map[int64]quiz.Card),
		reverseStore:   make(map[int64]quiz.ReverseCard),
		freeformStore:  make(map[int64]quiz.FreeformCard),
		etymologyStore: make(map[int64]quiz.EtymologyCard),
		nextID:         1,
	}
}

// SetNoteRepository sets an optional note repository for DB-based card resolution.
// When set, handlers can look up cards from the database when no active quiz session exists.
func (h *QuizHandler) SetNoteRepository(repo notebook.NoteRepository) {
	h.noteRepository = repo
}

// GetQuizOptions returns available notebooks with review counts.
func (h *QuizHandler) GetQuizOptions(
	ctx context.Context,
	req *connect.Request[apiv1.GetQuizOptionsRequest],
) (*connect.Response[apiv1.GetQuizOptionsResponse], error) {
	summaries, err := h.svc.LoadNotebookSummaries()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load notebook summaries: %w", err))
	}

	sort.Slice(summaries, func(i, j int) bool {
		di, dj := summaries[i].LatestStoryDate, summaries[j].LatestStoryDate
		if !di.Equal(dj) {
			return di.After(dj)
		}
		return summaries[i].NotebookID < summaries[j].NotebookID
	})

	protoSummaries := make([]*apiv1.NotebookSummary, 0, len(summaries))
	for _, s := range summaries {
		protoSummaries = append(protoSummaries, &apiv1.NotebookSummary{
			NotebookId:            s.NotebookID,
			Name:                  s.Name,
			ReviewCount:           int32(s.ReviewCount),
			Kind:                  s.Kind,
			ReverseReviewCount:    int32(s.ReverseReviewCount),
			EtymologyReviewCount:  int32(s.EtymologyReviewCount),
		})
	}

	return connect.NewResponse(&apiv1.GetQuizOptionsResponse{
		Notebooks: protoSummaries,
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

	cards, err := h.svc.LoadCards(req.Msg.GetNotebookIds(), req.Msg.GetIncludeUnstudied())
	if err != nil {
		var notFoundErr *quiz.NotFoundError
		if errors.As(err, &notFoundErr) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load cards: %w", err))
	}

	localStore := make(map[int64]quiz.Card)
	var nextID int64 = 1

	var flashcards []*apiv1.Flashcard
	for _, card := range cards {
		noteID := nextID
		nextID++
		localStore[noteID] = card

		var examples []*apiv1.Example
		for _, ex := range card.Examples {
			examples = append(examples, &apiv1.Example{
				Text:    ex.Text,
				Speaker: ex.Speaker,
			})
		}

		flashcards = append(flashcards, &apiv1.Flashcard{
			NoteId:   noteID,
			Entry:    card.Entry,
			Examples: examples,
		})
	}

	h.mu.Lock()
	h.noteStore = localStore
	h.nextID = nextID
	h.mu.Unlock()

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
	card, ok := h.noteStore[noteID]
	h.mu.Unlock()

	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d not found in active quiz session", noteID))
	}

	grade, err := h.svc.GradeNotebookAnswer(ctx, card, answer, req.Msg.GetResponseTimeMs())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade answer: %w", err))
	}

	if err := h.svc.SaveResult(ctx, card, grade, req.Msg.GetResponseTimeMs()); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update learning history: %w", err))
	}

	learnedAt, nextReviewDate := h.svc.GetLatestLearnedInfo(card.NotebookName, card.Entry, notebook.QuizTypeNotebook)

	return connect.NewResponse(&apiv1.SubmitAnswerResponse{
		Correct:        grade.Correct,
		Meaning:        card.Meaning,
		Reason:         grade.Reason,
		WordDetail:     toProtoWordDetail(card.WordDetail),
		NextReviewDate: nextReviewDate,
		LearnedAt:      learnedAt,
		Images:         card.Images,
	}), nil
}

func toProtoWordDetail(wd quiz.WordDetail) *apiv1.WordDetail {
	if wd.Origin == "" && wd.Pronunciation == "" && wd.PartOfSpeech == "" && len(wd.Synonyms) == 0 && len(wd.Antonyms) == 0 && wd.Memo == "" {
		return nil
	}
	return &apiv1.WordDetail{
		Origin:        wd.Origin,
		Pronunciation: wd.Pronunciation,
		PartOfSpeech:  wd.PartOfSpeech,
		Synonyms:      wd.Synonyms,
		Antonyms:      wd.Antonyms,
		Memo:          wd.Memo,
	}
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

// StartReverseQuiz starts a reverse quiz session.
func (h *QuizHandler) StartReverseQuiz(
	ctx context.Context,
	req *connect.Request[apiv1.StartReverseQuizRequest],
) (*connect.Response[apiv1.StartReverseQuizResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}

	cards, err := h.svc.LoadReverseCards(req.Msg.GetNotebookIds(), req.Msg.GetListMissingContext())
	if err != nil {
		var notFoundErr *quiz.NotFoundError
		if errors.As(err, &notFoundErr) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load reverse cards: %w", err))
	}

	localStore := make(map[int64]quiz.ReverseCard)
	var nextID int64 = 1

	var flashcards []*apiv1.ReverseFlashcard
	for _, card := range cards {
		noteID := nextID
		nextID++
		localStore[noteID] = card

		var contexts []*apiv1.ContextSentence
		for _, ctx := range card.Contexts {
			contexts = append(contexts, &apiv1.ContextSentence{
				Context:       ctx.Context,
				MaskedContext: ctx.MaskedContext,
			})
		}

		flashcards = append(flashcards, &apiv1.ReverseFlashcard{
			NoteId:       noteID,
			Meaning:      card.Meaning,
			Contexts:     contexts,
			NotebookName: card.NotebookName,
			StoryTitle:   card.StoryTitle,
			SceneTitle:   card.SceneTitle,
		})
	}

	h.mu.Lock()
	h.reverseStore = localStore
	h.nextID = nextID
	h.mu.Unlock()

	return connect.NewResponse(&apiv1.StartReverseQuizResponse{
		Flashcards: flashcards,
	}), nil
}

// SubmitReverseAnswer grades a reverse quiz answer.
func (h *QuizHandler) SubmitReverseAnswer(
	ctx context.Context,
	req *connect.Request[apiv1.SubmitReverseAnswerRequest],
) (*connect.Response[apiv1.SubmitReverseAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}

	noteID := req.Msg.GetNoteId()
	answer := req.Msg.GetAnswer()

	h.mu.Lock()
	card, ok := h.reverseStore[noteID]
	h.mu.Unlock()

	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d not found in active reverse quiz session", noteID))
	}

	grade, err := h.svc.GradeReverseAnswer(ctx, card, answer, req.Msg.GetResponseTimeMs())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade answer: %w", err))
	}

	// Don't save synonym results — the frontend will offer a retry
	if grade.Classification != string(inference.ClassificationSynonym) {
		if err := h.svc.SaveReverseResult(ctx, card, grade, req.Msg.GetResponseTimeMs()); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update learning history: %w", err))
		}
	}

	var contexts []string
	for _, ctx := range card.Contexts {
		contexts = append(contexts, ctx.Context)
	}

	learnedAt, nextReviewDate := h.svc.GetLatestLearnedInfo(card.NotebookName, card.Expression, notebook.QuizTypeReverse)

	return connect.NewResponse(&apiv1.SubmitReverseAnswerResponse{
		Correct:        grade.Correct,
		Expression:     card.Expression,
		Meaning:        card.Meaning,
		Reason:         grade.Reason,
		Contexts:       contexts,
		WordDetail:     toProtoWordDetail(card.WordDetail),
		Classification: grade.Classification,
		NextReviewDate: nextReviewDate,
		LearnedAt:      learnedAt,
		Images:         card.Images,
	}), nil
}

// StartFreeformQuiz starts a freeform quiz session.
func (h *QuizHandler) StartFreeformQuiz(
	ctx context.Context,
	req *connect.Request[apiv1.StartFreeformQuizRequest],
) (*connect.Response[apiv1.StartFreeformQuizResponse], error) {
	cards, err := h.svc.LoadAllWords()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load all words: %w", err))
	}

	nextReviewDates, err := h.svc.GetFreeformNextReviewDates(cards)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get next review dates: %w", err))
	}

	h.mu.Lock()
	h.freeformCards = cards
	h.mu.Unlock()

	seen := make(map[string]struct{}, len(cards)*2)
	expressions := make([]string, 0, len(cards))
	addExpr := func(expr string) {
		lower := strings.ToLower(expr)
		if expr != "" {
			if _, ok := seen[lower]; !ok {
				seen[lower] = struct{}{}
				expressions = append(expressions, expr)
			}
		}
	}
	for _, card := range cards {
		addExpr(card.Expression)
		addExpr(card.OriginalExpression)
	}

	return connect.NewResponse(&apiv1.StartFreeformQuizResponse{
		WordCount:                int32(len(cards)),
		Expressions:              expressions,
		ExpressionNextReviewDate: nextReviewDates,
	}), nil
}

// SubmitFreeformAnswer grades a freeform quiz answer.
func (h *QuizHandler) SubmitFreeformAnswer(
	ctx context.Context,
	req *connect.Request[apiv1.SubmitFreeformAnswerRequest],
) (*connect.Response[apiv1.SubmitFreeformAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}

	h.mu.Lock()
	cards := h.freeformCards
	h.mu.Unlock()

	grade, err := h.svc.GradeFreeformAnswer(ctx, req.Msg.GetWord(), req.Msg.GetMeaning(), req.Msg.GetResponseTimeMs(), cards)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade answer: %w", err))
	}

	if grade.MatchedCard != nil {
		if err := h.svc.SaveFreeformResult(ctx, *grade.MatchedCard, grade, req.Msg.GetResponseTimeMs()); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update learning history: %w", err))
		}
	}

	var learnedAt, nextReviewDate string
	var noteID int64
	if grade.MatchedCard != nil {
		learnedAt, nextReviewDate = h.svc.GetLatestLearnedInfo(grade.MatchedCard.NotebookName, grade.MatchedCard.Expression, notebook.QuizTypeFreeform)

		h.mu.Lock()
		noteID = h.nextID
		h.nextID++
		h.freeformStore[noteID] = *grade.MatchedCard
		h.mu.Unlock()
	}

	return connect.NewResponse(&apiv1.SubmitFreeformAnswerResponse{
		Correct:      grade.Correct,
		Word:         grade.Word,
		Meaning:      grade.Meaning,
		Reason:       grade.Reason,
		Context:      grade.Context,
		NotebookName: grade.NotebookName,
		WordDetail: func() *apiv1.WordDetail {
			if grade.MatchedCard != nil {
				return toProtoWordDetail(grade.MatchedCard.WordDetail)
			}
			return nil
		}(),
		NextReviewDate: nextReviewDate,
		LearnedAt:      learnedAt,
		NoteId:         noteID,
		Images: func() []string {
			if grade.MatchedCard != nil {
				return grade.MatchedCard.Images
			}
			return nil
		}(),
	}), nil
}

// resolveCardInfo resolves a note_id to a CardInfo by first checking the in-memory
// session stores, then falling back to the database if no active quiz session has it.
func (h *QuizHandler) resolveCardInfo(ctx context.Context, noteID int64) (*quiz.CardInfo, error) {
	h.mu.Lock()
	if card, ok := h.noteStore[noteID]; ok {
		h.mu.Unlock()
		info := quiz.CardInfoFromCard(card)
		return &info, nil
	}
	if card, ok := h.reverseStore[noteID]; ok {
		h.mu.Unlock()
		info := quiz.CardInfoFromReverseCard(card)
		return &info, nil
	}
	if fcard, ok := h.freeformStore[noteID]; ok {
		h.mu.Unlock()
		info := quiz.CardInfoFromFreeformCard(fcard)
		return &info, nil
	}
	if ecard, found := h.etymologyStore[noteID]; found {
		h.mu.Unlock()
		info := quiz.CardInfo{
			NotebookName: ecard.NotebookName,
			Expression:   ecard.Expression,
		}
		return &info, nil
	}
	h.mu.Unlock()

	// Fall back to DB lookup
	if h.noteRepository == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d not found: no active quiz session and no database configured", noteID))
	}

	noteRecord, err := h.noteRepository.FindByID(ctx, noteID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d not found in database: %w", noteID, err))
	}

	// Determine notebook ID from notebook_notes
	var notebookName, group, subgroup string
	if len(noteRecord.NotebookNotes) > 0 {
		nn := noteRecord.NotebookNotes[0]
		notebookName = nn.NotebookID
		group = nn.Group
		subgroup = nn.Subgroup
	}
	if notebookName == "" {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d has no linked notebook", noteID))
	}

	expression := noteRecord.Entry
	if expression == "" {
		expression = noteRecord.Usage
	}

	info := quiz.CardInfo{
		NotebookName: notebookName,
		StoryTitle:   group,
		SceneTitle:   subgroup,
		Expression:   expression,
	}
	return &info, nil
}

// protoQuizTypeToNotebook converts a proto QuizType enum to a notebook.QuizType.
func protoQuizTypeToNotebook(qt apiv1.QuizType) notebook.QuizType {
	switch qt {
	case apiv1.QuizType_QUIZ_TYPE_REVERSE:
		return notebook.QuizTypeReverse
	case apiv1.QuizType_QUIZ_TYPE_FREEFORM:
		return notebook.QuizTypeFreeform
	case apiv1.QuizType_QUIZ_TYPE_ETYMOLOGY_BREAKDOWN:
		return notebook.QuizTypeEtymologyBreakdown
	case apiv1.QuizType_QUIZ_TYPE_ETYMOLOGY_ASSEMBLY:
		return notebook.QuizTypeEtymologyAssembly
	default:
		return notebook.QuizTypeNotebook
	}
}

// SkipWord marks a word as skipped so it won't appear in quizzes until the specified date.
func (h *QuizHandler) SkipWord(
	ctx context.Context,
	req *connect.Request[apiv1.SkipWordRequest],
) (*connect.Response[apiv1.SkipWordResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}

	info, err := h.resolveCardInfo(ctx, req.Msg.GetNoteId())
	if err != nil {
		return nil, err
	}

	quizType := protoQuizTypeToNotebook(req.Msg.GetQuizType())
	if err := h.svc.SkipWord(*info, req.Msg.GetSkipUntil(), quizType); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("skip word: %w", err))
	}

	return connect.NewResponse(&apiv1.SkipWordResponse{}), nil
}

// ResumeWord removes the skip from a word so it appears in quizzes again.
func (h *QuizHandler) ResumeWord(
	ctx context.Context,
	req *connect.Request[apiv1.ResumeWordRequest],
) (*connect.Response[apiv1.ResumeWordResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}

	info, err := h.resolveCardInfo(ctx, req.Msg.GetNoteId())
	if err != nil {
		return nil, err
	}

	quizType := protoQuizTypeToNotebook(req.Msg.GetQuizType())
	if err := h.svc.ResumeWord(*info, quizType); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("resume word: %w", err))
	}

	return connect.NewResponse(&apiv1.ResumeWordResponse{}), nil
}

// StartEtymologyQuiz starts an etymology quiz session.
func (h *QuizHandler) StartEtymologyQuiz(
	ctx context.Context,
	req *connect.Request[apiv1.StartEtymologyQuizRequest],
) (*connect.Response[apiv1.StartEtymologyQuizResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}

	cards, err := h.svc.LoadEtymologyCards(
		req.Msg.GetEtymologyNotebookIds(),
		req.Msg.GetDefinitionNotebookIds(),
		req.Msg.GetIncludeUnstudied(),
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load etymology cards: %w", err))
	}

	localStore := make(map[int64]quiz.EtymologyCard)
	var nextID int64 = 1

	var protoCards []*apiv1.EtymologyQuizCard
	for _, card := range cards {
		cardID := nextID
		nextID++
		localStore[cardID] = card

		var parts []*apiv1.EtymologyQuizOriginPart
		for _, p := range card.OriginParts {
			parts = append(parts, &apiv1.EtymologyQuizOriginPart{
				Origin:   p.Origin,
				Type:     p.Type,
				Language: p.Language,
				Meaning:  p.Meaning,
			})
		}

		protoCards = append(protoCards, &apiv1.EtymologyQuizCard{
			CardId:       cardID,
			Expression:   card.Expression,
			Meaning:      card.Meaning,
			OriginParts:  parts,
			NotebookName: card.NotebookName,
		})
	}

	h.mu.Lock()
	h.etymologyStore = localStore
	h.nextID = nextID
	h.mu.Unlock()

	return connect.NewResponse(&apiv1.StartEtymologyQuizResponse{
		Cards: protoCards,
	}), nil
}

// SubmitEtymologyBreakdownAnswer grades a breakdown answer.
func (h *QuizHandler) SubmitEtymologyBreakdownAnswer(
	ctx context.Context,
	req *connect.Request[apiv1.SubmitEtymologyBreakdownAnswerRequest],
) (*connect.Response[apiv1.SubmitEtymologyBreakdownAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}

	cardID := req.Msg.GetCardId()

	h.mu.Lock()
	card, ok := h.etymologyStore[cardID]
	h.mu.Unlock()

	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("card %d not found in active etymology quiz session", cardID))
	}

	var userOrigins []inference.EtymologyUserOrigin
	for _, a := range req.Msg.GetAnswers() {
		userOrigins = append(userOrigins, inference.EtymologyUserOrigin{
			Origin:  a.GetOrigin(),
			Meaning: a.GetMeaning(),
		})
	}

	grade, err := h.svc.GradeEtymologyBreakdownAnswer(ctx, card, userOrigins, req.Msg.GetResponseTimeMs())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade etymology breakdown: %w", err))
	}

	if err := h.svc.SaveEtymologyResult(card, grade.Quality, grade.Correct, req.Msg.GetResponseTimeMs(), notebook.QuizTypeEtymologyBreakdown); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("save etymology result: %w", err))
	}

	learnedAt, nextReviewDate := h.svc.GetLatestLearnedInfo(card.NotebookName, card.Expression, notebook.QuizTypeEtymologyBreakdown)
	reader, err := h.svc.NewReader()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create reader for related definitions: %w", err))
	}
	relatedDefs := h.svc.FindRelatedDefinitions(reader, card)

	var protoGrades []*apiv1.EtymologyOriginGrade
	for _, g := range grade.OriginGrades {
		pg := &apiv1.EtymologyOriginGrade{
			UserOrigin:     g.UserOrigin,
			UserMeaning:    g.UserMeaning,
			OriginCorrect:  g.OriginCorrect,
			MeaningCorrect: g.MeaningCorrect,
		}
		if g.CorrectOrigin != nil {
			pg.CorrectOrigin = &apiv1.EtymologyOriginAnswer{
				Origin:  g.CorrectOrigin.Origin,
				Meaning: g.CorrectOrigin.Meaning,
			}
		}
		protoGrades = append(protoGrades, pg)
	}

	var protoRelated []*apiv1.RelatedDefinition
	for _, r := range relatedDefs {
		protoRelated = append(protoRelated, &apiv1.RelatedDefinition{
			Expression:   r.Expression,
			Meaning:      r.Meaning,
			NotebookName: r.NotebookName,
		})
	}

	return connect.NewResponse(&apiv1.SubmitEtymologyBreakdownAnswerResponse{
		Correct:            grade.Correct,
		Reason:             grade.Reason,
		OriginGrades:       protoGrades,
		RelatedDefinitions: protoRelated,
		NextReviewDate:     nextReviewDate,
		LearnedAt:          learnedAt,
		NoteId:             req.Msg.GetCardId(),
		Images:             card.Images,
	}), nil
}

// SubmitEtymologyAssemblyAnswer grades an assembly answer.
func (h *QuizHandler) SubmitEtymologyAssemblyAnswer(
	ctx context.Context,
	req *connect.Request[apiv1.SubmitEtymologyAssemblyAnswerRequest],
) (*connect.Response[apiv1.SubmitEtymologyAssemblyAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}

	cardID := req.Msg.GetCardId()

	h.mu.Lock()
	card, ok := h.etymologyStore[cardID]
	h.mu.Unlock()

	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("card %d not found in active etymology quiz session", cardID))
	}

	grade, err := h.svc.GradeEtymologyAssemblyAnswer(ctx, card, req.Msg.GetAnswer(), req.Msg.GetResponseTimeMs())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade etymology assembly: %w", err))
	}

	if err := h.svc.SaveEtymologyResult(card, grade.Quality, grade.Correct, req.Msg.GetResponseTimeMs(), notebook.QuizTypeEtymologyAssembly); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("save etymology result: %w", err))
	}

	learnedAt, nextReviewDate := h.svc.GetLatestLearnedInfo(card.NotebookName, card.Expression, notebook.QuizTypeEtymologyAssembly)
	reader, err := h.svc.NewReader()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create reader for related definitions: %w", err))
	}
	relatedDefs := h.svc.FindRelatedDefinitions(reader, card)

	var parts []*apiv1.EtymologyQuizOriginPart
	for _, p := range card.OriginParts {
		parts = append(parts, &apiv1.EtymologyQuizOriginPart{
			Origin:   p.Origin,
			Type:     p.Type,
			Language: p.Language,
			Meaning:  p.Meaning,
		})
	}

	var protoRelated []*apiv1.RelatedDefinition
	for _, r := range relatedDefs {
		protoRelated = append(protoRelated, &apiv1.RelatedDefinition{
			Expression:   r.Expression,
			Meaning:      r.Meaning,
			NotebookName: r.NotebookName,
		})
	}

	return connect.NewResponse(&apiv1.SubmitEtymologyAssemblyAnswerResponse{
		Correct:            grade.Correct,
		Reason:             grade.Reason,
		CorrectExpression:  card.Expression,
		OriginParts:        parts,
		RelatedDefinitions: protoRelated,
		NextReviewDate:     nextReviewDate,
		LearnedAt:          learnedAt,
		NoteId:             req.Msg.GetCardId(),
		Images:             card.Images,
	}), nil
}

// StartEtymologyFreeformQuiz starts a freeform etymology quiz.
func (h *QuizHandler) StartEtymologyFreeformQuiz(
	ctx context.Context,
	req *connect.Request[apiv1.StartEtymologyFreeformQuizRequest],
) (*connect.Response[apiv1.StartEtymologyFreeformQuizResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}

	cards, err := h.svc.LoadEtymologyFreeformExpressions(
		req.Msg.GetEtymologyNotebookIds(),
		req.Msg.GetDefinitionNotebookIds(),
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load etymology freeform expressions: %w", err))
	}

	nextReviewDates, err := h.svc.GetEtymologyNextReviewDates(cards)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get etymology next review dates: %w", err))
	}

	h.mu.Lock()
	h.etymologyCards = cards
	h.mu.Unlock()

	seen := make(map[string]struct{})
	var expressions []string
	for _, card := range cards {
		lower := strings.ToLower(card.Expression)
		if _, ok := seen[lower]; !ok {
			seen[lower] = struct{}{}
			expressions = append(expressions, card.Expression)
		}
	}

	return connect.NewResponse(&apiv1.StartEtymologyFreeformQuizResponse{
		Expressions:    expressions,
		NextReviewDates: nextReviewDates,
	}), nil
}

// SubmitEtymologyFreeformAnswer grades a freeform etymology answer.
func (h *QuizHandler) SubmitEtymologyFreeformAnswer(
	ctx context.Context,
	req *connect.Request[apiv1.SubmitEtymologyFreeformAnswerRequest],
) (*connect.Response[apiv1.SubmitEtymologyFreeformAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}

	h.mu.Lock()
	cards := h.etymologyCards
	h.mu.Unlock()

	expression := req.Msg.GetExpression()

	// Find matching card
	var matchedCard *quiz.EtymologyCard
	for _, card := range cards {
		if strings.EqualFold(card.Expression, expression) {
			c := card
			matchedCard = &c
			break
		}
	}

	if matchedCard == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("expression %q not found", expression))
	}

	var userOrigins []inference.EtymologyUserOrigin
	for _, a := range req.Msg.GetAnswers() {
		userOrigins = append(userOrigins, inference.EtymologyUserOrigin{
			Origin:  a.GetOrigin(),
			Meaning: a.GetMeaning(),
		})
	}

	grade, err := h.svc.GradeEtymologyBreakdownAnswer(ctx, *matchedCard, userOrigins, req.Msg.GetResponseTimeMs())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade etymology freeform: %w", err))
	}

	// etymology_freeform shares SM-2 track with etymology_breakdown
	if err := h.svc.SaveEtymologyResult(*matchedCard, grade.Quality, grade.Correct, req.Msg.GetResponseTimeMs(), notebook.QuizTypeEtymologyBreakdown); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("save etymology result: %w", err))
	}

	learnedAt, nextReviewDate := h.svc.GetLatestLearnedInfo(matchedCard.NotebookName, matchedCard.Expression, notebook.QuizTypeEtymologyBreakdown)
	reader, err := h.svc.NewReader()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create reader for related definitions: %w", err))
	}
	relatedDefs := h.svc.FindRelatedDefinitions(reader, *matchedCard)

	var protoGrades []*apiv1.EtymologyOriginGrade
	for _, g := range grade.OriginGrades {
		pg := &apiv1.EtymologyOriginGrade{
			UserOrigin:     g.UserOrigin,
			UserMeaning:    g.UserMeaning,
			OriginCorrect:  g.OriginCorrect,
			MeaningCorrect: g.MeaningCorrect,
		}
		if g.CorrectOrigin != nil {
			pg.CorrectOrigin = &apiv1.EtymologyOriginAnswer{
				Origin:  g.CorrectOrigin.Origin,
				Meaning: g.CorrectOrigin.Meaning,
			}
		}
		protoGrades = append(protoGrades, pg)
	}

	var protoRelated []*apiv1.RelatedDefinition
	for _, r := range relatedDefs {
		protoRelated = append(protoRelated, &apiv1.RelatedDefinition{
			Expression:   r.Expression,
			Meaning:      r.Meaning,
			NotebookName: r.NotebookName,
		})
	}

	return connect.NewResponse(&apiv1.SubmitEtymologyFreeformAnswerResponse{
		Correct:            grade.Correct,
		Reason:             grade.Reason,
		OriginGrades:       protoGrades,
		RelatedDefinitions: protoRelated,
		NextReviewDate:     nextReviewDate,
		LearnedAt:          learnedAt,
		NotebookName:       matchedCard.NotebookName,
		NoteId:             0, // freeform cards are not stored with IDs
		Images:             matchedCard.Images,
	}), nil
}

// OverrideAnswer toggles the correctness of the most recent answer for a word.
func (h *QuizHandler) OverrideAnswer(
	ctx context.Context,
	req *connect.Request[apiv1.OverrideAnswerRequest],
) (*connect.Response[apiv1.OverrideAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}

	info, err := h.resolveCardInfo(ctx, req.Msg.GetNoteId())
	if err != nil {
		return nil, err
	}

	quizType := protoQuizTypeToNotebook(req.Msg.GetQuizType())
	nextReviewDate, err := h.svc.OverrideAnswer(*info, quizType)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("override answer: %w", err))
	}

	return connect.NewResponse(&apiv1.OverrideAnswerResponse{
		NextReviewDate: nextReviewDate,
	}), nil
}

// UndoOverrideAnswer reverts the most recent answer override for a word.
func (h *QuizHandler) UndoOverrideAnswer(
	ctx context.Context,
	req *connect.Request[apiv1.UndoOverrideAnswerRequest],
) (*connect.Response[apiv1.UndoOverrideAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}

	info, err := h.resolveCardInfo(ctx, req.Msg.GetNoteId())
	if err != nil {
		return nil, err
	}

	quizType := protoQuizTypeToNotebook(req.Msg.GetQuizType())
	nextReviewDate, err := h.svc.UndoOverrideAnswer(*info, quizType)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("undo override answer: %w", err))
	}

	return connect.NewResponse(&apiv1.UndoOverrideAnswerResponse{
		NextReviewDate: nextReviewDate,
	}), nil
}
