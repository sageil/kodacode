package app

import (
	"encoding/json"
	"strings"

	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
	"github.com/sageil/kodacode/internal/workspace"
)

type stepToolBatch struct {
	StepIndex int
	Calls     []stepToolCall
}

type stepToolCall struct {
	CallID                 string
	ToolName               string
	ToolKind               provider.ToolKind
	Arguments              string
	GoogleThoughtSignature []byte
	OpenAIReasoningContent string
}

type stepToolResult struct {
	CallID              string
	ToolName            string
	CanonicalArguments  string
	Output              string
	Error               string
	FailureClass        string
	TurnFailure         error
	Status              ToolExecutionStatus
	PendingRequestID    string
	RetryOfCallID       string
	ReusedFromCallID    string
	ReusedFromSessionID string
	ReusedFromTurnID    string
}

type stepToolSchedule struct {
	Executable stepToolBatch
	Deferred   []stepToolCall
}

type stepToolBatchExecution struct {
	Schedule stepToolSchedule
	Results  []stepToolResult
}

func (b stepToolBatch) Len() int {
	return len(b.Calls)
}

func (b stepToolBatch) CallIDs() []string {
	if len(b.Calls) == 0 {
		return nil
	}
	out := make([]string, 0, len(b.Calls))
	for _, call := range b.Calls {
		if call.CallID != "" {
			out = append(out, call.CallID)
		}
	}
	return out
}

func (b stepToolBatch) HasCallID(callID string) bool {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return false
	}
	for _, call := range b.Calls {
		if call.CallID == callID {
			return true
		}
	}
	return false
}

func (b *stepToolBatch) AppendCall(call stepToolCall) bool {
	if b == nil {
		return false
	}
	call.CallID = strings.TrimSpace(call.CallID)
	call.ToolName = strings.TrimSpace(call.ToolName)
	call.ToolKind = normalizeStepToolKind(call.ToolKind)
	if call.ToolKind != provider.ToolKindCustom {
		call.Arguments = strings.TrimSpace(call.Arguments)
	}
	if call.CallID == "" || b.HasCallID(call.CallID) {
		return false
	}
	call.GoogleThoughtSignature = append([]byte(nil), call.GoogleThoughtSignature...)
	b.Calls = append(b.Calls, call)
	return true
}

func normalizeStepToolKind(kind provider.ToolKind) provider.ToolKind {
	switch kind {
	case provider.ToolKindCustom:
		return provider.ToolKindCustom
	default:
		return provider.ToolKindFunction
	}
}

func scheduleStepToolBatch(batch stepToolBatch) stepToolSchedule {
	return scheduleStepToolBatchWithResolver(nil, batch)
}

func scheduleStepToolBatchWithResolver(resolver stepToolCapabilityResolver, batch stepToolBatch) stepToolSchedule {
	scheduled := stepToolSchedule{
		Executable: stepToolBatch{
			StepIndex: batch.StepIndex,
			Calls:     make([]stepToolCall, 0, len(batch.Calls)),
		},
	}
	for idx, call := range batch.Calls {
		if !providerStepToolCallCanJoinPrefixWithResolver(resolver, scheduled.Executable.Calls, call) {
			scheduled.Deferred = cloneStepToolCalls(batch.Calls[idx:])
			return scheduled
		}
		scheduled.Executable.Calls = append(scheduled.Executable.Calls, cloneStepToolCall(call))
		if providerStepBoundaryToolCallWithResolver(resolver, call.ToolName, call.Arguments) {
			scheduled.Deferred = cloneStepToolCalls(batch.Calls[idx+1:])
			return scheduled
		}
	}
	return scheduled
}

func interruptedStreamSafeStepToolBatchWithResolver(resolver stepToolCapabilityResolver, batch stepToolBatch) stepToolBatch {
	safe := stepToolBatch{
		StepIndex: batch.StepIndex,
		Calls:     make([]stepToolCall, 0, len(batch.Calls)),
	}
	for _, call := range batch.Calls {
		if !providerStepParallelReadToolCallWithResolver(resolver, call.ToolName, call.Arguments) {
			break
		}
		safe.Calls = append(safe.Calls, cloneStepToolCall(call))
	}
	return safe
}

func cloneStepToolCalls(calls []stepToolCall) []stepToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]stepToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, cloneStepToolCall(call))
	}
	return out
}

func cloneStepToolCall(call stepToolCall) stepToolCall {
	call.GoogleThoughtSignature = append([]byte(nil), call.GoogleThoughtSignature...)
	return call
}

type toolExecutionClass string

const (
	toolExecutionParallelRead toolExecutionClass = "parallel_read"
	toolExecutionSequential   toolExecutionClass = "sequential"
	toolExecutionBlocking     toolExecutionClass = "blocking"
)

type toolSchedulingCapability struct {
	ExecutionClass    toolExecutionClass
	ParallelSafe      bool
	StepBoundaryAfter bool
}

type stepToolCapabilityResolver func(toolName, arguments string) toolSchedulingCapability

func newStepToolCapabilityResolver(executor *ToolExecutor) stepToolCapabilityResolver {
	return func(toolName, arguments string) toolSchedulingCapability {
		return providerStepToolSchedulingCapabilityForExecutor(executor, toolName, arguments)
	}
}

func providerStepToolExecutionClass(toolName, arguments string) toolExecutionClass {
	return providerStepToolSchedulingCapability(toolName, arguments).ExecutionClass
}

func providerStepToolSchedulingCapability(toolName, arguments string) toolSchedulingCapability {
	toolName = strings.TrimSpace(toolName)
	if capability, ok := providerStepStaticToolSchedulingCapability(toolName); ok {
		return capability
	}
	return providerStepArgumentToolSchedulingCapability(toolName, arguments)
}

