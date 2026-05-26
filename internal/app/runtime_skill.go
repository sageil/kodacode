package app

import (
	"fmt"
	"runtime"
	"slices"
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/agent"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/skill"
	toolpkg "github.com/sageil/kodacode/internal/tool"
)

func (r *Runtime) resolveTurnSkills(workspaceRoot string, skillIDs []string) ([]skill.Definition, error) {
	if len(skillIDs) == 0 {
		return nil, nil
	}
	if r.Skills == nil {
		return nil, skill.ErrSkillNotFound
	}
	return r.Skills.GetMany(workspaceRoot, skillIDs)
}

func (r *Runtime) availableTurnSkills(workspaceRoot string) ([]skill.Definition, error) {
	if r == nil || r.Skills == nil {
		return nil, nil
	}
	catalog, err := r.Skills.Catalog(workspaceRoot)
	if err != nil {
		return nil, err
	}
	if len(catalog) == 0 {
		return nil, nil
	}
	definitions := make([]skill.Definition, 0, len(catalog))
	for _, definition := range catalog {
		if strings.TrimSpace(definition.ID) == "" {
			continue
		}
		definitions = append(definitions, definition)
	}
	sort.Slice(definitions, func(i, j int) bool {
		if rank := availableSkillSourceRank(string(definitions[i].Source)) - availableSkillSourceRank(string(definitions[j].Source)); rank != 0 {
			return rank < 0
		}
		if definitions[i].ID != definitions[j].ID {
			return definitions[i].ID < definitions[j].ID
		}
		return definitions[i].Path < definitions[j].Path
	})
	return definitions, nil
}

const coreRuntimePolicyContent = `Runtime contract:
	- Use the available tool surface when a tool can perform the requested action; if runtime blocks it, explain the concrete blocker.
	- Do not claim you cannot access files, modify code, run commands, or manage tasks when the relevant tools are available in this turn.
	- Do not run git commit, create commits, or otherwise persist version-control history unless the user explicitly asks you to commit.
	- Workspace boundaries, allowed tools, MCP availability, and permission stops are enforced by runtime.`

func defaultTurnFragments(
	definition agent.Definition,
	workspaceRoot string,
	additionalRoots []string,
	availableSkills []skill.Definition,
	skills []skill.Definition,
	allowedTools []string,
	mcpState *events.SessionMCPState,
	memories *MemoryService,
	responseStyle ResponseStyle,
	executionConfig ExecutionConfig,
	inspectionProgress []inspectionProgressPromptEntry,
) ([]prompt.Fragment, error) {
	fragments := []prompt.Fragment{
		{
			Kind:      prompt.KindPolicy,
			Source:    prompt.SourceBuiltin,
			Stability: prompt.StabilityStable,
			Key:       "core-policy",
			Label:     "core-policy",
			Content:   coreRuntimePolicyContent,
		},
	}
	if definition.HasPrompt() {
		fragments = append(fragments, definition.PromptFragment())
	}

	sourceFragments, err := loadPromptSourceFragments(workspaceRoot)
	if err != nil {
		return nil, err
	}
	fragments = append(fragments, sourceFragments...)
	if fragment, ok, err := memoryPromptFragment(memories, workspaceRoot); err != nil {
		return nil, err
	} else if ok {
		fragments = append(fragments, fragment)
	}
	if fragment, ok := availableSkillsPromptFragment(availableSkills); ok {
		fragments = append(fragments, fragment)
	}
	for _, selected := range skills {
		if strings.TrimSpace(selected.Prompt) == "" {
			continue
		}
		fragments = append(fragments, selected.PromptFragment())
	}
	if fragment, ok := responseStylePromptFragment(responseStyle); ok {
		fragments = append(fragments, fragment)
	}
	fragments = append(fragments, prompt.Fragment{
		Kind:      prompt.KindRuntime,
		Source:    prompt.SourceRuntime,
		Stability: prompt.StabilityDynamic,
		Key:       "workspace",
		Label:     "workspace",
		Content:   runtimeWorkspaceFragmentContent(workspaceRoot, additionalRoots),
	})
	fragments = append(fragments, prompt.Fragment{
		Kind:      prompt.KindRuntime,
		Source:    prompt.SourceRuntime,
		Stability: prompt.StabilityDynamic,
		Key:       "execution-environment",
		Label:     "execution-environment",
		Content:   runtimeExecutionEnvironmentFragmentContent(executionConfig),
	})
	if inspectionProgressContent, ok := runtimeInspectionProgressFragmentContent(inspectionProgress); ok {
		fragments = append(fragments, prompt.Fragment{
			Kind:      prompt.KindRuntime,
			Source:    prompt.SourceRuntime,
			Stability: prompt.StabilityDynamic,
			Key:       "inspection-progress",
			Label:     "inspection-progress",
			Content:   inspectionProgressContent,
		})
	}
	if mcpContent, ok := runtimeMCPFragmentContent(mcpState, allowedTools); ok {
		fragments = append(fragments, prompt.Fragment{
			Kind:      prompt.KindRuntime,
			Source:    prompt.SourceRuntime,
			Stability: prompt.StabilityDynamic,
			Key:       "mcp",
			Label:     "mcp",
			Content:   mcpContent,
		})
	}
	return fragments, nil
}

