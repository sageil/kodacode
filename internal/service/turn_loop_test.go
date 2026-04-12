package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/permission"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/repository"
	"github.com/sageil/kodacode/v1/internal/sandbox"
	"github.com/sageil/kodacode/v1/internal/tool"
)

type loopStopTestProvider struct {
	calls        int
	toolsPerCall []int
	volatile     []string
}

type stalledExecutionProvider struct {
	calls        int
	toolsPerCall []int
	volatile     []string
}

type workflowIntentTestProvider struct {
	calls    int
	response string
	chatErr  error
	assert   func(model string, messages []provider.Message, opts provider.ChatOptions)
}

type blockedPrebuildProvider struct {
	calls int
}

type streamingBlockedPrebuildProvider struct {
	calls int
}

type turnLoopMessageRepo struct {
	messages []repository.Message
	nextID   int
}

func ptr[T any](v T) *T { return &v }

func (p *workflowIntentTestProvider) ID() string   { return "workflow-intent-test" }
func (p *workflowIntentTestProvider) Name() string { return "workflow-intent-test" }
func (p *workflowIntentTestProvider) Models(_ context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "fake-model", Name: "Fake Model", ContextSize: 8192}}, nil
}

func (p *workflowIntentTestProvider) Chat(_ context.Context, model string, messages []provider.Message, opts provider.ChatOptions) (<-chan provider.StreamChunk, error) {
	p.calls++
	if p.assert != nil {
		p.assert(model, messages, opts)
	}
	if p.chatErr != nil {
		return nil, p.chatErr
	}
	ch := make(chan provider.StreamChunk, 2)
	go func() {
		defer close(ch)
		ch <- provider.StreamChunk{Delta: p.response}
		ch <- provider.StreamChunk{FinishReason: "stop", Usage: &provider.Usage{InputTokens: 10, OutputTokens: 5}}
	}()
	return ch, nil
}

func (p *blockedPrebuildProvider) ID() string   { return "fake-openai" }
func (p *blockedPrebuildProvider) Name() string { return "Fake OpenAI" }
func (p *blockedPrebuildProvider) Models(_ context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "gpt-5.3-codex", Name: "Fake Codex", ContextSize: 163200}}, nil
}

func (p *blockedPrebuildProvider) Chat(_ context.Context, _ string, messages []provider.Message, _ provider.ChatOptions) (<-chan provider.StreamChunk, error) {
	p.calls++
	ch := make(chan provider.StreamChunk, 2)
	go func() {
		defer close(ch)
		switch p.calls {
		case 1:
			ch <- provider.StreamChunk{
				ToolCalls: []provider.ToolCall{
					{
						ID:        "search-1",
						Name:      "search",
						Arguments: `{"query":"rate limiting"}`,
					},
					{
						ID:        "search-2",
						Name:      "search",
						Arguments: `{"query":"rate limit middleware"}`,
					},
					{
						ID:        "search-3",
						Name:      "search",
						Arguments: `{"query":"rate limiter config"}`,
					},
				},
				FinishReason: "tool_calls",
			}
		case 2:
			if !messagesContainText(messages, `Tool "search" is blocked in the current prebuild phase. Do not edit yet. Run the test tool first.`) {
				ch <- provider.StreamChunk{Delta: "blocked-tool-result-missing", FinishReason: "stop"}
				return
			}
			ch <- provider.StreamChunk{
				ToolCalls: []provider.ToolCall{{
					ID:        "test-1",
					Name:      "test",
					Arguments: `{"command":"npm run test:unit"}`,
				}},
				FinishReason: "tool_calls",
			}
		default:
			ch <- provider.StreamChunk{Delta: "done", FinishReason: "stop"}
		}
	}()
	return ch, nil
}

func (p *streamingBlockedPrebuildProvider) ID() string   { return "fake-openai" }
func (p *streamingBlockedPrebuildProvider) Name() string { return "Fake OpenAI" }
func (p *streamingBlockedPrebuildProvider) Models(_ context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "gpt-5.3-codex", Name: "Fake Codex", ContextSize: 163200}}, nil
}

func (p *streamingBlockedPrebuildProvider) Chat(_ context.Context, _ string, messages []provider.Message, _ provider.ChatOptions) (<-chan provider.StreamChunk, error) {
	p.calls++
	ch := make(chan provider.StreamChunk, 16)
	go func() {
		defer close(ch)
		switch p.calls {
		case 1:
			ch <- provider.StreamChunk{ToolCallDelta: &provider.ToolCallDelta{Index: 0, ID: "search-1", Name: "search"}}
			ch <- provider.StreamChunk{ToolCallDelta: &provider.ToolCallDelta{Index: 0, ArgumentsDelta: `{"query":"rate limiting"}`}}
			ch <- provider.StreamChunk{ToolCallDelta: &provider.ToolCallDelta{Index: 1, ID: "search-2", Name: "search"}}
			ch <- provider.StreamChunk{ToolCallDelta: &provider.ToolCallDelta{Index: 1, ArgumentsDelta: `{"query":"rate limit middleware"}`}}
			ch <- provider.StreamChunk{ToolCallDelta: &provider.ToolCallDelta{Index: 2, ID: "search-3", Name: "search"}}
			ch <- provider.StreamChunk{ToolCallDelta: &provider.ToolCallDelta{Index: 2, ArgumentsDelta: `{"query":"rate limiter config"}`}}
			ch <- provider.StreamChunk{
				ToolCalls: []provider.ToolCall{
					{ID: "search-1", Name: "search", Arguments: `{"query":"rate limiting"}`},
					{ID: "search-2", Name: "search", Arguments: `{"query":"rate limit middleware"}`},
					{ID: "search-3", Name: "search", Arguments: `{"query":"rate limiter config"}`},
				},
				FinishReason: "tool_calls",
			}
		case 2:
			if !messagesContainText(messages, `Tool "search" is blocked in the current prebuild phase. Do not edit yet. Run the test tool first.`) {
				ch <- provider.StreamChunk{Delta: "blocked-tool-result-missing", FinishReason: "stop"}
				return
			}
			ch <- provider.StreamChunk{
				ToolCalls: []provider.ToolCall{{
					ID:        "test-1",
					Name:      "test",
					Arguments: `{"command":"npm run test:unit"}`,
				}},
				FinishReason: "tool_calls",
			}
		default:
			ch <- provider.StreamChunk{Delta: "done", FinishReason: "stop"}
		}
	}()
	return ch, nil
}

func messagesContainText(messages []provider.Message, want string) bool {
	for _, m := range messages {
		for _, p := range m.Parts {
			switch part := p.(type) {
			case provider.TextPart:
				if strings.Contains(part.Text, want) {
					return true
				}
			case provider.ToolResultPart:
				if strings.Contains(part.Output, want) {
					return true
				}
			}
		}
	}
	return false
}

func (r *turnLoopMessageRepo) Create(_ context.Context, m repository.Message) (repository.Message, error) {
	r.nextID++
	m.ID = fmt.Sprintf("m%d", r.nextID)
	r.messages = append(r.messages, m)
	return m, nil
}

func (r *turnLoopMessageRepo) CreateWithParts(ctx context.Context, m repository.Message, parts []repository.MessagePart) (repository.Message, error) {
	created, err := r.Create(ctx, m)
	if err != nil {
		return repository.Message{}, err
	}
	created.Parts = make([]repository.MessagePart, 0, len(parts))
	for _, p := range parts {
		p.MessageID = created.ID
		if p.SessionID == "" {
			p.SessionID = created.SessionID
		}
		part, err := r.CreatePart(ctx, p)
		if err != nil {
			for i := len(r.messages) - 1; i >= 0; i-- {
				if r.messages[i].ID == created.ID {
					r.messages = append(r.messages[:i], r.messages[i+1:]...)
					break
				}
			}
			return repository.Message{}, err
		}
		created.Parts = append(created.Parts, part)
	}
	for i := range r.messages {
		if r.messages[i].ID == created.ID {
			r.messages[i] = created
			break
		}
	}
	return created, nil
}

func (r *turnLoopMessageRepo) Get(_ context.Context, id string) (repository.Message, error) {
	for _, m := range r.messages {
		if m.ID == id {
			return m, nil
		}
	}
	return repository.Message{}, repository.ErrNotFound
}

func (r *turnLoopMessageRepo) ListBySession(_ context.Context, sessionID string) ([]repository.Message, error) {
	var out []repository.Message
	for _, m := range r.messages {
		if m.SessionID == sessionID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (r *turnLoopMessageRepo) DeleteBySession(_ context.Context, sessionID string) error {
	filtered := r.messages[:0]
	for _, m := range r.messages {
		if m.SessionID != sessionID {
			filtered = append(filtered, m)
		}
	}
	r.messages = filtered
	return nil
}

func (r *turnLoopMessageRepo) ListMessagesWithParts(_ context.Context, sessionID string) ([]repository.Message, error) {
	var out []repository.Message
	for _, m := range r.messages {
		if m.SessionID == sessionID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (r *turnLoopMessageRepo) CreatePart(_ context.Context, p repository.MessagePart) (repository.MessagePart, error) {
	r.nextID++
	p.ID = fmt.Sprintf("p%d", r.nextID)
	p.CreatedAt = time.Now()
	for i := range r.messages {
		if r.messages[i].ID == p.MessageID {
			r.messages[i].Parts = append(r.messages[i].Parts, p)
			return p, nil
		}
	}
	return repository.MessagePart{}, repository.ErrNotFound
}

func (r *turnLoopMessageRepo) ListPartsByMessage(_ context.Context, messageID string) ([]repository.MessagePart, error) {
	for _, m := range r.messages {
		if m.ID == messageID {
			return append([]repository.MessagePart(nil), m.Parts...), nil
		}
	}
	return nil, repository.ErrNotFound
}

func (r *turnLoopMessageRepo) ListPartsBySession(_ context.Context, sessionID string) ([]repository.MessagePart, error) {
	var out []repository.MessagePart
	for _, m := range r.messages {
		if m.SessionID == sessionID {
			out = append(out, m.Parts...)
		}
	}
	return out, nil
}

func (r *turnLoopMessageRepo) UpdatePart(_ context.Context, part repository.MessagePart) error {
	for i := range r.messages {
		for j := range r.messages[i].Parts {
			if r.messages[i].Parts[j].ID == part.ID {
				r.messages[i].Parts[j] = part
				return nil
			}
		}
	}
	return repository.ErrNotFound
}

func (r *turnLoopMessageRepo) DeletePart(_ context.Context, _ string) error { return nil }

func (r *turnLoopMessageRepo) BatchUpdateParts(_ context.Context, parts []repository.MessagePart) error {
	for _, p := range parts {
		if err := r.UpdatePart(context.Background(), p); err != nil {
			return err
		}
	}
	return nil
}

func (r *turnLoopMessageRepo) DeletePartsBySession(_ context.Context, sessionID string) error {
	for i := range r.messages {
		if r.messages[i].SessionID == sessionID {
			r.messages[i].Parts = nil
		}
	}
	return nil
}

func (p *loopStopTestProvider) ID() string   { return "fake-openai" }
func (p *loopStopTestProvider) Name() string { return "Fake OpenAI" }

func (p *loopStopTestProvider) Models(_ context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "gpt-5.3-codex", Name: "Fake Codex", ContextSize: 163200}}, nil
}

func (p *loopStopTestProvider) Chat(
	_ context.Context,
	_ string,
	_ []provider.Message,
	opts provider.ChatOptions,
) (<-chan provider.StreamChunk, error) {
	p.toolsPerCall = append(p.toolsPerCall, len(opts.Tools))
	if len(opts.SystemParts) > 2 {
		p.volatile = append(p.volatile, opts.SystemParts[2])
	} else {
		p.volatile = append(p.volatile, "")
	}

	ch := make(chan provider.StreamChunk, 1)
	if len(opts.Tools) == 0 {
		ch <- provider.StreamChunk{Delta: "Final answer", FinishReason: "stop"}
	} else {
		ch <- provider.StreamChunk{
			ToolCalls: []provider.ToolCall{{
				ID:        fmt.Sprintf("call-%d", p.calls),
				Name:      "git",
				Arguments: `{"action":"status","args":"","limit":20}`,
			}},
			FinishReason: "tool_calls",
		}
	}
	close(ch)
	p.calls++
	return ch, nil
}

func (p *stalledExecutionProvider) ID() string   { return "fake-openai" }
func (p *stalledExecutionProvider) Name() string { return "Fake OpenAI" }

func (p *stalledExecutionProvider) Models(_ context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "gpt-5.3-codex", Name: "Fake Codex", ContextSize: 163200}}, nil
}

func (p *stalledExecutionProvider) Chat(
	_ context.Context,
	_ string,
	_ []provider.Message,
	opts provider.ChatOptions,
) (<-chan provider.StreamChunk, error) {
	p.toolsPerCall = append(p.toolsPerCall, len(opts.Tools))
	if len(opts.SystemParts) > 2 {
		p.volatile = append(p.volatile, opts.SystemParts[2])
	} else {
		p.volatile = append(p.volatile, "")
	}

	ch := make(chan provider.StreamChunk, 1)
	ch <- provider.StreamChunk{
		Delta:        fmt.Sprintf("stop-%d", p.calls+1),
		FinishReason: "stop",
	}
	close(ch)
	p.calls++
	return ch, nil
}

func TestTurnLoop_DisablesToolsAfterLoopStop(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(&tool.Tool{
		Name:        "git",
		Description: "Inspect git status",
		Parameters:  []byte(`{"type":"object"}`),
		ReadOnly:    true,
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			return &tool.Result{Output: "clean working tree"}, nil
		},
	})

	prov := &loopStopTestProvider{}
	req := &pipeline.TurnRequest{
		SessionID:  "loop-stop-test",
		ProviderID: "openai",
		Messages: []provider.Message{{
			Role:  "user",
			Parts: []provider.MessagePart{provider.TextPart{Text: "Inspect the repository state."}},
		}},
		Tools: []provider.Tool{{
			Name:        "git",
			Description: "Inspect git status",
			Parameters:  []byte(`{"type":"object"}`),
		}},
		Model:     provider.Model{ID: "gpt-5.3-codex", Name: "Fake Codex", ContextSize: 163200},
		Ephemeral: true,
	}

	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      prov,
		modelID:   req.Model.ID,
		publish:   func(string, SSEEvent) {},
		sndbx:     sandbox.New(t.TempDir(), sandbox.OriginTUI, reg, permission.Config{}),
		cfg:       &config.SessionConfig{SubagentMaxSteps: exactRepeatStop + 5},
		toolCache: newToolResultCache(),
	}

	if err := tl.run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if prov.calls != exactRepeatStop+1 {
		t.Fatalf("provider call count = %d, want %d (tool calls should stop before max-step fallback)", prov.calls, exactRepeatStop+1)
	}
	if len(prov.toolsPerCall) != prov.calls {
		t.Fatalf("toolsPerCall len = %d, want %d", len(prov.toolsPerCall), prov.calls)
	}
	for i := 0; i < exactRepeatStop; i++ {
		if prov.toolsPerCall[i] == 0 {
			t.Fatalf("call %d unexpectedly had no tools before loop stop: %v", i, prov.toolsPerCall)
		}
	}
	if got := prov.toolsPerCall[len(prov.toolsPerCall)-1]; got != 0 {
		t.Fatalf("final provider call still had tools enabled: %v", prov.toolsPerCall)
	}
	if got := prov.volatile[len(prov.volatile)-1]; !strings.Contains(got, "Repeated identical tool calls detected") {
		t.Fatalf("final provider call missing loop-stop directive: %q", got)
	}
}

