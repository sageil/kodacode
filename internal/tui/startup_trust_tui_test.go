package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/app"
)

func TestStartupTrustPromptDecisionHonorsSelections(t *testing.T) {
	model := newStartupTrustPromptModel(nil, app.StartupTrustState{
		WorkspaceRoot:     "/repo",
		WorkspaceRequired: true,
		Servers: []app.StartupTrustServer{{
			Name:        "filesystem",
			Type:        "stdio",
			Fingerprint: "fp-filesystem",
		}},
	})

	updated, _ := model.Update(tea.KeyPressMsg{Text: "x", Code: 'x'})
	model = updated.(startupTrustPromptModel)
	updated, _ = model.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	model = updated.(startupTrustPromptModel)
	updated, _ = model.Update(tea.KeyPressMsg{Text: "x", Code: 'x'})
	model = updated.(startupTrustPromptModel)

	decision := model.decision()
	if !decision.TrustWorkspace {
		t.Fatal("decision.TrustWorkspace = false, want true")
	}
	if !decision.ServerDecisions["fp-filesystem"] {
		t.Fatalf("server decisions = %#v, want filesystem trusted", decision.ServerDecisions)
	}
}

func TestStartupTrustPromptEscapeCancels(t *testing.T) {
	model := newStartupTrustPromptModel(nil, app.StartupTrustState{
		WorkspaceRoot:     "/repo",
		WorkspaceRequired: true,
		Servers: []app.StartupTrustServer{{
			Name:        "filesystem",
			Type:        "stdio",
			Fingerprint: "fp-filesystem",
		}},
	})

	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	model = updated.(startupTrustPromptModel)

	if !model.cancelled {
		t.Fatal("model.cancelled = false, want true after esc")
	}
}

func TestStartupTrustPromptSpaceTogglesCurrentSelection(t *testing.T) {
	model := newStartupTrustPromptModel(nil, app.StartupTrustState{
		WorkspaceRoot:     "/repo",
		WorkspaceRequired: true,
	})

	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	model = updated.(startupTrustPromptModel)

	if !model.trustWorkspace {
		t.Fatal("trustWorkspace = false, want true after pressing space")
	}
}

func TestStartupTrustPromptRequiresWorkspaceSelectionToContinue(t *testing.T) {
	model := newStartupTrustPromptModel(nil, app.StartupTrustState{
		WorkspaceRoot:     "/repo",
		WorkspaceRequired: true,
	})

	if model.canContinue() {
		t.Fatal("canContinue() = true, want false before trusting workspace")
	}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "x", Code: 'x'})
	model = updated.(startupTrustPromptModel)
	if !model.canContinue() {
		t.Fatal("canContinue() = false, want true after trusting workspace")
	}
}

func TestStartupTrustPromptKeepsWidestDialogWidth(t *testing.T) {
	model := newStartupTrustPromptModel(nil, app.StartupTrustState{
		WorkspaceRoot:     "/Users/sageil/dev/typescript/projects/very-long-workspace-name-for-sizing",
		WorkspaceRequired: true,
		Servers: []app.StartupTrustServer{{
			Name:        "seq",
			Type:        "stdio",
			Fingerprint: "fp-seq",
			Command:     "npx",
		}},
	})
	initialWidth := model.dialogWidth

	// With always-visible detail rows, all details are shown regardless of cursor
	// position, so the body width stays constant. Verify dialogWidth is not reduced.
	updated, _ := model.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	model = updated.(startupTrustPromptModel)

	if model.dialogWidth < initialWidth {
		t.Fatalf("dialog width = %d, want at least initial width %d to be preserved", model.dialogWidth, initialWidth)
	}
}

