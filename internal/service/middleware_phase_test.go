package service

import (
	"context"
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
)

func TestNewPhaseFilterMiddleware_ApprovedPlanAnswerSkipsFilter(t *testing.T) {
	planApproval := true
	req := &pipeline.TurnRequest{
		AgentID: "engineer",
		Tools: []provider.Tool{
			{Name: "bash"},
			{Name: "subagent"},
			{Name: "question"},
			{Name: "write"},
		},
		Messages: planReviewMessages("How would you like to proceed?\n> Proceed without saving"),
	}

	var called bool
	err := NewPhaseFilterMiddleware(&config.SessionConfig{PlanApproval: &planApproval})(
		context.Background(),
		req,
		func(_ context.Context, _ *pipeline.TurnRequest) error {
			called = true
			return nil
		},
	)
	if err != nil {
		t.Fatalf("middleware error = %v", err)
	}
	if !called {
		t.Fatal("next handler was not called")
	}
	if req.PhaseFilterActive {
		t.Fatal("PhaseFilterActive should be false when the plan was approved")
	}
	if !hasTool(req.Tools, "write") {
		t.Fatal("write tool should remain available after approval")
	}
}

func TestNewPhaseFilterMiddleware_RejectedPlanAnswerKeepsExecutionBlocked(t *testing.T) {
	planApproval := true
	req := &pipeline.TurnRequest{
		AgentID: "engineer",
		Tools: []provider.Tool{
			{Name: "bash"},
			{Name: "subagent"},
			{Name: "question"},
			{Name: "write"},
		},
		Messages: planReviewMessages("How would you like to proceed?\n> Reject plan"),
	}

	var called bool
	err := NewPhaseFilterMiddleware(&config.SessionConfig{PlanApproval: &planApproval})(
		context.Background(),
		req,
		func(_ context.Context, _ *pipeline.TurnRequest) error {
			called = true
			return nil
		},
	)
	if err != nil {
		t.Fatalf("middleware error = %v", err)
	}
	if !called {
		t.Fatal("next handler was not called")
	}
	if !req.PhaseFilterActive {
		t.Fatal("PhaseFilterActive should be true when the plan was rejected")
	}
	if hasTool(req.Tools, "write") {
		t.Fatal("write tool should remain blocked after rejection")
	}
	if hasTool(req.Tools, "question") {
		t.Fatal("question tool should be removed after rejection to prevent loop")
	}
}

func TestApplyPhaseRules_ApprovedPlanRestoresExecution(t *testing.T) {
	req := &pipeline.TurnRequest{
		AgentID: "engineer",
		Tools: []provider.Tool{
			{Name: "bash"},
			{Name: "subagent"},
			{Name: "question"},
		},
		PhaseFilterActive: true,
		FullTools: []provider.Tool{
			{Name: "bash"},
			{Name: "subagent"},
			{Name: "question"},
			{Name: "write"},
		},
		Messages: planReviewMessages("How would you like to proceed?\n> Save plan and proceed"),
	}

	ApplyPhaseRules(req)

	if req.PhaseFilterActive {
		t.Fatal("PhaseFilterActive should be false after approval")
	}
	if hasTool(req.Tools, "question") {
		t.Fatal("question tool should be removed after approval")
	}
	if !hasTool(req.Tools, "write") {
		t.Fatal("write tool should be restored after approval")
	}

	if got := req.SystemParts[2]; !strings.Contains(got, "Plan approved.") {
		t.Fatalf("expected approval directive, got %q", got)
	}
}

func TestApplyPhaseRules_RejectedPlanKeepsExecutionBlocked(t *testing.T) {
	req := &pipeline.TurnRequest{
		AgentID: "engineer",
		Tools: []provider.Tool{
			{Name: "bash"},
			{Name: "subagent"},
			{Name: "question"},
		},
		PhaseFilterActive: true,
		FullTools: []provider.Tool{
			{Name: "bash"},
			{Name: "subagent"},
			{Name: "question"},
			{Name: "write"},
		},
		Messages: planReviewMessages("The user cancelled the question without selecting an answer."),
	}

	ApplyPhaseRules(req)

	if req.PhaseFilterActive {
		t.Fatal("PhaseFilterActive should be false after rejection (terminal state)")
	}
	if hasTool(req.Tools, "write") {
		t.Fatal("write tool should remain blocked after rejection")
	}
	if hasTool(req.Tools, "question") {
		t.Fatal("question tool should be removed after rejection to prevent loop")
	}

	if got := req.SystemParts[2]; !strings.Contains(got, "The plan was declined.") {
		t.Fatalf("expected rejection directive, got %q", got)
	}
}

