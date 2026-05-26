package tool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/provider"
	searchsvc "github.com/sageil/kodacode/internal/search"
	"github.com/sageil/kodacode/internal/workspace"
)

func TestSearchToolPathRequestsDeclaresReadAccess(t *testing.T) {
	requests, err := NewSearchTool().PathRequests(json.RawMessage(`{"query":"TODO","path":".","mode":"lexical","glob":"","case_sensitive":false,"max_matches":10}`))
	if err != nil {
		t.Fatalf("PathRequests() error = %v", err)
	}
	if len(requests) != 1 || requests[0].Access != workspace.AccessRead || requests[0].Path != "." {
		t.Fatalf("requests = %#v", requests)
	}
}

func TestSearchToolPathRequestsRejectsFilesystemRoot(t *testing.T) {
	_, err := NewSearchTool().PathRequests(json.RawMessage(`{"query":"TODO","path":"` + filesystemRootPathForSearchTest(t) + `","mode":"lexical","glob":"","case_sensitive":false,"max_matches":10}`))
	if err == nil {
		t.Fatal("PathRequests() error = nil, want invalid arguments")
	}
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("errors.Is(err, ErrInvalidArguments) = false, err = %v", err)
	}
	if !strings.Contains(err.Error(), `use "." or a workspace-relative path for project-wide search`) {
		t.Fatalf("err.Error() = %q", err.Error())
	}
}

func TestSearchToolDefinitionRequiresOnlyCoreSchemaProperties(t *testing.T) {
	definition := NewSearchTool().Definition()
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(definition.InputSchema, &schema); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(schema.Properties) != 9 {
		t.Fatalf("properties = %#v", schema.Properties)
	}
	for _, field := range []string{"query", "path"} {
		if !slices.Contains(schema.Required, field) {
			t.Fatalf("required = %#v, missing %q", schema.Required, field)
		}
	}
	for _, field := range []string{"mode", "glob", "regex", "case_sensitive", "max_matches", "limit", "max_results"} {
		if slices.Contains(schema.Required, field) {
			t.Fatalf("required = %#v, should omit %q", schema.Required, field)
		}
	}
}