func TestTurnLoop_ExecutionStallAsksUserAndBlocksTask(t *testing.T) {
	taskStore := tool.NewTaskStore(nil)
	taskTool := tool.NewTaskTool(taskStore)
	if _, err := taskTool.Execute(t.Context(), tool.ExecutionContext{SessionID: "execution-stall-test"}, []byte(`{"action":"create","title":"Task 1","kind":"implementation","notes":"Do the work","status":"in_progress"}`)); err != nil {
		t.Fatalf("create task: %v", err)
	}

	prov := &stalledExecutionProvider{}
	asked := 0
	req := &pipeline.TurnRequest{
		SessionID:  "execution-stall-test",
		AgentID:    "engineer",
		ProviderID: "openai",
		Messages: []provider.Message{{
			Role:  "user",
			Parts: []provider.MessagePart{provider.TextPart{Text: "Continue implementing the approved task."}},
		}},
		Tools: []provider.Tool{
			{Name: "read"},
			{Name: "task"},
		},
		Model: provider.Model{ID: "gpt-5.3-codex", Name: "Fake Codex", ContextSize: 163200},
		Workflow: &pipeline.WorkflowState{
			Plan: pipeline.WorkflowPlanState{EffectiveStatus: pipeline.WorkflowApprovalApproved},
		},
	}

	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      prov,
		modelID:   req.Model.ID,
		publish:   func(string, SSEEvent) {},
		sndbx:     sandbox.New(t.TempDir(), sandbox.OriginTUI, tool.NewRegistry(), permission.Config{}),
		msgs:      &turnLoopMessageRepo{},
		taskStore: taskStore,
		toolCache: newToolResultCache(),
		askUser: func(_ context.Context, _ string) func(string, []string, bool, string) (string, error) {
			return func(question string, options []string, multiple bool, purpose string) (string, error) {
				asked++
				if question != executionStallQuestionText {
					t.Fatalf("question = %q, want %q", question, executionStallQuestionText)
				}
				if purpose != executionStallPurpose {
					t.Fatalf("purpose = %q, want %q", purpose, executionStallPurpose)
				}
				if multiple {
					t.Fatal("multiple = true, want false")
				}
				wantOptions := []string{
					executionStallContinueOption,
					executionStallBlockOption,
					executionStallStopOption,
				}
				if fmt.Sprint(options) != fmt.Sprint(wantOptions) {
					t.Fatalf("options = %#v, want %#v", options, wantOptions)
				}
				return executionStallBlockOption, nil
			}
		},
	}

	if err := tl.run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if asked != 1 {
		t.Fatalf("asked = %d, want 1", asked)
	}
	if prov.calls != 5 {
		t.Fatalf("provider calls = %d, want %d", prov.calls, 5)
	}
	tasks := taskStore.GetTasks(req.SessionID)
	if len(tasks) != 1 || tasks[0].Status != "blocked" {
		t.Fatalf("tasks = %+v, want one blocked task", tasks)
	}
	volatile := strings.Join(prov.volatile, "\n")
	if !strings.Contains(volatile, "Task execution is not complete") {
		t.Fatalf("volatile directives = %#v, want execution retry prompt", prov.volatile)
	}
	if !strings.Contains(volatile, "The user chose to block") {
		t.Fatalf("volatile directives = %#v, want block-stop prompt", prov.volatile)
	}
}

func TestTurnLoop_ExecutionStallUsesConfiguredMaxRetries(t *testing.T) {
	taskStore := tool.NewTaskStore(nil)
	taskTool := tool.NewTaskTool(taskStore)
	if _, err := taskTool.Execute(t.Context(), tool.ExecutionContext{SessionID: "execution-stall-configured-retries-test"}, []byte(`{"action":"create","title":"Task 1","kind":"implementation","notes":"Do the work","status":"in_progress"}`)); err != nil {
		t.Fatalf("create task: %v", err)
	}

	prov := &stalledExecutionProvider{}
	asked := 0
	req := &pipeline.TurnRequest{
		SessionID:  "execution-stall-configured-retries-test",
		AgentID:    "engineer",
		ProviderID: "openai",
		Messages: []provider.Message{{
			Role:  "user",
			Parts: []provider.MessagePart{provider.TextPart{Text: "Continue implementing the approved task."}},
		}},
		Tools: []provider.Tool{
			{Name: "read"},
			{Name: "task"},
		},
		Model: provider.Model{ID: "gpt-5.3-codex", Name: "Fake Codex", ContextSize: 163200},
		Workflow: &pipeline.WorkflowState{
			Plan: pipeline.WorkflowPlanState{EffectiveStatus: pipeline.WorkflowApprovalApproved},
		},
	}

	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      prov,
		modelID:   req.Model.ID,
		publish:   func(string, SSEEvent) {},
		sndbx:     sandbox.New(t.TempDir(), sandbox.OriginTUI, tool.NewRegistry(), permission.Config{}),
		msgs:      &turnLoopMessageRepo{},
		taskStore: taskStore,
		toolCache: newToolResultCache(),
		cfg:       &config.SessionConfig{EngineerReviewLimit: 1},
		askUser: func(_ context.Context, _ string) func(string, []string, bool, string) (string, error) {
			return func(string, []string, bool, string) (string, error) {
				asked++
				return executionStallBlockOption, nil
			}
		},
	}

	if err := tl.run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if asked != 1 {
		t.Fatalf("asked = %d, want 1", asked)
	}
	if prov.calls != 3 {
		t.Fatalf("provider calls = %d, want 3", prov.calls)
	}
}

func TestTurnLoop_ExecutionStallDoesNotReaskAfterKeepWorkingWithoutProgress(t *testing.T) {
	taskStore := tool.NewTaskStore(nil)
	taskTool := tool.NewTaskTool(taskStore)
	if _, err := taskTool.Execute(t.Context(), tool.ExecutionContext{SessionID: "execution-stall-continue-test"}, []byte(`{"action":"create","title":"Task 1","kind":"implementation","notes":"Do the work","status":"in_progress"}`)); err != nil {
		t.Fatalf("create task: %v", err)
	}

	prov := &stalledExecutionProvider{}
	asked := 0
	req := &pipeline.TurnRequest{
		SessionID:  "execution-stall-continue-test",
		AgentID:    "engineer",
		ProviderID: "openai",
		Messages: []provider.Message{{
			Role:  "user",
			Parts: []provider.MessagePart{provider.TextPart{Text: "Continue implementing the approved task."}},
		}},
		Tools: []provider.Tool{
			{Name: "read"},
			{Name: "task"},
		},
		Model: provider.Model{ID: "gpt-5.3-codex", Name: "Fake Codex", ContextSize: 163200},
		Workflow: &pipeline.WorkflowState{
			Plan: pipeline.WorkflowPlanState{EffectiveStatus: pipeline.WorkflowApprovalApproved},
		},
	}

	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      prov,
		modelID:   req.Model.ID,
		publish:   func(string, SSEEvent) {},
		sndbx:     sandbox.New(t.TempDir(), sandbox.OriginTUI, tool.NewRegistry(), permission.Config{}),
		msgs:      &turnLoopMessageRepo{},
		taskStore: taskStore,
		toolCache: newToolResultCache(),
		askUser: func(_ context.Context, _ string) func(string, []string, bool, string) (string, error) {
			return func(string, []string, bool, string) (string, error) {
				asked++
				return executionStallContinueOption, nil
			}
		},
	}

	if err := tl.run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if asked != 1 {
		t.Fatalf("asked = %d, want 1", asked)
	}
	if prov.calls != 9 {
		t.Fatalf("provider calls = %d, want 9", prov.calls)
	}
	tasks := taskStore.GetTasks(req.SessionID)
	if len(tasks) != 1 || tasks[0].Status != "blocked" {
		t.Fatalf("tasks = %+v, want one blocked task", tasks)
	}
	volatile := strings.Join(prov.volatile, "\n")
	if !strings.Contains(volatile, "after the user chose to keep working") {
		t.Fatalf("volatile directives = %#v, want auto-block directive after continue", prov.volatile)
	}
}

func TestTurnLoop_ContextPressureDoesNotTriggerExecutionStallQuestion(t *testing.T) {
	taskStore := tool.NewTaskStore(nil)
	taskTool := tool.NewTaskTool(taskStore)
	if _, err := taskTool.Execute(t.Context(), tool.ExecutionContext{SessionID: "execution-stall-context-pressure-test"}, []byte(`{"action":"create","title":"Task 1","kind":"implementation","notes":"Do the work","status":"in_progress"}`)); err != nil {
		t.Fatalf("create task: %v", err)
	}

	prov := &stalledExecutionProvider{}
	asked := 0
	req := &pipeline.TurnRequest{
		SessionID:  "execution-stall-context-pressure-test",
		AgentID:    "engineer",
		ProviderID: "openai",
		Step:       1,
		Messages: []provider.Message{{
			Role:  "user",
			Parts: []provider.MessagePart{provider.TextPart{Text: "Continue implementing the approved task."}},
		}},
		Tools: []provider.Tool{
			{Name: "read"},
			{Name: "task"},
			{Name: "test"},
		},
		Model: provider.Model{ID: "gpt-5.3-codex", Name: "Fake Codex", ContextSize: 64000},
		Usage: &provider.Usage{
			InputTokens: 42000,
		},
		Workflow: &pipeline.WorkflowState{
			Plan: pipeline.WorkflowPlanState{EffectiveStatus: pipeline.WorkflowApprovalApproved},
		},
	}

	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      prov,
		modelID:   req.Model.ID,
		publish:   func(string, SSEEvent) {},
		sndbx:     sandbox.New(t.TempDir(), sandbox.OriginTUI, tool.NewRegistry(), permission.Config{}),
		msgs:      &turnLoopMessageRepo{},
		taskStore: taskStore,
		toolCache: newToolResultCache(),
		cfg:       &config.SessionConfig{},
		askUser: func(_ context.Context, _ string) func(string, []string, bool, string) (string, error) {
			return func(string, []string, bool, string) (string, error) {
				asked++
				return executionStallBlockOption, nil
			}
		},
	}

	if err := tl.run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if asked != 0 {
		t.Fatalf("asked = %d, want 0", asked)
	}
	if prov.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", prov.calls)
	}
	if len(prov.toolsPerCall) != 1 || prov.toolsPerCall[0] != 0 {
		t.Fatalf("toolsPerCall = %#v, want [0]", prov.toolsPerCall)
	}
	volatile := strings.Join(prov.volatile, "\n")
	if !strings.Contains(volatile, "The context window is nearly full") {
		t.Fatalf("volatile directives = %#v, want context-pressure directive", prov.volatile)
	}
	if strings.Contains(volatile, "Task execution is not complete") {
		t.Fatalf("volatile directives = %#v, did not want execution-stall directive", prov.volatile)
	}
	tasks := taskStore.GetTasks(req.SessionID)
	if len(tasks) != 1 || tasks[0].Status != "in_progress" {
		t.Fatalf("tasks = %+v, want one in_progress task", tasks)
	}
}

func TestTurnLoop_PrebuildBlockedToolResultStaysInHistory(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(&tool.Tool{
		Name:     "test",
		ReadOnly: true,
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			return &tool.Result{Output: "FAIL", Metadata: map[string]any{"exit_code": 1}}, nil
		},
	})

	prov := &blockedPrebuildProvider{}
	req := &pipeline.TurnRequest{
		SessionID:         "prebuild-blocked-history-test",
		AgentID:           "engineer",
		ProviderID:        "openai",
		PhaseFilterActive: true,
		Tools: []provider.Tool{
			{Name: "test"},
			{Name: "skill"},
		},
		FullTools: []provider.Tool{
			{Name: "test"},
			{Name: "skill"},
			{Name: "search"},
		},
		Messages: []provider.Message{{
			Role:  "user",
			Parts: []provider.MessagePart{provider.TextPart{Text: "continue"}},
		}},
		Model: provider.Model{ID: "gpt-5.3-codex", Name: "Fake Codex", ContextSize: 163200},
		Workflow: &pipeline.WorkflowState{
			Phase: pipeline.WorkflowPhasePrebuild,
		},
	}

	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      prov,
		modelID:   req.Model.ID,
		publish:   func(string, SSEEvent) {},
		sndbx:     sandbox.New(t.TempDir(), sandbox.OriginTUI, reg, permission.Config{}),
		msgs:      &turnLoopMessageRepo{},
		toolCache: newToolResultCache(),
		cfg:       &config.SessionConfig{},
	}

	if err := tl.run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if prov.calls != 3 {
		t.Fatalf("provider calls = %d, want 3", prov.calls)
	}
	if !messagesContainText(req.Messages, `Tool "search" is blocked in the current prebuild phase. Do not edit yet. Run the test tool first.`) {
		t.Fatalf("messages did not retain blocked tool result: %#v", req.Messages)
	}
	searchCalls := 0
	searchResults := 0
	for _, msg := range req.Messages {
		for _, part := range msg.Parts {
			switch p := part.(type) {
			case provider.ToolCallPart:
				if p.Name == "search" {
					searchCalls++
				}
			case provider.ToolResultPart:
				if p.ToolCallID == "search-1" || p.ToolCallID == "search-2" || p.ToolCallID == "search-3" {
					searchResults++
				}
			}
		}
	}
	if searchCalls != 1 {
		t.Fatalf("search tool calls retained = %d, want 1", searchCalls)
	}
	if searchResults != 1 {
		t.Fatalf("search tool results retained = %d, want 1", searchResults)
	}
	if ensureWorkflowState(req).Phase != pipeline.WorkflowPhasePreplan {
		t.Fatalf("phase = %q, want preplan after test attempt", ensureWorkflowState(req).Phase)
	}
}

