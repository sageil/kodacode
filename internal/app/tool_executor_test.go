package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/observability"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/textdiff"
	"github.com/sageil/kodacode/internal/tool"
)

type stubDelegateRuntime struct {
	last   DelegateSessionTurnInput
	result DelegateSessionTurnResult
	err    error
}

func (s *stubDelegateRuntime) DelegateSessionTurn(_ context.Context, input DelegateSessionTurnInput) (DelegateSessionTurnResult, error) {
	s.last = input
	if s.err != nil {
		return DelegateSessionTurnResult{}, s.err
	}
	return s.result, nil
}

func appendDelegatedHandoffForToolReuseTest(t *testing.T, sessions *SessionService, handoff events.AgentHandoffPayload) {
	t.Helper()

	for _, draft := range []events.Draft{
		{SessionID: handoff.ParentSessionID, TurnID: handoff.ParentTurnID, Type: events.TypeAgentHandoff, Payload: handoff},
		{SessionID: handoff.ChildSessionID, TurnID: handoff.ChildTurnID, Type: events.TypeAgentHandoff, Payload: handoff},
	} {
		if _, err := sessions.append(context.Background(), draft); err != nil {
			t.Fatalf("append handoff(%s) error = %v", draft.SessionID, err)
		}
	}
}

func TestToolExecutorProviderToolsReturnsSortedMetadata(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	executor, err := NewToolExecutor(sessions,
		stubTool{definition: tool.Definition{
			Name:        tool.BashToolName,
			Description: "Run a shell command",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
		stubTool{definition: tool.Definition{
			Name:        "write",
			Description: "Write a file",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
		stubTool{definition: tool.Definition{
			Name:        "read",
			Description: "Read a file",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
	)
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	tools := executor.ProviderTools()
	if len(tools) != 3 {
		t.Fatalf("tools = %#v", tools)
	}
	if tools[0].Name != "read" || tools[0].Description != "Read a file" || tools[0].InputSchema != `{"type":"object"}` {
		t.Fatalf("tools[0] = %#v", tools[0])
	}
	if tools[1].Name != "write" || tools[1].Description != "Write a file" || tools[1].InputSchema != `{"type":"object"}` {
		t.Fatalf("tools[1] = %#v", tools[1])
	}
	if tools[2].Name != tool.BashToolName || tools[2].Description != "Run a shell command" || tools[2].InputSchema != `{"type":"object"}` {
		t.Fatalf("tools[2] = %#v", tools[2])
	}
}

func TestToolExecutorLogsToolErrorDetailsOnlyInDebug(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	logDir := t.TempDir()
	logger, err := observability.New(observability.Config{Dir: logDir, DebugEnabled: true})
	if err != nil {
		t.Fatalf("observability.New() error = %v", err)
	}
	t.Cleanup(func() {
		_ = logger.Close()
	})

	executor, err := NewToolExecutor(sessions, stubTool{
		definition: tool.Definition{
			Name:        "failing_tool",
			Description: "Fails for log coverage",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
		result: tool.Result{Error: "sensitive detailed tool failure"},
	})
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	executor.SetLogger(logger.With("component", "tool_executor"))
	root := t.TempDir()
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "failing_tool",
		Arguments:  json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Error != "sensitive detailed tool failure" {
		t.Fatalf("result.Error = %q", result.Error)
	}

	opsLog := readOperationsLog(t, logDir)
	if !strings.Contains(opsLog, "tool execution completed with error") {
		t.Fatalf("ops log = %q", opsLog)
	}
	if !strings.Contains(opsLog, "tool_error_bytes=") {
		t.Fatalf("ops log = %q", opsLog)
	}
	if strings.Contains(opsLog, "sensitive detailed tool failure") || strings.Contains(opsLog, "tool_error=") {
		t.Fatalf("ops log leaked tool error detail: %q", opsLog)
	}

	debugLog := readDebugLog(t, logDir)
	if !strings.Contains(debugLog, "tool execution output") || !strings.Contains(debugLog, "sensitive detailed tool failure") {
		t.Fatalf("debug log = %q", debugLog)
	}
}

func TestToolExecutorExecuteApplyPatchPreservesRawCustomInput(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewApplyPatchTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if _, err := sessions.CreateSession(context.TODO(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	patch := `*** Begin Patch
*** Add File: notes.txt
+hello
*** End Patch
`
	result, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.ApplyPatchToolName,
		ToolKind:   provider.ToolKindCustom,
		Arguments:  json.RawMessage(patch),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || !strings.Contains(result.Output, "A notes.txt") {
		t.Fatalf("result = %#v", result)
	}
	content, err := os.ReadFile(filepath.Join(root, "notes.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "hello\n" {
		t.Fatalf("content = %q", string(content))
	}

	replayed, err := store.Replay(context.TODO(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	start, ok := replayed[1].Payload.(events.ToolExecStartPayload)
	if !ok {
		t.Fatalf("payload = %T, want ToolExecStartPayload", replayed[1].Payload)
	}
	if start.ToolKind != string(provider.ToolKindCustom) || start.Input != patch {
		t.Fatalf("start payload = %#v", start)
	}
}

func TestToolExecutorExecuteApplyPatchAcceptsFunctionArguments(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewApplyPatchTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if _, err := sessions.CreateSession(context.TODO(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	patch := `*** Begin Patch
*** Add File: notes.txt
+hello
*** End Patch
`
	args, err := json.Marshal(map[string]string{"patch": patch})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	result, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.ApplyPatchToolName,
		ToolKind:   provider.ToolKindFunction,
		Arguments:  json.RawMessage(args),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.CanonicalArguments != patch {
		t.Fatalf("result = %#v", result)
	}
	content, err := os.ReadFile(filepath.Join(root, "notes.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "hello\n" {
		t.Fatalf("content = %q", string(content))
	}

	replayed, err := store.Replay(context.TODO(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	start, ok := replayed[1].Payload.(events.ToolExecStartPayload)
	if !ok {
		t.Fatalf("payload = %T, want ToolExecStartPayload", replayed[1].Payload)
	}
	if start.ToolKind != string(provider.ToolKindFunction) || start.Input != patch {
		t.Fatalf("start payload = %#v", start)
	}
}

func TestToolExecutorExecuteApplyPatchPersistsRuntimeWriteMutations(t *testing.T) {
	store := events.NewMemoryStore()
	blobs := newTestSQLiteBlobStore(t)
	sessions, err := NewSessionServiceWithBlobs(store, blobs)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewApplyPatchTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	before := "header\n" + strings.Repeat("x\n", toolResultBlobInlineLimit/2+1) + "old\n"
	if err := os.WriteFile(filepath.Join(root, "existing.txt"), []byte(before), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.TODO(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	patch := `*** Begin Patch
*** Add File: created.txt
+created
*** Update File: existing.txt
@@
-old
+new
*** End Patch
`
	if _, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.ApplyPatchToolName,
		ToolKind:   provider.ToolKindCustom,
		Arguments:  json.RawMessage(patch),
	}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	replayed, err := store.Replay(context.TODO(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	var payload events.ToolExecEndPayload
	for _, event := range replayed {
		if event.Type == events.TypeToolExecEnd {
			payload = event.Payload.(events.ToolExecEndPayload)
			break
		}
	}
	if len(payload.WriteMutations) != 2 {
		t.Fatalf("WriteMutations = %#v", payload.WriteMutations)
	}
	mutations := make(map[string]events.WriteMutation, len(payload.WriteMutations))
	for _, mutation := range payload.WriteMutations {
		mutations[filepath.Base(mutation.Path)] = mutation
	}
	created := mutations["created.txt"]
	if created.Existed || created.Before != "" || created.DiffPreview == nil {
		t.Fatalf("created mutation = %#v", created)
	}
	added, removed := textdiff.LineStats(*created.DiffPreview)
	if added != 1 || removed != 0 {
		t.Fatalf("created diff stats = +%d -%d", added, removed)
	}
	existing := mutations["existing.txt"]
	if !existing.Existed || !existing.BeforeTruncated || existing.BeforeBlob == nil || existing.DiffPreview == nil {
		t.Fatalf("existing mutation = %#v", existing)
	}
	loaded, err := blobs.Load(context.TODO(), existing.BeforeBlob.Ref)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded != before {
		t.Fatalf("loaded before length = %d, want %d", len(loaded), len(before))
	}
	added, removed = textdiff.LineStats(*existing.DiffPreview)
	if added != 1 || removed != 1 {
		t.Fatalf("existing diff stats = +%d -%d", added, removed)
	}
}

func TestToolExecutorProviderToolsAllowedIncludesMCPWildcardTools(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions,
		stubTool{definition: tool.Definition{
			Name:        "read",
			Description: "Read a file",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
	)
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	executor.ReplaceMCPTools([]tool.Tool{
		stubTool{definition: tool.Definition{
			Name:        "mcp_demo_echo",
			Description: "Echo via MCP",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
	})

	tools := executor.ProviderToolsAllowed([]string{tool.MCPToolWildcard})
	if len(tools) != 1 {
		t.Fatalf("tools = %#v, want only MCP tool", tools)
	}
	if tools[0].Name != "mcp_demo_echo" {
		t.Fatalf("tools[0] = %#v", tools[0])
	}
}

func TestToolExecutorExecuteReturnsFullErrorWhileStoringPreview(t *testing.T) {
	store := events.NewMemoryStore()
	blobs := newTestSQLiteBlobStore(t)
	sessions, err := NewSessionServiceWithBlobs(store, blobs)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	largeError := strings.Repeat("error detail\n", toolResultBlobInlineLimit)
	executor, err := NewToolExecutor(sessions, stubTool{
		definition: tool.Definition{Name: tool.WebFetchToolName, Description: "Fetch web content"},
		result:     tool.Result{Error: largeError},
	})
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.WebFetchToolName,
		Arguments:  json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Error != largeError {
		t.Fatalf("result.Error length = %d, want full error length %d", len(result.Error), len(largeError))
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1"})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	var payload events.ToolExecEndPayload
	for _, event := range replayed {
		if event.Type == events.TypeToolExecEnd {
			payload = event.Payload.(events.ToolExecEndPayload)
			break
		}
	}
	if payload.ErrorBlob == nil {
		t.Fatalf("ErrorBlob = nil, want offloaded error")
	}
	if result.Error == payload.Error {
		t.Fatalf("result.Error should remain full while payload stores preview")
	}
}

func TestToolExecutorProviderToolsCompactSchemaDescriptions(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions,
		stubTool{definition: tool.Definition{
			Name:        "custom",
			Description: "Read a file",
			InputSchema: json.RawMessage(`{"type":"object","description":"top","properties":{"path":{"type":"string","description":"file path"},"description":{"type":"string","description":"user-facing description"},"mode":{"type":"string","enum":["short","full"],"description":"render mode"}},"required":["path"],"additionalProperties":false}`),
		}},
	)
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	surface := executor.providerToolSurfaceAllowed(nil)
	if len(surface.Tools) != 1 {
		t.Fatalf("surface.Tools = %#v", surface.Tools)
	}
	if surface.DescriptionTokensSaved() != 0 {
		t.Fatalf("DescriptionTokensSaved() = %d, want 0", surface.DescriptionTokensSaved())
	}
	if surface.SchemaTokensSaved() <= 0 {
		t.Fatalf("SchemaTokensSaved() = %d, want > 0", surface.SchemaTokensSaved())
	}
	var schema map[string]any
	if err := json.Unmarshal([]byte(surface.Tools[0].InputSchema), &schema); err != nil {
		t.Fatalf("provider schema json invalid: %v", err)
	}
	if schema["type"] != "object" {
		t.Fatalf("provider schema = %#v", schema)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("provider schema properties = %#v", schema["properties"])
	}
	if _, ok := properties["description"]; !ok {
		t.Fatalf("provider schema dropped description argument: %#v", properties)
	}
	for _, key := range []string{"path", "description", "mode"} {
		propertySchema, ok := properties[key].(map[string]any)
		if !ok {
			t.Fatalf("provider schema property %q = %#v", key, properties[key])
		}
		if strings.Contains(surface.Tools[0].InputSchema, `"`+key+`":{"description"`) {
			t.Fatalf("provider schema kept nested description for %q: %s", key, surface.Tools[0].InputSchema)
		}
		if _, ok := propertySchema["description"]; ok {
			t.Fatalf("provider schema kept description annotation for %q: %#v", key, propertySchema)
		}
	}
}

func TestToolExecutorProviderToolsPreserveSchemaDescriptionsForHighGuidanceTools(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewWriteTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	surface := executor.providerToolSurfaceAllowed(nil)
	if len(surface.Tools) != 1 {
		t.Fatalf("surface.Tools = %#v", surface.Tools)
	}
	if surface.SchemaTokensSaved() != 0 {
		t.Fatalf("SchemaTokensSaved() = %d, want 0", surface.SchemaTokensSaved())
	}
	var schema struct {
		Properties map[string]map[string]any `json:"properties"`
	}
	if err := json.Unmarshal([]byte(surface.Tools[0].InputSchema), &schema); err != nil {
		t.Fatalf("provider schema json invalid: %v", err)
	}
	if _, ok := schema.Properties["content"]["description"]; !ok {
		t.Fatalf("provider schema dropped content description: %#v", schema.Properties["content"])
	}
}

func TestToolExecutorProviderToolsIncludeSimpleExamplesForHighGuidanceTools(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	readTool := tool.NewReadTool()
	definition := readTool.Definition()
	executor, err := NewToolExecutor(sessions, readTool)
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	surface := executor.providerToolSurfaceAllowed(nil)
	if len(surface.Tools) != 1 {
		t.Fatalf("surface.Tools = %#v", surface.Tools)
	}
	got := surface.Tools[0].Description
	want := definition.ProviderDescription + " Example: " + definition.ArgumentExamples[0]
	if got != want {
		t.Fatalf("provider description = %q, want %q", got, want)
	}
	if strings.Contains(got, "reuse that context instead of calling read again") {
		t.Fatalf("provider read description still exposes removed reuse guidance: %q", got)
	}
	if strings.Contains(surface.Tools[0].InputSchema, "end_line") ||
		strings.Contains(surface.Tools[0].InputSchema, "after_line") ||
		strings.Contains(surface.Tools[0].InputSchema, "start_line") ||
		strings.Contains(surface.Tools[0].InputSchema, "max_lines") {
		t.Fatalf("provider read schema exposed deprecated range controls: %s", surface.Tools[0].InputSchema)
	}
	if !strings.Contains(surface.Tools[0].InputSchema, "offset") || !strings.Contains(surface.Tools[0].InputSchema, "limit") {
		t.Fatalf("provider read schema missing offset/limit: %s", surface.Tools[0].InputSchema)
	}
}

func TestToolExecutorProviderToolsExposeParallelSafety(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool(), tool.NewWriteTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	surface := executor.providerToolSurfaceAllowed(nil)
	parallelSafeByName := make(map[string]bool, len(surface.Tools))
	for _, available := range surface.Tools {
		parallelSafeByName[available.Name] = available.ParallelSafe
	}
	if !parallelSafeByName[tool.ReadToolName] {
		t.Fatalf("read ParallelSafe = false")
	}
	if parallelSafeByName[tool.WriteToolName] {
		t.Fatalf("write ParallelSafe = true")
	}
}

func TestToolExecutorProviderToolsExposeApplyPatchAsCustomTool(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewApplyPatchTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	tools := executor.ProviderTools()
	if len(tools) != 1 {
		t.Fatalf("tools = %#v", tools)
	}
	if tools[0].Name != tool.ApplyPatchToolName || tools[0].Kind != provider.ToolKindCustom {
		t.Fatalf("tools[0] = %#v, want custom apply_patch first", tools[0])
	}
	if tools[0].InputFormat == nil || tools[0].InputFormat.Syntax != "lark" {
		t.Fatalf("apply_patch input format = %#v", tools[0].InputFormat)
	}
}

func TestToolExecutorProviderToolsExposeApplyPatchAsFunctionWhenCustomUnsupported(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewApplyPatchTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	surface := executor.providerToolSurfaceAllowedForModel(nil, provider.ModelRef{ProviderID: "github-copilot", ModelID: "gpt-4.1"})
	if len(surface.Tools) != 1 {
		t.Fatalf("tools = %#v", surface.Tools)
	}
	got := surface.Tools[0]
	if got.Name != tool.ApplyPatchToolName || got.KindOrDefault() != provider.ToolKindFunction {
		t.Fatalf("tool = %#v, want function apply_patch", got)
	}
	if got.InputFormat != nil {
		t.Fatalf("input format = %#v, want nil for function adapter", got.InputFormat)
	}
	if !strings.Contains(got.InputSchema, `"patch"`) || !strings.Contains(got.Description, "JSON") ||
		!strings.Contains(got.Description, `Patch lines MUST NOT include read output line number prefixes like "40:"`) ||
		!strings.Contains(got.InputSchema, `Patch lines MUST NOT include read output line number prefixes like \"40:\"`) {
		t.Fatalf("tool = %#v, want JSON patch function adapter", got)
	}
}

func TestToolExecutorProviderToolsPreserveGuidanceForCodeIntelTools(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	symbolsTool := tool.NewSymbolsTool()
	definition := symbolsTool.Definition()
	executor, err := NewToolExecutor(sessions, symbolsTool)
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	surface := executor.providerToolSurfaceAllowed(nil)
	if len(surface.Tools) != 1 {
		t.Fatalf("surface.Tools = %#v", surface.Tools)
	}
	if surface.SchemaTokensSaved() != 0 {
		t.Fatalf("SchemaTokensSaved() = %d, want 0", surface.SchemaTokensSaved())
	}
	got := surface.Tools[0].Description
	want := definition.ProviderDescription + " Example: " + definition.ArgumentExamples[0]
	if got != want {
		t.Fatalf("provider description = %q, want %q", got, want)
	}
	if !strings.Contains(surface.Tools[0].InputSchema, `"description":"Symbol name or partial symbol name."`) {
		t.Fatalf("provider schema dropped symbol query description: %s", surface.Tools[0].InputSchema)
	}
}

func TestToolExecutorExecuteRunsAuthorizedReadAndEmitsExecutionEvents(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "read",
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.Output != expectedReadSingleLineOutput("package main") {
		t.Fatalf("result = %#v", result)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed) != 3 || replayed[1].Type != events.TypeToolExecStart || replayed[2].Type != events.TypeToolExecEnd {
		t.Fatalf("events = %#v", replayed)
	}
}

func TestToolExecutorExecuteRunsDuplicateReadToolCall(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "read",
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(first) error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted || first.Output != expectedReadSingleLineOutput("package main") {
		t.Fatalf("first result = %#v", first)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-2",
		ToolName:   "read",
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(second) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.Output != first.Output || second.ReusedFromCallID != "" {
		t.Fatalf("second result = %#v", second)
	}
}

func TestToolExecutorExecuteRunsDuplicateReadEvenWhenPriorResultVisible(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "read",
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(first) error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted || first.Output == "" {
		t.Fatalf("first result = %#v", first)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-2",
		ToolName:   "read",
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
		ModelVisibleInputs: []provider.Input{{
			Kind:     provider.InputKindToolResult,
			CallID:   "call-1",
			ToolName: tool.ReadToolName,
			Output:   first.Output,
		}},
	})
	if err != nil {
		t.Fatalf("Execute(second) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.ReusedFromCallID != "" {
		t.Fatalf("second result = %#v", second)
	}
	if !strings.Contains(second.Output, "package main") {
		t.Fatalf("second output = %q", second.Output)
	}

	third, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-3",
		ToolName:   "read",
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
		ModelVisibleInputs: []provider.Input{
			{Kind: provider.InputKindToolResult, CallID: "call-1", ToolName: tool.ReadToolName, Output: first.Output},
			{Kind: provider.InputKindToolResult, CallID: "call-2", ToolName: tool.ReadToolName, Output: second.Output},
		},
	})
	if err != nil {
		t.Fatalf("Execute(third) error = %v", err)
	}
	if third.Status != ToolExecutionStatusExecuted || third.ReusedFromCallID != "" {
		t.Fatalf("third result = %#v", third)
	}
	if !strings.Contains(third.Output, "package main") {
		t.Fatalf("third output = %q", third.Output)
	}
}

func TestToolExecutorExecuteRunsDuplicateReadAfterFailedPatch(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(first read) error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted {
		t.Fatalf("first read = %#v", first)
	}

	failedPatch, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-2",
		ToolName:   tool.ApplyPatchToolName,
		Arguments: json.RawMessage(`*** Begin Patch
*** Update File: app.go
@@
-missing
+replacement
*** End Patch
`),
	})
	if err != nil {
		t.Fatalf("Execute(apply_patch) error = %v", err)
	}
	if failedPatch.Status != ToolExecutionStatusExecuted ||
		!strings.Contains(failedPatch.Error, "patch failed") {
		t.Fatalf("failed patch = %#v", failedPatch)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-3",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(second read) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.Output != first.Output || second.ReusedFromCallID != "" {
		t.Fatalf("second read = %#v", second)
	}
}

func TestToolExecutorExecuteApplyPatchMatchesCurrentContentAfterUnrelatedMutation(t *testing.T) {
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
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("top\nbefore\nbottom\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	firstPatch, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-patch-1",
		ToolName:   tool.ApplyPatchToolName,
		Arguments: json.RawMessage(`*** Begin Patch
*** Update File: app.go
@@
-top
+TOP
*** End Patch
`),
	})
	if err != nil {
		t.Fatalf("Execute(first patch) error = %v", err)
	}
	if firstPatch.Error != "" {
		t.Fatalf("first patch = %#v", firstPatch)
	}

	secondPatch, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-patch-2",
		ToolName:   tool.ApplyPatchToolName,
		Arguments: json.RawMessage(`*** Begin Patch
*** Update File: app.go
@@
-before
+after
*** End Patch
`),
	})
	if err != nil {
		t.Fatalf("Execute(second patch) error = %v", err)
	}
	if secondPatch.Error != "" {
		t.Fatalf("second patch = %#v", secondPatch)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "TOP\nafter\nbottom\n" {
		t.Fatalf("final content = %q", string(content))
	}
}

func TestToolExecutorExecuteRunsReadAcrossUnrelatedWriteWithCanonicalArgs(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app.go) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("draft\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(first read) error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted || first.Output != expectedReadSingleLineOutput("package main") {
		t.Fatalf("first read = %#v", first)
	}

	written, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-2",
		ToolName:   "write",
		Arguments:  json.RawMessage(`{"path":"notes.txt","content":"updated\n"}`),
	})
	if err != nil {
		t.Fatalf("Execute(write) error = %v", err)
	}
	if written.Status != ToolExecutionStatusExecuted {
		t.Fatalf("write result = %#v", written)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-3",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":[" app.go "],"offset":null,"limit":null}`),
	})
	if err != nil {
		t.Fatalf("Execute(second read) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.Output != first.Output || second.ReusedFromCallID != "" {
		t.Fatalf("second read = %#v", second)
	}
}

func TestToolExecutorExecuteRunsCoveredExplicitReadWindow(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app.go) error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(first read) error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted {
		t.Fatalf("first read = %#v", first)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-2",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["app.go"],"offset":0,"limit":1000}`),
	})
	if err != nil {
		t.Fatalf("Execute(second read) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted ||
		second.Output != first.Output ||
		second.ReusedFromCallID != "" {
		t.Fatalf("second read = %#v", second)
	}
}

func TestToolExecutorExecuteRunsPathAliasReadRepeat(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "server.ts"), []byte("line 1\nline 2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(server.ts) error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["server.ts"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(first read) error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted {
		t.Fatalf("first read = %#v", first)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-2",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"path":"server.ts","offset":0,"limit":2000}`),
	})
	if err != nil {
		t.Fatalf("Execute(second read) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted ||
		second.Output != first.Output ||
		second.ReusedFromCallID != "" {
		t.Fatalf("second read = %#v", second)
	}
}

func TestToolExecutorExecuteRunsDuplicateImplicitPartialRead(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	var content strings.Builder
	for i := 1; i <= 5000; i++ {
		fmt.Fprintf(&content, "line %04d %s\n", i, strings.Repeat("x", 80))
	}
	if err := os.WriteFile(filepath.Join(root, "big.go"), []byte(content.String()), 0o644); err != nil {
		t.Fatalf("WriteFile(big.go) error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["big.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(first read) error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted || !strings.Contains(first.Output, "line 0001") {
		t.Fatalf("first read = %#v", first)
	}
	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	observed := state.Turns["turn-1"].ToolCalls["call-1"].ObservedResources
	if len(observed) != 1 || observed[0].Complete || observed[0].StartLine != 1 || observed[0].EndLine != 1000 || observed[0].TotalLines != 5000 {
		t.Fatalf("observed coverage = %#v, want first default read window", observed)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-2",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["big.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(second read) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted ||
		second.Output != first.Output ||
		second.ReusedFromCallID != "" {
		t.Fatalf("second read = %#v", second)
	}
}

func TestToolExecutorExecuteMixedReadBatchReadsRequestedTargets(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	files := map[string]string{
		"cache.ts":  "cache body\n",
		"task.ts":   "task body\n",
		"server.ts": "server body\n",
		"models.ts": "models body\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["cache.ts","task.ts","server.ts"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(first read) error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted || !strings.Contains(first.Output, "server body") {
		t.Fatalf("first read = %#v", first)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-2",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["models.ts","server.ts"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(second read) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted ||
		!strings.Contains(second.Output, "models body") ||
		!strings.Contains(second.Output, "server body") ||
		second.ReusedFromCallID != "" {
		t.Fatalf("second read = %#v", second)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	call := state.Turns["turn-1"].ToolCalls["call-2"]
	if call == nil || call.Input != `{"paths":["models.ts","server.ts"]}` {
		t.Fatalf("call-2 input = %#v", call)
	}
	if len(call.ObservedResources) != 2 {
		t.Fatalf("call-2 observed resources = %#v", call.ObservedResources)
	}
	var sawModels, sawServer bool
	for _, resource := range call.ObservedResources {
		if strings.HasSuffix(resource.Path, "/models.ts") {
			sawModels = true
		}
		if strings.HasSuffix(resource.Path, "/server.ts") {
			sawServer = true
		}
	}
	if !sawModels || !sawServer {
		t.Fatalf("call-2 observed resources = %#v", call.ObservedResources)
	}
}

func TestToolExecutorExecuteAcceptsStringifiedReadRangeArguments(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("one\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["notes.txt"],"offset":"1","limit":"2"}`),
	})
	if err != nil {
		t.Fatalf("Execute(first read) error = %v", err)
	}
	want := expectedReadOutputForPath("notes.txt", "2: two\n3: three\n(showing lines 2-3 of 4. Use offset=3 (0-based) to continue.)")
	if first.Status != ToolExecutionStatusExecuted || first.Output != want {
		t.Fatalf("first read = %#v", first)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-2",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["notes.txt"],"offset":1,"limit":2}`),
	})
	if err != nil {
		t.Fatalf("Execute(second read) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.Output != first.Output || second.ReusedFromCallID != "" {
		t.Fatalf("second read = %#v", second)
	}
}

func TestToolExecutorExecuteDoesNotReuseReadAfterSameFileWrite(t *testing.T) {
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
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(first read) error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted || first.Output != expectedReadSingleLineOutput("package main") {
		t.Fatalf("first read = %#v", first)
	}

	written, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-2",
		ToolName:   "write",
		Arguments:  json.RawMessage(`{"path":"app.go","content":"package changed\n"}`),
	})
	if err != nil {
		t.Fatalf("Execute(write) error = %v", err)
	}
	if written.Status != ToolExecutionStatusExecuted {
		t.Fatalf("write result = %#v", written)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-3",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(second read) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.Output != expectedReadSingleLineOutput("package changed") {
		t.Fatalf("second read = %#v", second)
	}
}

func TestToolExecutorExecuteDoesNotReuseReadAfterSameFilePatch(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app.go) error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(first read) error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted || first.Output != expectedReadSingleLineOutput("package main") {
		t.Fatalf("first read = %#v", first)
	}

	patched, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-2",
		ToolName:   tool.ApplyPatchToolName,
		Arguments: json.RawMessage(`*** Begin Patch
*** Update File: app.go
@@
-package main
+package changed
*** End Patch
`),
	})
	if err != nil {
		t.Fatalf("Execute(apply_patch) error = %v", err)
	}
	if patched.Status != ToolExecutionStatusExecuted || strings.TrimSpace(patched.Error) != "" {
		t.Fatalf("apply_patch = %#v", patched)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-3",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(second read) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted ||
		second.Output != expectedReadSingleLineOutput("package changed") ||
		second.ReusedFromCallID != "" {
		t.Fatalf("second read = %#v", second)
	}
}

func TestToolExecutorExecuteDoesNotReuseReadAfterExternalSameTurnRewrite(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(first read) error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted || first.Output != expectedReadSingleLineOutput("package main") {
		t.Fatalf("first read = %#v", first)
	}
	if err := os.WriteFile(path, []byte("package changed\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(rewrite) error = %v", err)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-2",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(second read) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.Output != expectedReadSingleLineOutput("package changed") || second.ReusedFromCallID != "" {
		t.Fatalf("second read = %#v", second)
	}
}

func TestToolExecutorExecuteRunsReadAcrossTurnsWhenFileIsUnchanged(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app.go) error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(first read) error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted {
		t.Fatalf("first read = %#v", first)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-2",
		ToolCallID: "call-2",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":[" app.go "],"offset":null,"limit":null}`),
	})
	if err != nil {
		t.Fatalf("Execute(second read) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.Output != first.Output || second.ReusedFromCallID != "" {
		t.Fatalf("second read = %#v", second)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed) != 5 {
		t.Fatalf("events = %#v", replayed)
	}
	firstEnd, ok := replayed[2].Payload.(events.ToolExecEndPayload)
	if !ok {
		t.Fatalf("first end payload = %#v", replayed[2].Payload)
	}
	secondEnd, ok := replayed[4].Payload.(events.ToolExecEndPayload)
	if !ok {
		t.Fatalf("second end payload = %#v", replayed[5].Payload)
	}
	if len(firstEnd.ObservedResources) != 1 || len(secondEnd.ObservedResources) != 1 {
		t.Fatalf("observed resources = %#v / %#v", firstEnd.ObservedResources, secondEnd.ObservedResources)
	}
	if firstEnd.ObservedResources[0] != secondEnd.ObservedResources[0] {
		t.Fatalf("observed resources differ:\nfirst=%#v\nsecond=%#v", firstEnd.ObservedResources[0], secondEnd.ObservedResources[0])
	}
}

func TestToolExecutorExecuteDoesNotReuseReadAcrossTurnsAfterExternalRewrite(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app.go) error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(first read) error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted || first.Output != expectedReadSingleLineOutput("package main") {
		t.Fatalf("first read = %#v", first)
	}

	if err := os.WriteFile(path, []byte("package changed\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(rewrite) error = %v", err)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-2",
		ToolCallID: "call-2",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":[" app.go "],"offset":null,"limit":null}`),
	})
	if err != nil {
		t.Fatalf("Execute(second read) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.Output != expectedReadSingleLineOutput("package changed") {
		t.Fatalf("second read = %#v", second)
	}
}

func TestToolExecutorExecuteRunsReadAcrossTurnsAfterRestartFromPersistedState(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app.go) error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(first read) error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted {
		t.Fatalf("first read = %#v", first)
	}

	restartedSessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService(restart) error = %v", err)
	}
	restartedExecutor, err := NewToolExecutor(restartedSessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor(restart) error = %v", err)
	}

	second, err := restartedExecutor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-2",
		ToolCallID: "call-2",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":[" app.go "],"offset":null,"limit":null}`),
	})
	if err != nil {
		t.Fatalf("Execute(second read) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.Output != first.Output || second.ReusedFromCallID != "" {
		t.Fatalf("second read = %#v", second)
	}
}

func TestToolExecutorExecuteRunsReadAcrossTurnsAfterRestartWithCompactedSnapshot(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app.go) error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(first read) error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted {
		t.Fatalf("first read = %#v", first)
	}

	appendCompactedSessionSnapshotForTest(t, store, sessions, "session-1")

	restartedSessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService(restart) error = %v", err)
	}
	restartedExecutor, err := NewToolExecutor(restartedSessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor(restart) error = %v", err)
	}

	second, err := restartedExecutor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-2",
		ToolCallID: "call-2",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":[" app.go "],"offset":null,"limit":null}`),
	})
	if err != nil {
		t.Fatalf("Execute(second read) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.Output != first.Output || second.ReusedFromCallID != "" {
		t.Fatalf("second read = %#v", second)
	}
}

func TestToolExecutorExecuteRunsReadFromDelegatedChildTurn(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app.go) error = %v", err)
	}
	for _, input := range []CreateSessionInput{
		{SessionID: "session-parent", WorkspaceRoot: root},
		{SessionID: "session-child", WorkspaceRoot: root},
	} {
		if _, err := sessions.CreateSession(context.Background(), input); err != nil {
			t.Fatalf("CreateSession(%s) error = %v", input.SessionID, err)
		}
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-parent",
		TurnID:     "turn-parent",
		ToolCallID: "call-parent",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(parent read) error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted {
		t.Fatalf("parent read = %#v", first)
	}

	appendDelegatedHandoffForToolReuseTest(t, sessions, events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: "session-parent",
		ParentTurnID:    "turn-parent",
		ParentAgentID:   "planner",
		ChildSessionID:  "session-child",
		ChildTurnID:     "turn-child",
		ChildAgentID:    "reviewer",
		Task:            "inspect the implementation",
		ContextSummary:  "Read the code and return findings.",
		Model:           "openai/gpt-5",
	})

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-child",
		TurnID:     "turn-child",
		ToolCallID: "call-child",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(child read) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.Output != first.Output || second.ReusedFromCallID != "" || second.ReusedFromSessionID != "" || second.ReusedFromTurnID != "" {
		t.Fatalf("child read = %#v", second)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-child", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay(child) error = %v", err)
	}
	end, ok := replayed[len(replayed)-1].Payload.(events.ToolExecEndPayload)
	if !ok {
		t.Fatalf("child end payload = %#v", replayed[len(replayed)-1].Payload)
	}
	if end.ReusedFromCallID != "" || end.ReusedFromSessionID != "" || end.ReusedFromTurnID != "" {
		t.Fatalf("child end payload = %#v", end)
	}

	childState, err := sessions.Snapshot(context.Background(), "session-child")
	if err != nil {
		t.Fatalf("Snapshot(child) error = %v", err)
	}
	call := childState.Turns["turn-child"].ToolCalls["call-child"]
	if call == nil || call.ReusedFromCallID != "" || call.ReusedFromSessionID != "" || call.ReusedFromTurnID != "" {
		t.Fatalf("child call state = %#v", call)
	}
}

func TestToolExecutorExecuteRunsReadFromDelegatedChildTurnAfterRestartWithCompactedSnapshot(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app.go) error = %v", err)
	}
	for _, input := range []CreateSessionInput{
		{SessionID: "session-parent", WorkspaceRoot: root},
		{SessionID: "session-child", WorkspaceRoot: root},
	} {
		if _, err := sessions.CreateSession(context.Background(), input); err != nil {
			t.Fatalf("CreateSession(%s) error = %v", input.SessionID, err)
		}
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-parent",
		TurnID:     "turn-parent",
		ToolCallID: "call-parent",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(parent read) error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted {
		t.Fatalf("parent read = %#v", first)
	}
	appendDelegatedHandoffForToolReuseTest(t, sessions, events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: "session-parent",
		ParentTurnID:    "turn-parent",
		ParentAgentID:   "planner",
		ChildSessionID:  "session-child",
		ChildTurnID:     "turn-child",
		ChildAgentID:    "reviewer",
		Task:            "inspect the implementation",
		ContextSummary:  "Read the code and return findings.",
		Model:           "openai/gpt-5",
	})
	appendCompactedSessionSnapshotForTest(t, store, sessions, "session-parent")
	appendCompactedSessionSnapshotForTest(t, store, sessions, "session-child")

	restartedSessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService(restart) error = %v", err)
	}
	restartedExecutor, err := NewToolExecutor(restartedSessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor(restart) error = %v", err)
	}

	second, err := restartedExecutor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-child",
		TurnID:     "turn-child",
		ToolCallID: "call-child",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(child read) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.Output != first.Output || second.ReusedFromCallID != "" || second.ReusedFromSessionID != "" || second.ReusedFromTurnID != "" {
		t.Fatalf("child read = %#v", second)
	}
}

