package app

import (
	"bytes"
	"encoding/json"
)

func cloneStructuredResult(raw json.RawMessage) json.RawMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	out := make(json.RawMessage, len(trimmed))
	copy(out, trimmed)
	return out
}
