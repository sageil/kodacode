package openaicache

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

const (
	RetentionInMemory = "in_memory"
	Retention24h      = "24h"
)

var ErrRetentionInvalid = errors.New("openai prompt_cache_retention must be empty, in_memory, or 24h")

type KeyInput struct {
	ModelID      string
	AgentID      string
	StaticPrompt string
	Tools        []Tool
}

type Tool struct {
	Name        string
	Description string
	InputSchema string
}

func ValidateRetention(value string) error {
	switch strings.TrimSpace(value) {
	case "", RetentionInMemory, Retention24h:
		return nil
	default:
		return ErrRetentionInvalid
	}
}

func NormalizeRetention(value string) string {
	value = strings.TrimSpace(value)
	if err := ValidateRetention(value); err != nil {
		return ""
	}
	return value
}

func Key(input KeyInput) string {
	staticPrompt := strings.TrimSpace(input.StaticPrompt)
	if staticPrompt == "" && len(input.Tools) == 0 {
		return ""
	}
	sum := sha256.New()
	writeKeyPart(sum, input.ModelID)
	writeKeyPart(sum, input.AgentID)
	writeKeyPart(sum, staticPrompt)
	for _, tool := range input.Tools {
		writeKeyPart(sum, tool.Name)
		writeKeyPart(sum, tool.Description)
		writeKeyPart(sum, tool.InputSchema)
	}
	encoded := hex.EncodeToString(sum.Sum(nil)[:12])
	return "kodacode-" + encoded
}

type keyWriter interface {
	Write([]byte) (int, error)
}

func writeKeyPart(w keyWriter, value string) {
	value = strings.TrimSpace(value)
	_, _ = w.Write([]byte(value))
	_, _ = w.Write([]byte{0})
}
