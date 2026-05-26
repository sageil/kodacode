package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

func TestRuntimeStartSessionReviewUsesReviewModelAndReviewPrompt(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{
				Kind:           provider.EventKindAssistantDelta,
				AssistantDelta: `{"findings":[{"severity":"P1","path":"internal/app/runtime_review.go","line":57,"title":"Review mode skips validation on parse fallback","explanation":"When the review output is not valid JSON, the runtime currently falls back silently and the dedicated review state is never recorded."}],"overall_correctness":"incorrect","overall_summary":"The review flow works, but invalid structured output currently degrades back to plain assistant text."}`,
			}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"},
	}
	runtime.Config.Workflow.ReviewModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"},
	}

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := runtime.StartSessionReview(context.Background(), StartSessionReviewInput{
		SessionID:    sessionID,
		TurnID:       "turn-1",
		Instructions: "focus on the auth and cache layers",
	})
	if err != nil {
		t.Fatalf("StartSessionReview() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	req := client.requests[0]
	if req.AgentID != reviewerAgentID {
		t.Fatalf("request agent = %q, want %q", req.AgentID, reviewerAgentID)
	}
	if got := req.Model.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("request model = %q, want openai/gpt-5-mini", got)
	}
	if !strings.Contains(req.Instructions, "Review mode is active.") {
		t.Fatalf("request instructions missing review rubric:\n%s", req.Instructions)
	}
	if !strings.Contains(req.Instructions, "Return exactly one JSON object and nothing else.") {
		t.Fatalf("request instructions missing structured output contract:\n%s", req.Instructions)
	}
	if !strings.Contains(req.Instructions, "Additional user review instructions: focus on the auth and cache layers") {
		t.Fatalf("request instructions missing user focus:\n%s", req.Instructions)
	}
	toolNames := make([]string, 0, len(req.Tools))
	for _, tool := range req.Tools {
		toolNames = append(toolNames, tool.Name)
	}
	if slices.Contains(toolNames, "web_search") {
		t.Fatalf("tool surface = %#v, want web_search excluded", toolNames)
	}
	if slices.Contains(toolNames, "web_fetch") {
		t.Fatalf("tool surface = %#v, want web_fetch excluded", toolNames)
	}
	if !slices.Contains(toolNames, "git_diff") || !slices.Contains(toolNames, "git_status") {
		t.Fatalf("tool surface = %#v, want git review tools preserved", toolNames)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.Review == nil {
		t.Fatalf("turn review = %#v, want structured review state", turn)
	}
	if turn.Config == nil || !turn.Config.HideAssistantPreview {
		t.Fatalf("turn config = %#v, want hide assistant preview enabled for manual review", turn.Config)
	}
	if turn.Config == nil || !turn.Config.PreserveSessionModel {
		t.Fatalf("turn config = %#v, want review model override to preserve session model", turn.Config)
	}
	if got := turn.Config.Model; got != "openai/gpt-5-mini" {
		t.Fatalf("turn config model = %q, want review model", got)
	}
	if len(turn.Review.Findings) != 1 || turn.Review.Findings[0].Severity != events.ReviewSeverityP1 {
		t.Fatalf("review findings = %#v", turn.Review.Findings)
	}
	if got := state.Model; got != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("session model = %q, want primary session model preserved", got)
	}
	if turn.Review.Title != "Focus on the auth and cache layers Review" {
		t.Fatalf("review title = %q, want custom review title", turn.Review.Title)
	}
	if turn.Review.OverallCorrectness != events.ReviewOverallCorrectnessIncorrect {
		t.Fatalf("overall correctness = %q", turn.Review.OverallCorrectness)
	}
	fragmentContent := manualReviewPromptFragments("focus on the auth and cache layers")[0].Content
	if !strings.Contains(fragmentContent, "Return exactly one JSON object and nothing else.") {
		t.Fatalf("review fragment missing output contract:\n%s", fragmentContent)
	}
	for _, unwanted := range []string{
		"Prefer no findings over speculative findings.",
		"Do not flag style, formatting, typos",
		"Prove impact from repository evidence.",
	} {
		if strings.Contains(fragmentContent, unwanted) {
			t.Fatalf("review fragment should not own tunable reviewer guidance %q:\n%s", unwanted, fragmentContent)
		}
	}
}

