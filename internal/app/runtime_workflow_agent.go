package app

import (
	"context"
	"slices"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/tool"
	workflowpkg "github.com/sageil/kodacode/internal/workflow"
)

type workflowPhaseTurnContext struct {
	Active     bool
	WorkflowID string
	Definition workflowpkg.Definition
	Phase      workflowpkg.Phase
}

func (r *Runtime) prepareWorkflowPhaseTurn(ctx context.Context, input runExistingTurnInput, view turnStartSessionView) (turnStartSessionView, workflowPhaseTurnContext, string, error) {
	workflowID := strings.TrimSpace(input.WorkflowID)
	workflow := view.workflow
	if workflowID != "" {
		if workflow != nil && workflow.Status != events.WorkflowStatusCompleted && strings.TrimSpace(workflow.WorkflowID) != workflowID {
			return view, workflowPhaseTurnContext{}, workflowID, ErrWorkflowAlreadyActive
		}
		if workflow == nil || workflow.Status == events.WorkflowStatusCompleted {
			if err := r.StartWorkflow(ctx, StartWorkflowInput{
				SessionID:     input.SessionID,
				TurnID:        input.TurnID,
				WorkspaceRoot: view.workspaceRoot,
				WorkflowID:    workflowID,
			}); err != nil {
				return view, workflowPhaseTurnContext{}, workflowID, err
			}
			reloaded, err := r.loadTurnStartSessionView(ctx, input.SessionID)
			if err != nil {
				return view, workflowPhaseTurnContext{}, workflowID, err
			}
			view = reloaded
			workflow = view.workflow
		}
	}
	if workflow == nil || strings.TrimSpace(workflow.WorkflowID) == "" || workflow.Status == events.WorkflowStatusCompleted {
		return view, workflowPhaseTurnContext{}, workflowID, nil
	}
	if workflow.Status == events.WorkflowStatusBlocked {
		return view, workflowPhaseTurnContext{}, workflow.WorkflowID, ErrWorkflowPhaseBlocked
	}
	definition, err := r.resolveWorkflow(ctx, view.workspaceRoot, workflow.WorkflowID)
	if err != nil {
		return view, workflowPhaseTurnContext{}, workflow.WorkflowID, err
	}
	phase, ok := workflowPhaseByID(definition, workflow.CurrentPhaseID)
	if !ok {
		return view, workflowPhaseTurnContext{}, workflow.WorkflowID, ErrWorkflowTransitionInvalid
	}
	return view, workflowPhaseTurnContext{
		Active:     true,
		WorkflowID: workflow.WorkflowID,
		Definition: definition,
		Phase:      phase,
	}, workflow.WorkflowID, nil
}

func workflowPhaseAgentID(inputAgentID string, phase workflowpkg.Phase) string {
	if agentID := strings.TrimSpace(phase.Agent); agentID != "" {
		return agentID
	}
	if phase.EffectiveType() == workflowpkg.PhaseTypeVerification {
		return "engineer"
	}
	if phase.EffectiveType() == workflowpkg.PhaseTypeReview {
		return reviewerAgentID
	}
	return strings.TrimSpace(inputAgentID)
}

func workflowPhaseAllowedTools(base []string, phase workflowpkg.Phase) []string {
	if workflowPhaseIsUserApproval(phase) || workflowPhaseIsFinal(phase) {
		return nil
	}
	allowed := slices.Clone(base)
	if phase.Tools.Allow != nil {
		allowed = intersectToolSurface(allowed, phase.Tools.Allow)
	}
	if len(phase.Tools.Deny) > 0 {
		allowed = subtractToolSurface(allowed, phase.Tools.Deny)
	}
	if phase.EffectiveType() == workflowpkg.PhaseTypeVerification && phase.Tools.Allow == nil && len(phase.Commands) > 0 {
		allowed = intersectToolSurface(allowed, []string{tool.TestToolName, tool.BashToolName})
	}
	if workflowPhaseIsReadFocused(phase) {
		allowed = removeWorkflowMutationTools(allowed)
	}
	return allowWorkflowRuntimeOwnedTools(allowed, workflowRuntimeToolScope{Phase: &phase})
}

type workflowRuntimeToolScope struct {
	Phase         *workflowpkg.Phase
	ChildAgentID  string
	DelegatedTask string
}

