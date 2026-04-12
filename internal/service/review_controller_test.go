package service

import (
	"context"
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/tool"
)

func TestMaybeHandleCompletedTaskReview_ReopensFailedTask(t *testing.T) {
	ctx := context.Background()
	store := tool.NewTaskStore(nil)
	task, _, err := store.CreateTask(ctx, "review-test", "Harden tracing", "completed", "Acceptance criteria: 1. Trace ID is present.")
	if err != nil {
		t.Fatal(err)
	}

	var events []SSEEvent
	req := &pipeline.TurnRequest{
		SessionID: "review-test",
		AgentID:   "engineer",
		Tools:     []provider.Tool{{Name: "subagent"}},
	}
	tl := &turnLoop{
		ctx:       ctx,
		req:       req,
		taskStore: store,
		publish: func(_ string, ev SSEEvent) {
			events = append(events, ev)
		},
		msgs: &turnLoopMessageRepo{},
		spawnSubagent: func(_ context.Context, _ string, agentID, taskPrompt string, _ ProgressFunc) (string, error) {
			if agentID != "reviewer" {
				t.Fatalf("agentID = %q, want reviewer", agentID)
			}
			if !strings.Contains(taskPrompt, task.Title) {
				t.Fatalf("reviewer task prompt missing task title: %q", taskPrompt)
			}
			return "[FAIL] Trace ID is never attached\nOverall: FAIL", nil
		},
	}

	reviewedTasks := map[string]bool{}
	reviewSinceIdx := 0
	reviewTaskID := ""
	handled, disableTools, err := tl.maybeHandleCompletedTaskReview(nil, reviewedTasks, &reviewSinceIdx, &reviewTaskID, false, false)
	if err != nil {
		t.Fatalf("maybeHandleCompletedTaskReview() error = %v", err)
	}
	if !handled {
		t.Fatal("expected completed task review to be handled")
	}
	if disableTools {
		t.Fatal("disableTools = true, want false")
	}

	got := store.GetTasks("review-test")
	if len(got) != 1 || got[0].Status != "in_progress" {
		t.Fatalf("task status after failed review = %#v, want in_progress", got)
	}
	if got[0].ReviewStatus != tool.TaskReviewFail {
		t.Fatalf("task review status after failed review = %q, want %q", got[0].ReviewStatus, tool.TaskReviewFail)
	}
	if !strings.Contains(got[0].LastReviewSummary, "Trace ID is never attached") {
		t.Fatalf("task review summary after failed review = %q, want reviewer finding", got[0].LastReviewSummary)
	}
	if reviewedTasks[task.ID] {
		t.Fatalf("task %s should not be marked reviewed on FAIL", task.ID)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("len(req.Messages) = %d, want 2", len(req.Messages))
	}
	if len(req.SystemParts) < 3 || !strings.Contains(req.SystemParts[2], "did not pass acceptance-criteria review") {
		t.Fatalf("missing failure workflow directive: %#v", req.SystemParts)
	}
	foundSync := false
	for _, ev := range events {
		if ev.Type != "task_sync" {
			continue
		}
		foundSync = true
		sync, ok := ev.Data.(SSETaskSyncData)
		if !ok {
			t.Fatalf("task_sync data type = %T", ev.Data)
		}
		if sync.ActiveTaskID != task.ID {
			t.Fatalf("task_sync ActiveTaskID = %q, want %q", sync.ActiveTaskID, task.ID)
		}
	}
	if !foundSync {
		t.Fatal("expected task_sync event after failed review")
	}
}

