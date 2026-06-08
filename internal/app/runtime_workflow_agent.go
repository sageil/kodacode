package app

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/tool"
	workflowpkg "github.com/sageil/kodacode/internal/workflow"
)

type workflowPhaseTurnContext struct {
	Active                     bool
	WorkflowID                 string
	Definition                 workflowpkg.Definition
	Phase                      workflowpkg.Phase
	PhaseStartRecordedThisTurn bool
	ResumedFromBlock           bool
	BlockedStopReason          string
	RevisionTrigger            *events.WorkflowEvidenceState
}

func (r *Runtime) prepareWorkflowPhaseTurn(ctx context.Context, input runExistingTurnInput, view turnStartSessionView) (turnStartSessionView, workflowPhaseTurnContext, string, error) {
	workflowID := strings.TrimSpace(input.WorkflowID)
	workflow := view.workflow
	phaseStartRecordedThisTurn := false
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
			phaseStartRecordedThisTurn = true
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
		blockedStopReason := strings.TrimSpace(workflow.StopReason)
		if strings.TrimSpace(input.UserText) == "" && input.Continuation == nil {
			return view, workflowPhaseTurnContext{}, workflow.WorkflowID, ErrWorkflowPhaseBlocked
		}
		if err := r.ResumeWorkflow(ctx, ResumeWorkflowInput{
			SessionID: input.SessionID,
			TurnID:    input.TurnID,
			PhaseID:   workflow.CurrentPhaseID,
		}); err != nil {
			return view, workflowPhaseTurnContext{}, workflow.WorkflowID, err
		}
		reloaded, err := r.loadTurnStartSessionView(ctx, input.SessionID)
		if err != nil {
			return view, workflowPhaseTurnContext{}, workflow.WorkflowID, err
		}
		view = reloaded
		workflow = view.workflow
		if workflow == nil {
			return view, workflowPhaseTurnContext{}, workflowID, ErrWorkflowStateMissing
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
			Active:                     true,
			WorkflowID:                 workflow.WorkflowID,
			Definition:                 definition,
			Phase:                      phase,
			PhaseStartRecordedThisTurn: false,
			ResumedFromBlock:           true,
			BlockedStopReason:          blockedStopReason,
			RevisionTrigger:            workflowLatestRevisionTriggerForPhase(workflow, phase.ID),
		}, workflow.WorkflowID, nil
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
		Active:                     true,
		WorkflowID:                 workflow.WorkflowID,
		Definition:                 definition,
		Phase:                      phase,
		PhaseStartRecordedThisTurn: phaseStartRecordedThisTurn,
		RevisionTrigger:            workflowLatestRevisionTriggerForPhase(workflow, phase.ID),
	}, workflow.WorkflowID, nil
}

func workflowPhaseAgentID(phase workflowpkg.Phase) string {
	if agentID := strings.TrimSpace(phase.Agent); agentID != "" {
		return agentID
	}
	return ""
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
	} else if containsTrimmed(base, tool.TaskWorkflowToolName) && !containsTrimmed(phase.Tools.Deny, tool.TaskWorkflowToolName) {
		allowed = appendToolIfMissing(allowed, tool.TaskWorkflowToolName)
	}
	return allowWorkflowRuntimeOwnedTools(allowed, workflowRuntimeToolScope{Phase: &phase})
}

type workflowRuntimeToolScope struct {
	Phase *workflowpkg.Phase
}

