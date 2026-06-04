package app

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
)

func BenchmarkSessionServiceSnapshotWarm(b *testing.B) {
	service, err := NewSessionService(events.NewMemoryStore())
	if err != nil {
		b.Fatalf("NewSessionService() error = %v", err)
	}
	state := benchmarkSessionState(48, 4)
	projector := events.NewProjectorFromSnapshot(state)
	runtime := service.runtimeForSession(state.SessionID)
	runtime.mu.Lock()
	runtime.projector = projector
	runtime.lastDurable = projector.CurrentState().LastSequence
	runtime.mu.Unlock()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := service.Snapshot(context.Background(), state.SessionID); err != nil {
			b.Fatalf("Snapshot() error = %v", err)
		}
	}
}

func BenchmarkSessionServiceSnapshotCold(b *testing.B) {
	state := benchmarkSessionState(48, 4)
	store := events.NewMemoryStore()
	if _, err := store.Append(context.Background(), events.Draft{
		SessionID: state.SessionID,
		TurnID:    sessionTurnID,
		Type:      events.TypeSessionStateSnapshot,
		Payload: events.SessionStateSnapshotPayload{
			BaseSequence: state.LastSequence,
			State:        state,
		},
	}); err != nil {
		b.Fatalf("Append(snapshot) error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		service, err := NewSessionService(store)
		if err != nil {
			b.Fatalf("NewSessionService() error = %v", err)
		}
		if _, err := service.Snapshot(context.Background(), state.SessionID); err != nil {
			b.Fatalf("Snapshot() error = %v", err)
		}
	}
}

func BenchmarkSessionServiceWatchColdReplay(b *testing.B) {
	store := events.NewMemoryStore()
	sessionID := "session-watch-bench"
	content := strings.Repeat("assistant delta ", 32)
	for i := 0; i < 256; i++ {
		if _, err := store.Append(context.Background(), events.Draft{
			SessionID: sessionID,
			TurnID:    fmt.Sprintf("turn-%d", i/4),
			Type:      events.TypeAssistantCommit,
			Payload:   events.AssistantCommitPayload{Content: content},
		}); err != nil {
			b.Fatalf("Append() error = %v", err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		service, err := NewSessionService(store)
		if err != nil {
			b.Fatalf("NewSessionService() error = %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		stream, err := service.Watch(ctx, sessionID, -1)
		if err != nil {
			cancel()
			b.Fatalf("Watch() error = %v", err)
		}
		cancel()
		for event := range stream {
			_ = event
		}
	}
}

func BenchmarkAppendBackgroundOutputTail(b *testing.B) {
	asciiCurrent := strings.Repeat("a", 4096)
	asciiChunk := strings.Repeat("b", 2048)
	utf8Current := strings.Repeat("界", 5000)
	utf8Chunk := strings.Repeat("測", 5000)

	b.Run("under_limit_ascii", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = appendBackgroundOutputTail(asciiCurrent, asciiChunk, backgroundOutputTailLimit)
		}
	})

	b.Run("over_limit_utf8", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = appendBackgroundOutputTail(utf8Current, utf8Chunk, backgroundOutputTailLimit)
		}
	})
}

func BenchmarkBuildSessionConversationStateReasoningDeltaBatch(b *testing.B) {
	const fragmentsPerIteration = 100
	fragment := strings.Repeat("reasoning-", 12)
	replayed := make([]events.Event, 0, fragmentsPerIteration+2)
	replayed = append(replayed, events.Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  0,
		Type:      events.TypeUserMessage,
		Payload:   events.UserMessagePayload{Content: "inspect the provider contract"},
	})
	for j := 0; j < fragmentsPerIteration; j++ {
		replayed = append(replayed, events.Event{
			SessionID: "session-1",
			TurnID:    "turn-1",
			Sequence:  int64(j + 1),
			Type:      events.TypeReasoningDelta,
			Payload: events.ReasoningDeltaPayload{
				Content:   fragment,
				SegmentID: "seg-1",
			},
		})
	}
	replayed = append(replayed, events.Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  fragmentsPerIteration + 1,
		Type:      events.TypeTurnDone,
		Payload:   events.TurnDonePayload{},
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := buildSessionConversationState(replayed, "turn-2", nil); err != nil {
			b.Fatalf("buildSessionConversationState() error = %v", err)
		}
	}
}

