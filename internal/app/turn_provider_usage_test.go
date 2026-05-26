package app

import (
	"context"
	"errors"
	"io"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/provider"
)

func TestTurnRunnerRunRecordsProviderUsageEstimate(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "hello"},
		})},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:         "gpt-5",
				CostInput:  1.25,
				CostOutput: 10,
			}},
		},
	})

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "hello",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	usage := state.Turns["turn-1"].ProviderUsage
	if usage == nil {
		t.Fatal("ProviderUsage = nil")
	}
	if usage.Steps != 1 {
		t.Fatalf("Steps = %d, want 1", usage.Steps)
	}
	if usage.Attempts != 1 {
		t.Fatalf("Attempts = %d, want 1", usage.Attempts)
	}
	if usage.RequestTokens <= 0 {
		t.Fatalf("RequestTokens = %d, want > 0", usage.RequestTokens)
	}
	if usage.CompletionTokens <= 0 {
		t.Fatalf("CompletionTokens = %d, want > 0", usage.CompletionTokens)
	}
	if usage.EstimatedInputCost <= 0 {
		t.Fatalf("EstimatedInputCost = %f, want > 0", usage.EstimatedInputCost)
	}
	if usage.EstimatedOutputCost <= 0 {
		t.Fatalf("EstimatedOutputCost = %f, want > 0", usage.EstimatedOutputCost)
	}
	if len(state.Turns["turn-1"].ProviderAttempts) != 1 {
		t.Fatalf("ProviderAttempts = %d, want 1", len(state.Turns["turn-1"].ProviderAttempts))
	}
}

func TestNormalizeRecordedTurnProviderUsageClearsUnstartedEstimate(t *testing.T) {
	payload := normalizeRecordedTurnProviderUsagePayload(events.TurnProviderUsageRecordedPayload{
		Model:                                     "openai/gpt-5",
		Step:                                      1,
		Attempt:                                   1,
		RequestStarted:                            false,
		EstimatedRequestTokens:                    1000,
		EstimatedPromptTokens:                     100,
		EstimatedConversationTokens:               200,
		EstimatedToolNameTokens:                   10,
		EstimatedToolDescriptionTokens:            30,
		EstimatedToolSchemaTokens:                 60,
		EstimatedPromptCompactionTokensSaved:      50,
		EstimatedHistoryCompactionTokensSaved:     70,
		EstimatedCurrentTurnProjectionTokensSaved: 90,
		EstimatedToolDescriptionTokensSaved:       20,
		EstimatedToolSchemaTokensSaved:            40,
		EstimatedInputSavingsCost:                 0.001,
		ToolCount:                                 4,
		EstimatedCompletionTokens:                 80,
		EstimatedInputCost:                        0.002,
		EstimatedOutputCost:                       0.003,
		Error:                                     "provider rejected request",
	})

	if payload.EstimatedRequestTokens != 0 || payload.EstimatedCompletionTokens != 0 ||
		payload.EstimatedInputCost != 0 || payload.EstimatedOutputCost != 0 {
		t.Fatalf("usage estimate = %#v, want unstarted request estimate cleared", payload)
	}
	if payload.EstimatedPromptTokens != 0 || payload.EstimatedConversationTokens != 0 ||
		payload.EstimatedToolNameTokens != 0 || payload.EstimatedToolDescriptionTokens != 0 ||
		payload.EstimatedToolSchemaTokens != 0 || payload.ToolCount != 0 {
		t.Fatalf("request mix = %#v, want unstarted request mix cleared", payload)
	}
	if payload.EstimatedPromptCompactionTokensSaved != 0 ||
		payload.EstimatedHistoryCompactionTokensSaved != 0 ||
		payload.EstimatedCurrentTurnProjectionTokensSaved != 0 ||
		payload.EstimatedToolDescriptionTokensSaved != 0 ||
		payload.EstimatedToolSchemaTokensSaved != 0 ||
		payload.EstimatedInputSavingsCost != 0 {
		t.Fatalf("savings estimate = %#v, want unstarted savings cleared", payload)
	}
}

