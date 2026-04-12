package tui

// ConfigState groups model, agent, and variant selection state that was
// previously scattered across App fields. It does not encapsulate behavior —
// App reads fields directly — but consolidates related state into one place.
type ConfigState struct {
	// Model selection.
	Model     string              // active model ID, e.g. "anthropic/claude-opus"
	ModelInfo string              // cached display string for header
	Models    map[string]ModelItem // full model metadata lookup

	// Agent selection.
	Agent           string            // active agent ID
	AgentName       string            // display name of active agent
	AgentNames      map[string]string // ID → display name
	AgentIDs        []string          // all agent IDs (sorted, includes subagents)
	PrimaryAgentIDs []string          // primary agent IDs only (no subagents)

	// Reasoning / variant.
	Variant         string   // reasoning effort level ("adaptive"/"low"/"high"/"max")
	VariantNames    []string // available variant names
	HasReasoning    bool     // whether current model supports reasoning

	// Planner workflow.
	PreplanAgent    string // agent ID to restore after planner finishes
	PlannerPending  bool   // waiting for planner approval answer
	PlannerChoice   string // user's chosen approval action
}

// NewConfigState returns a ConfigState with sensible defaults.
func NewConfigState() ConfigState {
	return ConfigState{
		VariantNames: []string{"adaptive", "low", "high", "max"},
		Variant:      "adaptive",
	}
}
