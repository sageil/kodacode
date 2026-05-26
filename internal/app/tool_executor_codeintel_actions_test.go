package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func TestToolExecutorExecuteRenameSymbolPersistsResult(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewRenameSymbolTool())
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
			rename: func(request tool.CodeIntelRenameRequest) (tool.CodeIntelMutationSummary, error) {
				return tool.CodeIntelMutationSummary{
					Paths:     []string{request.Path, filepath.Join(root, "other.go")},
					TextEdits: 2,
				}, nil
			},
		},
	})

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.RenameSymbolToolName,
		Arguments:  json.RawMessage(`{"path":"main.go","line":3,"character":4,"new_name":"Store"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.Output, `Renamed symbol to "Store" across 2 file(s) with 2 text edit(s).`) {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestToolExecutorExecuteCodeActionPersistsResult(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewCodeActionTool())
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
			codeAction: func(request tool.CodeIntelCodeActionRequest) (tool.CodeIntelCodeActionResult, error) {
				return tool.CodeIntelCodeActionResult{
					Title: "Organize Imports",
					Summary: tool.CodeIntelMutationSummary{
						Paths:     []string{request.Path},
						TextEdits: 1,
					},
				}, nil
			},
		},
	})

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.CodeActionToolName,
		Arguments:  json.RawMessage(`{"path":"main.go","start_line":1,"start_character":0,"end_line":1,"end_character":10,"title":"Organize Imports","kind":"source.organizeImports","only_preferred":null}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.Output, `Applied code action "Organize Imports" across 1 file(s) with 1 text edit(s).`) {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestToolExecutorExecuteCodeActionNoticeDoesNotFailTool(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewCodeActionTool())
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
			codeAction: func(tool.CodeIntelCodeActionRequest) (tool.CodeIntelCodeActionResult, error) {
				return tool.CodeIntelCodeActionResult{}, tool.NewCodeIntelNoticeError(tool.CodeIntelNoticeKindUnsupported, "no code actions available. No edit was applied; use apply_patch for a manual source edit if the change is still needed")
			},
		},
	})

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.CodeActionToolName,
		Arguments:  json.RawMessage(`{"path":"main.go","start_line":1,"start_character":0,"end_line":1,"end_character":10,"kind":"quickfix"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.Error != "" {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.Output, "code_action unsupported: no code actions available") ||
		!strings.Contains(result.Output, "use apply_patch") {
		t.Fatalf("output = %q", result.Output)
	}
}
