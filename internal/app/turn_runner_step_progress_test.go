package app

import (
	"testing"

	"github.com/sageil/kodacode/internal/tool"
)

func TestStepToolProgressRecordsExecutedToolInteractionsAndFailure(t *testing.T) {
	progress := newStepToolProgress()
	progress.Record(nil, stepToolCall{
		CallID:    "call-read",
		ToolName:  tool.ReadToolName,
		Arguments: `{"paths":["app.go"]}`,
	}, `{"paths":["app.go"]}`, stepToolResult{
		CallID:   "call-read",
		Status:   ToolExecutionStatusExecuted,
		Output:   "contents",
		ToolName: tool.ReadToolName,
	})
	progress.Record(nil, stepToolCall{
		CallID:    "call-task",
		ToolName:  tool.TaskWorkflowToolName,
		Arguments: `{"action":"create","title":"Do work"}`,
	}, `{"action":"create","title":"Do work"}`, stepToolResult{
		CallID:       "call-task",
		ToolName:     tool.TaskWorkflowToolName,
		Status:       ToolExecutionStatusExecuted,
		Error:        "failed",
		FailureClass: "validation",
	})

	result := progress.Result(assistantRoundtripOutcomeToolResult, 2, "")
	if result.ExecutedTools != 2 || result.FailedTools != 1 {
		t.Fatalf("counts = executed %d failed %d", result.ExecutedTools, result.FailedTools)
	}
	if result.TaskWorkflowError != "failed" {
		t.Fatalf("TaskWorkflowError = %q", result.TaskWorkflowError)
	}
	if len(result.ToolInteractionSigs) != 2 {
		t.Fatalf("tool interaction signatures = %#v", result.ToolInteractionSigs)
	}
}

func TestStepToolProgressDoesNotRecordReusedToolInteraction(t *testing.T) {
	progress := newStepToolProgress()
	progress.Record(nil, stepToolCall{
		CallID:    "call-read",
		ToolName:  tool.ReadToolName,
		Arguments: `{"paths":["app.go"]}`,
	}, `{"paths":["app.go"]}`, stepToolResult{
		CallID:   "call-read",
		ToolName: tool.ReadToolName,
		Status:   ToolExecutionStatusReused,
	})

	result := progress.Result(assistantRoundtripOutcomeToolResult, 1, "")
	if result.ReusedTools != 1 || result.ExecutedTools != 0 {
		t.Fatalf("counts = reused %d executed %d", result.ReusedTools, result.ExecutedTools)
	}
	if len(result.ToolInteractionSigs) != 0 {
		t.Fatalf("tool interaction signatures = %#v", result.ToolInteractionSigs)
	}
}
