package server

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
)

// relearnDefaultWindowHours is used when the request leaves window_hours unset
// (0). relearnMaxWindowHours caps the look-back at 7 days.
const (
	relearnDefaultWindowHours = 24
	relearnMaxWindowHours     = 168
)

// clampRelearnWindow normalises the requested look-back window: 0 (unset) maps
// to the default, and the value is clamped to [1, relearnMaxWindowHours].
func clampRelearnWindow(hours int32) time.Duration {
	if hours <= 0 {
		hours = relearnDefaultWindowHours
	}
	if hours > relearnMaxWindowHours {
		hours = relearnMaxWindowHours
	}
	return time.Duration(hours) * time.Hour
}

// StartRelearnQuiz builds the wrong-word pool for the requested window and
// stores the resolved cards for the session. It reads learning history but
// writes nothing.
func (h *QuizHandler) StartRelearnQuiz(ctx context.Context, req *connect.Request[apiv1.StartRelearnQuizRequest]) (*connect.Response[apiv1.StartRelearnQuizResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	window := clampRelearnWindow(req.Msg.GetWindowHours())

	cards, err := h.svc.LoadRelearnPool(time.Now().Add(-window))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load relearn pool: %w", err))
	}

	localStore := make(map[int64]quiz.RelearnCard, len(cards))
	var nextID int64 = 1
	protoCards := make([]*apiv1.RelearnCard, 0, len(cards))
	for _, card := range cards {
		noteID := nextID
		nextID++
		localStore[noteID] = card
		var examples []*apiv1.Example
		for _, ex := range card.Examples {
			examples = append(examples, &apiv1.Example{Text: ex.Text, Speaker: ex.Speaker})
		}
		var contexts []*apiv1.ContextSentence
		for _, c := range card.Contexts {
			contexts = append(contexts, &apiv1.ContextSentence{Context: c.Context, MaskedContext: c.MaskedContext})
		}
		protoCards = append(protoCards, &apiv1.RelearnCard{
			NoteId:         noteID,
			Entry:          card.Entry,
			SourceQuizType: notebookQuizTypeToProto(card.Format),
			Meaning:        card.Meaning,
			Examples:       examples,
			Contexts:       contexts,
			Type:           card.OriginType,
			Language:       card.Language,
		})
	}

	h.mu.Lock()
	h.relearnStore = localStore
	h.nextID = nextID
	h.mu.Unlock()

	return connect.NewResponse(&apiv1.StartRelearnQuizResponse{Cards: protoCards}), nil
}

// SubmitRelearnAnswer grades one relearn answer. It records NOTHING: it calls
// only the pure meaning graders and writes no state at all — not learning
// history and not any relearn-local marker. Relearn is repeatable, so a word
// stays in the pool until it ages out of the window or is fixed in a real quiz.
// It never touches Save*, UpdateLog, or GetLatestLearnedInfo, and the response
// carries no next_review_date / learned_at.
func (h *QuizHandler) SubmitRelearnAnswer(ctx context.Context, req *connect.Request[apiv1.SubmitRelearnAnswerRequest]) (*connect.Response[apiv1.SubmitRelearnAnswerResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	noteID := req.Msg.GetNoteId()
	h.mu.Lock()
	card, ok := h.relearnStore[noteID]
	h.mu.Unlock()
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("relearn card %d not found", noteID))
	}

	grade, err := h.gradeRelearn(ctx, card, req.Msg.GetAnswer(), req.Msg.GetResponseTimeMs(), req.Msg.GetIsSkipped())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade relearn answer: %w", err))
	}

	return connect.NewResponse(h.buildRelearnResponse(ctx, card, grade)), nil
}

// BatchSubmitRelearnAnswers grades a batch of relearn answers. Grading runs in
// parallel. Like the single-shot path it writes no state.
func (h *QuizHandler) BatchSubmitRelearnAnswers(ctx context.Context, req *connect.Request[apiv1.BatchSubmitRelearnAnswersRequest]) (*connect.Response[apiv1.BatchSubmitRelearnAnswersResponse], error) {
	if err := validateRequest(req.Msg); err != nil {
		return nil, err
	}
	answers := req.Msg.GetAnswers()

	cards := make([]quiz.RelearnCard, len(answers))
	h.mu.Lock()
	for i, a := range answers {
		card, ok := h.relearnStore[a.GetNoteId()]
		if !ok {
			h.mu.Unlock()
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("relearn card %d not found", a.GetNoteId()))
		}
		cards[i] = card
	}
	h.mu.Unlock()

	grades, err := parallelGrade(ctx, answers, func(i int) (quiz.GradeResult, error) {
		return h.gradeRelearn(ctx, cards[i], answers[i].GetAnswer(), answers[i].GetResponseTimeMs(), answers[i].GetIsSkipped())
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grade relearn answers: %w", err))
	}

	responses := make([]*apiv1.SubmitRelearnAnswerResponse, len(answers))
	for i := range answers {
		responses[i] = h.buildRelearnResponse(ctx, cards[i], grades[i])
	}
	return connect.NewResponse(&apiv1.BatchSubmitRelearnAnswersResponse{Responses: responses}), nil
}

