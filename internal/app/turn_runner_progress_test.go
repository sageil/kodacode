package app

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func TestRepeatedToolLoopStateDetectsAlternatingRepeatsWithinWindow(t *testing.T) {
	state := repeatedToolLoopState{}
	for idx := 0; idx < 5; idx++ {
		state = nextRepeatedToolLoopState(state, assistantRoundtripResult{
			Outcome:             assistantRoundtripOutcomeToolResult,
			ToolInteractionSigs: []string{"read-a"},
		})
		if idx < 4 && state.Repeated() {
			t.Fatalf("state repeated too early after read-a #%d: %#v", idx+1, state)
		}
		state = nextRepeatedToolLoopState(state, assistantRoundtripResult{
			Outcome:             assistantRoundtripOutcomeToolResult,
			ToolInteractionSigs: []string{"read-b"},
		})
	}
	if !state.Repeated() {
		t.Fatalf("state did not detect alternating repeated tools: %#v", state)
	}
}

func TestProviderStepExplorationTargetsReadKeepsOffsetWindows(t *testing.T) {
	first, ok := providerStepExplorationTargets("read", `{"paths":["app.go"]}`)
	if !ok {
		t.Fatal("first target missing")
	}
	second, ok := providerStepExplorationTargets("read", `{"paths":["app.go"],"offset":19,"limit":5}`)
	if !ok {
		t.Fatal("second target missing")
	}
	if slices.Equal(first, second) {
		t.Fatalf("line-window read targets should differ: %v vs %v", first, second)
	}
	if len(first) != 1 || !slices.Contains(first, "read:path=app.go:offset=0:limit=1000") {
		t.Fatalf("implicit single-file read target = %v", first)
	}
}

func TestProviderStepExplorationTargetsLocateKeepsMaterialOptions(t *testing.T) {
	defaultTarget, ok := providerStepExplorationTargets("locate", `{"query":"*.go","path":"src"}`)
	if !ok {
		t.Fatal("default target missing")
	}
	hiddenTarget, ok := providerStepExplorationTargets("locate", `{"query":"*.go","path":"src","include_hidden":true}`)
	if !ok {
		t.Fatal("hidden target missing")
	}
	if slices.Equal(defaultTarget, hiddenTarget) {
		t.Fatalf("locate include_hidden should change target: %v vs %v", defaultTarget, hiddenTarget)
	}
}

func TestProviderStepExplorationTargetsSearchKeepsImplicitAutoModeDistinctFromLexical(t *testing.T) {
	implicit, ok := providerStepExplorationTargets("search", `{"path":".","query":"TODO"}`)
	if !ok {
		t.Fatal("implicit search target missing")
	}
	explicit, ok := providerStepExplorationTargets("search", `{"path":".","query":"TODO","mode":"lexical"}`)
	if !ok {
		t.Fatal("explicit search target missing")
	}
	if slices.Equal(implicit, explicit) {
		t.Fatalf("search auto targets should differ from explicit lexical: %v vs %v", implicit, explicit)
	}
	if len(implicit) != 1 || !slices.Contains(implicit, "search:path=.:query=TODO:mode=auto:glob=:regex=false:case_sensitive=false") {
		t.Fatalf("implicit search target = %v", implicit)
	}
}

func TestProviderStepBatchableToolName(t *testing.T) {
	for _, name := range []string{
		tool.ReadToolName,
		tool.LocateToolName,
		tool.SearchToolName,
		tool.DefinitionToolName,
		tool.DiagnosticsToolName,
		tool.SymbolsToolName,
		"git_status",
		"git_diff",
		"git_show",
	} {
		if !providerStepBatchableToolName(name) {
			t.Fatalf("%s should be batchable", name)
		}
	}
	for _, name := range []string{
		"write",
		tool.ApplyPatchToolName,
		tool.BashToolName,
		tool.QuestionToolName,
		tool.TestToolName,
		tool.WebFetchToolName,
		tool.TaskWorkflowToolName,
		tool.TaskReviewToolName,
	} {
		if providerStepBatchableToolName(name) {
			t.Fatalf("%s should not be batchable", name)
		}
	}
}

