package tui

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestTranscriptPaneCacheKeyDependsOnViewportAndSelectionInputs(t *testing.T) {
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
	model.messages.SetSize(24, 2)
	model.messages.Sync("alpha\nbeta\ngamma", false)
	model.messages.GotoTop()

	state := events.SessionState{SessionID: "session-1", LastSequence: 1}
	base := transcriptPaneCacheKey(model, state, 72)
	if base == 0 {
		t.Fatal("transcriptPaneCacheKey() returned zero key")
	}
	if got := transcriptPaneCacheKey(model, state, 72); got != base {
		t.Fatalf("transcriptPaneCacheKey() unstable for identical inputs")
	}

	if got := transcriptPaneCacheKey(model, state, 64); got == base {
		t.Fatalf("transcriptPaneCacheKey() did not vary with width")
	}

	nextState := state
	nextState.LastSequence = 2
	if got := transcriptPaneCacheKey(model, nextState, 72); got != base {
		t.Fatalf("transcriptPaneCacheKey() varied with state sequence only")
	}

	pendingChanged := state
	pendingChanged.PendingPermissionOrder = []string{"permission-1"}
	pendingChanged.PendingPermissions = map[string]*events.PermissionRequestState{
		"permission-1": {
			RequestID:        "permission-1",
			TurnID:           "turn-1",
			ToolName:         "bash",
			Kind:             events.PermissionRequestKindExecution,
			Access:           "write",
			WorkingDirectory: "/repo",
			Command:          "npm install",
			Reason:           "Needs approval",
		},
	}
	if got := transcriptPaneCacheKey(model, pendingChanged, 72); got == base {
		t.Fatalf("transcriptPaneCacheKey() did not vary with pending prompt")
	}

	rawChanged := model
	rawChanged.messages.Sync("delta\nepsilon\nzeta", false)
	if got := transcriptPaneCacheKey(rawChanged, state, 72); got == base {
		t.Fatalf("transcriptPaneCacheKey() did not vary with viewport raw content")
	}

	offsetChanged := model
	offsetChanged.messages.GotoLine(1)
	if got := transcriptPaneCacheKey(offsetChanged, state, 72); got == base {
		t.Fatalf("transcriptPaneCacheKey() did not vary with viewport offset")
	}

	wrapChanged := model
	wrapChanged.messages.SetSoftWrap(true)
	if got := transcriptPaneCacheKey(wrapChanged, state, 72); got == base {
		t.Fatalf("transcriptPaneCacheKey() did not vary with soft-wrap mode")
	}

	cursorChanged := model
	cursorChanged.transcriptView.cursorInitialized = true
	cursorChanged.transcriptView.cursorLine = 1
	cursorChanged.transcriptView.cursorColumn = 2
	cursorChanged.transcriptView.cursorGoalColumn = 2
	if got := transcriptPaneCacheKey(cursorChanged, state, 72); got == base {
		t.Fatalf("transcriptPaneCacheKey() did not vary with transcript cursor")
	}

	visualChanged := model
	visualChanged.transcriptView.cursorInitialized = true
	visualChanged.transcriptView.visualActive = true
	visualChanged.transcriptView.visualAnchorLine = 1
	visualChanged.transcriptView.visualAnchorColumn = 3
	if got := transcriptPaneCacheKey(visualChanged, state, 72); got == base {
		t.Fatalf("transcriptPaneCacheKey() did not vary with visual selection")
	}
}

func TestSplitTranscriptPaneCacheKeyDependsOnLayoutInputs(t *testing.T) {
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
	model.messages.SetSize(80, 12)
	model.messages.Sync("alpha\nbeta\ngamma", false)

	state := events.SessionState{SessionID: "session-1", LastSequence: 1}
	base := splitTranscriptPaneCacheKey(model, state, 72, 18, false)
	if base == 0 {
		t.Fatal("splitTranscriptPaneCacheKey() returned zero key")
	}
	if got := splitTranscriptPaneCacheKey(model, state, 72, 18, false); got != base {
		t.Fatalf("splitTranscriptPaneCacheKey() unstable for identical inputs")
	}
	if got := splitTranscriptPaneCacheKey(model, state, 80, 18, false); got == base {
		t.Fatalf("splitTranscriptPaneCacheKey() did not vary with width")
	}
	if got := splitTranscriptPaneCacheKey(model, state, 72, 20, false); got == base {
		t.Fatalf("splitTranscriptPaneCacheKey() did not vary with height")
	}
	if got := splitTranscriptPaneCacheKey(model, state, 72, 18, true); got == base {
		t.Fatalf("splitTranscriptPaneCacheKey() did not vary with borderless mode")
	}
}
