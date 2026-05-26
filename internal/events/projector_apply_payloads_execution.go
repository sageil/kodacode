package events

func (p *Projector) applyExecutionPayload(event Event) (bool, error) {
	switch payload := event.Payload.(type) {
	case ExecutionDeclaredPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		clearTurnPassTransientState(turn, event.Sequence)
		call := p.ensureCall(event.TurnID, payload.ToolCallID, payload.ToolName)
		call.ToolName = payload.ToolName
		call.Execution = &ExecutionState{
			ExecutionID:      payload.ExecutionID,
			ToolCallID:       payload.ToolCallID,
			ToolName:         payload.ToolName,
			Kind:             payload.Kind,
			Intent:           payload.Intent,
			Effect:           payload.Effect,
			Command:          append([]string(nil), payload.Command...),
			CommandPreview:   payload.CommandPreview,
			WorkingDirectory: payload.WorkingDirectory,
			TimeoutMS:        payload.TimeoutMS,
			OutputLimit:      payload.OutputLimit,
			Status:           ExecutionStatusPendingApproval,
		}
		call.LastUpdatedSeq = event.Sequence
		return true, nil
	case ToolExecStartPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		clearTurnPassTransientState(turn, event.Sequence)
		call := p.ensureCall(event.TurnID, payload.CallID, payload.ToolName)
		call.ToolName = payload.ToolName
		call.Input = payload.Input
		call.StructuredResult = nil
		call.Executing = true
		call.LastUpdatedSeq = event.Sequence
		return true, nil
	case ExecutionStartedPayload:
		delete(p.state.ApprovedExecutions, payload.ExecutionID)
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		clearTurnPassTransientState(turn, event.Sequence)
		call := p.ensureCall(event.TurnID, payload.ToolCallID, payload.ToolName)
		call.ToolName = payload.ToolName
		call.Input = payload.Input
		call.Output = ""
		call.Error = ""
		call.OutputBlob = nil
		call.ErrorBlob = nil
		call.OutputTruncated = false
		call.ErrorTruncated = false
		call.Runtime = nil
		call.Completed = false
		if call.Execution != nil {
			call.Execution.ToolName = payload.ToolName
			call.Execution.Input = payload.Input
			call.Execution.Output = ""
			call.Execution.Error = ""
			call.Execution.OutputBlob = nil
			call.Execution.ErrorBlob = nil
			call.Execution.OutputTruncated = false
			call.Execution.ErrorTruncated = false
			call.Execution.Status = ExecutionStatusInProgress
			call.Execution.ExitCode = nil
			call.Execution.DurationMS = 0
			call.Execution.CommandActions = nil
			call.Execution.Runtime = nil
			call.Execution.Background = nil
			call.Execution.Executing = true
			call.Execution.Completed = false
		}
		call.Executing = true
		call.LastUpdatedSeq = event.Sequence
		return true, nil
	case ToolExecOutputPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		call := p.ensureCall(event.TurnID, payload.CallID, "")
		call.Output += payload.Chunk
		call.LastUpdatedSeq = event.Sequence
		return true, nil
	case ExecutionOutputPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		call := p.ensureCall(event.TurnID, payload.ToolCallID, "")
		backgroundHandled := false
		if call.Execution != nil && call.Execution.Background != nil {
			appendExecutionBackgroundOutput(call.Execution.Background, payload.Chunk)
			backgroundHandled = true
		}
		if !backgroundHandled {
			call.Output += payload.Chunk
		}
		if call.Execution != nil && !backgroundHandled {
			call.Execution.Output += payload.Chunk
		}
		call.LastUpdatedSeq = event.Sequence
		return true, nil
	case ExecutionBackgroundStartedPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		call := p.ensureCall(event.TurnID, payload.ToolCallID, payload.ToolName)
		call.ToolName = payload.ToolName
		if call.Execution != nil {
			background := ensureExecutionBackgroundState(call.Execution)
			background.PID = payload.PID
			background.ProcessIdentity = payload.ProcessIdentity
			background.SupervisorID = payload.SupervisorID
			background.Status = initialExecutionBackgroundStatus(call.Execution.Intent, payload.ReadyPatterns)
			background.LogRef = payload.LogRef
			background.ReadyPatterns = append([]string(nil), payload.ReadyPatterns...)
			background.Started = true
			background.StartedAtSeq = event.Sequence
		}
		call.LastUpdatedSeq = event.Sequence
		return true, nil
	case ExecutionBackgroundObservedPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		call := p.ensureCall(event.TurnID, payload.ToolCallID, payload.ToolName)
		call.ToolName = payload.ToolName
		if call.Execution != nil {
			background := ensureExecutionBackgroundState(call.Execution)
			background.OutputTail = payload.OutputTail
			background.OutputBytes = payload.OutputBytes
			if background.Status == "" {
				background.Status = initialExecutionBackgroundStatus(call.Execution.Intent, background.ReadyPatterns)
			}
			if background.Status == ExecutionBackgroundStatusStarting && len(background.ReadyPatterns) == 0 {
				background.Status = ExecutionBackgroundStatusRunning
			}
		}
		call.LastUpdatedSeq = event.Sequence
		return true, nil
	case ExecutionBackgroundReadyPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		call := p.ensureCall(event.TurnID, payload.ToolCallID, payload.ToolName)
		call.ToolName = payload.ToolName
		if call.Execution != nil {
			background := ensureExecutionBackgroundState(call.Execution)
			background.Status = ExecutionBackgroundStatusReady
			background.Ready = true
			background.ReadyAtSeq = event.Sequence
			background.ReadyMessage = payload.Message
			background.Port = payload.Port
		}
		call.LastUpdatedSeq = event.Sequence
		return true, nil
	case ExecutionBackgroundExitedPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		call := p.ensureCall(event.TurnID, payload.ToolCallID, payload.ToolName)
		call.ToolName = payload.ToolName
		if call.Execution != nil {
			background := ensureExecutionBackgroundState(call.Execution)
			background.Status = ExecutionBackgroundStatusExited
			background.Exited = true
			background.ExitedAtSeq = event.Sequence
			background.ExitCode = cloneExecutionExitCode(payload.ExitCode)
			background.Error = payload.Error
		}
		call.LastUpdatedSeq = event.Sequence
		return true, nil
	case ExecutionBackgroundLostPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		call := p.ensureCall(event.TurnID, payload.ToolCallID, payload.ToolName)
		call.ToolName = payload.ToolName
		if call.Execution != nil {
			background := ensureExecutionBackgroundState(call.Execution)
			background.Status = ExecutionBackgroundStatusSupervisionLost
			background.Error = payload.Error
		}
		call.LastUpdatedSeq = event.Sequence
		return true, nil
	case ToolExecEndPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		call := p.ensureCall(event.TurnID, payload.CallID, payload.ToolName)
		call.ToolName = payload.ToolName
		call.ReusedFromCallID = payload.ReusedFromCallID
		call.ReusedFromSessionID = payload.ReusedFromSessionID
		call.ReusedFromTurnID = payload.ReusedFromTurnID
		call.RetryOfCallID = payload.RetryOfCallID
		call.HandoffID = payload.HandoffID
		call.FailureClass = payload.FailureClass
		call.Succeeded = payload.Successful()
		call.Output = payload.Output
		call.Error = payload.Error
		call.ErrorDetail = cloneToolErrorDetail(payload.ErrorDetail)
		call.StructuredResult = cloneStructuredResult(payload.StructuredResult)
		call.MutationRanges = cloneMutationRanges(payload.MutationRanges)
		call.WriteMutation = cloneWriteMutation(payload.WriteMutation)
		call.WriteMutations = cloneWriteMutations(payload.WriteMutations)
		call.ObservedResources = cloneObservedResources(payload.ObservedResources)
		call.OutputBlob = cloneToolResultBlobRef(payload.OutputBlob)
		call.ErrorBlob = cloneToolResultBlobRef(payload.ErrorBlob)
		call.OutputTruncated = payload.OutputTruncated
		call.ErrorTruncated = payload.ErrorTruncated
		call.Runtime = toolExecRuntimeStateFromPayload(payload)
		if execution := ensureExecutionStateForCall(call, payload.ExecutionID); execution != nil {
			execution.ExecutionID = payload.ExecutionID
			execution.ToolCallID = call.CallID
			execution.ToolName = payload.ToolName
			execution.Status = ExecutionStatus(payload.ExecutionStatus)
			execution.Output = payload.Output
			execution.Error = payload.Error
			execution.OutputBlob = cloneToolResultBlobRef(payload.OutputBlob)
			execution.ErrorBlob = cloneToolResultBlobRef(payload.ErrorBlob)
			execution.OutputTruncated = payload.OutputTruncated
			execution.ErrorTruncated = payload.ErrorTruncated
			execution.ExitCode = cloneExecutionExitCode(payload.ExitCode)
			execution.DurationMS = payload.DurationMS
			execution.CommandActions = append([]string(nil), payload.CommandActions...)
			execution.Runtime = toolExecRuntimeStateFromPayload(payload)
			execution.Executing = false
			execution.Completed = true
		}
		call.Executing = false
		call.Completed = true
		call.LastUpdatedSeq = event.Sequence
		return true, nil
	default:
		return false, nil
	}
}
