package workflow

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/agent"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
	"gopkg.in/yaml.v3"
)

var (
	ErrWorkflowIDRequired           = errors.New("workflow id is required")
	ErrWorkflowPhaseRequired        = errors.New("workflow phase is required")
	ErrWorkflowPhaseIDRequired      = errors.New("workflow phase id is required")
	ErrWorkflowPhaseDuplicate       = errors.New("workflow phase id is duplicated")
	ErrWorkflowPhaseTypeInvalid     = errors.New("workflow phase type is invalid")
	ErrWorkflowPhaseModeInvalid     = errors.New("workflow phase mode is invalid")
	ErrWorkflowAgentRequired        = errors.New("workflow phase agent is required")
	ErrWorkflowAgentUnknown         = errors.New("workflow phase agent is unknown")
	ErrWorkflowToolUnknown          = errors.New("workflow phase tool is unknown")
	ErrWorkflowToolForbidden        = errors.New("workflow phase tool is forbidden by agent policy")
	ErrWorkflowToolUnsafe           = errors.New("workflow phase tool is unsafe for phase mode")
	ErrWorkflowReviewModeInvalid    = errors.New("workflow review_mode is invalid")
	ErrWorkflowReviewPassInvalid    = errors.New("workflow review_passes is invalid")
	ErrWorkflowRevisionLoopsInvalid = errors.New("workflow max_revision_loops is invalid")
	ErrWorkflowApprovalSkipInvalid  = errors.New("workflow approval skip_when is invalid")
	ErrWorkflowTransitionInvalid    = errors.New("workflow transition is invalid")
	ErrWorkflowModelInvalid         = errors.New("workflow model is invalid")
	ErrWorkflowBudgetInvalid        = errors.New("workflow budgets are invalid")
	ErrWorkflowCommandInvalid       = errors.New("workflow command is invalid")
	ErrWorkflowAutoContinueInvalid  = errors.New("workflow auto_continue is invalid")
)

type PhaseType string

const (
	PhaseTypeAgent        PhaseType = "agent"
	PhaseTypeUserApproval PhaseType = "user_approval"
	PhaseTypeVerification PhaseType = "verification"
	PhaseTypeReview       PhaseType = "review"
	PhaseTypeFinal        PhaseType = "final"
)

type PhaseMode string

const (
	PhaseModeDefault  PhaseMode = ""
	PhaseModeReadOnly PhaseMode = "read_only"
)

type Definition struct {
	ID               string       `yaml:"id"`
	Description      string       `yaml:"description"`
	Model            string       `yaml:"model"`
	ReviewMode       string       `yaml:"review_mode"`
	MaxRevisionLoops int          `yaml:"max_revision_loops"`
	Budgets          Budgets      `yaml:"budgets"`
	Phases           []Phase      `yaml:"phases"`
	Transitions      []Transition `yaml:"transitions"`
}

type Phase struct {
	ID             string                `yaml:"id"`
	Type           PhaseType             `yaml:"type"`
	Agent          string                `yaml:"agent"`
	Mode           PhaseMode             `yaml:"mode"`
	Model          string                `yaml:"model"`
	Prompt         string                `yaml:"prompt"`
	Tools          ToolPolicy            `yaml:"tools"`
	Requires       EvidenceRequirements  `yaml:"requires"`
	Completion     PhaseCompletion       `yaml:"completion"`
	RequiresOutput []string              `yaml:"requires_output"`
	Commands       []VerificationCommand `yaml:"commands"`
	Required       bool                  `yaml:"required"`
	Include        []string              `yaml:"include"`
	SkipWhen       ApprovalSkipRules     `yaml:"skip_when"`
	ReviewPasses   []ReviewPass          `yaml:"review_passes"`
	AutoContinue   *bool                 `yaml:"auto_continue"`
}

type ToolPolicy struct {
	Allow []string `yaml:"allow"`
	Deny  []string `yaml:"deny"`
}

type EvidenceRequirements struct {
	Items  []string
	Fields map[string]string
}

type PhaseCompletion struct {
	Requires EvidenceRequirements `yaml:"requires"`
}