func TestApplyPhaseRules_PurposeRoleApproval(t *testing.T) {
	req := &pipeline.TurnRequest{
		AgentID: "engineer",
		Tools: []provider.Tool{
			{Name: "bash"},
			{Name: "question"},
		},
		PhaseFilterActive: true,
		FullTools: []provider.Tool{
			{Name: "bash"},
			{Name: "question"},
			{Name: "write"},
		},
		Messages: planReviewMessagesWithPurpose(
			`{"question":"Ready?","purpose":"plan_approval","options":[{"label":"Let's go","role":"approve"},{"label":"No thanks","role":"reject"}]}`,
			"Ready?\n> Let's go",
		),
	}

	ApplyPhaseRules(req)

	if req.PhaseFilterActive {
		t.Fatal("PhaseFilterActive should be false after purpose-based approval")
	}
	if !hasTool(req.Tools, "write") {
		t.Fatal("write tool should be restored after purpose-based approval")
	}
}

func TestApplyPhaseRules_ApprovedSavePlanDirective(t *testing.T) {
	req := &pipeline.TurnRequest{
		AgentID: "engineer",
		Tools: []provider.Tool{
			{Name: "bash"},
			{Name: "question"},
		},
		PhaseFilterActive: true,
		FullTools: []provider.Tool{
			{Name: "bash"},
			{Name: "question"},
			{Name: "write"},
		},
		Messages: []provider.Message{
			{
				Role: "assistant",
				Parts: []provider.MessagePart{
					provider.ToolCallPart{ID: "planner-call", Name: "subagent", Arguments: `{"agent_id":"planner"}`},
				},
			},
			{
				Role: "user",
				Parts: []provider.MessagePart{
					provider.TextPart{Text: encodePlanApprovalDecision(planApprovalApproved, planApprovalSaveOption)},
				},
			},
		},
	}

	ApplyPhaseRules(req)

	got := req.SystemParts[2]
	if !strings.Contains(got, planApprovalSaveOption) {
		t.Fatalf("expected save-plan directive, got %q", got)
	}
	if !strings.Contains(got, "docs/kodacode/plans/") {
		t.Fatalf("expected save path in directive, got %q", got)
	}
}

func TestApplyPhaseRules_PurposeRoleRejection(t *testing.T) {
	req := &pipeline.TurnRequest{
		AgentID: "engineer",
		Tools: []provider.Tool{
			{Name: "bash"},
			{Name: "question"},
		},
		PhaseFilterActive: true,
		FullTools: []provider.Tool{
			{Name: "bash"},
			{Name: "question"},
			{Name: "write"},
		},
		Messages: planReviewMessagesWithPurpose(
			`{"question":"Ready?","purpose":"plan_approval","options":[{"label":"Ship it","role":"approve"},{"label":"Cancel","role":"reject"}]}`,
			"Ready?\n> Cancel",
		),
	}

	ApplyPhaseRules(req)

	if req.PhaseFilterActive {
		t.Fatal("PhaseFilterActive should be false after rejection (terminal state)")
	}
	if hasTool(req.Tools, "write") {
		t.Fatal("write tool should remain blocked after purpose-based rejection")
	}
}

func TestIsPlanApprovalQuestion_Purpose(t *testing.T) {
	if !isPlanApprovalQuestion(`{"question":"Anything","purpose":"plan_approval","options":[]}`) {
		t.Fatal("should detect plan_approval purpose")
	}
	if isPlanApprovalQuestion(`{"question":"Anything","purpose":"other","options":[]}`) {
		t.Fatal("should not match non-plan_approval purpose")
	}
}

func TestLatestPlanApprovalDecision_SyntheticMarker(t *testing.T) {
	msgs := []provider.Message{
		{
			Role: "assistant",
			Parts: []provider.MessagePart{
				provider.ToolCallPart{ID: "planner-call", Name: "subagent", Arguments: `{"agent_id":"planner"}`},
			},
		},
		{
			Role: "user",
			Parts: []provider.MessagePart{
				provider.TextPart{Text: encodePlanApprovalDecision(planApprovalApproved, planApprovalProceedOption)},
			},
		},
	}

	got := latestPlanApprovalDecision(msgs)
	if got != planApprovalApproved {
		t.Fatalf("latestPlanApprovalDecision() = %d, want %d", got, planApprovalApproved)
	}
}

