package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/tool"
)

// MCPRegistry manages connections to multiple MCP servers.
type MCPRegistry struct {
	clients map[string]MCPTransport
	configs map[string]config.MCPServerConfig
	mu      sync.RWMutex
	cached  []tool.Tool
}

// NewMCPRegistryFromClients constructs a registry from a name→transport map.
func NewMCPRegistryFromClients(clients map[string]MCPTransport) *MCPRegistry {
	return &MCPRegistry{clients: clients}
}

// NewMCPRegistryFromClientsWithConfig constructs a registry from a
// name→transport map plus the matching configured server entries.
func NewMCPRegistryFromClientsWithConfig(clients map[string]MCPTransport, cfg []config.MCPServerConfig) *MCPRegistry {
	configs := make(map[string]config.MCPServerConfig, len(cfg))
	for _, server := range cfg {
		configs[server.Name] = server
	}
	return &MCPRegistry{clients: clients, configs: configs}
}

// NewMCPRegistry constructs an MCPRegistry from config, starting each server
// concurrently with a per-server timeout. Failed or slow connections are
// logged and skipped so startup is never blocked for more than a few seconds.
func NewMCPRegistry(ctx context.Context, cfg []config.MCPServerConfig) (*MCPRegistry, error) {
	const perServerTimeout = 5 * time.Second

	type result struct {
		name      string
		transport MCPTransport
	}

	var wg sync.WaitGroup
	results := make(chan result, len(cfg))

	for _, sc := range cfg {
		sc := sc
		if !sc.IsEnabled() {
			continue
		}
		switch sc.Type {
		case "stdio", "sse":
		default:
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sctx, scancel := context.WithTimeout(ctx, perServerTimeout)
			defer scancel()

			var (
				transport MCPTransport
				err       error
			)
			switch sc.Type {
			case "stdio":
				transport, err = NewStdioTransport(sc.Command, sc.Args, sc.Env)
			case "sse":
				transport = NewSSETransport(sc.URL, sc.Headers)
			}
			if err != nil {
				log.Printf("mcp: failed to start %s transport for %q: %v", sc.Type, sc.Name, err)
				return
			}
			if err := initializeTransport(sctx, transport); err != nil {
				log.Printf("mcp: initialize handshake failed for %q (continuing anyway): %v", sc.Name, err)
			}
			log.Printf("mcp: server %q connected successfully", sc.Name)
			results <- result{name: sc.Name, transport: transport}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	clients := make(map[string]MCPTransport, len(cfg))
	for r := range results {
		clients[r.name] = r.transport
	}
	return NewMCPRegistryFromClientsWithConfig(clients, cfg), nil
}

// ConnectedServers returns the names of successfully connected MCP servers.
func (r *MCPRegistry) ConnectedServers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.clients))
	for name := range r.clients {
		names = append(names, name)
	}
	return names
}

