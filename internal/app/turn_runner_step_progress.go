package app

import "strings"

type stepToolProgress struct {
	ExecutedTools     int
	FailedTools       int
	TaskWorkflowError string
	ReusedTools       int
	InteractionSigs   []string
}

func newStepToolProgress() stepToolProgress {
	return stepToolProgress{
		InteractionSigs: make([]string, 0, 4),
	}
}

func (p *stepToolProgress) Record(tools *ToolExecutor, call stepToolCall, arguments string, result stepToolResult) {
	if p == nil {
		return
	}
	switch result.Status {
	case ToolExecutionStatusReused:
		p.ReusedTools++
		return
	default:
		p.ExecutedTools++
		if strings.TrimSpace(result.Error) != "" {
			p.FailedTools++
			if strings.TrimSpace(call.ToolName) == "task_workflow" {
				p.TaskWorkflowError = strings.TrimSpace(result.Error)
			}
		}
	}
	if sig := providerStepToolInteractionSignature(tools, call.ToolName, arguments, result); sig != "" {
		p.InteractionSigs = append(p.InteractionSigs, sig)
	}
}

func (p stepToolProgress) Result(outcome assistantRoundtripOutcome, batchSize int, pendingRequestID string) assistantRoundtripResult {
	return assistantRoundtripResult{
		Outcome:             outcome,
		ToolBatchSize:       batchSize,
		ExecutedTools:       p.ExecutedTools,
		FailedTools:         p.FailedTools,
		TaskWorkflowError:   p.TaskWorkflowError,
		ReusedTools:         p.ReusedTools,
		ToolInteractionSigs: append([]string(nil), p.InteractionSigs...),
		PendingRequestID:    pendingRequestID,
	}
}
