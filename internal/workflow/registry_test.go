package workflow

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistryLoadsEmbeddedWorkflows(t *testing.T) {
	registry, err := NewRegistry(RegistryConfig{GlobalDir: filepath.Join(t.TempDir(), "missing")})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	catalog, err := registry.Catalog(t.TempDir(), testValidationContext())
	if err != nil {
		t.Fatalf("Catalog() error = %v", err)
	}
	definition, err := catalog.Get("delivery")
	if err != nil {
		t.Fatalf("Get(delivery) error = %v", err)
	}
	if definition.Description == "" || len(definition.Phases) == 0 {
		t.Fatalf("delivery workflow = %#v, want populated built-in", definition)
	}
}

func TestRegistryLoadsAllEmbeddedWorkflows(t *testing.T) {
	registry, err := NewRegistry(RegistryConfig{GlobalDir: filepath.Join(t.TempDir(), "missing")})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	catalog, err := registry.Catalog(t.TempDir(), testValidationContext())
	if err != nil {
		t.Fatalf("Catalog() error = %v", err)
	}
	for _, id := range []string{"debug", "delivery", "explore", "review"} {
		definition, err := catalog.Get(id)
		if err != nil {
			t.Fatalf("Get(%s) error = %v", id, err)
		}
		if definition.Description == "" || len(definition.Phases) == 0 {
			t.Fatalf("%s workflow = %#v, want populated built-in", id, definition)
		}
	}
}

func TestRegistryLoadsGlobalWorkflowAndOverridesEmbedded(t *testing.T) {
	globalDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(globalDir, "delivery.yaml"), []byte(workflowYAML("delivery", "global delivery", "builder")), 0o644); err != nil {
		t.Fatalf("WriteFile(global) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "ops.yml"), []byte(workflowYAML("ops", "global ops", "builder")), 0o644); err != nil {
		t.Fatalf("WriteFile(global ops) error = %v", err)
	}
	registry, err := NewRegistry(RegistryConfig{GlobalDir: globalDir})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	catalog, err := registry.Catalog(t.TempDir(), testValidationContext())
	if err != nil {
		t.Fatalf("Catalog() error = %v", err)
	}
	delivery, err := catalog.Get("delivery")
	if err != nil {
		t.Fatalf("Get(delivery) error = %v", err)
	}
	if delivery.Description != "global delivery" {
		t.Fatalf("delivery description = %q, want global delivery", delivery.Description)
	}
	if _, err := catalog.Get("ops"); err != nil {
		t.Fatalf("Get(ops) error = %v", err)
	}
}

func TestRegistryLoadsProjectWorkflowAndOverridesGlobal(t *testing.T) {
	globalDir := t.TempDir()
	workspaceRoot := t.TempDir()
	projectDir := filepath.Join(workspaceRoot, ".kodacode", "workflows")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(project) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "delivery.yaml"), []byte(workflowYAML("delivery", "global delivery", "builder")), 0o644); err != nil {
		t.Fatalf("WriteFile(global) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "delivery.yaml"), []byte(workflowYAML("delivery", "project delivery", "planner")), 0o644); err != nil {
		t.Fatalf("WriteFile(project) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "docs.yaml"), []byte(workflowYAML("docs", "project docs", "planner")), 0o644); err != nil {
		t.Fatalf("WriteFile(project docs) error = %v", err)
	}
	registry, err := NewRegistry(RegistryConfig{GlobalDir: globalDir})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	catalog, err := registry.Catalog(workspaceRoot, testValidationContext())
	if err != nil {
		t.Fatalf("Catalog() error = %v", err)
	}
	delivery, err := catalog.Get("delivery")
	if err != nil {
		t.Fatalf("Get(delivery) error = %v", err)
	}
	if delivery.Description != "project delivery" || delivery.Phases[0].Agent != "planner" {
		t.Fatalf("delivery = %#v, want project override", delivery)
	}
	if _, err := catalog.Get("docs"); err != nil {
		t.Fatalf("Get(docs) error = %v", err)
	}
}

func TestRegistryReportsInvalidProjectWorkflowPath(t *testing.T) {
	workspaceRoot := t.TempDir()
	projectDir := filepath.Join(workspaceRoot, ".kodacode", "workflows")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(project) error = %v", err)
	}
	badPath := filepath.Join(projectDir, "bad.yaml")
	if err := os.WriteFile(badPath, []byte(`
id: bad
phases:
  - id: plan
    agent: missing
`), 0o644); err != nil {
		t.Fatalf("WriteFile(project bad) error = %v", err)
	}
	registry, err := NewRegistry(RegistryConfig{GlobalDir: filepath.Join(t.TempDir(), "missing")})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	_, err = registry.Catalog(workspaceRoot, testValidationContext())
	if err == nil || !strings.Contains(err.Error(), badPath) || !errors.Is(err, ErrWorkflowAgentUnknown) {
		t.Fatalf("Catalog() error = %v, want bad path and ErrWorkflowAgentUnknown", err)
	}
}

func workflowYAML(id, description, agentID string) string {
	return `
id: ` + id + `
description: ` + description + `
phases:
  - id: run
    agent: ` + agentID + `
`
}
