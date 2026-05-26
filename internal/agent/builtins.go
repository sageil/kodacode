package agent

func NewBuiltinsCatalog() (*Catalog, error) {
	builder, err := parseMarkdownDefinition(DefaultID, []byte(builderPrompt))
	if err != nil {
		return nil, err
	}
	engineer, err := parseMarkdownDefinition("engineer", []byte(engineerPrompt))
	if err != nil {
		return nil, err
	}
	reviewer, err := parseMarkdownDefinition("reviewer", []byte(reviewerPrompt))
	if err != nil {
		return nil, err
	}
	planner, err := parseMarkdownDefinition("planner", []byte(plannerPrompt))
	if err != nil {
		return nil, err
	}

	return newCatalog(map[string]Definition{
		builder.ID:  builder,
		engineer.ID: engineer,
		reviewer.ID: reviewer,
		planner.ID:  planner,
	})
}