func TestToolExecutorExecuteDoesNotReuseDelegatedParentReadAfterExternalRewrite(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app.go) error = %v", err)
	}
	for _, input := range []CreateSessionInput{
		{SessionID: "session-parent", WorkspaceRoot: root},
		{SessionID: "session-child", WorkspaceRoot: root},
	} {
		if _, err := sessions.CreateSession(context.Background(), input); err != nil {
			t.Fatalf("CreateSession(%s) error = %v", input.SessionID, err)
		}
	}

	if _, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-parent",
		TurnID:     "turn-parent",
		ToolCallID: "call-parent",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	}); err != nil {
		t.Fatalf("Execute(parent read) error = %v", err)
	}
	appendDelegatedHandoffForToolReuseTest(t, sessions, events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: "session-parent",
		ParentTurnID:    "turn-parent",
		ParentAgentID:   "planner",
		ChildSessionID:  "session-child",
		ChildTurnID:     "turn-child",
		ChildAgentID:    "reviewer",
		Task:            "inspect the implementation",
		ContextSummary:  "Read the code and return findings.",
		Model:           "openai/gpt-5",
	})
	if err := os.WriteFile(path, []byte("package changed\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(rewrite) error = %v", err)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-child",
		TurnID:     "turn-child",
		ToolCallID: "call-child",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(child read) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.ReusedFromCallID != "" {
		t.Fatalf("child read = %#v", second)
	}
	if second.Output != expectedReadSingleLineOutput("package changed") {
		t.Fatalf("child read output = %q", second.Output)
	}
}

