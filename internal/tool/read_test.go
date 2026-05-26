package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/workspace"
)

func TestReadToolPathRequestsDeclaresReadAccessForEachPath(t *testing.T) {
	requests, err := NewReadTool().PathRequests(json.RawMessage(`{"paths":["app.go","README.md"]}`))
	if err != nil {
		t.Fatalf("PathRequests() error = %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %#v", requests)
	}
	if requests[0].Access != workspace.AccessRead || requests[0].Path != "app.go" {
		t.Fatalf("requests[0] = %#v", requests[0])
	}
	if requests[1].Access != workspace.AccessRead || requests[1].Path != "README.md" {
		t.Fatalf("requests[1] = %#v", requests[1])
	}
}

func TestReadToolPathRequestsAcceptsSinglePath(t *testing.T) {
	requests, err := NewReadTool().PathRequests(json.RawMessage(`{"path":"app.go"}`))
	if err != nil {
		t.Fatalf("PathRequests() error = %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("requests = %#v", requests)
	}
	if requests[0].Access != workspace.AccessRead || requests[0].Path != "app.go" {
		t.Fatalf("requests[0] = %#v", requests[0])
	}
}

func TestReadToolDefinitionSupportsPathOrPaths(t *testing.T) {
	definition := NewReadTool().Definition()
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(definition.InputSchema, &schema); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(schema.Properties) != 6 {
		t.Fatalf("properties = %#v", schema.Properties)
	}
	if len(schema.Required) != 0 {
		t.Fatalf("required = %#v, want no schema-required fields", schema.Required)
	}
	for _, property := range []string{"path", "paths"} {
		if _, ok := schema.Properties[property]; !ok {
			t.Fatalf("schema missing %q property: %#v", property, schema.Properties)
		}
	}
	for _, want := range []string{"Send either `path` or `paths`, not both", "Omit `offset` and `limit` for normal reads", "known ranges, large files"} {
		if !strings.Contains(definition.Description, want) {
			t.Fatalf("description missing %q: %q", want, definition.Description)
		}
	}
	if !strings.Contains(definition.ProviderDescription, "Send either `path` or `paths`, not both") ||
		!strings.Contains(definition.ProviderDescription, "Omit `offset` and `limit` for normal reads") ||
		!strings.Contains(definition.ProviderDescription, "known ranges, large files") {
		t.Fatalf("provider description missing read guidance: %q", definition.ProviderDescription)
	}
	var providerSchema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(definition.ProviderInputSchema, &providerSchema); err != nil {
		t.Fatalf("ProviderInputSchema unmarshal error = %v", err)
	}
	for _, hidden := range []string{"anchor", "after_line", "start_line", "end_line", "max_lines"} {
		if _, ok := providerSchema.Properties[hidden]; ok {
			t.Fatalf("provider schema should not expose %q: %#v", hidden, providerSchema.Properties)
		}
	}
	for _, exposed := range []string{"path", "paths", "offset", "limit"} {
		if _, ok := providerSchema.Properties[exposed]; !ok {
			t.Fatalf("provider schema missing %q: %#v", exposed, providerSchema.Properties)
		}
	}
	var limitSchema struct {
		Default int `json:"default"`
	}
	if err := json.Unmarshal(providerSchema.Properties["limit"], &limitSchema); err != nil {
		t.Fatalf("limit schema unmarshal error = %v", err)
	}
	if limitSchema.Default != DefaultReadLimit {
		t.Fatalf("limit default = %d, want %d", limitSchema.Default, DefaultReadLimit)
	}
}

func TestReadToolExecuteAcceptsNullOptionalWindow(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewReadTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"paths":["app.go"],"offset":null,"limit":null}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != expectedSingleLineRead("package main") {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestReadToolExecuteAcceptsStringNullOptionalWindow(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewReadTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"paths":["app.go"],"offset":"null","limit":"null"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != expectedSingleLineRead("package main") {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestReadToolExecuteReadsAuthorizedFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewReadTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"paths":["app.go"]}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != expectedSingleLineRead("package main") {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestReadToolExecuteReadsEmptyFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "empty.ts"), nil, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewReadTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"paths":["empty.ts"]}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Error != "" {
		t.Fatalf("Error = %q", result.Error)
	}
	if result.Output != expectedTaggedRead("empty.ts", "(empty file)") {
		t.Fatalf("Output = %q", result.Output)
	}
}

func TestRenderReadPathWithTransientRetryRetriesEOF(t *testing.T) {
	attempts := 0
	sleeps := 0
	result, err := renderReadPathWithTransientRetryFunc(
		func(resolvedPath, displayPath string, startLine, limit int) (readResult, error) {
			attempts++
			if attempts < 3 {
				return readResult{}, io.EOF
			}
			return readResult{path: displayPath, body: "ok"}, nil
		},
		func(time.Duration) { sleeps++ },
		"/workspace/app.go",
		"app.go",
		1,
		100,
	)
	if err != nil {
		t.Fatalf("renderReadPathWithTransientRetryFunc() error = %v", err)
	}
	if attempts != 3 || sleeps != 2 {
		t.Fatalf("attempts = %d, sleeps = %d", attempts, sleeps)
	}
	if result.body != "ok" {
		t.Fatalf("result = %#v", result)
	}
}

func TestRenderReadPathWithTransientRetryReportsActionableEOF(t *testing.T) {
	attempts := 0
	_, err := renderReadPathWithTransientRetryFunc(
		func(resolvedPath, displayPath string, startLine, limit int) (readResult, error) {
			attempts++
			return readResult{}, io.EOF
		},
		nil,
		"/workspace/app.go",
		"app.go",
		1,
		100,
	)
	if err == nil {
		t.Fatalf("renderReadPathWithTransientRetryFunc() error = nil")
	}
	for _, want := range []string{
		"file changed while reading",
		"hit EOF after 3 attempts",
		"truncated or replaced",
		"retry the read after the write finishes",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d", attempts)
	}
}

func TestReadToolExecuteRecordsObservedVersionThatMatchesHelper(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewReadTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"paths":["app.go"]}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(result.ObservedResources) != 1 {
		t.Fatalf("observed resources = %#v", result.ObservedResources)
	}
	decision, err := scope.Check(workspace.AccessRead, "app.go")
	if err != nil {
		t.Fatalf("scope.Check() error = %v", err)
	}
	if result.ObservedResources[0].Kind != ObservedResourceFileContent || result.ObservedResources[0].Path != decision.ResolvedPath {
		t.Fatalf("observed resource = %#v", result.ObservedResources[0])
	}

	version, err := ReadObservedVersion(decision.ResolvedPath)
	if err != nil {
		t.Fatalf("ReadObservedVersion() error = %v", err)
	}
	if result.ObservedResources[0].Version != version {
		t.Fatalf("observed version = %q, want %q", result.ObservedResources[0].Version, version)
	}
	if strings.TrimSpace(result.ObservedResources[0].State) == "" {
		t.Fatalf("observed state = %#v", result.ObservedResources[0])
	}
	if !result.ObservedResources[0].Complete || result.ObservedResources[0].StartLine != 1 || result.ObservedResources[0].EndLine != 1 || result.ObservedResources[0].TotalLines != 1 {
		t.Fatalf("observed coverage = %#v", result.ObservedResources[0])
	}
}

func TestReadToolExecuteReadsMultipleFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app.go) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hello docs\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md) error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewReadTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"paths":["app.go","README.md"]}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := strings.Join([]string{
		"=== app.go ===",
		expectedSingleLineRead("package main"),
		"",
		"=== README.md ===",
		expectedSingleLineReadPath("README.md", "hello docs"),
	}, "\n")
	if result.Output != want {
		t.Fatalf("output = %q, want %q", result.Output, want)
	}
}

func TestReadToolExecuteReportsPartialMultiFileFailureInOutput(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app.go) error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewReadTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"paths":["app.go","missing.go"]}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Error != "" {
		t.Fatalf("Error = %q, want partial failure in output only", result.Error)
	}
	for _, want := range []string{
		"Partial read: 1 of 2 requested files read successfully.",
		"Failed path:",
		"- missing.go:",
		"If needed, correct or replace only the failed path.",
		"=== app.go ===",
		"1: package main",
	} {
		if !strings.Contains(result.Output, want) {
			t.Fatalf("Output missing %q:\n%s", want, result.Output)
		}
	}
	if strings.Contains(result.Output, "read failed for 1 path") {
		t.Fatalf("Output uses all-failed wording:\n%s", result.Output)
	}
	if len(result.ObservedResources) != 1 || !strings.HasSuffix(result.ObservedResources[0].Path, "/app.go") {
		t.Fatalf("ObservedResources = %#v", result.ObservedResources)
	}
}

