package tool

import (
	"context"
	"encoding/json"
)

const SymbolsToolName = "symbols"

type SymbolsTool struct{}

func NewSymbolsTool() SymbolsTool {
	return SymbolsTool{}
}

func (SymbolsTool) Definition() Definition {
	description := "Search semantic workspace symbols by name. Prefer over text search when you know a declaration name but not its file. Use refs for usages and definition when the symbol is already visible."
	return Definition{
		Name:                 SymbolsToolName,
		Description:          description,
		ProviderDescription:  description,
		InputSchema:          json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Symbol name or partial symbol name."}},"required":["query"],"additionalProperties":false}`),
		ArgumentExamples:     []string{`{"query":"ProjectController"}`},
		RequiresWorkspace:    true,
		ProviderRichGuidance: true,
		ParallelSafe:         true,
	}
}

func (SymbolsTool) Execute(ctx context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	codeIntel, err := ectx.CodeIntel()
	if err != nil {
		return Result{}, err
	}
	input, err := parseSymbolsInput(args)
	if err != nil {
		return Result{}, err
	}
	symbols, err := codeIntel.Symbols(ctx, input.Query)
	if err != nil {
		return Result{}, err
	}
	return Result{Output: formatSymbols(input.Query, symbols)}, nil
}
