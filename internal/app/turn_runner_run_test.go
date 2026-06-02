package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

type errorAfterEventsStream struct {
	events []provider.Event
	err    error
	index  int
	closed bool
}

func (s *errorAfterEventsStream) Recv() (provider.Event, error) {
	if s.index < len(s.events) {
		event := s.events[s.index]
		s.index++
		return event, nil
	}
	if s.err != nil {
		err := s.err
		s.err = nil
		return provider.Event{}, err
	}
	return provider.Event{}, io.EOF
}

func (s *errorAfterEventsStream) Close() error {
	s.closed = true
	return nil
}

func TestTurnRunnerRunStreamsAssistantTextAndCompletesTurn(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "hel"},
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "lo"},
		})},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "hello",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Turns["turn-1"].AssistantText != "hello" {
		t.Fatalf("assistant text = %q", state.Turns["turn-1"].AssistantText)
	}
	if state.Turns["turn-1"].Status != events.TurnStatusCompleted {
		t.Fatalf("turn status = %q", state.Turns["turn-1"].Status)
	}
}

func TestTurnRunnerRunAutoContinuesAfterLengthFinishReason(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStreamWithFinishReason([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "partial "},
			}, provider.FinishReasonLength),
			provider.NewSliceStreamWithFinishReason([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "final"},
			}, provider.FinishReasonStop),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "hello",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want auto-continuation request", len(client.requests))
	}
	continued := client.requests[1]
	if len(continued.Inputs) < 3 {
		t.Fatalf("continued request inputs = %#v", continued.Inputs)
	}
	if continued.Inputs[len(continued.Inputs)-2].Kind != provider.InputKindAssistantMessage || continued.Inputs[len(continued.Inputs)-2].Content != "partial " {
		t.Fatalf("continued assistant input = %#v", continued.Inputs[len(continued.Inputs)-2])
	}
	if continued.Inputs[len(continued.Inputs)-1].Kind != provider.InputKindUserMessage || continued.Inputs[len(continued.Inputs)-1].Content != outputContinuationInstruction {
		t.Fatalf("continued synthetic input = %#v", continued.Inputs[len(continued.Inputs)-1])
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.AssistantText != "partial final" {
		t.Fatalf("assistant text = %q, want stitched output", turn.AssistantText)
	}
	if len(turn.ProviderAttempts) != 2 || turn.ProviderAttempts[0].FinishReason != string(provider.FinishReasonLength) || turn.ProviderAttempts[1].FinishReason != string(provider.FinishReasonStop) {
		t.Fatalf("provider attempts = %#v", turn.ProviderAttempts)
	}
}

func TestTurnRunnerRunAutoContinuesAfterReasoningOnlyLengthFinishReason(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStreamWithFinishReason(nil, provider.FinishReasonLength),
			provider.NewSliceStreamWithFinishReason([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "final"},
			}, provider.FinishReasonStop),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "hello",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want auto-continuation request", len(client.requests))
	}
	continued := client.requests[1]
	if len(continued.Inputs) < 2 {
		t.Fatalf("continued request inputs = %#v", continued.Inputs)
	}
	last := continued.Inputs[len(continued.Inputs)-1]
	if last.Kind != provider.InputKindUserMessage || last.Content != outputContinuationNoVisibleOutputInstruction {
		t.Fatalf("continued synthetic input = %#v", last)
	}
	for _, input := range continued.Inputs {
		if input.Kind == provider.InputKindAssistantMessage && input.Content == "" {
			t.Fatalf("continued request contains empty assistant input: %#v", continued.Inputs)
		}
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.AssistantText != "final" {
		t.Fatalf("assistant text = %q, want continuation output", turn.AssistantText)
	}
	if len(turn.ProviderAttempts) != 2 || turn.ProviderAttempts[0].FinishReason != string(provider.FinishReasonLength) || turn.ProviderAttempts[1].FinishReason != string(provider.FinishReasonStop) {
		t.Fatalf("provider attempts = %#v", turn.ProviderAttempts)
	}
}

func TestTurnRunnerRunAutoContinuationTrimsRepeatedPrefix(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStreamWithFinishReason([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "1. Done\n\n4. En"},
			}, provider.FinishReasonLength),
			provider.NewSliceStreamWithFinishReason([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "4. Enforce consistent error handling."},
			}, provider.FinishReasonStop),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "hello",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	want := "1. Done\n\n4. Enforce consistent error handling."
	if turn.AssistantText != want {
		t.Fatalf("assistant text = %q, want %q", turn.AssistantText, want)
	}
	if strings.Contains(turn.AssistantText, "4. En4. Enforce") {
		t.Fatalf("assistant text has duplicated continuation boundary: %q", turn.AssistantText)
	}
}

func TestTurnRunnerRunStopsOutputContinuationAtConfiguredLimit(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStreamWithFinishReason([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "one "},
			}, provider.FinishReasonLength),
			provider.NewSliceStreamWithFinishReason([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "two "},
			}, provider.FinishReasonLength),
			provider.NewSliceStreamWithFinishReason([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "three"},
			}, provider.FinishReasonStop),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "hello",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want one bounded continuation", len(client.requests))
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if got := state.Turns["turn-1"].AssistantText; got != "one two " {
		t.Fatalf("assistant text = %q, want output up to continuation limit", got)
	}
}

func TestTurnRunnerRunRetriesPromiseOnlyContextContinuation(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "I'll continue and finish the work. Proceeding now."},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: tool.ReadToolName, InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: tool.ReadToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Read app.go and confirmed it is unchanged."},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:          "session-1",
		TurnID:             "turn-1",
		AgentID:            "builder",
		Fragments:          baseFragments(),
		ModelRoute:         baseModelRoute(),
		ContinuationReason: events.TurnContinuationReasonContextLimit,
		InitialState: &turnLoopState{
			LatestToolStepStart: -1,
			WorkState: turnWorkState{Summary: turnWorkSummary{
				Objective: "Finish the previous implementation turn.",
				OpenItems: []string{
					"Read app.go before continuing.",
				},
			}},
		},
		SkipUserMessageEvent: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}
	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want promise retry, tool step, final response", len(client.requests))
	}
	if !requestInputsContain(client.requests[1], contextContinuationPromiseRetryOpenItem) {
		t.Fatalf("second request did not include retry open item: %#v", client.requests[1].Inputs)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if call := turn.ToolCalls["call-1"]; call == nil || !call.Succeeded {
		t.Fatalf("read call = %#v", call)
	}
	if !strings.Contains(turn.AssistantText, "Read app.go and confirmed") {
		t.Fatalf("assistant text = %q", turn.AssistantText)
	}
}

func TestPromiseOnlyContinuationTextDetection(t *testing.T) {
	for _, text := range []string{
		"I'll continue and finish Phase A. Proceeding now.",
		"I will proceed with the remaining edits now.",
		"Continuing now.",
	} {
		if !isPromiseOnlyContinuationText(text) {
			t.Fatalf("isPromiseOnlyContinuationText(%q) = false", text)
		}
	}
	longReport := strings.Repeat("completed work ", 90)
	if isPromiseOnlyContinuationText(longReport) {
		t.Fatalf("long report treated as promise-only")
	}
}

func requestInputsContain(request provider.Request, text string) bool {
	for _, input := range request.Inputs {
		if strings.Contains(input.Content, text) {
			return true
		}
	}
	return false
}

func TestTurnRunnerRunDeclaresAndExecutesToolCall(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "checking file"},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]`},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream(nil),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	if err := runner.appendTurnConfigured(context.Background(), "session-1", "turn-1", newTurnConfiguredPayload(
		TurnCapabilities{
			AgentID:      "engineer",
			ModelRoute:   baseModelRoute(),
			AllowedTools: []string{tool.TaskWorkflowToolName},
		},
		nil,
		"",
		false,
		false,
		"",
		ResponseStyleDefault,
		false,
	)); err != nil {
		t.Fatalf("appendTurnConfigured() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "read app.go",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	if call == nil || !call.Completed || call.Output != expectedReadSingleLineOutput("package main") {
		t.Fatalf("tool call = %#v", call)
	}
	if len(state.Turns["turn-1"].Transcript) < 2 {
		t.Fatalf("transcript = %#v, want user + worklog/tool entries", state.Turns["turn-1"].Transcript)
	}
	if state.Turns["turn-1"].Transcript[1].Kind != events.TranscriptEntryWorklog || state.Turns["turn-1"].Transcript[1].Text != "checking file" {
		t.Fatalf("transcript[1] = %#v, want durable pre-tool worklog", state.Turns["turn-1"].Transcript[1])
	}
}

func TestTurnRunnerRunCommitsPlannerPlanBeforeSaveQuestion(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, tools, eng, shaper := newTurnRunnerTestDepsWithStore(t, store)

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	plan := "# SSO implementation plan\n\n1. Add OIDC provider support.\n"
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: plan},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-question", ToolName: tool.QuestionToolName, InputDelta: `{"question":"Save or revise the plan?","options":["Save plan","Revise plan"],"purpose":"planner_save_plan"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-question", ToolName: tool.QuestionToolName},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "planner",
		UserText:     "plan SSO",
		Fragments:    baseFragments(),
		ModelRoute:   baseModelRoute(),
		AllowedTools: []string{tool.QuestionToolName},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusPending || result.PendingRequestID == "" {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.AssistantText != plan {
		t.Fatalf("turn assistant text = %#v, want %q", turn, plan)
	}
	if len(state.PlanOrder) != 1 {
		t.Fatalf("plan order = %#v, want recorded planner plan", state.PlanOrder)
	}
	recordedPlan := state.Plans[state.PlanOrder[0]]
	if recordedPlan == nil || recordedPlan.SourceTurnID != "turn-1" || recordedPlan.Title != "SSO implementation plan" ||
		recordedPlan.Markdown != strings.TrimRight(plan, "\n") || recordedPlan.CreatedByAgent != "planner" {
		t.Fatalf("recorded plan = %#v", recordedPlan)
	}
	request := state.PendingQuestions[result.PendingRequestID]
	if request == nil || request.Purpose != events.QuestionPurposePlannerPlanDecision || request.Question != plannerPlanApprovalQuestion ||
		len(request.Options) != 4 || request.Options[0] != plannerPlanApprovalSave || request.Options[1] != plannerPlanApprovalApply ||
		request.Options[2] != plannerPlanApprovalRevise || request.Options[3] != plannerPlanApprovalStop {
		t.Fatalf("planner plan decision request = %#v", request)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	assistantIndex := -1
	questionIndex := -1
	for i, event := range replayed {
		switch event.Type {
		case events.TypeAssistantCommit:
			assistantIndex = i
		case events.TypeQuestionRequested:
			questionIndex = i
		}
	}
	if assistantIndex < 0 || questionIndex < 0 || assistantIndex > questionIndex {
		t.Fatalf("event order assistant=%d question=%d events=%#v", assistantIndex, questionIndex, replayed)
	}
}

func TestTurnRunnerRunFailsPlannerSaveQuestionWithoutVisiblePlanInsteadOfLooping(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-question", ToolName: tool.QuestionToolName, InputDelta: `{"question":"Save or revise the plan?","options":["Save plan","Revise plan"],"purpose":"planner_save_plan"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-question", ToolName: tool.QuestionToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "this stream should not be consumed"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "planner",
		UserText:     "plan SSO",
		Fragments:    baseFragments(),
		ModelRoute:   baseModelRoute(),
		AllowedTools: []string{tool.QuestionToolName},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusFailed {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.Status != events.TurnStatusFailed {
		t.Fatalf("turn = %#v", turn)
	}
	if turn.Error != "The planner attempted to finish the save-plan workflow before showing the plan and receiving user approval." {
		t.Fatalf("turn error = %q", turn.Error)
	}
	if len(state.PendingQuestionOrder) != 0 {
		t.Fatalf("pending questions = %#v", state.PendingQuestionOrder)
	}
	call := turn.ToolCalls["call-question"]
	if call == nil || !call.Completed || call.FailureClass != toolFailureClassContract || call.Error != plannerSavePlanQuestionRequiresVisiblePlanText {
		t.Fatalf("tool call = %#v", call)
	}
}

func TestTurnRunnerRunLoopsAfterToolResult(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "checking file"},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]`},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "read app.go",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}

	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	if strings.Contains(client.requests[0].Instructions, toolResultVisibilityInstruction) {
		t.Fatalf("first request instructions unexpectedly contain visibility note: %q", client.requests[0].Instructions)
	}
	second := client.requests[1]
	if !strings.Contains(second.Instructions, toolResultVisibilityInstruction) {
		t.Fatalf("second request instructions missing visibility note: %q", second.Instructions)
	}
	if len(second.Inputs) != 3 {
		t.Fatalf("second request inputs = %#v", second.Inputs)
	}
	if second.Inputs[0].Kind != provider.InputKindUserMessage || second.Inputs[0].Content != "read app.go" {
		t.Fatalf("input[0] = %#v", second.Inputs[0])
	}
	if second.Inputs[1].Kind != provider.InputKindToolCall || second.Inputs[1].CallID != "call-1" || second.Inputs[1].ToolName != "read" || second.Inputs[1].Arguments != `{"paths":["app.go"]}` {
		t.Fatalf("input[1] = %#v", second.Inputs[1])
	}
	if second.Inputs[2].Kind != provider.InputKindToolResult || second.Inputs[2].CallID != "call-1" || second.Inputs[2].ToolName != "read" || second.Inputs[2].Output != expectedReadSingleLineOutput("package main") {
		t.Fatalf("input[2] = %#v", second.Inputs[2])
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Turns["turn-1"].AssistantText != "done" {
		t.Fatalf("assistant text = %q", state.Turns["turn-1"].AssistantText)
	}
	if state.Turns["turn-1"].Status != events.TurnStatusCompleted {
		t.Fatalf("turn status = %q", state.Turns["turn-1"].Status)
	}
	if len(state.Turns["turn-1"].Transcript) < 3 {
		t.Fatalf("transcript = %#v, want user + worklog + assistant", state.Turns["turn-1"].Transcript)
	}
	if state.Turns["turn-1"].Transcript[1].Kind != events.TranscriptEntryWorklog || state.Turns["turn-1"].Transcript[1].Text != "checking file" {
		t.Fatalf("transcript[1] = %#v, want durable pre-tool worklog", state.Turns["turn-1"].Transcript[1])
	}
}

func TestTurnRunnerRunExecutesExactEditWithoutHistoricalCallID(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-read", ToolName: "read", InputDelta: `{"paths":["notes.txt"],"offset":1,"limit":1}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-read", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-patch", ToolName: "apply_patch", InputDelta: "*** Begin Patch\n*** Update File: notes.txt\n@@\n-two\n+TWO\n*** End Patch\n"},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-patch", ToolName: "apply_patch"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "update notes.txt",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "one\nTWO\nthree\n" {
		t.Fatalf("content = %q", string(data))
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	call := state.Turns["turn-1"].ToolCalls["call-patch"]
	if call == nil || !call.Completed || !call.Succeeded {
		t.Fatalf("call = %#v", call)
	}
	if !strings.Contains(call.Input, "*** Update File: notes.txt") || strings.Contains(call.Input, `"old_text"`) {
		t.Fatalf("call.Input = %q, want raw apply_patch input", call.Input)
	}
}

