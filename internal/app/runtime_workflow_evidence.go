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

var ErrWorkflowEvidenceMissing = errors.New("workflow evidence is missing")

type RecordWorkflowEvidenceInput struct {
	SessionID   string
	TurnID      string
	PhaseID     string
	Type        string
	ArtifactID  string
	ToolCallID  string
	ExecutionID string
	TaskID      string
	ReviewID    string
	Command     string
	ExitCode    *int
	Successful  *bool
	Summary     string
	Fields      map[string]string
}

func (r *Runtime) RecordWorkflowEvidence(ctx context.Context, input RecordWorkflowEvidenceInput) error {
	_, definition, workflow, err := r.activeWorkflowState(ctx, input.SessionID)
	if err != nil {
		return err
	}
	phaseID := strings.TrimSpace(input.PhaseID)
	if phaseID == "" {
		phaseID = strings.TrimSpace(workflow.CurrentPhaseID)
	}
	if !workflowDefinitionHasPhase(definition, phaseID) {
		return ErrWorkflowTransitionInvalid
	}
	evidenceType := strings.TrimSpace(input.Type)
	if evidenceType == "" {
		return errors.New("workflow evidence type is required")
	}
	evidenceID := newRuntimeID("workflow-evidence")
	if _, err := r.Sessions.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    workflowEventTurnID(input.TurnID),
		Type:      events.TypeWorkflowEvidenceRecorded,
		Payload: events.WorkflowEvidenceRecordedPayload{
			EvidenceID:  evidenceID,
			WorkflowID:  workflow.WorkflowID,
			PhaseID:     phaseID,
			Type:        evidenceType,
			ArtifactID:  strings.TrimSpace(input.ArtifactID),
			ToolCallID:  strings.TrimSpace(input.ToolCallID),
			ExecutionID: strings.TrimSpace(input.ExecutionID),
			TaskID:      strings.TrimSpace(input.TaskID),
			ReviewID:    strings.TrimSpace(input.ReviewID),
			Command:     strings.TrimSpace(input.Command),
			ExitCode:    cloneInt(input.ExitCode),
			Successful:  cloneBool(input.Successful),
			Summary:     strings.TrimSpace(input.Summary),
			Fields:      cloneEvidenceFields(input.Fields),
		},
	}); err != nil {
		return err
	}
	if evidenceType == events.WorkflowEvidenceTypeVerificationResult && input.Successful != nil && !*input.Successful {
		reason := strings.TrimSpace(input.Summary)
		if reason == "" {
			reason = "verification failed"
		}
		return r.blockWorkflowPhase(ctx, input.SessionID, input.TurnID, workflow.WorkflowID, phaseID, reason)
	}
	return nil
}

func (r *Runtime) ensureWorkflowEvidenceAllowsAdvance(ctx context.Context, sessionID, turnID string, state events.SessionState, definition workflowpkg.Definition, fromPhaseID, toPhaseID string) error {
	reason := missingWorkflowEvidenceReason(state, definition, fromPhaseID, toPhaseID)
	if reason == "" {
		return nil
	}
	workflow := state.Workflow
	if workflow == nil {
		return ErrWorkflowStateMissing
	}
	if err := r.blockWorkflowPhase(ctx, sessionID, turnID, workflow.WorkflowID, fromPhaseID, reason); err != nil {
		return err
	}
	return fmt.Errorf("%w: %s", ErrWorkflowEvidenceMissing, reason)
}

func (r *Runtime) blockWorkflowPhase(ctx context.Context, sessionID, turnID, workflowID, phaseID, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "workflow phase blocked"
	}
	_, err := r.Sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    workflowEventTurnID(turnID),
		Type:      events.TypeWorkflowPhaseBlocked,
		Payload: events.WorkflowPhaseBlockedPayload{
			WorkflowID: strings.TrimSpace(workflowID),
			PhaseID:    strings.TrimSpace(phaseID),
			StopReason: reason,
		},
	})
	return err
}

