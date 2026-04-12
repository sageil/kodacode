package tool

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

var (
	gitPath     string
	gitPathErr  error
	gitPathOnce sync.Once
)

// lookupGit resolves the git binary path once and caches the result.
func lookupGit() (string, error) {
	gitPathOnce.Do(func() {
		gitPath, gitPathErr = exec.LookPath("git")
	})
	return gitPath, gitPathErr
}

var gitParams = []byte(`{
	"type": "object",
	"properties": {
		"action": {"type": "string", "enum": ["status", "diff", "log", "branch", "add", "commit", "push", "reset", "checkout", "clean", "stash", "show", "blame", "changed_files", "config"], "description": "Git operation to perform"},
		"args": {"type": "string", "description": "Additional arguments (e.g. file path for diff/blame, message for commit, branch name)"},
		"limit": {"type": "number", "description": "Max entries for log (default: 20)"}
	},
	"required": ["action"]
}`)

// NewGitTool returns a Tool that executes structured git operations.
func NewGitTool() *Tool {
	return &Tool{
		Name:        "git",
		ReadOnly:    false,
		Description: prompt("git"),
		Parameters:  gitParams,
		Execute:     executeGit,
	}
}

func executeGit(ctx context.Context, ectx ExecutionContext, args []byte) (*Result, error) {
	var params struct {
		Action string  `json:"action"`
		Args   string  `json:"args"`
		Limit  float64 `json:"limit"`
	}
	if err := flexUnmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.Action == "" {
		return nil, fmt.Errorf("action is required")
	}

	if _, err := lookupGit(); err != nil {
		return nil, fmt.Errorf("git is not installed or not in PATH")
	}

	dir := ectx.WorkDir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("getwd: %w", err)
		}
	}

	gitArgs, err := buildGitArgs(params.Action, params.Args, params.Limit)
	if err != nil {
		return nil, err
	}

	if gitArgs == nil && params.Action == "config" {
		return gitConfigSummary(ctx, dir)
	}

	cmd := exec.CommandContext(ctx, "git", gitArgs...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	cmdErr := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" && !strings.HasSuffix(output, "\n") {
			output += "\n"
		}
		output += stderr.String()
	}

	tr := TruncateWithBudget(output, "tail", ectx.ContextUsage)

	title := "git " + strings.Join(gitArgs, " ")

	meta := map[string]any{
		"exit_code": exitCode(cmdErr),
	}

	return &Result{
		Title:    title,
		Output:   tr.Content,
		Metadata: meta,
	}, nil
}

func gitConfigSummary(ctx context.Context, dir string) (*Result, error) {
	keys := []string{"user.name", "user.email", "core.editor", "init.defaultBranch"}
	var sb strings.Builder
	for _, key := range keys {
		cmd := exec.CommandContext(ctx, "git", "config", "--get", key)
		cmd.Dir = dir
		out, err := cmd.Output()
		if err != nil {
			continue
		}
		fmt.Fprintf(&sb, "%s = %s\n", key, strings.TrimSpace(string(out)))
	}
	output := strings.TrimSpace(sb.String())
	if output == "" {
		output = "No git config values found."
	}
	return &Result{
		Title:  "git config",
		Output: output,
		Metadata: map[string]any{
			"action": "config",
		},
	}, nil
}

