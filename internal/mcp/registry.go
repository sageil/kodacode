package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sageil/kodacode/internal/tool"
)

const (
	perServerStartupTimeout    = 5 * time.Second
	mcpMaxOutputChars          = 32 * 1024
	mcpOutputTruncationMessage = "\n\n[output truncated to protect context budget]"
)

type ServerConfig struct {
	Name    string
	Type    string
	Command string
	Args    []string
	Env     map[string]string
}

type Registry struct {
	clients map[string]Transport
	cached  []tool.Tool
}

func NewRegistry(ctx context.Context, servers []ServerConfig) (*Registry, error) {
	clients := make(map[string]Transport, len(servers))
	var errs []error
	for _, server := range servers {
		if !strings.EqualFold(strings.TrimSpace(server.Type), "stdio") {
			continue
		}
		startCtx, cancel := context.WithTimeout(ctx, perServerStartupTimeout)
		transport, err := NewStdioTransport(server.Command, server.Args, server.Env)
		if err == nil {
			err = initializeTransport(startCtx, transport)
		}
		cancel()
		if err != nil {
			errs = append(errs, fmt.Errorf("mcp server %q: %w", strings.TrimSpace(server.Name), err))
			continue
		}
		clients[strings.TrimSpace(server.Name)] = transport
	}
	return &Registry{clients: clients}, errors.Join(errs...)
}

func (r *Registry) Tools(ctx context.Context) ([]tool.Tool, error) {
	if r == nil {
		return nil, nil
	}
	if r.cached != nil {
		return append([]tool.Tool(nil), r.cached...), nil
	}
	names := make([]string, 0, len(r.clients))
	for name := range r.clients {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]tool.Tool, 0, len(names))
	var errs []error
	for _, serverName := range names {
		tools, err := discoverTools(ctx, serverName, r.clients[serverName])
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out = append(out, tools...)
	}
	r.cached = append([]tool.Tool(nil), out...)
	return append([]tool.Tool(nil), out...), errors.Join(errs...)
}

func (r *Registry) Close() error {
	if r == nil {
		return nil
	}
	var errs []error
	for _, client := range r.clients {
		if err := client.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type mcpToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type listToolsResponse struct {
	Tools []mcpToolDef `json:"tools"`
}

var validToolName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func discoverTools(ctx context.Context, serverName string, transport Transport) ([]tool.Tool, error) {
	raw, err := transport.Call(ctx, "tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("mcp tools/list for %q: %w", serverName, err)
	}
	var response listToolsResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("mcp decode tools/list for %q: %w", serverName, err)
	}
	out := make([]tool.Tool, 0, len(response.Tools))
	for _, def := range response.Tools {
		if !validToolName.MatchString(strings.TrimSpace(def.Name)) {
			continue
		}
		var schema map[string]any
		if err := json.Unmarshal(def.InputSchema, &schema); err != nil {
			continue
		}
		out = append(out, newTool(serverName, def, transport))
	}
	return out, nil
}

type registryTool struct {
	definition tool.Definition
	serverName string
	toolName   string
	transport  Transport
}

func newTool(serverName string, def mcpToolDef, transport Transport) registryTool {
	name := tool.MCPToolName(serverName, def.Name)
	return registryTool{
		definition: tool.Definition{
			Name:                name,
			Description:         strings.TrimSpace(def.Description),
			ProviderDescription: strings.TrimSpace(def.Description),
			InputSchema:         append(json.RawMessage(nil), def.InputSchema...),
		},
		serverName: serverName,
		toolName:   strings.TrimSpace(def.Name),
		transport:  transport,
	}
}

func (t registryTool) Definition() tool.Definition {
	return t.definition
}

func (t registryTool) Execute(ctx context.Context, _ tool.ExecutionContext, args json.RawMessage) (tool.Result, error) {
	params := map[string]any{
		"name":      t.toolName,
		"arguments": json.RawMessage(args),
	}
	raw, err := t.transport.Call(ctx, "tools/call", params)
	if err != nil {
		return tool.Result{}, fmt.Errorf("mcp call %s/%s: %w", t.serverName, t.toolName, err)
	}
	var payload struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return tool.Result{Output: truncateMCPOutput(string(raw))}, nil
	}
	var builder strings.Builder
	for _, part := range payload.Content {
		if part.Type == "text" {
			builder.WriteString(part.Text)
		}
	}
	return tool.Result{Output: truncateMCPOutput(builder.String())}, nil
}

func truncateMCPOutput(output string) string {
	if mcpMaxOutputChars <= 0 || utf8.RuneCountInString(output) <= mcpMaxOutputChars {
		return output
	}
	runes := []rune(output)
	if mcpMaxOutputChars == 1 {
		return string(runes[:1]) + mcpOutputTruncationMessage
	}
	return string(runes[:mcpMaxOutputChars-1]) + "…" + mcpOutputTruncationMessage
}
