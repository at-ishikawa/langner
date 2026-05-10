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
				Suite: func() proto.Message { return &notebookcases.GetNotebookDetailTestSuite{} },
			},
			"ExportNotebookPDF": {
				Suite: func() proto.Message { return &notebookcases.ExportNotebookPDFTestSuite{} },
			},
			"LookupWord": {
				Suite: func() proto.Message { return &notebookcases.LookupWordTestSuite{} },
			},
			"GetEtymologyNotebook": {
				Suite: func() proto.Message { return &notebookcases.GetEtymologyNotebookTestSuite{} },
			},
		},
	})
}

// TestQuizService_Cases runs every .textpb case under
// proto/cases/api.v1.QuizService/<Method>/. All entries provide a Suite
// factory so cases parse and validate; per-RPC invokers land in follow-up
// PRs as quiz handler dependencies are made test-friendly.
func TestQuizService_Cases(t *testing.T) {
	testrunner.Run(t, testrunner.Config{
		CasesRoot: casesRoot,
		Service:   "api.v1.QuizService",
		Methods:   quizSuiteFactories(),
	})
}

func quizSuiteFactories() map[string]testrunner.MethodBinding {
	return map[string]testrunner.MethodBinding{
		"GetQuizOptions":                       {Suite: func() proto.Message { return &quizcases.GetQuizOptionsTestSuite{} }},
		"StartQuiz":                            {Suite: func() proto.Message { return &quizcases.StartQuizTestSuite{} }},
		"SubmitAnswer":                         {Suite: func() proto.Message { return &quizcases.SubmitAnswerTestSuite{} }},
		"BatchSubmitAnswers":                   {Suite: func() proto.Message { return &quizcases.BatchSubmitAnswersTestSuite{} }},
		"StartReverseQuiz":                     {Suite: func() proto.Message { return &quizcases.StartReverseQuizTestSuite{} }},
		"SubmitReverseAnswer":                  {Suite: func() proto.Message { return &quizcases.SubmitReverseAnswerTestSuite{} }},
		"BatchSubmitReverseAnswers":            {Suite: func() proto.Message { return &quizcases.BatchSubmitReverseAnswersTestSuite{} }},
		"StartFreeformQuiz":                    {Suite: func() proto.Message { return &quizcases.StartFreeformQuizTestSuite{} }},
		"SubmitFreeformAnswer":                 {Suite: func() proto.Message { return &quizcases.SubmitFreeformAnswerTestSuite{} }},
		"OverrideAnswer":                       {Suite: func() proto.Message { return &quizcases.OverrideAnswerTestSuite{} }},
		"UndoOverrideAnswer":                   {Suite: func() proto.Message { return &quizcases.UndoOverrideAnswerTestSuite{} }},
		"SkipWord":                             {Suite: func() proto.Message { return &quizcases.SkipWordTestSuite{} }},
		"ResumeWord":                           {Suite: func() proto.Message { return &quizcases.ResumeWordTestSuite{} }},
		"StartEtymologyQuiz":                   {Suite: func() proto.Message { return &quizcases.StartEtymologyQuizTestSuite{} }},
		"SubmitEtymologyStandardAnswer":        {Suite: func() proto.Message { return &quizcases.SubmitEtymologyStandardAnswerTestSuite{} }},
		"BatchSubmitEtymologyStandardAnswers":  {Suite: func() proto.Message { return &quizcases.BatchSubmitEtymologyStandardAnswersTestSuite{} }},
		"SubmitEtymologyReverseAnswer":         {Suite: func() proto.Message { return &quizcases.SubmitEtymologyReverseAnswerTestSuite{} }},
		"BatchSubmitEtymologyReverseAnswers":   {Suite: func() proto.Message { return &quizcases.BatchSubmitEtymologyReverseAnswersTestSuite{} }},
		"StartEtymologyFreeformQuiz":           {Suite: func() proto.Message { return &quizcases.StartEtymologyFreeformQuizTestSuite{} }},
		"SubmitEtymologyFreeformAnswer":        {Suite: func() proto.Message { return &quizcases.SubmitEtymologyFreeformAnswerTestSuite{} }},
	}
}
