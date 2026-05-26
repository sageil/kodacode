package app

import (
	"context"
	"math"
	"testing"

	"github.com/sageil/kodacode/internal/events"
)

func TestSessionServiceUsageSummaryIncludesDelegatedChildSessions(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	ctx := context.Background()
	workspaceRoot := t.TempDir()
	for _, sessionID := range []string{"session-parent", "session-child"} {
		if _, err := sessions.CreateSession(ctx, CreateSessionInput{
			SessionID:     sessionID,
			WorkspaceRoot: workspaceRoot,
		}); err != nil {
			t.Fatalf("CreateSession(%q) error = %v", sessionID, err)
		}
	}

	handoff := events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: "session-parent",
		ParentTurnID:    "turn-parent",
		ParentAgentID:   "builder",
		ChildSessionID:  "session-child",
		ChildTurnID:     "turn-child",
		ChildAgentID:    "planner",
		Task:            "Plan the implementation.",
		ContextSummary:  "Ground the plan in the repo.",
		Model:           "openai/gpt-5",
	}
	for _, draft := range []events.Draft{
		{SessionID: "session-parent", TurnID: "turn-parent", Type: events.TypeAgentHandoff, Payload: handoff},
		{SessionID: "session-child", TurnID: "turn-child", Type: events.TypeAgentHandoff, Payload: handoff},
		{SessionID: "session-parent", TurnID: "turn-parent", Type: events.TypeToolCallDeclared, Payload: events.ToolCallDeclaredPayload{
			CallID:   "call-parent-1",
			ToolName: "delegate",
			Input:    `{"agent":"planner"}`,
		}},
		{SessionID: "session-child", TurnID: "turn-child", Type: events.TypeToolCallDeclared, Payload: events.ToolCallDeclaredPayload{
			CallID:   "call-child-1",
			ToolName: "read",
			Input:    `{"paths":["README.md"]}`,
		}},
		{SessionID: "session-parent", TurnID: "turn-parent", Type: events.TypeToolExecEnd, Payload: events.ToolExecEndPayload{
			CallID:    "call-parent-1",
			ToolName:  "delegate",
			Succeeded: true,
			Output:    "delegated planner session-child",
		}},
		{SessionID: "session-child", TurnID: "turn-child", Type: events.TypeToolExecEnd, Payload: events.ToolExecEndPayload{
			CallID:       "call-child-1",
			ToolName:     "read",
			Error:        "read README.md first, then retry apply_patch",
			FailureClass: toolFailureClassContract,
			Succeeded:    false,
		}},
	} {
		if _, err := sessions.append(ctx, draft); err != nil {
			t.Fatalf("append handoff error = %v", err)
		}
	}
	if _, err := sessions.append(ctx, events.Draft{
		SessionID: "session-parent",
		TurnID:    "turn-parent",
		Type:      events.TypeTurnProviderUsageRecorded,
		Payload: events.TurnProviderUsageRecordedPayload{
			Model:                     "openai/gpt-5-mini",
			Step:                      1,
			Attempt:                   1,
			EstimatedRequestTokens:    400,
			EstimatedCompletionTokens: 80,
			EstimatedInputCost:        0.0012,
			EstimatedOutputCost:       0.0006,
		},
	}); err != nil {
		t.Fatalf("append parent usage error = %v", err)
	}
	if _, err := sessions.append(ctx, events.Draft{
		SessionID: "session-child",
		TurnID:    "turn-child",
		Type:      events.TypeTurnProviderUsageRecorded,
		Payload: events.TurnProviderUsageRecordedPayload{
			Model:                     "openai/gpt-5",
			Step:                      1,
			Attempt:                   1,
			EstimatedRequestTokens:    700,
			EstimatedCompletionTokens: 120,
			EstimatedInputCost:        0.003,
			EstimatedOutputCost:       0.0014,
		},
	}); err != nil {
		t.Fatalf("append child recorded usage error = %v", err)
	}
	if _, err := sessions.append(ctx, events.Draft{
		SessionID: "session-child",
		TurnID:    "turn-child",
		Type:      events.TypeTurnProviderUsageReported,
		Payload: events.TurnProviderUsageReportedPayload{
			Model:                "openai/gpt-5",
			RequestID:            "resp_child_1",
			Step:                 1,
			Attempt:              1,
			InputTokens:          650,
			CacheReadInputTokens: 90,
			OutputTokens:         110,
			ReasoningTokens:      25,
			TotalTokens:          760,
			EstimatedInputCost:   0.0027,
			EstimatedOutputCost:  0.0013,
			CachePricingApplied:  true,
		},
	}); err != nil {
		t.Fatalf("append child reported usage error = %v", err)
	}

	summary, err := sessions.UsageSummary(ctx, "session-parent")
	if err != nil {
		t.Fatalf("UsageSummary() error = %v", err)
	}

	if !summary.ValidFor("session-parent") {
		t.Fatalf("summary root = %q, want session-parent", summary.RootSessionID)
	}
	if summary.SessionCount != 2 || summary.ChildSessionCount != 1 {
		t.Fatalf("session counts = root:%d child:%d, want 2/1", summary.SessionCount, summary.ChildSessionCount)
	}
	if summary.UsageTurns != 2 || summary.DelegatedUsageTurns() != 1 {
		t.Fatalf("usage turns = total:%d delegated:%d, want 2/1", summary.UsageTurns, summary.DelegatedUsageTurns())
	}
	if summary.ToolCalls != 2 || summary.DelegatedToolCalls() != 1 {
		t.Fatalf("tool calls = total:%d delegated:%d, want 2/1", summary.ToolCalls, summary.DelegatedToolCalls())
	}
	if summary.CompletedToolCalls != 2 || summary.DelegatedCompletedToolCalls() != 1 {
		t.Fatalf("completed tool calls = total:%d delegated:%d, want 2/1", summary.CompletedToolCalls, summary.DelegatedCompletedToolCalls())
	}
	if summary.FailedToolCalls != 1 || summary.DelegatedFailedToolCalls() != 1 {
		t.Fatalf("failed tool calls = total:%d delegated:%d, want 1/1", summary.FailedToolCalls, summary.DelegatedFailedToolCalls())
	}
	if summary.ContractViolationCalls != 1 || summary.DelegatedContractViolationCalls() != 1 {
		t.Fatalf("contract violations = total:%d delegated:%d, want 1/1", summary.ContractViolationCalls, summary.DelegatedContractViolationCalls())
	}
	if summary.Local.CompletedToolCalls != 1 || summary.Local.FailedToolCalls != 0 || summary.Local.ContractViolationCalls != 0 {
		t.Fatalf("local tool outcomes = %#v", summary.Local)
	}
	if summary.RequestTokens != 1050 || summary.CompletionTokens != 190 {
		t.Fatalf("summary tokens = %d/%d, want 1050/190", summary.RequestTokens, summary.CompletionTokens)
	}
	if summary.Local.RequestTokens != 400 || summary.Local.CompletionTokens != 80 {
		t.Fatalf("local tokens = %d/%d, want 400/80", summary.Local.RequestTokens, summary.Local.CompletionTokens)
	}
	if summary.DelegatedRequestTokens() != 650 || summary.DelegatedCompletionTokens() != 110 {
		t.Fatalf("delegated tokens = %d/%d, want 650/110", summary.DelegatedRequestTokens(), summary.DelegatedCompletionTokens())
	}
	if summary.CacheReadInputTokens != 90 || summary.ReasoningTokens != 25 {
		t.Fatalf("aggregate reported extras = cache:%d reasoning:%d, want 90/25", summary.CacheReadInputTokens, summary.ReasoningTokens)
	}
	if math.Abs(summary.EstimatedCost-0.0058) > 1e-9 {
		t.Fatalf("EstimatedCost = %f, want 0.0058", summary.EstimatedCost)
	}
	if math.Abs(summary.DelegatedEstimatedCost()-0.004) > 1e-9 {
		t.Fatalf("DelegatedEstimatedCost = %f, want 0.004", summary.DelegatedEstimatedCost())
	}
	if summary.Local.Exact || summary.Exact {
		t.Fatalf("exact flags = local:%v total:%v, want local:false total:false", summary.Local.Exact, summary.Exact)
	}
	if len(summary.Sessions) != 2 || summary.Sessions[1].AgentID != "planner" {
		t.Fatalf("session entries = %#v", summary.Sessions)
	}
}

