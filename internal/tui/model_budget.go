package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
)

func loadBudgetStatusCmd(ctx context.Context, controller controller, sessionID string) tea.Cmd {
	return func() tea.Msg {
		status, err := controller.BudgetStatus(ctx, sessionID)
		return budgetStatusLoadedMsg{
			sessionID: sessionID,
			status:    status,
			err:       err,
		}
	}
}

func loadSessionUsageSummaryCmd(ctx context.Context, controller controller, sessionID string) tea.Cmd {
	return func() tea.Msg {
		summary, err := controller.SessionUsageSummary(ctx, sessionID)
		return sessionUsageSummaryLoadedMsg{
			sessionID: sessionID,
			summary:   summary,
			err:       err,
		}
	}
}
