package app

import (
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

func TestBuildTurnReplayIncludesExecutionToolResult(t *testing.T) {
	replayed := []events.Event{
		turnHistoryEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "inspect the repo"}),
		turnHistoryEvent(1, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-1",
			ToolName: "bash",
			Input:    `{"cmd":"printf 'hello\\n'"}`,
		}),
		turnHistoryEvent(2, "session-1", "turn-1", events.ExecutionDeclaredPayload{
			ExecutionID:      "exec-call-1",
			ToolCallID:       "call-1",
			ToolName:         "bash",
			Kind:             "bash",
			Command:          []string{"sh", "-lc", "printf 'hello\\n'"},
			WorkingDirectory: "/repo",
		}),
		turnHistoryEvent(3, "session-1", "turn-1", events.ToolExecEndPayload{
			CallID:          "call-1",
			ToolName:        "bash",
			ExecutionID:     "exec-call-1",
			ExecutionStatus: string(events.ExecutionStatusCompleted),
			Succeeded:       true,
			Output:          "hello\n",
		}),
		turnHistoryEvent(4, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-2",
			ToolName: "read",
			Input:    `{"paths":["/tmp/outside.txt"]}`,
		}),
		turnHistoryEvent(5, "session-1", "turn-1", events.PermissionRequestedPayload{
			Kind:       events.PermissionRequestKindPath,
			RequestID:  "perm-1",
			ToolCallID: "call-2",
			Access:     "read",
			Path:       "/tmp/outside.txt",
			ToolName:   "read",
			Command:    `read {"paths":["/tmp/outside.txt"]}`,
			Reason:     "requires external read access",
		}),
		turnHistoryEvent(6, "session-1", "turn-1", events.PermissionResolvedPayload{
			RequestID: "perm-1",
			Decision:  events.PermissionDecisionApproved,
			Scope:     events.PermissionScopeOnce,
		}),
	}

	history, err := buildTurnReplay(replayed, ResumeTurnInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		UserText:  "inspect the repo",
		RequestID: "perm-1",
	})
	if err != nil {
		t.Fatalf("buildTurnReplay() error = %v", err)
	}
	if len(history.Conversation) != 4 {
		t.Fatalf("conversation = %#v", history.Conversation)
	}
	if history.Conversation[1].Kind != provider.InputKindToolCall || history.Conversation[1].CallID != "call-1" {
		t.Fatalf("tool call = %#v", history.Conversation[1])
	}
	if history.Conversation[2].Kind != provider.InputKindToolResult || history.Conversation[2].Output != "hello\n" {
		t.Fatalf("tool result = %#v", history.Conversation[2])
	}
	if history.Conversation[3].Kind != provider.InputKindToolCall || history.Conversation[3].CallID != "call-2" {
		t.Fatalf("pending tool call = %#v", history.Conversation[3])
	}
	if history.PendingTool == nil || history.PendingTool.CallID != "call-2" {
		t.Fatalf("pending tool = %#v", history.PendingTool)
	}
}

