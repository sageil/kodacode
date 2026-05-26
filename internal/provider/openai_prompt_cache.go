package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

const (
	OpenAIPromptCacheRetentionInMemory = "in_memory"
	OpenAIPromptCacheRetention24h      = "24h"
)

var ErrOpenAIPromptCacheRetentionInvalid = errors.New("openai prompt_cache_retention must be empty, in_memory, or 24h")

func ValidateOpenAIPromptCacheRetention(value string) error {
	switch strings.TrimSpace(value) {
	case "", OpenAIPromptCacheRetentionInMemory, OpenAIPromptCacheRetention24h:
		return nil
	default:
		return ErrOpenAIPromptCacheRetentionInvalid
	}
}

func normalizeOpenAIPromptCacheRetention(value string) string {
	value = strings.TrimSpace(value)
	if err := ValidateOpenAIPromptCacheRetention(value); err != nil {
		return ""
	}
	return value
}

func openAIPromptCacheKey(req Request) string {
	if CanonicalProviderID(req.Model.ProviderID) != "openai" {
		return ""
	}
	req = NormalizePromptRequest(req)
	staticPrompt := firstNonBlank(strings.TrimSpace(req.CacheablePrefix), strings.TrimSpace(req.Instructions))
	if staticPrompt == "" && len(req.Tools) == 0 {
		return ""
	}
	sum := sha256.New()
	writeCacheKeyPart(sum, req.Model.ModelID)
	writeCacheKeyPart(sum, req.AgentID)
	writeCacheKeyPart(sum, staticPrompt)
	for _, tool := range req.Tools {
		writeCacheKeyPart(sum, tool.Name)
		writeCacheKeyPart(sum, tool.Description)
		writeCacheKeyPart(sum, tool.InputSchema)
	}
	encoded := hex.EncodeToString(sum.Sum(nil)[:12])
	return "kodacode-" + encoded
}

type cacheKeyWriter interface {
	Write([]byte) (int, error)
}

func writeCacheKeyPart(w cacheKeyWriter, value string) {
	value = strings.TrimSpace(value)
	_, _ = w.Write([]byte(value))
	_, _ = w.Write([]byte{0})
}
