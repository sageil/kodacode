package tool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	searchsvc "github.com/sageil/kodacode/internal/search"
	websearchsvc "github.com/sageil/kodacode/internal/websearch"
	"github.com/sageil/kodacode/internal/workspace"
)

var ErrWorkspaceRequired = errors.New("workspace scope is required")
var ErrQuestionAskerRequired = errors.New("question asker is required")
var ErrCodeIntelRequired = errors.New("code intel is required")
var ErrDelegateManagerRequired = errors.New("delegate manager is required")
var ErrWorkflowPhaseOutputManagerRequired = errors.New("workflow phase output manager is required")
var ErrWorkflowReviewResultManagerRequired = errors.New("workflow review result manager is required")

type Definition struct {
	Name                string
	Description         string
	ProviderDescription string
	InputKind           InputKind
	InputSchema         json.RawMessage
	InputFormat         *InputFormat
	ProviderInputSchema json.RawMessage
	ArgumentExamples    []string
	RequiresWorkspace   bool
	// ProviderRichGuidance keeps schema descriptions and a compact example in the
	// provider-facing tool surface for tools where precise usage materially
	// reduces bad calls.
	ProviderRichGuidance bool
	// ParallelSafe means Execute is read-only, does not use direct permission,
	// network, or execution append paths, and may run concurrently while the
	// runtime commits durable tool events later in provider order.
	ParallelSafe bool
}

type InputKind string

const (
	InputKindFunction InputKind = "function"
	InputKindCustom   InputKind = "custom"
)

type InputFormat struct {
	Type       string
	Syntax     string
	Definition string
}

func (d Definition) InputKindOrDefault() InputKind {
	if strings.TrimSpace(string(d.InputKind)) == "" {
		return InputKindFunction
	}
	return d.InputKind
}

func (d Definition) ProviderDescriptionText() string {
	if text := strings.TrimSpace(d.ProviderDescription); text != "" {
		return text
	}
	return strings.TrimSpace(d.Description)
}

type PathRequest struct {
	Access workspace.Access
	Path   string
	Reason string
}

type NetworkRequest struct {
	Target  string
	URL     string
	Command string
	Reason  string
}

type Result struct {
	Output            string
	Error             string
	StructuredResult  json.RawMessage
	Execution         *ExecutionRuntime
	PendingQuestionID string
	MutationRanges    []MutationRange
	TextMutations     []TextMutation
	ObservedResources []ObservedResource
}

type MutationRange struct {
	OldStartLine int
	NewStartLine int
}

type TextMutation struct {
	Path    string
	Before  string
	After   string
	Existed bool
	Mode    uint32
}

type ObservedResourceKind string

const (
	ObservedResourceFileContent ObservedResourceKind = "file_content"
	ObservedResourceDirEntries  ObservedResourceKind = "dir_entries"
)

type ObservedResource struct {
	Kind       ObservedResourceKind
	Path       string
	Version    string
	State      string
	Complete   bool
	StartLine  int
	EndLine    int
	TotalLines int
}

type OutputChunk struct {
	Stream string
	Chunk  string
}

type OutputEmitter func(OutputChunk) error

type BeforeFileMutation func(resolvedPath string) error

type QuestionRequest struct {
	Question string
	Options  []string
	Multiple bool
	Purpose  string
}

type QuestionResponse struct {
	RequestID string
	Answer    string
	Answered  bool
}

type QuestionAsker func(QuestionRequest) (QuestionResponse, error)

type ExecutionContext struct {
	SessionID       string
	Workspace       *workspace.Scope
	Search          *searchsvc.Service
	WebSearch       *websearchsvc.Service
	OutputEmitter   OutputEmitter
	BeforeMutation  BeforeFileMutation
	QuestionAsker   QuestionAsker
	TaskManager     TaskManager
	DelegateManager DelegateManager
	CodeIntelAPI    CodeIntel
	MemoryManager   MemoryManager
	SkillCatalog    SkillCatalog
	WorkflowOutput  WorkflowPhaseOutputManager
	WorkflowReview  WorkflowReviewResultManager
}

