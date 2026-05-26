package app

import (
	"strings"

	"github.com/sageil/kodacode/internal/tool"
)

func toolPathPermissionPreview(toolName string, request tool.PathRequest) string {
	name := strings.TrimSpace(toolName)
	target := strings.TrimSpace(request.Path)
	switch {
	case name == "" && target == "":
		return ""
	case name == "":
		return target
	case target == "":
		return name
	default:
		return name + " " + target
	}
}

func toolNetworkPermissionPreview(toolName string, request tool.NetworkRequest) string {
	command := strings.TrimSpace(request.Command)
	if command != "" {
		return command
	}
	name := strings.TrimSpace(toolName)
	target := strings.TrimSpace(request.Target)
	switch {
	case name == "" && target == "":
		return ""
	case name == "":
		return target
	case target == "":
		return name
	default:
		return name + " " + target
	}
}