func (r *MCPRegistry) Tools(ctx context.Context) ([]tool.Tool, error) {
	r.mu.RLock()
	if r.cached != nil {
		tools := r.cached
		r.mu.RUnlock()
		return tools, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cached != nil { // double-check after acquiring write lock
		return r.cached, nil
	}
	type result struct {
		tools []tool.Tool
	}
	var wg sync.WaitGroup
	results := make(chan result, len(r.clients))
	for name, transport := range r.clients {
		name, transport := name, transport
		serverCfg := r.configs[name]
		wg.Add(1)
		go func() {
			defer wg.Done()
			ts, err := discoverTools(ctx, name, transport, serverCfg)
			if err != nil {
				log.Printf("mcp: tool discovery failed for %q: %v", name, err)
				return
			}
			log.Printf("mcp: discovered %d tools from %q", len(ts), name)
			results <- result{tools: ts}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	var tools []tool.Tool
	for res := range results {
		tools = append(tools, res.tools...)
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	r.cached = tools
	return tools, nil
}

// InvalidateTools clears the cached tool list so the next call to Tools
// re-discovers tools from all connected servers.
func (r *MCPRegistry) InvalidateTools() {
	r.mu.Lock()
	r.cached = nil
	r.mu.Unlock()
}

// Close terminates all server connections.
func (r *MCPRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var errs []error
	for _, t := range r.clients {
		if err := t.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("mcp close errors: %v", errs)
	}
	return nil
}

type mcpToolDef struct {
	Name        string          `json:"name"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations mcpAnnotations  `json:"annotations,omitempty"`
}

type mcpAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool  `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`
}

type listToolsResponse struct {
	Tools []mcpToolDef `json:"tools"`
}

const maxToolsPerServer = 200

var validToolName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func discoverTools(ctx context.Context, serverName string, t MCPTransport, serverCfg config.MCPServerConfig) ([]tool.Tool, error) {
	raw, err := t.Call(ctx, "tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("mcp tools/list for %q: %w", serverName, err)
	}
	var resp listToolsResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("mcp decode tools/list for %q: %w", serverName, err)
	}
	if len(resp.Tools) > maxToolsPerServer {
		log.Printf("mcp: server %q returned %d tools, capping at %d", serverName, len(resp.Tools), maxToolsPerServer)
		resp.Tools = resp.Tools[:maxToolsPerServer]
	}
	tools := make([]tool.Tool, 0, len(resp.Tools))
	for _, def := range resp.Tools {
		if !validToolName.MatchString(def.Name) {
			log.Printf("mcp: skipping tool with invalid name %q from %q", def.Name, serverName)
			continue
		}
		var schema map[string]any
		if err := json.Unmarshal(def.InputSchema, &schema); err != nil {
			log.Printf("mcp: skipping tool %q from %q: invalid input schema: %v", def.Name, serverName, err)
			continue
		}
		def := def
		sn := serverName
		tr := t
		tools = append(tools, tool.Tool{
			Name:        sn + "_" + def.Name,
			Description: def.Description,
			Parameters:  []byte(def.InputSchema),
			PromptHints: buildMCPPromptHints(def, serverCfg),
			IsMCP:       true,
			Execute:     makeMCPExecutor(sn, def.Name, tr),
		})
	}
	return tools, nil
}

func buildMCPPromptHints(def mcpToolDef, serverCfg config.MCPServerConfig) provider.ToolPromptHints {
	hints := provider.ToolPromptHints{}
	title := strings.TrimSpace(def.Annotations.Title)
	if title == "" {
		title = strings.TrimSpace(def.Title)
	}
	if title != "" {
		hints.Summary = title
	}
	if def.Annotations.ReadOnlyHint != nil && *def.Annotations.ReadOnlyHint {
		hints.Guidance = "Prefer this read-only external tool when the task clearly targets its domain."
		hints.Triggers = append(hints.Triggers, "read external system", "lookup external data")
	}
	if def.Annotations.OpenWorldHint != nil && *def.Annotations.OpenWorldHint {
		if hints.Guidance == "" {
			hints.Guidance = "Use only when the task clearly needs data or actions outside the local project."
		}
		hints.Triggers = append(hints.Triggers, "external system", "remote service")
	}
	if def.Annotations.DestructiveHint != nil && *def.Annotations.DestructiveHint {
		hints.Guidance = "Use carefully; this tool may make destructive external changes. Verify intent before calling."
		hints.PreserveParameterDocs = true
		hints.Triggers = append(hints.Triggers, "destructive external change", "dangerous external action")
	} else if def.Annotations.ReadOnlyHint != nil && !*def.Annotations.ReadOnlyHint {
		hints.Guidance = "Use when you need to create or update resources in this external system."
		hints.Triggers = append(hints.Triggers, "update external system", "create external resource")
	}
	if def.Annotations.IdempotentHint != nil && *def.Annotations.IdempotentHint && hints.Guidance == "" {
		hints.Guidance = "Prefer when you need a repeatable action against this external system."
	}
	hints = mergePromptHints(hints, mcpConfigPromptHints(serverCfg.ToolHints["*"]))
	hints = mergePromptHints(hints, mcpConfigPromptHints(serverCfg.ToolHints[def.Name]))
	return hints
}

func mcpConfigPromptHints(cfg config.MCPToolHintConfig) provider.ToolPromptHints {
	return provider.ToolPromptHints{
		Summary:               cfg.Summary,
		Guidance:              cfg.Guidance,
		Triggers:              append([]string(nil), cfg.Triggers...),
		FileExts:              append([]string(nil), cfg.FileExts...),
		PreserveParameterDocs: config.Bool(cfg.PreserveParameterDocs),
	}
}

func mergePromptHints(base, overlay provider.ToolPromptHints) provider.ToolPromptHints {
	if overlay.Summary != "" {
		base.Summary = overlay.Summary
	}
	if overlay.Guidance != "" {
		base.Guidance = overlay.Guidance
	}
	if len(overlay.Triggers) > 0 {
		base.Triggers = appendUniqueHintStrings(base.Triggers, overlay.Triggers...)
	}
	if len(overlay.FileExts) > 0 {
		base.FileExts = appendUniqueHintStrings(base.FileExts, overlay.FileExts...)
	}
	if overlay.PreserveParameterDocs {
		base.PreserveParameterDocs = true
	}
	return base
}

func appendUniqueHintStrings(base []string, extra ...string) []string {
	out := append([]string(nil), base...)
	seen := make(map[string]bool, len(out))
	for _, item := range out {
		seen[item] = true
	}
	for _, item := range extra {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func makeMCPExecutor(serverName, toolName string, t MCPTransport) func(context.Context, tool.ExecutionContext, []byte) (*tool.Result, error) {
	return func(ctx context.Context, _ tool.ExecutionContext, args []byte) (*tool.Result, error) {
		params := map[string]any{
			"name":      toolName,
			"arguments": json.RawMessage(args),
		}
		raw, err := t.Call(ctx, "tools/call", params)
		if err != nil {
			return nil, fmt.Errorf("mcp call %s/%s: %w", serverName, toolName, err)
		}
		var result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			return &tool.Result{Output: string(raw)}, nil
		}
		var sb strings.Builder
		for _, c := range result.Content {
			if c.Type == "text" {
				sb.WriteString(c.Text)
			}
		}
		return &tool.Result{Output: sb.String()}, nil
	}
}
