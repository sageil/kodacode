package service_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/permission"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/sandbox"
	"github.com/sageil/kodacode/v1/internal/service"
	"github.com/sageil/kodacode/v1/internal/tool"
)

// fakeToolProvider is a provider that returns tool_calls on the first Chat()
// call, then "stop" on the second call (after tool results are fed back).
type fakeToolProvider struct {
	id     string
	calls  int
	chunks [][]provider.StreamChunk // chunks[0] = first call, chunks[1] = second call, ...
	mu     sync.Mutex
	inputs []string
}

func (p *fakeToolProvider) ID() string   { return p.id }
func (p *fakeToolProvider) Name() string { return p.id }
func (p *fakeToolProvider) Models(_ context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "fake-model", Name: "Fake", ContextSize: 8192}}, nil
}
func (p *fakeToolProvider) Chat(_ context.Context, _ string, messages []provider.Message, _ provider.ChatOptions) (<-chan provider.StreamChunk, error) {
	p.mu.Lock()
	p.inputs = append(p.inputs, lastUserMessage(messages))
	p.mu.Unlock()
	idx := p.calls
	if idx >= len(p.chunks) {
		idx = len(p.chunks) - 1
	}
	p.calls++
	ch := make(chan provider.StreamChunk, len(p.chunks[idx]))
	for _, c := range p.chunks[idx] {
		ch <- c
	}
	close(ch)
	return ch, nil
}

func (p *fakeToolProvider) Inputs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, len(p.inputs))
	copy(out, p.inputs)
	return out
}

type testAgentLookup interface {
	Get(string) (config.AgentConfig, error)
}

func newToolDispatchService(t *testing.T, fp provider.Provider, cfg *config.Config, toolReg *tool.Registry, agents testAgentLookup) *service.SessionService {
	t.Helper()
	if cfg == nil {
		cfg = testCfg()
	}
	projectDir := t.TempDir()
	sessRepo := newFakeSessionRepo()
	msgRepo := newFakeMessageRepo()
	registry := testRegistry(fp)

	svc := service.NewSessionService(
		sessRepo,
		msgRepo,
		registry,
		cfg,
		toolReg,
		agents,
		projectDir,
		nil,
	)

	sndbx := sandbox.New(projectDir, sandbox.OriginTUI, toolReg, cfg.ResolvedPermission(nil), sandbox.SandboxOptions{
		OnBackgroundDone: svc.BackgroundDoneHandler(),
	})
	cc := service.ChainConfig{
		Sandbox:      sndbx,
		Config:       cfg,
		Registry:     registry,
		ToolRegistry: toolReg,
		Messages:     msgRepo,
		Sessions:     sessRepo,
		Publish:      svc.Publisher(),
		AskPerm:      svc.AskPermission(),
		AskUser:      svc.AskUser(),
		GetCost:      svc.GetOrCreateCost,
	}
	svc.SetChain(pipeline.BuildChain(
		sandbox.NewMiddleware(sndbx),
		service.NewToolResolverMiddleware(toolReg, nil, ""),
		service.NewLLMMiddleware(&cc),
	))
	return svc
}

func lastUserMessage(messages []provider.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		return provider.TextFromParts(messages[i].Parts)
	}
	return ""
}

// ---- dispatchTool via Send (allow) -----------------------------------------

func TestDispatchTool_allow_executesTool(t *testing.T) {
	ctx := context.Background()

	// Provider: first call returns a tool_calls chunk; second returns stop.
	fp := &fakeToolProvider{
		id: "fake",
		chunks: [][]provider.StreamChunk{
			{
				{ToolCalls: []provider.ToolCall{{ID: "tc1", Name: "echo", Arguments: `{"text":"hi"}`}}, FinishReason: "tool_calls"},
			},
			{
				{Delta: "done", FinishReason: "stop"},
			},
		},
	}

	// Register a simple "echo" tool.
	toolReg := tool.NewRegistry()
	toolReg.Register(&tool.Tool{
		Name:        "echo",
		Description: "echoes text",
		Parameters:  []byte(`{}`),
		Execute: func(_ context.Context, _ tool.ExecutionContext, args []byte) (*tool.Result, error) {
			return &tool.Result{Output: "echoed: " + string(args)}, nil
		},
	})

	svc := newToolDispatchService(t, fp, testCfg(), toolReg, nil)

	sess, _ := svc.Create(ctx, "default", "fake/fake-model")
	sub, cancel := svc.Subscribe(sess.ID)
	defer cancel()

	if err := svc.Send(ctx, sess.ID, "use echo", nil, sandbox.OriginTUI); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	// Collect all events.
	var types []string
	timeout := time.After(2 * time.Second)
loop:
	for {
		select {
		case ev, ok := <-sub:
			if !ok {
				break loop
			}
			types = append(types, ev.Type)
			if ev.Type == "done" || ev.Type == "error" {
				break loop
			}
		case <-timeout:
			t.Fatal("Send() with tool call timed out")
		}
	}

	// Must have tool_start and tool_end events.
	has := func(name string) bool {
		for _, et := range types {
			if et == name {
				return true
			}
		}
		return false
	}
	if !has("tool_start") {
		t.Errorf("events = %v, want tool_start", types)
	}
	if !has("tool_end") {
		t.Errorf("events = %v, want tool_end", types)
	}
}

