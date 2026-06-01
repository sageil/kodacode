package skill

import (
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/prompt"
)

func TestParseMarkdownDefinitionReadsFrontMatter(t *testing.T) {
	definition, err := parseMarkdownDefinition("review", "/repo/.kodacode/skills/review/SKILL.md", prompt.SourceProject, []byte(`---
name: repo-review
description: Review carefully
when_to_use: Use when reviewing broad repository changes.
user-invocable: false
disable-model-invocation: true
arguments:
  - scope
  - focus
---

Review this repo.
`))
	if err != nil {
		t.Fatalf("parseMarkdownDefinition() error = %v", err)
	}
	if definition.ID != "repo-review" {
		t.Fatalf("id = %q", definition.ID)
	}
	if definition.Path != "/repo/.kodacode/skills/review/SKILL.md" {
		t.Fatalf("path = %q", definition.Path)
	}
	if definition.Description != "Review carefully" {
		t.Fatalf("description = %q", definition.Description)
	}
	if definition.WhenToUse != "Use when reviewing broad repository changes." {
		t.Fatalf("when_to_use = %q", definition.WhenToUse)
	}
	if definition.EffectiveUserInvocable() {
		t.Fatal("EffectiveUserInvocable() = true, want false")
	}
	if definition.ModelVisible() {
		t.Fatal("ModelVisible() = true, want false")
	}
	if strings.Join(definition.Arguments, ",") != "scope,focus" {
		t.Fatalf("arguments = %#v", definition.Arguments)
	}
	if definition.Source != prompt.SourceProject {
		t.Fatalf("source = %q", definition.Source)
	}
}

func TestParseMarkdownDefinitionRejectsSkillToolPolicyFrontMatter(t *testing.T) {
	_, err := parseMarkdownDefinition("review", "/repo/.kodacode/skills/review/SKILL.md", prompt.SourceProject, []byte(`---
description: Review carefully
AllowTools:
  - read
---

Review this repo.
`))
	if err == nil {
		t.Fatal("parseMarkdownDefinition() error = nil, want unsupported tool policy error")
	}
	if !strings.Contains(err.Error(), "skills cannot declare tool policy") {
		t.Fatalf("parseMarkdownDefinition() error = %v", err)
	}
}
