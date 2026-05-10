// Package testrunner loads .textpb TestSuite files and runs them as Go subtests.
//
// The runner is generic via proto reflection — it does not depend on any
// specific schema. Callers provide, per RPC method:
//   - a SuiteFactory returning an empty per-RPC TestSuite proto.Message
//     whose shape is: cases (repeated TestCase), and each TestCase has
//     name (string), setup (repeated SetupStep), request (typed),
//     expected (Expected with status string + body + error + state).
//   - an Invoker that calls the handler with the typed request.
//
// Case files live at <CasesRoot>/<Service>/<Method>/*.textpb. Sandbox-relative
// paths in setup/state assertions are resolved against a per-case tempdir.
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

type Deps struct {
	TempDir string
}

type Invoker func(ctx context.Context, deps Deps, req proto.Message) (proto.Message, error)

type SuiteFactory func() proto.Message

type MethodBinding struct {
	Suite   SuiteFactory
	Invoker Invoker
}

type Config struct {
	CasesRoot string
	Service   string
	Methods   map[string]MethodBinding
}

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
				t.Logf("%s/%s: no binding registered; case file not parsed", cfg.Service, method)
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
			t.Run(name, func(t *testing.T) { runOne(t, c, binding.Invoker) })
		}
	}
}

type oneCase struct {
	name     string
	setup    []protoreflect.Message
	request  proto.Message
	expected protoreflect.Message
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
		out = append(out, parseCase(list.Get(i).Message()))
	}
	return out, nil
}

func parseCase(m protoreflect.Message) oneCase {
	desc := m.Descriptor()
	c := oneCase{}
	if f := desc.Fields().ByName("name"); f != nil {
		c.name = m.Get(f).String()
	}
	if f := desc.Fields().ByName("setup"); f != nil && f.IsList() {
		list := m.Get(f).List()
		for i := 0; i < list.Len(); i++ {
			c.setup = append(c.setup, list.Get(i).Message())
		}
	}
	if f := desc.Fields().ByName("request"); f != nil && f.Kind() == protoreflect.MessageKind {
		c.request = m.Get(f).Message().Interface()
	}
	if f := desc.Fields().ByName("expected"); f != nil && f.Kind() == protoreflect.MessageKind {
		c.expected = m.Get(f).Message()
	}
	return c
}

func runOne(t *testing.T, c oneCase, invoker Invoker) {
	t.Helper()
	deps := Deps{TempDir: t.TempDir()}

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

// applySetup interprets SetupStep oneof variants by field name. Unknown
// kinds are ignored.
func applySetup(deps Deps, steps []protoreflect.Message) error {
	for _, s := range steps {
		var firstErr error
		s.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
			switch fd.Name() {
			case "write_file":
				wf := v.Message()
				path := resolvePath(getStringField(wf, "path"), deps)
				content := getBytesField(wf, "content")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					firstErr = fmt.Errorf("write_file mkdir: %w", err)
					return false
				}
				if err := os.WriteFile(path, content, 0o644); err != nil {
					firstErr = fmt.Errorf("write_file %q: %w", path, err)
					return false
				}
			case "clear_dir":
				path := resolvePath(getStringField(v.Message(), "path"), deps)
				if err := os.RemoveAll(path); err != nil {
					firstErr = fmt.Errorf("clear_dir %q: %w", path, err)
					return false
				}
				if err := os.MkdirAll(path, 0o755); err != nil {
					firstErr = fmt.Errorf("clear_dir mkdir %q: %w", path, err)
					return false
				}
			}
			return true
		})
		if firstErr != nil {
			return firstErr
		}
	}
	return nil
}

// resolvePath joins a sandbox-relative path against the per-case tempdir.
// Absolute paths are passed through.
func resolvePath(p string, deps Deps) string {
	if p == "" {
		return deps.TempDir
	}
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(deps.TempDir, p)
}

func assertExpected(t *testing.T, expected protoreflect.Message, gotMsg proto.Message, gotErr error, deps Deps) {
	t.Helper()
	if expected == nil {
		return
	}
	desc := expected.Descriptor()

	wantStatus := connect.Code(0)
	if f := desc.Fields().ByName("status"); f != nil {
		statusStr := strings.TrimSpace(expected.Get(f).String())
		code, err := codeFromString(statusStr)
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		wantStatus = code
	}
	if wantStatus != 0 {
		if gotErr == nil {
			t.Fatalf("want code %s, got nil error", wantStatus)
		}
		var ce *connect.Error
		if !errors.As(gotErr, &ce) {
			t.Fatalf("want connect error, got %T: %v", gotErr, gotErr)
		}
		if ce.Code() != wantStatus {
			t.Fatalf("want code %s, got %s: %v", wantStatus, ce.Code(), gotErr)
		}
		if errField := desc.Fields().ByName("error"); errField != nil && errField.Kind() == protoreflect.MessageKind {
			rx := getStringField(expected.Get(errField).Message(), "message_regex")
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

// codeFromString translates the human-readable status string used in
// .textpb files into a Connect code. "" and "OK" both mean success.
func codeFromString(s string) (connect.Code, error) {
	if s == "" {
		return 0, nil
	}
	upper := strings.ToUpper(s)
	switch upper {
	case "OK":
		return 0, nil
	case "CANCELED", "CANCELLED":
		return connect.CodeCanceled, nil
	case "UNKNOWN":
		return connect.CodeUnknown, nil
	case "INVALID_ARGUMENT", "INVALIDARGUMENT":
		return connect.CodeInvalidArgument, nil
	case "DEADLINE_EXCEEDED", "DEADLINEEXCEEDED":
		return connect.CodeDeadlineExceeded, nil
	case "NOT_FOUND", "NOTFOUND":
		return connect.CodeNotFound, nil
	case "ALREADY_EXISTS", "ALREADYEXISTS":
		return connect.CodeAlreadyExists, nil
	case "PERMISSION_DENIED", "PERMISSIONDENIED":
		return connect.CodePermissionDenied, nil
	case "RESOURCE_EXHAUSTED", "RESOURCEEXHAUSTED":
		return connect.CodeResourceExhausted, nil
	case "FAILED_PRECONDITION", "FAILEDPRECONDITION":
		return connect.CodeFailedPrecondition, nil
	case "ABORTED":
		return connect.CodeAborted, nil
	case "OUT_OF_RANGE", "OUTOFRANGE":
		return connect.CodeOutOfRange, nil
	case "UNIMPLEMENTED":
		return connect.CodeUnimplemented, nil
	case "INTERNAL":
		return connect.CodeInternal, nil
	case "UNAVAILABLE":
		return connect.CodeUnavailable, nil
	case "DATA_LOSS", "DATALOSS":
		return connect.CodeDataLoss, nil
	case "UNAUTHENTICATED":
		return connect.CodeUnauthenticated, nil
	}
	return 0, fmt.Errorf("unknown status %q", s)
}

func hasAnyField(m protoreflect.Message) bool {
	any := false
	m.Range(func(protoreflect.FieldDescriptor, protoreflect.Value) bool { any = true; return false })
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
			path := resolvePath(getStringField(v.Message(), "path"), deps)
			if _, err := os.Stat(path); err != nil {
				firstErr = fmt.Errorf("file_exists %q: %w", path, err)
				return false
			}
		case "file_absent":
			path := resolvePath(getStringField(v.Message(), "path"), deps)
			if _, err := os.Stat(path); err == nil {
				firstErr = fmt.Errorf("file_absent %q: file exists", path)
				return false
			}
		case "file_contains":
			fc := v.Message()
			path := resolvePath(getStringField(fc, "path"), deps)
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