func BenchmarkSQLiteBackgroundExecutionLogStoreReadFrom(b *testing.B) {
	sqliteStore, err := events.NewSQLiteStore(filepath.Join(b.TempDir(), "kodacode.db"))
	if err != nil {
		b.Fatalf("events.NewSQLiteStore() error = %v", err)
	}
	b.Cleanup(func() {
		_ = sqliteStore.Close()
	})
	store := NewSQLiteBackgroundExecutionLogStore(sqliteStore)
	handle, err := store.Create(context.Background(), BackgroundExecutionLogKey{
		SessionID:   "session-1",
		TurnID:      "turn-1",
		ExecutionID: "exec-1",
	})
	if err != nil {
		b.Fatalf("Create() error = %v", err)
	}
	content := strings.Repeat("background log line\n", 2048)
	if _, err := io.WriteString(handle.Writer, content); err != nil {
		b.Fatalf("WriteString() error = %v", err)
	}
	if err := handle.Writer.Close(); err != nil {
		b.Fatalf("Close() error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := store.ReadFrom(context.Background(), handle.Ref, 0, backgroundLogReadLimit); err != nil {
			b.Fatalf("ReadFrom() error = %v", err)
		}
	}
}

func BenchmarkSQLiteStoreAppendWithConcurrentBackgroundLogs(b *testing.B) {
	for _, workers := range []int{0, 1, 2, 4} {
		b.Run(fmt.Sprintf("workers_%d", workers), func(b *testing.B) {
			ctx := context.Background()
			sqliteStore, err := events.NewSQLiteStore(filepath.Join(b.TempDir(), "kodacode.db"))
			if err != nil {
				b.Fatalf("events.NewSQLiteStore() error = %v", err)
			}
			b.Cleanup(func() {
				_ = sqliteStore.Close()
			})
			backgroundStore := NewSQLiteBackgroundExecutionLogStore(sqliteStore)
			if _, err := sqliteStore.Append(ctx, events.Draft{
				SessionID: "session-1",
				TurnID:    sessionTurnID,
				Type:      events.TypeSessionConfigured,
				Payload:   events.SessionConfiguredPayload{WorkspaceRoot: "/workspace"},
			}); err != nil {
				b.Fatalf("Append(session_configured) error = %v", err)
			}

			stop := make(chan struct{})
			errCh := make(chan error, workers)
			var wg sync.WaitGroup
			handles := make([]BackgroundExecutionLogHandle, 0, workers)
			flushChunk := []byte(strings.Repeat("background-log-chunk-", 64))
			for i := 0; i < workers; i++ {
				handle, err := backgroundStore.Create(ctx, BackgroundExecutionLogKey{
					SessionID:   "session-1",
					TurnID:      "turn-1",
					ExecutionID: fmt.Sprintf("exec-%d", i),
				})
				if err != nil {
					b.Fatalf("Create() error = %v", err)
				}
				handles = append(handles, handle)
				wg.Add(1)
				go func(writer io.WriteCloser) {
					defer wg.Done()
					ticker := time.NewTicker(5 * time.Millisecond)
					defer ticker.Stop()
					for {
						select {
						case <-stop:
							return
						case <-ticker.C:
						}
						if _, err := writer.Write(flushChunk); err != nil {
							select {
							case errCh <- err:
							default:
							}
							return
						}
					}
				}(handle.Writer)
			}

			payload := events.AssistantCommitPayload{Content: strings.Repeat("assistant output ", 16)}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := sqliteStore.Append(ctx, events.Draft{
					SessionID: "session-1",
					TurnID:    fmt.Sprintf("turn-%d", i),
					Type:      events.TypeAssistantCommit,
					Payload:   payload,
				}); err != nil {
					b.Fatalf("Append() error = %v", err)
				}
				select {
				case err := <-errCh:
					b.Fatalf("concurrent background append error = %v", err)
				default:
				}
			}
			b.StopTimer()
			close(stop)
			wg.Wait()
			for _, handle := range handles {
				if err := handle.Writer.Close(); err != nil {
					b.Fatalf("background writer close error = %v", err)
				}
			}
			select {
			case err := <-errCh:
				b.Fatalf("concurrent background append error = %v", err)
			default:
			}
		})
	}
}

