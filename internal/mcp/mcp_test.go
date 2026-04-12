package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/mcp"
	"github.com/sageil/kodacode/v1/internal/tool"
)

// fakeTransport implements MCPTransport.
type fakeTransport struct {
	tools []map[string]any
}

func (f *fakeTransport) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if method == "tools/list" {
		resp := map[string]any{"tools": f.tools}
		b, _ := json.Marshal(resp)
		return b, nil
	}
	if method == "tools/call" {
		b, _ := json.Marshal(map[string]any{
			"content": []map[string]any{{"type": "text", "text": "tool result"}},
		})
		return b, nil
	}
	return nil, nil
}

func (f *fakeTransport) Close() error { return nil }

func TestMCPRegistry_DiscoverTools(t *testing.T) {
	fake := &fakeTransport{
		tools: []map[string]any{
			{
				"name":        "search",
				"description": "web search",
				"inputSchema": map[string]any{"type": "object"},
			},
		},
	}
	reg := mcp.NewMCPRegistryFromClients(map[string]mcp.MCPTransport{
		"webtools": fake,
	})
	tools, err := reg.Tools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("want 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "webtools_search" {
		t.Errorf("want 'webtools_search', got %q", tools[0].Name)
	}
}

func TestMCPRegistry_ExecuteTool(t *testing.T) {
	fake := &fakeTransport{
		tools: []map[string]any{
			{
				"name":        "search",
				"description": "web search",
				"inputSchema": map[string]any{"type": "object"},
			},
		},
	}
	reg := mcp.NewMCPRegistryFromClients(map[string]mcp.MCPTransport{
		"webtools": fake,
	})
	tools, err := reg.Tools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) == 0 {
		t.Fatal("no tools returned")
	}
	ectx := tool.ExecutionContext{WorkDir: "/tmp"}
	result, err := tools[0].Execute(context.Background(), ectx, []byte(`{"query":"golang"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Output == "" {
		t.Error("expected non-empty output")
	}
}

func TestMCPRegistry_DerivesPromptHintsFromAnnotations(t *testing.T) {
	fake := &fakeTransport{
		tools: []map[string]any{
			{
				"name":        "issues_search",
				"title":       "Search GitHub Issues",
				"description": "Search issues and pull requests",
				"inputSchema": map[string]any{"type": "object"},
				"annotations": map[string]any{
					"readOnlyHint":  true,
					"openWorldHint": true,
				},
			},
		},
	}
	reg := mcp.NewMCPRegistryFromClients(map[string]mcp.MCPTransport{
		"github": fake,
	})
	tools, err := reg.Tools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("want 1 tool, got %d", len(tools))
	}
	if tools[0].PromptHints.Summary != "Search GitHub Issues" {
		t.Fatalf("summary = %q, want annotation title", tools[0].PromptHints.Summary)
	}
	if !strings.Contains(tools[0].PromptHints.Guidance, "read-only external tool") {
		t.Fatalf("guidance = %q, want read-only guidance", tools[0].PromptHints.Guidance)
	}
	if len(tools[0].PromptHints.Triggers) == 0 {
		t.Fatal("expected annotation-derived triggers")
	}
}

func TestMCPRegistry_ConfigToolHintsOverrideAnnotations(t *testing.T) {
	fake := &fakeTransport{
		tools: []map[string]any{
			{
				"name":        "search",
				"description": "Search vendor issues",
				"inputSchema": map[string]any{"type": "object"},
				"annotations": map[string]any{
					"readOnlyHint": true,
				},
			},
		},
	}
	reg := mcp.NewMCPRegistryFromClientsWithConfig(
		map[string]mcp.MCPTransport{"github": fake},
		[]config.MCPServerConfig{{
			Name: "github",
			ToolHints: map[string]config.MCPToolHintConfig{
				"*": {
					Guidance: "Use only for GitHub-hosted systems.",
				},
				"search": {
					Summary:  "Search GitHub issues and PRs",
					Triggers: []string{"github issues", "pull request search"},
				},
			},
		}},
	)
	tools, err := reg.Tools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("want 1 tool, got %d", len(tools))
	}
	if tools[0].PromptHints.Summary != "Search GitHub issues and PRs" {
		t.Fatalf("summary = %q, want config override", tools[0].PromptHints.Summary)
	}
	if tools[0].PromptHints.Guidance != "Use only for GitHub-hosted systems." {
		t.Fatalf("guidance = %q, want wildcard config guidance", tools[0].PromptHints.Guidance)
	}
	if strings.Join(tools[0].PromptHints.Triggers, ",") != "read external system,lookup external data,github issues,pull request search" {
		t.Fatalf("triggers = %v, want additive config triggers", tools[0].PromptHints.Triggers)
	}
}

func TestMCPRegistry_ToolsCached(t *testing.T) {
	callCount := 0
	fake := &fakeTransport{
		tools: []map[string]any{
			{"name": "foo", "description": "bar", "inputSchema": map[string]any{"type": "object"}},
		},
	}
	counting := &countingTransport{inner: fake, counter: &callCount}
	reg := mcp.NewMCPRegistryFromClients(map[string]mcp.MCPTransport{"srv": counting})
	reg.Tools(context.Background()) //nolint:errcheck
	reg.Tools(context.Background()) //nolint:errcheck
	if callCount != 1 {
		t.Errorf("want tools/list called once (cached), got %d calls", callCount)
	}
}

type countingTransport struct {
	inner   mcp.MCPTransport
	counter *int
}

func (c *countingTransport) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if method == "tools/list" {
		*c.counter++
	}
	return c.inner.Call(ctx, method, params)
}

func (c *countingTransport) Close() error { return c.inner.Close() }

func TestMCPRegistry_InvalidateTools(t *testing.T) {
	callCount := 0
	fake := &fakeTransport{
		tools: []map[string]any{
			{"name": "foo", "description": "bar", "inputSchema": map[string]any{"type": "object"}},
		},
	}
	counting := &countingTransport{inner: fake, counter: &callCount}
	reg := mcp.NewMCPRegistryFromClients(map[string]mcp.MCPTransport{"srv": counting})

	tools1, err := reg.Tools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if callCount != 1 {
		t.Fatalf("first Tools() call count = %d, want 1", callCount)
	}

	reg.InvalidateTools()

	tools2, err := reg.Tools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if callCount != 2 {
		t.Errorf("after InvalidateTools(), call count = %d, want 2", callCount)
	}
	if len(tools1) != len(tools2) {
		t.Errorf("tool counts differ: before=%d after=%d", len(tools1), len(tools2))
	}
}

func TestMCPRegistry_PartialDiscoveryFailure(t *testing.T) {
	good := &fakeTransport{
		tools: []map[string]any{
			{"name": "search", "description": "web search", "inputSchema": map[string]any{"type": "object"}},
		},
	}
	bad := &failingTransport{err: fmt.Errorf("connection refused")}
	reg := mcp.NewMCPRegistryFromClients(map[string]mcp.MCPTransport{
		"good-server": good,
		"bad-server":  bad,
	})

	tools, err := reg.Tools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("want 1 tool from good server, got %d", len(tools))
	}
	if tools[0].Name != "good-server_search" {
		t.Errorf("want tool from good-server, got %q", tools[0].Name)
	}
}

func TestMCPRegistry_ExecutionFailureOnDeadTransport(t *testing.T) {
	ft := &switchableTransport{
		callFn: func(ctx context.Context, method string, params any) (json.RawMessage, error) {
			if method == "tools/list" {
				resp := map[string]any{"tools": []map[string]any{
					{"name": "run", "description": "run a task", "inputSchema": map[string]any{"type": "object"}},
				}}
				b, _ := json.Marshal(resp)
				return b, nil
			}
			return nil, fmt.Errorf("stream closed before response")
		},
	}
	reg := mcp.NewMCPRegistryFromClients(map[string]mcp.MCPTransport{"srv": ft})

	tools, err := reg.Tools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("want 1 tool, got %d", len(tools))
	}

	ectx := tool.ExecutionContext{WorkDir: "/tmp"}
	_, err = tools[0].Execute(context.Background(), ectx, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error from dead transport, got nil")
	}
	if !strings.Contains(err.Error(), "stream closed") {
		t.Errorf("error = %q, want to contain 'stream closed'", err.Error())
	}
}

func TestMCPRegistry_InvalidateThenFailedDiscoveryReturnsEmpty(t *testing.T) {
	callCount := 0
	ft := &switchableTransport{
		callFn: func(ctx context.Context, method string, params any) (json.RawMessage, error) {
			if method == "tools/list" {
				callCount++
				if callCount == 1 {
					resp := map[string]any{"tools": []map[string]any{
						{"name": "foo", "description": "bar", "inputSchema": map[string]any{"type": "object"}},
					}}
					b, _ := json.Marshal(resp)
					return b, nil
				}
				return nil, fmt.Errorf("server crashed")
			}
			return nil, nil
		},
	}
	reg := mcp.NewMCPRegistryFromClients(map[string]mcp.MCPTransport{"srv": ft})

	tools1, err := reg.Tools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tools1) != 1 {
		t.Fatalf("first discovery: want 1 tool, got %d", len(tools1))
	}

	reg.InvalidateTools()

	tools2, err := reg.Tools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tools2) != 0 {
		t.Errorf("after invalidate + failed discovery: want 0 tools, got %d", len(tools2))
	}
}

func TestMCPRegistry_Close(t *testing.T) {
	fake := &fakeTransport{}
	reg := mcp.NewMCPRegistryFromClients(map[string]mcp.MCPTransport{"s": fake})
	if err := reg.Close(); err != nil {
		t.Errorf("unexpected close error: %v", err)
	}
}

type failingTransport struct {
	err error
}

func (f *failingTransport) Call(_ context.Context, _ string, _ any) (json.RawMessage, error) {
	return nil, f.err
}
func (f *failingTransport) Close() error { return nil }

type switchableTransport struct {
	callFn func(context.Context, string, any) (json.RawMessage, error)
}

func (s *switchableTransport) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	return s.callFn(ctx, method, params)
}
func (s *switchableTransport) Close() error { return nil }
