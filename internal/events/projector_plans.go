package events

import "strings"

func (p *Projector) ensurePlanStore() {
	if p.state.Plans == nil {
		p.state.Plans = make(map[string]*PlanState)
	}
}

func (p *Projector) applyPlanRecorded(payload PlanRecordedPayload) error {
	p.ensurePlanStore()
	plan := planStateFromPayload(payload)
	planID := strings.TrimSpace(plan.PlanID)
	if existing := p.state.Plans[planID]; existing == nil {
		p.state.PlanOrder = append(p.state.PlanOrder, planID)
	}
	p.state.Plans[planID] = plan
	return nil
}