func TestBuildTurnReplayOrdersBatchedCallsBeforeResultsWhenPending(t *testing.T) {
	replayed := []events.Event{
		turnHistoryEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "inspect files"}),
		turnHistoryEvent(1, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-1",
			ToolName: "read",
			Input:    `{"paths":["app.go"]}`,
		}),
		turnHistoryEvent(2, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-2",
			ToolName: "read",
			Input:    `{"paths":["/tmp/outside.txt"]}`,
		}),
		turnHistoryEvent(3, "session-1", "turn-1", events.ToolCallBatchPayload{
			CallIDs: []string{"call-1", "call-2"},
		}),
		turnHistoryEvent(4, "session-1", "turn-1", events.ToolExecEndPayload{
			CallID:    "call-1",
			ToolName:  "read",
			Succeeded: true,
			Output:    "1: package main\n",
		}),
		turnHistoryEvent(5, "session-1", "turn-1", events.PermissionRequestedPayload{
			Kind:       events.PermissionRequestKindPath,
			RequestID:  "perm-1",
			ToolCallID: "call-2",
			Access:     "read",
			Path:       "/tmp/outside.txt",
			ToolName:   "read",
			Command:    `read {"paths":["/tmp/outside.txt"]}`,
			Reason:     "requires external read access",
		}),
		turnHistoryEvent(6, "session-1", "turn-1", events.PermissionResolvedPayload{
			RequestID: "perm-1",
			Decision:  events.PermissionDecisionApproved,
			Scope:     events.PermissionScopeOnce,
		}),
	}

	history, err := buildTurnReplay(replayed, ResumeTurnInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		UserText:  "inspect files",
		RequestID: "perm-1",
	})
	if err != nil {
		t.Fatalf("buildTurnReplay() error = %v", err)
	}
	if len(history.Conversation) != 4 {
		t.Fatalf("conversation = %#v", history.Conversation)
	}
	if history.Conversation[1].Kind != provider.InputKindToolCall || history.Conversation[1].CallID != "call-1" {
		t.Fatalf("conversation[1] = %#v", history.Conversation[1])
	}
	if history.Conversation[2].Kind != provider.InputKindToolCall || history.Conversation[2].CallID != "call-2" {
		t.Fatalf("conversation[2] = %#v", history.Conversation[2])
	}
	if history.Conversation[3].Kind != provider.InputKindToolResult || history.Conversation[3].CallID != "call-1" {
		t.Fatalf("conversation[3] = %#v", history.Conversation[3])
	}
	if history.PendingTool == nil || history.PendingTool.CallID != "call-2" {
		t.Fatalf("pending tool = %#v", history.PendingTool)
	}
}

func TestBuildTurnReplaySynthesizesEmptySuccessfulToolResult(t *testing.T) {
	replayed := []events.Event{
		turnHistoryEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "inspect templates"}),
		turnHistoryEvent(1, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-1",
			ToolName: "list",
			Input:    `{"path":"src/routes/templates","include_hidden":false}`,
		}),
		turnHistoryEvent(2, "session-1", "turn-1", events.ToolExecEndPayload{
			CallID:    "call-1",
			ToolName:  "list",
			Succeeded: true,
		}),
		turnHistoryEvent(3, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-2",
			ToolName: "read",
			Input:    `{"paths":["README.md"]}`,
		}),
		turnHistoryEvent(4, "session-1", "turn-1", events.PermissionRequestedPayload{
			Kind:       events.PermissionRequestKindPath,
			RequestID:  "perm-1",
			ToolCallID: "call-2",
			Access:     "read",
			Path:       "/tmp/README.md",
			ToolName:   "read",
			Command:    `read {"paths":["README.md"]}`,
			Reason:     "requires external read access",
		}),
		turnHistoryEvent(5, "session-1", "turn-1", events.PermissionResolvedPayload{
			RequestID: "perm-1",
			Decision:  events.PermissionDecisionApproved,
			Scope:     events.PermissionScopeOnce,
		}),
	}

	history, err := buildTurnReplay(replayed, ResumeTurnInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		UserText:  "inspect templates",
		RequestID: "perm-1",
	})
	if err != nil {
		t.Fatalf("buildTurnReplay() error = %v", err)
	}
	if len(history.Conversation) != 4 {
		t.Fatalf("conversation = %#v", history.Conversation)
	}
	if history.Conversation[2].Kind != provider.InputKindToolResult || history.Conversation[2].Output != "completed successfully with no output" || history.Conversation[2].Error != "" {
		t.Fatalf("tool result = %#v", history.Conversation[2])
	}
}