func TestMaybeHandleCompletedTaskReview_KeepsPassedTaskCompleted(t *testing.T) {
	ctx := context.Background()
	store := tool.NewTaskStore(nil)
	task, _, err := store.CreateTask(ctx, "review-pass-test", "Tighten validation", "completed", "Acceptance criteria: 1. Validation rejects invalid payloads.")
	if err != nil {
		t.Fatal(err)
	}

	req := &pipeline.TurnRequest{
		SessionID: "review-pass-test",
		AgentID:   "engineer",
		Tools:     []provider.Tool{{Name: "subagent"}},
	}
	tl := &turnLoop{
		ctx:       ctx,
		req:       req,
		taskStore: store,
		publish:   func(string, SSEEvent) {},
		msgs:      &turnLoopMessageRepo{},
		spawnSubagent: func(_ context.Context, _ string, agentID, _ string, _ ProgressFunc) (string, error) {
			if agentID != "reviewer" {
				t.Fatalf("agentID = %q, want reviewer", agentID)
			}
			return "[PASS] Validation rejects invalid payloads\nOverall: PASS", nil
		},
	}

	reviewedTasks := map[string]bool{}
	reviewSinceIdx := 0
	reviewTaskID := ""
	handled, disableTools, err := tl.maybeHandleCompletedTaskReview(nil, reviewedTasks, &reviewSinceIdx, &reviewTaskID, false, false)
	if err != nil {
		t.Fatalf("maybeHandleCompletedTaskReview() error = %v", err)
	}
	if !handled {
		t.Fatal("expected completed task review to be handled")
	}
	if disableTools {
		t.Fatal("disableTools = true, want false")
	}

	got := store.GetTasks("review-pass-test")
	if len(got) != 1 || got[0].Status != "completed" {
		t.Fatalf("task status after passing review = %#v, want completed", got)
	}
	if got[0].ReviewStatus != tool.TaskReviewPass {
		t.Fatalf("task review status after PASS = %q, want %q", got[0].ReviewStatus, tool.TaskReviewPass)
	}
	if !strings.Contains(got[0].LastReviewSummary, "Acceptance review passed") {
		t.Fatalf("task review summary after PASS = %q, want pass summary", got[0].LastReviewSummary)
	}
	if !reviewedTasks[task.ID] {
		t.Fatalf("task %s should be marked reviewed on PASS", task.ID)
	}
	if len(req.SystemParts) >= 3 && strings.Contains(req.SystemParts[2], "did not pass acceptance-criteria review") {
		t.Fatalf("unexpected failure directive after PASS: %#v", req.SystemParts)
	}
}

func TestMaybeHandleCompletedTaskReview_ReviewsAnalysisTasks(t *testing.T) {
	ctx := context.Background()
	store := tool.NewTaskStore(nil)
	task, _, err := store.CreateTaskWithKind(ctx, "analysis-review-test", "Audit middleware", tool.TaskKindAnalysis, "completed", "Review middleware for gaps.")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateTask(ctx, "analysis-review-test", task.ID, "completed", "audit complete, findings documented", "", false); err != nil {
		t.Fatal(err)
	}

	req := &pipeline.TurnRequest{
		SessionID: "analysis-review-test",
		AgentID:   "engineer",
		Tools:     []provider.Tool{{Name: "subagent"}},
	}
	tl := &turnLoop{
		ctx:       ctx,
		req:       req,
		taskStore: store,
		publish:   func(string, SSEEvent) {},
		msgs:      &turnLoopMessageRepo{},
		spawnSubagent: func(_ context.Context, _ string, agentID, taskPrompt string, _ ProgressFunc) (string, error) {
			if agentID != "reviewer" {
				t.Fatalf("agentID = %q, want reviewer", agentID)
			}
			if !strings.Contains(taskPrompt, "Review mode: analysis verification") {
				t.Fatalf("missing analysis review mode in prompt: %q", taskPrompt)
			}
			if !strings.Contains(taskPrompt, "audit complete, findings documented") {
				t.Fatalf("missing completion summary in prompt: %q", taskPrompt)
			}
			return "[PASS] Findings summary is concrete and grounded\nOverall: PASS", nil
		},
	}

	reviewedTasks := map[string]bool{}
	reviewSinceIdx := 0
	reviewTaskID := ""
	handled, disableTools, err := tl.maybeHandleCompletedTaskReview(nil, reviewedTasks, &reviewSinceIdx, &reviewTaskID, false, false)
	if err != nil {
		t.Fatalf("maybeHandleCompletedTaskReview() error = %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true for analysis task")
	}
	if disableTools {
		t.Fatal("disableTools = true, want false")
	}
	got := store.GetTasks("analysis-review-test")
	if len(got) != 1 || got[0].Status != "completed" {
		t.Fatalf("task status after analysis review = %#v, want completed", got)
	}
	if !reviewedTasks[task.ID] {
		t.Fatalf("task %s should be marked reviewed after PASS", task.ID)
	}
}

