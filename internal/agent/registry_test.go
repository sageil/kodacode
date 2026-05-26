package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryLoadsProjectAndGlobalAgentsWithProjectPrecedence(t *testing.T) {
	globalDir := filepath.Join(t.TempDir(), "global")
	workspaceRoot := t.TempDir()
	projectDir := filepath.Join(workspaceRoot, ".kodacode", "agents")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(global) error = %v", err)
	}
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(project) error = %v", err)
	}

	globalAgent := `---
description: global reviewer
model: openai/gpt-5-mini
AllowTools:
  - read
---

You are the global reviewer.
`
	projectAgent := `---
description: project reviewer
model: openai/gpt-5
AllowTools:
  - read
  - search
---

You are the project reviewer.
`
	if err := os.WriteFile(filepath.Join(globalDir, "reviewer.md"), []byte(globalAgent), 0o644); err != nil {
		t.Fatalf("WriteFile(global) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "reviewer.md"), []byte(projectAgent), 0o644); err != nil {
		t.Fatalf("WriteFile(project) error = %v", err)
	}

	registry, err := NewRegistry(RegistryConfig{GlobalDir: globalDir})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	reviewer, err := registry.Get(workspaceRoot, "reviewer")
	if err != nil {
		t.Fatalf("Get(\"reviewer\") error = %v", err)
	}
	if reviewer.Description != "project reviewer" {
		t.Fatalf("description = %q", reviewer.Description)
	}
	if got := reviewer.ModelRoute.Primary.String(); got != "openai/gpt-5" {
		t.Fatalf("primary model = %q", got)
	}
	if !reviewer.AllowsTool("search") || reviewer.AllowsTool("write") || reviewer.AllowsTool("edit") {
		t.Fatalf("tools = %#v", reviewer.AllowedTools)
	}
}

func TestRegistryLoadsGlobalAgentAndOverridesBuiltin(t *testing.T) {
	globalDir := filepath.Join(t.TempDir(), "global")
	workspaceRoot := t.TempDir()
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(global) error = %v", err)
	}

	builderAgent := `---
description: global builder override
model: openai/gpt-5-mini
AllowTools:
  - read
---

You are the global builder.
`
	opsAgent := `---
description: global ops agent
AllowTools:
  - search
---

You are the ops agent.
`
	if err := os.WriteFile(filepath.Join(globalDir, "builder.md"), []byte(builderAgent), 0o644); err != nil {
		t.Fatalf("WriteFile(builder) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "ops.md"), []byte(opsAgent), 0o644); err != nil {
		t.Fatalf("WriteFile(ops) error = %v", err)
	}

	registry, err := NewRegistry(RegistryConfig{GlobalDir: globalDir})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	builder, err := registry.Get(workspaceRoot, DefaultID)
	if err != nil {
		t.Fatalf("Get(builder) error = %v", err)
	}
	if builder.Description != "global builder override" {
		t.Fatalf("builder description = %q", builder.Description)
	}
	if got := builder.ModelRoute.Primary.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("builder model = %q", got)
	}
	if !builder.AllowsTool("read") || builder.AllowsTool("search") {
		t.Fatalf("builder tools = %#v", builder.AllowedTools)
	}
	if builder.Prompt != "You are the global builder." {
		t.Fatalf("builder prompt = %q", builder.Prompt)
	}

	ops, err := registry.Get(workspaceRoot, "ops")
	if err != nil {
		t.Fatalf("Get(ops) error = %v", err)
	}
	if ops.Description != "global ops agent" {
		t.Fatalf("ops description = %q", ops.Description)
	}
	if !ops.AllowsTool("search") || ops.AllowsTool("read") {
		t.Fatalf("ops tools = %#v", ops.AllowedTools)
	}
}