func TestProviderStepBatchableToolCall(t *testing.T) {
	for _, test := range []struct {
		name      string
		args      string
		batchable bool
	}{
		{name: tool.MemoryToolName, args: `{"action":"list","content":null,"id":null}`, batchable: true},
		{name: tool.MemoryToolName, args: `{"action":"save","content":"note","id":null}`, batchable: false},
		{name: tool.BashToolName, args: `{"cmd":"rg \"TODO\" internal"}`, batchable: true},
		{name: tool.BashToolName, args: `{"cmd":"npm install"}`, batchable: false},
		{name: tool.TaskWorkflowToolName, args: `{"action":"list"}`, batchable: true},
		{name: tool.TaskWorkflowToolName, args: `{"action":"create","title":"Do work"}`, batchable: false},
		{name: tool.TaskWorkflowToolName, args: `{"action":"update","task_id":"task-1","status":"in_progress"}`, batchable: false},
		{name: tool.TaskReviewToolName, args: `{"action":"list"}`, batchable: true},
		{name: tool.TaskReviewToolName, args: `{"action":"review","task_id":"task-1","review_status":"pass","review_summary":"ok"}`, batchable: false},
	} {
		if got := providerStepBatchableToolCall(test.name, test.args); got != test.batchable {
			t.Fatalf("providerStepBatchableToolCall(%q, %s) = %v, want %v", test.name, test.args, got, test.batchable)
		}
	}
	if providerStepPotentiallyBatchableToolName(tool.MemoryToolName) == false {
		t.Fatal("memory should be potentially batchable before full arguments are known")
	}
	if providerStepPotentiallyBatchableToolName(tool.BashToolName) == false {
		t.Fatal("bash should be potentially batchable before full arguments are known")
	}
}

func TestProviderStepParallelReadToolCall(t *testing.T) {
	for _, test := range []struct {
		name         string
		args         string
		parallelRead bool
	}{
		{name: tool.ReadToolName, args: `{"paths":["app.go"]}`, parallelRead: true},
		{name: tool.SearchToolName, args: `{"path":".","query":"TODO"}`, parallelRead: true},
		{name: "git_diff", args: `{"staged":false}`, parallelRead: true},
		{name: tool.BashToolName, args: `{"cmd":"grep -rn \"TODO\" internal | head -20"}`, parallelRead: true},
		{name: tool.BashToolName, args: `{"cmd":"npm install"}`, parallelRead: false},
		{name: tool.MemoryToolName, args: `{"action":"list"}`, parallelRead: true},
		{name: tool.MemoryToolName, args: `{"action":"save","content":"note"}`, parallelRead: false},
		{name: tool.TaskWorkflowToolName, args: `{"action":"list"}`, parallelRead: true},
		{name: tool.TaskWorkflowToolName, args: `{"action":"create","title":"Do work"}`, parallelRead: false},
		{name: tool.TaskReviewToolName, args: `{"action":"review","task_id":"task-1","review_status":"pass"}`, parallelRead: false},
		{name: tool.ApplyPatchToolName, args: ``, parallelRead: false},
		{name: tool.QuestionToolName, args: `{"question":"Proceed?"}`, parallelRead: false},
	} {
		if got := providerStepParallelReadToolCall(test.name, test.args); got != test.parallelRead {
			t.Fatalf("providerStepParallelReadToolCall(%q, %s) = %v, want %v", test.name, test.args, got, test.parallelRead)
		}
	}
}

func TestProviderStepToolExecutionClass(t *testing.T) {
	for _, test := range []struct {
		name  string
		args  string
		class toolExecutionClass
	}{
		{name: tool.ReadToolName, args: `{"paths":["app.go"]}`, class: toolExecutionParallelRead},
		{name: "git_status", args: `{}`, class: toolExecutionParallelRead},
		{name: tool.BashToolName, args: `{"cmd":"cat go.mod"}`, class: toolExecutionParallelRead},
		{name: tool.MemoryToolName, args: `{"action":"list"}`, class: toolExecutionParallelRead},
		{name: tool.MemoryToolName, args: `{"action":"save","content":"note"}`, class: toolExecutionSequential},
		{name: tool.TaskWorkflowToolName, args: `{"action":"list"}`, class: toolExecutionParallelRead},
		{name: tool.TaskWorkflowToolName, args: `{"action":"create","title":"Do work"}`, class: toolExecutionSequential},
		{name: tool.TaskReviewToolName, args: `{"action":"review","task_id":"task-1","review_status":"pass"}`, class: toolExecutionSequential},
		{name: tool.QuestionToolName, args: `{"question":"Proceed?"}`, class: toolExecutionBlocking},
		{name: tool.ApplyPatchToolName, args: ``, class: toolExecutionSequential},
		{name: tool.BashToolName, args: `{"cmd":"printf hi"}`, class: toolExecutionParallelRead},
		{name: tool.BashToolName, args: `{"cmd":"npm install"}`, class: toolExecutionSequential},
	} {
		if got := providerStepToolExecutionClass(test.name, test.args); got != test.class {
			t.Fatalf("providerStepToolExecutionClass(%q, %s) = %q, want %q", test.name, test.args, got, test.class)
		}
	}
}

