package lsp

type ServerConfig struct {
	Name        string
	Command     string
	Args        []string
	Env         map[string]string
	Extensions  []string
	InitOptions map[string]any
	Enabled     *bool
}

func (c ServerConfig) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}
