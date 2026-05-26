package provider

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemPrompt(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	tests := []struct {
		name         string
		providerID   string
		modelID      string
		wantContains string
	}{
		{
			name:         "anthropic claude",
			providerID:   "anthropic",
			modelID:      "claude-3-5-sonnet",
			wantContains: "<anthropic_guidance>",
		},
		{
			name:         "openai gpt family",
			providerID:   "openai",
			modelID:      "gpt-5",
			wantContains: "Keep reasoning private.",
		},
		{
			name:         "copilot gpt family uses openai prompt",
			providerID:   "github-copilot",
			modelID:      "gpt-5",
			wantContains: "Keep reasoning private.",
		},
		{
			name:         "compatible openai model uses openai prompt",
			providerID:   "openrouter",
			modelID:      "openai/gpt-4.1",
			wantContains: "Keep reasoning private.",
		},
		{
			name:         "nvidia gpt oss model uses nvidia gpt oss prompt",
			providerID:   "nvidia",
			modelID:      "openai/gpt-oss-20b",
			wantContains: "supports reasoning effort controls and tool use",
		},
		{
			name:         "google gemini",
			providerID:   "google",
			modelID:      "gemini-2.5-flash",
			wantContains: "Be explicit about assumptions",
		},
		{
			name:         "deepseek provider",
			providerID:   "deepseek",
			modelID:      "deepseek-reasoner",
			wantContains: "reasoning_content",
		},
		{
			name:         "deepseek model on compatible provider",
			providerID:   "openrouter",
			modelID:      "deepseek/deepseek-reasoner",
			wantContains: "reasoning_content",
		},
		{
			name:         "qwen model on compatible provider uses qwen prompt",
			providerID:   "openrouter",
			modelID:      "qwen/qwen3.5-32b",
			wantContains: "<qwen_guidance>",
		},
		{
			name:         "qwencloud provider uses qwen prompt",
			providerID:   "qwencloud",
			modelID:      "qwen3.6-plus",
			wantContains: "<qwen_guidance>",
		},
		{
			name:         "deepseek model on nvidia uses nvidia deepseek prompt",
			providerID:   "nvidia",
			modelID:      "deepseek-ai/deepseek-v4-pro",
			wantContains: "NVIDIA-hosted DeepSeek model",
		},
		{
			name:         "nvidia llama model uses nvidia llama prompt",
			providerID:   "nvidia",
			modelID:      "meta/llama-3.3-70b-instruct",
			wantContains: "instruction-tuned open model",
		},
		{
			name:         "nvidia usdcode model uses nvidia usdcode prompt",
			providerID:   "nvidia",
			modelID:      "nvidia/usdcode-llama-3.1-70b-instruct",
			wantContains: "specialized for OpenUSD code and knowledge tasks",
		},
		{
			name:         "unknown model uses default prompt",
			providerID:   "other",
			modelID:      "unknown-model",
			wantContains: "<default_provider_guidance>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SystemPrompt(tt.providerID, tt.modelID)
			if got == "" {
				t.Fatal("SystemPrompt() returned empty string")
			}
			if !promptContains(got, tt.wantContains) {
				t.Fatalf("SystemPrompt() missing %q in %q", tt.wantContains, got)
			}
		})
	}
}