func TestToolExecutorExecuteDoesNotReuseDelegatedParentReadWithoutObservedResources(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app.go) error = %v", err)
	}
	for _, input := range []CreateSessionInput{
		{SessionID: "session-parent", WorkspaceRoot: root},
		{SessionID: "session-child", WorkspaceRoot: root},
	} {
		if _, err := sessions.CreateSession(context.Background(), input); err != nil {
			t.Fatalf("CreateSession(%s) error = %v", input.SessionID, err)
		}
	}

	for _, draft := range []events.Draft{
		{
			SessionID: "session-parent",
			TurnID:    "turn-parent",
			Type:      events.TypeToolCallDeclared,
			Payload: events.ToolCallDeclaredPayload{
				CallID:   "call-parent",
				ToolName: tool.ReadToolName,
				Input:    `{"paths":["app.go"]}`,
			},
		},
		{
			SessionID: "session-parent",
			TurnID:    "turn-parent",
			Type:      events.TypeToolExecEnd,
			Payload: events.ToolExecEndPayload{
				CallID:    "call-parent",
				ToolName:  tool.ReadToolName,
				Succeeded: true,
				Output:    expectedReadSingleLineOutput("package main"),
			},
		},
	} {
		if _, err := sessions.append(context.Background(), draft); err != nil {
			t.Fatalf("append parent call(%s) error = %v", draft.Type, err)
		}
	}
	appendDelegatedHandoffForToolReuseTest(t, sessions, events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: "session-parent",
		ParentTurnID:    "turn-parent",
		ParentAgentID:   "planner",
		ChildSessionID:  "session-child",
		ChildTurnID:     "turn-child",
		ChildAgentID:    "reviewer",
		Task:            "inspect the implementation",
		ContextSummary:  "Read the code and return findings.",
		Model:           "openai/gpt-5",
	})

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-child",
		TurnID:     "turn-child",
		ToolCallID: "call-child",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(child read) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.ReusedFromCallID != "" {
		t.Fatalf("child read = %#v", second)
	}
}

