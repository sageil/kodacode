package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func definitionInspectorParams(call *events.ToolCallState) []inspectorParam {
	input, ok := parseDefinitionToolViewInput(call.Input)
	if !ok {
		return defaultInspectorParams(call)
	}
	params := []inspectorParam{
		{Label: "Path", Value: input.Path},
		{Label: "Line", Value: fmt.Sprintf("%d", input.Line)},
		{Label: "Character", Value: fmt.Sprintf("%d", input.Character)},
	}
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}

func diagnosticsInspectorParams(call *events.ToolCallState) []inspectorParam {
	input, ok := parseDiagnosticsToolViewInput(call.Input)
	if !ok {
		return defaultInspectorParams(call)
	}
	params := []inspectorParam{
		{Label: "Files", Value: strings.Join(input.Paths, ", ")},
	}
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}

func symbolsInspectorParams(call *events.ToolCallState) []inspectorParam {
	input, ok := parseSymbolsToolViewInput(call.Input)
	if !ok {
		return defaultInspectorParams(call)
	}
	params := []inspectorParam{
		{Label: "Query", Value: input.Query},
	}
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}

func traceInspectorParams(call *events.ToolCallState) []inspectorParam {
	input, ok := parseTraceToolViewInput(call.Input)
	if !ok {
		return defaultInspectorParams(call)
	}
	params := []inspectorParam{
		{Label: "Path", Value: input.Path},
		{Label: "Line", Value: fmt.Sprintf("%d", input.Line)},
		{Label: "Character", Value: fmt.Sprintf("%d", input.Character)},
		{Label: "Mode", Value: input.Mode},
	}
	if input.Depth > 0 {
		params = append(params, inspectorParam{Label: "Depth", Value: fmt.Sprintf("%d", input.Depth)})
	}
	if input.MaxNodes > 0 {
		params = append(params, inspectorParam{Label: "Max Nodes", Value: fmt.Sprintf("%d", input.MaxNodes)})
	}
	if result, ok := parseTraceStructuredToolResult(call); ok {
		params = append(params, inspectorParam{Label: "Status", Value: traceStructuredStatus(result)})
		if len(result.Nodes) > 0 {
			root := strings.TrimSpace(result.Nodes[0].Name)
			if root != "" {
				params = append(params, inspectorParam{Label: "Root", Value: root})
			}
		}
		params = append(params,
			inspectorParam{Label: "Nodes", Value: fmt.Sprintf("%d", len(result.Nodes))},
			inspectorParam{Label: "Edges", Value: fmt.Sprintf("%d", len(result.Edges))},
		)
		if result.Truncated {
			params = append(params, inspectorParam{Label: "Truncated", Value: onOffLabel(true)})
		}
	}
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}

func refsInspectorParams(call *events.ToolCallState) []inspectorParam {
	input, ok := parseRefsToolViewInput(call.Input)
	if !ok {
		return defaultInspectorParams(call)
	}
	params := []inspectorParam{
		{Label: "Path", Value: input.Path},
		{Label: "Line", Value: fmt.Sprintf("%d", input.Line)},
		{Label: "Character", Value: fmt.Sprintf("%d", input.Character)},
		{Label: "Mode", Value: input.Mode},
	}
	if input.MaxResults > 0 {
		params = append(params, inspectorParam{Label: "Max Results", Value: fmt.Sprintf("%d", input.MaxResults)})
	}
	params = append(params, inspectorParam{Label: "Include Decl", Value: onOffLabel(input.IncludeDeclaration)})
	if result, ok := parseRefsStructuredToolResult(call); ok {
		params = append(params, inspectorParam{Label: "Status", Value: refsStructuredStatus(result)})
		if target := strings.TrimSpace(result.Target.Name); target != "" {
			params = append(params, inspectorParam{Label: "Target", Value: target})
		}
		params = append(params, inspectorParam{Label: "References", Value: fmt.Sprintf("%d", len(result.References))})
		if kinds := refsStructuredKindsSummary(result.References); kinds != "" {
			params = append(params, inspectorParam{Label: "Kinds", Value: kinds})
		}
		params = append(params, inspectorParam{Label: "Classified", Value: onOffLabel(result.ClassificationSupported)})
		if result.ClassificationIncomplete {
			params = append(params, inspectorParam{Label: "Partial", Value: onOffLabel(true)})
		}
		if result.Truncated {
			params = append(params, inspectorParam{Label: "Truncated", Value: onOffLabel(true)})
		}
	}
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}

