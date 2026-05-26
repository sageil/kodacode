package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/textdiff"
	"github.com/sageil/kodacode/internal/tool"
)

func TestToolExecutorExecuteRunsAuthorizedWriteAndEmitsExecutionEvents(t *testing.T) {
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
	if _, err := sessions.CreateSession(context.TODO(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "write",
		Arguments:  json.RawMessage(`{"path":"notes.txt","content":"hello\n"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || !strings.HasPrefix(result.Output, "wrote 6 bytes to ") || !strings.HasSuffix(result.Output, "/notes.txt") {
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
	if len(replayed) != 3 || replayed[1].Type != events.TypeToolExecStart || replayed[2].Type != events.TypeToolExecEnd {
		t.Fatalf("events = %#v", replayed)
	}
}

func TestToolExecutorExecuteWriteRequiresApprovalInReadOnlyMode(t *testing.T) {
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
	if _, err := sessions.CreateSession(context.TODO(), CreateSessionInput{
		SessionID:      "session-1",
		WorkspaceRoot:  root,
		PermissionMode: PermissionModeReadOnly,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "write",
		Arguments:  json.RawMessage(`{"path":"notes.txt","content":"approved\n"}`),
	})
	if err != nil {
		t.Fatalf("Execute(first) error = %v", err)
	}
	if first.Status != ToolExecutionStatusExecuted || first.PendingRequestID != "" {
		t.Fatalf("first = %#v", first)
	}

	readResult, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-2",
		ToolCallID: "call-read",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["notes.txt"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(read) error = %v", err)
	}
	if readResult.Status != ToolExecutionStatusExecuted || readResult.Error != "" {
		t.Fatalf("readResult = %#v", readResult)
	}

	second, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-2",
		ToolCallID: "call-2",
		ToolName:   "write",
		Arguments:  json.RawMessage(`{"path":"notes.txt","content":"approved\n"}`),
	})
	if err != nil {
		t.Fatalf("Execute(second) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.PendingRequestID != "" || second.Error != "" {
		t.Fatalf("second = %#v", second)
	}

	content, err := os.ReadFile(filepath.Join(root, "notes.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "approved\n" {
		t.Fatalf("content = %q", string(content))
	}
}

func TestToolExecutorExecuteWriteRejectsExistingFileWithoutPriorRead(t *testing.T) {
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
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.TODO(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "write",
		Arguments:  json.RawMessage(`{"path":"notes.txt","content":"after\n"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || !strings.Contains(result.Error, "existing-file write replaces the entire file body") || result.FailureClass != toolFailureClassContract {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.Error, "Read the complete current file with read") {
		t.Fatalf("write error should guide model to read first: %#v", result)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "before\n" {
		t.Fatalf("content = %q", string(content))
	}

	replayed, err := store.Replay(context.TODO(), events.Query{SessionID: "session-1", AfterSequence: -1})
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
	if !strings.Contains(payload.Error, "existing-file write replaces the entire file body") || payload.Succeeded {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.FailureClass != toolFailureClassContract {
		t.Fatalf("payload failure class = %q", payload.FailureClass)
	}
}

func TestToolExecutorExecuteWriteAllowsExistingFileAfterRead(t *testing.T) {
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
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.TODO(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	readResult, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-read-1",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["notes.txt"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(read) error = %v", err)
	}
	if readResult.Status != ToolExecutionStatusExecuted || readResult.Error != "" {
		t.Fatalf("read result = %#v", readResult)
	}

	writeResult, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-write",
		ToolName:   "write",
		Arguments:  json.RawMessage(`{"path":"notes.txt","content":"after\n"}`),
	})
	if err != nil {
		t.Fatalf("Execute(write) error = %v", err)
	}
	if writeResult.Status != ToolExecutionStatusExecuted || writeResult.Error != "" {
		t.Fatalf("write result = %#v", writeResult)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "after\n" {
		t.Fatalf("content = %q", string(content))
	}
}

func TestToolExecutorExecuteWriteAllowsExistingFileLiteralOmissionLikeText(t *testing.T) {
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
	path := filepath.Join(root, "notes.txt")
	before := "package main\nfunc keep() {}\n"
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.TODO(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	readResult, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-read",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["notes.txt"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(read) error = %v", err)
	}
	if readResult.Status != ToolExecutionStatusExecuted || readResult.Error != "" {
		t.Fatalf("readResult = %#v", readResult)
	}

	writeResult, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-write",
		ToolName:   tool.WriteToolName,
		Arguments:  json.RawMessage(`{"path":"notes.txt","content":"package main\n// ... rest of the file is unchanged\n"}`),
	})
	if err != nil {
		t.Fatalf("Execute(write) error = %v", err)
	}
	if writeResult.Status != ToolExecutionStatusExecuted || writeResult.Error != "" {
		t.Fatalf("writeResult = %#v", writeResult)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "package main\n// ... rest of the file is unchanged\n" {
		t.Fatalf("content = %q", string(content))
	}
}

func TestToolExecutorExecuteWriteRejectsExistingFileAfterPartialReadCoverage(t *testing.T) {
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
	path := filepath.Join(root, "notes.txt")
	before := "one\ntwo\nthree\nfour\n"
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.TODO(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	readResult, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-read",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["notes.txt"],"offset":1,"limit":2}`),
	})
	if err != nil {
		t.Fatalf("Execute(read) error = %v", err)
	}
	if readResult.Status != ToolExecutionStatusExecuted || readResult.Error != "" {
		t.Fatalf("read result = %#v", readResult)
	}

	writeResult, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-write",
		ToolName:   "write",
		Arguments:  json.RawMessage(`{"path":"notes.txt","content":"patched\n"}`),
	})
	if err != nil {
		t.Fatalf("Execute(write) error = %v", err)
	}
	if writeResult.Status != ToolExecutionStatusExecuted || !strings.Contains(writeResult.Error, "available file coverage is incomplete") || writeResult.FailureClass != toolFailureClassContract {
		t.Fatalf("writeResult = %#v", writeResult)
	}
	if !strings.Contains(writeResult.Error, "Read the complete current file with read") {
		t.Fatalf("write error should guide model to read first: %#v", writeResult)
	}
	if !strings.Contains(writeResult.Error, "lines 2-3 of 4") {
		t.Fatalf("write error missing current coverage span: %#v", writeResult)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != before {
		t.Fatalf("content = %q", string(content))
	}
}

