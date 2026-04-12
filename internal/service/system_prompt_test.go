package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/v1/internal/agent"
	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/provider"
)

func newTestBuilder(t *testing.T, projectDir string) *SystemPromptBuilder {
	t.Helper()
	return NewSystemPromptBuilder(SystemPromptBuilderConfig{
		ProjectDir: projectDir,
		SkillsDir:  t.TempDir(),
	})
}

type buildPromptOpts struct {
	ephemeral bool
	tools     []provider.Tool
}

type buildPromptOption func(*buildPromptOpts)

func withEphemeral(v bool) buildPromptOption { return func(o *buildPromptOpts) { o.ephemeral = v } }
func withTools(t []provider.Tool) buildPromptOption {
	return func(o *buildPromptOpts) { o.tools = t }
}

func buildPromptParts(t *testing.T, builder *SystemPromptBuilder, agentPrompt, providerID, modelID, summaryText string, opts ...buildPromptOption) []string {
	t.Helper()
	var o buildPromptOpts
	for _, fn := range opts {
		fn(&o)
	}
	in := SystemPromptBuildInput{
		AgentPrompt: agentPrompt,
		ProviderID:  providerID,
		ModelID:     modelID,
		SummaryText: summaryText,
		Ephemeral:   o.ephemeral,
		Tools:       o.tools,
	}
	parts, err := builder.Build(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	return parts
}

var subagentTool = []provider.Tool{{Name: "subagent"}}

func TestBuildEnvironmentPrompt(t *testing.T) {
	tmpDir := t.TempDir()

	builder := newTestBuilder(t, tmpDir)
	env := builder.buildEnvironmentPrompt("")

	if !strings.Contains(env, "Working directory:") {
		t.Error("missing working directory")
	}
	if !strings.Contains(env, "Platform:") {
		t.Error("missing platform")
	}
	if !strings.Contains(env, "Today's date:") {
		t.Error("missing date")
	}
}

func TestBuildEnvironmentPrompt_GitRepo(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.Mkdir(filepath.Join(tmpDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	builder := newTestBuilder(t, tmpDir)
	env := builder.buildEnvironmentPrompt("")

	if !strings.Contains(env, "Is directory a git repo: yes") {
		t.Error("should detect git repo")
	}
}

func TestBuildEnvironmentPrompt_NotGitRepo(t *testing.T) {
	tmpDir := t.TempDir()

	builder := newTestBuilder(t, tmpDir)
	env := builder.buildEnvironmentPrompt("")

	if !strings.Contains(env, "Is directory a git repo: no") {
		t.Error("should detect non-git repo")
	}
}

func TestLoadInstructionFiles(t *testing.T) {
	tmpDir := t.TempDir()

	agentsContent := "# Project Instructions\n\nUse Go 1.21+"
	if err := os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte(agentsContent), 0644); err != nil {
		t.Fatal(err)
	}

	builder := newTestBuilder(t, tmpDir)
	instructions, _ := builder.loadInstructionFiles()

	if len(instructions) == 0 {
		t.Fatal("expected at least one instruction file")
	}

	if !strings.Contains(instructions[0], "Project Instructions") {
		t.Error("missing AGENTS.md content")
	}

	if !strings.Contains(instructions[0], "Instructions from:") {
		t.Error("missing instruction header")
	}
}

func TestLoadInstructionFiles_Truncation(t *testing.T) {
	tmpDir := t.TempDir()

	largeContent := strings.Repeat("x", 35000)
	if err := os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte(largeContent), 0644); err != nil {
		t.Fatal(err)
	}

	builder := newTestBuilder(t, tmpDir)
	instructions, _ := builder.loadInstructionFiles()

	if !strings.Contains(instructions[0], "[truncated") {
		t.Error("should truncate large files")
	}
}

func TestLoadInstructionFiles_Hierarchical(t *testing.T) {
	tmpDir := t.TempDir()

	rootContent := "# Root Instructions\n\nRoot level"
	if err := os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte(rootContent), 0644); err != nil {
		t.Fatal(err)
	}

	subDir := filepath.Join(tmpDir, "submodule")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	subContent := "# Submodule Instructions\n\nSubmodule level"
	if err := os.WriteFile(filepath.Join(subDir, "AGENTS.md"), []byte(subContent), 0644); err != nil {
		t.Fatal(err)
	}

	builder := newTestBuilder(t, subDir)
	instructions, _ := builder.loadInstructionFiles()

	if len(instructions) < 2 {
		t.Fatalf("expected at least 2 instruction files, got %d", len(instructions))
	}

	if !strings.Contains(instructions[0], "Root Instructions") {
		t.Error("root instructions should come first")
	}

	if !strings.Contains(instructions[1], "Submodule Instructions") {
		t.Error("submodule instructions should come second")
	}
}

