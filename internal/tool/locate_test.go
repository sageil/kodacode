package tool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/workspace"
)

func TestLocateToolPathRequestsDeclaresListAccess(t *testing.T) {
	requests, err := NewLocateTool().PathRequests(json.RawMessage(`{"query":"*.go","path":".","include_hidden":false,"max_matches":10}`))
	if err != nil {
		t.Fatalf("PathRequests() error = %v", err)
	}
	if len(requests) != 1 || requests[0].Access != workspace.AccessList || requests[0].Path != "." {
		t.Fatalf("requests = %#v", requests)
	}
}

func TestLocateToolDefinitionRequiresOnlyPath(t *testing.T) {
	definition := NewLocateTool().Definition()
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
	if !slices.Contains(schema.Required, "path") {
		t.Fatalf("required = %#v, missing path", schema.Required)
	}
	for _, field := range []string{"query", "include_hidden", "max_matches", "limit", "max_results"} {
		if slices.Contains(schema.Required, field) {
			t.Fatalf("required = %#v, should omit %q", schema.Required, field)
		}
	}
}

func TestLocateToolDefinitionMentionsBoundedNonPageableContract(t *testing.T) {
	definition := NewLocateTool().Definition()
	for _, needle := range []string{
		"bounded and non-pageable",
		"narrow `query` or `path`",
	} {
		if !strings.Contains(definition.Description, needle) {
			t.Fatalf("description = %q, missing %q", definition.Description, needle)
		}
	}
	for _, needle := range []string{
		"bounded and non-pageable",
		"narrow `query` or `path`",
	} {
		if !strings.Contains(definition.ProviderDescription, needle) {
			t.Fatalf("provider description = %q, missing %q", definition.ProviderDescription, needle)
		}
	}
	if !strings.Contains(string(definition.InputSchema), "There is no pagination or continuation token") {
		t.Fatalf("input schema = %s", string(definition.InputSchema))
	}
}

