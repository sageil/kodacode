package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/engine"
	"github.com/sageil/kodacode/internal/observability"
	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/provider"
)

var (
	ErrTurnEngineRequired               = errors.New("turn engine is required")
	ErrPromptShaperRequired             = errors.New("prompt shaper is required")
	ErrProviderClientRequired           = errors.New("provider client is required")
	ErrToolExecutorRequired             = errors.New("tool executor is required")
	ErrAgentIDRequired                  = errors.New("agent id is required")
	ErrPromptInstructionsRequired       = errors.New("prompt instructions are required")
	ErrTurnStalledNoProgress            = errors.New("the model repeated the same tool calls without making progress")
	ErrTurnExceededProviderRequestLimit = errors.New("the turn hit the provider request limit without completing")
)

const defaultProviderRetryAttempts = 2

type TurnRunner struct {
	engine                     *engine.Engine
	shaper                     prompt.Shaper
	provider                   provider.Client
	models                     modelCatalog
	modelOverrides             []ModelOverrideConfig
	outputBudgets              OutputBudgetsConfig
	utilityModel               provider.ModelRef
	utilityClientFactory       func(string) (provider.Client, error)
	utilityProviderEnabled     utilityProviderAvailableFunc
	utilityModelTimeout        time.Duration
	utilityRetryPolicy         utilityRetryPolicy
	sessionConfig              SessionConfig
	sessions                   *SessionService
	tools                      *ToolExecutor
	logger                     *observability.Logger
	now                        func() time.Time
	wait                       func(context.Context, time.Duration) error
	retries                    int
	maxProviderRequestsPerTurn int
	maxOutputContinuations     int
}

func NewTurnRunner(eng *engine.Engine, shaper prompt.Shaper, client provider.Client, sessions *SessionService, tools *ToolExecutor) (*TurnRunner, error) {
	if eng == nil {
		return nil, ErrTurnEngineRequired
	}
	if shaper == nil {
		return nil, ErrPromptShaperRequired
	}
	if client == nil {
		return nil, ErrProviderClientRequired
	}
	if sessions == nil {
		return nil, ErrSessionServiceRequired
	}
	if tools == nil {
		return nil, ErrToolExecutorRequired
	}
	return &TurnRunner{
		engine:                     eng,
		shaper:                     shaper,
		provider:                   client,
		sessions:                   sessions,
		tools:                      tools,
		now:                        time.Now,
		wait:                       waitWithContext,
		outputBudgets:              defaultOutputBudgets(),
		utilityRetryPolicy:         defaultUtilityRetryPolicy(),
		retries:                    defaultProviderRetryAttempts,
		maxProviderRequestsPerTurn: defaultMaxProviderRequestsPerTurn,
		maxOutputContinuations:     defaultMaxOutputContinuations,
	}, nil
}

func (r *TurnRunner) SetLogger(logger *observability.Logger) {
	r.logger = logger
}

func (r *TurnRunner) SetModelCatalog(catalog modelCatalog) {
	r.models = catalog
}

func (r *TurnRunner) SetOutputBudgetConfig(budgets OutputBudgetsConfig, overrides []ModelOverrideConfig) {
	if r == nil {
		return
	}
	r.outputBudgets = budgets.Effective()
	r.modelOverrides = append([]ModelOverrideConfig(nil), overrides...)
}

func (r *TurnRunner) SetUtilityModelConfig(model provider.ModelRef, factory func(string) (provider.Client, error)) {
	if r == nil {
		return
	}
	r.utilityModel = model
	r.utilityClientFactory = factory
}

func (r *TurnRunner) SetUtilityProviderAvailability(check utilityProviderAvailableFunc) {
	if r == nil {
		return
	}
	r.utilityProviderEnabled = check
}

func (r *TurnRunner) SetUtilityModelTimeout(timeout time.Duration) {
	if r == nil {
		return
	}
	r.utilityModelTimeout = timeout
}

func (r *TurnRunner) SetUtilityRetryPolicy(policy utilityRetryPolicy) {
	if r == nil {
		return
	}
	r.utilityRetryPolicy = policy
}

func (r *TurnRunner) SetSessionConfig(config SessionConfig) {
	if r == nil {
		return
	}
	r.sessionConfig = config
	r.retries = config.MaxRetries
	r.maxProviderRequestsPerTurn = config.EffectiveMaxProviderRequestsPerTurn()
	r.maxOutputContinuations = config.EffectiveMaxOutputContinuations()
}

type RunTurnInput struct {
	SessionID            string
	TurnID               string
	AgentID              string
	UserText             string
	Attachments          []provider.Attachment
	Fragments            []prompt.Fragment
	ModelRoute           provider.ModelRoute
	ThinkingSupported    bool
	ThinkingEnabled      bool
	ThinkingMode         string
	AllowedTools         []string
	InitialState         *turnLoopState
	ContinuationReason   string
	SkipUserMessageEvent bool
	TurnStartAfterSeq    int64
	WorkflowBudget       workflowTurnBudget
	HistoryMode          turnHistoryMode
}

