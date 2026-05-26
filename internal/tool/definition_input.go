package tool

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrDefinitionPathRequired      = errors.New("path is required")
	ErrDefinitionLineRequired      = errors.New("line must be >= 1")
	ErrDefinitionCharacterInvalid  = errors.New("provide character >= 0 or symbol")
	ErrSymbolsQueryRequired        = errors.New("query is required")
	ErrTraceModeRequired           = errors.New("mode must be callers, callees, or graph")
	ErrTraceDepthInvalid           = errors.New("depth must be between 1 and 4")
	ErrTraceMaxNodesInvalid        = errors.New("max_nodes must be between 1 and 500")
	ErrRefsModeInvalid             = errors.New("mode must be all, readers, or writers")
	ErrRefsMaxResultsInvalid       = errors.New("max_results must be between 1 and 500")
	ErrDiagnosticsPathsRequired    = errors.New("paths must contain at least one file")
	ErrDiagnosticsPathBlank        = errors.New("paths must not contain blank entries")
	ErrDiagnosticsPathLimitInvalid = errors.New("paths must contain at most 32 files")
)

type definitionInput struct {
	Path         string
	Line         int
	Character    int
	HasCharacter bool
	Symbol       string
}

func parseDefinitionInput(args json.RawMessage) (_ definitionInput, err error) {
	defer func() {
		err = normalizeToolInputError(DefinitionToolName, err)
	}()
	var raw struct {
		Path      string          `json:"path"`
		Line      json.RawMessage `json:"line"`
		Character json.RawMessage `json:"character"`
		Symbol    string          `json:"symbol"`
	}
	if err := DecodeArgs(DefinitionToolName, args, &raw); err != nil {
		return definitionInput{}, err
	}
	line, _, err := decodeOptionalIntArg(DefinitionToolName, raw.Line, "line")
	if err != nil {
		return definitionInput{}, err
	}
	character, hasCharacter, err := decodeOptionalIntArg(DefinitionToolName, raw.Character, "character")
	if err != nil {
		return definitionInput{}, err
	}
	if strings.TrimSpace(raw.Path) == "" {
		return definitionInput{}, ErrDefinitionPathRequired
	}
	if line < 1 {
		return definitionInput{}, ErrDefinitionLineRequired
	}
	if hasCharacter && character < 0 {
		return definitionInput{}, ErrDefinitionCharacterInvalid
	}
	if !hasCharacter && strings.TrimSpace(raw.Symbol) == "" {
		return definitionInput{}, ErrDefinitionCharacterInvalid
	}
	return definitionInput{
		Path:         strings.TrimSpace(raw.Path),
		Line:         line,
		Character:    character,
		HasCharacter: hasCharacter,
		Symbol:       strings.TrimSpace(raw.Symbol),
	}, nil
}

type diagnosticsInput struct {
	Paths []string
}

func parseDiagnosticsInput(args json.RawMessage) (_ diagnosticsInput, err error) {
	defer func() {
		err = normalizeToolInputError(DiagnosticsToolName, err)
	}()
	var raw struct {
		Path  json.RawMessage `json:"path"`
		Paths json.RawMessage `json:"paths"`
	}
	if err := DecodeArgs(DiagnosticsToolName, args, &raw); err != nil {
		return diagnosticsInput{}, err
	}
	paths, ok, err := decodeOptionalStringArrayArg(DiagnosticsToolName, raw.Paths, "paths")
	if err != nil {
		return diagnosticsInput{}, err
	}
	if !ok {
		path, err := decodeOptionalStringArg(DiagnosticsToolName, raw.Path, "path")
		if err != nil {
			return diagnosticsInput{}, err
		}
		if strings.TrimSpace(path) != "" {
			paths = []string{path}
			ok = true
		}
	}
	if !ok || len(paths) == 0 {
		return diagnosticsInput{}, ErrDiagnosticsPathsRequired
	}
	if len(paths) > 32 {
		return diagnosticsInput{}, ErrDiagnosticsPathLimitInvalid
	}
	normalized := make([]string, 0, len(paths))
	for _, value := range paths {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return diagnosticsInput{}, ErrDiagnosticsPathBlank
		}
		normalized = append(normalized, trimmed)
	}
	return diagnosticsInput{Paths: normalized}, nil
}

type symbolsInput struct {
	Query string
}

func parseSymbolsInput(args json.RawMessage) (_ symbolsInput, err error) {
	defer func() {
		err = normalizeToolInputError(SymbolsToolName, err)
	}()
	var raw struct {
		Query string `json:"query"`
	}
	if err := DecodeArgs(SymbolsToolName, args, &raw); err != nil {
		return symbolsInput{}, err
	}
	if strings.TrimSpace(raw.Query) == "" {
		return symbolsInput{}, ErrSymbolsQueryRequired
	}
	return symbolsInput{Query: strings.TrimSpace(raw.Query)}, nil
}

type traceInput struct {
	Path         string
	Line         int
	Character    int
	HasCharacter bool
	Symbol       string
	Mode         CodeIntelTraceMode
	Depth        int
	MaxNodes     int
}

