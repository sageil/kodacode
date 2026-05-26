package app

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type ToolResultDetail struct {
	Output           string
	Error            string
	StructuredResult json.RawMessage
}

func (s *SessionService) LoadToolResult(ctx context.Context, sessionID, turnID, callID string) (ToolResultDetail, error) {
	state, err := s.Snapshot(ctx, sessionID)
	if err != nil {
		return ToolResultDetail{}, err
	}
	turn := state.Turns[turnID]
	if turn == nil {
		return ToolResultDetail{}, ErrTurnIDRequired
	}
	call := turn.ToolCalls[callID]
	if call == nil {
		return ToolResultDetail{}, ErrToolCallIDRequired
	}
	output := call.Output
	errorText := call.Error
	structured := cloneStructuredResult(call.StructuredResult)
	if call.OutputBlob != nil && strings.TrimSpace(call.OutputBlob.Ref) != "" && call.OutputBlob.Bytes > 0 && s.blobs != nil {
		if loaded, err := s.blobs.Load(ctx, call.OutputBlob.Ref); err == nil {
			output = loaded
		}
	}
	if call.ErrorBlob != nil && strings.TrimSpace(call.ErrorBlob.Ref) != "" && call.ErrorBlob.Bytes > 0 && s.blobs != nil {
		if loaded, err := s.blobs.Load(ctx, call.ErrorBlob.Ref); err == nil {
			errorText = loaded
		}
	}
	if output == "" && errorText == "" && structured == nil && call.Completed {
		fallbackOutput, fallbackError, fallbackStructured, ok, err := s.loadToolResultFromEvents(ctx, sessionID, turnID, callID)
		if err != nil {
			return ToolResultDetail{}, err
		}
		if ok {
			output = fallbackOutput
			errorText = fallbackError
			structured = fallbackStructured
		}
	}
	return ToolResultDetail{
		Output:           output,
		Error:            errorText,
		StructuredResult: structured,
	}, nil
}

func (s *SessionService) loadToolResultFromEvents(ctx context.Context, sessionID, turnID, callID string) (string, string, json.RawMessage, bool, error) {
	replayed, err := s.store.Replay(ctx, events.Query{
		SessionID:     sessionID,
		AfterSequence: -1,
		ExcludeTypes:  []events.Type{events.TypeSessionHistoryCheckpoint, events.TypeSessionStateSnapshot},
	})
	if err != nil {
		return "", "", nil, false, err
	}
	var (
		output     string
		errText    string
		structured json.RawMessage
		found      bool
	)
	for _, event := range replayed {
		if event.TurnID != turnID {
			continue
		}
		switch payload := event.Payload.(type) {
		case events.ToolExecEndPayload:
			if payload.CallID != callID {
				continue
			}
			output = payload.Output
			errText = payload.Error
			structured = cloneStructuredResult(payload.StructuredResult)
			found = true
		}
	}
	return output, errText, structured, found, nil
}
