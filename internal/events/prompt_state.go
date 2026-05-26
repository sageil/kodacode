package events

type PromptState struct {
	Shape            string
	BaseInstructions string
	Instructions     string
	CacheablePrefix  string
	DynamicSuffix    string
	Fragments        []PromptFragmentState
}

type PromptFragmentState struct {
	Kind      string
	Source    string
	Stability string
	Key       string
	Label     string
	Bytes     int
	Tokens    int
}

func clonePromptState(state *PromptState) *PromptState {
	if state == nil {
		return nil
	}

	out := &PromptState{
		Shape:            state.Shape,
		BaseInstructions: state.BaseInstructions,
		Instructions:     state.Instructions,
		CacheablePrefix:  state.CacheablePrefix,
		DynamicSuffix:    state.DynamicSuffix,
		Fragments:        make([]PromptFragmentState, len(state.Fragments)),
	}
	copy(out.Fragments, state.Fragments)
	return out
}