func TestLocateToolNormalizedInputKeyNormalizesDefaults(t *testing.T) {
	tool := NewLocateTool()
	implicit, err := tool.NormalizedInputKey(json.RawMessage(`{"path":".","query":"*.go"}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(implicit) error = %v", err)
	}
	explicit, err := tool.NormalizedInputKey(json.RawMessage(`{"query":"*.go","path":".","include_hidden":false,"max_matches":200}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(explicit) error = %v", err)
	}
	if implicit != explicit {
		t.Fatalf("input keys differ:\nimplicit=%s\nexplicit=%s", implicit, explicit)
	}
}

func TestLocateToolNormalizedInputKeyNormalizesCaseInsensitiveStringArgs(t *testing.T) {
	tool := NewLocateTool()
	stringKey, err := tool.NormalizedInputKey(json.RawMessage(`{"query":"*.go","path":".","include_hidden":"False","max_matches":"20"}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(stringKey) error = %v", err)
	}
	canonicalKey, err := tool.NormalizedInputKey(json.RawMessage(`{"query":"*.go","path":".","include_hidden":false,"max_matches":20}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(canonicalKey) error = %v", err)
	}
	if stringKey != canonicalKey {
		t.Fatalf("input keys differ:\nstring=%s\ncanonical=%s", stringKey, canonicalKey)
	}
}

func TestLocateToolNormalizedInputKeyNormalizesLimitAliases(t *testing.T) {
	tool := NewLocateTool()
	limitKey, err := tool.NormalizedInputKey(json.RawMessage(`{"query":"*.go","path":".","limit":20}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(limit) error = %v", err)
	}
	maxResultsKey, err := tool.NormalizedInputKey(json.RawMessage(`{"query":"*.go","path":".","max_results":20}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(max_results) error = %v", err)
	}
	canonicalKey, err := tool.NormalizedInputKey(json.RawMessage(`{"query":"*.go","path":".","max_matches":20}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(canonical) error = %v", err)
	}
	if limitKey != canonicalKey || maxResultsKey != canonicalKey {
		t.Fatalf("input keys differ:\nlimit=%s\nmax_results=%s\ncanonical=%s", limitKey, maxResultsKey, canonicalKey)
	}
}

func TestLocateToolExecuteFindsSubstringAndGlobMatches(t *testing.T) {
	root := t.TempDir()
	mustWriteLocateFile(t, filepath.Join(root, "src", "controllers", "ProjectController.ts"), "export {}")
	mustWriteLocateFile(t, filepath.Join(root, "src", "models", "Project.ts"), "export {}")

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewLocateTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"controllers","path":".","include_hidden":false,"max_matches":10}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "src/controllers/") {
		t.Fatalf("output = %q", result.Output)
	}

	result, err = NewLocateTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"*.ts","path":"src","include_hidden":false,"max_matches":10}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, needle := range []string{"src/controllers/ProjectController.ts", "src/models/Project.ts"} {
		if !strings.Contains(result.Output, needle) {
			t.Fatalf("output = %q, missing %q", result.Output, needle)
		}
	}
}

func TestLocateToolExecuteListsPathWhenQueryEmptyOrOmitted(t *testing.T) {
	root := t.TempDir()
	mustWriteLocateFile(t, filepath.Join(root, "src", "controllers", "ProjectController.ts"), "export {}")
	mustWriteLocateFile(t, filepath.Join(root, "src", "models", "Project.ts"), "export {}")

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	for _, args := range []json.RawMessage{
		json.RawMessage(`{"query":"","path":"src","include_hidden":false,"max_matches":10}`),
		json.RawMessage(`{"path":"src","include_hidden":false,"max_matches":10}`),
		json.RawMessage(`{"query":null,"path":"src","include_hidden":false,"max_matches":10}`),
	} {
		result, err := NewLocateTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, args)
		if err != nil {
			t.Fatalf("Execute(%s) error = %v", args, err)
		}
		for _, needle := range []string{"src/controllers/", "src/controllers/ProjectController.ts", "src/models/", "src/models/Project.ts"} {
			if !strings.Contains(result.Output, needle) {
				t.Fatalf("Execute(%s) output = %q, missing %q", args, result.Output, needle)
			}
		}
	}
}

func TestLocateToolExecuteRespectsIncludeHidden(t *testing.T) {
	root := t.TempDir()
	mustWriteLocateFile(t, filepath.Join(root, ".hidden", "secret.txt"), "x")

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewLocateTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"secret","path":".","include_hidden":false,"max_matches":10}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.Contains(result.Output, "secret.txt") {
		t.Fatalf("output = %q, hidden file should be excluded", result.Output)
	}

	result, err = NewLocateTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"secret","path":".","include_hidden":true,"max_matches":10}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, ".hidden/secret.txt") {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestLocateToolExecuteAcceptsCaseInsensitiveStringArgs(t *testing.T) {
	root := t.TempDir()
	mustWriteLocateFile(t, filepath.Join(root, ".hidden", "secret.txt"), "x")

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewLocateTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"secret","path":".","include_hidden":"TRUE","max_matches":"20"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, ".hidden/secret.txt") {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestLocateToolExecuteRejectsFileRoot(t *testing.T) {
	root := t.TempDir()
	mustWriteLocateFile(t, filepath.Join(root, "tasks.txt"), "TODO")

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	_, err = NewLocateTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"tasks","path":"tasks.txt","include_hidden":false,"max_matches":10}`))
	if !errors.Is(err, ErrLocatePathNotFolder) {
		t.Fatalf("Execute() error = %v, want ErrLocatePathNotFolder", err)
	}
}

func TestLocateToolExecuteClampsOversizedMaxMatchesAndReportsNotice(t *testing.T) {
	root := t.TempDir()
	mustWriteLocateFile(t, filepath.Join(root, "one.txt"), "1")
	mustWriteLocateFile(t, filepath.Join(root, "two.txt"), "2")

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewLocateTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"*.txt","path":".","include_hidden":false,"max_matches":1000}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "notice: max_matches clamped to 200") {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestLocateToolExecuteReportsTruncationGuidance(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"one.txt", "two.txt", "three.txt"} {
		mustWriteLocateFile(t, filepath.Join(root, name), name)
	}

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewLocateTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"*.txt","path":".","include_hidden":false,"max_matches":2}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, needle := range []string{
		"notice: showing first 2 matches.",
		"raise max_matches up to 200",
	} {
		if !strings.Contains(result.Output, needle) {
			t.Fatalf("output = %q, missing %q", result.Output, needle)
		}
	}
}

func TestLocateToolExecuteRejectsInvalidGlobQuery(t *testing.T) {
	scope, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	_, err = NewLocateTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"[broken","path":".","include_hidden":false,"max_matches":10}`))
	if err == nil || !strings.Contains(err.Error(), "invalid glob pattern") {
		t.Fatalf("Execute() error = %v, want invalid glob pattern", err)
	}
}

func mustWriteLocateFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
