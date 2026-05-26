package tool

import (
	"context"
	"encoding/json"

	"github.com/sageil/kodacode/internal/workspace"
)

const TraceToolName = "trace"

type TraceTool struct{}

func NewTraceTool() TraceTool {
	return TraceTool{}
}

func (TraceTool) Definition() Definition {
	description := "Trace call relationships for a callable symbol. Use callers for incoming edges, callees for outgoing edges, and graph for bounded multi-hop flow. Use refs instead for flat usage lists. Send `symbol` instead of calculating `character` when visible."
	return Definition{
		Name:                 TraceToolName,
		Description:          description,
		ProviderDescription:  description,
		InputSchema:          json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Workspace-relative source file."},"line":{"type":["integer","string"],"minimum":1,"description":"1-based line containing the callable symbol."},"character":{"type":["integer","string","null"],"minimum":0,"description":"0-based character offset. Use when known; otherwise send symbol."},"symbol":{"type":["string","null"],"description":"Exact symbol text visible on the line."},"mode":{"type":"string","enum":["callers","callees","graph"],"description":"callers for incoming edges, callees for outgoing edges, graph for bounded multi-hop traversal."},"depth":{"type":["integer","string","null"],"minimum":1,"maximum":4,"description":"Graph traversal depth. Defaults to 2."},"max_nodes":{"type":["integer","string","null"],"minimum":1,"maximum":500,"description":"Maximum nodes before truncating. Defaults to 200."}},"required":["path","line","mode"],"anyOf":[{"required":["character"]},{"required":["symbol"]}],"additionalProperties":false}`),
		ArgumentExamples:     []string{`{"path":"internal/search/service.go","line":55,"symbol":"Search","mode":"callers","depth":2,"max_nodes":200}`, `{"path":"internal/search/service.go","line":55,"character":5,"mode":"callers","depth":2,"max_nodes":200}`},
		RequiresWorkspace:    true,
		ProviderRichGuidance: true,
	}
}

func (TraceTool) PathRequests(args json.RawMessage) ([]PathRequest, error) {
	input, err := parseTraceInput(args)
	if err != nil {
		return nil, err
	}
	return []PathRequest{{
		Access: workspace.AccessRead,
		Path:   input.Path,
		Reason: "inspect source for call hierarchy tracing",
	}}, nil
}

func (TraceTool) Execute(ctx context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	codeIntel, err := ectx.CodeIntel()
	if err != nil {
		return Result{}, err
	}
	input, err := parseTraceInput(args)
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
	input.Character = character
	input.HasCharacter = true
	result, err := codeIntel.Trace(ctx, CodeIntelTraceRequest{
		Path:      decision.ResolvedPath,
		Line:      input.Line,
		Character: character,
		Mode:      input.Mode,
		Depth:     input.Depth,
		MaxNodes:  input.MaxNodes,
	})
	if err != nil {
		if notice, ok := AsCodeIntelNotice(err); ok {
			result := CodeIntelTraceResult{Notice: notice}
			structured, marshalErr := MarshalStructuredResult(result)
			if marshalErr != nil {
				return Result{}, marshalErr
			}
			return Result{Output: formatTraceResult(input.asRequest(decision.ResolvedPath), result), StructuredResult: structured}, nil
		}
		return Result{}, err
	}
	structured, err := MarshalStructuredResult(result)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Output:           formatTraceResult(input.asRequest(decision.ResolvedPath), result),
		StructuredResult: structured,
	}, nil
}

func (i traceInput) asRequest(path string) CodeIntelTraceRequest {
	return CodeIntelTraceRequest{
		Path:      path,
		Line:      i.Line,
		Character: i.Character,
		Mode:      i.Mode,
		Depth:     i.Depth,
		MaxNodes:  i.MaxNodes,
	}
}