func renameSymbolInspectorParams(call *events.ToolCallState) []inspectorParam {
	input, ok := parseRenameSymbolToolViewInput(call.Input)
	if !ok {
		return defaultInspectorParams(call)
	}
	params := []inspectorParam{
		{Label: "Path", Value: input.Path},
		{Label: "Line", Value: fmt.Sprintf("%d", input.Line)},
		{Label: "Character", Value: fmt.Sprintf("%d", input.Character)},
		{Label: "New Name", Value: input.NewName},
	}
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}

func codeActionInspectorParams(call *events.ToolCallState) []inspectorParam {
	input, ok := parseCodeActionToolViewInput(call.Input)
	if !ok {
		return defaultInspectorParams(call)
	}
	params := []inspectorParam{
		{Label: "Path", Value: input.Path},
		{Label: "Range", Value: fmt.Sprintf("%d:%d-%d:%d", input.StartLine, input.StartCharacter, input.EndLine, input.EndCharacter)},
	}
	if input.Title != nil && strings.TrimSpace(*input.Title) != "" {
		params = append(params, inspectorParam{Label: "Title", Value: strings.TrimSpace(*input.Title)})
	}
	if input.Kind != nil && strings.TrimSpace(*input.Kind) != "" {
		params = append(params, inspectorParam{Label: "Kind", Value: strings.TrimSpace(*input.Kind)})
	}
	params = append(params, inspectorParam{Label: "Preferred", Value: onOffLabel(input.OnlyPreferred != nil && *input.OnlyPreferred)})
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}

func parseDefinitionToolViewInput(raw string) (struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Character int    `json:"character"`
}, bool) {
	var wire struct {
		Path      string          `json:"path"`
		Line      json.RawMessage `json:"line"`
		Character json.RawMessage `json:"character"`
	}
	if json.Unmarshal([]byte(raw), &wire) != nil {
		return struct {
			Path      string `json:"path"`
			Line      int    `json:"line"`
			Character int    `json:"character"`
		}{}, false
	}
	line, hasLine := parseToolViewOptionalInt(wire.Line)
	character, hasCharacter := parseToolViewOptionalInt(wire.Character)
	if strings.TrimSpace(wire.Path) == "" || !hasLine || !hasCharacter || line < 1 || character < 0 {
		return struct {
			Path      string `json:"path"`
			Line      int    `json:"line"`
			Character int    `json:"character"`
		}{}, false
	}
	return struct {
		Path      string `json:"path"`
		Line      int    `json:"line"`
		Character int    `json:"character"`
	}{
		Path:      strings.TrimSpace(wire.Path),
		Line:      line,
		Character: character,
	}, true
}

func parseDiagnosticsToolViewInput(raw string) (struct {
	Paths []string `json:"paths"`
}, bool) {
	var input struct {
		Paths []string `json:"paths"`
	}
	if json.Unmarshal([]byte(raw), &input) != nil || len(input.Paths) == 0 {
		return input, false
	}
	return input, true
}

func parseSymbolsToolViewInput(raw string) (struct {
	Query string `json:"query"`
}, bool) {
	var input struct {
		Query string `json:"query"`
	}
	if json.Unmarshal([]byte(raw), &input) != nil || strings.TrimSpace(input.Query) == "" {
		return input, false
	}
	return input, true
}

func parseTraceToolViewInput(raw string) (struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Character int    `json:"character"`
	Mode      string `json:"mode"`
	Depth     int    `json:"depth"`
	MaxNodes  int    `json:"max_nodes"`
}, bool) {
	var wire struct {
		Path      string          `json:"path"`
		Line      json.RawMessage `json:"line"`
		Character json.RawMessage `json:"character"`
		Mode      string          `json:"mode"`
		Depth     json.RawMessage `json:"depth"`
		MaxNodes  json.RawMessage `json:"max_nodes"`
	}
	if json.Unmarshal([]byte(raw), &wire) != nil {
		return struct {
			Path      string `json:"path"`
			Line      int    `json:"line"`
			Character int    `json:"character"`
			Mode      string `json:"mode"`
			Depth     int    `json:"depth"`
			MaxNodes  int    `json:"max_nodes"`
		}{}, false
	}
	line, hasLine := parseToolViewOptionalInt(wire.Line)
	character, hasCharacter := parseToolViewOptionalInt(wire.Character)
	depth, hasDepth := parseToolViewOptionalInt(wire.Depth)
	maxNodes, hasMaxNodes := parseToolViewOptionalInt(wire.MaxNodes)
	if strings.TrimSpace(wire.Path) == "" || !hasLine || !hasCharacter || line < 1 || character < 0 || strings.TrimSpace(wire.Mode) == "" {
		return struct {
			Path      string `json:"path"`
			Line      int    `json:"line"`
			Character int    `json:"character"`
			Mode      string `json:"mode"`
			Depth     int    `json:"depth"`
			MaxNodes  int    `json:"max_nodes"`
		}{}, false
	}
	if !hasDepth {
		depth = 0
	}
	if !hasMaxNodes {
		maxNodes = 0
	}
	return struct {
		Path      string `json:"path"`
		Line      int    `json:"line"`
		Character int    `json:"character"`
		Mode      string `json:"mode"`
		Depth     int    `json:"depth"`
		MaxNodes  int    `json:"max_nodes"`
	}{
		Path:      strings.TrimSpace(wire.Path),
		Line:      line,
		Character: character,
		Mode:      strings.TrimSpace(wire.Mode),
		Depth:     depth,
		MaxNodes:  maxNodes,
	}, true
}