func TestTurnRunnerRunExecutesReadAndApplyPatchFromSingleProviderStep(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-read", ToolName: "read", InputDelta: `{"paths":["notes.txt"],"offset":1,"limit":1}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-read", ToolName: "read"},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-patch", ToolName: "apply_patch", InputDelta: "*** Begin Patch\n*** Update File: notes.txt\n@@\n-two\n+TWO\n*** End Patch\n"},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-patch", ToolName: "apply_patch"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "update notes.txt",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "one\nTWO\nthree\n" {
		t.Fatalf("content = %q", string(data))
	}

	second := client.requests[1]
	if len(second.Inputs) != 5 {
		t.Fatalf("second request inputs = %#v", second.Inputs)
	}
	if second.Inputs[1].Kind != provider.InputKindToolCall || second.Inputs[1].CallID != "call-read" {
		t.Fatalf("read call = %#v", second.Inputs[1])
	}
	if second.Inputs[2].Kind != provider.InputKindToolCall || second.Inputs[2].CallID != "call-patch" {
		t.Fatalf("apply_patch call = %#v", second.Inputs[2])
	}
	if second.Inputs[3].Kind != provider.InputKindToolResult || second.Inputs[3].CallID != "call-read" {
		t.Fatalf("read result = %#v", second.Inputs[3])
	}
	if second.Inputs[4].Kind != provider.InputKindToolResult || second.Inputs[4].CallID != "call-patch" {
		t.Fatalf("apply_patch result = %#v", second.Inputs[4])
	}
}

func TestTurnRunnerRunPreservesApplyPatchArgumentsAcrossContinuation(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-read", ToolName: "read", InputDelta: `{"paths":["notes.txt"],"offset":1,"limit":1}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-read", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-patch", ToolName: "apply_patch", InputDelta: "*** Begin Patch\n*** Update File: notes.txt\n@@\n-two\n+TWO\n*** End Patch\n"},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-patch", ToolName: "apply_patch"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "update notes.txt",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got := string(data); got != "one\nTWO\nthree\n" {
		t.Fatalf("content = %q", got)
	}

	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want 3", len(client.requests))
	}
	var patchInput *provider.Input
	for i := range client.requests[2].Inputs {
		input := &client.requests[2].Inputs[i]
		if input.Kind == provider.InputKindToolCall && input.ToolName == tool.ApplyPatchToolName {
			patchInput = input
			break
		}
	}
	if patchInput == nil {
		t.Fatalf("third request inputs = %#v, want apply_patch tool call", client.requests[2].Inputs)
	}
	if !strings.Contains(patchInput.Arguments, "*** Update File: notes.txt") || strings.Contains(patchInput.Arguments, `"old_text"`) {
		t.Fatalf("apply_patch arguments = %q, want raw patch", patchInput.Arguments)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	call := state.Turns["turn-1"].ToolCalls["call-patch"]
	if call == nil || !call.Completed || !call.Succeeded {
		t.Fatalf("call-patch = %#v", call)
	}
	if !strings.Contains(call.Input, "*** Update File: notes.txt") || strings.Contains(call.Input, `"old_text"`) {
		t.Fatalf("call.Input = %q, want raw apply_patch input", call.Input)
	}
}

func TestTurnRunnerRunStoresRawFunctionApplyPatchInputAndReplaysJSONArguments(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	patch := "*** Begin Patch\n*** Update File: notes.txt\n@@\n-two\n+TWO\n*** End Patch\n"
	patchArgs, err := json.Marshal(struct {
		Patch string `json:"patch"`
	}{Patch: patch})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-read", ToolName: tool.ReadToolName, InputDelta: `{"paths":["notes.txt"],"offset":1,"limit":1}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-read", ToolName: tool.ReadToolName},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-patch", ToolName: tool.ApplyPatchToolName, ToolKind: provider.ToolKindFunction, InputDelta: string(patchArgs)},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-patch", ToolName: tool.ApplyPatchToolName, ToolKind: provider.ToolKindFunction},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "update notes.txt",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got := string(data); got != "one\nTWO\nthree\n" {
		t.Fatalf("content = %q", got)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	call := state.Turns["turn-1"].ToolCalls["call-patch"]
	if call == nil || !call.Completed || !call.Succeeded {
		t.Fatalf("call-patch = %#v", call)
	}
	if call.Input != patch {
		t.Fatalf("call.Input = %q, want raw patch", call.Input)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	var replayed *provider.Input
	for idx := range client.requests[1].Inputs {
		input := &client.requests[1].Inputs[idx]
		if input.Kind == provider.InputKindToolCall && input.CallID == "call-patch" {
			replayed = input
			break
		}
	}
	if replayed == nil {
		t.Fatalf("second request inputs = %#v, want replayed apply_patch call", client.requests[1].Inputs)
	}
	if strings.TrimRight(replayed.Arguments, "\r\n") != strings.TrimRight(string(patchArgs), "\r\n") {
		t.Fatalf("replayed arguments = %q, want JSON patch arguments", replayed.Arguments)
	}
}

func TestTurnRunnerRunAppliesFunctionPatchWithClassContextMarker(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "src/controllers/ProjectController.ts")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	before := "export class ProjectController {\n  async listProjects() {\n    return 'old';\n  }\n}\n"
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	patch := "*** Begin Patch\n*** Update File: src/controllers/ProjectController.ts\n@@ export class ProjectController\n-    return 'old';\n+    return 'new';\n*** End Patch\n"
	patchArgs, err := json.Marshal(struct {
		Patch string `json:"patch"`
	}{Patch: patch})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-read", ToolName: tool.ReadToolName, InputDelta: `{"paths":["src/controllers/ProjectController.ts"],"offset":0,"limit":8}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-read", ToolName: tool.ReadToolName},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-patch", ToolName: tool.ApplyPatchToolName, ToolKind: provider.ToolKindFunction, InputDelta: string(patchArgs)},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-patch", ToolName: tool.ApplyPatchToolName, ToolKind: provider.ToolKindFunction},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "update project controller",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	want := "export class ProjectController {\n  async listProjects() {\n    return 'new';\n  }\n}\n"
	if got := string(data); got != want {
		t.Fatalf("content = %q", got)
	}
}

func TestTurnRunnerRunReplaysAnthropicThinkingBeforeToolContinuation(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{
					Kind: provider.EventKindAnthropicThinkingCommitted,
					AnthropicThinking: &provider.AnthropicThinkingBlock{
						Type:      provider.AnthropicThinkingBlockTypeThinking,
						Thinking:  "Inspect the file before reading it.",
						Signature: "sig_123",
					},
				},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "builder",
		UserText:        "read app.go",
		Fragments:       baseFragments(),
		ModelRoute:      provider.ModelRoute{Primary: provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"}},
		ThinkingEnabled: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		state, snapshotErr := sessions.Snapshot(context.Background(), "session-1")
		t.Fatalf("result = %#v snapshot_err=%v turn=%#v", result, snapshotErr, state.Turns["turn-1"])
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}

	second := client.requests[1]
	thinkingIndex := -1
	toolCallIndex := -1
	toolResultIndex := -1
	for idx, input := range second.Inputs {
		switch input.Kind {
		case provider.InputKindAnthropicThinking:
			if input.AnthropicThinking != nil && input.AnthropicThinking.Signature == "sig_123" {
				thinkingIndex = idx
			}
		case provider.InputKindToolCall:
			if input.CallID == "call-1" {
				toolCallIndex = idx
			}
		case provider.InputKindToolResult:
			if input.CallID == "call-1" {
				toolResultIndex = idx
			}
		}
	}
	if thinkingIndex < 0 || toolCallIndex < 0 || toolResultIndex < 0 {
		t.Fatalf("second request inputs = %#v", second.Inputs)
	}
	if thinkingIndex >= toolCallIndex || toolCallIndex >= toolResultIndex {
		t.Fatalf("input ordering thinking=%d tool_call=%d tool_result=%d inputs=%#v", thinkingIndex, toolCallIndex, toolResultIndex, second.Inputs)
	}
}

func TestTurnRunnerRunReplaysOpenAIEncryptedReasoningBeforeToolContinuation(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, tools, eng, shaper := newTurnRunnerTestDepsWithStore(t, store)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	reasoningItem := json.RawMessage(`{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"enc_123"}`)
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindOpenAIReasoningCommitted, OpenAIReasoningItem: reasoningItem},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "builder",
		UserText:        "read app.go",
		Fragments:       baseFragments(),
		ModelRoute:      baseModelRoute(),
		ThinkingEnabled: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		state, snapshotErr := sessions.Snapshot(context.Background(), "session-1")
		t.Fatalf("result = %#v snapshot_err=%v turn=%#v", result, snapshotErr, state.Turns["turn-1"])
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}

	second := client.requests[1]
	reasoningIndex := -1
	toolCallIndex := -1
	toolResultIndex := -1
	for idx, input := range second.Inputs {
		switch input.Kind {
		case provider.InputKindOpenAIReasoning:
			if string(input.OpenAIReasoningItem) == string(reasoningItem) {
				reasoningIndex = idx
			}
		case provider.InputKindToolCall:
			if input.CallID == "call-1" {
				toolCallIndex = idx
			}
		case provider.InputKindToolResult:
			if input.CallID == "call-1" {
				toolResultIndex = idx
			}
		}
	}
	if reasoningIndex < 0 || toolCallIndex < 0 || toolResultIndex < 0 {
		t.Fatalf("second request inputs = %#v", second.Inputs)
	}
	if reasoningIndex >= toolCallIndex || toolCallIndex >= toolResultIndex {
		t.Fatalf("input ordering reasoning=%d tool_call=%d tool_result=%d inputs=%#v", reasoningIndex, toolCallIndex, toolResultIndex, second.Inputs)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	for _, event := range replayed {
		payload, ok := event.Payload.(events.OpenAIReasoningCommittedPayload)
		if event.Type == events.TypeOpenAIReasoningCommitted && ok && string(payload.Item) == string(reasoningItem) {
			return
		}
	}
	t.Fatalf("replayed events = %#v, want persisted OpenAI reasoning item", replayed)
}