const availableSkillsPromptBudget = 8000

func availableSkillsPromptFragment(skills []skill.Definition) (prompt.Fragment, bool) {
	if len(skills) == 0 {
		return prompt.Fragment{}, false
	}
	lines := []string{
		"Available skills:",
		"- Skills are reusable workflows. The list below includes only name, description, and SKILL.md path.",
		"- If the user explicitly mentions a skill as `$name` or `${name}`, use that skill for the task.",
		"- For implicit use, choose a skill only when the request matches its description. Load full instructions with the `skill` tool before following a skill that is not already included as a full prompt fragment.",
	}
	for _, definition := range skills {
		id := strings.TrimSpace(definition.ID)
		if id == "" {
			continue
		}
		line := "- $" + id
		if description := strings.TrimSpace(definition.Description); description != "" {
			line += ": " + description
		}
		if path := strings.TrimSpace(definition.Path); path != "" {
			line += " (" + path + ")"
		}
		next := append(lines, line)
		if len(strings.Join(next, "\n")) > availableSkillsPromptBudget {
			lines = append(lines, "- Additional skills omitted from this initial list to keep the prompt bounded. Use `search_skills` when discovery is needed.")
			break
		}
		lines = next
	}
	content := strings.TrimSpace(strings.Join(lines, "\n"))
	if content == "" {
		return prompt.Fragment{}, false
	}
	return prompt.Fragment{
		Kind:      prompt.KindTooling,
		Source:    prompt.SourceRuntime,
		Stability: prompt.StabilityDynamic,
		Key:       "available-skills",
		Label:     "available-skills",
		Content:   content,
	}, true
}

func memoryPromptFragment(memories *MemoryService, workspaceRoot string) (prompt.Fragment, bool, error) {
	if memories == nil {
		return prompt.Fragment{}, false, nil
	}
	return memories.PromptFragment(workspaceRoot)
}

func runtimeWorkspaceFragmentContent(workspaceRoot string, additionalRoots []string) string {
	lines := []string{
		"Current workspace directory: " + workspaceRoot,
		"The workspace directory is the current project directory, not the filesystem root `/`.",
		"For workspace-wide operations, use `.` or another workspace-relative path, not `/`.",
	}
	if len(additionalRoots) == 0 {
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "Additional writable roots:")
	for _, root := range additionalRoots {
		trimmed := strings.TrimSpace(root)
		if trimmed == "" {
			continue
		}
		lines = append(lines, "- "+trimmed)
	}
	return strings.Join(lines, "\n")
}

func runtimeExecutionEnvironmentFragmentContent(config ExecutionConfig) string {
	lines := []string{
		"Execution environment:",
		"- Host operating system: " + hostOperatingSystemLabel(runtime.GOOS),
	}
	if shellPath := strings.TrimSpace(preferredShellPath(config)); shellPath != "" {
		lines = append(lines, "- Default execution shell: "+shellPath)
	}
	return strings.Join(lines, "\n")
}

