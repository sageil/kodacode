# Agentic Workflows for KodaCode

This note summarizes research on agentic workflow patterns and maps the useful
parts to KodaCode's product and runtime architecture.

## Local product fit

KodaCode already has the primitives needed for workflow-driven coding:

- Runtime-owned agent definitions: `builder`, `engineer`, `planner`, and
  `reviewer`
- Delegated child agents through the `delegate` tool
- Durable task state through `task_workflow`
- Durable review outcomes through `task_review`
- Workspace-rooted sandboxing and explicit approval gates
- Replayable event history and `/trace`
- Session resume, timeline branching, and history compaction
- Budgets, cost tracking, model routing, MCP tools, and skills

The strongest opportunity is not to add more prompt-only agent personas. It is
to make repeatable software delivery workflows explicit, durable, inspectable,
and resumable.

## Current status

KodaCode's workflow feature is currently an MVP-plus runtime feature. The core
contract is implemented and durable:

- Built-in, global, and project-local workflow catalogs.
- Workflow YAML validation with known-field checking.
- Durable workflow start, phase advance, block, resume, evidence, and complete
  events.
- Phase-bound agents, narrowed tool surfaces, and read-only phase enforcement.
- Structured `requires_output` gates.
- User approval phases and small-plan approval skip through
  `skip_when.max_affected_files`.
- Runtime-executed verification commands through the `test` tool.
- Explicit `transitions` for supported non-linear events such as `skipped`,
  `verification_failed`, and `review_failed`.
- Bounded verification revision loops through transition `max_loops` or
  workflow-level `max_revision_loops`.
- Final phases synthesize a deterministic summary from recorded workflow
  evidence and call out requested fields that were not recorded.
- Advisory workflow route recommendations are recorded per turn when no
  workflow is explicitly selected.
- Review phases can declare review pass metadata, optionally fan out reviewer
  child sessions with `review_fanout`, and final summaries aggregate multiple
  recorded review outcomes.
- Per-workflow `review_mode`.
- Workflow and phase `model` routing with phase-level precedence.
- Per-workflow budgets through `budgets.max_cost` and
  `budgets.max_provider_requests_per_turn`.
- TUI workflow selection, footer status, and trace visibility.

Remaining work:

- Additional transition events beyond `skipped`, `verification_failed`, and
  `review_failed`.
- Parallel reviewer scheduling; current workflow fan-out runs bounded reviewer
  child sessions sequentially.
- Stronger revision evidence tying each retry to the exact failed check or
  review finding.
- Broader verification command types when the `test` tool is too narrow.
- Better user controls for retrying or resuming blocked workflows.

## Research baseline

The most useful public taxonomy comes from Anthropic's "Building Effective
Agents." It separates common workflow patterns from more open-ended autonomous
agents:

- Prompt chaining: break a task into fixed sequential steps, with optional gates
  between steps.
- Routing: classify the input and send it to a specialized path.
- Parallelization: run independent subtasks or multiple attempts concurrently,
  then aggregate the results.
- Orchestrator-workers: let a central agent dynamically decompose a task and
  delegate work to specialized workers.
- Evaluator-optimizer: generate a candidate, evaluate it, revise it, and repeat
  under clear stopping conditions.

OpenAI's Agents SDK emphasizes similar operational primitives: orchestration,
handoffs, tools, guardrails, sessions, and tracing. The important product lesson
is that workflows need observable runtime structure, not hidden prompt behavior.

LangGraph's durable execution guidance is also relevant. Long-running workflows
should persist state, support human-in-the-loop approval, resume after
interruption, and avoid repeating side effects during replay.

Sources:

- Anthropic, "Building Effective Agents":
  https://www.anthropic.com/engineering/building-effective-agents
- OpenAI Agents SDK, "Agents":
  https://openai.github.io/openai-agents-python/agents/
- OpenAI Agents SDK, "Tracing":
  https://openai.github.io/openai-agents-python/tracing/
- OpenAI Agents SDK, "Guardrails":
  https://openai.github.io/openai-agents-python/guardrails/
- LangGraph, "Durable execution":
  https://docs.langchain.com/oss/python/langgraph/durable-execution

## Workflow types KodaCode should support

### 1. Task router workflow

Classify the user's request before execution and recommend the right runtime
path. Routing should be advisory unless the user explicitly selected a workflow
or a deterministic runtime contract applies.