func TestProviderStepToolSchedulingCapability(t *testing.T) {
	for _, test := range []struct {
		name         string
		args         string
		class        toolExecutionClass
		parallelSafe bool
	}{
		{name: tool.ReadToolName, args: `{"paths":["app.go"]}`, class: toolExecutionParallelRead, parallelSafe: true},
		{name: "git_status", args: `{}`, class: toolExecutionParallelRead, parallelSafe: true},
		{name: tool.MemoryToolName, args: `{"action":"list"}`, class: toolExecutionParallelRead, parallelSafe: true},
		{name: tool.MemoryToolName, args: `{"action":"save","content":"note"}`, class: toolExecutionSequential, parallelSafe: false},
		{name: tool.TaskWorkflowToolName, args: `{"action":"list"}`, class: toolExecutionParallelRead, parallelSafe: true},
		{name: tool.TaskWorkflowToolName, args: `{"action":"create","title":"Do work"}`, class: toolExecutionSequential, parallelSafe: false},
		{name: tool.BashToolName, args: `{"cmd":"cat go.mod"}`, class: toolExecutionParallelRead, parallelSafe: true},
		{name: tool.BashToolName, args: `{"cmd":"npm install"}`, class: toolExecutionSequential, parallelSafe: false},
		{name: tool.QuestionToolName, args: `{"question":"Proceed?"}`, class: toolExecutionBlocking, parallelSafe: false},
		{name: tool.ApplyPatchToolName, args: ``, class: toolExecutionSequential, parallelSafe: false},
	} {
		got := providerStepToolSchedulingCapability(test.name, test.args)
		if got.ExecutionClass != test.class || got.ParallelSafe != test.parallelSafe {
			t.Fatalf("providerStepToolSchedulingCapability(%q, %s) = %#v, want class=%q parallelSafe=%v", test.name, test.args, got, test.class, test.parallelSafe)
		}
	}
}

