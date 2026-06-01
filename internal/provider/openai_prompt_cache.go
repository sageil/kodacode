package provider

import (
	"strings"

	"github.com/sageil/kodacode/internal/provider/openaicache"
)

const (
	OpenAIPromptCacheRetentionInMemory = openaicache.RetentionInMemory
	OpenAIPromptCacheRetention24h      = openaicache.Retention24h
)

var ErrOpenAIPromptCacheRetentionInvalid = openaicache.ErrRetentionInvalid

func ValidateOpenAIPromptCacheRetention(value string) error {
	return openaicache.ValidateRetention(value)
}

func normalizeOpenAIPromptCacheRetention(value string) string {
	return openaicache.NormalizeRetention(value)
}

func openAIPromptCacheKey(req Request) string {
	if CanonicalProviderID(req.Model.ProviderID) != "openai" {
		return ""
	}
	req = NormalizePromptRequest(req)
	staticPrompt := firstNonBlank(strings.TrimSpace(req.CacheablePrefix), strings.TrimSpace(req.Instructions))
	tools := make([]openaicache.Tool, 0, len(req.Tools))
	for _, tool := range req.Tools {
		tools = append(tools, openaicache.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}
	return openaicache.Key(openaicache.KeyInput{
		ModelID:      req.Model.ModelID,
		AgentID:      req.AgentID,
		StaticPrompt: staticPrompt,
		Tools:        tools,
	})
}
