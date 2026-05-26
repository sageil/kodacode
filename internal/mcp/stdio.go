package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const stdioMaxFrameBytes = 1 << 20

var errMCPResponseTooLarge = errors.New("stdio mcp: response exceeded maximum frame size")

type Transport interface {
	Call(ctx context.Context, method string, params any) (json.RawMessage, error)
	Notify(ctx context.Context, method string, params any) error
	Close() error
}

type stdioTransport struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	seq   atomic.Int64

	writeMu sync.Mutex

	pendingMu sync.Mutex
	pending   map[int64]chan rpcResult
	readErr   error
}

type rpcResult struct {
	data json.RawMessage
	err  error
}

type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonRPCNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonRPCError struct {
	Message string `json:"message"`
}

type jsonRPCResponse struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *jsonRPCError   `json:"error,omitempty"`
}

var safeEnvKeys = map[string]bool{
	"PATH":     true,
	"HOME":     true,
	"LANG":     true,
	"LC_ALL":   true,
	"LC_CTYPE": true,
	"TERM":     true,
	"USER":     true,
	"SHELL":    true,
	"TMPDIR":   true,
}

func NewStdioTransport(command string, args []string, env map[string]string) (*stdioTransport, error) {
	cmd := exec.Command(strings.TrimSpace(command), args...)
	cmd.Env = filteredEnv(env)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdio mcp stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdio mcp stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("stdio mcp start: %w", err)
	}
	transport := &stdioTransport{
		cmd:     cmd,
		stdin:   stdin,
		pending: make(map[int64]chan rpcResult),
	}
	go transport.readLoop(bufio.NewReader(stdout))
	return transport, nil
}

func (t *stdioTransport) readLoop(reader *bufio.Reader) {
	for {
		line, err := readRPCFrame(reader)
		if len(bytes.TrimSpace(line)) > 0 {
			var response jsonRPCResponse
			if decodeErr := json.Unmarshal(line, &response); decodeErr == nil && response.ID != 0 {
				result := rpcResult{data: response.Result}
				if response.Error != nil {
					result.err = fmt.Errorf("mcp rpc error: %s", response.Error.Message)
				}
				t.pendingMu.Lock()
				ch, ok := t.pending[response.ID]
				if ok {
					delete(t.pending, response.ID)
				}
				t.pendingMu.Unlock()
				if ok {
					ch <- result
				}
			}
		}
		if err == nil {
			continue
		}
		if err == io.EOF {
			err = fmt.Errorf("stdio mcp: subprocess stdout closed")
		}
		t.pendingMu.Lock()
		t.readErr = err
		for id, ch := range t.pending {
			ch <- rpcResult{err: fmt.Errorf("stdio mcp: stream closed before response for id=%d: %w", id, err)}
			delete(t.pending, id)
		}
		t.pendingMu.Unlock()
		return
	}
}

func readRPCFrame(reader *bufio.Reader) ([]byte, error) {
	if reader == nil {
		return nil, io.EOF
	}
	var line []byte
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(line)+len(fragment) > stdioMaxFrameBytes {
			return nil, errMCPResponseTooLarge
		}
		line = append(line, fragment...)
		if err == nil {
			return line, nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return line, err
	}
}

func (t *stdioTransport) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}
	requestID := t.seq.Add(1)
	request := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  method,
		Params:  params,
	}
	responseCh := make(chan rpcResult, 1)
	t.pendingMu.Lock()
	if t.readErr != nil {
		t.pendingMu.Unlock()
		return nil, fmt.Errorf("mcp call %s: transport closed: %w", method, t.readErr)
	}
	t.pending[requestID] = responseCh
	t.pendingMu.Unlock()

	payload, err := json.Marshal(request)
	if err != nil {
		t.removePending(requestID)
		return nil, fmt.Errorf("marshal rpc request: %w", err)
	}
	payload = append(payload, '\n')
	t.writeMu.Lock()
	_, err = t.stdin.Write(payload)
	t.writeMu.Unlock()
	if err != nil {
		t.removePending(requestID)
		return nil, fmt.Errorf("write rpc request: %w", err)
	}

	select {
	case result := <-responseCh:
		return result.data, result.err
	case <-ctx.Done():
		t.removePending(requestID)
		return nil, fmt.Errorf("mcp call %s timed out: %w", method, ctx.Err())
	}
}

func (t *stdioTransport) Notify(_ context.Context, method string, params any) error {
	notification := jsonRPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	payload, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("marshal rpc notification: %w", err)
	}
	payload = append(payload, '\n')
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	if _, err := t.stdin.Write(payload); err != nil {
		return fmt.Errorf("write rpc notification: %w", err)
	}
	return nil
}

func (t *stdioTransport) Close() error {
	if t == nil {
		return nil
	}
	_ = t.stdin.Close()
	done := make(chan error, 1)
	go func() {
		done <- t.cmd.Wait()
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(500 * time.Millisecond):
		if t.cmd.Process != nil {
			_ = t.cmd.Process.Kill()
		}
		return nil
	}
}

func (t *stdioTransport) removePending(id int64) {
	t.pendingMu.Lock()
	delete(t.pending, id)
	t.pendingMu.Unlock()
}

type initializeRequest struct {
	ProtocolVersion string `json:"protocolVersion"`
	Capabilities    any    `json:"capabilities"`
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

func initializeTransport(ctx context.Context, transport Transport) error {
	request := initializeRequest{
		ProtocolVersion: "2024-11-05",
		Capabilities:    map[string]any{},
	}
	request.ClientInfo.Name = "kodacode"
	request.ClientInfo.Version = "0"
	if _, err := transport.Call(ctx, "initialize", request); err != nil {
		return fmt.Errorf("mcp initialize: %w", err)
	}
	if err := transport.Notify(ctx, "notifications/initialized", map[string]any{}); err != nil {
		return fmt.Errorf("mcp initialized notification: %w", err)
	}
	return nil
}

func filteredEnv(extra map[string]string) []string {
	out := make([]string, 0, len(safeEnvKeys)+len(extra))
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if ok && safeEnvKeys[key] {
			out = append(out, entry)
		}
	}
	for key, value := range extra {
		out = append(out, key+"="+value)
	}
	return out
}
