package app

import (
	"context"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
)

func containsAll(value string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}

func appendCompactedSessionSnapshotForTest(t *testing.T, store events.Appender, sessions *SessionService, sessionID string) {
	t.Helper()

	state, err := sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot(%s) error = %v", sessionID, err)
	}
	if _, err := store.Append(context.Background(), events.Draft{
		SessionID: sessionID,
		TurnID:    sessionTurnID,
		Type:      events.TypeSessionStateSnapshot,
		Payload: events.SessionStateSnapshotPayload{
			BaseSequence: state.LastSequence,
			State:        events.SnapshotSessionState(state),
		},
	}); err != nil {
		t.Fatalf("Append(session_state_snapshot %s) error = %v", sessionID, err)
	}
}
