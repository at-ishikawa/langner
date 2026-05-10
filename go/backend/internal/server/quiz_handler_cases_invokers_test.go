package server

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/quiz"
	"github.com/at-ishikawa/langner/testrunner"
)

// newQuizHandlerForCases returns a QuizHandler with all directory paths
// pointing at empty per-case tempdirs. Handlers that walk these to load
// notebook/learning state see an empty corpus rather than panicking on
// missing dirs. RPCs that hit OpenAI or RapidAPI need richer wiring;
// those cases stay validation-only for now.
func newQuizHandlerForCases(deps testrunner.Deps) *QuizHandler {
	dir := deps.TempDir
	cfg := config.NotebooksConfig{
		StoriesDirectories:     []string{dir},
		FlashcardsDirectories:  []string{dir},
		BooksDirectories:       []string{dir},
		DefinitionsDirectories: []string{dir},
		EtymologyDirectories:   []string{dir},
		LearningNotesDirectory: dir,
	}
	svc := quiz.NewService(
		cfg,
		nil,
		make(map[string]rapidapi.Response),
		nil,
		config.QuizConfig{},
	)
	return NewQuizHandler(svc)
}

// quizInvoker wraps a typed RPC call into the testrunner's generic Invoker.
// ReqT and RespT are the concrete proto Go structs (not pointers).
func quizInvoker[ReqT, RespT any](
	call func(ctx context.Context, h *QuizHandler, req *connect.Request[ReqT]) (*connect.Response[RespT], error),
) testrunner.Invoker {
	return func(ctx context.Context, deps testrunner.Deps, raw proto.Message) (proto.Message, error) {
		typedReq, ok := any(raw).(*ReqT)
		if !ok {
			return nil, fmt.Errorf("wrong request type: %T", raw)
		}
		h := newQuizHandlerForCases(deps)
		resp, err := call(ctx, h, connect.NewRequest(typedReq))
		if err != nil {
			return nil, err
		}
		return any(resp.Msg).(proto.Message), nil
	}
}

