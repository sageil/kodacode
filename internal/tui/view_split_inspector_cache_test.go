package tui

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestSplitInspectorPaneCacheKeyDependsOnRenderInputs(t *testing.T) {
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
	model.inspector.body.SetSize(24, 2)
	model.inspector.body.Sync("alpha\nbeta\ngamma", false)
	model.inspector.body.GotoTop()

	base := splitInspectorPaneCacheKey(model, 72, 18)
	if base == 0 {
		t.Fatal("splitInspectorPaneCacheKey() returned zero key")
	}
	if got := splitInspectorPaneCacheKey(model, 72, 18); got != base {
		t.Fatalf("splitInspectorPaneCacheKey() unstable for identical inputs")
	}

	if got := splitInspectorPaneCacheKey(model, 80, 18); got == base {
		t.Fatalf("splitInspectorPaneCacheKey() did not vary with width")
	}
	if got := splitInspectorPaneCacheKey(model, 72, 20); got == base {
		t.Fatalf("splitInspectorPaneCacheKey() did not vary with height")
	}

	focusChanged := model
	focusChanged.chrome.focus = focusInspector
	if got := splitInspectorPaneCacheKey(focusChanged, 72, 18); got == base {
		t.Fatalf("splitInspectorPaneCacheKey() did not vary with focus")
	}

	tabChanged := model
	tabChanged.inspector.tab = inspectorTabTools
	if got := splitInspectorPaneCacheKey(tabChanged, 72, 18); got == base {
		t.Fatalf("splitInspectorPaneCacheKey() did not vary with active tab")
	}

	agentChanged := model
	agentChanged.agentID = "engineer"
	if got := splitInspectorPaneCacheKey(agentChanged, 72, 18); got == base {
		t.Fatalf("splitInspectorPaneCacheKey() did not vary with tab availability inputs")
	}

	rawChanged := model
	rawChanged.inspector.body.Sync("delta\nepsilon\nzeta", false)
	if got := splitInspectorPaneCacheKey(rawChanged, 72, 18); got == base {
		t.Fatalf("splitInspectorPaneCacheKey() did not vary with viewport raw content")
	}

	offsetChanged := model
	offsetChanged.inspector.body.GotoLine(1)
	if got := splitInspectorPaneCacheKey(offsetChanged, 72, 18); got == base {
		t.Fatalf("splitInspectorPaneCacheKey() did not vary with viewport offset")
	}

	wrapChanged := model
	wrapChanged.inspector.body.SetSoftWrap(false)
	if got := splitInspectorPaneCacheKey(wrapChanged, 72, 18); got == base {
		t.Fatalf("splitInspectorPaneCacheKey() did not vary with soft-wrap mode")
	}
}
