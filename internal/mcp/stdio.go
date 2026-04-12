package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type stdioTransport struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	seq   atomic.Int64

	// writeMu serializes writes to stdin.
	writeMu sync.Mutex

	// pending tracks in-flight requests. The reader goroutine dispatches
	// responses to the appropriate channel by ID.
	pendingMu sync.Mutex
	pending   map[int64]chan rpcResult
	readErr   error // set once when the reader goroutine exits
}

type rpcResult struct {
	data json.RawMessage
	err  error
}

// safeEnvKeys are the only parent environment variables inherited by MCP
// subprocesses. Everything else (API keys, credentials, tokens) is excluded
// to prevent accidental secret leakage to third-party servers.
var safeEnvKeys = map[string]bool{
	"PATH":    true,
	"HOME":    true,
	"LANG":    true,
	"LC_ALL":  true,
	"LC_CTYPE": true,
	"TERM":    true,
	"USER":    true,
	"SHELL":   true,
	"TMPDIR":  true,
}

// NewStdioTransport starts command with args as an MCP subprocess.
// Only safe environment variables are inherited from the parent process;
// additional variables can be passed via the env parameter.
func NewStdioTransport(command string, args []string, env map[string]string) (*stdioTransport, error) {
	cmd := exec.Command(command, args...)
	cmd.Env = filteredEnv(env)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdio mcp stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdio mcp stdout pipe: %w", err)
	}
	setProcGroup(cmd)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("stdio mcp start: %w", err)
	}
	t := &stdioTransport{
		cmd:     cmd,
		stdin:   stdin,
		pending: make(map[int64]chan rpcResult),
	}
	go t.readLoop(bufio.NewScanner(stdoutPipe))
	return t, nil
}

// readLoop is the single goroutine that reads from the subprocess stdout.
// It dispatches JSON-RPC responses to waiting callers and cleans up on exit.
func (t *stdioTransport) readLoop(scanner *bufio.Scanner) {
	for scanner.Scan() {
		var resp jsonRPCResponse
		if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
			continue
		}
		if resp.ID == 0 {
			// Notification from server — no caller waiting.
			continue
		}

		var res rpcResult
		if resp.Error != nil {
			res.err = fmt.Errorf("mcp rpc error: %s", resp.Error.Message)
		} else {
			res.data = resp.Result
		}

		t.pendingMu.Lock()
		ch, ok := t.pending[resp.ID]
		if ok {
			delete(t.pending, resp.ID)
		}
		t.pendingMu.Unlock()

		if ok {
			ch <- res
		}
	}

	// Scanner done — notify all waiting callers.
	readErr := scanner.Err()
	if readErr == nil {
		readErr = fmt.Errorf("stdio mcp: subprocess stdout closed")
	}

	t.pendingMu.Lock()
	t.readErr = readErr
	for id, ch := range t.pending {
		ch <- rpcResult{err: fmt.Errorf("stdio mcp: stream closed before response for id=%d: %w", id, readErr)}
		delete(t.pending, id)
	}
	t.pendingMu.Unlock()
}

func (t *stdioTransport) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	// Apply a default timeout if the context has no deadline.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	id := t.seq.Add(1)
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	ch := make(chan rpcResult, 1)
	t.pendingMu.Lock()
	if t.readErr != nil {
		t.pendingMu.Unlock()
		return nil, fmt.Errorf("mcp call %s: transport closed: %w", method, t.readErr)
	}
	t.pending[id] = ch
	t.pendingMu.Unlock()

	t.writeMu.Lock()
	b, err := json.Marshal(req)
	if err != nil {
		t.writeMu.Unlock()
		t.removePending(id)
		return nil, fmt.Errorf("marshal rpc request: %w", err)
	}
	b = append(b, '\n')
	_, err = t.stdin.Write(b)
	t.writeMu.Unlock()
	if err != nil {
		t.removePending(id)
		return nil, fmt.Errorf("write rpc request: %w", err)
	}

	// Wait for response or context cancellation.
	select {
	case res := <-ch:
		return res.data, res.err
	case <-ctx.Done():
		t.removePending(id)
		return nil, fmt.Errorf("mcp call %s timed out: %w", method, ctx.Err())
	}
}

func (t *stdioTransport) removePending(id int64) {
	t.pendingMu.Lock()
	delete(t.pending, id)
	t.pendingMu.Unlock()
}

// Notify sends a JSON-RPC notification (no ID, no response expected).
func (t *stdioTransport) Notify(ctx context.Context, method string, params any) error {
	notif := jsonRPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	b, err := json.Marshal(notif)
	if err != nil {
		return fmt.Errorf("marshal rpc notification: %w", err)
	}
	b = append(b, '\n')
	if _, err := t.stdin.Write(b); err != nil {
		return fmt.Errorf("write rpc notification: %w", err)
	}
	return nil
}

func (t *stdioTransport) Close() error {
	_ = t.stdin.Close()
	done := make(chan error, 1)
	go func() { done <- t.cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(500 * time.Millisecond):
		killProcessGroup(t.cmd)
		return nil
	}
}

// filteredEnv builds a subprocess environment from the safe allowlist plus
// any user-configured overrides. Parent env vars not in safeEnvKeys are
// excluded to prevent leaking secrets like API keys to MCP servers.
func filteredEnv(extra map[string]string) []string {
	result := make([]string, 0, len(safeEnvKeys)+len(extra))
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if ok && safeEnvKeys[key] {
			result = append(result, entry)
		}
	}
	for k, v := range extra {
		result = append(result, k+"="+v)
	}
	return result
}