func parseRefsToolViewInput(raw string) (struct {
	Path               string `json:"path"`
	Line               int    `json:"line"`
	Character          int    `json:"character"`
	Mode               string `json:"mode"`
	MaxResults         int    `json:"max_results"`
	IncludeDeclaration bool   `json:"include_declaration"`
}, bool) {
	var wire struct {
		Path               string          `json:"path"`
		Line               json.RawMessage `json:"line"`
		Character          json.RawMessage `json:"character"`
		Mode               *string         `json:"mode"`
		MaxResults         json.RawMessage `json:"max_results"`
		IncludeDeclaration json.RawMessage `json:"include_declaration"`
	}
	if json.Unmarshal([]byte(raw), &wire) != nil {
		return struct {
			Path               string `json:"path"`
			Line               int    `json:"line"`
			Character          int    `json:"character"`
			Mode               string `json:"mode"`
			MaxResults         int    `json:"max_results"`
			IncludeDeclaration bool   `json:"include_declaration"`
		}{}, false
	}
	line, hasLine := parseToolViewOptionalInt(wire.Line)
	character, hasCharacter := parseToolViewOptionalInt(wire.Character)
	maxResults, hasMaxResults := parseToolViewOptionalInt(wire.MaxResults)
	includeDeclaration, hasIncludeDeclaration := parseToolViewOptionalBool(wire.IncludeDeclaration)
	if strings.TrimSpace(wire.Path) == "" || !hasLine || !hasCharacter || line < 1 || character < 0 {
		return struct {
			Path               string `json:"path"`
			Line               int    `json:"line"`
			Character          int    `json:"character"`
			Mode               string `json:"mode"`
			MaxResults         int    `json:"max_results"`
			IncludeDeclaration bool   `json:"include_declaration"`
		}{}, false
	}
	mode := "all"
	if wire.Mode != nil && strings.TrimSpace(*wire.Mode) != "" {
		mode = strings.TrimSpace(*wire.Mode)
	}
	if !hasMaxResults {
		maxResults = 0
	}
	if !hasIncludeDeclaration {
		includeDeclaration = false
	}
	return struct {
		Path               string `json:"path"`
		Line               int    `json:"line"`
		Character          int    `json:"character"`
		Mode               string `json:"mode"`
		MaxResults         int    `json:"max_results"`
		IncludeDeclaration bool   `json:"include_declaration"`
	}{
		Path:               strings.TrimSpace(wire.Path),
		Line:               line,
		Character:          character,
		Mode:               mode,
		MaxResults:         maxResults,
		IncludeDeclaration: includeDeclaration,
	}, true
}

func parseRenameSymbolToolViewInput(raw string) (struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Character int    `json:"character"`
	NewName   string `json:"new_name"`
}, bool) {
	var wire struct {
		Path      string          `json:"path"`
		Line      json.RawMessage `json:"line"`
		Character json.RawMessage `json:"character"`
		NewName   string          `json:"new_name"`
	}
	if json.Unmarshal([]byte(raw), &wire) != nil {
		return struct {
			Path      string `json:"path"`
			Line      int    `json:"line"`
			Character int    `json:"character"`
			NewName   string `json:"new_name"`
		}{}, false
	}
	line, hasLine := parseToolViewOptionalInt(wire.Line)
	character, hasCharacter := parseToolViewOptionalInt(wire.Character)
	if strings.TrimSpace(wire.Path) == "" || !hasLine || !hasCharacter || line < 1 || character < 0 || strings.TrimSpace(wire.NewName) == "" {
		return struct {
			Path      string `json:"path"`
			Line      int    `json:"line"`
			Character int    `json:"character"`
			NewName   string `json:"new_name"`
		}{}, false
	}
	return struct {
		Path      string `json:"path"`
		Line      int    `json:"line"`
		Character int    `json:"character"`
		NewName   string `json:"new_name"`
	}{
		Path:      strings.TrimSpace(wire.Path),
		Line:      line,
		Character: character,
		NewName:   strings.TrimSpace(wire.NewName),
	}, true
}

