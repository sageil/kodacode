package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/provider"
)

func TestRuntimeRunSessionTurnUsesPlannerToolSurface(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "plan"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)

	_, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "analyze the repo",
		AgentID:       "planner",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}

	got := make([]string, 0, len(client.requests[0].Tools))
	for _, tool := range client.requests[0].Tools {
		got = append(got, tool.Name)
	}
	want := []string{"definition", "diagnostics", "git_diff", "git_show", "git_status", "locate", "question", "read", "refs", "search", "symbols", "trace"}
	if len(got) != len(want) {
		t.Fatalf("planner tool count = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("planner tools = %#v, want %#v", got, want)
		}
	}
}

func TestRuntimeRunSessionTurnReturnsUnknownAgentError(t *testing.T) {
	runtime := newRuntimeWithClient(t, &fakeProvider{})

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "analyze the repo",
		AgentID:       "missing",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusFailed {
		t.Fatalf("result status = %q", result.Status)
	}
	if result.Error != "The selected agent could not be found." {
		t.Fatalf("result error = %q", result.Error)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns[result.TurnID]
	if turn == nil || turn.Status != events.TurnStatusFailed || turn.Error == "" {
		t.Fatalf("turn = %#v", turn)
	}
}

func TestRuntimeRunSessionTurnLoadsProjectMarkdownAgent(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, ".kodacode", "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "reviewer.md"), []byte(`---
description: project reviewer
model: openai/gpt-5-mini
AllowTools:
  - read
  - search
---

You are the reviewer agent.
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "review"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)

	_, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		UserText:      "review the repo",
		AgentID:       "reviewer",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	if got := client.requests[0].Model.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("request model = %q", got)
	}

	got := make([]string, 0, len(client.requests[0].Tools))
	for _, tool := range client.requests[0].Tools {
		got = append(got, tool.Name)
	}
	want := []string{"read", "search"}
	if len(got) != len(want) {
		t.Fatalf("reviewer tools = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("reviewer tools = %#v, want %#v", got, want)
		}
	}
}

func TestRuntimeListAgentsIncludesProjectModelRoute(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, ".kodacode", "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "reviewer.md"), []byte(`---
description: project reviewer
model: openai/gpt-5-mini
AllowTools:
  - read
---

You are the reviewer agent.
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runtime := newRuntimeWithClient(t, &fakeProvider{})
	agents, err := runtime.ListAgents(context.Background(), root)
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}
	for _, available := range agents {
		if available.ID != "reviewer" {
			continue
		}
		if got := available.ModelRoute.Primary.String(); got != "openai/gpt-5-mini" {
			t.Fatalf("reviewer model route = %q, want openai/gpt-5-mini", got)
		}
		return
	}
	t.Fatalf("reviewer agent missing from %#v", agents)
}

