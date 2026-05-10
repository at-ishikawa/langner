// Package testrunner loads .textpb TestSuite files and runs them as Go subtests.
//
// The runner is generic via proto reflection — it does not depend on any
// specific schema. Callers provide, per RPC method:
//   - a SuiteFactory that returns an empty per-RPC TestSuite proto.Message.
//     The TestSuite type defines `cases` (repeated TestCase), and each
//     TestCase defines `name` (string), `setup` (repeated SetupStep),
//     `request` (the typed request), and `expected` (Expected).
//   - an Invoker that calls the handler with the typed request.
//
// The runner walks these by field name via protoreflect, so any schema
// that follows the convention above will work.
//
// Case files live at <CasesRoot>/<Service>/<Method>/*.textpb.
package testrunner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// Deps is the per-case sandbox provided to invokers.
type Deps struct {
	TempDir string
	Vars    map[string]string
}

// Invoker invokes one RPC with a typed request and returns its typed response.
type Invoker func(ctx context.Context, deps Deps, req proto.Message) (proto.Message, error)

// SuiteFactory returns an empty per-RPC TestSuite proto.Message.
type SuiteFactory func() proto.Message

// MethodBinding wires one RPC method.
type MethodBinding struct {
	Suite   SuiteFactory
	Invoker Invoker
}

// Config is the runner configuration.
type Config struct {
	CasesRoot string
	Service   string                   // service fullname, e.g. "api.v1.NotebookService"
	Methods   map[string]MethodBinding // method name → binding (nil/missing = smoke only)
}

// Run discovers all .textpb cases for cfg.Service and runs each as a subtest.
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
			runMethod(t, cfg, svcDir, method)
		})
	}
}

func runMethod(t *testing.T, cfg Config, svcDir, method string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(svcDir, method, "*.textpb"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	sort.Strings(files)

	binding, hasBinding := cfg.Methods[method]
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Run(filepath.Base(f), func(t *testing.T) { t.Fatalf("read %s: %v", f, err) })
			continue
		}

		if !hasBinding {
			t.Run(filepath.Base(f), func(t *testing.T) {
				t.Logf("%s/%s: no binding registered; smoke check only", cfg.Service, method)
			})
			continue
		}

		suite := binding.Suite()
		opts := prototext.UnmarshalOptions{
			Resolver:     protoregistry.GlobalTypes,
			AllowPartial: true,
		}
		if err := opts.Unmarshal(data, suite); err != nil {
			t.Run(filepath.Base(f), func(t *testing.T) {
				t.Fatalf("parse %s: %v", f, err)
			})
			continue
		}

		cases, err := extractCases(suite)
		if err != nil {
			t.Run(filepath.Base(f), func(t *testing.T) {
				t.Fatalf("extract cases from %s: %v", f, err)
			})
			continue
		}

		for _, c := range cases {
			name := c.name
			if name == "" {
				name = strings.TrimSuffix(filepath.Base(f), ".textpb")
			}
			t.Run(name, func(t *testing.T) {
				runOne(t, c, binding.Invoker)
			})
		}
	}
}

type oneCase struct {
	name     string
	setup    []protoreflect.Message
	request  proto.Message
	expected protoreflect.Message
	vars     map[string]string
}

func extractCases(suite proto.Message) ([]oneCase, error) {
	msg := suite.ProtoReflect()
	desc := msg.Descriptor()
	casesField := desc.Fields().ByName("cases")
	if casesField == nil {
		return nil, fmt.Errorf("suite type %s has no `cases` field", desc.FullName())
	}
	if !casesField.IsList() || casesField.Kind() != protoreflect.MessageKind {
		return nil, fmt.Errorf("`cases` must be a repeated message field")
	}
	list := msg.Get(casesField).List()
	out := make([]oneCase, 0, list.Len())
	for i := 0; i < list.Len(); i++ {
		caseMsg := list.Get(i).Message()
		c, err := parseCase(caseMsg)
		if err != nil {
			return nil, fmt.Errorf("case %d: %w", i, err)
		}
		out = append(out, c)
	}
	return out, nil
}

