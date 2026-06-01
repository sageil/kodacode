package events

type PromptState struct {
	Shape            string
	BaseInstructions string
	Instructions     string
	CacheablePrefix  string
	DynamicSuffix    string
	Layers           []PromptLayerState
	Fragments        []PromptFragmentState
}

type PromptFragmentState struct {
	Kind      string
	Source    string
	Stability string
	Layer     string
	Key       string
	Label     string
	Bytes     int
	Tokens    int
}

type PromptLayerState struct {
	Name      string
	Kind      string
	Source    string
	Stability string
	Status    string
	Fragments int
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
		Layers:           make([]PromptLayerState, len(state.Layers)),
		Fragments:        make([]PromptFragmentState, len(state.Fragments)),
	}
	copy(out.Layers, state.Layers)
	copy(out.Fragments, state.Fragments)
	return out
}
