package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
)

func TestTrustDialogCursorVisibleInView(t *testing.T) {
	state := app.WorkspaceTrustState{
		Trusted:       true,
		WorkspaceRoot: "/repo",
		Servers: []app.WorkspaceMCPTrustState{
			{Fingerprint: "fp-1", Label: "server-one", Kind: "stdio", Trusted: true},
			{Fingerprint: "fp-2", Label: "server-two", Kind: "sse", Trusted: false},
		},
	}
	dialog := newTrustDialog(state, nil)
	dialog.SetFrame(96, 30)

	// cursor=0: first row (Workspace) should have the selected checkbox marker.
	view0 := renderTestDialogContentPlain(dialog)
	workspaceLineSelected := false
	for _, line := range strings.Split(view0, "\n") {
		if strings.Contains(line, "[ * ]") && strings.Contains(line, "Workspace") {
			workspaceLineSelected = true
		}
	}
	if !workspaceLineSelected {
		t.Fatalf("cursor=0: expected selected Workspace row in view, got:\n%s", view0)
	}

	// move down: cursor=1, server-one should have the selected checkbox marker.
	updated, _ := dialog.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	d := updated.(*trustDialog)
	d.SetFrame(96, 30)

	view1 := renderTestDialogContentPlain(d)
	serverLineSelected := false
	for _, line := range strings.Split(view1, "\n") {
		if strings.Contains(line, "[ * ]") && strings.Contains(line, "server-one") {
			serverLineSelected = true
		}
	}
	if !serverLineSelected {
		t.Fatalf("cursor=1: expected selected server-one row in view, got:\n%s", view1)
	}
}
