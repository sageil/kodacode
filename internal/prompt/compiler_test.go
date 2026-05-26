package prompt

import (
	"context"
	"strings"
	"testing"
)

func TestStaticCompilerCompileBuildsCanonicalDocument(t *testing.T) {
	compiler := NewStaticCompiler()

	got, err := compiler.Compile(context.Background(), Request{
		Fragments: []Fragment{
			{Kind: KindPolicy, Source: SourceBuiltin, Stability: StabilityStable, Label: "core-policy", Content: "Be direct."},
			{Kind: KindRole, Source: SourceProject, Stability: StabilityStable, Label: "builder", Content: "You are the builder agent."},
			{Kind: KindRuntime, Source: SourceRuntime, Stability: StabilityDynamic, Label: "current-task", Content: "Focus on the failing test."},
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(got.Fragments) != 3 {
		t.Fatalf("fragment count = %d, want 3", len(got.Fragments))
	}
	if !strings.Contains(got.Document, "Be direct.\n\nYou are the builder agent.") {
		t.Fatalf("document missing ordered stable fragments:\n%s", got.Document)
	}
	if !strings.Contains(got.Document, "Focus on the failing test.") {
		t.Fatalf("document missing dynamic section:\n%s", got.Document)
	}
	if strings.Contains(got.StablePrefix, "Focus on the failing test.") {
		t.Fatalf("stable prefix should not contain dynamic fragment:\n%s", got.StablePrefix)
	}
	if !strings.Contains(got.DynamicSuffix, "Focus on the failing test.") {
		t.Fatalf("dynamic suffix missing runtime fragment:\n%s", got.DynamicSuffix)
	}
	if got.ProviderDocument != got.Document {
		t.Fatalf("provider document = %q, want fallback to full document", got.ProviderDocument)
	}
}

func TestStaticCompilerCompileBuildsProviderDocumentWhenPresent(t *testing.T) {
	compiler := NewStaticCompiler()

	got, err := compiler.Compile(context.Background(), Request{
		Fragments: []Fragment{
			{
				Kind:            KindPolicy,
				Source:          SourceProject,
				Stability:       StabilityStable,
				Label:           "project",
				Content:         "Long project instructions.",
				ProviderContent: "Compact project instructions.",
			},
			{
				Kind:      KindRuntime,
				Source:    SourceRuntime,
				Stability: StabilityDynamic,
				Label:     "runtime",
				Content:   "Dynamic note.",
			},
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if got.ProviderStablePrefix != "Compact project instructions." {
		t.Fatalf("provider stable prefix = %q", got.ProviderStablePrefix)
	}
	if got.ProviderDocument != "Compact project instructions.\n\nDynamic note." {
		t.Fatalf("provider document = %q", got.ProviderDocument)
	}
}

func TestStaticCompilerCompileRejectsMissingStability(t *testing.T) {
	compiler := NewStaticCompiler()

	_, err := compiler.Compile(context.Background(), Request{
		Fragments: []Fragment{
			{Kind: KindPolicy, Source: SourceBuiltin, Content: "Be direct."},
		},
	})
	if err == nil {
		t.Fatal("Compile() error = nil, want validation error")
	}
}
