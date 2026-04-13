package agent_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/sageil/kodacode/v1/internal/agent"
	"github.com/sageil/kodacode/v1/internal/permission"
)

func TestParseMarkdown_WithFrontmatter(t *testing.T) {
	content := `---
name: builder
description: Expert Go engineer
model: openai/gpt-4o
temperature: 0.3
max_tokens: 8192
tools:
  - bash
  - read
permission:
  bash:
    "*": allow
    "rm *": deny
  read: allow
---
You are an expert Go engineer.`

	dir := t.TempDir()
	path := filepath.Join(dir, "builder.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("setup: write file: %v", err)
	}

	agents, err := agent.LoadDir(dir, false)
	if err != nil {
		t.Fatalf("LoadDir(%q) error: %v", dir, err)
	}
	if len(agents) != 1 {
		t.Fatalf("LoadDir(%q) returned %d agents, want 1", dir, len(agents))
	}

	temp := 0.3
	want := agent.Agent{
		ID:          "builder",
		Name:        "builder",
		Description: "Expert Go engineer",
		Model:       "openai/gpt-4o",
		Temperature: &temp,
		MaxTokens:   8192,
		Tools:       []string{"bash", "read"},
		Permission: permission.Config{
			"bash": &permission.Rule{
				Patterns: []permission.Pattern{
					{Glob: "*", Action: permission.ActionAllow},
					{Glob: "rm *", Action: permission.ActionDeny},
				},
			},
			"read": &permission.Rule{Action: permission.ActionAllow},
		},
		SystemPrompt: "You are an expert Go engineer.",
		Builtin:      false,
	}

	if diff := cmp.Diff(want, agents[0]); diff != "" {
		t.Errorf("LoadDir agent mismatch (-want +got):\n%s", diff)
	}
}

func TestParseMarkdown_NoFrontmatter(t *testing.T) {
	content := "Just a plain system prompt."

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plain.md"), []byte(content), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	agents, err := agent.LoadDir(dir, true)
	if err != nil {
		t.Fatalf("LoadDir error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("got %d agents, want 1", len(agents))
	}

	got := agents[0]
	if got.ID != "plain" {
		t.Errorf("agent ID = %q, want %q", got.ID, "plain")
	}
	if got.Name != "plain" {
		t.Errorf("agent Name = %q, want %q", got.Name, "plain")
	}
	if got.SystemPrompt != "Just a plain system prompt." {
		t.Errorf("SystemPrompt = %q, want %q", got.SystemPrompt, "Just a plain system prompt.")
	}
	if !got.Builtin {
		t.Errorf("Builtin = false, want true")
	}
}

func TestParseMarkdown_NameFallsBackToID(t *testing.T) {
	// If frontmatter has no "name", use the ID (filename).
	content := "---\ndescription: no name here\n---\nsome prompt"

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "myagent.md"), []byte(content), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	agents, err := agent.LoadDir(dir, false)
	if err != nil {
		t.Fatalf("LoadDir error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("got %d agents, want 1", len(agents))
	}
	if agents[0].Name != "myagent" {
		t.Errorf("Name = %q, want %q", agents[0].Name, "myagent")
	}
}

func TestLoadDir_SkipsMalformedFrontmatter(t *testing.T) {
	// This YAML is invalid.
	content := "---\nname: [unclosed\n---\nprompt"

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.md"), []byte(content), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	agents, err := agent.LoadDir(dir, false)
	if err != nil {
		t.Fatalf("LoadDir returned unexpected error: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("LoadDir returned %d agents for malformed file, want 0", len(agents))
	}
}

func TestLoadDir_OnlyMarkdownFiles(t *testing.T) {
	dir := t.TempDir()
	// Write an .md and a .txt file.
	if err := os.WriteFile(filepath.Join(dir, "agent.md"), []byte("---\nname: a\n---\nprompt"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not an agent"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	agents, err := agent.LoadDir(dir, false)
	if err != nil {
		t.Fatalf("LoadDir error: %v", err)
	}
	if len(agents) != 1 {
		t.Errorf("LoadDir returned %d agents, want 1 (only .md files)", len(agents))
	}
}

func TestBuiltinAgents(t *testing.T) {
	agents, err := agent.BuiltinAgents()
	if err != nil {
		t.Fatalf("BuiltinAgents() error: %v", err)
	}
	if len(agents) == 0 {
		t.Fatal("BuiltinAgents() returned 0 agents, want at least 1")
	}
	for _, a := range agents {
		if !a.Builtin {
			t.Errorf("agent %q: Builtin = false, want true", a.ID)
		}
		if a.ID == "" {
			t.Errorf("agent has empty ID")
		}
	}
}

func TestParseMarkdown_PermissionStringShorthand(t *testing.T) {
	content := "---\nname: simple\npermission:\n  bash: allow\n  edit: deny\n---\nprompt"

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "simple.md"), []byte(content), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	agents, err := agent.LoadDir(dir, false)
	if err != nil {
		t.Fatalf("LoadDir error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("got %d agents, want 1", len(agents))
	}

	a := agents[0]
	bashRule, ok := a.Permission["bash"]
	if !ok {
		t.Fatalf("Permission missing key %q", "bash")
	}
	if bashRule.Action != permission.ActionAllow {
		t.Errorf("Permission[\"bash\"].Action = %q, want %q", bashRule.Action, permission.ActionAllow)
	}

	editRule, ok := a.Permission["edit"]
	if !ok {
		t.Fatalf("Permission missing key %q", "edit")
	}
	if editRule.Action != permission.ActionDeny {
		t.Errorf("Permission[\"edit\"].Action = %q, want %q", editRule.Action, permission.ActionDeny)
	}
}
