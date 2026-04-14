package tui

import (
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

const workspaceStatusRefreshInterval = 5 * time.Second

type workspaceStatusMsg struct {
	branch       string
	changedFiles int
	lspServers   []string
}

type workspaceStatusTickMsg struct{}

func (a App) loadWorkspaceStatus() tea.Cmd {
	projectDir := a.projectDir
	lspManager := a.lspManager

	return func() tea.Msg {
		branch, changedFiles := currentGitWorkspaceStatus(projectDir)

		var lspServers []string
		if lspManager != nil {
			lspServers = lspManager.RunningServerNames()
		}

		return workspaceStatusMsg{
			branch:       branch,
			changedFiles: changedFiles,
			lspServers:   lspServers,
		}
	}
}

func (a App) workspaceStatusTick() tea.Cmd {
	return tea.Tick(workspaceStatusRefreshInterval, func(time.Time) tea.Msg {
		return workspaceStatusTickMsg{}
	})
}

func currentGitWorkspaceStatus(projectDir string) (branch string, changedFiles int) {
	cmd := exec.Command("git", "status", "--porcelain=1", "--branch", "--untracked-files=normal")
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err != nil {
		return "", 0
	}
	return parseGitStatusPorcelain(string(out))
}

func parseGitStatusPorcelain(output string) (branch string, changedFiles int) {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) == 0 {
		return "", 0
	}

	if strings.HasPrefix(lines[0], "## ") {
		branch = strings.TrimSpace(strings.TrimPrefix(lines[0], "## "))
		switch {
		case strings.HasPrefix(branch, "No commits yet on "):
			branch = strings.TrimPrefix(branch, "No commits yet on ")
		case strings.HasPrefix(branch, "HEAD "):
			branch = strings.TrimPrefix(branch, "HEAD ")
		}
		if idx := strings.Index(branch, "..."); idx >= 0 {
			branch = branch[:idx]
		}
	}

	for _, line := range lines[1:] {
		if strings.TrimSpace(line) != "" {
			changedFiles++
		}
	}
	return branch, changedFiles
}
