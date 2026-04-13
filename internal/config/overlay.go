package config

import "github.com/sageil/kodacode/v1/internal/permission"

type configOverlay struct {
	Version              *int                      `yaml:"version"`
	Providers            []ProviderConfig          `yaml:"providers"`
	UtilityModel         *string                   `yaml:"utility_model"`
	ReviewerModel        *string                   `yaml:"reviewer_model"`
	DefaultAgent         *string                   `yaml:"default_agent"`
	FallbackModels       []string                  `yaml:"fallback_models"`
	Session              sessionConfigOverlay      `yaml:"session"`
	Server               serverConfigOverlay       `yaml:"server"`
	TUI                  tuiConfigOverlay          `yaml:"tui"`
	Permission           permission.Config         `yaml:"permission"`
	Debug                *bool                     `yaml:"debug"`
	LogPrompts           *bool                     `yaml:"log_prompts"`
	InstructionFiles     []string                  `yaml:"instruction_files"`
	MaxInstructionSize   *int64                    `yaml:"max_instruction_size"`
	MCP                  MCPConfig                 `yaml:"mcp"`
	LSP                  lspConfigOverlay          `yaml:"lsp"`
	Diagnostics          DiagnosticsConfig         `yaml:"diagnostics"`
	SearchIndex          SearchIndexConfig         `yaml:"search_index"`
	Skills               globalSkillsConfigOverlay `yaml:"skills"`
	MemoryBudget         *int                      `yaml:"memory_budget"`
	ModelRefreshInterval *int                      `yaml:"model_refresh_interval"`
	AllowedPaths         []string                  `yaml:"allowed_paths"`
	IgnorePatterns       []string                  `yaml:"ignore_patterns"`
}

type sessionConfigOverlay struct {
	CompactionThreshold         *float64                             `yaml:"compaction_threshold"`
	CompactionKeepTurns         *int                                 `yaml:"compaction_keep_turns"`
	PruneProtectTokens          *int                                 `yaml:"prune_protect_tokens"`
	PruneMinSavings             *int                                 `yaml:"prune_min_savings"`
	ContextLimit                *float64                             `yaml:"context_limit"`
	MaxRetries                  *int                                 `yaml:"max_retries"`
	ToolCallArgumentTimeout     *int                                 `yaml:"tool_call_argument_timeout"`
	EngineerReviewLimit         *int                                 `yaml:"engineer_review_limit"`
	EngineerExecutionRetryLimit *int                                 `yaml:"engineer_execution_retry_limit"`
	MaxSubagents                *int                                 `yaml:"max_subagents"`
	SubagentTimeout             *int                                 `yaml:"subagent_timeout"`
	PlanApproval                *bool                                `yaml:"plan_approval"`
	BackgroundAutoReact         *bool                                `yaml:"background_auto_react"`
	Snapshot                    *bool                                `yaml:"snapshot"`
	Trace                       *bool                                `yaml:"trace"`
	Budget                      *float64                             `yaml:"budget"`
	BudgetWarn                  *float64                             `yaml:"budget_warn"`
	TotalBudget                 *float64                             `yaml:"total_budget"`
	TotalBudgetWarn             *float64                             `yaml:"total_budget_warn"`
	PrimaryMaxSteps             *int                                 `yaml:"primary_max_steps"`
	SubagentMaxSteps            *int                                 `yaml:"subagent_max_steps"`
	Models                      map[string]modelSessionConfigOverlay `yaml:"models"`
}

type modelSessionConfigOverlay struct {
	CompactionThreshold     *float64 `yaml:"compaction_threshold"`
	CompactionKeepTurns     *int     `yaml:"compaction_keep_turns"`
	PruneProtectTokens      *int     `yaml:"prune_protect_tokens"`
	PruneMinSavings         *int     `yaml:"prune_min_savings"`
	ContextLimit            *float64 `yaml:"context_limit"`
	MaxInputTokens          *int     `yaml:"max_input_tokens"`
	ToolCallArgumentTimeout *int     `yaml:"tool_call_argument_timeout"`
}

type serverConfigOverlay struct {
	Port *int `yaml:"port"`
}

type tuiConfigOverlay struct {
	InputMaxHeight    *int    `yaml:"input_max_height"`
	Theme             *string `yaml:"theme"`
	DisplayTurns      *int    `yaml:"display_turns"`
	ErrorDisplayTime  *int    `yaml:"error_display_time"`
	AutoResume        *bool   `yaml:"auto_resume"`
	MaxAttachmentSize *int64  `yaml:"max_attachment_size"`
	SSEReadTimeout    *int    `yaml:"sse_read_timeout"`
}

type lspConfigOverlay struct {
	Servers []lspServerConfigOverlay `yaml:"servers"`
}

type lspServerConfigOverlay struct {
	Name        *string           `yaml:"name"`
	Command     *string           `yaml:"command"`
	Args        []string          `yaml:"args"`
	Env         map[string]string `yaml:"env"`
	Extensions  []string          `yaml:"extensions"`
	Enabled     *bool             `yaml:"enabled"`
	InitOptions map[string]any    `yaml:"init_options"`
}

type globalSkillsConfigOverlay struct {
	Models map[string]modelSkillsConfigOverlay `yaml:"models"`
}

type modelSkillsConfigOverlay struct {
	AllowAll *bool    `yaml:"allow_all"`
	Deny     []string `yaml:"deny"`
}