func TestToolExecutorExecuteWriteRejectsExistingFileAfterDisjointPartialReadCoverage(t *testing.T) {
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
	path := filepath.Join(root, "notes.txt")
	before := "one\ntwo\nthree\nfour\nfive\nsix\n"
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.TODO(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	for idx, args := range []string{
		`{"paths":["notes.txt"],"offset":0,"limit":2}`,
		`{"paths":["notes.txt"],"offset":4,"limit":2}`,
	} {
		readResult, err := executor.Execute(context.TODO(), ExecuteToolInput{
			SessionID:  "session-1",
			TurnID:     "turn-1",
			ToolCallID: "call-read-" + strconv.Itoa(idx+1),
			ToolName:   tool.ReadToolName,
			Arguments:  json.RawMessage(args),
		})
		if err != nil {
			t.Fatalf("Execute(read %s) error = %v", args, err)
		}
		if readResult.Status != ToolExecutionStatusExecuted || readResult.Error != "" {
			t.Fatalf("read result for %s = %#v", args, readResult)
		}
	}

	writeResult, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-write",
		ToolName:   tool.WriteToolName,
		Arguments:  json.RawMessage(`{"path":"notes.txt","content":"patched\n"}`),
	})
	if err != nil {
		t.Fatalf("Execute(write) error = %v", err)
	}
	if writeResult.Status != ToolExecutionStatusExecuted || !strings.Contains(writeResult.Error, "lines 1-2 and 5-6 of 6") || writeResult.FailureClass != toolFailureClassContract {
		t.Fatalf("writeResult = %#v", writeResult)
	}
}

func TestToolExecutorExecuteWriteAllowsExistingFileAfterCompleteReadCoverageAcrossWindows(t *testing.T) {
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
	path := filepath.Join(root, "notes.txt")
	var beforeBuilder strings.Builder
	var afterBuilder strings.Builder
	for i := 1; i <= 250; i++ {
		line := "line " + strconv.Itoa(i) + "\n"
		beforeBuilder.WriteString(line)
		if i == 200 {
			afterBuilder.WriteString("line 200 updated\n")
			continue
		}
		afterBuilder.WriteString(line)
	}
	before := beforeBuilder.String()
	after := afterBuilder.String()
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.TODO(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	for idx, args := range []string{
		`{"paths":["notes.txt"]}`,
		`{"paths":["notes.txt"],"offset":200,"limit":200}`,
	} {
		readResult, err := executor.Execute(context.TODO(), ExecuteToolInput{
			SessionID:  "session-1",
			TurnID:     "turn-1",
			ToolCallID: "call-read-" + strconv.Itoa(idx+1),
			ToolName:   tool.ReadToolName,
			Arguments:  json.RawMessage(args),
		})
		if err != nil {
			t.Fatalf("Execute(read %s) error = %v", args, err)
		}
		if readResult.Status != ToolExecutionStatusExecuted || readResult.Error != "" {
			t.Fatalf("read result for %s = %#v", args, readResult)
		}
	}

	writeResult, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-write",
		ToolName:   "write",
		Arguments:  json.RawMessage(`{"path":"notes.txt","content":` + strconv.Quote(after) + `}`),
	})
	if err != nil {
		t.Fatalf("Execute(write) error = %v", err)
	}
	if writeResult.Status != ToolExecutionStatusExecuted || writeResult.Error != "" {
		t.Fatalf("writeResult = %#v", writeResult)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != after {
		t.Fatalf("content mismatch after write")
	}
}

