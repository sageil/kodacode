package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/events"
)

const wheelInputCoalesceWindow = 32 * time.Millisecond
const wheelInputStopAfterClickWindow = 400 * time.Millisecond
const wheelInputMaxCoalescedSteps = 6

type coalescedWheelMsg struct {
	msg   tea.MouseWheelMsg
	steps int
}

type inputMouseRect struct {
	x      int
	y      int
	width  int
	height int
}

func (r inputMouseRect) contains(x, y int) bool {
	if r.width <= 0 || r.height <= 0 {
		return false
	}
	return x >= r.x && x < r.x+r.width && y >= r.y && y < r.y+r.height
}

type inputMouseRegions struct {
	transcript inputMouseRect
	inspector  inputMouseRect
	sessions   inputMouseRect
}

type wheelInputLimiter struct {
	lastAt     time.Time
	lastTarget mouseWheelTarget
	lastDialog string
	lastButton tea.MouseButton
	pending    int
	stopUntil  time.Time
	stopTarget mouseWheelTarget
	stopDialog string
}

func newInputFilter() func(tea.Model, tea.Msg) tea.Msg {
	limiter := &wheelInputLimiter{}
	return func(model tea.Model, msg tea.Msg) tea.Msg {
		return inputFilterWithWheelLimiter(model, msg, limiter, time.Now())
	}
}

func inputFilter(model tea.Model, msg tea.Msg) tea.Msg {
	return inputFilterWithWheelLimiter(model, msg, nil, time.Now())
}

func inputFilterWithWheelLimiter(model tea.Model, msg tea.Msg, limiter *wheelInputLimiter, now time.Time) tea.Msg {
	m, ok := model.(Model)
	if !ok {
		return msg
	}

	switch typed := msg.(type) {
	case tea.MouseClickMsg:
		if limiter != nil {
			limiter.stopWheelAfterClick(m, typed, now)
		}
		return msg
	case tea.MouseMotionMsg:
		if m.transcriptView.mouseSelecting {
			return typed
		}
		return nil
	case tea.MouseWheelMsg:
		if limiter != nil {
			filtered, reason := limiter.filter(m, typed, now)
			if filtered == nil {
				m.traceWheelFilterDecision(reason, typed)
				return nil
			}
			m.traceWheelFilterDecision("keep", typed)
			return filtered
		}
		if m.shouldDropWheel(typed) {
			m.traceWheelFilterDecision("drop:boundary", typed)
			return nil
		}
		m.traceWheelFilterDecision("keep", typed)
	}
	return msg
}

func (l *wheelInputLimiter) shouldDrop(m Model, msg tea.MouseWheelMsg, now time.Time) (bool, string) {
	filtered, reason := l.filter(m, msg, now)
	if filtered != nil {
		return false, "keep"
	}
	return filtered == nil, reason
}

func (l *wheelInputLimiter) filter(m Model, msg tea.MouseWheelMsg, now time.Time) (tea.Msg, string) {
	if l == nil {
		if m.shouldDropWheel(msg) {
			return nil, "drop:boundary"
		}
		return msg, "keep"
	}
	if m.shouldDropWheel(msg) {
		l.clearWheelBurstState()
		return nil, "drop:boundary"
	}
	button := msg.Mouse().Button
	if button != tea.MouseWheelUp && button != tea.MouseWheelDown {
		l.clearWheelBurstState()
		return msg, "keep"
	}
	target, dialogID := wheelLimiterTarget(m, msg)
	if l.shouldStopWheelAfterClick(target, dialogID, now) {
		return nil, "drop:click-stop"
	}
	sameBurst := !l.lastAt.IsZero() &&
		target == l.lastTarget &&
		dialogID == l.lastDialog &&
		button == l.lastButton
	if sameBurst && now.Sub(l.lastAt) < wheelInputCoalesceWindow {
		l.pending = min(l.pending+1, wheelInputMaxCoalescedSteps-1)
		return nil, "drop:coalesce"
	}
	steps := 1
	if sameBurst && l.pending > 0 {
		steps = min(1+l.pending, wheelInputMaxCoalescedSteps)
	}
	l.pending = 0
	l.lastAt = now
	l.lastTarget = target
	l.lastDialog = dialogID
	l.lastButton = button
	if steps > 1 {
		return coalescedWheelMsg{msg: msg, steps: steps}, "keep:coalesced"
	}
	return msg, "keep"
}

func (l *wheelInputLimiter) stopWheelAfterClick(m Model, msg tea.MouseClickMsg, now time.Time) {
	if l == nil || msg.Mouse().Button != tea.MouseLeft {
		return
	}
	if m.dialog != nil {
		l.stopTarget = mouseWheelTargetNone
		l.stopDialog = m.dialog.ID()
		l.stopUntil = now.Add(wheelInputStopAfterClickWindow)
		l.clearWheelBurstState()
		return
	}
	mouse := msg.Mouse()
	target := m.mouseTargetFromRegions(mouse.X, mouse.Y)
	switch target {
	case mouseWheelTargetTranscript, mouseWheelTargetInspector, mouseWheelTargetSessions:
		l.stopTarget = target
		l.stopDialog = ""
		l.stopUntil = now.Add(wheelInputStopAfterClickWindow)
		l.clearWheelBurstState()
	default:
		l.clearWheelStopState()
	}
}

func (l *wheelInputLimiter) shouldStopWheelAfterClick(target mouseWheelTarget, dialogID string, now time.Time) bool {
	if l == nil || l.stopUntil.IsZero() {
		return false
	}
	if !now.Before(l.stopUntil) {
		l.clearWheelStopState()
		return false
	}
	return target == l.stopTarget && dialogID == l.stopDialog
}