// Workflow runtime-owned tools are durable workflow result channels, not
// ordinary agent capabilities. Agent and phase allow/deny policy filters the
// normal tool surface first; this pass then restores only the tools the active
// workflow context requires the runtime to receive.
func allowWorkflowRuntimeOwnedTools(allowed []string, scope workflowRuntimeToolScope) []string {
	for _, name := range workflowRuntimeOwnedTools(scope) {
		allowed = appendToolIfMissing(allowed, name)
	}
	return allowed
}

func workflowRuntimeOwnedTools(scope workflowRuntimeToolScope) []string {
	var out []string
	if scope.Phase != nil {
		if len(trimmedWorkflowValues(scope.Phase.RequiresOutput)) > 0 {
			out = appendToolIfMissing(out, tool.WorkflowPhaseOutputToolName)
		}
		if workflowPhaseIsReview(*scope.Phase) {
			out = appendToolIfMissing(out, tool.WorkflowReviewResultToolName)
		}
	}
	if workflowReviewPassIDFromTask(scope.DelegatedTask) != "" {
		out = appendToolIfMissing(out, tool.WorkflowReviewResultToolName)
	}
	return out
}

func workflowPhaseIsReadFocused(phase workflowpkg.Phase) bool {
	return phase.Mode == workflowpkg.PhaseModeReadOnly || phase.EffectiveType() == workflowpkg.PhaseTypeReview || strings.TrimSpace(phase.Agent) == reviewerAgentID
}

func workflowPhaseIsUserApproval(phase workflowpkg.Phase) bool {
	return phase.EffectiveType() == workflowpkg.PhaseTypeUserApproval
}

func workflowPhaseIsVerification(phase workflowpkg.Phase) bool {
	return phase.EffectiveType() == workflowpkg.PhaseTypeVerification || phase.Required
}

func workflowPhaseSupportsDeterministicVerification(phase workflowpkg.Phase) bool {
	if len(phase.Commands) == 0 {
		return false
	}
	if phase.Tools.Allow == nil {
		return true
	}
	for _, command := range phase.Commands {
		if !containsTrimmed(phase.Tools.Allow, command.Tool) {
			return false
		}
	}
	return true
}

func workflowPhaseIsFinal(phase workflowpkg.Phase) bool {
	return phase.EffectiveType() == workflowpkg.PhaseTypeFinal
}

func workflowPhaseIsReview(phase workflowpkg.Phase) bool {
	return phase.EffectiveType() == workflowpkg.PhaseTypeReview
}