// Workflow runtime-owned tools are saved workflow result channels, not
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
		"mkdir",
		tool.RenameSymbolToolName,
		tool.TaskWorkflowToolName,
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
	if ctx.RevisionTrigger != nil {
		lines = append(lines, workflowRevisionPromptLines(ctx.RevisionTrigger)...)
	}
	if ctx.ResumedFromBlock {
		lines = append(lines, "- Workflow recovery: this phase was blocked and has been resumed for this turn.")
		if reason := strings.TrimSpace(ctx.BlockedStopReason); reason != "" {
			lines = append(lines, "- Previous block reason: "+reason)
		}
		lines = append(lines, "- Address the blocking condition before attempting to advance the workflow.")
	}
	if text := strings.TrimSpace(ctx.Phase.Prompt); text != "" {
		lines = append(lines, "- Phase prompt: "+text)
	}
	if len(ctx.Phase.RequiresOutput) > 0 {
		required := trimmedWorkflowValues(ctx.Phase.RequiresOutput)
		lines = append(lines, "- Required phase outputs: "+strings.Join(required, ", "))
		lines = append(lines, "- You MUST record the required phase outputs before any final prose. Call `"+tool.WorkflowPhaseOutputToolName+"` with every required output key.")
		lines = append(lines, "- If a required output has no findings, still record that key with a short value such as `None identified`.")
		lines = append(lines, "- If you skip `"+tool.WorkflowPhaseOutputToolName+"`, the workflow phase will block instead of advancing.")
		lines = append(lines, "- The final response may be human-readable after that tool call. Prose, markdown, or JSON in the assistant response does not satisfy required phase outputs.")
		if containsTrimmed(required, "implementation_tasks") {
			lines = append(lines, "- `implementation_tasks` must list concrete implementation tasks that can each be represented by workflow tasks during implementation.")
		}
		if containsTrimmed(required, "acceptance_criteria") {
			lines = append(lines, "- `acceptance_criteria` must list the behavior or quality checks that define done for the approved plan.")
		}
		if containsTrimmed(required, "verification_plan") {
			lines = append(lines, "- `verification_plan` must say how acceptance criteria should be verified, including tests or manual checks.")
		}
	}
	if required := workflowPhaseCompletionRequirementLabels(ctx.Phase); len(required) > 0 {
		lines = append(lines, "- Phase completion requirements: "+strings.Join(required, ", "))
	}
	if len(ctx.Phase.Commands) > 0 {
		lines = append(lines, "- Declared verification commands: "+strings.Join(workflowVerificationCommandDisplays(ctx.Phase.Commands), " | "))
	}
	if workflowPhaseIsVerification(ctx.Phase) && len(ctx.Phase.Commands) == 0 {
		lines = append(lines,
			"- This verification phase is evidence-only. Do not implement fixes, edit files, or patch code in this phase.",
			"- Determine and run appropriate verification for this project. Prefer existing project scripts, tasks, or documented commands over framework guesses.",
			"- Verify against the approved `acceptance_criteria` and `verification_plan`, not only whether one command passed.",
			"- If any criterion is unverified, deferred, or failed, record it in `unverified_criteria`, `deferred_items`, or `failures` and stop. The workflow revision transition will return to an implementation phase.",
			"- Record commands run, result, criteria checked, unverified criteria, deferred items, failures, and confidence with `"+tool.WorkflowPhaseOutputToolName+"` before final prose.",
		)
	}
	if strings.TrimSpace(ctx.Phase.ID) == "implement" {
		if ctx.RevisionTrigger != nil {
			lines = append(lines, "- This is a revision loop. Use the revision target above as the runnable work item; an active task is not required before revising.")
			lines = append(lines, "- Use `"+tool.TaskWorkflowToolName+"` only when useful for tracking substantial multi-step revision work. If you create a revision task, complete it before finishing the phase.")
		} else {
			lines = append(lines, "- Create workflow tasks for every approved `implementation_tasks` item before implementation work. The phase cannot advance until those planned tasks are complete.")
		}
	}
	if len(ctx.Phase.Include) > 0 {
		lines = append(lines, "- Final summary should include: "+strings.Join(trimmedWorkflowValues(ctx.Phase.Include), ", "))
	}
	if len(ctx.Phase.ReviewPasses) > 0 {
		lines = append(lines, workflowReviewPassInstructionLines(ctx.Phase.ReviewPasses)...)
	}
	if workflowPhaseIsReadFocused(ctx.Phase) {
		lines = append(lines, "- This phase is read-focused. Do not perform workspace mutations.")
	}
	if workflowPhaseIsVerification(ctx.Phase) && len(ctx.Phase.Commands) > 0 {
		lines = append(lines, "- Run only declared verification commands for workflow verification evidence.")
	}
	if workflowPhaseIsVerification(ctx.Phase) {
		lines = append(lines, "- Verification does not own implementation. Do not call mutation tools or attempt repairs during this phase.")
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

func workflowRevisionPromptLines(trigger *events.WorkflowEvidenceState) []string {
	if trigger == nil {
		return nil
	}
	fields := trigger.Fields
	lines := []string{"Workflow revision target is active."}
	if event := strings.TrimSpace(fields["revision_event"]); event != "" {
		lines = append(lines, "- Revision event: "+event)
	}
	if summary := strings.TrimSpace(trigger.Summary); summary != "" {
		lines = append(lines, "- Revision summary: "+summary)
	} else if summary := strings.TrimSpace(fields["source_summary"]); summary != "" {
		lines = append(lines, "- Revision summary: "+summary)
	}
	if sourceType := strings.TrimSpace(fields["source_evidence_type"]); sourceType != "" {
		lines = append(lines, "- Source evidence type: "+sourceType)
	}
	if sourcePhase := strings.TrimSpace(fields["source_phase_id"]); sourcePhase != "" {
		lines = append(lines, "- Source phase: "+sourcePhase)
	}
	if failedCheck := strings.TrimSpace(fields["failed_check"]); failedCheck != "" {
		lines = append(lines, "- Failed check: "+failedCheck)
	}
	if reviewPass := strings.TrimSpace(fields["review_pass"]); reviewPass != "" {
		lines = append(lines, "- Review pass: "+reviewPass)
	}
	if reviewStatus := strings.TrimSpace(fields["review_status"]); reviewStatus != "" {
		lines = append(lines, "- Review status: "+reviewStatus)
	}
	for i := 1; ; i++ {
		prefix := "finding_" + strconv.Itoa(i) + "_"
		title := strings.TrimSpace(fields[prefix+"title"])
		path := strings.TrimSpace(fields[prefix+"path"])
		explanation := strings.TrimSpace(fields[prefix+"explanation"])
		if title == "" && path == "" && explanation == "" {
			break
		}
		line := "- Review finding " + strconv.Itoa(i) + ":"
		if title != "" {
			line += " " + title
		}
		if path != "" {
			line += " (" + path
			if lineNumber := strings.TrimSpace(fields[prefix+"line"]); lineNumber != "" {
				line += ":" + lineNumber
			}
			line += ")"
		}
		if explanation != "" {
			line += " - " + explanation
		}
		lines = append(lines, line)
	}
	return lines
}

func workflowReviewPassInstructionLines(passes []workflowpkg.ReviewPass) []string {
	lines := []string{
		"Workflow review phase instructions:",
		"Complete each required review pass below as a separate lens. Follow its instructions, inspect only what is needed, then record exactly one `" + tool.WorkflowReviewResultToolName + "` result for that pass.",
		"",
		"Required review passes:",
	}
	number := 1
	for _, pass := range passes {
		id := strings.TrimSpace(pass.ID)
		if id == "" {
			continue
		}
		lines = append(lines, "")
		lines = append(lines, strconv.Itoa(number)+". `"+id+"`")
		number++
		if description := strings.TrimSpace(pass.Description); description != "" {
			lines = append(lines, "   Goal: "+description)
		}
		instructions := trimmedWorkflowValues(pass.Instructions)
		if len(instructions) == 0 {
			lines = append(lines, "   Instructions: None provided; use the goal above.")
			continue
		}
		lines = append(lines, "   Instructions:")
		for _, instruction := range instructions {
			lines = append(lines, "   - "+instruction)
		}
	}
	lines = append(lines,
		"",
		"For each pass:",
		"- Call `"+tool.WorkflowReviewResultToolName+"` exactly once.",
		"- Set `review_pass` to the pass id.",
		"- Use `findings: []` if no issue was found for that pass.",
		"- Set `overall_correctness` to \"correct\" only with no material issue; otherwise \"incorrect\".",
		"- Summarize what you checked in `overall_summary`.",
		"",
		"Do not skip passes, combine passes, or mutate workspace files.",
	)
	return lines
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
