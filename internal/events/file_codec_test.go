package events

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestEncodeEventCompressesSessionSnapshotPayloadWhenBeneficial(t *testing.T) {
	event := Event{
		ID:        "session-1:64",
		SessionID: "session-1",
		TurnID:    "_session",
		Sequence:  64,
		Time:      time.Now().UTC(),
		Type:      TypeSessionStateSnapshot,
		Payload: SessionStateSnapshotPayload{
			BaseSequence: 64,
			State: SessionState{
				SessionID:    "session-1",
				LastSequence: 64,
				TurnOrder:    []string{"turn-1"},
				Turns: map[string]*TurnState{
					"turn-1": {
						TurnID:        "turn-1",
						Status:        TurnStatusCompleted,
						AssistantText: string(bytes.Repeat([]byte("provider-attempt-history "), 256)),
					},
				},
			},
		},
	}

	encoded, err := encodeEvent(event)
	if err != nil {
		t.Fatalf("encodeEvent() error = %v", err)
	}
	if !bytes.Contains(encoded, []byte(`"encoding":"gzip+base64"`)) {
		t.Fatalf("encoded snapshot payload was not compressed:\n%s", encoded)
	}

	decoded, err := decodeEvent(encoded)
	if err != nil {
		t.Fatalf("decodeEvent() error = %v", err)
	}
	payload, ok := decoded.Payload.(SessionStateSnapshotPayload)
	if !ok {
		t.Fatalf("decoded payload = %T", decoded.Payload)
	}
	got := payload.State.Turns["turn-1"].AssistantText
	want := event.Payload.(SessionStateSnapshotPayload).State.Turns["turn-1"].AssistantText
	if got != want {
		t.Fatalf("AssistantText mismatch after round trip")
	}
}

func TestDecodeEventAcceptsPlainSessionSnapshotPayload(t *testing.T) {
	plainPayload, err := json.Marshal(SessionStateSnapshotPayload{
		BaseSequence: 3,
		State: SessionState{
			SessionID:    "session-1",
			LastSequence: 3,
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	record, err := json.Marshal(eventRecord{
		ID:        "session-1:3",
		SessionID: "session-1",
		TurnID:    "_session",
		Sequence:  3,
		Time:      time.Now().UTC().Format(jsonTimeLayout),
		Type:      TypeSessionStateSnapshot,
		Payload:   plainPayload,
	})
	if err != nil {
		t.Fatalf("json.Marshal(record) error = %v", err)
	}

	decoded, err := decodeEvent(record)
	if err != nil {
		t.Fatalf("decodeEvent() error = %v", err)
	}
	payload, ok := decoded.Payload.(SessionStateSnapshotPayload)
	if !ok {
		t.Fatalf("decoded payload = %T", decoded.Payload)
	}
	if payload.BaseSequence != 3 || payload.State.SessionID != "session-1" {
		t.Fatalf("decoded payload = %#v", payload)
	}
}