func TestToolExecutorExecuteWriteRejectsExistingFileWhenReadVersionIsStale(t *testing.T) {
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
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.TODO(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	for idx, args := range []string{
		`{"paths":["notes.txt"]}`,
		`{"paths":["notes.txt"],"offset":200,"limit":200}`,
		`{"paths":["notes.txt"],"offset":400,"limit":200}`,
	} {
		readResult, err := executor.Execute(context.TODO(), ExecuteToolInput{
			SessionID:  "session-1",
			TurnID:     "turn-1",
			ToolCallID: "call-read-" + strconv.Itoa(idx+1),
			ToolName:   tool.ReadToolName,
			Arguments:  json.RawMessage(args),
		})
		if err != nil {
			t.Fatalf("Execute(read %s) error = %v", args, err)
		}
		if readResult.Status != ToolExecutionStatusExecuted || readResult.Error != "" {
			t.Fatalf("read result for %s = %#v", args, readResult)
		}
	}

	if err := os.WriteFile(path, []byte("manual drift\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(drift) error = %v", err)
	}

	writeResult, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-write",
		ToolName:   "write",
		Arguments:  json.RawMessage(`{"path":"notes.txt","content":"after\n"}`),
	})
	if err != nil {
		t.Fatalf("Execute(write) error = %v", err)
	}
	if writeResult.Status != ToolExecutionStatusExecuted || !strings.Contains(writeResult.Error, "replacement context was captured") || writeResult.FailureClass != toolFailureClassContract {
		t.Fatalf("writeResult = %#v", writeResult)
	}
	if !strings.Contains(writeResult.Error, "Read the complete current file with read") {
		t.Fatalf("write error should guide model to read first: %#v", writeResult)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "manual drift\n" {
		t.Fatalf("content = %q", string(content))
	}
}

func TestToolExecutorExecuteWriteAllowsRepeatedWriteWithoutRereadAfterSuccessfulWrite(t *testing.T) {
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
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.TODO(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	readResult, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-read",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["notes.txt"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(read) error = %v", err)
	}
	if readResult.Status != ToolExecutionStatusExecuted || readResult.Error != "" {
		t.Fatalf("read result = %#v", readResult)
	}

	firstWrite, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-write-1",
		ToolName:   "write",
		Arguments:  json.RawMessage(`{"path":"notes.txt","content":"after one\n"}`),
	})
	if err != nil {
		t.Fatalf("Execute(first write) error = %v", err)
	}
	if firstWrite.Status != ToolExecutionStatusExecuted || firstWrite.Error != "" {
		t.Fatalf("firstWrite = %#v", firstWrite)
	}

	secondWrite, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-write-2",
		ToolName:   "write",
		Arguments:  json.RawMessage(`{"path":"notes.txt","content":"after two\n"}`),
	})
	if err != nil {
		t.Fatalf("Execute(second write) error = %v", err)
	}
	if secondWrite.Status != ToolExecutionStatusExecuted || secondWrite.Error != "" {
		t.Fatalf("secondWrite = %#v", secondWrite)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "after two\n" {
		t.Fatalf("content = %q", string(content))
	}
}

