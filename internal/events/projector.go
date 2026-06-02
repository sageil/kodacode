package events

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/textutil"
)

type TurnStatus string

const (
	TurnStatusRunning   TurnStatus = "running"
	TurnStatusCompleted TurnStatus = "completed"
	TurnStatusCanceled  TurnStatus = "canceled"
	TurnStatusFailed    TurnStatus = "failed"
)

type TranscriptEntryKind string

const (
	TranscriptEntryUser       TranscriptEntryKind = "user"
	TranscriptEntryAssistant  TranscriptEntryKind = "assistant"
	TranscriptEntryReview     TranscriptEntryKind = "review"
	TranscriptEntryWorklog    TranscriptEntryKind = "worklog"
	TranscriptEntryCompaction TranscriptEntryKind = "compaction"
	TranscriptEntryReasoning  TranscriptEntryKind = "reasoning"
	TranscriptEntryTool       TranscriptEntryKind = "tool"
	TranscriptEntryError      TranscriptEntryKind = "error"
)

type TranscriptEntryState struct {
	Kind      TranscriptEntryKind
	Sequence  int64
	Text      string
	CallID    string
	SegmentID string
}

type SessionState struct {
	SessionID                    string
	WorkspaceRoot                string
	AdditionalWorkspaceRoots     []string
	PermissionMode               string
	ProviderRequestLimitDisabled bool
	Branch                       *SessionBranchState
	MCP                          *SessionMCPState
	Model                        string
	Title                        string
	SessionGrantDecisions        []SessionGrantDecisionState
	WorkspaceGrants              []WorkspaceGrantState
	ExecutionGrants              []ExecutionGrantState
	NetworkGrants                []NetworkGrantState
	TaskOrder                    []string
	Tasks                        map[string]*TaskState
	Workflow                     *WorkflowState
	ReviewOrder                  []string
	Reviews                      map[string]*ReviewState
	PlanOrder                    []string
	Plans                        map[string]*PlanState
	ApprovedExecutions           map[string]*ApprovedExecutionState
	PendingExecutionOrder        []string
	PendingExecutions            map[string]*ExecutionApprovalState
	PendingPermissionOrder       []string
	PendingPermissions           map[string]*PermissionRequestState
	PendingQuestionOrder         []string
	PendingQuestions             map[string]*QuestionRequestState
	QuestionAnswers              map[string]*QuestionAnswerState
	LastSequence                 int64
	TurnOrder                    []string
	Turns                        map[string]*TurnState
}

type SessionBranchState struct {
	ParentSessionID string
	ParentTurnID    string
	ParentSequence  int64
}

type TurnState struct {
	TurnID                string
	Status                TurnStatus
	UserText              string
	UserAttachments       []UserAttachmentPayload
	Config                *TurnConfigState
	ContinuationStart     *TurnContinuationState
	Prompt                *PromptState
	Pruning               *PruningState
	CompactionAttempt     *CompactionAttemptState
	CompactionFailure     *CompactionFailureState
	HistoryCompactionUI   *HistoryCompactionUIState
	Continuation          *HistoryContinuationState
	ContextUsage          *TurnContextUsageState
	WorkflowRoute         *WorkflowRouteRecommendationState
	Handoffs              map[string]*AgentHandoffState
	HandoffOrder          []string
	AssistantText         string
	StreamingText         string
	ReasoningText         string
	Retry                 *TurnRetryState
	ProviderUsage         *TurnProviderUsageState
	ProviderReportedUsage *TurnProviderReportedUsageState
	ProviderAttempts      []TurnProviderAttemptState
	Review                *ReviewState
	WorkState             *TurnWorkState
	Error                 string
	ErrorCode             TurnFailureCode
	ErrorRetryable        bool
	Transcript            []TranscriptEntryState
	ToolCallBatches       []ToolCallBatchState
	ToolCallOrder         []string
	ToolCalls             map[string]*ToolCallState
	CompletedAtSeq        int64
	LastUpdatedAtSeq      int64
}

type WorkspaceGrantState struct {
	Path      string
	Recursive bool
}

type SessionGrantDecisionSource string

type TurnRetryState struct {
	Message     string
	Attempt     int
	MaxAttempts int
	RetryAt     time.Time
}

const (
	SessionGrantDecisionSourcePermission        SessionGrantDecisionSource = "permission"
	SessionGrantDecisionSourceExecutionApproval SessionGrantDecisionSource = "execution_approval"
)

type SessionGrantDecisionState struct {
	Source           SessionGrantDecisionSource
	PermissionKind   PermissionRequestKind
	ToolName         string
	Command          string
	Path             string
	WorkingDirectory string
	ResolvedAtSeq    int64
}

type ExecutionGrantState struct {
	PrefixRule     []string
	SessionPaths   []string
	NetworkTargets []string
}

type NetworkGrantState struct {
	Target string
}

