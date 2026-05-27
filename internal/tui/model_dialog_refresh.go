package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

const dialogRefreshThrottle = 66 * time.Millisecond

func dialogRefreshTickCmd(delay time.Duration) tea.Cmd {
	if delay < 0 {
		delay = 0
	}
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return dialogRefreshTickMsg{}
	})
}

func (m *Model) clearDialogRefreshQueue() {
	if m == nil {
		return
	}
	m.dialogRefresh.id = ""
	m.dialogRefresh.deferred = false
	m.dialogRefresh.pending = false
	m.dialogRefresh.ticking = false
}

func (m *Model) resetDialogRefreshState() {
	if m == nil {
		return
	}
	m.clearDialogRefreshQueue()
	m.dialogRefresh.lastAt = time.Time{}
}

func (m Model) dialogRefreshDelay(now time.Time) time.Duration {
	if m.dialogRefresh.lastAt.IsZero() {
		return 0
	}
	elapsed := now.Sub(m.dialogRefresh.lastAt)
	if elapsed >= dialogRefreshThrottle {
		return 0
	}
	return dialogRefreshThrottle - elapsed
}

func (m *Model) queueDialogRefresh(dialogID string) {
	if m == nil {
		return
	}
	dialogID = strings.TrimSpace(dialogID)
	if dialogID == "" {
		return
	}
	m.dialogRefresh.id = dialogID
	m.dialogRefresh.pending = true
}

func (m Model) liveDialogWheelState(dialogID string) (int, bool, bool) {
	if m.dialog == nil || strings.TrimSpace(m.dialog.ID()) != strings.TrimSpace(dialogID) {
		return 0, false, false
	}
	dialog, ok := m.dialog.(wheelBoundaryDialog)
	if !ok {
		return 0, false, false
	}
	offset, atBottom := dialog.wheelState()
	return offset, atBottom, true
}

func (m Model) shouldDeferLiveDialogRefresh(dialogID string) bool {
	if !m.busy {
		return false
	}
	offset, atBottom, ok := m.liveDialogWheelState(dialogID)
	if !ok {
		return false
	}
	return !atBottom && offset > 0
}

func (m *Model) scheduleDialogRefresh(now time.Time, delay time.Duration) tea.Cmd {
	if m == nil {
		return nil
	}
	m.dialogRefresh.pending = true
	if m.dialogRefresh.ticking {
		return nil
	}
	m.dialogRefresh.ticking = true
	return dialogRefreshTickCmd(delay)
}

func (m *Model) syncDialogByID(dialogID string, now time.Time) bool {
	if m == nil {
		return false
	}
	dialogID = strings.TrimSpace(dialogID)
	if dialogID == "" || m.dialog == nil || strings.TrimSpace(m.dialog.ID()) != dialogID {
		m.clearDialogRefreshQueue()
		return false
	}
	switch dialogID {
	case dialogIDCost:
		m.syncCostDialog()
	case dialogIDTrace:
		m.syncTraceDialog()
	case dialogIDShellTools:
		m.syncShellToolsDialog()
	case dialogIDToolDetail:
		m.syncToolDetailDialog()
	case dialogIDHandoffDetail:
		m.syncHandoffDetailDialog()
	case dialogIDTaskDetail:
		m.syncTaskDetailDialog()
	default:
		m.clearDialogRefreshQueue()
		return false
	}
	m.clearDialogRefreshQueue()
	m.dialogRefresh.lastAt = now
	return true
}

func (m *Model) requestDialogRefresh(dialogID string) tea.Cmd {
	if m == nil {
		return nil
	}
	dialogID = strings.TrimSpace(dialogID)
	if dialogID == "" || m.dialog == nil || strings.TrimSpace(m.dialog.ID()) != dialogID {
		return nil
	}
	if m.shouldDeferLiveDialogRefresh(dialogID) {
		m.queueDialogRefresh(dialogID)
		m.dialogRefresh.deferred = true
		return nil
	}
	m.dialogRefresh.deferred = false
	now := time.Now()
	if m.busy {
		if delay := m.dialogRefreshDelay(now); delay > 0 {
			m.queueDialogRefresh(dialogID)
			return m.scheduleDialogRefresh(now, delay)
		}
	}
	m.syncDialogByID(dialogID, now)
	return nil
}

func (m *Model) flushPendingDialogRefresh(now time.Time) tea.Cmd {
	if m == nil || !m.dialogRefresh.pending {
		return nil
	}
	if m.dialog == nil || strings.TrimSpace(m.dialog.ID()) != strings.TrimSpace(m.dialogRefresh.id) {
		m.clearDialogRefreshQueue()
		return nil
	}
	if _, atBottom, ok := m.liveDialogWheelState(m.dialogRefresh.id); ok && !atBottom {
		if m.dialogRefresh.deferred {
			return nil
		}
		if m.shouldDeferLiveDialogRefresh(m.dialogRefresh.id) {
			m.dialogRefresh.deferred = true
			return nil
		}
	}
	m.dialogRefresh.deferred = false
	if m.busy {
		if delay := m.dialogRefreshDelay(now); delay > 0 {
			return m.scheduleDialogRefresh(now, delay)
		}
	}
	m.syncDialogByID(m.dialogRefresh.id, now)
	return nil
}

func (m *Model) syncDeferredDialogIfNeeded() tea.Cmd {
	if m == nil || !m.dialogRefresh.deferred {
		return nil
	}
	return m.flushPendingDialogRefresh(time.Now())
}
