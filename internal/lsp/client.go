package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Client is a JSON-RPC 2.0 transport that communicates with an LSP server
// over stdin/stdout using Content-Length framed messages.
//
// A background reader goroutine routes responses to callers by matching
// request IDs, and dispatches server-initiated notifications to handlers.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	writeMu sync.Mutex   // guards writes to stdin
	seq     atomic.Int64 // request ID counter

	pending sync.Map // map[int64]chan *jsonRPCMessage

	// Notification handlers (optional).
	onDiagnostics func(PublishDiagnosticsParams)

	done    chan struct{} // closed when reader goroutine exits
	readErr error        // set by reader goroutine on fatal error
}

// NewClient starts an LSP server process and begins reading responses
// from its stdout in a background goroutine.
func NewClient(command string, args []string, env map[string]string, workDir string) (*Client, error) {
	cmd := exec.Command(command, args...)
	baseEnv := os.Environ()
	cmd.Env = make([]string, len(baseEnv), len(baseEnv)+len(env)+1)
	copy(cmd.Env, baseEnv)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	// Prepend common LSP server locations to PATH so servers installed
	// via Neovim Mason or Homebrew are found without manual config.
	cmd.Env = prependPath(cmd.Env)
	if workDir != "" {
		cmd.Dir = workDir
	}
	setProcGroup(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp stdout pipe: %w", err)
	}
	// Discard stderr to prevent buffer deadlocks. Errors are returned via
	// JSON-RPC error responses, not stderr.
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("lsp start %s: %w", command, err)
	}

	c := &Client{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		done:   make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

// Alive reports whether the background reader is still running.
func (c *Client) Alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

// Call sends a JSON-RPC request and blocks until the matching response
// arrives or ctx is cancelled.
func (c *Client) Call(ctx context.Context, method string, params any, result any) error {
	id := c.seq.Add(1)

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	ch := make(chan *jsonRPCMessage, 1)
	c.pending.Store(id, ch)
	defer c.pending.Delete(id)

	if err := c.writeMessage(req); err != nil {
		return fmt.Errorf("lsp call %s: %w", method, err)
	}

	select {
	case msg := <-ch:
		if msg.Error != nil {
			return fmt.Errorf("lsp %s error: %s (code %d)", method, msg.Error.Message, msg.Error.Code)
		}
		if result != nil && len(msg.Result) > 0 {
			if err := json.Unmarshal(msg.Result, result); err != nil {
				return fmt.Errorf("lsp %s unmarshal result: %w", method, err)
			}
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		if c.readErr != nil {
			return fmt.Errorf("lsp connection lost: %w", c.readErr)
		}
		return fmt.Errorf("lsp connection lost")
	}
}

// Notify sends a JSON-RPC notification (no ID, no response expected).
func (c *Client) Notify(method string, params any) error {
	notif := jsonRPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return c.writeMessage(notif)
}

// SetDiagnosticsHandler registers a callback for publishDiagnostics notifications.
func (c *Client) SetDiagnosticsHandler(fn func(PublishDiagnosticsParams)) {
	c.onDiagnostics = fn
}

// Close closes stdin to signal the server to exit, waits briefly for a clean
// exit, then forcibly kills the process to prevent orphans.
func (c *Client) Close() error {
	_ = c.stdin.Close()

	select {
	case <-c.done:
	case <-time.After(500 * time.Millisecond):
	}

	waitDone := make(chan error, 1)
	go func() { waitDone <- c.cmd.Wait() }()
	select {
	case err := <-waitDone:
		return err
	case <-time.After(500 * time.Millisecond):
		killProcessGroup(c.cmd)
		<-waitDone
		return nil
	}
}

// lspBinDirs returns additional directories where LSP servers may be installed.
// Covers: Neovim Mason, npm global, pnpm global, Homebrew, and ~/.local/bin.
func lspBinDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	dirs := []string{
		filepath.Join(home, ".local", "share", "nvim", "mason", "bin"),
		filepath.Join(home, ".local", "bin"),
	}
	dirs = append(dirs,
		"/usr/local/bin",
		filepath.Join(home, ".npm-global", "bin"),
	)
	// pnpm global bin.
	if pnpmHome := os.Getenv("PNPM_HOME"); pnpmHome != "" {
		dirs = append(dirs, pnpmHome)
	} else {
		dirs = append(dirs, filepath.Join(home, ".local", "share", "pnpm"))
	}
	return dirs
}

// prependPath adds LSP server directories to the PATH environment variable.
func prependPath(environ []string) []string {
	extra := lspBinDirs()
	if len(extra) == 0 {
		return environ
	}
	prefix := strings.Join(extra, string(os.PathListSeparator))
	for i, e := range environ {
		if strings.HasPrefix(e, "PATH=") {
			environ[i] = "PATH=" + prefix + string(os.PathListSeparator) + e[5:]
			return environ
		}
	}
	return append(environ, "PATH="+prefix)
}

// writeMessage serializes msg as Content-Length framed JSON and writes it to stdin.
func (c *Client) writeMessage(msg any) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := io.WriteString(c.stdin, header); err != nil {
		return err
	}
	_, err = c.stdin.Write(body)
	return err
}

// readLoop reads Content-Length framed messages from stdout and routes them.
// It runs in a dedicated goroutine and closes c.done on exit.
func (c *Client) readLoop() {
	defer close(c.done)
	reader := bufio.NewReader(c.stdout)

	for {
		body, err := readContentLengthMessage(reader)
		if err != nil {
			c.readErr = err
			// Wake all pending callers.
			c.pending.Range(func(key, value any) bool {
				close(value.(chan *jsonRPCMessage))
				c.pending.Delete(key)
				return true
			})
			return
		}

		var msg jsonRPCMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			log.Printf("lsp: failed to parse message: %v", err)
			continue
		}

		// Response to a request we sent (has ID, no method).
		if msg.ID != nil && msg.Method == "" {
			if ch, ok := c.pending.Load(*msg.ID); ok {
				ch.(chan *jsonRPCMessage) <- &msg
			}
			continue
		}

		// Server-initiated notification (has method, no ID).
		if msg.Method != "" && msg.ID == nil {
			c.handleNotification(msg.Method, msg.Params)
			continue
		}

		// Server-initiated request (has both ID and method). We don't handle these
		// but some servers send window/logMessage etc. Ignore gracefully.
	}
}

// handleNotification dispatches a server notification to the appropriate handler.
func (c *Client) handleNotification(method string, params json.RawMessage) {
	switch method {
	case "textDocument/publishDiagnostics":
		if c.onDiagnostics != nil {
			var p PublishDiagnosticsParams
			if err := json.Unmarshal(params, &p); err == nil {
				c.onDiagnostics(p)
			}
		}
	// window/logMessage, window/showMessage, etc. — ignored.
	}
}

// readContentLengthMessage reads one Content-Length framed message from r.
// Format: "Content-Length: N\r\n\r\n<N bytes of JSON>"
func readContentLengthMessage(r *bufio.Reader) ([]byte, error) {
	var contentLength int
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read header: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			// Empty line = end of headers.
			break
		}
		if val, ok := strings.CutPrefix(line, "Content-Length:"); ok {
			val = strings.TrimSpace(val)
			n, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("parse Content-Length %q: %w", val, err)
			}
			contentLength = n
		}
		// Content-Type and other headers are ignored.
	}
	if contentLength <= 0 {
		return nil, fmt.Errorf("missing or zero Content-Length")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("read body (%d bytes): %w", contentLength, err)
	}
	return body, nil
}