func TestReadToolExecuteDefaultsToFullContextLimit(t *testing.T) {
	root := t.TempDir()
	content := numberedLines(1250)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewReadTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"paths":["notes.txt"]}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "1: line-001") {
		t.Fatalf("output missing first line: %q", result.Output)
	}
	if strings.Contains(result.Output, "1001: line-1001") {
		t.Fatalf("output exceeded default limit: %q", result.Output)
	}
	if !strings.Contains(result.Output, "(showing lines 1-1000 of 1250. Use offset=1000 (0-based) to continue.)") {
		t.Fatalf("output missing offset continuation footer: %q", result.Output)
	}
	if len(result.ObservedResources) != 1 || result.ObservedResources[0].Complete {
		t.Fatalf("observed resources = %#v, want partial default coverage", result.ObservedResources)
	}
}

func TestReadToolExecuteAllowsExplicitLimitBeyondDefault(t *testing.T) {
	root := t.TempDir()
	content := numberedLines(250)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewReadTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"paths":["notes.txt"],"limit":250}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "250: line-250") {
		t.Fatalf("output missing final line: %q", result.Output)
	}
	if !strings.Contains(result.Output, "(End of file - total 250 lines; shown lines 1-250)") {
		t.Fatalf("output missing full-file footer: %q", result.Output)
	}
	if len(result.ObservedResources) != 1 || !result.ObservedResources[0].Complete {
		t.Fatalf("observed resources = %#v, want complete single-file coverage", result.ObservedResources)
	}
}

func TestReadToolExecuteReadsOffsetWindow(t *testing.T) {
	root := t.TempDir()
	content := "one\ntwo\nthree\nfour\n"
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewReadTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"paths":["notes.txt"],"offset":1,"limit":2}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := expectedTaggedRead("notes.txt", "2: two\n3: three\n(showing lines 2-3 of 4. Use offset=3 (0-based) to continue.)")
	if result.Output != want {
		t.Fatalf("output = %q, want %q", result.Output, want)
	}
	if len(result.ObservedResources) != 1 {
		t.Fatalf("observed resources = %#v", result.ObservedResources)
	}
	if result.ObservedResources[0].Complete {
		t.Fatalf("observed coverage = %#v, want partial coverage", result.ObservedResources[0])
	}
	if result.ObservedResources[0].StartLine != 2 || result.ObservedResources[0].EndLine != 3 || result.ObservedResources[0].TotalLines != 4 {
		t.Fatalf("observed coverage = %#v", result.ObservedResources[0])
	}
}

func TestReadToolExecuteContinuesWithOffset(t *testing.T) {
	root := t.TempDir()
	content := "one\ntwo\nthree\nfour\n"
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewReadTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"paths":["notes.txt"],"offset":2,"limit":2}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := expectedTaggedRead("notes.txt", "3: three\n4: four\n(End of file - total 4 lines; shown lines 3-4)")
	if result.Output != want {
		t.Fatalf("output = %q, want %q", result.Output, want)
	}
	if len(result.ObservedResources) != 1 {
		t.Fatalf("observed resources = %#v", result.ObservedResources)
	}
	if result.ObservedResources[0].StartLine != 3 || result.ObservedResources[0].EndLine != 4 || result.ObservedResources[0].TotalLines != 4 {
		t.Fatalf("observed coverage = %#v", result.ObservedResources[0])
	}
}

