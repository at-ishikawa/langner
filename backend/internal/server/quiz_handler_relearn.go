package server

import (
	"context"
	"fmt"
	"log/slog"
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

	var clears map[string]time.Time
	if h.relearnClears != nil {
		var err error
		clears, err = h.relearnClears.AllClears(ctx)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("load relearn clears: %w", err))
		}
	}

	cards, err := h.svc.LoadRelearnPool(time.Now().Add(-window), clears)
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
		protoCards = append(protoCards, &apiv1.RelearnCard{
			NoteId:         noteID,
			Entry:          card.Entry,
			SourceQuizType: notebookQuizTypeToProto(card.SourceQuizType),
			Examples:       examples,
		})
	}

	h.mu.Lock()
	h.relearnStore = localStore
	h.nextID = nextID
	h.mu.Unlock()

	return connect.NewResponse(&apiv1.StartRelearnQuizResponse{Cards: protoCards}), nil
}

// SubmitRelearnAnswer grades one relearn answer. It records NOTHING to learning
// history: it calls only the pure meaning graders and, on a correct answer,
// writes the non-SR relearn-clear marker. It never touches Save*, UpdateLog, or
// GetLatestLearnedInfo, and the response carries no next_review_date /
// learned_at.
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

	h.markRelearnCleared(ctx, card, grade)
	return connect.NewResponse(h.buildRelearnResponse(ctx, card, grade)), nil
}

// BatchSubmitRelearnAnswers grades a batch of relearn answers. Grading runs in
// parallel; clear markers are written serially. Like the single-shot path it
// writes no learning history.
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
		h.markRelearnCleared(ctx, cards[i], grades[i])
		responses[i] = h.buildRelearnResponse(ctx, cards[i], grades[i])
	}
	return connect.NewResponse(&apiv1.BatchSubmitRelearnAnswersResponse{Responses: responses}), nil
}

// gradeRelearn dispatches to the pure grader matching the card's kind. A skip
// is graded as wrong without calling the grader (same as the other quizzes).
func (h *QuizHandler) gradeRelearn(ctx context.Context, card quiz.RelearnCard, answer string, responseTimeMs int64, skipped bool) (quiz.GradeResult, error) {
	if skipped {
		return skippedGradeResult(), nil
	}
	if card.IsEtymology {
		return h.svc.GradeEtymologyStandardAnswer(ctx, card.EtymologyCard(), answer, responseTimeMs)
	}
	return h.svc.GradeNotebookAnswer(ctx, card.VocabCard(), answer, responseTimeMs)
}

// markRelearnCleared records the non-SR clear marker when the answer is
// correct. A marker-write failure is logged but never fails the answer — the
// only consequence is that the word may reappear in the next relearn session.
func (h *QuizHandler) markRelearnCleared(ctx context.Context, card quiz.RelearnCard, grade quiz.GradeResult) {
	if !grade.Correct || h.relearnClears == nil {
		return
	}
	if err := h.relearnClears.MarkCleared(ctx, card.ClearKey, time.Now()); err != nil {
		slog.Warn("failed to record relearn clear", "clear_key", card.ClearKey, "error", err)
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
	if card.IsEtymology {
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