func TestRuntimeStartSessionReviewUsesAgentModelBeforeReviewModel(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, ".kodacode", "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "reviewer.md"), []byte(`---
description: project reviewer
model: openai/gpt-4.1
AllowTools:
  - read
---

You are the project reviewer.
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{
				Kind:           provider.EventKindAssistantDelta,
				AssistantDelta: `{"findings":[],"overall_correctness":"correct","overall_summary":"Looks good."}`,
			}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
	}
	runtime.Config.Workflow.ReviewModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"},
	}

	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := runtime.StartSessionReview(context.Background(), StartSessionReviewInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
	}); err != nil {
		t.Fatalf("StartSessionReview() error = %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	if got := client.requests[0].Model.String(); got != "openai/gpt-4.1" {
		t.Fatalf("request model = %q, want reviewer agent model", got)
	}
}

func TestRuntimeStartSessionReviewFallsBackToCurrentSessionModelWhenReviewerModelUnset(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{
				Kind:           provider.EventKindAssistantDelta,
				AssistantDelta: `{"findings":[],"overall_correctness":"correct","overall_summary":"Looks good."}`,
			}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
	}

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"},
	}

	if _, err := runtime.StartSessionReview(context.Background(), StartSessionReviewInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
	}); err != nil {
		t.Fatalf("StartSessionReview() error = %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	if got := client.requests[0].Model.String(); got != "openai/gpt-5" {
		t.Fatalf("request model = %q, want current session model", got)
	}
}

func TestRuntimeStartSessionReviewDefaultsTranscriptUserText(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{
				Kind:           provider.EventKindAssistantDelta,
				AssistantDelta: `{"findings":[],"overall_correctness":"correct","overall_summary":"Repository state looks good."}`,
			}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := runtime.StartSessionReview(context.Background(), StartSessionReviewInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
	})
	if err != nil {
		t.Fatalf("StartSessionReview() error = %v", err)
	}
	if result.UserText != manualReviewDefaultUserText {
		t.Fatalf("result user text = %q, want default review text", result.UserText)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn-1 missing from snapshot")
	}
	if turn.UserText != manualReviewDefaultUserText {
		t.Fatalf("turn user text = %q, want default review text", turn.UserText)
	}
	if turn.Config == nil || turn.Config.AgentID != reviewerAgentID {
		t.Fatalf("turn config = %#v, want reviewer agent", turn.Config)
	}
	if turn.Prompt == nil {
		t.Fatal("prompt state = nil, want compiled review prompt")
	}
	keys := make([]string, 0, len(turn.Prompt.Fragments))
	for _, fragment := range turn.Prompt.Fragments {
		keys = append(keys, fragment.Key)
	}
	if !slices.Contains(keys, manualReviewPromptFragmentKey) {
		t.Fatalf("prompt fragment keys = %#v, want %q", keys, manualReviewPromptFragmentKey)
	}
	if !slices.Contains(keys, manualReviewInstructionsKey) {
		t.Fatalf("prompt fragment keys = %#v, want %q", keys, manualReviewInstructionsKey)
	}
	if turn.Review == nil {
		t.Fatal("turn review = nil, want structured review state")
	}
	if turn.Review.Title != manualReviewDefaultTitle {
		t.Fatalf("turn review title = %q, want %q", turn.Review.Title, manualReviewDefaultTitle)
	}
	if len(turn.Review.Findings) != 0 || turn.Review.OverallSummary != "Repository state looks good." {
		t.Fatalf("turn review = %#v", turn.Review)
	}
}

func TestRuntimeStartSessionReviewLeavesPlainAssistantFallbackWhenJSONIsInvalid(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{
				Kind:           provider.EventKindAssistantDelta,
				AssistantDelta: "not valid json",
			}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if _, err := runtime.StartSessionReview(context.Background(), StartSessionReviewInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
	}); err != nil {
		t.Fatalf("StartSessionReview() error = %v", err)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn-1 missing from snapshot")
	}
	if turn.Review != nil {
		t.Fatalf("turn review = %#v, want nil for invalid json", turn.Review)
	}
	if turn.AssistantText != "not valid json" {
		t.Fatalf("assistant text = %q, want raw fallback text", turn.AssistantText)
	}
}