func TestTurnRunnerRunFailsWhenOpenAIReasoningContinuationContractIsUnsupported(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindReasoningDelta, ReasoningDelta: "Inspect the file before reading it."},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "builder",
		UserText:        "read app.go",
		Fragments:       baseFragments(),
		ModelRoute:      provider.ModelRoute{Primary: provider.ModelRef{ProviderID: "openrouter", ModelID: "deepseek/deepseek-v4-pro"}},
		ThinkingEnabled: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusFailed {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.Status != events.TurnStatusFailed {
		t.Fatalf("turn = %#v", turn)
	}
	if !strings.Contains(turn.Error, "openai_reasoning_tool_loop") {
		t.Fatalf("turn error = %q, want openai reasoning continuation contract failure", turn.Error)
	}
	if turn.WorkState == nil || turn.WorkState.NativeContinuation == nil || turn.WorkState.NativeContinuation.Contract != "openai_reasoning_tool_loop" {
		t.Fatalf("turn work state = %#v", turn.WorkState)
	}
}

func TestTurnRunnerRunLoopsAfterToolResultOnCompatibleTransport(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "read app.go",
		Fragments:  baseFragments(),
		ModelRoute: provider.ModelRoute{Primary: provider.ModelRef{ProviderID: "nvidia", ModelID: "openai/gpt-oss-20b"}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}

	second := client.requests[1]
	if len(second.Inputs) != 3 {
		t.Fatalf("second request inputs = %#v", second.Inputs)
	}
	if second.Inputs[1].Kind != provider.InputKindToolCall || second.Inputs[1].CallID != "call-1" || second.Inputs[1].ToolName != "read" || second.Inputs[1].Arguments != `{"paths":["app.go"]}` {
		t.Fatalf("input[1] = %#v", second.Inputs[1])
	}
	if second.Inputs[2].Kind != provider.InputKindToolResult || second.Inputs[2].CallID != "call-1" || second.Inputs[2].ToolName != "read" || second.Inputs[2].Output != expectedReadSingleLineOutput("package main") {
		t.Fatalf("input[2] = %#v", second.Inputs[2])
	}
}

func TestTurnRunnerRunSuppressesRawWritePayloadFromAssistantText(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-read", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-read", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Writing file"},
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "\nwrite {\"path\":\"app.go\",\"content\":\"package app\\n\"}"},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "write", InputDelta: `{"path":"app.go","content":"package app\n"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "write"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "update app.go",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if strings.Contains(turn.AssistantText, `{"path":"app.go"`) || strings.Contains(turn.AssistantText, "\nwrite ") {
		t.Fatalf("assistant text leaked raw write payload = %q", turn.AssistantText)
	}
	if turn.AssistantText != "done" {
		t.Fatalf("assistant text = %q", turn.AssistantText)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got := strings.TrimSpace(string(content)); got != "package app" {
		t.Fatalf("written file = %q", got)
	}
}

func TestTurnRunnerRunExposesMinimalBashSchemaToProvider(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
		})},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "run a command",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}

	var bashTool *provider.Tool
	for i := range client.requests[0].Tools {
		if client.requests[0].Tools[i].Name == tool.BashToolName {
			bashTool = &client.requests[0].Tools[i]
			break
		}
	}
	if bashTool == nil {
		t.Fatal("bash tool missing from provider request")
	}

	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal([]byte(bashTool.InputSchema), &schema); err != nil {
		t.Fatalf("json.Unmarshal(schema) error = %v", err)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "cmd" {
		t.Fatalf("bash schema required = %#v, want only cmd", schema.Required)
	}
}

func TestTurnRunnerRunDoesNotReplayMalformedToolCallBackToProvider(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "checking files"},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["README.md"]`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "recovered"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "review files",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}

	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	second := client.requests[1]
	if len(second.Inputs) != 3 {
		t.Fatalf("second request inputs = %#v", second.Inputs)
	}
	if second.Inputs[0].Kind != provider.InputKindUserMessage || second.Inputs[0].Content != "review files" {
		t.Fatalf("input[0] = %#v", second.Inputs[0])
	}
	if second.Inputs[1].Kind != provider.InputKindToolCall || second.Inputs[1].ToolName != "read" {
		t.Fatalf("input[1] = %#v", second.Inputs[1])
	}
	if second.Inputs[2].Kind != provider.InputKindToolResult {
		t.Fatalf("input[2] = %#v", second.Inputs[2])
	}
	if got := second.Inputs[2].Error; !strings.Contains(got, "`read` failed.") || !strings.Contains(got, "JSON ended before the object was complete") || !strings.Contains(got, `Use either path for one file or paths for one or more files; do not send both.`) {
		t.Fatalf("input[2].Error = %q", got)
	}
}

func TestTurnRunnerRunFailsWithSpecificProviderMessageAfterMalformedBashCall(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "checking"},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: tool.BashToolName, InputDelta: `{"cmd":"pwd","workdir": `},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: tool.BashToolName, InputDelta: `/tmp}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: tool.BashToolName},
			}),
			&errorAfterEventsStream{err: errors.New("stream error: stream ID 43; INTERNAL_ERROR; received from peer")},
			&errorAfterEventsStream{err: errors.New("stream error: stream ID 43; INTERNAL_ERROR; received from peer")},
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetSessionConfig(SessionConfig{MaxRetries: 1})
	runner.wait = func(context.Context, time.Duration) error { return nil }

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "review the project",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusFailed {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.Status != events.TurnStatusFailed {
		t.Fatalf("turn status = %q", turn.Status)
	}
	if turn.Error != "The provider hit a temporary internal error before the response finished. Please try again. Details: stream error: stream ID 43; INTERNAL_ERROR; received from peer." {
		t.Fatalf("turn error = %q", turn.Error)
	}
	call := turn.ToolCalls["call-1"]
	if call == nil {
		t.Fatal("bash tool call missing")
	}
	if !strings.Contains(call.Error, "`bash` failed.") || !strings.Contains(call.Error, `Example: {"cmd":"git status"}.`) {
		t.Fatalf("tool call error = %q", call.Error)
	}
}

func TestTurnRunnerRunReplaysInvalidToolArgumentsBackToProviderAsToolError(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "checking files"},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":123}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "recovered"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "review files",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}

	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	second := client.requests[1]
	if len(second.Inputs) != 3 {
		t.Fatalf("second request inputs = %#v", second.Inputs)
	}
	if second.Inputs[1].Kind != provider.InputKindToolCall || second.Inputs[1].ToolName != "read" {
		t.Fatalf("input[1] = %#v", second.Inputs[1])
	}
	if second.Inputs[2].Kind != provider.InputKindToolResult {
		t.Fatalf("input[2] = %#v", second.Inputs[2])
	}
	if got := second.Inputs[2].Error; !strings.Contains(got, "`read` failed.") || !strings.Contains(got, "paths must be an array of strings; got number") || !strings.Contains(got, `Use either path for one file or paths for one or more files; do not send both.`) {
		t.Fatalf("input[2].Error = %q", got)
	}
}

func TestTurnRunnerRunPreservesProviderFailureWhenFailTurnPersistenceFails(t *testing.T) {
	providerErr := errors.New("provider exploded")
	persistErr := errors.New("turn error append failed")
	store := &executionServiceFailingStore{
		MemoryStore: events.NewMemoryStore(),
		failTypes: map[events.Type]error{
			events.TypeTurnError: persistErr,
		},
	}
	sessions, tools, eng, shaper := newTurnRunnerTestDepsWithStore(t, store)
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{err: providerErr}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	_, err = runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "review the project",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err == nil {
		t.Fatal("Run() error = nil, want joined provider + persistence error")
	}
	if !errors.Is(err, providerErr) {
		t.Fatalf("Run() error = %v, want provider error", err)
	}
	if !errors.Is(err, persistErr) {
		t.Fatalf("Run() error = %v, want persistence error", err)
	}
}

func TestTurnRunnerRunContinuesAfterInvalidTestToolArguments(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "running tests"},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: tool.TestToolName, InputDelta: mustMarshalJSON(t, map[string]any{
					"command": `npx jest -t "ProjectController" --runInBand`,
					"path":    "",
					"filter":  nil,
					"timeout": 120000,
				})},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: tool.TestToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "skipping that invalid test target"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "review the project",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	second := client.requests[1]
	if len(second.Inputs) != 3 {
		t.Fatalf("second request inputs = %#v", second.Inputs)
	}
	callInput := second.Inputs[1]
	if callInput.Kind != provider.InputKindToolCall || callInput.ToolName != tool.TestToolName {
		t.Fatalf("tool call = %#v", callInput)
	}
	resultInput := second.Inputs[2]
	if resultInput.Kind != provider.InputKindToolResult {
		t.Fatalf("tool result = %#v", resultInput)
	}
	if !strings.Contains(resultInput.Error, "`test` failed.") || !strings.Contains(resultInput.Error, "path must not be empty") || !strings.Contains(resultInput.Error, `Example: {"command":null,"path":"internal/tool","filter":null,"timeout":90000}.`) {
		t.Fatalf("tool result error = %q", resultInput.Error)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.Status != events.TurnStatusCompleted {
		t.Fatalf("turn status = %q", turn.Status)
	}
	if turn.Error != "" {
		t.Fatalf("turn error = %q, want empty", turn.Error)
	}
	call := turn.ToolCalls["call-1"]
	if call == nil || !strings.Contains(call.Error, "`test` failed.") || !strings.Contains(call.Error, `Example: {"command":null,"path":"internal/tool","filter":null,"timeout":90000}.`) {
		t.Fatalf("tool call = %#v", call)
	}
}

func TestTurnRunnerRunRecoversFromInvalidReadArgumentsWithCorrectedReadStep(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "README.md")
	if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "checking files"},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":123}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["README.md"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "recovered"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "review files",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}

	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want 3", len(client.requests))
	}
	second := client.requests[1]
	if len(second.Inputs) != 3 {
		t.Fatalf("second request inputs = %#v", second.Inputs)
	}
	if second.Inputs[1].Kind != provider.InputKindToolCall || second.Inputs[1].CallID != "call-1" || second.Inputs[1].ToolName != "read" {
		t.Fatalf("second request tool call = %#v", second.Inputs[1])
	}
	if second.Inputs[2].Kind != provider.InputKindToolResult || second.Inputs[2].CallID != "call-1" || second.Inputs[2].ToolName != "read" {
		t.Fatalf("second request tool result = %#v", second.Inputs[2])
	}
	if got := second.Inputs[2].Error; !strings.Contains(got, "`read` failed.") || !strings.Contains(got, "paths must be an array of strings; got number") || !strings.Contains(got, `Use either path for one file or paths for one or more files; do not send both.`) {
		t.Fatalf("second request tool result error = %q", got)
	}

	third := client.requests[2]
	if len(third.Inputs) != 5 {
		t.Fatalf("third request inputs = %#v", third.Inputs)
	}
	if third.Inputs[2].Kind != provider.InputKindToolResult || third.Inputs[2].CallID != "call-1" {
		t.Fatalf("third request invalid result = %#v", third.Inputs[2])
	}
	if third.Inputs[3].Kind != provider.InputKindToolCall || third.Inputs[3].CallID != "call-2" || third.Inputs[3].ToolName != "read" {
		t.Fatalf("third request corrected call = %#v", third.Inputs[3])
	}
	if third.Inputs[4].Kind != provider.InputKindToolResult || third.Inputs[4].CallID != "call-2" || third.Inputs[4].ToolName != "read" || third.Inputs[4].Output != expectedReadSingleLineOutputForPath("README.md", "hello") || third.Inputs[4].Error != "" {
		t.Fatalf("third request corrected result = %#v", third.Inputs[4])
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.AssistantText != "recovered" {
		t.Fatalf("assistant text = %q", turn.AssistantText)
	}
	if turn.ToolCalls["call-1"] == nil || !strings.Contains(turn.ToolCalls["call-1"].Error, "`read` failed.") {
		t.Fatalf("call-1 = %#v", turn.ToolCalls["call-1"])
	}
	if turn.ToolCalls["call-2"] == nil || !turn.ToolCalls["call-2"].Completed || !turn.ToolCalls["call-2"].Succeeded || turn.ToolCalls["call-2"].Output != expectedReadSingleLineOutputForPath("README.md", "hello") {
		t.Fatalf("call-2 = %#v", turn.ToolCalls["call-2"])
	}
}

func TestTurnRunnerRunExecutesRepeatedReadInSameTurn(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	largeContent := strings.Repeat("package main\n", 80)
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte(largeContent), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]`},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["app.go"]`},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "read app.go twice",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}

	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want 3", len(client.requests))
	}
	third := client.requests[2]
	var (
		call1Result *provider.Input
		call2Tool   *provider.Input
		call2Result *provider.Input
	)
	for i := range third.Inputs {
		input := &third.Inputs[i]
		switch {
		case input.Kind == provider.InputKindToolResult && input.CallID == "call-1":
			call1Result = input
		case input.Kind == provider.InputKindToolCall && input.CallID == "call-2":
			call2Tool = input
		case input.Kind == provider.InputKindToolResult && input.CallID == "call-2":
			call2Result = input
		}
	}
	if call1Result == nil || call2Tool == nil || call2Result == nil {
		t.Fatalf("third request missing terminal replay pair: %#v", third.Inputs)
	}
	if !strings.Contains(call1Result.Output, "1: package main") || call1Result.Error != "" {
		t.Fatalf("call1 result = %#v", call1Result)
	}
	if !strings.Contains(call2Result.Output, "1: package main") || call2Result.Error != "" {
		t.Fatalf("call2 result = %#v", call2Result)
	}
	if call2Result.ReusedFromCallID != "" {
		t.Fatalf("call2 result = %#v", call2Result)
	}
}

func TestTurnRunnerRunExecutesSemanticRepeatedReadAndContinuationWindow(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	var content strings.Builder
	for i := 1; i <= 300; i++ {
		fmt.Fprintf(&content, "line %d\n", i)
	}
	if err := os.WriteFile(filepath.Join(root, "server.ts"), []byte(content.String()), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["server.ts"]`},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"path":"server.ts","offset":0,"limit":2000`},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-3", ToolName: "read", InputDelta: `{"paths":["server.ts"],"offset":200,"limit":100`},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-3", ToolName: "read", InputDelta: `}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-3", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "read server.ts and continue later",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}
	if len(client.requests) != 4 {
		t.Fatalf("provider requests = %d, want 4", len(client.requests))
	}

	finalRequest := client.requests[3]
	var call2Result, call3Result *provider.Input
	for i := range finalRequest.Inputs {
		input := &finalRequest.Inputs[i]
		switch {
		case input.Kind == provider.InputKindToolResult && input.CallID == "call-2":
			call2Result = input
		case input.Kind == provider.InputKindToolResult && input.CallID == "call-3":
			call3Result = input
		}
	}
	if call2Result == nil || !strings.Contains(call2Result.Output, "1: line 1") || !strings.Contains(call2Result.Output, "300: line 300") {
		t.Fatalf("call-2 result = %#v", call2Result)
	}
	if call2Result.ReusedFromCallID != "" {
		t.Fatalf("call-2 result = %#v", call2Result)
	}
	if call3Result == nil || !strings.Contains(call3Result.Output, "201: line 201") || call3Result.ReusedFromCallID != "" {
		t.Fatalf("call-3 result = %#v", call3Result)
	}
}

