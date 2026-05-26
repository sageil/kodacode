package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/provider"
)

func TestDedupePromptFragmentsKeepsLastStableFragmentForKey(t *testing.T) {
	fragments := []prompt.Fragment{
		{Kind: prompt.KindPolicy, Source: prompt.SourceBuiltin, Stability: prompt.StabilityStable, Key: "core-policy", Label: "core-policy", Content: "base"},
		{Kind: prompt.KindPolicy, Source: prompt.SourceProject, Stability: prompt.StabilityStable, Key: "core-policy", Label: "project-policy", Content: "override"},
		{Kind: prompt.KindRuntime, Source: prompt.SourceRuntime, Stability: prompt.StabilityDynamic, Key: "workspace", Label: "workspace", Content: "workspace one"},
		{Kind: prompt.KindRuntime, Source: prompt.SourceRuntime, Stability: prompt.StabilityDynamic, Key: "workspace", Label: "workspace", Content: "workspace two"},
	}

	got := dedupePromptFragments(fragments)
	if len(got) != 3 {
		t.Fatalf("fragment count = %d, want 3", len(got))
	}
	if got[0].Content != "override" {
		t.Fatalf("stable fragment content = %q", got[0].Content)
	}
	if got[1].Content != "workspace one" || got[2].Content != "workspace two" {
		t.Fatalf("dynamic fragments = %#v", got[1:])
	}
}

func TestProviderPromptOverrideFragmentReportsActiveOverride(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	promptDir := filepath.Join(configHome, "kodacode", "prompts")
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	overridePath := filepath.Join(promptDir, "openai.txt")
	override := "<openai_guidance>\ncustom override\n</openai_guidance>"
	if err := os.WriteFile(overridePath, []byte(override), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	fragment, ok := providerPromptOverrideFragment(provider.ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5"})
	if !ok {
		t.Fatal("providerPromptOverrideFragment() ok = false, want true")
	}
	if fragment.Key != "provider-prompt-override" || fragment.Source != string(prompt.SourceUser) || fragment.Kind != string(prompt.KindMetadata) {
		t.Fatalf("fragment = %#v", fragment)
	}
	if !strings.Contains(fragment.Label, overridePath) {
		t.Fatalf("fragment label = %q, want override path %q", fragment.Label, overridePath)
	}
	if fragment.Bytes != len(override) || fragment.Tokens <= 0 {
		t.Fatalf("fragment size = %#v", fragment)
	}
}

func TestProviderPromptOverrideFragmentOmittedForBuiltinPrompt(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if fragment, ok := providerPromptOverrideFragment(provider.ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5"}); ok {
		t.Fatalf("providerPromptOverrideFragment() = %#v, want omitted", fragment)
	}
}
