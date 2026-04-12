package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
)

type sseTransport struct {
	url     string
	headers map[string]string
	client  *http.Client
	seq     atomic.Int64
}

// NewSSETransport creates an HTTP-based MCP transport that speaks the MCP
// Streamable HTTP protocol (POST to the endpoint with
// Accept: application/json, text/event-stream). Responses may be returned
// as raw JSON or as SSE-framed events; both are handled.
// headers is an optional map of additional request headers (e.g. Authorization).
func NewSSETransport(url string, headers map[string]string) *sseTransport {
	return &sseTransport{
		url:     url,
		headers: headers,
		client:  &http.Client{},
	}
}

func (t *sseTransport) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := t.seq.Add(1)
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal sse rpc request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build sse http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// MCP Streamable HTTP requires both content types in Accept.
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range t.headers {
		httpReq.Header.Set(k, v)
	}
	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sse http call: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sse mcp http error: status %d: %s", resp.StatusCode, string(b))
	}

	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/event-stream") {
		// SSE-framed response: scan for the first "data:" line.
		return parseSingleSSEEvent(resp.Body)
	}

	// Plain JSON response.
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read sse response: %w", err)
	}
	return parseRPCResult(respBody)
}

// parseSingleSSEEvent reads SSE lines and returns the JSON from the first
// non-empty "data:" field. It ignores "event:", "id:", and comment lines.
func parseSingleSSEEvent(r io.Reader) (json.RawMessage, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" || data == "[DONE]" {
				continue
			}
			return parseRPCResult([]byte(data))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read sse stream: %w", err)
	}
	return nil, fmt.Errorf("sse mcp: stream ended without a data event")
}

func parseRPCResult(b []byte) (json.RawMessage, error) {
	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(b, &rpcResp); err != nil {
		return nil, fmt.Errorf("decode sse rpc response: %w", err)
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("sse mcp rpc error: %s", rpcResp.Error.Message)
	}
	// Detect non-JSON-RPC responses (e.g. {"code":500,"msg":"404 NOT_FOUND"})
	// that parse without error but have no jsonrpc/id/result fields.
	if rpcResp.ID == 0 && rpcResp.Result == nil && rpcResp.Error == nil {
		return nil, fmt.Errorf("sse mcp: not a valid JSON-RPC response: %s", string(b))
	}
	return rpcResp.Result, nil
}

// Notify sends a JSON-RPC notification (no ID, no response expected).
func (t *sseTransport) Notify(ctx context.Context, method string, params any) error {
	notif := jsonRPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	body, err := json.Marshal(notif)
	if err != nil {
		return fmt.Errorf("marshal sse rpc notification: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build sse notification request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range t.headers {
		httpReq.Header.Set(k, v)
	}
	resp, err := t.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("sse notification: %w", err)
	}
	_ = resp.Body.Close()
	return nil
}

func (t *sseTransport) Close() error { return nil }
