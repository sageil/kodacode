package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/cellbuf"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

var (
	benchmarkViewSurfaceBuffer *cellbuf.Buffer
	benchmarkViewRendered      string
	benchmarkViewCursor        *tea.Cursor
)

func BenchmarkRenderModelRootSurfaceBufferRepeatedFrame(b *testing.B) {
	model := benchmarkRenderModel(b)
	state := model.projector.Snapshot()
	layout := resolveShellLayout(model, state)

	benchmarkViewSurfaceBuffer = renderModelRootSurfaceBuffer(model, state, layout)
	if benchmarkViewSurfaceBuffer == nil {
		b.Fatal("renderModelRootSurfaceBuffer() returned nil buffer")
	}
	if model.renderCache.rootSurface == nil || model.renderCache.rootSurface.buffer == nil {
		b.Fatal("root surface cache was not populated by the warm render")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkViewSurfaceBuffer = renderModelRootSurfaceBuffer(model, state, layout)
	}
}

func BenchmarkRenderModelSurfaceRepeatedFrame(b *testing.B) {
	model := benchmarkRenderModel(b)

	benchmarkViewRendered, benchmarkViewCursor = renderModelSurface(model)
	if benchmarkViewRendered == "" {
		b.Fatal("renderModelSurface() returned empty content")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkViewRendered, benchmarkViewCursor = renderModelSurface(model)
	}
}

func BenchmarkRenderModelSurfaceRepeatedFrameWithComposerPopup(b *testing.B) {
	model := benchmarkRenderModel(b)
	model.chrome.focus = focusComposer
	model.composerState.popupMode = composerPopupHistory
	model.composerState.promptHistoryBusy = true

	benchmarkViewRendered, benchmarkViewCursor = renderModelSurface(model)
	if benchmarkViewRendered == "" {
		b.Fatal("renderModelSurface() returned empty content")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkViewRendered, benchmarkViewCursor = renderModelSurface(model)
	}
}

func BenchmarkRenderModelSurfaceRepeatedFrameWithDialog(b *testing.B) {
	model := benchmarkRenderModel(b)
	model.dialog = &framedStaticDialog{
		id:      dialogIDCommandPalette,
		theme:   model.theme,
		width:   40,
		content: "Overlay body",
	}

	benchmarkViewRendered, benchmarkViewCursor = renderModelSurface(model)
	if benchmarkViewRendered == "" {
		b.Fatal("renderModelSurface() returned empty content")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkViewRendered, benchmarkViewCursor = renderModelSurface(model)
	}
}

func benchmarkRenderModel(b *testing.B) Model {
	b.Helper()

	defaultTheme := theme.StaticDefault()
	state := benchmarkRenderSessionState()
	ctx, cancel := context.WithCancel(context.Background())
	b.Cleanup(cancel)

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     state.SessionID,
		TurnID:        state.TurnOrder[len(state.TurnOrder)-1],
		WorkspaceRoot: state.WorkspaceRoot,
		InitialState:  &state,
	})
	model.chrome.focus = focusTranscript
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
	model = modelIface.(Model)

	return model
}

