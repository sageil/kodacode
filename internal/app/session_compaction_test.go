package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func testSessionCompactionBudget(limit, trigger, target int) sessionHistoryBudget {
	return sessionHistoryBudget{
		InputLimitSource:    "test",
		InputLimitTokens:    limit,
		TriggerTokens:       trigger,
		TargetTokens:        target,
		SummaryBudgetTokens: byteBudgetToTokenBudget(compactionSummaryBudgetBytes),
		SummaryBudgetBytes:  compactionSummaryBudgetBytes,
		RawTailBudgetTokens: limit,
		RawTailBudgetBytes:  tokenBudgetToByteBudget(limit),
	}
}

func testSessionCompactionRequest(turnID string) provider.Request {
	return provider.Request{
		SessionID:    "session-1",
		TurnID:       turnID,
		AgentID:      "session-history",
		Instructions: "Continue the coding task using the preserved session history.",
	}
}

func TestEstimateSessionRequestTokensUsesPreparedProviderFacingTotal(t *testing.T) {
	request := provider.Request{
		SessionID:    "session-1",
		TurnID:       "turn-4",
		AgentID:      "engineer",
		Model:        provider.ModelRef{ProviderID: "github-copilot", ModelID: "gpt-4.1"},
		Instructions: "Review the project changes and continue the task.",
		Tools: []provider.Tool{{
			Name:        "read",
			Description: "Read one or more files.",
			InputSchema: `{"type":"object","properties":{"paths":{"type":"array","items":{"type":"string"}}}}`,
		}},
	}
	historyInputs := []provider.Input{
		{Kind: provider.InputKindUserMessage, Content: "review the modified files"},
		{Kind: provider.InputKindAssistantMessage, Content: "Reviewing the changed files now."},
		{Kind: provider.InputKindToolCall, CallID: "call-1", ToolName: "read", Arguments: `{"paths":["src/server.ts"]}`},
		{Kind: provider.InputKindToolResult, CallID: "call-1", ToolName: "read", Output: strings.Repeat("x", 512)},
	}
	currentTurnInputs := []provider.Input{{Kind: provider.InputKindUserMessage, Content: "validate all changes"}}

	got := estimateSessionRequestTokens(request, historyInputs, currentTurnInputs)

	providerFacing := request
	providerFacing.Inputs = append(append([]provider.Input(nil), historyInputs...), currentTurnInputs...)
	providerFacing = provider.PreparePromptRequest(providerFacing)
	want := provider.EstimateRequestTokenBreakdown(providerFacing).TotalTokens
	if got != want {
		t.Fatalf("estimateSessionRequestTokens() = %d, want provider-facing total %d", got, want)
	}
}

