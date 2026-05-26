package app

import (
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

func TestBuildTurnProviderRequestProjectionUsesHistoryPlusTurnWorkState(t *testing.T) {
	historyOutput := strings.Repeat("h", 3000)
	currentOutput := strings.Repeat("x", 3500)
	input := turnLoopInput{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "agent-1",
		Instructions:    "inspect the project",
		CacheablePrefix: "repo rules",
		DynamicSuffix:   "current request",
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
	}
	history := sessionConversation{
		Inputs: []provider.Input{
			{Kind: provider.InputKindAssistantMessage, Content: "history summary"},
			{Kind: provider.InputKindToolCall, CallID: "history-call", ToolName: "read", Arguments: `{"paths":["history.go"]}`},
			{Kind: provider.InputKindToolResult, CallID: "history-call", ToolName: "read", Output: historyOutput},
		},
	}
	state := turnLoopState{
		UserInput: provider.Input{Kind: provider.InputKindUserMessage, Content: "inspect files"},
		Conversation: []provider.Input{
			{Kind: provider.InputKindUserMessage, Content: "inspect files"},
			{Kind: provider.InputKindToolCall, CallID: "call-1", ToolName: "read", Arguments: `{"paths":["a.go"]}`},
			{Kind: provider.InputKindToolResult, CallID: "call-1", ToolName: "read", Output: currentOutput},
		},
		WorkState: turnWorkState{
			Summary: turnWorkSummary{
				CompletedWork: []string{"Updated a.go"},
				Failures:      []string{"read a.go permission_denied (2x)"},
			},
		},
	}
	catalog := &fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     2048,
				MaxInputTokens:  2048,
				MaxOutputTokens: 2048,
			}},
		},
	}

	baseRequest := buildTurnBaseProviderRequest(input, nil, catalog, turnRequestOutputBudgetConfig{})
	projection, err := buildTurnProviderRequestProjection(baseRequest, history, state)
	if err != nil {
		t.Fatalf("buildTurnProviderRequestProjection() error = %v", err)
	}
	if history.Inputs[2].Output != historyOutput {
		t.Fatalf("history input mutated: %#v", history.Inputs[2])
	}
	if state.Conversation[2].Output != currentOutput {
		t.Fatalf("current-turn ledger mutated: %#v", state.Conversation[2])
	}
	if projection.Request.Inputs[2].Output != historyOutput {
		t.Fatalf("projection history prefix mutated: %#v", projection.Request.Inputs[2])
	}
	last := projection.Request.Inputs[len(projection.Request.Inputs)-1]
	if last.Kind != provider.InputKindAssistantMessage || !strings.Contains(last.Content, "Active turn summary:") {
		t.Fatalf("projection current-turn suffix = %#v", projection.Request.Inputs)
	}
	if strings.Contains(last.Content, currentOutput) {
		t.Fatalf("projection leaked raw tool chatter: %q", last.Content)
	}
	if !strings.Contains(projection.Request.Instructions, toolResultVisibilityInstruction) {
		t.Fatalf("projection instructions = %q", projection.Request.Instructions)
	}
	if !strings.Contains(projection.Request.DynamicSuffix, toolResultVisibilityInstruction) {
		t.Fatalf("projection dynamic suffix = %q", projection.Request.DynamicSuffix)
	}
	for _, input := range projection.Request.Inputs {
		if input.Kind == provider.InputKindAssistantMessage && strings.Contains(input.Content, "Compacted active-turn context:") {
			t.Fatalf("projection unexpectedly rendered deleted current-turn compaction summary: %#v", projection.Request.Inputs)
		}
	}
}

func TestBuildTurnProviderRequestProjectionPreservesActiveTurnReadResults(t *testing.T) {
	olderOutput := strings.Repeat("older read output\n", 120)
	failedError := strings.Repeat("permission denied\n", 80)
	latestOutput := strings.Repeat("latest read output\n", 120)
	input := turnLoopInput{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "agent-1",
		Instructions: "inspect the project",
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
	}
	state := turnLoopState{
		UserInput:           provider.Input{Kind: provider.InputKindUserMessage, Content: "inspect files"},
		LatestToolStepStart: 5,
		WorkState: turnWorkState{
			NativeContinuation: &turnNativeContinuation{
				Contract: "openai_tool_loop",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "inspect files"},
					{Kind: provider.InputKindToolCall, CallID: "call-1", ToolName: tool.ReadToolName, Arguments: `{"paths":["older.go"]}`},
					{Kind: provider.InputKindToolResult, CallID: "call-1", ToolName: tool.ReadToolName, Output: olderOutput},
					{Kind: provider.InputKindToolCall, CallID: "call-2", ToolName: tool.ReadToolName, Arguments: `{"paths":["denied.go"]}`},
					{Kind: provider.InputKindToolResult, CallID: "call-2", ToolName: tool.ReadToolName, Error: failedError},
					{Kind: provider.InputKindToolCall, CallID: "call-3", ToolName: tool.ReadToolName, Arguments: `{"paths":["latest.go"]}`},
					{Kind: provider.InputKindToolResult, CallID: "call-3", ToolName: tool.ReadToolName, Output: latestOutput},
				},
			},
		},
	}

	baseRequest := buildTurnBaseProviderRequest(input, nil, nil, turnRequestOutputBudgetConfig{})
	projection, err := buildTurnProviderRequestProjection(baseRequest, sessionConversation{}, state)
	if err != nil {
		t.Fatalf("buildTurnProviderRequestProjection() error = %v", err)
	}
	if projection.CurrentInputTokensSaved != 0 {
		t.Fatalf("CurrentInputTokensSaved = %d, want 0 for read results", projection.CurrentInputTokensSaved)
	}
	if state.WorkState.NativeContinuation.Inputs[2].Output != olderOutput {
		t.Fatalf("native continuation mutated: %#v", state.WorkState.NativeContinuation.Inputs[2])
	}
	if projection.CurrentInputs[2].Output != olderOutput {
		t.Fatalf("durable current input = %#v, want raw older output", projection.CurrentInputs[2])
	}
	older := projection.Request.Inputs[2]
	if older.Output != olderOutput {
		t.Fatalf("older output = %q, want preserved read output", older.Output)
	}
	failed := projection.Request.Inputs[4]
	if failed.Error != failedError {
		t.Fatalf("failed result error = %q, want preserved", failed.Error)
	}
	latest := projection.Request.Inputs[6]
	if latest.Output != latestOutput {
		t.Fatalf("latest output = %q, want preserved", latest.Output)
	}

	persisted := state
	persistTurnProviderRequestProjection(&persisted, projection)
	if persisted.WorkState.NativeContinuation.Inputs[2].Output != olderOutput {
		t.Fatalf("persisted native continuation = %#v, want raw older output", persisted.WorkState.NativeContinuation.Inputs[2])
	}
}
