package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/sageil/kodacode/internal/workspace"
)

const DiagnosticsToolName = "diagnostics"

type DiagnosticsTool struct{}

func NewDiagnosticsTool() DiagnosticsTool {
	return DiagnosticsTool{}
}

func (DiagnosticsTool) Definition() Definition {
	description := "Get language-server diagnostics for existing concrete source files. Pass files in one `paths` array; do not pass directories. Use after edits before slower lint, typecheck, or test commands when LSP diagnostics are available."
	return Definition{
		Name:                 DiagnosticsToolName,
		Description:          description,
		ProviderDescription:  description,
		InputSchema:          json.RawMessage(`{"type":"object","properties":{"path":{"type":["string","null"],"description":"Single source file path alias for paths."},"paths":{"type":["array","string","null"],"minItems":1,"maxItems":32,"items":{"type":"string"},"description":"Existing workspace-relative source file paths. Pass files, not directories."}},"anyOf":[{"required":["paths"]},{"required":["path"]}],"additionalProperties":false}`),
		ArgumentExamples:     []string{`{"paths":["src/app.ts"]}`},
		RequiresWorkspace:    true,
		ProviderRichGuidance: true,
		ParallelSafe:         true,
	}
}

func (DiagnosticsTool) PathRequests(args json.RawMessage) ([]PathRequest, error) {
	input, err := parseDiagnosticsInput(args)
	if err != nil {
		return nil, err
	}
	requests := make([]PathRequest, 0, len(input.Paths))
	for _, item := range input.Paths {
		requests = append(requests, PathRequest{
			Access: workspace.AccessRead,
			Path:   item,
			Reason: "inspect source for diagnostics",
		})
	}
	return requests, nil
}

func (DiagnosticsTool) Execute(ctx context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	codeIntel, err := ectx.CodeIntel()
	if err != nil {
		return Result{}, err
	}
	input, err := parseDiagnosticsInput(args)
	if err != nil {
		return Result{}, err
	}

	paths := make([]string, 0, len(input.Paths))
	for _, item := range input.Paths {
		decision, err := ectx.ResolvePath(workspace.AccessRead, item)
		if err != nil {
			return Result{}, err
		}
		if err := validateDiagnosticsTarget(decision); err != nil {
			return Result{}, err
		}
		paths = append(paths, decision.ResolvedPath)
	}
	diagnostics, err := codeIntel.Diagnostics(ctx, paths)
	if err != nil {
		return Result{}, err
	}
	return Result{Output: formatDiagnostics(diagnostics)}, nil
}

func validateDiagnosticsTarget(decision workspace.Decision) error {
	info, err := os.Stat(decision.ResolvedPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return InvalidArguments(DiagnosticsToolName, fmt.Errorf("path %q does not exist; pass a concrete source file path", decision.InputPath))
		}
		return fmt.Errorf("stat diagnostics target %q: %w", decision.InputPath, err)
	}
	if info.IsDir() {
		return InvalidArguments(DiagnosticsToolName, fmt.Errorf("path %q is a directory; pass concrete source file paths, not directories", decision.InputPath))
	}
	return nil
}
