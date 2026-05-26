package tool

import (
	"bytes"
	"encoding/json"
	"errors"
)

var ErrStructuredResultInvalid = errors.New("structured result must be valid JSON")

func MarshalStructuredResult(value any) (json.RawMessage, error) {
	if value == nil {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	if !json.Valid(trimmed) {
		return nil, ErrStructuredResultInvalid
	}
	return json.RawMessage(trimmed), nil
}