func TestMaybeHandleCompletedTaskReview_ReviewCapAsksUserAndReopensTask(t *testing.T) {
	ctx := context.Background()
	store := tool.NewTaskStore(nil)
	task, _, err := store.CreateTask(ctx, "review-cap-continue-test", "Fix cache invalidation", "completed", "Acceptance criteria: 1. Cache keys are deterministic.")
	if err != nil {
		t.Fatal(err)
	}

	req := &pipeline.TurnRequest{
		SessionID: "review-cap-continue-test",
		AgentID:   "engineer",
		Tools:     []provider.Tool{{Name: "subagent"}},
		Messages: []provider.Message{
			{
				Role:  "user",
				Parts: []provider.MessagePart{provider.TextPart{Text: "start review window"}},
			},
			{
				Role: "assistant",
				Parts: []provider.MessagePart{
					provider.ToolCallPart{ID: "review-1", Name: "subagent", Arguments: `{"agent_id":"reviewer","task":"review once"}`},
					provider.ToolCallPart{ID: "review-2", Name: "subagent", Arguments: `{"agent_id":"reviewer","task":"review twice"}`},
					provider.ToolCallPart{ID: "review-3", Name: "subagent", Arguments: `{"agent_id":"reviewer","task":"review thrice"}`},
				},
			},
		},
	}

	tl := &turnLoop{
		ctx:       ctx,
		req:       req,
		taskStore: store,
		publish:   func(string, SSEEvent) {},
		msgs:      &turnLoopMessageRepo{},
		spawnSubagent: func(_ context.Context, _ string, _ string, _ string, _ ProgressFunc) (string, error) {
			t.Fatal("spawnSubagent should not be called after reviewer cap is reached")
			return "", nil
		},
		askUser: func(_ context.Context, _ string) func(string, []string, bool, string) (string, error) {
			return func(question string, options []string, multiple bool, purpose string) (string, error) {
				if question != reviewCapQuestionText {
					t.Fatalf("question = %q, want %q", question, reviewCapQuestionText)
				}
				if purpose != reviewCapPurpose {
					t.Fatalf("purpose = %q, want %q", purpose, reviewCapPurpose)
				}
				if multiple {
					t.Fatal("multiple = true, want false")
				}
				if len(options) != 3 {
					t.Fatalf("len(options) = %d, want 3", len(options))
				}
				return reviewCapContinueFixOption, nil
			}
		},
	}

	reviewedTasks := map[string]bool{}
	reviewSinceIdx := 1
	reviewTaskID := task.ID
	handled, disableTools, err := tl.maybeHandleCompletedTaskReview(nil, reviewedTasks, &reviewSinceIdx, &reviewTaskID, false, false)
	if err != nil {
		t.Fatalf("maybeHandleCompletedTaskReview() error = %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true")
	}
	if disableTools {
		t.Fatal("disableTools = true, want false")
	}

	got := store.GetTasks("review-cap-continue-test")
	if len(got) != 1 || got[0].Status != "in_progress" {
		t.Fatalf("task status after review cap continue = %#v, want in_progress", got)
	}
	if got[0].BlockReason != "" {
		t.Fatalf("task block reason after continue = %q, want empty", got[0].BlockReason)
	}
	if !reviewedTasks[task.ID] {
		t.Fatalf("task %s should be marked reviewed after user override", task.ID)
	}
	if len(req.SystemParts) < 3 || !strings.Contains(req.SystemParts[2], "without further automatic reviewer retries") {
		t.Fatalf("missing continue-fixing directive: %#v", req.SystemParts)
	}
}

