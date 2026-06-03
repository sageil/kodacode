package app

import (
	"context"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
	"github.com/sageil/kodacode/internal/workspace"
)

func (e *ToolExecutor) executionContext(ctx context.Context, state events.SessionState, input ExecuteToolInput, scope *workspace.Scope) tool.ExecutionContext {
	return tool.ExecutionContext{
		SessionID:       input.SessionID,
		Workspace:       scope,
		Search:          e.search,
		WebSearch:       e.webSearch,
		OutputEmitter:   nil,
		QuestionAsker:   e.toolQuestionAsker(ctx, state, input),
		TaskManager:     e.toolTaskManager(ctx, input),
		DelegateManager: e.toolDelegateManager(ctx, state, input),
		CodeIntelAPI:    e.toolCodeIntel(state),
		MemoryManager:   e.toolMemoryManager(state),
		SkillCatalog:    e.toolSkillCatalog(state),
		WorkflowOutput:  e.toolWorkflowPhaseOutputManager(ctx, state, input),
		WorkflowReview:  e.toolWorkflowReviewResultManager(ctx, state, input),
	}
}

func (e *ToolExecutor) toolOutputEmitter(ctx context.Context, input ExecuteToolInput) tool.OutputEmitter {
	return func(chunk tool.OutputChunk) error {
		if chunk.Chunk == "" {
			return nil
		}
		return e.sessions.publishEphemeral(input.SessionID, input.TurnID, events.TypeToolExecOutput, events.ToolExecOutputPayload{
			CallID: input.ToolCallID,
			Chunk:  chunk.Chunk,
			Stream: chunk.Stream,
		})
	}
}
