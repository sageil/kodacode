package app

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

type blockingProvider struct {
	started chan struct{}
	calls   int
}

func (p *blockingProvider) Stream(ctx context.Context, _ provider.Request) (provider.Stream, error) {
	p.calls++
	select {
	case <-p.started:
	default:
		close(p.started)
	}
	return &blockingStream{ctx: ctx}, nil
}

type blockingStream struct {
	ctx context.Context
}

func (s *blockingStream) Recv() (provider.Event, error) {
	<-s.ctx.Done()
	return provider.Event{}, s.ctx.Err()
}

func (s *blockingStream) Close() error {
	return nil
}

func TestRuntimeCancelSessionTurnCancelsActiveTurn(t *testing.T) {
	client := &blockingProvider{started: make(chan struct{})}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	type runResult struct {
		result RunSessionResult
		err    error
	}
	done := make(chan runResult, 1)
	go func() {
		result, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
			SessionID: sessionID,
			TurnID:    "turn-1",
			UserText:  "start the server",
		})
		done <- runResult{result: result, err: err}
	}()

	select {
	case <-client.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for provider stream to start")
	}

	if err := runtime.CancelSessionTurn(context.Background(), CancelSessionTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
	}); err != nil {
		t.Fatalf("CancelSessionTurn() error = %v", err)
	}

	var finished runResult
	select {
	case finished = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for canceled turn to finish")
	}

	if finished.err != nil {
		t.Fatalf("StartSessionTurn() error = %v", finished.err)
	}
	if finished.result.Status != TurnRunStatusCanceled {
		t.Fatalf("result status = %q, want %q", finished.result.Status, TurnRunStatusCanceled)
	}
	if finished.result.Error != "" {
		t.Fatalf("result error = %q, want empty", finished.result.Error)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn state missing")
	}
	if turn.Status != events.TurnStatusCanceled {
		t.Fatalf("turn status = %q, want %q", turn.Status, events.TurnStatusCanceled)
	}
	if turn.Error != "" {
		t.Fatalf("turn error = %q, want empty", turn.Error)
	}
	for _, entry := range turn.Transcript {
		if entry.Kind == events.TranscriptEntryError {
			t.Fatalf("canceled turn should not retain transcript error entry: %#v", entry)
		}
	}
}

func TestRuntimeCancelSessionTurnDoesNotDuplicateAttachmentOnlyMessage(t *testing.T) {
	client := &blockingProvider{started: make(chan struct{})}
	runtime := newRuntimeWithClient(t, client)
	workspaceRoot := t.TempDir()
	attachmentPath := mustWriteTestPNG(t, workspaceRoot, "pixel.png")

	sessionID, err := runtime.CreateSession(context.Background(), workspaceRoot)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	type runResult struct {
		result RunSessionResult
		err    error
	}
	done := make(chan runResult, 1)
	go func() {
		result, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
			SessionID: sessionID,
			TurnID:    "turn-1",
			Attachments: []AttachmentInput{{
				Path: attachmentPath,
			}},
		})
		done <- runResult{result: result, err: err}
	}()

	select {
	case <-client.started:
	case finished := <-done:
		t.Fatalf("StartSessionTurn() finished before cancellation: result=%#v err=%v", finished.result, finished.err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for provider stream to start")
	}

	if err := runtime.CancelSessionTurn(context.Background(), CancelSessionTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
	}); err != nil {
		t.Fatalf("CancelSessionTurn() error = %v", err)
	}

	var finished runResult
	select {
	case finished = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for canceled turn to finish")
	}
	if finished.err != nil {
		t.Fatalf("StartSessionTurn() error = %v", finished.err)
	}
	if finished.result.Status != TurnRunStatusCanceled {
		t.Fatalf("result status = %q, want %q", finished.result.Status, TurnRunStatusCanceled)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn state missing")
	}
	if len(turn.UserAttachments) != 1 || turn.UserAttachments[0].Name != "pixel.png" {
		t.Fatalf("user attachments = %#v", turn.UserAttachments)
	}
	userEntries := 0
	for _, entry := range turn.Transcript {
		if entry.Kind == events.TranscriptEntryUser {
			userEntries++
		}
	}
	if userEntries != 1 {
		t.Fatalf("user transcript entries = %d, want 1; transcript = %#v", userEntries, turn.Transcript)
	}
}

func TestRuntimeCancelSessionTurnCancelsActiveForegroundBashExecution(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "bash", InputDelta: mustMarshalJSON(t, map[string]any{"cmd": "sleep 30"})},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "bash"},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	started := make(chan struct{})
	useExecutionRunnerHooks(t, func(ctx context.Context, contract executionContract, opts executionRunOptions) (executionRunResult, error) {
		if got := contract.Command; len(got) == 0 {
			t.Fatalf("execution command = %#v", got)
		}
		select {
		case <-started:
		default:
			close(started)
		}
		<-ctx.Done()
		return executionRunResult{}, ctx.Err()
	})

	type runResult struct {
		result RunSessionResult
		err    error
	}
	done := make(chan runResult, 1)
	go func() {
		result, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
			SessionID: sessionID,
			TurnID:    "turn-1",
			UserText:  "run the performance command",
		})
		done <- runResult{result: result, err: err}
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for bash execution to start")
	}

	if err := runtime.CancelSessionTurn(context.Background(), CancelSessionTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
	}); err != nil {
		t.Fatalf("CancelSessionTurn() error = %v", err)
	}

	var finished runResult
	select {
	case finished = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for canceled bash turn to finish")
	}
	if finished.err != nil {
		t.Fatalf("StartSessionTurn() error = %v", finished.err)
	}
	if finished.result.Status != TurnRunStatusCanceled {
		t.Fatalf("result status = %q, want %q", finished.result.Status, TurnRunStatusCanceled)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn state missing")
	}
	call := turn.ToolCalls["call-1"]
	if call == nil {
		t.Fatal("call state missing")
	}
	if call.Executing {
		t.Fatal("call.Executing = true, want false after cancellation")
	}
	if !call.Completed {
		t.Fatal("call.Completed = false, want true after cancellation")
	}
	if call.Execution == nil {
		t.Fatal("call.Execution = nil, want execution state")
	}
	if call.Execution.Executing {
		t.Fatal("call.Execution.Executing = true, want false after cancellation")
	}
	if !call.Execution.Completed {
		t.Fatal("call.Execution.Completed = false, want true after cancellation")
	}
}

