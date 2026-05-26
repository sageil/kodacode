package app

import (
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/agent"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/skill"
	"github.com/sageil/kodacode/internal/tool"
)

func TestDefaultTurnFragmentsIncludesProjectMemoryWhenPresent(t *testing.T) {
	root := t.TempDir()
	memories := NewMemoryService()
	store := memories.Store(root)
	if store == nil {
		t.Fatal("Store() returned nil")
	}
	if _, err := store.SaveMemory(tool.MemorySaveRequest{
		Content: "The search tool supports regex and content search only.",
	}); err != nil {
		t.Fatalf("SaveMemory() error = %v", err)
	}

	fragments, err := defaultTurnFragments(agent.Definition{}, root, nil, nil, nil, nil, nil, memories, ResponseStyleDefault, ExecutionConfig{}, nil)
	if err != nil {
		t.Fatalf("defaultTurnFragments() error = %v", err)
	}
	for _, fragment := range fragments {
		if fragment.Kind != prompt.KindMemory {
			continue
		}
		if !strings.Contains(fragment.Content, "The search tool supports regex") {
			t.Fatalf("memory fragment = %#v", fragment)
		}
		return
	}
	t.Fatal("expected memory fragment in defaultTurnFragments output")
}

func TestDefaultTurnFragmentsIncludesCoreRuntimePolicy(t *testing.T) {
	fragments, err := defaultTurnFragments(agent.Definition{}, "/repo", nil, nil, nil, nil, nil, nil, ResponseStyleDefault, ExecutionConfig{}, nil)
	if err != nil {
		t.Fatalf("defaultTurnFragments() error = %v", err)
	}
	for _, fragment := range fragments {
		if fragment.Key != "core-policy" {
			continue
		}
		for _, want := range []string{
			"Use the available tool surface when a tool can perform the requested action",
			"Workspace boundaries, allowed tools, MCP availability, and permission stops are enforced by runtime.",
		} {
			if !strings.Contains(fragment.Content, want) {
				t.Fatalf("core policy missing %q: %#v", want, fragment)
			}
		}
		return
	}
	t.Fatal("expected core policy fragment in defaultTurnFragments output")
}

func TestDefaultTurnFragmentsRequiresExplicitUserRequestForGitCommit(t *testing.T) {
	fragments, err := defaultTurnFragments(agent.Definition{}, "/repo", nil, nil, nil, nil, nil, nil, ResponseStyleDefault, ExecutionConfig{}, nil)
	if err != nil {
		t.Fatalf("defaultTurnFragments() error = %v", err)
	}
	for _, fragment := range fragments {
		if fragment.Key != "core-policy" {
			continue
		}
		for _, want := range []string{
			"Do not run git commit, create commits, or otherwise persist version-control history unless the user explicitly asks you to commit.",
		} {
			if !strings.Contains(fragment.Content, want) {
				t.Fatalf("core policy missing %q: %#v", want, fragment)
			}
		}
		return
	}
	t.Fatal("expected core policy fragment in defaultTurnFragments output")
}

func TestDefaultTurnFragmentsLeavesDetailedMutationMechanicsToToolDescriptions(t *testing.T) {
	fragments, err := defaultTurnFragments(agent.Definition{}, "/repo", nil, nil, nil, nil, nil, nil, ResponseStyleDefault, ExecutionConfig{}, nil)
	if err != nil {
		t.Fatalf("defaultTurnFragments() error = %v", err)
	}
	for _, fragment := range fragments {
		if fragment.Key != "core-policy" {
			continue
		}
		for _, redundant := range []string{
			"Choose exactly one `apply_patch` mode.",
			"Range-reuse mode is `{path, start_line, end_line",
			"`write` does not patch",
			"read line-number prefixes",
		} {
			if strings.Contains(fragment.Content, redundant) {
				t.Fatalf("core policy contains tool-specific mechanics %q: %#v", redundant, fragment)
			}
		}
		return
	}
	t.Fatal("expected core policy fragment in defaultTurnFragments output")
}

func TestDefaultTurnFragmentsIncludesTerseResponseStyleHintWhenConfigured(t *testing.T) {
	fragments, err := defaultTurnFragments(agent.Definition{}, "/repo", nil, nil, nil, nil, nil, nil, ResponseStyleTerse, ExecutionConfig{}, nil)
	if err != nil {
		t.Fatalf("defaultTurnFragments() error = %v", err)
	}
	for _, fragment := range fragments {
		if fragment.Key != "response-style" {
			continue
		}
		if !strings.Contains(fragment.Content, "Response style: terse.") {
			t.Fatalf("response style fragment = %#v", fragment)
		}
		if !strings.Contains(fragment.Content, "Do not shorten safety, permission, destructive-action, or ambiguity clarifications.") {
			t.Fatalf("response style guardrail missing: %#v", fragment)
		}
		return
	}
	t.Fatal("expected response-style fragment in defaultTurnFragments output")
}

