package tool

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"time"
)

// maxOutputBuffer is the maximum bytes kept in a background task's output buffer.
// Once exceeded, the oldest bytes are discarded (ring-buffer style via truncation).
const maxOutputBuffer = 512 * 1024 // 512 KB

// taskTTLAfterDone is how long a completed task stays in the registry before
// being garbage-collected. Gives the LLM time to read the output.
const taskTTLAfterDone = 5 * time.Minute

// BashTask represents a background bash process.
type BashTask struct {
	ID          string
	SessionID   string
	Command     string
	Description string
	StartTime   time.Time

	mu          sync.Mutex
	buf         bytes.Buffer
	done        bool
	completedAt time.Time
	exitCode    int
	doneCh      chan struct{} // closed when process exits
	pid         int           // OS process ID for cleanup
	onDone      func(*BashTask)
}

// Output returns the current buffered output.
func (t *BashTask) Output() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.buf.String()
}

// Done reports whether the process has exited.
func (t *BashTask) Done() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.done
}

// ExitCode returns the process exit code (-1 if still running or killed).
func (t *BashTask) ExitCode() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.exitCode
}

// WaitDone returns a channel that is closed when the process exits.
func (t *BashTask) WaitDone() <-chan struct{} {
	return t.doneCh
}

// kill sends SIGKILL to the process tree. No-op if already exited.
func (t *BashTask) kill() {
	t.mu.Lock()
	pid := t.pid
	done := t.done
	t.mu.Unlock()
	if !done && pid > 0 {
		_ = killTree(pid)
		log.Printf("bash background: killed task %s (pid=%d)", t.ID, pid)
	}
}

var (
	bgMu      sync.Mutex
	bgTasks   = make(map[string]*BashTask)
	bgSeq     = make(map[string]int) // sessionID → sequence
	gcStarted bool
)

// ensureGC starts the background garbage collector once.
func ensureGC() {
	if gcStarted {
		return
	}
	gcStarted = true
	go gcLoop()
}

// gcLoop periodically removes completed tasks older than taskTTLAfterDone.
func gcLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		bgMu.Lock()
		now := time.Now()
		for id, task := range bgTasks {
			if shouldRemoveBackgroundTask(now, task) {
				delete(bgTasks, id)
				log.Printf("bash background: gc removed task %s", id)
			}
		}
		bgMu.Unlock()
	}
}

func shouldRemoveBackgroundTask(now time.Time, task *BashTask) bool {
	task.mu.Lock()
	defer task.mu.Unlock()
	if !task.done {
		return false
	}
	anchor := task.completedAt
	if anchor.IsZero() {
		anchor = task.StartTime
	}
	return now.Sub(anchor) > taskTTLAfterDone
}

// StartBackground spawns a command in the background and returns immediately.
// Output is buffered internally (capped at maxOutputBuffer); use GetBashTask to read it.
// writeOutput is called for each chunk so the TUI can show initial progress.
func StartBackground(sessionID, command, description, workdir string, writeOutput func(string), onDone func(*BashTask)) *BashTask {
	bgMu.Lock()
	ensureGC()
	bgSeq[sessionID]++
	seq := bgSeq[sessionID]
	prefix := sessionID
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	id := fmt.Sprintf("bg-%s-%d", prefix, seq)
	task := &BashTask{
		ID:          id,
		SessionID:   sessionID,
		Command:     command,
		Description: description,
		StartTime:   time.Now(),
		doneCh:      make(chan struct{}),
		onDone:      onDone,
	}
	bgTasks[id] = task
	bgMu.Unlock()
	log.Printf("bash background: started task %s (session=%s, command=%q)", id, sessionID, command)

	sh := acceptableShell()
	cmd := exec.Command(sh, "-c", command)
	cmd.Dir = workdir
	setProcAttr(cmd)

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	// Drain pipe into buffer + writeOutput.
	go func() {
		tmp := make([]byte, 4096)
		for {
			n, err := pr.Read(tmp)
			if n > 0 {
				chunk := string(tmp[:n])
				task.mu.Lock()
				task.buf.Write(tmp[:n])
				// Cap buffer size: keep only the tail when over limit.
				if task.buf.Len() > maxOutputBuffer {
					trimmed := task.buf.Bytes()[task.buf.Len()-maxOutputBuffer:]
					task.buf.Reset()
					task.buf.WriteString("[...output truncated...]\n")
					task.buf.Write(trimmed)
				}
				task.mu.Unlock()
				if writeOutput != nil {
					writeOutput(chunk)
				}
			}
			if err != nil {
				return
			}
		}
	}()

	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		task.mu.Lock()
		task.done = true
		task.completedAt = time.Now()
		task.exitCode = -1
		fmt.Fprintf(&task.buf, "\nFailed to start: %v", err)
		task.mu.Unlock()
		close(task.doneCh)
		log.Printf("bash background: failed to start %q: %v", command, err)
		return task
	}

	task.mu.Lock()
	task.pid = cmd.Process.Pid
	task.mu.Unlock()

	// Wait for process in background goroutine.
	go func() {
		err := cmd.Wait()
		_ = pw.Close()
		task.mu.Lock()
		task.done = true
		task.completedAt = time.Now()
		task.exitCode = exitCode(err)
		task.mu.Unlock()
		close(task.doneCh)
		log.Printf("bash background: task %s exited (code=%d)", id, exitCode(err))
		if task.onDone != nil {
			task.onDone(task)
		}
	}()

	return task
}

