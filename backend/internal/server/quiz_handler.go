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
	etymologyOriginStore map[int64]quiz.EtymologyOriginCard
	etymologyOriginCards []quiz.EtymologyOriginCard
	etymologyQuizMode    apiv1.EtymologyQuizMode
	nextID         int64
}

// NewQuizHandler creates a new QuizHandler.
func NewQuizHandler(svc *quiz.Service) *QuizHandler {
	return &QuizHandler{
		svc:                  svc,
		noteStore:            make(map[int64]quiz.Card),
		reverseStore:         make(map[int64]quiz.ReverseCard),
		freeformStore:        make(map[int64]quiz.FreeformCard),
		etymologyOriginStore: make(map[int64]quiz.EtymologyOriginCard),
		nextID:               1,
	}
}

func (h *QuizHandler) SetNoteRepository(repo notebook.NoteRepository) {
	h.noteRepository = repo
}

func (h *QuizHandler) GetQuizOptions(ctx context.Context, req *connect.Request[apiv1.GetQuizOptionsRequest]) (*connect.Response[apiv1.GetQuizOptionsResponse], error) {
	summaries, err := h.svc.LoadNotebookSummaries()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load notebook summaries: %w", err))
	}
	sort.Slice(summaries, func(i, j int) bool {
		di, dj := summaries[i].LatestStoryDate, summaries[j].LatestStoryDate
		if !di.Equal(dj) { return di.After(dj) }
		return summaries[i].NotebookID < summaries[j].NotebookID
	})
	protoSummaries := make([]*apiv1.NotebookSummary, 0, len(summaries))
	for _, s := range summaries {
		protoSummaries = append(protoSummaries, &apiv1.NotebookSummary{
			NotebookId: s.NotebookID, Name: s.Name, ReviewCount: int32(s.ReviewCount),
			Kind: s.Kind, ReverseReviewCount: int32(s.ReverseReviewCount),
			EtymologyReviewCount: int32(s.EtymologyReviewCount),
			HasContent:           s.HasContent,
		})
	}
	return connect.NewResponse(&apiv1.GetQuizOptionsResponse{Notebooks: protoSummaries}), nil
}

func (h *QuizHandler) StartQuiz(ctx context.Context, req *connect.Request[apiv1.StartQuizRequest]) (*connect.Response[apiv1.StartQuizResponse], error) {
	if err := validateRequest(req.Msg); err != nil { return nil, err }
	cards, err := h.svc.LoadCards(req.Msg.GetNotebookIds(), req.Msg.GetIncludeUnstudied())
	if err != nil {
		var notFoundErr *quiz.NotFoundError
		if errors.As(err, &notFoundErr) { return nil, connect.NewError(connect.CodeNotFound, err) }
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load cards: %w", err))
	}
	localStore := make(map[int64]quiz.Card)
	var nextID int64 = 1
	var flashcards []*apiv1.Flashcard
	for _, card := range cards {
		noteID := nextID; nextID++; localStore[noteID] = card
		var examples []*apiv1.Example
		for _, ex := range card.Examples { examples = append(examples, &apiv1.Example{Text: ex.Text, Speaker: ex.Speaker}) }
		flashcards = append(flashcards, &apiv1.Flashcard{NoteId: noteID, Entry: card.Entry, Examples: examples})
	}
	h.mu.Lock(); h.noteStore = localStore; h.nextID = nextID; h.mu.Unlock()
	return connect.NewResponse(&apiv1.StartQuizResponse{Flashcards: flashcards}), nil
}

