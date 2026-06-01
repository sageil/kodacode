package skill

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/sageil/kodacode/internal/prompt"
)

func TestRegistrySearchMatchesDescriptionAndPrompt(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, ".kodacode", "skills")
	skillDir := filepath.Join(projectDir, "migration")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(project) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
description: Mongo migration workflow
---

# Migration

Use this skill when updating mongoose aggregate pipelines and indexes.
`), 0o644); err != nil {
		t.Fatalf("WriteFile(skill) error = %v", err)
	}

	registry, err := NewRegistry(RegistryConfig{GlobalDir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	matches, err := registry.Search(root, "mongoose indexes", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches = %#v, want 1 result", matches)
	}
	if matches[0].Definition.ID != "migration" {
		t.Fatalf("match id = %q", matches[0].Definition.ID)
	}
	if len(matches[0].Reasons) == 0 {
		t.Fatalf("match reasons = %#v, want non-empty", matches[0].Reasons)
	}
}

func TestRegistrySearchMatchesWhenToUseAndSkipsModelHiddenSkills(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, ".kodacode", "skills")
	releaseDir := filepath.Join(projectDir, "release")
	hiddenDir := filepath.Join(projectDir, "hidden")
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(release) error = %v", err)
	}
	if err := os.MkdirAll(hiddenDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(hidden) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(releaseDir, "SKILL.md"), []byte(`---
description: Release workflow
when_to_use: Use when preparing changelogs and tags.
---

Release instructions.
`), 0o644); err != nil {
		t.Fatalf("WriteFile(release skill) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(hiddenDir, "SKILL.md"), []byte(`---
description: Hidden workflow
when_to_use: Use when preparing changelogs and tags.
disable-model-invocation: true
---

Hidden release instructions.
`), 0o644); err != nil {
		t.Fatalf("WriteFile(hidden skill) error = %v", err)
	}

	registry, err := NewRegistry(RegistryConfig{GlobalDir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	matches, err := registry.Search(root, "changelogs", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches = %#v, want only model-visible skill", matches)
	}
	if matches[0].Definition.ID != "release" {
		t.Fatalf("match id = %q", matches[0].Definition.ID)
	}
	if !slices.Contains(matches[0].Reasons, "when_to_use") {
		t.Fatalf("match reasons = %#v, want when_to_use", matches[0].Reasons)
	}
}

func TestRegistrySearchWildcardListsProjectAndGlobalSkills(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, ".kodacode", "skills")
	projectSkillDir := filepath.Join(projectDir, "migration")
	if err := os.MkdirAll(projectSkillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(project) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectSkillDir, "SKILL.md"), []byte(`---
description: Mongo migration workflow
---

Use this skill for migrations.
`), 0o644); err != nil {
		t.Fatalf("WriteFile(project skill) error = %v", err)
	}

	globalDir := t.TempDir()
	globalSkillDir := filepath.Join(globalDir, "review")
	if err := os.MkdirAll(globalSkillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(global) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalSkillDir, "SKILL.md"), []byte(`---
description: Code review workflow
---

Use this skill for reviews.
`), 0o644); err != nil {
		t.Fatalf("WriteFile(global skill) error = %v", err)
	}

	registry, err := NewRegistry(RegistryConfig{GlobalDir: globalDir})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	matches, err := registry.Search(root, "*", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("matches = %#v, want 2 results", matches)
	}
	if matches[0].Definition.ID != "migration" || matches[1].Definition.ID != "review" {
		t.Fatalf("match ids = %q, %q", matches[0].Definition.ID, matches[1].Definition.ID)
	}
	if matches[0].Definition.Source != prompt.SourceProject {
		t.Fatalf("migration source = %q, want project", matches[0].Definition.Source)
	}
	if matches[1].Definition.Source != prompt.SourceGlobal {
		t.Fatalf("review source = %q, want global", matches[1].Definition.Source)
	}
	if len(matches[1].Reasons) == 0 || matches[1].Reasons[0] != "all skills" {
		t.Fatalf("review reasons = %#v, want all skills", matches[1].Reasons)
	}
}
