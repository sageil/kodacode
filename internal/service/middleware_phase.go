package service

import (
	"context"
	"encoding/json"
	"log"
	"maps"
	"regexp"
	"strings"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
)

// ---------------------------------------------------------------------------
// Tool sets for planner approval and execution states
// ---------------------------------------------------------------------------
var readOnlyTools = map[string]bool{
	"read": true, "read_files": true, "glob": true, "grep": true,
	"search": true, "lsp": true, "tree": true,
}

var postplanTools = mergeMaps(readOnlyTools, map[string]bool{
	"bash": true, "test": true,
	"subagent": true, "question": true, "task": true, "task_output": true, "skill": true,
	"memory": true,
})

var prebuildTools = map[string]bool{
	"test":   true,
	"skill":  true,
	"memory": true,
}

// ---------------------------------------------------------------------------
// Functional phase rules
// ---------------------------------------------------------------------------

// PhaseResult is the output of evaluating a phase rule.
type PhaseResult struct {
	Tools            []provider.Tool
	Messages         []provider.Message
	RuntimeDirective string
	Done             bool   // true = this rule matched, stop evaluating
	Name             string // phase name for logging
}

// PhaseRule evaluates whether a phase applies and returns the transformed
// tool set and messages. If the rule does not apply it returns Done=false.
type PhaseRule func(allTools []provider.Tool, msgs []provider.Message, step int, workflow *pipeline.WorkflowState) PhaseResult

// builderPhaseRules enforce a real verification-first prebuild gate, then
// govern planner approval / execution states. Engineer stays in prebuild until
// a test attempt completes; it does not auto-advance on step count.
var builderPhaseRules = []PhaseRule{
	phasePrebuild,
	phaseApproved,
	phasePostplanRejected,
	phasePostplanPending,
}

// evalPhases evaluates rules in order, returning the first match.
func evalPhases(rules []PhaseRule, allTools []provider.Tool, msgs []provider.Message, step int, workflow *pipeline.WorkflowState) PhaseResult {
	for _, rule := range rules {
		if r := rule(allTools, msgs, step, workflow); r.Done {
			return r
		}
	}
	return PhaseResult{Tools: allTools, Messages: msgs, Done: true, Name: "passthrough"}
}

// ---------------------------------------------------------------------------
// Individual phase rules (pure functions)
// ---------------------------------------------------------------------------

func phaseApproved(allTools []provider.Tool, msgs []provider.Message, _ int, workflow *pipeline.WorkflowState) PhaseResult {
	if workflow == nil || workflow.Plan.EffectiveStatus != pipeline.WorkflowApprovalApproved {
		return PhaseResult{Done: false}
	}
	return PhaseResult{
		Tools:            removeToolByName(allTools, "question"),
		Messages:         msgs,
		RuntimeDirective: approvalRuntimeDirective(workflow.Plan.LatestAnswer),
		Done:             true,
		Name:             "approved",
	}
}

func phasePrebuild(allTools []provider.Tool, msgs []provider.Message, _ int, workflow *pipeline.WorkflowState) PhaseResult {
	if workflow == nil || workflow.Phase != pipeline.WorkflowPhasePrebuild {
		return PhaseResult{Done: false}
	}
	return PhaseResult{
		Tools:    filterTools(allTools, prebuildTools),
		Messages: msgs,
		Done:     true,
		Name:     "prebuild",
	}
}

func phasePostplanRejected(allTools []provider.Tool, msgs []provider.Message, _ int, workflow *pipeline.WorkflowState) PhaseResult {
	if workflow == nil || !workflow.HasCalledPlanner {
		return PhaseResult{Done: false}
	}
	if workflow.Plan.LatestStatus != pipeline.WorkflowApprovalRejected {
		return PhaseResult{Done: false}
	}
	return PhaseResult{
		Tools:            removeToolByName(filterTools(allTools, postplanTools), "question"),
		Messages:         msgs,
		RuntimeDirective: rejectionRuntimeDirective(),
		Done:             true,
		Name:             "postplan-rejected",
	}
}

func phasePostplanPending(allTools []provider.Tool, msgs []provider.Message, _ int, workflow *pipeline.WorkflowState) PhaseResult {
	if workflow == nil || !workflow.HasCalledPlanner {
		return PhaseResult{Done: false}
	}
	return PhaseResult{
		Tools:    filterTools(allTools, postplanTools),
		Messages: msgs,
		Done:     true,
		Name:     "postplan-pending",
	}
}