func TestTurnRunnerRunRecordsPromptCompactionSavings(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "hello"},
		})},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:        "gpt-5",
				CostInput: 1.25,
			}},
		},
	})

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	fragments := []prompt.Fragment{
		{
			Kind:      prompt.KindPolicy,
			Source:    prompt.SourceBuiltin,
			Stability: prompt.StabilityStable,
			Content: strings.Join([]string{
				"# Core standard",
				"",
				"Build for long-term quality, not short-term convenience.",
				"",
				"## Design rules",
				"- Prefer explicit contracts over implicit behavior.",
				"- Keep the runtime as the authority for orchestration.",
				"- Make prompt assembly inspectable.",
				"",
				"## Cost and user value",
				"- Treat token usage, latency, and tool churn as product costs.",
				"- Optimize for real user value, not internal cleverness.",
			}, "\n"),
		},
		{Kind: prompt.KindRole, Source: prompt.SourceProject, Stability: prompt.StabilityStable, Content: "builder"},
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "hello",
		Fragments:  fragments,
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	attempts := state.Turns["turn-1"].ProviderAttempts
	if len(attempts) != 1 {
		t.Fatalf("ProviderAttempts = %d, want 1", len(attempts))
	}
	if attempts[0].PromptCompactionTokensSaved <= 0 {
		t.Fatalf("PromptCompactionTokensSaved = %d, want > 0", attempts[0].PromptCompactionTokensSaved)
	}
}

func TestTurnRunnerRunRecordsProviderReportedUsage(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	client := &fakeProvider{
		streams: []provider.Stream{&providerUsageReportStream{
			events: []provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "hello"}},
			report: provider.UsageReport{
				RequestID:            "resp_123",
				Model:                "openai/gpt-5",
				InputTokens:          1400,
				CacheReadInputTokens: 300,
				OutputTokens:         120,
				ReasoningTokens:      40,
				TotalTokens:          1520,
			},
		}},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:            "gpt-5",
				CostInput:     1.25,
				CostCacheRead: 0.125,
				CostOutput:    10,
			}},
		},
	})

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "hello",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q, want completed", result.Status)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.ProviderReportedUsage == nil {
		t.Fatalf("ProviderReportedUsage missing: %#v", turn)
	}
	if turn.ProviderReportedUsage.CacheReadInputTokens != 300 || turn.ProviderReportedUsage.CacheWriteInputTokens != 0 {
		t.Fatalf("reported cache tokens = %#v", turn.ProviderReportedUsage)
	}
	if math.Abs(turn.ProviderUsage.EstimatedInputCost-0.0014125) > 1e-9 {
		t.Fatalf("EstimatedInputCost = %f, want 0.0014125", turn.ProviderUsage.EstimatedInputCost)
	}
	if math.Abs(turn.ProviderUsage.EstimatedOutputCost-0.0012) > 1e-9 {
		t.Fatalf("EstimatedOutputCost = %f, want 0.001200", turn.ProviderUsage.EstimatedOutputCost)
	}
	if !turn.ProviderReportedUsage.CachePricingApplied || turn.ProviderReportedUsage.CachePricingMissing {
		t.Fatalf("cache pricing flags = %#v", turn.ProviderReportedUsage)
	}
	if len(turn.ProviderAttempts) != 1 || turn.ProviderAttempts[0].ReportedRequestID != "resp_123" {
		t.Fatalf("ProviderAttempts = %#v", turn.ProviderAttempts)
	}
}

