package app

import (
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/workspace"
)

type PermissionMode string

const (
	PermissionModeAuto       PermissionMode = "auto"
	PermissionModeReadOnly   PermissionMode = "read_only"
	PermissionModeFullAccess PermissionMode = "full_access"
)

func normalizePermissionMode(mode PermissionMode) PermissionMode {
	switch strings.TrimSpace(string(mode)) {
	case "", string(PermissionModeAuto):
		return PermissionModeAuto
	case string(PermissionModeReadOnly):
		return PermissionModeReadOnly
	case string(PermissionModeFullAccess):
		return PermissionModeFullAccess
	default:
		return PermissionModeAuto
	}
}

func statePermissionMode(state events.SessionState, fallback PermissionMode) PermissionMode {
	if mode := normalizePermissionMode(PermissionMode(state.PermissionMode)); mode != PermissionModeAuto || strings.TrimSpace(state.PermissionMode) != "" {
		return mode
	}
	return normalizePermissionMode(fallback)
}

func permissionModeGrantsFullAccess(mode PermissionMode) bool {
	return normalizePermissionMode(mode) == PermissionModeFullAccess
}

func permissionModeFilesystemGrant(root string) *workspace.Grant {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	hostRoot := string(filepath.Separator)
	if volume := filepath.VolumeName(root); volume != "" {
		hostRoot = volume + string(filepath.Separator)
	}
	grant := workspace.Grant{Path: hostRoot, Recursive: true}
	return &grant
}