func TestTurnLoop_PrebuildBlockedStreamingToolCallsDoNotPublishGhostRunningTools(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(&tool.Tool{
		Name:     "test",
		ReadOnly: true,
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			return &tool.Result{Output: "FAIL", Metadata: map[string]any{"exit_code": 1}}, nil
		},
	})

	prov := &streamingBlockedPrebuildProvider{}
	req := &pipeline.TurnRequest{
		SessionID:         "prebuild-blocked-streaming-test",
		AgentID:           "engineer",
		ProviderID:        "openai",
		PhaseFilterActive: true,
		Tools: []provider.Tool{
			{Name: "test"},
			{Name: "skill"},
		},
		FullTools: []provider.Tool{
			{Name: "test"},
			{Name: "skill"},
			{Name: "search"},
		},
		Messages: []provider.Message{{
			Role:  "user",
			Parts: []provider.MessagePart{provider.TextPart{Text: "continue"}},
		}},
		Model: provider.Model{ID: "gpt-5.3-codex", Name: "Fake Codex", ContextSize: 163200},
		Workflow: &pipeline.WorkflowState{
			Phase: pipeline.WorkflowPhasePrebuild,
		},
	}

	var toolStarts, toolEnds int
	tl := &turnLoop{
		ctx:     context.Background(),
		req:     req,
		prov:    prov,
		modelID: req.Model.ID,
		publish: func(_ string, ev SSEEvent) {
			switch ev.Type {
			case "tool_start":
				if data, ok := ev.Data.(SSEToolStartData); ok && data.Tool == "search" {
					toolStarts++
				}
			case "tool_end":
				if data, ok := ev.Data.(SSEToolEndData); ok && data.Tool == "search" {
					toolEnds++
				}
			}
		},
		sndbx:     sandbox.New(t.TempDir(), sandbox.OriginTUI, reg, permission.Config{}),
		msgs:      &turnLoopMessageRepo{},
		toolCache: newToolResultCache(),
		cfg:       &config.SessionConfig{},
	}

	if err := tl.run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if toolStarts != 0 {
		t.Fatalf("search tool_start events = %d, want 0 for filtered blocked streaming calls", toolStarts)
	}
	if toolEnds != 1 {
		t.Fatalf("search tool_end events = %d, want 1", toolEnds)
	}
}

func TestToolCallArgumentTimeout_UsesSessionAndModelOverride(t *testing.T) {
	tl := &turnLoop{
		cfg: &config.SessionConfig{
			ToolCallArgumentTimeout: 240,
			Models: map[string]config.ModelSessionConfig{
				"ollama/qwen2.5-coder": {ToolCallArgumentTimeout: 900},
			},
		},
	}

	if got := tl.toolCallArgumentTimeout("openai", "gpt-4.1"); got != 4*time.Minute {
		t.Fatalf("session timeout = %v, want 4m", got)
	}
	if got := tl.toolCallArgumentTimeout("ollama", "qwen2.5-coder"); got != 15*time.Minute {
		t.Fatalf("model override timeout = %v, want 15m", got)
	}
}

func TestTurnLoopPublishUsageUpdateWarnsOnceForCrossSessionBudget(t *testing.T) {
	var events []SSEEvent
	tl := &turnLoop{
		ctx: context.Background(),
		req: &pipeline.TurnRequest{
			SessionID: "s1",
			Usage:     &provider.Usage{InputTokens: 10},
			Model:     provider.Model{ID: "m1", ContextSize: 4096},
		},
		publish: func(_ string, ev SSEEvent) {
			events = append(events, ev)
		},
		cfg: &config.SessionConfig{
			TotalBudget:     10,
			TotalBudgetWarn: 0.8,
		},
		sc: NewSessionCost(),
		budgetStatus: func(context.Context, string, *config.SessionConfig) BudgetStatus {
			return BudgetStatus{
				SessionCost: 1.5,
				TotalCost:   8.5,
				TotalWarn:   true,
			}
		},
	}

	if exceeded := tl.publishUsageUpdate("m1"); exceeded {
		t.Fatal("publishUsageUpdate() = true, want false for warning-only state")
	}
	if exceeded := tl.publishUsageUpdate("m1"); exceeded {
		t.Fatal("second publishUsageUpdate() = true, want false for warning-only state")
	}

	var warnings int
	var usageBudgetWarn bool
	for _, ev := range events {
		switch ev.Type {
		case "warning":
			warnings++
		case "usage":
			data := ev.Data.(SSEDoneData)
			usageBudgetWarn = usageBudgetWarn || data.BudgetWarn
		}
	}
	if warnings != 1 {
		t.Fatalf("warning events = %d, want 1", warnings)
	}
	if !usageBudgetWarn {
		t.Fatal("usage BudgetWarn = false, want true")
	}
}

func TestTurnLoopPublishUsageUpdateStopsOnCrossSessionBudgetExceeded(t *testing.T) {
	var warningMessage string
	tl := &turnLoop{
		ctx: context.Background(),
		req: &pipeline.TurnRequest{
			SessionID: "s1",
			Usage:     &provider.Usage{InputTokens: 10},
			Model:     provider.Model{ID: "m1", ContextSize: 4096},
		},
		publish: func(_ string, ev SSEEvent) {
			if ev.Type != "warning" {
				return
			}
			if data, ok := ev.Data.(map[string]string); ok {
				warningMessage = data["message"]
			}
		},
		cfg: &config.SessionConfig{
			TotalBudget:     10,
			TotalBudgetWarn: 0.8,
		},
		sc: NewSessionCost(),
		budgetStatus: func(context.Context, string, *config.SessionConfig) BudgetStatus {
			return BudgetStatus{
				SessionCost:   2.0,
				TotalCost:     10.5,
				TotalWarn:     true,
				TotalExceeded: true,
			}
		},
	}

	if exceeded := tl.publishUsageUpdate("m1"); !exceeded {
		t.Fatal("publishUsageUpdate() = false, want true when cross-session budget is exceeded")
	}
	if !strings.Contains(warningMessage, "Cross-session budget reached") {
		t.Fatalf("warning message = %q, want cross-session budget warning", warningMessage)
	}
}

func TestTurnLoop_DisablesToolsAfterLoopStopForNonOpenAIProvider(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(&tool.Tool{
		Name:        "git",
		Description: "Inspect git status",
		Parameters:  []byte(`{"type":"object"}`),
		ReadOnly:    true,
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			return &tool.Result{Output: "clean working tree"}, nil
		},
	})

	prov := &loopStopTestProvider{}
	req := &pipeline.TurnRequest{
		SessionID:  "loop-stop-google",
		ProviderID: "google",
		Messages: []provider.Message{{
			Role:  "user",
			Parts: []provider.MessagePart{provider.TextPart{Text: "Inspect the repository state."}},
		}},
		Tools: []provider.Tool{{
			Name:        "git",
			Description: "Inspect git status",
			Parameters:  []byte(`{"type":"object"}`),
		}},
		Model:     provider.Model{ID: "gemini-2.5-pro", Name: "Fake Gemini", ContextSize: 163200},
		Ephemeral: true,
	}

	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      prov,
		modelID:   req.Model.ID,
		publish:   func(string, SSEEvent) {},
		sndbx:     sandbox.New(t.TempDir(), sandbox.OriginTUI, reg, permission.Config{}),
		cfg:       &config.SessionConfig{SubagentMaxSteps: exactRepeatStop + 5},
		toolCache: newToolResultCache(),
	}

	if err := tl.run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if got := prov.toolsPerCall[len(prov.toolsPerCall)-1]; got != 0 {
		t.Fatalf("final provider call still had tools enabled for non-openai provider: %v", prov.toolsPerCall)
	}
}

type primaryMaxStepProvider struct {
	calls int
}

func (p *primaryMaxStepProvider) ID() string   { return "fake-primary" }
func (p *primaryMaxStepProvider) Name() string { return "Fake Primary" }
func (p *primaryMaxStepProvider) Models(_ context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "gpt-5.3-codex", Name: "Fake Codex", ContextSize: 163200}}, nil
}

func (p *primaryMaxStepProvider) Chat(
	_ context.Context,
	_ string,
	_ []provider.Message,
	opts provider.ChatOptions,
) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk, 1)
	if len(opts.Tools) == 0 {
		ch <- provider.StreamChunk{Delta: "final", FinishReason: "stop"}
	} else {
		ch <- provider.StreamChunk{
			ToolCalls: []provider.ToolCall{{
				ID:        fmt.Sprintf("call-%d", p.calls),
				Name:      "git",
				Arguments: fmt.Sprintf(`{"action":"show","args":"step-%d","limit":20}`, p.calls),
			}},
			FinishReason: "tool_calls",
		}
	}
	close(ch)
	p.calls++
	return ch, nil
}

func TestTurnLoop_AppliesPrimaryMaxToolSteps(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(&tool.Tool{
		Name:        "git",
		Description: "Inspect git status",
		Parameters:  []byte(`{"type":"object"}`),
		ReadOnly:    true,
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			return &tool.Result{Output: "clean working tree"}, nil
		},
	})

	prov := &primaryMaxStepProvider{}
	req := &pipeline.TurnRequest{
		SessionID:  "loop-cap-primary",
		ProviderID: "openai",
		Messages: []provider.Message{{
			Role:  "user",
			Parts: []provider.MessagePart{provider.TextPart{Text: "keep checking"}},
		}},
		Tools: []provider.Tool{{
			Name:        "git",
			Description: "Inspect git status",
			Parameters:  []byte(`{"type":"object"}`),
		}},
		Model: provider.Model{ID: "gpt-5.3-codex", Name: "Fake Codex", ContextSize: 163200},
	}

	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      prov,
		modelID:   req.Model.ID,
		publish:   func(string, SSEEvent) {},
		sndbx:     sandbox.New(t.TempDir(), sandbox.OriginTUI, reg, permission.Config{}),
		cfg:       &config.SessionConfig{},
		toolCache: newToolResultCache(),
	}

	if err := tl.run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if got, want := prov.calls, defaultPrimaryMaxToolLoopSteps+1; got != want {
		t.Fatalf("provider calls = %d, want %d", got, want)
	}
}

type verifyNudgeTestProvider struct {
	calls         int
	thirdVolatile string
	thirdLastRole string
}

func (p *verifyNudgeTestProvider) ID() string   { return "fake" }
func (p *verifyNudgeTestProvider) Name() string { return "Fake" }
func (p *verifyNudgeTestProvider) Models(_ context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "m1", Name: "M1", ContextSize: 128000}}, nil
}

func (p *verifyNudgeTestProvider) Chat(
	_ context.Context,
	_ string,
	msgs []provider.Message,
	opts provider.ChatOptions,
) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk, 1)
	defer close(ch)
	p.calls++
	if p.calls == 3 && len(opts.SystemParts) > 2 {
		p.thirdVolatile = opts.SystemParts[2]
		if len(msgs) > 0 {
			p.thirdLastRole = msgs[len(msgs)-1].Role
		}
	}

	switch p.calls {
	case 1:
		// Step 1: agent calls write tool
		ch <- provider.StreamChunk{
			ToolCalls: []provider.ToolCall{{
				ID: "w1", Name: "write",
				Arguments: `{"file_path":"/tmp/test.go","content":"package main"}`,
			}},
			FinishReason: "tool_calls",
		}
	case 2:
		// Step 2: agent tries to finish without verifying
		ch <- provider.StreamChunk{Delta: "Done!", FinishReason: "stop"}
	case 3:
		// Step 3: after nudge, agent runs test
		ch <- provider.StreamChunk{
			ToolCalls: []provider.ToolCall{{
				ID: "t1", Name: "test",
				Arguments: `{"command":"go test ./..."}`,
			}},
			FinishReason: "tool_calls",
		}
	default:
		// Step 4: agent finishes for real
		ch <- provider.StreamChunk{Delta: "All tests pass.", FinishReason: "stop"}
	}
	return ch, nil
}

func TestTurnLoop_VerificationNudge(t *testing.T) {
	dir := t.TempDir()
	reg := tool.NewRegistry()
	reg.Register(&tool.Tool{
		Name:     "write",
		ReadOnly: false,
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			return &tool.Result{Output: "wrote file"}, nil
		},
	})
	reg.Register(&tool.Tool{
		Name:     "test",
		ReadOnly: true,
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			return &tool.Result{Output: "PASS", Metadata: map[string]any{"exit_code": 0}}, nil
		},
	})

	prov := &verifyNudgeTestProvider{}
	req := &pipeline.TurnRequest{
		SessionID:  "verify-nudge-test",
		ProviderID: "fake",
		Messages: []provider.Message{{
			Role:  "user",
			Parts: []provider.MessagePart{provider.TextPart{Text: "Write some code"}},
		}},
		Tools: []provider.Tool{
			{Name: "write", Parameters: []byte(`{"type":"object"}`)},
			{Name: "test", Parameters: []byte(`{"type":"object"}`)},
		},
		Model: provider.Model{ID: "m1", Name: "M1", ContextSize: 128000},
	}

	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      prov,
		modelID:   "m1",
		publish:   func(string, SSEEvent) {},
		sndbx:     sandbox.New(dir, sandbox.OriginTUI, reg, permission.Config{}),
		msgs:      &turnLoopMessageRepo{},
		cfg:       &config.SessionConfig{},
		toolCache: newToolResultCache(),
	}

	if err := tl.run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	// Provider should be called 4 times: write → stop(nudged) → test → stop(accepted)
	if prov.calls != 4 {
		t.Fatalf("provider calls = %d, want 4", prov.calls)
	}

	if !strings.Contains(prov.thirdVolatile, "You modified files but have not used the test tool") {
		t.Fatalf("third provider call missing verification directive: %q", prov.thirdVolatile)
	}
	if prov.thirdLastRole != "user" {
		t.Fatalf("third provider call last role = %q, want user to avoid assistant-prefill retries", prov.thirdLastRole)
	}
}