func TestMaybeHandleCompletedTaskReview_ReviewCapResetsForNextTask(t *testing.T) {
	ctx := context.Background()
	store := tool.NewTaskStore(nil)
	first, _, err := store.CreateTask(ctx, "review-cap-reset-test", "Fix cache invalidation", "completed", "Acceptance criteria: 1. Cache keys are deterministic.")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := store.CreateTask(ctx, "review-cap-reset-test", "Add regression tests", "completed", "Acceptance criteria: 1. Regression tests cover the cache fix.")
	if err != nil {
		t.Fatal(err)
	}

	req := &pipeline.TurnRequest{
		SessionID: "review-cap-reset-test",
		AgentID:   "engineer",
		Tools:     []provider.Tool{{Name: "subagent"}},
		Messages: []provider.Message{
			{
				Role:  "user",
				Parts: []provider.MessagePart{provider.TextPart{Text: "start review window"}},
			},
			{
				Role: "assistant",
				Parts: []provider.MessagePart{
					provider.ToolCallPart{ID: "review-1", Name: "subagent", Arguments: `{"agent_id":"reviewer","task":"review once"}`},
					provider.ToolCallPart{ID: "review-2", Name: "subagent", Arguments: `{"agent_id":"reviewer","task":"review twice"}`},
					provider.ToolCallPart{ID: "review-3", Name: "subagent", Arguments: `{"agent_id":"reviewer","task":"review thrice"}`},
				},
			},
		},
	}

	tl := &turnLoop{
		ctx:       ctx,
		req:       req,
		taskStore: store,
		publish:   func(string, SSEEvent) {},
		msgs:      &turnLoopMessageRepo{},
		spawnSubagent: func(_ context.Context, _ string, agentID, taskPrompt string, _ ProgressFunc) (string, error) {
			if agentID != "reviewer" {
				t.Fatalf("agentID = %q, want reviewer", agentID)
			}
			if !strings.Contains(taskPrompt, second.Title) {
				t.Fatalf("reviewer task prompt missing second task title: %q", taskPrompt)
			}
			return "Overall: PASS", nil
		},
		askUser: func(_ context.Context, _ string) func(string, []string, bool, string) (string, error) {
			return func(question string, options []string, multiple bool, purpose string) (string, error) {
				t.Fatalf("askUser should not be called for the next task after cap reset, got question=%q purpose=%q options=%v multiple=%v", question, purpose, options, multiple)
				return "", nil
			}
		},
	}

	reviewedTasks := map[string]bool{first.ID: true}
	reviewSinceIdx := 1
	reviewTaskID := first.ID
	handled, disableTools, err := tl.maybeHandleCompletedTaskReview(nil, reviewedTasks, &reviewSinceIdx, &reviewTaskID, false, false)
	if err != nil {
		t.Fatalf("maybeHandleCompletedTaskReview() error = %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true")
	}
	if disableTools {
		t.Fatal("disableTools = true, want false")
	}
	if !reviewedTasks[second.ID] {
		t.Fatalf("task %s should be marked reviewed after PASS", second.ID)
	}
	if reviewTaskID != second.ID {
		t.Fatalf("reviewTaskID = %q, want %q", reviewTaskID, second.ID)
	}
}

