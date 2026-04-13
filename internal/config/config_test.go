package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/permission"
)

// writeYAML creates a temporary YAML file with the given content and returns
// its path. The file is removed when the test ends.
func writeYAML(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeYAML: write %s: %v", path, err)
	}
	return path
}

func TestDefaults(t *testing.T) {
	// Point to an empty dir so no files are loaded.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.DefaultAgent != "builder" {
		t.Errorf("DefaultAgent = %q, want %q", cfg.DefaultAgent, "builder")
	}
	if cfg.Session.CompactionThreshold == nil || *cfg.Session.CompactionThreshold != 0.8 {
		t.Errorf("Session.CompactionThreshold = %v, want 0.8", cfg.Session.CompactionThreshold)
	}
	if cfg.Session.CompactionKeepTurns == nil || *cfg.Session.CompactionKeepTurns != 10 {
		t.Errorf("Session.CompactionKeepTurns = %v, want 10", cfg.Session.CompactionKeepTurns)
	}
	if cfg.Server.Port != 0 {
		t.Errorf("Server.Port = %d, want 0", cfg.Server.Port)
	}
	if cfg.TUI.InputMaxHeight != 8 {
		t.Errorf("TUI.InputMaxHeight = %d, want 8", cfg.TUI.InputMaxHeight)
	}
	if cfg.TUI.MaxAttachmentSize != 20*1024*1024 {
		t.Errorf("TUI.MaxAttachmentSize = %d, want %d", cfg.TUI.MaxAttachmentSize, 20*1024*1024)
	}
}

func TestLoadGlobalConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	kodacodeDir := filepath.Join(dir, "kodacode")
	if err := os.MkdirAll(kodacodeDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", kodacodeDir, err)
	}

	writeYAML(t, kodacodeDir, "config.yaml", `
default_agent: builder
providers:
  - id: groq
    api_key: test-key
    base_url: https://api.groq.com/openai/v1
session:
  compaction_threshold: 0.9
  compaction_keep_turns: 5
  context_limit: 0.95
server:
  port: 8080
tui:
  input_max_height: 12
  max_attachment_size: 52428800
`)

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.DefaultAgent != "builder" {
		t.Errorf("DefaultAgent = %q, want %q", cfg.DefaultAgent, "builder")
	}
	if cfg.Session.CompactionThreshold == nil || *cfg.Session.CompactionThreshold != 0.9 {
		t.Errorf("Session.CompactionThreshold = %v, want 0.9", cfg.Session.CompactionThreshold)
	}
	if cfg.Session.CompactionKeepTurns == nil || *cfg.Session.CompactionKeepTurns != 5 {
		t.Errorf("Session.CompactionKeepTurns = %v, want 5", cfg.Session.CompactionKeepTurns)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.TUI.InputMaxHeight != 12 {
		t.Errorf("TUI.InputMaxHeight = %d, want 12", cfg.TUI.InputMaxHeight)
	}
	if cfg.TUI.MaxAttachmentSize != 50*1024*1024 {
		t.Errorf("TUI.MaxAttachmentSize = %d, want %d", cfg.TUI.MaxAttachmentSize, 50*1024*1024)
	}
	if len(cfg.Providers) != 1 || cfg.Providers[0].ID != "groq" {
		t.Errorf("Providers = %+v, want [{ID:groq ...}]", cfg.Providers)
	}
}

func TestProjectConfigOverridesGlobal(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	globalDir := filepath.Join(xdgDir, "kodacode")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", globalDir, err)
	}

	writeYAML(t, globalDir, "config.yaml", `
providers:
  - id: openai
    api_key: global-key
    base_url: https://api.openai.com/v1
`)

	projectDir := t.TempDir()
	writeYAML(t, projectDir, "kodacode.yaml", `
providers:
  - id: openai
    api_key: project-key
    base_url: https://api.openai.com/v1
`)

	cfg, err := config.Load(projectDir)
	if err != nil {
		t.Fatalf("Load(%q) error = %v, want nil", projectDir, err)
	}

	// Provider with same ID should be overridden.
	if len(cfg.Providers) != 1 {
		t.Fatalf("len(Providers) = %d, want 1", len(cfg.Providers))
	}
	if cfg.Providers[0].APIKey != "project-key" {
		t.Errorf("Providers[0].APIKey = %q, want %q", cfg.Providers[0].APIKey, "project-key")
	}
}