func TestTurnLoop_VerificationNudge_SkippedForEphemeral(t *testing.T) {
	dir := t.TempDir()
	reg := tool.NewRegistry()
	reg.Register(&tool.Tool{
		Name:     "write",
		ReadOnly: false,
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			return &tool.Result{Output: "wrote file"}, nil
		},
	})
	reg.Register(&tool.Tool{
		Name:     "test",
		ReadOnly: true,
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			return &tool.Result{Output: "PASS", Metadata: map[string]any{"exit_code": 0}}, nil
		},
	})

	calls := 0
	prov := &fakeVerifyEphemeralProvider{calls: &calls}
	req := &pipeline.TurnRequest{
		SessionID:  "verify-ephemeral-test",
		ProviderID: "fake",
		Ephemeral:  true,
		Messages: []provider.Message{{
			Role:  "user",
			Parts: []provider.MessagePart{provider.TextPart{Text: "Write some code"}},
		}},
		Tools: []provider.Tool{
			{Name: "write", Parameters: []byte(`{"type":"object"}`)},
			{Name: "test", Parameters: []byte(`{"type":"object"}`)},
		},
		Model: provider.Model{ID: "m1", Name: "M1", ContextSize: 128000},
	}

	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      prov,
		modelID:   "m1",
		publish:   func(string, SSEEvent) {},
		sndbx:     sandbox.New(dir, sandbox.OriginTUI, reg, permission.Config{}),
		msgs:      &turnLoopMessageRepo{},
		cfg:       &config.SessionConfig{SubagentMaxSteps: 30},
		toolCache: newToolResultCache(),
	}

	if err := tl.run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	// Ephemeral: write → stop (no nudge). Only 2 calls.
	if *prov.calls != 2 {
		t.Fatalf("provider calls = %d, want 2 (no nudge for ephemeral)", *prov.calls)
	}
}

func TestTurnLoop_VerificationNudge_SkippedAfterVerificationBash(t *testing.T) {
	dir := t.TempDir()
	reg := tool.NewRegistry()
	reg.Register(&tool.Tool{
		Name:     "write",
		ReadOnly: false,
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			return &tool.Result{Output: "wrote file"}, nil
		},
	})
	reg.Register(tool.NewBashTool())
	reg.Register(&tool.Tool{
		Name:     "test",
		ReadOnly: true,
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			return &tool.Result{Output: "PASS", Metadata: map[string]any{"exit_code": 0}}, nil
		},
	})

	calls := 0
	prov := &fakeVerifyViaBashProvider{calls: &calls}
	req := &pipeline.TurnRequest{
		SessionID:  "verify-bash-test",
		ProviderID: "fake",
		Messages: []provider.Message{{
			Role:  "user",
			Parts: []provider.MessagePart{provider.TextPart{Text: "Write some code and verify it"}},
		}},
		Tools: []provider.Tool{
			{Name: "write", Parameters: []byte(`{"type":"object"}`)},
			{Name: "bash", Parameters: []byte(`{"type":"object"}`)},
			{Name: "test", Parameters: []byte(`{"type":"object"}`)},
		},
		Model: provider.Model{ID: "m1", Name: "M1", ContextSize: 128000},
	}

	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      prov,
		modelID:   "m1",
		publish:   func(string, SSEEvent) {},
		sndbx:     sandbox.New(dir, sandbox.OriginTUI, reg, permission.Config{}),
		msgs:      &turnLoopMessageRepo{},
		cfg:       &config.SessionConfig{},
		toolCache: newToolResultCache(),
	}

	if err := tl.run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if *prov.calls != 3 {
		t.Fatalf("provider calls = %d, want 3", *prov.calls)
	}

	if strings.Contains(prov.thirdVolatile, "You modified files but have not used the test tool") {
		t.Fatal("verification nudge should not be injected after successful bash verification")
	}
}

type fakeVerifyEphemeralProvider struct {
	calls *int
}

func (p *fakeVerifyEphemeralProvider) ID() string   { return "fake" }
func (p *fakeVerifyEphemeralProvider) Name() string { return "Fake" }
func (p *fakeVerifyEphemeralProvider) Models(_ context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "m1", Name: "M1", ContextSize: 128000}}, nil
}

func (p *fakeVerifyEphemeralProvider) Chat(
	_ context.Context, _ string, _ []provider.Message, _ provider.ChatOptions,
) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk, 1)
	defer close(ch)
	*p.calls++
	if *p.calls == 1 {
		ch <- provider.StreamChunk{
			ToolCalls: []provider.ToolCall{{
				ID: "w1", Name: "write",
				Arguments: `{"file_path":"/tmp/test.go","content":"package main"}`,
			}},
			FinishReason: "tool_calls",
		}
	} else {
		ch <- provider.StreamChunk{Delta: "Done!", FinishReason: "stop"}
	}
	return ch, nil
}

type fakeVerifyViaBashProvider struct {
	calls         *int
	thirdVolatile string
}

func (p *fakeVerifyViaBashProvider) ID() string   { return "fake" }
func (p *fakeVerifyViaBashProvider) Name() string { return "Fake" }
func (p *fakeVerifyViaBashProvider) Models(_ context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "m1", Name: "M1", ContextSize: 128000}}, nil
}

func (p *fakeVerifyViaBashProvider) Chat(
	_ context.Context, _ string, _ []provider.Message, opts provider.ChatOptions,
) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk, 1)
	defer close(ch)
	*p.calls++
	if *p.calls == 3 && len(opts.SystemParts) > 2 {
		p.thirdVolatile = opts.SystemParts[2]
	}
	switch *p.calls {
	case 1:
		ch <- provider.StreamChunk{
			ToolCalls: []provider.ToolCall{{
				ID: "w1", Name: "write",
				Arguments: `{"file_path":"/tmp/test.go","content":"package main"}`,
			}},
			FinishReason: "tool_calls",
		}
	case 2:
		ch <- provider.StreamChunk{
			ToolCalls: []provider.ToolCall{{
				ID:        "b1",
				Name:      "bash",
				Arguments: `{"command":"echo PASS","description":"Run verification","purpose":"verification"}`,
			}},
			FinishReason: "tool_calls",
		}
	default:
		ch <- provider.StreamChunk{Delta: "Done!", FinishReason: "stop"}
	}
	return ch, nil
}

type failedPatchNarrationProvider struct {
	calls *int
}

func (p *failedPatchNarrationProvider) ID() string   { return "fake" }
func (p *failedPatchNarrationProvider) Name() string { return "Fake" }
func (p *failedPatchNarrationProvider) Models(_ context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "m1", Name: "M1", ContextSize: 128000}}, nil
}

func (p *failedPatchNarrationProvider) Chat(
	_ context.Context, _ string, _ []provider.Message, _ provider.ChatOptions,
) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk, 2)
	defer close(ch)
	*p.calls++
	switch *p.calls {
	case 1:
		ch <- provider.StreamChunk{Delta: "The patch patches were applied successfully."}
		ch <- provider.StreamChunk{
			ToolCalls: []provider.ToolCall{{
				ID:        "p1",
				Name:      "patch",
				Arguments: `{"filePath":"src/routes/users.ts","edits":[{"oldString":"missing","newString":"replacement"}]}`,
			}},
			FinishReason: "tool_calls",
		}
	default:
		ch <- provider.StreamChunk{Delta: "I will retry with the actual file contents.", FinishReason: "stop"}
	}
	return ch, nil
}

func TestTurnLoop_SuppressesOptimisticNarrationWhenWriteBatchFails(t *testing.T) {
	dir := t.TempDir()
	reg := tool.NewRegistry()
	reg.Register(&tool.Tool{
		Name:       "patch",
		Parameters: []byte(`{"type":"object"}`),
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			return nil, fmt.Errorf("edit 0: oldString not found in content")
		},
	})

	calls := 0
	var events []SSEEvent
	req := &pipeline.TurnRequest{
		SessionID:  "failed-patch-test",
		ProviderID: "fake",
		Messages: []provider.Message{{
			Role:  "user",
			Parts: []provider.MessagePart{provider.TextPart{Text: "Apply the patch."}},
		}},
		Tools: []provider.Tool{{
			Name:       "patch",
			Parameters: []byte(`{"type":"object"}`),
		}},
		Model:     provider.Model{ID: "m1", Name: "M1", ContextSize: 128000},
		Ephemeral: true,
	}

	tl := &turnLoop{
		ctx:     context.Background(),
		req:     req,
		prov:    &failedPatchNarrationProvider{calls: &calls},
		modelID: "m1",
		publish: func(_ string, ev SSEEvent) { events = append(events, ev) },
		sndbx:   sandbox.New(dir, sandbox.OriginTUI, reg, permission.Config{}),
		cfg:     &config.SessionConfig{},
	}

	if err := tl.run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	for _, m := range req.Messages {
		for _, p := range m.Parts {
			tp, ok := p.(provider.TextPart)
			if !ok {
				continue
			}
			if strings.Contains(tp.Text, "patches were applied successfully") {
				t.Fatalf("optimistic pre-tool narration should not be kept in message history: %q", tp.Text)
			}
		}
	}
}

func TestShouldNudgeVerification(t *testing.T) {
	tl := &turnLoop{
		req: &pipeline.TurnRequest{
			Tools: []provider.Tool{
				{Name: "write"},
				{Name: "test"},
			},
		},
	}

	// Should nudge: files modified, no verification, not sent yet
	if !tl.shouldNudgeVerification(true, false, false) {
		t.Error("expected nudge when files modified without verification")
	}

	// Should not nudge: already sent
	if tl.shouldNudgeVerification(true, false, true) {
		t.Error("should not nudge when already sent")
	}

	// Should not nudge: already verified
	if tl.shouldNudgeVerification(true, true, false) {
		t.Error("should not nudge when already verified")
	}

	// Should not nudge: no files modified
	if tl.shouldNudgeVerification(false, false, false) {
		t.Error("should not nudge when no files modified")
	}

	// Should not nudge: ephemeral
	tl.req.Ephemeral = true
	if tl.shouldNudgeVerification(true, false, false) {
		t.Error("should not nudge for ephemeral sessions")
	}

	// Should not nudge: no test tool available (bash alone doesn't count)
	tl.req.Ephemeral = false
	tl.req.Tools = []provider.Tool{{Name: "write"}, {Name: "bash"}}
	if tl.shouldNudgeVerification(true, false, false) {
		t.Error("should not nudge when agent only has bash, not test")
	}
}

func TestFailedFileMutationRetry(t *testing.T) {
	tl := &turnLoop{
		req: &pipeline.TurnRequest{
			SessionID: "file-mutation-retry-test",
			AgentID:   "engineer",
		},
	}

	got := tl.failedFileMutationRetry(false, []toolExecution{{
		call: provider.ToolCall{
			Name:      "patch",
			Arguments: `{"filePath":"/tmp/example.ts","edits":[{"oldString":"a","newString":"b"}]}`,
		},
		errStr: ptr("tool error [patch]: edit 0: oldString not found in content"),
	}})
	if !strings.Contains(got, "file mutation tools failed (patch)") {
		t.Fatalf("retry directive = %q, want patch failure guidance", got)
	}
	if !strings.Contains(got, "/tmp/example.ts") {
		t.Fatalf("retry directive = %q, want affected file path", got)
	}
	if !strings.Contains(got, "reread the affected file") {
		t.Fatalf("retry directive = %q, want reread guidance", got)
	}

	if got := tl.failedFileMutationRetry(true, []toolExecution{{
		call:   provider.ToolCall{Name: "patch"},
		errStr: ptr("failed"),
	}}); got != "" {
		t.Fatalf("retry directive with retrySent = %q, want empty", got)
	}
}

type fakePrebuildRetryProvider struct {
	calls          int
	secondVolatile string
}

func (p *fakePrebuildRetryProvider) ID() string   { return "fake" }
func (p *fakePrebuildRetryProvider) Name() string { return "Fake" }
func (p *fakePrebuildRetryProvider) Models(_ context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "m1", Name: "M1", ContextSize: 128000}}, nil
}

func (p *fakePrebuildRetryProvider) Chat(
	_ context.Context, _ string, _ []provider.Message, opts provider.ChatOptions,
) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk, 1)
	defer close(ch)
	p.calls++
	if p.calls == 2 && len(opts.SystemParts) > 2 {
		p.secondVolatile = opts.SystemParts[2]
	}
	switch p.calls {
	case 1:
		ch <- provider.StreamChunk{Delta: "I will start by reviewing the project structure.", FinishReason: "stop"}
	case 2:
		ch <- provider.StreamChunk{
			ToolCalls: []provider.ToolCall{{
				ID:        "t1",
				Name:      "test",
				Arguments: `{"command":"go test ./..."}`,
			}},
			FinishReason: "tool_calls",
		}
	default:
		ch <- provider.StreamChunk{Delta: "Verification complete.", FinishReason: "stop"}
	}
	return ch, nil
}

func TestTurnLoop_PrebuildStopRetriesOnce(t *testing.T) {
	dir := t.TempDir()
	reg := tool.NewRegistry()
	reg.Register(&tool.Tool{
		Name:     "test",
		ReadOnly: true,
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			return &tool.Result{Output: "PASS", Metadata: map[string]any{"exit_code": 0}}, nil
		},
	})

	prov := &fakePrebuildRetryProvider{}
	req := &pipeline.TurnRequest{
		SessionID:  "prebuild-retry-test",
		ProviderID: "fake",
		AgentID:    "engineer",
		Messages: []provider.Message{{
			Role:  "user",
			Parts: []provider.MessagePart{provider.TextPart{Text: "Review the project and suggest improvements"}},
		}},
		Tools: []provider.Tool{
			{Name: "test", Parameters: []byte(`{"type":"object"}`)},
		},
		Model: provider.Model{ID: "m1", Name: "M1", ContextSize: 128000},
	}

	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      prov,
		modelID:   "m1",
		publish:   func(string, SSEEvent) {},
		sndbx:     sandbox.New(dir, sandbox.OriginTUI, reg, permission.Config{}),
		msgs:      &turnLoopMessageRepo{},
		cfg:       &config.SessionConfig{},
		toolCache: newToolResultCache(),
	}

	if err := tl.run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if prov.calls != 3 {
		t.Fatalf("provider calls = %d, want 3", prov.calls)
	}
	if !strings.Contains(prov.secondVolatile, "Prebuild is incomplete") {
		t.Fatalf("second request volatile part = %q, want prebuild retry directive", prov.secondVolatile)
	}
}

type fakePreplanResearchRetryProvider struct {
	calls int
}

func (p *fakePreplanResearchRetryProvider) ID() string   { return "fake" }
func (p *fakePreplanResearchRetryProvider) Name() string { return "Fake" }
func (p *fakePreplanResearchRetryProvider) Models(_ context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "m1", Name: "M1", ContextSize: 128000}}, nil
}

