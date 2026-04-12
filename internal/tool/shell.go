package tool

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// shellBlacklist contains shells whose syntax is incompatible with POSIX-style
// `-c "command"` invocation used by bash/test tools (e.g. fish uses different
// quoting rules; nu uses a different command grammar).
var shellBlacklist = []string{"fish", "nu"}

// acceptableShell returns the user's preferred shell unless it is
// blacklisted (fish, nu), in which case a platform fallback is used.
func acceptableShell() string {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = shellFallback()
	}
	base := filepath.Base(sh)
	for _, b := range shellBlacklist {
		if strings.EqualFold(base, b) {
			return shellFallback()
		}
	}
	return sh
}

// shellExecArgs returns the shell binary and the "-c" flag used to run a
// command string.
func shellExecArgs(command string) (string, []string) {
	sh := acceptableShell()
	return sh, []string{"-c", command}
}

func shellFallback() string {
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{"zsh", "bash", "sh"}
	default:
		candidates = []string{"bash", "zsh", "sh"}
	}
	for _, name := range candidates {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return "/bin/sh"
}
