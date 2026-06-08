package agent

import (
	"strings"
	"testing"
)

func TestNewBuiltinsCatalogIncludesBuiltinsAndSubagents(t *testing.T) {
	catalog, err := NewBuiltinsCatalog()
	if err != nil {
		t.Fatalf("NewBuiltinsCatalog() error = %v", err)
	}

	builder, err := catalog.Get("")
	if err != nil {
		t.Fatalf("Get(\"\") error = %v", err)
	}
	if builder.ID != DefaultID {
		t.Fatalf("default agent = %q, want %q", builder.ID, DefaultID)
	}
	if builder.AllowsTool("task_workflow") {
		t.Fatal("builder should not allow task_workflow")
	}
	if builder.AllowsTool("task_review") {
		t.Fatal("builder should not allow task_review")
	}
	if !builder.HasPrompt() {
		t.Fatal("builder prompt = empty, want built-in prompt")
	}
	for _, want := range []string{
		"Solve the user's requested task directly",
		"Keep the turn simple",
		"Use the available tools only when they help complete the current task",
		"After each tool result, decide whether you now have enough information",
		"Continue autonomously inside the user's requested scope.",
		"Implementation details are the agent's decision",
		"Local repairs after edits, type errors, lint failures, test failures caused by",
		"Lost visible context, compaction, needing to re-read files, or conserving turn\nbudget is not a user blocker.",
		"Do not ask the user to send \"continue\" or another follow-up solely to resume\nwork",
		"Do not ask generic \"Proceed?\", \"which area next?\", or optional next-step questions.",
		"Do not end by announcing readiness for the next task, file, method, or area.",
		"Treat only external\nrequirements as blockers",
		"ordinary repair loops, keep working inside scope instead of asking what to do",
	} {
		if !strings.Contains(builder.PromptFragment().Content, want) {
			t.Fatalf("builder prompt missing guidance %q: %q", want, builder.PromptFragment().Content)
		}
	}
	for _, unwanted := range []string{
		"engineer agent",
		"delegate to `planner`",
		"Delegate to `reviewer`",
		"Do not ask the user to choose a subagent",
		"task_workflow",
	} {
		if strings.Contains(builder.PromptFragment().Content, unwanted) {
			t.Fatalf("builder prompt should stay direct and non-orchestrating; found %q in %q", unwanted, builder.PromptFragment().Content)
		}
	}
	engineer, err := catalog.Get("engineer")
	if err != nil {
		t.Fatalf("Get(\"engineer\") error = %v", err)
	}
	if engineer.ID != "engineer" {
		t.Fatalf("engineer id = %q", engineer.ID)
	}
	if !engineer.HasPrompt() {
		t.Fatal("engineer prompt = empty, want built-in prompt")
	}
	if engineer.PromptFragment().Label != "engineer" {
		t.Fatalf("engineer prompt label = %q", engineer.PromptFragment().Label)
	}
	for _, want := range []string{
		"Own multi-step implementation work from start to finish",
		"Use `task_workflow` when the work breaks into meaningful steps",
		"Keep narrow local work inline. A one or two-file review",
		"small local planning question should be handled directly",
		"use configured workflow phases rather than a separate handoff mechanism",
		"Treat execution, implementation, fixing bugs, applying requested changes",
		"For broad review, audit, regression checking, repo review, performance review",
		"For broad planning, architecture mapping, refactoring strategy",
		"cross-module tradeoff analysis",
		"recommend improvements",
		"For compound requests that include review findings and an implementation plan",
		"Do not create a separate handoff",
		"Routing examples:",
		"\"Review the current project and recommend performance improvements\" -> review directly",
		"\"Turn those findings into a step-by-step implementation plan\" -> produce the plan directly",
		"\"Perform a performance review and create an execution plan\" -> report findings first",
		"\"Implement the approved plan\" -> `engineer`",
		"Continue autonomously inside the user's requested scope.",
		"Implementation details are the agent's decision",
		"Local repairs after edits, type errors, lint failures, test failures caused by",
		"Lost visible context, compaction, needing to re-read files, or conserving turn\nbudget is not a user blocker.",
		"Do not ask the user to send \"continue\" or another follow-up solely to resume\nwork",
		"Do not ask generic \"Proceed?\", \"which area next?\", or optional next-step questions.",
		"Do not end by announcing readiness for the next task, file, method, or area.",
		"Treat only external\nrequirements as blockers",
		"ordinary repair loops, keep working inside scope instead of asking what to do",
		"keep one active\ntask path",
		"Use `parent_task_id` when creating follow-up tasks",
		"Parent tasks organize child tasks. A parent can stay in_progress while one child task is the current step.",
		"immediately set\nthe first active task to in_progress before starting implementation",
		"call `task_workflow` to record progress before moving to another task or\ngiving a final answer",
		"complete finished tasks with a short summary",
		"Block\ntasks only for external blockers",
		"Do not leave unrelated task branches in_progress",
		"Run verification after the implementation pass is complete, not after every file edit.",
		"Run intermediate tests only when the result is needed to choose the next edit",
	} {
		if !strings.Contains(engineer.PromptFragment().Content, want) {
			t.Fatalf("engineer prompt missing guidance %q: %q", want, engineer.PromptFragment().Content)
		}
	}
	for _, unwanted := range []string{
		"Use the `delegate` tool",
		"You MUST delegate",
		"After a delegated planner returns",
		"child-agent",
	} {
		if strings.Contains(engineer.PromptFragment().Content, unwanted) {
			t.Fatalf("engineer prompt contains delegation guidance %q: %q", unwanted, engineer.PromptFragment().Content)
		}
	}
	if !engineer.AllowsTool("read") || !engineer.AllowsTool("write") {
		t.Fatal("engineer should allow the general tool surface")
	}
	if !engineer.AllowsTool("task_workflow") {
		t.Fatal("engineer should allow task_workflow")
	}
	if engineer.AllowsTool("task_review") {
		t.Fatal("engineer should not allow task_review")
	}

	reviewer, err := catalog.Get("reviewer")
	if err != nil {
		t.Fatalf("Get(\"reviewer\") error = %v", err)
	}
	if reviewer.ID != "reviewer" {
		t.Fatalf("reviewer id = %q", reviewer.ID)
	}
	if !reviewer.AllowsTool("task_review") {
		t.Fatal("reviewer should allow task_review")
	}
	if !reviewer.AllowsTool("question") {
		t.Fatal("reviewer should allow question")
	}
	if !reviewer.AllowsTool("test") {
		t.Fatal("reviewer should allow test")
	}
	if reviewer.AllowsTool("task_workflow") {
		t.Fatal("reviewer should not allow task_workflow")
	}
	if reviewer.AllowsTool("write") {
		t.Fatal("reviewer should not allow write")
	}
	if reviewer.AllowsTool("edit") {
		t.Fatal("reviewer should not allow edit")
	}
	if len(reviewer.Handoff.Provides) != 1 || reviewer.Handoff.Provides[0].Kind != "review_findings" {
		t.Fatalf("reviewer handoff provides = %#v", reviewer.Handoff.Provides)
	}
	if got := strings.Count(reviewer.PromptFragment().Content, "\n") + 1; got > 40 {
		t.Fatalf("reviewer prompt lines = %d, want compact prompt", got)
	}
	for _, want := range []string{
		"Stay read-only: do not implement, plan, or\nsave files.",
		"`git_status` first and prefer targeted reads over full diffs.",
		"Report defensible findings first with file/line evidence.",
		"When `workflow_review_result` is available for a workflow review pass",
		"When `task_review` is available for an assigned saved task",
	} {
		if !strings.Contains(reviewer.PromptFragment().Content, want) {
			t.Fatalf("reviewer prompt missing guidance %q: %q", want, reviewer.PromptFragment().Content)
		}
	}

	planner, err := catalog.Get("planner")
	if err != nil {
		t.Fatalf("Get(\"planner\") error = %v", err)
	}
	if planner.Description == "" || !planner.HasPrompt() || planner.PromptFragment().Content == "" {
		t.Fatalf("planner = %#v", planner)
	}
	if planner.Description != "Read-only planning agent for architecture mapping, design exploration, and implementation planning." {
		t.Fatalf("planner description = %q", planner.Description)
	}
	if !planner.AllowsTool("read") ||
		!planner.AllowsTool("web_search") ||
		!planner.AllowsTool("definition") ||
		!planner.AllowsTool("diagnostics") ||
		!planner.AllowsTool("refs") ||
		!planner.AllowsTool("trace") ||
		planner.AllowsTool("rename_symbol") ||
		planner.AllowsTool("code_action") ||
		planner.AllowsTool("save_plan") ||
		planner.AllowsTool("write") ||
		planner.AllowsTool("edit") {
		t.Fatalf("planner allowed tools = %#v", planner.AllowedTools)
	}
	if !planner.AllowsTool("question") {
		t.Fatal("planner should allow question")
	}
	if len(planner.Handoff.Provides) != 1 || planner.Handoff.Provides[0].Kind != "implementation_plan" {
		t.Fatalf("planner handoff provides = %#v", planner.Handoff.Provides)
	}
	if len(planner.Handoff.Consumes) != 1 || planner.Handoff.Consumes[0].Kind != "review_findings" {
		t.Fatalf("planner handoff consumes = %#v", planner.Handoff.Consumes)
	}
	if !planner.Delegatable() || !planner.Selectable() {
		t.Fatalf("planner mode = %q", planner.EffectiveMode())
	}
	for _, want := range []string{
		"Planning analysis here means design and sequencing analysis, not repository review or issue discovery.",
		"Read the repository only to understand structure, dependencies, tradeoffs, and",
		"Repository-scoped issue discovery, performance review, repo review, audit, bug-finding, or recommendation gathering are NOT planner work.",
		"minimum relevant files before",
		"generic framework recipe",
		"When a workflow or parent turn assigns planning work",
		"Return the complete plan as assistant text and stop",
		"Do not ask questions or persist plan files.",
		"Do not ask a save/apply/revise plan-decision question unless the runtime provides explicit planner approval instructions",
		"Reference the files, modules, or subsystems you inspected",
		"Do not perform acceptance review, correctness audit, bug-finding, or code",
		"performance review, or regression check, state that reviewer is the appropriate agent",
		"Treat `question` as a logical job",
		"Use tools only when they resolve concrete planning uncertainty.",
	} {
		if !strings.Contains(planner.PromptFragment().Content, want) {
			t.Fatalf("planner prompt missing guidance %q: %q", want, planner.PromptFragment().Content)
		}
	}
	for _, unwanted := range []string{
		"Use exactly these options:\n- Save plan\n- Revise plan",
		"Use a purpose string of `planner_save_plan`",
		"If the answer is `Save plan`, `Apply plan`, or `Stop`, the runtime owns the next action.",
	} {
		if strings.Contains(planner.PromptFragment().Content, unwanted) {
			t.Fatalf("planner prompt contains opt-in approval guidance %q: %q", unwanted, planner.PromptFragment().Content)
		}
	}
}

func TestCatalogReturnsErrorForUnknownAgent(t *testing.T) {
	catalog, err := NewBuiltinsCatalog()
	if err != nil {
		t.Fatalf("NewBuiltinsCatalog() error = %v", err)
	}

	if _, err := catalog.Get("missing"); err == nil {
		t.Fatal("Get(\"missing\") error = nil, want error")
	}
}