func TestProvidersMergeByID(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	globalDir := filepath.Join(xdgDir, "kodacode")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", globalDir, err)
	}

	writeYAML(t, globalDir, "config.yaml", `
providers:
  - id: openai
    api_key: key-a
    base_url: https://api.openai.com/v1
  - id: groq
    api_key: key-b
    base_url: https://api.groq.com/openai/v1
`)

	projectDir := t.TempDir()
	writeYAML(t, projectDir, "kodacode.yaml", `
providers:
  - id: groq
    api_key: key-b-override
    base_url: https://api.groq.com/openai/v1
  - id: ollama
    api_key: ""
    base_url: http://localhost:11434/v1
`)

	cfg, err := config.Load(projectDir)
	if err != nil {
		t.Fatalf("Load(%q) error = %v, want nil", projectDir, err)
	}

	want := []config.ProviderConfig{
		{ID: "openai", APIKey: "key-a", BaseURL: "https://api.openai.com/v1"},
		{ID: "groq", APIKey: "key-b-override", BaseURL: "https://api.groq.com/openai/v1"},
		{ID: "ollama", APIKey: "", BaseURL: "http://localhost:11434/v1"},
	}
	if diff := cmp.Diff(want, cfg.Providers); diff != "" {
		t.Errorf("Providers mismatch (-want +got):\n%s", diff)
	}
}

func TestEnvVarExpansionInAPIKey(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	globalDir := filepath.Join(xdgDir, "kodacode")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", globalDir, err)
	}

	t.Setenv("TEST_OPENAI_KEY", "sk-secret-123")
	writeYAML(t, globalDir, "config.yaml", `
providers:
  - id: openai
    api_key: ${TEST_OPENAI_KEY}
    base_url: https://api.openai.com/v1
`)

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if len(cfg.Providers) == 0 {
		t.Fatal("Providers is empty, want 1 provider")
	}
	if cfg.Providers[0].APIKey != "sk-secret-123" {
		t.Errorf("Providers[0].APIKey = %q, want %q", cfg.Providers[0].APIKey, "sk-secret-123")
	}
}

func TestLoadMCPToolHints(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	globalDir := filepath.Join(xdgDir, "kodacode")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", globalDir, err)
	}

	writeYAML(t, globalDir, "config.yaml", `
mcp:
  servers:
    - name: github
      type: sse
      url: https://example.com/mcp
      tool_hints:
        "*":
          guidance: Use for GitHub-hosted systems only.
        search:
          summary: Search GitHub issues and PRs
          triggers:
            - github issues
            - pull request search
`)

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if len(cfg.MCP.Servers) != 1 {
		t.Fatalf("len(MCP.Servers) = %d, want 1", len(cfg.MCP.Servers))
	}
	hints := cfg.MCP.Servers[0].ToolHints
	if hints["search"].Summary != "Search GitHub issues and PRs" {
		t.Fatalf("search summary = %q", hints["search"].Summary)
	}
	if len(hints["search"].Triggers) != 2 {
		t.Fatalf("search triggers = %v, want 2 entries", hints["search"].Triggers)
	}
	if hints["*"].Guidance != "Use for GitHub-hosted systems only." {
		t.Fatalf("wildcard guidance = %q", hints["*"].Guidance)
	}
}

