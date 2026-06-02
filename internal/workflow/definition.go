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
	ErrWorkflowAgentUnknown         = errors.New("workflow phase agent is unknown")
	ErrWorkflowToolUnknown          = errors.New("workflow phase tool is unknown")
	ErrWorkflowToolForbidden        = errors.New("workflow phase tool is forbidden by agent policy")
	ErrWorkflowToolUnsafe           = errors.New("workflow phase tool is unsafe for phase mode")
	ErrWorkflowReviewModeInvalid    = errors.New("workflow review_mode is invalid")
	ErrWorkflowRevisionLoopsInvalid = errors.New("workflow max_revision_loops is invalid")
	ErrWorkflowApprovalSkipInvalid  = errors.New("workflow approval skip_when is invalid")
	ErrWorkflowTransitionInvalid    = errors.New("workflow transition is invalid")
)

type PhaseType string

const (
	PhaseTypeAgent        PhaseType = "agent"
	PhaseTypeUserApproval PhaseType = "user_approval"
	PhaseTypeVerification PhaseType = "verification"
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
	ReviewMode       string       `yaml:"review_mode"`
	MaxRevisionLoops int          `yaml:"max_revision_loops"`
	Phases           []Phase      `yaml:"phases"`
	Transitions      []Transition `yaml:"transitions"`
}

type Phase struct {
	ID             string               `yaml:"id"`
	Type           PhaseType            `yaml:"type"`
	Agent          string               `yaml:"agent"`
	Mode           PhaseMode            `yaml:"mode"`
	Prompt         string               `yaml:"prompt"`
	Tools          ToolPolicy           `yaml:"tools"`
	Requires       EvidenceRequirements `yaml:"requires"`
	RequiresOutput []string             `yaml:"requires_output"`
	Commands       []string             `yaml:"commands"`
	Required       bool                 `yaml:"required"`
	Include        []string             `yaml:"include"`
	SkipWhen       ApprovalSkipRules    `yaml:"skip_when"`
}

type ToolPolicy struct {
	Allow []string `yaml:"allow"`
	Deny  []string `yaml:"deny"`
}

type EvidenceRequirements struct {
	Items  []string
	Fields map[string]string
}

type ApprovalSkipRules struct {
	MaxAffectedFiles int `yaml:"max_affected_files"`
}

type Transition struct {
	From     string `yaml:"from"`
	On       string `yaml:"on"`
	To       string `yaml:"to"`
	MaxLoops int    `yaml:"max_loops"`
}

const (
	TransitionOnSkipped            = "skipped"
	TransitionOnVerificationFailed = "verification_failed"
)

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
		return Definition{}, fmt.Errorf("workflow yaml: %w", err)
	}
	if err := definition.Validate(ctx); err != nil {
		return Definition{}, err
	}
	return definition, nil
}

func (d Definition) Validate(ctx ValidationContext) error {
	if strings.TrimSpace(d.ID) == "" {
		return ErrWorkflowIDRequired
	}
	if err := validateReviewMode(d.ReviewMode); err != nil {
		return err
	}
	if d.MaxRevisionLoops < 0 {
		return fmt.Errorf("%w: %d", ErrWorkflowRevisionLoopsInvalid, d.MaxRevisionLoops)
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
	if err := validatePhaseAgent(p, ctx); err != nil {
		return err
	}
	if err := validatePhaseTools(p, ctx); err != nil {
		return err
	}
	if err := validatePhaseApprovalSkip(p); err != nil {
		return err
	}
	return nil
}

func validatePhaseType(p Phase) error {
	typ := p.EffectiveType()
	switch typ {
	case PhaseTypeAgent, PhaseTypeUserApproval, PhaseTypeVerification, PhaseTypeFinal:
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

func validatePhaseApprovalSkip(p Phase) error {
	if p.SkipWhen.MaxAffectedFiles < 0 {
		return fmt.Errorf("%w: max_affected_files must be non-negative", ErrWorkflowApprovalSkipInvalid)
	}
	if p.SkipWhen.MaxAffectedFiles > 0 && p.EffectiveType() != PhaseTypeUserApproval {
		return fmt.Errorf("%w: skip_when is only supported on user_approval phases", ErrWorkflowApprovalSkipInvalid)
	}
	return nil
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
		case TransitionOnSkipped, TransitionOnVerificationFailed:
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
		tool.DelegateToolName,
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
