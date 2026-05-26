package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/internal/permissionpolicy"
	"gopkg.in/yaml.v3"
)

type StoredConfig struct {
	Version                          int                          `yaml:"version,omitempty"`
	Providers                        []StoredProvider             `yaml:"providers,omitempty"`
	Model                            StoredModelConfig            `yaml:"model,omitempty"`
	ModelOverrides                   []StoredModelOverride        `yaml:"model_overrides,omitempty"`
	OutputBudgets                    StoredOutputBudgetsConfig    `yaml:"output_budgets,omitempty"`
	MCP                              StoredMCPConfig              `yaml:"mcp,omitempty"`
	Workflow                         StoredWorkflowConfig         `yaml:"workflow,omitempty"`
	UtilityModel                     string                       `yaml:"utility_model,omitempty"`
	UtilityModelTimeoutSeconds       *int                         `yaml:"utility_model_timeout_seconds,omitempty"`
	UtilityModelRetryAttempts        *int                         `yaml:"utility_model_retry_attempts,omitempty"`
	UtilityModelRetryAfterMaxSeconds *int                         `yaml:"utility_model_retry_after_max_seconds,omitempty"`
	Search                           StoredSearchConfig           `yaml:"search,omitempty"`
	WebSearch                        StoredWebSearchConfig        `yaml:"web_search,omitempty"`
	ContextPacket                    StoredContextPacketConfig    `yaml:"context_packet,omitempty"`
	Sessions                         StoredSessionConfig          `yaml:"sessions,omitempty"`
	TUI                              StoredTUIConfig              `yaml:"tui,omitempty"`
	ModelCache                       StoredModelCacheConfig       `yaml:"model_cache,omitempty"`
	Retention                        StoredRetentionConfig        `yaml:"retention,omitempty"`
	Execution                        StoredExecutionConfig        `yaml:"execution,omitempty"`
	Permissions                      StoredPermissionPolicyConfig `yaml:"permissions,omitempty"`
	Logging                          StoredLoggingConfig          `yaml:"logging,omitempty"`
}

type StoredProvider struct {
	ID                       string `yaml:"id"`
	BaseURL                  string `yaml:"base_url,omitempty"`
	PromptCacheRetention     string `yaml:"prompt_cache_retention,omitempty"`
	ResponsesStore           *bool  `yaml:"responses_store,omitempty"`
	EncryptedReasoningReplay *bool  `yaml:"encrypted_reasoning_replay,omitempty"`
}

type StoredModelConfig struct {
	Primary string `yaml:"primary,omitempty"`
}

type StoredModelOverride struct {
	Ref                 string   `yaml:"ref"`
	Name                string   `yaml:"name,omitempty"`
	ContextSize         *int     `yaml:"context_size,omitempty"`
	MaxInputTokens      *int     `yaml:"max_input_tokens,omitempty"`
	MaxOutputTokens     *int     `yaml:"max_output_tokens,omitempty"`
	DefaultOutputTokens *int     `yaml:"default_output_tokens,omitempty"`
	Reasoning           *bool    `yaml:"reasoning,omitempty"`
	ToolCalls           *bool    `yaml:"tool_calls,omitempty"`
	Vision              *bool    `yaml:"vision,omitempty"`
	CostInput           *float64 `yaml:"cost_input,omitempty"`
	CostOutput          *float64 `yaml:"cost_output,omitempty"`
}

type StoredOutputBudgetsConfig struct {
	SessionTitle      *int `yaml:"session_title,omitempty"`
	UtilityText       *int `yaml:"utility_text,omitempty"`
	Review            *int `yaml:"review,omitempty"`
	AgentTurn         *int `yaml:"agent_turn,omitempty"`
	AgentTurnThinking *int `yaml:"agent_turn_thinking,omitempty"`
	WorkspaceCompress *int `yaml:"workspace_compress,omitempty"`
	SessionCompaction *int `yaml:"session_compaction,omitempty"`
}

type StoredSearchConfig struct {
	IndexDir             string   `yaml:"index_dir,omitempty"`
	SkipDirs             []string `yaml:"skip_dirs,omitempty"`
	EmbeddingsModel      string   `yaml:"embeddings_model,omitempty"`
	EmbeddingsDimensions int      `yaml:"embeddings_dimensions,omitempty"`
	PrewarmEmbeddings    bool     `yaml:"prewarm_embeddings,omitempty"`
}

type StoredWebSearchConfig struct {
	DefaultProvider string                                   `yaml:"default_provider,omitempty"`
	Providers       map[string]StoredWebSearchProviderConfig `yaml:"providers,omitempty"`
}

type StoredWebSearchProviderConfig struct {
	Kind      string `yaml:"kind,omitempty"`
	BaseURL   string `yaml:"base_url,omitempty"`
	TimeoutMS int    `yaml:"timeout_ms,omitempty"`
}

