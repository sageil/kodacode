package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestInputFilterDropsTranscriptWheelAtTop(t *testing.T) {
	model := newInputFilterTestModel(t)
	model.messages.GotoTop()
	x, y := pointInMouseRect(model.mouseRegions.transcript)

	got := inputFilter(model, tea.MouseWheelMsg(tea.Mouse{
		X:      x,
		Y:      y,
		Button: tea.MouseWheelUp,
	}))
	if got != nil {
		t.Fatal("wheel up at transcript top was not dropped")
	}
}

func TestInputFilterDropsTranscriptWheelAtBottom(t *testing.T) {
	model := newInputFilterTestModel(t)
	model.messages.GotoBottom()
	x, y := pointInMouseRect(model.mouseRegions.transcript)

	got := inputFilter(model, tea.MouseWheelMsg(tea.Mouse{
		X:      x,
		Y:      y,
		Button: tea.MouseWheelDown,
	}))
	if got != nil {
		t.Fatal("wheel down at transcript bottom was not dropped")
	}
}

func TestInputFilterKeepsTranscriptWheelAwayFromBoundary(t *testing.T) {
	model := newInputFilterTestModel(t)
	model.messages.GotoTop()
	x, y := pointInMouseRect(model.mouseRegions.transcript)
	msg := tea.MouseWheelMsg(tea.Mouse{
		X:      x,
		Y:      y,
		Button: tea.MouseWheelDown,
	})

	got := inputFilter(model, msg)
	if got == nil {
		t.Fatal("wheel away from transcript boundary was dropped")
	}
}

func TestInputFilterKeepsWheelWhenCursorIsOutsideScrollPanes(t *testing.T) {
	model := newInputFilterTestModel(t)
	msg := tea.MouseWheelMsg(tea.Mouse{
		X:      1,
		Y:      model.mouseRegions.transcript.y + model.mouseRegions.transcript.height,
		Button: tea.MouseWheelDown,
	})

	got := inputFilter(model, msg)
	if got == nil {
		t.Fatal("wheel outside scroll panes was dropped")
	}
}

func TestInputFilterDropsDialogWheelAtTop(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	body := NewMessagesWithTone(&defaultTheme, "panel-alt")
	body.SetSize(40, 5)
	body.Sync(strings.Repeat("line\n", 40), false)
	body.GotoTop()

	model := Model{
		dialog: &costDialog{body: body},
	}
	got := inputFilter(model, tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp}))
	if got != nil {
		t.Fatal("wheel up at dialog top was not dropped")
	}
}

func TestInputFilterDropsMouseMotionWhenTranscriptSelectionIsInactive(t *testing.T) {
	got := inputFilter(Model{}, tea.MouseMotionMsg(tea.Mouse{X: 1, Y: 1, Button: tea.MouseRight}))
	if got != nil {
		t.Fatal("mouse motion was not dropped")
	}
}

func TestInputFilterKeepsMouseMotionWhileTranscriptSelectionIsActive(t *testing.T) {
	model := Model{transcriptView: transcriptViewState{mouseSelecting: true}}
	msg := tea.MouseMotionMsg(tea.Mouse{X: 1, Y: 1, Button: tea.MouseLeft})
	got := inputFilter(model, msg)
	if got == nil {
		t.Fatal("mouse motion during transcript selection was dropped")
	}
}

func TestInputFilterDropsHorizontalWheel(t *testing.T) {
	model := newInputFilterTestModel(t)
	x, y := pointInMouseRect(model.mouseRegions.transcript)

	for _, button := range []tea.MouseButton{tea.MouseWheelLeft, tea.MouseWheelRight} {
		got := inputFilter(model, tea.MouseWheelMsg(tea.Mouse{
			X:      x,
			Y:      y,
			Button: button,
		}))
		if got != nil {
			t.Fatalf("%s was not dropped", button)
		}
	}
}

