package tool

import (
	"os"
	"path/filepath"
	"strings"
)

func resolveLiteralShellWorkingDir(currentWorkingDir string, args []string) (string, bool) {
	currentWorkingDir = strings.TrimSpace(currentWorkingDir)
	if currentWorkingDir == "" {
		return "", false
	}
	target, ok := literalShellWorkingDirTarget(args)
	if !ok {
		return "", false
	}
	return resolveLiteralShellWorkingDirTarget(currentWorkingDir, target)
}

func literalShellWorkingDirTarget(args []string) (string, bool) {
	targets := make([]string, 0, 1)
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "-") {
			if trimmed == "-" {
				return "", false
			}
			continue
		}
		targets = append(targets, trimmed)
	}
	switch len(targets) {
	case 0:
		return "~", true
	case 1:
		return targets[0], true
	default:
		return "", false
	}
}

func resolveLiteralShellWorkingDirTarget(currentWorkingDir, target string) (string, bool) {
	currentWorkingDir = strings.TrimSpace(currentWorkingDir)
	target = strings.TrimSpace(target)
	if currentWorkingDir == "" || target == "" || target == "-" {
		return "", false
	}
	switch {
	case filepath.IsAbs(target):
		return filepath.Clean(target), true
	case target == "~" || strings.HasPrefix(target, "~/"):
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false
		}
		if target == "~" {
			return home, true
		}
		return filepath.Join(home, strings.TrimPrefix(target, "~/")), true
	default:
		return filepath.Clean(filepath.Join(currentWorkingDir, target)), true
	}
}

func canonicalShellWorkingDir(path string) string {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	if cleaned == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		return filepath.Clean(resolved)
	}
	return cleaned
}

func isShellChangeDirectoryBuiltin(command string) bool {
	return filepath.Base(strings.TrimSpace(command)) == "cd"
}

func isDirectoryChangingShellBuiltin(command string) bool {
	switch filepath.Base(strings.TrimSpace(command)) {
	case "cd", "pushd", "popd":
		return true
	default:
		return false
	}
}
