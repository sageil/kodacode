package tui

import (
	"strings"
	"testing"
)

func TestStatusBarView_HidesCostAndTokenMeters(t *testing.T) {
	sb := StatusBar{
		width:          120,
		gitBranch:      "main",
		sessionCost:    0.42,
		inputTokens:    27900,
		outputTokens:   1200,
		maxInputTokens: 128000,
	}

	view := sb.View()

	if !strings.Contains(view, "⎇ main") {
		t.Fatalf("missing branch in status bar: %q", view)
	}
	if strings.Contains(view, "$0.42") {
		t.Fatalf("status bar should not show session cost: %q", view)
	}
	if strings.Contains(view, "27.9k") || strings.Contains(view, "128k") {
		t.Fatalf("status bar should not show detailed token meter: %q", view)
	}
}

func TestStatusBarView_ShowsWarningChips(t *testing.T) {
	sb := StatusBar{
		width:          120,
		budgetWarn:     true,
		inputTokens:    120000,
		maxInputTokens: 128000,
	}

	view := sb.View()

	if !strings.Contains(view, "⚠ budget") {
		t.Fatalf("missing budget warning: %q", view)
	}
	if !strings.Contains(view, "⚠ context 94%") {
		t.Fatalf("missing context warning: %q", view)
	}
}

func TestStatusBarView_ShowsLSPServersAndChangedFiles(t *testing.T) {
	sb := StatusBar{
		width:        120,
		gitBranch:    "main",
		lspServers:   []string{"eslint", "tsserver"},
		changedFiles: 3,
	}

	view := sb.View()

	if !strings.Contains(view, "⎇ main") {
		t.Fatalf("missing branch in status bar: %q", view)
	}
	if !strings.Contains(view, "LSP eslint + tsserver") {
		t.Fatalf("missing LSP server names in status bar: %q", view)
	}
	if !strings.Contains(view, "3 changed") {
		t.Fatalf("missing changed-file count in status bar: %q", view)
	}
}
