package app

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/observability"
	"github.com/sageil/kodacode/internal/provider"
	searchsvc "github.com/sageil/kodacode/internal/search"
	"github.com/sageil/kodacode/internal/skill"
	"github.com/sageil/kodacode/internal/tool"
	websearchsvc "github.com/sageil/kodacode/internal/websearch"
	"github.com/sageil/kodacode/internal/workspace"
)

var (
	ErrSessionServiceRequired      = errors.New("session service is required")
	ErrToolNameRequired            = errors.New("tool_name is required")
	ErrToolCallIDRequired          = errors.New("tool_call_id is required")
	ErrToolNotFound                = errors.New("tool not found")
	ErrToolNotAllowed              = errors.New("tool not allowed")
	ErrToolPendingQuestionConflict = errors.New("pending question result cannot include output, error, execution runtime, or execution error")
)

type ToolExecutor struct {
	sessions                     *SessionService
	exec                         *ExecutionService
	tools                        map[string]tool.Tool
	mcpTools                     map[string]struct{}
	search                       *searchsvc.Service
	webSearch                    *websearchsvc.Service
	skills                       *skill.Registry
	codeIntel                    codeIntelRuntime
	memory                       *MemoryService
	workflowConfig               WorkflowConfig
	workflowPhaseCommandResolver workflowPhaseCommandResolver
	logger                       *observability.Logger
}

type workflowPhaseCommandResolver func(ctx context.Context, workspaceRoot, workflowID, phaseID string) ([]workflowVerificationCommandSpec, error)

func NewToolExecutor(sessions *SessionService, tools ...tool.Tool) (*ToolExecutor, error) {
	return NewToolExecutorWithConfig(sessions, defaultExecutionConfig(), tools...)
}

func NewToolExecutorWithConfig(sessions *SessionService, executionConfig ExecutionConfig, tools ...tool.Tool) (*ToolExecutor, error) {
	if sessions == nil {
		return nil, ErrSessionServiceRequired
	}
	var backgroundLogs BackgroundExecutionLogStore
	if sqliteStore, ok := sessions.store.(*events.SQLiteStore); ok {
		backgroundLogs = NewSQLiteBackgroundExecutionLogStore(sqliteStore)
	}
	executor := &ToolExecutor{
		sessions: sessions,
		exec:     NewExecutionServiceWithConfig(sessions, executionConfig),
		tools:    make(map[string]tool.Tool),
		mcpTools: make(map[string]struct{}),
	}
	executor.exec.SetBackgroundLogStore(backgroundLogs)
	for _, tl := range tools {
		executor.Register(tl)
	}
	return executor, nil
}

func (e *ToolExecutor) SetLogger(logger *observability.Logger) {
	e.logger = logger
	if e.exec != nil {
		e.exec.SetLogger(logger)
	}
}

func (e *ToolExecutor) SetSearchService(service *searchsvc.Service) {
	e.search = service
}

func (e *ToolExecutor) SetWebSearchService(service *websearchsvc.Service) {
	e.webSearch = service
}

func (e *ToolExecutor) SetBackgroundLogStore(store BackgroundExecutionLogStore) {
	if e == nil || e.exec == nil {
		return
	}
	e.exec.SetBackgroundLogStore(store)
}

func (e *ToolExecutor) SetSkillRegistry(registry *skill.Registry) {
	e.skills = registry
}

func (e *ToolExecutor) SetWorkflowConfig(config WorkflowConfig) {
	if e == nil {
		return
	}
	e.workflowConfig = config
}

func (e *ToolExecutor) SetWorkflowPhaseCommandResolver(resolver workflowPhaseCommandResolver) {
	if e == nil {
		return
	}
	e.workflowPhaseCommandResolver = resolver
}

func (e *ToolExecutor) ResumeBackgroundExecutions(ctx context.Context, sessionID string) error {
	if e == nil || e.exec == nil {
		return nil
	}
	return e.exec.ResumeBackgroundExecutions(ctx, sessionID)
}

