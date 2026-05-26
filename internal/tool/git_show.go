package tool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

var ErrGitShowRevisionRequired = errors.New("rev is required")

type GitShowTool struct{}

type gitShowInput struct {
	Rev string `json:"rev"`
}

func NewGitShowTool() GitShowTool {
	return GitShowTool{}
}

func (GitShowTool) Definition() Definition {
	return Definition{
		Name:                "git_show",
		Description:         "Show workspace-scoped Git review context for one commit or revision. Use this to inspect commit metadata and the patch for the current working directory subtree. `revision`, `commit`, and `ref` are accepted as aliases for `rev`.",
		ProviderDescription: "Show one commit or revision plus its workspace-scoped patch. `revision`, `commit`, and `ref` are accepted as aliases for `rev`.",
		InputSchema:         json.RawMessage(`{"type":"object","properties":{"rev":{"type":["string","null"],"description":"Commit, tag, or revision to inspect with git show."},"revision":{"type":["string","null"],"description":"Alias for rev. Prefer rev for new calls."},"commit":{"type":["string","null"],"description":"Alias for rev. Prefer rev for new calls."},"ref":{"type":["string","null"],"description":"Alias for rev. Prefer rev for new calls."}},"anyOf":[{"required":["rev"]},{"required":["revision"]},{"required":["commit"]},{"required":["ref"]}],"additionalProperties":false}`),
		ArgumentExamples:    []string{`{"rev":"HEAD~1"}`},
		RequiresWorkspace:   true,
		ParallelSafe:        true,
	}
}

func (GitShowTool) Execute(ctx context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	if ectx.Workspace == nil {
		return Result{}, ErrWorkspaceRequired
	}
	input, err := parseGitShowInput(args)
	if err != nil {
		return Result{}, err
	}

	command := []string{"git", "show", "--no-color", "--no-ext-diff", "--format=fuller", "--unified=3", input.Rev, "--", "."}
	runResult, output, err := runGitReviewCommand(ctx, ectx.Workspace.Root(), []string{"show", "--no-color", "--no-ext-diff", "--format=fuller", "--unified=3", input.Rev, "--", "."})
	if err != nil {
		return Result{
			Error:     formatGitReviewError(command, err),
			Execution: gitReviewExecutionRuntime(runResult),
		}, nil
	}
	return Result{
		Output:    formatGitShowResult(command, input.Rev, output, runResult.Truncated),
		Execution: gitReviewExecutionRuntime(runResult),
	}, nil
}

func parseGitShowInput(args json.RawMessage) (_ gitShowInput, err error) {
	defer func() {
		err = normalizeToolInputError("git_show", err)
	}()
	var raw struct {
		Rev      *string `json:"rev"`
		Revision *string `json:"revision"`
		Commit   *string `json:"commit"`
		Ref      *string `json:"ref"`
	}
	if err := DecodeArgs("git_show", args, &raw); err != nil {
		return gitShowInput{}, err
	}
	rev := firstNonBlankStringPtr(raw.Rev, raw.Revision, raw.Commit, raw.Ref)
	if rev == "" {
		return gitShowInput{}, ErrGitShowRevisionRequired
	}
	return gitShowInput{Rev: rev}, nil
}

func firstNonBlankStringPtr(values ...*string) string {
	for _, value := range values {
		if value == nil {
			continue
		}
		if trimmed := strings.TrimSpace(*value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
