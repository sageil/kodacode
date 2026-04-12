package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/service"
	"github.com/sageil/kodacode/v1/internal/tool"
)

func TestBuildBackgroundAutoReactPromptTruncatesOutputAndSuggestsTaskOutput(t *testing.T) {
	var output strings.Builder
	for i := 0; i < 120; i++ {
		fmt.Fprintf(&output, "line %03d %s\n", i, strings.Repeat("x", 32))
	}

	prompt := service.BuildBackgroundAutoReactPrompt("bg-1", "go test ./...", 0, output.String())

	if !strings.Contains(prompt, "call `task_output`") {
		t.Fatalf("prompt does not direct the model to task_output for more context:\n%s", prompt)
	}
	if !strings.Contains(prompt, "[...truncated; showing tail...]") {
		t.Fatalf("prompt does not mark truncated output:\n%s", prompt)
	}
	if strings.Contains(prompt, "line 000") {
		t.Fatalf("prompt included the oldest lines instead of a bounded tail:\n%s", prompt)
	}
	if !strings.Contains(prompt, "line 119") {
		t.Fatalf("prompt did not include the latest task output:\n%s", prompt)
	}
	if len(prompt) > service.BackgroundAutoReactOutputChars+400 {
		t.Fatalf("prompt exceeded expected size budget: %d chars", len(prompt))
	}
}

func TestBuildBackgroundAutoReactPromptHandlesFailuresWithoutOutput(t *testing.T) {
	prompt := service.BuildBackgroundAutoReactPrompt("bg-2", "npm test", 2, "")

	if !strings.Contains(prompt, "failed (exit 2)") {
		t.Fatalf("prompt missing failure status:\n%s", prompt)
	}
	if !strings.Contains(prompt, "No output was captured.") {
		t.Fatalf("prompt missing empty-output guidance:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Please review and take any necessary action.") {
		t.Fatalf("prompt missing action request:\n%s", prompt)
	}
}

func TestHandleSSEEvent_SubagentToolEndRefreshesTaskPanel(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.route = routeSession
	app.ready = true
	app.sessionID = "sess-1"
	app.width = 100
	app.height = 30

	store := tool.NewTaskStore(nil)
	app.taskStore = store
	if _, _, err := store.CreateTask(t.Context(), "sess-1", "First task", "pending", ""); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if app.session.taskPanel.HasTasks() {
		t.Fatal("task panel unexpectedly populated before refresh")
	}

	payload, err := json.Marshal(struct {
		Tool   string  `json:"tool"`
		Output string  `json:"output"`
		Error  *string `json:"error"`
		CallID string  `json:"call_id"`
	}{
		Tool:   "subagent",
		Output: "planner finished",
		CallID: "planner-1",
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	updated, _ := app.handleSSEEvent(SSEEventMsg{
		SessionID: "sess-1",
		Type:      "tool_end",
		Data:      payload,
	})
	app = updated.(App)

	if !app.session.taskPanel.HasTasks() {
		t.Fatal("task panel was not refreshed on subagent tool_end")
	}
	if got := len(app.session.taskPanel.tasks); got != 1 {
		t.Fatalf("task count = %d, want 1", got)
	}
	if got := app.session.taskPanel.tasks[0].Title; got != "First task" {
		t.Fatalf("task title = %q, want %q", got, "First task")
	}
}

func TestHandleSSEEvent_TaskSyncRefreshesTaskPanel(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.route = routeSession
	app.ready = true
	app.sessionID = "sess-1"
	app.width = 100
	app.height = 30

	store := tool.NewTaskStore(nil)
	app.taskStore = store
	if _, _, err := store.CreateTask(t.Context(), "sess-1", "First task", "pending", ""); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, activated, err := store.EnsureActiveTask(t.Context(), "sess-1"); err != nil || !activated {
		if err != nil {
			t.Fatalf("EnsureActiveTask() error = %v", err)
		}
		t.Fatal("EnsureActiveTask() did not activate the first task")
	}
	if app.session.taskPanel.HasTasks() {
		t.Fatal("task panel unexpectedly populated before refresh")
	}

	updated, _ := app.handleSSEEvent(SSEEventMsg{
		SessionID: "sess-1",
		Type:      "task_sync",
		Data:      []byte(`{"active_task_id":"task 1"}`),
	})
	app = updated.(App)

	if !app.session.taskPanel.HasTasks() {
		t.Fatal("task panel was not refreshed on task_sync")
	}
	if got := app.session.taskPanel.tasks[0].Status; got != "in_progress" {
		t.Fatalf("task status = %q, want %q", got, "in_progress")
	}
}
