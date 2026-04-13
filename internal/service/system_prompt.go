package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sageil/kodacode/v1/internal/agent"
	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/skills"
)

// AgentLister provides agent listing for system prompt injection.
type AgentLister interface {
	List() []agent.Agent
}

// SystemPromptBuilderConfig configures a SystemPromptBuilder.
type SystemPromptBuilderConfig struct {
	ProjectDir  string
	SkillsDir   string         // default: ~/.config/kodacode/skills/
	Config      *config.Config // may be nil in tests
	Agents      AgentLister    // optional: when set, available agents are listed in the prompt
	MemoryStore *MemoryStore   // optional: when set, auto-memories are injected into the prompt
}

// SystemPromptBuilder assembles the three-part system prompt used by every turn.
// Part 0 (stable): agent-specific instructions. Changes only when the agent changes (~100% cache hit).
// Part 1 (semi-stable): environment, agents, skills, instructions, and ignore patterns. Changes rarely (~95% cache hit).
// Part 2 (volatile): compaction summary. Changes only on compaction events.
type SystemPromptBuilder struct {
	cfg   SystemPromptBuilderConfig
	mu    sync.RWMutex
	cache map[semiStableCacheKey]semiStableCacheEntry
}

type semiStableCacheKey struct {
	modelID          string
	ephemeral        bool
	skillFingerprint string
	skillToolMode    string
}

type semiStableCacheEntry struct {
	content           string
	gitMtime          time.Time
	memMtime          time.Time
	watchedPaths      map[string]time.Time
	dateKey           string
	ignoreFingerprint string
}

type SystemPromptBuildInput struct {
	AgentPrompt  string
	ProviderID   string
	ModelID      string
	SummaryText  string
	Agent        config.AgentConfig
	Tools        []provider.Tool
	UserMessage  string
	TouchedFiles []string
	Ephemeral    bool
}

// NewSystemPromptBuilder constructs a SystemPromptBuilder from cfg.
func NewSystemPromptBuilder(cfg SystemPromptBuilderConfig) *SystemPromptBuilder {
	if cfg.SkillsDir == "" {
		home, _ := os.UserHomeDir()
		cfg.SkillsDir = filepath.Join(home, ".config", "kodacode", "skills")
	}
	return &SystemPromptBuilder{
		cfg:   cfg,
		cache: make(map[semiStableCacheKey]semiStableCacheEntry),
	}
}

// Build assembles and returns exactly three system prompt parts:
// [stablePart, semiStablePart, volatilePart].
//
//   - stablePart: agent-specific or provider-specific base prompt.
//   - semiStablePart: environment, agents, skills, instruction files, ignore patterns.
//     Cached and invalidated when watched prompt inputs change.
//   - volatilePart: compaction summary (changes on each compaction event). Empty if no summary.
func (b *SystemPromptBuilder) Build(_ context.Context, in SystemPromptBuildInput) ([]string, error) {
	stablePart := b.buildStablePart(in.AgentPrompt, in.ProviderID, in.ModelID)
	semiStablePart := b.buildSemiStablePart(in.ProviderID, in.ModelID, in.Agent, in.Ephemeral)
	volatilePart := in.SummaryText

	if !in.Ephemeral && hasTool(in.Tools, "subagent") {
		if agentBlock := b.buildAgentsBlock(); agentBlock != "" {
			if volatilePart != "" {
				volatilePart = agentBlock + "\n\n" + volatilePart
			} else {
				volatilePart = agentBlock
			}
		}
	}
	if overlay := b.BuildRelevantSkillsOverlay(in.ProviderID, in.ModelID, in.Agent, in.UserMessage, in.TouchedFiles); overlay != "" {
		if volatilePart != "" {
			volatilePart = overlay + "\n\n" + volatilePart
		} else {
			volatilePart = overlay
		}
	}
	if overlay := b.BuildRelevantToolsOverlay(in.Tools, in.UserMessage, in.TouchedFiles); overlay != "" {
		if volatilePart != "" {
			volatilePart = overlay + "\n\n" + volatilePart
		} else {
			volatilePart = overlay
		}
	}
	return []string{stablePart, semiStablePart, volatilePart}, nil
}

func (b *SystemPromptBuilder) buildStablePart(agentPrompt, providerID, modelID string) string {
	providerBase := agent.ProviderPrompt(providerID, modelID)
	if agentPrompt == "" {
		return providerBase
	}
	return providerBase + "\n\n" + agentPrompt
}