func TestToolExecutorExecuteRunsSearchAcrossCanonicalArgs(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewSearchTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tasks.txt"), []byte("TODO ship it\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(tasks.txt) error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.SearchToolName,
		Arguments:  json.RawMessage(`{"query":"TODO","path":"."}`),
	})
	if err != nil {
		t.Fatalf("Execute(first search) error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted {
		t.Fatalf("first search = %#v", first)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-2",
		ToolName:   tool.SearchToolName,
		Arguments:  json.RawMessage(`{"path":".","query":"TODO","mode":"lexical","glob":"","regex":false,"case_sensitive":false,"max_matches":200}`),
	})
	if err != nil {
		t.Fatalf("Execute(second search) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.Output != first.Output || second.ReusedFromCallID != "" {
		t.Fatalf("second search = %#v", second)
	}
}

func TestToolExecutorExecuteRunsSearchNoMatchFromDelegatedParentTurn(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewSearchTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tasks.txt"), []byte("ship it\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(tasks.txt) error = %v", err)
	}
	for _, input := range []CreateSessionInput{
		{SessionID: "session-parent", WorkspaceRoot: root},
		{SessionID: "session-child", WorkspaceRoot: root},
	} {
		if _, err := sessions.CreateSession(context.Background(), input); err != nil {
			t.Fatalf("CreateSession(%s) error = %v", input.SessionID, err)
		}
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-parent",
		TurnID:     "turn-parent",
		ToolCallID: "call-parent",
		ToolName:   tool.SearchToolName,
		Arguments:  json.RawMessage(`{"path":".","query":"MISSING","mode":"lexical"}`),
	})
	if err != nil {
		t.Fatalf("Execute(parent search) error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted {
		t.Fatalf("parent search = %#v", first)
	}
	appendDelegatedHandoffForToolReuseTest(t, sessions, events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: "session-parent",
		ParentTurnID:    "turn-parent",
		ParentAgentID:   "planner",
		ChildSessionID:  "session-child",
		ChildTurnID:     "turn-child",
		ChildAgentID:    "reviewer",
		Task:            "inspect the implementation",
		ContextSummary:  "Read the code and return findings.",
		Model:           "openai/gpt-5",
	})

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-child",
		TurnID:     "turn-child",
		ToolCallID: "call-child",
		ToolName:   tool.SearchToolName,
		Arguments:  json.RawMessage(`{"path":".","query":"MISSING","mode":"lexical"}`),
	})
	if err != nil {
		t.Fatalf("Execute(child search) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.Output != first.Output || second.ReusedFromCallID != "" {
		t.Fatalf("child search = %#v", second)
	}
}

