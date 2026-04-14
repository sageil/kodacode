package agent

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/v1/internal/config"
)

// ProviderPrompt returns a system prompt tailored for the provider/model combination.
//
// Resolution order:
//  1. Per-model: ~/.config/kodacode/prompts/{providerID}/{modelID}.md
//  2. Per-provider: ~/.config/kodacode/prompts/{providerID}.md
//  3. Canonical override: ~/.config/kodacode/prompts/anthropic.md, openai.md, etc.
//  4. Built-in embedded prompt
//
// Examples:
//
//	~/.config/kodacode/prompts/lmstudio/qwen3-coder-30b.md  → specific model on LM Studio
//	~/.config/kodacode/prompts/lmstudio.md                  → all LM Studio models
//	~/.config/kodacode/prompts/anthropic.md                 → overrides built-in Claude prompt
//	~/.config/kodacode/prompts/default.md                   → overrides the fallback prompt
func ProviderPrompt(providerID, modelID string) string {
	// 1. Per-model override: prompts/{providerID}/{modelID}.md
	if modelID != "" {
		if content := loadUserPrompt(filepath.Join(providerID, modelID)); content != "" {
			return content
		}
	}

	// 2. Per-provider override: prompts/{providerID}.md
	if content := loadUserPrompt(providerID); content != "" {
		return content
	}

	// 3. Map to canonical name for built-in prompt lookup.
	modelLower := strings.ToLower(modelID)
	var canonical string
	switch {
	case strings.Contains(modelLower, "claude"):
		canonical = "anthropic"
	case providerID == "deepseek" || strings.Contains(modelLower, "deepseek"):
		canonical = "deepseek"
	case providerID == "openai":
		canonical = "openai"
	case strings.Contains(modelLower, "gemini"):
		canonical = "gemini"
	default:
		canonical = "default"
	}

	// Check for user override by canonical name.
	if canonical != providerID {
		if content := loadUserPrompt(canonical); content != "" {
			return content
		}
	}

	// 4. Built-in embedded prompt.
	return builtinPrompt(canonical)
}

// loadUserPrompt checks for a user-provided prompt file in the config
// prompts directory. Supports both .md and .txt extensions.
// Returns empty string if no override exists.
func loadUserPrompt(name string) string {
	dir := filepath.Join(config.ConfigDir(), "prompts")
	for _, ext := range []string{".md", ".txt"} {
		path := filepath.Join(dir, name+ext)
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			return string(data)
		}
	}
	return ""
}
