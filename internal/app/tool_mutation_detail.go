package app

import (
	"context"
	"errors"
)

var ErrToolMutationDetailUnavailable = errors.New("tool mutation detail is not available")

type ToolMutationDetail struct {
	Path    string
	Before  string
	After   string
	Existed bool
}

func (s *SessionService) LoadToolMutationDetail(ctx context.Context, sessionID, turnID, callID string) (ToolMutationDetail, error) {
	state, err := s.Snapshot(ctx, sessionID)
	if err != nil {
		return ToolMutationDetail{}, err
	}
	turn := state.Turns[turnID]
	if turn == nil {
		return ToolMutationDetail{}, ErrTurnIDRequired
	}
	call := turn.ToolCalls[callID]
	if call == nil {
		return ToolMutationDetail{}, ErrToolCallIDRequired
	}
	if call.WriteMutation == nil {
		return ToolMutationDetail{}, ErrToolMutationDetailUnavailable
	}
	before, err := loadWriteMutationBeforeContent(ctx, s.blobs, call.WriteMutation)
	if err != nil {
		return ToolMutationDetail{}, err
	}
	after, ok, err := restoreAfterContentForCall(call.ToolName, call.Input, before)
	if err != nil {
		return ToolMutationDetail{}, err
	}
	if !ok {
		return ToolMutationDetail{}, ErrToolMutationDetailUnavailable
	}
	return ToolMutationDetail{
		Path:    call.WriteMutation.Path,
		Before:  before,
		After:   after,
		Existed: call.WriteMutation.Existed,
	}, nil
}
