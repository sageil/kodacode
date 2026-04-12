package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/permission"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/sandbox"
	"github.com/sageil/kodacode/v1/internal/tool"
)

func newDispatchTestLoop(t *testing.T, reg *tool.Registry) *turnLoop {
	t.Helper()
	var tools []provider.Tool
	for _, name := range reg.Names() {
		tools = append(tools, provider.Tool{Name: name})
	}
	return &turnLoop{
		ctx: context.Background(),
		req: &pipeline.TurnRequest{
			SessionID: "dispatch-test",
			Model:     provider.Model{ID: "test", ContextSize: 128000},
			Tools:     tools,
		},
		publish:   func(string, SSEEvent) {},
		sndbx:     sandbox.New(t.TempDir(), sandbox.OriginTUI, reg, permission.Config{}),
		toolCache: newToolResultCache(),
	}
}

func TestDispatchToolCalls_ReadOnlyDedup(t *testing.T) {
	var execCount atomic.Int32
	reg := tool.NewRegistry()
	reg.Register(&tool.Tool{
		Name:        "read",
		Description: "Read a file",
		Parameters:  []byte(`{"type":"object"}`),
		ReadOnly:    true,
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			execCount.Add(1)
			return &tool.Result{Output: "file contents"}, nil
		},
	})

	tl := newDispatchTestLoop(t, reg)
	calls := []provider.ToolCall{
		{ID: "c1", Name: "read", Arguments: `{"path":"foo.go"}`},
		{ID: "c2", Name: "read", Arguments: `{"path":"foo.go"}`},
	}

	results := tl.dispatchToolCalls(calls)
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if got := execCount.Load(); got != 1 {
		t.Errorf("read-only tool executed %d times, want 1 (should be deduped)", got)
	}
	if results[0].output != results[1].output {
		t.Errorf("deduped results differ: %q vs %q", results[0].output, results[1].output)
	}
}

func TestDispatchToolCalls_SideEffectingNotDeduped(t *testing.T) {
	var execCount atomic.Int32
	reg := tool.NewRegistry()
	reg.Register(&tool.Tool{
		Name:        "bash",
		Description: "Run a shell command",
		Parameters:  []byte(`{"type":"object"}`),
		ReadOnly:    false,
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			execCount.Add(1)
			return &tool.Result{Output: "ok"}, nil
		},
	})

	tl := newDispatchTestLoop(t, reg)
	calls := []provider.ToolCall{
		{ID: "c1", Name: "bash", Arguments: `{"cmd":"echo hello"}`},
		{ID: "c2", Name: "bash", Arguments: `{"cmd":"echo hello"}`},
	}

	results := tl.dispatchToolCalls(calls)
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if got := execCount.Load(); got != 2 {
		t.Errorf("side-effecting tool executed %d times, want 2 (must not be deduped)", got)
	}
}

func TestDispatchToolCalls_MixedReadOnlyAndSideEffecting(t *testing.T) {
	var readCount, bashCount atomic.Int32
	reg := tool.NewRegistry()
	reg.Register(&tool.Tool{
		Name:        "read",
		Description: "Read a file",
		Parameters:  []byte(`{"type":"object"}`),
		ReadOnly:    true,
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			readCount.Add(1)
			return &tool.Result{Output: "contents"}, nil
		},
	})
	reg.Register(&tool.Tool{
		Name:        "write",
		Description: "Write a file",
		Parameters:  []byte(`{"type":"object"}`),
		ReadOnly:    false,
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			bashCount.Add(1)
			return &tool.Result{Output: "written"}, nil
		},
	})

	tl := newDispatchTestLoop(t, reg)
	calls := []provider.ToolCall{
		{ID: "c1", Name: "read", Arguments: `{"path":"a.go"}`},
		{ID: "c2", Name: "read", Arguments: `{"path":"a.go"}`},
		{ID: "c3", Name: "write", Arguments: `{"path":"a.go","content":"x"}`},
		{ID: "c4", Name: "write", Arguments: `{"path":"a.go","content":"x"}`},
	}

	results := tl.dispatchToolCalls(calls)
	if len(results) != 4 {
		t.Fatalf("len(results) = %d, want 4", len(results))
	}
	if got := readCount.Load(); got != 1 {
		t.Errorf("read executed %d times, want 1 (should be deduped)", got)
	}
	if got := bashCount.Load(); got != 2 {
		t.Errorf("write executed %d times, want 2 (must not be deduped)", got)
	}
}

func TestDispatchToolCalls_BashInvalidatesReadCache(t *testing.T) {
	var readCount, bashCount atomic.Int32
	reg := tool.NewRegistry()
	reg.Register(&tool.Tool{
		Name:        "read",
		Description: "Read a file",
		Parameters:  []byte(`{"type":"object"}`),
		ReadOnly:    true,
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			readCount.Add(1)
			return &tool.Result{Output: "contents"}, nil
		},
	})
	reg.Register(&tool.Tool{
		Name:        "bash",
		Description: "Run a shell command",
		Parameters:  []byte(`{"type":"object"}`),
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			bashCount.Add(1)
			return &tool.Result{Output: "ok"}, nil
		},
	})

	tl := newDispatchTestLoop(t, reg)

	tl.dispatchToolCalls([]provider.ToolCall{{ID: "r1", Name: "read", Arguments: `{"path":"a.go"}`}})
	tl.dispatchToolCalls([]provider.ToolCall{{ID: "r2", Name: "read", Arguments: `{"path":"a.go"}`}})
	if got := readCount.Load(); got != 1 {
		t.Fatalf("read executed %d times before bash, want cached reuse", got)
	}

	tl.dispatchToolCalls([]provider.ToolCall{{ID: "b1", Name: "bash", Arguments: `{"command":"echo hi"}`}})
	if got := bashCount.Load(); got != 1 {
		t.Fatalf("bash executed %d times, want 1", got)
	}

	tl.dispatchToolCalls([]provider.ToolCall{{ID: "r3", Name: "read", Arguments: `{"path":"a.go"}`}})
	if got := readCount.Load(); got != 2 {
		t.Fatalf("read executed %d times after bash, want cache invalidated", got)
	}
}

