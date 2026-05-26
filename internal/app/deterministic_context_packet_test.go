package app

import (
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/provider"
)

func TestBuildDeterministicContextPacketHasNoDefaultSections(t *testing.T) {
	packet := buildDeterministicContextPacket(deterministicContextPacketInput{
		ResolvedInputLimitTokens: 100000,
		Sections: []deterministicContextPacketSectionInput{
			{Key: "repo", Label: "Repository", Source: "git status", Freshness: "current", Content: "workspace: kodacode"},
		},
	})

	if packet.Content != "" {
		t.Fatalf("Content = %q, want empty", packet.Content)
	}
	if len(packet.Sections) != 0 {
		t.Fatalf("sections = %d, want 0", len(packet.Sections))
	}
	if packet.TokenBudget != deterministicContextPacketMaxTokens {
		t.Fatalf("TokenBudget = %d, want %d", packet.TokenBudget, deterministicContextPacketMaxTokens)
	}
	if packet.InputLimitTokens != 100000 {
		t.Fatalf("InputLimitTokens = %d, want 100000", packet.InputLimitTokens)
	}
}

func TestDeterministicContextPacketTokenBudgetCapsAtFivePercentOrTwoThousand(t *testing.T) {
	tests := []struct {
		name       string
		inputLimit int
		want       int
	}{
		{name: "unset", inputLimit: 0, want: 0},
		{name: "tiny", inputLimit: 10, want: 1},
		{name: "normal", inputLimit: 20000, want: 1000},
		{name: "capped", inputLimit: 100000, want: 2000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deterministicContextPacketTokenBudget(tt.inputLimit); got != tt.want {
				t.Fatalf("deterministicContextPacketTokenBudget(%d) = %d, want %d", tt.inputLimit, got, tt.want)
			}
		})
	}
}

func TestBuildDeterministicContextPacketIncludesEnabledSectionsWithAttribution(t *testing.T) {
	packet := buildDeterministicContextPacket(deterministicContextPacketInput{
		ResolvedInputLimitTokens: 20000,
		InputLimitSource:         currentTurnInputLimitSourceCatalog,
		EnabledSections:          []string{"git", "repo"},
		Sections: []deterministicContextPacketSectionInput{
			{Key: "repo", Label: "Repository", Source: "repo map", Freshness: "current", Content: "workspace: kodacode"},
			{Key: "git", Label: "Git", Source: "git status", Freshness: "current", Content: "branch: main"},
		},
	})

	if len(packet.Sections) != 2 {
		t.Fatalf("sections = %d, want 2", len(packet.Sections))
	}
	if packet.Sections[0].Key != "repo" || packet.Sections[1].Key != "git" {
		t.Fatalf("section order = %#v, want input order", packet.Sections)
	}
	if packet.Tokens == 0 || packet.Tokens > packet.TokenBudget {
		t.Fatalf("Tokens = %d, TokenBudget = %d", packet.Tokens, packet.TokenBudget)
	}
	if packet.Tokens != provider.EstimateTextTokens(packet.Content) {
		t.Fatalf("Tokens = %d, want rendered token estimate", packet.Tokens)
	}
	if packet.InputLimitTokens != 20000 || packet.InputLimitSource != currentTurnInputLimitSourceCatalog {
		t.Fatalf("input attribution = %d/%q, want resolved catalog budget", packet.InputLimitTokens, packet.InputLimitSource)
	}
	for _, section := range packet.Sections {
		if section.Tokens <= 0 {
			t.Fatalf("section %q Tokens = %d, want positive", section.Key, section.Tokens)
		}
		if section.Bytes != len(section.Content) {
			t.Fatalf("section %q Bytes = %d, want %d", section.Key, section.Bytes, len(section.Content))
		}
		if section.Source == "" || section.Freshness == "" {
			t.Fatalf("section %q attribution = source %q freshness %q, want populated", section.Key, section.Source, section.Freshness)
		}
	}
	if !strings.Contains(packet.Content, `<section key="repo" label="Repository" source="repo map" freshness="current">`) {
		t.Fatalf("Content missing repo section: %q", packet.Content)
	}
}