func BenchmarkSQLiteBackgroundExecutionLogStoreAppendThroughput(b *testing.B) {
	sqliteStore, err := events.NewSQLiteStore(filepath.Join(b.TempDir(), "kodacode.db"))
	if err != nil {
		b.Fatalf("events.NewSQLiteStore() error = %v", err)
	}
	b.Cleanup(func() {
		_ = sqliteStore.Close()
	})
	store := NewSQLiteBackgroundExecutionLogStore(sqliteStore)
	handle, err := store.Create(context.Background(), BackgroundExecutionLogKey{
		SessionID:   "session-1",
		TurnID:      "turn-1",
		ExecutionID: "exec-throughput",
	})
	if err != nil {
		b.Fatalf("Create() error = %v", err)
	}
	chunk := []byte(strings.Repeat("append-throughput-", 256))

	b.SetBytes(int64(len(chunk)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := handle.Writer.Write(chunk); err != nil {
			b.Fatalf("Write() error = %v", err)
		}
	}
	b.StopTimer()
	if err := handle.Writer.Close(); err != nil {
		b.Fatalf("Close() error = %v", err)
	}
}

func BenchmarkSQLiteBackgroundExecutionLogStoreResumeCatchup(b *testing.B) {
	sqliteStore, err := events.NewSQLiteStore(filepath.Join(b.TempDir(), "kodacode.db"))
	if err != nil {
		b.Fatalf("events.NewSQLiteStore() error = %v", err)
	}
	b.Cleanup(func() {
		_ = sqliteStore.Close()
	})
	store := NewSQLiteBackgroundExecutionLogStore(sqliteStore)
	handle, err := store.Create(context.Background(), BackgroundExecutionLogKey{
		SessionID:   "session-1",
		TurnID:      "turn-1",
		ExecutionID: "exec-catchup",
	})
	if err != nil {
		b.Fatalf("Create() error = %v", err)
	}

	chunk := strings.Repeat("resume-catchup-line\n", 2048)
	const totalChunks = 256
	for i := 0; i < totalChunks; i++ {
		if _, err := io.WriteString(handle.Writer, chunk); err != nil {
			b.Fatalf("WriteString() error = %v", err)
		}
	}
	if err := handle.Writer.Close(); err != nil {
		b.Fatalf("Close() error = %v", err)
	}

	_, size, err := store.ReadFrom(context.Background(), handle.Ref, 0, backgroundLogReadLimit)
	if err != nil {
		b.Fatalf("ReadFrom(size probe) error = %v", err)
	}
	offsetStart := max(size-(512*1024), int64(0))
	b.SetBytes(size - offsetStart)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		offset := offsetStart
		for {
			segment, currentSize, err := store.ReadFrom(context.Background(), handle.Ref, offset, backgroundLogReadLimit)
			if err != nil {
				b.Fatalf("ReadFrom() error = %v", err)
			}
			if segment == "" {
				break
			}
			offset += int64(len(segment))
			if offset >= currentSize {
				break
			}
		}
	}
}

