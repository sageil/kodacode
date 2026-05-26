package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

func openSystemBrowser(rawURL string) (bool, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return false, fmt.Errorf("browser url is required")
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "linux":
		cmd = exec.Command("xdg-open", rawURL)
	default:
		return true, nil
	}
	if err := cmd.Run(); err != nil {
		return true, err
	}
	return false, nil
}