type ResumeTurnInput struct {
	SessionID                   string
	TurnID                      string
	AgentID                     string
	UserText                    string
	QuestionAnswer              string
	Attachments                 []provider.Attachment
	Fragments                   []prompt.Fragment
	Instructions                string
	CacheablePrefix             string
	DynamicSuffix               string
	PromptCompactionTokensSaved int
	ModelRoute                  provider.ModelRoute
	ThinkingSupported           bool
	ThinkingEnabled             bool
	ThinkingMode                string
	AllowedTools                []string
	RequestID                   string
}

type TurnRunStatus string

const (
	TurnRunStatusCompleted TurnRunStatus = "completed"
	TurnRunStatusCanceled  TurnRunStatus = "canceled"
	TurnRunStatusFailed    TurnRunStatus = "failed"
	TurnRunStatusPending   TurnRunStatus = "pending_interaction"
	TurnRunStatusRolled    TurnRunStatus = "rolled_over"
)

type TurnRolloverReason string

const TurnRolloverReasonContextLimit TurnRolloverReason = "context_limit"

type RunTurnResult struct {
	Status           TurnRunStatus
	PendingRequestID string
	RolloverReason   TurnRolloverReason
}

func (r *TurnRunner) Run(ctx context.Context, input RunTurnInput) (RunTurnResult, error) {
	historyReplayAfterSequence := input.TurnStartAfterSeq
	if !input.SkipUserMessageEvent {
		userEvent, err := r.appendUserMessageEvent(ctx, input.SessionID, input.TurnID, input.UserText, input.Attachments)
		if err != nil {
			return RunTurnResult{}, err
		}
		historyReplayAfterSequence = userEvent.Sequence - 1
	}

	preparedPrompt, err := r.preparePrompt(ctx, input.SessionID, input.TurnID, input.AgentID, input.UserText, input.Fragments)
	if err != nil {
		return r.failTurn(ctx, input.SessionID, input.TurnID, input.ModelRoute, err)
	}
	if err := r.appendPromptCompiled(ctx, input.SessionID, input.TurnID, preparedPrompt, input.ModelRoute.Primary); err != nil {
		return RunTurnResult{}, err
	}
	return r.executeTurnLoop(ctx, turnLoopInput{
		SessionID:                   input.SessionID,
		TurnID:                      input.TurnID,
		AgentID:                     input.AgentID,
		Instructions:                preparedPrompt.View.Instructions,
		CacheablePrefix:             preparedPrompt.View.CacheablePrefix,
		DynamicSuffix:               preparedPrompt.View.DynamicSuffix,
		PromptCompactionTokensSaved: preparedPrompt.PromptTokensSaved(),
		ModelRoute:                  input.ModelRoute,
		ThinkingSupported:           input.ThinkingSupported,
		ThinkingEnabled:             input.ThinkingEnabled,
		ThinkingMode:                input.ThinkingMode,
		AllowedTools:                slices.Clone(input.AllowedTools),
		HistoryReplayAfterSequence:  historyReplayAfterSequence,
		ContinuationReason:          input.ContinuationReason,
		WorkflowBudget:              input.WorkflowBudget,
		HistoryMode:                 input.HistoryMode,
		State:                       initialTurnLoopState(input),
	})
}

func initialTurnLoopState(input RunTurnInput) turnLoopState {
	userInput := provider.Input{}
	if strings.TrimSpace(input.UserText) != "" || len(input.Attachments) > 0 {
		userInput = provider.Input{
			Kind:        provider.InputKindUserMessage,
			Content:     input.UserText,
			Attachments: cloneProviderAttachments(input.Attachments),
		}
	}
	if input.InitialState == nil {
		conversation := make([]provider.Input, 0, 1)
		if userInput.Kind != "" {
			conversation = append(conversation, userInput)
		}
		return turnLoopState{
			UserInput:           userInput,
			Conversation:        conversation,
			LatestToolStepStart: -1,
		}
	}
	state := cloneTurnLoopState(*input.InitialState)
	state.UserInput = userInput
	if len(state.Conversation) == 0 {
		if userInput.Kind != "" {
			state.Conversation = []provider.Input{userInput}
		} else {
			state.Conversation = nil
		}
	}
	return state
}

func (r *TurnRunner) failTurn(ctx context.Context, sessionID, turnID string, modelRoute provider.ModelRoute, cause error) (RunTurnResult, error) {
	if err := r.appendTurnError(ctx, sessionID, turnID, cause); err != nil {
		return RunTurnResult{}, err
	}
	if err := r.appendSessionHistoryCheckpoint(ctx, sessionID, modelRoute, nil, nil, -1); err != nil {
		return RunTurnResult{}, err
	}
	return RunTurnResult{Status: TurnRunStatusFailed}, nil
}

func waitWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