Possible routes:

- Quick local code change: `builder`
- Broad, risky, or architectural change: `engineer`
- Read-only repository analysis: `planner`
- Acceptance check or regression review: `reviewer`
- Docs or research-heavy task: read-heavy route with web tools when configured
- Sensitive action: preflight approval before writes, network, or destructive
  commands

User benefit:

- Users do not need to know the exact agent or workflow mode up front.
- KodaCode can pick cheaper models for simple paths and stronger models for
  high-risk paths.
- The system avoids bad starts, such as implementing before planning or using a
  mutation-capable agent for read-only exploration.

Runtime requirements:

- A request classification event.
- A visible route recommendation with reason, confidence, selected agent, and
  alternatives.
- Optional model route override per classified path.
- A way for the user to accept, reject, or override the route before execution.
- A read-only or plan-first fallback when confidence is low.

### 2. Plan, implement, verify, review workflow

This should be the default serious delivery workflow.

Phases:

1. Route: classify the task and select the workflow.
2. Plan: use `planner` or inline `engineer` planning based on scope.
3. Approve: pause before edits when the plan is broad or risky.
4. Implement: use `engineer` with durable `task_workflow` state.
5. Verify: run focused tests, diagnostics, and diff checks.
6. Review: use `reviewer` to record pass, concern, fail, or accepted outcomes.
7. Summarize: return changes, verification, risks, and next steps.

User benefit:

- Gives users a predictable delivery path for real code changes.
- Makes checkpoints visible and replayable.
- Keeps the review result separate from implementation confidence.

Runtime requirements:

- Workflow phase events.
- Phase status in the TUI.
- A durable link between tasks, verification commands, and review outcomes.
- Configurable review mode and review model.

### 3. Orchestrator-workers workflow

Use `engineer` as the parent orchestrator for broad or uncertain tasks.

Pattern:

- `engineer` identifies unknowns and delegates bounded read-only work.
- `planner` maps architecture, ownership, and likely affected files.
- Optional specialist child agents inspect isolated subsystems.
- Parent `engineer` synthesizes one plan and owns implementation.
- `reviewer` evaluates the parent work session after implementation.

User benefit:

- Handles large repositories without forcing one model turn to carry all
  context.
- Preserves clear ownership: children investigate; parent implements.
- Keeps child context isolated while returning durable handoff summaries.

Runtime requirements:

- Child session lineage that is visible in `/trace` and `/timeline`.
- Delegation summaries that include scope, files inspected, findings, and
  confidence.
- A hard boundary that child-only agents cannot mutate files unless explicitly
  configured to do so.

### 4. Parallel review workflow

Run several focused review passes and aggregate the results.

Suggested passes:

- Correctness and behavioral regressions
- Test coverage and edge cases
- Security, sandbox, and permission boundaries
- API, config, and compatibility contracts
- Documentation and user-facing command accuracy
- Cost, context, and model-routing impact for agent-runtime changes

User benefit:

- Improves confidence on high-risk changes.
- Reduces the chance that a single broad review misses a specific class of
  issue.
- Produces review findings that are easier to act on.

Runtime requirements:

- Parallel or sequential reviewer child sessions.
- Review category metadata.
- Aggregated review summary with deduped findings and severity.
- Budget-aware caps so review fan-out cannot run uncontrolled.

### 5. Evaluator-optimizer workflow

Use bounded revision loops when success criteria are clear.

Pattern:

1. Generate or implement a candidate.
2. Verify with tests, diagnostics, or structured checks.
3. Evaluate with `reviewer` against acceptance criteria.
4. Revise only if the evaluator finds concrete failures.
5. Stop on pass, iteration cap, budget cap, or user checkpoint.

User benefit:

- Better final quality for tasks with measurable acceptance criteria.
- Avoids pretending that a model's first implementation is done when tests or
  review indicate otherwise.
- Keeps iteration bounded and inspectable.

Runtime requirements:

- Iteration counter.
- Explicit stop reasons.
- Link from each revision to the failed check or review concern that caused it.
- Budget and provider-request caps per workflow.

### 6. Timeline branch comparison workflow

Use KodaCode's existing `/timeline` branching as a first-class exploration
workflow.

Pattern:

