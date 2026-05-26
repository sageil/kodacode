package app

import (
	"context"
	"sync"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/observability"
	"github.com/sageil/kodacode/internal/tool"
	"github.com/sageil/kodacode/internal/workspace"
)

type ExecutionService struct {
	sessions   *SessionService
	config     ExecutionConfig
	logger     *observability.Logger
	instanceID string

	backgroundLogs BackgroundExecutionLogStore
	backgroundMu   sync.Mutex
	backgroundRuns map[string]context.CancelFunc
}

type resolvedExecutionRequest struct {
	Request  tool.ExecutionRequest
	Contract executionContract
}

type contractPathRequest struct {
	Access workspace.Access
	Path   string
	Reason string
}

func NewExecutionService(sessions *SessionService) *ExecutionService {
	return NewExecutionServiceWithConfig(sessions, defaultExecutionConfig())
}

func NewExecutionServiceWithConfig(sessions *SessionService, config ExecutionConfig) *ExecutionService {
	return &ExecutionService{
		sessions:       sessions,
		config:         config,
		instanceID:     newRuntimeID("background-supervisor"),
		backgroundRuns: make(map[string]context.CancelFunc),
	}
}

func (s *ExecutionService) SetLogger(logger *observability.Logger) {
	s.logger = logger
}

func (s *ExecutionService) SetBackgroundLogStore(store BackgroundExecutionLogStore) {
	if s == nil {
		return
	}
	s.backgroundLogs = store
}

func (s *ExecutionService) Execute(ctx context.Context, tl tool.Tool, state events.SessionState, input ExecuteToolInput) (ToolExecutionResult, bool, error) {
	introspector, ok := tl.(tool.ExecutionRequestIntrospector)
	if !ok {
		return ToolExecutionResult{}, false, nil
	}

	resolved, err := resolveExecutionRequest(state, s.config, introspector, input.Arguments, input.ExecutionExecPolicy)
	if err != nil {
		return ToolExecutionResult{}, true, err
	}
	if err := s.appendExecutionDeclared(ctx, input, state, resolved); err != nil {
		return ToolExecutionResult{}, true, err
	}

	if pending, ok, err := s.authorizeExecution(ctx, input, state, resolved); err != nil {
		return ToolExecutionResult{}, true, err
	} else if ok {
		if s.logger != nil {
			s.logger.Op("execution pending permission",
				"session_id", input.SessionID,
				"turn_id", input.TurnID,
				"tool_call_id", input.ToolCallID,
				"tool_name", input.ToolName,
				"request_id", pending.PendingRequestID,
				"execution_id", executionID(input.ToolCallID),
			)
		}
		return pending, true, nil
	}
	if executionRunsInBackground(resolved.Request.Intent) {
		backgroundResult, err := s.executeBackgroundIntent(ctx, input, resolved)
		return backgroundResult, true, err
	}

	if _, err := s.sessions.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      events.TypeExecutionStarted,
		Payload: events.ExecutionStartedPayload{
			ExecutionID: executionID(input.ToolCallID),
			ToolCallID:  input.ToolCallID,
			ToolName:    input.ToolName,
			Input:       string(input.Arguments),
		},
	}); err != nil {
		return ToolExecutionResult{}, true, err
	}

	runResult, runErr := runExecutionCommand(ctx, resolved.Contract, executionRunOptions{
		StdoutStream: "combined",
		StderrStream: "combined",
		Emit: func(chunk executionOutputChunk) error {
			return s.sessions.publishEphemeral(input.SessionID, input.TurnID, events.TypeExecutionOutput, events.ExecutionOutputPayload{
				ExecutionID: executionID(input.ToolCallID),
				ToolCallID:  input.ToolCallID,
				Stream:      chunk.Stream,
				Chunk:       chunk.Chunk,
			})
		},
	})
	formatted := formatExecutionResult(resolved.Request, string(runResult.Output), runResult.Truncated, runErr)
	status := executionStatusFromRunError(runErr)
	output := formatted
	errorText := ""
	if runErr != nil {
		output = ""
		errorText = formatted
	}

	finalizeCtx, cancelFinalize := executionFinalizeContext(ctx)
	defer cancelFinalize()

	runtimePayload, err := executionToolExecEndPayload(finalizeCtx, s.sessions.blobs, resolved.Request, input, status, output, errorText, executionRuntimeFromRunResult(runResult))
	if err != nil {
		return ToolExecutionResult{}, true, err
	}
	if err := s.appendExecutionToolEnd(finalizeCtx, input, runtimePayload); err != nil {
		return ToolExecutionResult{}, true, err
	}

	return ToolExecutionResult{
		Status:      ToolExecutionStatusExecuted,
		Output:      output,
		Error:       errorText,
		ErrorDetail: runtimePayload.ErrorDetail,
	}, true, nil
}
