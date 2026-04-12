package config

import "github.com/sageil/kodacode/v1/internal/permission"

func boolPtr(v bool) *bool          { return &v }
func float64Ptr(v float64) *float64 { return &v }
func intPtr(v int) *int             { return &v }

// Bool returns the value of a *bool field, defaulting to false if nil.
func Bool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

const CurrentConfigVersion = 1

type Config struct {
	Version              int                `yaml:"version"`
	Providers            []ProviderConfig   `yaml:"providers"`
	UtilityModel         string             `yaml:"utility_model"`
	ReviewerModel        string             `yaml:"reviewer_model"`
	DefaultAgent         string             `yaml:"default_agent"`
	FallbackModels       []string           `yaml:"fallback_models"`
	Session              SessionConfig      `yaml:"session"`
	Server               ServerConfig       `yaml:"server"`
	TUI                  TUIConfig          `yaml:"tui"`
	Permission           permission.Config  `yaml:"permission"`
	Debug                *bool              `yaml:"debug"`
	LogPrompts           *bool              `yaml:"log_prompts"`
	InstructionFiles     []string           `yaml:"instruction_files"`
	MaxInstructionSize   int64              `yaml:"max_instruction_size"`
	MCP                  MCPConfig          `yaml:"mcp"`
	LSP                  LSPConfig          `yaml:"lsp"`
	Diagnostics          DiagnosticsConfig  `yaml:"diagnostics"`
	SearchIndex          SearchIndexConfig  `yaml:"search_index"`
	Skills               GlobalSkillsConfig `yaml:"skills"`
	MemoryBudget         int                `yaml:"memory_budget"`
	ModelRefreshInterval int                `yaml:"model_refresh_interval"`
	AllowedPaths         []string           `yaml:"allowed_paths"`
	IgnorePatterns       []string           `yaml:"ignore_patterns"`
}

type DiagnosticsConfig struct {
	Enabled *bool          `yaml:"enabled"`
	Linters []LinterConfig `yaml:"linters"`
}

type SearchIndexConfig struct {
	Enabled         *bool           `yaml:"enabled"`
	CtagsBinary     string          `yaml:"ctags_binary"`
	ExcludePatterns []string        `yaml:"exclude_patterns"`
	MaxFileSize     int64           `yaml:"max_file_size"`
	Embedding       EmbeddingConfig `yaml:"embedding"`
}

func (c SearchIndexConfig) IsEnabled() bool {
	return c.Enabled != nil && *c.Enabled
}

type EmbeddingConfig struct {
	Enabled    *bool  `yaml:"enabled"`
	Model      string `yaml:"model"`      // "provider/model" format (e.g. "ollama/nomic-embed-text")
	Dimensions int    `yaml:"dimensions"` // expected vector dimensions; 0 = model default
	BatchSize  int    `yaml:"batch_size"` // symbols per API call; 0 = default (100)
}

func (e EmbeddingConfig) IsEnabled() bool {
	return e.Enabled != nil && *e.Enabled && e.Model != ""
}

func (e EmbeddingConfig) EffectiveBatchSize() int {
	if e.BatchSize > 0 {
		return e.BatchSize
	}
	return 100
}

func (d DiagnosticsConfig) IsEnabled() bool {
	return d.Enabled == nil || *d.Enabled
}

type LinterConfig struct {
	Command    string   `yaml:"command"`
	Extensions []string `yaml:"extensions"`
	DirMode    bool     `yaml:"dir_mode"`
}

type MCPConfig struct {
	Servers []MCPServerConfig `yaml:"servers"`
}

type MCPToolHintConfig struct {
	Summary               string   `yaml:"summary"`
	Guidance              string   `yaml:"guidance"`
	Triggers              []string `yaml:"triggers"`
	FileExts              []string `yaml:"file_exts"`
	PreserveParameterDocs *bool    `yaml:"preserve_parameter_docs"`
}

type MCPServerConfig struct {
	Name      string                       `yaml:"name"`
	Type      string                       `yaml:"type"`
	Command   string                       `yaml:"command"`
	Args      []string                     `yaml:"args"`
	URL       string                       `yaml:"url"`
	Headers   map[string]string            `yaml:"headers"`
	Env       map[string]string            `yaml:"env"`
	ToolHints map[string]MCPToolHintConfig `yaml:"tool_hints"`
	Enabled   *bool                        `yaml:"enabled"`
}

func (c MCPServerConfig) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

type LSPConfig struct {
	Servers []LSPServerConfig `yaml:"servers"`
}

type LSPServerConfig struct {
	Name        string            `yaml:"name"`
	Command     string            `yaml:"command"`
	Args        []string          `yaml:"args"`
	Env         map[string]string `yaml:"env"`
	Extensions  []string          `yaml:"extensions"`
	Enabled     *bool             `yaml:"enabled"`
	InitOptions map[string]any    `yaml:"init_options"`
}

func (c LSPServerConfig) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

type GlobalSkillsConfig struct {
	Models map[string]ModelSkillsConfig `yaml:"models"`
}

type ModelSkillsConfig struct {
	AllowAll bool     `yaml:"allow_all"`
	Deny     []string `yaml:"deny"`
}

