package service_test

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/service"
	"github.com/sageil/kodacode/v1/internal/tool"
)

func TestToolResolverMiddleware_PopulatesTools(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(&tool.Tool{Name: "bash", Description: "run shell commands"})
	reg.Register(&tool.Tool{Name: "read", Description: "read a file"})
	reg.Register(&tool.Tool{Name: "memory", Description: "project memory"})

	mw := service.NewToolResolverMiddleware(reg, nil, "")
	req := &pipeline.TurnRequest{
		Agent: config.AgentConfig{Tools: []string{"bash"}},
	}
	called := false
	err := mw(context.Background(), req, func(_ context.Context, r *pipeline.TurnRequest) error {
		called = true
		if len(r.Tools) != 2 {
			t.Fatalf("NewToolResolverMiddleware() tool count = %d, want 2", len(r.Tools))
		}
		if r.Tools[0].Name != "bash" && r.Tools[1].Name != "bash" {
			t.Errorf("NewToolResolverMiddleware() tools = %v, want bash included", r.Tools)
		}
		if r.Tools[0].Name != "memory" && r.Tools[1].Name != "memory" {
			t.Errorf("NewToolResolverMiddleware() tools = %v, want memory included", r.Tools)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("next was not called")
	}
}

func TestToolResolverMiddleware_AllToolsWhenNoFilter(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(&tool.Tool{Name: "bash"})
	reg.Register(&tool.Tool{Name: "read"})
	reg.Register(&tool.Tool{Name: "memory"})

	mw := service.NewToolResolverMiddleware(reg, nil, "")
	req := &pipeline.TurnRequest{
		Agent: config.AgentConfig{Tools: nil},
	}
	err := mw(context.Background(), req, func(_ context.Context, r *pipeline.TurnRequest) error {
		if len(r.Tools) != 3 {
			t.Errorf("NewToolResolverMiddleware() with no filter = %d tools, want 3", len(r.Tools))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestToolResolverMiddleware_RestrictsMCPToolsByAllowList(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(&tool.Tool{Name: "read", ReadOnly: true})
	reg.Register(&tool.Tool{Name: "filesystem_read", ReadOnly: true, IsMCP: true})

	mw := service.NewToolResolverMiddleware(reg, nil, "")
	req := &pipeline.TurnRequest{
		Agent: config.AgentConfig{Tools: []string{"read"}},
	}
	err := mw(context.Background(), req, func(_ context.Context, r *pipeline.TurnRequest) error {
		if len(r.Tools) != 1 || r.Tools[0].Name != "read" {
			t.Fatalf("tools = %v, want only read", r.Tools)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestToolResolverMiddleware_AllowsWildcardMCPTools(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(&tool.Tool{Name: "read", ReadOnly: true})
	reg.Register(&tool.Tool{Name: "filesystem_read", ReadOnly: true, IsMCP: true})

	mw := service.NewToolResolverMiddleware(reg, nil, "")
	req := &pipeline.TurnRequest{
		Agent: config.AgentConfig{Tools: []string{"read", "mcp:*"}},
	}
	err := mw(context.Background(), req, func(_ context.Context, r *pipeline.TurnRequest) error {
		if len(r.Tools) != 2 {
			t.Fatalf("tools = %v, want read + MCP tool", r.Tools)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestToolResolverMiddleware_PrioritizesRelevantTools(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(&tool.Tool{Name: "bash", Description: "Run shell commands"})
	reg.Register(&tool.Tool{Name: "edit", Description: "Precise file edits"})
	reg.Register(&tool.Tool{Name: "lsp", Description: "Symbol lookup, references, diagnostics"})
	reg.Register(&tool.Tool{Name: "search", Description: "Broad project search"})

	mw := service.NewToolResolverMiddleware(reg, nil, "")
	req := &pipeline.TurnRequest{
		Agent: config.AgentConfig{Tools: nil},
		CurrentInput: &provider.Message{
			Role: "user",
			Parts: []provider.MessagePart{
				provider.TextPart{Text: "Find all references to SessionService before editing it"},
			},
		},
	}
	err := mw(context.Background(), req, func(_ context.Context, r *pipeline.TurnRequest) error {
		if len(r.Tools) < 2 {
			t.Fatalf("tools = %v, want at least 2", r.Tools)
		}
		if r.Tools[0].Name != "lsp" && r.Tools[0].Name != "search" {
			t.Fatalf("first tool = %q, want lsp or search for reference lookup", r.Tools[0].Name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestToolResolverMiddleware_AlwaysIncludesMemoryDespiteAgentConfig(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(&tool.Tool{Name: "read", ReadOnly: true})
	reg.Register(&tool.Tool{Name: "memory"})

	mw := service.NewToolResolverMiddleware(reg, nil, "")
	req := &pipeline.TurnRequest{
		Agent: config.AgentConfig{
			Tools:     []string{"read"},
			DenyTools: []string{"memory"},
		},
	}
	err := mw(context.Background(), req, func(_ context.Context, r *pipeline.TurnRequest) error {
		if len(r.Tools) != 2 {
			t.Fatalf("tools = %v, want read + memory", r.Tools)
		}
		if r.Tools[0].Name != "memory" && r.Tools[1].Name != "memory" {
			t.Fatalf("tools = %v, want memory included despite deny/allow config", r.Tools)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
