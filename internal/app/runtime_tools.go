package app

import (
	"context"

	"github.com/sageil/kodacode/internal/observability"
	searchsvc "github.com/sageil/kodacode/internal/search"
	"github.com/sageil/kodacode/internal/skill"
	"github.com/sageil/kodacode/internal/tool"
	websearchsvc "github.com/sageil/kodacode/internal/websearch"
)

func buildRuntimeTools(webSearch *websearchsvc.Service) []tool.Tool {
	tools := append([]tool.Tool(nil), tool.DefaultRuntimeTools()...)
	if webSearch != nil && webSearch.Enabled() {
		tools = append(tools, tool.NewWebSearchTool())
	}
	return tools
}

type runtimeToolExecutorConfig struct {
	Sessions     *SessionService
	Execution    ExecutionConfig
	Search       *searchsvc.Service
	WebSearch    *websearchsvc.Service
	CodeIntel    *CodeIntelService
	Memory       *MemoryService
	Skills       *skill.Registry
	Delegate     delegateRuntime
	Logger       *observability.Logger
	Background   BackgroundExecutionLogStore
	RuntimeTools []tool.Tool
	MCPTools     []tool.Tool
}

func newRuntimeToolExecutor(config runtimeToolExecutorConfig) (*ToolExecutor, error) {
	executor, err := NewToolExecutorWithConfig(config.Sessions, config.Execution, config.RuntimeTools...)
	if err != nil {
		return nil, err
	}
	if config.Background != nil {
		executor.SetBackgroundLogStore(config.Background)
	}
	executor.SetSearchService(config.Search)
	executor.SetWebSearchService(config.WebSearch)
	executor.SetCodeIntelService(config.CodeIntel)
	executor.SetMemoryService(config.Memory)
	executor.SetSkillRegistry(config.Skills)
	executor.SetDelegateRuntime(config.Delegate)
	executor.SetLogger(config.Logger)
	if len(config.MCPTools) > 0 {
		executor.ReplaceMCPTools(config.MCPTools)
	}
	return executor, nil
}

func (r *Runtime) currentMCPToolsOrNil(ctx context.Context) []tool.Tool {
	if r == nil {
		return nil
	}
	return r.currentMCPTools(ctx)
}
