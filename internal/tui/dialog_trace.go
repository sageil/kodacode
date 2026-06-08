package tui

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

type traceDialog struct {
	id          string
	turnID      string
	frameWidth  int
	frameHeight int
	theme       *theme.Theme
	subtitle    string
	body        Messages
}

func newTraceDialog(m Model, state events.SessionState, turnID string) *traceDialog {
	dialog := &traceDialog{
		id:          dialogIDTrace,
		turnID:      turnID,
		frameWidth:  104,
		frameHeight: 32,
		theme:       m.theme,
		body:        NewMessagesWithTone(m.theme, "panel-alt"),
	}
	width, height := dialogRenderSize(m, state)
	dialog.SetFrame(width, height)
	dialog.Sync(state, turnID)
	return dialog
}

func (d *traceDialog) ID() string { return d.id }

func (d *traceDialog) ignoreWheel(msg tea.MouseWheelMsg) bool {
	return shouldDropVerticalWheel(d.body, msg)
}

func (d *traceDialog) wheelState() (int, bool) {
	return d.body.YOffset(), d.body.AtBottom()
}

func (d *traceDialog) ApplyTheme(th *theme.Theme) {
	d.theme = th
	d.body.ApplyTheme(th)
}

func (d *traceDialog) SetFrame(width, height int) {
	d.frameWidth = width
	d.frameHeight = height
	dialogWidth := traceDialogWidth(width)
	bodyWidth := max(dialogWidth-6, 1)
	bodyHeight := traceDialogBodyHeight(height)
	if d.body.Width() == bodyWidth && d.body.Height() == bodyHeight {
		return
	}
	d.body.SetSize(bodyWidth, bodyHeight)
}

func (d *traceDialog) Update(msg tea.Msg) (dialogModel, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.KeyPressMsg:
		switch typed.String() {
		case "q", "esc", "ctrl+c":
			return d, closeDialog(d.id, nil)
		case "up", "k":
			d.body.ScrollUp(1)
			return d, nil
		case "down", "j":
			d.body.ScrollDown(1)
			return d, nil
		case "pgup", "ctrl+u":
			d.body.PageUp()
			return d, nil
		case "pgdown", "ctrl+d":
			d.body.PageDown()
			return d, nil
		case "home", "g":
			d.body.GotoTop()
			return d, nil
		case "end", "G":
			d.body.GotoBottom()
			return d, nil
		}
	case tea.MouseWheelMsg:
		cmd := d.body.Update(typed)
		return d, cmd
	}
	return d, nil
}

func (d *traceDialog) Draw(surface dialogSurface, area dialogRenderArea) *tea.Cursor {
	d.SetFrame(area.width, area.height)
	parts := make([]string, 0, 2)
	if strings.TrimSpace(d.subtitle) != "" {
		parts = append(parts, lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(d.theme, "subtext", "#9da8ca"))).
			Render(d.subtitle))
	}
	parts = append(parts, d.body.View())
	width := traceDialogWidth(d.frameWidth)
	content := renderStandaloneDialogContent(d.theme, max(width-dialogFrameInset*2, 1), dialogStandaloneFrame{
		Title: "Turn Trace",
		Body:  strings.Join(parts, "\n\n"),
		Hint:  "q close • ↑/↓ scroll • pgup/pgdn page",
	})
	return drawDialogFrameOnSurface(surface, area, d.theme, width, content, nil)
}

func (d *traceDialog) Sync(state events.SessionState, turnID string) {
	d.turnID = turnID
	d.subtitle = traceDialogSubtitle(state, turnID)
	wasEmpty := strings.TrimSpace(d.body.raw) == ""
	d.body.Sync(traceDialogBody(d.theme, state, turnID), false)
	if wasEmpty {
		d.body.GotoTop()
	}
}

func (d *traceDialog) OverlayCacheKey() uint64 {
	hasher := fnv.New64a()
	writeTranscriptSignatureString(hasher, d.subtitle)
	appendMessagesRenderCacheSignature(hasher, d.body)
	return hasher.Sum64()
}

func traceDialogWidth(frameWidth int) int {
	if frameWidth <= 0 {
		return 104
	}
	return min(max(frameWidth-8, 72), 124)
}

func traceDialogBodyHeight(frameHeight int) int {
	if frameHeight <= 0 {
		return 18
	}
	outerHeight := min(max(frameHeight-6, 14), 38)
	return max(outerHeight-9, 6)
}

func (m *Model) openTraceDialog(turnArg string) tea.Cmd {
	state := m.projector.Snapshot()
	turnID, err := resolveTraceDialogTurnIDArg(state, turnArg)
	if err != nil {
		m.setComposerError(err.Error())
		return nil
	}
	m.clearComposerError()
	return func() tea.Msg {
		return dialogOpenedMsg{dialog: newTraceDialog(*m, state, turnID)}
	}
}

