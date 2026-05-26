package tool

import "strings"

func PolicyListContainsTool(list []string, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, candidate := range list {
		if ToolPatternMatches(candidate, name) {
			return true
		}
	}
	return false
}

func ToolPatternMatches(pattern, name string) bool {
	pattern = strings.TrimSpace(pattern)
	name = strings.TrimSpace(name)
	if pattern == "" || name == "" {
		return false
	}
	if pattern == name {
		return true
	}
	return pattern == MCPToolWildcard && IsMCPToolName(name)
}