type PermissionRequestState struct {
	Kind             PermissionRequestKind
	RequestID        string
	ExecutionID      string
	TurnID           string
	ToolCallID       string
	Access           string
	Path             string
	WorkingDirectory string
	ToolName         string
	Command          string
	Reason           string
	RequestedAtSeq   int64
}

type QuestionRequestState struct {
	QuestionID     string
	TurnID         string
	ToolCallID     string
	ToolName       string
	PlanID         string
	Question       string
	Options        []string
	Multiple       bool
	Purpose        string
	RequestedAtSeq int64
}

type QuestionAnswerState struct {
	QuestionID    string
	TurnID        string
	ToolCallID    string
	ToolName      string
	PlanID        string
	Question      string
	Purpose       string
	Answer        string
	AnsweredAtSeq int64
}

type ToolCallState struct {
	CallID              string
	ToolName            string
	ReusedFromCallID    string
	ReusedFromSessionID string
	ReusedFromTurnID    string
	RetryOfCallID       string
	HandoffID           string
	Execution           *ExecutionState
	Runtime             *ToolExecRuntimeState
	FailureClass        string
	Succeeded           bool
	Input               string
	Output              string
	Error               string
	ErrorDetail         *ToolErrorDetail
	StructuredResult    json.RawMessage
	MutationRanges      []MutationRange
	WriteMutation       *WriteMutation
	WriteMutations      []WriteMutation
	ObservedResources   []ObservedResource
	OutputBlob          *ToolResultBlobRef
	ErrorBlob           *ToolResultBlobRef
	OutputTruncated     bool
	ErrorTruncated      bool
	Declared            bool
	Executing           bool
	Completed           bool
	LastUpdatedSeq      int64
}

type ToolCallBatchState struct {
	CallIDs  []string
	Sequence int64
}

type Projector struct {
	state     SessionState
	toolCalls map[string]*ToolCallState
	reasoning map[string]*turnReasoningAccumulator
}

func NewProjector(sessionID string) *Projector {
	return &Projector{
		state: SessionState{
			SessionID:          sessionID,
			PermissionMode:     "auto",
			LastSequence:       -1,
			Tasks:              make(map[string]*TaskState),
			Reviews:            make(map[string]*ReviewState),
			Plans:              make(map[string]*PlanState),
			ApprovedExecutions: make(map[string]*ApprovedExecutionState),
			PendingExecutions:  make(map[string]*ExecutionApprovalState),
			PendingPermissions: make(map[string]*PermissionRequestState),
			PendingQuestions:   make(map[string]*QuestionRequestState),
			QuestionAnswers:    make(map[string]*QuestionAnswerState),
			Turns:              make(map[string]*TurnState),
		},
		toolCalls: make(map[string]*ToolCallState),
		reasoning: make(map[string]*turnReasoningAccumulator),
	}
}

func (p *Projector) Apply(event Event) error {
	if err := event.Validate(); err != nil {
		return err
	}
	if p.state.SessionID == "" {
		p.state.SessionID = event.SessionID
	}
	if event.SessionID != p.state.SessionID {
		return fmt.Errorf("event session mismatch: got %q want %q", event.SessionID, p.state.SessionID)
	}
	if !event.Ephemeral && event.Sequence <= p.state.LastSequence {
		return fmt.Errorf("event sequence out of order: got %d after %d", event.Sequence, p.state.LastSequence)
	}
	if err := p.applyPayload(event); err != nil {
		return err
	}
	if !event.Ephemeral {
		p.state.LastSequence = event.Sequence
	}
	return nil
}

