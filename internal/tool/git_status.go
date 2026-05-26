package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrGitStatusNoArguments = errors.New("git_status takes no fields")

type GitStatusTool struct{}

func NewGitStatusTool() GitStatusTool {
	return GitStatusTool{}
}

func (GitStatusTool) Definition() Definition {
	return Definition{
		Name:                "git_status",
		Description:         "Show workspace-scoped Git status for the current working directory. Use this to understand tracked, modified, staged, and untracked files before reviewing diffs.",
		ProviderDescription: "Show workspace Git status.",
		InputSchema:         json.RawMessage(`{"type":"object","properties":{},"required":[],"additionalProperties":false}`),
		ArgumentExamples:    []string{`{}`},
		RequiresWorkspace:   true,
		ParallelSafe:        true,
	}
}

func (GitStatusTool) Execute(ctx context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	if ectx.Workspace == nil {
		return Result{}, ErrWorkspaceRequired
	}
	if err := parseGitStatusInput(args); err != nil {
		return Result{}, err
	}

	command := []string{"git", "status", "--porcelain=v1", "--branch", "--untracked-files=all", "--", "."}
	runResult, output, err := runGitReviewCommand(ctx, ectx.Workspace.Root(), []string{"status", "--porcelain=v1", "--branch", "--untracked-files=all", "--", "."})
	if err != nil {
		return Result{
			Error:     formatGitReviewError(command, err),
			Execution: gitReviewExecutionRuntime(runResult),
		}, nil
	}
	return Result{
		Output:    formatGitStatusResult(command, output, runResult.Truncated),
		Execution: gitReviewExecutionRuntime(runResult),
	}, nil
}

func parseGitStatusInput(args json.RawMessage) (err error) {
	defer func() {
		err = normalizeToolInputError("git_status", err)
	}()
	if strings.TrimSpace(string(args)) == "" || strings.TrimSpace(string(args)) == "null" {
		return nil
	}
	var input map[string]any
	if err := DecodeArgs("git_status", args, &input); err != nil {
		return err
	}
	if len(input) != 0 {
		field := firstUnexpectedJSONField(input)
		if field == "" {
			return InvalidArguments("git_status", ErrGitStatusNoArguments)
		}
		return InvalidArguments("git_status", fmt.Errorf("git_status takes no fields like %q", field))
	}
	return nil
}

func firstUnexpectedJSONField(input map[string]any) string {
	if len(input) == 0 {
		return ""
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		keys = append(keys, trimmed)
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Strings(keys)
	return keys[0]
}