// buildSemiStablePart builds the semi-stable system prompt content (Part 1).
// This content changes rarely and is cached until any watched prompt input
// changes (git state, day rollover, agents, skills, instructions, or config).
func (b *SystemPromptBuilder) buildSemiStablePart(providerID, modelID string, agentCfg config.AgentConfig, ephemeral bool) string {
	access := skills.ResolveAccess(b.cfg.Config, providerID, modelID, agentCfg)
	key := semiStableCacheKey{
		modelID:          providerID + "/" + modelID,
		ephemeral:        ephemeral,
		skillFingerprint: skillAccessFingerprint(access),
		skillToolMode:    skillToolMode(agentCfg),
	}

	if cached := b.checkSemiStableCache(key); cached != "" {
		return cached
	}

	var parts []string
	dateKey := time.Now().Format("2006-01-02")

	env := b.buildEnvironmentPromptWithDate(modelID, dateKey)
	if env != "" {
		parts = append(parts, env)
	}
	parts = append(parts,
		"Tool result rule:\nWhen you call tools, text emitted before the tool results describes intended actions only. Do not claim files were edited, patches applied, commands succeeded, or tests passed until the corresponding tool results confirm it.")

	skillsBlock, skillPaths := b.buildSkillsBlock(access, agentCfg)
	if skillsBlock != "" {
		parts = append(parts, skillsBlock)
	}

	instructions, loadedPaths := b.loadInstructionFiles()
	parts = append(parts, instructions...)

	if b.cfg.MemoryStore != nil {
		budget := 4000
		if b.cfg.Config != nil && b.cfg.Config.MemoryBudget > 0 {
			budget = b.cfg.Config.MemoryBudget
		}
		if block := b.cfg.MemoryStore.LoadWithBudget(budget); block != "" {
			parts = append(parts, block)
		}
	}

	// Inject ignore patterns so the model avoids scanning these directories.
	if b.cfg.Config != nil && len(b.cfg.Config.IgnorePatterns) > 0 {
		var sb strings.Builder
		sb.WriteString("Ignored directories (NEVER list, search, or scan these):\n")
		for _, p := range b.cfg.Config.IgnorePatterns {
			sb.WriteString("  - ")
			sb.WriteString(p)
			sb.WriteByte('\n')
		}
		sb.WriteString("Use the tree, glob, or search tools instead of ls or find — they automatically skip these directories.")
		parts = append(parts, sb.String())
	}

	result := strings.Join(parts, "\n\n")
	watchedPaths := append(append([]string(nil), loadedPaths...), skillPaths...)
	entry := semiStableCacheEntry{
		content:           result,
		watchedPaths:      make(map[string]time.Time, len(watchedPaths)),
		dateKey:           dateKey,
		ignoreFingerprint: b.currentIgnoreFingerprint(),
	}
	gitHead := filepath.Join(b.cfg.ProjectDir, ".git", "HEAD")
	if info, err := os.Stat(gitHead); err == nil {
		entry.gitMtime = info.ModTime()
	}
	if b.cfg.MemoryStore != nil {
		if info, err := os.Stat(b.cfg.MemoryStore.Dir()); err == nil {
			entry.memMtime = info.ModTime()
		}
	}
	for _, p := range watchedPaths {
		entry.watchedPaths[p] = statMtimeOrZero(p)
	}
	b.mu.Lock()
	b.cache[key] = entry
	b.mu.Unlock()
	return result
}

// checkSemiStableCache returns the cached semi-stable content if still valid.
func (b *SystemPromptBuilder) checkSemiStableCache(key semiStableCacheKey) string {
	b.mu.RLock()
	entry, ok := b.cache[key]
	b.mu.RUnlock()
	if !ok || entry.content == "" {
		return ""
	}
	gitHead := filepath.Join(b.cfg.ProjectDir, ".git", "HEAD")
	if info, err := os.Stat(gitHead); err == nil && !info.ModTime().Equal(entry.gitMtime) {
		b.mu.Lock()
		delete(b.cache, key)
		b.mu.Unlock()
		return ""
	}
	if b.cfg.MemoryStore != nil {
		if info, err := os.Stat(b.cfg.MemoryStore.Dir()); err == nil && !info.ModTime().Equal(entry.memMtime) {
			b.mu.Lock()
			delete(b.cache, key)
			b.mu.Unlock()
			return ""
		}
	}
	if entry.dateKey != time.Now().Format("2006-01-02") {
		b.mu.Lock()
		delete(b.cache, key)
		b.mu.Unlock()
		return ""
	}
	if entry.ignoreFingerprint != b.currentIgnoreFingerprint() {
		b.mu.Lock()
		delete(b.cache, key)
		b.mu.Unlock()
		return ""
	}
	for path, cachedMtime := range entry.watchedPaths {
		info, err := os.Stat(path)
		switch {
		case err != nil && cachedMtime.IsZero():
			continue
		case err == nil && info.ModTime().Equal(cachedMtime):
			continue
		default:
			b.mu.Lock()
			delete(b.cache, key)
			b.mu.Unlock()
			return ""
		}
	}
	return entry.content
}

