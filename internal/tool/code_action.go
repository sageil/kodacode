package tool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/sageil/kodacode/internal/workspace"
)

const CodeActionToolName = "code_action"

var (
	ErrCodeActionPathRequired          = errors.New("path is required")
	ErrCodeActionStartLineInvalid      = errors.New("start_line must be at least 1")
	ErrCodeActionEndLineInvalid        = errors.New("end_line must be at least 1")
	ErrCodeActionStartCharacterInvalid = errors.New("start_character must be at least 0")
	ErrCodeActionEndCharacterInvalid   = errors.New("end_character must be at least 0")
	ErrCodeActionRangeInvalid          = errors.New("end position must not be before the start position")
)

type CodeActionTool struct{}

func NewCodeActionTool() CodeActionTool {
	return CodeActionTool{}
}

func (CodeActionTool) Definition() Definition {
	description := "Apply a language-server code action that produces workspace edits. Use after diagnostics for matching quick fixes, refactors, or organize imports. Use `only_preferred` for the highest-confidence fix. Command-only actions are rejected."
	return Definition{
		Name:                 CodeActionToolName,
		Description:          description,
		ProviderDescription:  description,
		InputSchema:          json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Workspace-relative source file."},"start_line":{"type":["integer","string"],"minimum":1,"description":"1-based range start line."},"start_character":{"type":["integer","string"],"minimum":0,"description":"0-based range start character."},"end_line":{"type":["integer","string"],"minimum":1,"description":"1-based range end line."},"end_character":{"type":["integer","string"],"minimum":0,"description":"0-based range end character."},"title":{"type":["string","null"],"description":"Optional exact action title."},"kind":{"type":["string","null"],"description":"Optional action kind such as quickfix or source.organizeImports."},"only_preferred":{"type":["boolean","string","null"],"description":"True selects only preferred actions. Defaults to false."}},"required":["path","start_line","start_character","end_line","end_character"],"additionalProperties":false}`),
		ArgumentExamples:     []string{`{"path":"src/app.ts","start_line":12,"start_character":4,"end_line":12,"end_character":10,"title":null,"kind":"quickfix","only_preferred":true}`},
		RequiresWorkspace:    true,
		ProviderRichGuidance: true,
	}
}

func (CodeActionTool) PathRequests(args json.RawMessage) ([]PathRequest, error) {
	input, err := parseCodeActionInput(args)
	if err != nil {
		return nil, err
	}
	return []PathRequest{{
		Access: workspace.AccessWrite,
		Path:   input.Path,
		Reason: "apply code action workspace edits",
	}}, nil
}

func (CodeActionTool) Execute(ctx context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	input, err := parseCodeActionInput(args)
	if err != nil {
		return Result{}, err
	}
	codeIntel, err := ectx.CodeIntel()
	if err != nil {
		return Result{}, err
	}
	decision, err := ectx.ResolvePath(workspace.AccessWrite, input.Path)
	if err != nil {
		return Result{}, err
	}
	result, err := codeIntel.ApplyCodeAction(ctx, CodeIntelCodeActionRequest{
		Path:           decision.ResolvedPath,
		StartLine:      input.StartLine,
		StartCharacter: input.StartCharacter,
		EndLine:        input.EndLine,
		EndCharacter:   input.EndCharacter,
		Title:          input.Title,
		Kind:           input.Kind,
		OnlyPreferred:  input.OnlyPreferred,
	})
	if err != nil {
		if notice, ok := AsCodeIntelNotice(err); ok {
			return Result{Output: formatCodeIntelNotice(CodeActionToolName, notice)}, nil
		}
		return Result{}, err
	}
	if len(result.Summary.Paths) == 0 {
		title := strings.TrimSpace(result.Title)
		if title == "" {
			return Result{Output: "No code-action edits were produced."}, nil
		}
		return Result{Output: `Code action "` + title + `" produced no file changes.`}, nil
	}
	return Result{Output: formatCodeIntelMutationSummary(`Applied code action "`+result.Title+`"`, result.Summary)}, nil
}

type codeActionInput struct {
	Path           string
	StartLine      int
	StartCharacter int
	EndLine        int
	EndCharacter   int
	Title          string
	Kind           string
	OnlyPreferred  bool
}

func parseCodeActionInput(args json.RawMessage) (_ codeActionInput, err error) {
	defer func() {
		err = normalizeToolInputError(CodeActionToolName, err)
	}()
	var raw struct {
		Path           string          `json:"path"`
		StartLine      json.RawMessage `json:"start_line"`
		StartCharacter json.RawMessage `json:"start_character"`
		EndLine        json.RawMessage `json:"end_line"`
		EndCharacter   json.RawMessage `json:"end_character"`
		Title          *string         `json:"title"`
		Kind           *string         `json:"kind"`
		OnlyPreferred  json.RawMessage `json:"only_preferred"`
	}
	if err := DecodeArgs(CodeActionToolName, args, &raw); err != nil {
		return codeActionInput{}, err
	}
	startLine, _, err := decodeOptionalIntArg(CodeActionToolName, raw.StartLine, "start_line")
	if err != nil {
		return codeActionInput{}, err
	}
	startCharacter, _, err := decodeOptionalIntArg(CodeActionToolName, raw.StartCharacter, "start_character")
	if err != nil {
		return codeActionInput{}, err
	}
	endLine, _, err := decodeOptionalIntArg(CodeActionToolName, raw.EndLine, "end_line")
	if err != nil {
		return codeActionInput{}, err
	}
	endCharacter, _, err := decodeOptionalIntArg(CodeActionToolName, raw.EndCharacter, "end_character")
	if err != nil {
		return codeActionInput{}, err
	}
	onlyPreferred, hasOnlyPreferred, err := decodeOptionalBoolArg(CodeActionToolName, raw.OnlyPreferred, "only_preferred")
	if err != nil {
		return codeActionInput{}, err
	}
	if !hasOnlyPreferred {
		onlyPreferred = false
	}
	if strings.TrimSpace(raw.Path) == "" {
		return codeActionInput{}, ErrCodeActionPathRequired
	}
	if startLine < 1 {
		return codeActionInput{}, ErrCodeActionStartLineInvalid
	}
	if endLine < 1 {
		return codeActionInput{}, ErrCodeActionEndLineInvalid
	}
	if startCharacter < 0 {
		return codeActionInput{}, ErrCodeActionStartCharacterInvalid
	}
	if endCharacter < 0 {
		return codeActionInput{}, ErrCodeActionEndCharacterInvalid
	}
	if endLine < startLine || (endLine == startLine && endCharacter < startCharacter) {
		return codeActionInput{}, ErrCodeActionRangeInvalid
	}
	return codeActionInput{
		Path:           raw.Path,
		StartLine:      startLine,
		StartCharacter: startCharacter,
		EndLine:        endLine,
		EndCharacter:   endCharacter,
		Title:          strings.TrimSpace(stringValue(raw.Title)),
		Kind:           strings.TrimSpace(stringValue(raw.Kind)),
		OnlyPreferred:  onlyPreferred,
	}, nil
}