// ---- dispatchTool via Send (deny via agent permissions) --------------------

func TestDispatchTool_deny_returnsError(t *testing.T) {
	ctx := context.Background()

	fp := &fakeToolProvider{
		id: "fake",
		chunks: [][]provider.StreamChunk{
			{
				{ToolCalls: []provider.ToolCall{{ID: "tc1", Name: "bash", Arguments: `{"command":"rm -rf temp"}`}}, FinishReason: "tool_calls"},
			},
			{
				{Delta: "ok", FinishReason: "stop"},
			},
		},
	}

	toolReg := tool.NewRegistry()
	toolReg.Register(&tool.Tool{
		Name:        "bash",
		Description: "run shell",
		Parameters:  []byte(`{}`),
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			return &tool.Result{Output: "executed"}, nil
		},
	})

	// Agent-level sandbox rule denying rm/rmdir commands.
	agentSvc := &fakeAgentLookup{cfg: config.AgentConfig{
		Tools: []string{"bash"},
		Permission: permission.Config{
			"bash_rm": {Action: permission.ActionDeny},
		},
		SystemPrompt: "",
	}}

	svc := newToolDispatchService(t, fp, testCfg(), toolReg, agentSvc)

	sess, _ := svc.Create(ctx, "default", "fake/fake-model")
	sub, cancel := svc.Subscribe(sess.ID)
	defer cancel()

	// Send must still complete (dispatchTool returns error, but loop continues).
	if err := svc.Send(ctx, sess.ID, "run bash", nil, sandbox.OriginTUI); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	// Collect events.
	var toolEndErr *string
	timeout := time.After(2 * time.Second)
loop:
	for {
		select {
		case ev, ok := <-sub:
			if !ok {
				break loop
			}
			if ev.Type == "tool_end" {
				if d, ok := ev.Data.(service.SSEToolEndData); ok {
					toolEndErr = d.Error
				}
			}
			if ev.Type == "done" || ev.Type == "error" {
				break loop
			}
		case <-timeout:
			t.Fatal("Send() with denied tool timed out")
		}
	}

	if toolEndErr == nil {
		t.Error("expected tool_end.Error to be non-nil for denied tool, got nil")
	}
}

// ---- Answer unblocks askPermission -----------------------------------------

func TestAnswer_unblocks_granted(t *testing.T) {
	ctx := context.Background()

	fp := &fakeToolProvider{
		id: "fake",
		chunks: [][]provider.StreamChunk{
			{
				{ToolCalls: []provider.ToolCall{{ID: "tc1", Name: "bash", Arguments: `{"command":"rm -rf temp"}`}}, FinishReason: "tool_calls"},
			},
			{
				{Delta: "ok", FinishReason: "stop"},
			},
		},
	}

	toolReg := tool.NewRegistry()
	toolReg.Register(&tool.Tool{
		Name:        "bash",
		Description: "run shell",
		Parameters:  []byte(`{}`),
		Execute: func(_ context.Context, ectx tool.ExecutionContext, args []byte) (*tool.Result, error) {
			// The Ask function blocks until Answer is called.
			if ectx.Ask != nil {
				if err := ectx.Ask("bash", string(args)); err != nil {
					return nil, err
				}
			}
			return &tool.Result{Output: "executed"}, nil
		},
	})

	agentSvc := &fakeAgentLookup{cfg: config.AgentConfig{
		Tools: []string{"bash"},
		Permission: permission.Config{
			"bash": {Action: permission.ActionAsk},
		},
	}}

	svc := newToolDispatchService(t, fp, testCfg(), toolReg, agentSvc)

	sess, _ := svc.Create(ctx, "default", "fake/fake-model")
	sub, cancel := svc.Subscribe(sess.ID)
	defer cancel()

	// Run Send in the background. It will block while waiting for Answer.
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Send(ctx, sess.ID, "run bash", nil, sandbox.OriginTUI)
	}()

	// Wait for the "question" event, then answer it.
	var questionID string
	timeout := time.After(2 * time.Second)