func TestParseStructuredManualReviewAcceptsSingleMarkdownFence(t *testing.T) {
	rawJSON := `{
  "findings": [],
  "overall_correctness": "correct",
  "overall_summary": "No concrete issues found."
}`

	cases := []struct {
		name string
		raw  string
	}{
		{
			name: "json language fence",
			raw:  "```json\n" + rawJSON + "\n```",
		},
		{
			name: "plain fence",
			raw:  "```\n" + rawJSON + "\n```",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := parseStructuredManualReview(tc.raw, manualReviewDefaultTitle)
			if err != nil {
				t.Fatalf("parseStructuredManualReview() error = %v", err)
			}
			if payload.Title != manualReviewDefaultTitle {
				t.Fatalf("title = %q, want %q", payload.Title, manualReviewDefaultTitle)
			}
			if payload.OverallCorrectness != events.ReviewOverallCorrectnessCorrect {
				t.Fatalf("overall correctness = %q", payload.OverallCorrectness)
			}
			if payload.OverallSummary != "No concrete issues found." {
				t.Fatalf("overall summary = %q", payload.OverallSummary)
			}
		})
	}
}

func TestParseStructuredManualReviewExtractsSingleObjectFromProse(t *testing.T) {
	payload, err := parseStructuredManualReview("Summary:\n"+`{"findings":[],"overall_correctness":"correct","overall_summary":"No concrete issues found."}`+"\nDone.", manualReviewDefaultTitle)
	if err != nil {
		t.Fatalf("parseStructuredManualReview() error = %v", err)
	}
	if payload.OverallCorrectness != events.ReviewOverallCorrectnessCorrect {
		t.Fatalf("overall correctness = %q", payload.OverallCorrectness)
	}
}

func TestParseStructuredManualReviewRejectsAmbiguousObjects(t *testing.T) {
	_, err := parseStructuredManualReview(
		`{"findings":[],"overall_correctness":"correct","overall_summary":"No concrete issues found."}`+"\n"+
			`{"findings":[],"overall_correctness":"correct","overall_summary":"No concrete issues found."}`,
		manualReviewDefaultTitle,
	)
	if err == nil || !strings.Contains(err.Error(), "multiple JSON objects") {
		t.Fatalf("parseStructuredManualReview() error = %v, want multiple object rejection", err)
	}
}

func TestRuntimeStartSessionReviewUsesProjectReviewerOverrideWhileKeepingContract(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, ".kodacode", "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "reviewer.md"), []byte(`---
description: project reviewer
AllowTools:
  - git_diff
  - git_show
  - git_status
  - task_workflow
  - read
  - search
---

You are the project reviewer.
Prioritize authorization regressions and workspace trust edge cases before anything else.
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{
				Kind:           provider.EventKindAssistantDelta,
				AssistantDelta: `{"findings":[],"overall_correctness":"correct","overall_summary":"No concrete issues found."}`,
			}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := runtime.StartSessionReview(context.Background(), StartSessionReviewInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
	}); err != nil {
		t.Fatalf("StartSessionReview() error = %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	req := client.requests[0]
	if !strings.Contains(req.Instructions, "Prioritize authorization regressions and workspace trust edge cases before anything else.") {
		t.Fatalf("request instructions missing project reviewer override:\n%s", req.Instructions)
	}
	if !strings.Contains(req.Instructions, "Return exactly one JSON object and nothing else.") {
		t.Fatalf("request instructions missing runtime review contract:\n%s", req.Instructions)
	}
	toolNames := make([]string, 0, len(req.Tools))
	for _, runtimeTool := range req.Tools {
		toolNames = append(toolNames, runtimeTool.Name)
	}
	if slices.Contains(toolNames, tool.TaskWorkflowToolName) {
		t.Fatalf("review tools = %#v, want task_workflow filtered from manual review mode", toolNames)
	}
}
