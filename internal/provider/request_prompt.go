package provider

import "strings"

// JoinPromptSections renders the provider-facing prompt text from stable and
// dynamic prompt sections. When split sections are present, they are the
// canonical prompt authority.
func JoinPromptSections(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, "\n\n")
}

// PromptText returns the canonical prompt text for a request. Split prompt
// sections win when present so every provider sees the same effective prompt.
func PromptText(req Request) string {
	if strings.TrimSpace(req.CacheablePrefix) != "" || strings.TrimSpace(req.DynamicSuffix) != "" {
		return JoinPromptSections(req.CacheablePrefix, req.DynamicSuffix)
	}
	return strings.TrimSpace(req.Instructions)
}

// NormalizePromptRequest rewrites request prompt fields into their canonical
// form so Instructions always matches the effective provider prompt text.
func NormalizePromptRequest(req Request) Request {
	req.CacheablePrefix = strings.TrimSpace(req.CacheablePrefix)
	req.DynamicSuffix = strings.TrimSpace(req.DynamicSuffix)
	req.Instructions = PromptText(req)
	return req
}

// PreparePromptRequest normalizes the request and applies any provider/model-
// specific prompt supplement so callers can reason about the same provider-
// facing prompt shape used during execution.
func PreparePromptRequest(req Request) Request {
	req = NormalizePromptRequest(req)
	if req.CacheablePrefix == "" && req.DynamicSuffix == "" {
		req.Instructions = ComposeInstructions(req.Instructions, req.Model.ProviderID, req.Model.ModelID)
		return NormalizePromptRequest(req)
	}
	req.CacheablePrefix = ComposeInstructions(req.CacheablePrefix, req.Model.ProviderID, req.Model.ModelID)
	return NormalizePromptRequest(req)
}