func TestToolExecutorExecuteWritePersistsWriteMutation(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("before\nold value\nafter\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.TODO(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	readResult, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-read",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["notes.txt"]}`),
	})
	if err != nil {
		t.Fatalf("Execute(read) error = %v", err)
	}
	if readResult.Status != ToolExecutionStatusExecuted || readResult.Error != "" {
		t.Fatalf("read result = %#v", readResult)
	}

	result, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "write",
		Arguments:  json.RawMessage(`{"path":"notes.txt","content":"before\nnew value\nafter\n"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.Error != "" {
		t.Fatalf("result = %#v", result)
	}

	replayed, err := store.Replay(context.TODO(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	payload, ok := replayed[len(replayed)-1].Payload.(events.ToolExecEndPayload)
	if !ok {
		t.Fatalf("payload = %#v", replayed[len(replayed)-1].Payload)
	}
	if payload.WriteMutation == nil {
		t.Fatal("write mutation = nil")
	}
	if !payload.WriteMutation.Existed {
		t.Fatalf("write mutation = %#v, want existed file", payload.WriteMutation)
	}
	if payload.WriteMutation.Before != "before\nold value\nafter\n" {
		t.Fatalf("write mutation before = %q", payload.WriteMutation.Before)
	}
	if !strings.HasSuffix(payload.WriteMutation.Path, "/notes.txt") {
		t.Fatalf("write mutation path = %q", payload.WriteMutation.Path)
	}
	if payload.WriteMutation.Mode == 0 {
		t.Fatalf("write mutation mode = %#v", payload.WriteMutation)
	}
	if payload.WriteMutation.DiffPreview == nil {
		t.Fatal("write mutation diff preview = nil")
	}
	added, removed := textdiff.LineStats(*payload.WriteMutation.DiffPreview)
	if added != 1 || removed != 1 {
		t.Fatalf("write mutation diff stats = +%d -%d", added, removed)
	}
	if len(payload.ObservedResources) != 1 {
		t.Fatalf("observed resources = %#v", payload.ObservedResources)
	}
	if !payload.ObservedResources[0].Complete || payload.ObservedResources[0].StartLine != 1 || payload.ObservedResources[0].EndLine != 3 || payload.ObservedResources[0].TotalLines != 3 {
		t.Fatalf("observed coverage = %#v", payload.ObservedResources[0])
	}

	state, err := sessions.Snapshot(context.TODO(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	if call == nil || call.WriteMutation == nil {
		t.Fatalf("call write mutation = %#v", call)
	}
	if call.WriteMutation.Before != "before\nold value\nafter\n" {
		t.Fatalf("call write mutation before = %q", call.WriteMutation.Before)
	}
	if call.WriteMutation.DiffPreview == nil {
		t.Fatal("call write mutation diff preview = nil")
	}
	if len(call.ObservedResources) != 1 {
		t.Fatalf("call observed resources = %#v", call.ObservedResources)
	}
}

func TestToolExecutorExecuteWritePersistsPreviewWhenBeforeIsOffloaded(t *testing.T) {
	store := events.NewMemoryStore()
	blobs := newTestSQLiteBlobStore(t)
	sessions, err := NewSessionServiceWithBlobs(store, blobs)
	if err != nil {
		t.Fatalf("NewSessionServiceWithBlobs() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool(), tool.NewWriteTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	before := strings.Repeat("same line\n", 260) + "old value\n" + strings.Repeat("same line\n", 260)
	after := strings.Repeat("same line\n", 260) + "new value\n" + strings.Repeat("same line\n", 260)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte(before), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.TODO(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	for idx, args := range []string{
		`{"paths":["notes.txt"]}`,
		`{"paths":["notes.txt"],"offset":200,"limit":200}`,
		`{"paths":["notes.txt"],"offset":400,"limit":200}`,
	} {
		readResult, err := executor.Execute(context.TODO(), ExecuteToolInput{
			SessionID:  "session-1",
			TurnID:     "turn-1",
			ToolCallID: "call-read-" + strconv.Itoa(idx+1),
			ToolName:   tool.ReadToolName,
			Arguments:  json.RawMessage(args),
		})
		if err != nil {
			t.Fatalf("Execute(read %s) error = %v", args, err)
		}
		if readResult.Status != ToolExecutionStatusExecuted || readResult.Error != "" {
			t.Fatalf("read result for %s = %#v", args, readResult)
		}
	}

	arguments, err := json.Marshal(map[string]string{
		"path":    "notes.txt",
		"content": after,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	result, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "write",
		Arguments:  arguments,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.Error != "" {
		t.Fatalf("result = %#v", result)
	}

	replayed, err := store.Replay(context.TODO(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	payload, ok := replayed[len(replayed)-1].Payload.(events.ToolExecEndPayload)
	if !ok {
		t.Fatalf("payload = %#v", replayed[len(replayed)-1].Payload)
	}
	if payload.WriteMutation == nil {
		t.Fatal("write mutation = nil")
	}
	if !payload.WriteMutation.BeforeTruncated || payload.WriteMutation.BeforeBlob == nil {
		t.Fatalf("write mutation offload = %#v", payload.WriteMutation)
	}
	if payload.WriteMutation.DiffPreview == nil {
		t.Fatal("write mutation diff preview = nil")
	}
	added, removed := textdiff.LineStats(*payload.WriteMutation.DiffPreview)
	if added != 1 || removed != 1 {
		t.Fatalf("write mutation diff stats = +%d -%d", added, removed)
	}
}
