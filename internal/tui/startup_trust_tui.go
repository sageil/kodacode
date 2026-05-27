package tui

import (
	"fmt"
	"io"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/cellbuf"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/tui/theme"
)

const (
	startupTrustDefaultWidth  = 100
	startupTrustDefaultHeight = 30
	startupTrustMinWidth      = 72
	startupTrustMaxWidth      = 100
	startupTrustLogoGap       = 2
)

type startupTrustShimTickMsg struct{}

const startupTrustShimInterval = 60 * time.Millisecond

func startupTrustShimTick() tea.Cmd {
	return tea.Tick(startupTrustShimInterval, func(time.Time) tea.Msg { return startupTrustShimTickMsg{} })
}

type startupTrustRowKind string

const (
	startupTrustRowWorkspace startupTrustRowKind = "workspace"
	startupTrustRowServer    startupTrustRowKind = "server"
)

type startupTrustRow struct {
	Kind        startupTrustRowKind
	Fingerprint string
}

type startupTrustPromptModel struct {
	theme           *theme.Theme
	icons           terminalIconProfile
	state           app.StartupTrustState
	rows            []startupTrustRow
	width           int
	height          int
	dialogWidth     int
	shimCol         int
	cursor          int
	trustWorkspace  bool
	serverDecisions map[string]bool
	cancelled       bool
}

func promptStartupTrustTUI(
	in io.Reader,
	out io.Writer,
	th *theme.Theme,
	icons terminalIconProfile,
	state app.StartupTrustState,
) (app.ResolveStartupTrustInput, bool, error) {
	if !state.Pending() {
		return app.ResolveStartupTrustInput{}, true, nil
	}
	program := tea.NewProgram(
		newStartupTrustPromptModelWithIcons(th, icons, state),
		tea.WithInput(in),
		tea.WithOutput(out),
	)
	finalModel, err := program.Run()
	if err != nil {
		return app.ResolveStartupTrustInput{}, false, err
	}
	model, ok := finalModel.(startupTrustPromptModel)
	if !ok {
		return app.ResolveStartupTrustInput{}, false, fmt.Errorf("unexpected startup trust model %T", finalModel)
	}
	if model.cancelled {
		return app.ResolveStartupTrustInput{}, false, nil
	}
	return model.decision(), true, nil
}

func newStartupTrustPromptModel(th *theme.Theme, state app.StartupTrustState) startupTrustPromptModel {
	return newStartupTrustPromptModelWithIcons(th, defaultTerminalIconProfile, state)
}

func newStartupTrustPromptModelWithIcons(th *theme.Theme, icons terminalIconProfile, state app.StartupTrustState) startupTrustPromptModel {
	activeTheme := th
	if activeTheme == nil {
		defaultTheme := theme.StaticDefault()
		activeTheme = &defaultTheme
	}
	model := startupTrustPromptModel{
		theme:           activeTheme,
		icons:           icons,
		state:           state,
		rows:            startupTrustRows(state),
		width:           startupTrustDefaultWidth,
		height:          startupTrustDefaultHeight,
		serverDecisions: make(map[string]bool, len(state.Servers)),
	}
	model.syncDialogWidth()
	return model
}

func startupTrustRows(state app.StartupTrustState) []startupTrustRow {
	rows := make([]startupTrustRow, 0, len(state.Servers)+1)
	if state.WorkspaceRequired {
		rows = append(rows, startupTrustRow{Kind: startupTrustRowWorkspace})
	}
	for _, server := range state.Servers {
		rows = append(rows, startupTrustRow{
			Kind:        startupTrustRowServer,
			Fingerprint: strings.TrimSpace(server.Fingerprint),
		})
	}
	return rows
}

func (m startupTrustPromptModel) Init() tea.Cmd {
	return startupTrustShimTick()
}