func (l *wheelInputLimiter) clearWheelBurstState() {
	if l == nil {
		return
	}
	l.lastAt = time.Time{}
	l.lastTarget = mouseWheelTargetNone
	l.lastDialog = ""
	l.lastButton = 0
	l.pending = 0
}

func (l *wheelInputLimiter) clearWheelStopState() {
	if l == nil {
		return
	}
	l.stopUntil = time.Time{}
	l.stopTarget = mouseWheelTargetNone
	l.stopDialog = ""
}

func wheelLimiterTarget(m Model, msg tea.MouseWheelMsg) (mouseWheelTarget, string) {
	if m.dialog != nil {
		return mouseWheelTargetNone, m.dialog.ID()
	}
	mouse := msg.Mouse()
	return m.mouseTargetFromRegions(mouse.X, mouse.Y), ""
}

func (m Model) shouldDropWheel(msg tea.MouseWheelMsg) bool {
	if isHorizontalWheel(msg) {
		return true
	}

	if m.dialog != nil {
		dialog, ok := m.dialog.(wheelBoundaryDialog)
		if !ok {
			return false
		}
		return dialog.ignoreWheel(msg)
	}

	switch m.mouseTargetFromRegions(msg.Mouse().X, msg.Mouse().Y) {
	case mouseWheelTargetTranscript:
		return shouldDropVerticalWheel(m.messages, msg)
	case mouseWheelTargetInspector:
		return shouldDropVerticalWheel(m.inspector.body, msg)
	case mouseWheelTargetSessions:
		return shouldDropVerticalWheel(m.sessionsBody, msg)
	default:
		return false
	}
}

func isHorizontalWheel(msg tea.MouseWheelMsg) bool {
	switch msg.Mouse().Button {
	case tea.MouseWheelLeft, tea.MouseWheelRight:
		return true
	default:
		return false
	}
}

func shouldDropVerticalWheel(msgs Messages, msg tea.MouseWheelMsg) bool {
	switch msg.Mouse().Button {
	case tea.MouseWheelUp:
		return msgs.YOffset() == 0
	case tea.MouseWheelDown:
		return msgs.AtBottom()
	default:
		return false
	}
}

func (m Model) traceWheelFilterDecision(decision string, msg tea.MouseWheelMsg) {
	if m.inputTrace == nil {
		return
	}
	mouse := msg.Mouse()
	if m.dialog != nil {
		if dialog, ok := m.dialog.(wheelBoundaryDialog); ok {
			offset, atBottom := dialog.wheelState()
			m.inputTrace.logf(
				"filter=%s button=%s x=%d y=%d target=dialog offset=%d bottom=%t dialog=%s",
				decision,
				mouse.Button,
				mouse.X,
				mouse.Y,
				offset,
				atBottom,
				m.dialog.ID(),
			)
			return
		}
		m.inputTrace.logf(
			"filter=%s button=%s x=%d y=%d target=dialog dialog=%s state=unknown",
			decision,
			mouse.Button,
			mouse.X,
			mouse.Y,
			m.dialog.ID(),
		)
		return
	}

	target := m.mouseTargetFromRegions(mouse.X, mouse.Y)
	offset, atBottom, ok := m.wheelTargetState(target)
	if ok {
		m.inputTrace.logf(
			"filter=%s button=%s x=%d y=%d target=%s offset=%d bottom=%t focus=%s",
			decision,
			mouse.Button,
			mouse.X,
			mouse.Y,
			target,
			offset,
			atBottom,
			m.chrome.focus,
		)
		return
	}
	m.inputTrace.logf(
		"filter=%s button=%s x=%d y=%d target=%s focus=%s",
		decision,
		mouse.Button,
		mouse.X,
		mouse.Y,
		target,
		m.chrome.focus,
	)
}

func (m Model) wheelTargetState(target mouseWheelTarget) (int, bool, bool) {
	switch target {
	case mouseWheelTargetTranscript:
		return m.messages.YOffset(), m.messages.AtBottom(), true
	case mouseWheelTargetInspector:
		return m.inspector.body.YOffset(), m.inspector.body.AtBottom(), true
	case mouseWheelTargetSessions:
		return m.sessionsBody.YOffset(), m.sessionsBody.AtBottom(), true
	default:
		return 0, false, false
	}
}

func (m Model) mouseTargetFromRegions(mouseX, mouseY int) mouseWheelTarget {
	switch {
	case m.mouseRegions.sessions.contains(mouseX, mouseY):
		return mouseWheelTargetSessions
	case m.mouseRegions.transcript.contains(mouseX, mouseY):
		return mouseWheelTargetTranscript
	case m.mouseRegions.inspector.contains(mouseX, mouseY):
		return mouseWheelTargetInspector
	default:
		return mouseWheelTargetNone
	}
}

func (m *Model) syncMouseRegions(state events.SessionState, layout shellLayout) {
	if m == nil || m.width <= 0 || m.height <= 0 {
		return
	}
	rects := resolveShellRects(*m, state, layout)
	m.mouseRegions = inputMouseRegions{
		transcript: rects.transcript,
		inspector:  rects.inspector,
		sessions:   rects.sessions,
	}
}

func (t mouseWheelTarget) String() string {
	switch t {
	case mouseWheelTargetTranscript:
		return "transcript"
	case mouseWheelTargetInspector:
		return "inspector"
	case mouseWheelTargetSessions:
		return "sessions"
	case mouseWheelTargetNone:
		return "none"
	default:
		return fmt.Sprintf("target(%d)", int(t))
	}
}