func TestBuildNextCompactionUsesRequestTokenPressure(t *testing.T) {
	large := strings.Repeat("x", 3000)
	turns := map[string]*replayedSessionTurn{
		"turn-1": {
			TurnID: "turn-1",
			Inputs: []provider.Input{
				{Kind: provider.InputKindUserMessage, Content: "first"},
				{Kind: provider.InputKindAssistantMessage, Content: "one " + large},
			},
			UserText:       "first",
			AssistantText:  "one " + large,
			Terminal:       true,
			TerminalStatus: "completed",
		},
		"turn-2": {
			TurnID: "turn-2",
			Inputs: []provider.Input{
				{Kind: provider.InputKindUserMessage, Content: "second"},
				{Kind: provider.InputKindAssistantMessage, Content: "two " + large},
			},
			UserText:       "second",
			AssistantText:  "two " + large,
			Terminal:       true,
			TerminalStatus: "completed",
		},
		"turn-3": {
			TurnID: "turn-3",
			Inputs: []provider.Input{
				{Kind: provider.InputKindUserMessage, Content: "third"},
				{Kind: provider.InputKindAssistantMessage, Content: "three done"},
			},
			UserText:       "third",
			AssistantText:  "three done",
			Terminal:       true,
			TerminalStatus: "completed",
		},
	}

	order := []string{"turn-1", "turn-2", "turn-3"}
	currentTurnInputs := []provider.Input{{Kind: provider.InputKindUserMessage, Content: "continue"}}
	request := testSessionCompactionRequest("turn-4")

	payload := buildNextCompaction(
		nil,
		order,
		turns,
		request,
		currentTurnInputs,
		testSessionCompactionBudget(2048, 1600, 1200),
	)
	if payload == nil {
		t.Fatal("payload = nil")
	}
	if payload.ConsolidatedTurnCount != 3 || payload.FrontierTurnID != "turn-3" {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.NewlyConsolidatedTurnCount != 3 {
		t.Fatalf("newly compacted turn count = %d, want 3", payload.NewlyConsolidatedTurnCount)
	}
	if payload.UpdateReason != sessionHistoryCompactionReason {
		t.Fatalf("update reason = %q, want %q", payload.UpdateReason, sessionHistoryCompactionReason)
	}
	if got, want := payload.RenderedSummary, renderSessionCompactionArtifactSummary(payload.Artifact, compactionSummaryBudgetBytes); got != want {
		t.Fatalf("rendered_summary = %q, want %q", got, want)
	}
}

func TestBuildNextCompactionUsesSemanticClosureWithRetainedRawFrontierUnderTokenPressure(t *testing.T) {
	large := strings.Repeat("routing detail ", 800)
	turns := map[string]*replayedSessionTurn{
		"turn-1": {
			TurnID: "turn-1",
			Inputs: []provider.Input{
				{Kind: provider.InputKindUserMessage, Content: "inspect routing"},
				{Kind: provider.InputKindAssistantMessage, Content: "Reviewed the routing layer. " + large},
			},
			UserText:       "inspect routing",
			AssistantText:  "Reviewed the routing layer. " + large,
			Terminal:       true,
			TerminalStatus: "completed",
		},
		"turn-2": {
			TurnID: "turn-2",
			Inputs: []provider.Input{
				{Kind: provider.InputKindUserMessage, Content: "update middleware"},
				{Kind: provider.InputKindAssistantMessage, Content: "Updated the middleware."},
			},
			UserText:       "update middleware",
			AssistantText:  "Updated the middleware.",
			Terminal:       true,
			TerminalStatus: "completed",
		},
		"turn-3": {
			TurnID: "turn-3",
			Inputs: []provider.Input{
				{Kind: provider.InputKindUserMessage, Content: "refresh tests"},
				{Kind: provider.InputKindAssistantMessage, Content: "Refreshed the tests."},
			},
			UserText:       "refresh tests",
			AssistantText:  "Refreshed the tests.",
			Terminal:       true,
			TerminalStatus: "completed",
		},
	}

	payload := buildNextCompaction(
		nil,
		[]string{"turn-1", "turn-2", "turn-3"},
		turns,
		testSessionCompactionRequest("turn-4"),
		[]provider.Input{{Kind: provider.InputKindUserMessage, Content: "continue"}},
		testSessionCompactionBudget(2400, 1920, 1440),
	)
	if payload == nil {
		t.Fatal("payload = nil")
	}
	if payload.UpdateReason != events.HistoryContinuationUpdateReasonSemanticClosure {
		t.Fatalf("update reason = %q, want semantic_closure", payload.UpdateReason)
	}
	if payload.ConsolidatedTurnCount != 1 || payload.FrontierTurnID != "turn-1" {
		t.Fatalf("payload = %#v, want retained raw frontier of 2 turns", payload)
	}
	if payload.NewlyConsolidatedTurnCount != 1 {
		t.Fatalf("newly consolidated count = %d, want 1", payload.NewlyConsolidatedTurnCount)
	}
}

func TestBuildNextCompactionSkipsSemanticClosureBelowTrigger(t *testing.T) {
	large := strings.Repeat("routing detail ", 120)
	turns := map[string]*replayedSessionTurn{
		"turn-1": {
			TurnID: "turn-1",
			Inputs: []provider.Input{
				{Kind: provider.InputKindUserMessage, Content: "inspect routing"},
				{Kind: provider.InputKindAssistantMessage, Content: "Reviewed the routing layer. " + large},
			},
			UserText:       "inspect routing",
			AssistantText:  "Reviewed the routing layer. " + large,
			Terminal:       true,
			TerminalStatus: "completed",
		},
		"turn-2": {
			TurnID: "turn-2",
			Inputs: []provider.Input{
				{Kind: provider.InputKindUserMessage, Content: "update middleware"},
				{Kind: provider.InputKindAssistantMessage, Content: "Updated the middleware."},
			},
			UserText:       "update middleware",
			AssistantText:  "Updated the middleware.",
			Terminal:       true,
			TerminalStatus: "completed",
		},
		"turn-3": {
			TurnID: "turn-3",
			Inputs: []provider.Input{
				{Kind: provider.InputKindUserMessage, Content: "refresh tests"},
				{Kind: provider.InputKindAssistantMessage, Content: "Refreshed the tests."},
			},
			UserText:       "refresh tests",
			AssistantText:  "Refreshed the tests.",
			Terminal:       true,
			TerminalStatus: "completed",
		},
	}

	payload := buildNextCompaction(
		nil,
		[]string{"turn-1", "turn-2", "turn-3"},
		turns,
		testSessionCompactionRequest("turn-4"),
		[]provider.Input{{Kind: provider.InputKindUserMessage, Content: "continue"}},
		testSessionCompactionBudget(8192, 6553, 5324),
	)
	if payload != nil {
		t.Fatalf("payload = %#v, want nil below trigger", payload)
	}
}

func TestBuildNextCompactionCompactsAllEligibleTurns(t *testing.T) {
	large := strings.Repeat("token ", 800)
	turns := map[string]*replayedSessionTurn{
		"turn-1": {
			TurnID: "turn-1",
			Inputs: []provider.Input{
				{Kind: provider.InputKindUserMessage, Content: "project performance review"},
				{Kind: provider.InputKindAssistantMessage, Content: "one " + large},
			},
			UserText:       "project performance review",
			AssistantText:  "one " + large,
			Terminal:       true,
			TerminalStatus: "completed",
		},
		"turn-2": {
			TurnID: "turn-2",
			Inputs: []provider.Input{
				{Kind: provider.InputKindUserMessage, Content: "just do the work"},
				{Kind: provider.InputKindAssistantMessage, Content: "two " + large},
			},
			UserText:       "just do the work",
			AssistantText:  "two " + large,
			Terminal:       true,
			TerminalStatus: "completed",
		},
		"turn-3": {
			TurnID: "turn-3",
			Inputs: []provider.Input{
				{Kind: provider.InputKindUserMessage, Content: "continue"},
				{Kind: provider.InputKindAssistantMessage, Content: "three " + large},
			},
			UserText:       "continue",
			AssistantText:  "three " + large,
			Terminal:       true,
			TerminalStatus: "completed",
		},
	}

	payload := buildNextCompaction(
		nil,
		[]string{"turn-1", "turn-2", "turn-3"},
		turns,
		testSessionCompactionRequest("turn-4"),
		[]provider.Input{{Kind: provider.InputKindUserMessage, Content: "continue"}},
		testSessionCompactionBudget(2400, 1600, 1100),
	)
	if payload == nil {
		t.Fatal("payload = nil")
	}
	if payload.ConsolidatedTurnCount != 3 || payload.FrontierTurnID != "turn-3" {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.Artifact.SessionObjective != "project performance review" {
		t.Fatalf("session objective = %#v, want preserved concrete goal", payload.Artifact.SessionObjective)
	}
	if payload.Artifact.SessionObjective == "just do the work" {
		t.Fatalf("session objective = %#v, want low-signal steering filtered out", payload.Artifact.SessionObjective)
	}
}

func TestBuildNextCompactionExtendsExistingArtifact(t *testing.T) {
	existingArtifact := events.HistoryContinuationArtifact{
		SessionObjective: "project performance review",
		CompletedEpisodes: []events.HistoryEpisodePayload{{
			EpisodeID:     historyContinuationEpisodeID([]string{"turn-1"}),
			Summary:       "older summary",
			TouchedPaths:  []string{"internal/old.go"},
			SourceTurnIDs: []string{"turn-1"},
		}},
		WorkspaceFacts: []events.HistoryWorkspaceFactPayload{{
			Path:         "internal/old.go",
			Fact:         "older summary",
			SourceTurnID: "turn-1",
		}},
	}
	existing := &events.SessionHistoryContinuationUpdatedPayload{
		UpdateReason:               sessionHistoryCompactionReason,
		FrontierTurnID:             "turn-1",
		ConsolidatedTurnCount:      1,
		NewlyConsolidatedTurnCount: 1,
		Artifact:                   existingArtifact,
		RenderedSummary:            renderSessionCompactionArtifactSummary(existingArtifact, compactionSummaryBudgetBytes),
	}
	turns := map[string]*replayedSessionTurn{
		"turn-2": {
			TurnID: "turn-2",
			Inputs: []provider.Input{
				{Kind: provider.InputKindUserMessage, Content: "just do the work"},
				{Kind: provider.InputKindAssistantMessage, Content: "Implemented the new handler."},
			},
			UserText:       "just do the work",
			AssistantText:  "Implemented the new handler.",
			WorkspacePaths: []string{"internal/new.go"},
			Terminal:       true,
			TerminalStatus: "completed",
		},
	}

	payload := buildSessionCompactionPayload(existing, []string{"turn-1", "turn-2"}, turns, compactionSummaryBudgetBytes)
	if payload == nil {
		t.Fatal("payload = nil")
	}
	if payload.ConsolidatedTurnCount != 2 || payload.FrontierTurnID != "turn-2" {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.Artifact.SessionObjective != "project performance review" {
		t.Fatalf("session objective = %#v", payload.Artifact.SessionObjective)
	}
	summaries := make([]string, 0, len(payload.Artifact.CompletedEpisodes))
	for _, episode := range payload.Artifact.CompletedEpisodes {
		summaries = append(summaries, episode.Summary)
	}
	if !slices.Contains(summaries, "older summary") || !slices.Contains(summaries, "Implemented the new handler.") {
		t.Fatalf("completed episodes = %#v", payload.Artifact.CompletedEpisodes)
	}
	if !slices.Equal(historyContinuationArtifactPaths(payload.Artifact), []string{"internal/old.go", "internal/new.go"}) {
		t.Fatalf("workspace paths = %#v", historyContinuationArtifactPaths(payload.Artifact))
	}
}

func TestSameSessionCompactionScopeIgnoresWorkspacePathOrder(t *testing.T) {
	left := &events.SessionHistoryContinuationUpdatedPayload{
		UpdateReason:               sessionHistoryCompactionReason,
		FrontierTurnID:             "turn-2",
		ConsolidatedTurnCount:      2,
		NewlyConsolidatedTurnCount: 1,
		Artifact: events.HistoryContinuationArtifact{WorkspaceFacts: []events.HistoryWorkspaceFactPayload{
			{Path: "src/a.ts", Fact: "Touched during consolidated work", SourceTurnID: "turn-1"},
			{Path: "src/b.ts", Fact: "Touched during consolidated work", SourceTurnID: "turn-2"},
		}},
	}
	right := &events.SessionHistoryContinuationUpdatedPayload{
		UpdateReason:               sessionHistoryCompactionReason,
		FrontierTurnID:             "turn-2",
		ConsolidatedTurnCount:      2,
		NewlyConsolidatedTurnCount: 1,
		Artifact: events.HistoryContinuationArtifact{WorkspaceFacts: []events.HistoryWorkspaceFactPayload{
			{Path: "src/b.ts", Fact: "Touched during consolidated work", SourceTurnID: "turn-2"},
			{Path: "src/a.ts", Fact: "Touched during consolidated work", SourceTurnID: "turn-1"},
		}},
	}

	if !sameSessionCompactionScope(left, right) {
		t.Fatalf("sameSessionCompactionScope(%#v, %#v) = false, want true", left, right)
	}
}

func TestSessionCompactionPrefixCountReturnsZeroForMismatchedCutoff(t *testing.T) {
	payload := &events.SessionHistoryContinuationUpdatedPayload{
		ConsolidatedTurnCount: 2,
		FrontierTurnID:        "turn-missing",
	}

	if got := sessionCompactionPrefixCount([]string{"turn-1", "turn-2", "turn-3"}, payload); got != 0 {
		t.Fatalf("sessionCompactionPrefixCount() = %d, want 0", got)
	}
}

func TestCompactionGoalCandidateRejectsMetaPromptButKeepsRealCompactionTask(t *testing.T) {
	if got := compactionGoalCandidate("Create an anchored summary of the conversation and current project state for continuation."); got != "" {
		t.Fatalf("meta compaction goal = %q, want empty", got)
	}
	if got := compactionGoalCandidate("Fix compaction summary rendering in the TUI."); got != "Fix compaction summary rendering in the TUI." {
		t.Fatalf("real compaction task = %q", got)
	}
}

func TestBuildNextCompactionCompactsEligibleTurnsEvenWhenOverallRequestReductionIsSmall(t *testing.T) {
	turns := map[string]*replayedSessionTurn{
		"turn-1": {
			TurnID: "turn-1",
			Inputs: []provider.Input{
				{Kind: provider.InputKindUserMessage, Content: "a"},
				{Kind: provider.InputKindAssistantMessage, Content: "b"},
			},
			UserText:       "a",
			AssistantText:  "b",
			Terminal:       true,
			TerminalStatus: "completed",
		},
		"turn-2": {
			TurnID: "turn-2",
			Inputs: []provider.Input{
				{Kind: provider.InputKindUserMessage, Content: "c"},
				{Kind: provider.InputKindAssistantMessage, Content: "d"},
			},
			UserText:       "c",
			AssistantText:  "d",
			Terminal:       true,
			TerminalStatus: "completed",
		},
	}

	payload := buildNextCompaction(
		nil,
		[]string{"turn-1", "turn-2"},
		turns,
		provider.Request{
			SessionID:    "session-1",
			TurnID:       "turn-3",
			AgentID:      "session-history",
			Instructions: strings.Repeat("z", 8000),
		},
		[]provider.Input{{Kind: provider.InputKindUserMessage, Content: "continue"}},
		testSessionCompactionBudget(2400, 1600, 1100),
	)
	if payload == nil {
		t.Fatal("payload = nil")
	}
	if payload.ConsolidatedTurnCount != 2 || payload.FrontierTurnID != "turn-2" {
		t.Fatalf("payload = %#v", payload)
	}
}