func parseTraceInput(args json.RawMessage) (_ traceInput, err error) {
	defer func() {
		err = normalizeToolInputError(TraceToolName, err)
	}()
	var raw struct {
		Path      string          `json:"path"`
		Line      json.RawMessage `json:"line"`
		Character json.RawMessage `json:"character"`
		Symbol    string          `json:"symbol"`
		Mode      string          `json:"mode"`
		Depth     json.RawMessage `json:"depth"`
		MaxNodes  json.RawMessage `json:"max_nodes"`
	}
	if err := DecodeArgs(TraceToolName, args, &raw); err != nil {
		return traceInput{}, err
	}
	line, _, err := decodeOptionalIntArg(TraceToolName, raw.Line, "line")
	if err != nil {
		return traceInput{}, err
	}
	character, hasCharacter, err := decodeOptionalIntArg(TraceToolName, raw.Character, "character")
	if err != nil {
		return traceInput{}, err
	}
	depth, hasDepth, err := decodeOptionalIntArg(TraceToolName, raw.Depth, "depth")
	if err != nil {
		return traceInput{}, err
	}
	maxNodes, hasMaxNodes, err := decodeOptionalIntArg(TraceToolName, raw.MaxNodes, "max_nodes")
	if err != nil {
		return traceInput{}, err
	}
	if !hasDepth {
		depth = 2
	}
	if !hasMaxNodes {
		maxNodes = 200
	}
	if strings.TrimSpace(raw.Path) == "" {
		return traceInput{}, ErrDefinitionPathRequired
	}
	if line < 1 {
		return traceInput{}, ErrDefinitionLineRequired
	}
	if hasCharacter && character < 0 {
		return traceInput{}, ErrDefinitionCharacterInvalid
	}
	if !hasCharacter && strings.TrimSpace(raw.Symbol) == "" {
		return traceInput{}, ErrDefinitionCharacterInvalid
	}
	mode := CodeIntelTraceMode(strings.ToLower(strings.TrimSpace(raw.Mode)))
	switch mode {
	case CodeIntelTraceModeCallers, CodeIntelTraceModeCallees, CodeIntelTraceModeGraph:
	default:
		return traceInput{}, ErrTraceModeRequired
	}
	if depth < 1 || depth > 4 {
		return traceInput{}, ErrTraceDepthInvalid
	}
	if maxNodes < 1 || maxNodes > 500 {
		return traceInput{}, ErrTraceMaxNodesInvalid
	}
	return traceInput{
		Path:         strings.TrimSpace(raw.Path),
		Line:         line,
		Character:    character,
		HasCharacter: hasCharacter,
		Symbol:       strings.TrimSpace(raw.Symbol),
		Mode:         mode,
		Depth:        depth,
		MaxNodes:     maxNodes,
	}, nil
}

type refsInput struct {
	Path               string
	Line               int
	Character          int
	HasCharacter       bool
	Symbol             string
	Mode               CodeIntelRefsMode
	MaxResults         int
	IncludeDeclaration bool
}

func parseRefsInput(args json.RawMessage) (_ refsInput, err error) {
	defer func() {
		err = normalizeToolInputError(RefsToolName, err)
	}()
	var raw struct {
		Path               string          `json:"path"`
		Line               json.RawMessage `json:"line"`
		Character          json.RawMessage `json:"character"`
		Symbol             string          `json:"symbol"`
		Mode               *string         `json:"mode"`
		MaxResults         json.RawMessage `json:"max_results"`
		IncludeDeclaration json.RawMessage `json:"include_declaration"`
	}
	if err := DecodeArgs(RefsToolName, args, &raw); err != nil {
		return refsInput{}, err
	}
	line, _, err := decodeOptionalIntArg(RefsToolName, raw.Line, "line")
	if err != nil {
		return refsInput{}, err
	}
	character, hasCharacter, err := decodeOptionalIntArg(RefsToolName, raw.Character, "character")
	if err != nil {
		return refsInput{}, err
	}
	maxResults, hasMaxResults, err := decodeOptionalIntArg(RefsToolName, raw.MaxResults, "max_results")
	if err != nil {
		return refsInput{}, err
	}
	includeDeclaration, hasIncludeDeclaration, err := decodeOptionalBoolArg(RefsToolName, raw.IncludeDeclaration, "include_declaration")
	if err != nil {
		return refsInput{}, err
	}
	if !hasMaxResults {
		maxResults = 200
	}
	if !hasIncludeDeclaration {
		includeDeclaration = false
	}
	if strings.TrimSpace(raw.Path) == "" {
		return refsInput{}, ErrDefinitionPathRequired
	}
	if line < 1 {
		return refsInput{}, ErrDefinitionLineRequired
	}
	if hasCharacter && character < 0 {
		return refsInput{}, ErrDefinitionCharacterInvalid
	}
	if !hasCharacter && strings.TrimSpace(raw.Symbol) == "" {
		return refsInput{}, ErrDefinitionCharacterInvalid
	}
	mode := CodeIntelRefsModeAll
	if raw.Mode != nil {
		switch CodeIntelRefsMode(strings.ToLower(strings.TrimSpace(*raw.Mode))) {
		case "", CodeIntelRefsModeAll:
			mode = CodeIntelRefsModeAll
		case CodeIntelRefsModeReaders:
			mode = CodeIntelRefsModeReaders
		case CodeIntelRefsModeWriters:
			mode = CodeIntelRefsModeWriters
		default:
			return refsInput{}, ErrRefsModeInvalid
		}
	}
	if maxResults < 1 || maxResults > 500 {
		return refsInput{}, ErrRefsMaxResultsInvalid
	}
	return refsInput{
		Path:               strings.TrimSpace(raw.Path),
		Line:               line,
		Character:          character,
		HasCharacter:       hasCharacter,
		Symbol:             strings.TrimSpace(raw.Symbol),
		Mode:               mode,
		MaxResults:         maxResults,
		IncludeDeclaration: includeDeclaration,
	}, nil
}

func definitionTitle(path string, line, character int) string {
	return fmt.Sprintf("%s:%d:%d", path, line, character)
}