func TestStartupTrustPromptExpandsDialogWidthForWiderSelection(t *testing.T) {
	// With always-visible detail rows, the body already includes all server details
	// on initial render. Verify that a model with a long-command server is wider
	// than a comparable model with only the short-command server.
	shortOnly := newStartupTrustPromptModel(nil, app.StartupTrustState{
		Servers: []app.StartupTrustServer{
			{
				Name:        "short",
				Type:        "stdio",
				Fingerprint: "fp-short",
				Command:     "npx",
			},
		},
	})

	withLong := newStartupTrustPromptModel(nil, app.StartupTrustState{
		Servers: []app.StartupTrustServer{
			{
				Name:        "short",
				Type:        "stdio",
				Fingerprint: "fp-short",
				Command:     "npx",
			},
			{
				Name:        "long",
				Type:        "stdio",
				Fingerprint: "fp-long",
				Command:     "npx",
				Args: []string{
					"-y",
					"@modelcontextprotocol/server-sequential-thinking",
					"--with-extra-arguments-to-force-a-wider-details-line",
				},
			},
		},
	})

	if withLong.dialogWidth <= shortOnly.dialogWidth {
		t.Fatalf("dialog width with long server = %d, want wider than short-only width %d", withLong.dialogWidth, shortOnly.dialogWidth)
	}
}

func TestStartupTrustPromptViewIncludesBrandLogo(t *testing.T) {
	model := newStartupTrustPromptModel(nil, app.StartupTrustState{
		WorkspaceRoot:     "/repo",
		WorkspaceRequired: true,
	})
	model.width = 120
	model.height = 40

	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, "____") || !strings.Contains(view, "Startup Trust") {
		t.Fatalf("startup trust view missing logo or dialog title:\n%s", view)
	}
}

func TestStartupTrustPromptLogoDoesNotInheritDialogBackground(t *testing.T) {
	model := newStartupTrustPromptModel(nil, app.StartupTrustState{
		WorkspaceRoot:     "/repo",
		WorkspaceRequired: true,
	})
	model.width = 120
	model.height = 40

	rawView := model.View().Content
	dialogBG := backgroundANSI(toneValue(model.theme, toneBGAlt))
	if dialogBG == "" {
		t.Fatal("dialog background ANSI = empty, cannot verify logo background")
	}
	screenBG := backgroundANSI(toneValue(model.theme, toneBG))
	if screenBG == "" {
		t.Fatal("screen background ANSI = empty, cannot verify logo background")
	}

	for _, line := range strings.Split(rawView, "\n") {
		if !strings.Contains(ansi.Strip(line), "____") {
			continue
		}
		if strings.Contains(line, dialogBG) {
			t.Fatalf("logo line unexpectedly contains dialog background ANSI:\n%s", line)
		}
		if !strings.Contains(line, screenBG) {
			t.Fatalf("logo line missing screen background ANSI:\n%s", line)
		}
		return
	}
	t.Fatalf("failed to find logo line in rendered view:\n%s", ansi.Strip(rawView))
}

func TestStartupTrustPromptViewSetsBackgroundColorFromShellTone(t *testing.T) {
	model := newStartupTrustPromptModel(nil, app.StartupTrustState{
		WorkspaceRoot:     "/repo",
		WorkspaceRequired: true,
	})
	model.width = 120
	model.height = 40

	view := model.View()
	want := lipgloss.Color(toneValue(model.theme, toneBG))
	if !reflect.DeepEqual(view.BackgroundColor, want) {
		t.Fatalf("view.BackgroundColor = %#v, want %#v", view.BackgroundColor, want)
	}
}

func TestStartupTrustPromptShimTickAdvancesLogoAnimation(t *testing.T) {
	model := newStartupTrustPromptModel(nil, app.StartupTrustState{
		WorkspaceRoot:     "/repo",
		WorkspaceRequired: true,
	})
	if model.shimCol != 0 {
		t.Fatalf("initial shimCol = %d, want 0", model.shimCol)
	}

	updated, cmd := model.Update(startupTrustShimTickMsg{})
	model = updated.(startupTrustPromptModel)

	if model.shimCol != 1 {
		t.Fatalf("shimCol = %d, want 1 after tick", model.shimCol)
	}
	if cmd == nil {
		t.Fatal("tick update returned nil cmd, want next shimmer tick scheduled")
	}
}
