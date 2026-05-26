// Package configdir resolves kodacode user configuration directories.
package configdir

import (
	"os"
	"path/filepath"
	"strings"
)

// Root returns kodacode's user config root.
//
// Resolution order:
//  1. $XDG_CONFIG_HOME/kodacode
//  2. ~/.config/kodacode
//  3. a temp fallback if HOME is unavailable
func Root() string {
	return resolveRoot(os.Getenv, os.UserHomeDir, os.TempDir)
}

func resolveRoot(getenv func(string) string, userHomeDir func() (string, error), tempDir func() string) string {
	if xdg := strings.TrimSpace(getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, "kodacode")
	}
	home, err := userHomeDir()
	if err != nil {
		return filepath.Join(tempDir(), "kodacode-config")
	}
	return filepath.Join(home, ".config", "kodacode")
}