type ApprovalSkipRules struct {
	MaxAffectedFiles int `yaml:"max_affected_files"`
}

type ReviewPass struct {
	ID           string   `yaml:"id"`
	Description  string   `yaml:"description"`
	Instructions []string `yaml:"instructions"`
}

type VerificationCommand struct {
	Tool    string `yaml:"tool"`
	Command string `yaml:"command"`
}

type Transition struct {
	From     string `yaml:"from"`
	On       string `yaml:"on"`
	To       string `yaml:"to"`
	MaxLoops int    `yaml:"max_loops"`
}

type Budgets struct {
	MaxCost                    float64 `yaml:"max_cost"`
	WarnThreshold              float64 `yaml:"warn_threshold"`
	MaxProviderRequestsPerTurn int     `yaml:"max_provider_requests_per_turn"`
}

const (
	TransitionOnSkipped              = "skipped"
	TransitionOnVerificationFailed   = "verification_failed"
	TransitionOnReviewFailed         = "review_failed"
	TransitionOnTurnFailed           = "turn_failed"
	TransitionOnBudgetExceeded       = "budget_exceeded"
	TransitionOnProviderRequestLimit = "provider_request_limit"
	TransitionOnNoProgress           = "no_progress"
	TransitionOnCanceled             = "canceled"
)

const CompletionRequirementActivePhaseTasksComplete = "active_phase_tasks_complete"

type ValidationContext struct {
	Agents map[string]agent.Definition
	Tools  map[string]struct{}
}

func NewValidationContext(agents []agent.Definition, tools []tool.Tool) ValidationContext {
	ctx := ValidationContext{
		Agents: make(map[string]agent.Definition, len(agents)),
		Tools:  make(map[string]struct{}, len(tools)),
	}
	for _, definition := range agents {
		id := strings.TrimSpace(definition.ID)
		if id != "" {
			ctx.Agents[id] = definition
		}
	}
	for _, runtimeTool := range tools {
		name := strings.TrimSpace(runtimeTool.Definition().Name)
		if name != "" {
			ctx.Tools[name] = struct{}{}
		}
	}
	return ctx
}

func LoadFile(path string, ctx ValidationContext) (Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Definition{}, err
	}
	definition, err := LoadBytes(data, ctx)
	if err != nil {
		return Definition{}, fmt.Errorf("%s: %w", path, err)
	}
	return definition, nil
}

func LoadBytes(data []byte, ctx ValidationContext) (Definition, error) {
	var definition Definition
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&definition); err != nil {
		return Definition{}, workflowYAMLDecodeError(err)
	}
	if err := definition.Validate(ctx); err != nil {
		return Definition{}, err
	}
	return definition, nil
}

var removedWorkflowYAMLFields = map[string]string{
	"parallel_review": "parallel review fanout was removed with delegated workflow sessions; remove this key and keep review_passes for the review lenses that should run in the review phase",
	"review_fanout":   "review fanout was removed with delegated workflow sessions; remove this key and keep review_passes for the review lenses that should run in the review phase",
}

func workflowYAMLDecodeError(err error) error {
	var typeErr *yaml.TypeError
	if errors.As(err, &typeErr) {
		for _, msg := range typeErr.Errors {
			for field, guidance := range removedWorkflowYAMLFields {
				if strings.Contains(msg, "field "+field+" not found") {
					return fmt.Errorf("workflow yaml: removed field %s is not supported: %s: %w", field, guidance, err)
				}
			}
		}
	}
	return fmt.Errorf("workflow yaml: %w", err)
}

