package events

type PruningState struct {
	PriorTurns          int
	PriorInputBytes     int
	RawPriorTurns       int
	RawInputBytes       int
	CompactedPriorTurns int
	CompactedInputBytes int
	OmittedPriorTurns   int
	OmittedInputBytes   int
}

func clonePruningState(state *PruningState) *PruningState {
	if state == nil {
		return nil
	}
	copyState := *state
	return &copyState
}