func missingWorkflowEvidenceReason(state events.SessionState, definition workflowpkg.Definition, fromPhaseID, toPhaseID string) string {
	fromPhase, ok := workflowPhaseByID(definition, fromPhaseID)
	if !ok {
		return "current workflow phase is not defined"
	}
	if reason := missingWorkflowPhaseCompletionEvidence(state, fromPhase); reason != "" {
		return reason
	}
	toPhase, ok := workflowPhaseByID(definition, toPhaseID)
	if !ok {
		return "next workflow phase is not defined"
	}
	return missingWorkflowPhaseEntryEvidence(state, toPhase)
}

func missingWorkflowPhaseCompletionEvidence(state events.SessionState, phase workflowpkg.Phase) string {
	phaseID := strings.TrimSpace(phase.ID)
	if missing := missingWorkflowPhaseOutputKeys(state.Workflow, phaseID, phase.RequiresOutput); len(missing) > 0 {
		return workflowMissingPhaseOutputReason(missing, phase.RequiresOutput)
	}
	if reason := missingWorkflowPhaseCompletionRequirement(state, phase, phase.Completion.Requires); reason != "" {
		return reason
	}
	switch {
	case phase.EffectiveType() == workflowpkg.PhaseTypeUserApproval:
		if !workflowHasApprovalEvidence(state.Workflow, phaseID) {
			return "missing approval evidence"
		}
	case phase.EffectiveType() == workflowpkg.PhaseTypeVerification || phase.Required:
		if !workflowHasSuccessfulEvidence(state.Workflow, phaseID, events.WorkflowEvidenceTypeVerificationResult) {
			return "missing successful verification evidence"
		}
	case workflowPhaseIsReview(phase):
		if !workflowHasAnyEvidenceType(state.Workflow, phaseID, events.WorkflowEvidenceTypeReviewOutcome, events.WorkflowEvidenceTypeReview, events.WorkflowEvidenceTypeTaskReview) {
			return "missing review evidence"
		}
	}
	return ""
}

func missingWorkflowPhaseOutputKeys(workflow *events.WorkflowState, phaseID string, requiredKeys []string) []string {
	phaseID = strings.TrimSpace(phaseID)
	var missing []string
	for _, key := range requiredKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if !workflowHasPhaseOutputEvidence(workflow, phaseID, key) {
			missing = append(missing, key)
		}
	}
	return missing
}

func workflowMissingPhaseOutputReason(missingKeys, requiredKeys []string) string {
	missing := trimmedWorkflowValues(missingKeys)
	if len(missing) == 0 {
		return "missing required phase output"
	}
	required := trimmedWorkflowValues(requiredKeys)
	if len(required) == 0 {
		required = missing
	}
	return "missing required phase output: " + missing[0] + ". The phase cannot advance from prose. Call " + tool.WorkflowPhaseOutputToolName + " with all required fields: " + strings.Join(required, ", ") + "."
}

func missingWorkflowPhaseCompletionRequirement(state events.SessionState, phase workflowpkg.Phase, requirements workflowpkg.EvidenceRequirements) string {
	phaseID := strings.TrimSpace(phase.ID)
	for _, item := range requirements.Items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		switch item {
		case workflowpkg.CompletionRequirementActivePhaseTasksComplete:
			if reason := missingActiveWorkflowPhaseTaskCompletion(state, phaseID); reason != "" {
				return reason
			}
		case workflowpkg.CompletionRequirementPlannedTasksComplete:
			if workflowLatestRevisionTriggerForPhase(state.Workflow, phaseID) != nil {
				continue
			}
			if reason := missingPlannedWorkflowPhaseTaskCompletion(state, phase); reason != "" {
				return reason
			}
		default:
			if !workflowHasAnyEvidenceType(state.Workflow, phaseID, item) {
				return "missing required completion evidence: " + item
			}
		}
	}
	for key, value := range requirements.Fields {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			continue
		}
		if !workflowHasPhaseFieldEvidence(state.Workflow, phaseID, key, value) {
			return "missing required completion evidence: " + key
		}
	}
	return ""
}

