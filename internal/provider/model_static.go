package provider

func cloneModels(in []Model) []Model {
	out := make([]Model, len(in))
	copy(out, in)
	return out
}

func mergeConfiguredStaticModels(base, configured []Model) []Model {
	if len(base) == 0 {
		return cloneModels(configured)
	}
	merged := make(map[string]Model, len(base)+len(configured))
	order := make([]string, 0, len(base)+len(configured))
	for _, m := range base {
		merged[m.ID] = m
		order = append(order, m.ID)
	}
	for _, m := range configured {
		if existing, ok := merged[m.ID]; ok {
			merged[m.ID] = applyConfiguredModel(existing, m)
			continue
		}
		merged[m.ID] = m
		order = append(order, m.ID)
	}
	out := make([]Model, 0, len(order))
	seen := make(map[string]bool, len(order))
	for _, id := range order {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, merged[id])
	}
	return out
}

func applyConfiguredModel(base, configured Model) Model {
	out := base
	if configured.Name != "" {
		out.Name = configured.Name
	}
	if configured.ContextSize > 0 {
		out.ContextSize = configured.ContextSize
	}
	if configured.MaxInputTokens > 0 {
		out.MaxInputTokens = configured.MaxInputTokens
	}
	if configured.ThinkingBudget != nil {
		out.ThinkingBudget = configured.ThinkingBudget
	}
	return out
}
