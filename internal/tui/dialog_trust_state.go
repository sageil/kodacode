package tui

import (
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/app"
)

func (d *trustDialog) rowLabel(row trustDialogRow) string {
	switch row.Kind {
	case trustDialogRowWorkspace:
		if row.Trusted {
			return "Workspace"
		}
		return "Workspace  ○ not trusted"
	case trustDialogRowServer:
		label := row.Label
		if label == "" {
			label = row.Fingerprint
		}
		if row.KindLabel != "" {
			label += " (" + row.KindLabel + ")"
		}
		if !row.Trusted {
			label += "  ○ revoked"
		}
		return label
	default:
		return ""
	}
}

func (d *trustDialog) rowDetail(row trustDialogRow) string {
	switch row.Kind {
	case trustDialogRowWorkspace:
		root := strings.TrimSpace(d.state.WorkspaceRoot)
		if !row.UpdatedAt.IsZero() {
			return root + " · " + row.UpdatedAt.Local().Format("Jan 2, 2006")
		}
		return root
	case trustDialogRowServer:
		fp := row.Fingerprint
		if len(fp) > 16 {
			fp = fp[:8] + "…" + fp[len(fp)-4:]
		}
		if !row.UpdatedAt.IsZero() {
			return fp + " · " + row.UpdatedAt.Local().Format("Jan 2, 2006")
		}
		return fp
	}
	return ""
}

func (d *trustDialog) currentRow() (trustDialogRow, bool) {
	if d.cursor < 0 || d.cursor >= len(d.rows) {
		return trustDialogRow{}, false
	}
	return d.rows[d.cursor], true
}

func (d *trustDialog) moveCursor(delta int) {
	if len(d.rows) == 0 {
		d.cursor = 0
		d.offset = 0
		return
	}
	d.cursor = max(min(d.cursor+delta, len(d.rows)-1), 0)
	d.ensureVisible()
}

func (d *trustDialog) ensureVisible() {
	visible := d.visibleRowCount()
	if d.cursor < d.offset {
		d.offset = d.cursor
	}
	if d.cursor >= d.offset+visible {
		d.offset = d.cursor - visible + 1
	}
	if maxOffset := max(len(d.rows)-visible, 0); d.offset > maxOffset {
		d.offset = maxOffset
	}
	if d.offset < 0 {
		d.offset = 0
	}
}

func (d *trustDialog) visibleRowCount() int {
	if d.frameHeight <= 0 {
		return 4
	}
	return max(min((d.frameHeight-18)/2, 8), 2)
}

func (d *trustDialog) selectedRevokeAction() *trustDialogPendingAction {
	row, ok := d.currentRow()
	if !ok || !row.Trusted {
		return nil
	}
	switch row.Kind {
	case trustDialogRowWorkspace:
		return &trustDialogPendingAction{
			Scope:  app.RevokeTrustScopeWorkspace,
			Prompt: "Revoke workspace trust for this directory?",
		}
	case trustDialogRowServer:
		return &trustDialogPendingAction{
			Scope:       app.RevokeTrustScopeServer,
			Fingerprint: row.Fingerprint,
			Prompt:      fmt.Sprintf("Revoke MCP trust for %s in this workspace?", row.Label),
		}
	default:
		return nil
	}
}

func (d *trustDialog) workspaceResetAction() *trustDialogPendingAction {
	return &trustDialogPendingAction{
		Scope:  app.RevokeTrustScopeWorkspaceAll,
		Prompt: "Revoke workspace trust and all MCP trust for this workspace?",
	}
}

func (d *trustDialog) globalResetAction() *trustDialogPendingAction {
	return &trustDialogPendingAction{
		Scope:  app.RevokeTrustScopeAll,
		Prompt: "Revoke all stored workspace and MCP trust everywhere?",
	}
}

func trustDialogWidth(frameWidth int) int {
	natural := trustDialogDefaultWidth
	if frameWidth <= 0 {
		return natural
	}
	return desiredDialogWidth(frameWidth, trustDialogMinWidth, natural)
}