func TestToolExecutorExecuteRunsLocateAcrossCanonicalArgs(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewLocateTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatalf("MkdirAll(src) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(src/main.go) error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.LocateToolName,
		Arguments:  json.RawMessage(`{"query":"*.go","path":"."}`),
	})
	if err != nil {
		t.Fatalf("Execute(first locate) error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted {
		t.Fatalf("first locate = %#v", first)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-2",
		ToolName:   tool.LocateToolName,
		Arguments:  json.RawMessage(`{"path":".","query":"*.go","include_hidden":"False","max_matches":"200"}`),
	})
	if err != nil {
		t.Fatalf("Execute(second locate) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.Output != first.Output || second.ReusedFromCallID != "" {
		t.Fatalf("second locate = %#v", second)
	}
}

func TestToolExecutorExecutePersistsExecutionRuntimeMetadata(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, stubTool{
		definition: tool.Definition{
			Name:        "bash",
			Description: "Run command",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
		result: tool.Result{
			Output: "ok",
			Execution: &tool.ExecutionRuntime{
				Backend: "process",
			},
		},
	})
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	_, err = executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
		Arguments:  json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	end, ok := replayed[len(replayed)-1].Payload.(events.ToolExecEndPayload)
	if !ok {
		t.Fatalf("payload = %#v", replayed[len(replayed)-1].Payload)
	}
	if end.Backend != "process" {
		t.Fatalf("runtime payload = %#v", end)
	}
}

