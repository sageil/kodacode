package tool

import (
	"context"
	"encoding/json"

	"github.com/sageil/kodacode/internal/workspace"
)

const RefsToolName = "refs"

type RefsTool struct{}

func NewRefsTool() RefsTool {
	return RefsTool{}
}

func (RefsTool) Definition() Definition {
	description := "Find semantic references for a symbol at a known file position. Use before behavior-changing edits to signatures, return types, fields, or cross-module methods. Send `symbol` instead of calculating `character` when visible."
	return Definition{
		Name:                 RefsToolName,
		Description:          description,
		ProviderDescription:  description,
		InputSchema:          json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Workspace-relative source file."},"line":{"type":["integer","string"],"minimum":1,"description":"1-based line containing the symbol."},"character":{"type":["integer","string","null"],"minimum":0,"description":"0-based character offset. Use when known; otherwise send symbol."},"symbol":{"type":["string","null"],"description":"Exact symbol text visible on the line."},"mode":{"type":["string","null"],"enum":["all","readers","writers",null],"description":"Reference mode. Omit for all; use writers for assignments and readers for read usages."},"max_results":{"type":["integer","string","null"],"minimum":1,"maximum":500,"description":"Maximum references to return. Defaults to 200."},"include_declaration":{"type":["boolean","string","null"],"description":"Whether to include the declaration location. Defaults to false."}},"required":["path","line"],"anyOf":[{"required":["character"]},{"required":["symbol"]}],"additionalProperties":false}`),
		ArgumentExamples:     []string{`{"path":"src/store.ts","line":18,"symbol":"value","mode":"writers","max_results":100,"include_declaration":false}`, `{"path":"src/store.ts","line":18,"character":7,"mode":"writers","max_results":100,"include_declaration":false}`},
		RequiresWorkspace:    true,
		ProviderRichGuidance: true,
	}
}

func (RefsTool) PathRequests(args json.RawMessage) ([]PathRequest, error) {
	input, err := parseRefsInput(args)
	if err != nil {
		return nil, err
	}
	return []PathRequest{{
		Access: workspace.AccessRead,
		Path:   input.Path,
		Reason: "inspect source for reference lookup",
	}}, nil
}

func (RefsTool) Execute(ctx context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	codeIntel, err := ectx.CodeIntel()
	if err != nil {
		return Result{}, err
	}
	input, err := parseRefsInput(args)
	if err != nil {
		return Result{}, err
	}
	decision, err := ectx.ResolvePath(workspace.AccessRead, input.Path)
	if err != nil {
		return Result{}, err
	}
	character, err := resolveCodeIntelCharacter(decision.ResolvedPath, input.Line, input.Character, input.HasCharacter, input.Symbol)
	if err != nil {
		return Result{}, err
	}
	result, err := codeIntel.Refs(ctx, CodeIntelRefsRequest{
		Path:               decision.ResolvedPath,
		Line:               input.Line,
		Character:          character,
		Mode:               input.Mode,
		MaxResults:         input.MaxResults,
		IncludeDeclaration: input.IncludeDeclaration,
	})
	if err != nil {
		if notice, ok := AsCodeIntelNotice(err); ok {
			result := CodeIntelRefsResult{Notice: notice}
			structured, marshalErr := MarshalStructuredResult(result)
			if marshalErr != nil {
				return Result{}, marshalErr
			}
			return Result{Output: formatRefsResult(CodeIntelRefsRequest{
				Path:               decision.ResolvedPath,
				Line:               input.Line,
				Character:          character,
				Mode:               input.Mode,
				MaxResults:         input.MaxResults,
				IncludeDeclaration: input.IncludeDeclaration,
			}, result), StructuredResult: structured}, nil
		}
		return Result{}, err
	}
	structured, err := MarshalStructuredResult(result)
	if err != nil {
		return Result{}, err
	}
	return Result{Output: formatRefsResult(CodeIntelRefsRequest{
		Path:               decision.ResolvedPath,
		Line:               input.Line,
		Character:          character,
		Mode:               input.Mode,
		MaxResults:         input.MaxResults,
		IncludeDeclaration: input.IncludeDeclaration,
	}, result), StructuredResult: structured}, nil
}
