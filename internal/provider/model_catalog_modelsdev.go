package provider

type catalogEnvelope struct {
	Version       int                            `json:"version"`
	Providers     map[string]catalogProvider     `json:"providers"`
	RemoteSources map[string]catalogRemoteSource `json:"remote_sources,omitempty"`
}

type catalogRemoteSource struct {
	Kind    RemoteModelCatalogProviderKind `json:"kind,omitempty"`
	BaseURL string                         `json:"base_url,omitempty"`
}

type catalogProvider struct {
	ID     string                  `json:"id"`
	Name   string                  `json:"name"`
	Models map[string]CatalogModel `json:"models"`
}

type modelsDevProvider struct {
	ID     string                    `json:"id"`
	Name   string                    `json:"name"`
	Models map[string]modelsDevModel `json:"models"`
}

type modelsDevModel struct {
	ID                 string             `json:"id"`
	Name               string             `json:"name"`
	Family             string             `json:"family"`
	Limit              modelsDevLimit     `json:"limit"`
	Cost               modelsDevCost      `json:"cost"`
	ToolCall           bool               `json:"tool_call"`
	Reasoning          bool               `json:"reasoning"`
	Attachment         bool               `json:"attachment"`
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
	Input      *float64 `json:"input"`
	Output     *float64 `json:"output"`
	CacheRead  *float64 `json:"cache_read"`
	CacheWrite *float64 `json:"cache_write"`
}