func TestProviderStepToolSchedulingCapabilityForExecutorUsesToolMetadata(t *testing.T) {
	sessions, err := NewSessionService(events.NewMemoryStore())
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions,
		stubTool{definition: tool.Definition{Name: "metadata_read", InputSchema: json.RawMessage(`{"type":"object"}`), ParallelSafe: true}},
		stubTool{definition: tool.Definition{Name: "metadata_write", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	)
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	readCapability := providerStepToolSchedulingCapabilityForExecutor(executor, "metadata_read", `{}`)
	if readCapability.ExecutionClass != toolExecutionParallelRead || !readCapability.ParallelSafe {
		t.Fatalf("readCapability = %#v", readCapability)
	}
	writeCapability := providerStepToolSchedulingCapabilityForExecutor(executor, "metadata_write", `{}`)
	if writeCapability.ExecutionClass != toolExecutionSequential || writeCapability.ParallelSafe {
		t.Fatalf("writeCapability = %#v", writeCapability)
	}
}

func TestProviderStepStaticToolSchedulingCapability(t *testing.T) {
	readCapability, ok := providerStepStaticToolSchedulingCapability(tool.ReadToolName)
	if !ok || readCapability.ExecutionClass != toolExecutionParallelRead || !readCapability.ParallelSafe {
		t.Fatalf("read capability = %#v, %v", readCapability, ok)
	}
	questionCapability, ok := providerStepStaticToolSchedulingCapability(tool.QuestionToolName)
	if !ok || questionCapability.ExecutionClass != toolExecutionBlocking || questionCapability.ParallelSafe {
		t.Fatalf("question capability = %#v, %v", questionCapability, ok)
	}
	if _, ok := providerStepStaticToolSchedulingCapability(tool.MemoryToolName); ok {
		t.Fatal("memory is argument-dependent and should not have static scheduling capability")
	}
	if !providerStepParallelReadToolName(tool.ReadToolName) {
		t.Fatal("read should be statically parallel-read")
	}
	if !providerStepBlockingToolName(tool.QuestionToolName) {
		t.Fatal("question should be statically blocking")
	}
}

func TestProviderStepToolCallCanJoinPrefix(t *testing.T) {
	readCall := stepToolCall{CallID: "call-read", ToolName: tool.ReadToolName, Arguments: `{"paths":["app.go"]}`}
	searchCall := stepToolCall{CallID: "call-search", ToolName: tool.SearchToolName, Arguments: `{"path":".","query":"TODO"}`}
	patchCall := stepToolCall{CallID: "call-patch", ToolName: tool.ApplyPatchToolName, Arguments: ``}
	createCall := stepToolCall{CallID: "call-task", ToolName: tool.TaskWorkflowToolName, Arguments: `{"action":"create","title":"Do work"}`}

	if !providerStepToolCallCanJoinPrefix(nil, patchCall) {
		t.Fatal("first sequential call should be executable as the step barrier")
	}
	if !providerStepToolCallCanJoinPrefix([]stepToolCall{readCall}, searchCall) {
		t.Fatal("read/search calls should share a discovery prefix")
	}
	if !providerStepToolCallCanJoinPrefix([]stepToolCall{readCall}, patchCall) {
		t.Fatal("mutation should stay in the provider-declared step after discovery")
	}
	if !providerStepToolCallCanJoinPrefix([]stepToolCall{readCall}, createCall) {
		t.Fatal("task creation should stay in the provider-declared step after discovery")
	}
	if !providerStepToolCallCanJoinPrefix([]stepToolCall{patchCall}, createCall) {
		t.Fatal("sequential calls should stay in the provider-declared step")
	}
	if providerStepToolCallCanJoinPrefix([]stepToolCall{questionCallForPrefixTest()}, readCall) {
		t.Fatal("blocking prefix should not accept later discovery calls")
	}
}

func questionCallForPrefixTest() stepToolCall {
	return stepToolCall{CallID: "call-question", ToolName: tool.QuestionToolName, Arguments: `{"question":"Proceed?"}`}
}

func TestScheduleStepToolBatchSelectsExecutablePrefix(t *testing.T) {
	readCall := stepToolCall{CallID: "call-read", ToolName: tool.ReadToolName, Arguments: `{"paths":["app.go"]}`}
	searchCall := stepToolCall{CallID: "call-search", ToolName: tool.SearchToolName, Arguments: `{"path":".","query":"TODO"}`}
	patchCall := stepToolCall{CallID: "call-patch", ToolName: tool.ApplyPatchToolName, Arguments: ``}
	createCall := stepToolCall{CallID: "call-task", ToolName: tool.TaskWorkflowToolName, Arguments: `{"action":"create","title":"Do work"}`}
	questionCall := stepToolCall{CallID: "call-question", ToolName: tool.QuestionToolName, Arguments: `{"question":"Proceed?"}`}

	for _, test := range []struct {
		name            string
		calls           []stepToolCall
		executableIDs   []string
		deferredCallIDs []string
	}{
		{
			name:          "parallel discovery plus mutation",
			calls:         []stepToolCall{readCall, searchCall, patchCall},
			executableIDs: []string{"call-read", "call-search", "call-patch"},
		},
		{
			name:          "sequential tools remain in provider step",
			calls:         []stepToolCall{patchCall, createCall},
			executableIDs: []string{"call-patch", "call-task"},
		},
		{
			name:            "single blocking barrier",
			calls:           []stepToolCall{questionCall, readCall},
			executableIDs:   []string{"call-question"},
			deferredCallIDs: []string{"call-read"},
		},
		{
			name:          "blocking after discovery runs in order",
			calls:         []stepToolCall{readCall, questionCall},
			executableIDs: []string{"call-read", "call-question"},
		},
		{
			name:          "all discovery",
			calls:         []stepToolCall{readCall, searchCall},
			executableIDs: []string{"call-read", "call-search"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			schedule := scheduleStepToolBatch(stepToolBatch{StepIndex: 7, Calls: test.calls})
			if schedule.Executable.StepIndex != 7 {
				t.Fatalf("step index = %d, want 7", schedule.Executable.StepIndex)
			}
			if got := schedule.Executable.CallIDs(); !slices.Equal(got, test.executableIDs) {
				t.Fatalf("executable IDs = %v, want %v", got, test.executableIDs)
			}
			if got := stepToolCallIDs(schedule.Deferred); !slices.Equal(got, test.deferredCallIDs) {
				t.Fatalf("deferred IDs = %v, want %v", got, test.deferredCallIDs)
			}
		})
	}
}

func stepToolCallIDs(calls []stepToolCall) []string {
	if len(calls) == 0 {
		return nil
	}
	out := make([]string, 0, len(calls))
	for _, call := range calls {
		out = append(out, call.CallID)
	}
	return out
}