func TestProjectConfigCanOverrideZeroValueScalars(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	globalDir := filepath.Join(xdgDir, "kodacode")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", globalDir, err)
	}

	writeYAML(t, globalDir, "config.yaml", `
session:
  budget: 12.5
  budget_warn: 0.9
  total_budget: 99
  total_budget_warn: 0.8
  primary_max_steps: 20
  subagent_max_steps: 8
server:
  port: 8080
tui:
  auto_resume: false
`)

	projectDir := t.TempDir()
	writeYAML(t, projectDir, "kodacode.yaml", `
session:
  budget: 0
  budget_warn: 0
  total_budget: 0
  total_budget_warn: 0
  primary_max_steps: 0
  subagent_max_steps: 0
server:
  port: 0
tui:
  auto_resume: true
`)

	cfg, err := config.Load(projectDir)
	if err != nil {
		t.Fatalf("Load(%q) error = %v, want nil", projectDir, err)
	}

	if cfg.Session.Budget != 0 {
		t.Fatalf("Session.Budget = %v, want 0", cfg.Session.Budget)
	}
	if cfg.Session.BudgetWarn != 0 {
		t.Fatalf("Session.BudgetWarn = %v, want 0", cfg.Session.BudgetWarn)
	}
	if cfg.Session.TotalBudget != 0 {
		t.Fatalf("Session.TotalBudget = %v, want 0", cfg.Session.TotalBudget)
	}
	if cfg.Session.TotalBudgetWarn != 0 {
		t.Fatalf("Session.TotalBudgetWarn = %v, want 0", cfg.Session.TotalBudgetWarn)
	}
	if cfg.Session.PrimaryMaxSteps != 0 {
		t.Fatalf("Session.PrimaryMaxSteps = %d, want 0", cfg.Session.PrimaryMaxSteps)
	}
	if cfg.Session.SubagentMaxSteps != 0 {
		t.Fatalf("Session.SubagentMaxSteps = %d, want 0", cfg.Session.SubagentMaxSteps)
	}
	if cfg.Server.Port != 0 {
		t.Fatalf("Server.Port = %d, want 0", cfg.Server.Port)
	}
	if !cfg.TUI.AutoResume {
		t.Fatal("TUI.AutoResume = false, want true")
	}
}

func TestUnsetAPIKeyEnvVarReturnsError(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	globalDir := filepath.Join(xdgDir, "kodacode")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", globalDir, err)
	}

	writeYAML(t, globalDir, "config.yaml", `
providers:
  - id: openai
    api_key: ${KODACODE_TEST_UNSET_VAR_XYZ}
    base_url: https://api.openai.com/v1
`)

	_, err := config.Load("")
	if err == nil {
		t.Fatal("Load() should return error for unset env var in api_key")
	}
	if !strings.Contains(err.Error(), "unset environment variable") {
		t.Errorf("error should mention unset env var, got: %v", err)
	}
}

func TestLiteralAPIKeyWithDollarPreserved(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	globalDir := filepath.Join(xdgDir, "kodacode")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", globalDir, err)
	}

	writeYAML(t, globalDir, "config.yaml", `
providers:
  - id: custom
    api_key: "token$weird_key"
    base_url: http://localhost:8080
`)

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := "token$weird_key"
	if cfg.Providers[0].APIKey != want {
		t.Errorf("APIKey = %q, want %q (should not expand bare $)", cfg.Providers[0].APIKey, want)
	}
}

func TestMissingConfigFilesAreIgnored(t *testing.T) {
	// XDG dir with no kodacode/ subdir.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	// Should not return an error for missing files.
	_, err := config.Load("")
	if err != nil {
		t.Errorf("Load() error = %v, want nil", err)
	}
}

func TestProviderByID(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	globalDir := filepath.Join(xdgDir, "kodacode")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", globalDir, err)
	}

	writeYAML(t, globalDir, "config.yaml", `
providers:
  - id: openai
    api_key: key-a
    base_url: https://api.openai.com/v1
`)

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	t.Run("found", func(t *testing.T) {
		p, err := cfg.ProviderByID("openai")
		if err != nil {
			t.Fatalf("ProviderByID(%q) error = %v, want nil", "openai", err)
		}
		if p.APIKey != "key-a" {
			t.Errorf("ProviderByID(%q).APIKey = %q, want %q", "openai", p.APIKey, "key-a")
		}
	})

	t.Run("not_found", func(t *testing.T) {
		_, err := cfg.ProviderByID("nonexistent")
		if err == nil {
			t.Errorf("ProviderByID(%q) error = nil, want non-nil", "nonexistent")
		}
	})
}

func TestLoadPermissionFromConfig(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	globalDir := filepath.Join(xdgDir, "kodacode")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", globalDir, err)
	}

	writeYAML(t, globalDir, "config.yaml", `
permission:
  bash: deny
  edit: allow
`)

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	bashRule, ok := cfg.Permission["bash"]
	if !ok {
		t.Fatalf("cfg.Permission missing key %q", "bash")
	}
	if bashRule.Action != permission.ActionDeny {
		t.Errorf("cfg.Permission[\"bash\"].Action = %q, want %q", bashRule.Action, permission.ActionDeny)
	}

	editRule, ok := cfg.Permission["edit"]
	if !ok {
		t.Fatalf("cfg.Permission missing key %q", "edit")
	}
	if editRule.Action != permission.ActionAllow {
		t.Errorf("cfg.Permission[\"edit\"].Action = %q, want %q", editRule.Action, permission.ActionAllow)
	}
}