func parseCase(m protoreflect.Message) (oneCase, error) {
	desc := m.Descriptor()
	c := oneCase{vars: map[string]string{}}

	if f := desc.Fields().ByName("name"); f != nil {
		c.name = m.Get(f).String()
	}
	if f := desc.Fields().ByName("setup"); f != nil && f.IsList() {
		list := m.Get(f).List()
		for i := 0; i < list.Len(); i++ {
			c.setup = append(c.setup, list.Get(i).Message())
		}
	}
	if f := desc.Fields().ByName("vars"); f != nil && f.IsMap() {
		mp := m.Get(f).Map()
		mp.Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
			c.vars[k.String()] = v.String()
			return true
		})
	}
	if f := desc.Fields().ByName("request"); f != nil && f.Kind() == protoreflect.MessageKind {
		c.request = m.Get(f).Message().Interface()
	}
	if f := desc.Fields().ByName("expected"); f != nil && f.Kind() == protoreflect.MessageKind {
		c.expected = m.Get(f).Message()
	}
	return c, nil
}

func runOne(t *testing.T, c oneCase, invoker Invoker) {
	t.Helper()
	deps := Deps{TempDir: t.TempDir(), Vars: c.vars}

	if err := applySetup(deps, c.setup); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if invoker == nil {
		t.Logf("no invoker registered; case %q parsed but not invoked", c.name)
		return
	}
	resp, err := invoker(context.Background(), deps, c.request)
	assertExpected(t, c.expected, resp, err, deps)
}

// applySetup interprets standard SetupStep oneof variants by field name.
// Unknown step kinds are ignored — easy to extend later.
func applySetup(deps Deps, steps []protoreflect.Message) error {
	for _, s := range steps {
		if err := applyOneStep(deps, s); err != nil {
			return err
		}
	}
	return nil
}

func applyOneStep(deps Deps, s protoreflect.Message) error {
	var firstErr error
	s.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch fd.Name() {
		case "clear_dirs":
			cd := v.Message()
			pathsField := cd.Descriptor().Fields().ByName("paths")
			if pathsField == nil || !pathsField.IsList() {
				return true
			}
			paths := cd.Get(pathsField).List()
			for i := 0; i < paths.Len(); i++ {
				resolved := substitute(paths.Get(i).String(), deps)
				if err := os.RemoveAll(resolved); err != nil {
					firstErr = fmt.Errorf("clear_dirs %q: %w", resolved, err)
					return false
				}
				if err := os.MkdirAll(resolved, 0o755); err != nil {
					firstErr = fmt.Errorf("clear_dirs mkdir %q: %w", resolved, err)
					return false
				}
			}
		case "write_file":
			wf := v.Message()
			path := substitute(getStringField(wf, "path"), deps)
			content := getBytesField(wf, "content")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				firstErr = fmt.Errorf("write_file mkdir: %w", err)
				return false
			}
			if err := os.WriteFile(path, content, 0o644); err != nil {
				firstErr = fmt.Errorf("write_file %q: %w", path, err)
				return false
			}
		case "remove_files":
			rm := v.Message()
			pathsField := rm.Descriptor().Fields().ByName("paths")
			if pathsField == nil || !pathsField.IsList() {
				return true
			}
			paths := rm.Get(pathsField).List()
			for i := 0; i < paths.Len(); i++ {
				_ = os.Remove(substitute(paths.Get(i).String(), deps))
			}
		}
		return true
	})
	return firstErr
}

func substitute(s string, deps Deps) string {
	s = strings.ReplaceAll(s, "$TEMP_DIR", deps.TempDir)
	for k, v := range deps.Vars {
		s = strings.ReplaceAll(s, "$"+k, v)
	}
	return s
}

