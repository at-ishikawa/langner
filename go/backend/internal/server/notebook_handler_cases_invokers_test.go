package server

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/anypb"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/testrunner"
)

// notebookInvokers returns the per-RPC invokers wired to NotebookHandler.
// Only RPCs with an entry here run their cases against a real handler;
// entries left out fall through to smoke-only mode.
func notebookInvokers() map[string]testrunner.Invoker {
	return map[string]testrunner.Invoker{
		"RegisterDefinition": invokeRegisterDefinition,
	}
}

func invokeRegisterDefinition(ctx context.Context, deps testrunner.Deps, reqAny *anypb.Any) (*anypb.Any, error) {
	defsDir := deps.TempDir
	handler := NewNotebookHandler(
		config.NotebooksConfig{DefinitionsDirectories: []string{defsDir}},
		config.TemplatesConfig{},
		make(map[string]rapidapi.Response),
		nil,
		nil,
		notebook.NewYAMLNoteRepositoryWithDefsDir(defsDir),
	)

	req := &apiv1.RegisterDefinitionRequest{}
	if err := reqAny.UnmarshalTo(req); err != nil {
		return nil, fmt.Errorf("unmarshal RegisterDefinitionRequest: %w", err)
	}

	resp, err := handler.RegisterDefinition(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	out, err := anypb.New(resp.Msg)
	if err != nil {
		return nil, fmt.Errorf("marshal RegisterDefinitionResponse: %w", err)
	}
	return out, nil
}