func TestSystemPromptPrefersPerModelUserOverride(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	promptDir := filepath.Join(configHome, "kodacode", "prompts", "github-copilot")
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	override := "custom copilot model prompt"
	path := filepath.Join(promptDir, "gpt-5.md")
	if err := os.WriteFile(path, []byte(override), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if got := SystemPrompt("github-copilot", "gpt-5"); got != override {
		t.Fatalf("SystemPrompt() = %q, want %q", got, override)
	}
	resolution := ResolveSystemPrompt("github-copilot", "gpt-5")
	if !resolution.UserOverride || resolution.SourcePath != path || resolution.Content != override {
		t.Fatalf("ResolveSystemPrompt() = %#v, want override path %q", resolution, path)
	}
}

func TestSystemPromptPrefersPerProviderOverrideOverCanonicalPrompt(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	promptDir := filepath.Join(configHome, "kodacode", "prompts")
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	override := "provider override"
	path := filepath.Join(promptDir, "github-copilot.txt")
	if err := os.WriteFile(path, []byte(override), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if got := SystemPrompt("github-copilot", "gpt-5"); got != override {
		t.Fatalf("SystemPrompt() = %q, want %q", got, override)
	}
}

func TestComposeInstructionsPrependsProviderPrompt(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	got := ComposeInstructions("Base instructions.", "github-copilot", "gpt-5")
	if !strings.Contains(got, "Keep reasoning private.") {
		t.Fatalf("ComposeInstructions() = %q, want openai prompt content", got)
	}
	if !strings.HasSuffix(got, "Base instructions.") {
		t.Fatalf("ComposeInstructions() = %q, want base instructions at end", got)
	}
}

func TestOpenAIPromptIncludesSimpleToolGuidance(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	got := SystemPrompt("github-copilot", "gpt-4.1")
	for _, want := range []string{
		"Use tools only when they are needed to complete the requested action",
		"Do not call a tool just because one is available.",
		"Use `bash` or `test` for project commands and verification.",
		"Prefer `apply_patch` for source edits when it is available.",
		"structured patch tool; send the patch text directly, not JSON.",
		"The runtime applies typed patch hunks against current file content.",
	} {
		if !promptContains(got, want) {
			t.Fatalf("SystemPrompt() missing %q in %q", want, got)
		}
	}
}

func TestCanonicalPromptsUseStructuredSections(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	tests := []struct {
		providerID string
		modelID    string
	}{
		{providerID: "openai", modelID: "gpt-5"},
		{providerID: "anthropic", modelID: "claude-sonnet-4"},
		{providerID: "google", modelID: "gemini-2.5-pro"},
		{providerID: "deepseek", modelID: "deepseek-reasoner"},
		{providerID: "nvidia", modelID: "openai/gpt-oss-120b"},
		{providerID: "nvidia", modelID: "meta/llama-3.3-70b-instruct"},
		{providerID: "nvidia", modelID: "deepseek-ai/deepseek-v4-pro"},
		{providerID: "nvidia", modelID: "nvidia/usdcode-llama-3.1-70b-instruct"},
		{providerID: "other", modelID: "unknown-model"},
	}
	for _, test := range tests {
		got := SystemPrompt(test.providerID, test.modelID)
		rootTag := canonicalPromptRootTag(canonicalPromptName(test.providerID, test.modelID))
		wantSections := []string{
			"<" + rootTag + ">",
			"<workflow>",
			"<tool_use>",
			"<context_management>",
			"<communication>",
			"</" + rootTag + ">",
		}
		for _, want := range wantSections {
			if !promptContains(got, want) {
				t.Fatalf("SystemPrompt(%q, %q) missing %q in %q", test.providerID, test.modelID, want, got)
			}
		}
	}
}

func TestToolUsingCanonicalPromptsIncludeBatchingAndStructuredInspectionGuidance(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	tests := []struct {
		providerID string
		modelID    string
	}{
		{providerID: "openai", modelID: "gpt-5"},
		{providerID: "anthropic", modelID: "claude-sonnet-4"},
		{providerID: "google", modelID: "gemini-2.5-pro"},
		{providerID: "deepseek", modelID: "deepseek-reasoner"},
		{providerID: "nvidia", modelID: "openai/gpt-oss-120b"},
		{providerID: "nvidia", modelID: "meta/llama-3.3-70b-instruct"},
		{providerID: "nvidia", modelID: "deepseek-ai/deepseek-v4-pro"},
		{providerID: "nvidia", modelID: "nvidia/usdcode-llama-3.1-70b-instruct"},
		{providerID: "other", modelID: "unknown-model"},
	}
	for _, test := range tests {
		got := SystemPrompt(test.providerID, test.modelID)
		wantPrompts := []string{
			"Use tools only when they are needed to complete the requested action",
			"when it is blocked, ask one necessary clarification",
			"Do not end with optional offers",
			"If the needed lines are not visible yet, gather the missing context and continue",
			"Do not pause solely to ask permission to read more source context",
			"Lost or compacted visible context is not a blocker by itself",
			"do not ask the user to send \"continue\" just",
		}
		if test.providerID == "openai" {
			wantPrompts = append(wantPrompts,
				"For normal source files, call `read` without `offset` or `limit`",
				"Use `offset` and `limit` only for known line ranges",
				"Before editing or writing based on file contents, make sure the needed lines are visible",
				"Re-read only when the file was edited after the prior read",
				"Do not include the `N:` prefixes",
			)
		}
		for _, want := range wantPrompts {
			if !promptContains(got, want) {
				t.Fatalf("SystemPrompt(%q, %q) missing %q in %q", test.providerID, test.modelID, want, got)
			}
		}
	}
}

func canonicalPromptRootTag(canonical string) string {
	switch canonical {
	case "anthropic":
		return "anthropic_guidance"
	case "deepseek":
		return "deepseek_guidance"
	case "gemini":
		return "gemini_guidance"
	case "nvidia-deepseek":
		return "nvidia_deepseek_guidance"
	case "nvidia-gpt-oss":
		return "nvidia_gpt_oss_guidance"
	case "nvidia-llama":
		return "nvidia_llama_guidance"
	case "nvidia-usdcode":
		return "nvidia_usdcode_guidance"
	case "openai":
		return "openai_guidance"
	default:
		return "default_provider_guidance"
	}
}

func promptContains(got, want string) bool {
	if strings.Contains(got, want) {
		return true
	}
	return strings.Contains(strings.Join(strings.Fields(got), " "), strings.Join(strings.Fields(want), " "))
}

func TestDefaultPromptIncludesSharedOperationalGuidance(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	got := SystemPrompt("other", "unknown-model")
	for _, want := range []string{
		"Use tools only when they are needed to complete the requested action",
		"Treat context as finite. Keep exploration targeted and high-signal.",
		"When the user supplies concrete failure evidence, reason from that evidence first",
		"Be explicit about assumptions that affect execution, permissions, paths, or environment.",
		"Ask one concise clarification only when required information is missing or ambiguous",
		"when it is blocked, ask one necessary clarification",
		"When structured output is requested, return only the requested format.",
		"Do not end with optional offers",
		"Keep replies concise, operational, and grounded in tool results.",
	} {
		if !promptContains(got, want) {
			t.Fatalf("default prompt missing %q in %q", want, got)
		}
	}
}

func TestBuiltInPromptsAvoidStaleWorkflowMandates(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	forbiddenSnippets := []string{
		"Use the `question` tool for EVERY user-facing question",
		"When tasks exist, you MUST call the task tool",
		"search_skills",
	}
	for _, path := range []string{
		"prompts/anthropic.txt",
		"prompts/default.txt",
		"prompts/deepseek.txt",
		"prompts/gemini.txt",
		"prompts/nvidia-deepseek.txt",
		"prompts/nvidia-gpt-oss.txt",
		"prompts/nvidia-llama.txt",
		"prompts/nvidia-usdcode.txt",
		"prompts/openai.txt",
		"prompts/qwen.txt",
	} {
		data, err := fs.ReadFile(BuiltinPromptFS(), path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(data)
		for _, forbidden := range forbiddenSnippets {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s unexpectedly contains %q", path, forbidden)
			}
		}
	}
}
