package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/workspace"
)

func TestRuntimeOpenWorkspaceSessionResumesLatestWorkspaceSession(t *testing.T) {
	sessionDir := t.TempDir()
	workspaceRoot := t.TempDir()

	firstRuntime := newPersistentRuntimeWithClient(t, sessionDir, &fakeProvider{})
	created, err := firstRuntime.OpenWorkspaceSession(context.Background(), workspaceRoot, nil, false)
	if err != nil {
		t.Fatalf("OpenWorkspaceSession(create) error = %v", err)
	}

	secondRuntime := newPersistentRuntimeWithClient(t, sessionDir, &fakeProvider{})
	resumed, err := secondRuntime.OpenWorkspaceSession(context.Background(), workspaceRoot, nil, true)
	if err != nil {
		t.Fatalf("OpenWorkspaceSession(resume) error = %v", err)
	}
	if !resumed.Resumed {
		t.Fatalf("resumed = %#v, want resumed session", resumed)
	}
	if resumed.SessionID != created.SessionID {
		t.Fatalf("session id = %q, want %q", resumed.SessionID, created.SessionID)
	}
}

func TestRuntimeOpenWorkspaceSessionReturnsPendingPermissionResume(t *testing.T) {
	sessionDir := t.TempDir()
	workspaceRoot := t.TempDir()
	runtime := newPersistentRuntimeWithClient(t, sessionDir, &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "checking outside file"},
			{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["/tmp/outside.txt"]}`},
			{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
		})},
	})

	session, err := runtime.OpenWorkspaceSession(context.Background(), workspaceRoot, nil, false)
	if err != nil {
		t.Fatalf("OpenWorkspaceSession(create) error = %v", err)
	}
	if _, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: session.SessionID,
		TurnID:    "turn-1",
		UserText:  "read outside file",
		AgentID:   "builder",
	}); err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}

	reopened := newPersistentRuntimeWithClient(t, sessionDir, &fakeProvider{})
	resumed, err := reopened.OpenWorkspaceSession(context.Background(), workspaceRoot, nil, true)
	if err != nil {
		t.Fatalf("OpenWorkspaceSession(resume) error = %v", err)
	}
	if !resumed.Resumed {
		t.Fatalf("resumed = %#v, want resumed session", resumed)
	}
	if resumed.SessionID != session.SessionID {
		t.Fatalf("session id = %q, want %q", resumed.SessionID, session.SessionID)
	}
}

func TestRuntimeOpenWorkspaceSessionReplaysAnthropicThinkingAcrossRestart(t *testing.T) {
	sessionDir := t.TempDir()
	workspaceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	firstClient := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{
					Kind: provider.EventKindAnthropicThinkingCommitted,
					AnthropicThinking: &provider.AnthropicThinkingBlock{
						Type:      provider.AnthropicThinkingBlockTypeThinking,
						Thinking:  "Inspect the file before reading it.",
						Signature: "sig_123",
					},
				},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	firstRuntime := newPersistentRuntimeWithClient(t, sessionDir, firstClient)
	firstRuntime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"},
	}
	firstRuntime.Config.Anthropic.APIKey = "test-key"

	session, err := firstRuntime.OpenWorkspaceSession(context.Background(), workspaceRoot, nil, false)
	if err != nil {
		t.Fatalf("OpenWorkspaceSession(create) error = %v", err)
	}
	firstTurn, err := firstRuntime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID:       session.SessionID,
		TurnID:          "turn-1",
		UserText:        "read app.go",
		AgentID:         "builder",
		ThinkingEnabled: true,
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn(turn-1) error = %v", err)
	}
	if firstTurn.Status != TurnRunStatusCompleted {
		t.Fatalf("first turn = %#v", firstTurn)
	}

	secondClient := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "next"},
		})},
	}
	secondRuntime := newPersistentRuntimeWithClient(t, sessionDir, secondClient)
	secondRuntime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"},
	}
	secondRuntime.Config.Anthropic.APIKey = "test-key"

	resumed, err := secondRuntime.OpenWorkspaceSession(context.Background(), workspaceRoot, nil, true)
	if err != nil {
		t.Fatalf("OpenWorkspaceSession(resume) error = %v", err)
	}
	if !resumed.Resumed || resumed.SessionID != session.SessionID {
		t.Fatalf("resumed session = %#v", resumed)
	}
	secondTurn, err := secondRuntime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: resumed.SessionID,
		TurnID:    "turn-2",
		UserText:  "continue",
		AgentID:   "builder",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn(turn-2) error = %v", err)
	}
	if secondTurn.Status != TurnRunStatusCompleted {
		t.Fatalf("second turn = %#v", secondTurn)
	}
	if len(secondClient.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(secondClient.requests))
	}

	request := secondClient.requests[0]
	thinkingIndex := -1
	toolCallIndex := -1
	toolResultIndex := -1
	for idx, input := range request.Inputs {
		switch input.Kind {
		case provider.InputKindAnthropicThinking:
			if input.AnthropicThinking != nil && input.AnthropicThinking.Signature == "sig_123" {
				thinkingIndex = idx
			}
		case provider.InputKindToolCall:
			if input.CallID == "call-1" {
				toolCallIndex = idx
			}
		case provider.InputKindToolResult:
			if input.CallID == "call-1" {
				toolResultIndex = idx
			}
		}
	}
	if thinkingIndex < 0 || toolCallIndex < 0 || toolResultIndex < 0 {
		t.Fatalf("reopened request inputs = %#v", request.Inputs)
	}
	if thinkingIndex >= toolCallIndex || toolCallIndex >= toolResultIndex {
		t.Fatalf("input ordering thinking=%d tool_call=%d tool_result=%d inputs=%#v", thinkingIndex, toolCallIndex, toolResultIndex, request.Inputs)
	}
}

func TestRuntimeOpenWorkspaceSessionPersistsOpenAIReasoningContinuationFailureAcrossRestart(t *testing.T) {
	sessionDir := t.TempDir()
	workspaceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	firstClient := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindReasoningDelta, ReasoningDelta: "Inspect the file before reading it."},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	firstRuntime := newPersistentRuntimeWithClient(t, sessionDir, firstClient)
	firstRuntime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "deepseek", ModelID: "deepseek-v4-pro"},
	}
	firstRuntime.Config.DeepSeek.APIKey = "test-key"
	firstRuntime.ModelCatalog = &fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"deepseek": {{
				ID:        "deepseek-v4-pro",
				Name:      "DeepSeek V4 Pro",
				Reasoning: true,
			}},
		},
	}

	session, err := firstRuntime.OpenWorkspaceSession(context.Background(), workspaceRoot, nil, false)
	if err != nil {
		t.Fatalf("OpenWorkspaceSession(create) error = %v", err)
	}
	firstTurn, err := firstRuntime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID:       session.SessionID,
		TurnID:          "turn-1",
		UserText:        "read app.go",
		AgentID:         "builder",
		ThinkingEnabled: true,
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn(turn-1) error = %v", err)
	}
	if firstTurn.Status != TurnRunStatusFailed {
		t.Fatalf("first turn = %#v", firstTurn)
	}
	if !strings.Contains(firstTurn.Error, "openai_reasoning_tool_loop") {
		t.Fatalf("first turn error = %q", firstTurn.Error)
	}
	if len(firstClient.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(firstClient.requests))
	}

	secondRuntime := newPersistentRuntimeWithClient(t, sessionDir, &fakeProvider{})
	secondRuntime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "deepseek", ModelID: "deepseek-v4-pro"},
	}
	secondRuntime.Config.DeepSeek.APIKey = "test-key"
	secondRuntime.ModelCatalog = &fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"deepseek": {{
				ID:        "deepseek-v4-pro",
				Name:      "DeepSeek V4 Pro",
				Reasoning: true,
			}},
		},
	}

	resumed, err := secondRuntime.OpenWorkspaceSession(context.Background(), workspaceRoot, nil, true)
	if err != nil {
		t.Fatalf("OpenWorkspaceSession(resume) error = %v", err)
	}
	if !resumed.Resumed || resumed.SessionID != session.SessionID {
		t.Fatalf("resumed session = %#v", resumed)
	}

	state, err := secondRuntime.Sessions.Snapshot(context.Background(), resumed.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.Status != events.TurnStatusFailed || turn.WorkState == nil || turn.WorkState.NativeContinuation == nil || turn.WorkState.NativeContinuation.Contract != "openai_reasoning_tool_loop" {
		t.Fatalf("reopened turn = %#v", turn)
	}
}

func TestRuntimeOpenWorkspaceSessionReplaysGoogleToolLoopAcrossRestart(t *testing.T) {
	sessionDir := t.TempDir()
	workspaceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	firstClient := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read", GoogleThoughtSignature: []byte("sig_123")},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	firstRuntime := newPersistentRuntimeWithClient(t, sessionDir, firstClient)
	firstRuntime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "google", ModelID: "gemini-3-pro"},
	}
	firstRuntime.Config.Google.APIKey = "test-key"

	session, err := firstRuntime.OpenWorkspaceSession(context.Background(), workspaceRoot, nil, false)
	if err != nil {
		t.Fatalf("OpenWorkspaceSession(create) error = %v", err)
	}
	firstTurn, err := firstRuntime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: session.SessionID,
		TurnID:    "turn-1",
		UserText:  "read app.go",
		AgentID:   "builder",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn(turn-1) error = %v", err)
	}
	if firstTurn.Status != TurnRunStatusCompleted {
		t.Fatalf("first turn = %#v", firstTurn)
	}
	if len(firstClient.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(firstClient.requests))
	}

	secondClient := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "next"},
		})},
	}
	secondRuntime := newPersistentRuntimeWithClient(t, sessionDir, secondClient)
	secondRuntime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "google", ModelID: "gemini-3-pro"},
	}
	secondRuntime.Config.Google.APIKey = "test-key"

	resumed, err := secondRuntime.OpenWorkspaceSession(context.Background(), workspaceRoot, nil, true)
	if err != nil {
		t.Fatalf("OpenWorkspaceSession(resume) error = %v", err)
	}
	if !resumed.Resumed || resumed.SessionID != session.SessionID {
		t.Fatalf("resumed session = %#v", resumed)
	}
	secondTurn, err := secondRuntime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: resumed.SessionID,
		TurnID:    "turn-2",
		UserText:  "continue",
		AgentID:   "builder",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn(turn-2) error = %v", err)
	}
	if secondTurn.Status != TurnRunStatusCompleted {
		t.Fatalf("second turn = %#v", secondTurn)
	}
	if len(secondClient.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(secondClient.requests))
	}

	request := secondClient.requests[0]
	toolCallIndex := -1
	toolResultIndex := -1
	for idx, input := range request.Inputs {
		switch input.Kind {
		case provider.InputKindToolCall:
			if input.CallID == "call-1" {
				toolCallIndex = idx
				if got := string(input.GoogleThoughtSignature); got != "sig_123" {
					t.Fatalf("tool call thought signature = %q, want sig_123", got)
				}
			}
		case provider.InputKindToolResult:
			if input.CallID == "call-1" {
				toolResultIndex = idx
			}
		}
	}
	if toolCallIndex < 0 || toolResultIndex < 0 || toolCallIndex >= toolResultIndex {
		t.Fatalf("reopened request inputs = %#v", request.Inputs)
	}
}

func TestRuntimeOpenWorkspaceSessionAddsAdditionalRootsOnResume(t *testing.T) {
	sessionDir := t.TempDir()
	workspaceRoot := t.TempDir()
	extraRoot := t.TempDir()
	extraScope, err := workspace.New(extraRoot)
	if err != nil {
		t.Fatalf("workspace.New(extraRoot) error = %v", err)
	}

	firstRuntime := newPersistentRuntimeWithClient(t, sessionDir, &fakeProvider{})
	created, err := firstRuntime.OpenWorkspaceSession(context.Background(), workspaceRoot, nil, false)
	if err != nil {
		t.Fatalf("OpenWorkspaceSession(create) error = %v", err)
	}

	secondRuntime := newPersistentRuntimeWithClient(t, sessionDir, &fakeProvider{})
	resumed, err := secondRuntime.OpenWorkspaceSession(context.Background(), workspaceRoot, []string{extraRoot}, true)
	if err != nil {
		t.Fatalf("OpenWorkspaceSession(resume) error = %v", err)
	}
	if resumed.SessionID != created.SessionID {
		t.Fatalf("session id = %q, want %q", resumed.SessionID, created.SessionID)
	}

	state, err := secondRuntime.Sessions.Snapshot(context.Background(), resumed.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(state.AdditionalWorkspaceRoots) != 1 || state.AdditionalWorkspaceRoots[0] != extraScope.Root() {
		t.Fatalf("AdditionalWorkspaceRoots = %#v", state.AdditionalWorkspaceRoots)
	}
}

func TestRuntimeOpenWorkspaceSessionMarksBackgroundExecutionLostAcrossRestart(t *testing.T) {
	sessionDir := t.TempDir()
	workspaceRoot := t.TempDir()
	runtime := newPersistentRuntimeWithClient(t, sessionDir, &fakeProvider{})

	session, err := runtime.OpenWorkspaceSession(context.Background(), workspaceRoot, nil, false)
	if err != nil {
		t.Fatalf("OpenWorkspaceSession(create) error = %v", err)
	}

	appendEvent := func(turnID string, typ events.Type, payload events.Payload) {
		if _, err := runtime.Sessions.append(context.Background(), events.Draft{
			SessionID: session.SessionID,
			TurnID:    turnID,
			Type:      typ,
			Payload:   payload,
		}); err != nil {
			t.Fatalf("append(%s, %T) error = %v", typ, payload, err)
		}
	}
	appendEvent("turn-1", events.TypeExecutionDeclared, events.ExecutionDeclaredPayload{
		ExecutionID:      "exec-call-1",
		ToolCallID:       "call-1",
		ToolName:         "bash",
		Kind:             "bash",
		Intent:           "server",
		Command:          []string{"/bin/sh", "-c", "npm run dev"},
		CommandPreview:   "npm run dev",
		WorkingDirectory: filepath.Join(workspaceRoot, "client"),
		TimeoutMS:        120000,
		OutputLimit:      12000,
	})
	appendEvent("turn-1", events.TypeExecutionStarted, events.ExecutionStartedPayload{
		ExecutionID: "exec-call-1",
		ToolCallID:  "call-1",
		ToolName:    "bash",
		Input:       `{"cmd":"npm run dev","workdir":"client"}`,
	})
	appendEvent("turn-1", events.TypeExecutionBackgroundStarted, events.ExecutionBackgroundStartedPayload{
		ExecutionID:     "exec-call-1",
		ToolCallID:      "call-1",
		ToolName:        "bash",
		PID:             4242,
		ProcessIdentity: "identity-old",
		SupervisorID:    "background-supervisor-old",
		LogRef:          filepath.ToSlash(filepath.Join(session.SessionID, "turn-1", "exec-call-1.log")),
		ReadyPatterns:   []string{"local:"},
	})
	appendEvent("turn-1", events.TypeToolExecEnd, events.ToolExecEndPayload{
		CallID:          "call-1",
		ToolName:        "bash",
		ExecutionID:     "exec-call-1",
		ExecutionStatus: string(events.ExecutionStatusCompleted),
		Succeeded:       true,
		Output:          "Started server in background (pid 4242). Readiness not yet confirmed.",
	})

	prevInspect := loadBackgroundProcessStateFunc
	var alive atomic.Bool
	alive.Store(true)
	loadBackgroundProcessStateFunc = func(pid int) (backgroundProcessState, error) {
		if pid == 4242 && alive.Load() {
			return backgroundProcessState{Running: true, Identity: "identity-new"}, nil
		}
		return backgroundProcessState{}, nil
	}
	t.Cleanup(func() {
		loadBackgroundProcessStateFunc = prevInspect
	})

	reopened := newPersistentRuntimeWithClient(t, sessionDir, &fakeProvider{})
	if _, err := reopened.OpenWorkspaceSession(context.Background(), workspaceRoot, nil, true); err != nil {
		t.Fatalf("OpenWorkspaceSession(resume) error = %v", err)
	}

	waitForBackgroundState(t, reopened, session.SessionID, func(background *events.ExecutionBackgroundState) bool {
		return background != nil &&
			background.Status == events.ExecutionBackgroundStatusSupervisionLost &&
			strings.Contains(background.Error, "pid reuse or process replacement")
	})

	reopenedAgain := newPersistentRuntimeWithClient(t, sessionDir, &fakeProvider{})
	if _, err := reopenedAgain.OpenWorkspaceSession(context.Background(), workspaceRoot, nil, true); err != nil {
		t.Fatalf("OpenWorkspaceSession(resume again) error = %v", err)
	}
	replayed, err := reopenedAgain.Store.Replay(context.Background(), events.Query{SessionID: session.SessionID, AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	lostEvents := 0
	for _, event := range replayed {
		if event.Type == events.TypeExecutionBackgroundLost {
			lostEvents++
		}
	}
	if lostEvents != 1 {
		t.Fatalf("background lost events = %d, want 1", lostEvents)
	}
}

func TestRuntimeOpenWorkspaceSessionResumesBackgroundExecutionAcrossRestart(t *testing.T) {
	sessionDir := t.TempDir()
	workspaceRoot := t.TempDir()
	runtime := newPersistentRuntimeWithClient(t, sessionDir, &fakeProvider{})

	session, err := runtime.OpenWorkspaceSession(context.Background(), workspaceRoot, nil, false)
	if err != nil {
		t.Fatalf("OpenWorkspaceSession(create) error = %v", err)
	}

	logRef := filepath.ToSlash(filepath.Join(session.SessionID, "turn-1", "exec-call-1.log"))
	appendEvent := func(turnID string, typ events.Type, payload events.Payload) {
		if _, err := runtime.Sessions.append(context.Background(), events.Draft{
			SessionID: session.SessionID,
			TurnID:    turnID,
			Type:      typ,
			Payload:   payload,
		}); err != nil {
			t.Fatalf("append(%s, %T) error = %v", typ, payload, err)
		}
	}
	appendEvent("turn-1", events.TypeExecutionDeclared, events.ExecutionDeclaredPayload{
		ExecutionID:      "exec-call-1",
		ToolCallID:       "call-1",
		ToolName:         "bash",
		Kind:             "bash",
		Intent:           "server",
		Command:          []string{"/bin/sh", "-c", "npm run dev"},
		CommandPreview:   "npm run dev",
		WorkingDirectory: filepath.Join(workspaceRoot, "client"),
		TimeoutMS:        120000,
		OutputLimit:      12000,
	})
	appendEvent("turn-1", events.TypeExecutionStarted, events.ExecutionStartedPayload{
		ExecutionID: "exec-call-1",
		ToolCallID:  "call-1",
		ToolName:    "bash",
		Input:       `{"cmd":"npm run dev","workdir":"client"}`,
	})
	appendEvent("turn-1", events.TypeExecutionBackgroundStarted, events.ExecutionBackgroundStartedPayload{
		ExecutionID:     "exec-call-1",
		ToolCallID:      "call-1",
		ToolName:        "bash",
		PID:             4242,
		ProcessIdentity: "identity-live",
		SupervisorID:    "background-supervisor-old",
		LogRef:          logRef,
		ReadyPatterns:   []string{"local:"},
	})
	appendEvent("turn-1", events.TypeToolExecEnd, events.ToolExecEndPayload{
		CallID:          "call-1",
		ToolName:        "bash",
		ExecutionID:     "exec-call-1",
		ExecutionStatus: string(events.ExecutionStatusCompleted),
		Succeeded:       true,
		Output:          "Started server in background (pid 4242). Readiness not yet confirmed.",
	})

	appendTestBackgroundLog(t, runtime.Store, BackgroundExecutionLogKey{
		SessionID:   session.SessionID,
		TurnID:      "turn-1",
		ExecutionID: "exec-call-1",
	}, "Booting dev server...\nLocal: http://127.0.0.1:5173/\n")

	prevInspect := loadBackgroundProcessStateFunc
	var alive atomic.Bool
	alive.Store(true)
	loadBackgroundProcessStateFunc = func(pid int) (backgroundProcessState, error) {
		if pid == 4242 && alive.Load() {
			return backgroundProcessState{Running: true, Identity: "identity-live"}, nil
		}
		return backgroundProcessState{}, nil
	}
	t.Cleanup(func() {
		loadBackgroundProcessStateFunc = prevInspect
	})

	reopened := newPersistentRuntimeWithClient(t, sessionDir, &fakeProvider{})
	if _, err := reopened.OpenWorkspaceSession(context.Background(), workspaceRoot, nil, true); err != nil {
		t.Fatalf("OpenWorkspaceSession(resume) error = %v", err)
	}

	waitForBackgroundState(t, reopened, session.SessionID, func(background *events.ExecutionBackgroundState) bool {
		return background != nil &&
			background.Status == events.ExecutionBackgroundStatusReady &&
			background.Ready &&
			background.Port == 5173 &&
			background.OutputBytes > 0 &&
			strings.Contains(background.OutputTail, "Local: http://127.0.0.1:5173/")
	})

	alive.Store(false)
	waitForBackgroundState(t, reopened, session.SessionID, func(background *events.ExecutionBackgroundState) bool {
		return background != nil &&
			background.Status == events.ExecutionBackgroundStatusExited &&
			background.Exited &&
			strings.Contains(background.Error, "resumed supervision")
	})
}

func TestRuntimeOpenWorkspaceSessionRecoversBackgroundExitOnResume(t *testing.T) {
	sessionDir := t.TempDir()
	workspaceRoot := t.TempDir()
	runtime := newPersistentRuntimeWithClient(t, sessionDir, &fakeProvider{})

	session, err := runtime.OpenWorkspaceSession(context.Background(), workspaceRoot, nil, false)
	if err != nil {
		t.Fatalf("OpenWorkspaceSession(create) error = %v", err)
	}

	appendEvent := func(turnID string, typ events.Type, payload events.Payload) {
		if _, err := runtime.Sessions.append(context.Background(), events.Draft{
			SessionID: session.SessionID,
			TurnID:    turnID,
			Type:      typ,
			Payload:   payload,
		}); err != nil {
			t.Fatalf("append(%s, %T) error = %v", typ, payload, err)
		}
	}
	appendEvent("turn-1", events.TypeExecutionDeclared, events.ExecutionDeclaredPayload{
		ExecutionID:      "exec-call-1",
		ToolCallID:       "call-1",
		ToolName:         "bash",
		Kind:             "bash",
		Intent:           "watcher",
		Command:          []string{"/bin/sh", "-c", "npm run watch"},
		CommandPreview:   "npm run watch",
		WorkingDirectory: filepath.Join(workspaceRoot, "client"),
		TimeoutMS:        120000,
		OutputLimit:      12000,
	})
	appendEvent("turn-1", events.TypeExecutionStarted, events.ExecutionStartedPayload{
		ExecutionID: "exec-call-1",
		ToolCallID:  "call-1",
		ToolName:    "bash",
		Input:       `{"cmd":"npm run watch","workdir":"client"}`,
	})
	appendEvent("turn-1", events.TypeExecutionBackgroundStarted, events.ExecutionBackgroundStartedPayload{
		ExecutionID:     "exec-call-1",
		ToolCallID:      "call-1",
		ToolName:        "bash",
		PID:             4242,
		ProcessIdentity: "identity-old",
		SupervisorID:    "background-supervisor-old",
		LogRef:          filepath.ToSlash(filepath.Join(session.SessionID, "turn-1", "exec-call-1.log")),
	})
	appendEvent("turn-1", events.TypeToolExecEnd, events.ToolExecEndPayload{
		CallID:          "call-1",
		ToolName:        "bash",
		ExecutionID:     "exec-call-1",
		ExecutionStatus: string(events.ExecutionStatusCompleted),
		Succeeded:       true,
		Output:          "Started watch process in background (pid 4242).",
	})

	prevInspect := loadBackgroundProcessStateFunc
	var alive atomic.Bool
	alive.Store(false)
	loadBackgroundProcessStateFunc = func(pid int) (backgroundProcessState, error) {
		if pid == 4242 && alive.Load() {
			return backgroundProcessState{Running: true, Identity: "identity-old"}, nil
		}
		return backgroundProcessState{}, nil
	}
	t.Cleanup(func() {
		loadBackgroundProcessStateFunc = prevInspect
	})

	reopened := newPersistentRuntimeWithClient(t, sessionDir, &fakeProvider{})
	if _, err := reopened.OpenWorkspaceSession(context.Background(), workspaceRoot, nil, true); err != nil {
		t.Fatalf("OpenWorkspaceSession(resume) error = %v", err)
	}

	waitForBackgroundState(t, reopened, session.SessionID, func(background *events.ExecutionBackgroundState) bool {
		return background != nil &&
			background.Status == events.ExecutionBackgroundStatusExited &&
			background.Exited &&
			strings.Contains(background.Error, "resumed supervision")
	})
}

func TestRuntimeOpenWorkspaceSessionRecoversBackgroundReadyBeforeExitOnResume(t *testing.T) {
	sessionDir := t.TempDir()
	workspaceRoot := t.TempDir()
	runtime := newPersistentRuntimeWithClient(t, sessionDir, &fakeProvider{})

	session, err := runtime.OpenWorkspaceSession(context.Background(), workspaceRoot, nil, false)
	if err != nil {
		t.Fatalf("OpenWorkspaceSession(create) error = %v", err)
	}

	logRef := filepath.ToSlash(filepath.Join(session.SessionID, "turn-1", "exec-call-1.log"))
	appendEvent := func(turnID string, typ events.Type, payload events.Payload) {
		if _, err := runtime.Sessions.append(context.Background(), events.Draft{
			SessionID: session.SessionID,
			TurnID:    turnID,
			Type:      typ,
			Payload:   payload,
		}); err != nil {
			t.Fatalf("append(%s, %T) error = %v", typ, payload, err)
		}
	}
	appendEvent("turn-1", events.TypeExecutionDeclared, events.ExecutionDeclaredPayload{
		ExecutionID:      "exec-call-1",
		ToolCallID:       "call-1",
		ToolName:         "bash",
		Kind:             "bash",
		Intent:           "server",
		Command:          []string{"/bin/sh", "-c", "npm run dev"},
		CommandPreview:   "npm run dev",
		WorkingDirectory: filepath.Join(workspaceRoot, "client"),
		TimeoutMS:        120000,
		OutputLimit:      12000,
	})
	appendEvent("turn-1", events.TypeExecutionStarted, events.ExecutionStartedPayload{
		ExecutionID: "exec-call-1",
		ToolCallID:  "call-1",
		ToolName:    "bash",
		Input:       `{"cmd":"npm run dev","workdir":"client"}`,
	})
	appendEvent("turn-1", events.TypeExecutionBackgroundStarted, events.ExecutionBackgroundStartedPayload{
		ExecutionID:     "exec-call-1",
		ToolCallID:      "call-1",
		ToolName:        "bash",
		PID:             4242,
		ProcessIdentity: "identity-old",
		SupervisorID:    "background-supervisor-old",
		LogRef:          logRef,
		ReadyPatterns:   []string{"local:"},
	})
	appendEvent("turn-1", events.TypeToolExecEnd, events.ToolExecEndPayload{
		CallID:          "call-1",
		ToolName:        "bash",
		ExecutionID:     "exec-call-1",
		ExecutionStatus: string(events.ExecutionStatusCompleted),
		Succeeded:       true,
		Output:          "Started server in background (pid 4242). Readiness not yet confirmed.",
	})

	appendTestBackgroundLog(t, runtime.Store, BackgroundExecutionLogKey{
		SessionID:   session.SessionID,
		TurnID:      "turn-1",
		ExecutionID: "exec-call-1",
	}, "Booting dev server...\nLocal: http://127.0.0.1:5173/\n")

	prevInspect := loadBackgroundProcessStateFunc
	loadBackgroundProcessStateFunc = func(pid int) (backgroundProcessState, error) {
		return backgroundProcessState{}, nil
	}
	t.Cleanup(func() {
		loadBackgroundProcessStateFunc = prevInspect
	})

	reopened := newPersistentRuntimeWithClient(t, sessionDir, &fakeProvider{})
	if _, err := reopened.OpenWorkspaceSession(context.Background(), workspaceRoot, nil, true); err != nil {
		t.Fatalf("OpenWorkspaceSession(resume) error = %v", err)
	}

	waitForBackgroundState(t, reopened, session.SessionID, func(background *events.ExecutionBackgroundState) bool {
		return background != nil &&
			background.Status == events.ExecutionBackgroundStatusExited &&
			background.Ready &&
			background.Port == 5173 &&
			background.Exited &&
			strings.Contains(background.Error, "resumed supervision")
	})
}

func waitForBackgroundState(t *testing.T, runtime *Runtime, sessionID string, match func(*events.ExecutionBackgroundState) bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("Snapshot() error = %v", err)
		}
		if turn := state.Turns["turn-1"]; turn != nil {
			if call := turn.ToolCalls["call-1"]; call != nil && call.Execution != nil && match(call.Execution.Background) {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	var background *events.ExecutionBackgroundState
	if turn := state.Turns["turn-1"]; turn != nil {
		if call := turn.ToolCalls["call-1"]; call != nil && call.Execution != nil {
			background = call.Execution.Background
		}
	}
	t.Fatalf("background state did not match: %#v", background)
}