waitQ:
	for {
		select {
		case ev, ok := <-sub:
			if !ok {
				break waitQ
			}
			if ev.Type == "question" {
				if d, ok := ev.Data.(service.SSEQuestionData); ok {
					questionID = d.QuestionID
				}
				break waitQ
			}
		case <-timeout:
			t.Fatal("timed out waiting for question event")
		}
	}

	if questionID == "" {
		t.Fatal("expected a non-empty question_id in question event")
	}

	// Grant permission (once).
	if err := svc.Answer(ctx, sess.ID, questionID, service.AnswerOnce); err != nil {
		t.Fatalf("Answer() error = %v", err)
	}

	// Send should complete without error.
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Send() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Send() did not complete after Answer(granted=true)")
	}
}

func TestAnswer_unblocks_denied(t *testing.T) {
	ctx := context.Background()

	fp := &fakeToolProvider{
		id: "fake",
		chunks: [][]provider.StreamChunk{
			{
				{ToolCalls: []provider.ToolCall{{ID: "tc1", Name: "bash", Arguments: `{"command":"rm -rf temp"}`}}, FinishReason: "tool_calls"},
			},
			{
				{Delta: "ok", FinishReason: "stop"},
			},
		},
	}

	toolReg := tool.NewRegistry()
	toolReg.Register(&tool.Tool{
		Name:        "bash",
		Description: "run shell",
		Parameters:  []byte(`{}`),
		Execute: func(_ context.Context, ectx tool.ExecutionContext, args []byte) (*tool.Result, error) {
			// The Ask function blocks until Answer is called.
			if ectx.Ask != nil {
				if err := ectx.Ask("bash", string(args)); err != nil {
					return nil, err
				}
			}
			return &tool.Result{Output: "executed"}, nil
		},
	})

	agentSvc := &fakeAgentLookup{cfg: config.AgentConfig{
		Tools: []string{"bash"},
		Permission: permission.Config{
			"bash": {Action: permission.ActionAsk},
		},
	}}

	svc := newToolDispatchService(t, fp, testCfg(), toolReg, agentSvc)

	sess, _ := svc.Create(ctx, "default", "fake/fake-model")
	sub, cancel := svc.Subscribe(sess.ID)
	defer cancel()

	// Run Send in the background. It will block while waiting for Answer.
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Send(ctx, sess.ID, "run bash", nil, sandbox.OriginTUI)
	}()

	// Wait for the "question" event, then answer it.
	var questionID string
	timeout := time.After(2 * time.Second)
waitQ:
	for {
		select {
		case ev, ok := <-sub:
			if !ok {
				break waitQ
			}
			if ev.Type == "question" {
				if d, ok := ev.Data.(service.SSEQuestionData); ok {
					questionID = d.QuestionID
				}
				break waitQ
			}
		case <-timeout:
			t.Fatal("timed out waiting for question event")
		}
	}

	if questionID == "" {
		t.Fatal("expected a non-empty question_id in question event")
	}

	// Deny permission.
	if err := svc.Answer(ctx, sess.ID, questionID, service.AnswerReject); err != nil {
		t.Fatalf("Answer() error = %v", err)
	}

	// Send should complete (tool error is logged as tool_end, not a Send() error).
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Send() error = %v, want nil (tool errors surfaced via tool_end event)", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Send() did not complete after Answer(granted=false)")
	}
}

// ---- fakeAgentLookup -------------------------------------------------------

type fakeAgentLookup struct {
	cfg config.AgentConfig
}

func (f *fakeAgentLookup) Get(_ string) (config.AgentConfig, error) {
	return f.cfg, nil
}

// Ensure ErrDenied is accessible for test assertions.
var _ = errors.Is(tool.ErrDenied, tool.ErrDenied)

// ---- Session-scoped always-allow -------------------------------------------

