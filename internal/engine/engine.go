package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
)

type PromptCompiler interface {
	Compile(context.Context, prompt.Request) (prompt.Compiled, error)
}

type EventLog interface {
	Append(context.Context, events.Draft) (events.Event, error)
}

type Dependencies struct {
	Compiler PromptCompiler
	Events   EventLog
}

type Engine struct {
	compiler PromptCompiler
	events   EventLog
}

func New(deps Dependencies) (*Engine, error) {
	if deps.Compiler == nil {
		return nil, errors.New("compiler is required")
	}
	return &Engine{
		compiler: deps.Compiler,
		events:   deps.Events,
	}, nil
}

type TurnRequest struct {
	SessionID string
	TurnID    string
	AgentID   string
	UserText  string
	Fragments []prompt.Fragment
}

func (r TurnRequest) Validate() error {
	if strings.TrimSpace(r.SessionID) == "" {
		return errors.New("session_id is required")
	}
	if strings.TrimSpace(r.TurnID) == "" {
		return errors.New("turn_id is required")
	}
	if strings.TrimSpace(r.AgentID) == "" {
		return errors.New("agent_id is required")
	}
	if len(r.Fragments) == 0 {
		return errors.New("at least one prompt fragment is required")
	}
	return nil
}

type PreparedTurn struct {
	SessionID string
	TurnID    string
	AgentID   string
	Prompt    prompt.Compiled
}

func (e *Engine) PrepareTurn(ctx context.Context, req TurnRequest) (PreparedTurn, error) {
	if err := req.Validate(); err != nil {
		return PreparedTurn{}, err
	}

	compiled, err := e.compiler.Compile(ctx, prompt.Request{Fragments: req.Fragments})
	if err != nil {
		return PreparedTurn{}, fmt.Errorf("compile prompt: %w", err)
	}

	return PreparedTurn{
		SessionID: req.SessionID,
		TurnID:    req.TurnID,
		AgentID:   req.AgentID,
		Prompt:    compiled,
	}, nil
}
