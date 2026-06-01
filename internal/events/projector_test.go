package events

import (
	"strings"
	"testing"
	"time"
)

func TestProjectorBuildsTurnStateFromAssistantAndToolEvents(t *testing.T) {
	projector := NewProjector("session-1")

	events := []Event{
		testEvent(0, "session-1", "turn-1", UserMessagePayload{Content: "hello"}),
		testEvent(1, "session-1", "turn-1", AssistantPreviewDeltaPayload{Content: "hel"}),
		testEvent(2, "session-1", "turn-1", AssistantPreviewDeltaPayload{Content: "lo"}),
		testEvent(3, "session-1", "turn-1", ToolCallDeltaPayload{CallID: "call-1", ToolName: "read", InputDelta: "{\"path\":\"a"}),
		testEvent(4, "session-1", "turn-1", ToolCallDeclaredPayload{CallID: "call-1", ToolName: "read", Input: "{\"path\":\"app.go\"}"}),
		testEvent(5, "session-1", "turn-1", ToolExecStartPayload{CallID: "call-1", ToolName: "read", Input: "{\"path\":\"app.go\"}"}),
		testEvent(6, "session-1", "turn-1", ToolExecOutputPayload{CallID: "call-1", Chunk: "package app\n"}),
		testEvent(7, "session-1", "turn-1", ToolExecEndPayload{
			CallID:         "call-1",
			ToolName:       "read",
			Output:         "package app\n",
			MutationRanges: []MutationRange{{OldStartLine: 3, NewStartLine: 3}},
			FailureClass:   "contract_violation",
			ErrorDetail:    &ToolErrorDetail{Code: "read_paths_required", Message: "paths is required", Retryable: true, Recovery: "retry_with_valid_input"},
			Backend:        "process",
		}),
		testEvent(8, "session-1", "turn-1", AssistantCommitPayload{Content: "hello"}),
		testEvent(9, "session-1", "turn-1", TurnDonePayload{}),
	}

	for _, event := range events {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn state missing")
	}
	if turn.Status != TurnStatusCompleted {
		t.Fatalf("turn status = %q, want %q", turn.Status, TurnStatusCompleted)
	}
	if turn.StreamingText != "" {
		t.Fatalf("streaming text = %q, want empty after commit", turn.StreamingText)
	}
	if turn.AssistantText != "hello" {
		t.Fatalf("assistant text = %q", turn.AssistantText)
	}
	if turn.UserText != "hello" {
		t.Fatalf("user text = %q", turn.UserText)
	}
	call := turn.ToolCalls["call-1"]
	if call == nil {
		t.Fatal("tool call state missing")
	}
	if !call.Declared || !call.Completed || call.Executing {
		t.Fatalf("tool call flags = declared:%v completed:%v executing:%v", call.Declared, call.Completed, call.Executing)
	}
	if call.Output != "package app\n" {
		t.Fatalf("tool output = %q", call.Output)
	}
	if len(call.MutationRanges) != 1 || call.MutationRanges[0].OldStartLine != 3 || call.MutationRanges[0].NewStartLine != 3 {
		t.Fatalf("mutation ranges = %#v", call.MutationRanges)
	}
	if call.FailureClass != "contract_violation" {
		t.Fatalf("failure class = %q", call.FailureClass)
	}
	if call.ErrorDetail == nil || call.ErrorDetail.Code != "read_paths_required" {
		t.Fatalf("error detail = %#v", call.ErrorDetail)
	}
	if call.Runtime == nil || call.Runtime.Backend != "process" {
		t.Fatalf("tool runtime = %#v", call.Runtime)
	}
}