func TestBuildTurnReplayPreservesUserAttachments(t *testing.T) {
	replayed := []events.Event{
		turnHistoryEvent(0, "session-1", "turn-1", events.UserMessagePayload{
			Content: "inspect this image",
			Attachments: []events.UserAttachmentPayload{{
				Name:     "pixel.png",
				MIMEType: "image/png",
				DataURL:  "data:image/png;base64,AA==",
			}},
		}),
		turnHistoryEvent(1, "session-1", "turn-1", events.QuestionRequestedPayload{
			QuestionID: "q-1",
			Question:   "Continue?",
		}),
		turnHistoryEvent(2, "session-1", "turn-1", events.QuestionAnsweredPayload{
			QuestionID: "q-1",
			Answer:     "yes",
		}),
	}

	history, err := buildTurnReplay(replayed, ResumeTurnInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		UserText:  "inspect this image",
		RequestID: "q-1",
	})
	if err != nil {
		t.Fatalf("buildTurnReplay() error = %v", err)
	}
	if len(history.Conversation) != 1 {
		t.Fatalf("conversation = %#v", history.Conversation)
	}
	if history.Conversation[0].Kind != provider.InputKindUserMessage {
		t.Fatalf("conversation[0] = %#v", history.Conversation[0])
	}
	if len(history.Conversation[0].Attachments) != 1 || history.Conversation[0].Attachments[0].Name != "pixel.png" {
		t.Fatalf("conversation[0].Attachments = %#v", history.Conversation[0].Attachments)
	}
}

func TestBuildTurnReplaySynthesizesStoredAttachmentOnlyUserInput(t *testing.T) {
	replayed := []events.Event{
		turnHistoryEvent(1, "session-1", "turn-1", events.QuestionRequestedPayload{
			QuestionID: "q-1",
			Question:   "Continue?",
		}),
		turnHistoryEvent(2, "session-1", "turn-1", events.QuestionAnsweredPayload{
			QuestionID: "q-1",
			Answer:     "yes",
		}),
	}

	history, err := buildTurnReplay(replayed, ResumeTurnInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Attachments: []provider.Attachment{{
			Name:     "pixel.png",
			MIMEType: "image/png",
			DataURL:  "data:image/png;base64,AA==",
		}},
		RequestID: "q-1",
	})
	if err != nil {
		t.Fatalf("buildTurnReplay() error = %v", err)
	}
	if len(history.Conversation) != 1 {
		t.Fatalf("conversation = %#v", history.Conversation)
	}
	if history.Conversation[0].Kind != provider.InputKindUserMessage || history.Conversation[0].Content != "" {
		t.Fatalf("conversation[0] = %#v", history.Conversation[0])
	}
	if len(history.Conversation[0].Attachments) != 1 || history.Conversation[0].Attachments[0].Name != "pixel.png" {
		t.Fatalf("conversation[0].Attachments = %#v", history.Conversation[0].Attachments)
	}
}