func TestDefaultTurnFragmentsPlacesResponseStyleAfterStableRoleAndSkillPrompts(t *testing.T) {
	fragments, err := defaultTurnFragments(
		agent.Definition{ID: "reviewer", Prompt: "Be thorough."},
		"/repo",
		nil,
		nil,
		[]skill.Definition{{
			ID:     "compact-review",
			Prompt: "Prefer compact review output.",
			Source: prompt.SourceProject,
		}},
		nil,
		nil,
		nil,
		ResponseStyleTerse,
		ExecutionConfig{},
		nil,
	)
	if err != nil {
		t.Fatalf("defaultTurnFragments() error = %v", err)
	}
	indexByKey := map[string]int{}
	for index, fragment := range fragments {
		indexByKey[fragment.Key] = index
	}
	if indexByKey["response-style"] <= indexByKey["agent:reviewer"] {
		t.Fatalf("response-style index = %d, agent index = %d", indexByKey["response-style"], indexByKey["agent:reviewer"])
	}
	if indexByKey["response-style"] <= indexByKey["skill:compact-review"] {
		t.Fatalf("response-style index = %d, skill index = %d", indexByKey["response-style"], indexByKey["skill:compact-review"])
	}
}

func TestDefaultTurnFragmentsIncludesAvailableSkillMetadataOnly(t *testing.T) {
	fragments, err := defaultTurnFragments(
		agent.Definition{},
		"/repo",
		nil,
		[]skill.Definition{{
			ID:          "migration",
			Description: "Use when changing database migrations.",
			Prompt:      "Full migration instructions should not be in the catalog.",
			Path:        "/repo/.kodacode/skills/migration/SKILL.md",
			Source:      prompt.SourceProject,
		}},
		nil,
		nil,
		nil,
		nil,
		ResponseStyleDefault,
		ExecutionConfig{},
		nil,
	)
	if err != nil {
		t.Fatalf("defaultTurnFragments() error = %v", err)
	}
	for _, fragment := range fragments {
		if fragment.Key != "available-skills" {
			continue
		}
		if !strings.Contains(fragment.Content, "$migration: Use when changing database migrations.") {
			t.Fatalf("available skills fragment = %#v", fragment)
		}
		if !strings.Contains(fragment.Content, "/repo/.kodacode/skills/migration/SKILL.md") {
			t.Fatalf("available skills fragment missing path: %#v", fragment)
		}
		if strings.Contains(fragment.Content, "Full migration instructions") {
			t.Fatalf("available skills fragment leaked full prompt: %#v", fragment)
		}
		return
	}
	t.Fatal("expected available-skills fragment")
}

func TestDefaultTurnFragmentsIncludesAllowedMCPStateWhenPresent(t *testing.T) {
	fragments, err := defaultTurnFragments(
		agent.Definition{},
		"/repo",
		nil,
		nil,
		nil,
		[]string{"read", "mcp:*"},
		&events.SessionMCPState{
			Servers: []events.SessionMCPServerPayload{{
				Name:    "sequential-thinking",
				Type:    "stdio",
				Trusted: true,
				Active:  true,
			}},
			Tools: []events.SessionMCPToolPayload{{
				Name:        "mcp_sequential_thinking__sequentialthinking",
				Description: "Sequential reasoning tool",
				ServerName:  "sequential-thinking",
				RemoteName:  "sequentialthinking",
			}},
		},
		nil,
		ResponseStyleDefault,
		ExecutionConfig{},
		nil,
	)
	if err != nil {
		t.Fatalf("defaultTurnFragments() error = %v", err)
	}
	for _, fragment := range fragments {
		if fragment.Key != "mcp" {
			continue
		}
		if !strings.Contains(fragment.Content, "sequential-thinking") {
			t.Fatalf("mcp fragment missing server name: %#v", fragment)
		}
		if !strings.Contains(fragment.Content, "Tools (use these exact callable names when issuing tool calls):") {
			t.Fatalf("mcp fragment missing exact callable name instruction: %#v", fragment)
		}
		if !strings.Contains(fragment.Content, "- mcp_sequential_thinking__sequentialthinking: Sequential reasoning tool") {
			t.Fatalf("mcp fragment missing callable tool name: %#v", fragment)
		}
		if strings.Contains(fragment.Content, "prefer the friendly MCP server or tool label") {
			t.Fatalf("mcp fragment still contains friendly label instruction: %#v", fragment)
		}
		return
	}
	t.Fatal("expected mcp fragment in defaultTurnFragments output")
}