func providerStepToolSchedulingCapabilityForExecutor(executor *ToolExecutor, toolName, arguments string) toolSchedulingCapability {
	toolName = strings.TrimSpace(toolName)
	if providerStepBlockingToolName(toolName) {
		return toolSchedulingCapability{ExecutionClass: toolExecutionBlocking}
	}
	if executor != nil {
		if definition, ok := executor.toolDefinition(toolName); ok {
			if definition.ParallelSafe {
				return toolSchedulingCapability{ExecutionClass: toolExecutionParallelRead, ParallelSafe: true}
			}
			if executorToolCallIsWorkspaceMutation(executor, toolName, arguments) {
				return toolSchedulingCapability{ExecutionClass: toolExecutionSequential, StepBoundaryAfter: true}
			}
			if capability := providerStepArgumentToolSchedulingCapability(toolName, arguments); capability.ExecutionClass != toolExecutionSequential {
				return capability
			}
			return toolSchedulingCapability{ExecutionClass: toolExecutionSequential}
		}
	}
	if capability := providerStepArgumentToolSchedulingCapability(toolName, arguments); capability.ExecutionClass != toolExecutionSequential {
		return capability
	}
	return providerStepToolSchedulingCapability(toolName, arguments)
}

func executorToolCallIsWorkspaceMutation(executor *ToolExecutor, toolName, arguments string) bool {
	toolName = strings.TrimSpace(toolName)
	switch toolName {
	case tool.WriteToolName,
		tool.ApplyPatchToolName,
		tool.CodeActionToolName,
		tool.RenameSymbolToolName,
		"mkdir":
		return true
	case tool.BashToolName:
		effect, err := tool.BashArgumentsExecutionEffect(json.RawMessage(arguments))
		return err != nil || effect != tool.ExecutionEffectRead
	}
	if executor == nil {
		return false
	}
	tl, ok := executor.tools[toolName]
	if !ok {
		return false
	}
	introspector, ok := tl.(tool.PathIntrospector)
	if !ok {
		return false
	}
	requests, err := introspector.PathRequests(json.RawMessage(arguments))
	if err != nil {
		return false
	}
	for _, request := range requests {
		if request.Access == workspace.AccessWrite {
			return true
		}
	}
	return false
}

func providerStepParallelReadToolCall(toolName, arguments string) bool {
	capability := providerStepToolSchedulingCapability(toolName, arguments)
	return capability.ExecutionClass == toolExecutionParallelRead && capability.ParallelSafe
}

func providerStepParallelReadToolCallWithResolver(resolver stepToolCapabilityResolver, toolName, arguments string) bool {
	if resolver == nil {
		return providerStepParallelReadToolCall(toolName, arguments)
	}
	capability := resolver(toolName, arguments)
	return capability.ExecutionClass == toolExecutionParallelRead && capability.ParallelSafe
}

func providerStepStaticToolSchedulingCapability(toolName string) (toolSchedulingCapability, bool) {
	switch strings.TrimSpace(toolName) {
	case tool.ReadToolName,
		tool.LocateToolName,
		tool.SearchToolName,
		tool.DefinitionToolName,
		tool.DiagnosticsToolName,
		tool.SymbolsToolName,
		"git_status",
		"git_diff",
		"git_show":
		return toolSchedulingCapability{ExecutionClass: toolExecutionParallelRead, ParallelSafe: true}, true
	case tool.QuestionToolName:
		return toolSchedulingCapability{ExecutionClass: toolExecutionBlocking}, true
	default:
		return toolSchedulingCapability{}, false
	}
}

func providerStepArgumentToolSchedulingCapability(toolName, arguments string) toolSchedulingCapability {
	switch strings.TrimSpace(toolName) {
	case tool.BashToolName:
		effect, err := tool.BashArgumentsExecutionEffect([]byte(arguments))
		if err == nil && effect == tool.ExecutionEffectRead {
			return toolSchedulingCapability{ExecutionClass: toolExecutionParallelRead, ParallelSafe: true}
		}
	case tool.MemoryToolName,
		tool.TaskWorkflowToolName,
		tool.TaskReviewToolName:
		if providerStepBatchableToolCall(toolName, arguments) {
			return toolSchedulingCapability{ExecutionClass: toolExecutionParallelRead, ParallelSafe: true}
		}
	}
	return toolSchedulingCapability{ExecutionClass: toolExecutionSequential}
}

func providerStepParallelReadToolName(toolName string) bool {
	capability, ok := providerStepStaticToolSchedulingCapability(toolName)
	return ok && capability.ExecutionClass == toolExecutionParallelRead && capability.ParallelSafe
}

func providerStepBlockingToolName(toolName string) bool {
	capability, ok := providerStepStaticToolSchedulingCapability(toolName)
	return ok && capability.ExecutionClass == toolExecutionBlocking
}

func providerStepBoundaryToolCallWithResolver(resolver stepToolCapabilityResolver, toolName, arguments string) bool {
	var capability toolSchedulingCapability
	if resolver == nil {
		capability = providerStepToolSchedulingCapability(toolName, arguments)
	} else {
		capability = resolver(toolName, arguments)
	}
	return capability.ExecutionClass == toolExecutionBlocking || capability.StepBoundaryAfter
}

func providerStepToolCallCanJoinPrefix(prefix []stepToolCall, next stepToolCall) bool {
	return providerStepToolCallCanJoinPrefixWithResolver(nil, prefix, next)
}

func providerStepToolCallCanJoinPrefixWithResolver(resolver stepToolCapabilityResolver, prefix []stepToolCall, next stepToolCall) bool {
	for _, call := range prefix {
		if providerStepBoundaryToolCallWithResolver(resolver, call.ToolName, call.Arguments) {
			return false
		}
	}
	return true
}