func TestTurnRunnerRunRetainsEstimatedProviderUsageAcrossRetryAttempts(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	client := &fakeProvider{
		errs: []error{
			&provider.ProviderError{Message: "openai/gpt-5: unavailable", StatusCode: 503, Retryable: true},
		},
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "hello"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:         "gpt-5",
				CostInput:  1.25,
				CostOutput: 10,
			}},
		},
	})
	runner.wait = func(_ context.Context, _ time.Duration) error { return nil }
	runner.SetSessionConfig(SessionConfig{MaxRetries: 2})

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "hello",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if len(client.countRequests) != 0 {
		t.Fatalf("count requests = %d, want 0", len(client.countRequests))
	}
	expectedRequest := provider.PreparePromptRequest(client.requests[0])
	expectedRequestTokens := provider.EstimateRequestTokens(expectedRequest)

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.ProviderUsage == nil {
		t.Fatalf("ProviderUsage missing: %#v", turn)
	}
	if turn.ProviderUsage.Attempts != 2 {
		t.Fatalf("Attempts = %d, want 2", turn.ProviderUsage.Attempts)
	}
	if turn.ProviderUsage.RequestTokens != expectedRequestTokens*2 {
		t.Fatalf("RequestTokens = %d, want %d", turn.ProviderUsage.RequestTokens, expectedRequestTokens*2)
	}
	if len(turn.ProviderAttempts) != 2 {
		t.Fatalf("ProviderAttempts = %d, want 2", len(turn.ProviderAttempts))
	}

	first := turn.ProviderAttempts[0]
	if first.Step != 1 || first.Attempt != 1 {
		t.Fatalf("first attempt = %#v", first)
	}
	if !first.RequestStarted {
		t.Fatalf("first attempt RequestStarted = false, want true")
	}
	if first.RequestTokens != expectedRequestTokens {
		t.Fatalf("first attempt RequestTokens = %d, want %d", first.RequestTokens, expectedRequestTokens)
	}
	if first.RequestTokenSource != string(provider.TokenCountSourceEstimated) {
		t.Fatalf("first attempt RequestTokenSource = %q, want estimated", first.RequestTokenSource)
	}
	if first.Error == "" || !first.Retryable {
		t.Fatalf("first attempt = %#v, want retryable failure", first)
	}
	if first.CompletionTokens != 0 || first.EstimatedOutputCost != 0 {
		t.Fatalf("first attempt output usage = %d/%f, want 0/0", first.CompletionTokens, first.EstimatedOutputCost)
	}
	if first.PromptTokens <= 0 {
		t.Fatalf("first attempt PromptTokens = %d, want prompt diagnostics retained", first.PromptTokens)
	}

	second := turn.ProviderAttempts[1]
	if second.Step != 1 || second.Attempt != 2 {
		t.Fatalf("second attempt = %#v", second)
	}
	if !second.RequestStarted {
		t.Fatalf("second attempt RequestStarted = false, want true")
	}
	if second.RequestTokens != expectedRequestTokens {
		t.Fatalf("second attempt RequestTokens = %d, want %d", second.RequestTokens, expectedRequestTokens)
	}
	if second.RequestTokenSource != string(provider.TokenCountSourceEstimated) {
		t.Fatalf("second attempt RequestTokenSource = %q, want estimated", second.RequestTokenSource)
	}
	if second.Error != "" {
		t.Fatalf("second attempt Error = %q, want empty", second.Error)
	}
}