- Start from an accepted plan or known-good turn.
- Create branch A and branch B.
- Try different implementation strategies, models, or constraints.
- Review and compare diffs, tests, risk, and maintainability.
- Continue from the branch the user selects.

User benefit:

- Supports design exploration without losing history.
- Lets users compare approaches instead of committing to the first plausible
  path.
- Makes experimentation safer and more auditable.

Runtime requirements:

- Branch labels and summaries.
- Compare view for branch diffs, test results, costs, and review outcomes.
- A clear "continue from this branch" action.

### 7. Debugging workflow

Use a fixed chain for bug reports and failing tests.

Phases:

1. Reproduce the failure.
2. Capture the concrete signal.
3. Localize likely ownership.
4. Patch the root cause.
5. Add or update a regression test.
6. Verify the fix.
7. Review for collateral damage.

User benefit:

- Turns vague "fix this" prompts into a disciplined troubleshooting path.
- Prioritizes root cause over symptoms.
- Produces stronger regression protection.

Runtime requirements:

- Reproduction command capture.
- Failure artifact summary.
- Explicit root-cause note.
- Regression-test association in the final summary.

## Recommended first workflow

The best first product workflow is:

```text
route -> plan -> approve -> implement -> verify -> review -> summarize
```

Working name: `delivery`.

Why this should come first:

- It uses primitives KodaCode already has.
- It fits KodaCode's value proposition: trustworthy software delivery.
- It improves common user work without requiring speculative autonomy.
- It can be implemented incrementally behind runtime events and TUI status.

Example configuration shape:

```yaml
workflow_templates:
  delivery:
    review_mode: auto
    max_revision_loops: 2
    phases:
      - route
      - plan
      - approve
      - implement
      - verify
      - review
      - summarize
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
```

The workflow should be invokable explicitly:

```text
/workflow delivery
kodacode --workflow delivery "add the feature and verify it"
```

If the user explicitly selects a workflow, runtime should treat that as the
source of truth. Classifiers may still warn about mismatches, but they should
not silently switch workflows.

## Project-local workflow definitions

KodaCode should support project-local workflow YAML files so teams can define
their own delivery process without turning that process into prompt text.

Suggested locations:

- Built-in workflows embedded in runtime code.
- Global user workflows under `~/.config/kodacode/workflows/*.yaml`.
- Project-local workflows under `.kodacode/workflows/*.yaml`.

Resolution order should mirror agent resolution:

1. Embedded built-ins.
2. Global user workflows.
3. Project-local workflows.

That lets a project add new workflows or override a built-in workflow when the
team has stricter local requirements.

Workflow YAML should be a runtime contract. It should declare phases, agents,
tool boundaries, required evidence, approval gates, limits, and stop
conditions. It should not be arbitrary scripting and should not be treated as a
long prompt.

Example project-local workflow:

```yaml
id: delivery
description: Plan, implement, verify, and review a code change.

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

  - id: implement
    agent: engineer
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

  - id: verify
    type: verification
    commands:
      - go test ./...
    required: true

  - id: review
    agent: reviewer
    mode: read_only
    review_fanout: true
    review_passes:
      - id: correctness
        description: Behavioral regressions and implementation correctness.
      - id: tests
        description: Verification coverage, edge cases, and missing checks.
      - id: contracts
        description: API, config, permission, and compatibility contracts.
    requires:
      - git_diff
      - verification_result

  - id: summarize
    type: final
    include:
      - changed_files
      - verification_result
      - review_outcome
      - unresolved_risks

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
```

The workflow executor should split responsibility into three layers:

- Workflow definition: YAML declares what phases exist and what evidence each
  phase needs.
- Runtime executor: KodaCode validates the YAML, advances phases, enforces
  approvals, records events, and blocks invalid transitions.
- Agent execution: agents perform bounded work inside the current phase with a
  narrowed tool surface and explicit objective.

Project-local workflow definitions are especially useful for:

- Repositories with required review or verification steps.
- Teams that want every broad change to start with an approved plan.
- Projects with known test commands or release checks.
- Security-sensitive repos where shell, network, or external writes need
  stricter handling.
- Documentation repos where source accuracy checks matter more than code tests.

Important safety constraints:

- Workflow YAML must not override sandbox or permission policy.
- Workflow YAML must not grant tools that the selected agent or runtime mode
  forbids.