func TestSessionServiceBudgetStatusIncludesDelegatedChildCost(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	ctx := context.Background()
	workspaceRoot := t.TempDir()
	for _, sessionID := range []string{"session-parent", "session-child"} {
		if _, err := sessions.CreateSession(ctx, CreateSessionInput{
			SessionID:     sessionID,
			WorkspaceRoot: workspaceRoot,
		}); err != nil {
			t.Fatalf("CreateSession(%q) error = %v", sessionID, err)
		}
	}

	handoff := events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: "session-parent",
		ParentTurnID:    "turn-parent",
		ParentAgentID:   "builder",
		ChildSessionID:  "session-child",
		ChildTurnID:     "turn-child",
		ChildAgentID:    "reviewer",
		Task:            "Review the code.",
		ContextSummary:  "Inspect the runtime boundary.",
		Model:           "openai/gpt-5",
	}
	if _, err := sessions.append(ctx, events.Draft{
		SessionID: "session-parent",
		TurnID:    "turn-parent",
		Type:      events.TypeAgentHandoff,
		Payload:   handoff,
	}); err != nil {
		t.Fatalf("append handoff error = %v", err)
	}
	for _, draft := range []events.Draft{
		{
			SessionID: "session-parent",
			TurnID:    "turn-parent",
			Type:      events.TypeTurnProviderUsageRecorded,
			Payload: events.TurnProviderUsageRecordedPayload{
				Model:                     "openai/gpt-5-mini",
				Step:                      1,
				Attempt:                   1,
				EstimatedRequestTokens:    300,
				EstimatedCompletionTokens: 50,
				EstimatedInputCost:        0.001,
				EstimatedOutputCost:       0.0004,
			},
		},
		{
			SessionID: "session-child",
			TurnID:    "turn-child",
			Type:      events.TypeTurnProviderUsageRecorded,
			Payload: events.TurnProviderUsageRecordedPayload{
				Model:                     "openai/gpt-5",
				Step:                      1,
				Attempt:                   1,
				EstimatedRequestTokens:    900,
				EstimatedCompletionTokens: 140,
				EstimatedInputCost:        0.004,
				EstimatedOutputCost:       0.0015,
			},
		},
	} {
		if _, err := sessions.append(ctx, draft); err != nil {
			t.Fatalf("append usage error = %v", err)
		}
	}

	status, err := sessions.BudgetStatus(ctx, "session-parent", SessionConfig{
		Budget:     0.006,
		BudgetWarn: 0.8,
	})
	if err != nil {
		t.Fatalf("BudgetStatus() error = %v", err)
	}

	if math.Abs(status.SessionCost-0.0069) > 1e-9 {
		t.Fatalf("SessionCost = %f, want 0.0069", status.SessionCost)
	}
	if !status.SessionExceeded || !status.SessionWarn {
		t.Fatalf("session budget flags = warn:%v exceeded:%v", status.SessionWarn, status.SessionExceeded)
	}
}