// buildAgentsBlock returns a prompt section listing available subagents for the
// subagent tool. Only agents with mode "subagent" are listed because primary agents
// are for direct user interaction, not delegation.
// Returns "" if no agent lister is configured or no subagents exist.
func (b *SystemPromptBuilder) buildAgentsBlock() string {
	if b.cfg.Agents == nil {
		return ""
	}
	all := b.cfg.Agents.List()
	if len(all) == 0 {
		return ""
	}

	var subagents []agent.Agent
	for _, a := range all {
		if a.Mode == agent.ModeSubagent {
			subagents = append(subagents, a)
		}
	}
	if len(subagents) == 0 {
		return ""
	}
	sort.Slice(subagents, func(i, j int) bool { return subagents[i].ID < subagents[j].ID })

	var sb strings.Builder
	sb.WriteString("# Specialized agents\n")
	sb.WriteString("Use the `subagent` tool to delegate tasks. Follow your agent-specific instructions for when and how to delegate.\n\n")
	sb.WriteString("Available agents:\n")
	for _, a := range subagents {
		desc := a.Description
		if desc == "" {
			desc = a.Name
		}
		fmt.Fprintf(&sb, "  - %s: %s\n", a.ID, desc)
	}
	sb.WriteString("\nSubagent rules:\n")
	sb.WriteString("  - Give SPECIFIC, targeted tasks — not \"explore everything\".\n")
	sb.WriteString("  - NEVER ask subagents to return full file contents — ask for findings and patterns.\n")
	return sb.String()
}

func (b *SystemPromptBuilder) currentIgnoreFingerprint() string {
	if b.cfg.Config == nil || len(b.cfg.Config.IgnorePatterns) == 0 {
		return ""
	}
	return strings.Join(b.cfg.Config.IgnorePatterns, "\x00")
}

// EnvironmentInfo holds the resolved environment values for the <env> block.
type EnvironmentInfo struct {
	WorkingDir string
	GitRoot    string
	IsGitRepo  bool
	GitBranch  string
	GitLog     string // recent commits (one-line format)
	GitStatus  string // short status (staged/unstaged summary)
	Platform   string
	Shell      string
	Date       string
}

func (b *SystemPromptBuilder) buildEnvironmentPrompt(modelID string) string {
	return b.buildEnvironmentPromptWithDate(modelID, "")
}

func (b *SystemPromptBuilder) buildEnvironmentPromptWithDate(modelID, date string) string {
	env := b.getEnvironment(date)

	var sb strings.Builder
	if modelID != "" {
		fmt.Fprintf(&sb, "Model: %s\n\n", modelID)
	}
	sb.WriteString("Here is some useful information about the environment you are running in:\n")
	sb.WriteString("<env>\n")
	fmt.Fprintf(&sb, "  Working directory: %s\n", env.WorkingDir)
	if env.GitRoot != "" && env.GitRoot != env.WorkingDir {
		fmt.Fprintf(&sb, "  Workspace root: %s\n", env.GitRoot)
	}
	fmt.Fprintf(&sb, "  Is directory a git repo: %s\n", boolStr(env.IsGitRepo))
	if env.GitBranch != "" {
		fmt.Fprintf(&sb, "  Git branch: %s\n", env.GitBranch)
	}
	fmt.Fprintf(&sb, "  Platform: %s\n", env.Platform)
	if env.Shell != "" {
		fmt.Fprintf(&sb, "  Shell: %s\n", env.Shell)
	}
	fmt.Fprintf(&sb, "  Today's date: %s\n", env.Date)
	sb.WriteString("</env>")

	return sb.String()
}

func (b *SystemPromptBuilder) buildProjectOverview() string {
	env := b.getEnvironment("")

	var parts []string
	if tree := b.buildProjectTree(); tree != "" {
		parts = append(parts, "Project structure (depth 2 — do NOT call the tree tool for the project root, use this instead):\n"+tree)
	}
	if env.GitLog != "" {
		parts = append(parts, "Recent commits:\n"+env.GitLog)
	}
	return strings.Join(parts, "\n\n")
}