func hostOperatingSystemLabel(goos string) string {
	if strings.TrimSpace(goos) == "" {
		return "unknown"
	}
	switch goos {
	case "darwin":
		return "darwin (macOS)"
	default:
		return goos
	}
}

func runtimeMCPFragmentContent(state *events.SessionMCPState, allowedTools []string) (string, bool) {
	if state == nil {
		return "", false
	}
	accessibleTools := make([]events.SessionMCPToolPayload, 0, len(state.Tools))
	for _, tl := range state.Tools {
		if !toolAllowed(strings.TrimSpace(tl.Name), allowedTools) {
			continue
		}
		accessibleTools = append(accessibleTools, tl)
	}
	if len(accessibleTools) == 0 {
		return "", false
	}
	activeServers := make([]events.SessionMCPServerPayload, 0, len(state.Servers))
	for _, server := range state.Servers {
		if !server.Active {
			continue
		}
		activeServers = append(activeServers, server)
	}
	if len(activeServers) == 0 {
		return "", false
	}
	lines := []string{"Active MCP integrations available for this turn:"}
	lines = append(lines, "Servers:")
	for _, server := range activeServers {
		lines = append(lines, fmt.Sprintf("- %s (%s)", strings.TrimSpace(server.Name), strings.TrimSpace(server.Type)))
	}
	lines = append(lines, "Tools (use these exact callable names when issuing tool calls):")
	for _, tl := range accessibleTools {
		line := "- " + strings.TrimSpace(tl.Name)
		if description := strings.TrimSpace(tl.Description); description != "" {
			line += ": " + description
		}
		lines = append(lines, line)
	}
	lines = append(lines, "Do not say MCP is unavailable if it is listed here or present in the tool surface.")
	return strings.Join(lines, "\n"), true
}

func (r *Runtime) allowedToolsForTurn(state events.SessionState, definition agent.Definition) []string {
	return applyToolPolicy(toolPolicyUniverse(r.Tools, state.MCP), definition.AllowedTools, definition.DisallowedTools)
}

func copyStrings(values []string) []string {
	return slices.Clone(values)
}

func toolPolicyUniverse(executor *ToolExecutor, mcpState *events.SessionMCPState) []string {
	seen := map[string]struct{}{}
	if executor != nil {
		for name := range executor.tools {
			name = strings.TrimSpace(name)
			if name == "" || toolpkg.IsMCPToolName(name) {
				continue
			}
			seen[name] = struct{}{}
		}
	}
	if len(seen) == 0 {
		for _, runtimeTool := range toolpkg.DefaultRuntimeTools() {
			name := strings.TrimSpace(runtimeTool.Definition().Name)
			if name == "" {
				continue
			}
			seen[name] = struct{}{}
		}
	}
	if mcpState != nil {
		for _, mcpTool := range mcpState.Tools {
			name := strings.TrimSpace(mcpTool.Name)
			if name == "" {
				continue
			}
			seen[name] = struct{}{}
		}
	}
	return sortedToolNames(seen)
}

func sortedToolNames(names map[string]struct{}) []string {
	if len(names) == 0 {
		return nil
	}
	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func applyToolPolicy(current, allowed, disallowed []string) []string {
	next := copyStrings(current)
	if allowed != nil {
		next = intersectToolSurface(next, allowed)
	}
	if len(disallowed) > 0 {
		next = subtractToolSurface(next, disallowed)
	}
	return next
}

func intersectToolSurface(current, allowed []string) []string {
	if len(current) == 0 || len(allowed) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(current))
	for _, item := range current {
		if toolpkg.PolicyListContainsTool(allowed, item) {
			out = append(out, item)
		}
	}
	return out
}

func subtractToolSurface(current, disallowed []string) []string {
	if len(current) == 0 || len(disallowed) == 0 {
		return copyStrings(current)
	}
	out := make([]string, 0, len(current))
	for _, item := range current {
		if toolpkg.PolicyListContainsTool(disallowed, item) {
			continue
		}
		out = append(out, item)
	}
	return out
}
