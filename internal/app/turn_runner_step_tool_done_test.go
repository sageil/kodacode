package app

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

func TestHandleStepToolCallDoneAdmitsAndContinuesDiscovery(t *testing.T) {
	runner := &TurnRunner{}
	state := turnLoopState{}
	collector := newStepToolCallCollector(false)
	batch := stepToolBatch{StepIndex: 1}
	hasToolCalls := false
	segment := ""
	preview := newStepAssistantPreview(stepAssistantPreviewInput{
		State:        &state,
		Segment:      &segment,
		HasToolCalls: &hasToolCalls,
	})

	result, err := runner.handleStepToolCallDone(context.Background(), stepToolDoneInput{
		Event:     provider.Event{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: tool.ReadToolName},
		Collector: collector,
		Batch:     &batch,
		Preview:   preview,
	})
	if err != nil {
		t.Fatalf("handleStepToolCallDone() error = %v", err)
	}
	if !result.Accepted || !result.ContinueCollecting {
		t.Fatalf("result = %#v", result)
	}
	wantTokens := provider.EstimateTextTokens(tool.ReadToolName) + provider.EstimateTextTokens(`{}`)
	if result.CompletionTokens != wantTokens {
		t.Fatalf("CompletionTokens = %d, want %d", result.CompletionTokens, wantTokens)
	}
	if got := batch.CallIDs(); len(got) != 1 || got[0] != "call-1" {
		t.Fatalf("batch call IDs = %v", got)
	}
	if !hasToolCalls {
		t.Fatalf("hasToolCalls=%v", hasToolCalls)
	}
	if len(state.Conversation) != 0 {
		t.Fatalf("conversation = %#v", state.Conversation)
	}
}

func TestHandleStepToolCallDoneRejectsDuplicateBeforeExecution(t *testing.T) {
	state := turnLoopState{}
	collector := newStepToolCallCollector(false)
	batch := stepToolBatch{
		StepIndex: 1,
		Calls: []stepToolCall{
			{CallID: "call-1", ToolName: tool.ReadToolName, Arguments: `{}`},
		},
	}
	hasToolCalls := false
	segment := ""
	preview := newStepAssistantPreview(stepAssistantPreviewInput{
		State:        &state,
		Segment:      &segment,
		HasToolCalls: &hasToolCalls,
	})
	runner := &TurnRunner{}

	result, err := runner.handleStepToolCallDone(context.Background(), stepToolDoneInput{
		Event:     provider.Event{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: tool.ReadToolName},
		Collector: collector,
		Batch:     &batch,
		Preview:   preview,
	})
	if err != nil {
		t.Fatalf("handleStepToolCallDone() error = %v", err)
	}
	if result.Accepted || result.ContinueCollecting {
		t.Fatalf("result = %#v", result)
	}
	wantTokens := provider.EstimateTextTokens(tool.ReadToolName) + provider.EstimateTextTokens(`{}`)
	if result.CompletionTokens != wantTokens {
		t.Fatalf("CompletionTokens = %d, want %d", result.CompletionTokens, wantTokens)
	}
	if len(state.Conversation) != 0 {
		t.Fatalf("conversation = %#v", state.Conversation)
	}
	if got := batch.CallIDs(); len(got) != 1 || got[0] != "call-1" {
		t.Fatalf("batch call IDs = %v", got)
	}
}