func TestTurnRunnerRunPreservesTerminalToolResultForNextPassAfterLargeRead(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	largeContent := strings.Repeat("package main "+strings.Repeat("x", 160)+"\n", 3000)
	if err := os.WriteFile(path, []byte(largeContent), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "checking file"},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"],"limit":3000`},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream(nil),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "read app.go",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}

	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	second := client.requests[1]
	if len(second.Inputs) == 0 {
		t.Fatal("second request has no inputs")
	}
	last := second.Inputs[len(second.Inputs)-1]
	if last.Kind != provider.InputKindToolResult || last.CallID != "call-1" || last.ToolName != "read" {
		t.Fatalf("last input = %#v, want terminal tool result", last)
	}
	if !strings.Contains(last.Output, "1: package main") || !strings.Contains(last.Output, "400: package main") {
		t.Fatalf("last tool result missing read output: %q", last.Output)
	}
	if !strings.Contains(last.Output, "3000: package main") {
		t.Fatalf("last tool result missing final line; output bytes = %d", len(last.Output))
	}
	if strings.Contains(last.Output, "Output was capped to protect context budget") {
		t.Fatalf("last tool result was capped: %q", last.Output)
	}
}

func TestTurnRunnerRunBoundsMultiFileReadBatchBeforeNextPass(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	paths := make([]string, 0, 20)
	for idx := 1; idx <= 20; idx++ {
		name := fmt.Sprintf("file-%d.ts", idx)
		paths = append(paths, name)
		lines := make([]string, 0, 500)
		for line := 1; line <= 500; line++ {
			lines = append(lines, fmt.Sprintf("export const value_%d_%d = %q;", idx, line, strings.Repeat("x", 200)))
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	readArgs, err := json.Marshal(map[string]any{"paths": paths})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: string(readArgs)},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream(nil),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "inspect these files",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}

	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	second := client.requests[1]
	last := second.Inputs[len(second.Inputs)-1]
	if last.Kind != provider.InputKindToolResult || last.ToolName != "read" {
		t.Fatalf("last input = %#v, want terminal read tool result", last)
	}
	if !strings.Contains(last.Output, "=== file-1.ts ===") {
		t.Fatalf("last tool result missing first file header; output bytes = %d", len(last.Output))
	}
	if strings.Contains(last.Output, "Output was capped to protect context budget") {
		t.Fatalf("last tool result was capped; output bytes = %d", len(last.Output))
	}
	if strings.Count(last.Output, "=== file-") != len(paths) {
		t.Fatalf("last tool result did not render every file; output bytes = %d", len(last.Output))
	}
}

func TestTurnRunnerRunPreservesInvalidToolResultsAcrossToolOnlySteps(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	fileNames := []string{"a.go", "b.go", "c.go", "d.go", "e.go"}
	for _, name := range fileNames {
		if err := os.WriteFile(filepath.Join(root, name), []byte(strings.Repeat("package main\n", 600)), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	streams := make([]provider.Stream, 0, len(fileNames)+1)
	for idx := range fileNames {
		streams = append(streams, provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindToolCallDelta, ToolCallID: fmt.Sprintf("call-%d", idx+1), ToolName: "read", InputDelta: fmt.Sprintf(`{"paths":[],"offset":%d}`, idx+1)},
			{Kind: provider.EventKindToolCallDone, ToolCallID: fmt.Sprintf("call-%d", idx+1), ToolName: "read"},
		}))
	}
	streams = append(streams, provider.NewSliceStream(nil))

	client := &fakeProvider{streams: streams}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     128000,
				MaxInputTokens:  128000,
				MaxOutputTokens: 8192,
			}},
		},
	})
	runner.SetSessionConfig(SessionConfig{MaxRetries: defaultProviderRetryAttempts})

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		UserText:     "inspect several files",
		Fragments:    baseFragments(),
		ModelRoute:   baseModelRoute(),
		AllowedTools: []string{"read"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}

	lastRequest := client.requests[len(client.requests)-1]
	expectedInputs := 1 + 2*len(fileNames)
	if len(lastRequest.Inputs) != expectedInputs {
		t.Fatalf("last request inputs = %d, want %d; inputs = %#v", len(lastRequest.Inputs), expectedInputs, lastRequest.Inputs)
	}
	toolCalls := 0
	toolResults := 0
	for _, input := range lastRequest.Inputs {
		if input.Kind == provider.InputKindAssistantMessage && strings.Contains(input.Content, "Compacted current-turn exploration:") {
			t.Fatalf("last request unexpectedly compacted tool-only turn: %#v", input)
		}
		if input.Kind == provider.InputKindToolCall {
			toolCalls++
		}
		if input.Kind == provider.InputKindToolResult {
			toolResults++
			if !strings.Contains(input.Error, "path or paths is required") {
				t.Fatalf("tool result error = %q", input.Error)
			}
		}
	}
	if toolCalls != len(fileNames) || toolResults != len(fileNames) {
		t.Fatalf("tool replay counts = %d calls, %d results; want %d each", toolCalls, toolResults, len(fileNames))
	}
}

func TestExecuteTurnLoopFailsTurnWhenActiveTurnCompactionCannotAdvanceBeforeProtectedTail(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, tools, eng, shaper := newTurnRunnerTestDepsWithStore(t, store)

	root := t.TempDir()
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     2048,
				MaxInputTokens:  2048,
				MaxOutputTokens: 2048,
			}},
		},
	})

	result, err := runner.executeTurnLoop(context.Background(), turnLoopInput{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Instructions: "inspect the project",
		ModelRoute:   baseModelRoute(),
		State: turnLoopState{
			Conversation: []provider.Input{
				{Kind: provider.InputKindUserMessage, Content: "inspect the project"},
				{Kind: provider.InputKindToolCall, CallID: "call-1", ToolName: "read", Arguments: `{"paths":["file.go"]}`},
				{Kind: provider.InputKindToolResult, CallID: "call-1", ToolName: "read", Output: strings.Repeat("x", 12000)},
			},
			LatestToolStepStart: 1,
		},
	})
	if err != nil {
		t.Fatalf("executeTurnLoop() error = %v", err)
	}
	if result.Status != TurnRunStatusFailed {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	if got := len(client.requests); got != 0 {
		t.Fatalf("provider requests = %d, want 0 when compaction fails before send", got)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}

	foundFailed := false
	foundTurnError := false
	for _, event := range replayed {
		switch payload := event.Payload.(type) {
		case events.ContextCompactionFailedPayload:
			if event.TurnID != "turn-1" {
				continue
			}
			foundFailed = true
			if payload.Scope != events.CompactionScopeHistory {
				t.Fatalf("scope = %q, want %q", payload.Scope, events.CompactionScopeHistory)
			}
			if payload.Reason != "input_limit_unreachable" {
				t.Fatalf("reason = %q, want input_limit_unreachable", payload.Reason)
			}
		case events.TurnErrorPayload:
			if event.TurnID == "turn-1" {
				foundTurnError = true
			}
		}
	}
	if !foundFailed {
		t.Fatal("context_compaction_failed event missing for oversized request")
	}
	if !foundTurnError {
		t.Fatal("turn_error event missing for oversized request")
	}
}

func TestTurnRunnerRunRollsOverActiveTurnWhenExactTokenCountReachesTriggerPressure(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, tools, eng, shaper := newTurnRunnerTestDepsWithStore(t, store)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		counts:       []int{200, 900},
		countSources: []provider.TokenCountSource{provider.TokenCountSourceExact, provider.TokenCountSourceExact},
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: tool.ReadToolName, InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: tool.ReadToolName},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetSessionConfig(SessionConfig{
		CompactionThreshold:       0.80,
		CompactionTargetThreshold: 0.10,
	})
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:             "gpt-5",
				ContextSize:    1000,
				MaxInputTokens: 1000,
			}},
		},
	})

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "read app.go",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusRolled || result.RolloverReason != TurnRolloverReasonContextLimit {
		t.Fatalf("result = %#v, want context-limit rollover", result)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want exactly one before rollover", len(client.requests))
	}
	if len(client.countRequests) != 2 {
		t.Fatalf("count requests = %d, want preflight count for both steps", len(client.countRequests))
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	foundTurnDone := false
	foundCheckpoint := false
	for _, event := range replayed {
		if event.TurnID == "turn-1" && event.Type == events.TypeContextCompactionFailed {
			t.Fatal("context_compaction_failed event should not be emitted for active-turn rollover")
		}
		if event.TurnID == "turn-1" && event.Type == events.TypeTurnError {
			t.Fatal("turn_error event should not be emitted for active-turn rollover")
		}
		if event.TurnID == "turn-1" && event.Type == events.TypeTurnDone {
			foundTurnDone = true
		}
		if event.Type == events.TypeSessionHistoryCheckpoint {
			foundCheckpoint = true
		}
	}
	if !foundTurnDone {
		t.Fatal("turn_done event missing for active-turn rollover")
	}
	if !foundCheckpoint {
		t.Fatal("session_history_checkpoint event missing for active-turn rollover")
	}
}

func TestTurnRunnerRunRollsOverActiveTurnWhenProviderRejectsRequestForInputLimit(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, tools, eng, shaper := newTurnRunnerTestDepsWithStore(t, store)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		counts:       []int{200, 200},
		countSources: []provider.TokenCountSource{provider.TokenCountSourceExact, provider.TokenCountSourceExact},
		errs: []error{
			nil,
			&provider.ProviderError{Message: "prompt token count of 130431 exceeds the limit of 128000", StatusCode: 400},
		},
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: tool.ReadToolName, InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: tool.ReadToolName},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetSessionConfig(SessionConfig{
		CompactionThreshold:       0.80,
		CompactionTargetThreshold: 0.10,
	})
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:             "gpt-5",
				ContextSize:    1000,
				MaxInputTokens: 1000,
			}},
		},
	})

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "read app.go",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusRolled || result.RolloverReason != TurnRolloverReasonContextLimit {
		t.Fatalf("result = %#v, want context-limit rollover", result)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want second request to reach provider rejection", len(client.requests))
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	foundTurnDone := false
	foundCheckpoint := false
	for _, event := range replayed {
		if event.TurnID == "turn-1" && event.Type == events.TypeContextCompactionFailed {
			t.Fatal("context_compaction_failed event should not be emitted when active turn can roll over")
		}
		if event.TurnID == "turn-1" && event.Type == events.TypeTurnError {
			t.Fatal("turn_error event should not be emitted when active turn can roll over")
		}
		if event.TurnID == "turn-1" && event.Type == events.TypeTurnDone {
			foundTurnDone = true
		}
		if event.Type == events.TypeSessionHistoryCheckpoint {
			foundCheckpoint = true
		}
	}
	if !foundTurnDone {
		t.Fatal("turn_done event missing for provider input-limit rollover")
	}
	if !foundCheckpoint {
		t.Fatal("session_history_checkpoint event missing for provider input-limit rollover")
	}
}

