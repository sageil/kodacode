package tui

import (
	"strings"
)

type scopedToolCallKey struct {
	SessionID string
	TurnID    string
	CallID    string
}

type inspectorToolTarget struct {
	SessionID string
	Ref       sessionToolCallRef
}

func scopedToolKey(sessionID string, ref sessionToolCallRef) scopedToolCallKey {
	return scopedToolCallKey{
		SessionID: strings.TrimSpace(sessionID),
		TurnID:    strings.TrimSpace(ref.TurnID),
		CallID:    strings.TrimSpace(ref.CallID),
	}
}

func toolRefForSession(sessionID string, ref sessionToolCallRef) sessionToolCallRef {
	ref.SessionID = strings.TrimSpace(sessionID)
	return ref
}

func normalizeToolTargetSessionID(currentSessionID, sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID != "" {
		return sessionID
	}
	return strings.TrimSpace(currentSessionID)
}

func selectedToolSessionID(m Model) string {
	if sessionID := strings.TrimSpace(m.selection.callSessionID); sessionID != "" {
		return normalizeToolTargetSessionID(m.sessionID, sessionID)
	}
	return strings.TrimSpace(m.sessionID)
}

func selectedToolMatchesSession(m Model, sessionID string, ref sessionToolCallRef) bool {
	return normalizeToolTargetSessionID(m.sessionID, sessionID) == selectedToolSessionID(m) &&
		strings.TrimSpace(m.selection.callTurnID) == strings.TrimSpace(ref.TurnID) &&
		strings.TrimSpace(m.selection.callID) == strings.TrimSpace(ref.CallID)
}

func expandedToolSessionID(m Model) string {
	if sessionID := strings.TrimSpace(m.selection.expandedCallSessionID); sessionID != "" {
		return normalizeToolTargetSessionID(m.sessionID, sessionID)
	}
	return strings.TrimSpace(m.sessionID)
}

func expandedToolMatchesSession(m Model, sessionID string, ref sessionToolCallRef) bool {
	return normalizeToolTargetSessionID(m.sessionID, sessionID) == expandedToolSessionID(m) &&
		strings.TrimSpace(m.selection.expandedCallTurnID) == strings.TrimSpace(ref.TurnID) &&
		strings.TrimSpace(m.selection.expandedCallID) == strings.TrimSpace(ref.CallID)
}
