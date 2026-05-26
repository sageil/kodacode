package events

func (p *Projector) applyPlanPayload(event Event) (bool, error) {
	switch payload := event.Payload.(type) {
	case PlanRecordedPayload:
		return true, p.applyPlanRecorded(payload)
	default:
		return false, nil
	}
}
