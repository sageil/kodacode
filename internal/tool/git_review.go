package tool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const gitReviewOutputLimit = 0
const gitDiffRenderedOutputLimit = 12 * 1024
const gitReviewTimeout = 30 * time.Second

var ErrGitRepositoryRequired = errors.New("workspace directory is not inside a git repository")

type gitReviewRunResult struct {
	Output     []byte
	Truncated  bool
	ExitCode   *int
	DurationMS int64
	Backend    string
}

func runGitReviewCommand(ctx context.Context, root string, args []string) (gitReviewRunResult, string, error) {
	runCtx, cancel := context.WithTimeout(ctx, gitReviewTimeout)
	defer cancel()
	startedAt := time.Now()

	cmd := exec.CommandContext(runCtx, "git", gitCommandArgs(args)...)
	cmd.Dir = root

	collector := newGitReviewOutputCollector(gitReviewOutputLimit)
	cmd.Stdout = collector
	cmd.Stderr = collector

	err := cmd.Run()
	result := gitReviewRunResult{
		Output:     collector.Bytes(),
		Truncated:  collector.Truncated(),
		ExitCode:   gitReviewExitCodePointer(err),
		DurationMS: time.Since(startedAt).Milliseconds(),
		Backend:    "process",
	}
	text := normalizeGitReviewOutput(string(result.Output))
	if err != nil {
		return result, "", classifyGitReviewError(args, text, err)
	}
	return result, text, nil
}

func gitCommandArgs(args []string) []string {
	return append([]string{"-c", "color.ui=false", "-c", "core.pager=cat"}, args...)
}

func classifyGitReviewError(args []string, output string, runErr error) error {
	if strings.Contains(output, "not a git repository") {
		return ErrGitRepositoryRequired
	}
	if output == "" {
		return fmt.Errorf("git %s failed", strings.Join(args, " "))
	}
	return errors.New(output)
}

func formatGitStatusResult(command []string, raw string, truncated bool) string {
	branch, entries := parseGitStatusOutput(raw)
	if len(entries) == 0 {
		if truncated {
			return fmt.Sprintf("command: %s\nbranch: %s\nstatus: clean\nnote: output truncated", strings.Join(command, " "), branch)
		}
		return fmt.Sprintf("command: %s\nbranch: %s\nstatus: clean", strings.Join(command, " "), branch)
	}
	body := strings.Join(entries, "\n")
	if truncated {
		body += "\n\n[output truncated]"
	}
	return fmt.Sprintf("command: %s\nbranch: %s\nstatus:\n%s", strings.Join(command, " "), branch, body)
}

func formatGitDiffResult(command []string, staged bool, raw string, truncated bool) string {
	scope := "working"
	if staged {
		scope = "staged"
	}
	if strings.TrimSpace(raw) == "" {
		return fmt.Sprintf("command: %s\nscope: %s\nstatus: clean", strings.Join(command, " "), scope)
	}
	body := raw
	body, capped := capGitDiffRenderedOutput(body)
	if truncated || capped {
		body += "\n\n[diff output capped; use git_status and targeted file reads for large changes]"
	}
	return fmt.Sprintf("command: %s\nscope: %s\n\n%s", strings.Join(command, " "), scope, body)
}

func capGitDiffRenderedOutput(raw string) (string, bool) {
	if gitDiffRenderedOutputLimit <= 0 || len(raw) <= gitDiffRenderedOutputLimit {
		return raw, false
	}
	capped := raw[:gitDiffRenderedOutputLimit]
	for len(capped) > 0 && !utf8.ValidString(capped) {
		capped = capped[:len(capped)-1]
	}
	return strings.TrimRight(capped, "\n"), true
}

func formatGitShowResult(command []string, rev, raw string, truncated bool) string {
	if strings.TrimSpace(raw) == "" {
		return fmt.Sprintf("command: %s\nrevision: %s\nstatus: no workspace-scoped output", strings.Join(command, " "), rev)
	}
	body := raw
	if truncated {
		body += "\n\n[output truncated]"
	}
	return fmt.Sprintf("command: %s\nrevision: %s\n\n%s", strings.Join(command, " "), rev, body)
}

func formatGitReviewError(command []string, err error) string {
	return fmt.Sprintf("command: %s\nerror: %s", strings.Join(command, " "), err.Error())
}

func parseGitStatusOutput(raw string) (string, []string) {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	branch := "(unknown)"
	if len(lines) > 0 && strings.HasPrefix(lines[0], "## ") {
		branch = strings.TrimSpace(strings.TrimPrefix(lines[0], "## "))
		lines = lines[1:]
	}
	entries := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		entries = append(entries, line)
	}
	return branch, entries
}

func normalizeGitReviewOutput(output string) string {
	return strings.TrimSpace(strings.ReplaceAll(output, "\r\n", "\n"))
}

func gitReviewExecutionRuntime(result gitReviewRunResult) *ExecutionRuntime {
	if result.Backend == "" {
		return nil
	}
	return &ExecutionRuntime{
		Backend:    result.Backend,
		ExitCode:   cloneExecutionExitCode(result.ExitCode),
		DurationMS: result.DurationMS,
	}
}

type gitReviewOutputCollector struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func newGitReviewOutputCollector(limit int) *gitReviewOutputCollector {
	return &gitReviewOutputCollector{limit: limit}
}

func (c *gitReviewOutputCollector) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.limit == 0 {
		_, _ = c.buf.Write(p)
		return len(p), nil
	}
	remaining := c.limit - c.buf.Len()
	if remaining <= 0 {
		c.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = c.buf.Write(p[:remaining])
		c.truncated = true
		return len(p), nil
	}
	_, _ = c.buf.Write(p)
	return len(p), nil
}

func (c *gitReviewOutputCollector) Bytes() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.buf.Bytes()...)
}

func (c *gitReviewOutputCollector) Truncated() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.truncated
}

func gitReviewExitCodePointer(err error) *int {
	if err == nil {
		code := 0
		return &code
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		return &code
	}
	return nil
}

func cloneExecutionExitCode(code *int) *int {
	if code == nil {
		return nil
	}
	copyCode := *code
	return &copyCode
}