func TestTurnRunnerRunFailsFirstStepProviderInputLimitWithCompactionFailedEvent(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, tools, eng, shaper := newTurnRunnerTestDepsWithStore(t, store)

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		counts:       []int{200},
		countSources: []provider.TokenCountSource{provider.TokenCountSourceExact},
		errs: []error{
			&provider.ProviderError{Message: "prompt token count of 130431 exceeds the limit of 128000", StatusCode: 400},
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetSessionConfig(SessionConfig{
		CompactionThreshold:       0.80,
		CompactionTargetThreshold: 0.10,
	})
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:             "gpt-5",
				ContextSize:    1000,
				MaxInputTokens: 1000,
			}},
		},
	})

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "hello",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusFailed {
		t.Fatalf("result = %#v, want failed", result)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	foundFailed := false
	foundTurnError := false
	for _, event := range replayed {
		switch payload := event.Payload.(type) {
		case events.ContextCompactionFailedPayload:
			if event.TurnID != "turn-1" {
				continue
			}
			foundFailed = true
			if payload.Reason != "input_limit_unreachable" {
				t.Fatalf("reason = %q, want input_limit_unreachable", payload.Reason)
			}
		case events.TurnErrorPayload:
			if event.TurnID == "turn-1" {
				foundTurnError = true
			}
		}
	}
	if !foundFailed {
		t.Fatal("context_compaction_failed event missing for first-step provider input-limit rejection")
	}
	if !foundTurnError {
		t.Fatal("turn_error event missing for first-step provider input-limit rejection")
	}
}

func TestTurnRunnerRunAllowsRepeatedDuplicateOnlyToolStepsWithoutLoopPause(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]`},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["app.go"]`},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-3", ToolName: "read", InputDelta: `{"paths":["app.go"]`},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-3", ToolName: "read", InputDelta: `}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-3", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-4", ToolName: "read", InputDelta: `{"paths":["app.go"]`},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-4", ToolName: "read", InputDelta: `}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-4", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetSessionConfig(SessionConfig{MaxRetries: defaultProviderRetryAttempts})

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "read app.go",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted || result.PendingRequestID != "" {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.Status != events.TurnStatusCompleted {
		t.Fatalf("turn = %#v", turn)
	}
	if turn.Error != "" {
		t.Fatalf("turn error = %q, want empty", turn.Error)
	}
	if len(client.requests) != 5 {
		t.Fatalf("provider requests = %d, want 5", len(client.requests))
	}
}

func TestTurnRunnerRunAllowsSingleDuplicateOnlyPassToRecover(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]`},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["app.go"]`},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "read app.go",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.Status != events.TurnStatusCompleted {
		t.Fatalf("turn = %#v", turn)
	}
	if turn.Error != "" {
		t.Fatalf("turn error = %q, want empty", turn.Error)
	}
	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want 3", len(client.requests))
	}
}

func TestTurnRunnerRunPausesAfterRepeatedFailedToolSteps(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{streams: repeatedInvalidWriteStreams(5, "notes.md")}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetSessionConfig(SessionConfig{MaxRetries: defaultProviderRetryAttempts})

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "write the notes file",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusPending || result.PendingRequestID == "" {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.Status != events.TurnStatusRunning {
		t.Fatalf("turn = %#v", turn)
	}
	if turn.Error != "" {
		t.Fatalf("turn error = %q, want empty", turn.Error)
	}
	pending := pendingQuestionRequestState(state, result.PendingRequestID)
	if pending == nil || pending.Purpose != events.QuestionPurposeTurnLoopResolution {
		t.Fatalf("pending question = %#v", pending)
	}
	if len(client.requests) != 5 {
		t.Fatalf("provider requests = %d, want 5", len(client.requests))
	}
}

func TestTurnRunnerRunPausesWhenFreshToolFailuresRepeatWithoutProgress(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n// package\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{streams: repeatedInvalidWriteStreams(5, "notes.md")}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetSessionConfig(SessionConfig{MaxRetries: defaultProviderRetryAttempts})

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "keep reading app.go",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusPending || result.PendingRequestID == "" {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.Status != events.TurnStatusRunning {
		t.Fatalf("turn = %#v", turn)
	}
	if turn.Error != "" {
		t.Fatalf("turn error = %q, want empty", turn.Error)
	}
	pending := pendingQuestionRequestState(state, result.PendingRequestID)
	if pending == nil || pending.Purpose != events.QuestionPurposeTurnLoopResolution {
		t.Fatalf("pending question = %#v", pending)
	}
	if len(client.requests) != 5 {
		t.Fatalf("provider requests = %d, want 5", len(client.requests))
	}
}

func TestTurnRunnerRunAllowsDistinctReadWindowsWithoutExplorationStall(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"],"offset":0,"limit":1}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["app.go"],"offset":1,"limit":1}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-3", ToolName: "read", InputDelta: `{"paths":["app.go"],"offset":2,"limit":1}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-3", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "keep reading app.go until ready",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.Status != events.TurnStatusCompleted {
		t.Fatalf("turn = %#v", turn)
	}
	if turn.Error != "" {
		t.Fatalf("turn error = %q, want empty", turn.Error)
	}
	if len(client.requests) != 4 {
		t.Fatalf("provider requests = %d, want 4", len(client.requests))
	}
}

func TestTurnRunnerRunAllowsSerialDistinctSearches(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n// package\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "search", InputDelta: `{"path":".","query":"package","max_matches":1,"case_sensitive":false,"mode":"lexical"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "search"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "search", InputDelta: `{"path":".","query":"package","max_matches":2,"case_sensitive":false,"mode":"lexical"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "search"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-3", ToolName: "search", InputDelta: `{"path":".","query":"package","max_matches":3,"case_sensitive":false,"mode":"lexical"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-3", ToolName: "search"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetSessionConfig(SessionConfig{MaxRetries: defaultProviderRetryAttempts})

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "keep reading app.go until ready",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.Status != events.TurnStatusCompleted {
		t.Fatalf("turn = %#v", turn)
	}
	if turn.Error != "" {
		t.Fatalf("turn error = %q, want empty", turn.Error)
	}
	if len(client.requests) != 4 {
		t.Fatalf("provider requests = %d, want 4", len(client.requests))
	}
}

func TestTurnRunnerRunAllowsSerialExplorationBeforeProviderRequestLimit(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	for _, name := range []string{"app.go", "config.go", "server.go"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("package main\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "locate", InputDelta: `{"path":".","query":"app.go","max_matches":10,"include_hidden":false}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "locate"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "locate", InputDelta: `{"path":".","query":"config.go","max_matches":10,"include_hidden":false}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "locate"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-3", ToolName: "locate", InputDelta: `{"path":".","query":"server.go","max_matches":10,"include_hidden":false}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-3", ToolName: "locate"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetSessionConfig(SessionConfig{MaxRetries: defaultProviderRetryAttempts})

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "inspect the project",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.Status != events.TurnStatusCompleted {
		t.Fatalf("turn = %#v", turn)
	}
	if len(client.requests) != 4 {
		t.Fatalf("provider requests = %d, want 4", len(client.requests))
	}
}

func TestTurnRunnerRunAllowsNonConsecutiveDuplicateOnlyReads(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	for _, name := range []string{"app.go", "config.go"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("package main\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-seed-app", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-seed-app", ToolName: "read"},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-seed-config", ToolName: "read", InputDelta: `{"paths":["config.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-seed-config", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-repeat-app-1", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-repeat-app-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-repeat-config", ToolName: "read", InputDelta: `{"paths":["config.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-repeat-config", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-repeat-app-2", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-repeat-app-2", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetSessionConfig(SessionConfig{MaxRetries: defaultProviderRetryAttempts})

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "inspect app.go and config.go",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted || result.PendingRequestID != "" {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 5 {
		t.Fatalf("provider requests = %d, want 5", len(client.requests))
	}
}

func TestTurnRunnerRunAsksQuestionWhenProviderRequestLimitIsExceeded(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]`},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["app.go"],"offset":0,"limit":1}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetSessionConfig(SessionConfig{
		MaxProviderRequestsPerTurn: 2,
		MaxRetries:                 defaultProviderRetryAttempts,
	})

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "read app.go repeatedly",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusPending || result.PendingRequestID == "" {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.Status != events.TurnStatusRunning {
		t.Fatalf("turn = %#v", turn)
	}
	if turn.Error != "" {
		t.Fatalf("turn error = %q, want empty", turn.Error)
	}
	pending := pendingQuestionRequestState(state, result.PendingRequestID)
	if pending == nil || pending.Purpose != events.QuestionPurposeTurnLoopResolution {
		t.Fatalf("pending question = %#v", pending)
	}
	if !strings.Contains(pending.Question, "assistant roundtrip limit") {
		t.Fatalf("pending question text = %q", pending.Question)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
}

func TestTurnRunnerRunAllowsUnlimitedStepsWhenDisabled(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte(numberedLines(defaultMaxProviderRequestsPerTurn+1)), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{streams: readLineRoundtripStreams(defaultMaxProviderRequestsPerTurn + 1)}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetSessionConfig(SessionConfig{
		MaxProviderRequestsPerTurn: -1,
		MaxRetries:                 defaultProviderRetryAttempts,
	})

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "read app.go until done",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != defaultMaxProviderRequestsPerTurn+2 {
		t.Fatalf("provider requests = %d, want %d", len(client.requests), defaultMaxProviderRequestsPerTurn+2)
	}
}

func TestTurnRunnerRunAllowsProviderRequestLimitAboveDefault(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte(numberedLines(defaultMaxProviderRequestsPerTurn+1)), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{streams: readLineRoundtripStreams(defaultMaxProviderRequestsPerTurn + 1)}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetSessionConfig(SessionConfig{
		MaxProviderRequestsPerTurn: defaultMaxProviderRequestsPerTurn + 2,
		MaxRetries:                 defaultProviderRetryAttempts,
	})

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "read app.go until done",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != defaultMaxProviderRequestsPerTurn+2 {
		t.Fatalf("provider requests = %d, want %d", len(client.requests), defaultMaxProviderRequestsPerTurn+2)
	}
}

func numberedLines(count int) string {
	var builder strings.Builder
	for i := 1; i <= count; i++ {
		_, _ = fmt.Fprintf(&builder, "line %d\n", i)
	}
	return builder.String()
}

func readLineRoundtripStreams(toolRoundtrips int) []provider.Stream {
	streams := make([]provider.Stream, 0, toolRoundtrips+1)
	for i := 1; i <= toolRoundtrips; i++ {
		callID := fmt.Sprintf("call-%d", i)
		streams = append(streams, provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindToolCallDelta, ToolCallID: callID, ToolName: "read", InputDelta: fmt.Sprintf(`{"paths":["app.go"],"offset":%d,"limit":1}`, i)},
			{Kind: provider.EventKindToolCallDone, ToolCallID: callID, ToolName: "read"},
		}))
	}
	streams = append(streams, provider.NewSliceStream([]provider.Event{
		{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
	}))
	return streams
}

func TestTurnRunnerRunAppendsTurnErrorOnProviderFailure(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	runner, err := NewTurnRunner(eng, shaper, &fakeProvider{
		err: errors.New("stream failed"),
	}, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "hello",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusFailed {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Turns["turn-1"].Status != events.TurnStatusFailed {
		t.Fatalf("turn status = %q", state.Turns["turn-1"].Status)
	}
	if state.Turns["turn-1"].Error == "" {
		t.Fatal("turn error missing")
	}
}

func TestTurnRunnerRunRetriesRetryableProviderFailureBeforeFailingTurn(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		errs: []error{
			&provider.ProviderError{Message: "github-copilot/gpt-5-mini: unexpected EOF", Retryable: true, RetryAfter: 1500 * time.Millisecond},
		},
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "hello"}}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	waited := time.Duration(0)
	runner.wait = func(_ context.Context, delay time.Duration) error {
		waited = delay
		return nil
	}
	runner.SetSessionConfig(SessionConfig{MaxRetries: 2})

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "hello",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if waited != 1500*time.Millisecond {
		t.Fatalf("waited = %s, want 1.5s", waited)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.Status != events.TurnStatusCompleted {
		t.Fatalf("turn status = %q", turn.Status)
	}
	if turn.Error != "" {
		t.Fatalf("turn error = %q, want empty", turn.Error)
	}
}

func TestTurnRunnerRunUsesConfiguredAgentOutputBudget(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"}}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"anthropic": {{
				ID:              "claude-sonnet-4-6",
				ContextSize:     200000,
				MaxInputTokens:  200000,
				MaxOutputTokens: provider.SuggestedMaxOutputTokens(provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"}),
			}},
		},
	})

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		AgentID:   "builder",
		UserText:  "hello",
		Fragments: baseFragments(),
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	if client.requests[0].MaxOutputTokens != defaultOutputBudgetAgentTurn {
		t.Fatalf("request max output tokens = %d, want agent budget %d", client.requests[0].MaxOutputTokens, defaultOutputBudgetAgentTurn)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.Status != events.TurnStatusCompleted {
		t.Fatalf("turn status = %q", turn.Status)
	}
	if turn.AssistantText != "done" {
		t.Fatalf("assistant text = %q, want done", turn.AssistantText)
	}
}

func TestTurnRunnerRunContinuesAfterAnthropicMaxTokensWithToolProgress(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			&errorAfterEventsStream{
				events: []provider.Event{
					{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
					{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
				},
				err: &provider.ProviderError{Message: "anthropic: response stopped at max_tokens", Cause: provider.ErrAnthropicMaxTokensExceeded},
			},
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"}}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		AgentID:   "builder",
		UserText:  "read app.go",
		Fragments: baseFragments(),
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}

	second := client.requests[1]
	if len(second.Inputs) != 3 {
		t.Fatalf("second request inputs = %#v", second.Inputs)
	}
	if second.Inputs[1].Kind != provider.InputKindToolCall || second.Inputs[1].ToolName != "read" {
		t.Fatalf("second input[1] = %#v", second.Inputs[1])
	}
	if second.Inputs[2].Kind != provider.InputKindToolResult || second.Inputs[2].ToolName != "read" {
		t.Fatalf("second input[2] = %#v", second.Inputs[2])
	}
}

func TestTurnRunnerRunHonorsConfiguredMaxRetries(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		errs: []error{
			&provider.ProviderError{Message: "google/gemini-2.5-flash: unavailable", StatusCode: 503, Retryable: true},
			&provider.ProviderError{Message: "google/gemini-2.5-flash: unavailable", StatusCode: 503, Retryable: true},
		},
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "hello"}}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	waitCalls := 0
	runner.wait = func(_ context.Context, delay time.Duration) error {
		waitCalls++
		if delay != time.Second {
			t.Fatalf("delay = %s, want 1s", delay)
		}
		return nil
	}
	runner.SetSessionConfig(SessionConfig{MaxRetries: 1})

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "hello",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusFailed {
		t.Fatalf("result = %#v, want failed after exhausting retries", result)
	}
	if waitCalls != 1 {
		t.Fatalf("wait calls = %d, want 1", waitCalls)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.Status != events.TurnStatusFailed {
		t.Fatalf("turn status = %q, want failed", turn.Status)
	}
	if turn.Error != "The model is busy right now. Please try again in a moment." {
		t.Fatalf("turn error = %q", turn.Error)
	}
}

func TestTurnRunnerRunDoesNotRetryRetryableStreamEOFAfterOutput(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			&errorAfterEventsStream{
				events: []provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "hel"}},
				err:    io.ErrUnexpectedEOF,
			},
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.now = func() time.Time { return time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC) }
	runner.wait = func(_ context.Context, delay time.Duration) error {
		t.Fatalf("unexpected retry wait: %s", delay)
		return nil
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "hello",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusFailed {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn.AssistantText != "" {
		t.Fatalf("assistant text = %q", turn.AssistantText)
	}
	if turn.StreamingText != "" {
		t.Fatalf("streaming text = %q, want empty", turn.StreamingText)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	if len(turn.ProviderAttempts) != 1 || turn.ProviderAttempts[0].RetrySkippedReason != providerRetrySkippedCompletionTokens {
		t.Fatalf("provider attempts = %#v, want retry skipped after streamed output", turn.ProviderAttempts)
	}
}

func TestTurnRunnerRunDoesNotRetryRetryableStreamEOFAfterPartialToolCall(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			&errorAfterEventsStream{
				events: []provider.Event{{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: tool.ReadToolName, InputDelta: `{"paths"`}},
				err:    io.ErrUnexpectedEOF,
			},
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.wait = func(_ context.Context, delay time.Duration) error {
		t.Fatalf("unexpected retry wait: %s", delay)
		return nil
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "hello",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusFailed {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	if len(turn.ProviderAttempts) != 1 || turn.ProviderAttempts[0].RetrySkippedReason != providerRetrySkippedToolCallStarted {
		t.Fatalf("provider attempts = %#v, want retry skipped after partial tool call", turn.ProviderAttempts)
	}
}

func TestTurnRunnerRunPersistsReasoningTranscript(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindReasoningDelta, ReasoningDelta: "Inspecting the provider contract."},
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
		})},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "inspect the runtime contract",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.ReasoningText != "Inspecting the provider contract." {
		t.Fatalf("reasoning text = %q", turn.ReasoningText)
	}
	if len(turn.Transcript) < 2 {
		t.Fatalf("transcript = %#v, want reasoning and assistant entries", turn.Transcript)
	}
	if turn.Transcript[1].Kind != events.TranscriptEntryReasoning || turn.Transcript[1].Text != "Inspecting the provider contract." {
		t.Fatalf("reasoning transcript = %#v", turn.Transcript[1])
	}
}

func TestTurnRunnerRunPreservesReasoningSegments(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindReasoningDelta, ReasoningDelta: "first step", ReasoningSegmentID: "seg-1"},
			{Kind: provider.EventKindReasoningDelta, ReasoningDelta: " extended", ReasoningSegmentID: "seg-1"},
			{Kind: provider.EventKindReasoningDelta, ReasoningDelta: "second step", ReasoningSegmentID: "seg-2"},
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
		})},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "inspect the runtime contract",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if len(turn.Transcript) < 3 {
		t.Fatalf("transcript = %#v, want user + 2 reasoning + assistant", turn.Transcript)
	}
	if turn.Transcript[1].Kind != events.TranscriptEntryReasoning || turn.Transcript[1].Text != "first step extended" || turn.Transcript[1].SegmentID != "seg-1" {
		t.Fatalf("turn.Transcript[1] = %#v", turn.Transcript[1])
	}
	if turn.Transcript[2].Kind != events.TranscriptEntryReasoning || turn.Transcript[2].Text != "second step" || turn.Transcript[2].SegmentID != "seg-2" {
		t.Fatalf("turn.Transcript[2] = %#v", turn.Transcript[2])
	}
}

