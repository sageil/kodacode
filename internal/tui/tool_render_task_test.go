package tui

import (
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
)

func TestTaskToolDisplayNameRecoversTitleFromMalformedInput(t *testing.T) {
	call := &events.ToolCallState{
		ToolName: "task_workflow",
		Input:    `{"action":"create","title":"Avoid double-query on create","notes": Add caching}`,
	}

	if got := taskToolDisplayName(call); got != "Task: Avoid double-query on create" {
		t.Fatalf("taskToolDisplayName() = %q", got)
	}
}

func TestTaskToolDisplayNameUsesListTasksLabel(t *testing.T) {
	call := &events.ToolCallState{
		ToolName: "task_workflow",
		Input:    `{"action":"list"}`,
		Output:   `{"tasks":[{"task_id":"task-99","title":"Backend optimization","status":"completed"}]}`,
	}

	if got := taskToolDisplayName(call); got != "List Tasks" {
		t.Fatalf("taskToolDisplayName() = %q", got)
	}
	if got := groupedToolItemResultDetail(call); got != "1 task" {
		t.Fatalf("groupedToolItemResultDetail() = %q", got)
	}
}

func TestGroupedToolItemResultDetailUsesTaskSpecificMalformedSummary(t *testing.T) {
	call := &events.ToolCallState{
		ToolName: "task_workflow",
		Input:    `{"action":"create","title":"Avoid double-query on create","notes": Add caching}`,
		Error:    "`task_workflow` failed. invalid character 'A' after object key:value pair. Example: create={\"action\":\"create\",\"title\":\"Add caching\"} | update={\"action\":\"update\",\"task_id\":\"task-1\",\"status\":\"in_progress\"} | complete={\"action\":\"complete\",\"task_id\":\"task-1\"}.",
	}

	if got := groupedToolItemResultDetail(call); got != "malformed JSON · quote string values" {
		t.Fatalf("groupedToolItemResultDetail() = %q", got)
	}
}

func TestTaskToolErrorDisplayTextUsesConcreteGuidance(t *testing.T) {
	call := &events.ToolCallState{
		ToolName: "task_workflow",
		Input:    `{"action":"create","title":"Avoid double-query on create","notes": Add caching}`,
		Error:    "`task_workflow` failed. invalid character 'A' after object key:value pair. Example: create={\"action\":\"create\",\"title\":\"Add caching\"} | update={\"action\":\"update\",\"task_id\":\"task-1\",\"status\":\"in_progress\"} | complete={\"action\":\"complete\",\"task_id\":\"task-1\"}.",
	}

	got := taskToolErrorDisplayText(call, call.Error)
	for _, want := range []string{
		"create call malformed JSON",
		"quote every property name and every string value",
		"detail: invalid character 'A' after object key:value pair",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("taskToolErrorDisplayText() = %q, missing %q", got, want)
		}
	}
}

func TestTaskToolErrorDisplayTextExplainsBatchPayloads(t *testing.T) {
	call := &events.ToolCallState{
		ToolName: "task_workflow",
		Input:    `[{"action":"create","title":"One"},{"action":"create","title":"Two"}]`,
		Error:    "`task_workflow` failed. value must be an object. Example: create={\"action\":\"create\",\"title\":\"Add caching\"} | update={\"action\":\"update\",\"task_id\":\"task-1\",\"status\":\"in_progress\"} | complete={\"action\":\"complete\",\"task_id\":\"task-1\"}.",
	}

	got := taskToolErrorDisplayText(call, call.Error)
	for _, want := range []string{
		"create call expected one JSON object",
		"send one task per call, not an array or prose",
		"detail: value must be an object",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("taskToolErrorDisplayText() = %q, missing %q", got, want)
		}
	}
	if got := groupedToolItemResultDetail(call); got != "expected one JSON object · one task per call" {
		t.Fatalf("groupedToolItemResultDetail() = %q", got)
	}
}

func TestRenderTaskToolDetailMarkdownUsesStructuredTaskError(t *testing.T) {
	call := &events.ToolCallState{
		ToolName: "task_workflow",
		Input:    `{"action":"create","title":"Avoid double-query on create","notes": Add caching}`,
		Error:    "`task_workflow` failed. invalid character 'A' after object key:value pair. Example: create={\"action\":\"create\",\"title\":\"Add caching\"} | update={\"action\":\"update\",\"task_id\":\"task-1\",\"status\":\"in_progress\"} | complete={\"action\":\"complete\",\"task_id\":\"task-1\"}.",
	}

	rendered := renderTaskToolDetailMarkdown(Model{}, sessionToolCallRef{}, call)
	for _, want := range []string{
		"## Error",
		"create call malformed JSON",
		"quote every property name and every string value",
		"detail: invalid character 'A' after object key:value pair",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("renderTaskToolDetailMarkdown() missing %q\nrendered:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{
		"Send exactly one JSON object and no surrounding prose",
		"If you include notes",
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("renderTaskToolDetailMarkdown() unexpectedly includes %q\nrendered:\n%s", unwanted, rendered)
		}
	}
}

func TestRenderTaskToolDetailMarkdownPrettyPrintsTaskListJSON(t *testing.T) {
	call := &events.ToolCallState{
		ToolName: "task_workflow",
		Input:    `{"action":"list"}`,
		Output:   `{"tasks":[{"task_id":"task-99","title":"Backend optimization","status":"completed"},{"task_id":"task-100","title":"Frontend optimization","status":"pending"}]}`,
	}

	rendered := renderTaskToolDetailMarkdown(Model{}, sessionToolCallRef{}, call)
	for _, want := range []string{
		"## Output",
		"**Tasks:**",
		`"task_id": "task-99"`,
		`"title": "Backend optimization"`,
		`"task_id": "task-100"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("renderTaskToolDetailMarkdown() missing %q\nrendered:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, `{"tasks":[{"task_id":"task-99"`) {
		t.Fatalf("renderTaskToolDetailMarkdown() still rendered one-line JSON\nrendered:\n%s", rendered)
	}
}
