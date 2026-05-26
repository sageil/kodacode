package tui

import (
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func toolPrimaryListSummary(call *events.ToolCallState) string {
	if presenter, ok := toolPresenterForCall(call); ok && presenter.ListSummary != nil {
		return presenter.ListSummary(call)
	}
	return ""
}

func readToolListSummary(call *events.ToolCallState) string {
	input, ok := parseReadToolViewInput(call.Input)
	if !ok {
		return ""
	}
	lines := append([]string(nil), input.Paths...)
	if reused := reusedToolCallLabel(call); reused != "" {
		lines = append(lines, reused)
	}
	if input.HasOffset {
		lines = append(lines, fmt.Sprintf("offset: %d", input.Offset))
	}
	if input.HasLimit {
		lines = append(lines, fmt.Sprintf("limit: %d", input.Limit))
	}
	return strings.Join(lines, "\n")
}

func reusedToolCallLabel(call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	if callID := strings.TrimSpace(call.ReusedFromCallID); callID != "" {
		return "reused from: " + callID
	}
	return ""
}

func treeToolListSummary(call *events.ToolCallState) string {
	input, ok := parseTreeToolViewInput(call.Input)
	if !ok {
		return ""
	}
	return strings.Join([]string{
		input.Path,
		fmt.Sprintf("max depth: %d", input.MaxDepth),
		"hidden: " + onOffLabel(input.IncludeHidden),
	}, "\n")
}

func gitStatusToolListSummary(call *events.ToolCallState) string {
	return "scope: workspace root"
}

func gitDiffToolListSummary(call *events.ToolCallState) string {
	input, ok := parseGitDiffToolViewInput(call.Input)
	if !ok {
		return ""
	}
	return "scope: " + gitDiffScopeLabel(input.Staged)
}

func gitShowToolListSummary(call *events.ToolCallState) string {
	input, ok := parseGitShowToolViewInput(call.Input)
	if !ok {
		return ""
	}
	return strings.Join([]string{
		"rev: " + input.Rev,
		"scope: workspace root",
	}, "\n")
}

func searchToolListSummary(call *events.ToolCallState) string {
	input, ok := parseSearchToolViewInput(call.Input)
	if !ok {
		return ""
	}
	lines := []string{
		"query: " + input.Query,
		"path: " + input.Path,
		"mode: " + input.Mode,
		"regex: " + onOffLabel(input.Regex),
	}
	if strings.TrimSpace(input.Glob) != "" {
		lines = append(lines, "filter: "+input.Glob)
	}
	return strings.Join(lines, "\n")
}

func locateToolListSummary(call *events.ToolCallState) string {
	input, ok := parseLocateToolViewInput(call.Input)
	if !ok {
		return ""
	}
	return strings.Join([]string{
		"query: " + input.Query,
		"path: " + input.Path,
		"hidden: " + onOffLabel(input.IncludeHidden),
	}, "\n")
}

func webFetchToolListSummary(call *events.ToolCallState) string {
	input, ok := parseWebFetchToolViewInput(call.Input)
	if !ok {
		return ""
	}
	lines := []string{"url: " + input.URL}
	if input.Format != "" && input.Format != "auto" {
		lines = append(lines, "format: "+input.Format)
	}
	return strings.Join(lines, "\n")
}

func bashToolListSummary(call *events.ToolCallState) string {
	_, ok := parseBashToolViewInput(call.Input)
	if !ok {
		return ""
	}
	return ""
}

func testToolListSummary(call *events.ToolCallState) string {
	input, ok := parseTestToolViewInput(call.Input)
	if !ok {
		return ""
	}
	lines := make([]string, 0, 3)
	if input.Path != "" && input.Path != "." {
		lines = append(lines, "path: "+input.Path)
	}
	if input.Filter != "" {
		lines = append(lines, "filter: "+input.Filter)
	}
	if input.Timeout > 0 {
		lines = append(lines, "timeout: "+formatMilliseconds(input.Timeout))
	}
	return strings.Join(lines, "\n")
}

func isMCPToolCall(call *events.ToolCallState) bool {
	return call != nil && tool.IsMCPToolName(strings.TrimSpace(call.ToolName))
}

func toolDisplayNameForSession(state events.SessionState, call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	if label := mcpToolDisplayName(state, call); label != "" {
		return label
	}
	return toolDisplayNameForWorkspace(state.WorkspaceRoot, call)
}

func toolDisplayNameForWorkspace(workspaceRoot string, call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	if label := mcpToolDisplayName(events.SessionState{}, call); label != "" {
		return label
	}
	if presenter, ok := toolPresenterForCall(call); ok && presenter.DisplayName != nil {
		return presenter.DisplayName(workspaceRoot, call)
	}
	return strings.TrimSpace(call.ToolName)
}

func mcpToolDisplayName(state events.SessionState, call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	name := strings.TrimSpace(call.ToolName)
	if !tool.IsMCPToolName(name) {
		return ""
	}
	serverName, remoteToolName, ok := mcpToolDisplayParts(state, name)
	if !ok {
		return ""
	}
	if remoteToolName == "" || sameMCPDisplayName(serverName, remoteToolName) {
		return serverName
	}
	return serverName + " · " + remoteToolName
}

func mcpToolDisplayParts(state events.SessionState, toolName string) (string, string, bool) {
	name := strings.TrimSpace(toolName)
	if state.MCP != nil {
		for _, tl := range state.MCP.Tools {
			if strings.TrimSpace(tl.Name) != name {
				continue
			}
			serverName := strings.TrimSpace(tl.ServerName)
			remoteName := strings.TrimSpace(tl.RemoteName)
			if serverName != "" || remoteName != "" {
				return serverName, remoteName, true
			}
		}
	}
	serverComponent, remoteComponent, ok := tool.ParseMCPToolName(name)
	if !ok {
		return "", "", false
	}
	if state.MCP != nil {
		for _, server := range state.MCP.Servers {
			serverName := strings.TrimSpace(server.Name)
			if serverName == "" {
				continue
			}
			if tool.MCPToolNameComponent(serverName) == serverComponent {
				return serverName, displayMCPToolComponent(remoteComponent), true
			}
		}
	}
	return displayMCPToolComponent(serverComponent), displayMCPToolComponent(remoteComponent), true
}

func sameMCPDisplayName(left, right string) bool {
	normalize := func(value string) string {
		replacer := strings.NewReplacer("-", "", "_", "", " ", "")
		return strings.ToLower(strings.TrimSpace(replacer.Replace(value)))
	}
	return normalize(left) != "" && normalize(left) == normalize(right)
}

func displayMCPToolComponent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.ReplaceAll(value, "_", "-")
}