func TestTurnRunnerRunRetainsEstimatedInputUsageForFailedProviderAttempt(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	client := &fakeProvider{
		errs: []error{
			&provider.ProviderError{Message: "mistral/devstral-small-2505: invalid model", StatusCode: 400},
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"mistral": {{
				ID:         "devstral-small-2505",
				CostInput:  0.1,
				CostOutput: 0.3,
			}},
		},
	})

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		AgentID:   "builder",
		UserText:  "hello",
		Fragments: baseFragments(),
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "mistral", ModelID: "devstral-small-2505"},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusFailed {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	expectedRequest := provider.PreparePromptRequest(client.requests[0])
	expectedRequestTokens := provider.EstimateRequestTokens(expectedRequest)
	expectedInputCost := estimatedTokenCost(expectedRequestTokens, 0.1)

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.ProviderUsage == nil {
		t.Fatalf("ProviderUsage missing: %#v", turn)
	}
	if turn.ProviderUsage.RequestTokens != expectedRequestTokens || turn.ProviderUsage.CompletionTokens != 0 {
		t.Fatalf("ProviderUsage tokens = %d/%d, want %d/0", turn.ProviderUsage.RequestTokens, turn.ProviderUsage.CompletionTokens, expectedRequestTokens)
	}
	if math.Abs(turn.ProviderUsage.EstimatedInputCost-expectedInputCost) > 1e-9 || turn.ProviderUsage.EstimatedOutputCost != 0 {
		t.Fatalf("ProviderUsage estimated cost = %f/%f, want %f/0", turn.ProviderUsage.EstimatedInputCost, turn.ProviderUsage.EstimatedOutputCost, expectedInputCost)
	}
	if len(turn.ProviderAttempts) != 1 {
		t.Fatalf("ProviderAttempts = %d, want 1", len(turn.ProviderAttempts))
	}
	attempt := turn.ProviderAttempts[0]
	if attempt.Error == "" {
		t.Fatalf("attempt = %#v, want provider error recorded", attempt)
	}
	if !attempt.RequestStarted {
		t.Fatalf("attempt RequestStarted = false, want true")
	}
	if attempt.RequestTokens != expectedRequestTokens || attempt.CompletionTokens != 0 {
		t.Fatalf("attempt tokens = %d/%d, want %d/0", attempt.RequestTokens, attempt.CompletionTokens, expectedRequestTokens)
	}
	if math.Abs(attempt.EstimatedInputCost-expectedInputCost) > 1e-9 || attempt.EstimatedOutputCost != 0 {
		t.Fatalf("attempt cost = %f/%f, want %f/0", attempt.EstimatedInputCost, attempt.EstimatedOutputCost, expectedInputCost)
	}
	if attempt.PromptTokens <= 0 {
		t.Fatalf("attempt PromptTokens = %d, want prompt diagnostics retained", attempt.PromptTokens)
	}
}