- Shell commands declared in YAML must still use normal execution and approval
  policy.
- Phase completion must require durable evidence when the phase declares it.
- "Agent said done" should not satisfy verification or review requirements by
  itself.
- Invalid workflow definitions should fail at load time with clear errors.

## Suggested implementation order

1. Add workflow phase events.
2. Add a visible workflow phase/status model in the TUI.
3. Add the `delivery` workflow as a runtime template.
4. Add workflow YAML loading and validation for built-in, global, and
   project-local definitions.
5. Teach `engineer` to bind `task_workflow` tasks to phases.
6. Link verification commands and diagnostics to the active task or phase.
7. Attach automatic reviewer turns to the workflow result.
8. Add branch comparison as a later workflow, since it depends on richer
   summary and comparison UI.

## Design constraints

Workflow state must remain runtime-owned.

Prompts may guide behavior, but they should not be the source of truth for:

- Current phase
- Task status
- Approval state
- Review status
- Verification status
- Branch lineage
- Cost or budget stop conditions

Every workflow should be inspectable through events and `/trace`, resumable
through sessions, and bounded by explicit stop conditions.

## Avoiding heuristic dependence

KodaCode should not depend on hidden heuristics to decide what is allowed to
happen next. Heuristics are useful for recommendations, but runtime contracts
should define workflow authority.

Authority order:

1. Explicit user-selected workflow.
2. Deterministic runtime contract.
3. Workflow phase state.
4. Agent recommendation or classifier output.

Examples of explicit workflow selection:

- `/workflow delivery`
- `/workflow debug`
- `/workflow review`
- `kodacode --workflow delivery "..."`

Examples of deterministic runtime contracts:

- External path write requires approval.
- Destructive command requires approval.
- Network access follows configured network policy.
- Dirty worktree context can be shown without model classification.
- Review workflow uses a read-focused reviewer.
- Planner workflow remains read-only unless the user applies the plan.

Workflow phases should declare required evidence instead of relying on the
model's informal judgment. For example:

```yaml
workflow_templates:
  delivery:
    phases:
      - plan
      - approve
      - implement
      - verify
      - review
      - summarize
    requires:
      before_implement:
        - approved_plan
      before_review:
        - git_diff
      before_done:
        - verification_result
        - review_outcome
```

Classifier output should be structured and inspectable:

```yaml
recommendation:
  workflow: delivery
  confidence: medium
  reasons:
    - request touches implementation and verification
    - task appears broader than a single-file edit
  alternatives:
    - builder
    - planner
```

Low-confidence classification should ask the user or default to the least risky
path, usually planning or read-only exploration. The key rule is that
classification may recommend a workflow, but durable runtime state decides what
can happen next.

## Feature progress tracker

Status: done

Branch: `workflow`

Current milestone: none

Open decisions for later iterations:

- Whether verification commands should be reusable named checks or inline
  commands only.

Milestones:

### 1. Workflow definition schema

Scope:

- Define the YAML structure for workflow id, description, phases, agents, tool
  constraints, required evidence, approvals, limits, and stop conditions.
- Keep the schema declarative. It should not be arbitrary scripting.

Acceptance criteria:

- Runtime can parse a workflow YAML file into a typed definition.
- Validation rejects duplicate phase ids, missing phase ids, unknown agents,
  unknown phase types, unknown tools, malformed requirements, and unsafe tool
  grants.
- Workflow definitions cannot override sandbox, permission policy, or agent mode
  boundaries.

Tests:

- Valid delivery workflow parses successfully.
- Invalid YAML fails with a clear error.
- Duplicate phase ids fail.
- Unknown tool names fail.
- A workflow cannot grant a tool forbidden by the selected agent or runtime
  mode.

Status: done

### 2. Workflow catalog and resolution

Scope:

- Load embedded workflows.
- Load global workflows from `~/.config/kodacode/workflows/*.yaml`.
- Load project-local workflows from `.kodacode/workflows/*.yaml`.
- Resolve workflow ids using embedded, then global, then project-local order.

Acceptance criteria:

- Project-local workflow definitions can add new workflows.
- Project-local workflow definitions can override global or embedded workflows.
- Catalog load failures identify the file and validation problem.
- Runtime exposes the resolved workflow catalog to commands and session startup.

