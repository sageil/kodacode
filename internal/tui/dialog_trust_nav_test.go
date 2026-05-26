package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
)

func TestTrustDialogArrowNavigation(t *testing.T) {
	state := app.WorkspaceTrustState{
		Trusted:       true,
		WorkspaceRoot: "/repo",
		Servers: []app.WorkspaceMCPTrustState{
			{Fingerprint: "fp-1", Label: "server-one", Kind: "stdio", Trusted: true},
			{Fingerprint: "fp-2", Label: "server-two", Kind: "sse", Trusted: false},
		},
	}
	dialog := newTrustDialog(state, nil)

	if dialog.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", dialog.cursor)
	}

	// down arrow -> cursor 1
	updated, _ := dialog.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	d, ok := updated.(*trustDialog)
	if !ok {
		t.Fatalf("Update returned %T, want *trustDialog", updated)
	}
	if d.cursor != 1 {
		t.Fatalf("cursor after down = %d, want 1", d.cursor)
	}

	// down arrow again -> cursor 2
	updated, _ = d.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	d = updated.(*trustDialog)
	if d.cursor != 2 {
		t.Fatalf("cursor after second down = %d, want 2", d.cursor)
	}

	// down at end -> stays at 2
	updated, _ = d.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	d = updated.(*trustDialog)
	if d.cursor != 2 {
		t.Fatalf("cursor after down at end = %d, want 2", d.cursor)
	}

	// up arrow -> cursor 1
	updated, _ = d.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	d = updated.(*trustDialog)
	if d.cursor != 1 {
		t.Fatalf("cursor after up = %d, want 1", d.cursor)
	}

	// j key -> cursor 2
	updated, _ = d.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	d = updated.(*trustDialog)
	if d.cursor != 2 {
		t.Fatalf("cursor after j = %d, want 2", d.cursor)
	}

	// k key -> cursor 1
	updated, _ = d.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	d = updated.(*trustDialog)
	if d.cursor != 1 {
		t.Fatalf("cursor after k = %d, want 1", d.cursor)
	}
}

func TestStartupTrustDialogArrowNavigation(t *testing.T) {
	model := newStartupTrustPromptModel(nil, app.StartupTrustState{
		WorkspaceRoot:     "/repo",
		WorkspaceRequired: true,
		Servers: []app.StartupTrustServer{
			{Name: "server-one", Type: "stdio", Fingerprint: "fp-1"},
			{Name: "server-two", Type: "sse", Fingerprint: "fp-2"},
		},
	})

	if model.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", model.cursor)
	}

	// down arrow -> cursor 1
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m, ok := updated.(startupTrustPromptModel)
	if !ok {
		t.Fatalf("Update returned %T, want startupTrustPromptModel", updated)
	}
	if m.cursor != 1 {
		t.Fatalf("cursor after down = %d, want 1", m.cursor)
	}

	// down again -> cursor 2
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(startupTrustPromptModel)
	if m.cursor != 2 {
		t.Fatalf("cursor after second down = %d, want 2", m.cursor)
	}

	// up -> cursor 1
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = updated.(startupTrustPromptModel)
	if m.cursor != 1 {
		t.Fatalf("cursor after up = %d, want 1", m.cursor)
	}
}