func approvalRuntimeDirective(answer string) string {
	directive := "Plan approved. Execute the full approved task list now. Set the first task to in_progress and begin working on it immediately. After setting it to in_progress, make the first concrete tool call for that task right away. Do not stop with prose before starting work. Continue through the remaining tasks in order without asking the user for confirmation between tasks unless blocked. Do not re-present the plan."
	switch normalizePlanApprovalAnswer(answer) {
	case planApprovalSaveOption:
		directive = "Plan approved. The user chose \"Save plan and proceed\". The approved markdown plan has already been saved under docs/kodacode/plans/. Do not rewrite the plan files. Execute the full approved task list now. Set the first task to in_progress and begin working on it immediately. After setting it to in_progress, make the first concrete tool call for that task right away. Do not stop with prose before starting work. Continue through the remaining tasks in order without asking the user for confirmation between tasks unless blocked. Do not re-present the plan."
	case planApprovalProceedOption:
		directive = "Plan approved. The user chose \"Proceed without saving plan files\". Skip saving the plan files and execute the full approved task list now. Set the first task to in_progress and begin working on it immediately. After setting it to in_progress, make the first concrete tool call for that task right away. Do not stop with prose before starting work. Continue through the remaining tasks in order without asking the user for confirmation between tasks unless blocked. Do not re-present the plan."
	}
	return directive
}

func rejectionRuntimeDirective() string {
	return "The plan was declined. Do NOT execute it. Do not create tasks, edit files, or implement anything. Do not call any more tools. Respond with a short message acknowledging the rejection and wait for the user's next message."
}

// ---------------------------------------------------------------------------
// Middleware entry point
// ---------------------------------------------------------------------------

// NewPhaseFilterMiddleware enforces an engineer-only prebuild verification gate
// and the later approval/execution gating. After prebuild succeeds and before
// planner approval exists, tools remain unfiltered and the controller or turn
// loop drives workflow progression.
//
// Disabled when plan_approval is false. Skips ephemeral sessions and agents
// without both subagent and question tools.
func NewPhaseFilterMiddleware(cfg *config.SessionConfig) pipeline.TurnMiddleware {
	return func(ctx context.Context, req *pipeline.TurnRequest, next pipeline.TurnHandler) error {
		if cfg.PlanApproval != nil && !*cfg.PlanApproval {
			return next(ctx, req)
		}
		if !isEngineerWorkflowAgent(req.AgentID) || req.Ephemeral || !hasTool(req.Tools, "subagent") || !hasTool(req.Tools, "question") {
			return next(ctx, req)
		}

		workflow := ensureWorkflowState(req)
		result := evalPhases(builderPhaseRules, req.Tools, req.Messages, req.Step, workflow)
		setPhaseRuntimeDirective(req, result.RuntimeDirective)
		if result.Name != "passthrough" {
			req.Workflow.Phase = pipeline.WorkflowPhase(result.Name)
		}
		if result.Name == "approved" {
			req.Messages = result.Messages
			return next(ctx, req)
		}

		req.FullTools = req.Tools
		req.PhaseFilterActive = true
		req.Tools = result.Tools
		req.Messages = result.Messages
		return next(ctx, req)
	}
}

// ---------------------------------------------------------------------------
// Mid-turn phase re-evaluation
// ---------------------------------------------------------------------------

// ApplyPhaseRules re-evaluates phase rules mid-turn after tool results.
// Called from the turn loop after each tool batch. Both this function and
// NewPhaseFilterMiddleware use the same evalPhases call with the same rules,
// eliminating duplicated phase logic.
func ApplyPhaseRules(req *pipeline.TurnRequest) {
	if !req.PhaseFilterActive {
		return
	}

	workflow := ensureWorkflowState(req)
	result := evalPhases(builderPhaseRules, req.FullTools, req.Messages, req.Step, workflow)
	setPhaseRuntimeDirective(req, result.RuntimeDirective)
	req.Tools = result.Tools
	req.Messages = result.Messages
	if result.Name != "passthrough" {
		req.Workflow.Phase = pipeline.WorkflowPhase(result.Name)
	}

	switch result.Name {
	case "approved":
		req.PhaseFilterActive = false
		log.Printf("phase: plan approved — restoring execution tools")
	case "postplan-rejected":
		req.PhaseFilterActive = false
		log.Printf("phase: plan declined — keeping execution tools blocked, question tool removed")
	}
}