func TestTurnRunnerRunDoesNotBillUnstartedProviderAttempt(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	client, err := provider.NewRoutedClient(nil)
	if err != nil {
		t.Fatalf("NewRoutedClient() error = %v", err)
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "hello",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusFailed {
		t.Fatalf("status = %q, want failed", result.Status)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.ProviderUsage == nil {
		t.Fatalf("ProviderUsage missing: %#v", turn)
	}
	if turn.ProviderUsage.RequestTokens != 0 || turn.ProviderUsage.CompletionTokens != 0 {
		t.Fatalf("ProviderUsage tokens = %d/%d, want 0/0", turn.ProviderUsage.RequestTokens, turn.ProviderUsage.CompletionTokens)
	}
	if turn.ProviderUsage.EstimatedInputCost != 0 || turn.ProviderUsage.EstimatedOutputCost != 0 {
		t.Fatalf("ProviderUsage estimated cost = %f/%f, want 0/0", turn.ProviderUsage.EstimatedInputCost, turn.ProviderUsage.EstimatedOutputCost)
	}
	if len(turn.ProviderAttempts) != 1 {
		t.Fatalf("ProviderAttempts = %d, want 1", len(turn.ProviderAttempts))
	}
	attempt := turn.ProviderAttempts[0]
	if attempt.RequestStarted {
		t.Fatalf("attempt RequestStarted = true, want false")
	}
	if attempt.RequestTokens != 0 || attempt.EstimatedInputCost != 0 {
		t.Fatalf("attempt = %#v, want unstarted provider attempt usage normalized", attempt)
	}
	if attempt.RequestTokenSource != string(provider.TokenCountSourceEstimated) {
		t.Fatalf("attempt RequestTokenSource = %q, want estimated", attempt.RequestTokenSource)
	}
}

func TestTurnRunnerRunUsesEstimatedRequestTokensWithoutProviderCounting(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	client := &fakeProvider{
		countErrs: []error{errors.New("count unavailable")},
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "hello"},
		})},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:         "gpt-5",
				CostInput:  1.25,
				CostOutput: 10,
			}},
		},
	})

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "hello",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if len(client.countRequests) != 0 {
		t.Fatalf("count requests = %d, want 0", len(client.countRequests))
	}

	expectedRequest := provider.PreparePromptRequest(client.requests[0])
	expectedRequestTokens := provider.EstimateRequestTokens(expectedRequest)
	expectedRequestBreakdown := provider.EstimateRequestTokenBreakdown(expectedRequest)
	expectedInputCost := estimatedTokenCost(expectedRequestTokens, 1.25)
	allowedTools := make([]string, 0, len(client.requests[0].Tools))
	for _, tool := range client.requests[0].Tools {
		allowedTools = append(allowedTools, tool.Name)
	}
	expectedToolSurface := tools.providerToolSurfaceAllowed(allowedTools)
	expectedInputSavingsCost := estimatedTokenCost(expectedToolSurface.TotalTokensSaved(), 1.25)

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.ProviderUsage == nil {
		t.Fatalf("ProviderUsage missing: %#v", turn)
	}
	if turn.ProviderUsage.RequestTokens != expectedRequestTokens {
		t.Fatalf("RequestTokens = %d, want %d", turn.ProviderUsage.RequestTokens, expectedRequestTokens)
	}
	if math.Abs(turn.ProviderUsage.EstimatedInputCost-expectedInputCost) > 1e-9 {
		t.Fatalf("EstimatedInputCost = %f, want %f", turn.ProviderUsage.EstimatedInputCost, expectedInputCost)
	}
	if len(turn.ProviderAttempts) != 1 {
		t.Fatalf("ProviderAttempts = %d, want 1", len(turn.ProviderAttempts))
	}
	attempt := turn.ProviderAttempts[0]
	if attempt.RequestTokens != expectedRequestTokens {
		t.Fatalf("attempt RequestTokens = %d, want %d", attempt.RequestTokens, expectedRequestTokens)
	}
	if attempt.PromptTokens != expectedRequestBreakdown.PromptTokens || attempt.ConversationTokens != expectedRequestBreakdown.ConversationTokens {
		t.Fatalf("attempt request mix = %#v, want %#v", attempt, expectedRequestBreakdown)
	}
	if attempt.ToolNameTokens != expectedRequestBreakdown.ToolNameTokens ||
		attempt.ToolDescriptionTokens != expectedRequestBreakdown.ToolDescriptionTokens ||
		attempt.ToolSchemaTokens != expectedRequestBreakdown.ToolSchemaTokens {
		t.Fatalf("attempt tool mix = %#v, want %#v", attempt, expectedRequestBreakdown)
	}
	if attempt.ToolCount != expectedRequestBreakdown.ToolCount {
		t.Fatalf("attempt ToolCount = %d, want %d", attempt.ToolCount, expectedRequestBreakdown.ToolCount)
	}
	if attempt.ToolDescriptionTokensSaved != expectedToolSurface.DescriptionTokensSaved() ||
		attempt.ToolSchemaTokensSaved != expectedToolSurface.SchemaTokensSaved() {
		t.Fatalf("attempt tool savings = %#v, want %#v", attempt, expectedToolSurface)
	}
	if attempt.HistoryCompactionTokensSaved != 0 {
		t.Fatalf("attempt compaction savings = %#v, want none", attempt)
	}
	if math.Abs(attempt.EstimatedInputSavingsCost-expectedInputSavingsCost) > 1e-9 {
		t.Fatalf("attempt EstimatedInputSavingsCost = %f, want %f", attempt.EstimatedInputSavingsCost, expectedInputSavingsCost)
	}
	if attempt.RequestTokenSource != string(provider.TokenCountSourceEstimated) {
		t.Fatalf("attempt RequestTokenSource = %q, want estimated", attempt.RequestTokenSource)
	}
}

func TestEstimateReportedTurnProviderUsageAppliesCacheReadPricing(t *testing.T) {
	models := &fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:            "gpt-5",
				CostInput:     1.25,
				CostCacheRead: 0.125,
				CostOutput:    10,
			}},
		},
	}
	estimate := estimateReportedTurnProviderUsage(
		models,
		provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		provider.UsageReport{
			InputTokens:          1400,
			CacheReadInputTokens: 300,
			OutputTokens:         120,
		},
		turnProviderUsageEstimate{InputCost: 0.003, OutputCost: 0.002},
	)

	if math.Abs(estimate.InputCost-0.0014125) > 1e-9 {
		t.Fatalf("InputCost = %f, want 0.0014125", estimate.InputCost)
	}
	if math.Abs(estimate.OutputCost-0.0012) > 1e-9 {
		t.Fatalf("OutputCost = %f, want 0.001200", estimate.OutputCost)
	}
	if !estimate.CachePricingApplied || estimate.CachePricingMissing {
		t.Fatalf("cache pricing flags = %#v", estimate)
	}
	cacheSavings := estimateReportedCacheSavingsCost(
		models,
		provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		provider.UsageReport{
			InputTokens:          1400,
			CacheReadInputTokens: 300,
			OutputTokens:         120,
		},
	)
	if math.Abs(cacheSavings-0.0003375) > 1e-9 {
		t.Fatalf("cache savings = %f, want 0.0003375", cacheSavings)
	}
}