func TestRuntimeListAgentsExcludesSubagentOnlyAndHiddenAgents(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, ".kodacode", "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "hidden-helper.md"), []byte(`---
description: hidden helper
mode: primary
hidden: true
---

You are hidden.
`), 0o644); err != nil {
		t.Fatalf("WriteFile(hidden-helper) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "docs-helper.md"), []byte(`---
description: docs helper
mode: subagent
---

You are a subagent only.
`), 0o644); err != nil {
		t.Fatalf("WriteFile(docs-helper) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "shared-reviewer.md"), []byte(`---
description: shared reviewer
mode: all
---

You are selectable and delegatable.
`), 0o644); err != nil {
		t.Fatalf("WriteFile(shared-reviewer) error = %v", err)
	}

	runtime := newRuntimeWithClient(t, &fakeProvider{})
	agents, err := runtime.ListAgents(context.Background(), root)
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}
	if containsAvailableAgent(agents, "hidden-helper") {
		t.Fatalf("agents = %#v, want hidden helper excluded", agents)
	}
	if containsAvailableAgent(agents, "docs-helper") {
		t.Fatalf("agents = %#v, want subagent-only helper excluded", agents)
	}
	if !containsAvailableAgent(agents, "shared-reviewer") {
		t.Fatalf("agents = %#v, want shared-reviewer included", agents)
	}
}

func TestRuntimeRunSessionTurnLoadsPromptSourcesAndPersistsCompiledPrompt(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	globalDir := filepath.Join(configHome, "kodacode")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(global) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, promptInstructionsFilename), []byte("Global instruction text."), 0o644); err != nil {
		t.Fatalf("WriteFile(global) error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, promptInstructionsFilename), []byte("Project instruction text."), 0o644); err != nil {
		t.Fatalf("WriteFile(project) error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
		})},
	}
	runtime := newRuntimeWithClientConfigHome(t, client, configHome)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		UserText:      "build it",
		AgentID:       "builder",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	instructions := client.requests[0].Instructions
	if !strings.Contains(instructions, "Global instruction text.") {
		t.Fatalf("instructions missing global source: %q", instructions)
	}
	if !strings.Contains(instructions, "Project instruction text.") {
		t.Fatalf("instructions missing project source: %q", instructions)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns[result.TurnID]
	if turn == nil || turn.Prompt == nil {
		t.Fatal("prompt artifact missing")
	}
	if got := turn.Prompt.Shape; got != prompt.ViewShapeGeneric {
		t.Fatalf("prompt shape = %q", got)
	}
	if len(turn.Prompt.Fragments) != 7 {
		t.Fatalf("prompt fragment count = %d, want 7", len(turn.Prompt.Fragments))
	}
	gotLabels := make([]string, 0, len(turn.Prompt.Fragments))
	for _, fragment := range turn.Prompt.Fragments {
		gotLabels = append(gotLabels, fragment.Label)
	}
	wantLabels := []string{"core-policy", "builder", "global-instructions", "project-instructions", "response-style", "workspace", "execution-environment"}
	for i := range wantLabels {
		if gotLabels[i] != wantLabels[i] {
			t.Fatalf("prompt labels = %#v, want %#v", gotLabels, wantLabels)
		}
	}

	eventsList, err := runtime.Store.Replay(context.Background(), events.Query{
		SessionID:     result.SessionID,
		AfterSequence: -1,
	})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	found := false
	for _, event := range eventsList {
		if event.Type == events.TypePromptCompiled {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("prompt_compiled event missing")
	}
}

func TestRuntimeRunSessionTurnKeepsBuilderWithoutTaskMutationTools(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)

	_, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "work on the repo",
		AgentID:       "builder",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	gotTools := make([]string, 0, len(client.requests[0].Tools))
	for _, tool := range client.requests[0].Tools {
		gotTools = append(gotTools, tool.Name)
	}
	if containsString(gotTools, "task_workflow") {
		t.Fatalf("builder tools = %#v, want task_workflow excluded", gotTools)
	}
	if containsString(gotTools, "task_review") {
		t.Fatalf("builder tools = %#v, want task_review excluded", gotTools)
	}
}

func TestRuntimeRunSessionTurnSupportsBuiltinEngineerWithoutChangingDefaultBuilder(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "implement the change",
		AgentID:       "engineer",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	if got := client.requests[0].AgentID; got != "engineer" {
		t.Fatalf("request agent_id = %q, want engineer", got)
	}
	if !strings.Contains(client.requests[0].Instructions, "Do not claim you cannot access files, modify code, run commands, or manage tasks when the relevant tools are available in this turn.") {
		t.Fatalf("instructions missing false-unavailability guard: %q", client.requests[0].Instructions)
	}
	gotTools := make([]string, 0, len(client.requests[0].Tools))
	for _, tool := range client.requests[0].Tools {
		gotTools = append(gotTools, tool.Name)
	}
	if !containsString(gotTools, "task_workflow") {
		t.Fatalf("engineer tools = %#v, want task_workflow included", gotTools)
	}
	if containsString(gotTools, "task_review") {
		t.Fatalf("engineer tools = %#v, want task_review excluded", gotTools)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns[result.TurnID]
	if turn == nil || turn.Prompt == nil {
		t.Fatal("prompt artifact missing")
	}
	if len(turn.Prompt.Fragments) < 2 {
		t.Fatalf("prompt fragments = %#v", turn.Prompt.Fragments)
	}
	if got := turn.Prompt.Fragments[1].Label; got != "engineer" {
		t.Fatalf("agent prompt label = %q, want engineer", got)
	}
}

func TestRuntimeRunSessionTurnRemovesTaskWorkflowForReviewPlanHarnessParent(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)

	_, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "Perform a complete performance review and create a detailed plan of execution",
		AgentID:       "engineer",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	gotTools := make([]string, 0, len(client.requests[0].Tools))
	for _, tool := range client.requests[0].Tools {
		gotTools = append(gotTools, tool.Name)
	}
	if containsString(gotTools, "task_workflow") {
		t.Fatalf("review-plan harness parent tools = %#v, want task_workflow excluded", gotTools)
	}
	if containsString(gotTools, "delegate") {
		t.Fatalf("review-plan harness parent tools = %#v, want delegate removed", gotTools)
	}
}

func TestRuntimeRunSessionTurnSupportsBuiltinReviewerToolSurface(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "review"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)

	_, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "review the task",
		AgentID:       "reviewer",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	gotTools := make([]string, 0, len(client.requests[0].Tools))
	for _, tool := range client.requests[0].Tools {
		gotTools = append(gotTools, tool.Name)
	}
	if !containsString(gotTools, "task_review") {
		t.Fatalf("reviewer tools = %#v, want task_review included", gotTools)
	}
	if !containsString(gotTools, "question") {
		t.Fatalf("reviewer tools = %#v, want question included", gotTools)
	}
	if containsString(gotTools, "task_workflow") {
		t.Fatalf("reviewer tools = %#v, want task_workflow excluded", gotTools)
	}
	if containsString(gotTools, "write") {
		t.Fatalf("reviewer tools = %#v, want write excluded", gotTools)
	}
	if containsString(gotTools, "apply_patch") {
		t.Fatalf("reviewer tools = %#v, want apply_patch excluded", gotTools)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsAvailableAgent(values []AvailableAgent, want string) bool {
	for _, value := range values {
		if value.ID == want {
			return true
		}
	}
	return false
}
