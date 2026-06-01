package provider

import (
	"encoding/json"
	"io"
	"strings"
)

func readOpenAIError(body io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(body, 32*1024))
	if err != nil {
		return "", err
	}
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &payload); err == nil && strings.TrimSpace(payload.Error.Message) != "" {
		return payload.Error.Message, nil
	}
	if trimmed := strings.TrimSpace(string(data)); trimmed != "" {
		return trimmed, nil
	}
	return "unexpected error response", nil
}