func TestReadToolExecuteReadsLargeOffsetWindowWithCurrentPaddingContract(t *testing.T) {
	root := t.TempDir()
	content := numberedLines(150)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewReadTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"paths":["notes.txt"],"offset":94,"limit":11}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	lines := make([]string, 0, 12)
	for lineNumber := 95; lineNumber <= 105; lineNumber++ {
		lines = append(lines, fmt.Sprintf("%3d: line-%03d", lineNumber, lineNumber))
	}
	lines = append(lines, "(showing lines 95-105 of 150. Use offset=105 (0-based) to continue.)")
	want := expectedTaggedRead("notes.txt", strings.Join(lines, "\n"))
	if result.Output != want {
		t.Fatalf("output = %q, want %q", result.Output, want)
	}
}

func TestRenderReadTextFileKeepsOffsetFooterContract(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte(wideNumberedLines(20, 40)), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}

	result, err := renderReadTextFile(path, "notes.txt", 10, 6)
	if err != nil {
		t.Fatalf("renderReadTextFile() error = %v", err)
	}
	want := strings.Join([]string{
		fmt.Sprintf("10: line-010 %s", strings.Repeat("x", 40)),
		fmt.Sprintf("11: line-011 %s", strings.Repeat("x", 40)),
		fmt.Sprintf("12: line-012 %s", strings.Repeat("x", 40)),
		fmt.Sprintf("13: line-013 %s", strings.Repeat("x", 40)),
		fmt.Sprintf("14: line-014 %s", strings.Repeat("x", 40)),
		fmt.Sprintf("15: line-015 %s", strings.Repeat("x", 40)),
		"(showing lines 10-15 of 20. Use offset=15 (0-based) to continue.)",
	}, "\n")
	if result.body != want {
		t.Fatalf("body = %q, want %q", result.body, want)
	}
}

func TestReadToolExecuteAcceptsStringifiedWindowFromSessionPayload(t *testing.T) {
	root := t.TempDir()
	content := "one\ntwo\nthree\nfour\n"
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewReadTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"paths":["notes.txt"],"offset":"1","limit":"2"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := expectedTaggedRead("notes.txt", "2: two\n3: three\n(showing lines 2-3 of 4. Use offset=3 (0-based) to continue.)")
	if result.Output != want {
		t.Fatalf("output = %q, want %q", result.Output, want)
	}
}

func TestReadToolExecuteAcceptsLineStartEndAliases(t *testing.T) {
	root := t.TempDir()
	content := "one\ntwo\nthree\nfour\n"
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewReadTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"path":"notes.txt","line_start":2,"line_end":3}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := expectedTaggedRead("notes.txt", "2: two\n3: three\n(showing lines 2-3 of 4. Use offset=3 (0-based) to continue.)")
	if result.Output != want {
		t.Fatalf("output = %q, want %q", result.Output, want)
	}
}

func TestReadToolExecuteAcceptsCaseInsensitiveLineEndAlias(t *testing.T) {
	root := t.TempDir()
	content := "one\ntwo\nthree\nfour\n"
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewReadTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"path":"notes.txt","line_start":2,"Line_end":3}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := expectedTaggedRead("notes.txt", "2: two\n3: three\n(showing lines 2-3 of 4. Use offset=3 (0-based) to continue.)")
	if result.Output != want {
		t.Fatalf("output = %q, want %q", result.Output, want)
	}
}

func TestReadToolExecuteRejectsInvalidWindow(t *testing.T) {
	if _, err := NewReadTool().Execute(context.Background(), ExecutionContext{}, json.RawMessage(`{"paths":["notes.txt"],"offset":-1}`)); !errors.Is(err, ErrReadOffsetInvalid) {
		t.Fatalf("Execute(offset) error = %v, want ErrReadOffsetInvalid", err)
	}
	if _, err := NewReadTool().Execute(context.Background(), ExecutionContext{}, json.RawMessage(`{"paths":["notes.txt"],"limit":0}`)); !errors.Is(err, ErrReadLimitInvalid) {
		t.Fatalf("Execute(limit) error = %v, want ErrReadLimitInvalid", err)
	}
}

