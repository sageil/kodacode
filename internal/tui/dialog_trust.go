package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/tui/theme"
)

const (
	trustDialogDefaultWidth  = 96
	trustDialogDefaultHeight = 30
	trustDialogMinWidth      = 72
	trustDialogSidebarWidth  = 12
)

type trustDialogRowKind string

const (
	trustDialogRowWorkspace trustDialogRowKind = "workspace"
	trustDialogRowServer    trustDialogRowKind = "server"
)

type trustDialogPanel int

const (
	trustDialogPanelEntries trustDialogPanel = iota
	trustDialogPanelDanger
)

type trustDialogRow struct {
	Kind        trustDialogRowKind
	Fingerprint string
	Label       string
	KindLabel   string
	Trusted     bool
	UpdatedAt   time.Time
}

type trustDialogResult struct {
	Scope       app.RevokeTrustScope
	Fingerprint string
}

type trustDialogPendingAction struct {
	Scope       app.RevokeTrustScope
	Fingerprint string
	Prompt      string
}

type trustDialog struct {
	id          string
	frameWidth  int
	frameHeight int
	theme       *theme.Theme
	state       app.WorkspaceTrustState
	rows        []trustDialogRow
	cursor      int
	offset      int
	confirm     *trustDialogPendingAction
	panel       trustDialogPanel
}

func newTrustDialog(state app.WorkspaceTrustState, th *theme.Theme) *trustDialog {
	dialog := &trustDialog{
		id:          dialogIDTrust,
		frameWidth:  trustDialogDefaultWidth,
		frameHeight: trustDialogDefaultHeight,
		theme:       th,
	}
	dialog.Sync(state)
	return dialog
}

func (d *trustDialog) ID() string { return d.id }

func (d *trustDialog) ApplyTheme(th *theme.Theme) {
	d.theme = th
}

func (d *trustDialog) SetFrame(width, height int) {
	d.frameWidth = width
	d.frameHeight = height
	d.ensureVisible()
}

func (d *trustDialog) Update(msg tea.Msg) (dialogModel, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.KeyPressMsg:
		if d.confirm != nil {
			switch typed.String() {
			case "ctrl+c", "q":
				return d, closeDialog(d.id, nil)
			case "esc", "n":
				d.confirm = nil
				return d, nil
			case "enter", "y":
				result := trustDialogResult{
					Scope:       d.confirm.Scope,
					Fingerprint: d.confirm.Fingerprint,
				}
				return d, closeDialog(d.id, result)
			default:
				return d, nil
			}
		}
		switch typed.String() {
		case "ctrl+c", "q", "esc":
			return d, closeDialog(d.id, nil)
		case "tab":
			if d.panel == trustDialogPanelEntries {
				d.panel = trustDialogPanelDanger
			} else {
				d.panel = trustDialogPanelEntries
			}
			return d, nil
		case "up", "k":
			if d.panel == trustDialogPanelEntries {
				d.moveCursor(-1)
			}
			return d, nil
		case "down", "j":
			if d.panel == trustDialogPanelEntries {
				d.moveCursor(1)
			}
			return d, nil
		case "home", "g":
			if d.panel == trustDialogPanelEntries {
				d.cursor = 0
				d.ensureVisible()
			}
			return d, nil
		case "end", "G":
			if d.panel == trustDialogPanelEntries {
				if len(d.rows) > 0 {
					d.cursor = len(d.rows) - 1
				}
				d.ensureVisible()
			}
			return d, nil
		case "r":
			if d.panel == trustDialogPanelEntries {
				d.confirm = d.selectedRevokeAction()
			}
			return d, nil
		case "a":
			if d.panel == trustDialogPanelDanger {
				d.confirm = d.workspaceResetAction()
			}
			return d, nil
		case "A":
			if d.panel == trustDialogPanelDanger {
				d.confirm = d.globalResetAction()
			}
			return d, nil
		}
	}
	return d, nil
}

func (d *trustDialog) Draw(surface dialogSurface, area dialogRenderArea) *tea.Cursor {
	d.SetFrame(area.width, area.height)
	width := trustDialogWidth(d.frameWidth)
	content := renderStandaloneDialogContent(d.theme, max(width-dialogFrameInset*2, 1), dialogStandaloneFrame{
		Title: "Trust",
		Body:  d.renderBody(),
		Hint:  d.renderHint(),
	})
	return drawDialogFrameOnSurface(surface, area, d.theme, width, content, nil)
}

func (d *trustDialog) Sync(state app.WorkspaceTrustState) {
	d.state = state
	d.confirm = nil
	d.rows = buildTrustDialogRows(state)
	if len(d.rows) == 0 {
		d.cursor = 0
		d.offset = 0
		return
	}
	if d.cursor >= len(d.rows) {
		d.cursor = len(d.rows) - 1
	}
	d.ensureVisible()
}

func buildTrustDialogRows(state app.WorkspaceTrustState) []trustDialogRow {
	rows := []trustDialogRow{{
		Kind:      trustDialogRowWorkspace,
		Label:     "Workspace trust",
		Trusted:   state.Trusted,
		UpdatedAt: state.UpdatedAt,
	}}
	for _, server := range state.Servers {
		rows = append(rows, trustDialogRow{
			Kind:        trustDialogRowServer,
			Fingerprint: server.Fingerprint,
			Label:       strings.TrimSpace(server.Label),
			KindLabel:   strings.TrimSpace(server.Kind),
			Trusted:     server.Trusted,
			UpdatedAt:   server.UpdatedAt,
		})
	}
	return rows
}
