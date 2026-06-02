package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	workflowpkg "github.com/sageil/kodacode/internal/workflow"
)

var (
	ErrWorkflowStateMissing         = errors.New("workflow state is missing")
	ErrWorkflowAlreadyActive        = errors.New("workflow is already active")
	ErrWorkflowTransitionInvalid    = errors.New("workflow phase transition is invalid")
	ErrWorkflowPhaseBlocked         = errors.New("workflow phase is blocked")
	ErrWorkflowPhaseNotBlocked      = errors.New("workflow phase is not blocked")
	ErrWorkflowCompletionInvalid    = errors.New("workflow cannot complete before final phase")
	ErrWorkflowSessionNotConfigured = errors.New("workflow session is not configured")
)

type StartWorkflowInput struct {
	SessionID     string
	TurnID        string
	WorkspaceRoot string
	WorkflowID    string
}

type AdvanceWorkflowInput struct {
	SessionID  string
	TurnID     string
	ToPhaseID  string
	StopReason string
}

type BlockWorkflowInput struct {
	SessionID  string
	TurnID     string
	PhaseID    string
	StopReason string
}

type ResumeWorkflowInput struct {
	SessionID string
	TurnID    string
	PhaseID   string
}

type CompleteWorkflowInput struct {
	SessionID  string
	TurnID     string
	StopReason string
}

func (r *Runtime) StartWorkflow(ctx context.Context, input StartWorkflowInput) error {
	if strings.TrimSpace(input.SessionID) == "" {
		return ErrSessionIDRequired
	}
	definition, err := r.resolveWorkflow(ctx, input.WorkspaceRoot, input.WorkflowID)
	if err != nil {
		return err
	}
	firstPhaseID := firstWorkflowPhaseID(definition)
	if firstPhaseID == "" {
		return workflowpkg.ErrWorkflowPhaseRequired
	}
	state, err := r.Sessions.Snapshot(ctx, input.SessionID)
	if err != nil {
		return err
	}
	if state.Workflow != nil && state.Workflow.Status != events.WorkflowStatusCompleted {
		return ErrWorkflowAlreadyActive
	}
	turnID := workflowEventTurnID(input.TurnID)
	if _, err := r.Sessions.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    turnID,
		Type:      events.TypeWorkflowStarted,
		Payload: events.WorkflowStartedPayload{
			WorkflowID: definition.ID,
			PhaseID:    firstPhaseID,
		},
	}); err != nil {
		return err
	}
	_, err = r.Sessions.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    turnID,
		Type:      events.TypeWorkflowPhaseStarted,
		Payload: events.WorkflowPhaseStartedPayload{
			WorkflowID: definition.ID,
			PhaseID:    firstPhaseID,
		},
	})
	return err
}

func (r *Runtime) AdvanceWorkflow(ctx context.Context, input AdvanceWorkflowInput) error {
	state, definition, workflow, err := r.activeWorkflowState(ctx, input.SessionID)
	if err != nil {
		return err
	}
	if workflow.Status == events.WorkflowStatusBlocked {
		return ErrWorkflowPhaseBlocked
	}
	fromPhaseID := strings.TrimSpace(workflow.CurrentPhaseID)
	toPhaseID, err := nextWorkflowPhaseID(definition, fromPhaseID, input.ToPhaseID)
	if err != nil {
		return err
	}
	if err := r.ensureWorkflowEvidenceAllowsAdvance(ctx, input.SessionID, input.TurnID, state, definition, fromPhaseID, toPhaseID); err != nil {
		return err
	}
	return r.appendWorkflowPhaseAdvanced(ctx, input.SessionID, input.TurnID, workflow.WorkflowID, fromPhaseID, toPhaseID, input.StopReason)
}

func (r *Runtime) BlockWorkflow(ctx context.Context, input BlockWorkflowInput) error {
	_, _, workflow, err := r.activeWorkflowState(ctx, input.SessionID)
	if err != nil {
		return err
	}
	phaseID := strings.TrimSpace(input.PhaseID)
	if phaseID == "" {
		phaseID = strings.TrimSpace(workflow.CurrentPhaseID)
	}
	if phaseID != strings.TrimSpace(workflow.CurrentPhaseID) {
		return ErrWorkflowTransitionInvalid
	}
	return r.blockWorkflowPhase(ctx, input.SessionID, input.TurnID, workflow.WorkflowID, phaseID, input.StopReason)
}