func (m startupTrustPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(typed.Width, 1)
		m.height = max(typed.Height, 1)
		return m, nil
	case startupTrustShimTickMsg:
		m.shimCol = (m.shimCol + 1) % (brandLogoWidth + brandLogoShimWidth)
		return m, startupTrustShimTick()
	case tea.KeyPressMsg:
		switch typed.String() {
		case "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		case "esc":
			m.cancelled = true
			return m, tea.Quit
		case "up", "k", "shift+tab", "backtab":
			m.moveCursor(-1)
			m.syncDialogWidth()
			return m, nil
		case "down", "j", "tab":
			m.moveCursor(1)
			m.syncDialogWidth()
			return m, nil
		case "space", " ", "x":
			m.toggleCurrent()
			m.syncDialogWidth()
			return m, nil
		case "enter":
			if !m.canContinue() {
				return m, nil
			}
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m startupTrustPromptModel) View() tea.View {
	frameWidth := max(m.width, 1)
	frameHeight := max(m.height, 1)
	body := m.renderBody()
	hint := "↑/↓ move • space toggle • enter continue • esc cancel • ctrl+c quit"
	naturalWidth := max(m.dialogWidth, startupTrustNaturalWidth(body, hint))
	dialogWidth := desiredDialogWidth(frameWidth, startupTrustMinWidth, min(naturalWidth, startupTrustMaxWidth))
	dialogContent := renderStandaloneDialogContent(m.theme, max(dialogWidth-dialogFrameInset*2, 1), dialogStandaloneFrame{
		Title: "Startup Trust",
		Body:  body,
		Hint:  hint,
	})
	logo := renderBrandLogo(m.theme, m.shimCol)
	logoWidth, logoHeight := lipgloss.Size(logo)
	dialogHeight := lipgloss.Height(dialogContent) + dialogFrameInset*2
	totalHeight := logoHeight + startupTrustLogoGap + dialogHeight
	y := max((frameHeight-totalHeight)/2, 0)
	logoX := max((frameWidth-logoWidth)/2, 0)

	base := cellbuf.NewBuffer(frameWidth, frameHeight)
	cellbuf.SetContent(base, renderPersistentBackground(frameWidth, toneValue(m.theme, toneBG), placeBlock(frameWidth, frameHeight, "", "")))
	drawBlockOnSurface(base, logo, logoX, y)
	drawDialogFrameOnSurface(base, dialogRenderArea{
		y:      y + logoHeight + startupTrustLogoGap,
		width:  frameWidth,
		height: dialogHeight,
	}, m.theme, dialogWidth, dialogContent, nil)
	rendered := renderCellBuffer(base)
	view := tea.NewView(rendered)
	view.AltScreen = true
	if bg := toneValue(m.theme, toneBG); bg != "" {
		view.BackgroundColor = lipgloss.Color(bg)
	}
	return view
}

func startupTrustNaturalWidth(body, hint string) int {
	width := startupTrustMinWidth
	for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
		width = max(width, ansiWidth(line)+6)
	}
	for _, line := range strings.Split(strings.TrimSpace(hint), "\n") {
		width = max(width, ansiWidth(line)+6)
	}
	return width
}

func (m *startupTrustPromptModel) syncDialogWidth() {
	if m == nil {
		return
	}
	hint := "↑/↓ move • space toggle • enter continue • esc cancel • ctrl+c quit"
	m.dialogWidth = max(m.dialogWidth, startupTrustNaturalWidth(m.renderBody(), hint))
}

func (m *startupTrustPromptModel) moveCursor(delta int) {
	if len(m.rows) == 0 {
		m.cursor = 0
		return
	}
	m.cursor = max(min(m.cursor+delta, len(m.rows)-1), 0)
}

func (m *startupTrustPromptModel) toggleCurrent() {
	row, ok := m.currentRow()
	if !ok {
		return
	}
	switch row.Kind {
	case startupTrustRowWorkspace:
		m.trustWorkspace = !m.trustWorkspace
	case startupTrustRowServer:
		key := strings.TrimSpace(row.Fingerprint)
		m.serverDecisions[key] = !m.serverDecisions[key]
	}
}

func (m startupTrustPromptModel) currentRow() (startupTrustRow, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return startupTrustRow{}, false
	}
	return m.rows[m.cursor], true
}

func (m startupTrustPromptModel) renderBody() string {
	lines := []string{
		dialogItemStyle(m.theme).Render("Review the workspace and configured MCP servers before activation."),
	}
	if m.state.WorkspaceRequired {
		lines = append(lines, "", dialogSectionStyle(m.theme).Render("Workspace"))
		lines = append(lines, m.renderWorkspaceRow())
	}
	if len(m.state.Servers) > 0 {
		if !m.state.WorkspaceRequired {
			lines = append(lines, dialogHintStyle(m.theme).Render("Leave unchecked to continue without activating."))
		}
		lines = append(lines, "", dialogSectionStyle(m.theme).Render("MCP Servers"))
		for idx, server := range m.state.Servers {
			lines = append(lines, m.renderServerRow(server, idx))
		}
	}
	lines = append(lines, "", dialogHintStyle(m.theme).Render(m.selectionSummary()))
	return strings.Join(lines, "\n")
}

