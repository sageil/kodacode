package skill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sageil/kodacode/internal/prompt"
)

func TestRegistryLoadsProjectAndGlobalSkillsWithProjectPrecedence(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, ".kodacode", "skills", "review")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(project) error = %v", err)
	}

	globalDir := t.TempDir()
	globalSkillDir := filepath.Join(globalDir, "review")
	if err := os.MkdirAll(globalSkillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(global skill) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalSkillDir, "SKILL.md"), []byte(`---
description: global review
---

Global review instructions.
`), 0o644); err != nil {
		t.Fatalf("WriteFile(global) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "SKILL.md"), []byte(`---
name: review
description: project review
---

Project review instructions.
`), 0o644); err != nil {
		t.Fatalf("WriteFile(project) error = %v", err)
	}

	registry, err := NewRegistry(RegistryConfig{GlobalDir: globalDir})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	definition, err := registry.Get(root, "review")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if definition.Description != "project review" {
		t.Fatalf("description = %q", definition.Description)
	}
	if definition.Source != prompt.SourceProject {
		t.Fatalf("source = %q", definition.Source)
	}
}

func TestRegistrySkipsMalformedGlobalSkillWhenOtherSkillsAreValid(t *testing.T) {
	root := t.TempDir()

	globalDir := t.TempDir()
	validDir := filepath.Join(globalDir, "review")
	if err := os.MkdirAll(validDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(valid skill) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(validDir, "SKILL.md"), []byte(`---
description: global review
---

Global review instructions.
`), 0o644); err != nil {
		t.Fatalf("WriteFile(valid) error = %v", err)
	}

	invalidDir := filepath.Join(globalDir, "remember")
	if err := os.MkdirAll(invalidDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(invalid skill) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(invalidDir, "SKILL.md"), []byte(`---
name: remember
description: Save memories. Usage: /remember [what]
---

Remember instructions.
`), 0o644); err != nil {
		t.Fatalf("WriteFile(invalid) error = %v", err)
	}

	registry, err := NewRegistry(RegistryConfig{GlobalDir: globalDir})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	definition, err := registry.Get(root, "review")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if definition.Description != "global review" {
		t.Fatalf("description = %q", definition.Description)
	}
	if _, err := registry.Get(root, "remember"); err == nil {
		t.Fatal("Get(remember) error = nil, want skill not found")
	}
}