type StoredContextPacketConfig struct {
	EnabledSections []string `yaml:"enabled_sections,omitempty"`
}

type StoredMCPConfig struct {
	Servers []StoredMCPServerConfig `yaml:"servers,omitempty"`
}

type StoredMCPToolHintConfig struct {
	Summary               string   `yaml:"summary,omitempty"`
	Guidance              string   `yaml:"guidance,omitempty"`
	Triggers              []string `yaml:"triggers,omitempty"`
	FileExts              []string `yaml:"file_exts,omitempty"`
	PreserveParameterDocs *bool    `yaml:"preserve_parameter_docs,omitempty"`
}

type StoredMCPServerConfig struct {
	Name      string                             `yaml:"name,omitempty"`
	Type      string                             `yaml:"type,omitempty"`
	Command   string                             `yaml:"command,omitempty"`
	Args      []string                           `yaml:"args,omitempty"`
	URL       string                             `yaml:"url,omitempty"`
	Headers   map[string]string                  `yaml:"headers,omitempty"`
	Env       map[string]string                  `yaml:"env,omitempty"`
	ToolHints map[string]StoredMCPToolHintConfig `yaml:"tool_hints,omitempty"`
	Enabled   *bool                              `yaml:"enabled,omitempty"`
}

type StoredSessionConfig struct {
	DBPath                     string   `yaml:"db_path,omitempty"`
	Budget                     float64  `yaml:"budget,omitempty"`
	BudgetWarn                 float64  `yaml:"budget_warn,omitempty"`
	TotalBudget                float64  `yaml:"total_budget,omitempty"`
	TotalBudgetWarn            float64  `yaml:"total_budget_warn,omitempty"`
	ResponseStyle              string   `yaml:"response_style,omitempty"`
	CompactionThreshold        *float64 `yaml:"compaction_threshold,omitempty"`
	CompactionTargetThreshold  *float64 `yaml:"compaction_target_threshold,omitempty"`
	MaxProviderRequestsPerTurn *int     `yaml:"max_provider_requests_per_turn,omitempty"`
	MaxOutputContinuations     *int     `yaml:"max_output_continuations,omitempty"`
	MaxRetries                 *int     `yaml:"max_retries,omitempty"`
}

type StoredTUIConfig struct {
	Theme        string `yaml:"theme,omitempty"`
	DisplayTurns int    `yaml:"display_turns,omitempty"`
}

type StoredModelCacheConfig struct {
	ExpiryDays *int `yaml:"expiry_days,omitempty"`
}

type StoredRetentionConfig struct {
	ExpiryDays *int `yaml:"expiry_days,omitempty"`
}

type StoredExecutionConfig struct {
	PermissionMode  string `yaml:"permission_mode,omitempty"`
	Network         string `yaml:"network,omitempty"`
	AllowLoginShell *bool  `yaml:"allow_login_shell,omitempty"`
	ShellProgram    string `yaml:"shell_program,omitempty"`
}

type StoredPermissionPolicyConfig struct {
	ExternalDirectory StoredPermissionSubjectConfig `yaml:"external_directory,omitempty"`
	Bash              StoredPermissionSubjectConfig `yaml:"bash,omitempty"`
	WebFetch          StoredPermissionSubjectConfig `yaml:"web_fetch,omitempty"`
	NetworkTarget     StoredPermissionSubjectConfig `yaml:"network_target,omitempty"`
}

type StoredPermissionSubjectConfig []permissionpolicy.Rule

func (s *StoredPermissionSubjectConfig) UnmarshalYAML(node *yaml.Node) error {
	if s == nil || node == nil {
		return nil
	}
	switch node.Kind {
	case 0:
		return nil
	case yaml.MappingNode:
		rules := make([]permissionpolicy.Rule, 0, len(node.Content)/2)
		for idx := 0; idx+1 < len(node.Content); idx += 2 {
			patternNode := node.Content[idx]
			actionNode := node.Content[idx+1]
			rules = append(rules, permissionpolicy.Rule{
				Pattern: strings.TrimSpace(patternNode.Value),
				Action:  permissionpolicy.Action(strings.TrimSpace(actionNode.Value)),
			})
		}
		*s = rules
		return nil
	case yaml.ScalarNode:
		if strings.TrimSpace(node.Value) == "" {
			*s = nil
			return nil
		}
		return fmt.Errorf("permission subject rules must be a mapping")
	default:
		return fmt.Errorf("permission subject rules must be a mapping")
	}
}

type StoredLoggingConfig struct {
	Dir   string `yaml:"dir,omitempty"`
	Debug bool   `yaml:"debug,omitempty"`
}

type ConfigStore struct {
	path string
}

func NewConfigStore() *ConfigStore {
	return &ConfigStore{path: filepath.Join(appConfigDir(), "config.yaml")}
}

func NewConfigStoreAt(path string) *ConfigStore {
	return &ConfigStore{path: path}
}

func (s *ConfigStore) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}
