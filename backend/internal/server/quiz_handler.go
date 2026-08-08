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

	svc                  *quiz.Service
	noteRepository       notebook.NoteRepository
	mu                   sync.Mutex
	noteStore            map[int64]quiz.Card
	reverseStore         map[int64]quiz.ReverseCard
	freeformCards        []quiz.FreeformCard
	freeformStore        map[int64]quiz.FreeformCard
	relearnStore         map[int64]quiz.RelearnCard
	// grammarStore holds the in-flight grammar blanks for the current session,
	// keyed by the ephemeral note_id (same id scheme as noteStore) so Submit,
	// Override, and Skip all resolve a blank the same way vocab cards do.
	grammarStore map[int64]grammarBlankCtx
	nextID       int64
}

// NewQuizHandler creates a new QuizHandler.
func NewQuizHandler(svc *quiz.Service) *QuizHandler {
	return &QuizHandler{
		svc:                  svc,
		noteStore:            make(map[int64]quiz.Card),
		reverseStore:         make(map[int64]quiz.ReverseCard),
		freeformStore:        make(map[int64]quiz.FreeformCard),
		relearnStore:         make(map[int64]quiz.RelearnCard),
		grammarStore:         make(map[int64]grammarBlankCtx),
		nextID:               1,
	}
}

func (h *QuizHandler) SetNoteRepository(repo notebook.NoteRepository) {
	h.noteRepository = repo
}

func (h *QuizHandler) GetQuizOptions(ctx context.Context, req *connect.Request[apiv1.GetQuizOptionsRequest]) (*connect.Response[apiv1.GetQuizOptionsResponse], error) {
	summaries, err := h.svc.LoadNotebookSummaries(req.Msg.GetIncludeUnstudied())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load notebook summaries: %w", err))
	}
	sort.Slice(summaries, func(i, j int) bool {
		di, dj := summaries[i].LatestDate, summaries[j].LatestDate
		if !di.Equal(dj) {
			return di.After(dj)
		}
		return summaries[i].NotebookID < summaries[j].NotebookID
	})
	protoSummaries := make([]*apiv1.NotebookSummary, 0, len(summaries))
	for _, s := range summaries {
		var sections []*apiv1.NotebookSectionSummary
		for _, sec := range s.Sections {
			sections = append(sections, &apiv1.NotebookSectionSummary{
				Title:                       sec.Title,
				ReviewCount:                 int32(sec.ReviewCount),
				ReverseReviewCount:          int32(sec.ReverseReviewCount),
				EtymologyReviewCount:        int32(sec.EtymologyReviewCount),
				EtymologyReverseReviewCount: int32(sec.EtymologyReverseReviewCount),
				GrammarReviewCount:          int32(sec.GrammarReviewCount),
			})
		}
		protoSummaries = append(protoSummaries, &apiv1.NotebookSummary{
			NotebookId: s.NotebookID, Name: s.Name, ReviewCount: int32(s.ReviewCount),
			Kind: s.Kind, ReverseReviewCount: int32(s.ReverseReviewCount),
			EtymologyReviewCount:        int32(s.EtymologyReviewCount),
			EtymologyReverseReviewCount: int32(s.EtymologyReverseReviewCount),
			GrammarReviewCount:          int32(s.GrammarReviewCount),
			VocabularyCount:             int32(s.VocabularyCount),
			HasContent:                  s.HasContent,
			Sections:                    sections,
		})
	}
	return connect.NewResponse(&apiv1.GetQuizOptionsResponse{Notebooks: protoSummaries}), nil
}

func (h *QuizHandler) StartQuiz(ctx context.Context, req *connect.Request[apiv1.StartQuizRequest]) (*connect.Response[apiv1.StartQuizResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	notebookIDs, sectionTitles, err := resolveNotebookSections(req.Msg.GetNotebookIds(), req.Msg.GetNotebookSections())
	if err != nil {
		return nil, err
	}
	cards, err := h.svc.LoadCards(notebookIDs, req.Msg.GetIncludeUnstudied(), sectionTitles)
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
			examples = append(examples, &apiv1.Example{Text: ex.Text, Speaker: ex.Speaker})
		}
		flashcards = append(flashcards, &apiv1.Flashcard{
			NoteId: noteID, Entry: card.Entry, Examples: examples, OriginalEntry: card.OriginalEntry,
			ConceptHead: card.ConceptHead, ConceptMembers: card.ConceptMembers, ConceptMeaning: card.ConceptMeaning,
		})
	}
	h.mu.Lock()
	h.noteStore = localStore
	h.nextID = nextID
	h.mu.Unlock()
	return connect.NewResponse(&apiv1.StartQuizResponse{Flashcards: flashcards}), nil
}

