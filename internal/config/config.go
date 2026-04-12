// Package config handles loading and merging of kodacode configuration files.
package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/sageil/kodacode/v1/internal/permission"
)

func defaults() Config {
	return Config{
		Version:      CurrentConfigVersion,
		DefaultAgent: "builder",
		Session: SessionConfig{
			CompactionThreshold:     float64Ptr(0.8),
			CompactionKeepTurns:     intPtr(10),
			PruneProtectTokens:      intPtr(40000),
			PruneMinSavings:         intPtr(20000),
			ContextLimit:            float64Ptr(0.9),
			MaxRetries:              5,
			ToolCallArgumentTimeout: 300,
			EngineerReviewLimit:     3,
			MaxSubagents:            10,
			BackgroundAutoReact:     boolPtr(true),
			Trace:                   boolPtr(false),
		},
		Server: ServerConfig{
			Port: 0,
		},
		TUI: TUIConfig{
			InputMaxHeight:    8,
			DisplayTurns:      4,
			ErrorDisplayTime:  3,
			MaxAttachmentSize: 20 * 1024 * 1024, // 20 MB
		},
		ModelRefreshInterval: 7,
		AllowedPaths:         []string{ConfigDir()},
		IgnorePatterns: []string{
			"node_modules/**",
			".git/**",
			"__pycache__/**",
			".next/**",
			"dist/**",
			"build/**",
			"vendor/**",
			".cache/**",
			"coverage/**",
			".tox/**",
			".venv/**",
			"venv/**",
			".gradle/**",
			"target/**",
			".idea/**",
			".vscode/**",
		},
	}
}

// DataDir returns the platform-specific data directory for kodacode.
func DataDir() string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "kodacode")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "kodacode")
}

func ConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "kodacode")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "kodacode-config")
	}
	return filepath.Join(home, ".config", "kodacode")
}

func ThemesDir() string {
	return filepath.Join(ConfigDir(), "themes")
}

func Save(cfg *Config) error {
	path := filepath.Join(ConfigDir(), "config.yaml")
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}