func TestLoadInstructionFiles_NoFiles(t *testing.T) {
	tmpDir := t.TempDir()

	builder := newTestBuilder(t, tmpDir)
	instructions, _ := builder.loadInstructionFiles()

	if len(instructions) != 0 {
		t.Errorf("loadInstructionFiles() = %d files, want 0", len(instructions))
	}
}

func TestBuildSystemPrompt(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte("# Test Instructions"), 0644); err != nil {
		t.Fatal(err)
	}

	builder := newTestBuilder(t, tmpDir)
	parts := buildPromptParts(t, builder, "", "openai", "gpt-4", "")
	prompt := strings.Join(parts, "\n\n")

	if !strings.Contains(prompt, "autonomous") {
		t.Error("missing provider prompt")
	}

	if !strings.Contains(prompt, "Working directory:") {
		t.Error("missing environment prompt")
	}

	if !strings.Contains(prompt, "Test Instructions") {
		t.Error("missing instruction file content")
	}
}

func TestBuildSystemPrompt_NoInstructions(t *testing.T) {
	tmpDir := t.TempDir()

	builder := newTestBuilder(t, tmpDir)
	parts := buildPromptParts(t, builder, "", "openai", "gpt-4", "")
	prompt := strings.Join(parts, "\n\n")

	if !strings.Contains(prompt, "autonomous") {
		t.Error("missing provider prompt")
	}

	if !strings.Contains(prompt, "Working directory:") {
		t.Error("missing environment prompt")
	}
}

func TestBuildSystemPrompt_CacheIsModelScoped(t *testing.T) {
	tmpDir := t.TempDir()

	builder := newTestBuilder(t, tmpDir)

	partsA := buildPromptParts(t, builder, "", "openai", "gpt-4o", "")
	partsB := buildPromptParts(t, builder, "", "openai", "gpt-4.1-mini", "")

	if !strings.Contains(partsA[1], "Model: gpt-4o") {
		t.Fatalf("first build missing model marker: %q", partsA[1])
	}
	if !strings.Contains(partsB[1], "Model: gpt-4.1-mini") {
		t.Fatalf("second build missing model marker: %q", partsB[1])
	}
}

func TestBuildSystemPrompt_CacheIsEphemeralScoped(t *testing.T) {
	tmpDir := t.TempDir()

	builder := NewSystemPromptBuilder(SystemPromptBuilderConfig{
		ProjectDir: tmpDir,
		SkillsDir:  t.TempDir(),
		Agents: fakeAgentLister{
			agents: []agent.Agent{{ID: "explorer", Mode: agent.ModeSubagent, Description: "read-only"}},
		},
	})

	partsPrimary := buildPromptParts(t, builder, "", "openai", "gpt-4o", "", withTools(subagentTool))
	if !strings.Contains(partsPrimary[2], "Specialized agents") {
		t.Fatalf("primary prompt missing agents block in volatile part: %q", partsPrimary[2])
	}

	partsEphemeral := buildPromptParts(t, builder, "", "openai", "gpt-4o", "", withEphemeral(true), withTools(subagentTool))
	if strings.Contains(partsEphemeral[2], "Specialized agents") {
		t.Fatalf("ephemeral prompt should not include agents block: %q", partsEphemeral[2])
	}
}

