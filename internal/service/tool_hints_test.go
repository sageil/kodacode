package service

import (
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/provider"
)

func TestCompactToolDescription_UsesMetadataSummary(t *testing.T) {
	got := compactToolDescription("search", "A much longer tool description that should not survive compact mode.")
	if got != "Broad project/code search" {
		t.Fatalf("compactToolDescription() = %q, want metadata summary", got)
	}
}

func TestReorderToolsForTurn_PreservesAllTools(t *testing.T) {
	tools := []provider.Tool{
		{Name: "edit", Description: "Precise file edits"},
		{Name: "search", Description: "Broad project search"},
		{Name: "lsp", Description: "Symbol lookup, references, diagnostics"},
	}

	got := reorderToolsForTurn(tools, "Find references before editing", nil)
	if len(got) != len(tools) {
		t.Fatalf("len(reorderToolsForTurn) = %d, want %d", len(got), len(tools))
	}

	seen := make(map[string]bool, len(got))
	for _, tool := range got {
		seen[tool.Name] = true
	}
	for _, tool := range tools {
		if !seen[tool.Name] {
			t.Fatalf("missing tool %q after reordering", tool.Name)
		}
	}
}

func TestBuildRelevantToolsOverlay_IncludesGuidance(t *testing.T) {
	overlay := buildRelevantToolsOverlay([]provider.Tool{
		{Name: "search", Description: "Broad project search"},
		{Name: "lsp", Description: "Symbol lookup, references, diagnostics"},
		{Name: "edit", Description: "Precise file edits"},
	}, "Find all references to SessionService", nil)

	if !strings.Contains(overlay, "Likely Relevant Tools For This Turn") {
		t.Fatalf("overlay missing heading: %q", overlay)
	}
	if !strings.Contains(overlay, "Use first for broad intent or codebase discovery.") {
		t.Fatalf("overlay missing search guidance: %q", overlay)
	}
	if !strings.Contains(overlay, "Best for symbols, callers, definitions, and diagnostics.") {
		t.Fatalf("overlay missing lsp guidance: %q", overlay)
	}
}

func TestReorderToolsForTurn_PrioritizesDirectURLFetch(t *testing.T) {
	tools := []provider.Tool{
		{Name: "search", Description: "Broad project search"},
		{Name: "web_fetch", Description: "Fetch a URL"},
	}

	got := reorderToolsForTurn(tools, "Fetch https://example.com/docs/reference", nil)
	if got[0].Name != "web_fetch" {
		t.Fatalf("first tool = %q, want web_fetch for direct URL retrieval", got[0].Name)
	}

	overlay := buildRelevantToolsOverlay(tools, "Fetch https://example.com/docs/reference", nil)
	if !strings.Contains(overlay, "not a web search tool") {
		t.Fatalf("overlay missing direct-url guidance: %q", overlay)
	}
}

func TestReorderToolsForTurn_PrioritizesMultipleExplicitPaths(t *testing.T) {
	tools := []provider.Tool{
		{Name: "read", Description: "Read one file"},
		{Name: "read_files", Description: "Read several files"},
		{Name: "bash", Description: "Run shell commands"},
	}

	got := reorderToolsForTurn(tools, "Compare internal/service/session.go and cmd/kodacode/runtime.go", []string{
		"internal/service/session.go",
		"cmd/kodacode/runtime.go",
	})
	if got[0].Name != "read_files" {
		t.Fatalf("first tool = %q, want read_files for multiple explicit paths", got[0].Name)
	}
}

func TestReorderToolsForTurn_PrioritizesDerivedSpecializedTool(t *testing.T) {
	tools := []provider.Tool{
		{Name: "bash", Description: "Run shell commands"},
		{Name: "github_search_issues", Description: "Search GitHub issues and pull requests by query"},
	}

	got := reorderToolsForTurn(tools, "Search GitHub issues about session busy", nil)
	if got[0].Name != "github_search_issues" {
		t.Fatalf("first tool = %q, want derived specialized tool", got[0].Name)
	}

	overlay := buildRelevantToolsOverlay(tools, "Search GitHub issues about session busy", nil)
	if !strings.Contains(overlay, "Prefer this specialized read/search tool before shelling out") {
		t.Fatalf("overlay missing derived MCP-style guidance: %q", overlay)
	}
}

func TestToolPromptMetaForTool_AddsHintTriggersToBuiltinMetadata(t *testing.T) {
	meta := toolPromptMetaForTool(provider.Tool{
		Name: "web_fetch",
		PromptHints: provider.ToolPromptHints{
			Triggers: []string{"vendor docs"},
		},
	})
	foundBase := false
	foundHint := false
	for _, trigger := range meta.Triggers {
		if trigger == "fetch direct url" {
			foundBase = true
		}
		if trigger == "vendor docs" {
			foundHint = true
		}
	}
	if !foundBase || !foundHint {
		t.Fatalf("merged triggers = %v, want base + hint trigger", meta.Triggers)
	}
}
