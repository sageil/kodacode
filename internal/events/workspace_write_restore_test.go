package events

import (
	"testing"
)

func TestWorkspaceWriteRestoredPayloadRoundTripsThroughCodec(t *testing.T) {
	event := testEvent(7, "session-1", "_session", WorkspaceWriteRestoredPayload{
		SourceTurnID: "turn-4",
		Restores: []WorkspaceWriteRestoreItem{{
			CallID:        "call-2",
			Path:          "/repo/notes.txt",
			ExistedBefore: true,
		}},
	})

	encoded, err := encodeEvent(event)
	if err != nil {
		t.Fatalf("encodeEvent() error = %v", err)
	}
	decoded, err := decodeEvent(encoded)
	if err != nil {
		t.Fatalf("decodeEvent() error = %v", err)
	}
	payload, ok := decoded.Payload.(WorkspaceWriteRestoredPayload)
	if !ok {
		t.Fatalf("decoded payload = %T", decoded.Payload)
	}
	if payload.SourceTurnID != "turn-4" {
		t.Fatalf("source turn id = %q", payload.SourceTurnID)
	}
	if len(payload.Restores) != 1 || payload.Restores[0].Path != "/repo/notes.txt" || !payload.Restores[0].ExistedBefore {
		t.Fatalf("restores = %#v", payload.Restores)
	}
}

func TestProjectorAcceptsWorkspaceWriteRestoredPayload(t *testing.T) {
	projector := NewProjector("session-1")
	if err := projector.Apply(testEvent(0, "session-1", "_session", WorkspaceWriteRestoredPayload{
		SourceTurnID: "turn-2",
		Restores: []WorkspaceWriteRestoreItem{{
			CallID: "call-1",
			Path:   "/repo/file.txt",
		}},
	})); err != nil {
		t.Fatalf("Apply(workspace_write_restored) error = %v", err)
	}

	state := projector.Snapshot()
	if state.LastSequence != 0 {
		t.Fatalf("last sequence = %d", state.LastSequence)
	}
}
