package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestMessagesMouseWheelUsesSingleLineDelta(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	msgs := NewMessages(&defaultTheme)
	msgs.SetSize(40, 6)
	msgs.Sync(strings.Repeat("line\n", 40), false)
	msgs.GotoTop()

	_ = msgs.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown}))
	if got := msgs.YOffset(); got != 1 {
		t.Fatalf("mouse wheel delta = %d, want 1", got)
	}
}

func TestTranscriptMessagesIgnoreHorizontalScroll(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	msgs := NewMessages(&defaultTheme)
	msgs.SetSize(12, 4)
	msgs.Sync(strings.Repeat("very wide transcript line ", 4), false)

	_ = msgs.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelRight}))
	if got := msgs.vp.XOffset(); got != 0 {
		t.Fatalf("horizontal offset = %d, want 0", got)
	}
}
