package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestDelegatedPermissionSystemRowCachePartsVaryByRenderState(t *testing.T) {
	handoff := delegatedPermissionSystemRowTestHandoff()
	base := newDelegatedPermissionSystemRow(handoff, 80)
	if len(base.cacheParts()) == 0 {
		t.Fatal("system row cache parts empty")
	}

	changedHandoff := delegatedPermissionSystemRowTestHandoff()
	changedHandoff.PermissionPath = "/tmp/other"
	changed := newDelegatedPermissionSystemRow(changedHandoff, 80)
	if strings.Join(changed.cacheParts(), "\x00") == strings.Join(base.cacheParts(), "\x00") {
		t.Fatal("system row cache parts did not vary by handoff content")
	}

	withFocus := base
	withFocus.focused = true
	if strings.Join(withFocus.cacheParts(), "\x00") == strings.Join(base.cacheParts(), "\x00") {
		t.Fatal("system row cache parts did not vary by focus state")
	}
}

func TestDelegatedPermissionSystemRowRendersNotice(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:   ctx,
		Theme:     &defaultTheme,
		SessionID: "session-1",
		TurnID:    "turn-1",
	})
	row := newDelegatedPermissionSystemRow(delegatedPermissionSystemRowTestHandoff(), 80)

	rendered := ansi.Strip(row.render(model).content)
	for _, want := range []string{
		"DELEGATED CHILD WAITING ON APPROVAL",
		"Resolve the approval prompt",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("delegated permission system row missing %q:\n%s", want, rendered)
		}
	}
}

func delegatedPermissionSystemRowTestHandoff() *events.AgentHandoffState {
	return &events.AgentHandoffState{
		HandoffID:           "handoff-1",
		ChildAgentID:        "builder",
		Status:              events.AgentResultStatusPendingPermission,
		PermissionRequestID: "perm-1",
		PermissionKind:      events.PermissionRequestKindPath,
		PermissionToolName:  "read",
		PermissionAccess:    "read",
		PermissionPath:      "/tmp/secret.txt",
	}
}
