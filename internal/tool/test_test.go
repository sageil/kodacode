package tool

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/workspace"
)

func TestParseTestInputDefaultsTimeoutAndPath(t *testing.T) {
	input, err := parseTestInput(json.RawMessage(`{"command":null,"path":null,"filter":null,"timeout":null}`))
	if err != nil {
		t.Fatalf("parseTestInput() error = %v", err)
	}
	if input.Path != "." {
		t.Fatalf("Path = %q, want \".\"", input.Path)
	}
	if input.TimeoutMS != testDefaultTimeoutMS {
		t.Fatalf("TimeoutMS = %d, want %d", input.TimeoutMS, testDefaultTimeoutMS)
	}
}

func TestParseTestInputAcceptsStringTimeout(t *testing.T) {
	input, err := parseTestInput(json.RawMessage(`{"command":null,"path":null,"filter":null,"timeout":"90000"}`))
	if err != nil {
		t.Fatalf("parseTestInput() error = %v", err)
	}
	if input.TimeoutMS != 90000 {
		t.Fatalf("TimeoutMS = %d, want 90000", input.TimeoutMS)
	}
}

func TestParseTestInputRejectsInvalidTimeout(t *testing.T) {
	_, err := parseTestInput(json.RawMessage(`{"command":null,"path":null,"filter":null,"timeout":0}`))
	if !errors.Is(err, ErrTestTimeoutInvalid) {
		t.Fatalf("parseTestInput() error = %v, want %v", err, ErrTestTimeoutInvalid)
	}
}

func TestParseTestInputRejectsTooSmallTimeout(t *testing.T) {
	_, err := parseTestInput(json.RawMessage(`{"command":null,"path":null,"filter":null,"timeout":600}`))
	if !errors.Is(err, ErrTestTimeoutInvalid) {
		t.Fatalf("parseTestInput() error = %v, want %v", err, ErrTestTimeoutInvalid)
	}
	if !strings.Contains(err.Error(), "90000 means 90 seconds and 600 means 0.6 seconds") {
		t.Fatalf("parseTestInput() error = %q, want milliseconds guidance", err.Error())
	}
}

func TestTestToolExecutionRequestRejectsEmptyPathAsInvalidArguments(t *testing.T) {
	root := t.TempDir()
	_, _, err := NewTestTool().ExecutionRequest(root, json.RawMessage(`{"command":"npx jest -t \"ProjectController\" --runInBand","path":"","filter":null,"timeout":120000}`))
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("errors.Is(err, ErrInvalidArguments) = false, err = %v", err)
	}
	if !errors.Is(err, ErrTestPathRequired) {
		t.Fatalf("errors.Is(err, ErrTestPathRequired) = false, err = %v", err)
	}
	if got := err.Error(); !containsAll(got,
		"`test` failed.",
		`Example: {"command":null,"path":"internal/tool","filter":null,"timeout":90000}.`,
		"path must not be empty",
	) {
		t.Fatalf("err.Error() = %q", got)
	}
}

func TestTestToolExecutionRequestAutoDetectsGoPackageFromFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatalf("MkdirAll(pkg) error = %v", err)
	}
	testFile := filepath.Join(root, "pkg", "service_test.go")
	if err := os.WriteFile(testFile, []byte("package pkg\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(service_test.go) error = %v", err)
	}

	request, ok, err := NewTestTool().ExecutionRequest(root, json.RawMessage(`{"command":null,"path":"pkg/service_test.go","filter":"TestService","timeout":90000}`))
	if err != nil {
		t.Fatalf("ExecutionRequest() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutionRequest() ok = false, want true")
	}
	if request.Kind != TestToolName {
		t.Fatalf("Kind = %q, want %q", request.Kind, TestToolName)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	if request.WorkingDirectory != scope.Root() {
		t.Fatalf("WorkingDirectory = %q, want %q", request.WorkingDirectory, scope.Root())
	}
	if request.TimeoutMS != 90000 {
		t.Fatalf("TimeoutMS = %d, want 90000", request.TimeoutMS)
	}
	wantPreview := "go test ./pkg -run 'TestService'"
	if request.Preview != wantPreview {
		t.Fatalf("Preview = %q, want %q", request.Preview, wantPreview)
	}
}

func TestTestToolExecutionRequestUsesNodeScriptPassThrough(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"scripts":{"test":"vitest run"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile(package.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "pnpm-lock.yaml"), []byte("lockfileVersion: 9\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(pnpm-lock.yaml) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatalf("MkdirAll(src) error = %v", err)
	}
	file := filepath.Join(root, "src", "app.test.ts")
	if err := os.WriteFile(file, []byte("export {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app.test.ts) error = %v", err)
	}

	request, ok, err := NewTestTool().ExecutionRequest(root, json.RawMessage(`{"command":null,"path":"src/app.test.ts","filter":"renders","timeout":null}`))
	if err != nil {
		t.Fatalf("ExecutionRequest() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutionRequest() ok = false, want true")
	}
	if !strings.Contains(request.Preview, "pnpm test -- 'src/app.test.ts' -t 'renders'") {
		t.Fatalf("Preview = %q, want pnpm script pass-through args", request.Preview)
	}
}

func TestTestToolExecutionRequestRejectsWatchCommand(t *testing.T) {
	root := t.TempDir()
	_, _, err := NewTestTool().ExecutionRequest(root, json.RawMessage(`{"command":"vitest --watch","path":null,"filter":null,"timeout":null}`))
	if !errors.Is(err, ErrTestWatchModeUnsupported) {
		t.Fatalf("ExecutionRequest() error = %v, want %v", err, ErrTestWatchModeUnsupported)
	}
}