func TestBuildTurnReplayPreservesMalformedToolCallAndError(t *testing.T) {
	replayed := []events.Event{
		turnHistoryEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "inspect files"}),
		turnHistoryEvent(1, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-1",
			ToolName: "read",
			Input:    `{"paths":["README.md"]`,
		}),
		turnHistoryEvent(2, "session-1", "turn-1", events.ToolExecEndPayload{
			CallID:   "call-1",
			ToolName: "read",
			Error:    "`read` failed. JSON ended before the object was complete. Example: {\"paths\":[\"file.txt\"]}.",
		}),
		turnHistoryEvent(3, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-2",
			ToolName: "read",
			Input:    `{"paths":["/tmp/outside.txt"]}`,
		}),
		turnHistoryEvent(4, "session-1", "turn-1", events.PermissionRequestedPayload{
			Kind:       events.PermissionRequestKindPath,
			RequestID:  "perm-1",
			ToolCallID: "call-2",
			Access:     "read",
			Path:       "/tmp/outside.txt",
			ToolName:   "read",
			Command:    `read {"paths":["/tmp/outside.txt"]}`,
			Reason:     "requires external read access",
		}),
		turnHistoryEvent(5, "session-1", "turn-1", events.PermissionResolvedPayload{
			RequestID: "perm-1",
			Decision:  events.PermissionDecisionApproved,
			Scope:     events.PermissionScopeOnce,
		}),
	}

	history, err := buildTurnReplay(replayed, ResumeTurnInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		UserText:  "inspect files",
		RequestID: "perm-1",
	})
	if err != nil {
		t.Fatalf("buildTurnReplay() error = %v", err)
	}
	if len(history.Conversation) != 4 {
		t.Fatalf("conversation = %#v", history.Conversation)
	}
	if history.Conversation[1].Kind != provider.InputKindToolCall || history.Conversation[1].Arguments != `{"paths":["README.md"]` {
		t.Fatalf("conversation[1] = %#v", history.Conversation[1])
	}
	if history.Conversation[2].Kind != provider.InputKindToolResult {
		t.Fatalf("conversation[2] = %#v", history.Conversation[2])
	}
	if got := history.Conversation[2].Error; !strings.Contains(got, "`read` failed.") || !strings.Contains(got, "JSON ended before the object was complete") {
		t.Fatalf("conversation[2].Error = %q", got)
	}
	if history.Conversation[3].Kind != provider.InputKindToolCall || history.Conversation[3].CallID != "call-2" {
		t.Fatalf("conversation[3] = %#v", history.Conversation[3])
	}
}

func TestBuildTurnReplayResolvesDelegatedHandoffRequest(t *testing.T) {
	replayed := []events.Event{
		turnHistoryEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "perform a full code review"}),
		turnHistoryEvent(1, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-delegate",
			ToolName: tool.DelegateToolName,
			Input:    `{"agent_id":"reviewer","task":"Review the repo","context_summary":"Stay grounded in the code."}`,
		}),
		turnHistoryEvent(2, "session-1", "turn-1", events.AgentHandoffPayload{
			HandoffID:       "handoff-1",
			ToolCallID:      "call-delegate",
			ParentSessionID: "session-1",
			ParentTurnID:    "turn-1",
			ParentAgentID:   "planner",
			ChildSessionID:  "session-2",
			ChildTurnID:     "turn-2",
			ChildAgentID:    "reviewer",
			Task:            "Review the repo",
			ContextSummary:  "Stay grounded in the code.",
			Model:           "openai/gpt-5",
			AllowedTools:    []string{"read", "search"},
		}),
		turnHistoryEvent(3, "session-1", "turn-1", events.AgentResultPayload{
			HandoffID:         "handoff-1",
			ChildSessionID:    "session-2",
			ChildTurnID:       "turn-2",
			Status:            events.AgentResultStatusPendingQuestion,
			QuestionRequestID: "question-1",
			QuestionText:      "Continue or stop this turn?",
			QuestionOptions:   []string{"Continue", "Stop turn"},
		}),
	}

	history, err := buildTurnReplay(replayed, ResumeTurnInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		UserText:  "perform a full code review",
		RequestID: "handoff-1",
	})
	if err != nil {
		t.Fatalf("buildTurnReplay() error = %v", err)
	}
	if history.DelegatedHandoff == nil || history.DelegatedHandoff.HandoffID != "handoff-1" {
		t.Fatalf("delegated handoff = %#v", history.DelegatedHandoff)
	}
	if history.PendingTool == nil || history.PendingTool.CallID != "call-delegate" {
		t.Fatalf("pending tool = %#v", history.PendingTool)
	}
}