func (d Definition) Validate(ctx ValidationContext) error {
	if strings.TrimSpace(d.ID) == "" {
		return ErrWorkflowIDRequired
	}
	if err := validateReviewMode(d.ReviewMode); err != nil {
		return err
	}
	if err := validateWorkflowModel(d.Model); err != nil {
		return err
	}
	if d.MaxRevisionLoops < 0 {
		return fmt.Errorf("%w: %d", ErrWorkflowRevisionLoopsInvalid, d.MaxRevisionLoops)
	}
	if err := validateBudgets(d.Budgets); err != nil {
		return err
	}
	if len(d.Phases) == 0 {
		return ErrWorkflowPhaseRequired
	}
	seen := map[string]struct{}{}
	for index, phase := range d.Phases {
		if err := phase.Validate(ctx); err != nil {
			return fmt.Errorf("phase %d: %w", index+1, err)
		}
		id := strings.TrimSpace(phase.ID)
		if _, ok := seen[id]; ok {
			return fmt.Errorf("%w: %s", ErrWorkflowPhaseDuplicate, id)
		}
		seen[id] = struct{}{}
	}
	if err := validateTransitions(d.Transitions, seen); err != nil {
		return err
	}
	return nil
}

func validateReviewMode(mode string) error {
	switch strings.TrimSpace(mode) {
	case "", "off", "manual", "auto":
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrWorkflowReviewModeInvalid, mode)
	}
}

func validateWorkflowModel(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if _, err := provider.ParseModelRef(value); err != nil {
		return fmt.Errorf("%w: %v", ErrWorkflowModelInvalid, err)
	}
	return nil
}

func validateBudgets(budgets Budgets) error {
	if budgets.MaxCost < 0 {
		return fmt.Errorf("%w: max_cost must be non-negative", ErrWorkflowBudgetInvalid)
	}
	if budgets.WarnThreshold < 0 || budgets.WarnThreshold > 1 {
		return fmt.Errorf("%w: warn_threshold must be between 0 and 1", ErrWorkflowBudgetInvalid)
	}
	if budgets.MaxCost <= 0 && budgets.WarnThreshold > 0 {
		return fmt.Errorf("%w: warn_threshold requires a positive max_cost", ErrWorkflowBudgetInvalid)
	}
	if budgets.MaxProviderRequestsPerTurn < 0 {
		return fmt.Errorf("%w: max_provider_requests_per_turn must be non-negative", ErrWorkflowBudgetInvalid)
	}
	return nil
}

func (p Phase) Validate(ctx ValidationContext) error {
	id := strings.TrimSpace(p.ID)
	if id == "" {
		return ErrWorkflowPhaseIDRequired
	}
	if err := validatePhaseType(p); err != nil {
		return err
	}
	if err := validatePhaseMode(p); err != nil {
		return err
	}
	if err := validateWorkflowModel(p.Model); err != nil {
		return err
	}
	if err := validatePhaseAgent(p, ctx); err != nil {
		return err
	}
	if err := validatePhaseTools(p, ctx); err != nil {
		return err
	}
	if err := validatePhaseCommands(p, ctx); err != nil {
		return err
	}
	if err := validatePhaseApprovalSkip(p); err != nil {
		return err
	}
	if err := validatePhaseReviewPasses(p); err != nil {
		return err
	}
	if err := validatePhaseAutoContinue(p); err != nil {
		return err
	}
	return nil
}

func validatePhaseType(p Phase) error {
	typ := p.EffectiveType()
	switch typ {
	case PhaseTypeAgent, PhaseTypeUserApproval, PhaseTypeVerification, PhaseTypeReview, PhaseTypeFinal:
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrWorkflowPhaseTypeInvalid, typ)
	}
}

func validatePhaseMode(p Phase) error {
	switch strings.TrimSpace(string(p.Mode)) {
	case string(PhaseModeDefault), string(PhaseModeReadOnly):
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrWorkflowPhaseModeInvalid, p.Mode)
	}
}

func validatePhaseAgent(p Phase, ctx ValidationContext) error {
	id := strings.TrimSpace(p.Agent)
	if id == "" {
		switch p.EffectiveType() {
		case PhaseTypeAgent, PhaseTypeVerification, PhaseTypeReview:
			return fmt.Errorf("%w: %s", ErrWorkflowAgentRequired, strings.TrimSpace(p.ID))
		}
		return nil
	}
	definition, ok := ctx.Agents[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrWorkflowAgentUnknown, id)
	}
	if definition.EffectiveMode() == agent.ModeSubagent {
		return fmt.Errorf("%w: %s cannot run as primary workflow phase", agent.ErrAgentModeInvalid, id)
	}
	return nil
}

