package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestRenderInlinePermissionPromptCentersExecutionApprovalInTranscript(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "run tests",
	})
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeExecutionApprovalRequested, "session-1", "turn-1", events.ExecutionApprovalRequestedPayload{
		RequestID:          "exec-1",
		ExecutionID:        "execution-1",
		ToolCallID:         "call-1",
		ToolName:           "test",
		Command:            "go test ./...",
		WorkingDirectory:   "/repo",
		Reason:             "requires approval to run tests",
		AvailableDecisions: []events.ExecutionApprovalDecision{events.ExecutionApprovalDecisionAccept, events.ExecutionApprovalDecisionAcceptForSession, events.ExecutionApprovalDecisionDecline},
	}))

	rendered := ansi.Strip(renderInlinePermissionPrompt(model, model.projector.Snapshot(), 100))
	for _, needle := range []string{
		"Execution approval required",
		"requires approval to run tests",
		"go test ./...",
		"1. ● allow once",
		"2. ○ allow for session duration",
		"3. ○ deny",
	} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("inline permission prompt missing %q\n%s", needle, rendered)
		}
	}
}

func TestRenderInlinePermissionPromptSummarizesMultilineCommand(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "write the report",
	})
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	command := strings.Join([]string{
		"cat > /tmp/performance_review.md <<'EOF'",
		"# Performance Review",
		"- item one",
		"EOF",
	}, "\n")
	applyModelEvent(t, &model, draftEvent(1, events.TypePermissionRequested, "session-1", "turn-1", events.PermissionRequestedPayload{
		Kind:       events.PermissionRequestKindPath,
		RequestID:  "perm-1",
		ToolCallID: "call-1",
		Access:     "write",
		Path:       "/private/tmp/performance_review.md",
		ToolName:   "bash",
		Command:    command,
		Reason:     "command redirects output to a filesystem path",
	}))

	rendered := ansi.Strip(renderInlinePermissionPrompt(model, model.projector.Snapshot(), 100))
	if !strings.Contains(rendered, "command: cat > /tmp/performance_review.md <<'EOF' · heredoc, 3 more lines") {
		t.Fatalf("inline permission prompt missing summarized command\n%s", rendered)
	}
	for _, forbidden := range []string{"# Performance Review", "- item one"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("inline permission prompt leaked heredoc body %q\n%s", forbidden, rendered)
		}
	}
}

func TestRenderPendingPermissionInspectorShowsFullCommand(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "write the report",
	})
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	command := strings.Join([]string{
		"cat > /tmp/performance_review.md <<'EOF'",
		"# Performance Review",
		"- item one",
		"EOF",
	}, "\n")
	applyModelEvent(t, &model, draftEvent(1, events.TypePermissionRequested, "session-1", "turn-1", events.PermissionRequestedPayload{
		Kind:       events.PermissionRequestKindPath,
		RequestID:  "perm-1",
		ToolCallID: "call-1",
		Access:     "write",
		Path:       "/private/tmp/performance_review.md",
		ToolName:   "bash",
		Command:    command,
		Reason:     "command redirects output to a filesystem path",
	}))

	pending := model.pendingPermission()
	if pending == nil {
		t.Fatal("expected pending permission")
	}

	rendered := ansi.Strip(renderPendingPermissionInspector(model, model.projector.Snapshot(), *pending, 80))
	for _, needle := range []string{
		"Permission Required",
		"PATH",
		"/private/tmp/performance_review.md",
		"cat > /tmp/performance_review.md <<'EOF'",
		"# Performance Review",
		"Allow",
		"Session",
		"Deny",
	} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("permission inspector missing %q\n%s", needle, rendered)
		}
	}
}

func TestRenderInlinePermissionPromptHidesWhileSubmissionIsInFlight(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "run tests",
	})
	model.liveTurn.spinnerArmed = true
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypePermissionRequested, "session-1", "turn-1", events.PermissionRequestedPayload{
		Kind:       events.PermissionRequestKindPath,
		RequestID:  "perm-1",
		ToolCallID: "call-1",
		Access:     "read",
		Path:       "/private/tmp/report.txt",
		ToolName:   "read",
		Command:    `read {"path":"/private/tmp/report.txt"}`,
		Reason:     "requires access outside the workspace",
	}))

	model.interaction.resolveReq = "perm-1"

	if rendered := ansi.Strip(renderInlinePermissionPrompt(model, model.projector.Snapshot(), 100)); strings.TrimSpace(rendered) != "" {
		t.Fatalf("inline permission prompt should hide while submission is in flight\n%s", rendered)
	}
	if got := composerBlockedMessage(model); got != "waiting for the runtime to continue" {
		t.Fatalf("composerBlockedMessage() = %q", got)
	}
	active, label := model.liveTurnSpinnerState(model.projector.Snapshot())
	if !active || label != "Continuing" {
		t.Fatalf("liveTurnSpinnerState() = (%v, %q), want (true, %q)", active, label, "Continuing")
	}
}
