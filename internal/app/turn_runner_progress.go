package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/sageil/kodacode/internal/tool"
)

const defaultMaxProviderRequestsPerTurn = 32
const repeatedToolLoopWindowSize = 10
const repeatedToolLoopMaxRepeats = 4

type repeatedToolLoopState struct {
	Recent []string
	Counts map[string]int
	Match  string
}

func (r *TurnRunner) maxTurnProviderRequestsPerTurn() int {
	if r == nil {
		return defaultMaxProviderRequestsPerTurn
	}
	if r.maxProviderRequestsPerTurn < 0 {
		return 0
	}
	return r.maxProviderRequestsPerTurn
}

func (r *TurnRunner) sessionProviderRequestLimitDisabled(ctx context.Context, sessionID string) (bool, error) {
	if r == nil || r.sessions == nil {
		return false, nil
	}
	state, err := r.sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return false, err
	}
	return state.ProviderRequestLimitDisabled, nil
}

func (r *TurnRunner) maxOutputContinuationAttempts() int {
	if r == nil {
		return defaultMaxOutputContinuations
	}
	if r.maxOutputContinuations < 0 {
		return 0
	}
	return min(r.maxOutputContinuations, maxOutputContinuationsLimit)
}

func nextRepeatedToolLoopState(current repeatedToolLoopState, result assistantRoundtripResult) repeatedToolLoopState {
	if result.Outcome != assistantRoundtripOutcomeToolResult || len(result.ToolInteractionSigs) == 0 {
		return repeatedToolLoopState{}
	}
	next := repeatedToolLoopState{
		Recent: append([]string(nil), current.Recent...),
		Counts: make(map[string]int, len(current.Counts)+len(result.ToolInteractionSigs)),
	}
	for sig, count := range current.Counts {
		next.Counts[sig] = count
	}
	for _, sig := range result.ToolInteractionSigs {
		sig = strings.TrimSpace(sig)
		if sig == "" {
			continue
		}
		next.Recent = append(next.Recent, sig)
		next.Counts[sig]++
		if next.Counts[sig] > repeatedToolLoopMaxRepeats {
			next.Match = sig
		}
		for len(next.Recent) > repeatedToolLoopWindowSize {
			removed := next.Recent[0]
			next.Recent = next.Recent[1:]
			if count := next.Counts[removed]; count <= 1 {
				delete(next.Counts, removed)
			} else {
				next.Counts[removed] = count - 1
			}
		}
	}
	return next
}

func (s repeatedToolLoopState) Repeated() bool {
	return strings.TrimSpace(s.Match) != ""
}

func compactJSONForFingerprint(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return raw
	}
	out, err := json.Marshal(decoded)
	if err != nil {
		return raw
	}
	return string(out)
}

func providerStepToolInteractionSignature(tools *ToolExecutor, toolName, arguments string, result stepToolResult) string {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return ""
	}
	normalizedArgs := strings.TrimSpace(arguments)
	if tools != nil && tools.tools != nil {
		if key, ok := normalizedToolInputKey(tools.tools, toolName, json.RawMessage(arguments)); ok && strings.TrimSpace(key) != "" {
			normalizedArgs = key
		}
	}
	if normalizedArgs == "" {
		normalizedArgs = compactJSONForFingerprint(arguments)
	}
	normalizedResult := providerStepToolResultFingerprint(result)
	if normalizedResult == "" {
		return ""
	}
	return hashFingerprint(toolName + "\x00" + normalizedArgs + "\x00" + normalizedResult)
}

func providerStepToolResultFingerprint(result stepToolResult) string {
	status := strings.TrimSpace(string(result.Status))
	if status == "" {
		status = string(ToolExecutionStatusExecuted)
	}
	if failureClass := strings.TrimSpace(result.FailureClass); failureClass != "" {
		return status + ":failure:" + failureClass + ":" + hashFingerprint(singleLineCompact(result.Error))
	}
	if errText := strings.TrimSpace(result.Error); errText != "" {
		return status + ":error:" + hashFingerprint(singleLineCompact(errText))
	}
	return status + ":output:" + hashFingerprint(strings.TrimSpace(result.Output))
}

func providerStepFailedToolFingerprint(tools *ToolExecutor, toolName, arguments, failureClass string) string {
	toolName = strings.TrimSpace(toolName)
	failureClass = strings.TrimSpace(failureClass)
	if toolName == "" || failureClass == "" {
		return ""
	}
	normalizedArgs := strings.TrimSpace(arguments)
	if tools != nil && tools.tools != nil {
		if key, ok := normalizedToolInputKey(tools.tools, toolName, json.RawMessage(arguments)); ok && strings.TrimSpace(key) != "" {
			normalizedArgs = key
		}
	}
	if normalizedArgs == "" {
		normalizedArgs = compactJSONForFingerprint(arguments)
	}
	return hashFingerprint(toolName + "\x00" + normalizedArgs + "\x00" + failureClass)
}

func hashFingerprint(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// Batch local inspection tools plus low-risk workflow setup calls that do not
// require immediate re-planning after each result. Mutation, network, and
// question-style tools stay as provider request barriers.
func providerStepBatchableToolName(toolName string) bool {
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
		return true
	default:
		return false
	}
}

func providerStepPotentiallyBatchableToolName(toolName string) bool {
	if providerStepBatchableToolName(toolName) {
		return true
	}
	switch strings.TrimSpace(toolName) {
	case tool.BashToolName,
		tool.MemoryToolName,
		tool.TaskWorkflowToolName,
		tool.TaskReviewToolName:
		return true
	default:
		return false
	}
}

func providerStepBatchableToolCall(toolName, arguments string) bool {
	if providerStepBatchableToolName(toolName) {
		return true
	}
	switch strings.TrimSpace(toolName) {
	case tool.BashToolName:
		effect, err := tool.BashArgumentsExecutionEffect([]byte(arguments))
		return err == nil && effect == tool.ExecutionEffectRead
	case tool.MemoryToolName:
		var raw struct {
			Action string `json:"action"`
		}
		if err := json.Unmarshal([]byte(arguments), &raw); err != nil {
			return false
		}
		return strings.TrimSpace(raw.Action) == "list"
	case tool.TaskWorkflowToolName:
		var raw struct {
			Action string `json:"action"`
		}
		if err := json.Unmarshal([]byte(arguments), &raw); err != nil {
			return false
		}
		switch strings.TrimSpace(raw.Action) {
		case "list":
			return true
		default:
			return false
		}
	case tool.TaskReviewToolName:
		var raw struct {
			Action string `json:"action"`
		}
		if err := json.Unmarshal([]byte(arguments), &raw); err != nil {
			return false
		}
		return strings.TrimSpace(raw.Action) == "list"
	default:
		return false
	}
}

func boolFingerprint(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func boolPointerValue(value *bool) bool {
	if value == nil {
		return false
	}
	return *value
}

func intFingerprint(value int) string {
	return strings.TrimSpace(strconv.Itoa(value))
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
