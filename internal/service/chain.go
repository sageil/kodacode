package service

import (
	"context"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/mcp"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/repository"
	"github.com/sageil/kodacode/v1/internal/sandbox"
	"github.com/sageil/kodacode/v1/internal/snapshot"
	"github.com/sageil/kodacode/v1/internal/tool"
)

// ChainConfig holds all dependencies needed to build a session pipeline chain.
type ChainConfig struct {
	Sandbox       *sandbox.Sandbox
	PromptBuilder *SystemPromptBuilder
	Config        *config.Config
	Registry      *provider.Registry
	ToolRegistry  *tool.Registry
	MCPRegistry   *mcp.MCPRegistry
	Messages      repository.MessageRepo
	Sessions      repository.SessionRepo
	Publish       func(sessionID string, ev SSEEvent)
	AskPerm       AskPermissionFunc
	AskUser       AskUserFunc
	SpawnSubagent SpawnSubagentFunc
	SnapshotSvc   *snapshot.Service
	GetCost       func(ctx context.Context, sessionID string) *SessionCost
	GetBudget     func(ctx context.Context, sessionID string) BudgetStatus
	LSPDiag       tool.LSPDiagnoser
	TaskStore     *tool.TaskStore
	GetTraces     func(sessionID string) *SessionTraces
}

func BuildSessionChain(c ChainConfig) *pipeline.Chain {
	updateTitle := func(ctx context.Context, sessionID, title string) {
		sess, err := c.Sessions.Get(ctx, sessionID)
		if err != nil {
			return
		}
		sess.Title = title
		if err := c.Sessions.Update(ctx, sess); err != nil {
			return
		}
		c.Publish(sessionID, SSEEvent{
			Type: "title_updated",
			Data: SSETitleData{Title: title},
		})
	}

	return pipeline.BuildChain(
		sandbox.NewMiddleware(c.Sandbox),
		NewToolResolverMiddleware(c.ToolRegistry, c.MCPRegistry, toolResolverProjectDir(c.PromptBuilder)),
		NewPhaseFilterMiddleware(&c.Config.Session),
		NewSystemPromptMiddleware(c.PromptBuilder, c.TaskStore),
		NewCompactionMiddleware(&c.Config.Session, c.Messages, c.Registry, c.ToolRegistry, c.Config, c.Publish, c.GetCost),
		NewTitleMiddleware(c.Registry, c.Config, updateTitle, c.GetCost),
		NewLLMMiddleware(&c),
	)
}

func toolResolverProjectDir(builder *SystemPromptBuilder) string {
	if builder == nil {
		return ""
	}
	return builder.cfg.ProjectDir
}