func TestSessionServiceUsageSummaryKeepsHybridDelegatedTokenTotals(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	ctx := context.Background()
	workspaceRoot := t.TempDir()
	for _, sessionID := range []string{"session-parent", "session-child"} {
		if _, err := sessions.CreateSession(ctx, CreateSessionInput{
			SessionID:     sessionID,
			WorkspaceRoot: workspaceRoot,
		}); err != nil {
			t.Fatalf("CreateSession(%q) error = %v", sessionID, err)
		}
	}

	handoff := events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: "session-parent",
		ParentTurnID:    "turn-parent",
		ParentAgentID:   "builder",
		ChildSessionID:  "session-child",
		ChildTurnID:     "turn-child",
		ChildAgentID:    "reviewer",
		Task:            "Review the runtime boundary.",
		ContextSummary:  "Focus on delegated token accounting.",
		Model:           "openai/gpt-5",
	}
	for _, draft := range []events.Draft{
		{SessionID: "session-parent", TurnID: "turn-parent", Type: events.TypeAgentHandoff, Payload: handoff},
		{SessionID: "session-child", TurnID: "turn-child", Type: events.TypeAgentHandoff, Payload: handoff},
		{
			SessionID: "session-child",
			TurnID:    "turn-child",
			Type:      events.TypeTurnProviderUsageRecorded,
			Payload: events.TurnProviderUsageRecordedPayload{
				Model:                     "openai/gpt-5",
				Step:                      1,
				Attempt:                   1,
				EstimatedRequestTokens:    900,
				EstimatedCompletionTokens: 100,
				EstimatedInputCost:        0.0045,
				EstimatedOutputCost:       0.0005,
			},
		},
		{
			SessionID: "session-child",
			TurnID:    "turn-child",
			Type:      events.TypeTurnProviderUsageRecorded,
			Payload: events.TurnProviderUsageRecordedPayload{
				Model:                     "openai/gpt-5",
				Step:                      2,
				Attempt:                   1,
				EstimatedRequestTokens:    700,
				EstimatedCompletionTokens: 80,
				EstimatedInputCost:        0.0035,
				EstimatedOutputCost:       0.0004,
			},
		},
		{
			SessionID: "session-child",
			TurnID:    "turn-child",
			Type:      events.TypeTurnProviderUsageReported,
			Payload: events.TurnProviderUsageReportedPayload{
				Model:               "openai/gpt-5",
				RequestID:           "resp_child_1",
				Step:                2,
				Attempt:             1,
				InputTokens:         650,
				OutputTokens:        90,
				ReasoningTokens:     30,
				TotalTokens:         740,
				EstimatedInputCost:  0.00325,
				EstimatedOutputCost: 0.00045,
			},
		},
	} {
		if _, err := sessions.append(ctx, draft); err != nil {
			t.Fatalf("append(%s) error = %v", draft.Type, err)
		}
	}

	summary, err := sessions.UsageSummary(ctx, "session-parent")
	if err != nil {
		t.Fatalf("UsageSummary() error = %v", err)
	}

	if summary.RequestTokens != 1550 || summary.CompletionTokens != 190 {
		t.Fatalf("summary tokens = %d/%d, want 1550/190", summary.RequestTokens, summary.CompletionTokens)
	}
	if summary.DelegatedRequestTokens() != 1550 || summary.DelegatedCompletionTokens() != 190 {
		t.Fatalf("delegated tokens = %d/%d, want 1550/190", summary.DelegatedRequestTokens(), summary.DelegatedCompletionTokens())
	}
	if summary.ReasoningTokens != 30 {
		t.Fatalf("summary reasoning tokens = %d, want 30", summary.ReasoningTokens)
	}
	if summary.Exact || !summary.Local.Exact {
		t.Fatalf("exact flags = total:%v local:%v, want false/true", summary.Exact, summary.Local.Exact)
	}
	if summary.Local.RequestTokens != 0 || summary.Local.CompletionTokens != 0 {
		t.Fatalf("local tokens = %d/%d, want 0/0", summary.Local.RequestTokens, summary.Local.CompletionTokens)
	}
}

