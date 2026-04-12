package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// MCPTransport handles JSON-RPC communication with one MCP server.
type MCPTransport interface {
	Call(ctx context.Context, method string, params any) (json.RawMessage, error)
	Close() error
}

// jsonRPCRequest is a JSON-RPC 2.0 request envelope.
type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// jsonRPCNotification is a JSON-RPC 2.0 notification (no ID, no response expected).
type jsonRPCNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// jsonRPCResponse is a JSON-RPC 2.0 response envelope.
type jsonRPCResponse struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// initializeTransport performs the MCP initialize handshake on a transport.
// The MCP protocol requires: initialize request → response → notifications/initialized.
func initializeTransport(ctx context.Context, t MCPTransport) error {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]string{
			"name":    "kodacode",
			"version": "1.0.0",
		},
	}
	_, err := t.Call(ctx, "initialize", params)
	if err != nil {
		return fmt.Errorf("mcp initialize: %w", err)
	}
	// Send initialized notification (no response expected).
	// We use Call which expects a response, so we send via the transport directly
	// if it supports notifications. For now, use a fire-and-forget approach
	// by sending as a notification through the Notify method if available,
	// or by handling it at the transport level.
	if n, ok := t.(notifier); ok {
		if err := n.Notify(ctx, "notifications/initialized", nil); err != nil {
			return fmt.Errorf("mcp initialized notification: %w", err)
		}
	}
	return nil
}

// notifier is an optional interface for transports that can send
// JSON-RPC notifications (messages with no ID and no response).
type notifier interface {
	Notify(ctx context.Context, method string, params any) error
}
