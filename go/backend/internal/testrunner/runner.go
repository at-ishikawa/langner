// Package testrunner loads .textpb TestCase files and runs them as subtests.
//
// The case directory layout under cfg.CasesRoot is:
//   <service-fullname>/<method>/<scenario>.textpb
// e.g. api.v1.NotebookService/RegisterDefinition/happy_path.textpb
//
// For each case, the runner:
//   1. Parses it as testing.v1.TestCase.
//   2. If an Invoker is registered for the RPC ("<service>/<method>"), runs
//      setup, invokes the handler with the typed request, and asserts the
//      Expected block.
//   3. If no Invoker is registered, the case still parses (smoke check) and
//      the subtest passes — useful for newly-declared RPCs whose runner
//      wiring lands in a follow-up PR.
package testrunner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/known/anypb"

	testingv1 "github.com/at-ishikawa/langner/gen-protos/testing/v1"
)

// Deps is what the runner provides to handler-construction code: a per-case
// sandbox so handlers can be wired with isolated state.
type Deps struct {
	TempDir string            // fresh per case
	Vars    map[string]string // case.vars merged with runner-provided defaults
}

// Invoker invokes one RPC. Implementations unmarshal req into the typed
// request, call the handler, then marshal the response back into Any.
type Invoker func(ctx context.Context, deps Deps, req *anypb.Any) (*anypb.Any, error)

// Config wires the runner to a service.
type Config struct {
	CasesRoot string             // path to proto/cases
	Service   string             // service fullname, e.g. "api.v1.NotebookService"
	Invokers  map[string]Invoker // method name → invoker; nil/missing = parse-only
}

// Run discovers all cases for cfg.Service and runs each as a subtest.
func Run(t *testing.T, cfg Config) {
	t.Helper()
	svcDir := filepath.Join(cfg.CasesRoot, cfg.Service)
	methodDirs, err := os.ReadDir(svcDir)
	if err != nil {
		t.Fatalf("read service dir %q: %v", svcDir, err)
	}
	sort.Slice(methodDirs, func(i, j int) bool { return methodDirs[i].Name() < methodDirs[j].Name() })

	for _, md := range methodDirs {
		if !md.IsDir() {
			continue
		}
		method := md.Name()
		t.Run(method, func(t *testing.T) {
			files, err := filepath.Glob(filepath.Join(svcDir, method, "*.textpb"))
			if err != nil {
				t.Fatalf("glob: %v", err)
			}
			sort.Strings(files)
			for _, f := range files {
				tc, err := loadCase(f)
				if err != nil {
					t.Run(filepath.Base(f), func(t *testing.T) {
						t.Fatalf("parse %s: %v", f, err)
					})
					continue
				}
				name := tc.GetName()
				if name == "" {
					name = strings.TrimSuffix(filepath.Base(f), ".textpb")
				}
				t.Run(name, func(t *testing.T) {
					runOne(t, cfg, method, tc)
				})
			}
		})
	}
}

func loadCase(path string) (*testingv1.TestCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	tc := &testingv1.TestCase{}
	opts := prototext.UnmarshalOptions{
		Resolver:     protoregistry.GlobalTypes,
		AllowPartial: true,
	}
	if err := opts.Unmarshal(data, tc); err != nil {
		return nil, err
	}
	return tc, nil
}

func runOne(t *testing.T, cfg Config, method string, tc *testingv1.TestCase) {
	t.Helper()
	invoker, ok := cfg.Invokers[method]
	if !ok || invoker == nil {
		// No invoker wired yet — case parsed cleanly, that's the smoke check.
		t.Logf("no invoker registered for %s/%s; smoke check only", cfg.Service, method)
		return
	}
	if tc.GetRequest() == nil {
		// Invoker exists but case has no request — treat as smoke.
		t.Logf("case %q has no request; smoke check only", tc.GetName())
		return
	}

	deps := Deps{
		TempDir: t.TempDir(),
		Vars:    tc.GetVars(),
	}
	if err := applySetup(deps, tc.GetSetup()); err != nil {
		t.Fatalf("setup: %v", err)
	}

	ctx := context.Background()
	resp, err := invoker(ctx, deps, tc.GetRequest())
	assertExpected(t, tc.GetExpected(), resp, err, deps)
}