func TestMaybeHandleCompletedTaskReview_ReviewCapCanStopExecution(t *testing.T) {
	ctx := context.Background()
	store := tool.NewTaskStore(nil)
	task, _, err := store.CreateTask(ctx, "review-cap-stop-test", "Fix cache invalidation", "completed", "Acceptance criteria: 1. Cache keys are deterministic.")
	if err != nil {
		t.Fatal(err)
	}

	var syncEvents []SSEEvent
	req := &pipeline.TurnRequest{
		SessionID: "review-cap-stop-test",
		AgentID:   "engineer",
		Tools:     []provider.Tool{{Name: "subagent"}},
		Messages: []provider.Message{
			{
				Role:  "user",
				Parts: []provider.MessagePart{provider.TextPart{Text: "start review window"}},
			},
			{
				Role: "assistant",
				Parts: []provider.MessagePart{
					provider.ToolCallPart{ID: "review-1", Name: "subagent", Arguments: `{"agent_id":"reviewer","task":"review once"}`},
					provider.ToolCallPart{ID: "review-2", Name: "subagent", Arguments: `{"agent_id":"reviewer","task":"review twice"}`},
					provider.ToolCallPart{ID: "review-3", Name: "subagent", Arguments: `{"agent_id":"reviewer","task":"review thrice"}`},
				},
			},
		},
	}

	tl := &turnLoop{
		ctx:       ctx,
		req:       req,
		taskStore: store,
		publish: func(_ string, ev SSEEvent) {
			syncEvents = append(syncEvents, ev)
		},
		msgs: &turnLoopMessageRepo{},
		spawnSubagent: func(_ context.Context, _ string, _ string, _ string, _ ProgressFunc) (string, error) {
			t.Fatal("spawnSubagent should not be called after reviewer cap is reached")
			return "", nil
		},
		askUser: func(_ context.Context, _ string) func(string, []string, bool, string) (string, error) {
			return func(_ string, _ []string, _ bool, _ string) (string, error) {
				return reviewCapStopOption, nil
			}
		},
	}

	reviewedTasks := map[string]bool{}
	reviewSinceIdx := 1
	reviewTaskID := task.ID
	handled, disableTools, err := tl.maybeHandleCompletedTaskReview(nil, reviewedTasks, &reviewSinceIdx, &reviewTaskID, false, false)
	if err != nil {
		t.Fatalf("maybeHandleCompletedTaskReview() error = %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true")
	}
	if !disableTools {
		t.Fatal("disableTools = false, want true")
	}

	got := store.GetTasks("review-cap-stop-test")
	if len(got) != 1 || got[0].Status != "blocked" {
		t.Fatalf("task status after review cap stop = %#v, want blocked", got)
	}
	if got[0].BlockReason != tool.TaskBlockReasonReviewCap {
		t.Fatalf("task block reason after review cap stop = %q, want %q", got[0].BlockReason, tool.TaskBlockReasonReviewCap)
	}
	if !reviewedTasks[task.ID] {
		t.Fatalf("task %s should be marked reviewed after stop decision", task.ID)
	}
	if len(req.SystemParts) < 3 || !strings.Contains(req.SystemParts[2], "Do not call more tools") {
		t.Fatalf("missing stop directive: %#v", req.SystemParts)
	}
	foundSync := false
	for _, ev := range syncEvents {
		if ev.Type != "task_sync" {
			continue
		}
		foundSync = true
	}
	if !foundSync {
		t.Fatal("expected task_sync event after stop decision")
	}
}

func TestFilterWorkflowCompletionCalls_BlocksMultipleCompletions(t *testing.T) {
	tl := &turnLoop{
		req: &pipeline.TurnRequest{
			AgentID: "engineer",
		},
	}

	calls := []provider.ToolCall{
		{ID: "task-complete-1", Name: "task", Arguments: `{"action":"update","id":"task 1","status":"completed"}`},
		{ID: "task-complete-2", Name: "task", Arguments: `{"action":"update","id":"task 2","status":"completed"}`},
		{ID: "read-1", Name: "read", Arguments: `{"filePath":"/tmp/file.go"}`},
	}

	filtered, blocked := tl.filterWorkflowCompletionCalls(calls)
	if !blocked {
		t.Fatal("blocked = false, want true")
	}
	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}
	if filtered[0].ID != "task-complete-1" {
		t.Fatalf("first kept call = %q, want %q", filtered[0].ID, "task-complete-1")
	}
	if filtered[1].ID != "read-1" {
		t.Fatalf("second kept call = %q, want %q", filtered[1].ID, "read-1")
	}
}