func (m startupTrustPromptModel) renderWorkspaceRow() string {
	label := checkedLabelWithProfile(m.icons, m.trustWorkspace) + " Trust workspace for session startup"
	detail := strings.TrimSpace(m.state.WorkspaceRoot)
	return m.renderSelectableRowWithDetail(0, label, detail)
}

func (m startupTrustPromptModel) renderServerRow(server app.StartupTrustServer, serverIndex int) string {
	rowIndex := serverIndex
	if m.state.WorkspaceRequired {
		rowIndex++
	}
	label := checkedLabelWithProfile(m.icons, m.serverDecisions[strings.TrimSpace(server.Fingerprint)]) + " " + strings.TrimSpace(server.Name)
	if kind := strings.TrimSpace(server.Type); kind != "" {
		label += " (" + kind + ")"
	}
	detail := serverCommandPreview(server)
	if len(server.EnvKeys) > 0 {
		envStr := "env: " + strings.Join(server.EnvKeys, ", ")
		if detail != "" {
			detail += " · " + envStr
		} else {
			detail = envStr
		}
	}
	return m.renderSelectableRowWithDetail(rowIndex, label, detail)
}

func (m startupTrustPromptModel) renderSelectableRowWithDetail(rowIndex int, text, detail string) string {
	var lines []string
	if rowIndex == m.cursor {
		lines = append(lines, dialogSelectedItemStyle(m.theme).Render(" "+text))
	} else {
		lines = append(lines, dialogItemStyle(m.theme).Render(" "+text))
	}
	if detail != "" {
		lines = append(lines, dialogHintStyle(m.theme).Render("   "+detail))
	}
	return strings.Join(lines, "\n")
}

func serverCommandPreview(server app.StartupTrustServer) string {
	command := strings.TrimSpace(server.Command)
	if len(server.Args) == 0 {
		return command
	}
	if command == "" {
		return strings.Join(server.Args, " ")
	}
	return command + " " + strings.Join(server.Args, " ")
}

func (m startupTrustPromptModel) selectionSummary() string {
	trustedServers := 0
	for _, server := range m.state.Servers {
		if m.serverTrusted(server.Fingerprint) {
			trustedServers++
		}
	}
	switch {
	case m.state.WorkspaceRequired && !m.trustWorkspace:
		return "Workspace trust required"
	case !m.trustWorkspace && trustedServers == 0:
		return "Built-in tools only"
	case m.trustWorkspace && trustedServers == 0:
		return "Trust workspace only"
	case !m.trustWorkspace && trustedServers > 0 && !m.state.WorkspaceRequired:
		return fmt.Sprintf("Trust %d MCP server(s)", trustedServers)
	default:
		return fmt.Sprintf("Trust workspace and %d MCP server(s)", trustedServers)
	}
}

func (m startupTrustPromptModel) serverTrusted(fingerprint string) bool {
	return m.serverDecisions[strings.TrimSpace(fingerprint)]
}

func (m startupTrustPromptModel) decision() app.ResolveStartupTrustInput {
	result := app.ResolveStartupTrustInput{
		WorkspaceRoot:   strings.TrimSpace(m.state.WorkspaceRoot),
		TrustWorkspace:  m.trustWorkspace,
		ServerDecisions: make(map[string]bool, len(m.state.Servers)),
	}
	for _, server := range m.state.Servers {
		result.ServerDecisions[server.Fingerprint] = m.serverTrusted(server.Fingerprint)
	}
	return result
}

func (m startupTrustPromptModel) canContinue() bool {
	if m.state.WorkspaceRequired && !m.trustWorkspace {
		return false
	}
	return true
}

func checkedLabel(value bool) string {
	return checkedLabelWithProfile(defaultTerminalIconProfile, value)
}

func checkedLabelWithProfile(icons terminalIconProfile, value bool) string {
	if value {
		return "[" + icons.Icon(terminalIconSelected) + "]"
	}
	return "[" + icons.Icon(terminalIconUnselected) + "]"
}
