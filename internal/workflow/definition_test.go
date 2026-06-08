package workflow

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/agent"
	"github.com/sageil/kodacode/internal/tool"
)

func TestLoadBytesParsesValidDeliveryWorkflow(t *testing.T) {
	definition, err := LoadBytes([]byte(validDeliveryWorkflowYAML()), testValidationContext())
	if err != nil {
		t.Fatalf("LoadBytes() error = %v", err)
	}
	if definition.ID != "delivery" {
		t.Fatalf("ID = %q, want delivery", definition.ID)
	}
	if definition.MaxRevisionLoops != 2 {
		t.Fatalf("MaxRevisionLoops = %d, want 2", definition.MaxRevisionLoops)
	}
	if definition.Budgets.MaxCost != 1.25 {
		t.Fatalf("budget max cost = %v, want 1.25", definition.Budgets.MaxCost)
	}
	if definition.Budgets.WarnThreshold != 0.75 {
		t.Fatalf("budget warn threshold = %v, want 0.75", definition.Budgets.WarnThreshold)
	}
	if definition.Budgets.MaxProviderRequestsPerTurn != 4 {
		t.Fatalf("budget max provider requests = %d, want 4", definition.Budgets.MaxProviderRequestsPerTurn)
	}
	if definition.Model != "openai/gpt-5-mini" {
		t.Fatalf("Model = %q, want openai/gpt-5-mini", definition.Model)
	}
	if got := definition.PhaseIDs(); strings.Join(got, ",") != "approve,implement,plan,review" {
		t.Fatalf("PhaseIDs() = %#v", got)
	}
	approve := definition.Phases[1]
	if approve.SkipWhen.MaxAffectedFiles != 2 {
		t.Fatalf("approve skip_when = %#v, want max_affected_files 2", approve.SkipWhen)
	}
	implement := definition.Phases[2]
	if implement.Model != "openai/gpt-5" {
		t.Fatalf("implement model = %q, want openai/gpt-5", implement.Model)
	}
	if implement.Requires.Fields["approved_phase"] != "plan" {
		t.Fatalf("implement requires = %#v, want approved_phase plan", implement.Requires.Fields)
	}
	if strings.Join(implement.Completion.Requires.Items, ",") != "file_mutation" {
		t.Fatalf("implement completion requires = %#v", implement.Completion.Requires.Items)
	}
	plan := definition.Phases[0]
	if strings.Join(plan.RequiresOutput, ",") != "plan,affected_files,risks,implementation_tasks,acceptance_criteria,verification_plan" {
		t.Fatalf("plan requires_output = %#v", plan.RequiresOutput)
	}
	review := definition.Phases[3]
	if len(review.Requires.Items) != 0 {
		t.Fatalf("review requires = %#v, want none", review.Requires.Items)
	}
	if review.AutoContinue != nil {
		t.Fatalf("auto_continue = %#v, want unset", review.AutoContinue)
	}
	if len(review.ReviewPasses) != 3 || review.ReviewPasses[0].ID != "correctness" || review.ReviewPasses[1].ID != "verification" || review.ReviewPasses[2].ID != "tests" {
		t.Fatalf("review passes = %#v", review.ReviewPasses)
	}
	if len(definition.Transitions) != 2 {
		t.Fatalf("transitions = %#v, want 2", definition.Transitions)
	}
	if definition.Transitions[1].From != "review" || definition.Transitions[1].On != TransitionOnReviewFailed || definition.Transitions[1].To != "implement" || definition.Transitions[1].MaxLoops != 2 {
		t.Fatalf("review transition = %#v", definition.Transitions[1])
	}
}

func TestLoadBytesParsesFailureTransitions(t *testing.T) {
	definition, err := LoadBytes([]byte(`
id: failure-routes
phases:
  - id: implement
    agent: engineer
  - id: summarize
    type: final
transitions:
  - from: implement
    on: budget_exceeded
    to: summarize
  - from: implement
    on: provider_request_limit
    to: summarize
  - from: implement
    on: no_progress
    to: summarize
  - from: implement
    on: turn_failed
    to: summarize
  - from: implement
    on: canceled
    to: summarize
`), testValidationContext())
	if err != nil {
		t.Fatalf("LoadBytes() error = %v", err)
	}
	got := []string{}
	for _, transition := range definition.Transitions {
		got = append(got, transition.On)
	}
	want := strings.Join([]string{
		TransitionOnBudgetExceeded,
		TransitionOnProviderRequestLimit,
		TransitionOnNoProgress,
		TransitionOnTurnFailed,
		TransitionOnCanceled,
	}, ",")
	if strings.Join(got, ",") != want {
		t.Fatalf("transitions = %#v, want %s", got, want)
	}
}