func (b *SystemPromptBuilder) getEnvironment(date string) EnvironmentInfo {
	projectDir := b.cfg.ProjectDir

	isGit := false
	gitDir := filepath.Join(projectDir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		isGit = true
	}

	// Find git root by walking up the directory tree.
	gitRoot := ""
	dir := projectDir
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			gitRoot = dir
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Build a descriptive platform string so the model uses correct
	// platform-specific flags (e.g. BSD vs GNU coreutils).
	platform := runtime.GOOS + "/" + runtime.GOARCH
	switch runtime.GOOS {
	case "darwin":
		if ver, err := exec.Command("sw_vers", "-productVersion").Output(); err == nil {
			platform = "macOS " + strings.TrimSpace(string(ver)) + " (" + runtime.GOARCH + ")"
		} else {
			platform = "macOS (" + runtime.GOARCH + ")"
		}
	case "linux":
		if data, err := os.ReadFile("/etc/os-release"); err == nil {
			for line := range strings.SplitSeq(string(data), "\n") {
				if name, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
					platform = strings.Trim(name, "\"") + " (" + runtime.GOARCH + ")"
					break
				}
			}
		}
	}

	// Detect the user's default shell.
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "sh"
	} else {
		shell = filepath.Base(shell)
	}

	env := EnvironmentInfo{
		WorkingDir: projectDir,
		GitRoot:    gitRoot,
		IsGitRepo:  isGit,
		Platform:   platform,
		Shell:      shell,
		Date:       currentDateString(date),
	}

	// Inject git context so the model understands the current development state.
	if isGit {
		env.GitBranch = gitOutput(projectDir, "rev-parse", "--abbrev-ref", "HEAD")
		env.GitLog = gitOutput(projectDir, "log", "--oneline", "-5")
		env.GitStatus = gitOutput(projectDir, "status", "--short")
	}

	return env
}

func currentDateString(date string) string {
	if strings.TrimSpace(date) != "" {
		return date
	}
	return time.Now().Format("2006-01-02")
}

