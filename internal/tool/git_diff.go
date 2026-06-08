package tool

import (
	"context"
	"encoding/json"
	"errors"
)

var ErrGitDiffStagedRequired = errors.New("staged is required")

type GitDiffTool struct{}

type gitDiffInput struct {
	Staged bool `json:"staged"`
}

func NewGitDiffTool() GitDiffTool {
	return GitDiffTool{}
}

func (GitDiffTool) Definition() Definition {
	return Definition{
		Name:                "git_diff",
		Description:         "Show a capped workspace-scoped Git patch diff for the current working directory. Prefer git_status and targeted file reads for large changes. Set staged=true to review staged changes, or false to review unstaged working tree changes.",
		ProviderDescription: "Show a capped workspace Git patch diff. Prefer `git_status` and targeted file reads for large changes. Set `staged=true` for staged changes.",
		InputSchema:         json.RawMessage(`{"type":"object","properties":{"staged":{"type":["boolean","string"],"description":"Set true to diff staged changes. Set false to diff unstaged working tree changes."}},"required":["staged"],"additionalProperties":false}`),
		ArgumentExamples:    []string{`{"staged":false}`},
		RequiresWorkspace:   true,
		ParallelSafe:        true,
	}
}

func (GitDiffTool) Execute(ctx context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	if ectx.Workspace == nil {
		return Result{}, ErrWorkspaceRequired
	}
	input, err := parseGitDiffInput(args)
	if err != nil {
		return Result{}, err
	}

	command := []string{"git", "diff", "--no-color", "--no-ext-diff", "--unified=3"}
	runArgs := []string{"diff", "--no-color", "--no-ext-diff", "--unified=3"}
	if input.Staged {
		command = append(command, "--cached")
		runArgs = append(runArgs, "--cached")
	}
	command = append(command, "--", ".")
	runArgs = append(runArgs, "--", ".")

	runResult, output, err := runGitReviewCommand(ctx, ectx.Workspace.Root(), runArgs)
	if err != nil {
		return Result{
			Error:     formatGitReviewError(command, err),
			Execution: gitReviewExecutionRuntime(runResult),
		}, nil
	}
	return Result{
		Output:    formatGitDiffResult(command, input.Staged, output, runResult.Truncated),
		Execution: gitReviewExecutionRuntime(runResult),
	}, nil
}

func parseGitDiffInput(args json.RawMessage) (_ gitDiffInput, err error) {
	defer func() {
		err = normalizeToolInputError("git_diff", err)
	}()
	var raw struct {
		Staged json.RawMessage `json:"staged"`
	}
	if err := DecodeArgs("git_diff", args, &raw); err != nil {
		return gitDiffInput{}, err
	}
	staged, ok, err := decodeOptionalBoolArg("git_diff", raw.Staged, "staged")
	if err != nil {
		return gitDiffInput{}, err
	}
	if !ok {
		return gitDiffInput{}, ErrGitDiffStagedRequired
	}
	return gitDiffInput{Staged: staged}, nil
}