func (p *fakePreplanResearchRetryProvider) Chat(
	_ context.Context, _ string, _ []provider.Message, _ provider.ChatOptions,
) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk, 1)
	defer close(ch)
	p.calls++
	switch p.calls {
	case 1:
		ch <- provider.StreamChunk{
			ToolCalls: []provider.ToolCall{{
				ID:        "t1",
				Name:      "test",
				Arguments: `{"command":"go test ./..."}`,
			}},
			FinishReason: "tool_calls",
		}
	case 2:
		ch <- provider.StreamChunk{Delta: "The verification completed successfully. I will now proceed to research.", FinishReason: "stop"}
	default:
		ch <- provider.StreamChunk{Delta: "Starting explorer research now.", FinishReason: "stop"}
	}
	return ch, nil
}

func TestTurnLoop_PreplanResearchDoesNotInjectLegacyRetry(t *testing.T) {
	dir := t.TempDir()
	reg := tool.NewRegistry()
	reg.Register(&tool.Tool{
		Name:     "test",
		ReadOnly: true,
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			return &tool.Result{Output: "PASS", Metadata: map[string]any{"exit_code": 0}}, nil
		},
	})

	prov := &fakePreplanResearchRetryProvider{}
	req := &pipeline.TurnRequest{
		SessionID:  "preplan-retry-test",
		ProviderID: "fake",
		AgentID:    "engineer",
		Messages: []provider.Message{{
			Role:  "user",
			Parts: []provider.MessagePart{provider.TextPart{Text: "Please review the full project and suggest any enhancements or improvements."}},
		}},
		Tools: []provider.Tool{
			{Name: "test", Parameters: []byte(`{"type":"object"}`)},
			{Name: "subagent", Parameters: []byte(`{"type":"object"}`)},
			{Name: "question", Parameters: []byte(`{"type":"object"}`)},
		},
		Model: provider.Model{ID: "m1", Name: "M1", ContextSize: 128000},
	}

	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      prov,
		modelID:   "m1",
		publish:   func(string, SSEEvent) {},
		sndbx:     sandbox.New(dir, sandbox.OriginTUI, reg, permission.Config{}),
		msgs:      &turnLoopMessageRepo{},
		cfg:       &config.SessionConfig{},
		toolCache: newToolResultCache(),
	}

	if err := tl.run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if prov.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", prov.calls)
	}

	for _, m := range req.Messages {
		for _, p := range m.Parts {
			if tp, ok := p.(provider.TextPart); ok && strings.Contains(tp.Text, "Research is not complete") {
				t.Fatal("did not expect legacy preplan retry instruction in request messages")
			}
		}
	}
}

type fakeApprovedTaskRetryProvider struct {
	calls          int
	secondVolatile string
}

func (p *fakeApprovedTaskRetryProvider) ID() string   { return "fake" }
func (p *fakeApprovedTaskRetryProvider) Name() string { return "Fake" }
func (p *fakeApprovedTaskRetryProvider) Models(_ context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "m1", Name: "M1", ContextSize: 128000}}, nil
}

func (p *fakeApprovedTaskRetryProvider) Chat(
	_ context.Context, _ string, _ []provider.Message, opts provider.ChatOptions,
) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk, 1)
	defer close(ch)
	p.calls++
	if p.calls == 2 && len(opts.SystemParts) > 2 {
		p.secondVolatile = opts.SystemParts[2]
	}
	if p.calls == 1 {
		ch <- provider.StreamChunk{Delta: "Task 1 is done. I will move on next.", FinishReason: "stop"}
	} else {
		ch <- provider.StreamChunk{Delta: "Continuing execution.", FinishReason: "stop"}
	}
	return ch, nil
}

