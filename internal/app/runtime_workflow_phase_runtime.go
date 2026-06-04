package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/tool"
	workflowpkg "github.com/sageil/kodacode/internal/workflow"
)

const (
	workflowApprovalAnswerApprove = "Approve"
	workflowApprovalAnswerStop    = "Stop"
)

const workflowRequiredOutputRecoveryObjective = "Record required workflow phase outputs."

func (r *Runtime) startWorkflowApprovalPhaseTurn(ctx context.Context, input runExistingTurnInput, workflowPhase workflowPhaseTurnContext) (RunSessionResult, error) {
	if strings.TrimSpace(input.UserText) != "" || len(input.ResolvedAttachments) > 0 {
		if err := r.Runner.appendUserMessage(ctx, input.SessionID, input.TurnID, input.UserText, cloneProviderAttachments(input.ResolvedAttachments)); err != nil {
			return RunSessionResult{}, err
		}
	}
	state, err := r.Sessions.Snapshot(ctx, input.SessionID)
	if err != nil {
		return RunSessionResult{}, err
	}
	if workflowApprovalSkipAllowed(state, workflowPhase.Phase) {
		phaseID := strings.TrimSpace(workflowPhase.Phase.ID)
		if err := r.RecordWorkflowEvidence(ctx, RecordWorkflowEvidenceInput{
			SessionID:  input.SessionID,
			TurnID:     input.TurnID,
			PhaseID:    phaseID,
			Type:       events.WorkflowEvidenceTypeApproval,
			Successful: boolPointer(true),
			Summary:    "workflow approval skipped by max_affected_files rule",
			Fields:     workflowApprovalEvidenceFields(state, phaseID),
		}); err != nil {
			return RunSessionResult{}, err
		}
		if err := r.advanceSkippedWorkflowApproval(ctx, input.SessionID, input.TurnID, phaseID); err != nil {
			return RunSessionResult{}, err
		}
		if err := appendTextToParentTurn(ctx, r.Sessions, input.SessionID, input.TurnID, "Workflow approval skipped: affected files are within the configured limit."); err != nil {
			return RunSessionResult{}, err
		}
		if err := r.Runner.appendTurnDone(ctx, input.SessionID, input.TurnID); err != nil {
			return RunSessionResult{}, err
		}
		return r.continueAfterWorkflowApproval(ctx, state, AnswerSessionQuestionInput{
			SessionID: input.SessionID,
			TurnID:    input.TurnID,
			UserText:  input.UserText,
			AgentID:   input.AgentID,
			SkillIDs:  append([]string(nil), input.SkillIDs...),
			Fragments: input.Fragments,
		}, input.TurnID, nil, phaseID)
	}
	question := strings.TrimSpace(workflowPhase.Phase.Prompt)
	if question == "" {
		question = "Approve this workflow phase?"
	}
	requestID, err := r.Sessions.RequestQuestion(ctx, QuestionRequestInput{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Question:  question,
		Options:   []string{workflowApprovalAnswerApprove, workflowApprovalAnswerStop},
		Purpose:   events.QuestionPurposeWorkflowApproval,
	})
	if err != nil {
		return RunSessionResult{}, err
	}
	return r.loadSessionTurnResult(ctx, input.SessionID, input.TurnID, RunTurnResult{
		Status:           TurnRunStatusPending,
		PendingRequestID: requestID,
	})
}

func (r *Runtime) advanceSkippedWorkflowApproval(ctx context.Context, sessionID, turnID, phaseID string) error {
	state, definition, workflow, err := r.activeWorkflowState(ctx, sessionID)
	if err != nil {
		return err
	}
	phaseID = strings.TrimSpace(phaseID)
	if workflow == nil || strings.TrimSpace(workflow.CurrentPhaseID) != phaseID {
		return ErrWorkflowTransitionInvalid
	}
	toPhaseID := ""
	if transition, ok := workflowTransitionFor(definition, phaseID, workflowpkg.TransitionOnSkipped); ok {
		toPhaseID = strings.TrimSpace(transition.To)
	}
	if toPhaseID == "" {
		return r.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
			SessionID:  sessionID,
			TurnID:     turnID,
			StopReason: "approval skipped by max_affected_files rule",
		})
	}
	if err := r.ensureWorkflowEvidenceAllowsAdvance(ctx, sessionID, turnID, state, definition, phaseID, toPhaseID); err != nil {
		return err
	}
	return r.appendWorkflowPhaseAdvanced(ctx, sessionID, turnID, workflow.WorkflowID, phaseID, toPhaseID, "approval skipped by max_affected_files rule")
}

func (r *Runtime) startWorkflowVerificationPhaseTurn(ctx context.Context, input runExistingTurnInput, workflowPhase workflowPhaseTurnContext) (RunSessionResult, error) {
	if strings.TrimSpace(input.UserText) != "" || len(input.ResolvedAttachments) > 0 {
		if err := r.Runner.appendUserMessage(ctx, input.SessionID, input.TurnID, input.UserText, cloneProviderAttachments(input.ResolvedAttachments)); err != nil {
			return RunSessionResult{}, err
		}
	}
	if r.Tools == nil {
		return RunSessionResult{}, errors.New("workflow verification requires tool executor")
	}
	commands := workflowVerificationCommandSpecs(workflowPhase.Phase.Commands)
	if len(commands) == 0 {
		return RunSessionResult{}, workflowpkg.ErrWorkflowPhaseRequired
	}

	lines := []string{"Workflow verification:"}
	allSuccessful := true
	for _, command := range commands {
		args, err := workflowVerificationToolArgs(command)
		if err != nil {
			return RunSessionResult{}, err
		}
		callID := newRuntimeID("workflow-verification")
		result, err := r.Tools.Execute(ctx, ExecuteToolInput{
			SessionID:    input.SessionID,
			TurnID:       input.TurnID,
			ToolCallID:   callID,
			ToolName:     command.ToolName,
			Arguments:    args,
			AllowedTools: []string{command.ToolName},
		})
		if err != nil {
			return RunSessionResult{}, err
		}
		if result.Status == ToolExecutionStatusPending {
			return r.loadSessionTurnResult(ctx, input.SessionID, input.TurnID, RunTurnResult{
				Status:           TurnRunStatusPending,
				PendingRequestID: result.PendingRequestID,
			})
		}
		successful := strings.TrimSpace(result.Error) == ""
		if err := r.recordWorkflowVerificationToolEvidenceIfMissing(ctx, input.SessionID, input.TurnID, workflowPhase.Phase.ID, callID, command.ToolName, command.Command, successful, workflowVerificationToolResultSummary(result)); err != nil {
			return RunSessionResult{}, err
		}
		if successful {
			lines = append(lines, "- passed: "+command.Display)
			continue
		}
		allSuccessful = false
		lines = append(lines, "- failed: "+command.Display)
		break
	}

	if err := appendTextToParentTurn(ctx, r.Sessions, input.SessionID, input.TurnID, strings.Join(lines, "\n")); err != nil {
		return RunSessionResult{}, err
	}
	if err := r.Runner.appendTurnDone(ctx, input.SessionID, input.TurnID); err != nil {
		return RunSessionResult{}, err
	}
	result, err := r.loadSessionTurnResult(ctx, input.SessionID, input.TurnID, RunTurnResult{Status: TurnRunStatusCompleted})
	if err != nil {
		return RunSessionResult{}, err
	}
	if !allSuccessful {
		if _, err := r.maybeReviseWorkflowAfterVerificationFailure(ctx, input.SessionID, input.TurnID); err != nil {
			return RunSessionResult{}, err
		}
		return result, nil
	}
	if err := r.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
	}); err != nil {
		if errors.Is(err, ErrWorkflowEvidenceMissing) {
			return result, nil
		}
		return RunSessionResult{}, err
	}
	if continued, ok, err := r.continueWorkflowIfRunnable(ctx, input.SessionID, input.TurnID, result); err != nil || ok {
		return continued, err
	}
	_, definition, _, err := r.activeWorkflowState(ctx, input.SessionID)
	if err != nil {
		if errors.Is(err, ErrWorkflowStateMissing) {
			return result, nil
		}
		return RunSessionResult{}, err
	}
	return r.completeFinalWorkflowPhaseIfReached(ctx, input.SessionID, input.TurnID, definition, result)
}