func TestProjectConfigPermissionOverridesGlobal(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	globalDir := filepath.Join(xdgDir, "kodacode")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", globalDir, err)
	}

	writeYAML(t, globalDir, "config.yaml", `
permission:
  bash: ask
`)

	projectDir := t.TempDir()
	writeYAML(t, projectDir, "kodacode.yaml", `
permission:
  bash: deny
`)

	cfg, err := config.Load(projectDir)
	if err != nil {
		t.Fatalf("Load(%q) error = %v, want nil", projectDir, err)
	}

	if _, ok := cfg.Permission["bash"]; !ok {
		t.Fatalf("cfg.Permission missing key %q", "bash")
	}
	// After pattern-level merge, both layers become patterns.
	// Project's [* → deny] comes last and wins.
	got := permission.Resolve(cfg.Permission, "bash", "echo hello")
	if got != permission.ActionDeny {
		t.Errorf("Resolve(bash) = %q, want %q (project should override global)", got, permission.ActionDeny)
	}
}

func TestResolvedPermission_DefaultsApplied(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	resolved := cfg.ResolvedPermission(nil)

	// bash default is allow
	got := permission.Resolve(resolved, "bash", "echo hello")
	if got != permission.ActionAllow {
		t.Errorf("ResolvedPermission(nil) bash = %q, want %q", got, permission.ActionAllow)
	}

	// .env default is ask
	got = permission.Resolve(resolved, "read", ".env")
	if got != permission.ActionAsk {
		t.Errorf("ResolvedPermission(nil) read .env = %q, want %q", got, permission.ActionAsk)
	}
}

func TestResolvedPermission_ConfigOverridesDefaults(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	globalDir := filepath.Join(xdgDir, "kodacode")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", globalDir, err)
	}

	writeYAML(t, globalDir, "config.yaml", `
permission:
  bash: deny
`)

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	resolved := cfg.ResolvedPermission(nil)

	got := permission.Resolve(resolved, "bash", "echo hello")
	if got != permission.ActionDeny {
		t.Errorf("ResolvedPermission(nil) bash = %q, want %q", got, permission.ActionDeny)
	}
}

func TestResolvedPermission_AgentPermissionOverridesConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	agentPerm := permission.Config{
		"bash": &permission.Rule{Action: permission.ActionAsk},
	}

	resolved := cfg.ResolvedPermission(agentPerm)

	got := permission.Resolve(resolved, "bash", "echo hello")
	if got != permission.ActionAsk {
		t.Errorf("ResolvedPermission(agentPerm) bash = %q, want %q", got, permission.ActionAsk)
	}
}

func TestMerge_ModelSessionConfig_FieldByField(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	globalDir := filepath.Join(xdgDir, "kodacode")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", globalDir, err)
	}
	writeYAML(t, globalDir, "config.yaml", `
session:
  models:
    prov/model:
      compaction_threshold: 0.8
      prune_protect_tokens: 5000
`)

	projectDir := t.TempDir()
	writeYAML(t, projectDir, "kodacode.yaml", `
session:
  models:
    prov/model:
      compaction_keep_turns: 5
      prune_min_savings: 1000
      max_input_tokens: 4096
`)

	cfg, err := config.Load(projectDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	mc := cfg.Session.Models["prov/model"]

	if mc.CompactionThreshold == nil || *mc.CompactionThreshold != 0.8 {
		t.Errorf("CompactionThreshold = %v, want 0.8 (preserved from global)", mc.CompactionThreshold)
	}
	if mc.PruneProtectTokens == nil || *mc.PruneProtectTokens != 5000 {
		t.Errorf("PruneProtectTokens = %v, want 5000 (preserved from global)", mc.PruneProtectTokens)
	}
	if mc.CompactionKeepTurns == nil || *mc.CompactionKeepTurns != 5 {
		t.Errorf("CompactionKeepTurns = %v, want 5 (from project)", mc.CompactionKeepTurns)
	}
	if mc.PruneMinSavings == nil || *mc.PruneMinSavings != 1000 {
		t.Errorf("PruneMinSavings = %v, want 1000 (from project)", mc.PruneMinSavings)
	}
	if mc.MaxInputTokens != 4096 {
		t.Errorf("MaxInputTokens = %d, want 4096 (from project)", mc.MaxInputTokens)
	}
}