func (e ExecutionContext) ResolvePath(access workspace.Access, path string) (workspace.Decision, error) {
	if e.Workspace == nil {
		return workspace.Decision{}, ErrWorkspaceRequired
	}
	return e.Workspace.Check(access, path)
}

func (e ExecutionContext) EmitToolOutput(stream, chunk string) error {
	if e.OutputEmitter == nil || chunk == "" {
		return nil
	}
	return e.OutputEmitter(OutputChunk{
		Stream: stream,
		Chunk:  chunk,
	})
}

func (e ExecutionContext) AskQuestion(request QuestionRequest) (QuestionResponse, error) {
	if e.QuestionAsker == nil {
		return QuestionResponse{}, ErrQuestionAskerRequired
	}
	return e.QuestionAsker(request)
}

func (e ExecutionContext) CodeIntel() (CodeIntel, error) {
	if e.CodeIntelAPI == nil {
		return nil, ErrCodeIntelRequired
	}
	return e.CodeIntelAPI, nil
}

func (e ExecutionContext) Memories() (MemoryManager, error) {
	if e.MemoryManager == nil {
		return nil, ErrMemoryManagerRequired
	}
	return e.MemoryManager, nil
}

func (e ExecutionContext) Skills() (SkillCatalog, error) {
	if e.SkillCatalog == nil {
		return nil, ErrSkillCatalogRequired
	}
	return e.SkillCatalog, nil
}

func (e ExecutionContext) WorkflowPhaseOutput() (WorkflowPhaseOutputManager, error) {
	if e.WorkflowOutput == nil {
		return nil, ErrWorkflowPhaseOutputManagerRequired
	}
	return e.WorkflowOutput, nil
}

func (e ExecutionContext) WorkflowReviewResult() (WorkflowReviewResultManager, error) {
	if e.WorkflowReview == nil {
		return nil, ErrWorkflowReviewResultManagerRequired
	}
	return e.WorkflowReview, nil
}

type Tool interface {
	Definition() Definition
	Execute(context.Context, ExecutionContext, json.RawMessage) (Result, error)
}

type ExecutionIntent string

const (
	ExecutionIntentUnknown ExecutionIntent = "unknown"
	ExecutionIntentOneShot ExecutionIntent = "one_shot"
	ExecutionIntentWatcher ExecutionIntent = "watcher"
	ExecutionIntentServer  ExecutionIntent = "server"
)

type ExecutionEffect string

const (
	ExecutionEffectUnknown ExecutionEffect = "unknown"
	ExecutionEffectRead    ExecutionEffect = "read"
	ExecutionEffectWrite   ExecutionEffect = "write"
	ExecutionEffectNetwork ExecutionEffect = "network"
	ExecutionEffectProcess ExecutionEffect = "process"
)

type PathIntrospector interface {
	PathRequests(json.RawMessage) ([]PathRequest, error)
}

type NetworkIntrospector interface {
	NetworkRequests(json.RawMessage) ([]NetworkRequest, error)
}

type ExecutionRequest struct {
	Kind             string
	Preview          string
	WorkingDirectory string
	Command          []string
	ShellCommand     string
	PathRequests     []PathRequest
	OpaquePathReason string
	NetworkTargets   []string
	Intent           ExecutionIntent
	IntentCommand    string
	Effect           ExecutionEffect
	ReadyPatterns    []string
	TimeoutMS        int
	LoginShell       bool
	ShellProgram     string
	TTY              bool
	YieldTimeMS      int
	MaxOutputTokens  int
	PrefixRule       []string
}

type ExecutionRequestIntrospector interface {
	ExecutionRequest(workspaceRoot string, args json.RawMessage) (ExecutionRequest, bool, error)
}

type NormalizedInputKeyProvider interface {
	NormalizedInputKey(json.RawMessage) (string, error)
}
