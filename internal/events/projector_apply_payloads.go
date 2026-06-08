package events

import "fmt"

func (p *Projector) applyPayload(event Event) error {
	if handled, err := p.applySessionPayload(event); handled {
		return err
	}
	if handled, err := p.applyTaskPayload(event); handled {
		return err
	}
	if handled, err := p.applyWorkflowPayload(event); handled {
		return err
	}
	if handled, err := p.applyPlanPayload(event); handled {
		return err
	}
	if handled, err := p.applyTurnContextPayload(event); handled {
		return err
	}
	if handled, err := p.applyProviderPayload(event); handled {
		return err
	}
	if handled, err := p.applyStreamPayload(event); handled {
		return err
	}
	if handled, err := p.applyExecutionPayload(event); handled {
		return err
	}
	if handled, err := p.applyInteractionPayload(event); handled {
		return err
	}
	if handled, err := p.applyTurnOutcomePayload(event); handled {
		return err
	}
	return fmt.Errorf("unsupported payload type %T", event.Payload)
}
