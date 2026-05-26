package tool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/sageil/kodacode/internal/workspace"
)

const RenameSymbolToolName = "rename_symbol"

var (
	ErrRenameSymbolPathRequired     = errors.New("path is required")
	ErrRenameSymbolLineInvalid      = errors.New("line must be at least 1")
	ErrRenameSymbolCharacterInvalid = errors.New("character must be at least 0")
	ErrRenameSymbolNewNameRequired  = errors.New("new_name is required")
)

type RenameSymbolTool struct{}

func NewRenameSymbolTool() RenameSymbolTool {
	return RenameSymbolTool{}
}

func (RenameSymbolTool) Definition() Definition {
	description := "Rename a symbol using language-server refactoring support. Prefer over manual text replacement for semantic renames so references update when supported. Send `symbol` instead of calculating `character` when visible."
	return Definition{
		Name:                 RenameSymbolToolName,
		Description:          description,
		ProviderDescription:  description,
		InputSchema:          json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Workspace-relative source file containing the symbol."},"line":{"type":["integer","string"],"minimum":1,"description":"1-based line containing the symbol."},"character":{"type":["integer","string","null"],"minimum":0,"description":"0-based character offset. Use when known; otherwise send symbol."},"symbol":{"type":["string","null"],"description":"Exact symbol text visible on the line."},"new_name":{"type":"string","description":"New symbol name."}},"required":["path","line","new_name"],"anyOf":[{"required":["character"]},{"required":["symbol"]}],"additionalProperties":false}`),
		ArgumentExamples:     []string{`{"path":"src/app.ts","line":12,"symbol":"buildCacheKey","new_name":"buildProjectCacheKey"}`, `{"path":"src/app.ts","line":12,"character":4,"new_name":"buildCacheKey"}`},
		RequiresWorkspace:    true,
		ProviderRichGuidance: true,
	}
}

func (RenameSymbolTool) PathRequests(args json.RawMessage) ([]PathRequest, error) {
	input, err := parseRenameSymbolInput(args)
	if err != nil {
		return nil, err
	}
	return []PathRequest{{
		Access: workspace.AccessWrite,
		Path:   input.Path,
		Reason: "rename symbol and apply workspace edits",
	}}, nil
}

func (RenameSymbolTool) Execute(ctx context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	input, err := parseRenameSymbolInput(args)
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
	character, err := resolveCodeIntelCharacter(decision.ResolvedPath, input.Line, input.Character, input.HasCharacter, input.Symbol)
	if err != nil {
		return Result{}, err
	}
	summary, err := codeIntel.RenameSymbol(ctx, CodeIntelRenameRequest{
		Path:      decision.ResolvedPath,
		Line:      input.Line,
		Character: character,
		NewName:   input.NewName,
	})
	if err != nil {
		if notice, ok := AsCodeIntelNotice(err); ok {
			return Result{Output: formatCodeIntelNotice(RenameSymbolToolName, notice)}, nil
		}
		return Result{}, err
	}
	if len(summary.Paths) == 0 {
		return Result{Output: "No rename edits were produced."}, nil
	}
	return Result{Output: formatCodeIntelMutationSummary(`Renamed symbol to "`+input.NewName+`"`, summary)}, nil
}

type renameSymbolInput struct {
	Path         string
	Line         int
	Character    int
	HasCharacter bool
	Symbol       string
	NewName      string
}

func parseRenameSymbolInput(args json.RawMessage) (_ renameSymbolInput, err error) {
	defer func() {
		err = normalizeToolInputError(RenameSymbolToolName, err)
	}()
	var raw struct {
		Path      string          `json:"path"`
		Line      json.RawMessage `json:"line"`
		Character json.RawMessage `json:"character"`
		Symbol    string          `json:"symbol"`
		NewName   *string         `json:"new_name"`
	}
	if err := DecodeArgs(RenameSymbolToolName, args, &raw); err != nil {
		return renameSymbolInput{}, err
	}
	line, _, err := decodeOptionalIntArg(RenameSymbolToolName, raw.Line, "line")
	if err != nil {
		return renameSymbolInput{}, err
	}
	character, hasCharacter, err := decodeOptionalIntArg(RenameSymbolToolName, raw.Character, "character")
	if err != nil {
		return renameSymbolInput{}, err
	}
	if strings.TrimSpace(raw.Path) == "" {
		return renameSymbolInput{}, ErrRenameSymbolPathRequired
	}
	if line < 1 {
		return renameSymbolInput{}, ErrRenameSymbolLineInvalid
	}
	if hasCharacter && character < 0 {
		return renameSymbolInput{}, ErrRenameSymbolCharacterInvalid
	}
	if !hasCharacter && strings.TrimSpace(raw.Symbol) == "" {
		return renameSymbolInput{}, ErrRenameSymbolCharacterInvalid
	}
	if raw.NewName == nil || strings.TrimSpace(*raw.NewName) == "" {
		return renameSymbolInput{}, ErrRenameSymbolNewNameRequired
	}
	return renameSymbolInput{
		Path:         strings.TrimSpace(raw.Path),
		Line:         line,
		Character:    character,
		HasCharacter: hasCharacter,
		Symbol:       strings.TrimSpace(raw.Symbol),
		NewName:      strings.TrimSpace(*raw.NewName),
	}, nil
}