// GetBashTask returns the background task with the given ID, or nil.
func GetBashTask(id string) *BashTask {
	bgMu.Lock()
	defer bgMu.Unlock()
	return bgTasks[id]
}

// CleanupSession kills all running background tasks for the given session
// and removes them from the registry. Call when a session ends.
func CleanupSession(sessionID string) {
	bgMu.Lock()
	var toKill []*BashTask
	for id, task := range bgTasks {
		if task.SessionID == sessionID {
			toKill = append(toKill, task)
			delete(bgTasks, id)
		}
	}
	delete(bgSeq, sessionID)
	bgMu.Unlock()

	for _, task := range toKill {
		task.kill()
		log.Printf("bash background: cleaned up task %s for session %s", task.ID, sessionID)
	}
}

// CleanupAll kills all running background tasks. Call on application shutdown.
func CleanupAll() {
	bgMu.Lock()
	var toKill []*BashTask
	for id, task := range bgTasks {
		toKill = append(toKill, task)
		delete(bgTasks, id)
	}
	bgMu.Unlock()

	for _, task := range toKill {
		task.kill()
		log.Printf("bash background: shutdown cleanup task %s", task.ID)
	}
}

var taskOutputParams = []byte(`{
	"type": "object",
	"properties": {
		"task_id": {"type": "string", "description": "The background task ID to check"},
		"block":   {"type": "boolean", "description": "Whether to wait for completion (default true)"},
		"timeout": {"type": "number", "description": "Max wait time in ms (default 30000)"}
	},
	"required": ["task_id"]
}`)

// NewTaskOutputTool returns a tool that reads output from background bash tasks.
func NewTaskOutputTool() *Tool {
	return &Tool{
		Name:        "task_output",
		ReadOnly:    true,
		Description: prompt("task_output"),
		Parameters:  taskOutputParams,
		Execute:     executeTaskOutput,
	}
}

func executeTaskOutput(ctx context.Context, _ ExecutionContext, args []byte) (*Result, error) {
	var params struct {
		TaskID  string  `json:"task_id"`
		Block   *bool   `json:"block"`
		Timeout float64 `json:"timeout"`
	}
	if err := flexUnmarshal(args, &params); err != nil {
		log.Printf("task_output: unmarshal error: %v (args=%q)", err, string(args))
		return nil, err
	}
	if params.TaskID == "" {
		log.Printf("task_output: empty task_id (args=%q)", string(args))
		return ErrorResult(ErrCodeInvalidArgs, "task_id is required", false), nil
	}

	block := true
	if params.Block != nil {
		block = *params.Block
	}
	timeout := 30 * time.Second
	if params.Timeout > 0 {
		timeout = time.Duration(params.Timeout) * time.Millisecond
	}

	task := GetBashTask(params.TaskID)
	if task == nil {
		// List available tasks for debugging.
		bgMu.Lock()
		var ids []string
		for id := range bgTasks {
			ids = append(ids, id)
		}
		bgMu.Unlock()
		log.Printf("task_output: task %q not found, available: %v", params.TaskID, ids)
		return ErrorResult(ErrCodeNotFound, fmt.Sprintf("No background task found with ID %q. It may have been cleaned up after completing. Available tasks: %v", params.TaskID, ids), false), nil
	}

	if block && !task.Done() {
		select {
		case <-task.WaitDone():
		case <-time.After(timeout):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	output := task.Output()
	status := "running"
	if task.Done() {
		status = "completed"
	}

	elapsed := time.Since(task.StartTime)

	var sb bytes.Buffer
	fmt.Fprintf(&sb, "<task_status>\n")
	fmt.Fprintf(&sb, "  task_id: %s\n", task.ID)
	fmt.Fprintf(&sb, "  status: %s\n", status)
	fmt.Fprintf(&sb, "  command: %s\n", task.Command)
	fmt.Fprintf(&sb, "  elapsed: %s\n", elapsed.Round(time.Millisecond))
	if task.Done() {
		fmt.Fprintf(&sb, "  exit_code: %d\n", task.ExitCode())
	}
	fmt.Fprintf(&sb, "</task_status>\n\n")

	if output != "" {
		sb.WriteString(output)
	} else {
		sb.WriteString("(no output yet)")
	}

	return &Result{
		Title:  fmt.Sprintf("task_output %s", params.TaskID),
		Output: sb.String(),
		Metadata: map[string]any{
			"task_id":   task.ID,
			"status":    status,
			"command":   task.Command,
			"elapsed":   elapsed.Milliseconds(),
			"exit_code": task.ExitCode(),
		},
	}, nil
}
