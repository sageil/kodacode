package tui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

func (a *App) applyStatusBar() {
	a.session.SetToolCount(a.sbToolCount)
	a.session.SetMCPServers(a.sbMCPServers)
	a.session.SetGitBranch(a.sbGitBranch)
	a.session.SetLSPServers(a.sbLSPServers)
	a.session.SetChangedFiles(a.sbChangedFiles)
}

func (a App) providerSyncBlocked() bool {
	active, err := a.api.HasActiveTurns(a.ctx)
	if err != nil {
		return true
	}
	return active
}

func (a *App) replaceAvailableModels(providers []APIProviderModels) []ModelItem {
	items := buildModelItems(providers)
	a.cfg.Models = make(map[string]ModelItem, len(items))
	for _, item := range items {
		a.cfg.Models[item.ProviderID+"/"+item.ModelID] = item
	}
	if item, ok := a.cfg.Models[a.cfg.Model]; ok {
		a.cfg.ModelInfo = modelInfoString(item)
		a.cfg.HasReasoning = item.Reasoning
		a.home.SetProviderName(item.ProviderName)
		if a.route == routeSession {
			a.session.SetProviderName(item.ProviderName)
			a.session.SetModelInfo(a.cfg.ModelInfo)
		}
	}
	cmds := a.buildSlashCommands()
	a.home.footer.SetSlashCommands(cmds)
	a.session.footer.SetSlashCommands(cmds)
	return items
}

func (a *App) refreshHomeRecentSessions() {
	ctx, cancel := context.WithTimeout(a.ctx, 3*time.Second)
	defer cancel()
	sessions, err := a.api.ListSessions(ctx)
	if err != nil {
		return
	}
	var recent []RecentSession
	for _, s := range sessions {
		if s.Title == "" {
			continue
		}
		recent = append(recent, RecentSession{
			ID:        s.ID,
			Title:     s.Title,
			UpdatedAt: s.UpdatedAt,
		})
		if len(recent) >= 5 {
			break
		}
	}
	a.home.SetRecentSessions(recent)
}

func (a *App) refreshTaskPanel() {
	if a.sessionID != "" {
		a.session.SetTasks(a.taskStore.GetTasks(a.sessionID))
	}
}

func (a App) deferMCPRefresh() tea.Cmd {
	return a.mcpRefreshTick(0)
}

func (a App) mcpRefreshTick(attempt int) tea.Cmd {
	api := a.api
	ctx := a.ctx
	delay := 2 * time.Second
	if attempt == 0 {
		delay = 3 * time.Second
	}
	return tea.Tick(delay, func(time.Time) tea.Msg {
		cfg, err := api.GetConfig(ctx)
		if err != nil {
			return nil
		}
		return mcpStatusMsg{servers: cfg.MCPServers, attempt: attempt}
	})
}

func (a App) innerWidth() int  { return a.width }
func (a App) innerHeight() int { return a.height }

func (a *App) showErrorToast(err string) tea.Cmd {
	a.errorBanner = err
	a.retryBanner = ""
	a.infoBanner = ""
	return nil
}

func (a *App) showInfoToast(text string) tea.Cmd {
	a.infoBanner = text
	a.errorBanner = ""
	a.retryBanner = ""
	return nil
}

func renderBanner(text, colorKey, label string, w int, th *theme.Theme) string {
	accentColor := colorFrom(th, colorKey, lipgloss.Color("62"))
	dimColor := colorFrom(th, "subtext", lipgloss.Color("241"))

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentColor).
		Width(w-2).
		Padding(0, 1)

	title := lipgloss.NewStyle().Bold(true).Foreground(accentColor).Render(label)
	hint := lipgloss.NewStyle().Foreground(dimColor).Render("ctrl+y copy  esc dismiss")

	textWidth := w - 6 // border + padding
	wrapped := wordWrap(text, textWidth)

	header := title + strings.Repeat(" ", max(textWidth-lipgloss.Width(title)-lipgloss.Width(hint), 1)) + hint
	body := header + "\n" + wrapped

	return borderStyle.Render(body)
}

func wordWrap(text string, width int) string {
	if width <= 0 {
		return text
	}
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		for len(line) > width {
			cut := width
			if idx := strings.LastIndex(line[:cut], " "); idx > 0 {
				cut = idx
			}
			lines = append(lines, line[:cut])
			line = strings.TrimLeft(line[cut:], " ")
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (a *App) updateFooterAttachments() {
	switch a.route {
	case routeHome:
		a.home.footer.SetPendingAttachments(a.pendingAttachments)
	case routeSession:
		a.session.footer.SetPendingAttachments(a.pendingAttachments)
	}
}

func (a *App) appendPendingAttachment(att Attachment) bool {
	for _, existing := range a.pendingAttachments {
		if existing.Path == att.Path {
			a.updateFooterAttachments()
			return false
		}
	}
	a.pendingAttachments = append(a.pendingAttachments, att)
	a.updateFooterAttachments()
	return true
}

func (a *App) removePendingAttachmentAt(idx int) bool {
	if idx < 0 || idx >= len(a.pendingAttachments) {
		return false
	}
	a.pendingAttachments = append(a.pendingAttachments[:idx], a.pendingAttachments[idx+1:]...)
	a.updateFooterAttachments()
	return true
}

func padView(content string, w, h int) string {
	lines := strings.Split(content, "\n")
	var padded strings.Builder
	for i := range h {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		lw := lipgloss.Width(line)
		if lw < w {
			line += strings.Repeat(" ", w-lw)
		} else if lw > w {
			line = ansi.Truncate(line, w, "")
		}
		if i > 0 {
			padded.WriteByte('\n')
		}
		padded.WriteString(line)
	}
	return padded.String()
}