func (m *Model) openTraceDialogForTurnID(turnID string) tea.Cmd {
	state := m.projector.Snapshot()
	turnID = strings.TrimSpace(turnID)
	if turnID != "" && state.Turns[turnID] == nil {
		m.setComposerError(fmt.Sprintf("turn %s not found", turnID))
		return nil
	}
	m.clearComposerError()
	return func() tea.Msg {
		return dialogOpenedMsg{dialog: newTraceDialog(*m, state, turnID)}
	}
}

func (m *Model) syncTraceDialog() {
	if m == nil {
		return
	}
	dialog, ok := m.dialog.(*traceDialog)
	if !ok {
		return
	}
	state := m.projector.Snapshot()
	if strings.TrimSpace(dialog.turnID) != "" && state.Turns[dialog.turnID] == nil {
		m.dialog = nil
		return
	}
	dialog.ApplyTheme(m.theme)
	dialog.SetFrame(dialogRenderSize(*m, state))
	dialog.Sync(state, dialog.turnID)
}

func resolveTraceDialogTurnIDArg(state events.SessionState, turnArg string) (string, error) {
	turnIDs := orderedSessionTurnIDs(state)
	if len(turnIDs) == 0 {
		return "", fmt.Errorf("no turns are available yet")
	}
	turnArg = strings.TrimSpace(turnArg)
	if turnArg == "" {
		return "", nil
	}
	ordinal, err := strconv.Atoi(turnArg)
	if err != nil || ordinal < 1 || ordinal > len(turnIDs) {
		return "", fmt.Errorf("invalid turn number %q; valid range: 1-%d", turnArg, len(turnIDs))
	}
	return turnIDs[ordinal-1], nil
}

func resolveSessionTurnIDArg(m Model, state events.SessionState, turnArg string) (string, error) {
	turnIDs := orderedSessionTurnIDs(state)
	if len(turnIDs) == 0 {
		return "", fmt.Errorf("no turns are available yet")
	}
	turnArg = strings.TrimSpace(turnArg)
	if turnArg == "" {
		if turnID := strings.TrimSpace(effectiveDetailTurnID(m, state)); turnID != "" && state.Turns[turnID] != nil {
			return turnID, nil
		}
		if turnID := strings.TrimSpace(m.turnID); turnID != "" && state.Turns[turnID] != nil {
			return turnID, nil
		}
		return turnIDs[len(turnIDs)-1], nil
	}
	ordinal, err := strconv.Atoi(turnArg)
	if err != nil || ordinal < 1 || ordinal > len(turnIDs) {
		return "", fmt.Errorf("invalid turn number %q; valid range: 1-%d", turnArg, len(turnIDs))
	}
	return turnIDs[ordinal-1], nil
}

func traceDialogSubtitle(state events.SessionState, turnID string) string {
	if strings.TrimSpace(turnID) == "" {
		turnCount := len(orderedSessionTurnIDs(state))
		if turnCount <= 0 {
			return ""
		}
		return pluralize(turnCount, "turn")
	}
	turn := state.Turns[turnID]
	if turn == nil {
		return ""
	}
	parts := make([]string, 0, 4)
	if ordinal := sessionToolTurnOrdinal(state, turnID); ordinal > 0 {
		parts = append(parts, fmt.Sprintf("turn %d", ordinal))
	}
	parts = append(parts, costDialogTurnStatus(turn))
	if model := costDialogTurnModel(turn); model != "unknown" {
		parts = append(parts, model)
	}
	if turn.ProviderUsage != nil {
		switch {
		case costDialogPricingUnavailable(turn):
			parts = append(parts, "pricing unavailable")
		case turn.ProviderUsage.EstimatedInputCost+turn.ProviderUsage.EstimatedOutputCost > 0:
			parts = append(parts, formatEstimatedCost(turn.ProviderUsage.EstimatedInputCost+turn.ProviderUsage.EstimatedOutputCost))
		}
	}
	return strings.Join(parts, " | ")
}

func traceDialogBody(th *theme.Theme, state events.SessionState, turnID string) string {
	if strings.TrimSpace(turnID) == "" {
		return traceDialogSessionIndexBody(th, state)
	}
	turn := state.Turns[turnID]
	if turn == nil {
		return ""
	}
	sections := []string{
		traceDialogSummarySection(th, state, turnID, turn),
	}
	if section := traceDialogWorkflowSection(th, state, turn); section != "" {
		sections = append(sections, section)
	}
	if section := traceDialogProviderAttemptSection(th, turn); section != "" {
		sections = append(sections, section)
	}
	sections = append(sections, traceDialogPromptSection(th, turn))
	if section := traceDialogContextSection(th, turn); section != "" {
		sections = append(sections, section)
	}
	if section := traceDialogRetrySection(th, turn); section != "" {
		sections = append(sections, section)
	}
	if section := traceDialogToolSection(th, state, turn); section != "" {
		sections = append(sections, section)
	}
	return strings.Join(sections, "\n\n")
}
