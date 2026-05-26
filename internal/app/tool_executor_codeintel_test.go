package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

type fakeCodeIntelRuntime struct {
	navigator tool.CodeIntel
	synced    []codeIntelMutationSyncPlan
}

func (f *fakeCodeIntelRuntime) Navigator(string, []string) tool.CodeIntel {
	return f.navigator
}

func (f *fakeCodeIntelRuntime) SyncMutation(_ context.Context, _ string, _ []string, plan codeIntelMutationSyncPlan) {
	f.synced = append(f.synced, plan)
}

type fakeNavigator struct {
	definition func(string, int, int) ([]tool.CodeIntelLocation, error)
	diagnostic func([]string) ([]tool.CodeIntelFileDiagnostics, error)
	refs       func(tool.CodeIntelRefsRequest) (tool.CodeIntelRefsResult, error)
	trace      func(tool.CodeIntelTraceRequest) (tool.CodeIntelTraceResult, error)
	rename     func(tool.CodeIntelRenameRequest) (tool.CodeIntelMutationSummary, error)
	codeAction func(tool.CodeIntelCodeActionRequest) (tool.CodeIntelCodeActionResult, error)
}

func (f fakeNavigator) Definition(_ context.Context, path string, line, character int) ([]tool.CodeIntelLocation, error) {
	return f.definition(path, line, character)
}

func (f fakeNavigator) Diagnostics(_ context.Context, filePaths []string) ([]tool.CodeIntelFileDiagnostics, error) {
	if f.diagnostic == nil {
		return nil, nil
	}
	return f.diagnostic(filePaths)
}

func (f fakeNavigator) Symbols(context.Context, string) (tool.CodeIntelSymbolsResult, error) {
	return tool.CodeIntelSymbolsResult{}, nil
}

func (f fakeNavigator) Refs(_ context.Context, request tool.CodeIntelRefsRequest) (tool.CodeIntelRefsResult, error) {
	if f.refs == nil {
		return tool.CodeIntelRefsResult{}, nil
	}
	return f.refs(request)
}

func (f fakeNavigator) Trace(_ context.Context, request tool.CodeIntelTraceRequest) (tool.CodeIntelTraceResult, error) {
	if f.trace == nil {
		return tool.CodeIntelTraceResult{}, nil
	}
	return f.trace(request)
}

func (f fakeNavigator) RenameSymbol(_ context.Context, request tool.CodeIntelRenameRequest) (tool.CodeIntelMutationSummary, error) {
	if f.rename == nil {
		return tool.CodeIntelMutationSummary{}, nil
	}
	return f.rename(request)
}

func (f fakeNavigator) ApplyCodeAction(_ context.Context, request tool.CodeIntelCodeActionRequest) (tool.CodeIntelCodeActionResult, error) {
	if f.codeAction == nil {
		return tool.CodeIntelCodeActionResult{}, nil
	}
	return f.codeAction(request)
}