func TestReadToolExecuteRejectsRemovedRangeFields(t *testing.T) {
	_, err := NewReadTool().Execute(context.Background(), ExecutionContext{}, json.RawMessage(`{"paths":["notes.txt"],"start_line":1}`))
	if !errors.Is(err, ErrInvalidArguments) || !strings.Contains(err.Error(), `unknown field "start_line"`) {
		t.Fatalf("Execute() error = %v, want unknown start_line", err)
	}
}

func TestReadToolNormalizedInputKeyNormalizesDefaultWindow(t *testing.T) {
	tl := NewReadTool()

	implicit, err := tl.NormalizedInputKey(json.RawMessage(`{"paths":["app.go"]}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(implicit) error = %v", err)
	}
	explicit, err := tl.NormalizedInputKey(json.RawMessage(`{"paths":["app.go"],"offset":0,"limit":1000}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(explicit) error = %v", err)
	}
	if implicit != explicit {
		t.Fatalf("input keys differ:\nimplicit=%s\nexplicit=%s", implicit, explicit)
	}
}

func TestReadToolNormalizedInputKeyNormalizesLineStartEndAliases(t *testing.T) {
	tl := NewReadTool()

	alias, err := tl.NormalizedInputKey(json.RawMessage(`{"path":"app.go","line_start":2,"line_end":4}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(alias) error = %v", err)
	}
	canonical, err := tl.NormalizedInputKey(json.RawMessage(`{"path":"app.go","offset":1,"limit":3}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(canonical) error = %v", err)
	}
	if alias != canonical {
		t.Fatalf("input keys differ:\nalias=%s\ncanonical=%s", alias, canonical)
	}
}

func TestReadToolNormalizedInputKeyPreservesExplicitWindowLargerThanDefault(t *testing.T) {
	tl := NewReadTool()

	implicit, err := tl.NormalizedInputKey(json.RawMessage(`{"paths":["app.go"]}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(implicit) error = %v", err)
	}
	larger, err := tl.NormalizedInputKey(json.RawMessage(`{"paths":["app.go"],"limit":10000}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(larger) error = %v", err)
	}
	if implicit == larger {
		t.Fatalf("input keys unexpectedly matched:\nimplicit=%s\nlarger=%s", implicit, larger)
	}
}

func TestReadToolNormalizedInputKeyIncludesOffset(t *testing.T) {
	tl := NewReadTool()

	first, err := tl.NormalizedInputKey(json.RawMessage(`{"paths":["app.go"],"offset":200}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(first) error = %v", err)
	}
	second, err := tl.NormalizedInputKey(json.RawMessage(`{"paths":["app.go"],"offset":250}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(second) error = %v", err)
	}
	if first == second {
		t.Fatalf("input keys unexpectedly matched:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestReadToolNormalizedInputKeyNormalizesStringifiedWindowArguments(t *testing.T) {
	tl := NewReadTool()

	integerKey, err := tl.NormalizedInputKey(json.RawMessage(`{"paths":["app.go"],"offset":10,"limit":20}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(integer) error = %v", err)
	}
	stringKey, err := tl.NormalizedInputKey(json.RawMessage(`{"paths":["app.go"],"offset":"10","limit":"20"}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(string) error = %v", err)
	}
	if integerKey != stringKey {
		t.Fatalf("input keys differ:\ninteger=%s\nstring=%s", integerKey, stringKey)
	}
}

func TestReadToolNormalizedInputKeyDistinguishesDefaultAndExplicitBatchWindowArguments(t *testing.T) {
	tl := NewReadTool()

	implicit, err := tl.NormalizedInputKey(json.RawMessage(`{"paths":["a.go","b.go"]}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(implicit) error = %v", err)
	}
	explicit, err := tl.NormalizedInputKey(json.RawMessage(`{"paths":["a.go","b.go"],"limit":2000}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(explicit) error = %v", err)
	}
	if implicit == explicit {
		t.Fatalf("input keys unexpectedly match:\nimplicit=%s\nexplicit=%s", implicit, explicit)
	}
}

func TestReadToolExecuteReadsMultipleFilesWithPerFileCap(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app.go) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("doc1\ndoc2\ndoc3\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md) error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewReadTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"paths":["app.go","README.md"],"limit":2}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := strings.Join([]string{
		"=== app.go ===",
		expectedTaggedRead("app.go", "1: line1\n2: line2\n(showing lines 1-2 of 3. Use offset=2 (0-based) to continue.)"),
		"",
		"=== README.md ===",
		expectedTaggedRead("README.md", "1: doc1\n2: doc2\n(showing lines 1-2 of 3. Use offset=2 (0-based) to continue.)"),
	}, "\n")
	if result.Output != want {
		t.Fatalf("output = %q, want %q", result.Output, want)
	}
}

func TestReadToolExecuteDoesNotCapMultiFileOutputBudget(t *testing.T) {
	root := t.TempDir()
	paths := make([]string, 0, 30)
	for idx := 1; idx <= 30; idx++ {
		name := fmt.Sprintf("file-%d.txt", idx)
		paths = append(paths, name)
		if err := os.WriteFile(filepath.Join(root, name), []byte(wideNumberedLines(500, 200)), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	input, err := json.Marshal(map[string]any{"paths": paths})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	result, err := NewReadTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.Contains(result.Output, "Output was capped to protect context budget") {
		t.Fatalf("output was capped; output bytes = %d", len(result.Output))
	}
	if strings.Count(result.Output, "=== file-") != len(paths) {
		t.Fatalf("output did not render every file; output bytes = %d", len(result.Output))
	}
}

func TestReadToolExecuteAcceptsSingularPathInput(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewReadTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"path":"app.go"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != expectedSingleLineRead("package main") {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestReadToolExecuteRejectsPathAndPathsTogether(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app.go) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hello docs\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md) error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	_, err = NewReadTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"path":"app.go","paths":["README.md"]}`))
	if !errors.Is(err, ErrReadPathConflict) {
		t.Fatalf("Execute() error = %v, want ErrReadPathConflict", err)
	}
}

func TestReadToolExecuteAcceptsBareStringPathsAsSingleton(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewReadTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"paths":"app.go"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != expectedSingleLineRead("package main") {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestReadToolExecuteAcceptsStringifiedJSONArrayPaths(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "README.md")
	if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewReadTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"paths":"[\"README.md\"]"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != expectedSingleLineReadPath("README.md", "hello") {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestReadToolExecuteReadsExternalPath(t *testing.T) {
	scope, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	external := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(external, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	result, err := NewReadTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"paths":["`+external+`"]}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != expectedSingleLineReadPath(external, "outside") {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestReadToolExecuteReadsMultipleFilesWithPerFileWindow(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app.go) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("doc1\ndoc2\ndoc3\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md) error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewReadTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"paths":["app.go","README.md"],"offset":1,"limit":1}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := strings.Join([]string{
		"=== app.go ===",
		expectedTaggedRead("app.go", "2: line2\n(showing lines 2-2 of 3. Use offset=2 (0-based) to continue.)"),
		"",
		"=== README.md ===",
		expectedTaggedRead("README.md", "2: doc2\n(showing lines 2-2 of 3. Use offset=2 (0-based) to continue.)"),
	}, "\n")
	if result.Output != want {
		t.Fatalf("output = %q, want %q", result.Output, want)
	}
}

func expectedSingleLineRead(line string) string {
	return expectedSingleLineReadPath("app.go", line)
}

func expectedSingleLineReadPath(path, line string) string {
	return expectedTaggedRead(path, fmt.Sprintf("1: %s\n(End of file - total 1 lines; shown lines 1-1)", line))
}

func expectedTaggedRead(path, body string) string {
	return fmt.Sprintf("<path>%s</path>\n<type>file</type>\n<content>\n%s\n</content>", path, body)
}

func numberedLines(count int) string {
	lines := make([]string, 0, count)
	for idx := 1; idx <= count; idx++ {
		lines = append(lines, fmt.Sprintf("line-%03d", idx))
	}
	return strings.Join(lines, "\n") + "\n"
}

func wideNumberedLines(count, width int) string {
	lines := make([]string, 0, count)
	for idx := 1; idx <= count; idx++ {
		lines = append(lines, fmt.Sprintf("line-%03d %s", idx, strings.Repeat("x", width)))
	}
	return strings.Join(lines, "\n") + "\n"
}
