package app

type StoredWorkflowConfig struct {
	ReviewMode  string            `yaml:"review_mode,omitempty"`
	ReviewModel StoredModelConfig `yaml:"review_model,omitempty"`
}