func assertExpected(t *testing.T, expected protoreflect.Message, gotMsg proto.Message, gotErr error, deps Deps) {
	t.Helper()
	if expected == nil {
		return
	}
	desc := expected.Descriptor()

	wantStatus := connect.Code(0)
	if f := desc.Fields().ByName("status"); f != nil {
		wantStatus = connect.Code(uint32(expected.Get(f).Uint()))
	}
	if wantStatus != 0 {
		if gotErr == nil {
			t.Fatalf("want code %v, got nil error", wantStatus)
		}
		var ce *connect.Error
		if !errors.As(gotErr, &ce) {
			t.Fatalf("want connect error, got %T: %v", gotErr, gotErr)
		}
		if ce.Code() != wantStatus {
			t.Fatalf("want code %v, got %v: %v", wantStatus, ce.Code(), gotErr)
		}
		if errField := desc.Fields().ByName("error"); errField != nil && errField.Kind() == protoreflect.MessageKind {
			errMsg := expected.Get(errField).Message()
			rx := getStringField(errMsg, "message_regex")
			if rx != "" {
				ok, err := regexp.MatchString(rx, gotErr.Error())
				if err != nil {
					t.Fatalf("invalid message_regex %q: %v", rx, err)
				}
				if !ok {
					t.Fatalf("error message %q does not match %q", gotErr.Error(), rx)
				}
			}
		}
	} else if gotErr != nil {
		t.Fatalf("want OK, got error: %v", gotErr)
	}

	if bodyField := desc.Fields().ByName("body"); bodyField != nil && bodyField.Kind() == protoreflect.MessageKind {
		wantBody := expected.Get(bodyField).Message()
		if hasAnyField(wantBody) && gotMsg != nil {
			if err := assertBodyPartial(wantBody, gotMsg.ProtoReflect()); err != nil {
				t.Fatalf("body mismatch: %v", err)
			}
		}
	}

	if stateField := desc.Fields().ByName("state"); stateField != nil && stateField.IsList() {
		list := expected.Get(stateField).List()
		for i := 0; i < list.Len(); i++ {
			if err := assertStateOne(list.Get(i).Message(), deps); err != nil {
				t.Fatalf("state[%d]: %v", i, err)
			}
		}
	}
}

func hasAnyField(m protoreflect.Message) bool {
	any := false
	m.Range(func(protoreflect.FieldDescriptor, protoreflect.Value) bool {
		any = true
		return false
	})
	return any
}

func assertBodyPartial(want, got protoreflect.Message) error {
	var rangeErr error
	want.Range(func(fd protoreflect.FieldDescriptor, wv protoreflect.Value) bool {
		gv := got.Get(fd)
		if !valueEqual(fd, wv, gv) {
			rangeErr = fmt.Errorf("field %s: want %v, got %v", fd.Name(), wv.Interface(), gv.Interface())
			return false
		}
		return true
	})
	return rangeErr
}

func valueEqual(fd protoreflect.FieldDescriptor, a, b protoreflect.Value) bool {
	if fd.Kind() == protoreflect.MessageKind && !fd.IsList() && !fd.IsMap() {
		return proto.Equal(a.Message().Interface(), b.Message().Interface())
	}
	return a.Interface() == b.Interface()
}

func assertStateOne(s protoreflect.Message, deps Deps) error {
	var firstErr error
	s.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch fd.Name() {
		case "file_exists":
			path := substitute(getStringField(v.Message(), "path"), deps)
			if _, err := os.Stat(path); err != nil {
				firstErr = fmt.Errorf("file_exists %q: %w", path, err)
				return false
			}
		case "file_absent":
			path := substitute(getStringField(v.Message(), "path"), deps)
			if _, err := os.Stat(path); err == nil {
				firstErr = fmt.Errorf("file_absent %q: file exists", path)
				return false
			}
		case "file_contains":
			fc := v.Message()
			path := substitute(getStringField(fc, "path"), deps)
			data, err := os.ReadFile(path)
			if err != nil {
				firstErr = fmt.Errorf("file_contains read %q: %w", path, err)
				return false
			}
			text := string(data)
			containsField := fc.Descriptor().Fields().ByName("contains")
			if containsField == nil || !containsField.IsList() {
				return true
			}
			list := fc.Get(containsField).List()
			for i := 0; i < list.Len(); i++ {
				want := list.Get(i).String()
				if !strings.Contains(text, want) {
					firstErr = fmt.Errorf("file_contains %q missing %q", path, want)
					return false
				}
			}
		}
		return true
	})
	return firstErr
}

func getStringField(m protoreflect.Message, name string) string {
	f := m.Descriptor().Fields().ByName(protoreflect.Name(name))
	if f == nil {
		return ""
	}
	return m.Get(f).String()
}

func getBytesField(m protoreflect.Message, name string) []byte {
	f := m.Descriptor().Fields().ByName(protoreflect.Name(name))
	if f == nil {
		return nil
	}
	return m.Get(f).Bytes()
}