func benchmarkRenderSessionState() events.SessionState {
	state := events.SessionState{
		SessionID:          "session-bench",
		WorkspaceRoot:      "/workspace",
		PermissionMode:     "auto",
		Title:              "Render Benchmark Session",
		Tasks:              map[string]*events.TaskState{},
		ApprovedExecutions: map[string]*events.ApprovedExecutionState{},
		PendingExecutions:  map[string]*events.ExecutionApprovalState{},
		PendingPermissions: map[string]*events.PermissionRequestState{},
		PendingQuestions:   map[string]*events.QuestionRequestState{},
		QuestionAnswers:    map[string]*events.QuestionAnswerState{},
		TurnOrder:          make([]string, 0, 6),
		Turns:              make(map[string]*events.TurnState, 6),
	}

	assistantBody := strings.Join([]string{
		"## Render plan",
		"",
		"Repeated frames should reuse decoded buffers instead of reparsing styled strings.",
		"",
		"- move cache ownership to the long-lived renderer",
		"- keep cache keys tied to visible render inputs",
		"- preserve byte-for-byte surface output",
		"",
		"```go",
		"func renderFrame() string {",
		"\treturn \"cached surface\"",
		"}",
		"```",
	}, "\n")
	toolOutput := strings.Repeat("checked render path and found reusable decoded surface data\n", 12)

	var sequence int64
	for turnIndex := 0; turnIndex < 6; turnIndex++ {
		turnID := fmt.Sprintf("turn-%d", turnIndex+1)
		userText := fmt.Sprintf("Investigate repeated frame %d and keep the render path honest.", turnIndex+1)
		callID := turnID + "-bash"

		turn := &events.TurnState{
			TurnID:           turnID,
			Status:           events.TurnStatusCompleted,
			UserText:         userText,
			AssistantText:    assistantBody,
			ReasoningText:    strings.Repeat("reasoning detail ", 16),
			Transcript:       make([]events.TranscriptEntryState, 0, 5),
			ToolCallOrder:    []string{callID},
			ToolCalls:        make(map[string]*events.ToolCallState, 1),
			CompletedAtSeq:   sequence + 8,
			LastUpdatedAtSeq: sequence + 8,
		}
		turn.Transcript = append(turn.Transcript,
			events.TranscriptEntryState{Kind: events.TranscriptEntryUser, Sequence: sequence, Text: userText},
			events.TranscriptEntryState{Kind: events.TranscriptEntryReasoning, Sequence: sequence + 1, Text: strings.Repeat("cache analysis ", 10), SegmentID: "seg-1"},
			events.TranscriptEntryState{Kind: events.TranscriptEntryAssistant, Sequence: sequence + 2, Text: assistantBody},
			events.TranscriptEntryState{Kind: events.TranscriptEntryTool, Sequence: sequence + 3, CallID: callID, Text: toolOutput},
		)
		turn.ToolCalls[callID] = &events.ToolCallState{
			CallID:         callID,
			ToolName:       "bash",
			Input:          "rg renderModelSurface internal/tui",
			Output:         toolOutput,
			Declared:       true,
			Completed:      true,
			Succeeded:      true,
			LastUpdatedSeq: sequence + 3,
			Execution: &events.ExecutionState{
				ExecutionID:      callID + "-exec",
				ToolCallID:       callID,
				ToolName:         "bash",
				Command:          []string{"rg", "renderModelSurface", "internal/tui"},
				CommandPreview:   "rg renderModelSurface internal/tui",
				WorkingDirectory: "/workspace",
				Output:           toolOutput,
				Status:           events.ExecutionStatusCompleted,
				Completed:        true,
				DurationMS:       124,
				Runtime:          &events.ToolExecRuntimeState{Backend: "local"},
			},
			Runtime: &events.ToolExecRuntimeState{Backend: "local"},
		}

		if turnIndex == 5 {
			turn.Status = events.TurnStatusRunning
			turn.AssistantText = ""
			turn.CompletedAtSeq = 0
			turn.LastUpdatedAtSeq = sequence + 6
			turn.StreamingText = strings.Join([]string{
				"## Active stream",
				"",
				"Current assistant output is stable across repeated frames in this benchmark.",
				"",
				"- cached root surface should reuse decoded cells",
				"- transcript markdown should stay on owner-local state",
				"- final render must still match a full repaint",
				"",
				"```text",
				"rendered frame remains unchanged",
				"```",
			}, "\n")
			turn.Transcript = turn.Transcript[:2]
			turn.Transcript = append(turn.Transcript,
				events.TranscriptEntryState{Kind: events.TranscriptEntryTool, Sequence: sequence + 3, CallID: callID, Text: toolOutput},
			)
			turn.ToolCalls[callID].Completed = false
			turn.ToolCalls[callID].Executing = true
			turn.ToolCalls[callID].Succeeded = false
			turn.ToolCalls[callID].Execution.Status = events.ExecutionStatusInProgress
			turn.ToolCalls[callID].Execution.Completed = false
			turn.ToolCalls[callID].Execution.Executing = true
		}

		state.TurnOrder = append(state.TurnOrder, turnID)
		state.Turns[turnID] = turn
		sequence += 10
	}

	state.LastSequence = sequence
	return state
}
