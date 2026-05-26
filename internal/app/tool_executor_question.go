package app

import (
	"context"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func (e *ToolExecutor) toolQuestionAsker(ctx context.Context, state events.SessionState, input ExecuteToolInput) tool.QuestionAsker {
	return func(request tool.QuestionRequest) (tool.QuestionResponse, error) {
		if answered := findQuestionAnswerForToolCall(state, input.TurnID, input.ToolCallID); answered != nil {
			return tool.QuestionResponse{
				RequestID: answered.QuestionID,
				Answer:    answered.Answer,
				Answered:  true,
			}, nil
		}
		if pending := findPendingQuestionForToolCall(state, input.ToolCallID); pending != nil {
			return tool.QuestionResponse{
				RequestID: pending.QuestionID,
				Answered:  false,
			}, nil
		}

		requestID, err := e.sessions.RequestQuestion(ctx, QuestionRequestInput{
			SessionID:  input.SessionID,
			TurnID:     input.TurnID,
			ToolCallID: input.ToolCallID,
			ToolName:   input.ToolName,
			PlanID:     input.PlanID,
			Question:   request.Question,
			Options:    append([]string(nil), request.Options...),
			Multiple:   request.Multiple,
			Purpose:    request.Purpose,
		})
		if err != nil {
			return tool.QuestionResponse{}, err
		}
		return tool.QuestionResponse{
			RequestID: requestID,
			Answered:  false,
		}, nil
	}
}