func TestLoadBytesRejectsInvalidYAML(t *testing.T) {
	_, err := LoadBytes([]byte("id: ["), testValidationContext())
	if err == nil || !strings.Contains(err.Error(), "workflow yaml") {
		t.Fatalf("LoadBytes() error = %v, want workflow yaml error", err)
	}
}

func TestLoadBytesRejectsUnknownField(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
unknown: true
phases:
  - id: plan
    agent: planner
`), testValidationContext())
	if err == nil || !strings.Contains(err.Error(), "field unknown not found") {
		t.Fatalf("LoadBytes() error = %v, want unknown field error", err)
	}
}

func TestDefinitionValidateRejectsDuplicatePhaseID(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
phases:
  - id: plan
    agent: planner
  - id: plan
    agent: reviewer
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowPhaseDuplicate) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowPhaseDuplicate", err)
	}
}

func TestDefinitionValidateRejectsMissingPhaseID(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
phases:
  - agent: planner
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowPhaseIDRequired) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowPhaseIDRequired", err)
	}
}

func TestDefinitionValidateRejectsUnknownPhaseType(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
phases:
  - id: wait
    type: sleep
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowPhaseTypeInvalid) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowPhaseTypeInvalid", err)
	}
}

func TestDefinitionValidateRejectsUnknownReviewMode(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
review_mode: sometimes
phases:
  - id: plan
    agent: planner
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowReviewModeInvalid) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowReviewModeInvalid", err)
	}
}

func TestDefinitionValidateRejectsInvalidWorkflowModel(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
model: gpt-5
phases:
  - id: plan
    agent: planner
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowModelInvalid) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowModelInvalid", err)
	}
}

func TestDefinitionValidateRejectsInvalidPhaseModel(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
phases:
  - id: plan
    agent: planner
    model: gpt-5
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowModelInvalid) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowModelInvalid", err)
	}
}

func TestDefinitionValidateRejectsNegativeRevisionLoops(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
max_revision_loops: -1
phases:
  - id: plan
    agent: planner
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowRevisionLoopsInvalid) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowRevisionLoopsInvalid", err)
	}
}

func TestDefinitionValidateRejectsNegativeBudgets(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
budgets:
  max_cost: -0.01
phases:
  - id: plan
    agent: planner
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowBudgetInvalid) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowBudgetInvalid", err)
	}

	_, err = LoadBytes([]byte(`
id: delivery
budgets:
  max_cost: 1.00
  warn_threshold: 1.1
phases:
  - id: plan
    agent: planner
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowBudgetInvalid) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowBudgetInvalid", err)
	}

	_, err = LoadBytes([]byte(`
id: delivery
budgets:
  warn_threshold: 0.8
phases:
  - id: plan
    agent: planner
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowBudgetInvalid) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowBudgetInvalid", err)
	}

	_, err = LoadBytes([]byte(`
id: delivery
budgets:
  max_provider_requests_per_turn: -1
phases:
  - id: plan
    agent: planner
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowBudgetInvalid) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowBudgetInvalid", err)
	}
}

func TestDefinitionValidateRejectsApprovalSkipOnNonApprovalPhase(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
phases:
  - id: plan
    agent: planner
    skip_when:
      max_affected_files: 2
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowApprovalSkipInvalid) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowApprovalSkipInvalid", err)
	}
}

func TestDefinitionValidateRejectsReviewPassesOnNonReviewPhase(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
phases:
  - id: plan
    agent: planner
    review_passes:
      - id: correctness
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowReviewPassInvalid) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowReviewPassInvalid", err)
	}
}

func TestDefinitionValidateRequiresReviewTypeForReviewPasses(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
phases:
  - id: review
    agent: reviewer
    review_passes:
      - id: correctness
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowReviewPassInvalid) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowReviewPassInvalid", err)
	}
}

func TestDefinitionValidateParsesReviewPassInstructions(t *testing.T) {
	definition, err := LoadBytes([]byte(`
id: review
phases:
  - id: review
    type: review
    agent: reviewer
    review_passes:
      - id: side-effects
        description: Side effects in nearby code, config, or permissions.
        instructions:
          - Inspect adjacent code paths.
          - Check config and permissions.
`), testValidationContext())
	if err != nil {
		t.Fatalf("LoadBytes() error = %v", err)
	}
	pass := definition.Phases[0].ReviewPasses[0]
	if pass.ID != "side-effects" || pass.Description != "Side effects in nearby code, config, or permissions." {
		t.Fatalf("review pass = %#v", pass)
	}
	if strings.Join(pass.Instructions, "|") != "Inspect adjacent code paths.|Check config and permissions." {
		t.Fatalf("review pass instructions = %#v", pass.Instructions)
	}
}