func validatePhaseTools(p Phase, ctx ValidationContext) error {
	agentDefinition, hasAgent := ctx.Agents[strings.TrimSpace(p.Agent)]
	for _, name := range append(slices.Clone(p.Tools.Allow), p.Tools.Deny...) {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := ctx.Tools[name]; !ok {
			return fmt.Errorf("%w: %s", ErrWorkflowToolUnknown, name)
		}
	}
	for _, name := range p.Tools.Allow {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if hasAgent && !agentDefinition.AllowsTool(name) {
			return fmt.Errorf("%w: %s", ErrWorkflowToolForbidden, name)
		}
		if p.Mode == PhaseModeReadOnly && mutationToolName(name) {
			return fmt.Errorf("%w: %s", ErrWorkflowToolUnsafe, name)
		}
	}
	return nil
}

func validatePhaseCommands(p Phase, ctx ValidationContext) error {
	if len(p.Commands) == 0 {
		return nil
	}
	if p.EffectiveType() != PhaseTypeVerification && !p.Required {
		return fmt.Errorf("%w: commands are only supported on verification phases", ErrWorkflowCommandInvalid)
	}
	agentDefinition, hasAgent := ctx.Agents[strings.TrimSpace(p.Agent)]
	for index, command := range p.Commands {
		toolName := strings.TrimSpace(command.Tool)
		if toolName == "" {
			return fmt.Errorf("%w: commands[%d].tool is required", ErrWorkflowCommandInvalid, index)
		}
		switch toolName {
		case tool.TestToolName, tool.BashToolName:
		default:
			return fmt.Errorf("%w: commands[%d].tool %s is not supported for verification", ErrWorkflowCommandInvalid, index, toolName)
		}
		if _, ok := ctx.Tools[toolName]; !ok {
			return fmt.Errorf("%w: %s", ErrWorkflowToolUnknown, toolName)
		}
		if hasAgent && !agentDefinition.AllowsTool(toolName) {
			return fmt.Errorf("%w: commands[%d].tool %s is forbidden by agent policy", ErrWorkflowCommandInvalid, index, toolName)
		}
		if p.Tools.Allow != nil && !stringListContains(p.Tools.Allow, toolName) {
			return fmt.Errorf("%w: commands[%d].tool %s is not allowed by phase tools", ErrWorkflowCommandInvalid, index, toolName)
		}
		if strings.TrimSpace(command.Command) == "" {
			return fmt.Errorf("%w: commands[%d].command is required", ErrWorkflowCommandInvalid, index)
		}
	}
	return nil
}

func stringListContains(values []string, needle string) bool {
	needle = strings.TrimSpace(needle)
	for _, value := range values {
		if strings.TrimSpace(value) == needle {
			return true
		}
	}
	return false
}

func validatePhaseApprovalSkip(p Phase) error {
	if p.SkipWhen.MaxAffectedFiles < 0 {
		return fmt.Errorf("%w: max_affected_files must be non-negative", ErrWorkflowApprovalSkipInvalid)
	}
	if p.SkipWhen.MaxAffectedFiles > 0 && p.EffectiveType() != PhaseTypeUserApproval {
		return fmt.Errorf("%w: skip_when is only supported on user_approval phases", ErrWorkflowApprovalSkipInvalid)
	}
	return nil
}