func TestTurnRunnerRunContinuesAfterRecoverableToolPreflightError(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "drafting report"},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "write", InputDelta: `{"path":"CODE_REVIEW_RECOMMENDATIONS.md"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "write"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: " fixing write"},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "write", InputDelta: `{"path":"CODE_REVIEW_RECOMMENDATIONS.md","content":"# Recommendations\n- tighten the search tool contract\n"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "write"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "review the code and write recommendations to a markdown file",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want 3", len(client.requests))
	}
	second := client.requests[1]
	if len(second.Inputs) != 3 {
		t.Fatalf("second request inputs = %#v", second.Inputs)
	}
	callInput := second.Inputs[1]
	if callInput.Kind != provider.InputKindToolCall || callInput.ToolName != "write" {
		t.Fatalf("tool call = %#v", callInput)
	}
	resultInput := second.Inputs[2]
	if resultInput.Kind != provider.InputKindToolResult {
		t.Fatalf("tool result = %#v", resultInput)
	}
	if !strings.Contains(resultInput.Error, "`write` failed.") || !strings.Contains(resultInput.Error, "content is required") || !strings.Contains(resultInput.Error, `Example: {"path":"file.txt","content":"hello\n"}.`) {
		t.Fatalf("tool result error = %q", resultInput.Error)
	}

	content, err := os.ReadFile(filepath.Join(root, "CODE_REVIEW_RECOMMENDATIONS.md"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(content), "# Recommendations") || !strings.Contains(string(content), "- tighten the search tool contract") {
		t.Fatalf("content = %q", string(content))
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	first := state.Turns["turn-1"].ToolCalls["call-1"]
	if first == nil || !first.Completed || !strings.Contains(first.Error, "content is required") {
		t.Fatalf("first tool call = %#v", first)
	}
	secondCall := state.Turns["turn-1"].ToolCalls["call-2"]
	if secondCall == nil || !secondCall.Completed || !strings.Contains(secondCall.Output, "wrote") {
		t.Fatalf("second tool call = %#v", secondCall)
	}
}

func TestTurnRunnerRunExecutesProviderDeclaredWriteBatch(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("before\nmiddle\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-read", ToolName: "read", InputDelta: `{"paths":["main.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-read", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "write", InputDelta: `{"path":"main.go","content":"after\nmiddle\n"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "write"},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "write", InputDelta: `{"path":"main.go","content":"after\ncenter\n"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "write"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "rewrite main.go",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want 3", len(client.requests))
	}
	third := client.requests[2]
	if len(third.Inputs) != 5 {
		t.Fatalf("third request inputs = %#v", third.Inputs)
	}
	firstResult := third.Inputs[4]
	if firstResult.Kind != provider.InputKindToolResult || firstResult.CallID != "call-1" || firstResult.ToolName != "write" {
		t.Fatalf("first tool result = %#v", firstResult)
	}
	if !strings.Contains(firstResult.Output, "wrote") {
		t.Fatalf("first tool result output = %q", firstResult.Output)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got := string(content); got != "after\nmiddle\n" {
		t.Fatalf("content = %q", got)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.ToolCalls["call-1"] == nil || !turn.ToolCalls["call-1"].Completed {
		t.Fatalf("call-1 = %#v", turn.ToolCalls["call-1"])
	}
	if call := turn.ToolCalls["call-2"]; call != nil {
		t.Fatalf("call-2 should not have executed after write boundary: %#v", call)
	}
}

func TestTurnRunnerRunExecutesProviderDeclaredReadWriteBatch(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\nconst value = 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "write", InputDelta: `{"path":"app.go","content":"package main\nconst value = 2\n"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "write"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "read then rewrite app.go",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	second := client.requests[1]
	if len(second.Inputs) != 5 {
		t.Fatalf("second request inputs = %#v", second.Inputs)
	}
	toolResult := second.Inputs[3]
	if toolResult.Kind != provider.InputKindToolResult || toolResult.CallID != "call-1" || toolResult.ToolName != "read" {
		t.Fatalf("tool result = %#v", toolResult)
	}
	if !strings.Contains(toolResult.Output, "1: package main") {
		t.Fatalf("tool result output = %q", toolResult.Output)
	}
	writeResult := second.Inputs[4]
	if writeResult.Kind != provider.InputKindToolResult || writeResult.CallID != "call-2" || writeResult.ToolName != "write" {
		t.Fatalf("write result = %#v", writeResult)
	}
	if !strings.Contains(writeResult.Output, "wrote") {
		t.Fatalf("write result output = %q", writeResult.Output)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got := string(content); got != "package main\nconst value = 2\n" {
		t.Fatalf("content = %q", got)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.ToolCalls["call-1"] == nil || !turn.ToolCalls["call-1"].Completed {
		t.Fatalf("call-1 = %#v", turn.ToolCalls["call-1"])
	}
	if turn.ToolCalls["call-2"] == nil || !turn.ToolCalls["call-2"].Completed {
		t.Fatalf("call-2 = %#v", turn.ToolCalls["call-2"])
	}
}

func TestTurnRunnerRunCompletesReadWriteAssistantAcrossSteps(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	initial := "package main\nconst value = 1\n"
	updated := "package main\nconst value = 2\n"
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "write", InputDelta: `{"path":"app.go","content":"package main\nconst value = 2\n"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "write"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "read then rewrite app.go",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want 3", len(client.requests))
	}
	second := client.requests[1]
	if len(second.Inputs) != 3 {
		t.Fatalf("second request inputs = %#v", second.Inputs)
	}
	if second.Inputs[2].Kind != provider.InputKindToolResult || second.Inputs[2].CallID != "call-1" {
		t.Fatalf("second request tool result = %#v", second.Inputs[2])
	}
	if !strings.Contains(second.Inputs[2].Output, "1: package main") || !strings.Contains(second.Inputs[2].Output, "2: const value = 1") {
		t.Fatalf("second request tool result output = %q", second.Inputs[2].Output)
	}
	third := client.requests[2]
	if len(third.Inputs) != 5 {
		t.Fatalf("third request inputs = %#v", third.Inputs)
	}
	if third.Inputs[2].Kind != provider.InputKindToolResult || third.Inputs[2].CallID != "call-1" {
		t.Fatalf("third request read result = %#v", third.Inputs[2])
	}
	if third.Inputs[3].Kind != provider.InputKindToolCall || third.Inputs[3].CallID != "call-2" || third.Inputs[3].ToolName != "write" {
		t.Fatalf("third request write call = %#v", third.Inputs[3])
	}
	if third.Inputs[4].Kind != provider.InputKindToolResult || third.Inputs[4].CallID != "call-2" || third.Inputs[4].ToolName != "write" {
		t.Fatalf("third request write result = %#v", third.Inputs[4])
	}
	if !strings.HasPrefix(third.Inputs[4].Output, "wrote ") || !strings.HasSuffix(third.Inputs[4].Output, "/app.go") {
		t.Fatalf("third request write result output = %q", third.Inputs[4].Output)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got := string(content); got != updated {
		t.Fatalf("content = %q, want %q", got, updated)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.AssistantText != "done" {
		t.Fatalf("assistant text = %q", turn.AssistantText)
	}
	if turn.ToolCalls["call-1"] == nil || !turn.ToolCalls["call-1"].Completed {
		t.Fatalf("call-1 = %#v", turn.ToolCalls["call-1"])
	}
	if turn.ToolCalls["call-2"] == nil || !turn.ToolCalls["call-2"].Completed || !turn.ToolCalls["call-2"].Succeeded {
		t.Fatalf("call-2 = %#v", turn.ToolCalls["call-2"])
	}
}

func TestTurnRunnerRunCompletesSearchReadAssistantAcrossSteps(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\nconst value = 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "search", InputDelta: `{"path":".","query":"value","max_matches":5,"case_sensitive":false,"mode":"lexical"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "search"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "search then inspect app.go",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want 3", len(client.requests))
	}
	second := client.requests[1]
	if len(second.Inputs) != 3 {
		t.Fatalf("second request inputs = %#v", second.Inputs)
	}
	if second.Inputs[2].Kind != provider.InputKindToolResult || second.Inputs[2].CallID != "call-1" || second.Inputs[2].ToolName != "search" || !strings.Contains(second.Inputs[2].Output, "app.go") {
		t.Fatalf("second request search result = %#v", second.Inputs[2])
	}
	third := client.requests[2]
	if len(third.Inputs) != 5 {
		t.Fatalf("third request inputs = %#v", third.Inputs)
	}
	if third.Inputs[2].Kind != provider.InputKindToolResult || third.Inputs[2].CallID != "call-1" || third.Inputs[2].ToolName != "search" {
		t.Fatalf("third request search result = %#v", third.Inputs[2])
	}
	if third.Inputs[3].Kind != provider.InputKindToolCall || third.Inputs[3].CallID != "call-2" || third.Inputs[3].ToolName != "read" {
		t.Fatalf("third request read call = %#v", third.Inputs[3])
	}
	if third.Inputs[4].Kind != provider.InputKindToolResult || third.Inputs[4].CallID != "call-2" || third.Inputs[4].ToolName != "read" {
		t.Fatalf("third request read result = %#v", third.Inputs[4])
	}
	if !strings.Contains(third.Inputs[4].Output, "1: package main") || !strings.Contains(third.Inputs[4].Output, "2: const value = 1") {
		t.Fatalf("third request read result output = %q", third.Inputs[4].Output)
	}
}

func TestTurnRunnerRunGroupsParallelToolCallsBeforeResultsWithinSingleStep(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, tools, eng, shaper := newTurnRunnerTestDepsWithStore(t, store)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\nconst value = 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "search", InputDelta: `{"path":".","query":"value","max_matches":5,"case_sensitive":false,"mode":"lexical"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "search"},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "search then inspect app.go",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	second := client.requests[1]
	if len(second.Inputs) != 5 {
		t.Fatalf("second request inputs = %#v", second.Inputs)
	}
	if second.Inputs[1].Kind != provider.InputKindToolCall || second.Inputs[1].CallID != "call-1" || second.Inputs[1].ToolName != "search" {
		t.Fatalf("search call = %#v", second.Inputs[1])
	}
	if second.Inputs[2].Kind != provider.InputKindToolCall || second.Inputs[2].CallID != "call-2" || second.Inputs[2].ToolName != "read" {
		t.Fatalf("read call = %#v", second.Inputs[2])
	}
	if second.Inputs[3].Kind != provider.InputKindToolResult || second.Inputs[3].CallID != "call-1" || second.Inputs[3].ToolName != "search" {
		t.Fatalf("search result = %#v", second.Inputs[3])
	}
	if !strings.Contains(second.Inputs[3].Output, "app.go") {
		t.Fatalf("search result output = %q", second.Inputs[3].Output)
	}
	if second.Inputs[4].Kind != provider.InputKindToolResult || second.Inputs[4].CallID != "call-2" || second.Inputs[4].ToolName != "read" {
		t.Fatalf("read result = %#v", second.Inputs[4])
	}
	if !strings.Contains(second.Inputs[4].Output, "1: package main") {
		t.Fatalf("read result output = %q", second.Inputs[4].Output)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.ToolCalls["call-1"] == nil || !turn.ToolCalls["call-1"].Completed {
		t.Fatalf("call-1 = %#v", turn.ToolCalls["call-1"])
	}
	if turn.ToolCalls["call-2"] == nil || !turn.ToolCalls["call-2"].Completed {
		t.Fatalf("call-2 = %#v", turn.ToolCalls["call-2"])
	}
	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	batchEvents := 0
	for _, event := range replayed {
		payload, ok := event.Payload.(events.ToolCallBatchPayload)
		if !ok {
			continue
		}
		batchEvents++
		if len(payload.CallIDs) != 2 || payload.CallIDs[0] != "call-1" || payload.CallIDs[1] != "call-2" {
			t.Fatalf("ToolCallBatchPayload.CallIDs = %#v", payload.CallIDs)
		}
	}
	if batchEvents != 1 {
		t.Fatalf("batchEvents = %d, want 1", batchEvents)
	}
}

func TestTurnRunnerRunStopsBeforeDuplicateProviderToolCallID(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, tools, eng, shaper := newTurnRunnerTestDepsWithStore(t, store)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\nconst value = 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"],"offset":1,"limit":1}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "inspect app.go",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	second := client.requests[1]
	if len(second.Inputs) != 3 {
		t.Fatalf("second request inputs = %#v", second.Inputs)
	}
	if second.Inputs[1].Kind != provider.InputKindToolCall || second.Inputs[1].CallID != "call-1" {
		t.Fatalf("tool call = %#v", second.Inputs[1])
	}
	if second.Inputs[2].Kind != provider.InputKindToolResult || second.Inputs[2].CallID != "call-1" {
		t.Fatalf("tool result = %#v", second.Inputs[2])
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	declared := 0
	started := 0
	ended := 0
	for _, event := range replayed {
		switch event.Payload.(type) {
		case events.ToolCallDeclaredPayload:
			declared++
		case events.ToolExecStartPayload:
			started++
		case events.ToolExecEndPayload:
			ended++
		}
	}
	if declared != 1 || started != 1 || ended != 1 {
		t.Fatalf("tool event counts declared=%d started=%d ended=%d, want one each", declared, started, ended)
	}
}

func TestTurnRunnerRunClosesProviderStreamBeforeSpeculativeWriteAfterExplorationBatch(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\nconst value = 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first := &errorAfterEventsStream{
		events: []provider.Event{
			{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "search", InputDelta: `{"path":".","query":"value","max_matches":5,"case_sensitive":false,"mode":"lexical"}`},
			{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "search"},
			{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
			{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-3", ToolName: "write", InputDelta: `{"path":"app.go","content":"package main\nconst value = 2\n"}`},
		},
	}
	client := &fakeProvider{
		streams: []provider.Stream{
			first,
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"}}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "read then rewrite app.go",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if !first.closed {
		t.Fatal("first provider stream was not closed before the speculative write")
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.ToolCalls["call-1"] == nil || !turn.ToolCalls["call-1"].Completed {
		t.Fatalf("call-1 = %#v", turn.ToolCalls["call-1"])
	}
	if turn.ToolCalls["call-2"] == nil || !turn.ToolCalls["call-2"].Completed {
		t.Fatalf("call-2 = %#v", turn.ToolCalls["call-2"])
	}
	if turn.ToolCalls["call-3"] != nil {
		t.Fatalf("call-3 should not have executed: %#v", turn.ToolCalls["call-3"])
	}
}

func TestTurnRunnerRunRetriesFromDurableBatchBoundaryAfterProviderFailure(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\nconst value = 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first := &errorAfterEventsStream{
		events: []provider.Event{
			{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "search", InputDelta: `{"path":".","query":"value","max_matches":5,"case_sensitive":false,"mode":"lexical"}`},
			{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "search"},
			{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
			{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
		},
		err: &provider.ProviderError{Message: "openai/gpt-5: unavailable", StatusCode: 503, Retryable: true},
	}
	client := &fakeProvider{
		streams: []provider.Stream{
			first,
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"}}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.wait = func(context.Context, time.Duration) error { return nil }

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "search then inspect app.go",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}

	second := client.requests[1]
	if len(second.Inputs) != 5 {
		t.Fatalf("second request inputs = %#v", second.Inputs)
	}
	if second.Inputs[0].Kind != provider.InputKindUserMessage || second.Inputs[0].Content != "search then inspect app.go" {
		t.Fatalf("input[0] = %#v", second.Inputs[0])
	}
	if second.Inputs[1].Kind != provider.InputKindToolCall || second.Inputs[1].CallID != "call-1" || second.Inputs[1].ToolName != "search" {
		t.Fatalf("input[1] = %#v", second.Inputs[1])
	}
	if second.Inputs[2].Kind != provider.InputKindToolCall || second.Inputs[2].CallID != "call-2" || second.Inputs[2].ToolName != "read" {
		t.Fatalf("input[2] = %#v", second.Inputs[2])
	}
	if second.Inputs[3].Kind != provider.InputKindToolResult || second.Inputs[3].CallID != "call-1" || second.Inputs[3].ToolName != "search" {
		t.Fatalf("input[3] = %#v", second.Inputs[3])
	}
	if second.Inputs[4].Kind != provider.InputKindToolResult || second.Inputs[4].CallID != "call-2" || second.Inputs[4].ToolName != "read" {
		t.Fatalf("input[4] = %#v", second.Inputs[4])
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.ToolCalls["call-1"] == nil || !turn.ToolCalls["call-1"].Completed {
		t.Fatalf("call-1 = %#v", turn.ToolCalls["call-1"])
	}
	if turn.ToolCalls["call-2"] == nil || !turn.ToolCalls["call-2"].Completed {
		t.Fatalf("call-2 = %#v", turn.ToolCalls["call-2"])
	}
}

func TestTurnRunnerRunBatchesGitStatusAndReadWithinSingleStep(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	root := t.TempDir()
	cmd := exec.Command("git", "init", "-q", root)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init error = %v output=%s", err, output)
	}
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\nconst value = 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "git_status", InputDelta: `{}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "git_status"},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "check git status and inspect app.go",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	second := client.requests[1]
	if len(second.Inputs) != 5 {
		t.Fatalf("second request inputs = %#v", second.Inputs)
	}
	if second.Inputs[1].Kind != provider.InputKindToolCall || second.Inputs[1].CallID != "call-1" || second.Inputs[1].ToolName != "git_status" {
		t.Fatalf("git_status call = %#v", second.Inputs[1])
	}
	if second.Inputs[2].Kind != provider.InputKindToolCall || second.Inputs[2].CallID != "call-2" || second.Inputs[2].ToolName != "read" {
		t.Fatalf("read call = %#v", second.Inputs[2])
	}
	if second.Inputs[3].Kind != provider.InputKindToolResult || second.Inputs[3].CallID != "call-1" || second.Inputs[3].ToolName != "git_status" {
		t.Fatalf("git_status result = %#v", second.Inputs[3])
	}
	if !strings.Contains(second.Inputs[3].Output, "command: git status --porcelain=v1 --branch --untracked-files=all -- .") {
		t.Fatalf("git_status result output = %q", second.Inputs[3].Output)
	}
	if second.Inputs[4].Kind != provider.InputKindToolResult || second.Inputs[4].CallID != "call-2" || second.Inputs[4].ToolName != "read" {
		t.Fatalf("read result = %#v", second.Inputs[4])
	}
	if !strings.Contains(second.Inputs[4].Output, "1: package main") {
		t.Fatalf("read result output = %q", second.Inputs[4].Output)
	}
}

func TestTurnRunnerRunNormalizesEmptyToolCallInput(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	root := t.TempDir()
	cmd := exec.Command("git", "init", "-q", root)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init error = %v output=%s", err, output)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "git_status"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "check git status",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	if call == nil {
		t.Fatal("tool call missing")
	}
	if call.Input != `{}` {
		t.Fatalf("tool call input = %q, want {}", call.Input)
	}
}

func TestTurnRunnerRunBatchesGitDiffAndReadWithinSingleStep(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	root := t.TempDir()
	cmd := exec.Command("git", "init", "-q", root)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init error = %v output=%s", err, output)
	}
	for _, args := range [][]string{
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test User"},
	} {
		cmd = exec.Command("git", args...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v error = %v output=%s", args, err, output)
		}
	}
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\nconst value = 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	for _, args := range [][]string{
		{"add", "."},
		{"commit", "-q", "-m", "init"},
	} {
		cmd = exec.Command("git", args...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v error = %v output=%s", args, err, output)
		}
	}
	if err := os.WriteFile(path, []byte("package main\nconst value = 2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "git_diff", InputDelta: `{"staged":false}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "git_diff"},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "check diff and inspect app.go",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	second := client.requests[1]
	if len(second.Inputs) != 5 {
		t.Fatalf("second request inputs = %#v", second.Inputs)
	}
	if second.Inputs[1].Kind != provider.InputKindToolCall || second.Inputs[1].CallID != "call-1" || second.Inputs[1].ToolName != "git_diff" {
		t.Fatalf("git_diff call = %#v", second.Inputs[1])
	}
	if second.Inputs[2].Kind != provider.InputKindToolCall || second.Inputs[2].CallID != "call-2" || second.Inputs[2].ToolName != "read" {
		t.Fatalf("read call = %#v", second.Inputs[2])
	}
	if second.Inputs[3].Kind != provider.InputKindToolResult || second.Inputs[3].CallID != "call-1" || second.Inputs[3].ToolName != "git_diff" {
		t.Fatalf("git_diff result = %#v", second.Inputs[3])
	}
	if !strings.Contains(second.Inputs[3].Output, "diff --git a/app.go b/app.go") {
		t.Fatalf("git_diff result output = %q", second.Inputs[3].Output)
	}
	if second.Inputs[4].Kind != provider.InputKindToolResult || second.Inputs[4].CallID != "call-2" || second.Inputs[4].ToolName != "read" {
		t.Fatalf("read result = %#v", second.Inputs[4])
	}
	if !strings.Contains(second.Inputs[4].Output, "2: const value = 2") {
		t.Fatalf("read result output = %q", second.Inputs[4].Output)
	}
}

func TestTurnRunnerRunClosesProviderStreamBeforeBashAfterBatchableResult(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\nconst value = 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first := &errorAfterEventsStream{
		events: []provider.Event{
			{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
			{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: tool.BashToolName, InputDelta: `{"cmd":"printf hi"}`},
		},
	}
	client := &fakeProvider{
		streams: []provider.Stream{
			first,
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"}}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "inspect app.go then decide",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if !first.closed {
		t.Fatal("first provider stream was not closed before the speculative bash call")
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.ToolCalls["call-1"] == nil || !turn.ToolCalls["call-1"].Completed {
		t.Fatalf("call-1 = %#v", turn.ToolCalls["call-1"])
	}
	if turn.ToolCalls["call-2"] != nil {
		t.Fatalf("call-2 should not have executed: %#v", turn.ToolCalls["call-2"])
	}
}

func TestTurnRunnerRunBatchesMemoryListAndReadWithinSingleStep(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	tools.SetMemoryService(NewMemoryService())

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\nconst value = 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: tool.MemoryToolName, InputDelta: `{"action":"list","content":null,"id":null}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: tool.MemoryToolName},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "list memories and inspect app.go",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	second := client.requests[1]
	if len(second.Inputs) != 5 {
		t.Fatalf("second request inputs = %#v", second.Inputs)
	}
	if second.Inputs[1].Kind != provider.InputKindToolCall || second.Inputs[1].CallID != "call-1" || second.Inputs[1].ToolName != tool.MemoryToolName {
		t.Fatalf("memory call = %#v", second.Inputs[1])
	}
	if second.Inputs[2].Kind != provider.InputKindToolCall || second.Inputs[2].CallID != "call-2" || second.Inputs[2].ToolName != "read" {
		t.Fatalf("read call = %#v", second.Inputs[2])
	}
	if second.Inputs[3].Kind != provider.InputKindToolResult || second.Inputs[3].CallID != "call-1" || second.Inputs[3].ToolName != tool.MemoryToolName {
		t.Fatalf("memory result = %#v", second.Inputs[3])
	}
	if !strings.Contains(second.Inputs[3].Output, `"memories":[]`) {
		t.Fatalf("memory result output = %q", second.Inputs[3].Output)
	}
	if second.Inputs[4].Kind != provider.InputKindToolResult || second.Inputs[4].CallID != "call-2" || second.Inputs[4].ToolName != "read" {
		t.Fatalf("read result = %#v", second.Inputs[4])
	}
}

func TestTurnRunnerRunBatchesTaskWorkflowListAndReadWithinSingleStep(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\nconst value = 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := sessions.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "setup-turn",
		Title:     "Existing task",
	}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: tool.TaskWorkflowToolName, InputDelta: `{"action":"list"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: tool.TaskWorkflowToolName},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "list tasks and inspect app.go",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	second := client.requests[1]
	if len(second.Inputs) != 5 {
		t.Fatalf("second request inputs = %#v", second.Inputs)
	}
	if second.Inputs[1].Kind != provider.InputKindToolCall || second.Inputs[1].CallID != "call-1" || second.Inputs[1].ToolName != tool.TaskWorkflowToolName {
		t.Fatalf("task_workflow call = %#v", second.Inputs[1])
	}
	if second.Inputs[2].Kind != provider.InputKindToolCall || second.Inputs[2].CallID != "call-2" || second.Inputs[2].ToolName != "read" {
		t.Fatalf("read call = %#v", second.Inputs[2])
	}
	if second.Inputs[3].Kind != provider.InputKindToolResult || second.Inputs[3].CallID != "call-1" || second.Inputs[3].ToolName != tool.TaskWorkflowToolName {
		t.Fatalf("task_workflow result = %#v", second.Inputs[3])
	}
	if !strings.Contains(second.Inputs[3].Output, `"title":"Existing task"`) {
		t.Fatalf("task_workflow result output = %q", second.Inputs[3].Output)
	}
	if second.Inputs[4].Kind != provider.InputKindToolResult || second.Inputs[4].CallID != "call-2" || second.Inputs[4].ToolName != "read" {
		t.Fatalf("read result = %#v", second.Inputs[4])
	}
}

func TestTurnRunnerRunExecutesTaskWorkflowCreatesFromSingleProviderStep(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: tool.TaskWorkflowToolName, InputDelta: `{"action":"create","task_id":"task-1","title":"First task"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: tool.TaskWorkflowToolName},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: tool.TaskWorkflowToolName, InputDelta: `{"action":"create","task_id":"task-2","title":"Second task"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: tool.TaskWorkflowToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "create two workflow tasks",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	second := client.requests[1]
	if len(second.Inputs) != 5 {
		t.Fatalf("second request inputs = %#v", second.Inputs)
	}
	if second.Inputs[1].Kind != provider.InputKindToolCall || second.Inputs[1].CallID != "call-1" || second.Inputs[1].ToolName != tool.TaskWorkflowToolName {
		t.Fatalf("first task_workflow call = %#v", second.Inputs[1])
	}
	if second.Inputs[2].Kind != provider.InputKindToolCall || second.Inputs[2].CallID != "call-2" || second.Inputs[2].ToolName != tool.TaskWorkflowToolName {
		t.Fatalf("second task_workflow call = %#v", second.Inputs[2])
	}
	if second.Inputs[3].Kind != provider.InputKindToolResult || second.Inputs[3].CallID != "call-1" || second.Inputs[3].ToolName != tool.TaskWorkflowToolName {
		t.Fatalf("first task_workflow result = %#v", second.Inputs[3])
	}
	if !strings.Contains(second.Inputs[3].Output, `"title":"First task"`) {
		t.Fatalf("first task_workflow result output = %q", second.Inputs[3].Output)
	}
	if second.Inputs[4].Kind != provider.InputKindToolResult || second.Inputs[4].CallID != "call-2" || second.Inputs[4].ToolName != tool.TaskWorkflowToolName {
		t.Fatalf("second task_workflow result = %#v", second.Inputs[4])
	}
	if !strings.Contains(second.Inputs[4].Output, `"title":"Second task"`) {
		t.Fatalf("second task_workflow result output = %q", second.Inputs[4].Output)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(state.TaskOrder) != 2 {
		t.Fatalf("task order = %#v, want 2 tasks", state.TaskOrder)
	}
	if state.Tasks[state.TaskOrder[0]].Title != "First task" {
		t.Fatalf("first task = %#v", state.Tasks[state.TaskOrder[0]])
	}
	if state.Tasks[state.TaskOrder[1]].Title != "Second task" {
		t.Fatalf("second task = %#v", state.Tasks[state.TaskOrder[1]])
	}
	if state.Turns["turn-1"].ToolCalls["call-2"] == nil || !state.Turns["turn-1"].ToolCalls["call-2"].Completed {
		t.Fatalf("call-2 = %#v", state.Turns["turn-1"].ToolCalls["call-2"])
	}
}

func TestTurnRunnerRunAllowsTaskWorkflowKickoffProgressAfterCreateBatch(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := sessions.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "setup-turn",
		TaskID:    "task-1",
		Title:     "First task",
	}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: tool.TaskWorkflowToolName, InputDelta: `{"action":"create","title":"Second task"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: tool.TaskWorkflowToolName},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: tool.TaskWorkflowToolName, InputDelta: `{"action":"create","title":"Third task"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: tool.TaskWorkflowToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-3", ToolName: tool.TaskWorkflowToolName, InputDelta: `{"action":"update","task_id":"task-1","status":"in_progress","progress":"starting the first implementation pass"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-3", ToolName: tool.TaskWorkflowToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "create tasks then start the first one",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want 3", len(client.requests))
	}
	second := client.requests[1]
	if len(second.Inputs) != 5 {
		t.Fatalf("second request inputs = %#v", second.Inputs)
	}
	third := client.requests[2]
	if len(third.Inputs) != 7 {
		t.Fatalf("third request inputs = %#v", third.Inputs)
	}
	if third.Inputs[5].Kind != provider.InputKindToolCall || third.Inputs[5].CallID != "call-3" || third.Inputs[5].ToolName != tool.TaskWorkflowToolName {
		t.Fatalf("update task_workflow call = %#v", third.Inputs[5])
	}
	if third.Inputs[6].Kind != provider.InputKindToolResult || third.Inputs[6].CallID != "call-3" || third.Inputs[6].ToolName != tool.TaskWorkflowToolName {
		t.Fatalf("update task_workflow result = %#v", third.Inputs[6])
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	call := state.Turns["turn-1"].ToolCalls["call-3"]
	if call == nil || !call.Completed || !call.Succeeded || call.Error != "" {
		t.Fatalf("update tool call = %#v", call)
	}
	task := state.Tasks["task-1"]
	if task == nil {
		t.Fatal("task-1 missing")
	}
	if task.Status != events.TaskStatusInProgress {
		t.Fatalf("task status = %q, want in_progress", task.Status)
	}
	if task.Progress != "starting the first implementation pass" {
		t.Fatalf("task progress = %q", task.Progress)
	}
}

func TestTurnRunnerRunExecutesMemorySaveAfterEOFCollectedRead(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	tools.SetMemoryService(NewMemoryService())

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\nconst value = 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first := &errorAfterEventsStream{
		events: []provider.Event{
			{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
			{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: tool.MemoryToolName, InputDelta: `{"action":"save","content":"note","id":null}`},
			{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: tool.MemoryToolName},
		},
	}
	client := &fakeProvider{
		streams: []provider.Stream{
			first,
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"}}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "inspect app.go then maybe save memory",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if !first.closed {
		t.Fatal("first provider stream was not closed before the speculative memory save")
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.ToolCalls["call-1"] == nil || !turn.ToolCalls["call-1"].Completed {
		t.Fatalf("call-1 = %#v", turn.ToolCalls["call-1"])
	}
	if turn.ToolCalls["call-2"] == nil || !turn.ToolCalls["call-2"].Completed || !turn.ToolCalls["call-2"].Succeeded {
		t.Fatalf("call-2 = %#v", turn.ToolCalls["call-2"])
	}
}

func TestTurnRunnerRunSkipsMutationTailAfterInterruptedProviderStream(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	tools.SetMemoryService(NewMemoryService())

	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\nconst value = 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first := &errorAfterEventsStream{
		events: []provider.Event{
			{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]}`},
			{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: tool.MemoryToolName, InputDelta: `{"action":"save","content":"note","id":null}`},
			{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: tool.MemoryToolName},
		},
		err: errors.New("stream error: stream ID 19; INTERNAL_ERROR; received from peer"),
	}
	client := &fakeProvider{
		streams: []provider.Stream{
			first,
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"}}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "inspect app.go then maybe save memory",
		Fragments:  baseFragments(),
		ModelRoute: baseModelRoute(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.ToolCalls["call-1"] == nil || !turn.ToolCalls["call-1"].Completed {
		t.Fatalf("call-1 = %#v", turn.ToolCalls["call-1"])
	}
	if turn.ToolCalls["call-2"] != nil {
		t.Fatalf("call-2 should not have executed after interrupted stream: %#v", turn.ToolCalls["call-2"])
	}
	if len(turn.ProviderAttempts) == 0 || !turn.ProviderAttempts[0].Retryable || turn.ProviderAttempts[0].RetrySkippedReason != providerRetrySkippedDurableProgress {
		t.Fatalf("provider attempts = %#v, want retryable stream interruption with durable progress", turn.ProviderAttempts)
	}
}
