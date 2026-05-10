package server

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/testrunner"
)

// newNotebookHandlerForCases builds a NotebookHandler wired to a fresh
// per-case sandbox: tempdir-backed definitions directory and a YAML note
// repository pointing at the same dir. RPCs that need network upstreams
// (LookupWord, GetEtymologyNotebook) need richer wiring; this is enough
// for the filesystem-only handlers (RegisterDefinition, DeleteDefinition).
func newNotebookHandlerForCases(deps testrunner.Deps) *NotebookHandler {
	defsDir := deps.TempDir
	return NewNotebookHandler(
		config.NotebooksConfig{DefinitionsDirectories: []string{defsDir}},
		config.TemplatesConfig{},
		make(map[string]rapidapi.Response),
		nil,
		nil,
		notebook.NewYAMLNoteRepositoryWithDefsDir(defsDir),
	)
}

func invokeRegisterDefinition(ctx context.Context, deps testrunner.Deps, req proto.Message) (proto.Message, error) {
	typedReq, ok := req.(*apiv1.RegisterDefinitionRequest)
	if !ok || typedReq == nil {
		return nil, fmt.Errorf("RegisterDefinition: nil or wrong-type request")
	}
	handler := newNotebookHandlerForCases(deps)
	resp, err := handler.RegisterDefinition(ctx, connect.NewRequest(typedReq))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func invokeDeleteDefinition(ctx context.Context, deps testrunner.Deps, req proto.Message) (proto.Message, error) {
	typedReq, ok := req.(*apiv1.DeleteDefinitionRequest)
	if !ok || typedReq == nil {
		return nil, fmt.Errorf("DeleteDefinition: nil or wrong-type request")
	}
	handler := newNotebookHandlerForCases(deps)
	resp, err := handler.DeleteDefinition(ctx, connect.NewRequest(typedReq))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

// The four invokers below build a NotebookHandler with zero-valued upstream
// dependencies. They are sufficient for cases that exercise the validation
// layer that runs first; cases that need real handler behavior require
// upstream stubbing (separate PR).

func invokeGetNotebookDetail(ctx context.Context, deps testrunner.Deps, req proto.Message) (proto.Message, error) {
	typedReq, ok := req.(*apiv1.GetNotebookDetailRequest)
	if !ok || typedReq == nil {
		return nil, fmt.Errorf("GetNotebookDetail: nil or wrong-type request")
	}
	handler := newNotebookHandlerForCases(deps)
	resp, err := handler.GetNotebookDetail(ctx, connect.NewRequest(typedReq))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func invokeExportNotebookPDF(ctx context.Context, deps testrunner.Deps, req proto.Message) (proto.Message, error) {
	typedReq, ok := req.(*apiv1.ExportNotebookPDFRequest)
	if !ok || typedReq == nil {
		return nil, fmt.Errorf("ExportNotebookPDF: nil or wrong-type request")
	}
	handler := newNotebookHandlerForCases(deps)
	resp, err := handler.ExportNotebookPDF(ctx, connect.NewRequest(typedReq))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func invokeLookupWord(ctx context.Context, deps testrunner.Deps, req proto.Message) (proto.Message, error) {
	typedReq, ok := req.(*apiv1.LookupWordRequest)
	if !ok || typedReq == nil {
		return nil, fmt.Errorf("LookupWord: nil or wrong-type request")
	}
	handler := newNotebookHandlerForCases(deps)
	resp, err := handler.LookupWord(ctx, connect.NewRequest(typedReq))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func invokeGetEtymologyNotebook(ctx context.Context, deps testrunner.Deps, req proto.Message) (proto.Message, error) {
	typedReq, ok := req.(*apiv1.GetEtymologyNotebookRequest)
	if !ok || typedReq == nil {
		return nil, fmt.Errorf("GetEtymologyNotebook: nil or wrong-type request")
	}
	handler := newNotebookHandlerForCases(deps)
	resp, err := handler.GetEtymologyNotebook(ctx, connect.NewRequest(typedReq))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}
