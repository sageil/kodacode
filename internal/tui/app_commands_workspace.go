package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	runtimepprof "runtime/pprof"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type undoResultMsg struct {
	output       string
	err          string
	pendingFile  string
	clearPending bool
}

func (a App) handleUndoCommand(arg string) (App, tea.Cmd, bool) {
	confirm, file := parseUndoConfirm(arg)
	if !confirm {
		return a, runUndoPreviewCommand(arg), true
	}

	if file == "" {
		file = a.pendingUndoFile
	}
	if file == "" {
		return a, a.showErrorToast("Preview a file first with `/undo <file>`, then run `/undo confirm <file>`."), true
	}
	if a.pendingUndoFile == "" {
		return a, a.showErrorToast(fmt.Sprintf("Preview required. Run `/undo %s` before confirming.", file)), true
	}
	if a.pendingUndoFile != file {
		return a, a.showErrorToast(fmt.Sprintf("Pending undo is %q. Preview %q first before confirming it.", a.pendingUndoFile, file)), true
	}
	return a, confirmUndoCommand(file), true
}

func parseUndoConfirm(arg string) (bool, string) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return false, ""
	}
	fields := strings.Fields(arg)
	if len(fields) == 0 {
		return false, ""
	}
	switch fields[0] {
	case "confirm", "--confirm":
		return true, strings.TrimSpace(strings.TrimPrefix(arg, fields[0]))
	default:
		return false, arg
	}
}

func runUndoPreviewCommand(arg string) tea.Cmd {
	target := strings.TrimSpace(arg)
	return func() tea.Msg {
		if target != "" {
			return previewUndoFile(target)
		}

		files, err := listUndoCandidates()
		if err != nil {
			return undoResultMsg{err: err.Error()}
		}
		if len(files) == 0 {
			return undoResultMsg{err: "No unstaged tracked file changes to revert."}
		}
		if len(files) == 1 {
			return previewUndoFile(files[0])
		}

		var sb strings.Builder
		sb.WriteString("Multiple files have unstaged tracked changes.\n")
		sb.WriteString("Preview one first:\n")
		for _, f := range files {
			sb.WriteString("  /undo " + f + "\n")
		}
		return undoResultMsg{err: sb.String()}
	}
}

func listUndoCandidates() ([]string, error) {
	out, err := exec.Command("git", "diff", "--name-only").Output()
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %w", err)
	}
	files := splitNonEmpty(strings.TrimSpace(string(out)))
	if len(files) > 0 {
		return files, nil
	}

	staged, _ := exec.Command("git", "diff", "--cached", "--name-only").Output()
	if sf := splitNonEmpty(strings.TrimSpace(string(staged))); len(sf) > 0 {
		var sb strings.Builder
		sb.WriteString("No unstaged changes. These files have staged changes:\n")
		for _, f := range sf {
			sb.WriteString("  " + f + "\n")
		}
		sb.WriteString("Unstage them first with `git restore --staged -- <file>` or `git reset HEAD <file>`.")
		return nil, fmt.Errorf("%s", sb.String())
	}
	return nil, nil
}

func previewUndoFile(file string) undoResultMsg {
	if staged, err := hasStagedChanges(file); err != nil {
		return undoResultMsg{err: err.Error()}
	} else if staged {
		return undoResultMsg{err: "Cannot undo a file with staged changes. Unstage it first with `git restore --staged -- " + file + "`."}
	}

	status, _ := exec.Command("git", "status", "--short", "--", file).Output()
	statusText := strings.TrimSpace(string(status))
	if strings.HasPrefix(statusText, "?? ") {
		return undoResultMsg{err: "Untracked files are not reverted by `/undo`. Delete it manually if you want to remove " + file}
	}

	diffOut, _ := exec.Command("git", "diff", "--stat", "--", file).Output()
	diffSummary := strings.TrimSpace(string(diffOut))
	if diffSummary == "" {
		return undoResultMsg{err: "No unstaged tracked changes found in " + file}
	}

	var sb strings.Builder
	sb.WriteString("Preview revert for " + file + "\n\n")
	sb.WriteString(diffSummary)
	sb.WriteString("\n\nRun `/undo confirm ")
	sb.WriteString(file)
	sb.WriteString("` to discard the unstaged changes in this file with `git restore --worktree --source=HEAD -- ")
	sb.WriteString(file)
	sb.WriteString("`.")
	return undoResultMsg{output: sb.String(), pendingFile: file}
}

func confirmUndoCommand(file string) tea.Cmd {
	return func() tea.Msg {
		return confirmUndoFile(file)
	}
}

func confirmUndoFile(file string) undoResultMsg {
	if staged, err := hasStagedChanges(file); err != nil {
		return undoResultMsg{err: err.Error()}
	} else if staged {
		return undoResultMsg{err: "Refusing to revert while staged changes exist for " + file, clearPending: true}
	}

	diffOut, _ := exec.Command("git", "diff", "--stat", "--", file).Output()
	diffSummary := strings.TrimSpace(string(diffOut))
	if diffSummary == "" {
		return undoResultMsg{err: "No unstaged tracked changes found in " + file, clearPending: true}
	}

	if out, err := exec.Command("git", "restore", "--worktree", "--source=HEAD", "--", file).CombinedOutput(); err != nil {
		return undoResultMsg{err: "Revert failed: " + strings.TrimSpace(string(out)), clearPending: true}
	}

	return undoResultMsg{
		output:       "Reverted: " + file + "\n" + diffSummary,
		clearPending: true,
	}
}