// Load reads and merges configuration from:
//  1. Compiled-in defaults
//  2. Global config (~/.config/kodacode/config.yaml)
//  3. Project-local config (<projectDir>/kodacode.yaml), if projectDir is non-empty
//
// Later sources override earlier ones for scalar fields. Provider slices are
// merged by ID (later entry wins on conflict).
func Load(projectDir string) (*Config, error) {
	cfg := defaults()

	// 1. Global config.
	globalPath := filepath.Join(ConfigDir(), "config.yaml")
	if err := loadFile(globalPath, &cfg); err != nil {
		return nil, fmt.Errorf("config: load global: %w", err)
	}

	// 2. Project-local config.
	if projectDir != "" {
		localPath := filepath.Join(projectDir, "kodacode.yaml")
		if err := loadFile(localPath, &cfg); err != nil {
			return nil, fmt.Errorf("config: load project: %w", err)
		}
	}

	for i := range cfg.Providers {
		raw := cfg.Providers[i].APIKey
		if hasEnvVarRef(raw) {
			expanded := expandBracedEnvVars(raw)
			if expanded == "" {
				return nil, fmt.Errorf("config: provider %q: api_key references an unset environment variable", cfg.Providers[i].ID)
			}
			cfg.Providers[i].APIKey = expanded
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// LayeredConfig holds the three config layers separately so a viewer can
// show which file each value originated from.
type LayeredConfig struct {
	Default *Config // compiled-in defaults
	Global  *Config // global config merged on top of defaults (nil if no global file)
	Project *Config // project config merged on top of global+defaults (nil if no project file)
	Merged  *Config // final merged result
}

// LoadLayered loads each config layer separately for display purposes.
// It does NOT expand environment variables or validate — the result is
// intended for UI inspection, not for running the application.
func LoadLayered(projectDir string) LayeredConfig {
	result := LayeredConfig{}

	// Defaults.
	d := defaults()
	result.Default = &d

	// Global = defaults + global file.
	globalPath := filepath.Join(ConfigDir(), "config.yaml")
	g := defaults()
	if err := loadFile(globalPath, &g); err == nil {
		if hasGlobalFile(globalPath) {
			result.Global = &g
		}
	}

	// Merged = defaults + global + project.
	m := defaults()
	_ = loadFile(globalPath, &m)
	if projectDir != "" {
		localPath := filepath.Join(projectDir, "kodacode.yaml")
		p := defaults()
		_ = loadFile(globalPath, &p)
		if err := loadFile(localPath, &p); err == nil {
			if hasProjectFile(projectDir) {
				result.Project = &p
			}
		}
		_ = loadFile(localPath, &m)
	}
	result.Merged = &m
	return result
}

func hasGlobalFile(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func hasProjectFile(projectDir string) bool {
	_, err := os.Stat(filepath.Join(projectDir, "kodacode.yaml"))
	return err == nil
}

func loadFile(path string, dst *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	return parse(data, dst)
}

func parse(data []byte, dst *Config) error {
	var overlay configOverlay
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&overlay); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	merge(dst, &overlay)
	return nil
}

// merge applies non-zero fields from overlay onto dst.
// Providers are merged by ID; overlay entries override existing ones with the
// same ID, and new IDs are appended.
func merge(dst *Config, overlay *configOverlay) {
	if overlay.Version != nil {
		dst.Version = *overlay.Version
	}
	if overlay.UtilityModel != nil {
		dst.UtilityModel = *overlay.UtilityModel
	}
	if overlay.ReviewerModel != nil {
		dst.ReviewerModel = *overlay.ReviewerModel
	}
	if overlay.DefaultAgent != nil {
		dst.DefaultAgent = *overlay.DefaultAgent
	}
	mergeSessionConfig(&dst.Session, overlay.Session)
	mergeServerConfig(&dst.Server, overlay.Server)
	mergeTUIConfig(&dst.TUI, overlay.TUI)

	if overlay.MemoryBudget != nil {
		dst.MemoryBudget = *overlay.MemoryBudget
	}
	if overlay.ModelRefreshInterval != nil {
		dst.ModelRefreshInterval = *overlay.ModelRefreshInterval
	}
	mergeProviderConfigs(&dst.Providers, overlay.Providers)

	// Permission: merge per-tool (overlay replaces base tool-by-tool).
	if len(overlay.Permission) > 0 {
		dst.Permission = permission.Merge(dst.Permission, overlay.Permission)
	}
	mergeMCPServers(&dst.MCP.Servers, overlay.MCP.Servers)

	if len(overlay.LSP.Servers) > 0 {
		mergeLSPServers(&dst.LSP.Servers, overlay.LSP.Servers)
	}

	if overlay.Diagnostics.Enabled != nil {
		dst.Diagnostics = overlay.Diagnostics
	}

	if overlay.SearchIndex.Enabled != nil {
		dst.SearchIndex = overlay.SearchIndex
	}

	if len(overlay.Skills.Models) > 0 {
		mergeGlobalSkillsConfig(&dst.Skills, overlay.Skills)
	}

	if overlay.Debug != nil {
		dst.Debug = overlay.Debug
	}
	if overlay.LogPrompts != nil {
		dst.LogPrompts = overlay.LogPrompts
	}

	if len(overlay.FallbackModels) > 0 {
		dst.FallbackModels = overlay.FallbackModels
	}

	if len(overlay.InstructionFiles) > 0 {
		dst.InstructionFiles = overlay.InstructionFiles
	}
	if overlay.MaxInstructionSize != nil {
		dst.MaxInstructionSize = *overlay.MaxInstructionSize
	}
	dst.AllowedPaths = appendUniqueStrings(dst.AllowedPaths, overlay.AllowedPaths)
	dst.IgnorePatterns = appendUniqueStrings(dst.IgnorePatterns, overlay.IgnorePatterns)
}

func mergeSessionConfig(dst *SessionConfig, overlay sessionConfigOverlay) {
	if overlay.CompactionThreshold != nil {
		dst.CompactionThreshold = overlay.CompactionThreshold
	}
	if overlay.CompactionKeepTurns != nil {
		dst.CompactionKeepTurns = overlay.CompactionKeepTurns
	}
	if overlay.PruneProtectTokens != nil {
		dst.PruneProtectTokens = overlay.PruneProtectTokens
	}
	if overlay.PruneMinSavings != nil {
		dst.PruneMinSavings = overlay.PruneMinSavings
	}
	if overlay.ContextLimit != nil {
		dst.ContextLimit = overlay.ContextLimit
	}
	if overlay.MaxRetries != nil {
		dst.MaxRetries = *overlay.MaxRetries
	}
	if overlay.ToolCallArgumentTimeout != nil {
		dst.ToolCallArgumentTimeout = *overlay.ToolCallArgumentTimeout
	}
	if overlay.EngineerReviewLimit != nil {
		dst.EngineerReviewLimit = *overlay.EngineerReviewLimit
	}
	if overlay.MaxSubagents != nil {
		dst.MaxSubagents = *overlay.MaxSubagents
	}
	if overlay.SubagentTimeout != nil {
		dst.SubagentTimeout = *overlay.SubagentTimeout
	}
	if overlay.PlanApproval != nil {
		dst.PlanApproval = overlay.PlanApproval
	}
	if overlay.BackgroundAutoReact != nil {
		dst.BackgroundAutoReact = overlay.BackgroundAutoReact
	}
	if overlay.Snapshot != nil {
		dst.Snapshot = overlay.Snapshot
	}
	if overlay.Trace != nil {
		dst.Trace = overlay.Trace
	}
	if overlay.Budget != nil {
		dst.Budget = *overlay.Budget
	}
	if overlay.BudgetWarn != nil {
		dst.BudgetWarn = *overlay.BudgetWarn
	}
	if overlay.TotalBudget != nil {
		dst.TotalBudget = *overlay.TotalBudget
	}
	if overlay.TotalBudgetWarn != nil {
		dst.TotalBudgetWarn = *overlay.TotalBudgetWarn
	}
	if overlay.PrimaryMaxSteps != nil {
		dst.PrimaryMaxSteps = *overlay.PrimaryMaxSteps
	}
	if overlay.SubagentMaxSteps != nil {
		dst.SubagentMaxSteps = *overlay.SubagentMaxSteps
	}
	if len(overlay.Models) == 0 {
		return
	}
	if dst.Models == nil {
		dst.Models = make(map[string]ModelSessionConfig)
	}
	for k, v := range overlay.Models {
		dst.Models[k] = mergeModelSessionConfig(dst.Models[k], v)
	}
}

func mergeServerConfig(dst *ServerConfig, overlay serverConfigOverlay) {
	if overlay.Port != nil {
		dst.Port = *overlay.Port
	}
}

func mergeTUIConfig(dst *TUIConfig, overlay tuiConfigOverlay) {
	if overlay.InputMaxHeight != nil {
		dst.InputMaxHeight = *overlay.InputMaxHeight
	}
	if overlay.Theme != nil {
		dst.Theme = *overlay.Theme
	}
	if overlay.DisplayTurns != nil {
		dst.DisplayTurns = *overlay.DisplayTurns
	}
	if overlay.ErrorDisplayTime != nil {
		dst.ErrorDisplayTime = *overlay.ErrorDisplayTime
	}
	if overlay.AutoResume != nil {
		dst.AutoResume = *overlay.AutoResume
	}
	if overlay.MaxAttachmentSize != nil {
		dst.MaxAttachmentSize = *overlay.MaxAttachmentSize
	}
	if overlay.SSEReadTimeout != nil {
		dst.SSEReadTimeout = *overlay.SSEReadTimeout
	}
}

func mergeProviderConfigs(dst *[]ProviderConfig, overlay []ProviderConfig) {
	for _, op := range overlay {
		replaced := false
		for i, dp := range *dst {
			if dp.ID == op.ID {
				(*dst)[i] = op
				replaced = true
				break
			}
		}
		if !replaced {
			*dst = append(*dst, op)
		}
	}
}

func mergeMCPServers(dst *[]MCPServerConfig, overlay []MCPServerConfig) {
	for _, os := range overlay {
		key := os.Name
		if key == "" {
			key = os.Command
		}
		replaced := false
		for i, ds := range *dst {
			dk := ds.Name
			if dk == "" {
				dk = ds.Command
			}
			if dk == key {
				(*dst)[i] = os
				replaced = true
				break
			}
		}
		if !replaced {
			*dst = append(*dst, os)
		}
	}
}

func mergeLSPServers(dst *[]LSPServerConfig, overlay []lspServerConfigOverlay) {
	for _, os := range overlay {
		key := lspServerOverlayKey(os)
		if key == "" {
			*dst = append(*dst, lspServerFromOverlay(os))
			continue
		}
		replaced := false
		for i, ds := range *dst {
			if lspServerKey(ds) == key {
				(*dst)[i] = mergeLSPServerConfig(ds, os)
				replaced = true
				break
			}
		}
		if !replaced {
			*dst = append(*dst, lspServerFromOverlay(os))
		}
	}
}

func mergeGlobalSkillsConfig(dst *GlobalSkillsConfig, overlay globalSkillsConfigOverlay) {
	if len(overlay.Models) == 0 {
		return
	}
	if dst.Models == nil {
		dst.Models = make(map[string]ModelSkillsConfig)
	}
	for modelID, overlayCfg := range overlay.Models {
		existing := dst.Models[modelID]
		if overlayCfg.AllowAll != nil {
			existing.AllowAll = *overlayCfg.AllowAll
		}
		existing.Deny = appendUniqueStrings(existing.Deny, overlayCfg.Deny)
		dst.Models[modelID] = existing
	}
}

func lspServerKey(cfg LSPServerConfig) string {
	if cfg.Name != "" {
		return cfg.Name
	}
	return cfg.Command
}

func lspServerOverlayKey(cfg lspServerConfigOverlay) string {
	if cfg.Name != nil && *cfg.Name != "" {
		return *cfg.Name
	}
	if cfg.Command != nil {
		return *cfg.Command
	}
	return ""
}

func lspServerFromOverlay(cfg lspServerConfigOverlay) LSPServerConfig {
	out := LSPServerConfig{}
	if cfg.Name != nil {
		out.Name = *cfg.Name
	}
	if cfg.Command != nil {
		out.Command = *cfg.Command
	}
	if cfg.Args != nil {
		out.Args = cfg.Args
	}
	if cfg.Env != nil {
		out.Env = cfg.Env
	}
	if cfg.Extensions != nil {
		out.Extensions = cfg.Extensions
	}
	if cfg.Enabled != nil {
		out.Enabled = cfg.Enabled
	}
	if cfg.InitOptions != nil {
		out.InitOptions = cfg.InitOptions
	}
	return out
}

func mergeLSPServerConfig(dst LSPServerConfig, overlay lspServerConfigOverlay) LSPServerConfig {
	if overlay.Name != nil {
		dst.Name = *overlay.Name
	}
	if overlay.Command != nil {
		dst.Command = *overlay.Command
	}
	if overlay.Args != nil {
		dst.Args = overlay.Args
	}
	if overlay.Env != nil {
		dst.Env = overlay.Env
	}
	if overlay.Extensions != nil {
		dst.Extensions = overlay.Extensions
	}
	if overlay.Enabled != nil {
		dst.Enabled = overlay.Enabled
	}
	if overlay.InitOptions != nil {
		dst.InitOptions = overlay.InitOptions
	}
	return dst
}

func appendUniqueStrings(dst, overlay []string) []string {
	if len(overlay) == 0 {
		return dst
	}
	seen := make(map[string]bool, len(dst))
	for _, p := range dst {
		seen[p] = true
	}
	for _, p := range overlay {
		if !seen[p] {
			dst = append(dst, p)
		}
	}
	return dst
}

// mergeModelSessionConfig merges overlay into dst field-by-field, preserving
// dst fields that overlay does not set.
func mergeModelSessionConfig(dst ModelSessionConfig, overlay modelSessionConfigOverlay) ModelSessionConfig {
	if overlay.CompactionThreshold != nil {
		dst.CompactionThreshold = overlay.CompactionThreshold
	}
	if overlay.CompactionKeepTurns != nil {
		dst.CompactionKeepTurns = overlay.CompactionKeepTurns
	}
	if overlay.PruneProtectTokens != nil {
		dst.PruneProtectTokens = overlay.PruneProtectTokens
	}
	if overlay.PruneMinSavings != nil {
		dst.PruneMinSavings = overlay.PruneMinSavings
	}
	if overlay.ContextLimit != nil {
		dst.ContextLimit = overlay.ContextLimit
	}
	if overlay.MaxInputTokens != nil {
		dst.MaxInputTokens = *overlay.MaxInputTokens
	}
	if overlay.ToolCallArgumentTimeout != nil {
		dst.ToolCallArgumentTimeout = *overlay.ToolCallArgumentTimeout
	}
	return dst
}

// ResolvedPermission merges: built-in defaults → global/project config → agent overrides.
func (c *Config) ResolvedPermission(agentPermission permission.Config) permission.Config {
	result := permission.Merge(permission.Defaults(), c.Permission)
	result = permission.Merge(result, agentPermission)
	return result
}

func (c *Config) ResolvedUtilityModel() string {
	return c.UtilityModel
}

func (c *Config) ProviderByID(id string) (ProviderConfig, error) {
	for _, p := range c.Providers {
		if p.ID == id {
			return p, nil
		}
	}
	return ProviderConfig{}, fmt.Errorf("config: provider %q not found", id)
}
