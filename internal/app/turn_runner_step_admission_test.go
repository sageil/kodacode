package app

import (
	"testing"

	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

func TestAdmitStepToolCallAcceptsFirstCallAndAccountsTokens(t *testing.T) {
	batch := stepToolBatch{StepIndex: 1}
	call := stepToolCall{CallID: "call-1", ToolName: tool.ReadToolName, Arguments: `{"paths":["app.go"]}`}

	admission := admitStepToolCall(&batch, call)
	if !admission.Accepted {
		t.Fatal("admission.Accepted = false")
	}
	wantTokens := provider.EstimateTextTokens(call.ToolName) + provider.EstimateTextTokens(call.Arguments)
	if admission.CompletionTokens != wantTokens {
		t.Fatalf("CompletionTokens = %d, want %d", admission.CompletionTokens, wantTokens)
	}
	if got := batch.CallIDs(); len(got) != 1 || got[0] != "call-1" {
		t.Fatalf("batch call IDs = %v", got)
	}
}

func TestAdmitStepToolCallAcceptsSequentialCallAfterDiscovery(t *testing.T) {
	readCall := stepToolCall{CallID: "call-read", ToolName: tool.ReadToolName, Arguments: `{"paths":["app.go"]}`}
	patchCall := stepToolCall{CallID: "call-patch", ToolName: tool.ApplyPatchToolName, Arguments: ``}
	batch := stepToolBatch{StepIndex: 1, Calls: []stepToolCall{readCall}}

	admission := admitStepToolCall(&batch, patchCall)
	if !admission.Accepted {
		t.Fatal("admission.Accepted = false")
	}
	wantTokens := provider.EstimateTextTokens(patchCall.ToolName) + provider.EstimateTextTokens(patchCall.Arguments)
	if admission.CompletionTokens != wantTokens {
		t.Fatalf("CompletionTokens = %d, want %d", admission.CompletionTokens, wantTokens)
	}
	if got := batch.CallIDs(); len(got) != 2 || got[0] != "call-read" || got[1] != "call-patch" {
		t.Fatalf("batch call IDs = %v", got)
	}
}

func TestAdmitStepToolCallRejectsDuplicateAfterAccountingTokens(t *testing.T) {
	call := stepToolCall{CallID: "call-1", ToolName: tool.ReadToolName, Arguments: `{"paths":["app.go"]}`}
	batch := stepToolBatch{StepIndex: 1, Calls: []stepToolCall{call}}

	admission := admitStepToolCall(&batch, call)
	if admission.Accepted {
		t.Fatal("admission.Accepted = true")
	}
	wantTokens := provider.EstimateTextTokens(call.ToolName) + provider.EstimateTextTokens(call.Arguments)
	if admission.CompletionTokens != wantTokens {
		t.Fatalf("CompletionTokens = %d, want %d", admission.CompletionTokens, wantTokens)
	}
	if got := batch.CallIDs(); len(got) != 1 || got[0] != "call-1" {
		t.Fatalf("batch call IDs = %v", got)
	}
}