func TestSessionServiceUsageSummaryIncludesFailedDelegatedChildUsage(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	ctx := context.Background()
	workspaceRoot := t.TempDir()
	for _, sessionID := range []string{"session-parent", "session-child"} {
		if _, err := sessions.CreateSession(ctx, CreateSessionInput{
			SessionID:     sessionID,
			WorkspaceRoot: workspaceRoot,
		}); err != nil {
			t.Fatalf("CreateSession(%q) error = %v", sessionID, err)
		}
	}

	handoff := events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: "session-parent",
		ParentTurnID:    "turn-parent",
		ParentAgentID:   "builder",
		ChildSessionID:  "session-child",
		ChildTurnID:     "turn-child",
		ChildAgentID:    "reviewer",
		Task:            "Review the code and report issues.",
		ContextSummary:  "Focus on runtime accounting.",
		Model:           "openai/gpt-5",
	}
	for _, draft := range []events.Draft{
		{SessionID: "session-parent", TurnID: "turn-parent", Type: events.TypeAgentHandoff, Payload: handoff},
		{SessionID: "session-child", TurnID: "turn-child", Type: events.TypeAgentHandoff, Payload: handoff},
		{
			SessionID: "session-child",
			TurnID:    "turn-child",
			Type:      events.TypeTurnProviderUsageRecorded,
			Payload: events.TurnProviderUsageRecordedPayload{
				Model:                     "openai/gpt-5",
				Step:                      1,
				Attempt:                   1,
				EstimatedRequestTokens:    820,
				EstimatedCompletionTokens: 120,
				EstimatedInputCost:        0.0031,
				EstimatedOutputCost:       0.0012,
			},
		},
		{
			SessionID: "session-child",
			TurnID:    "turn-child",
			Type:      events.TypeTurnProviderUsageReported,
			Payload: events.TurnProviderUsageReportedPayload{
				Model:               "openai/gpt-5",
				RequestID:           "resp_child_1",
				Step:                1,
				Attempt:             1,
				InputTokens:         780,
				OutputTokens:        110,
				ReasoningTokens:     15,
				TotalTokens:         905,
				EstimatedInputCost:  0.0029,
				EstimatedOutputCost: 0.0011,
			},
		},
		{
			SessionID: "session-child",
			TurnID:    "turn-child",
			Type:      events.TypeTurnError,
			Payload: events.TurnErrorPayload{
				Message: "The provider could not complete this request.",
			},
		},
		{
			SessionID: "session-parent",
			TurnID:    "turn-parent",
			Type:      events.TypeAgentResult,
			Payload: events.AgentResultPayload{
				HandoffID:      "handoff-1",
				ChildSessionID: "session-child",
				ChildTurnID:    "turn-child",
				Status:         events.AgentResultStatusFailed,
				Error:          "The provider could not complete this request.",
			},
		},
		{
			SessionID: "session-child",
			TurnID:    "turn-child",
			Type:      events.TypeAgentResult,
			Payload: events.AgentResultPayload{
				HandoffID:      "handoff-1",
				ChildSessionID: "session-child",
				ChildTurnID:    "turn-child",
				Status:         events.AgentResultStatusFailed,
				Error:          "The provider could not complete this request.",
			},
		},
	} {
		if _, err := sessions.append(ctx, draft); err != nil {
			t.Fatalf("append(%s) error = %v", draft.Type, err)
		}
	}

	childState, err := sessions.Snapshot(ctx, "session-child")
	if err != nil {
		t.Fatalf("Snapshot(child) error = %v", err)
	}
	if childState.Turns["turn-child"] == nil || childState.Turns["turn-child"].Status != events.TurnStatusFailed {
		t.Fatalf("child turn = %#v, want failed turn with usage preserved", childState.Turns["turn-child"])
	}

	summary, err := sessions.UsageSummary(ctx, "session-parent")
	if err != nil {
		t.Fatalf("UsageSummary() error = %v", err)
	}
	if summary.RequestTokens != 780 || summary.CompletionTokens != 110 {
		t.Fatalf("summary tokens = %d/%d, want 780/110", summary.RequestTokens, summary.CompletionTokens)
	}
	if summary.DelegatedRequestTokens() != 780 || summary.DelegatedCompletionTokens() != 110 {
		t.Fatalf("delegated tokens = %d/%d, want 780/110", summary.DelegatedRequestTokens(), summary.DelegatedCompletionTokens())
	}
	if math.Abs(summary.EstimatedCost-0.004) > 1e-9 {
		t.Fatalf("EstimatedCost = %f, want 0.0040", summary.EstimatedCost)
	}
	if summary.ReasoningTokens != 15 {
		t.Fatalf("ReasoningTokens = %d, want 15", summary.ReasoningTokens)
	}

	status, err := sessions.BudgetStatus(ctx, "session-parent", SessionConfig{
		Budget: 0.003,
	})
	if err != nil {
		t.Fatalf("BudgetStatus() error = %v", err)
	}
	if math.Abs(status.SessionCost-0.004) > 1e-9 {
		t.Fatalf("SessionCost = %f, want 0.0040", status.SessionCost)
	}
	if !status.SessionExceeded {
		t.Fatalf("SessionExceeded = false, want true")
	}
}