func TestToolExecutorExecuteQuestionToolRequestsAndResumes(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewQuestionTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	args := json.RawMessage(`{"question":"Which path should I use?","options":["Use runtime","Use prompt"]}`)
	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.QuestionToolName,
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute(first) error = %v", err)
	}
	if first.Status != ToolExecutionStatusPending || first.PendingRequestID == "" {
		t.Fatalf("first result = %#v", first)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed) != 3 || replayed[1].Type != events.TypeToolExecStart || replayed[2].Type != events.TypeQuestionRequested {
		t.Fatalf("events = %#v", replayed)
	}

	if _, err := sessions.AnswerQuestion(context.Background(), AnswerQuestionInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		RequestID: first.PendingRequestID,
		Answer:    "Use runtime",
	}); err != nil {
		t.Fatalf("AnswerQuestion() error = %v", err)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.QuestionToolName,
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute(second) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.Output != `{"answer":"Use runtime"}` {
		t.Fatalf("second result = %#v", second)
	}

	replayed, err = store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed) != 6 || replayed[3].Type != events.TypeQuestionAnswered || replayed[4].Type != events.TypeToolExecStart || replayed[5].Type != events.TypeToolExecEnd {
		t.Fatalf("events after resume = %#v", replayed)
	}
}

