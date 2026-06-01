package app

import (
	"context"
	"errors"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

var (
	ErrHeadlessRuntimeRequired      = errors.New("headless runtime is required")
	ErrHeadlessCommandRequired      = errors.New("headless command is required")
	ErrHeadlessCommandTypeRequired  = errors.New("headless command type is required")
	ErrHeadlessCommandUnsupported   = errors.New("unsupported headless command")
	ErrHeadlessSessionIDRequired    = errors.New("headless session_id is required")
	ErrHeadlessWorkspaceRootMissing = errors.New("headless workspace_root is required")
)

type HeadlessCommandType string

const (
	HeadlessCommandOpenSession       HeadlessCommandType = "session.open"
	HeadlessCommandStartTurn         HeadlessCommandType = "turn.start"
	HeadlessCommandCancelTurn        HeadlessCommandType = "turn.cancel"
	HeadlessCommandAnswerQuestion    HeadlessCommandType = "question.answer"
	HeadlessCommandResolvePermission HeadlessCommandType = "permission.resolve"
)

type HeadlessCommand struct {
	Type              HeadlessCommandType
	OpenSession       *HeadlessOpenSessionCommand
	StartTurn         *HeadlessStartTurnCommand
	CancelTurn        *HeadlessCancelTurnCommand
	AnswerQuestion    *HeadlessAnswerQuestionCommand
	ResolvePermission *HeadlessResolvePermissionCommand
}

type HeadlessOpenSessionCommand struct {
	WorkspaceRoot   string
	AdditionalRoots []string
	Resume          bool
}

type HeadlessStartTurnCommand struct {
	SessionID       string
	TurnID          string
	UserText        string
	Attachments     []AttachmentInput
	AgentID         string
	SkillIDs        []string
	ThinkingEnabled bool
	ThinkingMode    string
}

type HeadlessCancelTurnCommand struct {
	SessionID string
	TurnID    string
}

type HeadlessAnswerQuestionCommand struct {
	SessionID string
	TurnID    string
	RequestID string
	Answer    string
	UserText  string
	SkillIDs  []string
}

type HeadlessResolvePermissionCommand struct {
	SessionID              string
	TurnID                 string
	RequestID              string
	UserText               string
	SkillIDs               []string
	Decision               events.PermissionDecision
	Scope                  events.PermissionScope
	GrantPath              string
	Recursive              bool
	ExecutionDecision      events.ExecutionApprovalDecision
	ExecutionExecPolicy    *events.ExecutionPolicyAmendment
	ExecutionNetworkPolicy *events.ExecutionNetworkPolicyAmendment
}

type HeadlessCommandResult struct {
	CommandType HeadlessCommandType
	SessionID   string
	TurnID      string
	Run         *RunSessionResult
	OpenSession *OpenWorkspaceSessionResult
}

type HeadlessWatchRequest struct {
	SessionID     string
	AfterSequence int64
}

type HeadlessEvent struct {
	Event events.Event
}

type HeadlessRuntimeTransport struct {
	runtime *Runtime
}

func NewHeadlessRuntimeTransport(runtime *Runtime) (*HeadlessRuntimeTransport, error) {
	if runtime == nil {
		return nil, ErrHeadlessRuntimeRequired
	}
	return &HeadlessRuntimeTransport{runtime: runtime}, nil
}

func (t *HeadlessRuntimeTransport) Execute(ctx context.Context, command HeadlessCommand) (HeadlessCommandResult, error) {
	if t == nil || t.runtime == nil {
		return HeadlessCommandResult{}, ErrHeadlessRuntimeRequired
	}
	if command.empty() {
		return HeadlessCommandResult{}, ErrHeadlessCommandRequired
	}
	switch command.Type {
	case HeadlessCommandOpenSession:
		return t.executeOpenSession(ctx, command.OpenSession)
	case HeadlessCommandStartTurn:
		return t.executeStartTurn(ctx, command.StartTurn)
	case HeadlessCommandCancelTurn:
		return t.executeCancelTurn(ctx, command.CancelTurn)
	case HeadlessCommandAnswerQuestion:
		return t.executeAnswerQuestion(ctx, command.AnswerQuestion)
	case HeadlessCommandResolvePermission:
		return t.executeResolvePermission(ctx, command.ResolvePermission)
	case "":
		return HeadlessCommandResult{}, ErrHeadlessCommandTypeRequired
	default:
		return HeadlessCommandResult{}, ErrHeadlessCommandUnsupported
	}
}

func (t *HeadlessRuntimeTransport) Watch(ctx context.Context, request HeadlessWatchRequest) (<-chan HeadlessEvent, error) {
	if t == nil || t.runtime == nil {
		return nil, ErrHeadlessRuntimeRequired
	}
	if strings.TrimSpace(request.SessionID) == "" {
		return nil, ErrHeadlessSessionIDRequired
	}
	eventsCh, err := t.runtime.WatchSession(ctx, request.SessionID, request.AfterSequence)
	if err != nil {
		return nil, err
	}
	out := make(chan HeadlessEvent)
	go func() {
		defer close(out)
		for event := range eventsCh {
			select {
			case out <- HeadlessEvent{Event: event}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (t *HeadlessRuntimeTransport) executeOpenSession(ctx context.Context, command *HeadlessOpenSessionCommand) (HeadlessCommandResult, error) {
	if command == nil {
		return HeadlessCommandResult{}, ErrHeadlessCommandRequired
	}
	if strings.TrimSpace(command.WorkspaceRoot) == "" {
		return HeadlessCommandResult{}, ErrHeadlessWorkspaceRootMissing
	}
	result, err := t.runtime.OpenWorkspaceSession(ctx, command.WorkspaceRoot, append([]string(nil), command.AdditionalRoots...), command.Resume)
	if err != nil {
		return HeadlessCommandResult{}, err
	}
	return HeadlessCommandResult{
		CommandType: HeadlessCommandOpenSession,
		SessionID:   result.SessionID,
		OpenSession: &result,
	}, nil
}

func (t *HeadlessRuntimeTransport) executeStartTurn(ctx context.Context, command *HeadlessStartTurnCommand) (HeadlessCommandResult, error) {
	if command == nil {
		return HeadlessCommandResult{}, ErrHeadlessCommandRequired
	}
	if strings.TrimSpace(command.SessionID) == "" {
		return HeadlessCommandResult{}, ErrHeadlessSessionIDRequired
	}
	turnID := strings.TrimSpace(command.TurnID)
	if turnID == "" {
		turnID = NewTurnID()
	}
	result, err := t.runtime.StartSessionTurn(ctx, StartSessionTurnInput{
		SessionID:       command.SessionID,
		TurnID:          turnID,
		UserText:        command.UserText,
		Attachments:     append([]AttachmentInput(nil), command.Attachments...),
		AgentID:         command.AgentID,
		SkillIDs:        append([]string(nil), command.SkillIDs...),
		ThinkingEnabled: command.ThinkingEnabled,
		ThinkingMode:    command.ThinkingMode,
	})
	if err != nil {
		return HeadlessCommandResult{}, err
	}
	return HeadlessCommandResult{
		CommandType: HeadlessCommandStartTurn,
		SessionID:   result.SessionID,
		TurnID:      result.TurnID,
		Run:         &result,
	}, nil
}

func (t *HeadlessRuntimeTransport) executeCancelTurn(ctx context.Context, command *HeadlessCancelTurnCommand) (HeadlessCommandResult, error) {
	if command == nil {
		return HeadlessCommandResult{}, ErrHeadlessCommandRequired
	}
	if strings.TrimSpace(command.SessionID) == "" {
		return HeadlessCommandResult{}, ErrHeadlessSessionIDRequired
	}
	if err := t.runtime.CancelSessionTurn(ctx, CancelSessionTurnInput{
		SessionID: command.SessionID,
		TurnID:    command.TurnID,
	}); err != nil {
		return HeadlessCommandResult{}, err
	}
	return HeadlessCommandResult{
		CommandType: HeadlessCommandCancelTurn,
		SessionID:   command.SessionID,
		TurnID:      command.TurnID,
	}, nil
}

func (t *HeadlessRuntimeTransport) executeAnswerQuestion(ctx context.Context, command *HeadlessAnswerQuestionCommand) (HeadlessCommandResult, error) {
	if command == nil {
		return HeadlessCommandResult{}, ErrHeadlessCommandRequired
	}
	if strings.TrimSpace(command.SessionID) == "" {
		return HeadlessCommandResult{}, ErrHeadlessSessionIDRequired
	}
	result, err := t.runtime.AnswerSessionQuestion(ctx, AnswerSessionQuestionInput{
		SessionID: command.SessionID,
		TurnID:    command.TurnID,
		RequestID: command.RequestID,
		Answer:    command.Answer,
		UserText:  command.UserText,
		SkillIDs:  append([]string(nil), command.SkillIDs...),
	})
	if err != nil {
		return HeadlessCommandResult{}, err
	}
	return HeadlessCommandResult{
		CommandType: HeadlessCommandAnswerQuestion,
		SessionID:   result.SessionID,
		TurnID:      result.TurnID,
		Run:         &result,
	}, nil
}

func (t *HeadlessRuntimeTransport) executeResolvePermission(ctx context.Context, command *HeadlessResolvePermissionCommand) (HeadlessCommandResult, error) {
	if command == nil {
		return HeadlessCommandResult{}, ErrHeadlessCommandRequired
	}
	if strings.TrimSpace(command.SessionID) == "" {
		return HeadlessCommandResult{}, ErrHeadlessSessionIDRequired
	}
	result, err := t.runtime.ResolveSessionTurn(ctx, ResolveSessionTurnInput{
		SessionID:              command.SessionID,
		TurnID:                 command.TurnID,
		PermissionRequestID:    command.RequestID,
		UserText:               command.UserText,
		SkillIDs:               append([]string(nil), command.SkillIDs...),
		Decision:               command.Decision,
		Scope:                  command.Scope,
		GrantPath:              command.GrantPath,
		Recursive:              command.Recursive,
		ExecutionDecision:      command.ExecutionDecision,
		ExecutionExecPolicy:    command.ExecutionExecPolicy,
		ExecutionNetworkPolicy: command.ExecutionNetworkPolicy,
	})
	if err != nil {
		return HeadlessCommandResult{}, err
	}
	return HeadlessCommandResult{
		CommandType: HeadlessCommandResolvePermission,
		SessionID:   result.SessionID,
		TurnID:      result.TurnID,
		Run:         &result,
	}, nil
}

func (c HeadlessCommand) empty() bool {
	return c.Type == "" &&
		c.OpenSession == nil &&
		c.StartTurn == nil &&
		c.CancelTurn == nil &&
		c.AnswerQuestion == nil &&
		c.ResolvePermission == nil
}