func TestRuntimeCancelSessionTurnCancelsPendingQuestion(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{
				Kind:       provider.EventKindToolCallDelta,
				ToolCallID: "call-question",
				ToolName:   tool.QuestionToolName,
				InputDelta: `{"question":"Which next step should I take?","options":["Apply changes","Stop"]}`,
			},
			{Kind: provider.EventKindToolCallDone, ToolCallID: "call-question", ToolName: tool.QuestionToolName},
		})},
	}
	runtime := newRuntimeWithClient(t, client)
	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "review the plan",
	})
	if err != nil {
		t.Fatalf("StartSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || first.PendingQuestion == nil {
		t.Fatalf("first result = %#v, want pending question", first)
	}

	if err := runtime.CancelSessionTurn(context.Background(), CancelSessionTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
	}); err != nil {
		t.Fatalf("CancelSessionTurn() error = %v", err)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn state missing")
	}
	if turn.Status != events.TurnStatusCanceled {
		t.Fatalf("turn status = %q, want %q", turn.Status, events.TurnStatusCanceled)
	}
	if len(state.PendingQuestionOrder) != 0 || len(state.PendingQuestions) != 0 {
		t.Fatalf("pending questions = order:%#v map:%#v, want none", state.PendingQuestionOrder, state.PendingQuestions)
	}
	call := turn.ToolCalls["call-question"]
	if call == nil {
		t.Fatal("question call missing")
	}
	if !call.Completed || call.Executing {
		t.Fatalf("question call flags = completed:%v executing:%v", call.Completed, call.Executing)
	}
}

func TestRuntimeStartSessionTurnSkipsDanglingToolCallFromCanceledTurn(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "continued"},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	replayed := []events.Draft{
		{
			SessionID: sessionID,
			TurnID:    "turn-1",
			Type:      events.TypeUserMessage,
			Payload:   events.UserMessagePayload{Content: "inspect templates"},
		},
		{
			SessionID: sessionID,
			TurnID:    "turn-1",
			Type:      events.TypeAssistantCommit,
			Payload:   events.AssistantCommitPayload{Content: "Checking templates."},
		},
		{
			SessionID: sessionID,
			TurnID:    "turn-1",
			Type:      events.TypeToolCallDeclared,
			Payload:   events.ToolCallDeclaredPayload{CallID: "call-1", ToolName: "list", Input: `{"path":"src/routes/templates","include_hidden":false}`},
		},
		{
			SessionID: sessionID,
			TurnID:    "turn-1",
			Type:      events.TypeTurnCanceled,
			Payload:   events.TurnCanceledPayload{Message: "turn canceled by user"},
		},
	}
	for idx, event := range replayed {
		if _, err := runtime.Store.Append(context.Background(), event); err != nil {
			t.Fatalf("Append(%d) error = %v", idx, err)
		}
	}

	result, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-2",
		UserText:  "continue",
	})
	if err != nil {
		t.Fatalf("StartSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted || result.AssistantText != "continued" {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	request := client.requests[0]
	if len(request.Inputs) != 3 {
		t.Fatalf("request inputs = %#v", request.Inputs)
	}
	if request.Inputs[0].Kind != provider.InputKindUserMessage || request.Inputs[0].Content != "inspect templates" {
		t.Fatalf("input[0] = %#v", request.Inputs[0])
	}
	if request.Inputs[1].Kind != provider.InputKindAssistantMessage || request.Inputs[1].Content != "Checking templates." {
		t.Fatalf("input[1] = %#v", request.Inputs[1])
	}
	if request.Inputs[2].Kind != provider.InputKindUserMessage || request.Inputs[2].Content != "continue" {
		t.Fatalf("input[2] = %#v", request.Inputs[2])
	}
	for idx, input := range request.Inputs {
		if input.Kind == provider.InputKindToolCall || input.Kind == provider.InputKindToolResult {
			t.Fatalf("unexpected tool replay input[%d] = %#v", idx, input)
		}
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Turns["turn-2"] == nil || state.Turns["turn-2"].Status != events.TurnStatusCompleted {
		t.Fatalf("turn-2 = %#v", state.Turns["turn-2"])
	}
	if got := fmt.Sprintf("%v", state.TurnOrder); got != "[turn-2]" {
		t.Fatalf("turn order = %s", got)
	}
}

func mustMarshalJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return string(raw)
}
