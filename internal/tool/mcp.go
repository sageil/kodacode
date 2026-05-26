package tool

import "strings"

const MCPToolNamePrefix = "mcp_"
const MCPToolWildcard = "mcp:*"
const mcpToolComponentSeparator = "__"

func IsMCPToolName(name string) bool {
	return strings.HasPrefix(strings.TrimSpace(name), MCPToolNamePrefix)
}

func MCPToolName(serverName, remoteToolName string) string {
	serverComponent := MCPToolNameComponent(serverName)
	if serverComponent == "" {
		serverComponent = "server"
	}
	remoteComponent := MCPToolNameComponent(remoteToolName)
	if remoteComponent == "" {
		remoteComponent = "tool"
	}
	return MCPToolNamePrefix + serverComponent + mcpToolComponentSeparator + remoteComponent
}

func MCPToolNameComponent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
			lastUnderscore = false
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r + ('a' - 'A'))
			lastUnderscore = false
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastUnderscore = false
		default:
			if lastUnderscore || builder.Len() == 0 {
				continue
			}
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(builder.String(), "_")
}

func ParseMCPToolName(name string) (string, string, bool) {
	name = strings.TrimSpace(name)
	if !IsMCPToolName(name) {
		return "", "", false
	}
	trimmed := strings.TrimPrefix(name, MCPToolNamePrefix)
	if trimmed == "" {
		return "", "", false
	}
	if separator := strings.Index(trimmed, mcpToolComponentSeparator); separator >= 0 {
		serverComponent := strings.TrimSpace(trimmed[:separator])
		remoteComponent := strings.TrimSpace(trimmed[separator+len(mcpToolComponentSeparator):])
		if serverComponent == "" && remoteComponent == "" {
			return "", "", false
		}
		return serverComponent, remoteComponent, true
	}
	// Support the older single-underscore MCP naming pattern for existing sessions.
	if separator := strings.Index(trimmed, "_"); separator >= 0 {
		serverComponent := strings.TrimSpace(trimmed[:separator])
		remoteComponent := strings.TrimSpace(trimmed[separator+1:])
		if serverComponent == "" && remoteComponent == "" {
			return "", "", false
		}
		return serverComponent, remoteComponent, true
	}
	return trimmed, "", true
}