func (h *QuizHandler) SubmitAnswer(ctx context.Context, req *connect.Request[apiv1.SubmitAnswerRequest]) (*connect.Response[apiv1.SubmitAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	noteID := req.Msg.GetNoteId()
	h.mu.Lock()
	card, ok := h.noteStore[noteID]
	h.mu.Unlock()
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d not found", noteID))
	}
	var grade quiz.GradeResult
	var err error
	if req.Msg.GetIsSkipped() {
		grade = skippedGradeResult()
	} else {
		grade, err = h.svc.GradeNotebookAnswer(ctx, card, req.Msg.GetAnswer(), req.Msg.GetResponseTimeMs())
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade answer: %w", err))
		}
	}
	if err := h.svc.SaveResult(ctx, card, grade, req.Msg.GetResponseTimeMs()); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update learning history: %w", err))
	}
	learnedAt, nextReviewDate := h.svc.GetLatestLearnedInfo(card.NotebookName, card.ID, card.Entry, notebook.QuizTypeNotebook)
	return connect.NewResponse(&apiv1.SubmitAnswerResponse{
		Correct: grade.Correct, Meaning: card.Meaning, Reason: grade.Reason,
		WordDetail: toProtoWordDetail(card.WordDetail), NextReviewDate: nextReviewDate,
		LearnedAt: learnedAt, Images: card.Images, SenseId: card.ID,
	}), nil
}

func toProtoWordDetail(wd quiz.WordDetail) *apiv1.WordDetail {
	if wd.Origin == "" && wd.Pronunciation == "" && wd.PartOfSpeech == "" && len(wd.Synonyms) == 0 && len(wd.Antonyms) == 0 && wd.Memo == "" && len(wd.OriginParts) == 0 {
		return nil
	}
	var parts []*apiv1.WordOriginPart
	for _, p := range wd.OriginParts {
		parts = append(parts, &apiv1.WordOriginPart{Origin: p.Origin, Type: p.Type, Language: p.Language, Meaning: p.Meaning})
	}
	return &apiv1.WordDetail{Origin: wd.Origin, Pronunciation: wd.Pronunciation, PartOfSpeech: wd.PartOfSpeech, Synonyms: wd.Synonyms, Antonyms: wd.Antonyms, Memo: wd.Memo, OriginParts: parts}
}

// resolveNotebookSections normalises a quiz request's notebook selection.
// When notebookSections is non-empty it takes precedence and produces both
// the flat list of notebook IDs and a per-notebook section filter map.
// Otherwise it falls back to the legacy notebookIDs list with no section
// filtering (an empty filter means "all sections").
//
// Empty results return an InvalidArgument error so callers can surface a
// consistent "select at least one notebook" message regardless of which
// field the client populated.
func resolveNotebookSections(notebookIDs []string, notebookSections []*apiv1.NotebookSection) ([]string, map[string][]string, error) {
	if len(notebookSections) > 0 {
		ids := make([]string, 0, len(notebookSections))
		filter := make(map[string][]string, len(notebookSections))
		for _, sec := range notebookSections {
			id := sec.GetNotebookId()
			if id == "" {
				continue
			}
			ids = append(ids, id)
			if titles := sec.GetSectionTitles(); len(titles) > 0 {
				filter[id] = titles
			}
		}
		if len(ids) == 0 {
			return nil, nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("notebook_sections must include at least one notebook_id"))
		}
		return ids, filter, nil
	}
	if len(notebookIDs) == 0 {
		return nil, nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("notebook_ids must include at least one item"))
	}
	return notebookIDs, nil, nil
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
			if detail, detailErr := connect.NewErrorDetail(&errdetails.BadRequest{FieldViolations: fieldViolations}); detailErr == nil {
				connectErr.AddDetail(detail)
			}
		}
		return connectErr
	}
	return nil
}