// gradeRelearn dispatches to the pure grader that matches the card's Format, so
// each word is graded in the direction it was failed in. A skip is graded as
// wrong without calling the grader (same as the other quizzes).
func (h *QuizHandler) gradeRelearn(ctx context.Context, card quiz.RelearnCard, answer string, responseTimeMs int64, skipped bool) (quiz.GradeResult, error) {
	if skipped {
		return skippedGradeResult(), nil
	}
	switch card.Format {
	case notebook.QuizTypeReverse:
		return h.svc.GradeReverseAnswer(ctx, card.ReverseCard(), answer, responseTimeMs)
	case notebook.QuizTypeEtymologyStandard:
		return h.svc.GradeEtymologyStandardAnswer(ctx, card.EtymologyCard(), answer, responseTimeMs)
	case notebook.QuizTypeEtymologyReverse:
		return h.svc.GradeEtymologyReverseAnswer(ctx, card.EtymologyCard(), answer, responseTimeMs)
	default:
		return h.svc.GradeNotebookAnswer(ctx, card.VocabCard(), answer, responseTimeMs)
	}
}

// buildRelearnResponse assembles the feedback response: the result card plus
// the Learn-page context. It deliberately sets no next_review_date /
// learned_at (they are not fields on the message).
func (h *QuizHandler) buildRelearnResponse(ctx context.Context, card quiz.RelearnCard, grade quiz.GradeResult) *apiv1.SubmitRelearnAnswerResponse {
	resp := &apiv1.SubmitRelearnAnswerResponse{
		Correct:       grade.Correct,
		Meaning:       card.Meaning,
		Reason:        grade.Reason,
		WordDetail:    toProtoWordDetail(card.WordDetail),
		Images:        card.Images,
		ContextScenes: toProtoRelearnScenes(card.ContextScenes),
	}
	if card.IsEtymology() {
		ec := card.EtymologyCard()
		resp.ExampleWords = h.loadCardExampleWords(ec)
		if r, err := h.svc.NewReader(); err == nil {
			concepts := loadBookConcepts(ctx, r, ec.NotebookName)
			resp.GraphContext = buildGraphContextForCard(ctx, r, ec, concepts)
		}
	}
	return resp
}

func toProtoRelearnScenes(scenes []quiz.RelearnContextScene) []*apiv1.RelearnContextScene {
	if len(scenes) == 0 {
		return nil
	}
	out := make([]*apiv1.RelearnContextScene, 0, len(scenes))
	for _, s := range scenes {
		var conversations []*apiv1.RelearnConversationLine
		for _, c := range s.Conversations {
			conversations = append(conversations, &apiv1.RelearnConversationLine{Speaker: c.Speaker, Quote: c.Quote})
		}
		out = append(out, &apiv1.RelearnContextScene{
			NotebookName:  s.NotebookName,
			SceneTitle:    s.SceneTitle,
			Statements:    s.Statements,
			Conversations: conversations,
		})
	}
	return out
}

// notebookQuizTypeToProto is the inverse of protoQuizTypeToNotebook. It labels
// where a pooled word originally came from; the value is never persisted.
func notebookQuizTypeToProto(qt notebook.QuizType) apiv1.QuizType {
	switch qt {
	case notebook.QuizTypeReverse:
		return apiv1.QuizType_QUIZ_TYPE_REVERSE
	case notebook.QuizTypeFreeform:
		return apiv1.QuizType_QUIZ_TYPE_FREEFORM
	case notebook.QuizTypeEtymologyStandard:
		return apiv1.QuizType_QUIZ_TYPE_ETYMOLOGY_STANDARD
	case notebook.QuizTypeEtymologyReverse:
		return apiv1.QuizType_QUIZ_TYPE_ETYMOLOGY_REVERSE
	case notebook.QuizTypeEtymologyFreeform:
		return apiv1.QuizType_QUIZ_TYPE_ETYMOLOGY_FREEFORM
	default:
		return apiv1.QuizType_QUIZ_TYPE_STANDARD
	}
}
