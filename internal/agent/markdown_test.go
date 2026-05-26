package agent

import (
	"errors"
	"testing"
)

func TestParseMarkdownDefinitionReadsFrontMatter(t *testing.T) {
	definition, err := parseMarkdownDefinition("reviewer", []byte(`---
description: Review code carefully
model: openai/gpt-5-mini
mode: all
hidden: true
AllowTools:
  - read
  - search
DisallowedTools:
  - mcp:secrets
handoff:
  provides:
    - kind: review_findings
      description: Findings and evidence.
  consumes:
    - kind: design_notes
      required: true
      from: latest
      max_sources: 2
      missing: reject
---

You are the reviewer agent.
`))
	if err != nil {
		t.Fatalf("parseMarkdownDefinition() error = %v", err)
	}

	if definition.ID != "reviewer" {
		t.Fatalf("id = %q", definition.ID)
	}
	if definition.Description != "Review code carefully" {
		t.Fatalf("description = %q", definition.Description)
	}
	if got := definition.ModelRoute.Primary.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("primary model = %q", got)
	}
	if definition.EffectiveMode() != ModeAll {
		t.Fatalf("mode = %q", definition.EffectiveMode())
	}
	if !definition.Hidden {
		t.Fatal("hidden = false, want true")
	}
	if len(definition.AllowedTools) != 2 || !definition.AllowsTool("read") || definition.AllowsTool("write") || definition.AllowsTool("edit") {
		t.Fatalf("tools = %#v", definition.AllowedTools)
	}
	if !definition.AllowsTool("search") || definition.AllowsTool("mcp:secrets") {
		t.Fatalf("disallowed tools = %#v", definition.DisallowedTools)
	}
	if len(definition.Handoff.Provides) != 1 || definition.Handoff.Provides[0].Kind != "review_findings" {
		t.Fatalf("handoff provides = %#v", definition.Handoff.Provides)
	}
	if len(definition.Handoff.Consumes) != 1 || definition.Handoff.Consumes[0].Kind != "design_notes" || !definition.Handoff.Consumes[0].Required || definition.Handoff.Consumes[0].MaxSources != 2 {
		t.Fatalf("handoff consumes = %#v", definition.Handoff.Consumes)
	}
}

func TestParseMarkdownDefinitionRejectsFallbackModels(t *testing.T) {
	_, err := parseMarkdownDefinition("reviewer", []byte(`---
description: Review code carefully
model: openai/gpt-5-mini
fallback_models:
  - openai/gpt-5
---
`))
	if !errors.Is(err, ErrAgentFallbackModelsUnsupported) {
		t.Fatalf("parseMarkdownDefinition() error = %v, want ErrAgentFallbackModelsUnsupported", err)
	}
}

func TestParseMarkdownDefinitionAllowsEmptyPrompt(t *testing.T) {
	definition, err := parseMarkdownDefinition("reviewer", []byte(`---
description: Review code carefully
AllowTools:
  - read
---

`))
	if err != nil {
		t.Fatalf("parseMarkdownDefinition() error = %v", err)
	}

	if definition.HasPrompt() {
		t.Fatalf("prompt = %q, want empty prompt", definition.Prompt)
	}
	if err := definition.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestParseMarkdownDefinitionPreservesExplicitEmptyAllowTools(t *testing.T) {
	definition, err := parseMarkdownDefinition("reviewer", []byte(`---
description: Review code carefully
AllowTools: []
---
`))
	if err != nil {
		t.Fatalf("parseMarkdownDefinition() error = %v", err)
	}
	if definition.AllowedTools == nil {
		t.Fatal("AllowedTools = nil, want explicit empty list")
	}
	if definition.AllowsTool("read") {
		t.Fatalf("AllowsTool(read) = true, want false for explicit empty AllowTools")
	}
}

func TestDefinitionModeAndVisibilityDefaults(t *testing.T) {
	definition, err := parseMarkdownDefinition("reviewer", []byte(`---
description: Review code carefully
---
`))
	if err != nil {
		t.Fatalf("parseMarkdownDefinition() error = %v", err)
	}
	if definition.EffectiveMode() != ModePrimary {
		t.Fatalf("mode = %q, want %q", definition.EffectiveMode(), ModePrimary)
	}
	if !definition.Selectable() {
		t.Fatal("Selectable() = false, want true")
	}
	if definition.Delegatable() {
		t.Fatal("Delegatable() = true, want false")
	}
}