func removeWorkflowMutationTools(tools []string) []string {
	if len(tools) == 0 {
		return nil
	}
	out := tools[:0]
	for _, name := range tools {
		if workflowMutationToolName(name) {
			continue
		}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func workflowMutationToolName(name string) bool {
	switch strings.TrimSpace(name) {
	case tool.ApplyPatchToolName,
		tool.BashToolName,
		tool.CodeActionToolName,
		tool.DelegateToolName,
		"mkdir",
		tool.RenameSymbolToolName,
		tool.TaskWorkflowToolName,
		tool.TestToolName,
		tool.WriteToolName:
		return true
	default:
		return false
	}
}

func workflowPhasePromptFragment(ctx workflowPhaseTurnContext, allowedTools []string) prompt.Fragment {
	phaseID := strings.TrimSpace(ctx.Phase.ID)
	lines := []string{
		"Workflow phase is active.",
		"- Workflow: " + strings.TrimSpace(ctx.WorkflowID),
		"- Phase: " + phaseID,
	}
	if agentID := strings.TrimSpace(ctx.Phase.Agent); agentID != "" {
		lines = append(lines, "- Phase agent: "+agentID)
	}
	if ctx.Phase.EffectiveType() != "" {
		lines = append(lines, "- Phase type: "+string(ctx.Phase.EffectiveType()))
	}
	if text := strings.TrimSpace(ctx.Phase.Prompt); text != "" {
		lines = append(lines, "- Phase prompt: "+text)
	}
	if len(ctx.Phase.RequiresOutput) > 0 {
		required := trimmedWorkflowValues(ctx.Phase.RequiresOutput)
		lines = append(lines, "- Required phase outputs: "+strings.Join(required, ", "))
		lines = append(lines, "- Before your final response, call `"+tool.WorkflowPhaseOutputToolName+"` with all required output keys.")
		lines = append(lines, "- The final response may be human-readable after that tool call. If the tool is unavailable, return exactly one JSON object containing the required output keys and no markdown fences.")
	}
	if required := workflowPhaseCompletionRequirementLabels(ctx.Phase); len(required) > 0 {
		lines = append(lines, "- Phase completion requirements: "+strings.Join(required, ", "))
	}
	if len(ctx.Phase.Commands) > 0 {
		lines = append(lines, "- Declared verification commands: "+strings.Join(workflowVerificationCommandDisplays(ctx.Phase.Commands), " | "))
	}
	if len(ctx.Phase.Include) > 0 {
		lines = append(lines, "- Final summary should include: "+strings.Join(trimmedWorkflowValues(ctx.Phase.Include), ", "))
	}
	if len(ctx.Phase.ReviewPasses) > 0 {
		lines = append(lines, "- Review passes:")
		for _, pass := range ctx.Phase.ReviewPasses {
			id := strings.TrimSpace(pass.ID)
			if id == "" {
				continue
			}
			line := "  - " + id
			if description := strings.TrimSpace(pass.Description); description != "" {
				line += ": " + description
			}
			lines = append(lines, line)
		}
		lines = append(lines, "- Record review outcomes for the relevant pass or passes.")
	}
	if workflowPhaseIsReadFocused(ctx.Phase) {
		lines = append(lines, "- This phase is read-focused. Do not perform workspace mutations.")
	}
	if workflowPhaseIsVerification(ctx.Phase) && len(ctx.Phase.Commands) > 0 {
		lines = append(lines, "- Run only declared verification commands for workflow verification evidence.")
	}
	if len(allowedTools) > 0 {
		lines = append(lines, "- Allowed tools for this phase: "+strings.Join(allowedTools, ", "))
	} else {
		lines = append(lines, "- No tools are available for this phase.")
	}
	return prompt.Fragment{
		Kind:      prompt.KindRuntime,
		Source:    prompt.SourceRuntime,
		Stability: prompt.StabilityDynamic,
		Layer:     "workflow",
		Key:       "workflow-phase",
		Label:     "workflow-phase",
		Content:   strings.Join(lines, "\n"),
	}
}

func trimmedWorkflowValues(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func workflowPhaseCompletionRequirementLabels(phase workflowpkg.Phase) []string {
	items := trimmedWorkflowValues(phase.Completion.Requires.Items)
	out := make([]string, 0, len(items))
	for _, item := range items {
		switch item {
		case workflowpkg.CompletionRequirementActivePhaseTasksComplete:
			out = append(out, "complete all tasks created for this workflow phase before finishing the phase")
		default:
			out = append(out, item)
		}
	}
	return out
}

func appendToolIfMissing(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if strings.TrimSpace(existing) == value {
			return values
		}
	}
	return append(values, value)
}

func cloneWorkflowStateForRuntime(state *events.WorkflowState) *events.WorkflowState {
	if state == nil {
		return nil
	}
	out := &events.WorkflowState{
		WorkflowID:        state.WorkflowID,
		Status:            state.Status,
		CurrentPhaseID:    state.CurrentPhaseID,
		PhaseOrder:        slices.Clone(state.PhaseOrder),
		Phases:            make(map[string]*events.WorkflowPhaseState, len(state.Phases)),
		EvidenceOrder:     slices.Clone(state.EvidenceOrder),
		Evidence:          make(map[string]*events.WorkflowEvidenceState, len(state.Evidence)),
		CompletedPhaseIDs: slices.Clone(state.CompletedPhaseIDs),
		BlockedPhaseIDs:   slices.Clone(state.BlockedPhaseIDs),
		StopReason:        state.StopReason,
		StartedAtSeq:      state.StartedAtSeq,
		UpdatedAtSeq:      state.UpdatedAtSeq,
		CompletedAtSeq:    state.CompletedAtSeq,
	}
	for id, phase := range state.Phases {
		if phase == nil {
			continue
		}
		copyPhase := *phase
		copyPhase.EvidenceIDs = slices.Clone(phase.EvidenceIDs)
		out.Phases[id] = &copyPhase
	}
	for id, evidence := range state.Evidence {
		if evidence == nil {
			continue
		}
		copyEvidence := *evidence
		copyEvidence.Fields = cloneEvidenceFields(evidence.Fields)
		copyEvidence.ExitCode = cloneInt(evidence.ExitCode)
		copyEvidence.Successful = cloneBool(evidence.Successful)
		out.Evidence[id] = &copyEvidence
	}
	return out
}
