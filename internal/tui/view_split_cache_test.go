package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
	if got := splitWideViewCacheKey(activeFrameChanged, state, layout); got != activeFrameBase {
		t.Fatalf("splitWideViewCacheKey() varied with active animation frame")
	}
}

func TestSplitWideLayoutHeightCalculationsMatchRenderedChrome(t *testing.T) {
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

	state := events.SessionState{
		SessionID:        "session-1",
		WorkspaceRoot:    "/repo",
		Title:            "Cache Review",
		PermissionMode:   "auto",
		TurnOrder:        []string{"turn-1"},
		Turns:            map[string]*events.TurnState{"turn-1": {TurnID: "turn-1", Status: events.TurnStatusRunning}},
		PendingQuestions: map[string]*events.QuestionRequestState{},
	}
	model.busy = true
	model.liveTurn.spinnerArmed = true

	if got, want := splitWideHeaderHeight(), lipgloss.Height(renderSplitWideHeader(model, state, model.width)); got != want {
		t.Fatalf("splitWideHeaderHeight() = %d, want rendered height %d", got, want)
	}
	if got, want := splitWideFooterHeight(model, state, model.width), lipgloss.Height(renderSplitWideFooter(model, state, model.width)); got != want {
		t.Fatalf("splitWideFooterHeight() = %d, want rendered height %d", got, want)
	}
}

func TestRenderModelSurfaceOverlaysSplitWideAnimationFrame(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	state := events.SessionState{
		SessionID:      "session-1",
		WorkspaceRoot:  "/repo",
		Title:          "Animation Review",
		PermissionMode: "auto",
		TurnOrder:      []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        events.TurnStatusRunning,
				StreamingText: "streaming",
			},
		},
	}
	model := NewModel(&fakeController{}, ModelConfig{
		Context:           ctx,
		Theme:             &defaultTheme,
		SessionID:         "session-1",
		TurnID:            "turn-1",
		WorkspaceRoot:     "/repo",
		InitialState:      &state,
		InitialStateOwned: true,
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
	model = modelIface.(Model)
	model.busy = true
	model.liveTurn.spinnerArmed = true
	model.messages.SetSize(80, 12)
	model.messages.Sync("alpha\nbeta\ngamma", false)
	model.inspector.body.SetSize(32, 12)
	model.inspector.body.Sync("detail\nlines", false)

	model.animation.frame = 0
	frame0, _ := renderModelSurface(model)
	if model.renderCache.rootSurface == nil || model.renderCache.rootSurface.buffer == nil {
		t.Fatal("rootSurfaceCache not populated after animated split-wide render")
	}
	cachedRoot := model.renderCache.rootSurface.buffer
	cachedSplitKey := model.renderCache.splitWideView.key

	model.animation.frame = 1
	frame1, _ := renderModelSurface(model)
	if frame0 == frame1 {
		t.Fatal("animated split-wide surface did not change between animation frames")
	}
	if model.renderCache.rootSurface.buffer != cachedRoot {
		t.Fatal("animation frame replaced cached root surface; want overlay over reused base")
	}
	if model.renderCache.splitWideView.key != cachedSplitKey {
		t.Fatal("animation frame changed split-wide view cache key")
	}
}

func TestRenderSplitWideViewFitsConfiguredSurface(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 28})
	model = modelIface.(Model)
	model.messages.SetSize(80, 12)
	model.messages.Sync("alpha\nbeta\ngamma", false)
	model.busy = true
	model.liveTurn.spinnerArmed = true

	state := events.SessionState{
		SessionID:      "session-1",
		WorkspaceRoot:  "/repo",
		Title:          "Fit Review",
		PermissionMode: "auto",
		TurnOrder:      []string{"turn-1"},
		Turns:          map[string]*events.TurnState{"turn-1": {TurnID: "turn-1", Status: events.TurnStatusRunning}},
	}
	rendered := renderSplitWideView(model, state, resolveShellLayout(model, state))
	lines := strings.Split(ansi.Strip(rendered), "\n")
	if len(lines) != model.height {
		t.Fatalf("rendered line count = %d, want %d", len(lines), model.height)
	}
	for i, line := range lines {
		if got := lipgloss.Width(line); got > model.width {
			t.Fatalf("line %d width = %d, want <= %d: %q", i, got, model.width, line)
		}
	}
}
