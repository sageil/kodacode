package provider

const modelsDevURL = "https://models.dev/api.json"
const copilotModelsURL = "https://api.githubcopilot.com/models"

// cacheVersion is bumped whenever the on-disk cache schema changes
// (e.g. new fields added to modelsDevModel). A version mismatch forces
// a re-fetch from models.dev regardless of the refresh interval.
const cacheVersion = 5

// cacheEnvelope wraps the provider map with a version marker on disk.
type cacheEnvelope struct {
	Version   int                          `json:"version"`
	Providers map[string]modelsDevProvider `json:"providers"`
}

// modelsDevProvider is the JSON shape of a provider entry in the registry.
type modelsDevProvider struct {
	ID     string                    `json:"id"`
	Name   string                    `json:"name"`
	Models map[string]modelsDevModel `json:"models"`
}

// modelsDevModel is the JSON shape of a model entry in the registry.
type modelsDevModel struct {
	ID                 string             `json:"id"`
	Name               string             `json:"name"`
	Family             string             `json:"family"`
	Limit              modelsDevLimit     `json:"limit"`
	Cost               modelsDevCost      `json:"cost"`
	ToolCall           bool               `json:"tool_call"`
	ToolCallKnown      bool               `json:"tool_call_known,omitempty"`
	Reasoning          bool               `json:"reasoning"`
	Attachment         bool               `json:"attachment"`
	AttachmentKnown    bool               `json:"attachment_known,omitempty"`
	VisionKnown        bool               `json:"vision_known,omitempty"`
	Modalities         *modelsDevModality `json:"modalities,omitempty"`
	SupportedEndpoints []string           `json:"supported_endpoints,omitempty"`
}

type modelsDevModality struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

type modelsDevLimit struct {
	Context int `json:"context"`
	Input   int `json:"input"`
	Output  int `json:"output"`
}

type modelsDevCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
}

// LocalProviderEndpoint describes a local provider whose models are
// discovered via its /v1/models API endpoint (e.g. Ollama, LMStudio).
type LocalProviderEndpoint struct {
	ID      string
	Name    string
	BaseURL string
}