// applySetup runs the SetupSteps that are meaningful without external
// services. Steps like StubResponse / InsertRows / ClearTables are
// implemented as needed by future PRs.
func applySetup(deps Deps, steps []*testingv1.SetupStep) error {
	for _, s := range steps {
		switch step := s.GetStep().(type) {
		case *testingv1.SetupStep_ClearDirs:
			for _, p := range step.ClearDirs.GetPaths() {
				resolved := substitute(p, deps)
				if err := os.RemoveAll(resolved); err != nil {
					return fmt.Errorf("clear_dirs %q: %w", resolved, err)
				}
				if err := os.MkdirAll(resolved, 0o755); err != nil {
					return fmt.Errorf("clear_dirs mkdir %q: %w", resolved, err)
				}
			}
		case *testingv1.SetupStep_WriteFile:
			resolved := substitute(step.WriteFile.GetPath(), deps)
			if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
				return fmt.Errorf("write_file mkdir: %w", err)
			}
			if err := os.WriteFile(resolved, step.WriteFile.GetContent(), 0o644); err != nil {
				return fmt.Errorf("write_file %q: %w", resolved, err)
			}
		case *testingv1.SetupStep_RemoveFiles:
			for _, p := range step.RemoveFiles.GetPaths() {
				_ = os.Remove(substitute(p, deps))
			}
		}
	}
	return nil
}

// substitute replaces $TEMP_DIR and $VAR_<name> in a path string with values
// from deps. Keep the substitution language tiny on purpose.
func substitute(s string, deps Deps) string {
	s = strings.ReplaceAll(s, "$TEMP_DIR", deps.TempDir)
	for k, v := range deps.Vars {
		s = strings.ReplaceAll(s, "$"+k, v)
	}
	return s
}

func assertExpected(t *testing.T, want *testingv1.Expected, got *anypb.Any, gotErr error, deps Deps) {
	t.Helper()
	wantStatus := connect.Code(want.GetStatus())

	if wantStatus != 0 {
		if gotErr == nil {
			t.Fatalf("want error code %v, got nil error", wantStatus)
		}
		var connectErr *connect.Error
		if !errors.As(gotErr, &connectErr) {
			t.Fatalf("want connect error code %v, got non-connect error: %v", wantStatus, gotErr)
		}
		if connectErr.Code() != wantStatus {
			t.Fatalf("want code %v, got %v: %v", wantStatus, connectErr.Code(), gotErr)
		}
		if rx := want.GetError().GetMessageRegex(); rx != "" {
			ok, _ := matchRegex(rx, gotErr.Error())
			if !ok {
				t.Fatalf("error message %q does not match %q", gotErr.Error(), rx)
			}
		}
	} else if gotErr != nil {
		t.Fatalf("want OK, got error: %v", gotErr)
	}

	if want.GetBody() != nil && got != nil {
		// Partial match: every field set in want.body must equal got.body's value.
		if err := assertBodyMatches(want.GetBody(), got); err != nil {
			t.Fatalf("body mismatch: %v", err)
		}
	}

	for _, st := range want.GetState() {
		if err := assertState(st, deps); err != nil {
			t.Fatalf("state assertion: %v", err)
		}
	}
}

func assertBodyMatches(want, got *anypb.Any) error {
	wantMsg, err := want.UnmarshalNew()
	if err != nil {
		return fmt.Errorf("unmarshal want: %w", err)
	}
	gotMsg, err := got.UnmarshalNew()
	if err != nil {
		return fmt.Errorf("unmarshal got: %w", err)
	}
	// Strict equality is good enough for now; partial matching is a follow-up.
	if !proto.Equal(wantMsg, gotMsg) {
		return fmt.Errorf("\nwant: %s\ngot:  %s",
			prototext.Format(wantMsg), prototext.Format(gotMsg))
	}
	return nil
}

func assertState(st *testingv1.StateAssertion, deps Deps) error {
	switch a := st.GetAssertion().(type) {
	case *testingv1.StateAssertion_FileExists:
		path := substitute(a.FileExists.GetPath(), deps)
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("file_exists %q: %w", path, err)
		}
	case *testingv1.StateAssertion_FileAbsent:
		path := substitute(a.FileAbsent.GetPath(), deps)
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("file_absent %q: file exists", path)
		}
	case *testingv1.StateAssertion_FileContains:
		path := substitute(a.FileContains.GetPath(), deps)
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("file_contains read %q: %w", path, err)
		}
		s := string(data)
		for _, want := range a.FileContains.GetContains() {
			if !strings.Contains(s, want) {
				return fmt.Errorf("file_contains %q missing %q", path, want)
			}
		}
	}
	return nil
}

// matchRegex is a tiny wrapper so the runner stays buildable even if the
// regex package import surface changes.
func matchRegex(pattern, s string) (bool, error) {
	re, err := compileRegex(pattern)
	if err != nil {
		return false, err
	}
	return re.MatchString(s), nil
}
