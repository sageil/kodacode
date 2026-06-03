package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestCompletedWorkflowReportRendersLastAndHidesResultToolRows(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-2",
		WorkspaceRoot: "/repo",
	})
	model.userText = "follow-up prompt"

	successful := true
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		Workflow: &events.WorkflowState{
			WorkflowID:     "review",
			Status:         events.WorkflowStatusCompleted,
			CurrentPhaseID: "review",
			EvidenceOrder:  []string{"evidence-phase", "evidence-review"},
			Evidence: map[string]*events.WorkflowEvidenceState{
				"evidence-phase": {
					EvidenceID: "evidence-phase",
					WorkflowID: "review",
					PhaseID:    "review",
					Type:       events.WorkflowEvidenceTypePhaseOutput,
					ToolCallID: "call-phase",
					Summary:    "workflow phase output recorded: decision",
					Fields: map[string]string{
						"decision": "ship the fix",
					},
				},
				"evidence-review": {
					EvidenceID: "evidence-review",
					WorkflowID: "review",
					PhaseID:    "review",
					Type:       events.WorkflowEvidenceTypeReviewOutcome,
					ToolCallID: "call-review",
					Successful: &successful,
					Summary:    "No regressions found.",
					Fields: map[string]string{
						"review_pass":   "correctness",
						"review_status": events.TaskReviewStatusPass,
						"source":        "workflow_review",
					},
				},
			},
			StopReason:     "delivered",
			CompletedAtSeq: 9,
		},
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:   "turn-1",
				Status:   events.TurnStatusCompleted,
				UserText: "review the current project",
				Config: &events.TurnConfigState{
					Model: "openai/gpt-5",
				},
				Review: &events.ReviewState{
					Title:              "Workflow review: correctness",
					OverallCorrectness: events.ReviewOverallCorrectnessCorrect,
					OverallSummary:     "No regressions found.",
				},
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryUser, Sequence: 1, Text: "review the current project"},
					{Kind: events.TranscriptEntryTool, Sequence: 2, CallID: "call-phase"},
					{Kind: events.TranscriptEntryTool, Sequence: 3, CallID: "call-test"},
					{Kind: events.TranscriptEntryTool, Sequence: 4, CallID: "call-review"},
					{Kind: events.TranscriptEntryReview, Sequence: 5},
				},
				ToolCallOrder: []string{"call-phase", "call-test", "call-review"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-phase": {
						CallID:    "call-phase",
						ToolName:  "workflow_phase_output",
						Input:     `{"fields":{"decision":"ship the fix"}}`,
						Output:    `{"recorded_keys":["decision"]}`,
						Completed: true,
						Succeeded: true,
					},
					"call-test": {
						CallID:    "call-test",
						ToolName:  "test",
						Input:     `{"command":"go test ./internal/tui"}`,
						Output:    "ok github.com/sageil/kodacode/internal/tui",
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
					"call-review": {
						CallID:    "call-review",
						ToolName:  "workflow_review_result",
						Input:     `{"review_pass":"correctness","findings":[],"overall_correctness":"correct","overall_summary":"No regressions found."}`,
						Output:    `{"review_id":"review-1","review_pass":"correctness","status":"pass"}`,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}

	rendered := ansi.Strip(renderTranscriptMessages(model, state, 120).content)
	if strings.Contains(rendered, "workflow_phase_output") || strings.Contains(rendered, "workflow_review_result") {
		t.Fatalf("workflow result tools should not render as raw transcript rows:\n%s", rendered)
	}
	if strings.Contains(rendered, "WORKFLOW REVIEW: correctness") {
		t.Fatalf("completed workflow review row should be represented by final workflow report instead:\n%s", rendered)
	}

	toolIndex := strings.Index(rendered, "go test ./internal/tui")
	draftIndex := strings.Index(rendered, "follow-up prompt")
	reportIndex := strings.LastIndex(rendered, "WORKFLOW REPORT")
	for label, index := range map[string]int{
		"tool row": toolIndex,
		"draft":    draftIndex,
		"report":   reportIndex,
	} {
		if index < 0 {
			t.Fatalf("rendered transcript missing %s:\n%s", label, rendered)
		}
	}
	if reportIndex < toolIndex || reportIndex < draftIndex {
		t.Fatalf("workflow report should be the last transcript section:\n%s", rendered)
	}
	for _, want := range []string{"Workflow review completed.", "decision:", "ship the fix", "correctness pass", "No regressions found."} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("workflow report missing %q:\n%s", want, rendered)
		}
	}
}

func TestCompletedWorkflowReportExpandsPhaseOutputFields(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	longConstraints := "TypeScript strict mode on both sides; shared validation schemas; session cookies; Socket.io live updates; pagination response shape; role-based permissions enforced server-side."
	finalSummary := "Workflow `explore` completed. Summary: " + longConstraints
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		Workflow: &events.WorkflowState{
			WorkflowID:        "explore",
			Status:            events.WorkflowStatusCompleted,
			CurrentPhaseID:    "summarize",
			CompletedPhaseIDs: []string{"inspect", "map", "summarize"},
			EvidenceOrder:     []string{"evidence-map", "evidence-summary"},
			Evidence: map[string]*events.WorkflowEvidenceState{
				"evidence-map": {
					EvidenceID: "evidence-map",
					WorkflowID: "explore",
					PhaseID:    "map",
					Type:       events.WorkflowEvidenceTypePhaseOutput,
					Summary:    "workflow phase output recorded: architecture_notes, constraints, inspected_files, next_steps",
					Fields: map[string]string{
						"constraints":        longConstraints,
						"architecture_notes": "Express API, Vue client, shared validation, and Socket.io.",
						"inspected_files":    `["server.ts","routes/auth.ts","client/src/main.ts","client/src/router/index.ts"]`,
						"next_steps":         "Verify missing views and confirm deployment pipeline.",
					},
				},
				"evidence-summary": {
					EvidenceID: "evidence-summary",
					WorkflowID: "explore",
					PhaseID:    "summarize",
					Type:       events.WorkflowEvidenceTypePhaseOutput,
					Summary:    finalSummary,
					Fields: map[string]string{
						"constraints": longConstraints,
					},
				},
			},
			StopReason:     "workflow completed",
			CompletedAtSeq: 9,
		},
	}

	rendered := ansi.Strip(renderTranscriptMessages(model, state, 92).content)
	for _, want := range []string{
		"constraints:",
		"role-based permissions enforced",
		"server-side.",
		"inspected_files:",
		"server.ts, routes/auth.ts, client/src/main.ts, client/src/router/index.ts",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("workflow report missing expanded field %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "role-based permissions enforced...") {
		t.Fatalf("workflow report still truncates phase output field:\n%s", rendered)
	}
	if strings.Contains(rendered, "phase summarize") {
		t.Fatalf("workflow report should hide final summary phase-output duplicate:\n%s", rendered)
	}
}

func TestShouldRenderToolCallInTranscriptHidesWorkflowResultTools(t *testing.T) {
	turn := &events.TurnState{TurnID: "turn-1"}
	for _, toolName := range []string{"workflow_phase_output", "workflow_review_result"} {
		call := &events.ToolCallState{
			CallID:    "call-1",
			ToolName:  toolName,
			Completed: true,
			Succeeded: true,
		}
		if shouldRenderToolCallInTranscript(turn, "call-1", call) {
			t.Fatalf("%s should not render as a raw transcript tool row", toolName)
		}
	}
}
