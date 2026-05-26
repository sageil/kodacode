package events

import "strings"

func (p *Projector) ensureReviewStore() {
	if p.state.Reviews == nil {
		p.state.Reviews = make(map[string]*ReviewState)
	}
}

func (p *Projector) applyReviewRecorded(payload ReviewRecordedPayload) {
	reviewID := strings.TrimSpace(payload.ReviewID)
	if reviewID == "" {
		reviewID = strings.TrimSpace(payload.SourceHandoffID)
	}
	if reviewID == "" {
		return
	}
	p.ensureReviewStore()
	if existing := p.state.Reviews[reviewID]; existing == nil {
		p.state.ReviewOrder = append(p.state.ReviewOrder, reviewID)
	}
	review := reviewStateFromPayload(payload)
	review.ReviewID = reviewID
	p.state.Reviews[reviewID] = review
}