func benchmarkSessionState(turnCount, toolCallsPerTurn int) events.SessionState {
	state := events.SessionState{
		SessionID:             "session-bench",
		WorkspaceRoot:         "/workspace",
		PermissionMode:        "auto",
		TaskOrder:             []string{"task-1", "task-2"},
		Tasks:                 map[string]*events.TaskState{},
		ApprovedExecutions:    map[string]*events.ApprovedExecutionState{},
		PendingExecutions:     map[string]*events.ExecutionApprovalState{},
		PendingPermissions:    map[string]*events.PermissionRequestState{},
		PendingQuestions:      map[string]*events.QuestionRequestState{},
		QuestionAnswers:       map[string]*events.QuestionAnswerState{},
		TurnOrder:             make([]string, 0, turnCount),
		Turns:                 make(map[string]*events.TurnState, turnCount),
		SessionGrantDecisions: []events.SessionGrantDecisionState{{Source: events.SessionGrantDecisionSourcePermission, ToolName: "read", Path: "notes.txt", ResolvedAtSeq: 7}},
		WorkspaceGrants:       []events.WorkspaceGrantState{{Path: "/workspace", Recursive: true}},
		ExecutionGrants:       []events.ExecutionGrantState{{PrefixRule: []string{"go", "test"}, SessionPaths: []string{"/workspace"}}},
		NetworkGrants:         []events.NetworkGrantState{{Target: "api.openai.com"}},
		LastSequence:          int64(turnCount*toolCallsPerTurn*4 + turnCount),
	}

	inputText := strings.Repeat("input-segment-", 32)
	outputText := strings.Repeat("output-segment-", 48)
	reasoningText := strings.Repeat("reasoning-segment-", 24)

	for turnIndex := 0; turnIndex < turnCount; turnIndex++ {
		turnID := fmt.Sprintf("turn-%d", turnIndex)
		turn := &events.TurnState{
			TurnID:           turnID,
			Status:           events.TurnStatusCompleted,
			UserText:         fmt.Sprintf("user prompt %d %s", turnIndex, inputText),
			AssistantText:    fmt.Sprintf("assistant response %d %s", turnIndex, outputText),
			ReasoningText:    reasoningText,
			Transcript:       make([]events.TranscriptEntryState, 0, 3+toolCallsPerTurn),
			ToolCallOrder:    make([]string, 0, toolCallsPerTurn),
			ToolCalls:        make(map[string]*events.ToolCallState, toolCallsPerTurn),
			CompletedAtSeq:   int64(turnIndex*100 + 99),
			LastUpdatedAtSeq: int64(turnIndex*100 + 99),
		}
		turn.Transcript = append(turn.Transcript,
			events.TranscriptEntryState{Kind: events.TranscriptEntryUser, Sequence: int64(turnIndex * 100), Text: turn.UserText},
			events.TranscriptEntryState{Kind: events.TranscriptEntryReasoning, Sequence: int64(turnIndex*100 + 1), Text: reasoningText, SegmentID: "seg-1"},
			events.TranscriptEntryState{Kind: events.TranscriptEntryAssistant, Sequence: int64(turnIndex*100 + 2), Text: turn.AssistantText},
		)

		for callIndex := 0; callIndex < toolCallsPerTurn; callIndex++ {
			callID := fmt.Sprintf("%s-call-%d", turnID, callIndex)
			turn.ToolCallOrder = append(turn.ToolCallOrder, callID)
			turn.Transcript = append(turn.Transcript, events.TranscriptEntryState{
				Kind:     events.TranscriptEntryTool,
				Sequence: int64(turnIndex*100 + 10 + callIndex),
				Text:     outputText,
				CallID:   callID,
			})
			turn.ToolCalls[callID] = &events.ToolCallState{
				CallID:    callID,
				ToolName:  "bash",
				Input:     inputText,
				Output:    outputText,
				Declared:  true,
				Executing: false,
				Completed: true,
				Execution: &events.ExecutionState{
					ExecutionID:      callID + "-exec",
					ToolCallID:       callID,
					ToolName:         "bash",
					Command:          []string{"bash", "-lc", "echo benchmark"},
					CommandPreview:   "bash -lc echo benchmark",
					WorkingDirectory: "/workspace",
					Output:           outputText,
					Status:           events.ExecutionStatusCompleted,
					Completed:        true,
					Runtime:          &events.ToolExecRuntimeState{Backend: "local"},
				},
				Runtime: &events.ToolExecRuntimeState{Backend: "local"},
				ObservedResources: []events.ObservedResource{{
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