Tests:

- Embedded-only catalog works.
- Global workflow overrides embedded workflow with the same id.
- Project-local workflow overrides both global and embedded definitions.
- Invalid project-local workflow reports the bad path.

Status: done

### 3. Workflow selection

Scope:

- Add explicit workflow selection in the TUI and CLI.
- Persist the selected workflow in session events.
- Keep classifier recommendations advisory.

Acceptance criteria:

- `/workflow` lists available workflows.
- `/workflow delivery` selects the delivery workflow.
- `kodacode --workflow delivery "..."` starts the turn with that workflow.
- Resume restores the selected workflow from events.
- Explicit user selection beats classifier recommendations.

Tests:

- Unknown workflow id is rejected.
- Selected workflow is recorded in the event stream.
- Resume restores workflow selection.
- Classifier recommendation does not silently override explicit selection.

Status: done

### 4. Runtime executor and phase events

Scope:

- Add workflow phase events.
- Track current phase, completed phases, blocked phases, and stop reasons.
- Restore workflow state from replay.

Acceptance criteria:

- Runtime emits events when a workflow starts, advances, blocks, resumes, and
  completes.
- Invalid phase transitions are blocked.
- Resume reconstructs active workflow and phase state without hidden UI state.
- Stop reasons are durable and visible.

Tests:

- Phase advance emits the expected event.
- Required prior phase prevents invalid transition.
- Blocked workflow resumes at the same phase.
- Replay reconstructs workflow status.

Status: done

### 5. Phase contracts and evidence

Scope:

- Enforce required evidence before phase transitions.
- Record approval, verification, diff, diagnostics, and review evidence as
  durable runtime artifacts.

Acceptance criteria:

- Approval phase blocks implementation until the user approves.
- Verification phase records command result, exit code, and relevant output.
- Review phase records `task_review` outcomes.
- A phase cannot complete from model prose alone when required evidence is
  declared.

Tests:

- Implementation is blocked before required approval.
- Review is blocked before required verification evidence.
- Failed verification records a stop reason.
- Required evidence survives replay.

Status: done

### 6. Agent integration

Scope:

- Run agents inside the active phase with narrowed context and tool surface.
- Bind `task_workflow` tasks to workflow phases.
- Ensure YAML cannot grant capabilities outside runtime or agent policy.

Acceptance criteria:

- Planner phases run read-only.
- Reviewer phases run read-focused and record review outcomes.
- Engineer implementation phases can mutate only through permitted tools.
- Task updates include the active workflow phase.

Tests:

- Read-only phase cannot call mutation tools.
- Reviewer phase cannot edit files.
- Tool surface is the intersection of runtime policy, agent policy, and phase
  policy.
- Task state records phase association.

Status: done

### 7. TUI, trace, and user visibility

Scope:

- Show active workflow, active phase, blocked state, and stop reason.
- Add workflow phase history to `/trace`.
- Make phase status visible in both shell and classic layouts.

Acceptance criteria:

- Users can see the active workflow and phase during a turn.
- Blocked workflow states explain what evidence or approval is missing.
- `/trace` shows workflow start, phase changes, evidence, and completion.
- Status display does not rely on prompt text.

Tests:

- Workflow status renders in shell layout.
- Workflow status renders in classic layout.
- Trace includes phase event history.
- Blocked reason is visible.

Status: done

### 8. Built-in workflows and docs

Scope:

- Ship built-in `delivery`, `debug`, `review`, and `explore` workflows.
- Document project-local workflow files.
- Provide examples and safety rules.

Acceptance criteria:

- Built-in workflows are available without local YAML.
- Docs explain `.kodacode/workflows/*.yaml`.
- Example workflows cover code delivery, debugging, review, and read-only
  exploration.
- Documentation states that workflow YAML cannot override sandbox or permission
  policy.

Tests:

- Built-in workflows parse and validate.
- Documentation examples parse and validate.

Status: done

## Success criteria

A KodaCode workflow feature is successful when:

- Users can see what phase the system is in.
- Users can inspect why an agent or model route was selected.
- Human approval gates happen before risky side effects.
- Reviewer outcomes are durable and tied to tasks.
- Verification evidence is visible, not just summarized.
- Resume continues the same workflow state instead of inventing a new one.
- Costs and iteration limits are enforced by runtime.