func TestAnswerAlways_storesPatternAndSkipsFutureQuestions(t *testing.T) {
	ctx := context.Background()

	// Provider: first call returns tool_calls; second returns tool_calls again;
	// third returns stop.
	fp := &fakeToolProvider{
		id: "fake",
		chunks: [][]provider.StreamChunk{
			{
				{ToolCalls: []provider.ToolCall{{ID: "tc1", Name: "bash", Arguments: `{"command":"echo hello"}`}}, FinishReason: "tool_calls"},
			},
			{
				{ToolCalls: []provider.ToolCall{{ID: "tc2", Name: "bash", Arguments: `{"command":"echo hello"}`}}, FinishReason: "tool_calls"},
			},
			{
				{Delta: "ok", FinishReason: "stop"},
			},
		},
	}

	toolReg := tool.NewRegistry()
	toolReg.Register(&tool.Tool{
		Name:        "bash",
		Description: "run shell",
		Parameters:  []byte(`{"type":"object","properties":{"command":{"type":"string"}}}`),
		Execute: func(_ context.Context, ectx tool.ExecutionContext, args []byte) (*tool.Result, error) {
			if ectx.Ask != nil {
				if err := ectx.Ask("bash", string(args)); err != nil {
					return nil, err
				}
			}
			return &tool.Result{Output: "executed"}, nil
		},
	})

	agentSvc := &fakeAgentLookup{cfg: config.AgentConfig{
		Tools: []string{"bash"},
		Permission: permission.Config{
			"bash": {Action: permission.ActionAsk},
		},
	}}

	svc := newToolDispatchService(t, fp, testCfg(), toolReg, agentSvc)

	sess, _ := svc.Create(ctx, "default", "fake/fake-model")
	sub, cancel := svc.Subscribe(sess.ID)
	defer cancel()

	// Run Send in the background. It will block while waiting for Answer.
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Send(ctx, sess.ID, "run bash", nil, sandbox.OriginTUI)
	}()

	// Wait for the first "question" event.
	var questionID string
	timeout := time.After(2 * time.Second)
waitQ1:
	for {
		select {
		case ev, ok := <-sub:
			if !ok {
				break waitQ1
			}
			if ev.Type == "question" {
				if d, ok := ev.Data.(service.SSEQuestionData); ok {
					questionID = d.QuestionID
					if d.SuggestedPattern != "echo hello" {
						t.Errorf("SuggestedPattern = %q, want %q", d.SuggestedPattern, "echo hello")
					}
				}
				break waitQ1
			}
		case <-timeout:
			t.Fatal("timed out waiting for first question event")
		}
	}

	if questionID == "" {
		t.Fatal("expected a non-empty question_id in first question event")
	}

	// Grant permission with "always".
	if err := svc.Answer(ctx, sess.ID, questionID, service.AnswerAlways); err != nil {
		t.Fatalf("Answer() error = %v", err)
	}

	// Wait for the second tool call with the same command. It should not produce a question.
	var questionCount int
	var toolEndCount int
waitDone:
	for {
		select {
		case ev, ok := <-sub:
			if !ok {
				break waitDone
			}
			if ev.Type == "question" {
				questionCount++
			}
			if ev.Type == "tool_end" {
				toolEndCount++
			}
			if ev.Type == "done" || ev.Type == "error" {
				break waitDone
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for Send to complete")
		}
	}

	// Should have 0 additional questions (the first was answered before we started counting).
	if questionCount != 0 {
		t.Errorf("question count after AnswerAlways = %d, want 0 (should skip asking)", questionCount)
	}
	// Should have 2 tool_end events (both tool calls executed).
	if toolEndCount != 2 {
		t.Errorf("tool_end count = %d, want 2", toolEndCount)
	}

	// Send should complete without error.
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Send() error = %v, want nil", err)
		}
	default:
		// Already completed
	}
}