func TestBuildTurnReplayPreservesReusedToolResultMetadata(t *testing.T) {
	replayed := []events.Event{
		turnHistoryEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "inspect files"}),
		turnHistoryEvent(1, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-1",
			ToolName: "search",
			Input:    `{"query":"body","path":"."}`,
		}),
		turnHistoryEvent(2, "session-1", "turn-1", events.ToolExecEndPayload{
			CallID:    "call-1",
			ToolName:  "search",
			Succeeded: true,
			Output:    "body",
		}),
		turnHistoryEvent(3, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-2",
			ToolName: "search",
			Input:    `{"query":"body","path":"."}`,
		}),
		turnHistoryEvent(4, "session-1", "turn-1", events.ToolExecEndPayload{
			CallID:              "call-2",
			ToolName:            "search",
			ReusedFromCallID:    "call-1",
			ReusedFromSessionID: "session-parent",
			ReusedFromTurnID:    "turn-parent",
			Succeeded:           true,
			Output:              "body",
		}),
		turnHistoryEvent(5, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-3",
			ToolName: "read",
			Input:    `{"paths":["/tmp/outside.txt"]}`,
		}),
		turnHistoryEvent(6, "session-1", "turn-1", events.PermissionRequestedPayload{
			Kind:       events.PermissionRequestKindPath,
			RequestID:  "perm-1",
			ToolCallID: "call-3",
			Access:     "read",
			Path:       "/tmp/outside.txt",
			ToolName:   "read",
			Command:    `read {"paths":["/tmp/outside.txt"]}`,
			Reason:     "requires external read access",
		}),
		turnHistoryEvent(7, "session-1", "turn-1", events.PermissionResolvedPayload{
			RequestID: "perm-1",
			Decision:  events.PermissionDecisionApproved,
			Scope:     events.PermissionScopeOnce,
		}),
	}

	history, err := buildTurnReplay(replayed, ResumeTurnInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		UserText:  "inspect files",
		RequestID: "perm-1",
	})
	if err != nil {
		t.Fatalf("buildTurnReplay() error = %v", err)
	}
	if history.Conversation[4].Kind != provider.InputKindToolResult ||
		history.Conversation[4].ReusedFromCallID != "call-1" ||
		history.Conversation[4].ReusedFromSessionID != "session-parent" ||
		history.Conversation[4].ReusedFromTurnID != "turn-parent" {
		t.Fatalf("tool result = %#v", history.Conversation[4])
	}
}

func TestBuildTurnReplayUsesQuestionRequestToolCallID(t *testing.T) {
	replayed := []events.Event{
		turnHistoryEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "help me choose"}),
		turnHistoryEvent(1, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-1",
			ToolName: "read",
			Input:    `{"path":"one.txt"}`,
		}),
		turnHistoryEvent(2, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-2",
			ToolName: "question",
			Input:    `{"question":"Which path should I use?","options":["one","two"]}`,
		}),
		turnHistoryEvent(3, "session-1", "turn-1", events.QuestionRequestedPayload{
			QuestionID: "q-1",
			ToolCallID: "call-2",
			ToolName:   "question",
			Question:   "Which path should I use?",
			Options:    []string{"one", "two"},
		}),
		turnHistoryEvent(4, "session-1", "turn-1", events.QuestionAnsweredPayload{
			QuestionID: "q-1",
			ToolCallID: "call-2",
			Answer:     "one",
		}),
	}

	history, err := buildTurnReplay(replayed, ResumeTurnInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		UserText:  "help me choose",
		RequestID: "q-1",
	})
	if err != nil {
		t.Fatalf("buildTurnReplay() error = %v", err)
	}
	if history.PendingTool == nil || history.PendingTool.CallID != "call-2" {
		t.Fatalf("pending tool = %#v", history.PendingTool)
	}
	if history.QuestionRequest == nil || history.QuestionRequest.ToolCallID != "call-2" {
		t.Fatalf("question request = %#v", history.QuestionRequest)
	}
}

