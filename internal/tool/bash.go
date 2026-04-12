package tool

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// MaxMetadataLength is the max bytes for the metadata output field.
const MaxMetadataLength = 30000

const (
	DefaultBashTimeout     = 120 * time.Second
	DefaultTestTimeout     = 120 * time.Second
	DefaultWebFetchTimeout = 30 * time.Second
)

var bashParams = []byte(`{
	"type": "object",
	"properties": {
		"command": {"type": "string", "description": "The command to execute"},
		"timeout": {"type": "number", "description": "Optional timeout in milliseconds"},
		"workdir": {"type": "string", "description": "The working directory to run the command in"},
		"description": {"type": "string", "description": "Clear, concise description of what this command does in 5-10 words"},
		"purpose": {"type": "string", "enum": ["verification", "build", "diagnostic", "other"], "description": "Why this command is being run. Use verification for tests, typechecks, linters, and validation before planning; build for compilation or packaging; diagnostic for inspection and ad hoc checks."},
		"run_in_background": {"type": "boolean", "description": "Set to true to run in the background. Returns a task ID immediately. Use task_output to read results later."}
	},
	"required": ["command", "description", "purpose"]
}`)

const (
	BashPurposeVerification = "verification"
	BashPurposeBuild        = "build"
	BashPurposeDiagnostic   = "diagnostic"
	BashPurposeOther        = "other"
)

// NewBashTool returns a Tool that executes shell commands.
func NewBashTool() *Tool {
	return &Tool{
		Name:        "bash",
		Description: prompt("bash"),
		Parameters:  bashParams,
		Execute:     executeBash,
	}
}

