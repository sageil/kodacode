package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

func TestStepStreamEventHandlerAssistantDeltaContinuesAfterBatchableResults(t *testing.T) {
	runner := &TurnRunner{}
	stepHasBatchableResults := true
	stepHasToolCalls := false
	completionTokens := 0
	segment := ""
	progress := newStepToolProgress()
	result := assistantRoundtripResult{}
	handler := newStepStreamEventHandler(stepStreamEventHandlerInput{
		Runner:                  runner,
		StepHasBatchableResults: &stepHasBatchableResults,
		StepHasToolCalls:        &stepHasToolCalls,
		CompletionTokens:        &completionTokens,
		Progress:                &progress,
		Result:                  &result,
		Preview: stepAssistantPreview{
			segment: &segment,
		},
	})

	eventResult, err := handler.Handle(provider.Event{
		Kind:           provider.EventKindAssistantDelta,
		AssistantDelta: "stop",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if eventResult.Complete {
		t.Fatalf("eventResult = %#v", eventResult)
	}
	if completionTokens != provider.EstimateTextTokens("stop") {
		t.Fatalf("completionTokens = %d", completionTokens)
	}
	if segment != "" {
		t.Fatalf("segment = %q", segment)
	}
}

func TestStepStreamEventHandlerCollectsBatchableToolsAcrossAssistantDelta(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, stubTool{
		definition: tool.Definition{Name: tool.ReadToolName, InputSchema: json.RawMessage(`{"type":"object"}`), ParallelSafe: true},
		result:     tool.Result{Output: "read output"},
	})
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	runner := &TurnRunner{sessions: sessions, tools: executor}
	state := turnLoopState{}
	collector := newStepToolCallCollector(false)
	batch := stepToolBatch{StepIndex: 1}
	progress := newStepToolProgress()
	result := assistantRoundtripResult{}
	stepHasBatchableResults := false
	stepHasToolCalls := false
	completionTokens := 0
	durableProgress := false
	stepStart := -1
	segment := ""
	commits := 0
	preview := newStepAssistantPreview(stepAssistantPreviewInput{
		Runner:       runner,
		Context:      context.Background(),
		SessionID:    "session-1",
		TurnID:       "turn-1",
		State:        &state,
		Segment:      &segment,
		HasToolCalls: &stepHasToolCalls,
	})
	handler := newStepStreamEventHandler(stepStreamEventHandlerInput{
		Runner:                  runner,
		Context:                 context.Background(),
		SessionID:               "session-1",
		TurnID:                  "turn-1",
		Model:                   provider.ModelRef{ProviderID: "test", ModelID: "model"},
		State:                   &state,
		Collector:               collector,
		Batch:                   &batch,
		Preview:                 preview,
		StepConversationStart:   &stepStart,
		Executor:                newStepToolExecutor(executor, []string{tool.ReadToolName}, nil, nil, nil),
		Progress:                &progress,
		Result:                  &result,
		StepHasBatchableResults: &stepHasBatchableResults,
		StepHasToolCalls:        &stepHasToolCalls,
		CompletionTokens:        &completionTokens,
		DurableProgress:         &durableProgress,
		CommitStepState: func() {
			commits++
		},
	})

	eventResult, err := handler.Handle(provider.Event{
		Kind:       provider.EventKindToolCallDelta,
		ToolCallID: "call-1",
		ToolName:   tool.ReadToolName,
		InputDelta: `{"paths":["a.go"]}`,
	})
	if err != nil {
		t.Fatalf("Handle(tool delta) error = %v", err)
	}
	if eventResult.Complete {
		t.Fatalf("eventResult = %#v", eventResult)
	}

	eventResult, err = handler.Handle(provider.Event{
		Kind:       provider.EventKindToolCallDone,
		ToolCallID: "call-1",
		ToolName:   tool.ReadToolName,
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if eventResult.Complete {
		t.Fatalf("eventResult = %#v", eventResult)
	}
	if !stepHasBatchableResults || !stepHasToolCalls || durableProgress {
		t.Fatalf("stepHasBatchableResults=%v stepHasToolCalls=%v durableProgress=%v", stepHasBatchableResults, stepHasToolCalls, durableProgress)
	}
	if result.Outcome != "" || progress.ExecutedTools != 0 {
		t.Fatalf("result=%#v progress=%#v", result, progress)
	}
	wantTokens := provider.EstimateTextTokens(tool.ReadToolName) + provider.EstimateTextTokens(`{"paths":["a.go"]}`)
	if completionTokens != wantTokens {
		t.Fatalf("completionTokens = %d, want %d", completionTokens, wantTokens)
	}

	eventResult, err = handler.Handle(provider.Event{
		Kind:           provider.EventKindAssistantDelta,
		AssistantDelta: "barrier",
	})
	if err != nil {
		t.Fatalf("Handle(assistant delta) error = %v", err)
	}
	if eventResult.Complete {
		t.Fatalf("eventResult = %#v", eventResult)
	}
	if durableProgress || progress.ExecutedTools != 0 {
		t.Fatalf("durableProgress=%v progress=%#v", durableProgress, progress)
	}
	wantTokens += provider.EstimateTextTokens("barrier")
	if completionTokens != wantTokens {
		t.Fatalf("completionTokens after barrier = %d, want %d", completionTokens, wantTokens)
	}

	eventResult, err = handler.Handle(provider.Event{
		Kind:       provider.EventKindToolCallDelta,
		ToolCallID: "call-2",
		ToolName:   tool.ReadToolName,
		InputDelta: `{"paths":["b.go"]}`,
	})
	if err != nil {
		t.Fatalf("Handle(second tool delta) error = %v", err)
	}
	if eventResult.Complete {
		t.Fatalf("eventResult = %#v", eventResult)
	}
	eventResult, err = handler.Handle(provider.Event{
		Kind:       provider.EventKindToolCallDone,
		ToolCallID: "call-2",
		ToolName:   tool.ReadToolName,
	})
	if err != nil {
		t.Fatalf("Handle(second tool done) error = %v", err)
	}
	if eventResult.Complete {
		t.Fatalf("eventResult = %#v", eventResult)
	}
	if batch.Len() != 2 || progress.ExecutedTools != 0 {
		t.Fatalf("batch=%#v progress=%#v", batch, progress)
	}

	eventResult, err = handler.executePendingToolBatchAndComplete()
	if err != nil {
		t.Fatalf("executePendingToolBatchAndComplete() error = %v", err)
	}
	if !eventResult.Complete || eventResult.Result.Outcome != assistantRoundtripOutcomeToolResult {
		t.Fatalf("eventResult = %#v", eventResult)
	}
	if !durableProgress || progress.ExecutedTools != 2 {
		t.Fatalf("durableProgress=%v progress=%#v", durableProgress, progress)
	}
	if result.Outcome != assistantRoundtripOutcomeToolResult {
		t.Fatalf("result = %#v", result)
	}
	if commits < 2 {
		t.Fatalf("commits = %d", commits)
	}
}

func TestStepStreamEventHandlerCollectsReadOnlyBashBatch(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, stubTool{
		definition: tool.Definition{Name: tool.BashToolName, InputSchema: json.RawMessage(`{"type":"object"}`)},
		result:     tool.Result{Output: "ok"},
	})
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	runner := &TurnRunner{sessions: sessions, tools: executor}
	state := turnLoopState{}
	collector := newStepToolCallCollector(false)
	batch := stepToolBatch{StepIndex: 1}
	progress := newStepToolProgress()
	result := assistantRoundtripResult{}
	stepHasBatchableResults := false
	stepHasToolCalls := false
	completionTokens := 0
	durableProgress := false
	stepStart := -1
	segment := ""
	handler := newStepStreamEventHandler(stepStreamEventHandlerInput{
		Runner:                  runner,
		Context:                 context.Background(),
		SessionID:               "session-1",
		TurnID:                  "turn-1",
		Model:                   provider.ModelRef{ProviderID: "test", ModelID: "model"},
		State:                   &state,
		Collector:               collector,
		Batch:                   &batch,
		Preview:                 newStepAssistantPreview(stepAssistantPreviewInput{Runner: runner, Context: context.Background(), SessionID: "session-1", TurnID: "turn-1", State: &state, Segment: &segment, HasToolCalls: &stepHasToolCalls}),
		StepConversationStart:   &stepStart,
		Executor:                newStepToolExecutor(executor, []string{tool.BashToolName}, nil, nil, nil),
		Progress:                &progress,
		Result:                  &result,
		StepHasBatchableResults: &stepHasBatchableResults,
		StepHasToolCalls:        &stepHasToolCalls,
		CompletionTokens:        &completionTokens,
		DurableProgress:         &durableProgress,
	})

	events := []provider.Event{
		{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: tool.BashToolName, InputDelta: `{"cmd":"grep -rn \"TODO\" internal | head -20"}`},
		{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: tool.BashToolName},
		{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: tool.BashToolName, InputDelta: `{"cmd":"cat go.mod"}`},
		{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: tool.BashToolName},
	}
	for _, event := range events {
		eventResult, err := handler.Handle(event)
		if err != nil {
			t.Fatalf("Handle(%s) error = %v", event.Kind, err)
		}
		if eventResult.Complete {
			t.Fatalf("Handle(%s) completed early: %#v", event.Kind, eventResult)
		}
	}
	if batch.Len() != 2 || !stepHasBatchableResults || progress.ExecutedTools != 0 {
		t.Fatalf("batch=%#v stepHasBatchableResults=%v progress=%#v", batch, stepHasBatchableResults, progress)
	}

	eventResult, err := handler.Handle(provider.Event{Kind: provider.EventKindAssistantDelta, AssistantDelta: "barrier"})
	if err != nil {
		t.Fatalf("Handle(assistant delta) error = %v", err)
	}
	if eventResult.Complete {
		t.Fatalf("eventResult = %#v", eventResult)
	}

	eventResult, err = handler.executePendingToolBatchAndComplete()
	if err != nil {
		t.Fatalf("executePendingToolBatchAndComplete() error = %v", err)
	}
	if !eventResult.Complete || eventResult.Result.Outcome != assistantRoundtripOutcomeToolResult {
		t.Fatalf("eventResult = %#v", eventResult)
	}
	if progress.ExecutedTools != 2 {
		t.Fatalf("progress = %#v, want 2 executed tools", progress)
	}
}