func TestLatestPlanApprovalDecision_NewPlannerResetsToPending(t *testing.T) {
	msgs := []provider.Message{
		{
			Role: "assistant",
			Parts: []provider.MessagePart{
				provider.ToolCallPart{ID: "planner-call-1", Name: "subagent", Arguments: `{"agent_id":"planner"}`},
			},
		},
		{
			Role: "user",
			Parts: []provider.MessagePart{
				provider.TextPart{Text: encodePlanApprovalDecision(planApprovalApproved, planApprovalProceedOption)},
			},
		},
		{
			Role: "assistant",
			Parts: []provider.MessagePart{
				provider.ToolCallPart{ID: "planner-call-2", Name: "subagent", Arguments: `{"agent_id":"planner"}`},
			},
		},
	}

	got := latestPlanApprovalDecision(msgs)
	if got != planApprovalPending {
		t.Fatalf("latestPlanApprovalDecision() = %d, want %d", got, planApprovalPending)
	}
}

func TestExtractAnswer(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{"standard format", "How would you like to proceed?\n> Save plan and proceed", "Save plan and proceed"},
		{"prefix only", "> Save plan", "Save plan"},
		{"raw answer", "Save plan and proceed", "Save plan and proceed"},
		{"empty", "", ""},
		{"multiline question", "Line1\nLine2\n> Yes", "Yes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractAnswer(tt.output)
			if got != tt.want {
				t.Errorf("extractAnswer(%q) = %q, want %q", tt.output, got, tt.want)
			}
		})
	}
}

func TestClassifyPlanApprovalAnswer_PlainStringOptions(t *testing.T) {
	// OpenAI models often send options as plain strings (no role field).
	// A selected option should be treated as approval, not rejection.
	opts := []planOption{
		{Label: "Save plan and proceed"},
		{Label: "Proceed without saving plan files"},
		{Label: "Cancel"},
	}

	tests := []struct {
		name   string
		output string
		want   planApprovalDecision
	}{
		{"selected proceed", "How would you like to proceed?\n> Proceed without saving plan files", planApprovalApproved},
		{"selected save", "How would you like to proceed?\n> Save plan and proceed", planApprovalApproved},
		{"selected cancel", "How would you like to proceed?\n> Cancel", planApprovalRejected},
		{"empty", "", planApprovalRejected},
		{"cancelled", "The user cancelled the question without selecting an answer.", planApprovalRejected},
		{"freeform approval", "yes let's do it", planApprovalApproved},
		{"freeform no", "no", planApprovalRejected},
		{"freeform no embedded", "Proceed without saving plan files", planApprovalApproved},
		{"legacy freeform no embedded", "Proceed without saving", planApprovalApproved},
		{"freeform with reject keyword", "I want to cancel this", planApprovalRejected},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyPlanApprovalAnswer(tt.output, opts)
			if got != tt.want {
				t.Errorf("classifyPlanApprovalAnswer(%q) = %d, want %d", tt.output, got, tt.want)
			}
		})
	}
}

func TestClassifyPlanApprovalAnswer_WithRoles(t *testing.T) {
	opts := []planOption{
		{Label: "Ship it", Role: "approve"},
		{Label: "Reject plan", Role: "reject"},
	}

	tests := []struct {
		name   string
		output string
		want   planApprovalDecision
	}{
		{"approve via role", "Ready?\n> Ship it", planApprovalApproved},
		{"reject via role", "Ready?\n> Reject plan", planApprovalRejected},
		{"unmatched freeform", "Ready?\n> Something else", planApprovalApproved},
		{"unmatched with reject keyword", "I want to cancel", planApprovalRejected},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyPlanApprovalAnswer(tt.output, opts)
			if got != tt.want {
				t.Errorf("classifyPlanApprovalAnswer(%q) = %d, want %d", tt.output, got, tt.want)
			}
		})
	}
}