func (r *Runtime) ResumeWorkflow(ctx context.Context, input ResumeWorkflowInput) error {
	_, _, workflow, err := r.activeWorkflowState(ctx, input.SessionID)
	if err != nil {
		return err
	}
	if workflow.Status != events.WorkflowStatusBlocked {
		return ErrWorkflowPhaseNotBlocked
	}
	phaseID := strings.TrimSpace(input.PhaseID)
	if phaseID == "" {
		phaseID = strings.TrimSpace(workflow.CurrentPhaseID)
	}
	if phaseID != strings.TrimSpace(workflow.CurrentPhaseID) {
		return ErrWorkflowTransitionInvalid
	}
	_, err = r.Sessions.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    workflowEventTurnID(input.TurnID),
		Type:      events.TypeWorkflowPhaseResumed,
		Payload: events.WorkflowPhaseResumedPayload{
			WorkflowID: workflow.WorkflowID,
			PhaseID:    phaseID,
		},
	})
	return err
}

func (r *Runtime) CompleteWorkflow(ctx context.Context, input CompleteWorkflowInput) error {
	state, definition, workflow, err := r.activeWorkflowState(ctx, input.SessionID)
	if err != nil {
		return err
	}
	if workflow.Status == events.WorkflowStatusBlocked {
		return ErrWorkflowPhaseBlocked
	}
	phaseID := strings.TrimSpace(workflow.CurrentPhaseID)
	if !isFinalWorkflowPhase(definition, phaseID) {
		return ErrWorkflowCompletionInvalid
	}
	phase, ok := workflowPhaseByID(definition, phaseID)
	if !ok {
		return ErrWorkflowTransitionInvalid
	}
	if reason := missingWorkflowPhaseCompletionEvidence(state, phase); reason != "" {
		if err := r.blockWorkflowPhase(ctx, input.SessionID, input.TurnID, workflow.WorkflowID, phaseID, reason); err != nil {
			return err
		}
		return fmt.Errorf("%w: %s", ErrWorkflowEvidenceMissing, reason)
	}
	_, err = r.Sessions.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    workflowEventTurnID(input.TurnID),
		Type:      events.TypeWorkflowCompleted,
		Payload: events.WorkflowCompletedPayload{
			WorkflowID: workflow.WorkflowID,
			PhaseID:    phaseID,
			StopReason: strings.TrimSpace(input.StopReason),
		},
	})
	return err
}

