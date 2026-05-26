package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestSplitWideViewCacheKeyDependsOnPaneAndFooterInputs(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
	model = modelIface.(Model)
	model.messages.SetSize(80, 12)
	model.messages.Sync("alpha\nbeta\ngamma", false)
	model.inspector.body.SetSize(32, 12)
	model.inspector.body.Sync("detail\nlines", false)

	state := events.SessionState{SessionID: "session-1", LastSequence: 1, Title: "Cache Review"}
	layout := resolveShellLayout(model, state)
	base := splitWideViewCacheKey(model, state, layout)
	if base == 0 {
		t.Fatal("splitWideViewCacheKey() returned zero key")
	}
	if got := splitWideViewCacheKey(model, state, layout); got != base {
		t.Fatalf("splitWideViewCacheKey() unstable for identical inputs")
	}

	nextState := state
	nextState.LastSequence = 2
	if got := splitWideViewCacheKey(model, nextState, layout); got != base {
		t.Fatalf("splitWideViewCacheKey() varied with state sequence only")
	}

	transcriptChanged := model
	transcriptChanged.messages.Sync("delta\nepsilon", false)
	if got := splitWideViewCacheKey(transcriptChanged, state, layout); got == base {
		t.Fatalf("splitWideViewCacheKey() did not vary with transcript pane content")
	}

	inspectorChanged := model
	inspectorChanged.inspector.body.Sync("other\ndetail", false)
	if got := splitWideViewCacheKey(inspectorChanged, state, layout); got == base {
		t.Fatalf("splitWideViewCacheKey() did not vary with inspector pane content")
	}

	composerChanged := model
	composerChanged.composer.SetValue("draft prompt")
	if got := splitWideViewCacheKey(composerChanged, state, layout); got == base {
		t.Fatalf("splitWideViewCacheKey() did not vary with composer value")
	}

	footerChanged := model
	footerChanged.footerNotice.err = "problem"
	if got := splitWideViewCacheKey(footerChanged, state, layout); got == base {
		t.Fatalf("splitWideViewCacheKey() did not vary with footer notice")
	}

	animChanged := model
	animChanged.animation.frame = 1
	if got := splitWideViewCacheKey(animChanged, state, layout); got != base {
		t.Fatalf("splitWideViewCacheKey() varied with inactive animation frame")
	}

	animActiveChanged := model
	animActiveChanged.busy = true
	if got := splitWideViewCacheKey(animActiveChanged, state, layout); got == base {
		t.Fatalf("splitWideViewCacheKey() did not vary with animation active state")
	}

	activeFrameChanged := animActiveChanged
	activeFrameBase := splitWideViewCacheKey(animActiveChanged, state, layout)
	activeFrameChanged.animation.frame = 1
	if got := splitWideViewCacheKey(activeFrameChanged, state, layout); got == activeFrameBase {
		t.Fatalf("splitWideViewCacheKey() did not vary with active animation frame")
	}
}