func TestToolExecutorRejectsPlannerSaveQuestionWithoutVisiblePlan(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewQuestionTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	args := json.RawMessage(`{"question":"Save or revise the plan?","options":["Save plan","Revise plan"],"purpose":"planner_save_plan"}`)
	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.QuestionToolName,
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.Error != plannerSavePlanQuestionRequiresVisiblePlanText {
		t.Fatalf("result = %#v", result)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	for _, event := range replayed {
		if event.Type == events.TypeQuestionRequested {
			t.Fatalf("question should not be requested when the plan is not visible: %#v", replayed)
		}
	}
	if len(replayed) == 0 || replayed[len(replayed)-1].Type != events.TypeToolExecEnd {
		t.Fatalf("events = %#v, want tool_exec_end", replayed)
	}
}

func TestToolExecutorRejectsBroadQuestionThatIncludesSavePlanOption(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewQuestionTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	args := json.RawMessage(`{"question":"Choose next action","options":["Implement top-priority quick wins now","Save the performance review & execution plan to .kodacode/plans/perf-review-plan.md","Generate a rollout checklist","Do nothing further"]}`)
	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.QuestionToolName,
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.Error != plannerSavePlanQuestionInvalidText {
		t.Fatalf("result = %#v", result)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	for _, event := range replayed {
		if event.Type == events.TypeQuestionRequested {
			t.Fatalf("invalid planner save question should not be requested: %#v", replayed)
		}
	}
}

func TestToolExecutorRejectsPlannerSaveQuestionWithExtraOptions(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewQuestionTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := sessions.append(context.Background(), events.Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      events.TypeAssistantCommit,
		Payload: events.AssistantCommitPayload{
			Content: "# Plan\n",
		},
	}); err != nil {
		t.Fatalf("append(assistant) error = %v", err)
	}

	args := json.RawMessage(`{"question":"Choose next action","options":["Save plan","Revise plan","Implement now"],"purpose":"planner_save_plan"}`)
	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.QuestionToolName,
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.Error != plannerSavePlanQuestionInvalidText {
		t.Fatalf("result = %#v", result)
	}
}

func TestToolExecutorExecuteQuestionToolDoesNotReuseAnswerAcrossTurns(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewQuestionTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	args := json.RawMessage(`{"question":"Which path should I use?","options":["Use runtime","Use prompt"]}`)
	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.QuestionToolName,
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute(turn-1) error = %v", err)
	}
	if _, err := sessions.AnswerQuestion(context.Background(), AnswerQuestionInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		RequestID: first.PendingRequestID,
		Answer:    "Use runtime",
	}); err != nil {
		t.Fatalf("AnswerQuestion() error = %v", err)
	}
	if _, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.QuestionToolName,
		Arguments:  args,
	}); err != nil {
		t.Fatalf("Execute(turn-1 resume) error = %v", err)
	}

	next, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-2",
		ToolCallID: "call-1",
		ToolName:   tool.QuestionToolName,
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute(turn-2) error = %v", err)
	}
	if next.Status != ToolExecutionStatusPending || next.PendingRequestID == "" {
		t.Fatalf("turn-2 result = %#v", next)
	}
}