type workflowVerificationCommandSpec struct {
	ToolName string
	Command  string
	Display  string
}

func workflowVerificationCommandSpecs(values []workflowpkg.VerificationCommand) []workflowVerificationCommandSpec {
	out := make([]workflowVerificationCommandSpec, 0, len(values))
	for _, value := range values {
		spec := workflowVerificationCommandSpecFromDefinition(value)
		if spec.Command == "" {
			continue
		}
		out = append(out, spec)
	}
	return out
}

func workflowVerificationCommandSpecFromDefinition(value workflowpkg.VerificationCommand) workflowVerificationCommandSpec {
	toolName := strings.TrimSpace(value.Tool)
	command := strings.TrimSpace(value.Command)
	display := command
	if toolName != tool.TestToolName {
		display = toolName + ": " + command
	}
	return workflowVerificationCommandSpec{
		ToolName: toolName,
		Command:  command,
		Display:  display,
	}
}

func workflowVerificationCommandDisplays(values []workflowpkg.VerificationCommand) []string {
	specs := workflowVerificationCommandSpecs(values)
	out := make([]string, 0, len(specs))
	for _, spec := range specs {
		out = append(out, spec.Display)
	}
	return out
}

func workflowVerificationToolArgs(command workflowVerificationCommandSpec) (json.RawMessage, error) {
	var payload any
	switch command.ToolName {
	case tool.BashToolName:
		payload = struct {
			Command string `json:"cmd"`
		}{
			Command: strings.TrimSpace(command.Command),
		}
	default:
		payload = struct {
			Command string `json:"command"`
		}{
			Command: strings.TrimSpace(command.Command),
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

func (r *Runtime) recordWorkflowVerificationToolEvidenceIfMissing(ctx context.Context, sessionID, turnID, phaseID, toolCallID, toolName, command string, successful bool, summary string) error {
	state, err := r.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return err
	}
	if workflowHasToolCallEvidence(state.Workflow, phaseID, toolCallID, events.WorkflowEvidenceTypeVerificationResult) {
		return nil
	}
	return r.RecordWorkflowEvidence(ctx, RecordWorkflowEvidenceInput{
		SessionID:  sessionID,
		TurnID:     turnID,
		PhaseID:    phaseID,
		Type:       events.WorkflowEvidenceTypeVerificationResult,
		ToolCallID: toolCallID,
		Command:    command,
		Successful: &successful,
		Summary:    summary,
		Fields: map[string]string{
			"verification_tool": toolName,
		},
	})
}

func workflowHasToolCallEvidence(workflow *events.WorkflowState, phaseID, toolCallID, evidenceType string) bool {
	phaseID = strings.TrimSpace(phaseID)
	toolCallID = strings.TrimSpace(toolCallID)
	evidenceType = strings.TrimSpace(evidenceType)
	if workflow == nil || phaseID == "" || toolCallID == "" || evidenceType == "" {
		return false
	}
	return workflowHasEvidence(workflow, phaseID, func(evidence *events.WorkflowEvidenceState) bool {
		return strings.TrimSpace(evidence.Type) == evidenceType && strings.TrimSpace(evidence.ToolCallID) == toolCallID
	})
}

func workflowVerificationToolResultSummary(result ToolExecutionResult) string {
	if output := strings.TrimSpace(result.Output); output != "" {
		return truncateWorkflowEvidenceSummary(output)
	}
	if err := strings.TrimSpace(result.Error); err != "" {
		return truncateWorkflowEvidenceSummary(err)
	}
	return "verification completed"
}

func (r *Runtime) appendWorkflowPhaseTurnConfigured(ctx context.Context, input runExistingTurnInput, view turnStartSessionView, workflowPhase workflowPhaseTurnContext, effectiveAgentID, effectiveWorkflowID string, effectiveSkillIDs []string, effectiveThinkingEnabled bool, effectiveThinkingMode string, responseStyle ResponseStyle) error {
	if err := r.appendWorkflowPhaseStartedForTurn(ctx, input, workflowPhase); err != nil {
		return err
	}
	capabilities, err := r.resolveTurnCapabilitiesFromState(view.capabilitiesState(), resolveTurnCapabilitiesOptions{
		AgentID:              effectiveAgentID,
		SkillIDs:             append([]string(nil), effectiveSkillIDs...),
		ModelRouteOverride:   input.ModelRouteOverride,
		AllowedToolsOverride: nil,
		StrictModelRoute:     true,
	})
	if err != nil {
		return err
	}
	capabilities.AllowedTools = workflowPhaseAllowedTools(capabilities.AllowedTools, workflowPhase.Phase)
	effectiveThinkingMode = capabilities.EffectiveReasoningVariant(effectiveThinkingMode)
	effectiveThinkingEnabled = capabilities.EffectiveThinkingEnabled(effectiveThinkingEnabled)
	selectedSkillIDs := input.SelectedSkillIDs
	if selectedSkillIDs == nil {
		selectedSkillIDs = input.SkillIDs
	}
	turnConfig := newTurnConfiguredPayload(capabilities.TurnCapabilities, selectedSkillIDs, effectiveWorkflowID, input.PreserveSessionModel, effectiveThinkingEnabled, effectiveThinkingMode, responseStyle, input.HideAssistantPreview)
	turnConfig.WorkflowPhaseID = strings.TrimSpace(workflowPhase.Phase.ID)
	if err := r.Runner.appendTurnConfigured(ctx, input.SessionID, input.TurnID, turnConfig); err != nil {
		return err
	}
	if input.InitialState != nil {
		if err := r.Runner.appendTurnWorkStateUpdated(ctx, input.SessionID, input.TurnID, input.InitialState.WorkState); err != nil {
			return err
		}
	}
	if input.Continuation != nil {
		summary := turnWorkSummary{}
		if input.InitialState != nil {
			summary = input.InitialState.WorkState.Summary
		}
		if err := r.Runner.appendTurnContinuationStarted(ctx, input.SessionID, input.TurnID, input.Continuation.PreviousTurnID, input.Continuation.Reason, summary); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) appendWorkflowPhaseStartedForTurn(ctx context.Context, input runExistingTurnInput, workflowPhase workflowPhaseTurnContext) error {
	if !workflowPhase.Active || workflowPhase.PhaseStartRecordedThisTurn {
		return nil
	}
	return r.appendWorkflowPhaseStarted(ctx, input.SessionID, input.TurnID, workflowPhase.WorkflowID, workflowPhase.Phase.ID)
}

func (r *Runtime) answerWorkflowApproval(ctx context.Context, state events.SessionState, input AnswerSessionQuestionInput, turnID string, request *events.QuestionRequestState, answer string) (RunSessionResult, bool, error) {
	if request == nil || strings.TrimSpace(request.Purpose) != events.QuestionPurposeWorkflowApproval {
		return RunSessionResult{}, false, nil
	}
	if _, err := r.Sessions.AnswerQuestion(ctx, AnswerQuestionInput{
		SessionID: input.SessionID,
		TurnID:    turnID,
		RequestID: input.RequestID,
		Answer:    input.Answer,
	}); err != nil {
		return RunSessionResult{}, true, err
	}
	workflow := state.Workflow
	if workflow == nil || workflow.Status != events.WorkflowStatusActive {
		return RunSessionResult{}, true, ErrWorkflowStateMissing
	}
	phaseID := strings.TrimSpace(workflow.CurrentPhaseID)
	switch strings.TrimSpace(answer) {
	case workflowApprovalAnswerApprove:
		if err := r.RecordWorkflowEvidence(ctx, RecordWorkflowEvidenceInput{
			SessionID:  input.SessionID,
			TurnID:     turnID,
			PhaseID:    phaseID,
			Type:       events.WorkflowEvidenceTypeApproval,
			Successful: boolPointer(true),
			Summary:    "workflow phase approved",
			Fields:     workflowApprovalEvidenceFields(state, phaseID),
		}); err != nil {
			return RunSessionResult{}, true, err
		}
		if err := r.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
			SessionID: input.SessionID,
			TurnID:    turnID,
		}); err != nil {
			return RunSessionResult{}, true, err
		}
		if err := appendTextToParentTurn(ctx, r.Sessions, input.SessionID, turnID, "Workflow phase approved."); err != nil {
			return RunSessionResult{}, true, err
		}
		if err := r.Runner.appendTurnDone(ctx, input.SessionID, turnID); err != nil {
			return RunSessionResult{}, true, err
		}
		result, err := r.continueAfterWorkflowApproval(ctx, state, input, turnID, request, phaseID)
		return result, true, err
	case workflowApprovalAnswerStop:
		if err := r.BlockWorkflow(ctx, BlockWorkflowInput{
			SessionID:  input.SessionID,
			TurnID:     turnID,
			PhaseID:    phaseID,
			StopReason: "workflow approval stopped",
		}); err != nil {
			return RunSessionResult{}, true, err
		}
		if err := appendTextToParentTurn(ctx, r.Sessions, input.SessionID, turnID, "Workflow approval stopped."); err != nil {
			return RunSessionResult{}, true, err
		}
	default:
		return RunSessionResult{}, true, ErrQuestionAnswerInvalid
	}
	if err := r.Runner.appendTurnDone(ctx, input.SessionID, turnID); err != nil {
		return RunSessionResult{}, true, err
	}
	result, err := r.loadSessionTurnResult(ctx, input.SessionID, turnID, RunTurnResult{Status: TurnRunStatusCompleted})
	return result, true, err
}

func (r *Runtime) continueAfterWorkflowApproval(ctx context.Context, state events.SessionState, input AnswerSessionQuestionInput, turnID string, request *events.QuestionRequestState, approvedPhaseID string) (RunSessionResult, error) {
	updated, err := r.Sessions.Snapshot(ctx, input.SessionID)
	if err != nil {
		return RunSessionResult{}, err
	}
	workflow := updated.Workflow
	if workflow == nil || workflow.Status != events.WorkflowStatusActive {
		return r.loadSessionTurnResult(ctx, input.SessionID, turnID, RunTurnResult{Status: TurnRunStatusCompleted})
	}
	nextPhaseID := strings.TrimSpace(workflow.CurrentPhaseID)
	if nextPhaseID == "" || nextPhaseID == strings.TrimSpace(approvedPhaseID) {
		return r.loadSessionTurnResult(ctx, input.SessionID, turnID, RunTurnResult{Status: TurnRunStatusCompleted})
	}
	nextTurnID := newRuntimeID("turn")
	question := ""
	if request != nil {
		question = request.Question
	}
	result, err := r.runExistingSessionTurn(ctx, runExistingTurnInput{
		SessionID:            input.SessionID,
		TurnID:               nextTurnID,
		UserText:             "",
		AgentID:              workflowApprovalContinuationAgentID(state, input, turnID),
		WorkflowID:           workflow.WorkflowID,
		SkillIDs:             append([]string(nil), input.SkillIDs...),
		Fragments:            input.Fragments,
		PreserveSessionModel: true,
		InitialState:         workflowApprovalContinuationInitialState(state, workflow.WorkflowID, approvedPhaseID, nextPhaseID),
		Continuation: &runtimeTurnContinuation{
			PreviousTurnID: turnID,
			Reason:         events.TurnContinuationReasonQuestionAnswer,
			Question:       question,
		},
	})
	if err != nil {
		return RunSessionResult{}, err
	}
	return r.maybeAdvanceWorkflowAfterTurn(ctx, input.SessionID, nextTurnID, result)
}

func workflowApprovalSkipAllowed(state events.SessionState, phase workflowpkg.Phase) bool {
	limit := phase.SkipWhen.MaxAffectedFiles
	if limit <= 0 {
		return false
	}
	phaseID := strings.TrimSpace(phase.ID)
	if phaseID == "" || state.Workflow == nil {
		return false
	}
	previousPhaseID := ""
	for index, id := range state.Workflow.PhaseOrder {
		if strings.TrimSpace(id) == phaseID && index > 0 {
			previousPhaseID = strings.TrimSpace(state.Workflow.PhaseOrder[index-1])
			break
		}
	}
	if previousPhaseID == "" {
		return false
	}
	count, ok := workflowAffectedFilesOutputCount(state.Workflow, previousPhaseID)
	return ok && count <= limit
}

func workflowAffectedFilesOutputCount(workflow *events.WorkflowState, phaseID string) (int, bool) {
	if workflow == nil {
		return 0, false
	}
	for _, evidenceID := range workflow.EvidenceOrder {
		evidence := workflow.Evidence[evidenceID]
		if evidence == nil || strings.TrimSpace(evidence.PhaseID) != strings.TrimSpace(phaseID) {
			continue
		}
		if evidence.Type != events.WorkflowEvidenceTypePhaseOutput {
			continue
		}
		count, ok := workflowAffectedFilesValueCount(evidence.Fields["affected_files"])
		if ok {
			return count, true
		}
	}
	return 0, false
}

func workflowAffectedFilesValueCount(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	var array []any
	if err := json.Unmarshal([]byte(value), &array); err == nil {
		return len(array), true
	}
	var scalar string
	if err := json.Unmarshal([]byte(value), &scalar); err == nil {
		return workflowAffectedFilesTextCount(scalar)
	}
	return workflowAffectedFilesTextCount(value)
}

func workflowAffectedFilesTextCount(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	normalized := strings.ToLower(value)
	switch normalized {
	case "none", "none identified", "no files", "no files identified", "[]":
		return 0, true
	}
	lines := strings.Split(value, "\n")
	if len(lines) > 1 {
		count := 0
		for _, line := range lines {
			line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-"))
			if line != "" {
				count++
			}
		}
		return count, true
	}
	parts := strings.Split(value, ",")
	if len(parts) > 1 {
		count := 0
		for _, part := range parts {
			if strings.TrimSpace(part) != "" {
				count++
			}
		}
		return count, true
	}
	if strings.Contains(value, "/") || strings.Contains(value, "\\") || strings.Contains(value, ".") {
		return 1, true
	}
	return 0, false
}

func workflowApprovalContinuationAgentID(state events.SessionState, input AnswerSessionQuestionInput, turnID string) string {
	if agentID := strings.TrimSpace(input.AgentID); agentID != "" {
		return agentID
	}
	turn := state.Turns[strings.TrimSpace(turnID)]
	if turn == nil || turn.Config == nil {
		return ""
	}
	return strings.TrimSpace(turn.Config.AgentID)
}

func workflowApprovalContinuationInitialState(state events.SessionState, workflowID, approvedPhaseID, nextPhaseID string) *turnLoopState {
	workflowID = strings.TrimSpace(workflowID)
	approvedPhaseID = strings.TrimSpace(approvedPhaseID)
	nextPhaseID = strings.TrimSpace(nextPhaseID)
	objective := "Continue the approved workflow."
	if workflowID != "" {
		objective = "Continue workflow `" + workflowID + "` after approval."
	}
	decision := "User approved the workflow phase."
	if approvedPhaseID != "" {
		decision = "User approved workflow phase `" + approvedPhaseID + "`."
	}
	openItem := "Run the next workflow phase."
	if nextPhaseID != "" {
		openItem = "Run workflow phase `" + nextPhaseID + "`."
	}
	summary := turnWorkSummary{
		Objective: objective,
		Decisions: []string{
			decision,
		},
		OpenItems: []string{
			openItem,
		},
	}
	if workflow := state.Workflow; workflow != nil {
		for _, evidenceID := range workflow.EvidenceOrder {
			evidence := workflow.Evidence[evidenceID]
			if evidence == nil || strings.TrimSpace(evidence.PhaseID) != approvedPhaseID {
				continue
			}
			if summaryText := strings.TrimSpace(evidence.Summary); summaryText != "" {
				summary.CompletedWork = append(summary.CompletedWork, summaryText)
			}
		}
	}
	return &turnLoopState{
		LatestToolStepStart: -1,
		WorkState: turnWorkState{
			Summary: summary,
		},
	}
}

func workflowApprovalEvidenceFields(state events.SessionState, phaseID string) map[string]string {
	fields := map[string]string{}
	phaseID = strings.TrimSpace(phaseID)
	if workflow := state.Workflow; workflow != nil {
		for index, id := range workflow.PhaseOrder {
			if strings.TrimSpace(id) != phaseID || index == 0 {
				continue
			}
			previous := strings.TrimSpace(workflow.PhaseOrder[index-1])
			if previous != "" {
				fields["approved_phase"] = previous
			}
			break
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func (r *Runtime) completeWorkflowFinalPhaseTurn(ctx context.Context, input runExistingTurnInput, workflowPhase workflowPhaseTurnContext) (RunSessionResult, error) {
	if strings.TrimSpace(input.UserText) != "" || len(input.ResolvedAttachments) > 0 {
		if err := r.Runner.appendUserMessage(ctx, input.SessionID, input.TurnID, input.UserText, cloneProviderAttachments(input.ResolvedAttachments)); err != nil {
			return RunSessionResult{}, err
		}
	}
	if err := r.appendWorkflowFinalSummary(ctx, input.SessionID, input.TurnID, workflowPhase.WorkflowID, workflowPhase.Phase); err != nil {
		return RunSessionResult{}, err
	}
	if err := r.CompleteWorkflow(ctx, CompleteWorkflowInput{
		SessionID:  input.SessionID,
		TurnID:     input.TurnID,
		StopReason: "workflow completed",
	}); err != nil {
		return RunSessionResult{}, err
	}
	if err := r.Runner.appendTurnDone(ctx, input.SessionID, input.TurnID); err != nil {
		return RunSessionResult{}, err
	}
	return r.loadSessionTurnResult(ctx, input.SessionID, input.TurnID, RunTurnResult{Status: TurnRunStatusCompleted})
}

func (r *Runtime) maybeAdvanceWorkflowAfterTurn(ctx context.Context, sessionID, turnID string, result RunSessionResult) (RunSessionResult, error) {
	if result.Status != TurnRunStatusCompleted {
		if _, err := r.maybeApplyWorkflowTurnResultTransition(ctx, sessionID, turnID, result); err != nil {
			return RunSessionResult{}, err
		}
		return result, nil
	}
	state, definition, workflow, err := r.activeWorkflowState(ctx, sessionID)
	if err != nil {
		if errors.Is(err, ErrWorkflowStateMissing) {
			return result, nil
		}
		return RunSessionResult{}, err
	}
	if workflow.Status != events.WorkflowStatusActive {
		if workflow.Status == events.WorkflowStatusBlocked {
			if _, reviseErr := r.maybeReviseWorkflowAfterVerificationFailure(ctx, sessionID, turnID); reviseErr != nil {
				return RunSessionResult{}, reviseErr
			}
		}
		return result, nil
	}
	phaseID := strings.TrimSpace(workflow.CurrentPhaseID)
	phase, ok := workflowPhaseByID(definition, phaseID)
	if !ok {
		return RunSessionResult{}, ErrWorkflowTransitionInvalid
	}
	if workflowPhaseIsUserApproval(phase) {
		return result, nil
	}
	if workflowPhaseIsFinal(phase) {
		return r.completeWorkflowAfterModelTurn(ctx, sessionID, turnID, definition, phase, result)
	}
	if err := r.recordWorkflowTurnCompletionEvidence(ctx, state, sessionID, turnID, phase, result.AssistantText); err != nil {
		return RunSessionResult{}, err
	}
	if continued, ok, err := r.continueWorkflowRequiredOutputRecoveryIfNeeded(ctx, state, sessionID, turnID, workflow.WorkflowID, phase, result); err != nil || ok {
		return continued, err
	}
	if workflowPhaseIsReview(phase) {
		revised, err := r.maybeReviseWorkflowAfterReviewFailure(ctx, sessionID, turnID)
		if err != nil {
			return RunSessionResult{}, err
		}
		if revised {
			return result, nil
		}
	}
	if err := r.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    turnID,
	}); err != nil {
		if errors.Is(err, ErrWorkflowEvidenceMissing) {
			return result, nil
		}
		return RunSessionResult{}, err
	}
	if continued, ok, err := r.continueWorkflowIfRunnable(ctx, sessionID, turnID, result); err != nil || ok {
		return continued, err
	}
	return r.completeFinalWorkflowPhaseIfReached(ctx, sessionID, turnID, definition, result)
}

func (r *Runtime) continueWorkflowRequiredOutputRecoveryIfNeeded(ctx context.Context, state events.SessionState, sessionID, previousTurnID, workflowID string, phase workflowpkg.Phase, result RunSessionResult) (RunSessionResult, bool, error) {
	phaseID := strings.TrimSpace(phase.ID)
	missing := missingWorkflowPhaseOutputKeys(state.Workflow, phaseID, phase.RequiresOutput)
	if len(missing) == 0 {
		return result, false, nil
	}
	if workflowRequiredOutputRecoveryAttempted(state, previousTurnID) {
		return result, false, nil
	}
	nextTurnID := newRuntimeID("turn")
	initialState := workflowRequiredOutputRecoveryInitialState(state, previousTurnID, workflowID, phaseID, missing, phase.RequiresOutput)
	continued, err := r.runExistingSessionTurn(ctx, runExistingTurnInput{
		SessionID:            sessionID,
		TurnID:               nextTurnID,
		UserText:             "",
		AgentID:              workflowPhaseContinuationAgentID(phase),
		WorkflowID:           workflowID,
		SkillIDs:             workflowPhaseContinuationSkillIDs(state, previousTurnID),
		ThinkingEnabled:      workflowPhaseContinuationThinkingEnabled(state, previousTurnID),
		ThinkingMode:         workflowPhaseContinuationThinkingMode(state, previousTurnID),
		AllowedToolsOverride: []string{tool.WorkflowPhaseOutputToolName},
		PreserveSessionModel: true,
		InitialState:         initialState,
		AdditionalFragments: []prompt.Fragment{
			workflowRequiredOutputRecoveryPromptFragment(workflowID, phaseID, missing, phase.RequiresOutput),
		},
		Continuation: &runtimeTurnContinuation{
			PreviousTurnID: previousTurnID,
			Reason:         events.TurnContinuationReasonWorkflowPhase,
		},
	})
	if err != nil {
		return RunSessionResult{}, false, err
	}
	return continued, true, nil
}

func workflowRequiredOutputRecoveryAttempted(state events.SessionState, turnID string) bool {
	turn := state.Turns[strings.TrimSpace(turnID)]
	if turn == nil || turn.ContinuationStart == nil {
		return false
	}
	if strings.TrimSpace(turn.ContinuationStart.Reason) != events.TurnContinuationReasonWorkflowPhase {
		return false
	}
	return strings.TrimSpace(turn.ContinuationStart.Summary.Objective) == workflowRequiredOutputRecoveryObjective
}

func workflowRequiredOutputRecoveryInitialState(state events.SessionState, previousTurnID, workflowID, phaseID string, missingKeys, requiredKeys []string) *turnLoopState {
	turn := state.Turns[strings.TrimSpace(previousTurnID)]
	workState := turnWorkState{}
	if turn != nil && turn.WorkState != nil {
		workState = turnWorkStateFromEventState(turn.WorkState)
		workState.NativeContinuation = nil
	}
	workState.Summary.Objective = workflowRequiredOutputRecoveryObjective
	workState.Summary.OpenItems = appendUniqueValues(workState.Summary.OpenItems, []string{
		"Call " + tool.WorkflowPhaseOutputToolName + " for workflow `" + strings.TrimSpace(workflowID) + "` phase `" + strings.TrimSpace(phaseID) + "`.",
		"Record required fields: " + strings.Join(trimmedWorkflowValues(requiredKeys), ", ") + ".",
		"Missing fields: " + strings.Join(trimmedWorkflowValues(missingKeys), ", ") + ".",
	})
	return &turnLoopState{
		LatestToolStepStart: -1,
		WorkState:           workState,
	}
}

func workflowRequiredOutputRecoveryPromptFragment(workflowID, phaseID string, missingKeys, requiredKeys []string) prompt.Fragment {
	required := trimmedWorkflowValues(requiredKeys)
	missing := trimmedWorkflowValues(missingKeys)
	lines := []string{
		"Required workflow phase output recovery.",
		"- Workflow: " + strings.TrimSpace(workflowID),
		"- Phase: " + strings.TrimSpace(phaseID),
		"- Missing required phase outputs: " + strings.Join(missing, ", "),
		"- Call `" + tool.WorkflowPhaseOutputToolName + "` now with fields for every required key: " + strings.Join(required, ", ") + ".",
		"- Do not restate the plan, summarize, or answer in prose before calling `" + tool.WorkflowPhaseOutputToolName + "`.",
		"- Prose, markdown, or JSON in the assistant response does not satisfy required phase outputs.",
	}
	return prompt.Fragment{
		Kind:      prompt.KindRuntime,
		Source:    prompt.SourceRuntime,
		Stability: prompt.StabilityDynamic,
		Layer:     "workflow",
		Key:       "workflow-required-output-recovery",
		Label:     "workflow-output-recovery",
		Content:   strings.Join(lines, "\n"),
	}
}

func (r *Runtime) maybeApplyWorkflowTurnResultTransition(ctx context.Context, sessionID, turnID string, result RunSessionResult) (bool, error) {
	transitionEvents := workflowTransitionEventsForTurnResult(result)
	if len(transitionEvents) == 0 {
		return false, nil
	}
	_, definition, workflow, err := r.activeWorkflowState(ctx, sessionID)
	if err != nil {
		if errors.Is(err, ErrWorkflowStateMissing) {
			return false, nil
		}
		return false, err
	}
	if workflow.Status != events.WorkflowStatusActive {
		return false, nil
	}
	phaseID := strings.TrimSpace(workflow.CurrentPhaseID)
	for _, event := range transitionEvents {
		transition, ok := workflowTransitionFor(definition, phaseID, event)
		if !ok {
			continue
		}
		if transition.MaxLoops > 0 && workflowTurnResultTransitionEvidenceCount(workflow, phaseID, event) >= transition.MaxLoops {
			reason := fmt.Sprintf("%s transition loop limit reached (%d/%d)", event, transition.MaxLoops, transition.MaxLoops)
			if err := r.blockWorkflowPhase(ctx, sessionID, turnID, workflow.WorkflowID, phaseID, reason); err != nil {
				return false, err
			}
			return true, nil
		}
		if err := r.recordWorkflowTurnResultTransitionEvidence(ctx, sessionID, turnID, phaseID, event, result); err != nil {
			return false, err
		}
		reason := workflowTurnResultTransitionReason(event, result)
		if err := r.appendWorkflowPhaseAdvanced(ctx, sessionID, turnID, workflow.WorkflowID, phaseID, transition.To, reason); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func (r *Runtime) continueWorkflowIfRunnable(ctx context.Context, sessionID, previousTurnID string, result RunSessionResult) (RunSessionResult, bool, error) {
	state, definition, workflow, err := r.activeWorkflowState(ctx, sessionID)
	if err != nil {
		if errors.Is(err, ErrWorkflowStateMissing) {
			return result, false, nil
		}
		return RunSessionResult{}, false, err
	}
	if workflow == nil || workflow.Status != events.WorkflowStatusActive {
		return result, false, nil
	}
	phase, ok := workflowPhaseByID(definition, workflow.CurrentPhaseID)
	if !ok {
		return RunSessionResult{}, false, ErrWorkflowTransitionInvalid
	}
	if workflowPhaseIsUserApproval(phase) && workflowApprovalSkipAllowed(state, phase) {
		return result, false, nil
	}
	if !workflowPhaseRunnableAsContinuation(phase) {
		return result, false, nil
	}
	nextTurnID := newRuntimeID("turn")
	continued, err := r.runExistingSessionTurn(ctx, runExistingTurnInput{
		SessionID:            sessionID,
		TurnID:               nextTurnID,
		UserText:             "",
		AgentID:              workflowPhaseContinuationAgentID(phase),
		WorkflowID:           workflow.WorkflowID,
		SkillIDs:             workflowPhaseContinuationSkillIDs(state, previousTurnID),
		ThinkingEnabled:      workflowPhaseContinuationThinkingEnabled(state, previousTurnID),
		ThinkingMode:         workflowPhaseContinuationThinkingMode(state, previousTurnID),
		PreserveSessionModel: true,
		InitialState:         workflowPhaseContinuationInitialState(state, previousTurnID, workflow.WorkflowID, phase.ID),
		Continuation: &runtimeTurnContinuation{
			PreviousTurnID: previousTurnID,
			Reason:         events.TurnContinuationReasonWorkflowPhase,
		},
	})
	if err != nil {
		return RunSessionResult{}, false, err
	}
	return continued, true, nil
}

func workflowPhaseRunnableAsContinuation(phase workflowpkg.Phase) bool {
	if phase.AutoContinueDisabled() {
		return false
	}
	switch phase.EffectiveType() {
	case workflowpkg.PhaseTypeAgent, workflowpkg.PhaseTypeReview, workflowpkg.PhaseTypeUserApproval, workflowpkg.PhaseTypeVerification:
		return true
	default:
		return false
	}
}

func workflowPhaseContinuationAgentID(phase workflowpkg.Phase) string {
	return strings.TrimSpace(workflowPhaseAgentID(phase))
}

func workflowPhaseContinuationSkillIDs(state events.SessionState, previousTurnID string) []string {
	turn := state.Turns[strings.TrimSpace(previousTurnID)]
	if turn == nil || turn.Config == nil {
		return nil
	}
	return append([]string(nil), turn.Config.SkillIDs...)
}

func workflowPhaseContinuationThinkingEnabled(state events.SessionState, previousTurnID string) bool {
	turn := state.Turns[strings.TrimSpace(previousTurnID)]
	return turn != nil && turn.Config != nil && turn.Config.ThinkingEnabled
}

func workflowPhaseContinuationThinkingMode(state events.SessionState, previousTurnID string) string {
	turn := state.Turns[strings.TrimSpace(previousTurnID)]
	if turn == nil || turn.Config == nil {
		return ""
	}
	return strings.TrimSpace(turn.Config.ThinkingMode)
}

func workflowPhaseContinuationInitialState(state events.SessionState, previousTurnID, workflowID, phaseID string) *turnLoopState {
	turn := state.Turns[strings.TrimSpace(previousTurnID)]
	workState := turnWorkState{}
	if turn != nil && turn.WorkState != nil {
		workState = turnWorkStateFromEventState(turn.WorkState)
		workState.NativeContinuation = nil
	}
	if turnWorkSummaryEmpty(workState.Summary) {
		workState.Summary = turnWorkSummary{
			Objective: "Continue workflow `" + strings.TrimSpace(workflowID) + "`.",
		}
	}
	if phaseID = strings.TrimSpace(phaseID); phaseID != "" {
		workState.Summary.OpenItems = appendUniqueValues(workState.Summary.OpenItems, []string{"Run workflow phase `" + phaseID + "`."})
	}
	return &turnLoopState{
		LatestToolStepStart: -1,
		WorkState:           workState,
	}
}

func workflowTurnResultTransitionEvidenceCount(workflow *events.WorkflowState, phaseID, transitionEvent string) int {
	if workflow == nil {
		return 0
	}
	phaseID = strings.TrimSpace(phaseID)
	transitionEvent = strings.TrimSpace(transitionEvent)
	count := 0
	for _, evidenceID := range workflow.EvidenceOrder {
		evidence := workflow.Evidence[evidenceID]
		if evidence == nil || evidence.Type != events.WorkflowEvidenceTypePhaseFailure {
			continue
		}
		if strings.TrimSpace(evidence.PhaseID) != phaseID {
			continue
		}
		if strings.TrimSpace(evidence.Fields["transition_event"]) != transitionEvent {
			continue
		}
		count++
	}
	return count
}

func workflowTransitionEventsForTurnResult(result RunSessionResult) []string {
	switch result.Status {
	case TurnRunStatusCanceled:
		return []string{workflowpkg.TransitionOnCanceled}
	case TurnRunStatusFailed:
		candidates := []string{}
		switch result.ErrorCode {
		case events.TurnFailureCodeBudgetExceeded:
			candidates = append(candidates, workflowpkg.TransitionOnBudgetExceeded)
		case events.TurnFailureCodeProviderRequestLimit:
			candidates = append(candidates, workflowpkg.TransitionOnProviderRequestLimit)
		case events.TurnFailureCodeNoProgress:
			candidates = append(candidates, workflowpkg.TransitionOnNoProgress)
		}
		candidates = append(candidates, workflowpkg.TransitionOnTurnFailed)
		return candidates
	default:
		return nil
	}
}

func (r *Runtime) recordWorkflowTurnResultTransitionEvidence(ctx context.Context, sessionID, turnID, phaseID, transitionEvent string, result RunSessionResult) error {
	state, err := r.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return err
	}
	if workflowHasEvidence(state.Workflow, phaseID, func(evidence *events.WorkflowEvidenceState) bool {
		return evidence.Type == events.WorkflowEvidenceTypePhaseFailure &&
			strings.TrimSpace(evidence.Fields["transition_event"]) == strings.TrimSpace(transitionEvent) &&
			strings.TrimSpace(evidence.Fields["turn_id"]) == strings.TrimSpace(turnID)
	}) {
		return nil
	}
	successful := false
	return r.RecordWorkflowEvidence(ctx, RecordWorkflowEvidenceInput{
		SessionID:  sessionID,
		TurnID:     turnID,
		PhaseID:    phaseID,
		Type:       events.WorkflowEvidenceTypePhaseFailure,
		Successful: &successful,
		Summary:    workflowTurnResultTransitionReason(transitionEvent, result),
		Fields: map[string]string{
			"transition_event": strings.TrimSpace(transitionEvent),
			"turn_id":          strings.TrimSpace(turnID),
			"turn_status":      string(result.Status),
			"failure_code":     string(result.ErrorCode),
		},
	})
}

func workflowTurnResultTransitionReason(transitionEvent string, result RunSessionResult) string {
	reason := strings.TrimSpace(result.Error)
	if reason == "" {
		switch result.Status {
		case TurnRunStatusCanceled:
			reason = "workflow phase turn canceled"
		case TurnRunStatusFailed:
			reason = "workflow phase turn failed"
		default:
			reason = "workflow phase transition"
		}
	}
	transitionEvent = strings.TrimSpace(transitionEvent)
	if transitionEvent == "" {
		return reason
	}
	return transitionEvent + ": " + reason
}

func (r *Runtime) recordWorkflowTurnCompletionEvidence(ctx context.Context, state events.SessionState, sessionID, turnID string, phase workflowpkg.Phase, assistantText string) error {
	phaseID := strings.TrimSpace(phase.ID)
	assistantText = strings.TrimSpace(assistantText)
	if workflowPhaseIsReview(phase) && assistantText != "" && !workflowHasAnyEvidenceType(state.Workflow, phaseID, events.WorkflowEvidenceTypeReviewOutcome, events.WorkflowEvidenceTypeReview, events.WorkflowEvidenceTypeTaskReview) {
		review, err := parseStructuredManualReview(assistantText, "Workflow review")
		if err != nil {
			return nil
		}
		successful := review.OverallCorrectness == events.ReviewOverallCorrectnessCorrect
		if err := r.RecordWorkflowEvidence(ctx, RecordWorkflowEvidenceInput{
			SessionID:  sessionID,
			TurnID:     turnID,
			PhaseID:    phaseID,
			Type:       events.WorkflowEvidenceTypeReviewOutcome,
			Successful: &successful,
			Summary:    truncateWorkflowEvidenceSummary(review.OverallSummary),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) completeFinalWorkflowPhaseIfReached(ctx context.Context, sessionID, turnID string, definition workflowpkg.Definition, result RunSessionResult) (RunSessionResult, error) {
	state, err := r.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return RunSessionResult{}, err
	}
	workflow := state.Workflow
	if workflow == nil || workflow.Status != events.WorkflowStatusActive {
		return result, nil
	}
	phaseID := strings.TrimSpace(workflow.CurrentPhaseID)
	if !isFinalWorkflowPhase(definition, phaseID) {
		return result, nil
	}
	phase, ok := workflowPhaseByID(definition, phaseID)
	if !ok {
		return RunSessionResult{}, ErrWorkflowTransitionInvalid
	}
	return r.completeWorkflowAfterModelTurn(ctx, sessionID, turnID, definition, phase, result)
}

func (r *Runtime) completeWorkflowAfterModelTurn(ctx context.Context, sessionID, turnID string, definition workflowpkg.Definition, phase workflowpkg.Phase, result RunSessionResult) (RunSessionResult, error) {
	if err := r.appendWorkflowFinalSummary(ctx, sessionID, turnID, definition.ID, phase); err != nil {
		return RunSessionResult{}, err
	}
	if err := r.CompleteWorkflow(ctx, CompleteWorkflowInput{
		SessionID:  sessionID,
		TurnID:     turnID,
		StopReason: "workflow completed",
	}); err != nil {
		if errors.Is(err, ErrWorkflowEvidenceMissing) {
			return result, nil
		}
		return RunSessionResult{}, err
	}
	return r.loadSessionTurnResult(ctx, sessionID, turnID, RunTurnResult{Status: TurnRunStatusCompleted})
}

func (r *Runtime) appendWorkflowFinalSummary(ctx context.Context, sessionID, turnID, workflowID string, phase workflowpkg.Phase) error {
	state, err := r.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return err
	}
	text, _ := workflowFinalAssistantText(workflowID, phase, state.Workflow)
	return appendTextToParentTurn(ctx, r.Sessions, sessionID, turnID, text)
}

func workflowFinalAssistantText(workflowID string, phase workflowpkg.Phase, workflow *events.WorkflowState) (string, map[string]string) {
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" {
		workflowID = "workflow"
	}
	lines := []string{fmt.Sprintf("Workflow `%s` completed.", workflowID)}
	fields := map[string]string{}
	missing := []string{}
	include := trimmedWorkflowValues(phase.Include)
	for _, key := range include {
		value := workflowFinalIncludedValue(workflow, key)
		if value == "" {
			missing = append(missing, key)
			continue
		}
		fields[key] = value
	}
	if len(fields) == 0 && len(include) == 0 {
		evidence := workflowFinalEvidenceSummary(workflow)
		if evidence != "" {
			fields["evidence"] = evidence
		}
	}
	if len(fields) > 0 {
		lines = append(lines, "", "Summary:")
		for _, key := range include {
			value := fields[key]
			if value == "" {
				continue
			}
			lines = append(lines, "- "+key+": "+value)
		}
		if len(include) == 0 {
			lines = append(lines, "- evidence: "+fields["evidence"])
		}
	}
	if len(missing) > 0 {
		lines = append(lines, "", "Not recorded:")
		for _, key := range missing {
			lines = append(lines, "- "+key)
		}
	}
	if len(fields) == 0 && len(missing) == 0 {
		lines = append(lines, "", "No workflow evidence was recorded for the final summary.")
	}
	if len(fields) == 0 {
		fields = nil
	}
	return strings.Join(lines, "\n"), fields
}

func workflowFinalIncludedValue(workflow *events.WorkflowState, key string) string {
	key = strings.TrimSpace(key)
	if workflow == nil || key == "" {
		return ""
	}
	if value := workflowFinalLatestField(workflow, key); value != "" {
		return value
	}
	switch key {
	case "changed_files":
		return workflowFinalLatestEvidenceSummary(workflow, events.WorkflowEvidenceTypeGitDiff)
	case "verification_result":
		return workflowFinalLatestVerificationSummary(workflow)
	case "review_outcome", "findings":
		return workflowFinalReviewSummary(workflow)
	case "evidence":
		return workflowFinalEvidenceSummary(workflow)
	default:
		return ""
	}
}

func workflowFinalLatestField(workflow *events.WorkflowState, key string) string {
	for i := len(workflow.EvidenceOrder) - 1; i >= 0; i-- {
		evidence := workflow.Evidence[workflow.EvidenceOrder[i]]
		if evidence == nil {
			continue
		}
		if value := workflowFinalDisplayValue(evidence.Fields[key]); value != "" {
			return value
		}
	}
	return ""
}

func workflowFinalLatestEvidenceSummary(workflow *events.WorkflowState, types ...string) string {
	for i := len(workflow.EvidenceOrder) - 1; i >= 0; i-- {
		evidence := workflow.Evidence[workflow.EvidenceOrder[i]]
		if evidence == nil || !containsTrimmed(types, strings.TrimSpace(evidence.Type)) {
			continue
		}
		if summary := workflowFinalDisplayValue(evidence.Summary); summary != "" {
			return summary
		}
	}
	return ""
}

func workflowFinalLatestVerificationSummary(workflow *events.WorkflowState) string {
	for i := len(workflow.EvidenceOrder) - 1; i >= 0; i-- {
		evidence := workflow.Evidence[workflow.EvidenceOrder[i]]
		if evidence == nil || evidence.Type != events.WorkflowEvidenceTypeVerificationResult {
			continue
		}
		status := "completed"
		if evidence.Successful != nil {
			if *evidence.Successful {
				status = "passed"
			} else {
				status = "failed"
			}
		}
		command := strings.TrimSpace(evidence.Command)
		summary := workflowFinalDisplayValue(evidence.Summary)
		prefix := status
		if command != "" {
			prefix += ": " + command
		}
		if summary != "" {
			return prefix + " - " + summary
		}
		return prefix
	}
	return ""
}

func workflowFinalReviewSummary(workflow *events.WorkflowState) string {
	entries := workflowFinalReviewEntries(workflow)
	if len(entries) == 0 {
		return ""
	}
	if len(entries) == 1 {
		return entries[0].Summary
	}
	counts := map[string]int{}
	details := make([]string, 0, len(entries))
	for _, entry := range entries {
		status := entry.Status
		if status == "" {
			status = "recorded"
		}
		counts[status]++
		detail := status
		if entry.PassID != "" {
			detail += " " + entry.PassID
		}
		if entry.Summary != "" {
			detail += " - " + entry.Summary
		}
		details = append(details, detail)
	}
	return fmt.Sprintf("%d review outcomes: %s. %s", len(entries), workflowFinalReviewCountSummary(counts), strings.Join(details, "; "))
}

type workflowFinalReviewEntry struct {
	Status  string
	PassID  string
	Summary string
}

func workflowFinalReviewEntries(workflow *events.WorkflowState) []workflowFinalReviewEntry {
	if workflow == nil {
		return nil
	}
	entries := []workflowFinalReviewEntry{}
	for _, evidenceID := range workflow.EvidenceOrder {
		evidence := workflow.Evidence[evidenceID]
		if evidence == nil {
			continue
		}
		switch evidence.Type {
		case events.WorkflowEvidenceTypeReviewOutcome, events.WorkflowEvidenceTypeReview, events.WorkflowEvidenceTypeTaskReview:
		default:
			continue
		}
		summary := workflowFinalDisplayValue(evidence.Summary)
		if summary == "" {
			continue
		}
		entries = append(entries, workflowFinalReviewEntry{
			Status:  workflowFinalReviewStatus(evidence),
			PassID:  workflowFinalDisplayValue(evidence.Fields["review_pass"]),
			Summary: summary,
		})
	}
	if len(entries) == 0 {
		return nil
	}
	return entries
}

func workflowFinalReviewStatus(evidence *events.WorkflowEvidenceState) string {
	if evidence == nil {
		return ""
	}
	if status := workflowFinalDisplayValue(evidence.Fields["review_status"]); status != "" {
		return status
	}
	if evidence.Successful == nil {
		return "recorded"
	}
	if *evidence.Successful {
		return "pass"
	}
	return "fail"
}

func workflowFinalReviewCountSummary(counts map[string]int) string {
	order := []string{"pass", "accepted", "concern", "fail", "recorded"}
	parts := []string{}
	used := map[string]struct{}{}
	for _, status := range order {
		count := counts[status]
		if count == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%d %s", count, status))
		used[status] = struct{}{}
	}
	for status, count := range counts {
		if _, ok := used[status]; ok || count == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%d %s", count, status))
	}
	return strings.Join(parts, ", ")
}

func workflowFinalEvidenceSummary(workflow *events.WorkflowState) string {
	if workflow == nil || len(workflow.EvidenceOrder) == 0 {
		return ""
	}
	parts := []string{}
	for _, evidenceID := range workflow.EvidenceOrder {
		evidence := workflow.Evidence[evidenceID]
		if evidence == nil {
			continue
		}
		summary := workflowFinalDisplayValue(evidence.Summary)
		if summary == "" {
			summary = strings.TrimSpace(evidence.Type)
		}
		if summary == "" {
			continue
		}
		phaseID := strings.TrimSpace(evidence.PhaseID)
		label := strings.TrimSpace(evidence.Type)
		if phaseID != "" && label != "" {
			parts = append(parts, phaseID+" "+label+": "+summary)
		} else {
			parts = append(parts, summary)
		}
	}
	return strings.Join(parts, "; ")
}

func workflowFinalDisplayValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var stringValues []string
	if err := json.Unmarshal([]byte(value), &stringValues); err == nil {
		values := trimmedWorkflowValues(stringValues)
		if len(values) > 0 {
			return strings.Join(values, ", ")
		}
	}
	return truncateWorkflowEvidenceSummary(value)
}

func boolPointer(value bool) *bool {
	return &value
}