func TestBuildTurnReplayIncludesBackgroundRuntimeNote(t *testing.T) {
	replayed := []events.Event{
		turnHistoryEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "start the dev server"}),
		turnHistoryEvent(1, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-1",
			ToolName: "bash",
			Input:    `{"cmd":"npm run dev","workdir":"client"}`,
		}),
		turnHistoryEvent(2, "session-1", "turn-1", events.ExecutionDeclaredPayload{
			ExecutionID:      "exec-call-1",
			ToolCallID:       "call-1",
			ToolName:         "bash",
			Kind:             "bash",
			Intent:           "server",
			Command:          []string{"sh", "-lc", "npm run dev"},
			CommandPreview:   "npm run dev",
			WorkingDirectory: "/repo/client",
		}),
		turnHistoryEvent(3, "session-1", "turn-1", events.ToolExecEndPayload{
			CallID:          "call-1",
			ToolName:        "bash",
			ExecutionID:     "exec-call-1",
			ExecutionStatus: string(events.ExecutionStatusCompleted),
			Succeeded:       true,
			Output:          "Started server in background (pid 4242).",
		}),
		turnHistoryEvent(4, "session-1", "turn-1", events.ExecutionBackgroundLostPayload{
			ExecutionID: "exec-call-1",
			ToolCallID:  "call-1",
			ToolName:    "bash",
			Error:       "background process is still running, but supervision belonged to a different runtime instance",
		}),
		turnHistoryEvent(5, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-2",
			ToolName: "read",
			Input:    `{"paths":["/tmp/outside.txt"]}`,
		}),
		turnHistoryEvent(6, "session-1", "turn-1", events.PermissionRequestedPayload{
			Kind:       events.PermissionRequestKindPath,
			RequestID:  "perm-1",
			ToolCallID: "call-2",
			Access:     "read",
			Path:       "/tmp/outside.txt",
			ToolName:   "read",
			Command:    `read {"paths":["/tmp/outside.txt"]}`,
			Reason:     "requires external read access",
		}),
		turnHistoryEvent(7, "session-1", "turn-1", events.PermissionResolvedPayload{
			RequestID: "perm-1",
			Decision:  events.PermissionDecisionApproved,
			Scope:     events.PermissionScopeOnce,
		}),
	}

	history, err := buildTurnReplay(replayed, ResumeTurnInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		UserText:  "start the dev server",
		RequestID: "perm-1",
	})
	if err != nil {
		t.Fatalf("buildTurnReplay() error = %v", err)
	}
	if len(history.Conversation) != 5 {
		t.Fatalf("conversation = %#v", history.Conversation)
	}
	note := history.Conversation[3]
	if note.Kind != provider.InputKindAssistantMessage || !strings.Contains(note.Content, `Background command "npm run dev"`) || !strings.Contains(note.Content, "/repo/client") {
		t.Fatalf("runtime note = %#v", note)
	}
	if history.Conversation[4].Kind != provider.InputKindToolCall || history.Conversation[4].CallID != "call-2" {
		t.Fatalf("pending tool call = %#v", history.Conversation[4])
	}
}