func TestEstimateReportedTurnProviderUsageFallsBackWhenCachePricingMissing(t *testing.T) {
	fallback := turnProviderUsageEstimate{InputCost: 0.0042, OutputCost: 0.001}
	estimate := estimateReportedTurnProviderUsage(
		&fakeModelCatalog{
			modelsByID: map[string][]provider.CatalogModel{
				"openai": {{
					ID:         "gpt-5",
					CostInput:  1.25,
					CostOutput: 10,
				}},
			},
		},
		provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		provider.UsageReport{
			InputTokens:          1400,
			CacheReadInputTokens: 300,
			OutputTokens:         120,
		},
		fallback,
	)

	if math.Abs(estimate.InputCost-fallback.InputCost) > 1e-9 {
		t.Fatalf("InputCost = %f, want fallback %f", estimate.InputCost, fallback.InputCost)
	}
	if math.Abs(estimate.OutputCost-0.0012) > 1e-9 {
		t.Fatalf("OutputCost = %f, want 0.001200", estimate.OutputCost)
	}
	if estimate.CachePricingApplied || !estimate.CachePricingMissing {
		t.Fatalf("cache pricing flags = %#v", estimate)
	}
}

func TestEstimateReportedTurnProviderUsagePrefersProviderReportedModelRef(t *testing.T) {
	estimate := estimateReportedTurnProviderUsage(
		&fakeModelCatalog{
			modelsByID: map[string][]provider.CatalogModel{
				"proxy": {{
					ID:         "alias",
					CostInput:  5,
					CostOutput: 20,
				}},
				"openai": {{
					ID:         "gpt-5-mini",
					CostInput:  0.25,
					CostOutput: 2,
				}},
			},
		},
		provider.ModelRef{ProviderID: "proxy", ModelID: "alias"},
		provider.UsageReport{
			Model:        "openai/gpt-5-mini",
			InputTokens:  1000,
			OutputTokens: 100,
		},
		turnProviderUsageEstimate{InputCost: 0.005, OutputCost: 0.002},
	)

	if math.Abs(estimate.InputCost-0.00025) > 1e-9 {
		t.Fatalf("InputCost = %f, want 0.000250", estimate.InputCost)
	}
	if math.Abs(estimate.OutputCost-0.0002) > 1e-9 {
		t.Fatalf("OutputCost = %f, want 0.000200", estimate.OutputCost)
	}
}

func TestTurnRunnerRunRecordsProviderRouteTrace(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	primary := &appRouteClient{stream: provider.NewSliceStream([]provider.Event{
		{Kind: provider.EventKindAssistantDelta, AssistantDelta: "ok"},
	})}
	client, err := provider.NewRoutedClient(map[string]provider.Client{
		"proxy": primary,
	})
	if err != nil {
		t.Fatalf("provider.NewRoutedClient() error = %v", err)
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"proxy": {{ID: "primary", CostInput: 0.5, CostOutput: 1}},
		},
	})

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		AgentID:   "builder",
		UserText:  "hello",
		Fragments: baseFragments(),
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "proxy", ModelID: "primary"},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.ProviderUsage == nil {
		t.Fatalf("turn/provider usage missing: %#v", turn)
	}
	if turn.ProviderUsage.Model != "proxy/primary" {
		t.Fatalf("ProviderUsage.Model = %q, want proxy/primary", turn.ProviderUsage.Model)
	}
	if len(turn.ProviderAttempts) != 1 {
		t.Fatalf("ProviderAttempts = %d, want 1", len(turn.ProviderAttempts))
	}
	attempt := turn.ProviderAttempts[0]
	if attempt.RequestedModel != "proxy/primary" {
		t.Fatalf("RequestedModel = %q, want proxy/primary", attempt.RequestedModel)
	}
	if attempt.Model != "proxy/primary" {
		t.Fatalf("Model = %q, want proxy/primary", attempt.Model)
	}
	if len(attempt.RouteAttempts) != 1 {
		t.Fatalf("RouteAttempts = %d, want 1", len(attempt.RouteAttempts))
	}
	if attempt.RouteAttempts[0].Model != "proxy/primary" || !attempt.RouteAttempts[0].Selected || attempt.RouteAttempts[0].Error != "" {
		t.Fatalf("route attempt = %#v", attempt.RouteAttempts[0])
	}
}