func (h *QuizHandler) StartReverseQuiz(ctx context.Context, req *connect.Request[apiv1.StartReverseQuizRequest]) (*connect.Response[apiv1.StartReverseQuizResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	notebookIDs, sectionTitles, err := resolveNotebookSections(req.Msg.GetNotebookIds(), req.Msg.GetNotebookSections())
	if err != nil {
		return nil, err
	}
	cards, err := h.svc.LoadReverseCards(notebookIDs, req.Msg.GetListMissingContext(), req.Msg.GetIncludeUnstudied(), sectionTitles)
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
		for _, c := range card.Contexts {
			contexts = append(contexts, &apiv1.ContextSentence{Context: c.Context, MaskedContext: c.MaskedContext})
		}
		flashcards = append(flashcards, &apiv1.ReverseFlashcard{
			NoteId: noteID, Meaning: card.Meaning, Contexts: contexts,
			NotebookName: card.NotebookName, StoryTitle: card.StoryTitle, SceneTitle: card.SceneTitle,
			ConceptHead: card.ConceptHead, ConceptMembers: card.ConceptMembers, ConceptMeaning: card.ConceptMeaning,
		})
	}
	h.mu.Lock()
	h.reverseStore = localStore
	h.nextID = nextID
	h.mu.Unlock()
	return connect.NewResponse(&apiv1.StartReverseQuizResponse{Flashcards: flashcards}), nil
}

func (h *QuizHandler) SubmitReverseAnswer(ctx context.Context, req *connect.Request[apiv1.SubmitReverseAnswerRequest]) (*connect.Response[apiv1.SubmitReverseAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	noteID := req.Msg.GetNoteId()
	h.mu.Lock()
	card, ok := h.reverseStore[noteID]
	h.mu.Unlock()
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d not found", noteID))
	}
	var grade quiz.GradeResult
	var err error
	if req.Msg.GetIsSkipped() {
		grade = skippedGradeResult()
	} else {
		grade, err = h.svc.GradeReverseAnswer(ctx, card, req.Msg.GetAnswer(), req.Msg.GetResponseTimeMs())
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade answer: %w", err))
		}
	}
	if grade.Classification != string(inference.ClassificationSynonym) {
		if err := h.svc.SaveReverseResult(ctx, card, grade, req.Msg.GetResponseTimeMs()); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update learning history: %w", err))
		}
	}
	var contexts []string
	for _, c := range card.Contexts {
		contexts = append(contexts, c.Context)
	}
	learnedAt, nextReviewDate := h.svc.GetLatestLearnedInfo(card.NotebookName, card.ID, card.Expression, notebook.QuizTypeReverse)
	return connect.NewResponse(&apiv1.SubmitReverseAnswerResponse{
		Correct: grade.Correct, Expression: card.Expression, Meaning: card.Meaning, Reason: grade.Reason,
		Contexts: contexts, WordDetail: toProtoWordDetail(card.WordDetail), Classification: grade.Classification,
		NextReviewDate: nextReviewDate, LearnedAt: learnedAt, Images: card.Images, SenseId: card.ID,
	}), nil
}

func (h *QuizHandler) StartFreeformQuiz(ctx context.Context, req *connect.Request[apiv1.StartFreeformQuizRequest]) (*connect.Response[apiv1.StartFreeformQuizResponse], error) {
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
	return connect.NewResponse(&apiv1.StartFreeformQuizResponse{WordCount: int32(len(cards)), Expressions: expressions, ExpressionNextReviewDate: nextReviewDates}), nil
}

