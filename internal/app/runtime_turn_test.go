package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

func requestInputIndex(request provider.Request, match func(provider.Input) bool) int {
	for idx, input := range request.Inputs {
		if match(input) {
			return idx
		}
	}
	return -1
}

func requireLocalTestListener(t *testing.T) {
	t.Helper()
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Skipf("local test listener unavailable: %v", err)
	}
	_ = listener.Close()
}

func TestRuntimeRunSessionTurnCompletesOneShotTurn(t *testing.T) {
	runtime := newRuntimeWithClient(t, &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "hello"},
		})},
	})

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "say hello",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.SessionID == "" || result.TurnID == "" {
		t.Fatalf("result ids = %#v", result)
	}
	if result.Status != TurnRunStatusCompleted || result.AssistantText != "hello" {
		t.Fatalf("result = %#v", result)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.WorkspaceRoot == "" || state.Turns[result.TurnID].Status != events.TurnStatusCompleted {
		t.Fatalf("state = %#v", state)
	}
}

func TestRuntimeRunSessionTurnDefaultsThinkingDisabled(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "hello"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "say hello",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	if client.requests[0].ThinkingEnabled {
		t.Fatalf("provider request thinking_enabled = true, want false")
	}
	if client.requests[0].ThinkingMode != "" {
		t.Fatalf("provider request thinking_mode = %q, want empty", client.requests[0].ThinkingMode)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns[result.TurnID]
	if turn == nil || turn.Config == nil {
		t.Fatalf("turn = %#v", turn)
	}
	if turn.Config.ThinkingEnabled {
		t.Fatalf("stored turn thinking_enabled = true, want false")
	}
	if turn.Config.ThinkingMode != "" {
		t.Fatalf("stored turn thinking_mode = %q, want empty", turn.Config.ThinkingMode)
	}
}

func TestRuntimeRunSessionTurnAppliesConfiguredTerseResponseStyle(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "hello"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.Sessions.ResponseStyle = ResponseStyleTerse
	runtime.Runner.SetSessionConfig(runtime.Config.Sessions)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "say hello",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	if !strings.Contains(provider.PromptText(client.requests[0]), "Response style: terse.") {
		t.Fatalf("provider prompt = %q", provider.PromptText(client.requests[0]))
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns[result.TurnID]
	if turn == nil || turn.Config == nil {
		t.Fatalf("turn = %#v", turn)
	}
	if turn.Config.ResponseStyle != string(ResponseStyleTerse) {
		t.Fatalf("stored turn response_style = %q, want %q", turn.Config.ResponseStyle, ResponseStyleTerse)
	}
}

func TestRuntimeRunSessionTurnDefaultsToTerseResponseStyle(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "hello"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "say hello",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	if !strings.Contains(provider.PromptText(client.requests[0]), "Response style: terse.") {
		t.Fatalf("provider prompt = %q", provider.PromptText(client.requests[0]))
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns[result.TurnID]
	if turn == nil || turn.Config == nil {
		t.Fatalf("turn = %#v", turn)
	}
	if turn.Config.ResponseStyle != string(ResponseStyleTerse) {
		t.Fatalf("stored turn response_style = %q, want %q", turn.Config.ResponseStyle, ResponseStyleTerse)
	}
}

func TestRuntimeStartSessionTurnDefaultsThinkingDisabledWhenUnset(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "hello"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "say hello",
	})
	if err != nil {
		t.Fatalf("StartSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	if client.requests[0].ThinkingEnabled {
		t.Fatalf("provider request thinking_enabled = true, want false")
	}
	if client.requests[0].ThinkingMode != "" {
		t.Fatalf("provider request thinking_mode = %q, want empty", client.requests[0].ThinkingMode)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.Config == nil {
		t.Fatalf("turn = %#v", turn)
	}
	if turn.Config.ThinkingEnabled {
		t.Fatalf("stored turn thinking_enabled = true, want false")
	}
	if turn.Config.ThinkingMode != "" {
		t.Fatalf("stored turn thinking_mode = %q, want empty", turn.Config.ThinkingMode)
	}
}

func TestRuntimeStartSessionTurnHonorsExplicitVariant(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "hello"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
		SessionID:    sessionID,
		TurnID:       "turn-1",
		UserText:     "say hello",
		ThinkingMode: provider.ReasoningVariantHigh,
	})
	if err != nil {
		t.Fatalf("StartSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	if client.requests[0].ThinkingMode != provider.ReasoningVariantHigh {
		t.Fatalf("provider request thinking_mode = %q, want high", client.requests[0].ThinkingMode)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.Config == nil {
		t.Fatalf("turn = %#v", turn)
	}
	if turn.Config.ThinkingMode != provider.ReasoningVariantHigh {
		t.Fatalf("stored turn thinking_mode = %q, want high", turn.Config.ThinkingMode)
	}
}

func TestRuntimeStartSessionTurnDoesNotAdvertiseStaleMCPCatalog(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := runtime.Sessions.append(context.Background(), events.Draft{
		SessionID: sessionID,
		TurnID:    sessionTurnID,
		Type:      events.TypeSessionMCPCatalogUpdated,
		Payload: events.SessionMCPCatalogUpdatedPayload{
			WorkspaceTrusted: true,
			Servers: []events.SessionMCPServerPayload{{
				Name:        "sequential-thinking",
				Type:        "stdio",
				Fingerprint: "server-fingerprint",
				Trusted:     true,
				Active:      true,
			}},
			Tools: []events.SessionMCPToolPayload{{
				Name:        "mcp_sequential_thinking__sequentialthinking",
				Description: "stale tool",
				InputSchema: `{"type":"object"}`,
			}},
		},
	}); err != nil {
		t.Fatalf("append stale mcp catalog error = %v", err)
	}

	result, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "say hello",
	})
	if err != nil {
		t.Fatalf("StartSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	if got := client.requests[0].Instructions; strings.Contains(got, "mcp_sequential_thinking__sequentialthinking") || strings.Contains(got, "Active MCP integrations available for this turn:") {
		t.Fatalf("instructions advertised stale MCP tool: %q", got)
	}
	if gotTools := requestToolNames(client.requests[0].Tools); slices.Contains(gotTools, "mcp_sequential_thinking__sequentialthinking") {
		t.Fatalf("provider tools = %#v, want stale MCP tool omitted", gotTools)
	}

}

func TestRuntimeRunSessionTurnReturnsFailedResultFromDurableTurnError(t *testing.T) {
	runtime := newRuntimeWithClient(t, &fakeProvider{
		err: errors.New("provider unavailable"),
	})

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "say hello",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusFailed {
		t.Fatalf("result = %#v", result)
	}
	if result.Error != "The model is busy right now. Please try again in a moment." {
		t.Fatalf("result error = %q", result.Error)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns[result.TurnID]
	if turn == nil || turn.Status != events.TurnStatusFailed || turn.Error != "The model is busy right now. Please try again in a moment." {
		t.Fatalf("turn = %#v", turn)
	}
}

func TestRuntimeRunSessionTurnDoesNotDuplicateAttachmentOnlyMessageOnFailure(t *testing.T) {
	root := t.TempDir()
	attachmentPath := mustWriteTestPNG(t, root, "pixel.png")
	runtime := newRuntimeWithClient(t, &fakeProvider{
		err: errors.New("provider unavailable"),
	})

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		Attachments: []AttachmentInput{{
			Path: attachmentPath,
		}},
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusFailed {
		t.Fatalf("result = %#v", result)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns[result.TurnID]
	if turn == nil {
		t.Fatal("turn state missing")
	}
	if len(turn.UserAttachments) != 1 || turn.UserAttachments[0].Name != "pixel.png" {
		t.Fatalf("user attachments = %#v", turn.UserAttachments)
	}
	userEntries := 0
	for _, entry := range turn.Transcript {
		if entry.Kind == events.TranscriptEntryUser {
			userEntries++
		}
	}
	if userEntries != 1 {
		t.Fatalf("user transcript entries = %d, want 1; transcript = %#v", userEntries, turn.Transcript)
	}
}

func TestRuntimeResolveSessionTurnPreservesTextAndAttachmentOnResume(t *testing.T) {
	root := t.TempDir()
	attachmentPath := mustWriteTestPNG(t, root, "pixel.png")
	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outsidePath, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["` + outsidePath + `"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Attached file inspected."},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	first, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		UserText:      "inspect the outside file",
		Attachments: []AttachmentInput{{
			Path: attachmentPath,
		}},
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || first.PendingPermission == nil {
		t.Fatalf("first result = %#v", first)
	}

	second, err := runtime.ResolveSessionTurn(context.Background(), ResolveSessionTurnInput{
		SessionID:           first.SessionID,
		TurnID:              first.TurnID,
		PermissionRequestID: first.PendingRequestID,
		Decision:            events.PermissionDecisionApproved,
		Scope:               events.PermissionScopeOnce,
	})
	if err != nil {
		t.Fatalf("ResolveSessionTurn() error = %v", err)
	}
	if second.Status != TurnRunStatusCompleted || second.AssistantText != "Attached file inspected." {
		t.Fatalf("second result = %#v", second)
	}

	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	resumed := client.requests[1]
	if len(resumed.Inputs) != 3 {
		t.Fatalf("resumed inputs = %#v", resumed.Inputs)
	}
	if resumed.Inputs[0].Kind != provider.InputKindUserMessage || resumed.Inputs[0].Content != "inspect the outside file" {
		t.Fatalf("input[0] = %#v", resumed.Inputs[0])
	}
	if len(resumed.Inputs[0].Attachments) != 1 || resumed.Inputs[0].Attachments[0].Name != "pixel.png" {
		t.Fatalf("input[0].Attachments = %#v", resumed.Inputs[0].Attachments)
	}
}

func TestRuntimeResolveSessionTurnReplaysBatchedPrefixBeforePendingPermission(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app.go) error = %v", err)
	}
	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outsidePath, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(outside.txt) error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["` + outsidePath + `"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	first, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		UserText:      "inspect both files",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || first.PendingPermission == nil {
		t.Fatalf("first result = %#v", first)
	}

	second, err := runtime.ResolveSessionTurn(context.Background(), ResolveSessionTurnInput{
		SessionID:           first.SessionID,
		TurnID:              first.TurnID,
		PermissionRequestID: first.PendingRequestID,
		Decision:            events.PermissionDecisionApproved,
		Scope:               events.PermissionScopeOnce,
	})
	if err != nil {
		t.Fatalf("ResolveSessionTurn() error = %v", err)
	}
	if second.Status != TurnRunStatusCompleted || second.AssistantText != "done" {
		t.Fatalf("second result = %#v", second)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}

	resumed := client.requests[1]
	if len(resumed.Inputs) != 5 {
		t.Fatalf("resumed inputs = %#v", resumed.Inputs)
	}
	if resumed.Inputs[1].Kind != provider.InputKindToolCall || resumed.Inputs[1].CallID != "call-1" {
		t.Fatalf("input[1] = %#v", resumed.Inputs[1])
	}
	if resumed.Inputs[2].Kind != provider.InputKindToolCall || resumed.Inputs[2].CallID != "call-2" {
		t.Fatalf("input[2] = %#v", resumed.Inputs[2])
	}
	if resumed.Inputs[3].Kind != provider.InputKindToolResult || resumed.Inputs[3].CallID != "call-1" || !strings.Contains(resumed.Inputs[3].Output, "package main") {
		t.Fatalf("input[3] = %#v", resumed.Inputs[3])
	}
	if resumed.Inputs[4].Kind != provider.InputKindToolResult || resumed.Inputs[4].CallID != "call-2" || !strings.Contains(resumed.Inputs[4].Output, "outside") {
		t.Fatalf("input[4] = %#v", resumed.Inputs[4])
	}
}

func TestRuntimeResolveSessionTurnAllowsOneShotWebFetchPermission(t *testing.T) {
	requireLocalTestListener(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("hello from server"))
	}))
	defer server.Close()

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "web_fetch", InputDelta: `{"url":"` + server.URL + `","format":"text"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "web_fetch"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Fetched."},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	first, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "fetch the docs page",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || first.PendingPermission == nil {
		t.Fatalf("first result = %#v", first)
	}
	if first.PendingPermission.Kind != events.PermissionRequestKindNetwork {
		t.Fatalf("pending permission = %#v", first.PendingPermission)
	}

	second, err := runtime.ResolveSessionTurn(context.Background(), ResolveSessionTurnInput{
		SessionID:           first.SessionID,
		TurnID:              first.TurnID,
		PermissionRequestID: first.PendingRequestID,
		Decision:            events.PermissionDecisionApproved,
		Scope:               events.PermissionScopeOnce,
	})
	if err != nil {
		t.Fatalf("ResolveSessionTurn() error = %v", err)
	}
	if second.Status != TurnRunStatusCompleted || second.AssistantText != "Fetched." {
		t.Fatalf("second result = %#v", second)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), first.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(state.PendingPermissions) != 0 {
		t.Fatalf("pending permissions = %#v", state.PendingPermissions)
	}
	if len(state.NetworkGrants) != 0 {
		t.Fatalf("network grants = %#v", state.NetworkGrants)
	}
}

func TestRuntimeAnswerSessionQuestionPreservesAttachmentOnlyInputOnSameTurn(t *testing.T) {
	root := t.TempDir()
	attachmentPath := mustWriteTestPNG(t, root, "pixel.png")
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: tool.QuestionToolName, InputDelta: `{"question":"Continue?","options":["yes","no"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: tool.QuestionToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Used the attachment."},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	first, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		Attachments: []AttachmentInput{{
			Path: attachmentPath,
		}},
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || first.PendingQuestion == nil {
		t.Fatalf("first result = %#v", first)
	}

	second, err := runtime.AnswerSessionQuestion(context.Background(), AnswerSessionQuestionInput{
		SessionID: first.SessionID,
		TurnID:    first.TurnID,
		RequestID: first.PendingRequestID,
		Answer:    "yes",
	})
	if err != nil {
		t.Fatalf("AnswerSessionQuestion() error = %v", err)
	}
	if second.Status != TurnRunStatusCompleted || second.AssistantText != "Used the attachment." {
		t.Fatalf("second result = %#v", second)
	}
	if second.TurnID != first.TurnID {
		t.Fatalf("second turn id = %q, want same turn %q", second.TurnID, first.TurnID)
	}

	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	continued := client.requests[1]
	if continued.TurnID != first.TurnID {
		t.Fatalf("continued request turn id = %q, want %q", continued.TurnID, first.TurnID)
	}
	originalUserIndex := requestInputIndex(continued, func(input provider.Input) bool {
		return input.Kind == provider.InputKindUserMessage && len(input.Attachments) == 1 && input.Attachments[0].Name == "pixel.png"
	})
	if originalUserIndex < 0 {
		t.Fatalf("continued inputs = %#v", continued.Inputs)
	}
	toolResultIndex := requestInputIndex(continued, func(input provider.Input) bool {
		return input.Kind == provider.InputKindToolResult && input.CallID == "call-1" && input.ToolName == tool.QuestionToolName && input.Output == `{"answer":"yes"}`
	})
	if toolResultIndex < 0 {
		t.Fatalf("continued inputs = %#v", continued.Inputs)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), first.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(state.TurnOrder) != 1 {
		t.Fatalf("turn order = %#v", state.TurnOrder)
	}
	if turn := state.Turns[first.TurnID]; turn == nil || turn.Status != events.TurnStatusCompleted {
		t.Fatalf("turn = %#v", turn)
	}
}

func TestRuntimeRunSessionTurnCompletesWithinConfiguredProviderRequestLimit(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]`},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["app.go"],"offset":0,"limit":1}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Runner.maxProviderRequestsPerTurn = 3

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		UserText:      "keep reading until done",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted || result.AssistantText != "done" {
		t.Fatalf("result = %#v", result)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(state.TurnOrder) != 1 {
		t.Fatalf("turn order = %#v", state.TurnOrder)
	}
	turn := state.Turns[state.TurnOrder[0]]
	if turn == nil || turn.Status != events.TurnStatusCompleted {
		t.Fatalf("turn = %#v", turn)
	}

	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want 3", len(client.requests))
	}
	resumedRequest := client.requests[2]
	if len(resumedRequest.Inputs) != 5 {
		t.Fatalf("continued request inputs = %#v", resumedRequest.Inputs)
	}
	if resumedRequest.Inputs[0].Kind != provider.InputKindUserMessage || resumedRequest.Inputs[0].Content != "keep reading until done" {
		t.Fatalf("input[0] = %#v", resumedRequest.Inputs[0])
	}
	for i := 1; i < len(resumedRequest.Inputs); i++ {
		if resumedRequest.Inputs[i].Kind == provider.InputKindUserMessage {
			t.Fatalf("unexpected synthetic continuation input[%d] = %#v", i, resumedRequest.Inputs[i])
		}
	}
	if resumedRequest.Inputs[2].Kind != provider.InputKindToolResult || resumedRequest.Inputs[2].Output != expectedReadSingleLineOutput("package main") {
		t.Fatalf("input[2] = %#v", resumedRequest.Inputs[2])
	}
	if resumedRequest.Inputs[4].Kind != provider.InputKindToolResult ||
		resumedRequest.Inputs[4].Output != expectedReadSingleLineOutput("package main") {
		t.Fatalf("input[4] = %#v", resumedRequest.Inputs[4])
	}
}

func TestRuntimeRunSessionTurnFailsWhenNoModelSelected(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	runtime, err := NewRuntime(Config{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "say hello",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusFailed {
		t.Fatalf("result = %#v", result)
	}
	if result.Error != ErrModelSelectionRequired.Error() {
		t.Fatalf("result error = %q", result.Error)
	}
}

func TestRuntimeAnswerSessionQuestionResumesSameTurnWithAnsweredToolResult(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Need your choice. "},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: tool.QuestionToolName, InputDelta: `{"question":"Which path should I use?","options":["Use runtime","Use prompt"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: tool.QuestionToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Picked runtime."},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	first, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "ask me to choose",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || first.PendingRequestID == "" || first.PendingQuestion == nil {
		t.Fatalf("first result = %#v", first)
	}
	if first.PendingQuestion.ToolCallID != "call-1" || first.PendingQuestion.Question != "Which path should I use?" {
		t.Fatalf("pending question = %#v", first.PendingQuestion)
	}

	second, err := runtime.AnswerSessionQuestion(context.Background(), AnswerSessionQuestionInput{
		SessionID: first.SessionID,
		TurnID:    first.TurnID,
		RequestID: first.PendingRequestID,
		Answer:    "Use runtime",
	})
	if err != nil {
		t.Fatalf("AnswerSessionQuestion() error = %v", err)
	}
	if second.Status != TurnRunStatusCompleted {
		t.Fatalf("second result = %#v", second)
	}
	if second.AssistantText != "Picked runtime." {
		t.Fatalf("assistant text = %q", second.AssistantText)
	}
	if second.TurnID != first.TurnID {
		t.Fatalf("second turn id = %q, want same turn %q", second.TurnID, first.TurnID)
	}

	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	continued := client.requests[1]
	if continued.TurnID != first.TurnID {
		t.Fatalf("continued request turn id = %q, want %q", continued.TurnID, first.TurnID)
	}
	toolCallIndex := requestInputIndex(continued, func(input provider.Input) bool {
		return input.Kind == provider.InputKindToolCall && input.CallID == "call-1" && input.ToolName == tool.QuestionToolName
	})
	toolResultIndex := requestInputIndex(continued, func(input provider.Input) bool {
		return input.Kind == provider.InputKindToolResult && input.CallID == "call-1" && input.ToolName == tool.QuestionToolName && input.Output == `{"answer":"Use runtime"}`
	})
	if toolCallIndex < 0 || toolResultIndex < 0 || toolCallIndex >= toolResultIndex {
		t.Fatalf("continued inputs = %#v", continued.Inputs)
	}
	if answerIndex := requestInputIndex(continued, func(input provider.Input) bool {
		return input.Kind == provider.InputKindUserMessage && input.Content == "Use runtime"
	}); answerIndex >= 0 {
		t.Fatalf("continued inputs = %#v", continued.Inputs)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), first.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(state.PendingQuestionOrder) != 0 {
		t.Fatalf("pending questions = %#v", state.PendingQuestionOrder)
	}
	if turn := state.Turns[first.TurnID]; turn == nil || turn.Status != events.TurnStatusCompleted {
		t.Fatalf("turn = %#v", turn)
	}
	if len(state.TurnOrder) != 1 {
		t.Fatalf("turn order = %#v", state.TurnOrder)
	}
	if turn := state.Turns[first.TurnID]; turn == nil || turn.ContinuationStart != nil {
		t.Fatalf("turn continuation = %#v", turn)
	}
}

func TestRuntimeAnswerSessionQuestionCarriesAnthropicThinkingOnSameTurn(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{
					Kind: provider.EventKindAnthropicThinkingCommitted,
					AnthropicThinking: &provider.AnthropicThinkingBlock{
						Type:      provider.AnthropicThinkingBlockTypeThinking,
						Thinking:  "Need the runtime choice first.",
						Signature: "sig_123",
					},
				},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: tool.QuestionToolName, InputDelta: `{"question":"Which path should I use?","options":["Use runtime","Use fallback"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: tool.QuestionToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Picked runtime."},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"},
	}
	runtime.Config.Anthropic.APIKey = "test-key"

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID:       sessionID,
		TurnID:          "turn-1",
		UserText:        "ask me to choose",
		ThinkingEnabled: true,
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || first.PendingRequestID == "" || first.PendingQuestion == nil {
		t.Fatalf("first result = %#v", first)
	}

	second, err := runtime.AnswerSessionQuestion(context.Background(), AnswerSessionQuestionInput{
		SessionID: first.SessionID,
		TurnID:    first.TurnID,
		RequestID: first.PendingRequestID,
		Answer:    "Use runtime",
	})
	if err != nil {
		t.Fatalf("AnswerSessionQuestion() error = %v", err)
	}
	if second.Status != TurnRunStatusCompleted {
		t.Fatalf("second result = %#v", second)
	}
	if second.TurnID != first.TurnID {
		t.Fatalf("second turn id = %q, want same turn %q", second.TurnID, first.TurnID)
	}

	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	continued := client.requests[1]
	thinkingIndex := -1
	toolCallIndex := -1
	toolResultIndex := -1
	for idx, input := range continued.Inputs {
		switch input.Kind {
		case provider.InputKindAnthropicThinking:
			if input.AnthropicThinking != nil && input.AnthropicThinking.Signature == "sig_123" {
				thinkingIndex = idx
			}
		case provider.InputKindToolCall:
			if input.CallID == "call-1" && input.ToolName == tool.QuestionToolName {
				toolCallIndex = idx
			}
		case provider.InputKindToolResult:
			if input.CallID == "call-1" && input.ToolName == tool.QuestionToolName {
				toolResultIndex = idx
			}
		}
	}
	if thinkingIndex < 0 || toolCallIndex < 0 || toolResultIndex < 0 {
		t.Fatalf("continued inputs = %#v", continued.Inputs)
	}
	if thinkingIndex >= toolCallIndex || toolCallIndex >= toolResultIndex {
		t.Fatalf("input ordering thinking=%d tool_call=%d tool_result=%d inputs=%#v", thinkingIndex, toolCallIndex, toolResultIndex, continued.Inputs)
	}
}

func TestRuntimeAnswerSessionQuestionResumesSameTurnForGoogleToolLoop(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: tool.QuestionToolName, InputDelta: `{"question":"Which path should I use?","options":["Use runtime","Use fallback"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: tool.QuestionToolName, GoogleThoughtSignature: []byte("sig_123")},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Picked runtime."},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "google", ModelID: "gemini-3-pro"},
	}
	runtime.Config.Google.APIKey = "test-key"

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "ask me to choose",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || first.PendingRequestID == "" || first.PendingQuestion == nil {
		t.Fatalf("first result = %#v", first)
	}

	second, err := runtime.AnswerSessionQuestion(context.Background(), AnswerSessionQuestionInput{
		SessionID: first.SessionID,
		TurnID:    first.TurnID,
		RequestID: first.PendingRequestID,
		Answer:    "Use runtime",
	})
	if err != nil {
		t.Fatalf("AnswerSessionQuestion() error = %v", err)
	}
	if second.Status != TurnRunStatusCompleted {
		t.Fatalf("second result = %#v", second)
	}
	if second.TurnID != first.TurnID {
		t.Fatalf("second turn id = %q, want same turn %q", second.TurnID, first.TurnID)
	}
	if second.AssistantText != "Picked runtime." {
		t.Fatalf("assistant text = %q", second.AssistantText)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	continued := client.requests[1]
	toolCallIndex := -1
	toolResultIndex := -1
	for idx, input := range continued.Inputs {
		switch {
		case input.Kind == provider.InputKindToolCall && input.CallID == "call-1" && input.ToolName == tool.QuestionToolName:
			toolCallIndex = idx
			if got := string(input.GoogleThoughtSignature); got != "sig_123" {
				t.Fatalf("tool call thought signature = %q, want sig_123", got)
			}
		case input.Kind == provider.InputKindToolResult && input.CallID == "call-1" && input.ToolName == tool.QuestionToolName && input.Output == `{"answer":"Use runtime"}`:
			toolResultIndex = idx
		}
	}
	if toolCallIndex < 0 || toolResultIndex < 0 || toolCallIndex >= toolResultIndex {
		t.Fatalf("continued inputs = %#v", continued.Inputs)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), first.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if turn := state.Turns[first.TurnID]; turn == nil || turn.Status != events.TurnStatusCompleted {
		t.Fatalf("first turn = %#v", turn)
	}
}

func TestRuntimeAnswerSessionQuestionResumesSameTurnForLoopPause(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n// package\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	streams := repeatedInvalidWriteStreams(5, "notes.md")
	streams = append(streams, provider.NewSliceStream([]provider.Event{
		{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
	}))
	client := &fakeProvider{streams: streams}
	runtime := newRuntimeWithClient(t, client)

	first, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		UserText:      "keep reading until done",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || first.PendingRequestID == "" || first.PendingQuestion == nil {
		t.Fatalf("first result = %#v", first)
	}
	if first.PendingQuestion.ToolCallID != "" || first.PendingQuestion.ToolName != "" || first.PendingQuestion.Purpose != events.QuestionPurposeTurnLoopResolution {
		t.Fatalf("pending question = %#v", first.PendingQuestion)
	}

	second, err := runtime.AnswerSessionQuestion(context.Background(), AnswerSessionQuestionInput{
		SessionID: first.SessionID,
		TurnID:    first.TurnID,
		RequestID: first.PendingRequestID,
		Answer:    loopResolutionAnswerContinue,
	})
	if err != nil {
		t.Fatalf("AnswerSessionQuestion() error = %v", err)
	}
	if second.Status != TurnRunStatusCompleted {
		t.Fatalf("second result = %#v", second)
	}
	if second.AssistantText != "done" {
		t.Fatalf("assistant text = %q", second.AssistantText)
	}
	if second.TurnID != first.TurnID {
		t.Fatalf("second turn id = %q, want same turn %q", second.TurnID, first.TurnID)
	}
	if len(client.requests) != 6 {
		t.Fatalf("provider requests = %d, want 6", len(client.requests))
	}
	continued := client.requests[5]
	if continued.TurnID != first.TurnID {
		t.Fatalf("continued request turn id = %q, want %q", continued.TurnID, first.TurnID)
	}
	answerIndex := requestInputIndex(continued, func(input provider.Input) bool {
		return input.Kind == provider.InputKindUserMessage && input.Content == loopResolutionAnswerContinue
	})
	if answerIndex >= 0 {
		t.Fatalf("continued inputs include runtime loop answer at index %d: %#v", answerIndex, continued.Inputs)
	}
}

func TestRuntimeAnswerSessionQuestionResumesSameTurnAfterProviderRequestLimit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "search", InputDelta: `{"path":".","query":"func main","max_matches":1,"mode":"lexical"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "search"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Runner.maxProviderRequestsPerTurn = 2

	first, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		UserText:      "inspect app.go and finish",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || first.PendingRequestID == "" || first.PendingQuestion == nil {
		t.Fatalf("first result = %#v", first)
	}
	if first.PendingQuestion.Purpose != events.QuestionPurposeTurnLoopResolution ||
		!strings.Contains(first.PendingQuestion.Question, "assistant roundtrip limit") {
		t.Fatalf("pending question = %#v", first.PendingQuestion)
	}

	second, err := runtime.AnswerSessionQuestion(context.Background(), AnswerSessionQuestionInput{
		SessionID: first.SessionID,
		TurnID:    first.TurnID,
		RequestID: first.PendingRequestID,
		Answer:    loopResolutionAnswerContinue,
	})
	if err != nil {
		t.Fatalf("AnswerSessionQuestion() error = %v", err)
	}
	if second.Status != TurnRunStatusCompleted || second.AssistantText != "done" {
		t.Fatalf("second result = %#v", second)
	}
	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want 3", len(client.requests))
	}
}

func TestRuntimeAnswerSessionQuestionStopsLoopPausedTurnWithoutExtraProviderCall(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n// package\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{streams: repeatedInvalidWriteStreams(5, "notes.md")}
	runtime := newRuntimeWithClient(t, client)

	first, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		UserText:      "keep reading until done",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || first.PendingQuestion == nil {
		t.Fatalf("first result = %#v", first)
	}

	second, err := runtime.AnswerSessionQuestion(context.Background(), AnswerSessionQuestionInput{
		SessionID: first.SessionID,
		TurnID:    first.TurnID,
		RequestID: first.PendingRequestID,
		Answer:    loopResolutionAnswerStop,
	})
	if err != nil {
		t.Fatalf("AnswerSessionQuestion() error = %v", err)
	}
	if second.Status != TurnRunStatusCanceled {
		t.Fatalf("second result = %#v", second)
	}
	if len(client.requests) != 5 {
		t.Fatalf("provider requests = %d, want 5", len(client.requests))
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), first.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if turn := state.Turns[first.TurnID]; turn == nil || turn.Status != events.TurnStatusCanceled {
		t.Fatalf("turn = %#v", turn)
	}
}

func TestRuntimeRunSessionTurnResumesApprovedServerIntentWithoutHanging(t *testing.T) {
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
			PID:             5151,
			ProcessIdentity: "identity-5151",
			Ready:           readyCh,
			Exited:          exitCh,
		}, nil
	})

	root := t.TempDir()
	clientDir := filepath.Join(root, "client")
	if err := os.MkdirAll(clientDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(clientDir, "package.json"), []byte(`{"scripts":{"dev":"vite --host 0.0.0.0 --port 5173"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	providerClient := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "bash", InputDelta: `{"cmd":"npm run dev","workdir":"client"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "bash"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Server launch recorded."},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, providerClient)

	first, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		UserText:      "start the dev server",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || first.PendingRequestID == "" || first.PendingExecution == nil {
		t.Fatalf("first = %#v", first)
	}
	if first.PendingExecution.Reason != "requires approval to start a persistent local server" {
		t.Fatalf("pending execution = %#v", first.PendingExecution)
	}

	second, err := runtime.ResolveSessionTurn(context.Background(), ResolveSessionTurnInput{
		SessionID:           first.SessionID,
		TurnID:              first.TurnID,
		PermissionRequestID: first.PendingRequestID,
		ExecutionDecision:   events.ExecutionApprovalDecisionAccept,
	})
	if err != nil {
		t.Fatalf("ResolveSessionTurn() error = %v", err)
	}
	if second.Status != TurnRunStatusCompleted {
		t.Fatalf("second = %#v", second)
	}
	if second.AssistantText != "Server launch recorded." {
		t.Fatalf("assistant text = %q", second.AssistantText)
	}

	if len(providerClient.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(providerClient.requests))
	}
	resumed := providerClient.requests[1]
	if strings.Contains(resumed.Instructions, `Runtime note: Command execution "npm run dev"`) {
		t.Fatalf("resumed instructions unexpectedly carry runtime approval note: %q", resumed.Instructions)
	}
	if strings.Contains(resumed.DynamicSuffix, `Runtime note: Command execution "npm run dev"`) {
		t.Fatalf("resumed dynamic suffix unexpectedly carries runtime approval note: %q", resumed.DynamicSuffix)
	}
	foundToolResult := false
	for _, input := range resumed.Inputs {
		if input.Kind != provider.InputKindToolResult || input.CallID != "call-1" || input.ToolName != "bash" {
			continue
		}
		if !strings.Contains(input.Output, `Runtime note: Command execution "npm run dev"`) {
			t.Fatalf("tool result missing explicit execution approval note: %q", input.Output)
		}
		if !strings.Contains(input.Output, "Started server in background (pid 5151).") {
			t.Fatalf("tool result output = %q", input.Output)
		}
		foundToolResult = true
	}
	if !foundToolResult {
		t.Fatalf("resumed inputs = %#v", resumed.Inputs)
	}
}

func TestRuntimeResolveSessionTurnReplaysBackgroundExitAfterToolResultOnLaterResume(t *testing.T) {
	useExecutionRunnerHooks(t, func(context.Context, executionContract, executionRunOptions) (executionRunResult, error) {
		t.Fatal("foreground execution runner should not be used for server intent")
		return executionRunResult{}, nil
	})
	useBackgroundExecutionRunnerHooks(t, func(context.Context, executionContract, executionBackgroundRunOptions) (executionBackgroundHandle, error) {
		readyCh := make(chan executionBackgroundReadyEvent)
		exitCh := make(chan executionBackgroundExitEvent, 1)
		exitCh <- executionBackgroundExitEvent{
			RunResult: executionRunResult{
				Backend:  "background_process",
				ExitCode: intPointer(0),
			},
		}
		close(exitCh)
		return executionBackgroundHandle{
			PID:             5151,
			ProcessIdentity: "identity-5151",
			Ready:           readyCh,
			Exited:          exitCh,
		}, nil
	})

	root := t.TempDir()
	clientDir := filepath.Join(root, "client")
	if err := os.MkdirAll(clientDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(clientDir, "package.json"), []byte(`{"scripts":{"dev":"vite --host 0.0.0.0 --port 5173"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outsidePath, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	providerClient := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "bash", InputDelta: `{"cmd":"npm run dev","workdir":"client"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "bash"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["` + outsidePath + `"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Done."},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, providerClient)

	first, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		UserText:      "start the dev server and inspect the outside file",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || first.PendingRequestID == "" || first.PendingExecution == nil {
		t.Fatalf("first = %#v", first)
	}

	second, err := runtime.ResolveSessionTurn(context.Background(), ResolveSessionTurnInput{
		SessionID:           first.SessionID,
		TurnID:              first.TurnID,
		PermissionRequestID: first.PendingRequestID,
		ExecutionDecision:   events.ExecutionApprovalDecisionAccept,
	})
	if err != nil {
		t.Fatalf("ResolveSessionTurn(server) error = %v", err)
	}
	if second.Status != TurnRunStatusPending || second.PendingRequestID == "" || second.PendingPermission == nil {
		t.Fatalf("second = %#v", second)
	}
	if second.PendingPermission.ToolCallID != "call-2" || second.PendingPermission.ToolName != "read" {
		t.Fatalf("pending permission = %#v", second.PendingPermission)
	}

	third, err := runtime.ResolveSessionTurn(context.Background(), ResolveSessionTurnInput{
		SessionID:           first.SessionID,
		TurnID:              first.TurnID,
		PermissionRequestID: second.PendingRequestID,
		Decision:            events.PermissionDecisionApproved,
		Scope:               events.PermissionScopeOnce,
	})
	if err != nil {
		t.Fatalf("ResolveSessionTurn(read) error = %v", err)
	}
	if third.Status != TurnRunStatusCompleted || third.AssistantText != "Done." {
		t.Fatalf("third = %#v", third)
	}

	if len(providerClient.requests) != 3 {
		t.Fatalf("provider requests = %d, want 3", len(providerClient.requests))
	}
	resumed := providerClient.requests[2]
	if len(resumed.Inputs) != 6 {
		t.Fatalf("resumed inputs = %#v", resumed.Inputs)
	}
	if resumed.Inputs[1].Kind != provider.InputKindToolCall || resumed.Inputs[1].CallID != "call-1" {
		t.Fatalf("input[1] = %#v", resumed.Inputs[1])
	}
	if resumed.Inputs[2].Kind != provider.InputKindToolResult || resumed.Inputs[2].CallID != "call-1" {
		t.Fatalf("input[2] = %#v", resumed.Inputs[2])
	}
	if resumed.Inputs[3].Kind != provider.InputKindAssistantMessage || !strings.Contains(resumed.Inputs[3].Content, `Background command "npm run dev"`) {
		t.Fatalf("input[3] = %#v", resumed.Inputs[3])
	}
	if resumed.Inputs[4].Kind != provider.InputKindToolCall || resumed.Inputs[4].CallID != "call-2" {
		t.Fatalf("input[4] = %#v", resumed.Inputs[4])
	}
	if resumed.Inputs[5].Kind != provider.InputKindToolResult || resumed.Inputs[5].CallID != "call-2" || !strings.Contains(resumed.Inputs[5].Output, "1: outside") {
		t.Fatalf("input[5] = %#v", resumed.Inputs[5])
	}
}

func TestRuntimeAnswerSessionQuestionAsksWhenConfiguredProviderRequestLimitIsReached(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-question", ToolName: tool.QuestionToolName, InputDelta: `{"question":"Proceed?","options":["yes","no"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-question", ToolName: tool.QuestionToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]`},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["app.go"],"offset":0,"limit":1}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "All set."},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Runner.maxProviderRequestsPerTurn = 3

	first, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		UserText:      "ask then inspect the file",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || first.PendingQuestion == nil {
		t.Fatalf("first result = %#v", first)
	}

	second, err := runtime.AnswerSessionQuestion(context.Background(), AnswerSessionQuestionInput{
		SessionID: first.SessionID,
		TurnID:    first.TurnID,
		RequestID: first.PendingRequestID,
		Answer:    "yes",
	})
	if err != nil {
		t.Fatalf("AnswerSessionQuestion() error = %v", err)
	}
	if second.Status != TurnRunStatusPending || second.PendingQuestion == nil {
		t.Fatalf("second result = %#v", second)
	}
	if second.TurnID != first.TurnID {
		t.Fatalf("second turn id = %q, want same turn %q", second.TurnID, first.TurnID)
	}
	if second.PendingQuestion.Purpose != events.QuestionPurposeTurnLoopResolution ||
		!strings.Contains(second.PendingQuestion.Question, "assistant roundtrip limit") {
		t.Fatalf("pending question = %#v", second.PendingQuestion)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), first.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(state.TurnOrder) != 1 {
		t.Fatalf("turn order = %#v", state.TurnOrder)
	}
	if turn := state.Turns[first.TurnID]; turn == nil || turn.Status != events.TurnStatusRunning || turn.ErrorCode != "" {
		t.Fatalf("turn = %#v", turn)
	}

	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want 3 before limit asks to continue", len(client.requests))
	}
}
