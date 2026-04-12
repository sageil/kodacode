package tui

import (
	"fmt"
	"strings"
	"testing"
)

func TestHumanizeArgs_UsesPermissionDisplay(t *testing.T) {
	got := humanizeArgs("bash", `{"command":"cat /etc/passwd","permission_display":"/tmp/external/path/that/should/be/shown"}`)
	want := "/tmp/external/path/that/should/be/shown"
	if got != want {
		t.Fatalf("humanizeArgs() = %q, want %q", got, want)
	}
}

func TestInlinePermissionPanel_ViewShowsFullPathWithoutTruncation(t *testing.T) {
	panel := NewInlinePermissionPanel("p1", PermissionRequest{
		ToolName:  "read",
		Arguments: `{"permission_display":"/tmp/external/path/that/should/be/shown/fully.txt"}`,
	}, 100, nil)

	view := fmt.Sprint(panel.View())
	if !strings.Contains(view, "/tmp/external/path/that/should/be/shown/fully.txt") {
		t.Fatalf("permission panel should show full path, got:\n%s", view)
	}
	if strings.Contains(view, "…") {
		t.Fatalf("permission panel should not truncate the displayed path, got:\n%s", view)
	}
}

func TestInlinePermissionPanel_PanelHeightExpandsForWrappedArguments(t *testing.T) {
	panel := NewInlinePermissionPanel("p2", PermissionRequest{
		ToolName:  "read",
		Arguments: `{"permission_display":"/tmp/external/path/that/should/stretch/across/multiple/lines/when/the/panel/is/narrow/fully.txt"}`,
	}, 24, nil)

	if got := panel.PanelHeight(); got <= 5 {
		t.Fatalf("PanelHeight() = %d, want more than the single-line height", got)
	}
}
