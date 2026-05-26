package tool

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/internal/workspace"
)

var (
	ErrRuntimeOwnedExecution    = errors.New("generic execution is runtime-owned and must be executed by the runtime service")
	ErrCommandRequired          = errors.New("command is required")
	ErrCommandWorkingDirMissing = errors.New("working_directory must exist")
	ErrCommandWorkingDirNotDir  = errors.New("working_directory must be a directory")
	ErrShellCommandRequired     = errors.New("command is required")
)

func resolveCommandWorkingDir(scope *workspace.Scope, raw string) (string, error) {
	if scope == nil {
		return "", ErrWorkspaceRequired
	}
	target := raw
	if strings.TrimSpace(target) == "" {
		target = "."
	}
	decision, err := scope.Check(workspace.AccessWorkdir, target)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(decision.ResolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrCommandWorkingDirMissing
		}
		return "", err
	}
	if !info.IsDir() {
		return "", ErrCommandWorkingDirNotDir
	}
	return decision.ResolvedPath, nil
}

func unwrapShellWrapperCommand(command []string) (string, bool, bool) {
	if len(command) < 3 {
		return "", false, false
	}
	name := strings.ToLower(filepath.Base(strings.TrimSpace(command[0])))
	switch name {
	case "bash", "sh", "zsh", "dash", "ksh", "fish":
		for idx := 1; idx < len(command); idx++ {
			switch strings.TrimSpace(command[idx]) {
			case "-c", "-lc":
				if idx+1 < len(command) && strings.TrimSpace(command[idx+1]) != "" {
					return command[idx+1], command[idx] == "-lc", true
				}
				return "", false, false
			}
		}
	case "pwsh", "powershell", "powershell.exe", "pwsh.exe":
		for idx := 1; idx < len(command); idx++ {
			switch strings.TrimSpace(strings.ToLower(command[idx])) {
			case "-c", "-command":
				if idx+1 < len(command) && strings.TrimSpace(command[idx+1]) != "" {
					return command[idx+1], false, true
				}
				return "", false, false
			}
		}
	case "cmd", "cmd.exe":
		for idx := 1; idx < len(command); idx++ {
			switch strings.TrimSpace(strings.ToLower(command[idx])) {
			case "/c", "/k":
				if idx+1 < len(command) && strings.TrimSpace(command[idx+1]) != "" {
					return command[idx+1], false, true
				}
				return "", false, false
			}
		}
	}
	return "", false, false
}

func normalizeShellCommand(raw string) (string, bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false, ErrShellCommandRequired
	}
	loginShell := false
	if args, err := splitShellWords(trimmed); err == nil {
		if normalized, login, ok := unwrapShellWrapperCommand(args); ok {
			trimmed = strings.TrimSpace(normalized)
			loginShell = login
		}
	}
	if trimmed == "" {
		return "", false, ErrShellCommandRequired
	}
	return trimmed, loginShell, nil
}

func splitShellWords(raw string) ([]string, error) {
	args := make([]string, 0, 8)
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false
	flush := func() {
		if current.Len() == 0 {
			return
		}
		args = append(args, current.String())
		current.Reset()
	}
	for _, r := range raw {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		switch r {
		case '\\':
			if inSingle {
				current.WriteRune(r)
				continue
			}
			escaped = true
		case '\'':
			if inDouble {
				current.WriteRune(r)
				continue
			}
			inSingle = !inSingle
		case '"':
			if inSingle {
				current.WriteRune(r)
				continue
			}
			inDouble = !inDouble
		case ' ', '\t', '\n', '\r':
			if inSingle || inDouble {
				current.WriteRune(r)
				continue
			}
			flush()
		default:
			current.WriteRune(r)
		}
	}
	if escaped || inSingle || inDouble {
		return nil, errors.New("command contains unterminated shell quoting")
	}
	flush()
	return args, nil
}

func withinPath(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