func buildGitArgs(action, args string, limit float64) ([]string, error) {
	switch action {
	case "status":
		return appendArgs([]string{"status", "--short"}, args), nil
	case "diff":
		return appendArgs([]string{"diff"}, args), nil
	case "log":
		n := 20
		if limit > 0 {
			n = int(limit)
		}
		base := []string{"log", "--oneline", "--no-decorate", "-n", strconv.Itoa(n)}
		return appendArgs(base, args), nil
	case "branch":
		return appendArgs([]string{"branch", "-a"}, args), nil
	case "add":
		if args == "" {
			return nil, fmt.Errorf("add requires file paths in args")
		}
		return appendArgs([]string{"add"}, args), nil
	case "commit":
		if args == "" {
			return nil, fmt.Errorf("commit requires a message in args")
		}
		return []string{"commit", "-m", args}, nil
	case "push":
		return appendArgs([]string{"push"}, args), nil
	case "reset":
		return appendArgs([]string{"reset"}, args), nil
	case "checkout":
		return appendArgs([]string{"checkout"}, args), nil
	case "clean":
		return appendArgs([]string{"clean"}, args), nil
	case "stash":
		return appendArgs([]string{"stash"}, args), nil
	case "show":
		return appendArgs([]string{"show"}, args), nil
	case "blame":
		if args == "" {
			return nil, fmt.Errorf("blame requires a file path in args")
		}
		return appendArgs([]string{"blame"}, args), nil
	case "changed_files":
		if args == "" {
			return []string{"diff", "--name-status", "--stat"}, nil
		}
		return []string{"diff", "--name-status", "--stat", args + "...HEAD"}, nil
	case "config":
		if args != "" {
			return []string{"config", "--get", args}, nil
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported git action: %s", action)
	}
}

// GitActionMutates reports whether the given git action and args represent a
// state-changing operation.
func GitActionMutates(action, args string) bool {
	if action == "add" || action == "commit" || action == "push" ||
		action == "reset" || action == "checkout" || action == "clean" {
		return true
	}
	if action == "stash" {
		a := strings.TrimSpace(args)
		if a == "" || a == "push" || strings.HasPrefix(a, "push ") {
			return true
		}
		if a == "drop" || strings.HasPrefix(a, "drop ") {
			return true
		}
		if a == "pop" || strings.HasPrefix(a, "pop ") {
			return true
		}
		if a == "apply" || strings.HasPrefix(a, "apply ") {
			return true
		}
	}
	if action == "branch" {
		for _, tok := range splitQuoted(strings.TrimSpace(args)) {
			if tok == "-d" || tok == "-D" || tok == "--delete" {
				return true
			}
		}
	}
	return false
}

// IsDestructiveGitCommand returns true if the command string contains
// destructive git operations that could cause unrecoverable data loss:
// force push, hard reset, clean with force, checkout discarding changes.
func IsDestructiveGitCommand(cmd string) bool {
	for _, pattern := range destructiveGitPatterns {
		if pattern.MatchString(cmd) {
			return true
		}
	}
	return false
}

var destructiveGitPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bgit\s+push\b.*--force\b`),
	regexp.MustCompile(`\bgit\s+push\b.*-f\b`),
	regexp.MustCompile(`\bgit\s+reset\b.*--hard\b`),
	regexp.MustCompile(`\bgit\s+clean\b.*-[a-zA-Z]*f`),
	regexp.MustCompile(`\bgit\s+checkout\b.*\s--\s`),
	regexp.MustCompile(`\bgit\s+checkout\b\s+\.`),         // git checkout . (discard all unstaged changes)
	regexp.MustCompile(`\bgit\s+restore\b\s+(\.|-- \.)`),   // git restore . or git restore -- . (not --staged)
	regexp.MustCompile(`\bgit\s+stash\s+drop\b`),          // git stash drop
	regexp.MustCompile(`\bgit\s+branch\b.*\s-D\b`),        // git branch -D (force delete unmerged)
}

func appendArgs(base []string, extra string) []string {
	extra = strings.TrimSpace(extra)
	if extra == "" {
		return base
	}
	return append(base, splitQuoted(extra)...)
}

// splitQuoted splits s on whitespace but preserves quoted substrings.
// Both single and double quotes are recognised; quotes are stripped from output.
func splitQuoted(s string) []string {
	var args []string
	var cur strings.Builder
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			} else {
				cur.WriteByte(c)
			}
		case c == '"' || c == '\'':
			quote = c
		case c == ' ' || c == '\t':
			if cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args
}