func TestAnswerOnce_doesNotStorePattern(t *testing.T) {
	ctx := context.Background()

	// Provider: first call returns tool_calls; second returns tool_calls again;
	// third returns stop.
	fp := &fakeToolProvider{
		id: "fake",
		chunks: [][]provider.StreamChunk{
			{
				{ToolCalls: []provider.ToolCall{{ID: "tc1", Name: "bash", Arguments: `{"command":"echo hello"}`}}, FinishReason: "tool_calls"},
			},
			{
				{ToolCalls: []provider.ToolCall{{ID: "tc2", Name: "bash", Arguments: `{"command":"echo hello"}`}}, FinishReason: "tool_calls"},
			},
			{
				{Delta: "ok", FinishReason: "stop"},
			},
		},
	}

	toolReg := tool.NewRegistry()
	toolReg.Register(&tool.Tool{
		Name:        "bash",
		Description: "run shell",
		Parameters:  []byte(`{"type":"object","properties":{"command":{"type":"string"}}}`),
		Execute: func(_ context.Context, ectx tool.ExecutionContext, args []byte) (*tool.Result, error) {
			if ectx.Ask != nil {
				if err := ectx.Ask("bash", string(args)); err != nil {
					return nil, err
				}
			}
			return &tool.Result{Output: "executed"}, nil
		},
	})

	agentSvc := &fakeAgentLookup{cfg: config.AgentConfig{
		Tools: []string{"bash"},
		Permission: permission.Config{
			"bash": {Action: permission.ActionAsk},
		},
	}}

	svc := newToolDispatchService(t, fp, testCfg(), toolReg, agentSvc)

	sess, _ := svc.Create(ctx, "default", "fake/fake-model")
	sub, cancel := svc.Subscribe(sess.ID)
	defer cancel()

	// Run Send in the background. It will block while waiting for Answer.
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Send(ctx, sess.ID, "run bash", nil, sandbox.OriginTUI)
	}()

	// Wait for the first "question" event, answer with "once".
	var questionID string
	timeout := time.After(2 * time.Second)
waitQ1:
	for {
		select {
		case ev, ok := <-sub:
			if !ok {
				break waitQ1
			}
			if ev.Type == "question" {
				if d, ok := ev.Data.(service.SSEQuestionData); ok {
					questionID = d.QuestionID
				}
				break waitQ1
			}
		case <-timeout:
			t.Fatal("timed out waiting for first question event")
		}
	}

	if questionID == "" {
		t.Fatal("expected a non-empty question_id in first question event")
	}

	// Grant permission with "once" (should NOT store pattern).
	if err := svc.Answer(ctx, sess.ID, questionID, service.AnswerOnce); err != nil {
		t.Fatalf("Answer() error = %v", err)
	}

	// Wait for the second tool call with the same command. It should produce another question.
	var questionIDs []string
	var toolEndCount int
waitDone:
	for {
		select {
		case ev, ok := <-sub:
			if !ok {
				break waitDone
			}
			if ev.Type == "question" {
				if d, ok := ev.Data.(service.SSEQuestionData); ok {
					questionIDs = append(questionIDs, d.QuestionID)
					// Auto-approve the second question so the loop can continue
					go func(qid string) {
						time.Sleep(10 * time.Millisecond)
						_ = svc.Answer(ctx, sess.ID, qid, service.AnswerOnce)
					}(d.QuestionID)
				}
			}
			if ev.Type == "tool_end" {
				toolEndCount++
			}
			if ev.Type == "done" || ev.Type == "error" {
				break waitDone
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for Send to complete")
		}
	}

	// Should have 1 additional question (because "once" doesn't store pattern).
	if len(questionIDs) != 1 {
		t.Errorf("question count after AnswerOnce = %d, want 1 (should ask again)", len(questionIDs))
	}
	// Should have 2 tool_end events (both tool calls executed).
	if toolEndCount != 2 {
		t.Errorf("tool_end count = %d, want 2", toolEndCount)
	}

	// Send should complete without error.
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Send() error = %v, want nil", err)
		}
	default:
		// Already completed
	}
}

func TestBashExternalPermissionQuestionUsesResolvedPathAndAnswerAlwaysSkipsRepeat(t *testing.T) {
	ctx := context.Background()

	externalFile := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(externalFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolvedExternalFile := externalFile
	if resolved, err := filepath.EvalSymlinks(externalFile); err == nil {
		resolvedExternalFile = resolved
	}

	fp := &fakeToolProvider{
		id: "fake",
		chunks: [][]provider.StreamChunk{
			{
				{ToolCalls: []provider.ToolCall{{ID: "tc1", Name: "bash", Arguments: fmt.Sprintf(`{"command":"cat %s"}`, externalFile)}}, FinishReason: "tool_calls"},
			},
			{
				{ToolCalls: []provider.ToolCall{{ID: "tc2", Name: "bash", Arguments: fmt.Sprintf(`{"command":"cat %s"}`, externalFile)}}, FinishReason: "tool_calls"},
			},
			{
				{Delta: "ok", FinishReason: "stop"},
			},
		},
	}

	toolReg := tool.NewRegistry()
	toolReg.Register(&tool.Tool{
		Name:        "bash",
		Description: "run shell",
		Parameters:  []byte(`{"type":"object","properties":{"command":{"type":"string"}}}`),
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			return &tool.Result{Output: "executed"}, nil
		},
	})

	svc := newToolDispatchService(t, fp, testCfg(), toolReg, nil)

	sess, _ := svc.Create(ctx, "default", "fake/fake-model")
	sub, cancel := svc.Subscribe(sess.ID)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Send(ctx, sess.ID, "run bash", nil, sandbox.OriginTUI)
	}()

	var questionID string
	timeout := time.After(2 * time.Second)