func TestTurnLoop_ApprovedExecutionRetriesWhenTaskProgressIsIncomplete(t *testing.T) {
	dir := t.TempDir()
	taskStore := tool.NewTaskStore(nil)
	taskTool := tool.NewTaskTool(taskStore)
	if _, err := taskTool.Execute(t.Context(), tool.ExecutionContext{SessionID: "approved-task-retry-test"}, []byte(`{"action":"create","title":"Task One","status":"in_progress"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := taskTool.Execute(t.Context(), tool.ExecutionContext{SessionID: "approved-task-retry-test"}, []byte(`{"action":"create","title":"Task Two"}`)); err != nil {
		t.Fatal(err)
	}

	prov := &fakeApprovedTaskRetryProvider{}
	req := &pipeline.TurnRequest{
		SessionID:  "approved-task-retry-test",
		ProviderID: "fake",
		AgentID:    "engineer",
		Messages: []provider.Message{{
			Role:  "user",
			Parts: []provider.MessagePart{provider.TextPart{Text: "continue"}},
		}},
		Tools: []provider.Tool{
			{Name: "task", Parameters: []byte(`{"type":"object"}`)},
			{Name: "write", Parameters: []byte(`{"type":"object"}`)},
		},
		Workflow: &pipeline.WorkflowState{
			Plan: pipeline.WorkflowPlanState{EffectiveStatus: pipeline.WorkflowApprovalApproved},
		},
		Model: provider.Model{ID: "m1", Name: "M1", ContextSize: 128000},
	}

	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      prov,
		modelID:   "m1",
		publish:   func(string, SSEEvent) {},
		sndbx:     sandbox.New(dir, sandbox.OriginTUI, tool.NewRegistry(), permission.Config{}),
		msgs:      &turnLoopMessageRepo{},
		cfg:       &config.SessionConfig{},
		toolCache: newToolResultCache(),
		taskStore: taskStore,
	}

	if err := tl.run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if prov.calls != 5 {
		t.Fatalf("provider calls = %d, want %d", prov.calls, 5)
	}
	if !strings.Contains(prov.secondVolatile, "Task execution is not complete") {
		t.Fatalf("second request volatile part = %q, want task retry directive", prov.secondVolatile)
	}
	tasks := taskStore.GetTasks(req.SessionID)
	if len(tasks) != 2 || tasks[0].Status != "blocked" {
		t.Fatalf("tasks after stalled execution = %+v, want first task blocked", tasks)
	}
}

func TestTurnLoop_PreplanResearchDoesNotRetryForNarrowRequests(t *testing.T) {
	dir := t.TempDir()
	reg := tool.NewRegistry()
	reg.Register(&tool.Tool{
		Name:     "test",
		ReadOnly: true,
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			return &tool.Result{Output: "PASS", Metadata: map[string]any{"exit_code": 0}}, nil
		},
	})

	prov := &fakePreplanResearchRetryProvider{}
	req := &pipeline.TurnRequest{
		SessionID:  "preplan-no-retry-test",
		ProviderID: "fake",
		AgentID:    "engineer",
		Messages: []provider.Message{{
			Role:  "user",
			Parts: []provider.MessagePart{provider.TextPart{Text: "Why is auth failing in production?"}},
		}},
		Tools: []provider.Tool{
			{Name: "test", Parameters: []byte(`{"type":"object"}`)},
			{Name: "subagent", Parameters: []byte(`{"type":"object"}`)},
			{Name: "question", Parameters: []byte(`{"type":"object"}`)},
		},
		Model: provider.Model{ID: "m1", Name: "M1", ContextSize: 128000},
	}

	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      prov,
		modelID:   "m1",
		publish:   func(string, SSEEvent) {},
		sndbx:     sandbox.New(dir, sandbox.OriginTUI, reg, permission.Config{}),
		msgs:      &turnLoopMessageRepo{},
		cfg:       &config.SessionConfig{},
		toolCache: newToolResultCache(),
	}

	if err := tl.run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if prov.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", prov.calls)
	}

	for _, m := range req.Messages {
		for _, p := range m.Parts {
			if tp, ok := p.(provider.TextPart); ok && strings.Contains(tp.Text, "Research is not complete") {
				t.Fatal("did not expect internal preplan retry instruction for narrow request")
			}
		}
	}
}

func TestMaybeHandlePlannerApproval_PublishesPlanAndPersistsDecision(t *testing.T) {
	msgRepo := &turnLoopMessageRepo{}
	taskStore := tool.NewTaskStore(nil)
	taskTool := tool.NewTaskTool(taskStore)
	if _, err := taskTool.Execute(t.Context(), tool.ExecutionContext{SessionID: "plan-approval-test"}, []byte(`{"action":"create","title":"Task 1","notes":"First step"}`)); err != nil {
		t.Fatal(err)
	}
	req := &pipeline.TurnRequest{
		SessionID: "plan-approval-test",
		AgentID:   "engineer",
		FullTools: []provider.Tool{
			{Name: "subagent"},
			{Name: "question"},
		},
		PhaseFilterActive: true,
		Messages: []provider.Message{
			{
				Role: "assistant",
				Parts: []provider.MessagePart{
					provider.ToolCallPart{ID: "planner-call", Name: "subagent", Arguments: `{"agent_id":"planner","task":"make a plan"}`},
				},
			},
			{
				Role: "user",
				Parts: []provider.MessagePart{
					provider.ToolResultPart{ToolCallID: "planner-call", Output: "Let me verify a few files before creating tasks.\n\nA concise plan summary.\n\nTask 1: Something else entirely"},
				},
			},
		},
	}

	var events []SSEEvent
	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		msgs:      msgRepo,
		taskStore: taskStore,
		publish: func(_ string, ev SSEEvent) {
			events = append(events, ev)
		},
		askUser: func(_ context.Context, _ string) func(string, []string, bool, string) (string, error) {
			return func(question string, options []string, multiple bool, purpose string) (string, error) {
				if question != planApprovalQuestionText {
					t.Fatalf("question = %q, want %q", question, planApprovalQuestionText)
				}
				if multiple {
					t.Fatal("plan approval should be single-select")
				}
				if purpose != planApprovalPurpose {
					t.Fatalf("purpose = %q, want %q", purpose, planApprovalPurpose)
				}
				if len(options) != len(planApprovalOptions) {
					t.Fatalf("len(options) = %d, want %d", len(options), len(planApprovalOptions))
				}
				return planApprovalProceedOption, nil
			}
		},
	}

	executions := []toolExecution{{
		call: provider.ToolCall{
			ID:        "planner-call",
			Name:      "subagent",
			Arguments: `{"agent_id":"planner","task":"make a plan"}`,
		},
		output: "Let me verify a few files before creating tasks.\n\nA concise plan summary.\n\nTask 1: Something else entirely",
	}}

	retry, err := tl.maybeHandlePlannerApproval(executions, nil)
	if err != nil {
		t.Fatalf("maybeHandlePlannerApproval() error = %v", err)
	}
	if retry {
		t.Fatal("retry = true, want false")
	}

	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
	if events[0].Type != "assistant_message" {
		t.Fatalf("event type = %q, want %q", events[0].Type, "assistant_message")
	}
	data, ok := events[0].Data.(SSEAssistantMessageData)
	if !ok {
		t.Fatalf("event data type = %T", events[0].Data)
	}
	if data.Content != "A concise plan summary.\n\n## Tasks\n\n### task 1: Task 1\nFirst step" {
		t.Fatalf("assistant_message content = %q", data.Content)
	}
	if events[1].Type != "task_sync" {
		t.Fatalf("event type = %q, want %q", events[1].Type, "task_sync")
	}
	sync, ok := events[1].Data.(SSETaskSyncData)
	if !ok {
		t.Fatalf("task_sync data type = %T", events[1].Data)
	}
	if sync.ActiveTaskID != "task 1" {
		t.Fatalf("task_sync active task = %q, want %q", sync.ActiveTaskID, "task 1")
	}

	if len(req.Messages) != 4 {
		t.Fatalf("len(req.Messages) = %d, want 4", len(req.Messages))
	}
	if got := provider.TextFromParts(req.Messages[2].Parts); got != "A concise plan summary.\n\n## Tasks\n\n### task 1: Task 1\nFirst step" {
		t.Fatalf("plan message = %q", got)
	}
	if got, _, ok := parsePlanApprovalDecisionText(provider.TextFromParts(req.Messages[3].Parts)); !ok || got != planApprovalApproved {
		t.Fatalf("plan decision marker not appended correctly: %q", provider.TextFromParts(req.Messages[3].Parts))
	}

	msgs, err := msgRepo.ListMessagesWithParts(context.Background(), req.SessionID)
	if err != nil {
		t.Fatalf("ListMessagesWithParts() error = %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("persisted message count = %d, want 2", len(msgs))
	}
	if msgs[0].Role != "assistant" {
		t.Fatalf("persisted message[0].Role = %q, want assistant", msgs[0].Role)
	}
	if msgs[1].Role != "user" {
		t.Fatalf("persisted message[1].Role = %q, want user", msgs[1].Role)
	}
	if len(msgs[1].Parts) != 1 || !msgs[1].Parts[0].Synthetic {
		t.Fatalf("plan decision marker should be persisted as one synthetic part, got %+v", msgs[1].Parts)
	}
	tasks := taskStore.GetTasks(req.SessionID)
	if len(tasks) != 1 || tasks[0].Status != "in_progress" {
		t.Fatalf("tasks after approval = %+v, want task 1 in_progress", tasks)
	}
}

func TestMaybeHandlePlannerApproval_RetriesWhenPlannerCreatesNoTasks(t *testing.T) {
	req := &pipeline.TurnRequest{
		SessionID: "plan-no-tasks-test",
		Messages: []provider.Message{
			{
				Role: "assistant",
				Parts: []provider.MessagePart{
					provider.ToolCallPart{ID: "planner-call", Name: "subagent", Arguments: `{"agent_id":"planner","task":"make a plan"}`},
				},
			},
			{
				Role: "user",
				Parts: []provider.MessagePart{
					provider.ToolResultPart{ToolCallID: "planner-call", Output: "## Goal\nShip the feature"},
				},
			},
		},
	}

	asked := false
	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		msgs:      &turnLoopMessageRepo{},
		taskStore: tool.NewTaskStore(nil),
		askUser: func(_ context.Context, _ string) func(string, []string, bool, string) (string, error) {
			return func(string, []string, bool, string) (string, error) {
				asked = true
				return "", nil
			}
		},
	}

	executions := []toolExecution{{
		call: provider.ToolCall{
			ID:        "planner-call",
			Name:      "subagent",
			Arguments: `{"agent_id":"planner","task":"make a plan"}`,
		},
		output: "## Goal\nShip the feature",
	}}

	retrySent := false
	retry, err := tl.maybeHandlePlannerApproval(executions, &retrySent)
	if err != nil {
		t.Fatalf("maybeHandlePlannerApproval() error = %v", err)
	}
	if !retry {
		t.Fatal("retry = false, want true")
	}
	if !retrySent {
		t.Fatal("retrySent = false, want true")
	}
	if asked {
		t.Fatal("askUser should not be called when planner created no tasks")
	}
	if got := req.SystemParts[2]; !strings.Contains(got, "created no tasks") {
		t.Fatalf("volatile directive = %q, want planner-no-tasks correction", got)
	}
}

func TestMaybeHandlePlannerApproval_SaveOptionWritesMarkdownPlanFile(t *testing.T) {
	dir := t.TempDir()
	msgRepo := &turnLoopMessageRepo{}
	taskStore := tool.NewTaskStore(nil)
	taskTool := tool.NewTaskTool(taskStore)
	if _, err := taskTool.Execute(t.Context(), tool.ExecutionContext{SessionID: "plan-save-test"}, []byte(`{"action":"create","title":"Task 1","notes":"First step"}`)); err != nil {
		t.Fatal(err)
	}

	req := &pipeline.TurnRequest{
		SessionID: "plan-save-test",
		AgentID:   "engineer",
		Messages: []provider.Message{
			{
				Role: "user",
				Parts: []provider.MessagePart{
					provider.TextPart{Text: "Please review the full project and let me know if any enhancements or code improvement is required"},
				},
			},
			{
				Role: "assistant",
				Parts: []provider.MessagePart{
					provider.ToolCallPart{ID: "planner-call", Name: "subagent", Arguments: `{"agent_id":"planner","task":"make a plan"}`},
				},
			},
			{
				Role: "user",
				Parts: []provider.MessagePart{
					provider.ToolResultPart{ToolCallID: "planner-call", Output: "Plan ready.\n\n## Tasks\n\n### task 1: Task 1\nFirst step"},
				},
			},
		},
	}

	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		msgs:      msgRepo,
		taskStore: taskStore,
		sndbx:     sandbox.New(dir, sandbox.OriginTUI, tool.NewRegistry(), permission.Config{}),
		askUser: func(_ context.Context, _ string) func(string, []string, bool, string) (string, error) {
			return func(string, []string, bool, string) (string, error) {
				return planApprovalSaveOption, nil
			}
		},
	}

	executions := []toolExecution{{
		call: provider.ToolCall{
			ID:        "planner-call",
			Name:      "subagent",
			Arguments: `{"agent_id":"planner","task":"make a plan"}`,
		},
		output: "Plan ready.\n\nTask 1: Mismatched raw summary",
	}}

	retry, err := tl.maybeHandlePlannerApproval(executions, nil)
	if err != nil {
		t.Fatalf("maybeHandlePlannerApproval() error = %v", err)
	}
	if retry {
		t.Fatal("retry = true, want false")
	}

	plans, err := filepath.Glob(filepath.Join(dir, "docs", "kodacode", "plans", "*.md"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(plans) != 1 {
		t.Fatalf("plan file count = %d, want 1 (%v)", len(plans), plans)
	}
	content, err := os.ReadFile(plans[0])
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	got := string(content)
	if !strings.HasPrefix(got, "# Approved Plan\n\nPlan ready.\n\n## Tasks") {
		t.Fatalf("saved plan content = %q", got)
	}
	if strings.Contains(got, `{"summary"`) {
		t.Fatalf("saved plan should be markdown, got raw json: %q", got)
	}
}

func TestMaybeBootstrapWorkflowController_CreatesTasksAndApprovesPlan(t *testing.T) {
	taskStore := tool.NewTaskStore(nil)
	msgRepo := &turnLoopMessageRepo{}
	var events []SSEEvent
	intentProv := &workflowIntentTestProvider{
		response: `{"kind":"broad_review_execute","confidence":0.96,"reason":"review plus requested improvements over a subsystem"}`,
		assert: func(model string, messages []provider.Message, opts provider.ChatOptions) {
			if model != "fake-model" {
				t.Fatalf("classifier model = %q, want fake-model", model)
			}
			if len(messages) != 1 || provider.TextFromParts(messages[0].Parts) != "review the rate limiting implementation and improve it" {
				t.Fatalf("classifier messages = %#v", messages)
			}
			if len(opts.Tools) != 0 {
				t.Fatalf("classifier should not receive tools, got %#v", opts.Tools)
			}
			if len(opts.SystemParts) == 0 || !strings.Contains(opts.SystemParts[0], "broad_review_execute") {
				t.Fatalf("missing classifier system prompt: %#v", opts.SystemParts)
			}
		},
	}
	req := &pipeline.TurnRequest{
		SessionID: "workflow-bootstrap-test",
		AgentID:   "engineer",
		Messages:  builderBootstrapVerifiedMessages("review the rate limiting implementation and improve it"),
		Tools: []provider.Tool{
			{Name: "subagent"},
			{Name: "question"},
			{Name: "task"},
		},
		FullTools:         []provider.Tool{{Name: "subagent"}, {Name: "question"}, {Name: "task"}},
		PhaseFilterActive: true,
	}

	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      intentProv,
		modelID:   "fake-model",
		msgs:      msgRepo,
		taskStore: taskStore,
		publish: func(_ string, ev SSEEvent) {
			events = append(events, ev)
		},
		spawnSubagent: func(ctx context.Context, sessionID, agentID, task string, _ ProgressFunc) (string, error) {
			switch agentID {
			case "explorer":
				if usesStructuredPlanner(ctx, agentID) {
					t.Fatal("explorer should not run in structured planner mode")
				}
				if !strings.Contains(task, "rate limiting implementation") {
					t.Fatalf("unexpected explorer task: %q", task)
				}
				return "Relevant findings\n[SCOPE: broad]", nil
			case "planner":
				if !usesStructuredPlanner(ctx, agentID) {
					t.Fatal("planner should run in structured planner mode")
				}
				if strings.Contains(task, "Do not call any tools") == false {
					t.Fatalf("expected structured planner instruction, got: %q", task)
				}
				if strings.Contains(task, "task tool is unavailable") == false {
					t.Fatalf("expected planner task-tool restriction, got: %q", task)
				}
				return `{"summary":"Plan ready.","tasks":[{"title":"Task 1","kind":"analysis","notes":"First step. Acceptance criteria: 1. Audit scope is documented. 2. Findings are actionable."},{"title":"Task 2","kind":"report","notes":"Second step. Acceptance criteria: 1. Final report summarizes the work. 2. Recommended follow-up is included."}]}`, nil
			default:
				t.Fatalf("unexpected agent %q", agentID)
				return "", nil
			}
		},
		askUser: func(_ context.Context, _ string) func(string, []string, bool, string) (string, error) {
			return func(question string, _ []string, _ bool, purpose string) (string, error) {
				if question != planApprovalQuestionText {
					t.Fatalf("question = %q, want %q", question, planApprovalQuestionText)
				}
				if purpose != planApprovalPurpose {
					t.Fatalf("purpose = %q, want %q", purpose, planApprovalPurpose)
				}
				return planApprovalProceedOption, nil
			}
		},
	}

	if _, err := tl.maybeBootstrapWorkflowController(); err != nil {
		t.Fatalf("maybeBootstrapWorkflowController() error = %v", err)
	}
	if intentProv.calls != 1 {
		t.Fatalf("classifier calls = %d, want 1", intentProv.calls)
	}

	tasks := taskStore.GetTasks(req.SessionID)
	if len(tasks) != 2 {
		t.Fatalf("task count = %d, want 2", len(tasks))
	}
	if tasks[0].Status != "in_progress" {
		t.Fatalf("task 1 status = %q, want in_progress", tasks[0].Status)
	}
	if tasks[1].Status != "pending" {
		t.Fatalf("task 2 status = %q, want pending", tasks[1].Status)
	}
	foundApproval := false
	for _, m := range req.Messages {
		if got, _, ok := parsePlanApprovalDecisionText(provider.TextFromParts(m.Parts)); ok && got == planApprovalApproved {
			foundApproval = true
			break
		}
	}
	if !foundApproval {
		t.Fatal("approval marker not appended correctly")
	}

	var eventTypes []string
	var toolStarts []SSEToolStartData
	var toolEnds []SSEToolEndData
	for _, ev := range events {
		eventTypes = append(eventTypes, ev.Type)
		if ev.Type == "system_message" {
			t.Fatalf("unexpected system_message event during controller bootstrap: %#v", ev.Data)
		}
		switch ev.Type {
		case "tool_start":
			data, ok := ev.Data.(SSEToolStartData)
			if !ok {
				t.Fatalf("tool_start data type = %T", ev.Data)
			}
			toolStarts = append(toolStarts, data)
		case "tool_end":
			data, ok := ev.Data.(SSEToolEndData)
			if !ok {
				t.Fatalf("tool_end data type = %T", ev.Data)
			}
			toolEnds = append(toolEnds, data)
		}
	}
	if len(toolStarts) != 3 {
		t.Fatalf("tool_start count = %d, want 3 (%v)", len(toolStarts), eventTypes)
	}
	if len(toolEnds) != 3 {
		t.Fatalf("tool_end count = %d, want 3 (%v)", len(toolEnds), eventTypes)
	}
	if toolStarts[0].CallID != "workflow-controller-intent" || toolStarts[1].CallID != "workflow-controller-explorer" || toolStarts[2].CallID != "workflow-controller-planner-1" {
		t.Fatalf("tool_start call IDs = %q, %q, %q", toolStarts[0].CallID, toolStarts[1].CallID, toolStarts[2].CallID)
	}
	if toolEnds[0].CallID != "workflow-controller-intent" || toolEnds[1].CallID != "workflow-controller-explorer" || toolEnds[2].CallID != "workflow-controller-planner-1" {
		t.Fatalf("tool_end call IDs = %q, %q, %q", toolEnds[0].CallID, toolEnds[1].CallID, toolEnds[2].CallID)
	}
	if !strings.Contains(toolEnds[0].Output, "Selected workflow: Review + Plan + Execute") {
		t.Fatalf("intent tool output = %q", toolEnds[0].Output)
	}
	if toolEnds[2].Output != "Plan ready.\n\n## Tasks\n\n### task 1: Task 1\nFirst step.\n\nAcceptance criteria\n- Audit scope is documented.\n- Findings are actionable.\n\n\n### task 2: Task 2\nSecond step.\n\nAcceptance criteria\n- Final report summarizes the work.\n- Recommended follow-up is included." {
		t.Fatalf("planner tool output = %q", toolEnds[2].Output)
	}

	if len(msgRepo.messages) < 5 {
		t.Fatalf("persisted message count = %d, want at least 5", len(msgRepo.messages))
	}

	foundPlannerResult := false
	for _, m := range req.Messages {
		if m.Role != "user" {
			continue
		}
		for _, p := range m.Parts {
			tr, ok := p.(provider.ToolResultPart)
			if !ok || tr.ToolCallID != "workflow-controller-planner-1" {
				continue
			}
			foundPlannerResult = true
			if tr.Output != "Plan ready.\n\n## Tasks\n\n### task 1: Task 1\nFirst step.\n\nAcceptance criteria\n- Audit scope is documented.\n- Findings are actionable.\n\n\n### task 2: Task 2\nSecond step.\n\nAcceptance criteria\n- Final report summarizes the work.\n- Recommended follow-up is included." {
				t.Fatalf("planner tool result output = %q", tr.Output)
			}
		}
	}
	if !foundPlannerResult {
		t.Fatal("planner tool result not appended to request history")
	}
}

func TestMaybeBootstrapWorkflowController_MarksInvalidPlannerRetryAsSuperseded(t *testing.T) {
	taskStore := tool.NewTaskStore(nil)
	var events []SSEEvent
	intentProv := &workflowIntentTestProvider{
		response: `{"kind":"broad_review_execute","confidence":0.95,"reason":"broad review and improvements"}`,
	}
	req := &pipeline.TurnRequest{
		SessionID:         "workflow-bootstrap-retry-test",
		AgentID:           "engineer",
		Messages:          builderBootstrapVerifiedMessages("Please review the full project and suggest improvements."),
		Tools:             []provider.Tool{{Name: "subagent"}, {Name: "question"}, {Name: "task"}},
		FullTools:         []provider.Tool{{Name: "subagent"}, {Name: "question"}, {Name: "task"}},
		PhaseFilterActive: true,
	}

	plannerCalls := 0
	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      intentProv,
		modelID:   "fake-model",
		msgs:      &turnLoopMessageRepo{},
		taskStore: taskStore,
		publish: func(_ string, ev SSEEvent) {
			events = append(events, ev)
		},
		spawnSubagent: func(ctx context.Context, sessionID, agentID, task string, _ ProgressFunc) (string, error) {
			switch agentID {
			case "explorer":
				return "Relevant findings\n[SCOPE: broad]", nil
			case "planner":
				plannerCalls++
				if plannerCalls == 1 {
					return "Review and modernize utilities and state handling before implementation begins.", nil
				}
				return `{"summary":"Plan ready.","tasks":[{"title":"Task 1","kind":"analysis","notes":"First step. Acceptance criteria: 1. Audit scope is documented. 2. Findings are actionable."}]}`, nil
			default:
				t.Fatalf("unexpected agent %q", agentID)
				return "", nil
			}
		},
		askUser: func(_ context.Context, _ string) func(string, []string, bool, string) (string, error) {
			return func(_ string, _ []string, _ bool, _ string) (string, error) {
				return planApprovalProceedOption, nil
			}
		},
	}

	if _, err := tl.maybeBootstrapWorkflowController(); err != nil {
		t.Fatalf("maybeBootstrapWorkflowController() error = %v", err)
	}

	var plannerEnds []SSEToolEndData
	for _, ev := range events {
		if ev.Type != "tool_end" {
			continue
		}
		data, ok := ev.Data.(SSEToolEndData)
		if !ok || data.Tool != "subagent" || !strings.HasPrefix(data.CallID, "workflow-controller-planner-") {
			continue
		}
		plannerEnds = append(plannerEnds, data)
	}
	if len(plannerEnds) != 2 {
		t.Fatalf("planner tool_end count = %d, want 2", len(plannerEnds))
	}
	if plannerEnds[0].Error == nil || !strings.Contains(plannerEnds[0].Output, "was not adopted") {
		t.Fatalf("first planner retry output = %#v, want invalid/superseded marker", plannerEnds[0])
	}
	if strings.Contains(plannerEnds[0].Output, `"summary":"Bad"`) {
		t.Fatalf("first planner retry should not surface raw invalid planner output: %q", plannerEnds[0].Output)
	}
	if plannerEnds[1].Error != nil {
		t.Fatalf("second planner output unexpectedly marked error: %#v", plannerEnds[1])
	}
	if plannerEnds[1].Output != "Plan ready.\n\n## Tasks\n\n### task 1: Task 1\nFirst step.\n\nAcceptance criteria\n- Audit scope is documented.\n- Findings are actionable." {
		t.Fatalf("second planner output = %q", plannerEnds[1].Output)
	}
}

func TestMaybeBootstrapWorkflowController_WaitsForVerification(t *testing.T) {
	taskStore := tool.NewTaskStore(nil)
	intentProv := &workflowIntentTestProvider{
		response: `{"kind":"broad_review_execute","confidence":0.95,"reason":"broad review and improvements"}`,
	}
	req := &pipeline.TurnRequest{
		SessionID: "workflow-bootstrap-no-verify-test",
		AgentID:   "engineer",
		Messages: []provider.Message{{
			Role:  "user",
			Parts: []provider.MessagePart{provider.TextPart{Text: "Please review the full project and suggest improvements."}},
		}},
		Tools:             []provider.Tool{{Name: "subagent"}, {Name: "question"}, {Name: "task"}, {Name: "test"}},
		FullTools:         []provider.Tool{{Name: "subagent"}, {Name: "question"}, {Name: "task"}, {Name: "test"}},
		PhaseFilterActive: true,
	}

	spawned := false
	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      intentProv,
		modelID:   "fake-model",
		msgs:      &turnLoopMessageRepo{},
		taskStore: taskStore,
		spawnSubagent: func(context.Context, string, string, string, ProgressFunc) (string, error) {
			spawned = true
			return "", nil
		},
	}

	if _, err := tl.maybeBootstrapWorkflowController(); err != nil {
		t.Fatalf("maybeBootstrapWorkflowController() error = %v", err)
	}
	if spawned {
		t.Fatal("controller bootstrapped before verification completed")
	}
	if intentProv.calls != 0 {
		t.Fatalf("classifier calls = %d, want 0 before verification", intentProv.calls)
	}
	if got := taskStore.GetTasks(req.SessionID); len(got) != 0 {
		t.Fatalf("task count = %d, want 0", len(got))
	}
}

func TestMaybeBootstrapWorkflowController_ReusesExistingExplorerOutputs(t *testing.T) {
	taskStore := tool.NewTaskStore(nil)
	intentProv := &workflowIntentTestProvider{
		response: `{"kind":"broad_review_execute","confidence":0.95,"reason":"broad review and improvements"}`,
	}
	req := &pipeline.TurnRequest{
		SessionID: "workflow-bootstrap-existing-explorer-test",
		AgentID:   "engineer",
		Messages: append(
			builderBootstrapVerifiedMessages("Please review the full project and suggest improvements."),
			provider.Message{
				Role: "assistant",
				Parts: []provider.MessagePart{
					provider.ToolCallPart{ID: "explorer-1", Name: "subagent", Arguments: `{"agent_id":"explorer","task":"inspect backend"}`},
					provider.ToolCallPart{ID: "explorer-2", Name: "subagent", Arguments: `{"agent_id":"explorer","task":"inspect frontend"}`},
				},
			},
			provider.Message{
				Role: "user",
				Parts: []provider.MessagePart{
					provider.ToolResultPart{ToolCallID: "explorer-1", Output: "Backend findings\n[SCOPE: broad]"},
					provider.ToolResultPart{ToolCallID: "explorer-2", Output: "Frontend findings\n[SCOPE: broad]"},
				},
			},
		),
		Tools:             []provider.Tool{{Name: "subagent"}, {Name: "question"}, {Name: "task"}},
		FullTools:         []provider.Tool{{Name: "subagent"}, {Name: "question"}, {Name: "task"}},
		PhaseFilterActive: true,
	}

	explorerCalls := 0
	plannerCalls := 0
	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      intentProv,
		modelID:   "fake-model",
		msgs:      &turnLoopMessageRepo{},
		taskStore: taskStore,
		spawnSubagent: func(ctx context.Context, sessionID, agentID, task string, _ ProgressFunc) (string, error) {
			switch agentID {
			case "explorer":
				explorerCalls++
				return "unexpected", nil
			case "planner":
				plannerCalls++
				if !strings.Contains(task, "Backend findings") || !strings.Contains(task, "Frontend findings") {
					t.Fatalf("planner task should include existing explorer findings, got: %q", task)
				}
				return `{"summary":"Plan ready.","tasks":[{"title":"Task 1","kind":"analysis","notes":"First step. Acceptance criteria: 1. Audit scope is documented. 2. Findings are actionable."}]}`, nil
			default:
				t.Fatalf("unexpected agent %q", agentID)
				return "", nil
			}
		},
		askUser: func(_ context.Context, _ string) func(string, []string, bool, string) (string, error) {
			return func(_ string, _ []string, _ bool, _ string) (string, error) {
				return planApprovalProceedOption, nil
			}
		},
	}

	bootstrapped, err := tl.maybeBootstrapWorkflowController()
	if err != nil {
		t.Fatalf("maybeBootstrapWorkflowController() error = %v", err)
	}
	if !bootstrapped {
		t.Fatal("bootstrapped = false, want true")
	}
	if explorerCalls != 0 {
		t.Fatalf("explorerCalls = %d, want 0", explorerCalls)
	}
	if plannerCalls != 1 {
		t.Fatalf("plannerCalls = %d, want 1", plannerCalls)
	}
	tasks := taskStore.GetTasks(req.SessionID)
	if len(tasks) != 1 || tasks[0].Title != "Task 1" {
		t.Fatalf("tasks = %+v, want one adopted planner task", tasks)
	}
}

func TestMaybeBootstrapWorkflowController_SkipsNonBroadIntent(t *testing.T) {
	taskStore := tool.NewTaskStore(nil)
	intentProv := &workflowIntentTestProvider{
		response: `{"kind":"direct_execute","confidence":0.92,"reason":"narrow targeted change"}`,
	}
	req := &pipeline.TurnRequest{
		SessionID: "workflow-bootstrap-direct-execute-test",
		AgentID:   "engineer",
		Messages:  builderBootstrapVerifiedMessages("rename foo to bar in src/x.ts"),
		Tools:     []provider.Tool{{Name: "subagent"}, {Name: "question"}, {Name: "task"}},
	}

	spawned := false
	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      intentProv,
		modelID:   "fake-model",
		msgs:      &turnLoopMessageRepo{},
		taskStore: taskStore,
		spawnSubagent: func(context.Context, string, string, string, ProgressFunc) (string, error) {
			spawned = true
			return "", nil
		},
	}

	bootstrapped, err := tl.maybeBootstrapWorkflowController()
	if err != nil {
		t.Fatalf("maybeBootstrapWorkflowController() error = %v", err)
	}
	if bootstrapped {
		t.Fatal("bootstrapped = true, want false")
	}
	if spawned {
		t.Fatal("spawned = true, want false")
	}
	if intentProv.calls != 1 {
		t.Fatalf("classifier calls = %d, want 1", intentProv.calls)
	}
}

func TestMaybeBootstrapWorkflowController_BootstrapsPlanOnlyIntent(t *testing.T) {
	taskStore := tool.NewTaskStore(nil)
	intentProv := &workflowIntentTestProvider{
		response: `{"kind":"plan_only","confidence":0.94,"reason":"explicit plan request"}`,
	}
	req := &pipeline.TurnRequest{
		SessionID: "workflow-bootstrap-plan-only-test",
		AgentID:   "engineer",
		Messages:  builderBootstrapVerifiedMessages("make a plan to refactor auth"),
		Tools:     []provider.Tool{{Name: "subagent"}, {Name: "question"}, {Name: "task"}},
	}

	spawned := 0
	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      intentProv,
		modelID:   "fake-model",
		msgs:      &turnLoopMessageRepo{},
		taskStore: taskStore,
		spawnSubagent: func(_ context.Context, _ string, agentID, _ string, _ ProgressFunc) (string, error) {
			spawned++
			switch agentID {
			case "explorer":
				return "Relevant findings\n[SCOPE: broad]", nil
			case "planner":
				return `{"summary":"Plan ready.","tasks":[{"title":"Task 1","kind":"analysis","notes":"First step. Acceptance criteria: 1. Audit scope is documented. 2. Findings are actionable."}]}`, nil
			default:
				t.Fatalf("unexpected agent %q", agentID)
				return "", nil
			}
		},
		askUser: func(_ context.Context, _ string) func(string, []string, bool, string) (string, error) {
			return func(_ string, _ []string, _ bool, _ string) (string, error) {
				return planApprovalProceedOption, nil
			}
		},
	}

	bootstrapped, err := tl.maybeBootstrapWorkflowController()
	if err != nil {
		t.Fatalf("maybeBootstrapWorkflowController() error = %v", err)
	}
	if !bootstrapped {
		t.Fatal("bootstrapped = false, want true")
	}
	if spawned != 2 {
		t.Fatalf("spawned = %d, want 2", spawned)
	}
}

func TestMaybeBootstrapWorkflowController_UsesUtilityModelForIntentPreflight(t *testing.T) {
	taskStore := tool.NewTaskStore(nil)
	primaryProv := &workflowIntentTestProvider{
		response: `{"kind":"direct_execute","confidence":0.20,"reason":"should not be used"}`,
	}
	utilityProv := &workflowIntentTestProvider{
		response: `{"kind":"broad_review_execute","confidence":0.97,"reason":"broad review plus improvements"}`,
		assert: func(model string, messages []provider.Message, opts provider.ChatOptions) {
			if model != "utility-model" {
				t.Fatalf("utility classifier model = %q, want utility-model", model)
			}
			if len(messages) != 1 || provider.TextFromParts(messages[0].Parts) != "review the rate limiting implementation and improve it" {
				t.Fatalf("utility classifier messages = %#v", messages)
			}
			if len(opts.Tools) != 0 {
				t.Fatalf("utility classifier should not receive tools, got %#v", opts.Tools)
			}
		},
	}
	req := &pipeline.TurnRequest{
		SessionID: "workflow-bootstrap-utility-intent-test",
		AgentID:   "engineer",
		Messages:  builderBootstrapVerifiedMessages("review the rate limiting implementation and improve it"),
		Tools:     []provider.Tool{{Name: "subagent"}, {Name: "question"}, {Name: "task"}},
	}

	spawned := false
	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      primaryProv,
		modelID:   "primary-model",
		utility:   utilityProvider{prov: utilityProv, modelID: "utility-model"},
		msgs:      &turnLoopMessageRepo{},
		taskStore: taskStore,
		spawnSubagent: func(context.Context, string, string, string, ProgressFunc) (string, error) {
			spawned = true
			return `{"summary":"Plan ready.","tasks":[{"title":"Task 1","kind":"analysis","notes":"First step. Acceptance criteria: 1. Audit scope is documented. 2. Findings are actionable."}]}`, nil
		},
		askUser: func(_ context.Context, _ string) func(string, []string, bool, string) (string, error) {
			return func(_ string, _ []string, _ bool, _ string) (string, error) {
				return planApprovalProceedOption, nil
			}
		},
	}

	_, err := tl.maybeBootstrapWorkflowController()
	if err != nil {
		t.Fatalf("maybeBootstrapWorkflowController() error = %v", err)
	}
	if primaryProv.calls != 0 {
		t.Fatalf("primary classifier calls = %d, want 0", primaryProv.calls)
	}
	if utilityProv.calls != 1 {
		t.Fatalf("utility classifier calls = %d, want 1", utilityProv.calls)
	}
	if !spawned {
		t.Fatal("spawned = false, want true")
	}
}

func TestMaybeBootstrapWorkflowController_StartsAfterFailedVerificationAttempt(t *testing.T) {
	taskStore := tool.NewTaskStore(nil)
	intentProv := &workflowIntentTestProvider{
		response: `{"kind":"broad_review_execute","confidence":0.95,"reason":"broad review and improvements"}`,
	}
	req := &pipeline.TurnRequest{
		SessionID: "workflow-bootstrap-failed-verify-test",
		AgentID:   "engineer",
		Messages:  builderBootstrapAttemptedVerificationMessages("review the rate limiting implementation and improve it"),
		Tools:     []provider.Tool{{Name: "subagent"}, {Name: "question"}, {Name: "task"}},
	}

	spawned := 0
	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      intentProv,
		modelID:   "fake-model",
		msgs:      &turnLoopMessageRepo{},
		taskStore: taskStore,
		spawnSubagent: func(_ context.Context, _ string, agentID, _ string, _ ProgressFunc) (string, error) {
			spawned++
			switch agentID {
			case "explorer":
				return "Relevant findings\n[SCOPE: broad]", nil
			case "planner":
				return `{"summary":"Plan ready.","tasks":[{"title":"Task 1","kind":"analysis","notes":"First step. Acceptance criteria: 1. Audit scope is documented. 2. Findings are actionable."}]}`, nil
			default:
				t.Fatalf("unexpected agent %q", agentID)
				return "", nil
			}
		},
		askUser: func(_ context.Context, _ string) func(string, []string, bool, string) (string, error) {
			return func(_ string, _ []string, _ bool, _ string) (string, error) {
				return planApprovalProceedOption, nil
			}
		},
	}

	bootstrapped, err := tl.maybeBootstrapWorkflowController()
	if err != nil {
		t.Fatalf("maybeBootstrapWorkflowController() error = %v", err)
	}
	if !bootstrapped {
		t.Fatal("bootstrapped = false, want true")
	}
	if spawned != 2 {
		t.Fatalf("spawned = %d, want 2", spawned)
	}
}

func TestMaybeBootstrapWorkflowController_FallsBackToPrimaryIntentModel(t *testing.T) {
	taskStore := tool.NewTaskStore(nil)
	primaryProv := &workflowIntentTestProvider{
		response: `{"kind":"broad_review_execute","confidence":0.91,"reason":"broad review after utility failure"}`,
	}
	utilityProv := &workflowIntentTestProvider{
		chatErr: fmt.Errorf("utility unavailable"),
	}
	req := &pipeline.TurnRequest{
		SessionID: "workflow-bootstrap-intent-fallback-test",
		AgentID:   "engineer",
		Messages:  builderBootstrapVerifiedMessages("review the rate limiting implementation and improve it"),
		Tools:     []provider.Tool{{Name: "subagent"}, {Name: "question"}, {Name: "task"}},
	}

	spawned := false
	var events []SSEEvent
	tl := &turnLoop{
		ctx:       context.Background(),
		req:       req,
		prov:      primaryProv,
		modelID:   "primary-model",
		utility:   utilityProvider{prov: utilityProv, modelID: "utility-model"},
		msgs:      &turnLoopMessageRepo{},
		taskStore: taskStore,
		publish: func(_ string, ev SSEEvent) {
			events = append(events, ev)
		},
		spawnSubagent: func(_ context.Context, _ string, agentID, _ string, _ ProgressFunc) (string, error) {
			spawned = true
			switch agentID {
			case "explorer":
				return "Relevant findings\n[SCOPE: broad]", nil
			case "planner":
				return `{"summary":"Plan ready.","tasks":[{"title":"Task 1","kind":"analysis","notes":"First step. Acceptance criteria: 1. Audit scope is documented. 2. Findings are actionable."}]}`, nil
			default:
				t.Fatalf("unexpected agent %q", agentID)
				return "", nil
			}
		},
		askUser: func(_ context.Context, _ string) func(string, []string, bool, string) (string, error) {
			return func(_ string, _ []string, _ bool, _ string) (string, error) {
				return planApprovalProceedOption, nil
			}
		},
	}

	bootstrapped, err := tl.maybeBootstrapWorkflowController()
	if err != nil {
		t.Fatalf("maybeBootstrapWorkflowController() error = %v", err)
	}
	if !bootstrapped {
		t.Fatal("bootstrapped = false, want true")
	}
	if utilityProv.calls != 1 {
		t.Fatalf("utility classifier calls = %d, want 1", utilityProv.calls)
	}
	if primaryProv.calls != 1 {
		t.Fatalf("primary classifier calls = %d, want 1", primaryProv.calls)
	}
	if !spawned {
		t.Fatal("spawned = false, want true")
	}
	var intentEnd *SSEToolEndData
	for _, ev := range events {
		if ev.Type != "tool_end" {
			continue
		}
		data, ok := ev.Data.(SSEToolEndData)
		if !ok || data.CallID != "workflow-controller-intent" {
			continue
		}
		intentEnd = &data
		break
	}
	if intentEnd == nil {
		t.Fatal("intent tool_end not published")
	}
	if !strings.Contains(intentEnd.Output, "primary-model") {
		t.Fatalf("intent output = %q, want primary fallback marker", intentEnd.Output)
	}
}

func TestTurnLoopCostModelFor_UsesResolvedFallbackModelPricing(t *testing.T) {
	tl := &turnLoop{
		req: &pipeline.TurnRequest{
			Model: provider.Model{
				ID:         "primary-model",
				CostInput:  10,
				CostOutput: 20,
			},
		},
		modelID:        "fallback-model",
		primaryModelID: "primary-model",
		requestCostModel: provider.Model{
			ID:         "fallback-model",
			CostInput:  1.5,
			CostOutput: 2.5,
		},
	}

	got := tl.costModelFor("fallback-model")
	if got.CostInput != 1.5 || got.CostOutput != 2.5 {
		t.Fatalf("fallback cost model = %#v, want fallback pricing", got)
	}
}

func builderBootstrapVerifiedMessages(prompt string) []provider.Message {
	return []provider.Message{
		{
			Role:  "user",
			Parts: []provider.MessagePart{provider.TextPart{Text: prompt}},
		},
		{
			Role: "assistant",
			Parts: []provider.MessagePart{provider.ToolCallPart{
				ID:        "verify-1",
				Name:      "test",
				Arguments: `{"command":"go test ./...","description":"Run tests"}`,
			}},
		},
		{
			Role: "user",
			Parts: []provider.MessagePart{provider.ToolResultPart{
				ToolCallID: "verify-1",
				Output:     "PASS",
				Metadata:   map[string]any{"exit_code": 0},
			}},
		},
	}
}

func builderBootstrapAttemptedVerificationMessages(prompt string) []provider.Message {
	return []provider.Message{
		{
			Role:  "user",
			Parts: []provider.MessagePart{provider.TextPart{Text: prompt}},
		},
		{
			Role: "assistant",
			Parts: []provider.MessagePart{provider.ToolCallPart{
				ID:        "verify-1",
				Name:      "test",
				Arguments: `{"command":"npm run test","description":"Run tests"}`,
			}},
		},
		{
			Role: "user",
			Parts: []provider.MessagePart{provider.ToolResultPart{
				ToolCallID: "verify-1",
				Output:     "FAIL",
				Metadata:   map[string]any{"exit_code": 1},
			}},
		},
	}
}

func TestParseStructuredPlan_FallsBackToJSONLikeExtraction(t *testing.T) {
	raw := `{"summary":"Plan summary","tasks":[{"title":"Task One","kind":"implementation","notes":"First note. Acceptance criteria: 1. First criterion. 2. Second criterion."},{"title":"Task Two","kind":"analysis","notes":"Second note. Acceptance criteria: 1. Audit is documented. 2. Findings are actionable.","tasks":[{"title":"nested"}]}`

	plan, err := parseStructuredPlan(raw)
	if err != nil {
		t.Fatalf("parseStructuredPlan() error = %v", err)
	}
	if plan.Summary != "Plan summary" {
		t.Fatalf("summary = %q, want %q", plan.Summary, "Plan summary")
	}
	if len(plan.Tasks) != 2 {
		t.Fatalf("task count = %d, want 2", len(plan.Tasks))
	}
	if plan.Tasks[0].Title != "Task One" || plan.Tasks[1].Title != "Task Two" {
		t.Fatalf("tasks = %#v", plan.Tasks)
	}
	if plan.Tasks[0].Kind != tool.TaskKindImplementation || plan.Tasks[1].Kind != tool.TaskKindAnalysis {
		t.Fatalf("task kinds = %#v", plan.Tasks)
	}
}

func TestParseStructuredPlan_FallsBackToProseTasks(t *testing.T) {
	raw := "Review and modernize peripheral areas.\n\nTasks:\n1. Refactor utility helpers\nKind: implementation\nConsolidate shared helpers.\nAcceptance criteria:\n- Shared helpers are centralized.\n- Existing callers still work.\n\n2. Tighten permission errors\nKind: implementation\nReturn a controlled ForbiddenError.\nAcceptance criteria:\n- Invalid permissions return ForbiddenError.\n- Silent failures are removed.\n"

	plan, err := parseStructuredPlan(raw)
	if err != nil {
		t.Fatalf("parseStructuredPlan() error = %v", err)
	}
	if plan.Summary != "Review and modernize peripheral areas." {
		t.Fatalf("summary = %q", plan.Summary)
	}
	if len(plan.Tasks) != 2 {
		t.Fatalf("task count = %d, want 2", len(plan.Tasks))
	}
	if plan.Tasks[0].Title != "Refactor utility helpers" {
		t.Fatalf("task 1 title = %q", plan.Tasks[0].Title)
	}
	if plan.Tasks[0].Kind != tool.TaskKindImplementation {
		t.Fatalf("task 1 kind = %q, want %q", plan.Tasks[0].Kind, tool.TaskKindImplementation)
	}
	if plan.Tasks[1].Title != "Tighten permission errors" {
		t.Fatalf("task 2 title = %q", plan.Tasks[1].Title)
	}
	if !strings.Contains(plan.Tasks[1].Notes, "ForbiddenError") {
		t.Fatalf("task 2 notes = %q", plan.Tasks[1].Notes)
	}
}

func TestParseStructuredPlan_FallsBackToInlineTaskSections(t *testing.T) {
	raw := "Review and modernize peripheral areas.\n\nTasks: task 1: Audit middleware consistency\nKind: analysis\nReview auth/session validation paths.\nAcceptance criteria:\n- Key auth/session paths are reviewed.\n- Findings identify concrete risks.\n task 2: Compile findings report\nKind: report\nSummarize the highest-risk issues.\nAcceptance criteria:\n- The report lists the top issues.\n- Recommended next steps are included."

	plan, err := parseStructuredPlan(raw)
	if err != nil {
		t.Fatalf("parseStructuredPlan() error = %v", err)
	}
	if len(plan.Tasks) != 2 {
		t.Fatalf("task count = %d, want 2", len(plan.Tasks))
	}
	if plan.Tasks[0].Title != "Audit middleware consistency" {
		t.Fatalf("task 1 title = %q", plan.Tasks[0].Title)
	}
	if plan.Tasks[0].Kind != tool.TaskKindAnalysis {
		t.Fatalf("task 1 kind = %q, want %q", plan.Tasks[0].Kind, tool.TaskKindAnalysis)
	}
	if plan.Tasks[1].Title != "Compile findings report" {
		t.Fatalf("task 2 title = %q", plan.Tasks[1].Title)
	}
	if plan.Tasks[1].Kind != tool.TaskKindReport {
		t.Fatalf("task 2 kind = %q, want %q", plan.Tasks[1].Kind, tool.TaskKindReport)
	}
}

func TestParseStructuredPlan_FallsBackToPartHeadings(t *testing.T) {
	raw := "Goal: tighten the surrounding architecture.\n\n## Part 1: Refactor utility helpers\nKind: implementation\nConsolidate repeated helpers.\nAcceptance criteria:\n- Repeated helpers are consolidated.\n- Existing behavior is preserved.\n\n## Part 2: Tighten permission errors\nKind: implementation\nReturn ForbiddenError consistently.\nAcceptance criteria:\n- ForbiddenError is returned consistently.\n- Permission failures are explicit.\n"

	plan, err := parseStructuredPlan(raw)
	if err != nil {
		t.Fatalf("parseStructuredPlan() error = %v", err)
	}
	if plan.Summary != "Goal: tighten the surrounding architecture." {
		t.Fatalf("summary = %q", plan.Summary)
	}
	if len(plan.Tasks) != 2 {
		t.Fatalf("task count = %d, want 2", len(plan.Tasks))
	}
	if plan.Tasks[0].Title != "Refactor utility helpers" {
		t.Fatalf("task 1 title = %q", plan.Tasks[0].Title)
	}
	if !strings.Contains(plan.Tasks[1].Notes, "ForbiddenError") {
		t.Fatalf("task 2 notes = %q", plan.Tasks[1].Notes)
	}
}

func TestParseStructuredPlan_RejectsMissingTaskKind(t *testing.T) {
	raw := "Review and modernize peripheral areas.\n\nTasks:\n1. Refactor utility helpers\nConsolidate shared helpers.\n"

	_, err := parseStructuredPlan(raw)
	if err == nil {
		t.Fatal("parseStructuredPlan() error = nil, want missing-kind failure")
	}
	if !strings.Contains(err.Error(), "missing required kind") {
		t.Fatalf("error = %v, want missing-kind failure", err)
	}
}

func TestParseStructuredPlan_RejectsMissingAcceptanceCriteria(t *testing.T) {
	raw := `{"summary":"Plan summary","tasks":[{"title":"Task One","kind":"implementation","notes":"First note only"}]}`

	_, err := parseStructuredPlan(raw)
	if err == nil {
		t.Fatal("parseStructuredPlan() error = nil, want missing-criteria failure")
	}
	if !strings.Contains(err.Error(), "missing required acceptance criteria") {
		t.Fatalf("error = %v, want missing-criteria failure", err)
	}
}

func TestFilterWorkflowCompletionCalls_BlocksNextTaskStartAfterCompletion(t *testing.T) {
	tl := &turnLoop{
		req: &pipeline.TurnRequest{
			AgentID: "engineer",
		},
	}

	calls := []provider.ToolCall{
		{ID: "complete-1", Name: "task", Arguments: `{"action":"update","id":"1","status":"completed"}`},
		{ID: "start-2", Name: "task", Arguments: `{"action":"update","id":"2","status":"in_progress"}`},
		{ID: "read-1", Name: "read", Arguments: `{"filePath":"/tmp/file"}`},
	}

	filtered, blocked := tl.filterWorkflowCompletionCalls(calls)
	if !blocked {
		t.Fatal("blocked = false, want true")
	}
	if len(filtered) != 2 {
		t.Fatalf("filtered call count = %d, want 2", len(filtered))
	}
	if filtered[0].ID != "complete-1" {
		t.Fatalf("first filtered call = %q, want completion call", filtered[0].ID)
	}
	if filtered[1].ID != "read-1" {
		t.Fatalf("second filtered call = %q, want non-task call retained", filtered[1].ID)
	}
}

func TestRenderStructuredPlan_FormatsAcceptanceCriteria(t *testing.T) {
	out := renderStructuredPlan("Plan ready.", []*tool.Task{{
		ID:    "task 1",
		Title: "Improve tracing",
		Notes: "Implement standardized request tracing. Acceptance criteria: 1. A trace ID is attached to each request. 2. Middleware propagates the trace ID.",
	}})

	want := "Plan ready.\n\n## Tasks\n\n### task 1: Improve tracing\nImplement standardized request tracing.\n\nAcceptance criteria\n- A trace ID is attached to each request.\n- Middleware propagates the trace ID."
	if out != want {
		t.Fatalf("renderStructuredPlan() = %q, want %q", out, want)
	}
}

func TestMaybeHandleDirectPlannerApproval_PersistsDecision(t *testing.T) {
	msgRepo := &turnLoopMessageRepo{}
	req := &pipeline.TurnRequest{
		SessionID: "direct-planner-test",
		AgentID:   "planner",
		Messages: []provider.Message{
			{
				Role: "assistant",
				Parts: []provider.MessagePart{
					provider.TextPart{Text: "Goal: Ship the feature"},
				},
			},
		},
	}

	tl := &turnLoop{
		ctx:  context.Background(),
		req:  req,
		msgs: msgRepo,
		askUser: func(_ context.Context, _ string) func(string, []string, bool, string) (string, error) {
			return func(question string, options []string, multiple bool, purpose string) (string, error) {
				if question != planApprovalQuestionText {
					t.Fatalf("question = %q, want %q", question, planApprovalQuestionText)
				}
				if purpose != planApprovalPurpose {
					t.Fatalf("purpose = %q, want %q", purpose, planApprovalPurpose)
				}
				return planApprovalSaveOption, nil
			}
		},
	}

	if err := tl.maybeHandleDirectPlannerApproval("Goal: Ship the feature"); err != nil {
		t.Fatalf("maybeHandleDirectPlannerApproval() error = %v", err)
	}

	if len(req.Messages) != 2 {
		t.Fatalf("len(req.Messages) = %d, want 2", len(req.Messages))
	}
	if got, answer, ok := parsePlanApprovalDecisionText(provider.TextFromParts(req.Messages[1].Parts)); !ok || got != planApprovalApproved || answer != planApprovalSaveOption {
		t.Fatalf("planner decision marker not appended correctly: %q", provider.TextFromParts(req.Messages[1].Parts))
	}

	msgs, err := msgRepo.ListMessagesWithParts(context.Background(), req.SessionID)
	if err != nil {
		t.Fatalf("ListMessagesWithParts() error = %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("persisted message count = %d, want 1", len(msgs))
	}
	if len(msgs[0].Parts) != 1 || !msgs[0].Parts[0].Synthetic {
		t.Fatalf("planner approval marker should be persisted as one synthetic part, got %+v", msgs[0].Parts)
	}
}