func (e *ToolExecutor) Register(tl tool.Tool) {
	e.tools[tl.Definition().Name] = tl
}

func (e *ToolExecutor) RegisteredToolsForValidation() []tool.Tool {
	if e == nil {
		return nil
	}
	names := make([]string, 0, len(e.tools))
	for name := range e.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]tool.Tool, 0, len(names))
	for _, name := range names {
		out = append(out, e.tools[name])
	}
	return out
}

func (e *ToolExecutor) toolDefinition(toolName string) (tool.Definition, bool) {
	if e == nil {
		return tool.Definition{}, false
	}
	tl, ok := e.tools[strings.TrimSpace(toolName)]
	if !ok {
		return tool.Definition{}, false
	}
	return tl.Definition(), true
}

func (e *ToolExecutor) ReplaceMCPTools(tools []tool.Tool) {
	if e == nil {
		return
	}
	for name := range e.mcpTools {
		delete(e.tools, name)
	}
	clear(e.mcpTools)
	for _, tl := range tools {
		name := tl.Definition().Name
		e.tools[name] = tl
		e.mcpTools[name] = struct{}{}
	}
}

func (e *ToolExecutor) MutationPaths(toolName string, args json.RawMessage) []string {
	if e == nil {
		return nil
	}
	return mutationPathsFromToolMap(e.tools, toolName, args)
}

func (e *ToolExecutor) ProviderTools() []provider.Tool {
	return e.ProviderToolsAllowed(nil)
}

func (e *ToolExecutor) ProviderToolsAllowed(allowed []string) []provider.Tool {
	return append([]provider.Tool(nil), e.providerToolSurfaceAllowed(allowed).Tools...)
}

type providerToolSurface struct {
	Tools                     []provider.Tool
	FullDescriptionTokens     int
	FullSchemaTokens          int
	ProviderDescriptionTokens int
	ProviderSchemaTokens      int
}

func (s providerToolSurface) DescriptionTokensSaved() int {
	return max(s.FullDescriptionTokens-s.ProviderDescriptionTokens, 0)
}

func (s providerToolSurface) SchemaTokensSaved() int {
	return max(s.FullSchemaTokens-s.ProviderSchemaTokens, 0)
}

func (s providerToolSurface) TotalTokensSaved() int {
	return s.DescriptionTokensSaved() + s.SchemaTokensSaved()
}

func (e *ToolExecutor) providerToolSurfaceAllowed(allowed []string) providerToolSurface {
	return e.providerToolSurfaceAllowedFiltered(allowed, nil)
}

func (e *ToolExecutor) providerToolSurfaceAllowedForModel(allowed []string, model provider.ModelRef) providerToolSurface {
	return e.providerToolSurfaceAllowedFiltered(allowed, func(definition tool.Definition) (tool.Definition, bool) {
		if definition.InputKindOrDefault() == tool.InputKindCustom {
			if provider.SupportsCustomTools(model) {
				return definition, true
			}
			if strings.TrimSpace(definition.Name) == tool.ApplyPatchToolName {
				return providerFunctionApplyPatchDefinition(definition), true
			}
			return tool.Definition{}, false
		}
		return definition, true
	})
}

func (e *ToolExecutor) providerToolSurfaceAllowedFiltered(allowed []string, include func(tool.Definition) (tool.Definition, bool)) providerToolSurface {
	if len(e.tools) == 0 {
		return providerToolSurface{}
	}
	names := make([]string, 0, len(e.tools))
	for name := range e.tools {
		if !toolAllowed(name, allowed) {
			continue
		}
		names = append(names, name)
	}
	sortProviderToolNames(names)

	out := providerToolSurface{Tools: make([]provider.Tool, 0, len(names))}
	for _, name := range names {
		tl := e.tools[name]
		definition := tl.Definition()
		if include != nil {
			var ok bool
			definition, ok = include(definition)
			if !ok {
				continue
			}
		}
		providerDescription := providerToolDescription(definition)
		compactSchema := compactProviderToolInputSchema(definition)
		out.FullDescriptionTokens += provider.EstimateTextTokens(definition.Description)
		out.FullSchemaTokens += provider.EstimateTextTokens(string(definition.InputSchema))
		out.ProviderDescriptionTokens += provider.EstimateTextTokens(providerDescription)
		out.ProviderSchemaTokens += provider.EstimateTextTokens(compactSchema)
		out.Tools = append(out.Tools, provider.Tool{
			Name:         definition.Name,
			Description:  providerDescription,
			Kind:         providerToolKind(definition),
			InputSchema:  compactSchema,
			InputFormat:  providerToolInputFormat(definition),
			ParallelSafe: definition.ParallelSafe,
		})
	}
	return out
}

