package app

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
)

var ErrTurnRolloverStateMissing = errors.New("turn rollover state is missing")

type runtimeTurnContinuation struct {
	PreviousTurnID string
	Reason         string
	Question       string
}

func continuationReason(continuation *runtimeTurnContinuation) string {
	if continuation == nil {
		return ""
	}
	return strings.TrimSpace(continuation.Reason)
}

func turnContinuationPromptFragment(continuation runtimeTurnContinuation) prompt.Fragment {
	return prompt.Fragment{
		Kind:      prompt.KindRuntime,
		Source:    prompt.SourceRuntime,
		Stability: prompt.StabilityDynamic,
		Layer:     "turn-continuation",
		Key:       "turn-continuation",
		Label:     "turn continuation",
		Content:   turnContinuationPromptContent(continuation),
	}
}

func turnContinuationPromptContent(continuation runtimeTurnContinuation) string {
	switch strings.TrimSpace(continuation.Reason) {
	case events.TurnContinuationReasonContextLimit:
		return strings.TrimSpace(`
This turn continues automatically from the previous turn because the prior turn reached the model input limit.

Use the carried active turn summary in the conversation as the authoritative state from the previous turn. Continue from that summary instead of restarting completed work. If the summary is insufficient to continue safely, ask the user for clarification.`)
	case events.TurnContinuationReasonQuestionAnswer:
		question := strings.TrimSpace(continuation.Question)
		if question == "" {
			return strings.TrimSpace(`
This turn continues from the previous turn after the user answered a pending question.

Use the carried active turn summary in the conversation as the authoritative state from the previous turn. The user message in this turn is the answer to that pending question. Continue from that answer instead of asking the same question again.`)
		}
		return strings.TrimSpace(`
This turn continues from the previous turn after the user answered a pending question.

Pending question from the previous turn: ` + question + `

Use the carried active turn summary in the conversation as the authoritative state from the previous turn. The user message in this turn is the answer to that pending question. Continue from that answer instead of asking the same question again.`)
	default:
		return strings.TrimSpace(`
This turn continues automatically from the previous turn.

Use the carried active turn summary in the conversation as the authoritative state from the previous turn.`)
	}
}

func buildTurnRolloverInitialState(turn *events.TurnState) (*turnLoopState, error) {
	if turn == nil {
		return nil, ErrTurnRolloverStateMissing
	}
	workState := turnWorkState{}
	if turn.WorkState != nil {
		workState = turnWorkStateFromEventState(turn.WorkState)
	}
	if turnWorkSummaryEmpty(workState.Summary) {
		workState.Summary = fallbackTurnRolloverSummary(turn)
	}
	workState.NativeContinuation = nil
	if turnWorkSummaryEmpty(workState.Summary) {
		return nil, ErrTurnRolloverStateMissing
	}
	return &turnLoopState{
		LatestToolStepStart: -1,
		WorkState:           workState,
	}, nil
}

func turnWorkSummaryEmpty(summary turnWorkSummary) bool {
	return strings.TrimSpace(summary.Objective) == "" &&
		len(summary.Decisions) == 0 &&
		len(summary.TouchedPaths) == 0 &&
		len(summary.CompletedWork) == 0 &&
		len(summary.Verification) == 0 &&
		len(summary.Failures) == 0 &&
		len(summary.OpenItems) == 0
}

func fallbackTurnRolloverSummary(turn *events.TurnState) turnWorkSummary {
	if turn == nil {
		return turnWorkSummary{}
	}
	objective := strings.TrimSpace(turn.UserText)
	if objective == "" && len(turn.UserAttachments) > 0 {
		objective = "Continue the previous turn using the existing attachments."
	}
	summary := turnWorkSummary{
		Objective: objective,
	}
	if decision := sessionCompactionDecisionCandidate(turn.AssistantText); decision != "" {
		summary.Decisions = appendUniqueValues(summary.Decisions, []string{decision})
	}
	completed, next := extractCompactionAssistantFacts(turn.AssistantText)
	summary.CompletedWork = appendUniqueValues(summary.CompletedWork, completed)
	if next != "" {
		summary.OpenItems = appendUniqueValues(summary.OpenItems, []string{next})
	}
	return summary
}

func (r *Runtime) continueRolledOverTurn(
	ctx context.Context,
	input runExistingTurnInput,
	capabilities resolvedTurnCapabilities,
	thinkingEnabled bool,
	thinkingMode string,
) (RunSessionResult, error) {
	state, err := r.Sessions.Snapshot(ctx, input.SessionID)
	if err != nil {
		return RunSessionResult{}, err
	}
	parentTurn := state.Turns[input.TurnID]
	initialState, err := buildTurnRolloverInitialState(parentTurn)
	if err != nil {
		return RunSessionResult{}, err
	}
	return r.runExistingSessionTurn(ctx, runExistingTurnInput{
		SessionID:            input.SessionID,
		TurnID:               newRuntimeID("turn"),
		UserText:             "",
		AgentID:              capabilities.AgentID,
		SkillIDs:             append([]string(nil), capabilities.SkillIDs...),
		SelectedSkillIDs:     rolloverSelectedSkillIDs(input),
		ThinkingEnabled:      thinkingEnabled,
		ThinkingMode:         thinkingMode,
		Fragments:            append([]prompt.Fragment(nil), input.Fragments...),
		AllowedToolsOverride: append([]string(nil), capabilities.AllowedTools...),
		ModelRouteOverride:   capabilities.ModelRoute,
		PreserveSessionModel: true,
		HideAssistantPreview: input.HideAssistantPreview,
		DisableAutoReview:    input.DisableAutoReview,
		WorkflowBudget:       input.WorkflowBudget,
		InitialState:         initialState,
		Continuation: &runtimeTurnContinuation{
			PreviousTurnID: input.TurnID,
			Reason:         events.TurnContinuationReasonContextLimit,
		},
	})
}

func (r *Runtime) continueRolledOverResumedTurn(
	ctx context.Context,
	sessionID string,
	turnID string,
	resolved resolvedResumeTurn,
	capabilities resolvedTurnCapabilities,
	thinkingEnabled bool,
	thinkingMode string,
	fragments []prompt.Fragment,
	hideAssistantPreview bool,
	disableAutoReview bool,
) (RunSessionResult, error) {
	return r.continueRolledOverTurn(ctx, runExistingTurnInput{
		SessionID:            sessionID,
		TurnID:               turnID,
		UserText:             resolved.userText,
		AgentID:              capabilities.AgentID,
		SkillIDs:             append([]string(nil), resolved.skillIDs...),
		ThinkingEnabled:      thinkingEnabled,
		ThinkingMode:         thinkingMode,
		Fragments:            append([]prompt.Fragment(nil), fragments...),
		AllowedToolsOverride: slices.Clone(capabilities.AllowedTools),
		ModelRouteOverride:   capabilities.ModelRoute,
		PreserveSessionModel: true,
		HideAssistantPreview: hideAssistantPreview,
		DisableAutoReview:    disableAutoReview,
	}, capabilities, thinkingEnabled, thinkingMode)
}

func rolloverSelectedSkillIDs(input runExistingTurnInput) []string {
	if input.SelectedSkillIDs != nil {
		return cloneSelectedSkillIDs(input.SelectedSkillIDs)
	}
	return cloneSelectedSkillIDs(input.SkillIDs)
}