// SessionModelConfig returns the per-session model override for the given
// provider and model IDs. Fully qualified provider/model keys are canonical;
// bare model IDs are supported as a compatibility fallback.
func (c *Config) SessionModelConfig(providerID, modelID string) (ModelSessionConfig, bool) {
	if c == nil {
		return ModelSessionConfig{}, false
	}
	return c.Session.ModelConfig(providerID, modelID)
}

// ModelThinkingBudget returns the per-model thinking budget for the given
// provider and model IDs. Returns nil if no budget is configured.
func (c *Config) ModelThinkingBudget(providerID, modelID string) *int {
	for _, pc := range c.Providers {
		if pc.ID != providerID {
			continue
		}
		for _, m := range pc.Models {
			if m.ID == modelID {
				return m.ThinkingBudget
			}
		}
	}
	return nil
}

// AgentConfig lives in config (not service) to avoid import cycles with pipeline.
type AgentConfig struct {
	Tools        []string
	DenyTools    []string
	Permission   permission.Config
	SystemPrompt string
	Model        string
	MaxTokens    int
	Skills       SkillsConfig
	Reasoning    ReasoningConfig
}

type SkillsConfig struct {
	Allow []string `yaml:"allow"`
	Deny  []string `yaml:"deny"`
}

type ReasoningConfig struct {
	Budget *int   `yaml:"budget"`
	Effort string `yaml:"effort"`
}

type ProviderConfig struct {
	ID             string                `yaml:"id"`
	APIKey         string                `yaml:"api_key"`
	BaseURL        string                `yaml:"base_url"`
	Models         []ProviderModelConfig `yaml:"models"`
	ThinkingBudget int                   `yaml:"thinking_budget"`
	ThinkingType   string                `yaml:"thinking_type"` // "adaptive" (default) or "enabled"
}

type ProviderModelConfig struct {
	ID             string `yaml:"id"`
	Name           string `yaml:"name"`
	ContextSize    int    `yaml:"context_size"`
	ThinkingBudget *int   `yaml:"thinking_budget"` // per-model reasoning token budget; nil = no thinking
}

type SessionConfig struct {
	CompactionThreshold     *float64                      `yaml:"compaction_threshold"`
	CompactionKeepTurns     *int                          `yaml:"compaction_keep_turns"`
	PruneProtectTokens      *int                          `yaml:"prune_protect_tokens"`
	PruneMinSavings         *int                          `yaml:"prune_min_savings"`
	ContextLimit            *float64                      `yaml:"context_limit"`
	MaxRetries              int                           `yaml:"max_retries"`
	ToolCallArgumentTimeout int                           `yaml:"tool_call_argument_timeout"`
	EngineerReviewLimit     int                           `yaml:"engineer_review_limit"`
	MaxSubagents            int                           `yaml:"max_subagents"`
	SubagentTimeout         int                           `yaml:"subagent_timeout"`
	PlanApproval            *bool                         `yaml:"plan_approval"`
	BackgroundAutoReact     *bool                         `yaml:"background_auto_react"`
	Snapshot                *bool                         `yaml:"snapshot"`
	Trace                   *bool                         `yaml:"trace"`
	Budget                  float64                       `yaml:"budget"`
	BudgetWarn              float64                       `yaml:"budget_warn"`
	TotalBudget             float64                       `yaml:"total_budget"`
	TotalBudgetWarn         float64                       `yaml:"total_budget_warn"`
	PrimaryMaxSteps         int                           `yaml:"primary_max_steps"`
	SubagentMaxSteps        int                           `yaml:"subagent_max_steps"`
	Models                  map[string]ModelSessionConfig `yaml:"models"`
}

// ModelConfig returns the per-model session override for the given provider and
// model IDs. Fully qualified provider/model keys are canonical; bare model IDs
// remain supported as a compatibility fallback.
func (c *SessionConfig) ModelConfig(providerID, modelID string) (ModelSessionConfig, bool) {
	if c == nil {
		return ModelSessionConfig{}, false
	}
	if providerID != "" && modelID != "" {
		if mc, ok := c.Models[providerID+"/"+modelID]; ok {
			return mc, true
		}
	}
	if modelID != "" {
		if mc, ok := c.Models[modelID]; ok {
			return mc, true
		}
	}
	return ModelSessionConfig{}, false
}

type ModelSessionConfig struct {
	CompactionThreshold     *float64 `yaml:"compaction_threshold"`
	CompactionKeepTurns     *int     `yaml:"compaction_keep_turns"`
	PruneProtectTokens      *int     `yaml:"prune_protect_tokens"`
	PruneMinSavings         *int     `yaml:"prune_min_savings"`
	ContextLimit            *float64 `yaml:"context_limit"`
	MaxInputTokens          int      `yaml:"max_input_tokens"`
	ToolCallArgumentTimeout int      `yaml:"tool_call_argument_timeout"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type TUIConfig struct {
	InputMaxHeight    int    `yaml:"input_max_height"`
	Theme             string `yaml:"theme"`
	DisplayTurns      int    `yaml:"display_turns"`
	ErrorDisplayTime  int    `yaml:"error_display_time"`
	AutoResume        bool   `yaml:"auto_resume"`
	MaxAttachmentSize int64  `yaml:"max_attachment_size"`
	SSEReadTimeout    int    `yaml:"sse_read_timeout"` // minutes; 0 = default (2 min)
}