func quizInvokers() map[string]testrunner.Invoker {
	return map[string]testrunner.Invoker{
		"GetQuizOptions": quizInvoker(func(ctx context.Context, h *QuizHandler, r *connect.Request[apiv1.GetQuizOptionsRequest]) (*connect.Response[apiv1.GetQuizOptionsResponse], error) {
			return h.GetQuizOptions(ctx, r)
		}),
		"StartQuiz": quizInvoker(func(ctx context.Context, h *QuizHandler, r *connect.Request[apiv1.StartQuizRequest]) (*connect.Response[apiv1.StartQuizResponse], error) {
			return h.StartQuiz(ctx, r)
		}),
		"SubmitAnswer": quizInvoker(func(ctx context.Context, h *QuizHandler, r *connect.Request[apiv1.SubmitAnswerRequest]) (*connect.Response[apiv1.SubmitAnswerResponse], error) {
			return h.SubmitAnswer(ctx, r)
		}),
		"BatchSubmitAnswers": quizInvoker(func(ctx context.Context, h *QuizHandler, r *connect.Request[apiv1.BatchSubmitAnswersRequest]) (*connect.Response[apiv1.BatchSubmitAnswersResponse], error) {
			return h.BatchSubmitAnswers(ctx, r)
		}),
		"StartReverseQuiz": quizInvoker(func(ctx context.Context, h *QuizHandler, r *connect.Request[apiv1.StartReverseQuizRequest]) (*connect.Response[apiv1.StartReverseQuizResponse], error) {
			return h.StartReverseQuiz(ctx, r)
		}),
		"SubmitReverseAnswer": quizInvoker(func(ctx context.Context, h *QuizHandler, r *connect.Request[apiv1.SubmitReverseAnswerRequest]) (*connect.Response[apiv1.SubmitReverseAnswerResponse], error) {
			return h.SubmitReverseAnswer(ctx, r)
		}),
		"BatchSubmitReverseAnswers": quizInvoker(func(ctx context.Context, h *QuizHandler, r *connect.Request[apiv1.BatchSubmitReverseAnswersRequest]) (*connect.Response[apiv1.BatchSubmitReverseAnswersResponse], error) {
			return h.BatchSubmitReverseAnswers(ctx, r)
		}),
		"StartFreeformQuiz": quizInvoker(func(ctx context.Context, h *QuizHandler, r *connect.Request[apiv1.StartFreeformQuizRequest]) (*connect.Response[apiv1.StartFreeformQuizResponse], error) {
			return h.StartFreeformQuiz(ctx, r)
		}),
		"SubmitFreeformAnswer": quizInvoker(func(ctx context.Context, h *QuizHandler, r *connect.Request[apiv1.SubmitFreeformAnswerRequest]) (*connect.Response[apiv1.SubmitFreeformAnswerResponse], error) {
			return h.SubmitFreeformAnswer(ctx, r)
		}),
		"OverrideAnswer": quizInvoker(func(ctx context.Context, h *QuizHandler, r *connect.Request[apiv1.OverrideAnswerRequest]) (*connect.Response[apiv1.OverrideAnswerResponse], error) {
			return h.OverrideAnswer(ctx, r)
		}),
		"UndoOverrideAnswer": quizInvoker(func(ctx context.Context, h *QuizHandler, r *connect.Request[apiv1.UndoOverrideAnswerRequest]) (*connect.Response[apiv1.UndoOverrideAnswerResponse], error) {
			return h.UndoOverrideAnswer(ctx, r)
		}),
		"SkipWord": quizInvoker(func(ctx context.Context, h *QuizHandler, r *connect.Request[apiv1.SkipWordRequest]) (*connect.Response[apiv1.SkipWordResponse], error) {
			return h.SkipWord(ctx, r)
		}),
		"ResumeWord": quizInvoker(func(ctx context.Context, h *QuizHandler, r *connect.Request[apiv1.ResumeWordRequest]) (*connect.Response[apiv1.ResumeWordResponse], error) {
			return h.ResumeWord(ctx, r)
		}),
		"StartEtymologyQuiz": quizInvoker(func(ctx context.Context, h *QuizHandler, r *connect.Request[apiv1.StartEtymologyQuizRequest]) (*connect.Response[apiv1.StartEtymologyQuizResponse], error) {
			return h.StartEtymologyQuiz(ctx, r)
		}),
		"SubmitEtymologyStandardAnswer": quizInvoker(func(ctx context.Context, h *QuizHandler, r *connect.Request[apiv1.SubmitEtymologyStandardAnswerRequest]) (*connect.Response[apiv1.SubmitEtymologyStandardAnswerResponse], error) {
			return h.SubmitEtymologyStandardAnswer(ctx, r)
		}),
		"BatchSubmitEtymologyStandardAnswers": quizInvoker(func(ctx context.Context, h *QuizHandler, r *connect.Request[apiv1.BatchSubmitEtymologyStandardAnswersRequest]) (*connect.Response[apiv1.BatchSubmitEtymologyStandardAnswersResponse], error) {
			return h.BatchSubmitEtymologyStandardAnswers(ctx, r)
		}),
		"SubmitEtymologyReverseAnswer": quizInvoker(func(ctx context.Context, h *QuizHandler, r *connect.Request[apiv1.SubmitEtymologyReverseAnswerRequest]) (*connect.Response[apiv1.SubmitEtymologyReverseAnswerResponse], error) {
			return h.SubmitEtymologyReverseAnswer(ctx, r)
		}),
		"BatchSubmitEtymologyReverseAnswers": quizInvoker(func(ctx context.Context, h *QuizHandler, r *connect.Request[apiv1.BatchSubmitEtymologyReverseAnswersRequest]) (*connect.Response[apiv1.BatchSubmitEtymologyReverseAnswersResponse], error) {
			return h.BatchSubmitEtymologyReverseAnswers(ctx, r)
		}),
		"StartEtymologyFreeformQuiz": quizInvoker(func(ctx context.Context, h *QuizHandler, r *connect.Request[apiv1.StartEtymologyFreeformQuizRequest]) (*connect.Response[apiv1.StartEtymologyFreeformQuizResponse], error) {
			return h.StartEtymologyFreeformQuiz(ctx, r)
		}),
		"SubmitEtymologyFreeformAnswer": quizInvoker(func(ctx context.Context, h *QuizHandler, r *connect.Request[apiv1.SubmitEtymologyFreeformAnswerRequest]) (*connect.Response[apiv1.SubmitEtymologyFreeformAnswerResponse], error) {
			return h.SubmitEtymologyFreeformAnswer(ctx, r)
		}),
	}
}