func TestBuildSystemPrompt_CacheInvalidatesOnDateChange(t *testing.T) {
	tmpDir := t.TempDir()
	builder := newTestBuilder(t, tmpDir)

	parts := buildPromptParts(t, builder, "", "openai", "gpt-4o", "")
	if parts[1] == "" {
		t.Fatal("semi-stable prompt should not be empty")
	}

	key := semiStableCacheKey{modelID: "openai/gpt-4o", skillFingerprint: "allow:|deny:", skillToolMode: "skill:true|search:true"}
	builder.mu.Lock()
	entry := builder.cache[key]
	entry.content = "stale-cache"
	entry.dateKey = "2000-01-01"
	builder.cache[key] = entry
	builder.mu.Unlock()

	parts = buildPromptParts(t, builder, "", "openai", "gpt-4o", "")
	if parts[1] == "stale-cache" {
		t.Fatal("expected stale semi-stable cache entry to be invalidated on date change")
	}
}

func TestBuildSystemPrompt_AgentsBlockInVolatileWhenSubagentAvailable(t *testing.T) {
	tmpDir := t.TempDir()
	builder := NewSystemPromptBuilder(SystemPromptBuilderConfig{
		ProjectDir: tmpDir,
		SkillsDir:  t.TempDir(),
		Agents: fakeAgentLister{
			agents: []agent.Agent{{ID: "explorer", Mode: agent.ModeSubagent, Description: "read-only"}},
		},
	})

	// With subagent in tools: agents block appears in volatile part.
	parts := buildPromptParts(t, builder, "", "openai", "gpt-4o", "", withTools(subagentTool))
	if !strings.Contains(parts[2], "explorer") {
		t.Fatalf("volatile part missing explorer agent when subagent tool present: %q", parts[2])
	}
	if strings.Contains(parts[1], "Specialized agents") {
		t.Fatalf("semi-stable part should not contain agents block: %q", parts[1])
	}

	// Without subagent in tools: agents block omitted entirely.
	partsNoSubagent := buildPromptParts(t, builder, "", "openai", "gpt-4o", "", withTools([]provider.Tool{{Name: "bash"}}))
	if strings.Contains(partsNoSubagent[2], "explorer") {
		t.Fatalf("volatile part should not contain agents block without subagent tool: %q", partsNoSubagent[2])
	}

	// Agent list changes are reflected immediately (not cached).
	builder.cfg.Agents = fakeAgentLister{
		agents: []agent.Agent{{ID: "reviewer", Mode: agent.ModeSubagent, Description: "checks changes"}},
	}
	parts = buildPromptParts(t, builder, "", "openai", "gpt-4o", "", withTools(subagentTool))
	if strings.Contains(parts[2], "explorer") {
		t.Fatalf("volatile part should not retain stale explorer entry: %q", parts[2])
	}
	if !strings.Contains(parts[2], "reviewer") {
		t.Fatalf("volatile part missing updated reviewer agent: %q", parts[2])
	}
}

