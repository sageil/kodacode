package events

func (p *Projector) applyWorkflowPayload(event Event) (bool, error) {
	switch payload := event.Payload.(type) {
	case WorkflowRouteRecommendedPayload:
		return true, p.applyWorkflowRouteRecommended(event.Sequence, event.TurnID, payload)
	case WorkflowStartedPayload:
		return true, p.applyWorkflowStarted(event.Sequence, payload)
	case WorkflowPhaseStartedPayload:
		return true, p.applyWorkflowPhaseStarted(event.Sequence, payload)
	case WorkflowPhaseAdvancedPayload:
		return true, p.applyWorkflowPhaseAdvanced(event.Sequence, payload)
	case WorkflowPhaseBlockedPayload:
		return true, p.applyWorkflowPhaseBlocked(event.Sequence, payload)
	case WorkflowPhaseResumedPayload:
		return true, p.applyWorkflowPhaseResumed(event.Sequence, payload)
	case WorkflowEvidenceRecordedPayload:
		return true, p.applyWorkflowEvidenceRecorded(event.Sequence, payload)
	case WorkflowCompletedPayload:
		return true, p.applyWorkflowCompleted(event.Sequence, payload)
	default:
		return false, nil
	}
}
