package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/permissionpolicy"
	"github.com/sageil/kodacode/internal/tool"
	"github.com/sageil/kodacode/internal/workspace"
)

func useExecutionRunnerHooks(
	t *testing.T,
	run func(context.Context, executionContract, executionRunOptions) (executionRunResult, error),
) {
	t.Helper()
	prevRun := runExecutionCommand
	if run == nil {
		run = runLocalExecutionCommand
	}
	runExecutionCommand = run
	t.Cleanup(func() {
		runExecutionCommand = prevRun
	})
}

func useLocalExecRunner(t *testing.T) {
	t.Helper()
	useExecutionRunnerHooks(t, runLocalExecutionCommand)
}

func useBackgroundExecutionRunnerHooks(
	t *testing.T,
	run func(context.Context, executionContract, executionBackgroundRunOptions) (executionBackgroundHandle, error),
) {
	t.Helper()
	prev := startBackgroundExecutionCommand
	if run == nil {
		run = startLocalBackgroundExecutionCommand
	}
	startBackgroundExecutionCommand = run
	t.Cleanup(func() {
		startBackgroundExecutionCommand = prev
	})
}

func TestToolExecutorExecuteExecCommandRequestsApprovalInReadOnlyMode(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:      "session-1",
		WorkspaceRoot:  root,
		PermissionMode: PermissionModeReadOnly,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	args, err := json.Marshal(map[string]any{
		"cmd":         "printf 'hello\\n'",
		"workdir":     nil,
		"prefix_rule": []string{"printf"},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusPending || result.PendingRequestID == "" {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	request := state.PendingExecutions[result.PendingRequestID]
	if request == nil {
		t.Fatalf("pending execution %q not found", result.PendingRequestID)
	}
	if request.Command != "printf 'hello\\n'" {
		t.Fatalf("request = %#v", request)
	}
	if len(request.PrefixRule) != 1 || request.PrefixRule[0] != "printf" {
		t.Fatalf("prefix rule = %#v", request.PrefixRule)
	}
}

func TestToolExecutorExecuteInvalidBashArgumentsPersistFriendlyError(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
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

	args := json.RawMessage(`{"cmd":["pwd"]}`)
	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.BashToolName,
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result.Status = %q", result.Status)
	}
	if got := result.Error; !strings.Contains(got, "`bash` failed.") || !strings.Contains(got, `Example: {"cmd":"git status"}.`) {
		t.Fatalf("result.Error = %q", got)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed) != 2 || replayed[1].Type != events.TypeToolExecEnd {
		t.Fatalf("events = %#v", replayed)
	}
	payload, ok := replayed[1].Payload.(events.ToolExecEndPayload)
	if !ok {
		t.Fatalf("payload = %#v", replayed[1].Payload)
	}
	if !strings.Contains(payload.Error, "`bash` failed.") || !strings.Contains(payload.Error, `Example: {"cmd":"git status"}.`) {
		t.Fatalf("payload.Error = %q", payload.Error)
	}
	if strings.Contains(payload.Error, "cannot unmarshal") {
		t.Fatalf("payload.Error leaked Go decode text: %q", payload.Error)
	}
}

func TestToolExecutorExecuteMalformedBashShellPersistsToolError(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
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

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.BashToolName,
		Arguments:  json.RawMessage(`{"cmd":"cat > out.txt <<'TS'\nmissing terminator\n"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.FailureClass != toolFailureClassInvalidArguments {
		t.Fatalf("result = %#v", result)
	}
	if result.TurnFailure != nil {
		t.Fatalf("TurnFailure = %v, want nil", result.TurnFailure)
	}
	if result.ErrorDetail == nil || result.ErrorDetail.Code != "tool_invalid_arguments" || !result.ErrorDetail.Retryable {
		t.Fatalf("ErrorDetail = %#v", result.ErrorDetail)
	}
	if !strings.Contains(result.Error, "unclosed here-document") {
		t.Fatalf("result.Error = %q, want heredoc parse detail", result.Error)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed) != 2 || replayed[1].Type != events.TypeToolExecEnd {
		t.Fatalf("events = %#v", replayed)
	}
	payload, ok := replayed[1].Payload.(events.ToolExecEndPayload)
	if !ok {
		t.Fatalf("payload = %#v", replayed[1].Payload)
	}
	if payload.FailureClass != toolFailureClassInvalidArguments {
		t.Fatalf("payload.FailureClass = %q", payload.FailureClass)
	}
	if payload.ErrorDetail == nil || payload.ErrorDetail.Code != "tool_invalid_arguments" {
		t.Fatalf("payload.ErrorDetail = %#v", payload.ErrorDetail)
	}
	if !strings.Contains(payload.Error, "unclosed here-document") {
		t.Fatalf("payload.Error = %q, want heredoc parse detail", payload.Error)
	}
}

func TestToolExecutorExecuteBashRerunsIdenticalCommandAfterWorkspaceChange(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	statePath := filepath.Join(root, "state.txt")
	if err := os.WriteFile(statePath, []byte("first"), 0o644); err != nil {
		t.Fatalf("WriteFile(statePath) error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	runCount := 0
	useExecutionRunnerHooks(t, func(_ context.Context, contract executionContract, _ executionRunOptions) (executionRunResult, error) {
		if len(contract.Command) == 0 {
			t.Fatalf("execution command = %#v", contract.Command)
		}
		runCount++
		data, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatalf("ReadFile(statePath) error = %v", err)
		}
		return executionRunResult{
			Output:  append([]byte(nil), data...),
			Backend: "test_backend",
		}, nil
	})

	args, err := json.Marshal(map[string]any{
		"cmd": "cat state.txt",
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.BashToolName,
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute(first) error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted || first.Output != "first" || first.Error != "" {
		t.Fatalf("first result = %#v", first)
	}

	if err := os.WriteFile(statePath, []byte("second"), 0o644); err != nil {
		t.Fatalf("WriteFile(statePath second) error = %v", err)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-2",
		ToolName:   tool.BashToolName,
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute(second) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.Output != "second" || second.Error != "" {
		t.Fatalf("second result = %#v", second)
	}
	if runCount != 2 {
		t.Fatalf("runCount = %d, want 2", runCount)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	found := false
	for _, event := range replayed {
		payload, ok := event.Payload.(events.ToolExecEndPayload)
		if !ok || payload.CallID != "call-2" {
			continue
		}
		found = true
		if payload.ReusedFromCallID != "" {
			t.Fatalf("ReusedFromCallID = %q, want empty", payload.ReusedFromCallID)
		}
		if payload.ExecutionID != "exec-call-2" {
			t.Fatalf("ExecutionID = %q, want exec-call-2", payload.ExecutionID)
		}
		if payload.ExecutionStatus != string(events.ExecutionStatusCompleted) {
			t.Fatalf("ExecutionStatus = %q, want completed", payload.ExecutionStatus)
		}
		if payload.Output != "second" || payload.Error != "" {
			t.Fatalf("payload = %#v", payload)
		}
	}
	if !found {
		t.Fatal("missing tool_exec_end for call-2")
	}
}

func TestToolExecutorExecuteSessionGrantDoesNotBypassDifferentExternalWorkingDirectory(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	externalA := t.TempDir()
	externalB := t.TempDir()
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:      "session-1",
		WorkspaceRoot:  root,
		PermissionMode: PermissionModeReadOnly,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	argsA, err := json.Marshal(map[string]any{
		"cmd":         "printf 'hello\\n'",
		"workdir":     externalA,
		"prefix_rule": []string{"printf"},
	})
	if err != nil {
		t.Fatalf("json.Marshal(argsA) error = %v", err)
	}
	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
		Arguments:  argsA,
	})
	if err != nil {
		t.Fatalf("Execute(first) error = %v", err)
	}
	if first.Status != ToolExecutionStatusPending || first.PendingRequestID == "" {
		t.Fatalf("first = %#v", first)
	}
	if _, err := sessions.ResolvePermission(context.Background(), ResolvePermissionInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		RequestID: first.PendingRequestID,
		Decision:  events.PermissionDecisionApproved,
		Scope:     events.PermissionScopeSession,
	}); err != nil {
		t.Fatalf("ResolvePermission() error = %v", err)
	}

	argsB, err := json.Marshal(map[string]any{
		"cmd":         "printf 'hello\\n'",
		"workdir":     externalB,
		"prefix_rule": []string{"printf"},
	})
	if err != nil {
		t.Fatalf("json.Marshal(argsB) error = %v", err)
	}
	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-2",
		ToolCallID: "call-2",
		ToolName:   "bash",
		Arguments:  argsB,
	})
	if err != nil {
		t.Fatalf("Execute(second) error = %v", err)
	}
	if second.Status != ToolExecutionStatusPending || second.PendingRequestID == "" {
		t.Fatalf("second = %#v", second)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	request := state.PendingExecutions[second.PendingRequestID]
	if request == nil {
		t.Fatalf("pending execution %q not found", second.PendingRequestID)
	}
	requestDir, err := filepath.EvalSymlinks(request.WorkingDirectory)
	if err != nil {
		t.Fatalf("EvalSymlinks(request.WorkingDirectory) error = %v", err)
	}
	externalDir, err := filepath.EvalSymlinks(externalB)
	if err != nil {
		t.Fatalf("EvalSymlinks(externalB) error = %v", err)
	}
	if requestDir != externalDir {
		t.Fatalf("working directory = %q, want %q", request.WorkingDirectory, externalB)
	}
	if len(state.ExecutionGrants) != 1 {
		t.Fatalf("execution grants = %#v", state.ExecutionGrants)
	}
	grant := state.ExecutionGrants[0]
	if len(grant.SessionPaths) != 1 {
		t.Fatalf("grant session paths = %#v", grant.SessionPaths)
	}
	grantDir, err := filepath.EvalSymlinks(grant.SessionPaths[0])
	if err != nil {
		t.Fatalf("EvalSymlinks(grant.SessionPaths[0]) error = %v", err)
	}
	externalGrantDir, err := filepath.EvalSymlinks(externalA)
	if err != nil {
		t.Fatalf("EvalSymlinks(externalA) error = %v", err)
	}
	if grantDir != externalGrantDir {
		t.Fatalf("grant session paths = %#v", grant.SessionPaths)
	}
}

func TestToolExecutorExecuteExecCommandAllowsAdditionalWorkspaceRoot(t *testing.T) {
	useExecutionRunnerHooks(t,
		func(_ context.Context, contract executionContract, _ executionRunOptions) (executionRunResult, error) {
			if contract.WorkingDirectory == "" {
				t.Fatal("working directory should be set")
			}
			return executionRunResult{
				Output:  []byte(contract.WorkingDirectory),
				Backend: "test_backend",
			}, nil
		},
	)

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	extra := t.TempDir()
	extraScope, err := workspace.New(extra)
	if err != nil {
		t.Fatalf("workspace.New(extra) error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:                "session-1",
		WorkspaceRoot:            root,
		AdditionalWorkspaceRoots: []string{extra},
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	args, err := json.Marshal(map[string]any{
		"cmd":     "pwd",
		"workdir": extra,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v", result)
	}
	if strings.TrimSpace(result.Output) != extraScope.Root() {
		t.Fatalf("Output = %q, want %q", result.Output, extraScope.Root())
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(state.PendingExecutionOrder) != 0 || len(state.PendingPermissionOrder) != 0 {
		t.Fatalf("pending approvals = %#v %#v", state.PendingExecutionOrder, state.PendingPermissionOrder)
	}
}

func TestToolExecutorExecuteExecCommandDoesNotReuseSessionGrantForDifferentCommand(t *testing.T) {
	ran := false
	useExecutionRunnerHooks(t, func(context.Context, executionContract, executionRunOptions) (executionRunResult, error) {
		ran = true
		return executionRunResult{}, nil
	})

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:      "session-1",
		WorkspaceRoot:  root,
		PermissionMode: PermissionModeReadOnly,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	args := json.RawMessage(`{"cmd":"printf 'hello\n'"}`)
	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute(first) error = %v", err)
	}
	if first.Status != ToolExecutionStatusPending || first.PendingRequestID == "" {
		t.Fatalf("first = %#v", first)
	}
	if _, err := sessions.ResolvePermission(context.Background(), ResolvePermissionInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		RequestID: first.PendingRequestID,
		Decision:  events.PermissionDecisionApproved,
		Scope:     events.PermissionScopeSession,
	}); err != nil {
		t.Fatalf("ResolvePermission() error = %v", err)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-2",
		ToolCallID: "call-2",
		ToolName:   "bash",
		Arguments:  json.RawMessage(`{"cmd":"printf 'goodbye\n'"}`),
	})
	if err != nil {
		t.Fatalf("Execute(second) error = %v", err)
	}
	if second.Status != ToolExecutionStatusPending || second.PendingRequestID == "" {
		t.Fatalf("second = %#v", second)
	}
	if ran {
		t.Fatal("runExecutionCommand() called before escalated approval")
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(state.ExecutionGrants) != 1 {
		t.Fatalf("execution grants = %#v", state.ExecutionGrants)
	}
	request := state.PendingExecutions[second.PendingRequestID]
	if request == nil || request.Command != "printf 'goodbye\n'" {
		t.Fatalf("request = %#v", request)
	}
}

func TestToolExecutorExecuteExecCommandUsesSessionRuleAfterApproval(t *testing.T) {
	useLocalExecRunner(t)

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	script := filepath.Join(root, "echo.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'approved\\n'\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", script, err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:      "session-1",
		WorkspaceRoot:  root,
		PermissionMode: PermissionModeReadOnly,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	args, err := json.Marshal(map[string]any{
		"cmd":     "./echo.sh",
		"workdir": nil,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute(first) error = %v", err)
	}
	if first.Status != ToolExecutionStatusPending || first.PendingRequestID == "" {
		t.Fatalf("result = %#v", first)
	}
	if _, err := sessions.ResolvePermission(context.Background(), ResolvePermissionInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		RequestID: first.PendingRequestID,
		Decision:  events.PermissionDecisionApproved,
		Scope:     events.PermissionScopeSession,
	}); err != nil {
		t.Fatalf("ResolvePermission() error = %v", err)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute(second) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.PendingRequestID != "" || second.Error != "" {
		t.Fatalf("result = %#v", second)
	}
	if second.Output == "" || second.Output == "(no output)" {
		t.Fatalf("result = %#v", second)
	}
}

func TestToolExecutorExecuteExecCommandAllowsLiteralCdInsideWorkspace(t *testing.T) {
	useLocalExecRunner(t)

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	clientDir := filepath.Join(root, "client")
	if err := os.MkdirAll(clientDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", clientDir, err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	args, err := json.Marshal(map[string]any{
		"cmd":     "printf 'root\\n' && cd client && printf 'child\\n'",
		"workdir": nil,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted || first.PendingRequestID != "" || first.Error != "" {
		t.Fatalf("result = %#v", first)
	}
	if got := strings.TrimSpace(first.Output); got != "root\nchild" {
		t.Fatalf("output = %q", first.Output)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(state.PendingExecutionOrder) != 0 || len(state.PendingPermissionOrder) != 0 {
		t.Fatalf("pending approvals = %#v %#v", state.PendingExecutionOrder, state.PendingPermissionOrder)
	}
	if len(state.ExecutionGrants) != 0 {
		t.Fatalf("execution grants = %#v", state.ExecutionGrants)
	}
}

func TestToolExecutorExecuteExecCommandRequestsPermissionForCdOutsideWorkspace(t *testing.T) {
	useLocalExecRunner(t)

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	outside := t.TempDir()
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	args, err := json.Marshal(map[string]any{
		"cmd":     "cd " + outside + " && pwd",
		"workdir": nil,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusPending || result.PendingRequestID == "" {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	decision, err := scope.Check(workspace.AccessWorkdir, outside)
	if err != nil {
		t.Fatalf("scope.Check() error = %v", err)
	}
	request := state.PendingPermissions[result.PendingRequestID]
	if request == nil {
		t.Fatalf("pending permission %q not found", result.PendingRequestID)
	}
	if request.Access != string(workspace.AccessWorkdir) || request.Path != decision.ResolvedPath {
		t.Fatalf("request = %#v", request)
	}
}

func TestToolExecutorExecuteExecCommandRequestsNetworkApprovalInAutoMode(t *testing.T) {
	useLocalExecRunner(t)

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
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

	args, err := json.Marshal(map[string]any{
		"cmd":     "printf 'ok\\n' # https://example.com",
		"workdir": nil,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusPending || result.PendingRequestID == "" {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	request := state.PendingExecutions[result.PendingRequestID]
	if request == nil {
		t.Fatalf("pending execution %q not found", result.PendingRequestID)
	}
	if request.ProposedNetworkPolicy == nil || !request.ProposedNetworkPolicy.Enabled {
		t.Fatalf("request = %#v", request)
	}
	if len(request.NetworkTargets) != 1 || request.NetworkTargets[0] != "example.com" {
		t.Fatalf("request = %#v", request)
	}
}

func TestToolExecutorExecuteExecCommandRunsWorkspaceScriptInAutoModeWithoutPreflightApproval(t *testing.T) {
	useLocalExecRunner(t)

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	script := filepath.Join(root, "probe.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'ok\\n'\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", script, err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	args := json.RawMessage(`{"cmd":"./probe.sh"}`)
	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.PendingRequestID != "" || result.Error != "" {
		t.Fatalf("result = %#v", result)
	}
	if result.Output != "ok" {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestToolExecutorExecuteExecCommandRunsSafeLocalCommandInAutoModeWithoutApproval(t *testing.T) {
	useLocalExecRunner(t)

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
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

	args := json.RawMessage(`{"cmd":"printf 'local-only\n'"}`)
	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.PendingRequestID != "" || result.Error != "" {
		t.Fatalf("result = %#v", result)
	}
	if result.Output != "local-only" {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestToolExecutorExecuteExecCommandFailsPipelineWhenHeadSucceeds(t *testing.T) {
	useLocalExecRunner(t)

	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutorWithConfig(sessions, ExecutionConfig{
		PermissionMode: PermissionModeAuto,
		Network:        ExecutionNetworkDisabled,
		ShellProgram:   bashPath,
	}, tool.NewBashTool())
	if err != nil {
		t.Fatalf("NewToolExecutorWithConfig() error = %v", err)
	}

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:      "session-1",
		WorkspaceRoot:  t.TempDir(),
		PermissionMode: PermissionModeFullAccess,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.BashToolName,
		Arguments:  json.RawMessage(`{"cmd":"false | head -1"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.Error == "" || result.Output != "" {
		t.Fatalf("result = %#v, want failed execution", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	if call == nil || call.Execution == nil || call.Execution.ExitCode == nil {
		t.Fatalf("call execution = %#v", call)
	}
	if *call.Execution.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", *call.Execution.ExitCode)
	}
	if call.Succeeded {
		t.Fatalf("call succeeded = true, want false")
	}
}

func TestToolExecutorExecuteExecCommandPreservesUserManagedPathForShellCommands(t *testing.T) {
	useLocalExecRunner(t)

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	toolDir := t.TempDir()
	npmPath := filepath.Join(toolDir, "npm")
	if err := os.WriteFile(npmPath, []byte("#!/bin/sh\nprintf 'npm-ok\\n'\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", npmPath, err)
	}
	t.Setenv("PATH", strings.Join([]string{toolDir, "/usr/bin", "/bin"}, string(os.PathListSeparator)))

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	args := json.RawMessage(`{"cmd":"npm run test:unit --silent"}`)
	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.PendingRequestID != "" || result.Error != "" {
		t.Fatalf("result = %#v", result)
	}
	if result.Output != "npm-ok" {
		t.Fatalf("output = %q", result.Output)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatalf("turn = %#v", turn)
	}
	call := turn.ToolCalls["call-1"]
	if call == nil || call.Execution == nil {
		t.Fatalf("call = %#v", call)
	}
	executionDir, err := filepath.EvalSymlinks(call.Execution.WorkingDirectory)
	if err != nil {
		t.Fatalf("EvalSymlinks(execution dir) error = %v", err)
	}
	rootDir, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks(root) error = %v", err)
	}
	if executionDir != rootDir {
		t.Fatalf("working directory = %q, want %q", call.Execution.WorkingDirectory, root)
	}
}

func TestToolExecutorExecuteExecCommandUsesSessionNetworkGrantAfterApproval(t *testing.T) {
	useLocalExecRunner(t)

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
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

	args := json.RawMessage(`{"cmd":"printf 'network-approved\n' # https://example.com","workdir":null}`)
	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute(first) error = %v", err)
	}
	if first.Status != ToolExecutionStatusPending || first.PendingRequestID == "" {
		t.Fatalf("first = %#v", first)
	}
	if _, err := sessions.ResolvePermission(context.Background(), ResolvePermissionInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		RequestID: first.PendingRequestID,
		Decision:  events.PermissionDecisionApproved,
		Scope:     events.PermissionScopeSession,
	}); err != nil {
		t.Fatalf("ResolvePermission() error = %v", err)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-2",
		ToolCallID: "call-2",
		ToolName:   "bash",
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute(second) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.PendingRequestID != "" || second.Error != "" {
		t.Fatalf("second = %#v", second)
	}
	if second.Output != "network-approved" {
		t.Fatalf("output = %q", second.Output)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(state.NetworkGrants) != 1 || state.NetworkGrants[0].Target != "example.com" {
		t.Fatalf("network grants = %#v", state.NetworkGrants)
	}
}

func TestToolExecutorExecuteExecCommandRequiresApprovalInReadOnlyMode(t *testing.T) {
	useLocalExecRunner(t)

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:      "session-1",
		WorkspaceRoot:  root,
		PermissionMode: PermissionModeReadOnly,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	args := json.RawMessage(`{"cmd":"printf 'read-only-approved\n'","workdir":null}`)
	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute(first) error = %v", err)
	}
	if first.Status != ToolExecutionStatusPending || first.PendingRequestID == "" {
		t.Fatalf("first = %#v", first)
	}
	if _, err := sessions.ResolvePermission(context.Background(), ResolvePermissionInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		RequestID: first.PendingRequestID,
		Decision:  events.PermissionDecisionApproved,
		Scope:     events.PermissionScopeSession,
	}); err != nil {
		t.Fatalf("ResolvePermission() error = %v", err)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-2",
		ToolCallID: "call-2",
		ToolName:   "bash",
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute(second) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.PendingRequestID != "" || second.Error != "" {
		t.Fatalf("second = %#v", second)
	}
	if second.Output != "read-only-approved" {
		t.Fatalf("output = %q", second.Output)
	}
}

func TestToolExecutorExecuteExecCommandAllowsByBashPolicyInReadOnlyMode(t *testing.T) {
	useLocalExecRunner(t)

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	if err := sessions.SetPermissionPolicy(permissionpolicy.Config{
		Bash: permissionpolicy.SubjectRules{
			{Pattern: "printf *", Action: permissionpolicy.ActionAllow},
		},
	}); err != nil {
		t.Fatalf("SetPermissionPolicy() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:      "session-1",
		WorkspaceRoot:  root,
		PermissionMode: PermissionModeReadOnly,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.BashToolName,
		Arguments:  json.RawMessage(`{"cmd":"printf 'policy-allowed\n'","workdir":null}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.PendingRequestID != "" || result.Error != "" {
		t.Fatalf("result = %#v", result)
	}
	if result.Output != "policy-allowed" {
		t.Fatalf("Output = %q", result.Output)
	}
}

func TestToolExecutorExecuteExecCommandAsksByBashPolicyInAutoMode(t *testing.T) {
	useLocalExecRunner(t)

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	if err := sessions.SetPermissionPolicy(permissionpolicy.Config{
		Bash: permissionpolicy.SubjectRules{
			{Pattern: "printf *", Action: permissionpolicy.ActionAsk},
		},
	}); err != nil {
		t.Fatalf("SetPermissionPolicy() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
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

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.BashToolName,
		Arguments:  json.RawMessage(`{"cmd":"printf 'policy-ask\n'","workdir":null}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusPending || result.PendingRequestID == "" {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	request := state.PendingExecutions[result.PendingRequestID]
	if request == nil {
		t.Fatalf("pending execution %q not found", result.PendingRequestID)
	}
	if request.Reason != "requires approval by permissions.bash policy" {
		t.Fatalf("Reason = %q", request.Reason)
	}
}

func TestToolExecutorExecuteExecCommandDeniesByBashPolicy(t *testing.T) {
	useLocalExecRunner(t)

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	if err := sessions.SetPermissionPolicy(permissionpolicy.Config{
		Bash: permissionpolicy.SubjectRules{
			{Pattern: "printf *", Action: permissionpolicy.ActionDeny},
		},
	}); err != nil {
		t.Fatalf("SetPermissionPolicy() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
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

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.BashToolName,
		Arguments:  json.RawMessage(`{"cmd":"printf 'blocked\n'","workdir":null}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.Error, "permissions.bash") {
		t.Fatalf("Error = %q", result.Error)
	}
}

func TestToolExecutorExecuteExecCommandAllowsExternalWorkdirByPolicy(t *testing.T) {
	useLocalExecRunner(t)

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	externalDir := t.TempDir()
	externalScope, err := workspace.New(externalDir)
	if err != nil {
		t.Fatalf("workspace.New(externalDir) error = %v", err)
	}
	if err := sessions.SetPermissionPolicy(permissionpolicy.Config{
		ExternalDirectory: permissionpolicy.SubjectRules{
			{Pattern: externalScope.Root(), Action: permissionpolicy.ActionAllow},
		},
	}); err != nil {
		t.Fatalf("SetPermissionPolicy() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
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

	args, err := json.Marshal(map[string]any{
		"cmd":     "pwd",
		"workdir": externalDir,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.BashToolName,
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.Error != "" {
		t.Fatalf("result = %#v", result)
	}
	if strings.TrimSpace(result.Output) != externalScope.Root() {
		t.Fatalf("Output = %q, want %q", result.Output, externalScope.Root())
	}
}

func TestToolExecutorExecuteExecCommandAllowsNetworkTargetByPolicy(t *testing.T) {
	useLocalExecRunner(t)

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	if err := sessions.SetPermissionPolicy(permissionpolicy.Config{
		NetworkTarget: permissionpolicy.SubjectRules{
			{Pattern: "example.com", Action: permissionpolicy.ActionAllow},
		},
	}); err != nil {
		t.Fatalf("SetPermissionPolicy() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
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

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.BashToolName,
		Arguments:  json.RawMessage(`{"cmd":"printf 'network-policy\n' # https://example.com","workdir":null}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.Error != "" {
		t.Fatalf("result = %#v", result)
	}
	if result.Output != "network-policy" {
		t.Fatalf("Output = %q", result.Output)
	}
}

func TestToolExecutorExecuteExecCommandAllowsExternalNetworkTargetByPolicy(t *testing.T) {
	useExecutionRunnerHooks(t, func(context.Context, executionContract, executionRunOptions) (executionRunResult, error) {
		return executionRunResult{
			Output:  []byte("external-network-policy"),
			Backend: "test_backend",
		}, nil
	})

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	if err := sessions.SetPermissionPolicy(permissionpolicy.Config{
		NetworkTarget: permissionpolicy.SubjectRules{
			{Pattern: "external network", Action: permissionpolicy.ActionAllow},
		},
	}); err != nil {
		t.Fatalf("SetPermissionPolicy() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
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

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.BashToolName,
		Arguments:  json.RawMessage(`{"cmd":"curl example.com >/dev/null","workdir":null}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.Error != "" {
		t.Fatalf("result = %#v", result)
	}
	if result.Output != "external-network-policy" {
		t.Fatalf("Output = %q", result.Output)
	}
}

func TestToolExecutorExecuteExecCommandAllowsPackageRegistriesTargetByPolicy(t *testing.T) {
	useExecutionRunnerHooks(t, func(context.Context, executionContract, executionRunOptions) (executionRunResult, error) {
		return executionRunResult{
			Output:  []byte("package-registries-policy"),
			Backend: "test_backend",
		}, nil
	})

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	if err := sessions.SetPermissionPolicy(permissionpolicy.Config{
		NetworkTarget: permissionpolicy.SubjectRules{
			{Pattern: "package registries", Action: permissionpolicy.ActionAllow},
		},
	}); err != nil {
		t.Fatalf("SetPermissionPolicy() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
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

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.BashToolName,
		Arguments:  json.RawMessage(`{"cmd":"npm install left-pad","workdir":null}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.Error != "" {
		t.Fatalf("result = %#v", result)
	}
	if result.Output != "package-registries-policy" {
		t.Fatalf("Output = %q", result.Output)
	}
}

func TestToolExecutorExecuteExecCommandBashAllowDoesNotBypassNetworkApproval(t *testing.T) {
	useLocalExecRunner(t)

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	if err := sessions.SetPermissionPolicy(permissionpolicy.Config{
		Bash: permissionpolicy.SubjectRules{
			{Pattern: "printf *", Action: permissionpolicy.ActionAllow},
		},
	}); err != nil {
		t.Fatalf("SetPermissionPolicy() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:      "session-1",
		WorkspaceRoot:  root,
		PermissionMode: PermissionModeReadOnly,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.BashToolName,
		Arguments:  json.RawMessage(`{"cmd":"printf 'still-needs-network\n' # https://example.com","workdir":null}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusPending || result.PendingRequestID == "" {
		t.Fatalf("result = %#v", result)
	}
}

func TestToolExecutorExecuteExecCommandDeniesNetworkTargetByPolicy(t *testing.T) {
	useLocalExecRunner(t)

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	if err := sessions.SetPermissionPolicy(permissionpolicy.Config{
		NetworkTarget: permissionpolicy.SubjectRules{
			{Pattern: "example.com", Action: permissionpolicy.ActionDeny},
		},
	}); err != nil {
		t.Fatalf("SetPermissionPolicy() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
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

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.BashToolName,
		Arguments:  json.RawMessage(`{"cmd":"printf 'blocked-network\n' # https://example.com","workdir":null}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.Error, "permissions.network_target") {
		t.Fatalf("Error = %q", result.Error)
	}
}

func TestToolExecutorExecuteExecCommandDeniesGitRemotesTargetByPolicy(t *testing.T) {
	useExecutionRunnerHooks(t, func(context.Context, executionContract, executionRunOptions) (executionRunResult, error) {
		t.Fatal("execution runner should not be used when policy denies")
		return executionRunResult{}, nil
	})

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	if err := sessions.SetPermissionPolicy(permissionpolicy.Config{
		NetworkTarget: permissionpolicy.SubjectRules{
			{Pattern: "git remotes", Action: permissionpolicy.ActionDeny},
		},
	}); err != nil {
		t.Fatalf("SetPermissionPolicy() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
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

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.BashToolName,
		Arguments:  json.RawMessage(`{"cmd":"git fetch origin","workdir":null}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.Error, "permissions.network_target") {
		t.Fatalf("Error = %q", result.Error)
	}
}

func TestToolExecutorExecuteExecCommandDenialPersistsExplicitlyInReplay(t *testing.T) {
	useLocalExecRunner(t)

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	if err := sessions.SetPermissionPolicy(permissionpolicy.Config{
		Bash: permissionpolicy.SubjectRules{
			{Pattern: "printf *", Action: permissionpolicy.ActionDeny},
		},
	}); err != nil {
		t.Fatalf("SetPermissionPolicy() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
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

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.BashToolName,
		Arguments:  json.RawMessage(`{"cmd":"printf 'blocked\n'","workdir":null}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || !strings.Contains(result.Error, "permissions.bash") {
		t.Fatalf("result = %#v", result)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed) == 0 {
		t.Fatal("replayed events = 0, want tool failure history")
	}
	last := replayed[len(replayed)-1]
	if last.Type != events.TypeToolExecEnd {
		t.Fatalf("last event type = %q, want tool_exec_end", last.Type)
	}
	payload, ok := last.Payload.(events.ToolExecEndPayload)
	if !ok {
		t.Fatalf("payload = %#v", last.Payload)
	}
	if !strings.Contains(payload.Error, "permissions.bash") {
		t.Fatalf("payload.Error = %q", payload.Error)
	}
}

func TestToolExecutorExecuteExecCommandRunsOutsideWorkspaceInFullAccessMode(t *testing.T) {
	useLocalExecRunner(t)

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	externalDir := t.TempDir()
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:      "session-1",
		WorkspaceRoot:  root,
		PermissionMode: PermissionModeFullAccess,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	args, err := json.Marshal(map[string]any{
		"cmd":     "pwd",
		"workdir": externalDir,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.PendingRequestID != "" || result.Error != "" {
		t.Fatalf("result = %#v", result)
	}
	resolvedExternalDir, err := filepath.EvalSymlinks(externalDir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s) error = %v", externalDir, err)
	}
	if result.Output != resolvedExternalDir {
		t.Fatalf("output = %q, want %q", result.Output, resolvedExternalDir)
	}
}

func TestToolExecutorExecuteRequestsApprovalForServerIntentInAutoMode(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	client := filepath.Join(root, "client")
	if err := os.MkdirAll(client, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(client, "package.json"), []byte(`{"scripts":{"dev":"vite --host 0.0.0.0 --port 5173"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	args := json.RawMessage(`{"cmd":"npm run dev","workdir":"client"}`)
	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusPending || result.PendingRequestID == "" {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	pending := state.PendingExecutions[result.PendingRequestID]
	if pending == nil {
		t.Fatalf("pending execution %q not found", result.PendingRequestID)
	}
	if pending.Reason != "requires approval to start a persistent local server" {
		t.Fatalf("reason = %q", pending.Reason)
	}
}

func TestToolExecutorExecuteRunsApprovedServerIntentInBackground(t *testing.T) {
	useExecutionRunnerHooks(t, func(context.Context, executionContract, executionRunOptions) (executionRunResult, error) {
		t.Fatal("foreground execution runner should not be used for server intent")
		return executionRunResult{}, nil
	})
	useBackgroundExecutionRunnerHooks(t, func(context.Context, executionContract, executionBackgroundRunOptions) (executionBackgroundHandle, error) {
		readyCh := make(chan executionBackgroundReadyEvent, 1)
		exitCh := make(chan executionBackgroundExitEvent, 1)
		readyCh <- executionBackgroundReadyEvent{
			Message: "Local: http://127.0.0.1:5173/",
			Port:    5173,
		}
		go func() {
			time.Sleep(10 * time.Millisecond)
			exitCh <- executionBackgroundExitEvent{
				RunResult: executionRunResult{
					Backend:  "background_process",
					ExitCode: intPointer(0),
				},
			}
			close(exitCh)
		}()
		return executionBackgroundHandle{
			PID:             4242,
			ProcessIdentity: "identity-4242",
			Ready:           readyCh,
			Exited:          exitCh,
		}, nil
	})

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewBashTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	executor.SetBackgroundLogStore(newTestSQLiteBackgroundLogStore(t))

	root := t.TempDir()
	client := filepath.Join(root, "client")
	if err := os.MkdirAll(client, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(client, "package.json"), []byte(`{"scripts":{"dev":"vite --host 0.0.0.0 --port 5173"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	args := json.RawMessage(`{"cmd":"npm run dev","workdir":"client"}`)
	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute(first) error = %v", err)
	}
	if first.Status != ToolExecutionStatusPending || first.PendingRequestID == "" {
		t.Fatalf("first = %#v", first)
	}
	if _, err := sessions.ResolvePermission(context.Background(), ResolvePermissionInput{
		SessionID:         "session-1",
		TurnID:            "turn-1",
		RequestID:         first.PendingRequestID,
		ExecutionDecision: events.ExecutionApprovalDecisionAccept,
	}); err != nil {
		t.Fatalf("ResolvePermission() error = %v", err)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute(second) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.Error != "" {
		t.Fatalf("second = %#v", second)
	}
	if !strings.Contains(second.Output, "Started server in background (pid 4242).") {
		t.Fatalf("output = %q", second.Output)
	}
	if !strings.Contains(second.Output, "Ready: Local: http://127.0.0.1:5173/") {
		t.Fatalf("output = %q", second.Output)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	if call == nil || call.Execution == nil || call.Execution.Background == nil {
		t.Fatalf("call = %#v", call)
	}
	if call.Execution.Intent != string(tool.ExecutionIntentServer) {
		t.Fatalf("intent = %q", call.Execution.Intent)
	}
	if !call.Execution.Background.Started || !call.Execution.Background.Ready || call.Execution.Background.PID != 4242 {
		t.Fatalf("background = %#v", call.Execution.Background)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	var sawStarted, sawReady, sawCompleted bool
	for _, event := range replayed {
		switch event.Type {
		case events.TypeExecutionBackgroundStarted:
			sawStarted = true
		case events.TypeExecutionBackgroundReady:
			sawReady = true
		case events.TypeToolExecEnd:
			sawCompleted = true
		}
	}
	if !sawStarted || !sawReady || !sawCompleted {
		t.Fatalf("background lifecycle events missing: started=%v ready=%v completed=%v", sawStarted, sawReady, sawCompleted)
	}
}
