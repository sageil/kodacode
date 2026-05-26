package events

import "errors"

type SessionStateSnapshotPayload struct {
	BaseSequence int64
	State        SessionState
}

func (SessionStateSnapshotPayload) eventType() Type { return TypeSessionStateSnapshot }

func (p SessionStateSnapshotPayload) validate() error {
	if p.BaseSequence < -1 {
		return errors.New("base_sequence must be >= -1")
	}
	if p.State.SessionID == "" {
		return errors.New("state.session_id is required")
	}
	if p.State.LastSequence != p.BaseSequence {
		return errors.New("state.last_sequence must equal base_sequence")
	}
	return nil
}
