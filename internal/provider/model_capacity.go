package provider

type ModelCapacity struct {
	InputTokens  int
	WindowTokens int
	OutputTokens int
}

func NormalizeModelCapacity(windowTokens, inputTokens, outputTokens int) ModelCapacity {
	if windowTokens < 0 {
		windowTokens = 0
	}
	if inputTokens < 0 {
		inputTokens = 0
	}
	if outputTokens < 0 {
		outputTokens = 0
	}
	if windowTokens == 0 {
		windowTokens = inputTokens
	}
	if inputTokens == 0 {
		inputTokens = windowTokens
	}
	if windowTokens > 0 && inputTokens > windowTokens {
		inputTokens = windowTokens
	}
	return ModelCapacity{
		InputTokens:  inputTokens,
		WindowTokens: windowTokens,
		OutputTokens: outputTokens,
	}
}

func (c ModelCapacity) HasDistinctWindow() bool {
	return c.InputTokens > 0 && c.WindowTokens > c.InputTokens
}

func (m CatalogModel) Capacity() ModelCapacity {
	return NormalizeModelCapacity(m.ContextSize, m.MaxInputTokens, m.MaxOutputTokens)
}