func TestDefinitionValidateAllowsReviewTypeWithCustomAgent(t *testing.T) {
	ctx := testValidationContext()
	ctx.Agents["security-reviewer"] = agent.Definition{ID: "security-reviewer", Mode: agent.ModeAll}

	definition, err := LoadBytes([]byte(`
id: custom-review
phases:
  - id: security
    type: review
    agent: security-reviewer
    review_passes:
      - id: auth
`), ctx)
	if err != nil {
		t.Fatalf("LoadBytes() error = %v", err)
	}
	phase := definition.Phases[0]
	if phase.EffectiveType() != PhaseTypeReview {
		t.Fatalf("phase type = %q, want %q", phase.EffectiveType(), PhaseTypeReview)
	}
	if phase.Agent != "security-reviewer" {
		t.Fatalf("phase agent = %q, want custom reviewer", phase.Agent)
	}
}

func TestDefinitionValidateParsesAutoContinueReview(t *testing.T) {
	definition, err := LoadBytes([]byte(`
id: review
phases:
  - id: review
    type: review
    agent: reviewer
    auto_continue: true
    review_passes:
      - id: correctness
`), testValidationContext())
	if err != nil {
		t.Fatalf("LoadBytes() error = %v", err)
	}
	if !definition.Phases[0].AutoContinueEnabled() {
		t.Fatal("auto_continue = false, want true")
	}
}

func TestDefinitionValidateParsesAutoContinueAgentPhase(t *testing.T) {
	definition, err := LoadBytes([]byte(`
id: delivery
phases:
  - id: implement
    agent: engineer
    auto_continue: true
`), testValidationContext())
	if err != nil {
		t.Fatalf("LoadBytes() error = %v", err)
	}
	if !definition.Phases[0].AutoContinueEnabled() {
		t.Fatal("auto_continue = false, want true")
	}
}

func TestDefinitionValidateParsesAutoContinueFalseAsOptOut(t *testing.T) {
	definition, err := LoadBytes([]byte(`
id: review
phases:
  - id: review
    type: review
    agent: reviewer
    auto_continue: false
    review_passes:
      - id: correctness
`), testValidationContext())
	if err != nil {
		t.Fatalf("LoadBytes() error = %v", err)
	}
	if !definition.Phases[0].AutoContinueDisabled() {
		t.Fatal("auto_continue disabled = false, want true")
	}
}

func TestDefinitionValidateRejectsAutoContinueUnsupportedPhase(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
phases:
  - id: approve
    type: user_approval
    auto_continue: true
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowAutoContinueInvalid) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowAutoContinueInvalid", err)
	}
}

func TestDefinitionValidateRejectsLegacyReviewFanoutKey(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
phases:
  - id: review
    agent: reviewer
    review_fanout: true
    review_passes:
      - id: correctness
`), testValidationContext())
	if err == nil || !strings.Contains(err.Error(), "removed field review_fanout is not supported") || !strings.Contains(err.Error(), "remove this key") {
		t.Fatalf("LoadBytes() error = %v, want removed review_fanout diagnostic", err)
	}
}

func TestDefinitionValidateRejectsLegacyParallelReviewKey(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
phases:
  - id: review
    agent: reviewer
    parallel_review: true
    review_passes:
      - id: correctness
`), testValidationContext())
	if err == nil || !strings.Contains(err.Error(), "removed field parallel_review is not supported") || !strings.Contains(err.Error(), "remove this key") {
		t.Fatalf("LoadBytes() error = %v, want removed parallel_review diagnostic", err)
	}
}

func TestDefinitionValidateRejectsUnknownTransitionPhase(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
phases:
  - id: plan
    agent: planner
transitions:
  - from: plan
    on: skipped
    to: missing
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowTransitionInvalid) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowTransitionInvalid", err)
	}
}

func TestDefinitionValidateRejectsUnknownTransitionEvent(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
phases:
  - id: plan
    agent: planner
  - id: implement
    agent: engineer
transitions:
  - from: plan
    on: sometimes
    to: implement
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowTransitionInvalid) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowTransitionInvalid", err)
	}
}

func TestDefinitionValidateRejectsUnknownAgent(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
phases:
  - id: plan
    agent: missing
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowAgentUnknown) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowAgentUnknown", err)
	}
}

func TestDefinitionValidateRejectsMissingRunnablePhaseAgent(t *testing.T) {
	for _, typ := range []PhaseType{PhaseTypeAgent, PhaseTypeVerification, PhaseTypeReview} {
		_, err := LoadBytes([]byte(`
id: missing-agent
phases:
  - id: run
    type: `+string(typ)+`
`), testValidationContext())
		if !errors.Is(err, ErrWorkflowAgentRequired) {
			t.Fatalf("LoadBytes(type %q) error = %v, want ErrWorkflowAgentRequired", typ, err)
		}
	}
}

func TestDefinitionValidateRejectsUnknownTool(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
phases:
  - id: implement
    agent: engineer
    tools:
      allow:
        - missing_tool
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowToolUnknown) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowToolUnknown", err)
	}
}