func isBashToolCall(call *events.ToolCallState) bool {
	return call != nil && strings.TrimSpace(call.ToolName) == "bash"
}

func isTestToolCall(call *events.ToolCallState) bool {
	return call != nil && strings.TrimSpace(call.ToolName) == "test"
}

func isCommandToolCall(call *events.ToolCallState) bool {
	return isTestToolCall(call)
}

func commandToolDisplayName(workspaceRoot string, call *events.ToolCallState) string {
	switch {
	case isBashToolCall(call):
		return bashCommandDisplayName(workspaceRoot, call)
	case isTestToolCall(call):
		return testCommandDisplayName(workspaceRoot, call)
	default:
		return strings.TrimSpace(call.ToolName)
	}
}

func readToolDisplayName(workspaceRoot string, call *events.ToolCallState) string {
	input, ok := parseReadToolViewInput(call.Input)
	if !ok || len(input.Paths) == 0 {
		return "read"
	}
	paths := make([]string, 0, len(input.Paths))
	for _, path := range input.Paths {
		paths = append(paths, displayToolPath(workspaceRoot, path))
	}
	return "read " + strings.Join(paths, ", ")
}

func writeToolDisplayName(workspaceRoot string, call *events.ToolCallState) string {
	input, ok := parseWriteToolViewInput(call.Input)
	if !ok {
		return "write"
	}
	return "write " + displayToolPath(workspaceRoot, input.Path)
}

func editToolDisplayName(workspaceRoot string, call *events.ToolCallState) string {
	input, ok := parseEditToolViewInput(call.Input)
	if !ok {
		return "edit"
	}
	label := "edit " + displayToolPath(workspaceRoot, input.Path)
	if detail := strings.TrimSpace(editMutationCompactDiffLabel(call, input)); detail != "" {
		label += " (" + detail + ")"
	} else if detail := strings.TrimSpace(editToolMatchLabel(input)); detail != "" {
		label += " · " + detail
	}
	return label
}

func searchToolDisplayName(workspaceRoot string, call *events.ToolCallState) string {
	input, ok := parseSearchToolViewInput(call.Input)
	if !ok {
		return "search"
	}
	path := displayToolPath(workspaceRoot, input.Path)
	if path == "." {
		path = ""
	}
	query := searchToolQueryLabel(input.Query, input.Regex)
	switch {
	case path != "" && query != "":
		return "search " + path + " · " + query
	case path != "":
		return "search " + path
	case query != "":
		return "search · " + query
	default:
		return "search"
	}
}

func locateToolDisplayName(workspaceRoot string, call *events.ToolCallState) string {
	input, ok := parseLocateToolViewInput(call.Input)
	if !ok {
		return "locate"
	}
	path := displayToolPath(workspaceRoot, input.Path)
	if path == "." {
		path = ""
	}
	query := strings.TrimSpace(input.Query)
	switch {
	case path != "" && query != "":
		return "locate " + path + " · " + query
	case path != "":
		return "locate " + path
	case query != "":
		return "locate · " + query
	default:
		return "locate"
	}
}

func webFetchToolDisplayName(call *events.ToolCallState) string {
	input, ok := parseWebFetchToolViewInput(call.Input)
	if !ok || strings.TrimSpace(input.URL) == "" {
		return "web_fetch"
	}
	return "web_fetch " + strings.TrimSpace(input.URL)
}

func searchToolQueryLabel(query string, regex bool) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	if regex {
		return "/" + query + "/"
	}
	return query
}

func writeToolListSummary(call *events.ToolCallState) string {
	display, ok := mutationDisplayFromCall("", call)
	if !ok {
		return ""
	}
	return display.Summary
}

func editToolListSummary(call *events.ToolCallState) string {
	display, ok := mutationDisplayFromCall("", call)
	if !ok {
		return ""
	}
	return display.Summary
}

func mkdirToolListSummary(call *events.ToolCallState) string {
	input, ok := parseMkdirToolViewInput(call.Input)
	if !ok {
		return ""
	}
	return mutationSummaryLabel(call, "created", "create") + " " + input.Path
}