func TestBuildSystemPrompt_CacheInvalidatesOnSkillFileChange(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := t.TempDir()
	skillDir := filepath.Join(skillsDir, "reviewer")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillFile := filepath.Join(skillDir, "SKILL.md")
	initial := "---\nname: reviewer\ndescription: First version\n---\n"
	if err := os.WriteFile(skillFile, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	builder := NewSystemPromptBuilder(SystemPromptBuilderConfig{
		ProjectDir: tmpDir,
		SkillsDir:  skillsDir,
	})
	parts := buildPromptParts(t, builder, "", "openai", "gpt-4o", "")
	if !strings.Contains(parts[1], "First version") {
		t.Fatalf("initial prompt missing skill description: %q", parts[1])
	}

	updated := "---\nname: reviewer\ndescription: Updated version\n---\n"
	if err := os.WriteFile(skillFile, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(time.Second)
	if err := os.Chtimes(skillFile, now, now); err != nil {
		t.Fatal(err)
	}

	parts = buildPromptParts(t, builder, "", "openai", "gpt-4o", "")
	if strings.Contains(parts[1], "First version") {
		t.Fatalf("prompt retained stale skill description after file change: %q", parts[1])
	}
	if !strings.Contains(parts[1], "Updated version") {
		t.Fatalf("prompt missing updated skill description: %q", parts[1])
	}
}

func TestBuildSystemPrompt_CacheInvalidatesOnIgnorePatternChange(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{IgnorePatterns: []string{"node_modules/**"}}
	builder := NewSystemPromptBuilder(SystemPromptBuilderConfig{
		ProjectDir: tmpDir,
		SkillsDir:  t.TempDir(),
		Config:     cfg,
	})

	parts := buildPromptParts(t, builder, "", "openai", "gpt-4o", "")
	if !strings.Contains(parts[1], "node_modules/**") {
		t.Fatalf("initial prompt missing ignore pattern: %q", parts[1])
	}

	cfg.IgnorePatterns = []string{"vendor/**"}
	parts = buildPromptParts(t, builder, "", "openai", "gpt-4o", "")
	if strings.Contains(parts[1], "node_modules/**") {
		t.Fatalf("prompt retained stale ignore pattern after config change: %q", parts[1])
	}
	if !strings.Contains(parts[1], "vendor/**") {
		t.Fatalf("prompt missing updated ignore pattern: %q", parts[1])
	}
}

func TestBuildSystemPrompt_SkillsPreferProjectOverride(t *testing.T) {
	tmpDir := t.TempDir()
	globalSkillsDir := t.TempDir()

	projectSkillDir := filepath.Join(tmpDir, ".kodacode", "skills", "reviewer")
	if err := os.MkdirAll(projectSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	globalSkillDir := filepath.Join(globalSkillsDir, "reviewer")
	if err := os.MkdirAll(globalSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(globalSkillDir, "SKILL.md"), []byte("---\nname: reviewer\ndescription: Global reviewer skill\n---\n# Reviewer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectSkillDir, "SKILL.md"), []byte("---\nname: reviewer\ndescription: Project reviewer skill\n---\n# Reviewer\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	builder := NewSystemPromptBuilder(SystemPromptBuilderConfig{
		ProjectDir: tmpDir,
		SkillsDir:  globalSkillsDir,
	})
	parts, err := builder.Build(context.Background(), SystemPromptBuildInput{
		ProviderID: "openai",
		ModelID:    "gpt-4o",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(parts[1], "Project reviewer skill") {
		t.Fatalf("prompt missing project skill description: %q", parts[1])
	}
	if strings.Contains(parts[1], "Global reviewer skill") {
		t.Fatalf("prompt should prefer project override over global skill: %q", parts[1])
	}
}

func TestBuildSystemPrompt_SkillsHonorModelPolicy(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := t.TempDir()
	skillDir := filepath.Join(skillsDir, "reviewer")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: reviewer\ndescription: Hidden reviewer skill\n---\n# Reviewer\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Skills: config.GlobalSkillsConfig{
			Models: map[string]config.ModelSkillsConfig{
				"openai/gpt-4o": {Deny: []string{"reviewer"}},
			},
		},
	}
	builder := NewSystemPromptBuilder(SystemPromptBuilderConfig{
		ProjectDir: tmpDir,
		SkillsDir:  skillsDir,
		Config:     cfg,
	})
	parts, err := builder.Build(context.Background(), SystemPromptBuildInput{
		ProviderID: "openai",
		ModelID:    "gpt-4o",
	})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(parts[1], "Hidden reviewer skill") {
		t.Fatalf("prompt should not include model-denied skill: %q", parts[1])
	}
}

func TestBuildSystemPrompt_RelevantSkillsOverlay(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := t.TempDir()
	skillDir := filepath.Join(skillsDir, "db-migration")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `---
name: db-migration
description: Database migration guidance
triggers:
  - database migration
suggests:
  before: [schema-review]
  after: [go-testing]
conditions:
  files:
    - db/*.sql
---
# Migration
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	builder := NewSystemPromptBuilder(SystemPromptBuilderConfig{
		ProjectDir: tmpDir,
		SkillsDir:  skillsDir,
	})
	parts, err := builder.Build(context.Background(), SystemPromptBuildInput{
		ProviderID:   "openai",
		ModelID:      "gpt-4o",
		UserMessage:  "Plan the database migration for this schema change",
		TouchedFiles: []string{"db/001_init.sql"},
		Agent: config.AgentConfig{
			Tools: []string{"skill", "search_skills"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(parts[2], "Likely Relevant Skills For This Turn") {
		t.Fatalf("volatile prompt missing relevant-skill overlay: %q", parts[2])
	}
	if !strings.Contains(parts[2], "db-migration") {
		t.Fatalf("volatile prompt missing matching skill: %q", parts[2])
	}
	if !strings.Contains(parts[2], "Load before: schema-review") {
		t.Fatalf("volatile prompt missing before hint: %q", parts[2])
	}
	if !strings.Contains(parts[2], "Consider after: go-testing") {
		t.Fatalf("volatile prompt missing after hint: %q", parts[2])
	}
}

func TestBuildSystemPrompt_RelevantToolsOverlay(t *testing.T) {
	tmpDir := t.TempDir()
	builder := NewSystemPromptBuilder(SystemPromptBuilderConfig{
		ProjectDir: tmpDir,
		SkillsDir:  t.TempDir(),
	})

	parts, err := builder.Build(context.Background(), SystemPromptBuildInput{
		ProviderID:  "openai",
		ModelID:     "gpt-4o",
		UserMessage: "Find all references to SessionService and inspect the callers",
		Tools: []provider.Tool{
			{Name: "edit", Description: "Precise file edits"},
			{Name: "lsp", Description: "Symbol lookup, references, diagnostics"},
			{Name: "search", Description: "Broad code search"},
			{Name: "bash", Description: "Run shell commands"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(parts[2], "Likely Relevant Tools For This Turn") {
		t.Fatalf("volatile prompt missing relevant-tool overlay: %q", parts[2])
	}
	if !strings.Contains(parts[2], "- lsp:") {
		t.Fatalf("volatile prompt missing lsp hint: %q", parts[2])
	}
	if !strings.Contains(parts[2], "- search:") {
		t.Fatalf("volatile prompt missing search hint: %q", parts[2])
	}
}

func TestBuildInstructionNamespace(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "submodule")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	builder := newTestBuilder(t, subDir)

	namespace := builder.buildInstructionNamespace(filepath.Join(tmpDir, "AGENTS.md"))
	expected := "[..]"
	if namespace != expected {
		t.Errorf("buildInstructionNamespace() = %q, want %q", namespace, expected)
	}
}

func TestBuildInstructionNamespace_Root(t *testing.T) {
	tmpDir := t.TempDir()

	builder := newTestBuilder(t, tmpDir)

	namespace := builder.buildInstructionNamespace(filepath.Join(tmpDir, "AGENTS.md"))
	if namespace != "" {
		t.Errorf("buildInstructionNamespace() = %q, want empty for root", namespace)
	}
}

func TestBoolStr(t *testing.T) {
	if boolStr(true) != "yes" {
		t.Error("boolStr(true) should be 'yes'")
	}
	if boolStr(false) != "no" {
		t.Error("boolStr(false) should be 'no'")
	}
}

type fakeAgentLister struct {
	agents []agent.Agent
}

func (f fakeAgentLister) List() []agent.Agent { return f.agents }