func validatePhaseReviewPasses(p Phase) error {
	if len(p.ReviewPasses) == 0 {
		return nil
	}
	if p.EffectiveType() != PhaseTypeReview {
		return fmt.Errorf("%w: review_passes are only supported on review phases", ErrWorkflowReviewPassInvalid)
	}
	seen := map[string]struct{}{}
	for index, pass := range p.ReviewPasses {
		id := strings.TrimSpace(pass.ID)
		if id == "" {
			return fmt.Errorf("%w: pass %d id is required", ErrWorkflowReviewPassInvalid, index+1)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("%w: duplicate pass id %s", ErrWorkflowReviewPassInvalid, id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func (p Phase) AutoContinueEnabled() bool {
	return p.AutoContinue != nil && *p.AutoContinue
}

func (p Phase) AutoContinueDisabled() bool {
	return p.AutoContinue != nil && !*p.AutoContinue
}

func validatePhaseAutoContinue(p Phase) error {
	if p.AutoContinue == nil || !p.AutoContinueEnabled() {
		return nil
	}
	switch p.EffectiveType() {
	case PhaseTypeAgent, PhaseTypeReview, PhaseTypeVerification:
		return nil
	default:
		return fmt.Errorf("%w: auto_continue requires a runtime-runnable phase", ErrWorkflowAutoContinueInvalid)
	}
}

func validateTransitions(transitions []Transition, phases map[string]struct{}) error {
	seen := map[string]struct{}{}
	for index, transition := range transitions {
		from := strings.TrimSpace(transition.From)
		on := strings.TrimSpace(transition.On)
		to := strings.TrimSpace(transition.To)
		if from == "" || on == "" || to == "" {
			return fmt.Errorf("transition %d: %w: from, on, and to are required", index+1, ErrWorkflowTransitionInvalid)
		}
		if _, ok := phases[from]; !ok {
			return fmt.Errorf("transition %d: %w: unknown from phase %s", index+1, ErrWorkflowTransitionInvalid, from)
		}
		if _, ok := phases[to]; !ok {
			return fmt.Errorf("transition %d: %w: unknown to phase %s", index+1, ErrWorkflowTransitionInvalid, to)
		}
		switch on {
		case TransitionOnSkipped,
			TransitionOnVerificationFailed,
			TransitionOnReviewFailed,
			TransitionOnTurnFailed,
			TransitionOnBudgetExceeded,
			TransitionOnProviderRequestLimit,
			TransitionOnNoProgress,
			TransitionOnCanceled:
		default:
			return fmt.Errorf("transition %d: %w: unknown event %s", index+1, ErrWorkflowTransitionInvalid, on)
		}
		if transition.MaxLoops < 0 {
			return fmt.Errorf("transition %d: %w: max_loops must be non-negative", index+1, ErrWorkflowTransitionInvalid)
		}
		key := from + "\x00" + on
		if _, ok := seen[key]; ok {
			return fmt.Errorf("transition %d: %w: duplicate transition for %s on %s", index+1, ErrWorkflowTransitionInvalid, from, on)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (p Phase) EffectiveType() PhaseType {
	typ := PhaseType(strings.TrimSpace(string(p.Type)))
	if typ != "" {
		return typ
	}
	if strings.TrimSpace(p.Agent) != "" {
		return PhaseTypeAgent
	}
	return typ
}

func (r *EvidenceRequirements) UnmarshalYAML(node *yaml.Node) error {
	if node == nil || node.Tag == "!!null" {
		return nil
	}
	switch node.Kind {
	case yaml.SequenceNode:
		items := make([]string, 0, len(node.Content))
		for _, item := range node.Content {
			var value string
			if err := item.Decode(&value); err != nil {
				return err
			}
			if strings.TrimSpace(value) != "" {
				items = append(items, strings.TrimSpace(value))
			}
		}
		r.Items = items
		return nil
	case yaml.MappingNode:
		fields := make(map[string]string, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := strings.TrimSpace(node.Content[i].Value)
			var value string
			if err := node.Content[i+1].Decode(&value); err != nil {
				return err
			}
			if key != "" {
				fields[key] = strings.TrimSpace(value)
			}
		}
		r.Fields = fields
		return nil
	default:
		return fmt.Errorf("requires must be a sequence or mapping")
	}
}

func mutationToolName(name string) bool {
	switch strings.TrimSpace(name) {
	case tool.ApplyPatchToolName,
		tool.BashToolName,
		tool.CodeActionToolName,
		"mkdir",
		tool.QuestionToolName,
		tool.RenameSymbolToolName,
		tool.TaskReviewToolName,
		tool.TaskWorkflowToolName,
		tool.TestToolName,
		tool.WriteToolName:
		return true
	default:
		return false
	}
}

func (d Definition) PhaseIDs() []string {
	out := make([]string, 0, len(d.Phases))
	for _, phase := range d.Phases {
		id := strings.TrimSpace(phase.ID)
		if id != "" {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}