func TestProjectorMergesConsecutiveAssistantCommits(t *testing.T) {
	projector := NewProjector("session-1")

	for _, event := range []Event{
		testEvent(0, "session-1", "turn-1", UserMessagePayload{Content: "hello"}),
		testEvent(1, "session-1", "turn-1", AssistantCommitPayload{Content: "partial "}),
		testEvent(2, "session-1", "turn-1", AssistantCommitPayload{Content: "partial final"}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	turn := projector.Snapshot().Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn state missing")
	}
	if turn.AssistantText != "partial final" {
		t.Fatalf("assistant text = %q, want stitched output", turn.AssistantText)
	}
	if len(turn.Transcript) != 2 {
		t.Fatalf("transcript = %#v, want user plus one assistant entry", turn.Transcript)
	}
	if entry := turn.Transcript[1]; entry.Kind != TranscriptEntryAssistant || entry.Text != "partial final" {
		t.Fatalf("assistant transcript entry = %#v", entry)
	}
}

func TestProjectorStoresToolCallBatchBoundaries(t *testing.T) {
	projector := NewProjector("session-1")

	for _, event := range []Event{
		testEvent(0, "session-1", "turn-1", ToolCallDeclaredPayload{CallID: "call-1", ToolName: "read", Input: `{"path":"a.go"}`}),
		testEvent(1, "session-1", "turn-1", ToolCallDeclaredPayload{CallID: "call-2", ToolName: "search", Input: `{"query":"TODO"}`}),
		testEvent(2, "session-1", "turn-1", ToolCallBatchPayload{CallIDs: []string{"call-1", "call-2"}}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	turn := projector.Snapshot().Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if len(turn.ToolCallBatches) != 1 {
		t.Fatalf("ToolCallBatches = %#v", turn.ToolCallBatches)
	}
	batch := turn.ToolCallBatches[0]
	if batch.Sequence != 2 {
		t.Fatalf("batch sequence = %d, want 2", batch.Sequence)
	}
	if len(batch.CallIDs) != 2 || batch.CallIDs[0] != "call-1" || batch.CallIDs[1] != "call-2" {
		t.Fatalf("batch CallIDs = %#v", batch.CallIDs)
	}
}

func TestProjectorStoresStructuredReviewTranscriptEntry(t *testing.T) {
	projector := NewProjector("session-1")

	for _, event := range []Event{
		testEvent(0, "session-1", "turn-1", AssistantCommitPayload{Content: `{"findings":[],"overall_correctness":"correct","overall_summary":"No concrete issues found."}`}),
		testEvent(1, "session-1", "turn-1", ReviewRecordedPayload{
			ReviewID:           "review-1",
			SourceHandoffID:    "handoff-1",
			Title:              "Full Project Review",
			Findings:           nil,
			OverallCorrectness: ReviewOverallCorrectnessCorrect,
			OverallSummary:     "No concrete issues found.",
		}),
		testEvent(2, "session-1", "turn-1", TurnDonePayload{}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.Review == nil {
		t.Fatal("review state missing")
	}
	if turn.Review.OverallCorrectness != ReviewOverallCorrectnessCorrect {
		t.Fatalf("overall correctness = %q", turn.Review.OverallCorrectness)
	}
	if turn.Review.Title != "Full Project Review" {
		t.Fatalf("review title = %q", turn.Review.Title)
	}
	if turn.Review.OverallSummary != "No concrete issues found." {
		t.Fatalf("overall summary = %q", turn.Review.OverallSummary)
	}
	if len(state.ReviewOrder) != 1 || state.ReviewOrder[0] != "review-1" {
		t.Fatalf("review order = %#v", state.ReviewOrder)
	}
	if review := state.Reviews["review-1"]; review == nil || review.SourceHandoffID != "handoff-1" || review.Title != "Full Project Review" {
		t.Fatalf("session review = %#v", review)
	}
	if len(turn.Transcript) != 2 {
		t.Fatalf("transcript = %#v, want assistant + review", turn.Transcript)
	}
	if turn.Transcript[1].Kind != TranscriptEntryReview {
		t.Fatalf("review transcript entry = %#v", turn.Transcript[1])
	}
}

func TestProjectorTurnCanceledClearsPendingQuestion(t *testing.T) {
	projector := NewProjector("session-1")

	for _, event := range []Event{
		testEvent(0, "session-1", "turn-1", UserMessagePayload{Content: "review"}),
		testEvent(1, "session-1", "turn-1", ToolCallDeclaredPayload{
			CallID:   "call-question",
			ToolName: "question",
			Input:    `{"question":"Which path?","options":["Apply","Stop"]}`,
		}),
		testEvent(2, "session-1", "turn-1", QuestionRequestedPayload{
			QuestionID: "q-1",
			ToolCallID: "call-question",
			ToolName:   "question",
			Question:   "Which path?",
			Options:    []string{"Apply", "Stop"},
		}),
		testEvent(3, "session-1", "turn-1", TurnCanceledPayload{Message: "turn canceled by user"}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	if len(state.PendingQuestionOrder) != 0 || len(state.PendingQuestions) != 0 {
		t.Fatalf("pending questions = order:%#v map:%#v, want none", state.PendingQuestionOrder, state.PendingQuestions)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.Status != TurnStatusCanceled {
		t.Fatalf("turn = %#v, want canceled", turn)
	}
	call := turn.ToolCalls["call-question"]
	if call == nil {
		t.Fatal("question call missing")
	}
	if !call.Completed || call.Executing {
		t.Fatalf("question call flags = completed:%v executing:%v", call.Completed, call.Executing)
	}
}

func TestProjectorStoresExecutionState(t *testing.T) {
	projector := NewProjector("session-1")

	if err := projector.Apply(testEvent(0, "session-1", "turn-1", ExecutionDeclaredPayload{
		ExecutionID:      "exec-call-1",
		ToolCallID:       "call-1",
		ToolName:         "bash",
		Kind:             "bash",
		Command:          []string{"task", "lint"},
		CommandPreview:   "task lint",
		WorkingDirectory: "/repo",
		TimeoutMS:        120000,
		OutputLimit:      12000,
	})); err != nil {
		t.Fatalf("Apply(execution_declared) error = %v", err)
	}

	state := projector.Snapshot()
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	if call == nil || call.Execution == nil {
		t.Fatal("execution state missing")
	}
	if got := call.Execution.Command[0]; got != "task" {
		t.Fatalf("command = %#v", call.Execution.Command)
	}
	if got := call.Execution.WorkingDirectory; got != "/repo" {
		t.Fatalf("working directory = %q", got)
	}
}

func TestProjectorStoresAttachmentOnlyUserMessage(t *testing.T) {
	projector := NewProjector("session-1")

	if err := projector.Apply(testEvent(0, "session-1", "turn-1", UserMessagePayload{
		Attachments: []UserAttachmentPayload{{
			Name:     "pixel.png",
			MIMEType: "image/png",
			DataURL:  "data:image/png;base64,AA==",
		}},
	})); err != nil {
		t.Fatalf("Apply(user_message) error = %v", err)
	}

	state := projector.Snapshot()
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn state missing")
	}
	if turn.UserText != "" {
		t.Fatalf("user text = %q, want empty attachment-only message", turn.UserText)
	}
	if len(turn.UserAttachments) != 1 || turn.UserAttachments[0].Name != "pixel.png" {
		t.Fatalf("user attachments = %#v", turn.UserAttachments)
	}
	if len(turn.Transcript) != 1 || turn.Transcript[0].Kind != TranscriptEntryUser || turn.Transcript[0].Text != "[Attached image: pixel.png]" {
		t.Fatalf("transcript = %#v", turn.Transcript)
	}
}

func TestProjectorRejectsOutOfOrderSequence(t *testing.T) {
	projector := NewProjector("session-1")

	if err := projector.Apply(testEvent(1, "session-1", "turn-1", AssistantPreviewDeltaPayload{Content: "a"})); err != nil {
		t.Fatalf("Apply(first) error = %v", err)
	}
	err := projector.Apply(testEvent(1, "session-1", "turn-1", AssistantCommitPayload{Content: "a"}))
	if err == nil {
		t.Fatal("Apply(second) error = nil, want sequence error")
	}
}

func TestProjectorClearsAssistantPreviewOnReset(t *testing.T) {
	projector := NewProjector("session-1")

	for _, event := range []Event{
		testEvent(0, "session-1", "turn-1", AssistantPreviewDeltaPayload{Content: "draft"}),
		testEvent(1, "session-1", "turn-1", AssistantPreviewResetPayload{}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.StreamingText != "" {
		t.Fatalf("streaming text = %q, want empty", turn.StreamingText)
	}
	if len(turn.Transcript) != 0 {
		t.Fatalf("transcript = %#v, want empty", turn.Transcript)
	}
}

func TestProjectorTracksTurnContextUsageAcrossCompactionBoundaries(t *testing.T) {
	projector := NewProjector("session-1")

	apply := func(event Event) *TurnState {
		t.Helper()
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
		turn := projector.Snapshot().Turns["turn-1"]
		if turn == nil {
			t.Fatal("turn missing")
		}
		return turn
	}

	turn := apply(testEvent(0, "session-1", "turn-1", TurnProviderUsageRecordedPayload{
		Kind:                   string(TurnProviderUsageKindAgent),
		Step:                   1,
		Attempt:                1,
		EstimatedRequestTokens: 35800,
		InputLimitTokens:       40000,
		RequestTokenSource:     "estimated",
	}))
	if turn.ContextUsage == nil || turn.ContextUsage.Tokens != 35800 || turn.ContextUsage.Limit != 40000 || turn.ContextUsage.Source != "estimated" {
		t.Fatalf("provider context usage = %#v", turn.ContextUsage)
	}

	turn = apply(testEvent(1, "session-1", "turn-1", SessionHistoryContinuationUpdatedPayload{
		EventVersion:          1,
		ArtifactVersion:       1,
		RendererVersion:       1,
		FrontierTurnID:        "turn-0",
		ConsolidatedTurnCount: 1,
		UpdateReason:          HistoryContinuationUpdateReasonTokenPressure,
		RenderedSummary:       "History Continuation:\n## Session Objective\n- continue",
		InputBudget: &HistoryInputBudgetPayload{
			InputLimitTokens:          40000,
			TriggerTokens:             32000,
			TargetTokens:              26000,
			EstimatedRequestTokens:    35800,
			ConsolidatedRequestTokens: 11786,
		},
		Attribution: HistoryContinuationAttribution{MeasurementSource: "estimated"},
	}))
	if turn.ContextUsage == nil || turn.ContextUsage.Tokens != 11786 || turn.ContextUsage.Limit != 40000 || turn.ContextUsage.Source != "estimated" {
		t.Fatalf("compacted context usage = %#v", turn.ContextUsage)
	}

	turn = apply(testEvent(2, "session-1", "turn-1", TurnProviderUsageRecordedPayload{
		Kind:                   string(TurnProviderUsageKindUtilityCompaction),
		Step:                   1,
		Attempt:                1,
		EstimatedRequestTokens: 900,
		InputLimitTokens:       8192,
		RequestTokenSource:     "exact",
	}))
	if turn.ContextUsage == nil || turn.ContextUsage.Tokens != 11786 || turn.ContextUsage.Limit != 40000 {
		t.Fatalf("utility compaction should not replace display context: %#v", turn.ContextUsage)
	}

	turn = apply(testEvent(3, "session-1", "turn-1", TurnProviderUsageRecordedPayload{
		Kind:                   string(TurnProviderUsageKindAgent),
		Step:                   2,
		Attempt:                1,
		EstimatedRequestTokens: 22000,
		InputLimitTokens:       40000,
		RequestTokenSource:     "exact",
	}))
	if turn.ContextUsage == nil || turn.ContextUsage.Tokens != 22000 || turn.ContextUsage.Limit != 40000 || turn.ContextUsage.Source != "exact" {
		t.Fatalf("later provider request should replace compacted display context: %#v", turn.ContextUsage)
	}
}

func TestProjectorCommitsAssistantWorklogIntoTranscript(t *testing.T) {
	projector := NewProjector("session-1")

	for _, event := range []Event{
		testEvent(0, "session-1", "turn-1", AssistantPreviewDeltaPayload{Content: "checking file"}),
		testEvent(1, "session-1", "turn-1", AssistantWorklogCommitPayload{Content: "checking file"}),
		testEvent(2, "session-1", "turn-1", AssistantPreviewResetPayload{}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	turn := projector.Snapshot().Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.StreamingText != "" {
		t.Fatalf("streaming text = %q, want empty", turn.StreamingText)
	}
	if len(turn.Transcript) != 1 {
		t.Fatalf("transcript = %#v, want worklog entry", turn.Transcript)
	}
	if turn.Transcript[0].Kind != TranscriptEntryWorklog || turn.Transcript[0].Text != "checking file" {
		t.Fatalf("turn.Transcript[0] = %#v", turn.Transcript[0])
	}
}

func TestProjectorPersistsReasoningInTranscriptAcrossTurnCompletion(t *testing.T) {
	projector := NewProjector("session-1")

	for _, event := range []Event{
		testEvent(0, "session-1", "turn-1", ReasoningDeltaPayload{Content: "Inspecting the current turn state."}),
		testEvent(1, "session-1", "turn-1", ReasoningDeltaPayload{Content: " Next I will verify the provider path."}),
		testEvent(2, "session-1", "turn-1", AssistantCommitPayload{Content: "Done."}),
		testEvent(3, "session-1", "turn-1", TurnDonePayload{}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.ReasoningText != "Inspecting the current turn state. Next I will verify the provider path." {
		t.Fatalf("reasoning text = %q", turn.ReasoningText)
	}
	if len(turn.Transcript) != 2 {
		t.Fatalf("transcript = %#v, want reasoning + assistant", turn.Transcript)
	}
	if turn.Transcript[0].Kind != TranscriptEntryReasoning || turn.Transcript[0].Text != turn.ReasoningText {
		t.Fatalf("reasoning transcript = %#v", turn.Transcript[0])
	}
	if turn.Transcript[1].Kind != TranscriptEntryAssistant || turn.Transcript[1].Text != "Done." {
		t.Fatalf("assistant transcript = %#v", turn.Transcript[1])
	}
}

func TestProjectorSplitsReasoningTranscriptBySegmentID(t *testing.T) {
	projector := NewProjector("session-1")

	for _, event := range []Event{
		testEvent(0, "session-1", "turn-1", ReasoningDeltaPayload{Content: "step 1a", SegmentID: "seg-1"}),
		testEvent(1, "session-1", "turn-1", ReasoningDeltaPayload{Content: " + step 1b", SegmentID: "seg-1"}),
		testEvent(2, "session-1", "turn-1", ReasoningDeltaPayload{Content: "step 2", SegmentID: "seg-2"}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	turn := projector.Snapshot().Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.ReasoningText != "step 1a + step 1bstep 2" {
		t.Fatalf("reasoning text = %q", turn.ReasoningText)
	}
	if len(turn.Transcript) != 2 {
		t.Fatalf("transcript = %#v, want 2 reasoning entries", turn.Transcript)
	}
	if turn.Transcript[0].Kind != TranscriptEntryReasoning || turn.Transcript[0].Text != "step 1a + step 1b" || turn.Transcript[0].SegmentID != "seg-1" {
		t.Fatalf("turn.Transcript[0] = %#v", turn.Transcript[0])
	}
	if turn.Transcript[1].Kind != TranscriptEntryReasoning || turn.Transcript[1].Text != "step 2" || turn.Transcript[1].SegmentID != "seg-2" {
		t.Fatalf("turn.Transcript[1] = %#v", turn.Transcript[1])
	}
}

func TestProjectorContinuesReasoningSegmentAcrossSnapshot(t *testing.T) {
	projector := NewProjector("session-1")
	if err := projector.Apply(testEvent(0, "session-1", "turn-1", ReasoningDeltaPayload{
		Content:   "step 1",
		SegmentID: "seg-1",
	})); err != nil {
		t.Fatalf("Apply(initial) error = %v", err)
	}

	resumed := NewProjectorFromSnapshot(projector.Snapshot())
	if err := resumed.Apply(testEvent(1, "session-1", "turn-1", ReasoningDeltaPayload{
		Content:   " + step 2",
		SegmentID: "seg-1",
	})); err != nil {
		t.Fatalf("Apply(resumed) error = %v", err)
	}

	turn := resumed.Snapshot().Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.ReasoningText != "step 1 + step 2" {
		t.Fatalf("reasoning text = %q", turn.ReasoningText)
	}
	if len(turn.Transcript) != 1 {
		t.Fatalf("transcript = %#v, want 1 reasoning entry", turn.Transcript)
	}
	if turn.Transcript[0].Kind != TranscriptEntryReasoning || turn.Transcript[0].Text != "step 1 + step 2" || turn.Transcript[0].SegmentID != "seg-1" {
		t.Fatalf("turn.Transcript[0] = %#v", turn.Transcript[0])
	}
}

func TestProjectorSupportsMultipleTurnsInOrder(t *testing.T) {
	projector := NewProjector("session-1")

	for _, event := range []Event{
		testEvent(0, "session-1", "turn-1", AssistantCommitPayload{Content: "one"}),
		testEvent(1, "session-1", "turn-1", TurnDonePayload{}),
		testEvent(2, "session-1", "turn-2", AssistantCommitPayload{Content: "two"}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%d) error = %v", event.Sequence, err)
		}
	}

	state := projector.Snapshot()
	if len(state.TurnOrder) != 2 || state.TurnOrder[0] != "turn-1" || state.TurnOrder[1] != "turn-2" {
		t.Fatalf("turn order = %#v", state.TurnOrder)
	}
	if state.Turns["turn-2"].AssistantText != "two" {
		t.Fatalf("turn-2 assistant = %q", state.Turns["turn-2"].AssistantText)
	}
}

func TestProjectorTracksWorkspaceRootAndPermissionLifecycle(t *testing.T) {
	projector := NewProjector("session-1")

	for _, event := range []Event{
		testEvent(0, "session-1", "_session", SessionConfiguredPayload{WorkspaceRoot: "/repo"}),
		testEvent(1, "session-1", "turn-1", PermissionRequestedPayload{
			Kind:       PermissionRequestKindPath,
			RequestID:  "perm-1",
			ToolCallID: "call-1",
			Access:     "read",
			Path:       "/tmp/outside.txt",
			ToolName:   "read",
			Command:    `read {"path":"/tmp/outside.txt"}`,
			Reason:     "user requested external file",
		}),
		testEvent(2, "session-1", "turn-1", PermissionResolvedPayload{
			RequestID: "perm-1",
			Decision:  PermissionDecisionApproved,
			Scope:     PermissionScopeSession,
			GrantPath: "/tmp/outside.txt",
		}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	if state.WorkspaceRoot != "/repo" {
		t.Fatalf("workspace root = %q", state.WorkspaceRoot)
	}
	if len(state.PendingPermissions) != 0 {
		t.Fatalf("pending permissions = %#v, want empty", state.PendingPermissions)
	}
	if len(state.SessionGrantDecisions) != 1 {
		t.Fatalf("session grant decisions = %#v", state.SessionGrantDecisions)
	}
	if got := state.SessionGrantDecisions[0]; got.Source != SessionGrantDecisionSourcePermission || got.PermissionKind != PermissionRequestKindPath || got.Path != "/tmp/outside.txt" || got.ToolName != "read" {
		t.Fatalf("session grant decision = %#v", got)
	}
	if len(state.WorkspaceGrants) != 1 || state.WorkspaceGrants[0].Path != "/tmp/outside.txt" {
		t.Fatalf("workspace grants = %#v", state.WorkspaceGrants)
	}
}

func TestProjectorTracksScheduledRetryAndClearsTransientStreamState(t *testing.T) {
	projector := NewProjector("session-1")
	retryAt := time.Date(2026, 4, 22, 12, 0, 2, 0, time.UTC)

	for _, event := range []Event{
		testEvent(0, "session-1", "turn-1", UserMessagePayload{Content: "hello"}),
		testEvent(1, "session-1", "turn-1", AssistantPreviewDeltaPayload{Content: "hel"}),
		testEvent(2, "session-1", "turn-1", ToolCallDeltaPayload{CallID: "call-1", ToolName: "read", InputDelta: `{"path":"a`}),
		testEvent(3, "session-1", "turn-1", TurnRetryScheduledPayload{
			Message:     "github-copilot/gpt-5-mini: unexpected EOF",
			Attempt:     1,
			MaxAttempts: 2,
			RetryAt:     retryAt,
		}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.Retry == nil {
		t.Fatal("retry state missing")
	}
	if turn.StreamingText != "" {
		t.Fatalf("streaming text = %q, want empty", turn.StreamingText)
	}
	if len(turn.ToolCalls) != 0 {
		t.Fatalf("tool calls = %#v, want cleared transient tool call", turn.ToolCalls)
	}
	if turn.Retry.Message != "github-copilot/gpt-5-mini: unexpected EOF" {
		t.Fatalf("retry message = %q", turn.Retry.Message)
	}
	if len(turn.Transcript) != 1 || turn.Transcript[0].Kind != TranscriptEntryUser {
		t.Fatalf("transcript = %#v, want user message only", turn.Transcript)
	}
}

func TestProjectorTracksMultipleGrantPathsFromPermissionResolution(t *testing.T) {
	projector := NewProjector("session-1")

	for _, event := range []Event{
		testEvent(0, "session-1", "_session", SessionConfiguredPayload{WorkspaceRoot: "/repo"}),
		testEvent(1, "session-1", "turn-1", PermissionRequestedPayload{
			Kind:       PermissionRequestKindPath,
			RequestID:  "perm-1",
			ToolCallID: "call-1",
			Access:     "exec",
			Path:       "/opt/custom/npm",
			ToolName:   "bash",
			Command:    "npm run test",
		}),
		testEvent(2, "session-1", "turn-1", PermissionResolvedPayload{
			RequestID:  "perm-1",
			Decision:   PermissionDecisionApproved,
			Scope:      PermissionScopeSession,
			GrantPaths: []string{"/opt/custom/npm", "/opt/custom/node"},
		}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	if len(state.WorkspaceGrants) != 2 {
		t.Fatalf("workspace grants = %#v", state.WorkspaceGrants)
	}
	if state.WorkspaceGrants[0].Path != "/opt/custom/npm" || state.WorkspaceGrants[1].Path != "/opt/custom/node" {
		t.Fatalf("workspace grants = %#v", state.WorkspaceGrants)
	}
}

func TestProjectorTracksExecutionApprovalLifecycle(t *testing.T) {
	projector := NewProjector("session-1")

	for _, event := range []Event{
		testEvent(0, "session-1", "_session", SessionConfiguredPayload{WorkspaceRoot: "/repo"}),
		testEvent(1, "session-1", "turn-1", ExecutionApprovalRequestedPayload{
			RequestID:          "perm-1",
			ExecutionID:        "exec-call-1",
			ToolCallID:         "call-1",
			ToolName:           "bash",
			Command:            "npm test",
			WorkingDirectory:   "/outside",
			Reason:             "requires approval for command execution",
			AvailableDecisions: []ExecutionApprovalDecision{ExecutionApprovalDecisionAccept, ExecutionApprovalDecisionAcceptForSession, ExecutionApprovalDecisionDecline},
		}),
		testEvent(2, "session-1", "turn-1", ExecutionApprovalResolvedPayload{
			RequestID:       "perm-1",
			Decision:        ExecutionApprovalDecisionAcceptForSession,
			GrantPrefixRule: []string{"npm test"},
			GrantPaths:      []string{"/outside", "/usr/local/bin/node"},
		}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	if len(state.SessionGrantDecisions) != 1 {
		t.Fatalf("session grant decisions = %#v", state.SessionGrantDecisions)
	}
	if got := state.SessionGrantDecisions[0]; got.Source != SessionGrantDecisionSourceExecutionApproval || got.Command != "npm test" || got.ToolName != "bash" {
		t.Fatalf("session grant decision = %#v", got)
	}
	if len(state.PendingPermissions) != 0 {
		t.Fatalf("pending permissions = %#v, want empty", state.PendingPermissions)
	}
	if len(state.WorkspaceGrants) != 2 {
		t.Fatalf("workspace grants = %#v", state.WorkspaceGrants)
	}
	if state.WorkspaceGrants[0].Path != "/outside" || state.WorkspaceGrants[1].Path != "/usr/local/bin/node" {
		t.Fatalf("workspace grants = %#v", state.WorkspaceGrants)
	}
	if len(state.ExecutionGrants) != 1 {
		t.Fatalf("execution grants = %#v", state.ExecutionGrants)
	}
	if got := state.ExecutionGrants[0]; len(got.SessionPaths) != 2 || got.SessionPaths[0] != "/outside" || got.SessionPaths[1] != "/usr/local/bin/node" {
		t.Fatalf("execution grant = %#v", got)
	}
}

func TestProjectorTracksOnceExecutionApprovalDecisionHistory(t *testing.T) {
	projector := NewProjector("session-1")

	for _, event := range []Event{
		testEvent(0, "session-1", "_session", SessionConfiguredPayload{WorkspaceRoot: "/repo"}),
		testEvent(1, "session-1", "turn-1", ExecutionApprovalRequestedPayload{
			RequestID:          "perm-1",
			ExecutionID:        "exec-call-1",
			ToolCallID:         "call-1",
			ToolName:           "bash",
			Command:            "ls -la $HOME",
			WorkingDirectory:   "/repo",
			AvailableDecisions: []ExecutionApprovalDecision{ExecutionApprovalDecisionAccept, ExecutionApprovalDecisionAcceptForSession, ExecutionApprovalDecisionDecline},
		}),
		testEvent(2, "session-1", "turn-1", ExecutionApprovalResolvedPayload{
			RequestID: "perm-1",
			Decision:  ExecutionApprovalDecisionAccept,
		}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	if len(state.SessionGrantDecisions) != 1 {
		t.Fatalf("session grant decisions = %#v", state.SessionGrantDecisions)
	}
	if got := state.SessionGrantDecisions[0]; got.Command != "ls -la $HOME" || got.Source != SessionGrantDecisionSourceExecutionApproval {
		t.Fatalf("session grant decision = %#v", got)
	}
}

func TestProjectorTracksAndConsumesApprovedExecutionResumeState(t *testing.T) {
	projector := NewProjector("session-1")

	for _, event := range []Event{
		testEvent(0, "session-1", "_session", SessionConfiguredPayload{WorkspaceRoot: "/repo"}),
		testEvent(1, "session-1", "turn-1", ExecutionApprovalRequestedPayload{
			RequestID:          "perm-1",
			ExecutionID:        "exec-call-1",
			ToolCallID:         "call-1",
			ToolName:           "bash",
			Command:            "npm run dev",
			WorkingDirectory:   "/repo/client",
			AvailableDecisions: []ExecutionApprovalDecision{ExecutionApprovalDecisionAccept, ExecutionApprovalDecisionDecline},
		}),
		testEvent(2, "session-1", "turn-1", ExecutionApprovalResolvedPayload{
			RequestID: "perm-1",
			Decision:  ExecutionApprovalDecisionAccept,
		}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	approved := state.ApprovedExecutions["exec-call-1"]
	if approved == nil {
		t.Fatalf("approved executions = %#v", state.ApprovedExecutions)
	}
	if approved.Command != "npm run dev" || approved.ToolCallID != "call-1" || approved.TurnID != "turn-1" {
		t.Fatalf("approved execution = %#v", approved)
	}

	if err := projector.Apply(testEvent(3, "session-1", "turn-1", ExecutionStartedPayload{
		ExecutionID: "exec-call-1",
		ToolCallID:  "call-1",
		ToolName:    "bash",
		Input:       `{"cmd":"npm run dev","workdir":"client"}`,
	})); err != nil {
		t.Fatalf("Apply(execution_started) error = %v", err)
	}

	state = projector.Snapshot()
	if len(state.ApprovedExecutions) != 0 {
		t.Fatalf("approved executions after start = %#v", state.ApprovedExecutions)
	}
}

func TestProjectorTracksOncePermissionDecisionHistory(t *testing.T) {
	projector := NewProjector("session-1")

	for _, event := range []Event{
		testEvent(0, "session-1", "_session", SessionConfiguredPayload{WorkspaceRoot: "/repo"}),
		testEvent(1, "session-1", "turn-1", PermissionRequestedPayload{
			Kind:       PermissionRequestKindPath,
			RequestID:  "perm-1",
			ToolCallID: "call-1",
			Access:     "read",
			Path:       "/tmp/outside.txt",
			ToolName:   "read",
			Command:    `read {"path":"/tmp/outside.txt"}`,
		}),
		testEvent(2, "session-1", "turn-1", PermissionResolvedPayload{
			RequestID: "perm-1",
			Decision:  PermissionDecisionApproved,
			Scope:     PermissionScopeOnce,
		}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	if len(state.SessionGrantDecisions) != 1 {
		t.Fatalf("session grant decisions = %#v", state.SessionGrantDecisions)
	}
	if got := state.SessionGrantDecisions[0]; got.Path != "/tmp/outside.txt" || got.Source != SessionGrantDecisionSourcePermission {
		t.Fatalf("session grant decision = %#v", got)
	}
}

func TestProjectorClearsExecutionOutputWhenExecutionRestarts(t *testing.T) {
	projector := NewProjector("session-1")

	for _, event := range []Event{
		testEvent(0, "session-1", "turn-1", ExecutionDeclaredPayload{
			ExecutionID:      "exec-call-1",
			ToolCallID:       "call-1",
			ToolName:         "bash",
			Kind:             "bash",
			Command:          []string{"./probe.sh"},
			CommandPreview:   "./probe.sh",
			WorkingDirectory: "/repo",
			TimeoutMS:        120000,
			OutputLimit:      12000,
		}),
		testEvent(1, "session-1", "turn-1", ExecutionStartedPayload{
			ExecutionID: "exec-call-1",
			ToolCallID:  "call-1",
			ToolName:    "bash",
			Input:       `{"cmd":"./probe.sh"}`,
		}),
		testEvent(2, "session-1", "turn-1", ExecutionOutputPayload{
			ExecutionID: "exec-call-1",
			ToolCallID:  "call-1",
			Stream:      "combined",
			Chunk:       "temporary failure in name resolution\n",
		}),
		testEvent(3, "session-1", "turn-1", ExecutionStartedPayload{
			ExecutionID: "exec-call-1",
			ToolCallID:  "call-1",
			ToolName:    "bash",
			Input:       `{"cmd":"./probe.sh"}`,
		}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	if call == nil || call.Execution == nil {
		t.Fatal("execution state missing")
	}
	if call.Output != "" || call.Error != "" {
		t.Fatalf("call output/error = %q / %q", call.Output, call.Error)
	}
	if call.Execution.Output != "" || call.Execution.Error != "" {
		t.Fatalf("execution output/error = %q / %q", call.Execution.Output, call.Execution.Error)
	}
}

func TestProjectorTracksBackgroundExecutionLifecycle(t *testing.T) {
	projector := NewProjector("session-1")

	for _, event := range []Event{
		testEvent(0, "session-1", "turn-1", ExecutionDeclaredPayload{
			ExecutionID:      "exec-call-1",
			ToolCallID:       "call-1",
			ToolName:         "bash",
			Kind:             "bash",
			Intent:           "server",
			Command:          []string{"/bin/sh", "-c", "npm run dev"},
			CommandPreview:   "npm run dev",
			WorkingDirectory: "/repo/client",
			TimeoutMS:        120000,
			OutputLimit:      12000,
		}),
		testEvent(1, "session-1", "turn-1", ExecutionStartedPayload{
			ExecutionID: "exec-call-1",
			ToolCallID:  "call-1",
			ToolName:    "bash",
			Input:       `{"cmd":"npm run dev","workdir":"client"}`,
		}),
		testEvent(2, "session-1", "turn-1", ExecutionBackgroundStartedPayload{
			ExecutionID:     "exec-call-1",
			ToolCallID:      "call-1",
			ToolName:        "bash",
			PID:             4242,
			ProcessIdentity: "identity-1",
			SupervisorID:    "background-supervisor-1",
			LogRef:          "session-1/turn-1/exec-call-1.log",
			ReadyPatterns:   []string{"local:"},
		}),
		testEvent(3, "session-1", "turn-1", ExecutionBackgroundObservedPayload{
			ExecutionID: "exec-call-1",
			ToolCallID:  "call-1",
			ToolName:    "bash",
			OutputTail:  "Local: http://127.0.0.1:5173/\n",
			OutputBytes: 29,
		}),
		testEvent(4, "session-1", "turn-1", ExecutionBackgroundReadyPayload{
			ExecutionID: "exec-call-1",
			ToolCallID:  "call-1",
			ToolName:    "bash",
			Message:     "Local: http://127.0.0.1:5173/",
			Port:        5173,
		}),
		testEvent(5, "session-1", "turn-1", ToolExecEndPayload{
			CallID:          "call-1",
			ToolName:        "bash",
			ExecutionID:     "exec-call-1",
			ExecutionStatus: "completed",
			Succeeded:       true,
			Output:          "Started server in background (pid 4242).",
		}),
		testEvent(6, "session-1", "turn-1", ExecutionBackgroundExitedPayload{
			ExecutionID: "exec-call-1",
			ToolCallID:  "call-1",
			ToolName:    "bash",
			ExitCode:    intPointer(0),
		}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	if call == nil || call.Execution == nil || call.Execution.Background == nil {
		t.Fatalf("call = %#v", call)
	}
	if call.Execution.Intent != "server" {
		t.Fatalf("intent = %q", call.Execution.Intent)
	}
	background := call.Execution.Background
	if !background.Started || !background.Ready || !background.Exited {
		t.Fatalf("background = %#v", background)
	}
	if background.PID != 4242 || background.Port != 5173 {
		t.Fatalf("background = %#v", background)
	}
	if background.ProcessIdentity != "identity-1" {
		t.Fatalf("background = %#v", background)
	}
	if background.SupervisorID != "background-supervisor-1" {
		t.Fatalf("background = %#v", background)
	}
	if background.LogRef != "session-1/turn-1/exec-call-1.log" {
		t.Fatalf("background = %#v", background)
	}
	if background.OutputBytes != 29 || strings.TrimSpace(background.OutputTail) != "Local: http://127.0.0.1:5173/" {
		t.Fatalf("background = %#v", background)
	}
}

func TestProjectorStoresSessionTitle(t *testing.T) {
	projector := NewProjector("session-1")

	if err := projector.Apply(testEvent(0, "session-1", "_session", SessionTitleUpdatedPayload{
		Title: "terminal shell · split operator",
	})); err != nil {
		t.Fatalf("Apply(session_title_updated) error = %v", err)
	}

	state := projector.Snapshot()
	if got := state.Title; got != "terminal shell · split operator" {
		t.Fatalf("title = %q", got)
	}
}

func TestProjectorStoresCompiledPromptState(t *testing.T) {
	projector := NewProjector("session-1")

	if err := projector.Apply(testEvent(0, "session-1", "turn-1", PromptCompiledPayload{
		Shape:            "generic",
		BaseInstructions: "base instructions",
		Instructions:     "compiled instructions",
		Fragments: []PromptFragmentPayload{
			{Kind: "policy", Source: "builtin", Stability: "stable", Layer: "core-policy", Key: "core-policy", Label: "core-policy", Bytes: 10},
			{Kind: "role", Source: "builtin", Stability: "stable", Layer: "agent-prompt", Key: "agent:builder", Label: "builder", Bytes: 12},
		},
	})); err != nil {
		t.Fatalf("Apply(prompt_compiled) error = %v", err)
	}

	state := projector.Snapshot()
	turn := state.Turns["turn-1"]
	if turn == nil || turn.Prompt == nil {
		t.Fatal("prompt state missing")
	}
	if got := turn.Prompt.Shape; got != "generic" {
		t.Fatalf("shape = %q", got)
	}
	if got := turn.Prompt.BaseInstructions; got != "base instructions" {
		t.Fatalf("base instructions = %q", got)
	}
	if got := turn.Prompt.Instructions; got != "compiled instructions" {
		t.Fatalf("instructions = %q", got)
	}
	if len(turn.Prompt.Fragments) != 2 {
		t.Fatalf("fragment count = %d", len(turn.Prompt.Fragments))
	}
	if got := turn.Prompt.Fragments[1].Label; got != "builder" {
		t.Fatalf("second fragment label = %q", got)
	}
	if len(turn.Prompt.Layers) != 2 {
		t.Fatalf("layer count = %d", len(turn.Prompt.Layers))
	}
	if got := turn.Prompt.Layers[1].Name; got != "agent-prompt" {
		t.Fatalf("second layer name = %q", got)
	}
}

func TestProjectorStoresPruningState(t *testing.T) {
	projector := NewProjector("session-1")

	if err := projector.Apply(testEvent(0, "session-1", "turn-2", ContextPrunedPayload{
		PriorTurns:          3,
		PriorInputBytes:     1024,
		RawPriorTurns:       2,
		RawInputBytes:       320,
		CompactedPriorTurns: 1,
		CompactedInputBytes: 96,
		OmittedPriorTurns:   0,
		OmittedInputBytes:   704,
	})); err != nil {
		t.Fatalf("Apply(context_pruned) error = %v", err)
	}

	state := projector.Snapshot()
	turn := state.Turns["turn-2"]
	if turn == nil || turn.Pruning == nil {
		t.Fatal("pruning state missing")
	}
	if got := turn.Pruning.CompactedInputBytes; got != 96 {
		t.Fatalf("compacted input bytes = %d", got)
	}
}

func TestProjectorStoresCompactionState(t *testing.T) {
	projector := NewProjector("session-1")

	if err := projector.Apply(testEvent(0, "session-1", "turn-3", SessionHistoryContinuationUpdatedPayload{
		EventVersion:               1,
		ArtifactVersion:            1,
		RendererVersion:            1,
		FrontierTurnID:             "turn-2",
		ConsolidatedTurnCount:      2,
		NewlyConsolidatedTurnCount: 1,
		UpdateReason:               HistoryContinuationUpdateReasonTokenPressure,
		Artifact: HistoryContinuationArtifact{
			WorkspaceFacts: []HistoryWorkspaceFactPayload{
				{Path: "src/old.ts", Fact: "Older work compacted", SourceTurnID: "turn-1"},
				{Path: "src/new.ts", Fact: "Newer work compacted", SourceTurnID: "turn-2"},
			},
		},
		RenderedSummary: "Compacted older turns.",
		InputBudget: &HistoryInputBudgetPayload{
			InputLimitTokens:          3072,
			TriggerTokens:             3072,
			TargetTokens:              2048,
			EstimatedRequestTokens:    4200,
			ConsolidatedRequestTokens: 1900,
		},
		Attribution: HistoryContinuationAttribution{
			Model:             "openai/gpt-5",
			InputLimitSource:  "history_budget_bytes",
			MeasurementSource: "exact",
			SummarySource:     "utility",
		},
	})); err != nil {
		t.Fatalf("Apply(context_compacted) error = %v", err)
	}

	state := projector.Snapshot()
	turn := state.Turns["turn-3"]
	if turn == nil || turn.Continuation == nil {
		t.Fatal("compaction state missing")
	}
	if got := turn.Continuation.UpdateReason; got != HistoryContinuationUpdateReasonTokenPressure {
		t.Fatalf("update reason = %q", got)
	}
	if got := turn.Continuation.ConsolidatedTurnCount; got != 2 {
		t.Fatalf("compaction turn count = %d", got)
	}
	if got := turn.Continuation.FrontierTurnID; got != "turn-2" {
		t.Fatalf("compaction cutoff = %q", got)
	}
	if got := turn.Continuation.NewlyConsolidatedTurnCount; got != 1 {
		t.Fatalf("newly compacted turn count = %d", got)
	}
	if got := turn.Continuation.Artifact.WorkspaceFacts; len(got) != 2 || got[0].Path != "src/old.ts" || got[1].Path != "src/new.ts" {
		t.Fatalf("workspace facts = %#v", got)
	}
	if got := turn.Continuation.Attribution.MeasurementSource; got != "exact" {
		t.Fatalf("measurement source = %q", got)
	}
	if got := turn.Continuation.Attribution.SummarySource; got != "utility" {
		t.Fatalf("summary source = %q", got)
	}
	if got := turn.Continuation.RenderedSummary; got != "Compacted older turns." {
		t.Fatalf("compaction summary = %q", got)
	}
	if got := len(turn.Transcript); got != 1 {
		t.Fatalf("transcript entries = %d, want 1 compaction entry", got)
	}
	if got := turn.Transcript[0].Kind; got != TranscriptEntryCompaction {
		t.Fatalf("transcript[0].Kind = %q, want %q", got, TranscriptEntryCompaction)
	}
	if got := turn.Transcript[0].Text; got != "Compacted older turns." {
		t.Fatalf("transcript[0].Text = %q, want compaction summary", got)
	}
}

func TestProjectorTracksCompactionAttemptState(t *testing.T) {
	projector := NewProjector("session-1")

	if err := projector.Apply(testEvent(0, "session-1", "turn-3", ContextCompactionStartedPayload{
		Scope:                  CompactionScopeHistory,
		InputLimitTokens:       3072,
		TriggerTokens:          2560,
		TargetTokens:           2048,
		EstimatedRequestTokens: 4200,
	})); err != nil {
		t.Fatalf("Apply(context_compaction_started) error = %v", err)
	}

	state := projector.Snapshot()
	turn := state.Turns["turn-3"]
	if turn == nil || turn.CompactionAttempt == nil {
		t.Fatal("compaction attempt state missing")
	}
	if got := turn.CompactionAttempt.Scope; got != CompactionScopeHistory {
		t.Fatalf("compaction attempt scope = %q", got)
	}

	if err := projector.Apply(testEvent(1, "session-1", "turn-3", SessionHistoryContinuationUpdatedPayload{
		EventVersion:               1,
		ArtifactVersion:            1,
		RendererVersion:            1,
		FrontierTurnID:             "turn-2",
		ConsolidatedTurnCount:      2,
		NewlyConsolidatedTurnCount: 1,
		UpdateReason:               HistoryContinuationUpdateReasonTokenPressure,
		Artifact: HistoryContinuationArtifact{
			WorkspaceFacts: []HistoryWorkspaceFactPayload{
				{Path: "src/old.ts", Fact: "Older work compacted", SourceTurnID: "turn-1"},
				{Path: "src/new.ts", Fact: "Newer work compacted", SourceTurnID: "turn-2"},
			},
		},
		RenderedSummary: "Compacted older turns.",
		InputBudget: &HistoryInputBudgetPayload{
			InputLimitTokens:          3072,
			TriggerTokens:             3072,
			TargetTokens:              2048,
			EstimatedRequestTokens:    4200,
			ConsolidatedRequestTokens: 1900,
		},
		Attribution: HistoryContinuationAttribution{
			Model:             "openai/gpt-5",
			InputLimitSource:  "history_budget_bytes",
			MeasurementSource: "estimated",
			SummarySource:     "runtime",
		},
	})); err != nil {
		t.Fatalf("Apply(context_compacted) error = %v", err)
	}

	state = projector.Snapshot()
	turn = state.Turns["turn-3"]
	if turn == nil {
		t.Fatal("turn state missing after compaction")
	}
	if turn.CompactionAttempt != nil {
		t.Fatalf("compaction attempt = %#v, want nil after context_compacted", turn.CompactionAttempt)
	}
	if turn.Continuation == nil {
		t.Fatal("compaction state missing after context_compacted")
	}
	if got := turn.Continuation.NewlyConsolidatedTurnCount; got != 1 {
		t.Fatalf("newly compacted turn count = %d", got)
	}
}

func TestProjectorClearsCompactionAttemptStateOnFailure(t *testing.T) {
	projector := NewProjector("session-1")

	if err := projector.Apply(testEvent(0, "session-1", "turn-3", ContextCompactionStartedPayload{
		Scope:                  CompactionScopeHistory,
		InputLimitTokens:       3072,
		TriggerTokens:          2560,
		TargetTokens:           2048,
		EstimatedRequestTokens: 4200,
	})); err != nil {
		t.Fatalf("Apply(context_compaction_started) error = %v", err)
	}
	if err := projector.Apply(testEvent(1, "session-1", "turn-3", ContextCompactionFailedPayload{
		Scope:                  CompactionScopeHistory,
		Reason:                 "artifact_generation_failed",
		Detail:                 "context deadline exceeded",
		InputLimitTokens:       3072,
		TriggerTokens:          2560,
		TargetTokens:           2048,
		EstimatedRequestTokens: 4200,
	})); err != nil {
		t.Fatalf("Apply(context_compaction_failed) error = %v", err)
	}

	state := projector.Snapshot()
	turn := state.Turns["turn-3"]
	if turn == nil {
		t.Fatal("turn state missing after compaction failure")
	}
	if turn.CompactionAttempt != nil {
		t.Fatalf("compaction attempt = %#v, want nil after context_compaction_failed", turn.CompactionAttempt)
	}
	if turn.CompactionFailure == nil {
		t.Fatal("compaction failure missing after context_compaction_failed")
	}
	if got := turn.CompactionFailure.Reason; got != "artifact_generation_failed" {
		t.Fatalf("compaction failure reason = %q", got)
	}
	if turn.Continuation != nil {
		t.Fatalf("compaction = %#v, want nil after failed compaction", turn.Continuation)
	}
}

func TestProjectorStoresAgentHandoffState(t *testing.T) {
	projector := NewProjector("session-1")

	if err := projector.Apply(testEvent(0, "session-1", "turn-1", ToolCallDeclaredPayload{
		CallID:   "call-1",
		ToolName: "delegate",
		Input:    `{"agent_id":"planner","task":"analyze","context_summary":"Read the current implementation."}`,
	})); err != nil {
		t.Fatalf("Apply(tool_call_declared) error = %v", err)
	}
	if err := projector.Apply(testEvent(1, "session-1", "turn-1", AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ToolCallID:      "call-1",
		ParentSessionID: "session-1",
		ParentTurnID:    "turn-1",
		ParentAgentID:   "builder",
		ChildSessionID:  "session-2",
		ChildTurnID:     "turn-2",
		ChildAgentID:    "planner",
		Task:            "analyze",
		ContextSummary:  "Read the current implementation.",
		ExplorationEntries: []AgentHandoffExplorationEntry{{
			ToolName: "read",
			Target:   "read:path=app.go:start_line=1:max_lines=200",
			Summary:  "read app.go -> 1: package main",
		}},
		Model:        "openai/gpt-5-mini",
		AllowedTools: []string{"read", "search"},
	})); err != nil {
		t.Fatalf("Apply(agent_handoff) error = %v", err)
	}

	state := projector.Snapshot()
	turn := state.Turns["turn-1"]
	if turn == nil || len(turn.HandoffOrder) != 1 {
		t.Fatalf("handoff order = %#v", turn)
	}
	handoff := turn.Handoffs["handoff-1"]
	if handoff == nil {
		t.Fatal("handoff state missing")
	}
	if handoff.ChildSessionID != "session-2" || handoff.ChildAgentID != "planner" || handoff.ToolCallID != "call-1" {
		t.Fatalf("handoff = %#v", handoff)
	}
	if len(handoff.ExplorationEntries) != 1 || handoff.ExplorationEntries[0].Target != "read:path=app.go:start_line=1:max_lines=200" {
		t.Fatalf("handoff exploration entries = %#v", handoff.ExplorationEntries)
	}
	call := turn.ToolCalls["call-1"]
	if call == nil || call.HandoffID != "handoff-1" {
		t.Fatalf("call state = %#v, want handoff-1", call)
	}
}

func TestProjectorUpdatesAgentHandoffResultState(t *testing.T) {
	projector := NewProjector("session-1")

	events := []Event{
		testEvent(0, "session-1", "turn-1", AgentHandoffPayload{
			HandoffID:       "handoff-1",
			ParentSessionID: "session-1",
			ParentTurnID:    "turn-1",
			ParentAgentID:   "builder",
			ChildSessionID:  "session-2",
			ChildTurnID:     "turn-2",
			ChildAgentID:    "planner",
			Task:            "analyze",
			ContextSummary:  "Read the current implementation.",
			Model:           "openai/gpt-5-mini",
			AllowedTools:    []string{"read", "search"},
		}),
		testEvent(1, "session-1", "turn-1", AgentHandoffPreviewPayload{
			HandoffID:      "handoff-1",
			ChildSessionID: "session-2",
			ChildTurnID:    "turn-2",
			Active:         true,
			ToolName:       "read",
			Action:         "running read",
			AssistantText:  "Scanning runtime_delegate.go",
		}),
		testEvent(2, "session-1", "turn-1", AgentResultPayload{
			HandoffID:           "handoff-1",
			ChildSessionID:      "session-2",
			ChildTurnID:         "turn-2",
			Status:              AgentResultStatusPendingPermission,
			AssistantText:       "Need approval first.",
			PermissionKind:      PermissionRequestKindPath,
			PermissionRequestID: "perm-1",
			PermissionToolName:  "read",
			PermissionAccess:    "read",
			PermissionPath:      "/tmp/outside.txt",
			PermissionCommand:   `read {"path":"/tmp/outside.txt"}`,
		}),
		testEvent(3, "session-1", "turn-1", AgentResultPayload{
			HandoffID:      "handoff-1",
			ChildSessionID: "session-2",
			ChildTurnID:    "turn-2",
			Status:         AgentResultStatusCompleted,
			AssistantText:  "Done.",
		}),
	}
	for _, event := range events {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	handoff := state.Turns["turn-1"].Handoffs["handoff-1"]
	if handoff == nil {
		t.Fatal("handoff state missing")
	}
	if handoff.Status != AgentResultStatusCompleted || handoff.AssistantText != "Done." {
		t.Fatalf("handoff result = %#v", handoff)
	}
	if handoff.PreviewActive || handoff.PreviewToolName != "" || handoff.PreviewAction != "" || handoff.PreviewAssistantText != "" {
		t.Fatalf("preview state = %#v, want cleared on completion", handoff)
	}
	if handoff.PermissionRequestID != "" {
		t.Fatalf("permission request id = %q, want cleared on completion", handoff.PermissionRequestID)
	}
	if handoff.PermissionCommand != "" || handoff.PermissionPath != "" {
		t.Fatalf("permission details = %#v, want cleared on completion", handoff)
	}
}

func TestProjectorStoresAgentHandoffPreviewState(t *testing.T) {
	projector := NewProjector("session-1")

	for _, event := range []Event{
		testEvent(0, "session-1", "turn-1", AgentHandoffPayload{
			HandoffID:       "handoff-1",
			ParentSessionID: "session-1",
			ParentTurnID:    "turn-1",
			ParentAgentID:   "builder",
			ChildSessionID:  "session-2",
			ChildTurnID:     "turn-2",
			ChildAgentID:    "planner",
			Task:            "analyze",
			ContextSummary:  "Read the current implementation.",
			Model:           "openai/gpt-5-mini",
			AllowedTools:    []string{"read", "search"},
		}),
		testEvent(1, "session-1", "turn-1", AgentHandoffPreviewPayload{
			HandoffID:      "handoff-1",
			ChildSessionID: "session-2",
			ChildTurnID:    "turn-2",
			Active:         true,
			ToolName:       "search",
			Action:         "running search",
			AssistantText:  "Looking for runtime delegate entry points.",
		}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	handoff := state.Turns["turn-1"].Handoffs["handoff-1"]
	if handoff == nil {
		t.Fatal("handoff state missing")
	}
	if !handoff.PreviewActive || handoff.PreviewToolName != "search" || handoff.PreviewAction != "running search" {
		t.Fatalf("preview state = %#v", handoff)
	}
	if handoff.PreviewAssistantText != "Looking for runtime delegate entry points." {
		t.Fatalf("preview assistant text = %q", handoff.PreviewAssistantText)
	}
}

func TestProjectorTracksDelegatedExecutionApprovalState(t *testing.T) {
	projector := NewProjector("session-1")
	allowLoginShell := true

	for _, event := range []Event{
		testEvent(0, "session-1", "turn-1", AgentHandoffPayload{
			HandoffID:       "handoff-1",
			ParentSessionID: "session-1",
			ParentTurnID:    "turn-1",
			ParentAgentID:   "builder",
			ChildSessionID:  "session-2",
			ChildTurnID:     "turn-2",
			ChildAgentID:    "planner",
			Task:            "launch the server",
			ContextSummary:  "Start the local dev server.",
			Model:           "openai/gpt-5-mini",
			AllowedTools:    []string{"bash"},
		}),
		testEvent(1, "session-1", "turn-1", AgentResultPayload{
			HandoffID:           "handoff-1",
			ChildSessionID:      "session-2",
			ChildTurnID:         "turn-2",
			Status:              AgentResultStatusPendingPermission,
			AssistantText:       "Need approval first.",
			PermissionKind:      PermissionRequestKindExecution,
			PermissionRequestID: "perm-1",
			PermissionToolName:  "bash",
			PermissionDir:       "/repo/client",
			PermissionCommand:   `bash {"cmd":"npm run dev","workdir":"client"}`,
			ExecutionApproval: &ExecutionApprovalState{
				RequestID:          "perm-1",
				ExecutionID:        "exec-1",
				TurnID:             "turn-2",
				ToolCallID:         "call-1",
				ToolName:           "bash",
				Command:            `bash {"cmd":"npm run dev","workdir":"client"}`,
				WorkingDirectory:   "/repo/client",
				AvailableDecisions: []ExecutionApprovalDecision{ExecutionApprovalDecisionAccept, ExecutionApprovalDecisionAcceptForSession, ExecutionApprovalDecisionAcceptWithExecPolicy, ExecutionApprovalDecisionDecline},
				ProposedExecPolicy: &ExecutionPolicyAmendment{AllowLoginShell: &allowLoginShell},
			},
		}),
		testEvent(2, "session-1", "turn-1", AgentResultPayload{
			HandoffID:      "handoff-1",
			ChildSessionID: "session-2",
			ChildTurnID:    "turn-2",
			Status:         AgentResultStatusCompleted,
			AssistantText:  "Done.",
		}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	handoff := state.Turns["turn-1"].Handoffs["handoff-1"]
	if handoff == nil {
		t.Fatal("handoff state missing")
	}
	if handoff.Status != AgentResultStatusCompleted {
		t.Fatalf("handoff status = %q", handoff.Status)
	}
	if handoff.ExecutionApproval != nil {
		t.Fatalf("execution approval = %#v, want cleared on completion", handoff.ExecutionApproval)
	}
}

func TestProjectorDoesNotPersistWorkspaceGrantForOnceApproval(t *testing.T) {
	projector := NewProjector("session-1")

	for _, event := range []Event{
		testEvent(0, "session-1", "_session", SessionConfiguredPayload{WorkspaceRoot: "/repo"}),
		testEvent(1, "session-1", "turn-1", PermissionRequestedPayload{
			Kind:       PermissionRequestKindPath,
			RequestID:  "perm-1",
			ToolCallID: "call-1",
			Access:     "read",
			Path:       "/tmp/outside.txt",
			ToolName:   "read",
			Command:    `read {"path":"/tmp/outside.txt"}`,
		}),
		testEvent(2, "session-1", "turn-1", PermissionResolvedPayload{
			RequestID: "perm-1",
			Decision:  PermissionDecisionApproved,
			Scope:     PermissionScopeOnce,
		}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	if len(state.WorkspaceGrants) != 0 {
		t.Fatalf("workspace grants = %#v, want none", state.WorkspaceGrants)
	}
}

func testEvent(sequence int64, sessionID, turnID string, payload Payload) Event {
	return Event{
		ID:        sessionID,
		SessionID: sessionID,
		TurnID:    turnID,
		Sequence:  sequence,
		Time:      time.Unix(sequence, 0).UTC(),
		Type:      payload.eventType(),
		Payload:   payload,
	}
}

func intPointer(value int) *int {
	return &value
}