func TestWheelInputLimiterDropsSameDirectionBurst(t *testing.T) {
	model := newInputFilterTestModel(t)
	model.messages.GotoTop()
	x, y := pointInMouseRect(model.mouseRegions.transcript)
	msg := tea.MouseWheelMsg(tea.Mouse{
		X:      x,
		Y:      y,
		Button: tea.MouseWheelDown,
	})
	limiter := &wheelInputLimiter{}
	now := time.Unix(1, 0)

	if drop, reason := limiter.shouldDrop(model, msg, now); drop || reason != "keep" {
		t.Fatalf("first wheel drop=%v reason=%q, want keep", drop, reason)
	}
	if drop, reason := limiter.shouldDrop(model, msg, now.Add(wheelInputCoalesceWindow/2)); !drop || reason != "drop:coalesce" {
		t.Fatalf("burst wheel drop=%v reason=%q, want coalesced drop", drop, reason)
	}
	if drop, reason := limiter.shouldDrop(model, msg, now.Add(wheelInputCoalesceWindow)); drop || reason != "keep" {
		t.Fatalf("later wheel drop=%v reason=%q, want keep", drop, reason)
	}
}

func TestWheelInputLimiterCoalescesBurstIntoAcceleratedWheel(t *testing.T) {
	model := newInputFilterTestModel(t)
	model.messages.GotoTop()
	x, y := pointInMouseRect(model.mouseRegions.transcript)
	msg := tea.MouseWheelMsg(tea.Mouse{
		X:      x,
		Y:      y,
		Button: tea.MouseWheelDown,
	})
	limiter := &wheelInputLimiter{}
	now := time.Unix(1, 0)

	if filtered, reason := limiter.filter(model, msg, now); filtered == nil || reason != "keep" {
		t.Fatalf("first wheel filtered=%#v reason=%q, want keep", filtered, reason)
	}
	if filtered, reason := limiter.filter(model, msg, now.Add(wheelInputCoalesceWindow/2)); filtered != nil || reason != "drop:coalesce" {
		t.Fatalf("burst wheel filtered=%#v reason=%q, want coalesced drop", filtered, reason)
	}
	filtered, reason := limiter.filter(model, msg, now.Add(wheelInputCoalesceWindow))
	coalesced, ok := filtered.(coalescedWheelMsg)
	if !ok {
		t.Fatalf("later wheel filtered=%T reason=%q, want coalescedWheelMsg", filtered, reason)
	}
	if coalesced.steps != 2 {
		t.Fatalf("coalesced steps = %d, want 2", coalesced.steps)
	}
}

func TestWheelInputLimiterKeepsOppositeDirectionDuringBurst(t *testing.T) {
	model := newInputFilterTestModel(t)
	model.messages.GotoLine(10)
	x, y := pointInMouseRect(model.mouseRegions.transcript)
	down := tea.MouseWheelMsg(tea.Mouse{
		X:      x,
		Y:      y,
		Button: tea.MouseWheelDown,
	})
	up := tea.MouseWheelMsg(tea.Mouse{
		X:      x,
		Y:      y,
		Button: tea.MouseWheelUp,
	})
	limiter := &wheelInputLimiter{}
	now := time.Unix(1, 0)

	if drop, reason := limiter.shouldDrop(model, down, now); drop || reason != "keep" {
		t.Fatalf("first wheel drop=%v reason=%q, want keep", drop, reason)
	}
	if drop, reason := limiter.shouldDrop(model, up, now.Add(time.Millisecond)); drop || reason != "keep" {
		t.Fatalf("opposite wheel drop=%v reason=%q, want keep", drop, reason)
	}
}

func TestWheelInputLimiterDropsTranscriptMomentumAfterClick(t *testing.T) {
	model := newInputFilterTestModel(t)
	model.messages.GotoTop()
	x, y := pointInMouseRect(model.mouseRegions.transcript)
	wheel := tea.MouseWheelMsg(tea.Mouse{
		X:      x,
		Y:      y,
		Button: tea.MouseWheelDown,
	})
	click := tea.MouseClickMsg(tea.Mouse{
		X:      x,
		Y:      y,
		Button: tea.MouseLeft,
	})
	limiter := &wheelInputLimiter{}
	now := time.Unix(1, 0)

	limiter.stopWheelAfterClick(model, click, now)
	if drop, reason := limiter.shouldDrop(model, wheel, now.Add(wheelInputStopAfterClickWindow/2)); !drop || reason != "drop:click-stop" {
		t.Fatalf("post-click wheel drop=%v reason=%q, want click-stop drop", drop, reason)
	}
	if drop, reason := limiter.shouldDrop(model, wheel, now.Add(wheelInputStopAfterClickWindow)); drop || reason != "keep" {
		t.Fatalf("later post-click wheel drop=%v reason=%q, want keep", drop, reason)
	}
}

