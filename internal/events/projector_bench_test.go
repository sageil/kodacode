package events

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func BenchmarkProjectorSnapshotLargeSession(b *testing.B) {
	projector := NewProjectorFromSnapshot(benchmarkProjectorState(48, 4))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = projector.Snapshot()
	}
}

func BenchmarkAppendExecutionOutputTail(b *testing.B) {
	asciiCurrent := strings.Repeat("a", 4096)
	asciiChunk := strings.Repeat("b", 2048)
	utf8Current := strings.Repeat("界", 5000)
	utf8Chunk := strings.Repeat("測", 5000)

	b.Run("under_limit_ascii", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = appendExecutionOutputTail(asciiCurrent, asciiChunk, 8192)
		}
	})

	b.Run("over_limit_utf8", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = appendExecutionOutputTail(utf8Current, utf8Chunk, 8192)
		}
	})
}

func BenchmarkProjectorApplyReasoningDeltaBatch(b *testing.B) {
	const fragmentsPerIteration = 100
	fragment := strings.Repeat("reasoning-", 12)
	eventTime := time.Unix(1700000000, 0).UTC()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		projector := NewProjector("session-1")
		for j := 0; j < fragmentsPerIteration; j++ {
			if err := projector.Apply(Event{
				SessionID: "session-1",
				TurnID:    "turn-1",
				Sequence:  int64(j),
				Time:      eventTime,
				Type:      TypeReasoningDelta,
				Payload: ReasoningDeltaPayload{
					Content:   fragment,
					SegmentID: "seg-1",
				},
			}); err != nil {
				b.Fatalf("Apply() error = %v", err)
			}
		}
	}
}

func benchmarkProjectorState(turnCount, toolCallsPerTurn int) SessionState {
	state := SessionState{
		SessionID:             "session-bench",
		WorkspaceRoot:         "/workspace",
		PermissionMode:        "auto",
		TaskOrder:             []string{"task-1", "task-2"},
		Tasks:                 map[string]*TaskState{},
		ApprovedExecutions:    map[string]*ApprovedExecutionState{},
		PendingExecutions:     map[string]*ExecutionApprovalState{},
		PendingPermissions:    map[string]*PermissionRequestState{},
		PendingQuestions:      map[string]*QuestionRequestState{},
		QuestionAnswers:       map[string]*QuestionAnswerState{},
		TurnOrder:             make([]string, 0, turnCount),
		Turns:                 make(map[string]*TurnState, turnCount),
		SessionGrantDecisions: []SessionGrantDecisionState{{Source: SessionGrantDecisionSourcePermission, ToolName: "read", Path: "notes.txt", ResolvedAtSeq: 7}},
		WorkspaceGrants:       []WorkspaceGrantState{{Path: "/workspace", Recursive: true}},
		ExecutionGrants:       []ExecutionGrantState{{PrefixRule: []string{"go", "test"}, SessionPaths: []string{"/workspace"}}},
		NetworkGrants:         []NetworkGrantState{{Target: "api.openai.com"}},
		LastSequence:          int64(turnCount*toolCallsPerTurn*4 + turnCount),
	}

	inputText := strings.Repeat("input-segment-", 32)
	outputText := strings.Repeat("output-segment-", 48)
	reasoningText := strings.Repeat("reasoning-segment-", 24)

	for turnIndex := 0; turnIndex < turnCount; turnIndex++ {
		turnID := fmt.Sprintf("turn-%d", turnIndex)
		turn := &TurnState{
			TurnID:           turnID,
			Status:           TurnStatusCompleted,
			UserText:         fmt.Sprintf("user prompt %d %s", turnIndex, inputText),
			AssistantText:    fmt.Sprintf("assistant response %d %s", turnIndex, outputText),
			ReasoningText:    reasoningText,
			Transcript:       make([]TranscriptEntryState, 0, 3+toolCallsPerTurn),
			ToolCallOrder:    make([]string, 0, toolCallsPerTurn),
			ToolCalls:        make(map[string]*ToolCallState, toolCallsPerTurn),
			CompletedAtSeq:   int64(turnIndex*100 + 99),
			LastUpdatedAtSeq: int64(turnIndex*100 + 99),
		}
		turn.Transcript = append(turn.Transcript,
			TranscriptEntryState{Kind: TranscriptEntryUser, Sequence: int64(turnIndex * 100), Text: turn.UserText},
			TranscriptEntryState{Kind: TranscriptEntryReasoning, Sequence: int64(turnIndex*100 + 1), Text: reasoningText, SegmentID: "seg-1"},
			TranscriptEntryState{Kind: TranscriptEntryAssistant, Sequence: int64(turnIndex*100 + 2), Text: turn.AssistantText},
		)

		for callIndex := 0; callIndex < toolCallsPerTurn; callIndex++ {
			callID := fmt.Sprintf("%s-call-%d", turnID, callIndex)
			turn.ToolCallOrder = append(turn.ToolCallOrder, callID)
			turn.Transcript = append(turn.Transcript, TranscriptEntryState{
				Kind:     TranscriptEntryTool,
				Sequence: int64(turnIndex*100 + 10 + callIndex),
				Text:     outputText,
				CallID:   callID,
			})
			turn.ToolCalls[callID] = &ToolCallState{
				CallID:    callID,
				ToolName:  "bash",
				Input:     inputText,
				Output:    outputText,
				Declared:  true,
				Executing: false,
				Completed: true,
				Execution: &ExecutionState{
					ExecutionID:      callID + "-exec",
					ToolCallID:       callID,
					ToolName:         "bash",
					Command:          []string{"bash", "-lc", "echo benchmark"},
					CommandPreview:   "bash -lc echo benchmark",
					WorkingDirectory: "/workspace",
					Output:           outputText,
					Status:           ExecutionStatusCompleted,
					Completed:        true,
					Runtime:          &ToolExecRuntimeState{Backend: "local"},
				},
				Runtime: &ToolExecRuntimeState{Backend: "local"},
				ObservedResources: []ObservedResource{{
					Kind:       "file_content",
					Path:       "notes.txt",
					Version:    "v1",
					Complete:   true,
					StartLine:  1,
					EndLine:    32,
					TotalLines: 32,
				}},
			}
		}

		state.TurnOrder = append(state.TurnOrder, turnID)
		state.Turns[turnID] = turn
	}

	return state
}
