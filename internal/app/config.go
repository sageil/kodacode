package app

import (
	"errors"
	"strings"

	"github.com/sageil/kodacode/internal/observability"
	"github.com/sageil/kodacode/internal/permissionpolicy"
	"github.com/sageil/kodacode/internal/provider"
)

var (
	ErrUnsupportedModelProvider           = errors.New("unsupported model provider")
	ErrOpenAICompatibleProviderIDRequired = errors.New("openai compatible provider_id is required")
	ErrOpenAICompatibleBaseURLRequired    = errors.New("openai compatible base_url is required")
	ErrOpenAICompatibleAPIKeyRequired     = errors.New("openai compatible api key is required")
	ErrOpenAICompatibleReservedProviderID = errors.New("openai compatible provider_id is reserved")
	ErrAnthropicAPIKeyRequired            = errors.New("anthropic api key is required")
	ErrGoogleAPIKeyRequired               = errors.New("google api key is required")
	ErrNVIDIAAPIKeyRequired               = errors.New("nvidia api key is required")
	ErrDeepSeekAPIKeyRequired             = errors.New("deepseek api key is required")
	ErrWebSearchDefaultProviderRequired   = errors.New("web search default provider is required when multiple providers are configured")
	ErrWebSearchDefaultProviderNotFound   = errors.New("web search default provider is not configured")
	ErrWebSearchProviderAPIKeyRequired    = errors.New("web search provider api key is required")
	ErrWebSearchProviderKindRequired      = errors.New("web search provider kind is required")
	ErrWebSearchProviderKindUnsupported   = errors.New("unsupported web search provider kind")
)

type Config struct {
	ModelRoute                       provider.ModelRoute
	ModelOverrides                   []ModelOverrideConfig
	OutputBudgets                    OutputBudgetsConfig
	UtilityModel                     provider.ModelRef
	UtilityModelTimeoutSeconds       int
	UtilityModelRetryAttempts        int
	UtilityModelRetryAfterMaxSeconds int
	ModelCache                       ModelCacheConfig
	Retention                        RetentionConfig
	MCP                              MCPConfig
	Execution                        ExecutionConfig
	Workflow                         WorkflowConfig
	OpenAI                           OpenAIProviderConfig
	Anthropic                        AnthropicProviderConfig
	Google                           GoogleProviderConfig
	NVIDIA                           NVIDIAProviderConfig
	OpenAICompatible                 OpenAICompatibleProviderConfig
	CompatibleProviders              map[string]OpenAICompatibleProviderConfig
	GitHubCopilot                    GitHubCopilotProviderConfig
	DeepSeek                         DeepSeekProviderConfig
	Search                           SearchConfig
	WebSearch                        WebSearchConfig
	LSP                              LSPConfig
	ContextPacket                    ContextPacketConfig
	Sessions                         SessionConfig
	Permissions                      permissionpolicy.Config
	Logging                          observability.Config
}

type OpenAIProviderConfig struct {
	APIKey                   string
	BaseURL                  string
	PromptCacheRetention     string
	ResponsesStore           bool
	EncryptedReasoningReplay *bool
}

type AnthropicProviderConfig struct {
	APIKey  string
	BaseURL string
}

type GoogleProviderConfig struct {
	APIKey  string
	BaseURL string
}

type NVIDIAProviderConfig struct {
	APIKey  string
	BaseURL string
}

type OpenAICompatibleProviderConfig struct {
	ProviderID string
	APIKey     string
	BaseURL    string
}

type GitHubCopilotProviderConfig struct {
	Token   string
	BaseURL string
}

type DeepSeekProviderConfig struct {
	APIKey  string
	BaseURL string
}

type SearchConfig struct {
	IndexDir             string
	SkipDirs             []string
	EmbeddingsModel      string
	EmbeddingsDimensions int
	PrewarmEmbeddings    bool
}

type WebSearchConfig struct {
	DefaultProvider string
	Providers       map[string]WebSearchProviderConfig
}

type WebSearchProviderConfig struct {
	Kind      string
	BaseURL   string
	TimeoutMS int
	APIKey    string
}

type ContextPacketConfig struct {
	EnabledSections []string
}

type MCPConfig struct {
	Servers []MCPServerConfig
}

type MCPToolHintConfig struct {
	Summary               string
	Guidance              string
	Triggers              []string
	FileExts              []string
	PreserveParameterDocs bool
}

type MCPServerConfig struct {
	Name      string
	Type      string
	Command   string
	Args      []string
	URL       string
	Headers   map[string]string
	Env       map[string]string
	ToolHints map[string]MCPToolHintConfig
	Enabled   *bool
}

func (c MCPServerConfig) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

type ModelCacheConfig struct {
	ExpiryDays int
}

type RetentionConfig struct {
	ExpiryDays int
}

const defaultMaxOutputContinuations = 1
const maxOutputContinuationsLimit = 2

type SessionConfig struct {
	DBPath                     string
	Budget                     float64
	BudgetWarn                 float64
	TotalBudget                float64
	TotalBudgetWarn            float64
	ResponseStyle              ResponseStyle
	CompactionThreshold        float64
	CompactionTargetThreshold  float64
	MaxProviderRequestsPerTurn int
	MaxOutputContinuations     int
	MaxRetries                 int
}

func (c SessionConfig) EffectiveMaxProviderRequestsPerTurn() int {
	switch {
	case c.MaxProviderRequestsPerTurn < 0:
		return 0
	case c.MaxProviderRequestsPerTurn == 0:
		return defaultMaxProviderRequestsPerTurn
	default:
		return c.MaxProviderRequestsPerTurn
	}
}

func (c SessionConfig) EffectiveMaxOutputContinuations() int {
	if c.MaxOutputContinuations < 0 {
		return 0
	}
	return c.MaxOutputContinuations
}

func (c SessionConfig) EffectiveResponseStyle() ResponseStyle {
	if strings.TrimSpace(string(c.ResponseStyle)) == "" {
		return ResponseStyleTerse
	}
	if style := normalizeResponseStyle(c.ResponseStyle); style != "" {
		return style
	}
	return ResponseStyleTerse
}

type ExecutionNetworkPolicy string

const (
	ExecutionNetworkDisabled ExecutionNetworkPolicy = "disabled"
	ExecutionNetworkEnabled  ExecutionNetworkPolicy = "enabled"
)

type ExecutionConfig struct {
	PermissionMode  PermissionMode
	Network         ExecutionNetworkPolicy
	AllowLoginShell bool
	ShellProgram    string
}
