package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	writeMu sync.Mutex
	metaMu  sync.RWMutex
	seq     atomic.Int64
	pending sync.Map

	onDiagnostics    func(PublishDiagnosticsParams)
	workspaceFolders []WorkspaceFolder

	done    chan struct{}
	readErr error
}

type jsonRPCMethodError struct {
	Method  string
	Code    int
	Message string
}

func (e *jsonRPCMethodError) Error() string {
	return fmt.Sprintf("lsp %s error: %s (code %d)", e.Method, e.Message, e.Code)
}

func isJSONRPCMethodNotFound(err error) bool {
	var methodErr *jsonRPCMethodError
	return errors.As(err, &methodErr) && methodErr.Code == -32601
}

func IsJSONRPCMethodNotFound(err error) bool {
	return isJSONRPCMethodNotFound(err)
}

func NewClient(command string, args []string, env map[string]string, workDir string) (*Client, error) {
	cmd := exec.Command(command, args...)
	baseEnv := os.Environ()
	cmd.Env = make([]string, len(baseEnv), len(baseEnv)+len(env)+1)
	copy(cmd.Env, baseEnv)
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
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
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("lsp start %s: %w", command, err)
	}

	client := &Client{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		done:   make(chan struct{}),
	}
	go client.readLoop()
	return client, nil
}

func (c *Client) Alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

func (c *Client) Call(ctx context.Context, method string, params any, result any) error {
	id := c.seq.Add(1)
	request := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	ch := make(chan *jsonRPCMessage, 1)
	c.pending.Store(id, ch)
	defer c.pending.Delete(id)

	if err := c.writeMessage(request); err != nil {
		return fmt.Errorf("lsp call %s: %w", method, err)
	}

	select {
	case message, ok := <-ch:
		if !ok || message == nil {
			return c.connectionLostError()
		}
		if message.Error != nil {
			return &jsonRPCMethodError{
				Method:  method,
				Code:    message.Error.Code,
				Message: message.Error.Message,
			}
		}
		if result != nil && len(message.Result) > 0 {
			if err := json.Unmarshal(message.Result, result); err != nil {
				return fmt.Errorf("lsp %s unmarshal result: %w", method, err)
			}
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return c.connectionLostError()
	}
}

func (c *Client) connectionLostError() error {
	if c.readErr != nil {
		return fmt.Errorf("lsp connection lost: %w", c.readErr)
	}
	return fmt.Errorf("lsp connection lost")
}

func (c *Client) Notify(method string, params any) error {
	return c.writeMessage(jsonRPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
}

func (c *Client) SetDiagnosticsHandler(fn func(PublishDiagnosticsParams)) {
	c.onDiagnostics = fn
}

func (c *Client) SetWorkspaceFolders(folders []WorkspaceFolder) {
	c.metaMu.Lock()
	defer c.metaMu.Unlock()
	c.workspaceFolders = append([]WorkspaceFolder(nil), folders...)
}

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

func prependPath(environ []string) []string {
	extra := lspBinDirs()
	if len(extra) == 0 {
		return environ
	}
	prefix := strings.Join(extra, string(os.PathListSeparator))
	for index, entry := range environ {
		if strings.HasPrefix(entry, "PATH=") {
			environ[index] = "PATH=" + prefix + string(os.PathListSeparator) + entry[5:]
			return environ
		}
	}
	return append(environ, "PATH="+prefix)
}

func lspBinDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	dirs := []string{
		filepath.Join(home, ".local", "share", "nvim", "mason", "bin"),
		filepath.Join(home, ".local", "bin"),
		"/opt/homebrew/bin",
		"/usr/local/bin",
		filepath.Join(home, ".npm-global", "bin"),
	}
	if pnpmHome := os.Getenv("PNPM_HOME"); pnpmHome != "" {
		dirs = append(dirs, pnpmHome)
	} else {
		dirs = append(dirs, filepath.Join(home, ".local", "share", "pnpm"))
	}
	return dirs
}

func (c *Client) writeMessage(message any) error {
	body, err := json.Marshal(message)
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

func (c *Client) readLoop() {
	defer close(c.done)
	reader := bufio.NewReader(c.stdout)

	for {
		body, err := readContentLengthMessage(reader)
		if err != nil {
			c.readErr = err
			c.pending.Range(func(key, value any) bool {
				close(value.(chan *jsonRPCMessage))
				c.pending.Delete(key)
				return true
			})
			return
		}

		var message jsonRPCMessage
		if err := json.Unmarshal(body, &message); err != nil {
			continue
		}
		if message.ID != nil && message.Method == "" {
			if ch, ok := c.pending.Load(*message.ID); ok {
				ch.(chan *jsonRPCMessage) <- &message
			}
			continue
		}
		if message.ID != nil && message.Method != "" {
			c.handleRequest(*message.ID, message.Method, message.Params)
			continue
		}
		if message.Method != "" && message.ID == nil {
			c.handleNotification(message.Method, message.Params)
		}
	}
}

func (c *Client) handleRequest(id int64, method string, params json.RawMessage) {
	var result any
	var errResp *jsonRPCError

	switch method {
	case "workspace/configuration":
		var request ConfigurationParams
		if err := json.Unmarshal(params, &request); err != nil {
			errResp = &jsonRPCError{Code: -32602, Message: "invalid params"}
			break
		}
		values := make([]any, len(request.Items))
		for index := range values {
			values[index] = map[string]any{}
		}
		result = values
	case "workspace/workspaceFolders":
		c.metaMu.RLock()
		result = append([]WorkspaceFolder(nil), c.workspaceFolders...)
		c.metaMu.RUnlock()
	case "client/registerCapability",
		"client/unregisterCapability",
		"window/workDoneProgress/create",
		"window/showMessageRequest",
		"workspace/codeLens/refresh",
		"workspace/diagnostic/refresh",
		"workspace/inlayHint/refresh",
		"workspace/inlineValue/refresh",
		"workspace/semanticTokens/refresh":
		result = nil
	default:
		errResp = &jsonRPCError{Code: -32601, Message: "method not found"}
	}

	response := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
		Error:   errResp,
	}
	if err := c.writeMessage(response); err != nil {
		return
	}
}

func (c *Client) handleNotification(method string, params json.RawMessage) {
	switch method {
	case "textDocument/publishDiagnostics":
		if c.onDiagnostics == nil {
			return
		}
		var payload PublishDiagnosticsParams
		if err := json.Unmarshal(params, &payload); err == nil {
			c.onDiagnostics(payload)
		}
	}
}

func readContentLengthMessage(reader *bufio.Reader) ([]byte, error) {
	var contentLength int
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read header: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if value, ok := strings.CutPrefix(line, "Content-Length:"); ok {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return nil, fmt.Errorf("parse Content-Length %q: %w", value, err)
			}
			contentLength = n
		}
	}
	if contentLength <= 0 {
		return nil, fmt.Errorf("missing or zero Content-Length")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, body); err != nil {
		return nil, fmt.Errorf("read body (%d bytes): %w", contentLength, err)
	}
	return body, nil
}
