package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func (m *Model) ensureWorkspaceStatusLoadedCmd() tea.Cmd {
	if m.footerStatus.workspaceLoading || m.controller == nil || m.ctx == nil {
		return nil
	}
	if m.sessionID == "" {
		return nil
	}
	m.footerStatus.workspaceLoading = true
	return loadWorkspaceStatusCmd(m.ctx, m.controller, m.sessionID)
}

func loadWorkspaceStatusCmd(ctx context.Context, controller controller, sessionID string) tea.Cmd {
	return func() tea.Msg {
		status, err := controller.WorkspaceStatus(ctx, sessionID)
		return workspaceStatusLoadedMsg{
			sessionID: sessionID,
			status:    status,
			err:       err,
		}
	}
}

func shouldRefreshWorkspaceStatusForEvent(event events.Event) bool {
	switch event.Type {
	case events.TypeSessionConfigured:
		return true
	case events.TypeWorkspaceWriteRestored:
		return true
	case events.TypeToolExecEnd:
		payload, ok := event.Payload.(events.ToolExecEndPayload)
		return ok && toolAffectsWorkspaceStatus(payload.ToolName)
	default:
		return false
	}
}

func toolAffectsWorkspaceStatus(toolName string) bool {
	switch strings.TrimSpace(toolName) {
	case "bash",
		"edit",
		"mkdir",
		tool.SearchToolName,
		"write",
		tool.CodeActionToolName,
		tool.DefinitionToolName,
		tool.DiagnosticsToolName,
		tool.RenameSymbolToolName,
		tool.SymbolsToolName:
		return true
	default:
		return false
	}
}

func footerGitStatus(status app.WorkspaceStatus) *app.WorkspaceGitStatus {
	return status.Git
}
