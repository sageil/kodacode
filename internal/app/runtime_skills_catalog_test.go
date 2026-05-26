package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sageil/kodacode/internal/skill"
)

func TestRuntimeListSkillsMergesProjectAndGlobalCatalogForSelection(t *testing.T) {
	root := t.TempDir()
	globalDir := t.TempDir()

	projectSkills := filepath.Join(root, ".kodacode", "skills")
	if err := os.MkdirAll(filepath.Join(projectSkills, "migration"), 0o755); err != nil {
		t.Fatalf("MkdirAll(project migration) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectSkills, "review"), 0o755); err != nil {
		t.Fatalf("MkdirAll(project review) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(globalDir, "review"), 0o755); err != nil {
		t.Fatalf("MkdirAll(global review) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(globalDir, "search"), 0o755); err != nil {
		t.Fatalf("MkdirAll(global search) error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(projectSkills, "migration", "SKILL.md"), []byte(`---
description: Mongo migration workflow
---

Use this skill when changing schema or migrations.
`), 0o644); err != nil {
		t.Fatalf("WriteFile(project migration) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectSkills, "review", "SKILL.md"), []byte(`---
description: Project review workflow
---

Use this skill when reviewing this repository.
`), 0o644); err != nil {
		t.Fatalf("WriteFile(project review) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "review", "SKILL.md"), []byte(`---
description: Global review workflow
---

Use this skill when reviewing any repository.
`), 0o644); err != nil {
		t.Fatalf("WriteFile(global review) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "search", "SKILL.md"), []byte(`---
description: Search workflow
---

Use this skill when searching a codebase.
`), 0o644); err != nil {
		t.Fatalf("WriteFile(global search) error = %v", err)
	}

	runtime := newRuntimeWithClient(t, &fakeProvider{})
	registry, err := skill.NewRegistry(skill.RegistryConfig{GlobalDir: globalDir})
	if err != nil {
		t.Fatalf("skill.NewRegistry() error = %v", err)
	}
	runtime.Skills = registry

	skills, err := runtime.ListSkills(context.Background(), root)
	if err != nil {
		t.Fatalf("ListSkills() error = %v", err)
	}
	if len(skills) != 3 {
		t.Fatalf("len(skills) = %d, want 3", len(skills))
	}
	if got := []string{skills[0].ID, skills[1].ID, skills[2].ID}; !reflect.DeepEqual(got, []string{"migration", "review", "search"}) {
		t.Fatalf("skill ids = %#v", got)
	}
	if skills[0].Source != "project" || skills[1].Source != "project" || skills[2].Source != "global" {
		t.Fatalf("sources = %#v", []string{skills[0].Source, skills[1].Source, skills[2].Source})
	}
	if skills[1].Description != "Project review workflow" {
		t.Fatalf("review description = %q", skills[1].Description)
	}
}