func statMtimeOrZero(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// gitOutput runs a git command in dir and returns trimmed stdout, or "" on error.
func gitOutput(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (b *SystemPromptBuilder) buildProjectTree() string {
	root := b.cfg.ProjectDir
	if root == "" {
		return ""
	}

	const (
		maxTopLevelEntries   = 24
		maxChildrenPerFolder = 10
	)

	var ignoreSet map[string]bool
	if b.cfg.Config != nil {
		ignoreSet = make(map[string]bool, len(b.cfg.Config.IgnorePatterns))
		for _, p := range b.cfg.Config.IgnorePatterns {
			dir := strings.TrimSuffix(p, "/**")
			dir = strings.TrimSuffix(dir, "/")
			ignoreSet[dir] = true
		}
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}

	var sb strings.Builder
	topLevelShown := 0
	for _, e := range entries {
		if topLevelShown >= maxTopLevelEntries {
			sb.WriteString("... (truncated)\n")
			break
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || ignoreSet[name] {
			continue
		}
		topLevelShown++
		if e.IsDir() {
			sb.WriteString(name + "/\n")
			subEntries, err := os.ReadDir(filepath.Join(root, name))
			if err != nil {
				continue
			}
			childrenShown := 0
			for _, se := range subEntries {
				if childrenShown >= maxChildrenPerFolder {
					sb.WriteString("  ...\n")
					break
				}
				seName := se.Name()
				if strings.HasPrefix(seName, ".") || ignoreSet[seName] {
					continue
				}
				childrenShown++
				if se.IsDir() {
					sb.WriteString("  " + seName + "/\n")
				} else {
					sb.WriteString("  " + seName + "\n")
				}
			}
		} else {
			sb.WriteString(name + "\n")
		}
	}
	return sb.String()
}

func boolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

const (
	maxInstructionSize = 32 * 1024
)

var defaultInstructionFiles = []string{"KODACODE.md", "AGENTS.md", "CLAUDE.md", "CONTEXT.md"}

// buildSkillsBlock reads SKILL.md files from global and project-local skills
// directories, extracts name/description from YAML frontmatter, and returns
// a prompt section listing available skills.
func (b *SystemPromptBuilder) buildSkillsBlock(policy skills.AccessPolicy, agentCfg config.AgentConfig) (string, []string) {
	idx := skills.LoadIndex(b.skillDirs())
	if !agentToolEnabled(agentCfg, "skill") {
		return "", idx.WatchedPaths()
	}
	filtered := idx.Filter(policy)
	if len(filtered) == 0 {
		return "", idx.WatchedPaths()
	}
	canSearch := agentToolEnabled(agentCfg, "search_skills")

	var sb strings.Builder
	if canSearch {
		sb.WriteString("Before writing, reviewing, or refactoring code when project-specific conventions may matter, use `search_skills` to discover relevant skills and `skill` to load them.\n\n")
	} else {
		sb.WriteString("Before writing, reviewing, or refactoring code when project-specific conventions may matter, load the relevant skill with `skill` before editing.\n\n")
	}
	if len(filtered) <= 8 || !canSearch {
		sb.WriteString("Available skills:\n")
		for _, s := range filtered {
			if s.Description != "" {
				fmt.Fprintf(&sb, "  - %s: %s\n", s.Name, s.Description)
			} else {
				fmt.Fprintf(&sb, "  - %s\n", s.Name)
			}
		}
	} else {
		sb.WriteString("Available skills: ")
		fmt.Fprintf(&sb, "%d total. Examples:\n", len(filtered))
		for _, s := range filtered[:8] {
			if s.Description != "" {
				fmt.Fprintf(&sb, "  - %s: %s\n", s.Name, s.Description)
			} else {
				fmt.Fprintf(&sb, "  - %s\n", s.Name)
			}
		}
		sb.WriteString("Use `search_skills` when you need the full list or are unsure which skill fits the task.\n")
	}
	return sb.String(), idx.WatchedPaths()
}

func (b *SystemPromptBuilder) BuildRelevantSkillsOverlay(providerID, modelID string, agentCfg config.AgentConfig, userMessage string, touchedFiles []string) string {
	if !agentToolEnabled(agentCfg, "skill") {
		return ""
	}
	idx := skills.LoadIndex(b.skillDirs())
	matches := idx.Relevant(userMessage, touchedFiles, skills.ResolveAccess(b.cfg.Config, providerID, modelID, agentCfg), 5)
	if len(matches) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("# Likely Relevant Skills For This Turn\n")
	sb.WriteString("Use `skill` to load any of these before editing if they apply:\n")
	for _, match := range matches {
		sb.WriteString("- ")
		sb.WriteString(match.Skill.Name)
		if match.Skill.Description != "" {
			sb.WriteString(": ")
			sb.WriteString(match.Skill.Description)
		}
		if len(match.Reasons) > 0 {
			sb.WriteString(" [")
			sb.WriteString(strings.Join(match.Reasons, ", "))
			sb.WriteString("]")
		}
		sb.WriteByte('\n')
		if len(match.Skill.Suggests.Before) > 0 {
			sb.WriteString("  Load before: ")
			sb.WriteString(strings.Join(match.Skill.Suggests.Before, ", "))
			sb.WriteByte('\n')
		}
		if len(match.Skill.Suggests.After) > 0 {
			sb.WriteString("  Consider after: ")
			sb.WriteString(strings.Join(match.Skill.Suggests.After, ", "))
			sb.WriteByte('\n')
		}
	}
	return strings.TrimSpace(sb.String())
}

func (b *SystemPromptBuilder) BuildRelevantToolsOverlay(tools []provider.Tool, userMessage string, touchedFiles []string) string {
	return buildRelevantToolsOverlay(tools, userMessage, touchedFiles)
}

func (b *SystemPromptBuilder) skillDirs() []string {
	return []string{
		filepath.Join(b.cfg.ProjectDir, ".kodacode", "skills"),
		b.cfg.SkillsDir,
	}
}

func skillAccessFingerprint(policy skills.AccessPolicy) string {
	allow := append([]string(nil), policy.Allow...)
	deny := append([]string(nil), policy.Deny...)
	sort.Strings(allow)
	sort.Strings(deny)
	parts := []string{
		fmt.Sprintf("allow_all_set:%t", policy.AllowAllSet),
		fmt.Sprintf("allow_all:%t", policy.AllowAll),
		"allow:" + strings.Join(allow, ","),
		"deny:" + strings.Join(deny, ","),
	}
	return strings.Join(parts, "|")
}

func skillToolMode(agentCfg config.AgentConfig) string {
	return fmt.Sprintf("skill:%t|search:%t", agentToolEnabled(agentCfg, "skill"), agentToolEnabled(agentCfg, "search_skills"))
}

func agentToolEnabled(agentCfg config.AgentConfig, name string) bool {
	if len(agentCfg.Tools) > 0 {
		for _, allowed := range agentCfg.Tools {
			if allowed == name {
				return true
			}
		}
		return false
	}
	for _, denied := range agentCfg.DenyTools {
		if denied == name {
			return false
		}
	}
	return true
}
