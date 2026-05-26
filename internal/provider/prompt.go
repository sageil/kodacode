package provider

import (
	"os"
	"path/filepath"
	"strings"
)

type SystemPromptResolution struct {
	Content       string
	CanonicalName string
	SourcePath    string
	UserOverride  bool
}

// SystemPrompt returns a provider/model-specific prompt supplement.
//
// Resolution order:
//  1. Per-model: ~/.config/kodacode/prompts/{providerID}/{modelID}.md|.txt
//  2. Per-provider: ~/.config/kodacode/prompts/{providerID}.md|.txt
//  3. Canonical override: ~/.config/kodacode/prompts/openai.md, deepseek.md, etc.
//  4. Built-in embedded prompt
func SystemPrompt(providerID, modelID string) string {
	return ResolveSystemPrompt(providerID, modelID).Content
}

func ResolveSystemPrompt(providerID, modelID string) SystemPromptResolution {
	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)
	canonical := canonicalPromptName(providerID, modelID)
	promptDir := filepath.Join(providerConfigDir(), "prompts")
	resolution := SystemPromptResolution{CanonicalName: canonical}

	if promptOverridesDirExists(promptDir) {
		if modelID != "" && providerID != "" {
			if content, path := loadUserPrompt(promptDir, filepath.Join(providerID, modelID)); content != "" {
				resolution.Content = content
				resolution.SourcePath = path
				resolution.UserOverride = true
				return resolution
			}
		}

		if providerID != "" {
			if content, path := loadUserPrompt(promptDir, providerID); content != "" {
				resolution.Content = content
				resolution.SourcePath = path
				resolution.UserOverride = true
				return resolution
			}
		}

		if canonical != providerID {
			if content, path := loadUserPrompt(promptDir, canonical); content != "" {
				resolution.Content = content
				resolution.SourcePath = path
				resolution.UserOverride = true
				return resolution
			}
		}
	}

	resolution.Content = builtinPrompt(canonical)
	return resolution
}

func ComposeInstructions(baseInstructions, providerID, modelID string) string {
	base := strings.TrimSpace(baseInstructions)
	supplement := strings.TrimSpace(SystemPrompt(providerID, modelID))

	switch {
	case supplement == "":
		return base
	case base == "":
		return supplement
	default:
		return supplement + "\n\n" + base
	}
}

func canonicalPromptName(providerID, modelID string) string {
	providerID = CanonicalProviderID(providerID)
	modelLower := strings.ToLower(strings.TrimSpace(modelID))

	switch {
	case providerID == "nvidia":
		return canonicalNVIDIAPromptName(modelLower)
	case providerID == "anthropic" || strings.Contains(modelLower, "claude"):
		return "anthropic"
	case providerID == "deepseek" || strings.Contains(modelLower, "deepseek"):
		return "deepseek"
	case providerID == "qwencloud" || looksLikeQwenModel(modelLower):
		return "qwen"
	case providerID == "google" || strings.Contains(modelLower, "gemini"):
		return "gemini"
	case providerID == "openai" || providerID == "github-copilot" || looksLikeOpenAIModel(modelLower):
		return "openai"
	default:
		return "default"
	}
}

func canonicalNVIDIAPromptName(model string) string {
	switch {
	case strings.Contains(model, "usdcode"):
		return "nvidia-usdcode"
	case strings.Contains(model, "deepseek"):
		return "nvidia-deepseek"
	case strings.Contains(model, "gpt-oss"):
		return "nvidia-gpt-oss"
	case strings.Contains(model, "llama"):
		return "nvidia-llama"
	default:
		return "default"
	}
}

func looksLikeOpenAIModel(model string) bool {
	if model == "" {
		return false
	}
	return strings.Contains(model, "gpt-") ||
		strings.Contains(model, "codex") ||
		strings.Contains(model, "/o1") ||
		strings.Contains(model, "/o3") ||
		strings.Contains(model, "/o4") ||
		strings.HasPrefix(model, "o1") ||
		strings.HasPrefix(model, "o3") ||
		strings.HasPrefix(model, "o4")
}

func looksLikeQwenModel(model string) bool {
	if model == "" {
		return false
	}
	return strings.Contains(model, "qwen") || strings.Contains(model, "qwq")
}

func loadUserPrompt(dir, name string) (string, string) {
	for _, ext := range []string{".md", ".txt"} {
		path := filepath.Join(dir, name+ext)
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			return string(data), path
		}
	}
	return "", ""
}

func promptOverridesDirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
