package tool

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/sageil/kodacode/internal/workspace"
)

const TestToolName = "test"

type TestTool struct{}

func NewTestTool() TestTool {
	return TestTool{}
}

func (TestTool) Definition() Definition {
	return Definition{
		Name:                TestToolName,
		Description:         "Run one-shot project tests under the runtime-owned execution contract. Use path to focus detection on a directory or file, filter to narrow tests for supported frameworks, and timeout in milliseconds to override the default 2 minute limit. For example, 90000 means 90 seconds and 600 means 0.6 seconds. This tool is for one-shot test runs only; watch or dev-server test commands are rejected.",
		ProviderDescription: "Run one-shot project tests. Use `path` or `filter` to focus; watch and dev-server test commands are rejected.",
		InputSchema:         json.RawMessage(`{"type":"object","properties":{"command":{"type":["string","null"],"description":"Optional explicit test command such as \"go test ./...\" or \"pytest\". Use null or omit this field to auto-detect from the selected path."},"path":{"type":["string","null"],"description":"Optional directory or file to test. Relative paths resolve from the workspace directory. Use null or omit this field to test from the workspace directory."},"filter":{"type":["string","null"],"description":"Optional test name filter for supported frameworks. Use null or omit this field to run the full target."},"timeout":{"type":["integer","string","null"],"minimum":5000,"description":"Optional timeout in milliseconds. Use null or omit this field to accept the default 120000 ms. Example: 90000 means 90 seconds, while 600 means 0.6 seconds and will be rejected as too small."}},"required":[],"additionalProperties":false}`),
		ArgumentExamples:    []string{`{"command":null,"path":"internal/tool","filter":null,"timeout":90000}`},
		RequiresWorkspace:   true,
	}
}

func (TestTool) Execute(context.Context, ExecutionContext, json.RawMessage) (Result, error) {
	return Result{}, ErrRuntimeOwnedExecution
}

func (TestTool) ExecutionRequest(workspaceRoot string, args json.RawMessage) (ExecutionRequest, bool, error) {
	if workspaceRoot == "" {
		return ExecutionRequest{}, false, ErrWorkspaceRequired
	}
	input, err := parseTestInput(args)
	if err != nil {
		return ExecutionRequest{}, false, normalizeTestExecutionRequestError(err)
	}
	scope, err := workspace.New(workspaceRoot)
	if err != nil {
		return ExecutionRequest{}, false, err
	}
	target, err := resolveTestTarget(scope, input.Path)
	if err != nil {
		return ExecutionRequest{}, false, normalizeTestExecutionRequestError(err)
	}
	plan, err := planTestCommand(scope.Root(), input, target)
	if err != nil {
		return ExecutionRequest{}, false, normalizeTestExecutionRequestError(err)
	}
	pathAnalysis, err := analyzeBashPathRequests(plan.WorkingDir, plan.Command)
	if err != nil {
		return ExecutionRequest{}, false, err
	}
	return ExecutionRequest{
		Kind:             TestToolName,
		Preview:          plan.Command,
		WorkingDirectory: plan.WorkingDir,
		ShellCommand:     plan.Command,
		PathRequests:     append([]PathRequest(nil), pathAnalysis.Requests...),
		OpaquePathReason: pathAnalysis.OpaquePathReason,
		NetworkTargets:   assessBashNetworkTargets(plan.Command, nil),
		Intent:           ExecutionIntentOneShot,
		IntentCommand:    plan.Command,
		TimeoutMS:        input.TimeoutMS,
	}, true, nil
}

func normalizeTestExecutionRequestError(err error) error {
	if err == nil || errors.Is(err, ErrInvalidArguments) {
		return err
	}
	switch {
	case errors.Is(err, ErrTestPathRequired),
		errors.Is(err, ErrTestTimeoutInvalid),
		errors.Is(err, ErrTestFilterUnsupported),
		errors.Is(err, ErrTestPathTargetUnsupported),
		errors.Is(err, ErrTestWatchModeUnsupported),
		errors.Is(err, ErrTestFrameworkNotDetected),
		errors.Is(err, workspace.ErrPathRequired),
		errors.Is(err, os.ErrNotExist):
		return InvalidArguments(TestToolName, err)
	default:
		return err
	}
}
