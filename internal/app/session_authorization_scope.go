package app

import (
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/workspace"
)

func scopeFromState(state events.SessionState, extraGrants ...workspace.Grant) (*workspace.Scope, error) {
	if strings.TrimSpace(state.WorkspaceRoot) == "" {
		return nil, ErrSessionNotConfigured
	}
	grants := make([]workspace.Grant, 0, len(state.WorkspaceGrants)+len(state.AdditionalWorkspaceRoots)+len(extraGrants))
	for _, root := range state.AdditionalWorkspaceRoots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		grants = append(grants, workspace.Grant{
			Path:      root,
			Recursive: true,
		})
	}
	for _, grant := range state.WorkspaceGrants {
		grants = append(grants, workspace.Grant{
			Path:      grant.Path,
			Recursive: grant.Recursive,
		})
	}
	grants = append(grants, extraGrants...)
	if permissionModeGrantsFullAccess(statePermissionMode(state, PermissionModeAuto)) {
		if grant := permissionModeFilesystemGrant(state.WorkspaceRoot); grant != nil {
			grants = append(grants, *grant)
		}
	}
	return workspace.New(state.WorkspaceRoot, workspace.Options{Grants: grants})
}

func withinSessionWorkspace(root, path string) bool {
	root = strings.TrimSpace(root)
	path = strings.TrimSpace(path)
	if root == "" || path == "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func withinSessionWorkspaceRoots(roots []string, path string) bool {
	for _, root := range roots {
		if withinSessionWorkspace(root, path) {
			return true
		}
	}
	return false
}

func sessionWorkspaceRoots(state events.SessionState) []string {
	roots := make([]string, 0, 1+len(state.AdditionalWorkspaceRoots))
	if trimmed := strings.TrimSpace(state.WorkspaceRoot); trimmed != "" {
		roots = append(roots, trimmed)
	}
	for _, root := range state.AdditionalWorkspaceRoots {
		trimmed := strings.TrimSpace(root)
		if trimmed == "" {
			continue
		}
		if withinSessionWorkspaceRoots(roots, trimmed) {
			continue
		}
		roots = append(roots, trimmed)
	}
	return roots
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