func TestSearchToolNormalizedInputKeyNormalizesDefaults(t *testing.T) {
	tool := NewSearchTool()
	implicit, err := tool.NormalizedInputKey(json.RawMessage(`{"path":".","query":"TODO"}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(implicit) error = %v", err)
	}
	explicit, err := tool.NormalizedInputKey(json.RawMessage(`{"query":"TODO","path":".","mode":"lexical","glob":"","regex":false,"case_sensitive":false,"max_matches":200}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(explicit) error = %v", err)
	}
	if implicit == explicit {
		t.Fatalf("input keys should keep implicit auto mode distinct:\nimplicit=%s\nexplicit=%s", implicit, explicit)
	}
}

func TestSearchToolNormalizedInputKeyNormalizesCaseInsensitiveStringArgs(t *testing.T) {
	tool := NewSearchTool()
	stringKey, err := tool.NormalizedInputKey(json.RawMessage(`{"query":"TODO","path":".","mode":"lexical","glob":"","regex":"FALSE","case_sensitive":"False","max_matches":"50"}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(stringKey) error = %v", err)
	}
	canonicalKey, err := tool.NormalizedInputKey(json.RawMessage(`{"query":"TODO","path":".","mode":"lexical","glob":"","regex":false,"case_sensitive":false,"max_matches":50}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(canonicalKey) error = %v", err)
	}
	if stringKey != canonicalKey {
		t.Fatalf("input keys differ:\nstring=%s\ncanonical=%s", stringKey, canonicalKey)
	}
}

func TestSearchToolNormalizedInputKeyNormalizesLimitAliases(t *testing.T) {
	tool := NewSearchTool()
	limitKey, err := tool.NormalizedInputKey(json.RawMessage(`{"query":"TODO","path":".","mode":"lexical","limit":50}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(limit) error = %v", err)
	}
	maxResultsKey, err := tool.NormalizedInputKey(json.RawMessage(`{"query":"TODO","path":".","mode":"lexical","max_results":50}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(max_results) error = %v", err)
	}
	canonicalKey, err := tool.NormalizedInputKey(json.RawMessage(`{"query":"TODO","path":".","mode":"lexical","max_matches":50}`))
	if err != nil {
		t.Fatalf("NormalizedInputKey(canonical) error = %v", err)
	}
	if limitKey != canonicalKey || maxResultsKey != canonicalKey {
		t.Fatalf("input keys differ:\nlimit=%s\nmax_results=%s\ncanonical=%s", limitKey, maxResultsKey, canonicalKey)
	}
}

func TestSearchToolExecuteFindsLiteralMatchesWithFilters(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {\n\tprintln(\"Hello\")\n}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.go) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("hello from notes\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewSearchTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"hello","path":".","mode":"lexical","glob":"*.go","case_sensitive":false,"max_matches":10}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != `main.go:4:println("Hello")` {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestSearchToolExecuteHonorsMaxMatchesAcrossFilesAndLines(t *testing.T) {
	root := t.TempDir()
	content := strings.Join([]string{
		"TODO first",
		"TODO second",
		"TODO third",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(root, "tasks.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewSearchTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"TODO","path":".","mode":"lexical","glob":"","case_sensitive":true,"max_matches":2}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "tasks.txt:1:TODO first\ntasks.txt:2:TODO second" {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestSearchToolExecuteDefaultsNullGlobToSearchAllFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tasks.txt"), []byte("TODO first\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewSearchTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"TODO","path":".","mode":"lexical","glob":null,"case_sensitive":true,"max_matches":10}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "tasks.txt:1:TODO first" {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestSearchToolExecuteDefaultsNullOptionalFields(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tasks.txt"), []byte("TODO first\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewSearchTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"todo","path":".","mode":null,"glob":null,"regex":null,"case_sensitive":null,"max_matches":null}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "tasks.txt:1:TODO first" {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestSearchToolExecuteDefaultsToHybridWhenSemanticSearchIsConfigured(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("authorization guard\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	service := searchsvc.NewService(&stubSearchToolEmbedder{
		vectors: map[string][]float32{
			"permission":                       {1, 0},
			"notes.txt:1\nauthorization guard": {1, 0},
		},
	}, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, t.TempDir(), nil)

	result, err := NewSearchTool().Execute(context.TODO(), ExecutionContext{
		Workspace: scope,
		Search:    service,
	}, json.RawMessage(`{"query":"permission","path":".","mode":null,"glob":null,"regex":false,"case_sensitive":false,"max_matches":10}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "[semantic] notes.txt:1:authorization guard" {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestSearchToolExecuteSearchesExternalPath(t *testing.T) {
	scope, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	external := t.TempDir()
	if err := os.WriteFile(filepath.Join(external, "tasks.txt"), []byte("TODO first\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	result, err := NewSearchTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"TODO","path":"`+external+`","mode":"lexical","glob":"","case_sensitive":false,"max_matches":10}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.HasSuffix(result.Output, "/tasks.txt:1:TODO first") {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestSearchToolExecuteRejectsFilesystemRootPath(t *testing.T) {
	scope, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	_, err = NewSearchTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"TODO","path":"`+filesystemRootPathForSearchTest(t)+`","mode":"lexical","glob":"","case_sensitive":false,"max_matches":10}`))
	if err == nil {
		t.Fatal("Execute() error = nil, want invalid arguments")
	}
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("errors.Is(err, ErrInvalidArguments) = false, err = %v", err)
	}
	if !strings.Contains(err.Error(), `use "." or a workspace-relative path for project-wide search`) {
		t.Fatalf("err.Error() = %q", err.Error())
	}
}

func TestSearchToolExecuteRequiresValidMaxMatches(t *testing.T) {
	scope, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	_, err = NewSearchTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"TODO","path":".","mode":"lexical","glob":"","case_sensitive":false,"max_matches":0}`))
	if !errors.Is(err, ErrSearchMaxMatchesInvalid) {
		t.Fatalf("Execute() error = %v, want ErrSearchMaxMatchesInvalid", err)
	}
}

func filesystemRootPathForSearchTest(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("Getwd() error = %v", err)
		}
		volume := filepath.VolumeName(wd)
		if volume == "" {
			volume = "C:"
		}
		return volume + `\`
	}
	return string(os.PathSeparator)
}

func TestSearchToolExecuteClampsOversizedMaxMatchesAndReportsNotice(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tasks.txt"), []byte("TODO first\nTODO second\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewSearchTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"TODO","path":".","mode":null,"glob":null,"regex":null,"case_sensitive":null,"max_matches":1000}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, needle := range []string{"notice: max_matches clamped to 200", "tasks.txt:1:TODO first", "tasks.txt:2:TODO second"} {
		if !strings.Contains(result.Output, needle) {
			t.Fatalf("output = %q, missing %q", result.Output, needle)
		}
	}
}

func TestSearchToolExecuteHybridFallsBackWithoutSearchService(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("permission check\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewSearchTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"permission","path":".","mode":"hybrid","glob":"","case_sensitive":false,"max_matches":10}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "semantic search is not configured") {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestSearchToolExecuteSupportsRegexSearch(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tasks.txt"), []byte("TODO-123 fix search\nTODO-456 ship locate\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewSearchTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"TODO-[0-9]{3}","path":".","mode":"lexical","glob":null,"regex":true,"case_sensitive":true,"max_matches":10}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "tasks.txt:1:TODO-123 fix search\ntasks.txt:2:TODO-456 ship locate" {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestSearchToolExecuteRejectsRegexHybridMode(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("permission check\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	_, err = NewSearchTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"perm.*","path":".","mode":"hybrid","glob":null,"regex":true,"case_sensitive":false,"max_matches":10}`))
	if !errors.Is(err, searchsvc.ErrRegexUnsupportedInHybrid) {
		t.Fatalf("Execute() error = %v, want ErrRegexUnsupportedInHybrid", err)
	}
}

func TestSearchToolExecuteRecordsObservedResourcesForCompleteLexicalSearch(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n// TODO ship search reuse\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewSearchTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"TODO","path":".","mode":"lexical","glob":"*.go","regex":false,"case_sensitive":true,"max_matches":10}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(result.ObservedResources) != 2 {
		t.Fatalf("observed resources = %#v", result.ObservedResources)
	}
	if result.ObservedResources[0].Kind != ObservedResourceDirEntries || result.ObservedResources[1].Kind != ObservedResourceFileContent {
		t.Fatalf("observed resources = %#v", result.ObservedResources)
	}
	if strings.TrimSpace(result.ObservedResources[1].State) == "" {
		t.Fatalf("observed resources = %#v", result.ObservedResources)
	}
	var structured struct {
		Mode    string `json:"mode"`
		Notice  string `json:"notice"`
		Results []struct {
			Path    string `json:"path"`
			Line    int    `json:"line"`
			Snippet string `json:"snippet"`
			Source  string `json:"source"`
		} `json:"results"`
	}
	if err := json.Unmarshal(result.StructuredResult, &structured); err != nil {
		t.Fatalf("Unmarshal(StructuredResult) error = %v", err)
	}
	if structured.Mode != "lexical" || structured.Notice != "" || len(structured.Results) != 1 {
		t.Fatalf("structured result = %#v", structured)
	}
	if structured.Results[0].Path != "main.go" || structured.Results[0].Line != 2 || structured.Results[0].Source != "lexical" {
		t.Fatalf("structured result = %#v", structured)
	}
	if !strings.Contains(structured.Results[0].Snippet, "TODO ship search reuse") {
		t.Fatalf("structured result = %#v", structured)
	}
}

func TestSearchToolExecuteOmitsObservedResourcesWhenSearchIsCapped(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tasks.txt"), []byte("TODO first\nTODO second\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewSearchTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"TODO","path":".","mode":"lexical","glob":"","regex":false,"case_sensitive":true,"max_matches":1}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(result.ObservedResources) != 0 {
		t.Fatalf("observed resources = %#v", result.ObservedResources)
	}
}

func TestSearchToolExecuteOmitsObservedResourcesForHybridFallback(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("permission check\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewSearchTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"permission","path":".","mode":"hybrid","glob":"","regex":false,"case_sensitive":false,"max_matches":10}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(result.ObservedResources) != 0 {
		t.Fatalf("observed resources = %#v", result.ObservedResources)
	}
}

func TestSearchToolExecuteRecordsObservedResourcesForDirectFileNoMatch(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("no markers here"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewSearchTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"TODO","path":"notes.txt","mode":"lexical","glob":"","regex":false,"case_sensitive":true,"max_matches":10}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "no matches found" {
		t.Fatalf("output = %q", result.Output)
	}
	if len(result.ObservedResources) != 1 || result.ObservedResources[0].Kind != ObservedResourceFileContent {
		t.Fatalf("observed resources = %#v", result.ObservedResources)
	}
	if strings.TrimSpace(result.ObservedResources[0].State) == "" {
		t.Fatalf("observed resources = %#v", result.ObservedResources)
	}
}

func TestSearchToolExecuteOmitsObservedResourcesWhenObservationScopeIsTooLarge(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 1030; i++ {
		name := filepath.Join(root, "pkg", "file-"+strconv.Itoa(i)+".txt")
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", name, err)
		}
		if err := os.WriteFile(name, []byte("content\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", name, err)
		}
	}

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewSearchTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"query":"missing","path":".","mode":"lexical","glob":"*.txt","regex":false,"case_sensitive":true,"max_matches":10}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "no matches found" {
		t.Fatalf("output = %q", result.Output)
	}
	if len(result.ObservedResources) != 0 {
		t.Fatalf("observed resources = %#v", result.ObservedResources)
	}
}

type stubSearchToolEmbedder struct {
	vectors map[string][]float32
}

func (s *stubSearchToolEmbedder) Embed(_ context.Context, req provider.EmbeddingRequest) ([][]float32, error) {
	out := make([][]float32, len(req.Inputs))
	for idx, input := range req.Inputs {
		vector := s.vectors[input]
		if vector == nil {
			vector = []float32{0, 0}
		}
		out[idx] = append([]float32(nil), vector...)
	}
	return out, nil
}