func missingPlannedWorkflowPhaseTaskCompletion(state events.SessionState, phase workflowpkg.Phase) string {
	if state.Workflow == nil {
		return ""
	}
	planPhaseID := strings.TrimSpace(phase.Requires.Fields["approved_phase"])
	if planPhaseID == "" {
		planPhaseID = "plan"
	}
	fields := workflowLatestPhaseOutputFields(state.Workflow, planPhaseID)
	plannedTasks := workflowStructuredListField(fields, "implementation_tasks")
	if len(plannedTasks) == 0 {
		return "approved plan has no implementation_tasks"
	}
	phaseID := strings.TrimSpace(phase.ID)
	workflowID := strings.TrimSpace(state.Workflow.WorkflowID)
	for _, planned := range plannedTasks {
		if planned == "" {
			continue
		}
		if !workflowHasCompletedMatchingTask(state, workflowID, phaseID, planned) {
			return "planned implementation task is not complete: " + planned
		}
	}
	return ""
}

func workflowHasCompletedMatchingTask(state events.SessionState, workflowID, phaseID, planned string) bool {
	needle := workflowTaskMatchText(planned)
	if needle == "" {
		return true
	}
	for _, taskID := range state.TaskOrder {
		task := state.Tasks[strings.TrimSpace(taskID)]
		if task == nil {
			continue
		}
		if strings.TrimSpace(task.WorkflowID) != workflowID || strings.TrimSpace(task.WorkflowPhaseID) != phaseID {
			continue
		}
		if strings.TrimSpace(task.Status) != events.TaskStatusCompleted {
			continue
		}
		haystack := workflowTaskMatchText(strings.Join([]string{task.Title, task.Notes, task.Progress}, " "))
		if strings.Contains(haystack, needle) || strings.Contains(needle, haystack) || workflowAllTokensContained(haystack, needle) {
			return true
		}
	}
	return false
}

func workflowTaskMatchText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Trim(value, "-*[]()0123456789. ")
	value = strings.Join(strings.Fields(value), " ")
	return value
}

func workflowAllTokensContained(haystack, needle string) bool {
	tokens := strings.Fields(needle)
	if len(tokens) == 0 {
		return true
	}
	for _, token := range tokens {
		if !strings.Contains(haystack, token) {
			return false
		}
	}
	return true
}

func workflowStructuredListField(fields map[string]string, key string) []string {
	value := strings.TrimSpace(fields[key])
	if value == "" {
		return nil
	}
	var rawItems []any
	if strings.HasPrefix(value, "[") && json.Unmarshal([]byte(value), &rawItems) == nil {
		items := make([]string, 0, len(rawItems))
		for _, raw := range rawItems {
			item := strings.TrimSpace(fmt.Sprint(raw))
			if item != "" {
				items = append(items, item)
			}
		}
		return items
	}
	var items []string
	for _, line := range strings.FieldsFunc(value, func(r rune) bool {
		return r == '\n' || r == ';' || r == ','
	}) {
		item := strings.TrimSpace(line)
		item = strings.Trim(item, "-* ")
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}

func missingActiveWorkflowPhaseTaskCompletion(state events.SessionState, phaseID string) string {
	if state.Workflow == nil {
		return ""
	}
	workflowID := strings.TrimSpace(state.Workflow.WorkflowID)
	phaseID = strings.TrimSpace(phaseID)
	seenPhaseTask := false
	for _, taskID := range state.TaskOrder {
		task := state.Tasks[strings.TrimSpace(taskID)]
		if task == nil {
			continue
		}
		if strings.TrimSpace(task.WorkflowID) != workflowID || strings.TrimSpace(task.WorkflowPhaseID) != phaseID {
			continue
		}
		seenPhaseTask = true
		if strings.TrimSpace(task.Status) == events.TaskStatusCompleted {
			continue
		}
		return "workflow phase has unfinished task: " + strings.TrimSpace(task.TaskID)
	}
	if !seenPhaseTask {
		if workflowLatestRevisionTriggerForPhase(state.Workflow, phaseID) != nil {
			return ""
		}
		return "workflow phase has no tasks"
	}
	return ""
}

func missingWorkflowPhaseEntryEvidence(state events.SessionState, phase workflowpkg.Phase) string {
	for _, item := range phase.Requires.Items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if !workflowHasAnyEvidenceType(state.Workflow, "", item) {
			return "missing required evidence: " + item
		}
	}
	for key, value := range phase.Requires.Fields {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			continue
		}
		if !workflowHasFieldEvidence(state.Workflow, key, value) {
			return "missing required evidence: " + key
		}
	}
	return ""
}

