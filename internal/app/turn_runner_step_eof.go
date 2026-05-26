package app

type providerRequestEOFInput struct {
	Progress               *stepToolProgress
	ToolBatchSize          int
	PendingRequestID       string
	Preview                stepAssistantPreview
	CommitToolStepBoundary func() error
}

func completeProviderRequestAfterStreamEOF(input providerRequestEOFInput) (assistantRoundtripResult, error) {
	if input.Progress != nil && (input.Progress.ExecutedTools > 0 || input.Progress.ReusedTools > 0) {
		if input.CommitToolStepBoundary != nil {
			if err := input.CommitToolStepBoundary(); err != nil {
				return assistantRoundtripResult{}, err
			}
		}
		return input.Progress.Result(assistantRoundtripOutcomeToolResult, input.ToolBatchSize, input.PendingRequestID), nil
	}
	if err := input.Preview.CommitAssistant(); err != nil {
		return assistantRoundtripResult{}, err
	}
	progress := input.Progress
	if progress == nil {
		defaultProgress := newStepToolProgress()
		progress = &defaultProgress
	}
	return progress.Result(assistantRoundtripOutcomeAssistantDone, input.ToolBatchSize, input.PendingRequestID), nil
}