func TestBuildTurnReplayMovesBackgroundRuntimeNoteAfterToolResult(t *testing.T) {
	replayed := []events.Event{
		turnHistoryEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "start the dev server"}),
		turnHistoryEvent(1, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-1",
			ToolName: "bash",
			Input:    `{"cmd":"npm run dev","workdir":"client"}`,
		}),
		turnHistoryEvent(2, "session-1", "turn-1", events.ExecutionDeclaredPayload{
			ExecutionID:      "exec-call-1",
			ToolCallID:       "call-1",
			ToolName:         "bash",
			Kind:             "bash",
			Intent:           "server",
			Command:          []string{"sh", "-lc", "npm run dev"},
			CommandPreview:   "npm run dev",
			WorkingDirectory: "/repo/client",
		}),
		turnHistoryEvent(3, "session-1", "turn-1", events.ExecutionBackgroundExitedPayload{
			ExecutionID: "exec-call-1",
			ToolCallID:  "call-1",
			ToolName:    "bash",
			ExitCode:    intPointer(0),
		}),
		turnHistoryEvent(4, "session-1", "turn-1", events.ToolExecEndPayload{
			CallID:          "call-1",
			ToolName:        "bash",
			ExecutionID:     "exec-call-1",
			ExecutionStatus: string(events.ExecutionStatusCompleted),
			Succeeded:       true,
			Output:          "(no output)",
		}),
		turnHistoryEvent(5, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-2",
			ToolName: "read",
			Input:    `{"paths":["/tmp/outside.txt"]}`,
		}),
		turnHistoryEvent(6, "session-1", "turn-1", events.PermissionRequestedPayload{
			Kind:       events.PermissionRequestKindPath,
			RequestID:  "perm-1",
			ToolCallID: "call-2",
			Access:     "read",
			Path:       "/tmp/outside.txt",
			ToolName:   "read",
			Command:    `read {"paths":["/tmp/outside.txt"]}`,
			Reason:     "requires external read access",
		}),
		turnHistoryEvent(7, "session-1", "turn-1", events.PermissionResolvedPayload{
			RequestID: "perm-1",
			Decision:  events.PermissionDecisionApproved,
			Scope:     events.PermissionScopeOnce,
		}),
	}

	history, err := buildTurnReplay(replayed, ResumeTurnInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		UserText:  "start the dev server",
		RequestID: "perm-1",
	})
	if err != nil {
		t.Fatalf("buildTurnReplay() error = %v", err)
	}
	if len(history.Conversation) != 5 {
		t.Fatalf("conversation = %#v", history.Conversation)
	}
	if history.Conversation[1].Kind != provider.InputKindToolCall || history.Conversation[1].CallID != "call-1" {
		t.Fatalf("tool call = %#v", history.Conversation[1])
	}
	if history.Conversation[2].Kind != provider.InputKindToolResult || history.Conversation[2].CallID != "call-1" {
		t.Fatalf("tool result = %#v", history.Conversation[2])
	}
	if history.Conversation[3].Kind != provider.InputKindAssistantMessage || !strings.Contains(history.Conversation[3].Content, `Background command "npm run dev"`) {
		t.Fatalf("runtime note = %#v", history.Conversation[3])
	}
	if history.Conversation[4].Kind != provider.InputKindToolCall || history.Conversation[4].CallID != "call-2" {
		t.Fatalf("pending tool call = %#v", history.Conversation[4])
	}
}

func turnHistoryEvent(sequence int64, sessionID, turnID string, payload events.Payload) events.Event {
	var eventType events.Type
	switch payload.(type) {
	case events.UserMessagePayload:
		eventType = events.TypeUserMessage
	case events.ToolCallDeclaredPayload:
		eventType = events.TypeToolCallDeclared
	case events.ToolCallBatchPayload:
		eventType = events.TypeToolCallBatch
	case events.ToolExecEndPayload:
		eventType = events.TypeToolExecEnd
	case events.ExecutionDeclaredPayload:
		eventType = events.TypeExecutionDeclared
	case events.ExecutionBackgroundReadyPayload:
		eventType = events.TypeExecutionBackgroundReady
	case events.ExecutionBackgroundExitedPayload:
		eventType = events.TypeExecutionBackgroundExited
	case events.ExecutionBackgroundLostPayload:
		eventType = events.TypeExecutionBackgroundLost
	case events.PermissionRequestedPayload:
		eventType = events.TypePermissionRequested
	case events.PermissionResolvedPayload:
		eventType = events.TypePermissionResolved
	case events.QuestionRequestedPayload:
		eventType = events.TypeQuestionRequested
	case events.QuestionAnsweredPayload:
		eventType = events.TypeQuestionAnswered
	case events.AgentHandoffPayload:
		eventType = events.TypeAgentHandoff
	case events.AgentResultPayload:
		eventType = events.TypeAgentResult
	default:
		panic("unsupported payload type")
	}
	return events.Event{
		ID:        turnID,
		SessionID: sessionID,
		TurnID:    turnID,
		Sequence:  sequence,
		Time:      time.Unix(sequence, 0).UTC(),
		Type:      eventType,
		Payload:   payload,
	}
}