func providerFunctionApplyPatchDefinition(definition tool.Definition) tool.Definition {
	description := "Edit files by sending JSON with one `patch` string. The patch string is raw structured patch text: first line \"*** Begin Patch\", one or more file operations, final line \"*** End Patch\". Supported operations are \"*** Add File: path\", \"*** Update File: path\", and \"*** Delete File: path\". Patch control lines must start at column 1; do not prefix them with \"+\", \"-\", or a space. For Add File, every file-content line must start with \"+\", including blank lines as \"+\". For Update File, hunk lines start with a space, \"+\", or \"-\", and should include enough context to locate the edit. Patch lines MUST NOT include read output line number prefixes like \"40:\" either directly or after patch prefixes like \"-40:\" or \"+40:\". If apply_patch returns a format error, retry apply_patch with the corrected complete patch."
	definition.InputKind = tool.InputKindFunction
	definition.InputFormat = nil
	definition.InputSchema = json.RawMessage(`{"type":"object","properties":{"patch":{"type":"string","description":"Raw structured patch text. First line: *** Begin Patch. Final line: *** End Patch. Patch control lines such as *** Begin Patch, *** Add File:, *** Update File:, *** Delete File:, and *** End Patch must start at column 1; do not prefix them with +, -, or a space. Add File content lines must all start with +, including blank lines as +. Update File hunk lines must start with a space, +, or -, and should include enough context to locate the edit. Patch lines MUST NOT include read output line number prefixes like \"40:\". Do not include Markdown fences or prose."}},"required":["patch"],"additionalProperties":false}`)
	definition.ProviderDescription = description
	definition.ArgumentExamples = []string{
		`{"patch":"*** Begin Patch\n*** Update File: file.txt\n@@\n-old\n+new\n*** End Patch\n"}`,
		`{"patch":"*** Begin Patch\n*** Add File: file.txt\n+first line\n+\n+third line\n*** End Patch\n"}`,
	}
	definition.ProviderRichGuidance = true
	return definition
}

func providerToolKind(definition tool.Definition) provider.ToolKind {
	switch definition.InputKindOrDefault() {
	case tool.InputKindCustom:
		return provider.ToolKindCustom
	default:
		return provider.ToolKindFunction
	}
}

func providerToolInputFormat(definition tool.Definition) *provider.ToolInputFormat {
	if definition.InputFormat == nil {
		return nil
	}
	return &provider.ToolInputFormat{
		Type:       definition.InputFormat.Type,
		Syntax:     definition.InputFormat.Syntax,
		Definition: definition.InputFormat.Definition,
	}
}

func sortProviderToolNames(names []string) {
	sort.Slice(names, func(i, j int) bool {
		left := strings.TrimSpace(names[i])
		right := strings.TrimSpace(names[j])
		// Keep shell execution last so lower-risk structured tools are presented
		// first when the model scans available tool names.
		if left == tool.BashToolName || right == tool.BashToolName {
			return right == tool.BashToolName && left != tool.BashToolName
		}
		return left < right
	})
}

func providerToolDescription(definition tool.Definition) string {
	description := definition.ProviderDescriptionText()
	if !definition.ProviderRichGuidance || len(definition.ArgumentExamples) == 0 {
		return description
	}
	example := strings.TrimSpace(definition.ArgumentExamples[0])
	if example == "" {
		return description
	}
	return description + " Example: " + example
}

