package server

import (
	"testing"

	"github.com/at-ishikawa/langner/internal/testrunner"
)

// TestNotebookService_Cases runs every .textpb case under
// proto/cases/api.v1.NotebookService/<Method>/. RPCs without a registered
// invoker pass as smoke checks (case file parses); RPCs with an invoker
// (e.g. RegisterDefinition) actually invoke the handler and assert.
func TestNotebookService_Cases(t *testing.T) {
	testrunner.Run(t, testrunner.Config{
		CasesRoot: "../../../../proto/cases",
		Service:   "api.v1.NotebookService",
		Invokers:  notebookInvokers(),
	})
}

// TestQuizService_Cases runs every .textpb case under
// proto/cases/api.v1.QuizService/<Method>/. All entries are smoke-only
// for now; per-RPC invokers land in follow-up PRs.
func TestQuizService_Cases(t *testing.T) {
	testrunner.Run(t, testrunner.Config{
		CasesRoot: "../../../../proto/cases",
		Service:   "api.v1.QuizService",
		Invokers:  nil,
	})
}
