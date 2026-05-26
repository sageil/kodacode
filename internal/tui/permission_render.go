package tui

import (
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func permissionKindLabel(kind events.PermissionRequestKind, access string) string {
	switch kind {
	case events.PermissionRequestKindExecution:
		return "command execution"
	case events.PermissionRequestKindNetwork:
		return "network"
	default:
		return permissionAccessLabel(access)
	}
}

func permissionTargetLabel(kind events.PermissionRequestKind, access string) string {
	switch kind {
	case events.PermissionRequestKindExecution:
		return "dir"
	case events.PermissionRequestKindNetwork:
		return "target"
	default:
		if access == "workdir" {
			return "dir"
		}
		return "path"
	}
}

func permissionAccessLabel(access string) string {
	switch access {
	case "list", "file", "read":
		return "read-only"
	case "workdir":
		return "working directory"
	case "write", "edit":
		return "write"
	case "exec", "command":
		return "execute"
	case "network":
		return "network"
	default:
		return access
	}
}

func executionApprovalKindLabel(pending events.ExecutionApprovalState) string {
	reason := strings.TrimSpace(pending.Reason)
	needsNetwork := len(pending.NetworkTargets) > 0 ||
		pending.ProposedNetworkPolicy != nil ||
		strings.Contains(reason, "network access")
	needsExecution := len(pending.SessionGrantPaths) > 0 ||
		strings.Contains(reason, "command execution") ||
		strings.Contains(reason, "working directory access")

	switch {
	case needsNetwork && needsExecution:
		return "command execution + network access"
	case needsNetwork:
		return "network access"
	default:
		return "command execution"
	}
}

func displayWorkingDirectory(workspaceRoot, path string) string {
	display := displaySessionPath(workspaceRoot, path)
	if strings.TrimSpace(display) == "." {
		return "workspace root"
	}
	return display
}

func displaySessionPath(workspaceRoot, path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	if strings.TrimSpace(workspaceRoot) == "" {
		return path
	}
	rel, err := filepath.Rel(workspaceRoot, path)
	if err != nil {
		return path
	}
	if rel == "." {
		return "."
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return rel
}