func TestWheelInputLimiterKeepsDifferentPaneWheelAfterClick(t *testing.T) {
	model := newInputFilterTestModel(t)
	model.messages.GotoTop()
	model.inspector.body.Sync(strings.Repeat("inspector\n", 80), false)
	model.inspector.body.GotoTop()
	transcriptX, transcriptY := pointInMouseRect(model.mouseRegions.transcript)
	inspectorX, inspectorY := pointInMouseRect(model.mouseRegions.inspector)
	click := tea.MouseClickMsg(tea.Mouse{
		X:      transcriptX,
		Y:      transcriptY,
		Button: tea.MouseLeft,
	})
	wheel := tea.MouseWheelMsg(tea.Mouse{
		X:      inspectorX,
		Y:      inspectorY,
		Button: tea.MouseWheelDown,
	})
	limiter := &wheelInputLimiter{}
	now := time.Unix(1, 0)

	limiter.stopWheelAfterClick(model, click, now)
	if drop, reason := limiter.shouldDrop(model, wheel, now.Add(wheelInputStopAfterClickWindow/2)); drop || reason != "keep" {
		t.Fatalf("different pane wheel drop=%v reason=%q, want keep", drop, reason)
	}
}

func TestWheelInputLimiterAppliesToToolDetailDialog(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	body := NewMessagesWithTone(&defaultTheme, "panel-alt")
	body.SetSize(40, 5)
	body.Sync(strings.Repeat("line\n", 80), false)
	body.GotoTop()
	model := Model{
		dialog: &toolDetailDialog{
			id:   dialogIDToolDetail,
			body: body,
		},
	}
	msg := tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown})
	limiter := &wheelInputLimiter{}
	now := time.Unix(1, 0)

	if drop, reason := limiter.shouldDrop(model, msg, now); drop || reason != "keep" {
		t.Fatalf("first dialog wheel drop=%v reason=%q, want keep", drop, reason)
	}
	if drop, reason := limiter.shouldDrop(model, msg, now.Add(time.Millisecond)); !drop || reason != "drop:coalesce" {
		t.Fatalf("dialog burst wheel drop=%v reason=%q, want coalesced drop", drop, reason)
	}
}

func TestWheelInputLimiterDropsToolDetailDialogMomentumAfterClick(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	body := NewMessagesWithTone(&defaultTheme, "panel-alt")
	body.SetSize(40, 5)
	body.Sync(strings.Repeat("line\n", 80), false)
	body.GotoTop()
	model := Model{
		dialog: &toolDetailDialog{
			id:   dialogIDToolDetail,
			body: body,
		},
	}
	click := tea.MouseClickMsg(tea.Mouse{Button: tea.MouseLeft})
	wheel := tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown})
	limiter := &wheelInputLimiter{}
	now := time.Unix(1, 0)

	limiter.stopWheelAfterClick(model, click, now)
	if drop, reason := limiter.shouldDrop(model, wheel, now.Add(wheelInputStopAfterClickWindow/2)); !drop || reason != "drop:click-stop" {
		t.Fatalf("post-click dialog wheel drop=%v reason=%q, want click-stop drop", drop, reason)
	}
	if drop, reason := limiter.shouldDrop(model, wheel, now.Add(wheelInputStopAfterClickWindow)); drop || reason != "keep" {
		t.Fatalf("later post-click dialog wheel drop=%v reason=%q, want keep", drop, reason)
	}
}

func newInputFilterTestModel(t *testing.T) Model {
	t.Helper()

	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	t.Cleanup(cancel)

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "summarize",
	})

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 18})
	model = modelIface.(Model)
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeAssistantCommit, "session-1", "turn-1", events.AssistantCommitPayload{
		Content: strings.Repeat("line\n", 80),
	}))
	return model
}

func pointInMouseRect(rect inputMouseRect) (int, int) {
	x := rect.x
	y := rect.y
	if rect.width > 1 {
		x++
	}
	if rect.height > 1 {
		y++
	}
	return x, y
}