func (r *Runtime) maybeReviseWorkflowAfterVerificationFailure(ctx context.Context, sessionID, turnID string) (bool, error) {
	state, definition, workflow, err := r.activeWorkflowState(ctx, sessionID)
	if err != nil {
		return false, err
	}
	if workflow == nil {
		return false, nil
	}
	phaseID := strings.TrimSpace(workflow.CurrentPhaseID)
	phase, ok := workflowPhaseByID(definition, phaseID)
	if !ok || !workflowPhaseIsVerification(phase) {
		return false, nil
	}
	transition, hasTransition := workflowTransitionFor(definition, phaseID, workflowpkg.TransitionOnVerificationFailed)
	maxLoops := definition.MaxRevisionLoops
	toPhaseID := workflowRevisionPhaseID(definition, phaseID)
	if hasTransition {
		if transition.MaxLoops > 0 {
			maxLoops = transition.MaxLoops
		}
		toPhaseID = strings.TrimSpace(transition.To)
	}
	if maxLoops <= 0 {
		return false, nil
	}
	failedCount, failedEvidence := workflowFailedVerificationEvidence(state.Workflow, phaseID)
	if failedCount == 0 || failedCount > maxLoops {
		return false, nil
	}
	if toPhaseID == "" {
		return false, nil
	}
	if err := r.recordWorkflowRevisionTriggerEvidence(ctx, sessionID, turnID, state, workflowpkg.TransitionOnVerificationFailed, phaseID, toPhaseID, failedCount, maxLoops, failedEvidence); err != nil {
		return false, err
	}
	if err := r.appendWorkflowPhaseAdvanced(ctx, sessionID, turnID, workflow.WorkflowID, phaseID, toPhaseID, fmt.Sprintf("revision loop %d/%d after failed verification", failedCount, maxLoops)); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Runtime) maybeReviseWorkflowAfterReviewFailure(ctx context.Context, sessionID, turnID string) (bool, error) {
	state, definition, workflow, err := r.activeWorkflowState(ctx, sessionID)
	if err != nil {
		return false, err
	}
	if workflow == nil {
		return false, nil
	}
	phaseID := strings.TrimSpace(workflow.CurrentPhaseID)
	phase, ok := workflowPhaseByID(definition, phaseID)
	if !ok || !workflowPhaseIsReview(phase) {
		return false, nil
	}
	transition, hasTransition := workflowTransitionFor(definition, phaseID, workflowpkg.TransitionOnReviewFailed)
	if !hasTransition {
		return false, nil
	}
	maxLoops := definition.MaxRevisionLoops
	if transition.MaxLoops > 0 {
		maxLoops = transition.MaxLoops
	}
	if maxLoops <= 0 {
		return false, nil
	}
	failedCount, failedEvidence := workflowFailedReviewEvidence(state.Workflow, phaseID)
	if failedCount == 0 || failedCount > maxLoops {
		return false, nil
	}
	toPhaseID := strings.TrimSpace(transition.To)
	if toPhaseID == "" {
		return false, nil
	}
	if err := r.recordWorkflowRevisionTriggerEvidence(ctx, sessionID, turnID, state, workflowpkg.TransitionOnReviewFailed, phaseID, toPhaseID, failedCount, maxLoops, failedEvidence); err != nil {
		return false, err
	}
	if err := r.appendWorkflowPhaseAdvanced(ctx, sessionID, turnID, workflow.WorkflowID, phaseID, toPhaseID, fmt.Sprintf("revision loop %d/%d after failed review", failedCount, maxLoops)); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Runtime) recordWorkflowRevisionTriggerEvidence(ctx context.Context, sessionID, turnID string, state events.SessionState, transitionEvent, fromPhaseID, toPhaseID string, revisionIndex, maxLoops int, source *events.WorkflowEvidenceState) error {
	if source == nil {
		return nil
	}
	transitionEvent = strings.TrimSpace(transitionEvent)
	if workflowHasEvidence(state.Workflow, fromPhaseID, func(evidence *events.WorkflowEvidenceState) bool {
		if evidence.Type != events.WorkflowEvidenceTypeRevisionTrigger {
			return false
		}
		return strings.TrimSpace(evidence.Fields["revision_event"]) == transitionEvent &&
			strings.TrimSpace(evidence.Fields["source_evidence_id"]) == strings.TrimSpace(source.EvidenceID)
	}) {
		return nil
	}
	fields := workflowRevisionTriggerFields(state, transitionEvent, fromPhaseID, toPhaseID, revisionIndex, maxLoops, source)
	summary := fmt.Sprintf("revision loop %d/%d after %s", revisionIndex, maxLoops, strings.ReplaceAll(transitionEvent, "_", " "))
	if sourceSummary := strings.TrimSpace(source.Summary); sourceSummary != "" {
		summary += ": " + sourceSummary
	}
	successful := false
	return r.RecordWorkflowEvidence(ctx, RecordWorkflowEvidenceInput{
		SessionID:  sessionID,
		TurnID:     turnID,
		PhaseID:    fromPhaseID,
		Type:       events.WorkflowEvidenceTypeRevisionTrigger,
		Successful: &successful,
		Summary:    summary,
		Fields:     fields,
	})
}

func workflowRevisionTriggerFields(state events.SessionState, transitionEvent, fromPhaseID, toPhaseID string, revisionIndex, maxLoops int, source *events.WorkflowEvidenceState) map[string]string {
	fields := map[string]string{
		"revision_event":       strings.TrimSpace(transitionEvent),
		"revision_from_phase":  strings.TrimSpace(fromPhaseID),
		"revision_to_phase":    strings.TrimSpace(toPhaseID),
		"revision_index":       strconv.Itoa(revisionIndex),
		"max_revision_loops":   strconv.Itoa(maxLoops),
		"source_evidence_id":   strings.TrimSpace(source.EvidenceID),
		"source_evidence_type": strings.TrimSpace(source.Type),
		"source_phase_id":      strings.TrimSpace(source.PhaseID),
		"source_summary":       strings.TrimSpace(source.Summary),
	}
	if command := strings.TrimSpace(source.Command); command != "" {
		fields["failed_check"] = command
		fields["command"] = command
	}
	if source.ExitCode != nil {
		fields["exit_code"] = strconv.Itoa(*source.ExitCode)
	}
	if taskID := strings.TrimSpace(source.TaskID); taskID != "" {
		fields["task_id"] = taskID
	}
	if reviewID := strings.TrimSpace(source.ReviewID); reviewID != "" {
		fields["review_id"] = reviewID
	}
	for _, key := range []string{"review_pass", "review_status"} {
		if value := strings.TrimSpace(source.Fields[key]); value != "" {
			fields[key] = value
		}
	}
	if state.Reviews != nil {
		addWorkflowRevisionReviewFindingFields(fields, state.Reviews[strings.TrimSpace(source.ReviewID)])
	}
	return fields
}