func parseCodeActionToolViewInput(raw string) (struct {
	Path           string  `json:"path"`
	StartLine      int     `json:"start_line"`
	StartCharacter int     `json:"start_character"`
	EndLine        int     `json:"end_line"`
	EndCharacter   int     `json:"end_character"`
	Title          *string `json:"title"`
	Kind           *string `json:"kind"`
	OnlyPreferred  *bool   `json:"only_preferred"`
}, bool) {
	var wire struct {
		Path           string          `json:"path"`
		StartLine      json.RawMessage `json:"start_line"`
		StartCharacter json.RawMessage `json:"start_character"`
		EndLine        json.RawMessage `json:"end_line"`
		EndCharacter   json.RawMessage `json:"end_character"`
		Title          *string         `json:"title"`
		Kind           *string         `json:"kind"`
		OnlyPreferred  json.RawMessage `json:"only_preferred"`
	}
	if json.Unmarshal([]byte(raw), &wire) != nil {
		return struct {
			Path           string  `json:"path"`
			StartLine      int     `json:"start_line"`
			StartCharacter int     `json:"start_character"`
			EndLine        int     `json:"end_line"`
			EndCharacter   int     `json:"end_character"`
			Title          *string `json:"title"`
			Kind           *string `json:"kind"`
			OnlyPreferred  *bool   `json:"only_preferred"`
		}{}, false
	}
	startLine, hasStartLine := parseToolViewOptionalInt(wire.StartLine)
	startCharacter, hasStartCharacter := parseToolViewOptionalInt(wire.StartCharacter)
	endLine, hasEndLine := parseToolViewOptionalInt(wire.EndLine)
	endCharacter, hasEndCharacter := parseToolViewOptionalInt(wire.EndCharacter)
	if strings.TrimSpace(wire.Path) == "" || !hasStartLine || !hasStartCharacter || !hasEndLine || !hasEndCharacter || startLine < 1 || endLine < 1 || startCharacter < 0 || endCharacter < 0 {
		return struct {
			Path           string  `json:"path"`
			StartLine      int     `json:"start_line"`
			StartCharacter int     `json:"start_character"`
			EndLine        int     `json:"end_line"`
			EndCharacter   int     `json:"end_character"`
			Title          *string `json:"title"`
			Kind           *string `json:"kind"`
			OnlyPreferred  *bool   `json:"only_preferred"`
		}{}, false
	}
	if endLine < startLine || (endLine == startLine && endCharacter < startCharacter) {
		return struct {
			Path           string  `json:"path"`
			StartLine      int     `json:"start_line"`
			StartCharacter int     `json:"start_character"`
			EndLine        int     `json:"end_line"`
			EndCharacter   int     `json:"end_character"`
			Title          *string `json:"title"`
			Kind           *string `json:"kind"`
			OnlyPreferred  *bool   `json:"only_preferred"`
		}{}, false
	}
	onlyPreferredValue, hasOnlyPreferred := parseToolViewOptionalBool(wire.OnlyPreferred)
	var onlyPreferred *bool
	if hasOnlyPreferred {
		value := onlyPreferredValue
		onlyPreferred = &value
	}
	return struct {
		Path           string  `json:"path"`
		StartLine      int     `json:"start_line"`
		StartCharacter int     `json:"start_character"`
		EndLine        int     `json:"end_line"`
		EndCharacter   int     `json:"end_character"`
		Title          *string `json:"title"`
		Kind           *string `json:"kind"`
		OnlyPreferred  *bool   `json:"only_preferred"`
	}{
		Path:           strings.TrimSpace(wire.Path),
		StartLine:      startLine,
		StartCharacter: startCharacter,
		EndLine:        endLine,
		EndCharacter:   endCharacter,
		Title:          wire.Title,
		Kind:           wire.Kind,
		OnlyPreferred:  onlyPreferred,
	}, true
}
