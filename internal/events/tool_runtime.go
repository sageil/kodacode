package events

type ToolExecRuntimeState struct {
	Backend string
}

func cloneToolExecRuntimeState(state *ToolExecRuntimeState) *ToolExecRuntimeState {
	if state == nil {
		return nil
	}
	copyState := *state
	return &copyState
}
