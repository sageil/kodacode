package tool

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/sageil/kodacode/internal/workspace"
)

func (ReadTool) Execute(_ context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	input, err := parseReadInput(args)
	if err != nil {
		return Result{}, err
	}

	startLine := input.Offset + 1
	limit := effectiveReadLimit(input)

	results := make([]readResult, 0, len(input.Paths))
	failures := make([]readFailure, 0, len(input.Paths))

	for _, path := range input.Paths {
		result, failure := executeSingleReadPath(ectx, path, startLine, limit)
		if failure != nil {
			failures = append(failures, *failure)
		} else {
			results = append(results, result)
		}
	}

	output := formatReadOutput(results, len(input.Paths) > 1)
	errorText := ""
	if len(results) == 0 {
		errorText = formatReadFailures(failures)
	} else if len(failures) > 0 {
		output = formatPartialReadOutput(output, len(results), len(input.Paths), failures)
	}
	if output == "" && errorText == "" {
		return Result{}, nil
	}
	observed := make([]ObservedResource, 0, len(results))
	for _, result := range results {
		if strings.TrimSpace(result.resolved) == "" || strings.TrimSpace(result.version) == "" {
			continue
		}
		observed = append(observed, ObservedResource{
			Kind:       ObservedResourceFileContent,
			Path:       result.resolved,
			Version:    result.version,
			State:      result.state,
			Complete:   result.complete,
			StartLine:  result.startLine,
			EndLine:    result.endLine,
			TotalLines: result.totalLines,
		})
	}
	return Result{
		Output:            output,
		Error:             errorText,
		ObservedResources: observed,
	}, nil
}

func executeSingleReadPath(ectx ExecutionContext, inputPath string, startLine, limit int) (readResult, *readFailure) {
	decision, err := ectx.ResolvePath(workspace.AccessRead, inputPath)
	if err != nil {
		return readResult{}, &readFailure{path: inputPath, error: err.Error()}
	}
	result, err := renderReadPathWithTransientRetry(decision.ResolvedPath, inputPath, startLine, limit)
	if err != nil {
		return readResult{}, &readFailure{path: inputPath, error: err.Error()}
	}
	result.resolved = decision.ResolvedPath
	return result, nil
}

func effectiveReadLimit(input readInput) int {
	if input.HasLimit {
		return input.Limit
	}
	return DefaultReadLimit
}