func TestDefinitionValidateRejectsMalformedVerificationCommand(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
phases:
  - id: verify
    type: verification
    agent: engineer
    tools:
      allow:
        - test
    commands:
      - tool: test
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowCommandInvalid) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowCommandInvalid", err)
	}
}

func TestDefinitionValidateRejectsVerificationCommandToolOutsidePhaseAllowlist(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
phases:
  - id: verify
    type: verification
    agent: engineer
    tools:
      allow:
        - test
    commands:
      - tool: bash
        command: go vet ./...
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowCommandInvalid) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowCommandInvalid", err)
	}
}

func TestDefinitionValidateRejectsMalformedRequirements(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
phases:
  - id: review
    agent: reviewer
    requires: verification_result
`), testValidationContext())
	if err == nil || !strings.Contains(err.Error(), "requires must be a sequence or mapping") {
		t.Fatalf("LoadBytes() error = %v, want malformed requires error", err)
	}
}

func TestDefinitionValidateRejectsToolForbiddenByAgent(t *testing.T) {
	ctx := testValidationContext()
	ctx.Agents["planner"] = agent.Definition{
		ID:           "planner",
		AllowedTools: []string{tool.ReadToolName},
	}
	_, err := LoadBytes([]byte(`
id: delivery
phases:
  - id: plan
    agent: planner
    tools:
      allow:
        - write
`), ctx)
	if !errors.Is(err, ErrWorkflowToolForbidden) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowToolForbidden", err)
	}
}

func TestDefinitionValidateRejectsMutationToolInReadOnlyPhase(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
phases:
  - id: plan
    agent: planner
    mode: read_only
    tools:
      allow:
        - write
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowToolUnsafe) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowToolUnsafe", err)
	}
}

func TestDefinitionValidateAllowsTestToolInReadOnlyReviewPhase(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
phases:
  - id: review
    type: review
    agent: reviewer
    mode: read_only
    tools:
      allow:
        - read
        - test
`), testValidationContext())
	if err != nil {
		t.Fatalf("LoadBytes() error = %v", err)
	}
}

func TestDefinitionValidateRejectsSubagentOnlyPhaseAgent(t *testing.T) {
	ctx := testValidationContext()
	ctx.Agents["helper"] = agent.Definition{ID: "helper", Mode: agent.ModeSubagent}
	_, err := LoadBytes([]byte(`
id: delivery
phases:
  - id: help
    agent: helper
`), ctx)
	if !errors.Is(err, agent.ErrAgentModeInvalid) {
		t.Fatalf("LoadBytes() error = %v, want ErrAgentModeInvalid", err)
	}
}

func TestLoadFileIncludesPathInValidationError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "delivery.yaml")
	if err := os.WriteFile(path, []byte(`
id: delivery
phases:
  - id: plan
    agent: missing
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	_, err := LoadFile(path, testValidationContext())
	if err == nil || !strings.Contains(err.Error(), path) || !errors.Is(err, ErrWorkflowAgentUnknown) {
		t.Fatalf("LoadFile() error = %v, want path and ErrWorkflowAgentUnknown", err)
	}
}

func testValidationContext() ValidationContext {
	agents := []agent.Definition{
		{ID: "builder"},
		{ID: "engineer"},
		{ID: "planner"},
		{ID: "reviewer"},
	}
	return NewValidationContext(agents, tool.AllBuiltInTools())
}

func validDeliveryWorkflowYAML() string {
	return `
id: delivery
description: Plan, implement, and review a code change.
model: openai/gpt-5-mini
review_mode: auto
max_revision_loops: 2
budgets:
  max_cost: 1.25
  warn_threshold: 0.75
  max_provider_requests_per_turn: 4

phases:
  - id: plan
    agent: planner
    mode: read_only
    requires_output:
      - plan
      - affected_files
      - risks
      - implementation_tasks
      - acceptance_criteria
      - verification_plan

  - id: approve
    type: user_approval
    prompt: Approve this plan before edits?
    skip_when:
      max_affected_files: 2

  - id: implement
    agent: engineer
    model: openai/gpt-5
    tools:
      allow:
        - read
        - search
        - apply_patch
        - write
        - bash
        - task_workflow
    requires:
      approved_phase: plan
    completion:
      requires:
        - file_mutation

  - id: review
    type: review
    agent: reviewer
    mode: read_only
    tools:
      allow:
        - read
        - search
        - test
        - git_status
        - workflow_review_result
    review_passes:
      - id: correctness
        description: Behavior changes and whether the implementation is correct.
      - id: verification
        description: Verification against acceptance criteria and the approved verification plan.
      - id: tests
        description: Test coverage, edge cases, and missing checks.
transitions:
  - from: approve
    on: skipped
    to: implement
  - from: review
    on: review_failed
    to: implement
    max_loops: 2
`
}
