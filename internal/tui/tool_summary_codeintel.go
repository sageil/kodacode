package tui

import (
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func definitionToolListSummary(call *events.ToolCallState) string {
	input, ok := parseDefinitionToolViewInput(call.Input)
	if !ok {
		return ""
	}
	return strings.Join([]string{
		"path: " + input.Path,
		fmt.Sprintf("line: %d", input.Line),
		fmt.Sprintf("character: %d", input.Character),
	}, "\n")
}

func diagnosticsToolListSummary(call *events.ToolCallState) string {
	input, ok := parseDiagnosticsToolViewInput(call.Input)
	if !ok {
		return ""
	}
	return strings.Join(input.Paths, "\n")
}

func symbolsToolListSummary(call *events.ToolCallState) string {
	input, ok := parseSymbolsToolViewInput(call.Input)
	if !ok {
		return ""
	}
	return "query: " + input.Query
}

func refsToolListSummary(call *events.ToolCallState) string {
	input, ok := parseRefsToolViewInput(call.Input)
	if !ok {
		return ""
	}
	lines := []string{
		"path: " + input.Path,
		fmt.Sprintf("line: %d", input.Line),
		fmt.Sprintf("character: %d", input.Character),
		"mode: " + input.Mode,
	}
	if input.MaxResults > 0 {
		lines = append(lines, fmt.Sprintf("max results: %d", input.MaxResults))
	}
	lines = append(lines, "include declaration: "+onOffLabel(input.IncludeDeclaration))
	return strings.Join(lines, "\n")
}

func traceToolListSummary(call *events.ToolCallState) string {
	input, ok := parseTraceToolViewInput(call.Input)
	if !ok {
		return ""
	}
	lines := []string{
		"path: " + input.Path,
		fmt.Sprintf("line: %d", input.Line),
		fmt.Sprintf("character: %d", input.Character),
		"mode: " + input.Mode,
	}
	if input.Depth > 0 {
		lines = append(lines, fmt.Sprintf("depth: %d", input.Depth))
	}
	if input.MaxNodes > 0 {
		lines = append(lines, fmt.Sprintf("max nodes: %d", input.MaxNodes))
	}
	return strings.Join(lines, "\n")
}

func renameSymbolToolListSummary(call *events.ToolCallState) string {
	input, ok := parseRenameSymbolToolViewInput(call.Input)
	if !ok {
		return ""
	}
	return strings.Join([]string{
		"path: " + input.Path,
		fmt.Sprintf("line: %d", input.Line),
		fmt.Sprintf("character: %d", input.Character),
		"new name: " + input.NewName,
	}, "\n")
}

func codeActionToolListSummary(call *events.ToolCallState) string {
	input, ok := parseCodeActionToolViewInput(call.Input)
	if !ok {
		return ""
	}
	lines := []string{
		"path: " + input.Path,
		fmt.Sprintf("range: %d:%d-%d:%d", input.StartLine, input.StartCharacter, input.EndLine, input.EndCharacter),
	}
	if input.Title != nil && strings.TrimSpace(*input.Title) != "" {
		lines = append(lines, "title: "+strings.TrimSpace(*input.Title))
	}
	if input.Kind != nil && strings.TrimSpace(*input.Kind) != "" {
		lines = append(lines, "kind: "+strings.TrimSpace(*input.Kind))
	}
	if input.OnlyPreferred != nil {
		lines = append(lines, "preferred: "+onOffLabel(*input.OnlyPreferred))
	}
	return strings.Join(lines, "\n")
}

func definitionToolDisplayName(workspaceRoot string, call *events.ToolCallState) string {
	input, ok := parseDefinitionToolViewInput(call.Input)
	if !ok {
		return "Definition"
	}
	return "Definition " + displayToolPath(workspaceRoot, input.Path)
}

func diagnosticsToolDisplayName(workspaceRoot string, call *events.ToolCallState) string {
	input, ok := parseDiagnosticsToolViewInput(call.Input)
	if !ok {
		return "Diagnostics"
	}
	if len(input.Paths) == 1 {
		return "Diagnostics " + displayToolPath(workspaceRoot, input.Paths[0])
	}
	return fmt.Sprintf("Diagnostics %d files", len(input.Paths))
}

func symbolsToolDisplayName(call *events.ToolCallState) string {
	input, ok := parseSymbolsToolViewInput(call.Input)
	if !ok {
		return "Symbols"
	}
	return "Symbols " + input.Query
}

func refsToolDisplayName(workspaceRoot string, call *events.ToolCallState) string {
	input, ok := parseRefsToolViewInput(call.Input)
	if !ok {
		return "Refs"
	}
	return "Refs " + displayToolPath(workspaceRoot, input.Path) + " · " + input.Mode
}

func traceToolDisplayName(workspaceRoot string, call *events.ToolCallState) string {
	input, ok := parseTraceToolViewInput(call.Input)
	if !ok {
		return "Trace"
	}
	return "Trace " + displayToolPath(workspaceRoot, input.Path) + " · " + input.Mode
}

func renameSymbolToolDisplayName(workspaceRoot string, call *events.ToolCallState) string {
	input, ok := parseRenameSymbolToolViewInput(call.Input)
	if !ok {
		return "Rename Symbol"
	}
	return "Rename " + displayToolPath(workspaceRoot, input.Path)
}

func codeActionToolDisplayName(workspaceRoot string, call *events.ToolCallState) string {
	input, ok := parseCodeActionToolViewInput(call.Input)
	if !ok {
		return "Code Action"
	}
	path := displayToolPath(workspaceRoot, input.Path)
	if input.Title != nil && strings.TrimSpace(*input.Title) != "" {
		return "Code Action " + path + " · " + strings.TrimSpace(*input.Title)
	}
	if input.Kind != nil && strings.TrimSpace(*input.Kind) != "" {
		return "Code Action " + path + " · " + strings.TrimSpace(*input.Kind)
	}
	return "Code Action " + path
}
