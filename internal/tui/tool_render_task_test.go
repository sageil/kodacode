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

func TestRenderTaskToolDetailMarkdownRendersTaskList(t *testing.T) {
	call := &events.ToolCallState{
		ToolName: "task_workflow",
		Input:    `{"action":"list"}`,
		Output:   `{"tasks":[{"task_id":"task-99","title":"Backend optimization","status":"completed"},{"task_id":"task-100","title":"Frontend optimization","status":"pending"}]}`,
	}

	rendered := renderTaskToolDetailMarkdown(Model{}, sessionToolCallRef{}, call)
	for _, want := range []string{
		"## Output",
		"**Tasks:**",
		"task-99 · Backend optimization · completed",
		"task-100 · Frontend optimization · pending",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("renderTaskToolDetailMarkdown() missing %q\nrendered:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{
		`{"tasks":[{"task_id":"task-99"`,
		`"task_id": "task-99"`,
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("renderTaskToolDetailMarkdown() still rendered JSON %q\nrendered:\n%s", unwanted, rendered)
		}
	}
}

func TestTaskToolTranscriptRendersTaskFieldsInsteadOfJSON(t *testing.T) {
	call := &events.ToolCallState{
		ToolName: "task_workflow",
		Input:    `{"action":"update","task_id":"task-57","status":"in_progress","notes":"Typecheck passed. Next: run unit tests and prepare PR notes."}`,
		Output:   `{"task":{"task_id":"task-57","title":"Implement initial index and query changes (text indexes, lean, projections)","status":"in_progress","notes":"Typecheck passed. Next: run unit tests and prepare PR notes."}}`,
	}

	input, ok := renderTaskToolInputTranscript(call, 120)
	if !ok {
		t.Fatal("renderTaskToolInputTranscript() returned false")
	}
	output, ok := renderTaskToolOutputTranscript(call.Output, 120)
	if !ok {
		t.Fatal("renderTaskToolOutputTranscript() returned false")
	}
	rendered := input + "\n\n" + output
	for _, want := range []string{
		"Action: update",
		"Task: Implement initial index and query changes (text indexes, lean, projections)",
		"Task ID: task-57",
		"Status: in_progress",
		"Notes: Typecheck passed. Next: run unit tests and prepare PR notes.",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("task transcript missing %q\nrendered:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{
		`{"action":"update"`,
		`{"task":{"task_id"`,
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("task transcript still rendered JSON %q\nrendered:\n%s", unwanted, rendered)
		}
	}
}
