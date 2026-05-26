package events

import "strings"

func (p *Projector) applyInteractionPayload(event Event) (bool, error) {
	switch payload := event.Payload.(type) {
	case ExecutionApprovalRequestedPayload:
		p.state.PendingExecutions[payload.RequestID] = &ExecutionApprovalState{
			RequestID:             payload.RequestID,
			ExecutionID:           payload.ExecutionID,
			TurnID:                event.TurnID,
			ToolCallID:            payload.ToolCallID,
			WorkingDirectory:      payload.WorkingDirectory,
			ToolName:              payload.ToolName,
			Command:               payload.Command,
			Reason:                payload.Reason,
			PrefixRule:            append([]string(nil), payload.PrefixRule...),
			SessionGrantPaths:     append([]string(nil), payload.SessionGrantPaths...),
			NetworkTargets:        append([]string(nil), payload.NetworkTargets...),
			AvailableDecisions:    append([]ExecutionApprovalDecision(nil), payload.AvailableDecisions...),
			ProposedExecPolicy:    cloneExecutionPolicyAmendment(payload.ProposedExecPolicy),
			ProposedNetworkPolicy: cloneExecutionNetworkPolicyAmendment(payload.ProposedNetworkPolicy),
			RequestedAtSeq:        event.Sequence,
		}
		call := p.ensureCall(event.TurnID, payload.ToolCallID, payload.ToolName)
		if call.Execution != nil {
			call.Execution.Status = ExecutionStatusPendingApproval
		}
		p.state.PendingExecutionOrder = append(p.state.PendingExecutionOrder, payload.RequestID)
		return true, nil
	case PermissionRequestedPayload:
		p.state.PendingPermissions[payload.RequestID] = &PermissionRequestState{
			Kind:             payload.Kind,
			RequestID:        payload.RequestID,
			ExecutionID:      payload.ExecutionID,
			TurnID:           event.TurnID,
			ToolCallID:       payload.ToolCallID,
			Access:           payload.Access,
			Path:             payload.Path,
			WorkingDirectory: payload.WorkingDirectory,
			ToolName:         payload.ToolName,
			Command:          payload.Command,
			Reason:           payload.Reason,
			RequestedAtSeq:   event.Sequence,
		}
		p.state.PendingPermissionOrder = append(p.state.PendingPermissionOrder, payload.RequestID)
		return true, nil
	case QuestionRequestedPayload:
		if strings.TrimSpace(payload.ToolCallID) != "" {
			delete(p.state.QuestionAnswers, questionAnswerStateKey(event.TurnID, payload.ToolCallID))
		}
		p.state.PendingQuestions[payload.QuestionID] = &QuestionRequestState{
			QuestionID:     payload.QuestionID,
			TurnID:         event.TurnID,
			ToolCallID:     payload.ToolCallID,
			ToolName:       payload.ToolName,
			PlanID:         strings.TrimSpace(payload.PlanID),
			Question:       payload.Question,
			Options:        append([]string(nil), payload.Options...),
			Multiple:       payload.Multiple,
			Purpose:        payload.Purpose,
			RequestedAtSeq: event.Sequence,
		}
		p.state.PendingQuestionOrder = append(p.state.PendingQuestionOrder, payload.QuestionID)
		if strings.TrimSpace(payload.ToolCallID) != "" {
			call := p.ensureCall(event.TurnID, payload.ToolCallID, payload.ToolName)
			call.ToolName = payload.ToolName
			call.Executing = false
			call.Completed = false
			call.LastUpdatedSeq = event.Sequence
		}
		return true, nil
	case ExecutionApprovalResolvedPayload:
		request := p.state.PendingExecutions[payload.RequestID]
		delete(p.state.PendingExecutions, payload.RequestID)
		p.state.PendingExecutionOrder = filterPendingRequestOrder(p.state.PendingExecutionOrder, payload.RequestID)
		if request != nil && executionApprovalDecisionAllowed(payload.Decision) {
			p.state.SessionGrantDecisions = append(p.state.SessionGrantDecisions, sessionGrantDecisionFromExecutionRequest(request, event.Sequence))
		}
		if request != nil {
			if executionApprovalDecisionAllowsResume(payload.Decision) {
				p.state.ApprovedExecutions[request.ExecutionID] = &ApprovedExecutionState{
					RequestID:            payload.RequestID,
					ExecutionID:          request.ExecutionID,
					TurnID:               request.TurnID,
					ToolCallID:           request.ToolCallID,
					ToolName:             request.ToolName,
					Command:              request.Command,
					WorkingDirectory:     request.WorkingDirectory,
					Decision:             payload.Decision,
					AppliedExecPolicy:    cloneExecutionPolicyAmendment(payload.AppliedExecPolicy),
					AppliedNetworkPolicy: cloneExecutionNetworkPolicyAmendment(payload.AppliedNetworkPolicy),
					ApprovedAtSeq:        event.Sequence,
				}
			} else {
				delete(p.state.ApprovedExecutions, request.ExecutionID)
			}
		}
		if payload.Decision == ExecutionApprovalDecisionAcceptForSession {
			if len(payload.GrantPrefixRule) > 0 {
				p.state.ExecutionGrants = append(p.state.ExecutionGrants, ExecutionGrantState{
					PrefixRule:     append([]string(nil), payload.GrantPrefixRule...),
					SessionPaths:   append([]string(nil), payload.GrantPaths...),
					NetworkTargets: append([]string(nil), payload.GrantNetworkTargets...),
				})
			}
			for _, path := range payload.GrantPaths {
				p.state.WorkspaceGrants = append(p.state.WorkspaceGrants, WorkspaceGrantState{
					Path:      path,
					Recursive: false,
				})
			}
			for _, target := range payload.GrantNetworkTargets {
				if strings.TrimSpace(target) == "" {
					continue
				}
				p.state.NetworkGrants = append(p.state.NetworkGrants, NetworkGrantState{Target: target})
			}
		}
		if request != nil {
			call := p.ensureCall(request.TurnID, request.ToolCallID, request.ToolName)
			if call.Execution != nil {
				switch payload.Decision {
				case ExecutionApprovalDecisionDecline, ExecutionApprovalDecisionCancel:
					call.Execution.Status = ExecutionStatusDeclined
					call.Execution.Executing = false
					call.Execution.Completed = true
				default:
					call.Execution.Status = ExecutionStatusInProgress
				}
			}
		}
		return true, nil
	case PermissionResolvedPayload:
		request := p.state.PendingPermissions[payload.RequestID]
		delete(p.state.PendingPermissions, payload.RequestID)
		p.state.PendingPermissionOrder = filterPendingRequestOrder(p.state.PendingPermissionOrder, payload.RequestID)
		if request != nil && payload.Decision == PermissionDecisionApproved {
			p.state.SessionGrantDecisions = append(p.state.SessionGrantDecisions, sessionGrantDecisionFromPermissionRequest(request, event.Sequence))
		}
		if payload.Decision == PermissionDecisionApproved && payload.Scope == PermissionScopeSession && request != nil {
			switch {
			case request.Kind == PermissionRequestKindNetwork:
				p.state.NetworkGrants = append(p.state.NetworkGrants, NetworkGrantState{Target: request.Path})
			case len(payload.GrantPaths) > 0:
				for _, path := range payload.GrantPaths {
					p.state.WorkspaceGrants = append(p.state.WorkspaceGrants, WorkspaceGrantState{
						Path:      path,
						Recursive: false,
					})
				}
			case payload.GrantPath != "":
				p.state.WorkspaceGrants = append(p.state.WorkspaceGrants, WorkspaceGrantState{
					Path:      payload.GrantPath,
					Recursive: payload.Recursive,
				})
			}
		}
		return true, nil
	case QuestionAnsweredPayload:
		request := p.state.PendingQuestions[payload.QuestionID]
		delete(p.state.PendingQuestions, payload.QuestionID)
		p.state.PendingQuestionOrder = filterPendingRequestOrder(p.state.PendingQuestionOrder, payload.QuestionID)
		if request != nil && strings.TrimSpace(request.ToolCallID) != "" {
			call := p.ensureCall(request.TurnID, request.ToolCallID, request.ToolName)
			call.ToolName = request.ToolName
			call.Executing = false
			call.Completed = false
			call.LastUpdatedSeq = event.Sequence
			p.state.QuestionAnswers[questionAnswerStateKey(request.TurnID, request.ToolCallID)] = &QuestionAnswerState{
				QuestionID:    payload.QuestionID,
				TurnID:        request.TurnID,
				ToolCallID:    request.ToolCallID,
				ToolName:      request.ToolName,
				PlanID:        request.PlanID,
				Question:      request.Question,
				Purpose:       request.Purpose,
				Answer:        payload.Answer,
				AnsweredAtSeq: event.Sequence,
			}
		}
		return true, nil
	default:
		return false, nil
	}
}