func workflowPhaseByID(definition workflowpkg.Definition, phaseID string) (workflowpkg.Phase, bool) {
	phaseID = strings.TrimSpace(phaseID)
	for _, phase := range definition.Phases {
		if strings.TrimSpace(phase.ID) == phaseID {
			return phase, true
		}
	}
	return workflowpkg.Phase{}, false
}

func workflowDefinitionHasPhase(definition workflowpkg.Definition, phaseID string) bool {
	_, ok := workflowPhaseByID(definition, phaseID)
	return ok
}

func workflowHasPhaseOutputEvidence(workflow *events.WorkflowState, phaseID, key string) bool {
	return workflowHasEvidence(workflow, phaseID, func(evidence *events.WorkflowEvidenceState) bool {
		if evidence.Type == key {
			return true
		}
		if evidence.Type != events.WorkflowEvidenceTypePhaseOutput {
			return false
		}
		return strings.TrimSpace(evidence.Fields[key]) != ""
	})
}

func workflowHasApprovalEvidence(workflow *events.WorkflowState, phaseID string) bool {
	return workflowHasEvidence(workflow, phaseID, func(evidence *events.WorkflowEvidenceState) bool {
		if evidence.Type != events.WorkflowEvidenceTypeApproval {
			return false
		}
		return evidence.Successful == nil || *evidence.Successful
	})
}

func workflowHasSuccessfulEvidence(workflow *events.WorkflowState, phaseID, evidenceType string) bool {
	return workflowHasEvidence(workflow, phaseID, func(evidence *events.WorkflowEvidenceState) bool {
		if evidence.Type != evidenceType {
			return false
		}
		return evidence.Successful != nil && *evidence.Successful
	})
}

func workflowHasAnyEvidenceType(workflow *events.WorkflowState, phaseID string, evidenceTypes ...string) bool {
	return workflowHasEvidence(workflow, phaseID, func(evidence *events.WorkflowEvidenceState) bool {
		for _, evidenceType := range evidenceTypes {
			if evidence.Type == strings.TrimSpace(evidenceType) {
				return true
			}
		}
		return false
	})
}

func workflowHasFieldEvidence(workflow *events.WorkflowState, key, value string) bool {
	return workflowHasEvidence(workflow, "", func(evidence *events.WorkflowEvidenceState) bool {
		got, ok := evidence.Fields[key]
		if !ok {
			return false
		}
		if value == "" {
			return strings.TrimSpace(got) != ""
		}
		return strings.TrimSpace(got) == value
	})
}

func workflowHasPhaseFieldEvidence(workflow *events.WorkflowState, phaseID, key, value string) bool {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	return workflowHasEvidence(workflow, phaseID, func(evidence *events.WorkflowEvidenceState) bool {
		got, ok := evidence.Fields[key]
		if !ok {
			return false
		}
		if value == "" {
			return strings.TrimSpace(got) != ""
		}
		return strings.TrimSpace(got) == value
	})
}

func workflowHasEvidence(workflow *events.WorkflowState, phaseID string, match func(*events.WorkflowEvidenceState) bool) bool {
	if workflow == nil || match == nil {
		return false
	}
	phaseID = strings.TrimSpace(phaseID)
	for _, evidenceID := range workflow.EvidenceOrder {
		evidence := workflow.Evidence[evidenceID]
		if evidence == nil {
			continue
		}
		if phaseID != "" && strings.TrimSpace(evidence.PhaseID) != phaseID {
			continue
		}
		if match(evidence) {
			return true
		}
	}
	return false
}

func cloneEvidenceFields(fields map[string]string) map[string]string {
	if len(fields) == 0 {
		return nil
	}
	out := make(map[string]string, len(fields))
	for key, value := range fields {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}