func TestNewPhaseFilterMiddleware_EngineerPrebuildAndPreplanTools(t *testing.T) {
	planApproval := true
	allTools := []provider.Tool{
		{Name: "bash"}, {Name: "read"}, {Name: "read_files"},
		{Name: "glob"}, {Name: "grep"}, {Name: "search"}, {Name: "lsp"},
		{Name: "tree"}, {Name: "write"}, {Name: "edit"}, {Name: "test"},
		{Name: "subagent"}, {Name: "question"}, {Name: "task"}, {Name: "skill"},
		{Name: "memory"},
		{Name: "task_output"},
	}
	tests := []struct {
		name     string
		messages []provider.Message
		step     int
		want     []string
		blocked  []string
	}{
		{"prebuild phase", nil, 0,
			[]string{"test", "skill", "memory"},
			[]string{"bash", "read", "read_files", "glob", "grep", "search", "lsp", "tree", "subagent", "question", "task", "write", "edit", "task_output"},
		},
		{"prebuild phase remains locked without verification", nil, 99,
			[]string{"test", "skill", "memory"},
			[]string{"bash", "read", "read_files", "glob", "grep", "search", "lsp", "tree", "subagent", "question", "task", "write", "edit", "task_output"},
		},
		{"preplan phase (after test)", []provider.Message{
			{Role: "assistant", Parts: []provider.MessagePart{
				provider.ToolCallPart{Name: "test", ID: "t1"},
			}},
			{Role: "user", Parts: []provider.MessagePart{
				provider.ToolResultPart{ToolCallID: "t1", Output: "PASS", Metadata: map[string]any{"exit_code": 0}},
			}},
		}, 1,
			[]string{"read", "read_files", "glob", "grep", "search", "lsp", "tree", "bash", "test", "subagent", "question", "task", "write", "edit", "skill", "memory", "task_output"},
			nil,
		},
		{"postplan phase (after planner)", []provider.Message{
			{
				Role: "assistant",
				Parts: []provider.MessagePart{
					provider.ToolCallPart{ID: "planner-call", Name: "subagent", Arguments: `{"agent_id":"planner"}`},
				},
			},
			{
				Role: "user",
				Parts: []provider.MessagePart{
					provider.ToolResultPart{ToolCallID: "planner-call", Output: "Plan: 1. Audit the codebase. 2. Propose improvements."},
				},
			},
			{
				Role: "assistant",
				Parts: []provider.MessagePart{
					provider.ToolCallPart{ID: "plan-q", Name: "question", Arguments: `{"question":"Ready?","purpose":"plan_approval","options":[{"label":"Go","role":"approve"}]}`},
				},
			},
		}, 4,
			[]string{"read", "read_files", "glob", "grep", "search", "lsp", "tree", "bash", "test", "subagent", "question", "task", "memory"},
			[]string{"write", "edit"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &pipeline.TurnRequest{
				AgentID:  "engineer",
				Tools:    append([]provider.Tool{}, allTools...),
				Messages: tt.messages,
				Step:     tt.step,
			}
			var captured []provider.Tool
			err := NewPhaseFilterMiddleware(&config.SessionConfig{PlanApproval: &planApproval})(
				context.Background(), req,
				func(_ context.Context, r *pipeline.TurnRequest) error {
					captured = r.Tools
					return nil
				},
			)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			for _, name := range tt.want {
				if !hasTool(captured, name) {
					t.Errorf("tool %q missing in %s", name, tt.name)
				}
			}
			for _, name := range tt.blocked {
				if hasTool(captured, name) {
					t.Errorf("tool %q should NOT be available in %s", name, tt.name)
				}
			}
		})
	}
}

func TestNewPhaseFilterMiddleware_SkipsNonEngineerAgents(t *testing.T) {
	planApproval := true
	req := &pipeline.TurnRequest{
		AgentID: "adviser",
		Tools: []provider.Tool{
			{Name: "bash"},
			{Name: "subagent"},
			{Name: "question"},
			{Name: "write"},
		},
	}

	var captured []provider.Tool
	err := NewPhaseFilterMiddleware(&config.SessionConfig{PlanApproval: &planApproval})(
		context.Background(),
		req,
		func(_ context.Context, r *pipeline.TurnRequest) error {
			captured = r.Tools
			return nil
		},
	)
	if err != nil {
		t.Fatalf("middleware error = %v", err)
	}
	if req.PhaseFilterActive {
		t.Fatal("PhaseFilterActive should remain false for non-engineer agents")
	}
	if len(captured) != len(req.Tools) {
		t.Fatalf("captured tool count = %d, want %d", len(captured), len(req.Tools))
	}
	if !hasTool(captured, "write") {
		t.Fatal("write tool should remain available for non-engineer agents")
	}
}

func planReviewMessagesWithPurpose(args, answer string) []provider.Message {
	return []provider.Message{
		{
			Role: "assistant",
			Parts: []provider.MessagePart{
				provider.ToolCallPart{ID: "planner-call", Name: "subagent", Arguments: `{"agent_id":"planner"}`},
			},
		},
		{
			Role: "assistant",
			Parts: []provider.MessagePart{
				provider.ToolCallPart{ID: "plan-q", Name: "question", Arguments: args},
			},
		},
		{
			Role: "user",
			Parts: []provider.MessagePart{
				provider.ToolResultPart{ToolCallID: "plan-q", Output: answer},
			},
		},
	}
}

func planReviewMessages(answer string) []provider.Message {
	return planReviewMessagesWithPurpose(
		`{"question":"How would you like to proceed?","purpose":"plan_approval","options":[{"label":"Save plan and proceed","role":"approve"},{"label":"Proceed without saving plan files","role":"approve"},{"label":"Reject plan","role":"reject"}]}`,
		answer,
	)
}
