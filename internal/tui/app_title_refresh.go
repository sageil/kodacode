package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

var (
	sessionTitleRefreshDelay       = time.Second
	sessionTitleRefreshMaxAttempts = 45
)

type sessionTitleRefreshMsg struct {
	sessionID string
	title     string
	attempt   int
	err       error
}

func (a App) refreshSessionTitleTick(sessionID string, attempt int) tea.Cmd {
	if sessionID == "" {
		return nil
	}
	return tea.Tick(sessionTitleRefreshDelay, func(time.Time) tea.Msg {
		sess, err := a.api.GetSession(a.ctx, sessionID)
		if err != nil {
			return sessionTitleRefreshMsg{
				sessionID: sessionID,
				attempt:   attempt,
				err:       err,
			}
		}
		return sessionTitleRefreshMsg{
			sessionID: sessionID,
			title:     sess.Title,
			attempt:   attempt,
		}
	})
}

func (a App) handleSessionTitleRefresh(msg sessionTitleRefreshMsg) (tea.Model, tea.Cmd) {
	if msg.sessionID == "" || msg.sessionID != a.sessionID {
		return a, nil
	}
	if msg.title != "" {
		a.session.SetTitle(msg.title)
		return a, nil
	}
	if a.session.header.title != "" || msg.attempt+1 >= sessionTitleRefreshMaxAttempts {
		return a, nil
	}
	return a, a.refreshSessionTitleTick(msg.sessionID, msg.attempt+1)
}
