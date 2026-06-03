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
	if got := definition.PhaseIDs(); strings.Join(got, ",") != "approve,implement,plan,review,verify" {
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
	if strings.Join(implement.Completion.Requires.Items, ",") != CompletionRequirementActivePhaseTasksComplete {
		t.Fatalf("implement completion requires = %#v", implement.Completion.Requires.Items)
	}
	verify := definition.Phases[3]
	if len(verify.Commands) != 1 || verify.Commands[0].Tool != "test" || verify.Commands[0].Command != "go test ./..." {
		t.Fatalf("verify commands = %#v", verify.Commands)
	}
	review := definition.Phases[4]
	if strings.Join(review.Requires.Items, ",") != "git_diff,verification_result" {
		t.Fatalf("review requires = %#v", review.Requires.Items)
	}
	if !review.ParallelReview {
		t.Fatal("parallel_review = false, want true")
	}
	if review.AutoContinue != nil {
		t.Fatalf("auto_continue = %#v, want unset", review.AutoContinue)
	}
	if len(review.ReviewPasses) != 2 || review.ReviewPasses[0].ID != "correctness" || review.ReviewPasses[1].ID != "tests" {
		t.Fatalf("review passes = %#v", review.ReviewPasses)
	}
	if len(definition.Transitions) != 3 {
		t.Fatalf("transitions = %#v, want 3", definition.Transitions)
	}
	if definition.Transitions[1].From != "verify" || definition.Transitions[1].On != TransitionOnVerificationFailed || definition.Transitions[1].To != "implement" || definition.Transitions[1].MaxLoops != 2 {
		t.Fatalf("verification transition = %#v", definition.Transitions[1])
	}
	if definition.Transitions[2].From != "review" || definition.Transitions[2].On != TransitionOnReviewFailed || definition.Transitions[2].To != "implement" || definition.Transitions[2].MaxLoops != 2 {
		t.Fatalf("review transition = %#v", definition.Transitions[2])
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

func TestDefinitionValidateAllowsReviewTypeWithCustomAgent(t *testing.T) {
	ctx := testValidationContext()
	ctx.Agents["security-reviewer"] = agent.Definition{ID: "security-reviewer", Mode: agent.ModeAll}

	definition, err := LoadBytes([]byte(`
id: custom-review
phases:
  - id: security
    type: review
    agent: security-reviewer
    parallel_review: true
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

func TestDefinitionValidateRejectsParallelReviewWithoutPasses(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
phases:
  - id: review
    type: review
    agent: reviewer
    parallel_review: true
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowReviewPassInvalid) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowReviewPassInvalid", err)
	}
}

func TestDefinitionValidateParsesAutoContinueParallelReview(t *testing.T) {
	definition, err := LoadBytes([]byte(`
id: review
phases:
  - id: review
    type: review
    agent: reviewer
    parallel_review: true
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

func TestDefinitionValidateParsesAutoContinueFalseAsOptOut(t *testing.T) {
	definition, err := LoadBytes([]byte(`
id: review
phases:
  - id: review
    type: review
    agent: reviewer
    parallel_review: true
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
  - id: implement
    agent: engineer
    auto_continue: true
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowAutoContinueInvalid) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowAutoContinueInvalid", err)
	}
}

func TestDefinitionValidateRejectsTooManyParallelReviewPasses(t *testing.T) {
	_, err := LoadBytes([]byte(`
id: delivery
phases:
  - id: review
    type: review
    agent: reviewer
    parallel_review: true
    review_passes:
      - id: pass_1
      - id: pass_2
      - id: pass_3
      - id: pass_4
      - id: pass_5
      - id: pass_6
      - id: pass_7
      - id: pass_8
      - id: pass_9
`), testValidationContext())
	if !errors.Is(err, ErrWorkflowReviewPassInvalid) {
		t.Fatalf("LoadBytes() error = %v, want ErrWorkflowReviewPassInvalid", err)
	}
	if err == nil || !strings.Contains(err.Error(), "at most 8") {
		t.Fatalf("LoadBytes() error = %v, want pass limit detail", err)
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
	if err == nil || !strings.Contains(err.Error(), "field review_fanout not found") {
		t.Fatalf("LoadBytes() error = %v, want unknown review_fanout field", err)
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
description: Plan, implement, verify, and review a code change.
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
        - bash
        - git_diff
        - task_workflow
    requires:
      approved_phase: plan
    completion:
      requires:
        - active_phase_tasks_complete

  - id: verify
    type: verification
    commands:
      - tool: test
        command: go test ./...
    required: true

  - id: review
    type: review
    agent: reviewer
    mode: read_only
    parallel_review: true
    review_passes:
      - id: correctness
        description: Behavioral regressions and implementation correctness.
      - id: tests
        description: Verification coverage, edge cases, and missing checks.
    requires:
      - git_diff
      - verification_result
transitions:
  - from: approve
    on: skipped
    to: implement
  - from: verify
    on: verification_failed
    to: implement
    max_loops: 2
  - from: review
    on: review_failed
    to: implement
    max_loops: 2
`
}
