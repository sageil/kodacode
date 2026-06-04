package tui

import (
	"hash/fnv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

type costDialog struct {
	id           string
	frameWidth   int
	frameHeight  int
	theme        *theme.Theme
	budgetStatus app.BudgetStatus
	usageSummary app.SessionUsageSummary
	body         Messages
}

type costDialogStats struct {
	TotalTurns                         int
	UsageTurns                         int
	MissingPricingTurns                int
	CompletedToolCalls                 int
	FailedToolCalls                    int
	ContractViolationCalls             int
	BatchedToolCallBatches             int
	BatchedToolCalls                   int
	EstimatedBatchProviderCallsAvoided int
	RequestTokens                      int
	CompletionTokens                   int
	CacheReadInputTokens               int
	CacheWriteInputTokens              int
	ReasoningTokens                    int
	UnpricedRequestTokens              int
	UnpricedCompletionTokens           int
	Steps                              int
	Attempts                           int
	EstimatedCost                      float64
	FullyReportedTurns                 int
	PartiallyReportedTurns             int
	CachePricingAppliedTurns           int
	CachePricingMissingTurns           int
	PromptCompactionTokensSaved        int
	HistoryCompactionTokensSaved       int
	CurrentTurnProjectionTokensSaved   int
	ToolDescriptionTokensSaved         int
	ToolSchemaTokensSaved              int
	DeterministicContextTokens         int
	DeterministicContextOmittedTokens  int
	EstimatedInputSavingsCost          float64
	EstimatedCacheSavingsCost          float64
	UtilityCompactionUsage             costDialogUsageKindTotals
	UtilityBranchSummaryUsage          costDialogUsageKindTotals
	HistoryCompactionSummaryUpdates    int
	HistoryCompactionPruningPasses     int
}

type costDialogUsageKindTotals struct {
	RequestTokens          int
	CompletionTokens       int
	Attempts               int
	EstimatedCost          float64
	MissingPricingAttempts int
}

func (d *costDialog) ignoreWheel(msg tea.MouseWheelMsg) bool {
	return shouldDropVerticalWheel(d.body, msg)
}

func (d *costDialog) wheelState() (int, bool) {
	return d.body.YOffset(), d.body.AtBottom()
}

type costDialogUsageCoverage int

const (
	costDialogUsageEstimated costDialogUsageCoverage = iota
	costDialogUsageMixed
	costDialogUsageReported
)

type costDialogTurn struct {
	TurnID                string
	Ordinal               int
	Turn                  *events.TurnState
	EstimatedCost         float64
	RequestTokens         int
	CompletionTokens      int
	CacheReadInputTokens  int
	CacheWriteInputTokens int
	ReasoningTokens       int
	TotalTokens           int
	PricingUnavailable    bool
	Coverage              costDialogUsageCoverage
	ReportedAttempts      int
	TotalAttempts         int
	Savings               costDialogSavings
}

func newCostDialog(m Model, state events.SessionState, budgetStatus app.BudgetStatus) *costDialog {
	dialog := &costDialog{
		id:           dialogIDCost,
		frameWidth:   104,
		frameHeight:  32,
		theme:        m.theme,
		budgetStatus: budgetStatus,
		usageSummary: m.footerStatus.sessionUsage,
		body:         NewMessagesWithTone(m.theme, "panel-alt"),
	}
	width, height := dialogRenderSize(m, state)
	dialog.SetFrame(width, height)
	dialog.Sync(state, budgetStatus, m.footerStatus.sessionUsage)
	return dialog
}

func (d *costDialog) ID() string { return d.id }

func (d *costDialog) ApplyTheme(th *theme.Theme) {
	d.theme = th
	d.body.ApplyTheme(th)
}

func (d *costDialog) SetFrame(width, height int) {
	d.frameWidth = width
	d.frameHeight = height
	dialogWidth := costDialogWidth(width)
	bodyWidth := max(dialogWidth-6, 1)
	bodyHeight := costDialogBodyHeight(height)
	if d.body.Width() == bodyWidth && d.body.Height() == bodyHeight {
		return
	}
	d.body.SetSize(bodyWidth, bodyHeight)
}

func (d *costDialog) Update(msg tea.Msg) (dialogModel, tea.Cmd) {
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

func (d *costDialog) Draw(surface dialogSurface, area dialogRenderArea) *tea.Cursor {
	d.SetFrame(area.width, area.height)
	width := costDialogWidth(d.frameWidth)
	content := renderStandaloneDialogContent(d.theme, max(width-dialogFrameInset*2, 1), dialogStandaloneFrame{
		Title: "Session Cost",
		Body:  d.body.View(),
		Hint:  "q close • ↑/↓ scroll • pgup/pgdn page",
	})
	return drawDialogFrameOnSurface(surface, area, d.theme, width, content, nil)
}

func (d *costDialog) Sync(state events.SessionState, budgetStatus app.BudgetStatus, usageSummary app.SessionUsageSummary) {
	d.budgetStatus = budgetStatus
	d.usageSummary = usageSummary
	wasEmpty := strings.TrimSpace(d.body.raw) == ""
	d.body.Sync(costDialogBody(d.theme, state, budgetStatus, usageSummary), false)
	if wasEmpty {
		d.body.GotoTop()
	}
}

func (d *costDialog) OverlayCacheKey() uint64 {
	hasher := fnv.New64a()
	appendMessagesRenderCacheSignature(hasher, d.body)
	return hasher.Sum64()
}

func costDialogWidth(frameWidth int) int {
	if frameWidth <= 0 {
		return 104
	}
	return min(max(frameWidth-8, 72), 124)
}

func costDialogBodyHeight(frameHeight int) int {
	if frameHeight <= 0 {
		return 18
	}
	outerHeight := min(max(frameHeight-6, 14), 38)
	return max(outerHeight-7, 6)
}

func (m *Model) openCostDialog() tea.Cmd {
	state := m.projector.Snapshot()
	return func() tea.Msg {
		budgetStatus, err := m.backend.BudgetStatus(m.ctx, m.sessionID)
		if err != nil {
			budgetStatus = m.footerStatus.budget
		}
		usageSummary, err := m.backend.SessionUsageSummary(m.ctx, m.sessionID)
		if err != nil {
			usageSummary = m.footerStatus.sessionUsage
		}
		dialogModel := *m
		dialogModel.footerStatus.sessionUsage = usageSummary
		return dialogOpenedMsg{dialog: newCostDialog(dialogModel, state, budgetStatus)}
	}
}

func (m *Model) syncCostDialog() {
	if m == nil {
		return
	}
	dialog, ok := m.dialog.(*costDialog)
	if !ok {
		return
	}
	dialog.ApplyTheme(m.theme)
	dialog.SetFrame(dialogRenderSize(*m, m.projector.Snapshot()))
	dialog.Sync(m.projector.Snapshot(), m.footerStatus.budget, m.footerStatus.sessionUsage)
}

func costDialogBody(th *theme.Theme, state events.SessionState, budgetStatus app.BudgetStatus, usageSummary app.SessionUsageSummary) string {
	stats, pricedTurns, unpricedTurns := costDialogUsage(state)
	aggregateUsage := usageSummary.ValidFor(state.SessionID) && usageSummary.HasUsage()
	sections := []string{costDialogSummarySection(th, state, stats, pricedTurns, unpricedTurns, budgetStatus, usageSummary)}
	if stats.UsageTurns == 0 && !aggregateUsage {
		return strings.Join(sections, "\n\n")
	}
	if len(pricedTurns) > 0 {
		title := "Priced Turns by Estimated Cost"
		sections = append(sections, dialogSectionStyle(th).Render(title))
		for _, entry := range pricedTurns {
			sections = append(sections, costDialogTurnSection(th, entry))
		}
	}
	if len(unpricedTurns) > 0 {
		title := "Turns Without Pricing by Token Load"
		sections = append(sections, dialogSectionStyle(th).Render(title))
	}
	for _, entry := range unpricedTurns {
		sections = append(sections, costDialogTurnSection(th, entry))
	}
	return strings.Join(sections, "\n\n")
}