func addWorkflowRevisionReviewFindingFields(fields map[string]string, review *events.ReviewState) {
	if review == nil || len(review.Findings) == 0 {
		return
	}
	fields["finding_count"] = strconv.Itoa(len(review.Findings))
	for index, finding := range review.Findings {
		prefix := fmt.Sprintf("finding_%d_", index+1)
		fields[prefix+"severity"] = strings.TrimSpace(finding.Severity)
		fields[prefix+"path"] = strings.TrimSpace(finding.Path)
		if finding.Line > 0 {
			fields[prefix+"line"] = strconv.Itoa(finding.Line)
		}
		fields[prefix+"title"] = strings.TrimSpace(finding.Title)
		fields[prefix+"explanation"] = strings.TrimSpace(finding.Explanation)
	}
}

func (r *Runtime) appendWorkflowPhaseAdvanced(ctx context.Context, sessionID, turnID, workflowID, fromPhaseID, toPhaseID, stopReason string) error {
	_, err := r.Sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    workflowEventTurnID(turnID),
		Type:      events.TypeWorkflowPhaseAdvanced,
		Payload: events.WorkflowPhaseAdvancedPayload{
			WorkflowID:  strings.TrimSpace(workflowID),
			FromPhaseID: strings.TrimSpace(fromPhaseID),
			ToPhaseID:   strings.TrimSpace(toPhaseID),
			StopReason:  strings.TrimSpace(stopReason),
		},
	})
	return err
}

func (r *Runtime) activeWorkflowState(ctx context.Context, sessionID string) (events.SessionState, workflowpkg.Definition, *events.WorkflowState, error) {
	if strings.TrimSpace(sessionID) == "" {
		return events.SessionState{}, workflowpkg.Definition{}, nil, ErrSessionIDRequired
	}
	state, err := r.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return events.SessionState{}, workflowpkg.Definition{}, nil, err
	}
	if strings.TrimSpace(state.WorkspaceRoot) == "" {
		return events.SessionState{}, workflowpkg.Definition{}, nil, ErrWorkflowSessionNotConfigured
	}
	if state.Workflow == nil || strings.TrimSpace(state.Workflow.WorkflowID) == "" {
		return events.SessionState{}, workflowpkg.Definition{}, nil, ErrWorkflowStateMissing
	}
	if state.Workflow.Status == events.WorkflowStatusCompleted {
		return events.SessionState{}, workflowpkg.Definition{}, nil, ErrWorkflowStateMissing
	}
	definition, err := r.resolveWorkflow(ctx, state.WorkspaceRoot, state.Workflow.WorkflowID)
	if err != nil {
		return events.SessionState{}, workflowpkg.Definition{}, nil, err
	}
	return state, definition, state.Workflow, nil
}

func workflowEventTurnID(turnID string) string {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return sessionTurnID
	}
	return turnID
}

func firstWorkflowPhaseID(definition workflowpkg.Definition) string {
	if len(definition.Phases) == 0 {
		return ""
	}
	return strings.TrimSpace(definition.Phases[0].ID)
}

func nextWorkflowPhaseID(definition workflowpkg.Definition, fromPhaseID, requestedToPhaseID string) (string, error) {
	fromPhaseID = strings.TrimSpace(fromPhaseID)
	requestedToPhaseID = strings.TrimSpace(requestedToPhaseID)
	for index, phase := range definition.Phases {
		if strings.TrimSpace(phase.ID) != fromPhaseID {
			continue
		}
		if index+1 >= len(definition.Phases) {
			return "", ErrWorkflowTransitionInvalid
		}
		next := strings.TrimSpace(definition.Phases[index+1].ID)
		if requestedToPhaseID != "" && requestedToPhaseID != next {
			return "", fmt.Errorf("%w: cannot advance from %s to %s", ErrWorkflowTransitionInvalid, fromPhaseID, requestedToPhaseID)
		}
		return next, nil
	}
	return "", fmt.Errorf("%w: unknown phase %s", ErrWorkflowTransitionInvalid, fromPhaseID)
}