waitQ1:
	for {
		select {
		case ev, ok := <-sub:
			if !ok {
				break waitQ1
			}
			if ev.Type == "question" {
				if d, ok := ev.Data.(service.SSEQuestionData); ok {
					questionID = d.QuestionID
					if !strings.Contains(d.Input, "permission_display") {
						t.Fatalf("question input should include sandbox permission display metadata, got %q", d.Input)
					}
					if !strings.Contains(d.Input, resolvedExternalFile) {
						t.Fatalf("question input should include full external path, got %q", d.Input)
					}
					if d.SuggestedPattern != "external:"+filepath.ToSlash(resolvedExternalFile) {
						t.Fatalf("SuggestedPattern = %q, want %q", d.SuggestedPattern, "external:"+filepath.ToSlash(resolvedExternalFile))
					}
				}
				break waitQ1
			}
		case <-timeout:
			t.Fatal("timed out waiting for first question event")
		}
	}

	if questionID == "" {
		t.Fatal("expected a non-empty question_id in first question event")
	}

	if err := svc.Answer(ctx, sess.ID, questionID, service.AnswerAlways); err != nil {
		t.Fatalf("Answer() error = %v", err)
	}

	var questionCount int
waitDone:
	for {
		select {
		case ev, ok := <-sub:
			if !ok {
				break waitDone
			}
			if ev.Type == "question" {
				questionCount++
			}
			if ev.Type == "done" || ev.Type == "error" {
				break waitDone
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for Send to complete")
		}
	}

	if questionCount != 0 {
		t.Fatalf("question count after AnswerAlways = %d, want 0", questionCount)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Send() error = %v, want nil", err)
		}
	default:
	}
}

func TestBackgroundDoneTriggersServiceLevelAutoReact(t *testing.T) {
	ctx := context.Background()
	cfg := testCfg()
	cfg.Session.BackgroundAutoReact = boolPtr(true)

	fp := &fakeToolProvider{
		id: "fake",
		chunks: [][]provider.StreamChunk{
			{
				{ToolCalls: []provider.ToolCall{{
					ID:        "tc1",
					Name:      "bash",
					Arguments: `{"command":"printf 'background complete\\n'","description":"Run background task","purpose":"other","run_in_background":true}`,
				}}, FinishReason: "tool_calls"},
			},
			{
				{Delta: "launched", FinishReason: "stop"},
			},
			{
				{Delta: "reacted", FinishReason: "stop"},
			},
		},
	}

	toolReg := tool.NewRegistry()
	toolReg.Register(tool.NewBashTool())
	toolReg.Register(tool.NewTaskOutputTool())

	svc := newToolDispatchService(t, fp, cfg, toolReg, nil)
	sess, _ := svc.Create(ctx, "default", "fake/fake-model")

	if err := svc.Send(ctx, sess.ID, "run a background task", nil, sandbox.OriginTUI); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		inputs := fp.Inputs()
		if len(inputs) >= 3 {
			if !strings.Contains(inputs[2], "Background task bg-") {
				t.Fatalf("auto-react input = %q, want background task prompt", inputs[2])
			}
			if !strings.Contains(inputs[2], "call `task_output`") {
				t.Fatalf("auto-react input = %q, want task_output guidance", inputs[2])
			}
			msgs, err := svc.ListMessages(ctx, sess.ID)
			if err != nil {
				t.Fatalf("ListMessages() error = %v", err)
			}
			var foundBackgroundEvent bool
			var foundSyntheticPrompt bool
			for _, msg := range msgs {
				for _, part := range msg.Parts {
					if part.Type == "background_event" && strings.Contains(part.Content, `"task_id":"bg-`) {
						foundBackgroundEvent = true
					}
					if part.Synthetic && part.Type == "text" && strings.Contains(part.Content, "Background task") {
						foundSyntheticPrompt = true
					}
				}
			}
			if !foundBackgroundEvent {
				t.Fatal("background completion was not persisted as a background_event message part")
			}
			if foundSyntheticPrompt {
				t.Fatal("background auto-react prompt should not be persisted as a synthetic transcript message")
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("provider saw %d chat calls, want 3 including background auto-react", len(fp.Inputs()))
}