func TestToolExecutorExecuteQuestionToolMarksRepeatedFailureAsRetry(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewQuestionTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	args := json.RawMessage(`{"question":"Which path should I use?","options":[]}`)
	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.QuestionToolName,
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute(first) error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted || first.Error == "" {
		t.Fatalf("first result = %#v", first)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-2",
		ToolName:   tool.QuestionToolName,
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute(second) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.Error == "" {
		t.Fatalf("second result = %#v", second)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	call := state.Turns["turn-1"].ToolCalls["call-2"]
	if call == nil || call.RetryOfCallID != "call-1" {
		t.Fatalf("call-2 state = %#v", call)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	last, ok := replayed[len(replayed)-1].Payload.(events.ToolExecEndPayload)
	if !ok {
		t.Fatalf("last payload = %#v", replayed[len(replayed)-1].Payload)
	}
	if last.RetryOfCallID != "call-1" {
		t.Fatalf("RetryOfCallID = %q, want call-1", last.RetryOfCallID)
	}
}

func TestToolExecutorExecuteDelegateToolPersistsHandoffID(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewDelegateTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	runtime := &stubDelegateRuntime{
		result: DelegateSessionTurnResult{
			HandoffID:      "handoff-1",
			ChildSessionID: "session-child",
			ChildTurn: RunSessionResult{
				TurnID:        "turn-child",
				Status:        TurnRunStatusCompleted,
				AssistantText: "child complete",
			},
		},
	}
	executor.SetDelegateRuntime(runtime)

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := sessions.append(context.Background(), events.Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      events.TypeTurnConfigured,
		Payload: newTurnConfiguredPayload(
			TurnCapabilities{
				AgentID:      "engineer",
				ModelRoute:   baseModelRoute(),
				AllowedTools: []string{tool.DelegateToolName},
			},
			nil,
			"",
			false,
			false,
			"",
			ResponseStyleDefault,
			false,
		),
	}); err != nil {
		t.Fatalf("append(turn_configured) error = %v", err)
	}

	args := json.RawMessage(`{"agent_id":"reviewer","task":"Inspect the cache layer","context_summary":"Need a focused review."}`)
	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.DelegateToolName,
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v, want executed", result)
	}
	if runtime.last.ParentSessionID != "session-1" || runtime.last.ParentTurnID != "turn-1" || runtime.last.ParentToolCallID != "call-1" {
		t.Fatalf("delegate input = %#v", runtime.last)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	if call == nil || call.HandoffID != "handoff-1" {
		t.Fatalf("call state = %#v, want handoff-1", call)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	last, ok := replayed[len(replayed)-1].Payload.(events.ToolExecEndPayload)
	if !ok {
		t.Fatalf("last payload = %#v", replayed[len(replayed)-1].Payload)
	}
	if last.HandoffID != "handoff-1" {
		t.Fatalf("HandoffID = %q, want handoff-1", last.HandoffID)
	}
}

func TestToolExecutorExecuteDelegateToolReturnsPendingForChildQuestion(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewDelegateTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	runtime := &stubDelegateRuntime{
		result: DelegateSessionTurnResult{
			HandoffID:      "handoff-1",
			ChildSessionID: "session-child",
			ChildTurn: RunSessionResult{
				TurnID:           "turn-child",
				Status:           TurnRunStatusPending,
				PendingRequestID: "question-1",
				PendingQuestion: &events.QuestionRequestState{
					QuestionID: "question-1",
					Question:   "Continue or stop this turn?",
					Options:    []string{"Continue", "Stop turn"},
				},
			},
		},
	}
	executor.SetDelegateRuntime(runtime)

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := sessions.append(context.Background(), events.Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      events.TypeTurnConfigured,
		Payload: newTurnConfiguredPayload(
			TurnCapabilities{
				AgentID:      "engineer",
				ModelRoute:   baseModelRoute(),
				AllowedTools: []string{tool.DelegateToolName},
			},
			nil,
			"",
			false,
			false,
			"",
			ResponseStyleDefault,
			false,
		),
	}); err != nil {
		t.Fatalf("append(turn_configured) error = %v", err)
	}

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.DelegateToolName,
		Arguments:  json.RawMessage(`{"agent_id":"reviewer","task":"Review the code","context_summary":"Stay grounded in the repo."}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusPending || result.PendingRequestID != "handoff-1" {
		t.Fatalf("result = %#v", result)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if got := replayed[len(replayed)-1].Type; got != events.TypeToolExecStart {
		t.Fatalf("last event type = %s, want tool_exec_start", got)
	}
	for _, event := range replayed {
		if event.Type == events.TypeToolExecEnd {
			t.Fatalf("replayed events unexpectedly contained tool_exec_end: %#v", replayed)
		}
	}
}

func TestSessionServiceAnswerQuestionRejectsAnswerOutsideOptions(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	requestID, err := sessions.RequestQuestion(context.Background(), QuestionRequestInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.QuestionToolName,
		Question:   "Which path should I use?",
		Options:    []string{"Use runtime", "Use prompt"},
	})
	if err != nil {
		t.Fatalf("RequestQuestion() error = %v", err)
	}

	_, err = sessions.AnswerQuestion(context.Background(), AnswerQuestionInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		RequestID: requestID,
		Answer:    "Something else",
	})
	if !errors.Is(err, ErrQuestionAnswerInvalid) {
		t.Fatalf("AnswerQuestion() error = %v, want ErrQuestionAnswerInvalid", err)
	}
}

func TestSessionServiceWatchStreamsToolExecutionEvents(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	watchCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := sessions.Watch(watchCtx, "session-1", 0)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	if _, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "read",
		Arguments:  json.RawMessage(`{"paths":["app.go"]}`),
	}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	first := mustReceiveEvent(t, stream)
	second := mustReceiveEvent(t, stream)
	if first.Type != events.TypeToolExecStart || second.Type != events.TypeToolExecEnd {
		t.Fatalf("event types = %q, %q", first.Type, second.Type)
	}
}

func mustReceiveEvent(t *testing.T, stream <-chan events.Event) events.Event {
	t.Helper()

	select {
	case event := <-stream:
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
		return events.Event{}
	}
}

func readDebugLog(t *testing.T, logDir string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(logDir, observability.DebugLogName))
	if err != nil {
		t.Fatalf("ReadFile(debug.log) error = %v", err)
	}
	return string(data)
}

type stubTool struct {
	definition tool.Definition
	result     tool.Result
}

func (s stubTool) Definition() tool.Definition {
	return s.definition
}

func (s stubTool) Execute(context.Context, tool.ExecutionContext, json.RawMessage) (tool.Result, error) {
	return s.result, nil
}
