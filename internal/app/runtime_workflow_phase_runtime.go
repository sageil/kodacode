package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
	workflowpkg "github.com/sageil/kodacode/internal/workflow"
)

const (
	workflowApprovalAnswerApprove = "Approve"
	workflowApprovalAnswerStop    = "Stop"
)

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
	commands := trimmedWorkflowValues(workflowPhase.Phase.Commands)
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
			ToolName:     tool.TestToolName,
			Arguments:    args,
			AllowedTools: []string{tool.TestToolName},
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
		if err := r.recordWorkflowVerificationToolEvidenceIfMissing(ctx, input.SessionID, input.TurnID, workflowPhase.Phase.ID, callID, command, successful, workflowVerificationToolResultSummary(result)); err != nil {
			return RunSessionResult{}, err
		}
		if successful {
			lines = append(lines, "- passed: "+command)
			continue
		}
		allSuccessful = false
		lines = append(lines, "- failed: "+command)
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
	_, definition, _, err := r.activeWorkflowState(ctx, input.SessionID)
	if err != nil {
		if errors.Is(err, ErrWorkflowStateMissing) {
			return result, nil
		}
		return RunSessionResult{}, err
	}
	return r.completeFinalWorkflowPhaseIfReached(ctx, input.SessionID, input.TurnID, definition, result)
}

func workflowVerificationToolArgs(command string) (json.RawMessage, error) {
	encoded, err := json.Marshal(struct {
		Command string `json:"command"`
	}{
		Command: strings.TrimSpace(command),
	})
	if err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

func (r *Runtime) recordWorkflowVerificationToolEvidenceIfMissing(ctx context.Context, sessionID, turnID, phaseID, toolCallID, command string, successful bool, summary string) error {
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
	return r.Runner.appendTurnConfigured(ctx, input.SessionID, input.TurnID, turnConfig)
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
	return r.runExistingSessionTurn(ctx, runExistingTurnInput{
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
	return r.completeFinalWorkflowPhaseIfReached(ctx, sessionID, turnID, definition, result)
}

func (r *Runtime) recordWorkflowTurnCompletionEvidence(ctx context.Context, state events.SessionState, sessionID, turnID string, phase workflowpkg.Phase, assistantText string) error {
	phaseID := strings.TrimSpace(phase.ID)
	assistantText = strings.TrimSpace(assistantText)
	if len(phase.RequiresOutput) > 0 && assistantText != "" {
		parsedFields := workflowPhaseOutputFields(assistantText, phase.RequiresOutput)
		fields := make(map[string]string, len(parsedFields))
		for _, key := range phase.RequiresOutput {
			key = strings.TrimSpace(key)
			if key == "" || workflowHasPhaseOutputEvidence(state.Workflow, phaseID, key) {
				continue
			}
			if value := strings.TrimSpace(parsedFields[key]); value != "" {
				fields[key] = value
			}
		}
		if len(fields) > 0 {
			if err := r.RecordWorkflowEvidence(ctx, RecordWorkflowEvidenceInput{
				SessionID: sessionID,
				TurnID:    turnID,
				PhaseID:   phaseID,
				Type:      events.WorkflowEvidenceTypePhaseOutput,
				Summary:   truncateWorkflowEvidenceSummary(assistantText),
				Fields:    fields,
			}); err != nil {
				return err
			}
		}
	}
	if workflowPhaseIsReview(phase) && assistantText != "" && !workflowHasAnyEvidenceType(state.Workflow, phaseID, events.WorkflowEvidenceTypeReviewOutcome, events.WorkflowEvidenceTypeReview, events.WorkflowEvidenceTypeTaskReview) {
		if err := r.RecordWorkflowEvidence(ctx, RecordWorkflowEvidenceInput{
			SessionID:  sessionID,
			TurnID:     turnID,
			PhaseID:    phaseID,
			Type:       events.WorkflowEvidenceTypeReviewOutcome,
			Successful: boolPointer(true),
			Summary:    truncateWorkflowEvidenceSummary(assistantText),
		}); err != nil {
			return err
		}
	}
	return nil
}

func workflowPhaseOutputFields(assistantText string, requiredKeys []string) map[string]string {
	assistantText = strings.TrimSpace(assistantText)
	if assistantText == "" || len(requiredKeys) == 0 {
		return nil
	}
	body, ok := workflowStructuredJSONBody(assistantText)
	if !ok {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return nil
	}
	out := make(map[string]string, len(requiredKeys))
	for _, key := range requiredKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		value, ok := payload[key]
		if !ok {
			continue
		}
		if formatted := workflowPhaseOutputValue(value); formatted != "" {
			out[key] = formatted
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func workflowStructuredJSONBody(text string) (string, bool) {
	if body, ok, err := unwrapStructuredReviewFence(text); err == nil && ok {
		return body, true
	}
	if body, err := extractSingleBalancedJSONObject(text); err == nil {
		return body, true
	}
	return "", false
}

func workflowPhaseOutputValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []any, map[string]any:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(encoded))
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
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
	text, fields := workflowFinalAssistantText(workflowID, phase, state.Workflow)
	if err := appendTextToParentTurn(ctx, r.Sessions, sessionID, turnID, text); err != nil {
		return err
	}
	if workflowHasAnyEvidenceType(state.Workflow, strings.TrimSpace(phase.ID), events.WorkflowEvidenceTypePhaseOutput) {
		return nil
	}
	return r.RecordWorkflowEvidence(ctx, RecordWorkflowEvidenceInput{
		SessionID: sessionID,
		TurnID:    turnID,
		PhaseID:   phase.ID,
		Type:      events.WorkflowEvidenceTypePhaseOutput,
		Summary:   truncateWorkflowEvidenceSummary(text),
		Fields:    fields,
	})
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
		return workflowFinalLatestEvidenceSummary(workflow, events.WorkflowEvidenceTypeReviewOutcome, events.WorkflowEvidenceTypeReview, events.WorkflowEvidenceTypeTaskReview)
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
