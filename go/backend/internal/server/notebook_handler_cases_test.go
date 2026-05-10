package server

import (
	"testing"

	"google.golang.org/protobuf/proto"

	notebookcases "github.com/at-ishikawa/langner/gen-protos/testing/cases/api/v1/notebook_service"
	quizcases "github.com/at-ishikawa/langner/gen-protos/testing/cases/api/v1/quiz_service"
	"github.com/at-ishikawa/langner/testrunner"
)

const casesRoot = "../../../../proto/cases"

// TestNotebookService_Cases runs every .textpb case under
// proto/cases/api.v1.NotebookService/<Method>/. The per-RPC Suite types are
// generated from notebook.proto by gen-test-suites.
func TestNotebookService_Cases(t *testing.T) {
	testrunner.Run(t, testrunner.Config{
		CasesRoot: casesRoot,
		Service:   "api.v1.NotebookService",
		Methods: map[string]testrunner.MethodBinding{
			"RegisterDefinition": {
				Suite:   func() proto.Message { return &notebookcases.RegisterDefinitionTestSuite{} },
				Invoker: invokeRegisterDefinition,
			},
			"DeleteDefinition": {
				Suite:   func() proto.Message { return &notebookcases.DeleteDefinitionTestSuite{} },
				Invoker: invokeDeleteDefinition,
			},
			"GetNotebookDetail": {
				Suite:   func() proto.Message { return &notebookcases.GetNotebookDetailTestSuite{} },
				Invoker: invokeGetNotebookDetail,
			},
			"ExportNotebookPDF": {
				Suite:   func() proto.Message { return &notebookcases.ExportNotebookPDFTestSuite{} },
				Invoker: invokeExportNotebookPDF,
			},
			"LookupWord": {
				Suite:   func() proto.Message { return &notebookcases.LookupWordTestSuite{} },
				Invoker: invokeLookupWord,
			},
			"GetEtymologyNotebook": {
				Suite:   func() proto.Message { return &notebookcases.GetEtymologyNotebookTestSuite{} },
				Invoker: invokeGetEtymologyNotebook,
			},
		},
	})
}

// TestQuizService_Cases runs every .textpb case under
// proto/cases/api.v1.QuizService/<Method>/. Invokers come from
// quizInvokers(); each builds a QuizHandler with zero-valued deps which is
// enough to exercise the validation layer that runs at handler entry.
func TestQuizService_Cases(t *testing.T) {
	suiteFactories := map[string]func() proto.Message{
		"GetQuizOptions":                      func() proto.Message { return &quizcases.GetQuizOptionsTestSuite{} },
		"StartQuiz":                           func() proto.Message { return &quizcases.StartQuizTestSuite{} },
		"SubmitAnswer":                        func() proto.Message { return &quizcases.SubmitAnswerTestSuite{} },
		"BatchSubmitAnswers":                  func() proto.Message { return &quizcases.BatchSubmitAnswersTestSuite{} },
		"StartReverseQuiz":                    func() proto.Message { return &quizcases.StartReverseQuizTestSuite{} },
		"SubmitReverseAnswer":                 func() proto.Message { return &quizcases.SubmitReverseAnswerTestSuite{} },
		"BatchSubmitReverseAnswers":           func() proto.Message { return &quizcases.BatchSubmitReverseAnswersTestSuite{} },
		"StartFreeformQuiz":                   func() proto.Message { return &quizcases.StartFreeformQuizTestSuite{} },
		"SubmitFreeformAnswer":                func() proto.Message { return &quizcases.SubmitFreeformAnswerTestSuite{} },
		"OverrideAnswer":                      func() proto.Message { return &quizcases.OverrideAnswerTestSuite{} },
		"UndoOverrideAnswer":                  func() proto.Message { return &quizcases.UndoOverrideAnswerTestSuite{} },
		"SkipWord":                            func() proto.Message { return &quizcases.SkipWordTestSuite{} },
		"ResumeWord":                          func() proto.Message { return &quizcases.ResumeWordTestSuite{} },
		"StartEtymologyQuiz":                  func() proto.Message { return &quizcases.StartEtymologyQuizTestSuite{} },
		"SubmitEtymologyStandardAnswer":       func() proto.Message { return &quizcases.SubmitEtymologyStandardAnswerTestSuite{} },
		"BatchSubmitEtymologyStandardAnswers": func() proto.Message { return &quizcases.BatchSubmitEtymologyStandardAnswersTestSuite{} },
		"SubmitEtymologyReverseAnswer":        func() proto.Message { return &quizcases.SubmitEtymologyReverseAnswerTestSuite{} },
		"BatchSubmitEtymologyReverseAnswers":  func() proto.Message { return &quizcases.BatchSubmitEtymologyReverseAnswersTestSuite{} },
		"StartEtymologyFreeformQuiz":          func() proto.Message { return &quizcases.StartEtymologyFreeformQuizTestSuite{} },
		"SubmitEtymologyFreeformAnswer":       func() proto.Message { return &quizcases.SubmitEtymologyFreeformAnswerTestSuite{} },
	}
	bindings := make(map[string]testrunner.MethodBinding, len(suiteFactories))
	invokers := quizInvokers()
	for method, factory := range suiteFactories {
		bindings[method] = testrunner.MethodBinding{Suite: factory, Invoker: invokers[method]}
	}
	testrunner.Run(t, testrunner.Config{
		CasesRoot: casesRoot,
		Service:   "api.v1.QuizService",
		Methods:   bindings,
	})
}