func (h *QuizHandler) SubmitAnswer(ctx context.Context, req *connect.Request[apiv1.SubmitAnswerRequest]) (*connect.Response[apiv1.SubmitAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil { return nil, err }
	noteID := req.Msg.GetNoteId()
	h.mu.Lock(); card, ok := h.noteStore[noteID]; h.mu.Unlock()
	if !ok { return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d not found", noteID)) }
	grade, err := h.svc.GradeNotebookAnswer(ctx, card, req.Msg.GetAnswer(), req.Msg.GetResponseTimeMs())
	if err != nil { return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade answer: %w", err)) }
	if err := h.svc.SaveResult(ctx, card, grade, req.Msg.GetResponseTimeMs()); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update learning history: %w", err))
	}
	learnedAt, nextReviewDate := h.svc.GetLatestLearnedInfo(card.NotebookName, card.Entry, notebook.QuizTypeNotebook)
	return connect.NewResponse(&apiv1.SubmitAnswerResponse{
		Correct: grade.Correct, Meaning: card.Meaning, Reason: grade.Reason,
		WordDetail: toProtoWordDetail(card.WordDetail), NextReviewDate: nextReviewDate,
		LearnedAt: learnedAt, Images: card.Images,
	}), nil
}

func toProtoWordDetail(wd quiz.WordDetail) *apiv1.WordDetail {
	if wd.Origin == "" && wd.Pronunciation == "" && wd.PartOfSpeech == "" && len(wd.Synonyms) == 0 && len(wd.Antonyms) == 0 && wd.Memo == "" && len(wd.OriginParts) == 0 { return nil }
	var parts []*apiv1.WordOriginPart
	for _, p := range wd.OriginParts {
		parts = append(parts, &apiv1.WordOriginPart{Origin: p.Origin, Type: p.Type, Language: p.Language, Meaning: p.Meaning})
	}
	return &apiv1.WordDetail{Origin: wd.Origin, Pronunciation: wd.Pronunciation, PartOfSpeech: wd.PartOfSpeech, Synonyms: wd.Synonyms, Antonyms: wd.Antonyms, Memo: wd.Memo, OriginParts: parts}
}

func validateRequest(msg proto.Message) *connect.Error {
	if err := protovalidate.Validate(msg); err != nil {
		connectErr := connect.NewError(connect.CodeInvalidArgument, err)
		var valErr *protovalidate.ValidationError
		if errors.As(err, &valErr) {
			var fieldViolations []*errdetails.BadRequest_FieldViolation
			for _, v := range valErr.Violations {
				fieldViolations = append(fieldViolations, &errdetails.BadRequest_FieldViolation{Field: protovalidate.FieldPathString(v.Proto.GetField()), Description: v.Proto.GetMessage()})
			}
			if detail, detailErr := connect.NewErrorDetail(&errdetails.BadRequest{FieldViolations: fieldViolations}); detailErr == nil { connectErr.AddDetail(detail) }
		}
		return connectErr
	}
	return nil
}

func (h *QuizHandler) StartReverseQuiz(ctx context.Context, req *connect.Request[apiv1.StartReverseQuizRequest]) (*connect.Response[apiv1.StartReverseQuizResponse], error) {
	if err := validateRequest(req.Msg); err != nil { return nil, err }
	cards, err := h.svc.LoadReverseCards(req.Msg.GetNotebookIds(), req.Msg.GetListMissingContext())
	if err != nil {
		var notFoundErr *quiz.NotFoundError
		if errors.As(err, &notFoundErr) { return nil, connect.NewError(connect.CodeNotFound, err) }
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load reverse cards: %w", err))
	}
	localStore := make(map[int64]quiz.ReverseCard); var nextID int64 = 1
	var flashcards []*apiv1.ReverseFlashcard
	for _, card := range cards {
		noteID := nextID; nextID++; localStore[noteID] = card
		var contexts []*apiv1.ContextSentence
		for _, c := range card.Contexts { contexts = append(contexts, &apiv1.ContextSentence{Context: c.Context, MaskedContext: c.MaskedContext}) }
		flashcards = append(flashcards, &apiv1.ReverseFlashcard{NoteId: noteID, Meaning: card.Meaning, Contexts: contexts, NotebookName: card.NotebookName, StoryTitle: card.StoryTitle, SceneTitle: card.SceneTitle})
	}
	h.mu.Lock(); h.reverseStore = localStore; h.nextID = nextID; h.mu.Unlock()
	return connect.NewResponse(&apiv1.StartReverseQuizResponse{Flashcards: flashcards}), nil
}

func (h *QuizHandler) SubmitReverseAnswer(ctx context.Context, req *connect.Request[apiv1.SubmitReverseAnswerRequest]) (*connect.Response[apiv1.SubmitReverseAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil { return nil, err }
	noteID := req.Msg.GetNoteId()
	h.mu.Lock(); card, ok := h.reverseStore[noteID]; h.mu.Unlock()
	if !ok { return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d not found", noteID)) }
	grade, err := h.svc.GradeReverseAnswer(ctx, card, req.Msg.GetAnswer(), req.Msg.GetResponseTimeMs())
	if err != nil { return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade answer: %w", err)) }
	if grade.Classification != string(inference.ClassificationSynonym) {
		if err := h.svc.SaveReverseResult(ctx, card, grade, req.Msg.GetResponseTimeMs()); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update learning history: %w", err))
		}
	}
	var contexts []string
	for _, c := range card.Contexts { contexts = append(contexts, c.Context) }
	learnedAt, nextReviewDate := h.svc.GetLatestLearnedInfo(card.NotebookName, card.Expression, notebook.QuizTypeReverse)
	return connect.NewResponse(&apiv1.SubmitReverseAnswerResponse{
		Correct: grade.Correct, Expression: card.Expression, Meaning: card.Meaning, Reason: grade.Reason,
		Contexts: contexts, WordDetail: toProtoWordDetail(card.WordDetail), Classification: grade.Classification,
		NextReviewDate: nextReviewDate, LearnedAt: learnedAt, Images: card.Images,
	}), nil
}

func (h *QuizHandler) StartFreeformQuiz(ctx context.Context, req *connect.Request[apiv1.StartFreeformQuizRequest]) (*connect.Response[apiv1.StartFreeformQuizResponse], error) {
	cards, err := h.svc.LoadAllWords()
	if err != nil { return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load all words: %w", err)) }
	nextReviewDates, err := h.svc.GetFreeformNextReviewDates(cards)
	if err != nil { return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get next review dates: %w", err)) }
	h.mu.Lock(); h.freeformCards = cards; h.mu.Unlock()
	seen := make(map[string]struct{}, len(cards)*2)
	expressions := make([]string, 0, len(cards))
	addExpr := func(expr string) {
		lower := strings.ToLower(expr)
		if expr != "" { if _, ok := seen[lower]; !ok { seen[lower] = struct{}{}; expressions = append(expressions, expr) } }
	}
	for _, card := range cards { addExpr(card.Expression); addExpr(card.OriginalExpression) }
	return connect.NewResponse(&apiv1.StartFreeformQuizResponse{WordCount: int32(len(cards)), Expressions: expressions, ExpressionNextReviewDate: nextReviewDates}), nil
}

func (h *QuizHandler) SubmitFreeformAnswer(ctx context.Context, req *connect.Request[apiv1.SubmitFreeformAnswerRequest]) (*connect.Response[apiv1.SubmitFreeformAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil { return nil, err }
	h.mu.Lock(); cards := h.freeformCards; h.mu.Unlock()
	grade, err := h.svc.GradeFreeformAnswer(ctx, req.Msg.GetWord(), req.Msg.GetMeaning(), req.Msg.GetResponseTimeMs(), cards)
	if err != nil { return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade answer: %w", err)) }
	if grade.MatchedCard != nil {
		if err := h.svc.SaveFreeformResult(ctx, *grade.MatchedCard, grade, req.Msg.GetResponseTimeMs()); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update learning history: %w", err))
		}
	}
	var learnedAt, nextReviewDate string; var noteID int64
	if grade.MatchedCard != nil {
		learnedAt, nextReviewDate = h.svc.GetLatestLearnedInfo(grade.MatchedCard.NotebookName, grade.MatchedCard.Expression, notebook.QuizTypeFreeform)
		h.mu.Lock(); noteID = h.nextID; h.nextID++; h.freeformStore[noteID] = *grade.MatchedCard; h.mu.Unlock()
	}
	return connect.NewResponse(&apiv1.SubmitFreeformAnswerResponse{
		Correct: grade.Correct, Word: grade.Word, Meaning: grade.Meaning, Reason: grade.Reason,
		Context: grade.Context, NotebookName: grade.NotebookName,
		WordDetail: func() *apiv1.WordDetail { if grade.MatchedCard != nil { return toProtoWordDetail(grade.MatchedCard.WordDetail) }; return nil }(),
		NextReviewDate: nextReviewDate, LearnedAt: learnedAt, NoteId: noteID,
		Images: func() []string { if grade.MatchedCard != nil { return grade.MatchedCard.Images }; return nil }(),
	}), nil
}

func (h *QuizHandler) resolveCardInfo(ctx context.Context, noteID int64) (*quiz.CardInfo, error) {
	h.mu.Lock()
	if card, ok := h.noteStore[noteID]; ok { h.mu.Unlock(); info := quiz.CardInfoFromCard(card); return &info, nil }
	if card, ok := h.reverseStore[noteID]; ok { h.mu.Unlock(); info := quiz.CardInfoFromReverseCard(card); return &info, nil }
	if fcard, ok := h.freeformStore[noteID]; ok { h.mu.Unlock(); info := quiz.CardInfoFromFreeformCard(fcard); return &info, nil }
	if ecard, found := h.etymologyOriginStore[noteID]; found {
		h.mu.Unlock()
		info := quiz.CardInfo{NotebookName: ecard.NotebookName, StoryTitle: ecard.NotebookTitle, Expression: ecard.Origin}
		return &info, nil
	}
	h.mu.Unlock()
	if h.noteRepository == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d not found: no active quiz session and no database configured", noteID))
	}
	noteRecord, err := h.noteRepository.FindByID(ctx, noteID)
	if err != nil { return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d not found in database: %w", noteID, err)) }
	var notebookName, group, subgroup string
	if len(noteRecord.NotebookNotes) > 0 { nn := noteRecord.NotebookNotes[0]; notebookName = nn.NotebookID; group = nn.Group; subgroup = nn.Subgroup }
	if notebookName == "" { return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d has no linked notebook", noteID)) }
	expression := noteRecord.Entry
	if expression == "" { expression = noteRecord.Usage }
	info := quiz.CardInfo{NotebookName: notebookName, StoryTitle: group, SceneTitle: subgroup, Expression: expression}
	return &info, nil
}

func protoQuizTypeToNotebook(qt apiv1.QuizType) notebook.QuizType {
	switch qt {
	case apiv1.QuizType_QUIZ_TYPE_REVERSE: return notebook.QuizTypeReverse
	case apiv1.QuizType_QUIZ_TYPE_FREEFORM: return notebook.QuizTypeFreeform
	case apiv1.QuizType_QUIZ_TYPE_ETYMOLOGY_STANDARD: return notebook.QuizTypeEtymologyStandard
	case apiv1.QuizType_QUIZ_TYPE_ETYMOLOGY_REVERSE: return notebook.QuizTypeEtymologyReverse
	case apiv1.QuizType_QUIZ_TYPE_ETYMOLOGY_FREEFORM: return notebook.QuizTypeEtymologyFreeform
	default: return notebook.QuizTypeNotebook
	}
}

func (h *QuizHandler) SkipWord(ctx context.Context, req *connect.Request[apiv1.SkipWordRequest]) (*connect.Response[apiv1.SkipWordResponse], error) {
	if err := validateRequest(req.Msg); err != nil { return nil, err }
	info, err := h.resolveCardInfo(ctx, req.Msg.GetNoteId())
	if err != nil { return nil, err }
	quizType := protoQuizTypeToNotebook(req.Msg.GetQuizType())
	if err := h.svc.SkipWord(*info, req.Msg.GetSkipUntil(), quizType); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("skip word: %w", err))
	}
	return connect.NewResponse(&apiv1.SkipWordResponse{}), nil
}

func (h *QuizHandler) ResumeWord(ctx context.Context, req *connect.Request[apiv1.ResumeWordRequest]) (*connect.Response[apiv1.ResumeWordResponse], error) {
	if err := validateRequest(req.Msg); err != nil { return nil, err }
	info, err := h.resolveCardInfo(ctx, req.Msg.GetNoteId())
	if err != nil { return nil, err }
	quizType := protoQuizTypeToNotebook(req.Msg.GetQuizType())
	if err := h.svc.ResumeWord(*info, quizType); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("resume word: %w", err))
	}
	return connect.NewResponse(&apiv1.ResumeWordResponse{}), nil
}

func (h *QuizHandler) StartEtymologyQuiz(ctx context.Context, req *connect.Request[apiv1.StartEtymologyQuizRequest]) (*connect.Response[apiv1.StartEtymologyQuizResponse], error) {
	if err := validateRequest(req.Msg); err != nil { return nil, err }
	cards, err := h.svc.LoadEtymologyOriginCards(req.Msg.GetEtymologyNotebookIds(), req.Msg.GetIncludeUnstudied(), false)
	if err != nil { return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load etymology origin cards: %w", err)) }
	localStore := make(map[int64]quiz.EtymologyOriginCard); var nextID int64 = 1
	var protoCards []*apiv1.EtymologyQuizCard
	for _, card := range cards {
		cardID := nextID; nextID++; localStore[cardID] = card
		protoCards = append(protoCards, &apiv1.EtymologyQuizCard{CardId: cardID, Origin: card.Origin, Type: card.Type, Language: card.Language, Meaning: card.Meaning, NotebookName: card.NotebookName})
	}
	h.mu.Lock(); h.etymologyOriginStore = localStore; h.etymologyQuizMode = req.Msg.GetMode(); h.nextID = nextID; h.mu.Unlock()
	return connect.NewResponse(&apiv1.StartEtymologyQuizResponse{Cards: protoCards}), nil
}

func (h *QuizHandler) SubmitEtymologyStandardAnswer(ctx context.Context, req *connect.Request[apiv1.SubmitEtymologyStandardAnswerRequest]) (*connect.Response[apiv1.SubmitEtymologyStandardAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil { return nil, err }
	cardID := req.Msg.GetCardId()
	h.mu.Lock(); card, ok := h.etymologyOriginStore[cardID]; h.mu.Unlock()
	if !ok { return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("card %d not found", cardID)) }
	grade, err := h.svc.GradeEtymologyStandardAnswer(ctx, card, req.Msg.GetAnswer(), req.Msg.GetResponseTimeMs())
	if err != nil { return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade etymology standard: %w", err)) }
	if err := h.svc.SaveEtymologyOriginResult(card, grade.Quality, grade.Correct, req.Msg.GetResponseTimeMs(), notebook.QuizTypeEtymologyStandard, true); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("save etymology result: %w", err))
	}
	learnedAt, nextReviewDate := h.svc.GetLatestLearnedInfo(card.NotebookName, card.Origin, notebook.QuizTypeEtymologyStandard)
	h.mu.Lock(); noteID := h.nextID; h.nextID++; h.etymologyOriginStore[noteID] = card; h.mu.Unlock()
	return connect.NewResponse(&apiv1.SubmitEtymologyStandardAnswerResponse{Correct: grade.Correct, Reason: grade.Reason, CorrectMeaning: card.Meaning, NextReviewDate: nextReviewDate, LearnedAt: learnedAt, NoteId: noteID}), nil
}

func (h *QuizHandler) SubmitEtymologyReverseAnswer(ctx context.Context, req *connect.Request[apiv1.SubmitEtymologyReverseAnswerRequest]) (*connect.Response[apiv1.SubmitEtymologyReverseAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil { return nil, err }
	cardID := req.Msg.GetCardId()
	h.mu.Lock(); card, ok := h.etymologyOriginStore[cardID]; h.mu.Unlock()
	if !ok { return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("card %d not found", cardID)) }
	grade, err := h.svc.GradeEtymologyReverseAnswer(ctx, card, req.Msg.GetAnswer(), req.Msg.GetResponseTimeMs())
	if err != nil { return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade etymology reverse: %w", err)) }
	if err := h.svc.SaveEtymologyOriginResult(card, grade.Quality, grade.Correct, req.Msg.GetResponseTimeMs(), notebook.QuizTypeEtymologyReverse, true); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("save etymology result: %w", err))
	}
	learnedAt, nextReviewDate := h.svc.GetLatestLearnedInfo(card.NotebookName, card.Origin, notebook.QuizTypeEtymologyReverse)
	h.mu.Lock(); noteID := h.nextID; h.nextID++; h.etymologyOriginStore[noteID] = card; h.mu.Unlock()
	return connect.NewResponse(&apiv1.SubmitEtymologyReverseAnswerResponse{Correct: grade.Correct, Reason: grade.Reason, CorrectOrigin: card.Origin, Type: card.Type, Language: card.Language, NextReviewDate: nextReviewDate, LearnedAt: learnedAt, NoteId: noteID}), nil
}

func (h *QuizHandler) StartEtymologyFreeformQuiz(ctx context.Context, req *connect.Request[apiv1.StartEtymologyFreeformQuizRequest]) (*connect.Response[apiv1.StartEtymologyFreeformQuizResponse], error) {
	if err := validateRequest(req.Msg); err != nil { return nil, err }
	cards, err := h.svc.LoadEtymologyOriginCards(req.Msg.GetEtymologyNotebookIds(), true, true)
	if err != nil { return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load etymology origin cards: %w", err)) }
	nextReviewDates, err := h.svc.GetEtymologyOriginNextReviewDates(cards)
	if err != nil { return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get etymology next review dates: %w", err)) }
	h.mu.Lock(); h.etymologyOriginCards = cards; h.mu.Unlock()
	seen := make(map[string]struct{}); var origins []string
	for _, card := range cards {
		lower := strings.ToLower(card.Origin)
		if _, ok := seen[lower]; !ok { seen[lower] = struct{}{}; origins = append(origins, card.Origin) }
	}
	return connect.NewResponse(&apiv1.StartEtymologyFreeformQuizResponse{Origins: origins, NextReviewDates: nextReviewDates}), nil
}

func (h *QuizHandler) SubmitEtymologyFreeformAnswer(ctx context.Context, req *connect.Request[apiv1.SubmitEtymologyFreeformAnswerRequest]) (*connect.Response[apiv1.SubmitEtymologyFreeformAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil { return nil, err }
	h.mu.Lock(); cards := h.etymologyOriginCards; h.mu.Unlock()
	origin := req.Msg.GetOrigin(); meaning := req.Msg.GetMeaning()
	var matchedCard *quiz.EtymologyOriginCard
	for _, card := range cards { if strings.EqualFold(card.Origin, origin) { c := card; matchedCard = &c; break } }
	if matchedCard == nil { return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("origin %q not found", origin)) }
	grade, err := h.svc.GradeEtymologyStandardAnswer(ctx, *matchedCard, meaning, req.Msg.GetResponseTimeMs())
	if err != nil { return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade etymology freeform: %w", err)) }
	if err := h.svc.SaveEtymologyOriginResult(*matchedCard, grade.Quality, grade.Correct, req.Msg.GetResponseTimeMs(), notebook.QuizTypeEtymologyFreeform, false); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("save etymology freeform result: %w", err))
	}
	learnedAt, nextReviewDate := h.svc.GetLatestLearnedInfo(matchedCard.NotebookName, matchedCard.Origin, notebook.QuizTypeEtymologyFreeform)
	h.mu.Lock(); noteID := h.nextID; h.nextID++; h.etymologyOriginStore[noteID] = *matchedCard; h.mu.Unlock()
	return connect.NewResponse(&apiv1.SubmitEtymologyFreeformAnswerResponse{
		Correct: grade.Correct, Reason: grade.Reason, CorrectMeaning: matchedCard.Meaning,
		Type: matchedCard.Type, Language: matchedCard.Language, NotebookName: matchedCard.NotebookName,
		NextReviewDate: nextReviewDate, LearnedAt: learnedAt, NoteId: noteID,
	}), nil
}

func (h *QuizHandler) OverrideAnswer(ctx context.Context, req *connect.Request[apiv1.OverrideAnswerRequest]) (*connect.Response[apiv1.OverrideAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil { return nil, err }
	info, err := h.resolveCardInfo(ctx, req.Msg.GetNoteId())
	if err != nil { return nil, err }
	quizType := protoQuizTypeToNotebook(req.Msg.GetQuizType())
	nextReviewDate, err := h.svc.OverrideAnswer(*info, quizType)
	if err != nil { return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("override answer: %w", err)) }
	return connect.NewResponse(&apiv1.OverrideAnswerResponse{NextReviewDate: nextReviewDate}), nil
}

func (h *QuizHandler) UndoOverrideAnswer(ctx context.Context, req *connect.Request[apiv1.UndoOverrideAnswerRequest]) (*connect.Response[apiv1.UndoOverrideAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil { return nil, err }
	info, err := h.resolveCardInfo(ctx, req.Msg.GetNoteId())
	if err != nil { return nil, err }
	quizType := protoQuizTypeToNotebook(req.Msg.GetQuizType())
	nextReviewDate, err := h.svc.UndoOverrideAnswer(*info, quizType)
	if err != nil { return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("undo override answer: %w", err)) }
	return connect.NewResponse(&apiv1.UndoOverrideAnswerResponse{NextReviewDate: nextReviewDate}), nil
}
