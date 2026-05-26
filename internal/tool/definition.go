package tool

import (
	"context"
	"encoding/json"

	"github.com/sageil/kodacode/internal/workspace"
)

const DefinitionToolName = "definition"

type DefinitionTool struct{}

func NewDefinitionTool() DefinitionTool {
	return DefinitionTool{}
}

func (DefinitionTool) Definition() Definition {
	description := "Find the declaration for a symbol at a known file position. Send `symbol` instead of calculating `character` when the symbol text is visible. Lines are 1-based; characters are 0-based."
	return Definition{
		Name:                 DefinitionToolName,
		Description:          description,
		ProviderDescription:  description,
		InputSchema:          json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Workspace-relative source file."},"line":{"type":["integer","string"],"minimum":1,"description":"1-based line containing the symbol."},"character":{"type":["integer","string","null"],"minimum":0,"description":"0-based character offset. Use when known; otherwise send symbol."},"symbol":{"type":["string","null"],"description":"Exact symbol text visible on the line."}},"required":["path","line"],"anyOf":[{"required":["character"]},{"required":["symbol"]}],"additionalProperties":false}`),
		ArgumentExamples:     []string{`{"path":"src/app.ts","line":12,"symbol":"buildCacheKey"}`, `{"path":"src/app.ts","line":12,"character":4}`},
		RequiresWorkspace:    true,
		ProviderRichGuidance: true,
		ParallelSafe:         true,
	}
}

func (DefinitionTool) PathRequests(args json.RawMessage) ([]PathRequest, error) {
	input, err := parseDefinitionInput(args)
	if err != nil {
		return nil, err
	}
	return []PathRequest{{
		Access: workspace.AccessRead,
		Path:   input.Path,
		Reason: "inspect source for definition lookup",
	}}, nil
}

func (DefinitionTool) Execute(ctx context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	codeIntel, err := ectx.CodeIntel()
	if err != nil {
		return Result{}, err
	}
	input, err := parseDefinitionInput(args)
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

	var locations []CodeIntelLocation
	for _, candidate := range bestEffortPositionCandidates(decision.ResolvedPath, input.Line, character) {
		locations, err = codeIntel.Definition(ctx, decision.ResolvedPath, input.Line-1, candidate)
		if err != nil {
			if notice, ok := AsCodeIntelNotice(err); ok {
				return Result{Output: formatCodeIntelNotice(DefinitionToolName, notice)}, nil
			}
			return Result{}, err
		}
		if len(locations) > 0 {
			break
		}
	}
	return Result{Output: formatDefinitionLocations(definitionTitle(decision.InputPath, input.Line, character), locations)}, nil
}