func TestDefaultTurnFragmentsOmitsMCPStateWhenNoMCPToolsAllowed(t *testing.T) {
	fragments, err := defaultTurnFragments(
		agent.Definition{},
		"/repo",
		nil,
		nil,
		nil,
		[]string{"read", "search"},
		&events.SessionMCPState{
			Servers: []events.SessionMCPServerPayload{{
				Name:    "sequential-thinking",
				Type:    "stdio",
				Trusted: true,
				Active:  true,
			}},
			Tools: []events.SessionMCPToolPayload{{
				Name:        "mcp_sequential_thinking__sequentialthinking",
				Description: "Sequential reasoning tool",
				ServerName:  "sequential-thinking",
				RemoteName:  "sequentialthinking",
			}},
		},
		nil,
		ResponseStyleDefault,
		ExecutionConfig{},
		nil,
	)
	if err != nil {
		t.Fatalf("defaultTurnFragments() error = %v", err)
	}
	for _, fragment := range fragments {
		if fragment.Key == "mcp" {
			t.Fatalf("unexpected mcp fragment: %#v", fragment)
		}
	}
}

func TestDefaultTurnFragmentsIncludesExecutionEnvironment(t *testing.T) {
	fragments, err := defaultTurnFragments(
		agent.Definition{},
		"/repo",
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		ResponseStyleDefault,
		ExecutionConfig{ShellProgram: "/bin/test-shell"},
		nil,
	)
	if err != nil {
		t.Fatalf("defaultTurnFragments() error = %v", err)
	}
	for _, fragment := range fragments {
		if fragment.Key != "execution-environment" {
			continue
		}
		if !strings.Contains(fragment.Content, "Execution environment:") {
			t.Fatalf("execution environment fragment missing header: %#v", fragment)
		}
		if !strings.Contains(fragment.Content, runtime.GOOS) {
			t.Fatalf("execution environment fragment missing host OS: %#v", fragment)
		}
		if !strings.Contains(fragment.Content, "/bin/test-shell") {
			t.Fatalf("execution environment fragment missing shell: %#v", fragment)
		}
		return
	}
	t.Fatal("expected execution-environment fragment in defaultTurnFragments output")
}

func TestDefaultTurnFragmentsClarifiesWorkspaceDirectory(t *testing.T) {
	fragments, err := defaultTurnFragments(
		agent.Definition{},
		"/repo",
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		ResponseStyleDefault,
		ExecutionConfig{},
		nil,
	)
	if err != nil {
		t.Fatalf("defaultTurnFragments() error = %v", err)
	}
	for _, fragment := range fragments {
		if fragment.Key != "workspace" {
			continue
		}
		if !strings.Contains(fragment.Content, "Current workspace directory: /repo") {
			t.Fatalf("workspace fragment missing workspace directory wording: %#v", fragment)
		}
		if !strings.Contains(fragment.Content, "not the filesystem root `/`") {
			t.Fatalf("workspace fragment missing filesystem-root clarification: %#v", fragment)
		}
		if !strings.Contains(fragment.Content, "use `.` or another workspace-relative path, not `/`") {
			t.Fatalf("workspace fragment missing project-wide path guidance: %#v", fragment)
		}
		return
	}
	t.Fatal("expected workspace fragment in defaultTurnFragments output")
}

func TestRuntimeAllowedToolsForTurnUsesImplicitAllWhenAllowListMissing(t *testing.T) {
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	tools := runtime.allowedToolsForTurn(events.SessionState{}, agent.Definition{})
	if len(tools) == 0 {
		t.Fatalf("allowed tools = %#v, want runtime surface", tools)
	}
	if !slices.Contains(tools, "read") || !slices.Contains(tools, "write") {
		t.Fatalf("allowed tools = %#v, want read and write in implicit runtime surface", tools)
	}
}

func TestRuntimeAllowedToolsForTurnAppliesDisallowedToolsToImplicitAll(t *testing.T) {
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	tools := runtime.allowedToolsForTurn(events.SessionState{}, agent.Definition{
		DisallowedTools: []string{"write", "apply_patch"},
	})
	if slices.Contains(tools, "write") || slices.Contains(tools, "apply_patch") {
		t.Fatalf("allowed tools = %#v, want write/apply_patch removed", tools)
	}
	if !slices.Contains(tools, "read") {
		t.Fatalf("allowed tools = %#v, want remaining tools preserved", tools)
	}
}

func TestRuntimeAllowedToolsForTurnPreservesExplicitEmptyAllowList(t *testing.T) {
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	tools := runtime.allowedToolsForTurn(events.SessionState{}, agent.Definition{
		AllowedTools: []string{},
	})
	if tools == nil {
		t.Fatal("allowed tools = nil, want explicit empty surface")
	}
	if len(tools) != 0 {
		t.Fatalf("allowed tools = %#v, want none", tools)
	}
}