func isFinalWorkflowPhase(definition workflowpkg.Definition, phaseID string) bool {
	phaseID = strings.TrimSpace(phaseID)
	if phaseID == "" || len(definition.Phases) == 0 {
		return false
	}
	return strings.TrimSpace(definition.Phases[len(definition.Phases)-1].ID) == phaseID
}

func workflowFailedVerificationEvidenceCount(workflow *events.WorkflowState, phaseID string) int {
	count, _ := workflowFailedVerificationEvidence(workflow, phaseID)
	return count
}

func workflowFailedVerificationEvidence(workflow *events.WorkflowState, phaseID string) (int, *events.WorkflowEvidenceState) {
	if workflow == nil {
		return 0, nil
	}
	count := 0
	var latest *events.WorkflowEvidenceState
	phaseID = strings.TrimSpace(phaseID)
	for _, evidenceID := range workflow.EvidenceOrder {
		evidence := workflow.Evidence[evidenceID]
		if evidence == nil || strings.TrimSpace(evidence.PhaseID) != phaseID {
			continue
		}
		if evidence.Type != events.WorkflowEvidenceTypeVerificationResult || evidence.Successful == nil || *evidence.Successful {
			continue
		}
		count++
		latest = evidence
	}
	return count, latest
}

func workflowFailedReviewEvidenceCount(workflow *events.WorkflowState, phaseID string) int {
	count, _ := workflowFailedReviewEvidence(workflow, phaseID)
	return count
}

func workflowFailedReviewEvidence(workflow *events.WorkflowState, phaseID string) (int, *events.WorkflowEvidenceState) {
	if workflow == nil {
		return 0, nil
	}
	count := 0
	var latest *events.WorkflowEvidenceState
	phaseID = strings.TrimSpace(phaseID)
	for _, evidenceID := range workflow.EvidenceOrder {
		evidence := workflow.Evidence[evidenceID]
		if evidence == nil || strings.TrimSpace(evidence.PhaseID) != phaseID {
			continue
		}
		switch evidence.Type {
		case events.WorkflowEvidenceTypeReviewOutcome, events.WorkflowEvidenceTypeReview, events.WorkflowEvidenceTypeTaskReview:
		default:
			continue
		}
		if evidence.Successful == nil || *evidence.Successful {
			continue
		}
		count++
		latest = evidence
	}
	return count, latest
}

func workflowRevisionPhaseID(definition workflowpkg.Definition, verificationPhaseID string) string {
	index := workflowPhaseIndex(definition, verificationPhaseID)
	if index <= 0 {
		return ""
	}
	for _, preferred := range []string{"implement", "patch"} {
		for i := 0; i < index; i++ {
			if strings.TrimSpace(definition.Phases[i].ID) == preferred {
				return preferred
			}
		}
	}
	for i := index - 1; i >= 0; i-- {
		phase := definition.Phases[i]
		if phase.EffectiveType() != workflowpkg.PhaseTypeAgent {
			continue
		}
		if workflowPhaseIsReadFocused(phase) || strings.TrimSpace(phase.Agent) == reviewerAgentID {
			continue
		}
		if id := strings.TrimSpace(phase.ID); id != "" {
			return id
		}
	}
	return ""
}

func workflowPhaseIndex(definition workflowpkg.Definition, phaseID string) int {
	phaseID = strings.TrimSpace(phaseID)
	for index, phase := range definition.Phases {
		if strings.TrimSpace(phase.ID) == phaseID {
			return index
		}
	}
	return -1
}

func workflowTransitionFor(definition workflowpkg.Definition, fromPhaseID, event string) (workflowpkg.Transition, bool) {
	fromPhaseID = strings.TrimSpace(fromPhaseID)
	event = strings.TrimSpace(event)
	if fromPhaseID == "" || event == "" {
		return workflowpkg.Transition{}, false
	}
	for _, transition := range definition.Transitions {
		if strings.TrimSpace(transition.From) == fromPhaseID && strings.TrimSpace(transition.On) == event {
			return transition, true
		}
	}
	return workflowpkg.Transition{}, false
}