func filterCanceledTurnErrors(entries []TranscriptEntryState, _ string) []TranscriptEntryState {
	if len(entries) == 0 {
		return entries
	}
	filtered := entries[:0]
	for _, entry := range entries {
		if entry.Kind == TranscriptEntryError {
			continue
		}
		filtered = append(filtered, entry)
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func clearTurnRetryState(turn *TurnState) {
	if turn == nil {
		return
	}
	turn.Retry = nil
}

type turnReasoningAccumulator struct {
	full         textutil.StringAccumulator
	segment      textutil.StringAccumulator
	segmentIndex int
}

func (p *Projector) appendReasoning(turnID string, turn *TurnState, sequence int64, content, segmentID string) {
	if turn == nil || content == "" {
		return
	}
	acc := p.reasoningAccumulator(turnID, turn)
	turn.ReasoningText = acc.full.Append(turn.ReasoningText, content)
	if acc.segmentIndex >= 0 && acc.segmentIndex == len(turn.Transcript)-1 {
		entry := &turn.Transcript[acc.segmentIndex]
		if entry.Kind == TranscriptEntryReasoning && entry.SegmentID == segmentID {
			entry.Text = acc.segment.Append(entry.Text, content)
			entry.Sequence = sequence
			return
		}
	}
	turn.Transcript = append(turn.Transcript, TranscriptEntryState{
		Kind:      TranscriptEntryReasoning,
		Sequence:  sequence,
		Text:      content,
		SegmentID: segmentID,
	})
	acc.segmentIndex = len(turn.Transcript) - 1
	acc.segment.Reset()
}

func (p *Projector) reasoningAccumulator(turnID string, turn *TurnState) *turnReasoningAccumulator {
	if p.reasoning == nil {
		p.reasoning = make(map[string]*turnReasoningAccumulator)
	}
	if acc := p.reasoning[turnID]; acc != nil {
		return acc
	}
	acc := &turnReasoningAccumulator{segmentIndex: -1}
	if turn != nil {
		if n := len(turn.Transcript); n > 0 && turn.Transcript[n-1].Kind == TranscriptEntryReasoning {
			acc.segmentIndex = n - 1
		}
	}
	p.reasoning[turnID] = acc
	return acc
}

func clearTurnPassTransientState(turn *TurnState, sequence int64) {
	clearTurnRetryState(turn)
	clearTurnCompactionAttemptState(turn)
	resumeTurnHistoryCompactionUI(turn, sequence)
}

func clearTurnCompactionAttemptState(turn *TurnState) {
	if turn == nil {
		return
	}
	turn.CompactionAttempt = nil
}

func clearUndeclaredToolCalls(turn *TurnState) {
	if turn == nil || len(turn.ToolCallOrder) == 0 {
		return
	}
	kept := turn.ToolCallOrder[:0]
	for _, callID := range turn.ToolCallOrder {
		call := turn.ToolCalls[callID]
		if call == nil {
			continue
		}
		if !call.Declared && !call.Executing && !call.Completed {
			delete(turn.ToolCalls, callID)
			continue
		}
		kept = append(kept, callID)
	}
	if len(kept) == 0 {
		turn.ToolCallOrder = nil
		return
	}
	turn.ToolCallOrder = kept
}

func ensureExecutionBackgroundState(execution *ExecutionState) *ExecutionBackgroundState {
	if execution == nil {
		return nil
	}
	if execution.Background == nil {
		execution.Background = &ExecutionBackgroundState{
			Status: initialExecutionBackgroundStatus(execution.Intent, nil),
		}
	}
	return execution.Background
}

func ensureExecutionStateForCall(call *ToolCallState, executionID string) *ExecutionState {
	if call == nil || strings.TrimSpace(executionID) == "" {
		return call.Execution
	}
	if call.Execution == nil {
		call.Execution = &ExecutionState{
			ExecutionID: executionID,
			ToolCallID:  call.CallID,
		}
	}
	return call.Execution
}

func initialExecutionBackgroundStatus(intent string, readyPatterns []string) ExecutionBackgroundStatus {
	if strings.TrimSpace(intent) == "watcher" || len(readyPatterns) == 0 {
		return ExecutionBackgroundStatusRunning
	}
	return ExecutionBackgroundStatusStarting
}

func appendExecutionBackgroundOutput(background *ExecutionBackgroundState, chunk string) {
	if background == nil || chunk == "" {
		return
	}
	background.OutputBytes += int64(len(chunk))
	background.OutputTail = appendExecutionOutputTail(background.OutputTail, chunk, 8192)
	if background.Status == "" {
		background.Status = ExecutionBackgroundStatusRunning
	}
}

func appendExecutionOutputTail(current, chunk string, limit int) string {
	return textutil.AppendRuneTail(current, chunk, limit)
}

func questionAnswerStateKey(turnID, toolCallID string) string {
	return turnID + "\x00" + toolCallID
}

func sessionGrantDecisionFromExecutionRequest(request *ExecutionApprovalState, resolvedAtSeq int64) SessionGrantDecisionState {
	return SessionGrantDecisionState{
		Source:           SessionGrantDecisionSourceExecutionApproval,
		ToolName:         request.ToolName,
		Command:          request.Command,
		WorkingDirectory: request.WorkingDirectory,
		ResolvedAtSeq:    resolvedAtSeq,
	}
}

func sessionGrantDecisionFromPermissionRequest(request *PermissionRequestState, resolvedAtSeq int64) SessionGrantDecisionState {
	return SessionGrantDecisionState{
		Source:           SessionGrantDecisionSourcePermission,
		PermissionKind:   request.Kind,
		ToolName:         request.ToolName,
		Command:          request.Command,
		Path:             request.Path,
		WorkingDirectory: request.WorkingDirectory,
		ResolvedAtSeq:    resolvedAtSeq,
	}
}

func executionApprovalDecisionAllowed(decision ExecutionApprovalDecision) bool {
	switch decision {
	case ExecutionApprovalDecisionAccept, ExecutionApprovalDecisionAcceptForSession, ExecutionApprovalDecisionAcceptWithExecPolicy, ExecutionApprovalDecisionApplyNetworkPolicy:
		return true
	default:
		return false
	}
}

func executionApprovalDecisionAllowsResume(decision ExecutionApprovalDecision) bool {
	return executionApprovalDecisionAllowed(decision)
}