func compactProviderToolInputSchema(definition tool.Definition) string {
	schema := definition.InputSchema
	if providerSchema := strings.TrimSpace(string(definition.ProviderInputSchema)); providerSchema != "" {
		schema = definition.ProviderInputSchema
	}
	trimmed := strings.TrimSpace(string(schema))
	if trimmed == "" {
		return ""
	}
	if definition.ProviderRichGuidance {
		return trimmed
	}
	var decoded any
	if err := json.Unmarshal(schema, &decoded); err != nil {
		return trimmed
	}
	compacted, changed := stripProviderToolSchemaDescriptions(decoded)
	if !changed {
		return trimmed
	}
	encoded, err := json.Marshal(compacted)
	if err != nil {
		return trimmed
	}
	return string(encoded)
}

func stripProviderToolSchemaDescriptions(value any) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		compacted := make(map[string]any, len(typed))
		changed := false
		for key, child := range typed {
			if key == "description" {
				changed = true
				continue
			}
			var (
				next         any
				childChanged bool
			)
			if providerToolSchemaMapKey(key) {
				next, childChanged = stripProviderToolSchemaMapValues(child)
			} else {
				next, childChanged = stripProviderToolSchemaDescriptions(child)
			}
			if childChanged {
				changed = true
			}
			compacted[key] = next
		}
		return compacted, changed
	case []any:
		compacted := make([]any, len(typed))
		changed := false
		for index, child := range typed {
			next, childChanged := stripProviderToolSchemaDescriptions(child)
			if childChanged {
				changed = true
			}
			compacted[index] = next
		}
		return compacted, changed
	default:
		return value, false
	}
}

func stripProviderToolSchemaMapValues(value any) (any, bool) {
	typed, ok := value.(map[string]any)
	if !ok {
		return value, false
	}
	compacted := make(map[string]any, len(typed))
	changed := false
	for key, child := range typed {
		next, childChanged := stripProviderToolSchemaDescriptions(child)
		if childChanged {
			changed = true
		}
		compacted[key] = next
	}
	return compacted, changed
}

func providerToolSchemaMapKey(key string) bool {
	switch key {
	case "$defs", "definitions", "dependentSchemas", "patternProperties", "properties":
		return true
	default:
		return false
	}
}

type ExecuteToolInput struct {
	SessionID               string
	TurnID                  string
	ToolCallID              string
	ToolName                string
	ToolKind                provider.ToolKind
	PlanID                  string
	Arguments               json.RawMessage
	ModelVisibleInputs      []provider.Input
	AllowedTools            []string
	TemporaryGrants         []workspace.Grant
	TemporaryNetworkTargets []string
	ExecutionExecPolicy     *events.ExecutionPolicyAmendment
	ExecutionNetworkPolicy  *events.ExecutionNetworkPolicyAmendment
	WorkflowBudget          workflowTurnBudget
}

type ToolExecutionStatus string

const (
	ToolExecutionStatusExecuted ToolExecutionStatus = "executed"
	ToolExecutionStatusPending  ToolExecutionStatus = "pending"
	ToolExecutionStatusReused   ToolExecutionStatus = "reused"
)

const (
	toolFailureClassInvalidArguments = "invalid_arguments"
	toolFailureClassToolNotFound     = "tool_not_found"
	toolFailureClassToolNotAllowed   = "tool_not_allowed"
	toolFailureClassContract         = "contract_violation"
	toolFailureClassInvalidResult    = "invalid_result"
	toolFailureClassExecution        = "execution_failed"
	toolFailureClassPermissionDenied = "permission_denied"
)

type ToolExecutionResult struct {
	Status              ToolExecutionStatus
	CanonicalArguments  string
	Output              string
	Error               string
	ErrorDetail         *events.ToolErrorDetail
	StructuredResult    json.RawMessage
	FailureClass        string
	TurnFailure         error
	PendingRequestID    string
	RetryOfCallID       string
	ReusedFromCallID    string
	ReusedFromSessionID string
	ReusedFromTurnID    string
}
