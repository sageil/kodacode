package tui

import (
	"strings"
	"testing"
)

func TestHeaderShowsAgentAndModel(t *testing.T) {
	h := NewHeader()
	h.SetSize(80)
	h.SetAgent("default", "Engineer")
	h.SetModel("openai/gpt-4o")
	view := h.View()
	if !strings.Contains(view, "Engineer") {
		t.Errorf("Header.View() missing agent, got: %q", view)
	}
	// Provider prefix is stripped: "openai/gpt-4o" → "gpt-4o"
	if !strings.Contains(view, "gpt-4o") {
		t.Errorf("Header.View() missing model, got: %q", view)
	}
}

func TestHeaderFallsBackToFormattedAgentID(t *testing.T) {
	h := NewHeader()
	h.SetSize(80)
	h.SetAgent("engineer", "")
	view := h.View()
	if !strings.Contains(view, "Engineer") {
		t.Fatalf("Header.View() missing formatted fallback agent name, got: %q", view)
	}
}

func TestStripProviderPrefix(t *testing.T) {
	cases := []struct{ input, want string }{
		{"anthropic/claude-sonnet-4", "claude-sonnet-4"},
		{"openai/gpt-4o", "gpt-4o"},
		{"gpt-4o", "gpt-4o"},
		{"", ""},
	}
	for _, c := range cases {
		if got := stripProviderPrefix(c.input); got != c.want {
			t.Errorf("stripProviderPrefix(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