func (h *QuizHandler) SubmitFreeformAnswer(ctx context.Context, req *connect.Request[apiv1.SubmitFreeformAnswerRequest]) (*connect.Response[apiv1.SubmitFreeformAnswerResponse], error) {
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
	var learnedAt, nextReviewDate, senseID string
	var noteID int64
	if grade.MatchedCard != nil {
		learnedAt, nextReviewDate = h.svc.GetLatestLearnedInfo(grade.MatchedCard.NotebookName, grade.MatchedCard.ID, grade.MatchedCard.Expression, notebook.QuizTypeFreeform)
		senseID = grade.MatchedCard.ID
		h.mu.Lock()
		noteID = h.nextID
		h.nextID++
		h.freeformStore[noteID] = *grade.MatchedCard
		h.mu.Unlock()
	}
	return connect.NewResponse(&apiv1.SubmitFreeformAnswerResponse{
		Correct: grade.Correct, Word: grade.Word, Meaning: grade.Meaning, Reason: grade.Reason,
		Context: grade.Context, NotebookName: grade.NotebookName,
		WordDetail: func() *apiv1.WordDetail {
			if grade.MatchedCard != nil {
				return toProtoWordDetail(grade.MatchedCard.WordDetail)
			}
			return nil
		}(),
		NextReviewDate: nextReviewDate, LearnedAt: learnedAt, NoteId: noteID, SenseId: senseID,
		Images: func() []string {
			if grade.MatchedCard != nil {
				return grade.MatchedCard.Images
			}
			return nil
		}(),
	}), nil
}

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
	if bc, ok := h.grammarStore[noteID]; ok {
		h.mu.Unlock()
		// Grammar history is a flat "journal"-titled bucket keyed by the
		// correction id, so Override/Skip target it the same way.
		info := quiz.CardInfo{
			NotebookName: bc.notebookID,
			StoryTitle:   notebook.JournalStoryTitle,
			Expression:   bc.blank.SenseID,
			ID:           bc.blank.SenseID,
		}
		return &info, nil
	}
	if rc, ok := h.relearnStore[noteID]; ok {
		h.mu.Unlock()
		// A grammar blank drilled in Relearn can be deliberately Excluded via
		// the same SkipWord RPC as everywhere else. The relearn store is keyed
		// by the stable relearnCardID hash, so resolve it here to the same
		// flat "journal" bucket keyed by the correction id the live grammar
		// quiz uses — the skip lands on the identical (notebook, senseID)
		// learning-history slot.
		if rc.IsGrammar() {
			info := quiz.CardInfo{
				NotebookName: rc.NotebookName,
				StoryTitle:   notebook.JournalStoryTitle,
				Expression:   rc.GrammarCard().SenseID,
				ID:           rc.GrammarCard().SenseID,
			}
			return &info, nil
		}
		info := quiz.CardInfo{NotebookName: rc.NotebookName, Expression: rc.Entry}
		return &info, nil
	}
	h.mu.Unlock()
	if h.noteRepository == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d not found: no active quiz session and no database configured", noteID))
	}
	noteRecord, err := h.noteRepository.FindByID(ctx, noteID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("note %d not found in database: %w", noteID, err))
	}
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
	info := quiz.CardInfo{NotebookName: notebookName, StoryTitle: group, SceneTitle: subgroup, Expression: expression}
	return &info, nil
}

func protoQuizTypeToNotebook(qt apiv1.QuizType) notebook.QuizType {
	switch qt {
	case apiv1.QuizType_QUIZ_TYPE_REVERSE:
		return notebook.QuizTypeReverse
	case apiv1.QuizType_QUIZ_TYPE_FREEFORM:
		return notebook.QuizTypeFreeform
	case apiv1.QuizType_QUIZ_TYPE_ETYMOLOGY_ORIGIN:
		return notebook.QuizTypeEtymologyOrigin
	case apiv1.QuizType_QUIZ_TYPE_GRAMMAR:
		return notebook.QuizTypeGrammar
	default:
		return notebook.QuizTypeNotebook
	}
}

func (h *QuizHandler) SkipWord(ctx context.Context, req *connect.Request[apiv1.SkipWordRequest]) (*connect.Response[apiv1.SkipWordResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	info, err := h.resolveCardInfo(ctx, req.Msg.GetNoteId())
	if err != nil {
		return nil, err
	}
	quizTypes := protoQuizTypesToNotebook(req.Msg.GetQuizTypes())
	if err := h.svc.SkipWord(*info, req.Msg.GetSkipUntil(), quizTypes); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("skip word: %w", err))
	}
	return connect.NewResponse(&apiv1.SkipWordResponse{}), nil
}

func (h *QuizHandler) ResumeWord(ctx context.Context, req *connect.Request[apiv1.ResumeWordRequest]) (*connect.Response[apiv1.ResumeWordResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	info, err := h.resolveCardInfo(ctx, req.Msg.GetNoteId())
	if err != nil {
		return nil, err
	}
	quizTypes := protoQuizTypesToNotebook(req.Msg.GetQuizTypes())
	if err := h.svc.ResumeWord(*info, quizTypes); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("resume word: %w", err))
	}
	return connect.NewResponse(&apiv1.ResumeWordResponse{}), nil
}

// protoQuizTypesToNotebook converts a repeated proto QuizType list to the
// internal notebook.QuizType slice. Used by SkipWord/ResumeWord which
// accept multiple types per request.
func protoQuizTypesToNotebook(qts []apiv1.QuizType) []notebook.QuizType {
	out := make([]notebook.QuizType, 0, len(qts))
	for _, qt := range qts {
		out = append(out, protoQuizTypeToNotebook(qt))
	}
	return out
}