func executeBash(ctx context.Context, ectx ExecutionContext, args []byte) (*Result, error) {
	var params struct {
		Command         string  `json:"command"`
		Timeout         float64 `json:"timeout"`
		Workdir         string  `json:"workdir"`
		Description     string  `json:"description"`
		Purpose         string  `json:"purpose"`
		RunInBackground bool    `json:"run_in_background"`
	}
	if err := flexUnmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.Command == "" {
		return nil, fmt.Errorf("command is required")
	}
	if !validBashPurpose(params.Purpose) {
		return nil, fmt.Errorf("purpose must be one of: verification, build, diagnostic, other")
	}

	if msg := rejectBashFileOp(params.Command); msg != "" {
		return &Result{Output: msg}, nil
	}

	timeout := DefaultBashTimeout
	if params.Timeout > 0 {
		timeout = time.Duration(params.Timeout) * time.Millisecond
	}

	dir := params.Workdir
	if dir == "" {
		if ectx.WorkDir != "" {
			dir = ectx.WorkDir
		} else {
			var err error
			dir, err = os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("getwd: %w", err)
			}
		}
	} else if !filepath.IsAbs(dir) && ectx.WorkDir != "" {
		dir = filepath.Join(ectx.WorkDir, dir)
	}

	// Background mode: spawn process and return immediately.
	if params.RunInBackground {
		task := StartBackground(ectx.SessionID, params.Command, params.Description, dir, ectx.WriteOutput, ectx.OnBackgroundDone)
		// Wait briefly so the TUI shows initial output (e.g. "Server listening on...").
		select {
		case <-task.WaitDone():
		case <-time.After(500 * time.Millisecond):
		}
		return &Result{
			Title:  params.Description,
			Output: fmt.Sprintf("Background task started with ID: %s. Use the task_output tool to check on it.", task.ID),
			Metadata: map[string]any{
				"task_id":     task.ID,
				"description": params.Description,
				"purpose":     params.Purpose,
			},
		}, nil
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	sh, shArgs := shellExecArgs(params.Command)
	log.Printf("bash: [%s] shell=%s dir=%s cmd=%q", ectx.SessionID, sh, dir, params.Command)

	cmd := exec.CommandContext(ctx, sh, shArgs...)
	cmd.Dir = dir
	setProcAttr(cmd)

	var buf bytes.Buffer
	var mu sync.Mutex

	pr, pw := io.Pipe()

	cmd.Stdout = pw
	cmd.Stderr = pw

	done := make(chan struct{})
	go func() {
		defer close(done)
		tmp := make([]byte, 4096)
		for {
			n, err := pr.Read(tmp)
			if n > 0 {
				chunk := string(tmp[:n])
				mu.Lock()
				buf.Write(tmp[:n])
				if ectx.WriteOutput != nil {
					ectx.WriteOutput(chunk)
				}
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	log.Printf("bash: [%s] starting process (elapsed=%s)", ectx.SessionID, time.Since(start))
	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		<-done
		return nil, fmt.Errorf("start: %w", err)
	}
	log.Printf("bash: [%s] process started pid=%d (elapsed=%s)", ectx.SessionID, cmd.Process.Pid, time.Since(start))

	cmdDone := make(chan error, 1)
	go func() {
		waitErr := cmd.Wait()
		log.Printf("bash: [%s] cmd.Wait returned err=%v (elapsed=%s)", ectx.SessionID, waitErr, time.Since(start))
		cmdDone <- waitErr
		_ = pw.Close()
	}()

	var timedOut bool
	var code int

	abort := ectx.Abort
	if abort == nil {
		abort = make(chan struct{})
	}

	select {
	case err := <-cmdDone:
		log.Printf("bash: [%s] completed normally exit=%d (elapsed=%s)", ectx.SessionID, exitCode(err), time.Since(start))
		code = exitCode(err)
	case <-ctx.Done():
		timedOut = true
		log.Printf("bash: [%s] TIMEOUT after %s, killing pid=%d", ectx.SessionID, time.Since(start), cmd.Process.Pid)
		_ = killTree(cmd.Process.Pid)
		_ = pw.Close()
		log.Printf("bash: [%s] waiting for cmd.Wait after kill...", ectx.SessionID)
		<-cmdDone
		log.Printf("bash: [%s] cmd.Wait returned after kill (elapsed=%s)", ectx.SessionID, time.Since(start))
		code = -1
	case <-abort:
		log.Printf("bash: [%s] ABORTED after %s, killing pid=%d", ectx.SessionID, time.Since(start), cmd.Process.Pid)
		_ = killTree(cmd.Process.Pid)
		_ = pw.Close()
		<-cmdDone
		code = -1
	}

	log.Printf("bash: [%s] waiting for pipe drain...", ectx.SessionID)
	<-done
	log.Printf("bash: [%s] pipe drained, total elapsed=%s", ectx.SessionID, time.Since(start))

	mu.Lock()
	output := buf.String()
	mu.Unlock()

	if timedOut {
		output += fmt.Sprintf("\n<bash_metadata>\nbash tool terminated command after exceeding timeout %s\n</bash_metadata>", timeout)
	}

	meta := map[string]any{
		"exit_code":   code,
		"description": params.Description,
		"purpose":     params.Purpose,
	}

	metaOutput := output
	if len(metaOutput) > MaxMetadataLength {
		metaOutput = metaOutput[len(metaOutput)-MaxMetadataLength:]
	}
	meta["output"] = metaOutput

	tr := TruncateWithBudget(output, "tail", ectx.ContextUsage)
	return &Result{
		Title:    params.Description,
		Output:   tr.Content,
		Metadata: meta,
	}, nil
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return exit.ExitCode()
	}
	return -1
}

func validBashPurpose(purpose string) bool {
	switch purpose {
	case BashPurposeVerification, BashPurposeBuild, BashPurposeDiagnostic, BashPurposeOther:
		return true
	default:
		return false
	}
}

var readOnlyBashPrefixes = []string{
	"ls ", "ls\t",
	"cat ", "head ", "tail ", "wc ",
	"echo ", "printf ",
	"git log", "git status", "git diff", "git show", "git branch", "git blame",
	"git rev-parse", "git merge-base", "git remote",
	"go list", "go version", "go env",
	"node -v", "node --version",
	"npm list", "npm ls", "npm --version",
	"python --version", "python3 --version",
	"which ", "type ", "command -v",
	"pwd", "date", "whoami", "uname",
	"tree ", "find ", "du ", "df ",
}

// IsReadOnlyBashCommand returns true if the command only reads state.
// Handles chained commands (&&, ||, ;) by checking each sub-command.
func IsReadOnlyBashCommand(cmd string) bool {
	for _, sub := range splitBashCommands(cmd) {
		sub = strings.TrimSpace(sub)
		if sub == "" {
			continue
		}
		if !matchesReadOnlyPrefix(sub) {
			return false
		}
	}
	return true
}

// BashHasMutatingGitCommand returns true when a bash command contains a direct
// git invocation that is not on the read-only allowlist. This is used to force
// mutating git work through the structured git tool instead of shell strings.
func BashHasMutatingGitCommand(cmd string) bool {
	for _, sub := range splitBashCommands(cmd) {
		gitCmd, ok := bashGitInvocation(sub)
		if !ok {
			continue
		}
		if !matchesReadOnlyPrefix(gitCmd) {
			return true
		}
	}
	return false
}

func splitBashCommands(cmd string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(cmd); i++ {
		if cmd[i] == '&' && i+1 < len(cmd) && cmd[i+1] == '&' {
			parts = append(parts, cmd[start:i])
			i++
			start = i + 1
		} else if cmd[i] == '|' && i+1 < len(cmd) && cmd[i+1] == '|' {
			parts = append(parts, cmd[start:i])
			i++
			start = i + 1
		} else if cmd[i] == ';' {
			parts = append(parts, cmd[start:i])
			start = i + 1
		}
	}
	parts = append(parts, cmd[start:])
	return parts
}

func matchesReadOnlyPrefix(cmd string) bool {
	for _, prefix := range readOnlyBashPrefixes {
		if strings.HasPrefix(cmd, prefix) || cmd == strings.TrimSpace(prefix) {
			return true
		}
	}
	return false
}

func bashGitInvocation(cmd string) (string, bool) {
	tokens := splitQuoted(strings.TrimSpace(cmd))
	if len(tokens) == 0 {
		return "", false
	}

	i := 0
	if tokens[i] == "env" {
		i++
	}
	for i < len(tokens) && isShellEnvAssignment(tokens[i]) {
		i++
	}
	if i < len(tokens) && (tokens[i] == "command" || tokens[i] == "builtin") {
		i++
	}
	if i >= len(tokens) {
		return "", false
	}
	if filepath.Base(tokens[i]) != "git" {
		return "", false
	}
	return strings.Join(tokens[i:], " "), true
}

func isShellEnvAssignment(tok string) bool {
	if tok == "" {
		return false
	}
	idx := strings.IndexByte(tok, '=')
	if idx <= 0 {
		return false
	}
	for i := 0; i < idx; i++ {
		c := tok[i]
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (c < '0' || c > '9' || i == 0) && c != '_' {
			return false
		}
	}
	return true
}