func TestToolExecutorExecuteDefinitionToolPersistsCodeIntelResult(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewDefinitionTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("targetValue()\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	executor.SetCodeIntelService(&fakeCodeIntelRuntime{
		navigator: fakeNavigator{
			definition: func(path string, line, character int) ([]tool.CodeIntelLocation, error) {
				return []tool.CodeIntelLocation{{
					Path:      filepath.Join(root, "pkg", "service.go"),
					Line:      12,
					Character: 3,
				}}, nil
			},
		},
	})

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.DefinitionToolName,
		Arguments:  json.RawMessage(`{"path":"main.go","line":1,"character":0}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v", result)
	}
	if result.Output == "" {
		t.Fatal("expected non-empty output")
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	if call == nil || call.Output == "" {
		t.Fatalf("call = %#v", call)
	}
}

func TestToolExecutorExecuteTraceToolPersistsCodeIntelResult(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewTraceTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc run() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	executor.SetCodeIntelService(&fakeCodeIntelRuntime{
		navigator: fakeNavigator{
			trace: func(request tool.CodeIntelTraceRequest) (tool.CodeIntelTraceResult, error) {
				return tool.CodeIntelTraceResult{
					Supported: true,
					Found:     true,
					RootID:    "root",
					Nodes: []tool.CodeIntelTraceNode{
						{ID: "root", Name: "run", Kind: "Function", Path: request.Path, Line: 2, Character: 5},
					},
				}, nil
			},
		},
	})

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.TraceToolName,
		Arguments:  json.RawMessage(`{"path":"main.go","line":2,"character":5,"mode":"graph"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || strings.TrimSpace(result.Output) == "" {
		t.Fatalf("result = %#v", result)
	}
	var structuredTrace tool.CodeIntelTraceResult
	if err := json.Unmarshal(result.StructuredResult, &structuredTrace); err != nil {
		t.Fatalf("Unmarshal(StructuredResult) error = %v", err)
	}
	if !structuredTrace.Supported || !structuredTrace.Found || structuredTrace.RootID != "root" || len(structuredTrace.Nodes) != 1 {
		t.Fatalf("structured result = %#v", structuredTrace)
	}
	if structuredTrace.Nodes[0].Name != "run" || filepath.Base(structuredTrace.Nodes[0].Path) != "main.go" {
		t.Fatalf("structured result = %#v", structuredTrace)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	if call == nil || call.Output == "" {
		t.Fatalf("call = %#v", call)
	}
	if string(call.StructuredResult) != string(result.StructuredResult) {
		t.Fatalf("call structured result = %s, want %s", string(call.StructuredResult), string(result.StructuredResult))
	}
}

func TestToolExecutorExecuteRefsToolPersistsCodeIntelResult(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewRefsTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nvar value int\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	executor.SetCodeIntelService(&fakeCodeIntelRuntime{
		navigator: fakeNavigator{
			refs: func(request tool.CodeIntelRefsRequest) (tool.CodeIntelRefsResult, error) {
				return tool.CodeIntelRefsResult{
					Supported: true,
					Found:     true,
					Target: tool.CodeIntelTraceNode{
						Name:      "value",
						Kind:      "Variable",
						Path:      request.Path,
						Line:      2,
						Character: 4,
					},
					References: []tool.CodeIntelReference{{
						Kind:      tool.CodeIntelReferenceKindRead,
						Path:      request.Path,
						Line:      2,
						Character: 4,
						Snippet:   "var value int",
					}},
					ClassificationSupported: true,
				}, nil
			},
		},
	})

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.RefsToolName,
		Arguments:  json.RawMessage(`{"path":"main.go","line":2,"character":4}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || strings.TrimSpace(result.Output) == "" {
		t.Fatalf("result = %#v", result)
	}
	var structuredRefs tool.CodeIntelRefsResult
	if err := json.Unmarshal(result.StructuredResult, &structuredRefs); err != nil {
		t.Fatalf("Unmarshal(StructuredResult) error = %v", err)
	}
	if !structuredRefs.Supported || !structuredRefs.Found || !structuredRefs.ClassificationSupported || len(structuredRefs.References) != 1 {
		t.Fatalf("structured result = %#v", structuredRefs)
	}
	if structuredRefs.Target.Name != "value" || filepath.Base(structuredRefs.Target.Path) != "main.go" {
		t.Fatalf("structured result = %#v", structuredRefs)
	}
	if structuredRefs.References[0].Kind != tool.CodeIntelReferenceKindRead || filepath.Base(structuredRefs.References[0].Path) != "main.go" || structuredRefs.References[0].Snippet != "var value int" {
		t.Fatalf("structured result = %#v", structuredRefs)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	if call == nil || call.Output == "" {
		t.Fatalf("call = %#v", call)
	}
	if string(call.StructuredResult) != string(result.StructuredResult) {
		t.Fatalf("call structured result = %s, want %s", string(call.StructuredResult), string(result.StructuredResult))
	}
}

func TestToolExecutorSyncsCodeIntelAfterWriteMutation(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewWriteTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	fake := &fakeCodeIntelRuntime{}
	executor.SetCodeIntelService(fake)

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "write",
		Arguments:  json.RawMessage(`{"path":"main.go","content":"package main\n"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v", result)
	}
	if len(fake.synced) != 1 {
		t.Fatalf("synced plans = %#v", fake.synced)
	}
	if len(fake.synced[0].Changed) != 1 || filepath.Base(fake.synced[0].Changed[0]) != "main.go" {
		t.Fatalf("sync plan = %#v", fake.synced[0])
	}
}

func TestToolExecutorWriteAppendsCurrentFileDiagnosticsWhenPresent(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewWriteTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	fake := &fakeCodeIntelRuntime{}
	fake.navigator = fakeNavigator{
		diagnostic: func(filePaths []string) ([]tool.CodeIntelFileDiagnostics, error) {
			if len(fake.synced) != 1 {
				t.Fatalf("diagnostics called before sync: %#v", fake.synced)
			}
			if len(filePaths) != 1 || filepath.Base(filePaths[0]) != "main.go" {
				t.Fatalf("filePaths = %#v", filePaths)
			}
			return []tool.CodeIntelFileDiagnostics{{
				Path: filePaths[0],
				Diagnostics: []tool.CodeIntelDiagnostic{{
					Line:      1,
					Character: 0,
					Severity:  "error",
					Message:   "unexpected token",
					Source:    "tsserver",
				}},
			}}, nil
		},
	}
	executor.SetCodeIntelService(fake)

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "write",
		Arguments:  json.RawMessage(`{"path":"main.go","content":"package main\n"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v", result)
	}
	for _, want := range []string{
		"wrote 13 bytes to",
		"Diagnostics detected in this file:",
		"[error] unexpected token (tsserver)",
	} {
		if !strings.Contains(result.Output, want) {
			t.Fatalf("result.Output = %q, missing %q", result.Output, want)
		}
	}
}

func TestToolExecutorWriteLeavesOutputPlainWhenNoDiagnosticsExist(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewWriteTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	executor.SetCodeIntelService(&fakeCodeIntelRuntime{
		navigator: fakeNavigator{
			diagnostic: func(filePaths []string) ([]tool.CodeIntelFileDiagnostics, error) {
				return []tool.CodeIntelFileDiagnostics{{
					Path: filePaths[0],
				}}, nil
			},
		},
	})

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "write",
		Arguments:  json.RawMessage(`{"path":"main.go","content":"package main\n"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v", result)
	}
	if strings.Contains(result.Output, "Diagnostics detected in this file:") {
		t.Fatalf("result.Output = %q", result.Output)
	}
}

func TestToolExecutorWriteKeepsExistingFileWhenNewErrorDiagnosticsAppear(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool(), tool.NewWriteTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\nfunc keep() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	readResult, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-read",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["main.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(read) error = %v", err)
	}
	if readResult.Status != ToolExecutionStatusExecuted || readResult.Error != "" {
		t.Fatalf("readResult = %#v", readResult)
	}

	fake := &fakeCodeIntelRuntime{}
	fake.navigator = fakeNavigator{
		diagnostic: func(filePaths []string) ([]tool.CodeIntelFileDiagnostics, error) {
			if len(fake.synced) != 1 {
				t.Fatalf("post-write diagnostics called after sync: %#v", fake.synced)
			}
			return []tool.CodeIntelFileDiagnostics{{
				Path: filePaths[0],
				Diagnostics: []tool.CodeIntelDiagnostic{{
					Line:      2,
					Character: 16,
					Severity:  "error",
					Message:   "introduced parse failure",
					Source:    "tsserver",
				}},
			}}, nil
		},
	}
	executor.SetCodeIntelService(fake)

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-write",
		ToolName:   tool.WriteToolName,
		Arguments:  json.RawMessage(`{"path":"main.go","content":"package main\nfunc broken() {\n"}`),
	})
	if err != nil {
		t.Fatalf("Execute(write) error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.Error != "" {
		t.Fatalf("result = %#v", result)
	}
	for _, want := range []string{
		"wrote",
		"Diagnostics detected in this file:",
		"[error] introduced parse failure (tsserver)",
	} {
		if !strings.Contains(result.Output, want) {
			t.Fatalf("result.Output = %q, missing %q", result.Output, want)
		}
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "package main\nfunc broken() {\n" {
		t.Fatalf("content = %q", string(content))
	}
	if len(fake.synced) != 1 {
		t.Fatalf("synced plans = %#v", fake.synced)
	}
	if filepath.Base(fake.synced[0].Changed[0]) != "main.go" {
		t.Fatalf("sync plan = %#v", fake.synced)
	}
}

func TestToolExecutorSyncsCodeIntelAfterApplyPatchMutation(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool(), tool.NewApplyPatchTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nconst value = 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	readResult, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-read",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["main.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(read) error = %v", err)
	}
	if readResult.Status != ToolExecutionStatusExecuted || readResult.Error != "" {
		t.Fatalf("readResult = %#v", readResult)
	}

	fake := &fakeCodeIntelRuntime{}
	executor.SetCodeIntelService(fake)

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-patch",
		ToolName:   tool.ApplyPatchToolName,
		Arguments: json.RawMessage(`*** Begin Patch
*** Update File: main.go
@@
-const value = 1
+const value = 2
*** End Patch
`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v", result)
	}
	if len(fake.synced) != 1 {
		t.Fatalf("synced plans = %#v", fake.synced)
	}
	if len(fake.synced[0].Changed) != 1 || filepath.Base(fake.synced[0].Changed[0]) != "main.go" {
		t.Fatalf("sync plan = %#v", fake.synced[0])
	}
}