// etymologyWordCardInfo builds the CardInfo that addresses one derived word's
// learning-history slot by its (notebook_id, expression). notebookID is the
// definitions-book id that owns the word — the SAME id the origin-family builder
// reads exclusion under (learning-history L2) — so excluding here drops the word
// from its origin family in the Relearn quiz. This mirrors
// grammarMistakeCardInfo: keyed by a stable string identity, no DB note_id.
func etymologyWordCardInfo(notebookID, expression string) quiz.CardInfo {
	return quiz.CardInfo{
		NotebookName: notebookID,
		Expression:   expression,
	}
}

// ExcludeEtymologyWord sets the etymology-origin skipped_at marker for one
// derived family word, addressed by its stable (notebook_id, expression). It
// reuses the same SkipWord / SetSkippedAt path (and EnsureExpressionStubForSkip
// when the word has no log yet) every other card's Exclude uses — no DB note_id
// required, so it works for YAML-only notebooks.
func (h *QuizHandler) ExcludeEtymologyWord(
	_ context.Context,
	req *connect.Request[apiv1.ExcludeEtymologyWordRequest],
) (*connect.Response[apiv1.ExcludeEtymologyWordResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	info := etymologyWordCardInfo(req.Msg.GetNotebookId(), req.Msg.GetExpression())
	if err := h.svc.SkipWord(info, "", []notebook.QuizType{notebook.QuizTypeEtymologyOrigin}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("exclude etymology word: %w", err))
	}
	return connect.NewResponse(&apiv1.ExcludeEtymologyWordResponse{}), nil
}

// ResumeEtymologyWord clears the etymology-origin skipped_at marker for one
// derived family word, making it due again in its origin family.
func (h *QuizHandler) ResumeEtymologyWord(
	_ context.Context,
	req *connect.Request[apiv1.ResumeEtymologyWordRequest],
) (*connect.Response[apiv1.ResumeEtymologyWordResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	info := etymologyWordCardInfo(req.Msg.GetNotebookId(), req.Msg.GetExpression())
	if err := h.svc.ResumeWord(info, []notebook.QuizType{notebook.QuizTypeEtymologyOrigin}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("resume etymology word: %w", err))
	}
	return connect.NewResponse(&apiv1.ResumeEtymologyWordResponse{}), nil
}

func (h *QuizHandler) OverrideAnswer(ctx context.Context, req *connect.Request[apiv1.OverrideAnswerRequest]) (*connect.Response[apiv1.OverrideAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	info, err := h.resolveCardInfo(ctx, req.Msg.GetNoteId())
	if err != nil {
		return nil, err
	}
	info.NoteID = req.Msg.GetNoteId()
	info.ID = req.Msg.GetSenseId()
	info.LearnedAt = req.Msg.GetLearnedAt()
	if req.Msg.MarkCorrect != nil {
		mc := req.Msg.GetMarkCorrect()
		info.MarkCorrect = &mc
	}
	quizType := protoQuizTypeToNotebook(req.Msg.GetQuizType())

	res, err := h.svc.OverrideAnswer(*info, quizType)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("override answer: %w", err))
	}
	return connect.NewResponse(&apiv1.OverrideAnswerResponse{
		NextReviewDate:       res.NextReviewDate,
		OriginalQuality:      int32(res.OriginalQuality),
		OriginalStatus:       res.OriginalStatus,
		OriginalIntervalDays: int32(res.OriginalIntervalDays),
	}), nil
}

func (h *QuizHandler) UndoOverrideAnswer(ctx context.Context, req *connect.Request[apiv1.UndoOverrideAnswerRequest]) (*connect.Response[apiv1.UndoOverrideAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	info, err := h.resolveCardInfo(ctx, req.Msg.GetNoteId())
	if err != nil {
		return nil, err
	}
	info.NoteID = req.Msg.GetNoteId()
	info.ID = req.Msg.GetSenseId()
	info.LearnedAt = req.Msg.GetLearnedAt()
	info.OriginalQuality = int(req.Msg.GetOriginalQuality())
	info.OriginalStatus = req.Msg.GetOriginalStatus()
	info.OriginalIntervalDays = int(req.Msg.GetOriginalIntervalDays())
	quizType := protoQuizTypeToNotebook(req.Msg.GetQuizType())
	correct, nextReviewDate, err := h.svc.UndoOverrideAnswer(*info, quizType)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("undo override answer: %w", err))
	}
	return connect.NewResponse(&apiv1.UndoOverrideAnswerResponse{
		Correct:        correct,
		NextReviewDate: nextReviewDate,
	}), nil
}