func TestDispatchToolCalls_BlocksPlannerInSameBatchAsExplorer(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(tool.NewSubagentTool(nil))

	tl := newDispatchTestLoop(t, reg)
	tl.req.AgentID = "engineer"

	var spawnCount atomic.Int32
	tl.spawnSubagent = func(_ context.Context, _ string, agentID, _ string, _ ProgressFunc) (string, error) {
		spawnCount.Add(1)
		return "ran " + agentID, nil
	}

	results := tl.dispatchToolCalls([]provider.ToolCall{
		{ID: "explorer-1", Name: "subagent", Arguments: `{"agent_id":"explorer","task":"inspect"}`},
		{ID: "planner-1", Name: "subagent", Arguments: `{"agent_id":"planner","task":"make a plan"}`},
	})

	if got := spawnCount.Load(); got != 1 {
		t.Fatalf("subagent spawn count = %d, want 1 (explorer only)", got)
	}
	if results[0].errStr != nil {
		t.Fatalf("explorer should be allowed, got err %v", *results[0].errStr)
	}
	if results[1].errStr == nil {
		t.Fatalf("planner should be blocked in same batch, got %+v", results[1])
	}
	if !strings.Contains(results[1].output, "same response as explorer") {
		t.Fatalf("unexpected planner block output: %q", results[1].output)
	}
}

func TestDispatchToolCalls_BlocksPatchWithPhaseSpecificGuidance(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(tool.NewPatchTool())

	tl := newDispatchTestLoop(t, reg)
	tl.req.AgentID = "engineer"
	tl.req.PhaseFilterActive = true
	tl.req.Tools = []provider.Tool{{Name: "read"}}
	tl.req.FullTools = []provider.Tool{{Name: "read"}, {Name: "patch"}}
	tl.req.Workflow = &pipeline.WorkflowState{Phase: pipeline.WorkflowPhasePreplan}

	results := tl.dispatchToolCalls([]provider.ToolCall{{
		ID:        "patch-1",
		Name:      "patch",
		Arguments: `{"filePath":"a.go","edits":[{"oldString":"x","newString":"y"}]}`,
	}})

	if len(results) != 1 || results[0].errStr == nil {
		t.Fatalf("expected blocked patch error result, got %+v", results)
	}
	if !strings.Contains(results[0].output, "blocked in the current preplan phase") {
		t.Fatalf("unexpected patch block output: %q", results[0].output)
	}
	if !strings.Contains(results[0].output, "Planning is still in progress") {
		t.Fatalf("unexpected patch guidance: %q", results[0].output)
	}
}

func TestDispatchToolCalls_InjectsSkillPolicyIntoSandboxExecution(t *testing.T) {
	skillsDir := t.TempDir()
	skillDir := filepath.Join(skillsDir, "secret-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: secret-skill\ndescription: Hidden skill\ntriggers:\n  - secret workflow\n---\n# Secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := tool.NewRegistry()
	reg.Register(tool.NewSearchSkillsTool())

	tl := &turnLoop{
		ctx: context.Background(),
		req: &pipeline.TurnRequest{
			SessionID:  "dispatch-test",
			ProviderID: "fake",
			Model:      provider.Model{ID: "fake-model", ContextSize: 128000},
			Agent:      config.AgentConfig{Tools: []string{"search_skills"}},
			Tools:      []provider.Tool{{Name: "search_skills"}},
		},
		publish:   func(string, SSEEvent) {},
		sndbx:     sandbox.New(t.TempDir(), sandbox.OriginTUI, reg, permission.Config{}, sandbox.SandboxOptions{SkillDirs: []string{skillsDir}}),
		toolCache: newToolResultCache(),
		globalCfg: &config.Config{
			Skills: config.GlobalSkillsConfig{
				Models: map[string]config.ModelSkillsConfig{
					"fake/fake-model": {Deny: []string{"secret-skill"}},
				},
			},
		},
	}

	results := tl.dispatchToolCalls([]provider.ToolCall{{
		ID:        "skill-search-1",
		Name:      "search_skills",
		Arguments: `{"query":"secret workflow"}`,
	}})

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if !strings.Contains(results[0].output, "No skills matched") {
		t.Fatalf("search output = %q, want denied skill to be hidden", results[0].output)
	}
}

func TestDispatchToolCalls_AllowsMemoryWithoutAgentToolConfig(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(&tool.Tool{
		Name:       "memory",
		Parameters: []byte(`{"type":"object"}`),
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			return &tool.Result{Output: "Memory saved."}, nil
		},
	})

	tl := newDispatchTestLoop(t, reg)
	tl.req.Tools = nil

	results := tl.dispatchToolCalls([]provider.ToolCall{{
		ID:        "memory-1",
		Name:      "memory",
		Arguments: `{"action":"list"}`,
	}})

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].errStr != nil {
		t.Fatalf("memory tool should be allowed globally, got err %v", *results[0].errStr)
	}
	if results[0].output != "Memory saved." {
		t.Fatalf("memory output = %q, want %q", results[0].output, "Memory saved.")
	}
}
