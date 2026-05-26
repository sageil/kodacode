package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/events"
)

type QuestionRequestInput struct {
	SessionID  string
	TurnID     string
	ToolCallID string
	ToolName   string
	PlanID     string
	Question   string
	Options    []string
	Multiple   bool
	Purpose    string
}

type AnswerQuestionInput struct {
	SessionID string
	TurnID    string
	RequestID string
	Answer    string
}

func (s *SessionService) RequestQuestion(ctx context.Context, input QuestionRequestInput) (string, error) {
	if strings.TrimSpace(input.SessionID) == "" {
		return "", ErrSessionIDRequired
	}
	if strings.TrimSpace(input.TurnID) == "" {
		return "", ErrTurnIDRequired
	}
	toolCallID := strings.TrimSpace(input.ToolCallID)
	toolName := strings.TrimSpace(input.ToolName)
	if toolCallID == "" && toolName != "" {
		return "", ErrToolCallIDRequired
	}
	if toolCallID != "" && toolName == "" {
		return "", ErrToolNameRequired
	}

	state, err := s.Snapshot(ctx, input.SessionID)
	if err != nil {
		return "", err
	}
	if toolCallID != "" {
		if existing := findPendingQuestionForToolCall(state, toolCallID); existing != nil {
			return existing.QuestionID, nil
		}
	}

	requestID := fmt.Sprintf("q-%d", time.Now().UTC().UnixNano())
	if _, err := s.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      events.TypeQuestionRequested,
		Payload: events.QuestionRequestedPayload{
			QuestionID: requestID,
			ToolCallID: toolCallID,
			ToolName:   toolName,
			PlanID:     strings.TrimSpace(input.PlanID),
			Question:   input.Question,
			Options:    append([]string(nil), input.Options...),
			Multiple:   input.Multiple,
			Purpose:    input.Purpose,
		},
	}); err != nil {
		return "", err
	}
	return requestID, nil
}

func (s *SessionService) AnswerQuestion(ctx context.Context, input AnswerQuestionInput) (events.Event, error) {
	if strings.TrimSpace(input.SessionID) == "" {
		return events.Event{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(input.TurnID) == "" {
		return events.Event{}, ErrTurnIDRequired
	}
	if strings.TrimSpace(input.RequestID) == "" {
		return events.Event{}, ErrQuestionRequestMissing
	}

	state, err := s.Snapshot(ctx, input.SessionID)
	if err != nil {
		return events.Event{}, err
	}
	request := pendingQuestionRequestState(state, input.RequestID)
	if request == nil {
		return events.Event{}, ErrQuestionRequestMissing
	}
	if !questionOptionAllowed(request.Options, input.Answer) {
		return events.Event{}, ErrQuestionAnswerInvalid
	}
	turnID := strings.TrimSpace(request.TurnID)
	if turnID == "" {
		turnID = input.TurnID
	}

	return s.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    turnID,
		Type:      events.TypeQuestionAnswered,
		Payload: events.QuestionAnsweredPayload{
			QuestionID: input.RequestID,
			ToolCallID: request.ToolCallID,
			PlanID:     request.PlanID,
			Answer:     input.Answer,
		},
	})
}

func findPendingQuestionForToolCall(state events.SessionState, toolCallID string) *events.QuestionRequestState {
	toolCallID = strings.TrimSpace(toolCallID)
	if toolCallID == "" {
		return nil
	}
	for _, requestID := range state.PendingQuestionOrder {
		request := state.PendingQuestions[requestID]
		if request == nil {
			continue
		}
		if request.ToolCallID == toolCallID {
			return request
		}
	}
	return nil
}

func findQuestionAnswerForToolCall(state events.SessionState, turnID, toolCallID string) *events.QuestionAnswerState {
	toolCallID = strings.TrimSpace(toolCallID)
	if toolCallID == "" {
		return nil
	}
	for _, answer := range state.QuestionAnswers {
		if answer == nil {
			continue
		}
		if answer.TurnID == turnID && answer.ToolCallID == toolCallID {
			return answer
		}
	}
	return nil
}

func pendingQuestionRequestState(state events.SessionState, requestID string) *events.QuestionRequestState {
	return state.PendingQuestions[requestID]
}

func questionOptionAllowed(options []string, answer string) bool {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return false
	}
	for _, option := range options {
		if strings.TrimSpace(option) == answer {
			return true
		}
	}
	return false
}