// ---------------------------------------------------------------------------
// Plan approval state
// ---------------------------------------------------------------------------

type planApprovalDecision int

const (
	planApprovalPending planApprovalDecision = iota
	planApprovalApproved
	planApprovalRejected
)

func latestPlanApprovalDecision(msgs []provider.Message) planApprovalDecision {
	decision, _ := latestPlanApprovalState(msgs)
	return decision
}

type planOption struct {
	Label string `json:"label"`
	Role  string `json:"role"`
}

func isPlanApprovalQuestion(args string) bool {
	var parsed struct {
		Purpose string `json:"purpose"`
	}
	return json.Unmarshal([]byte(args), &parsed) == nil && parsed.Purpose == "plan_approval"
}

func parsePlanOptions(args string) []planOption {
	var parsed struct {
		Options json.RawMessage `json:"options"`
	}
	if json.Unmarshal([]byte(args), &parsed) != nil {
		return nil
	}
	var opts []planOption
	if json.Unmarshal(parsed.Options, &opts) == nil && len(opts) > 0 && opts[0].Label != "" {
		return opts
	}
	var strs []string
	if json.Unmarshal(parsed.Options, &strs) == nil {
		out := make([]planOption, len(strs))
		for i, s := range strs {
			out[i] = planOption{Label: s}
		}
		return out
	}
	return nil
}

// extractAnswer pulls the user's selected answer from the question tool
// output. The tool formats output as "question\n> answer", but we also
// handle raw answers (no prefix) for robustness.
func extractAnswer(output string) string {
	if _, after, ok := strings.Cut(output, "\n> "); ok {
		return strings.TrimSpace(after)
	}
	if after, ok := strings.CutPrefix(output, "> "); ok {
		return strings.TrimSpace(after)
	}
	return strings.TrimSpace(output)
}

func classifyPlanApprovalAnswer(output string, options []planOption) planApprovalDecision {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" || strings.Contains(strings.ToLower(trimmed), "cancelled the question without selecting an answer") {
		return planApprovalRejected
	}

	answer := strings.ToLower(normalizePlanApprovalAnswer(extractAnswer(trimmed)))

	for _, o := range options {
		if strings.ToLower(o.Label) == answer {
			if o.Role == "reject" {
				return planApprovalRejected
			}
			if o.Role == "approve" {
				return planApprovalApproved
			}
			if rejectionPattern.MatchString(answer) {
				return planApprovalRejected
			}
			return planApprovalApproved
		}
	}

	if rejectionPattern.MatchString(answer) {
		return planApprovalRejected
	}
	return planApprovalApproved
}

var rejectionPattern = regexp.MustCompile(`(?i)\b(cancel|reject|stop|decline|no)\b`)

// ---------------------------------------------------------------------------
// Message history helpers
// ---------------------------------------------------------------------------

// hasCalledAgent checks if a subagent tool call for a specific agent ID
// exists in the message history.
func hasCalledAgent(msgs []provider.Message, agentID string) bool {
	for _, m := range msgs {
		if m.Role != "assistant" {
			continue
		}
		for _, p := range m.Parts {
			tc, ok := p.(provider.ToolCallPart)
			if !ok || tc.Name != "subagent" {
				continue
			}
			if plannerAgentIDFromArgs(tc.Arguments) == agentID {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Tool list helpers
// ---------------------------------------------------------------------------

func hasTool(tools []provider.Tool, name string) bool {
	for _, t := range tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

func filterTools(tools []provider.Tool, allowed map[string]bool) []provider.Tool {
	filtered := make([]provider.Tool, 0, len(tools))
	for _, t := range tools {
		if allowed[t.Name] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

func mergeMaps(base, overlay map[string]bool) map[string]bool {
	m := make(map[string]bool, len(base)+len(overlay))
	maps.Copy(m, base)
	maps.Copy(m, overlay)
	return m
}

func removeToolByName(tools []provider.Tool, name string) []provider.Tool {
	filtered := make([]provider.Tool, 0, len(tools))
	for _, t := range tools {
		if t.Name != name {
			filtered = append(filtered, t)
		}
	}
	return filtered
}