func TestBuildDeterministicContextPacketOmitsSectionsThatExceedBudget(t *testing.T) {
	packet := buildDeterministicContextPacket(deterministicContextPacketInput{
		ResolvedInputLimitTokens: 100,
		EnabledSections:          []string{"large"},
		Sections: []deterministicContextPacketSectionInput{
			{Key: "large", Label: "Large", Source: "test", Freshness: "current", Content: strings.Repeat("abcd", 80)},
		},
	})

	if packet.Content != "" {
		t.Fatalf("Content = %q, want empty", packet.Content)
	}
	if len(packet.Sections) != 0 {
		t.Fatalf("sections = %d, want 0", len(packet.Sections))
	}
	if len(packet.Omitted) != 1 {
		t.Fatalf("omitted = %d, want 1", len(packet.Omitted))
	}
	if packet.Omitted[0].Reason != "token_budget" {
		t.Fatalf("omission reason = %q, want token_budget", packet.Omitted[0].Reason)
	}
	if packet.Omitted[0].Source != "test" || packet.Omitted[0].Freshness != "current" {
		t.Fatalf("omission attribution = source %q freshness %q", packet.Omitted[0].Source, packet.Omitted[0].Freshness)
	}
}

func TestDeterministicContextPacketFragmentUsesRuntimeDynamicPromptFragment(t *testing.T) {
	packet := buildDeterministicContextPacket(deterministicContextPacketInput{
		ResolvedInputLimitTokens: 20000,
		EnabledSections:          []string{"repo"},
		Sections: []deterministicContextPacketSectionInput{
			{Key: "repo", Source: "repo map", Freshness: "current", Content: "workspace: kodacode"},
		},
	})
	fragment, ok := deterministicContextPacketFragment(packet)
	if !ok {
		t.Fatal("fragment not returned")
	}

	if fragment.Kind != prompt.KindRuntime {
		t.Fatalf("Kind = %q, want %q", fragment.Kind, prompt.KindRuntime)
	}
	if fragment.Source != prompt.SourceRuntime {
		t.Fatalf("Source = %q, want %q", fragment.Source, prompt.SourceRuntime)
	}
	if fragment.Stability != prompt.StabilityDynamic {
		t.Fatalf("Stability = %q, want %q", fragment.Stability, prompt.StabilityDynamic)
	}
	if fragment.Key != "deterministic_context_packet" {
		t.Fatalf("Key = %q, want deterministic_context_packet", fragment.Key)
	}
	if fragment.Content != packet.Content {
		t.Fatalf("Content = %q, want packet content", fragment.Content)
	}
}

func TestBuildDeterministicContextPacketForRequestUsesResolvedModelInputBudget(t *testing.T) {
	ref := provider.ModelRef{ProviderID: "nvidia", ModelID: "openai/gpt-oss-20b"}
	catalog := wrapModelCatalogOverrides(&fakeModelCatalog{}, []ModelOverrideConfig{{
		Ref:            ref,
		MaxInputTokens: intPtr(20000),
	}})

	packet := buildDeterministicContextPacketForRequest(deterministicContextPacketRequestInput{
		Request: provider.Request{Model: ref},
		Models:  catalog,
		EnabledSections: []string{
			"repo",
		},
		Sections: []deterministicContextPacketSectionInput{
			{Key: "repo", Source: "repo map", Freshness: "current", Content: "workspace: kodacode"},
		},
	})

	if packet.InputLimitTokens != 20000 {
		t.Fatalf("InputLimitTokens = %d, want resolved model input budget", packet.InputLimitTokens)
	}
	if packet.InputLimitSource != currentTurnInputLimitSourceCatalog {
		t.Fatalf("InputLimitSource = %q, want %q", packet.InputLimitSource, currentTurnInputLimitSourceCatalog)
	}
	if packet.TokenBudget != 1000 {
		t.Fatalf("TokenBudget = %d, want 1000", packet.TokenBudget)
	}
	if len(packet.Sections) != 1 {
		t.Fatalf("sections = %d, want 1", len(packet.Sections))
	}
}

func TestBuildDeterministicContextPacketForRequestOmitsWhenInputBudgetIsUnknown(t *testing.T) {
	packet := buildDeterministicContextPacketForRequest(deterministicContextPacketRequestInput{
		Request: provider.Request{Model: provider.ModelRef{ProviderID: "unknown", ModelID: "model"}},
		Models:  &fakeModelCatalog{},
		EnabledSections: []string{
			"repo",
		},
		Sections: []deterministicContextPacketSectionInput{
			{Key: "repo", Source: "repo map", Freshness: "current", Content: "workspace: kodacode"},
		},
	})

	if packet.TokenBudget != 0 || packet.Content != "" {
		t.Fatalf("packet = %#v, want no packet without resolved input budget", packet)
	}
}
