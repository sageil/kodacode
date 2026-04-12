package service

import (
	"context"
	"log"

	"github.com/sageil/kodacode/v1/internal/mcp"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/tool"
)

// NewToolResolverMiddleware builds TurnRequest.Tools from the tool registry
// and optional MCP registry before passing to the LLM.
func NewToolResolverMiddleware(
	toolRegistry *tool.Registry,
	mcpRegistry *mcp.MCPRegistry, // may be nil if no MCP configured
	projectDir string,
) pipeline.TurnMiddleware {
	return func(ctx context.Context, req *pipeline.TurnRequest, next pipeline.TurnHandler) error {
		var tools []provider.Tool

		denied := make(map[string]bool, len(req.Agent.DenyTools))
		for _, name := range req.Agent.DenyTools {
			denied[name] = true
		}

		seen := make(map[string]bool)
		for _, t := range toolRegistry.All() {
			if denied[t.Name] && !isAlwaysAllowedTool(t.Name) {
				continue
			}
			if !agentAllowsTool(req.Agent.Tools, t) {
				continue
			}
			tools = append(tools, provider.Tool{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
				PromptHints: t.PromptHints,
			})
			seen[t.Name] = true
		}

		// Include MCP tools discovered after startup, but only if the active
		// agent allows them.
		if mcpRegistry != nil {
			mcpTools, err := mcpRegistry.Tools(ctx)
			if err != nil {
				log.Printf("middleware_tools: mcp tool discovery failed: %v", err)
			} else {
				for _, t := range mcpTools {
					if (denied[t.Name] && !isAlwaysAllowedTool(t.Name)) || seen[t.Name] {
						continue
					}
					if !agentAllowsTool(req.Agent.Tools, &t) {
						continue
					}
					tools = append(tools, provider.Tool{
						Name:        t.Name,
						Description: t.Description,
						Parameters:  t.Parameters,
						PromptHints: t.PromptHints,
					})
					seen[t.Name] = true
				}
			}
		}

		userMessage, touchedFiles := currentTurnPromptContext(projectDir, req)
		req.Tools = reorderToolsForTurn(tools, userMessage, touchedFiles)
		if req.Step == 0 {
			names := make([]string, len(req.Tools))
			for i, t := range req.Tools {
				names[i] = t.Name
			}
			log.Printf("middleware_tools: %d tools available: %v", len(req.Tools), names)
		}
		return next(ctx, req)
	}
}

func agentAllowsTool(allowList []string, t *tool.Tool) bool {
	if isAlwaysAllowedTool(t.Name) {
		return true
	}
	if len(allowList) == 0 {
		return true
	}
	for _, allowed := range allowList {
		switch {
		case allowed == t.Name:
			return true
		case allowed == "mcp:*" && t.IsMCP:
			return true
		}
	}
	return false
}

func isAlwaysAllowedTool(name string) bool {
	return name == "memory"
}