func hasStagedChanges(file string) (bool, error) {
	out, err := exec.Command("git", "diff", "--cached", "--name-only", "--", file).Output()
	if err != nil {
		return false, fmt.Errorf("git diff --cached failed: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func splitNonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for line := range strings.SplitSeq(s, "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

type reloadResultMsg struct{ output string }

func runReloadCommand() tea.Cmd {
	return func() tea.Msg {
		var sb strings.Builder

		status, _ := exec.Command("git", "status", "--short").CombinedOutput()
		if s := strings.TrimSpace(string(status)); s != "" {
			sb.WriteString("Modified files:\n")
			sb.WriteString(s)
			sb.WriteString("\n")
		}

		stat, _ := exec.Command("git", "diff", "--stat").CombinedOutput()
		if s := strings.TrimSpace(string(stat)); s != "" {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(s)
		}

		if sb.Len() == 0 {
			return reloadResultMsg{output: "No external changes detected."}
		}
		return reloadResultMsg{output: sb.String()}
	}
}

type diffResultMsg struct{ output string }

func runDiffCommand(arg string) tea.Cmd {
	return func() tea.Msg {
		args := []string{"diff"}
		if arg != "" {
			args = append(args, strings.Fields(arg)...)
		} else {
			base, err := exec.Command("git", "merge-base", "HEAD", "main").Output()
			if err != nil {
				base, err = exec.Command("git", "merge-base", "HEAD", "master").Output()
			}
			if err == nil {
				args = append(args, strings.TrimSpace(string(base))+"...HEAD")
			}
		}
		args = append(args, "--stat")
		out, err := exec.Command("git", args...).CombinedOutput()
		if err != nil && len(out) == 0 {
			return diffResultMsg{output: "No changes found."}
		}
		return diffResultMsg{output: strings.TrimSpace(string(out))}
	}
}

var unsafeFileChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

func sanitizeFilename(s string) string {
	s = strings.ReplaceAll(s, " ", "_")
	s = unsafeFileChars.ReplaceAllString(s, "_")
	s = strings.TrimSpace(s)
	if s == "" {
		return "session-export"
	}
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

func exportSession(ctx context.Context, api Backend, sessionID, title, outPath string) exportResultMsg {
	if outPath == "" {
		name := sanitizeFilename(title)
		outPath = name + ".md"
	}
	if !strings.HasSuffix(outPath, ".md") {
		outPath += ".md"
	}

	msgs, err := api.ListMessages(ctx, sessionID)
	if err != nil {
		return exportResultMsg{err: fmt.Errorf("fetch messages: %w", err)}
	}

	var sb strings.Builder
	sb.WriteString("# ")
	if title != "" {
		sb.WriteString(title)
	} else {
		sb.WriteString("Session Export")
	}
	sb.WriteString("\n\n")
	sb.WriteString("*Exported: " + time.Now().Format("2006-01-02 15:04") + "*\n\n---\n\n")

	for _, m := range msgs {
		switch m.Role {
		case "user":
			sb.WriteString("## User\n\n")
		case "assistant":
			sb.WriteString("## Assistant\n\n")
		default:
			role := m.Role
			if len(role) > 0 {
				role = strings.ToUpper(role[:1]) + role[1:]
			}
			sb.WriteString("## " + role + "\n\n")
		}
		sb.WriteString(m.Content)
		sb.WriteString("\n\n---\n\n")
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil && !os.IsExist(err) {
		return exportResultMsg{err: fmt.Errorf("create directory: %w", err)}
	}
	if err := os.WriteFile(outPath, []byte(sb.String()), 0o644); err != nil {
		return exportResultMsg{err: fmt.Errorf("write file: %w", err)}
	}
	return exportResultMsg{path: outPath}
}

type heapResultMsg struct {
	output string
	err    string
}

func runHeapProfile() heapResultMsg {
	runtime.GC()

	profPath := filepath.Join(os.TempDir(), "kodacode-heap.prof")
	f, err := os.Create(profPath)
	if err != nil {
		return heapResultMsg{err: "create profile: " + err.Error()}
	}

	if err := runtimepprof.WriteHeapProfile(f); err != nil {
		_ = f.Close()
		return heapResultMsg{err: "write profile: " + err.Error()}
	}
	_ = f.Close()

	out, err := exec.Command("go", "tool", "pprof", "-top", "-inuse_space", profPath).CombinedOutput()
	if err != nil {
		return heapResultMsg{output: "Profile saved: " + profPath + "\nRun: go tool pprof -top " + profPath}
	}
	return heapResultMsg{output: "**" + profPath + "**\n```\n" + strings.TrimSpace(string(out)) + "\n```"}
}