func TestSessionModelConfig_PrefersQualifiedKey(t *testing.T) {
	cfg := &config.Config{
		Session: config.SessionConfig{
			Models: map[string]config.ModelSessionConfig{
				"openai/gpt-4o": {MaxInputTokens: 64000},
				"gpt-4o":        {MaxInputTokens: 32000},
			},
		},
	}

	mc, ok := cfg.SessionModelConfig("openai", "gpt-4o")
	if !ok {
		t.Fatal("SessionModelConfig() ok = false, want true")
	}
	if mc.MaxInputTokens != 64000 {
		t.Fatalf("MaxInputTokens = %d, want 64000", mc.MaxInputTokens)
	}
}

func TestSessionModelConfig_FallsBackToBareKey(t *testing.T) {
	cfg := &config.Config{
		Session: config.SessionConfig{
			Models: map[string]config.ModelSessionConfig{
				"gpt-4o": {MaxInputTokens: 32000},
			},
		},
	}

	mc, ok := cfg.SessionModelConfig("openai", "gpt-4o")
	if !ok {
		t.Fatal("SessionModelConfig() ok = false, want true")
	}
	if mc.MaxInputTokens != 32000 {
		t.Fatalf("MaxInputTokens = %d, want 32000", mc.MaxInputTokens)
	}
}

func TestLoad_ToolCallArgumentTimeoutOverrides(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	projectDir := t.TempDir()
	writeYAML(t, projectDir, "kodacode.yaml", `
session:
  tool_call_argument_timeout: 240
  models:
    ollama/qwen2.5-coder:
      tool_call_argument_timeout: 900
`)

	cfg, err := config.Load(projectDir)
	if err != nil {
		t.Fatalf("Load(%q) error = %v, want nil", projectDir, err)
	}
	if cfg.Session.ToolCallArgumentTimeout != 240 {
		t.Fatalf("Session.ToolCallArgumentTimeout = %d, want 240", cfg.Session.ToolCallArgumentTimeout)
	}
	mc, ok := cfg.SessionModelConfig("ollama", "qwen2.5-coder")
	if !ok {
		t.Fatal("SessionModelConfig() ok = false, want true")
	}
	if mc.ToolCallArgumentTimeout != 900 {
		t.Fatalf("ToolCallArgumentTimeout = %d, want 900", mc.ToolCallArgumentTimeout)
	}
}

func TestLoad_EngineerExecutionRetryLimitOverride(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	projectDir := t.TempDir()
	writeYAML(t, projectDir, "kodacode.yaml", `
session:
  engineer_execution_retry_limit: 9
`)

	cfg, err := config.Load(projectDir)
	if err != nil {
		t.Fatalf("Load(%q) error = %v, want nil", projectDir, err)
	}
	if cfg.Session.EngineerExecutionRetryLimit != 9 {
		t.Fatalf("Session.EngineerExecutionRetryLimit = %d, want 9", cfg.Session.EngineerExecutionRetryLimit)
	}
}

