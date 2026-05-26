package events

func (p *Projector) applyTaskPayload(event Event) (bool, error) {
	switch payload := event.Payload.(type) {
	case TaskCreatedPayload:
		return true, p.applyTaskCreated(event.Sequence, payload)
	case TaskProgressUpdatedPayload:
		return true, p.applyTaskProgressUpdated(event.Sequence, payload)
	case TaskBlockedPayload:
		return true, p.applyTaskBlocked(event.Sequence, payload)
	case TaskCompletedPayload:
		return true, p.applyTaskCompleted(event.Sequence, payload)
	case TaskReviewedPayload:
		return true, p.applyTaskReviewed(event.Sequence, payload)
	default:
		return false, nil
	}
}