func TestTurnRunnerRunRecordsRequestMixForSelectedPreparedRequest(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	primary := &capturingRouteClient{stream: provider.NewSliceStream([]provider.Event{
		{Kind: provider.EventKindAssistantDelta, AssistantDelta: "ok"},
	})}
	client, err := provider.NewRoutedClient(map[string]provider.Client{
		"github-copilot": provider.NewPromptingClient(primary),
	})
	if err != nil {
		t.Fatalf("provider.NewRoutedClient() error = %v", err)
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"github-copilot": {{
				ID:         "claude-sonnet-4",
				CostInput:  0.5,
				CostOutput: 1,
			}},
		},
	})

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		AgentID:   "builder",
		UserText:  "hello",
		Fragments: baseFragments(),
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "github-copilot", ModelID: "claude-sonnet-4"},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}
	if len(primary.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(primary.requests))
	}

	expected := provider.EstimateRequestTokenBreakdown(primary.requests[0])

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || len(turn.ProviderAttempts) != 1 {
		t.Fatalf("turn/provider attempts missing: %#v", turn)
	}
	attempt := turn.ProviderAttempts[0]
	if attempt.Model != "github-copilot/claude-sonnet-4" {
		t.Fatalf("attempt = %#v", attempt)
	}
	if attempt.RequestTokens != expected.TotalTokens {
		t.Fatalf("attempt RequestTokens = %d, want %d", attempt.RequestTokens, expected.TotalTokens)
	}
	if attempt.PromptTokens != expected.PromptTokens || attempt.ConversationTokens != expected.ConversationTokens {
		t.Fatalf("attempt request mix = %#v, want %#v", attempt, expected)
	}
	if attempt.ToolNameTokens != expected.ToolNameTokens ||
		attempt.ToolDescriptionTokens != expected.ToolDescriptionTokens ||
		attempt.ToolSchemaTokens != expected.ToolSchemaTokens {
		t.Fatalf("attempt tool mix = %#v, want %#v", attempt, expected)
	}
	instructions := strings.Join(strings.Fields(primary.requests[0].Instructions), " ")
	if !strings.Contains(instructions, "reason from that evidence first") {
		t.Fatalf("request instructions = %q, want anthropic provider prompt", primary.requests[0].Instructions)
	}
}

type appRouteClient struct {
	stream provider.Stream
	err    error
}

func (c *appRouteClient) Stream(_ context.Context, _ provider.Request) (provider.Stream, error) {
	if c.err != nil {
		return nil, c.err
	}
	return c.stream, nil
}

type capturingRouteClient struct {
	stream   provider.Stream
	err      error
	requests []provider.Request
}

func (c *capturingRouteClient) Stream(_ context.Context, req provider.Request) (provider.Stream, error) {
	c.requests = append(c.requests, req)
	if c.err != nil {
		return nil, c.err
	}
	return c.stream, nil
}

type providerUsageReportStream struct {
	events []provider.Event
	index  int
	report provider.UsageReport
}

func (s *providerUsageReportStream) Recv() (provider.Event, error) {
	if s.index >= len(s.events) {
		return provider.Event{}, io.EOF
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}

func (s *providerUsageReportStream) Close() error {
	return nil
}

func (s *providerUsageReportStream) UsageReport() (provider.UsageReport, bool) {
	return s.report, true
}