func TestMerge_LSPServers_ByNameFieldByField(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	globalDir := filepath.Join(xdgDir, "kodacode")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", globalDir, err)
	}
	writeYAML(t, globalDir, "config.yaml", `
lsp:
  servers:
    - name: gopls
      command: gopls
      args: ["serve"]
      extensions: [".go"]
    - name: vtsls
      command: vtsls
      args: ["--stdio"]
      extensions: [".ts", ".tsx"]
`)

	projectDir := t.TempDir()
	writeYAML(t, projectDir, "kodacode.yaml", `
lsp:
  servers:
    - name: gopls
      enabled: false
    - name: pyright
      command: pyright-langserver
      args: ["--stdio"]
      extensions: [".py"]
`)

	cfg, err := config.Load(projectDir)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if len(cfg.LSP.Servers) != 3 {
		t.Fatalf("len(LSP.Servers) = %d, want 3", len(cfg.LSP.Servers))
	}

	var gopls, vtsls, pyright *config.LSPServerConfig
	for i := range cfg.LSP.Servers {
		switch cfg.LSP.Servers[i].Name {
		case "gopls":
			gopls = &cfg.LSP.Servers[i]
		case "vtsls":
			vtsls = &cfg.LSP.Servers[i]
		case "pyright":
			pyright = &cfg.LSP.Servers[i]
		}
	}
	if gopls == nil || vtsls == nil || pyright == nil {
		t.Fatalf("expected merged LSP servers gopls/vtsls/pyright, got %+v", cfg.LSP.Servers)
	}
	if gopls.Command != "gopls" {
		t.Fatalf("gopls.Command = %q, want %q", gopls.Command, "gopls")
	}
	if diff := cmp.Diff([]string{"serve"}, gopls.Args); diff != "" {
		t.Fatalf("gopls.Args mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{".go"}, gopls.Extensions); diff != "" {
		t.Fatalf("gopls.Extensions mismatch (-want +got):\n%s", diff)
	}
	if gopls.Enabled == nil || *gopls.Enabled {
		t.Fatalf("gopls.Enabled = %v, want false", gopls.Enabled)
	}
	if vtsls.Command != "vtsls" {
		t.Fatalf("vtsls.Command = %q, want %q", vtsls.Command, "vtsls")
	}
	if pyright.Command != "pyright-langserver" {
		t.Fatalf("pyright.Command = %q, want %q", pyright.Command, "pyright-langserver")
	}
}

func TestMerge_SkillsModels_ByModelKey(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	globalDir := filepath.Join(xdgDir, "kodacode")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", globalDir, err)
	}
	writeYAML(t, globalDir, "config.yaml", `
skills:
  models:
    openai/gpt-4o:
      allow_all: true
      deny: ["dangerous-skill"]
    anthropic/claude-sonnet:
      deny: ["secret-skill"]
`)

	projectDir := t.TempDir()
	writeYAML(t, projectDir, "kodacode.yaml", `
skills:
  models:
    openai/gpt-4o:
      allow_all: false
      deny: ["slow-skill", "dangerous-skill"]
    google/gemini-2.5-pro:
      allow_all: true
`)

	cfg, err := config.Load(projectDir)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if len(cfg.Skills.Models) != 3 {
		t.Fatalf("len(Skills.Models) = %d, want 3", len(cfg.Skills.Models))
	}
	openaiSkills := cfg.Skills.Models["openai/gpt-4o"]
	if openaiSkills.AllowAll {
		t.Fatal("openai/gpt-4o AllowAll = true, want false from project override")
	}
	if diff := cmp.Diff([]string{"dangerous-skill", "slow-skill"}, openaiSkills.Deny); diff != "" {
		t.Fatalf("openai/gpt-4o deny mismatch (-want +got):\n%s", diff)
	}
	anthropicSkills := cfg.Skills.Models["anthropic/claude-sonnet"]
	if diff := cmp.Diff([]string{"secret-skill"}, anthropicSkills.Deny); diff != "" {
		t.Fatalf("anthropic/claude-sonnet deny mismatch (-want +got):\n%s", diff)
	}
	googleSkills := cfg.Skills.Models["google/gemini-2.5-pro"]
	if !googleSkills.AllowAll {
		t.Fatal("google/gemini-2.5-pro AllowAll = false, want true")
	}
}

func TestValidate_PerModelContextLimitVsThreshold(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	globalDir := filepath.Join(xdgDir, "kodacode")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", globalDir, err)
	}
	writeYAML(t, globalDir, "config.yaml", `
session:
  models:
    prov/model:
      compaction_threshold: 0.9
      context_limit: 0.8
`)

	_, err := config.Load("")
	if err == nil {
		t.Fatal("expected validation error for context_limit <= compaction_threshold")
	}
	if !strings.Contains(err.Error(), "context_limit") {
		t.Errorf("error should mention context_limit, got: %v", err)
	}
}

func TestValidate_KeepTurnsMinimum(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	globalDir := filepath.Join(xdgDir, "kodacode")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", globalDir, err)
	}
	writeYAML(t, globalDir, "config.yaml", `
session:
  compaction_keep_turns: 0
`)

	_, err := config.Load("")
	if err == nil {
		t.Fatal("expected validation error for compaction_keep_turns=0")
	}
	if !strings.Contains(err.Error(), "compaction_keep_turns") {
		t.Errorf("error should mention compaction_keep_turns, got: %v", err)
	}
}
