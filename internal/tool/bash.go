package tool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/sageil/kodacode/internal/workspace"
)

const (
	BashToolName = "bash"
)

var (
	ErrBashCommandRequired = errors.New("cmd is required")
)

type BashTool struct{}

type bashInput struct {
	Command          string   `json:"cmd"`
	WorkingDirectory string   `json:"workdir"`
	NetworkTargets   []string `json:"-"`
	LoginShell       bool     `json:"-"`
	ShellProgram     string   `json:"shell"`
	TTY              bool     `json:"tty"`
	YieldTimeMS      int      `json:"yield_time_ms"`
	MaxOutputTokens  int      `json:"max_output_tokens"`
	PrefixRule       []string `json:"prefix_rule"`
}

func NewBashTool() BashTool {
	return BashTool{}
}

func (BashTool) Definition() Definition {
	return Definition{
		Name:                BashToolName,
		Description:         "Run a shell command under the runtime-owned execution contract. Usually send only `cmd`; `workdir` and the other fields are optional overrides when the runtime defaults are not enough.",
		ProviderDescription: "Run a shell command under the runtime execution contract. Usually send only `cmd`; add overrides only when needed.",
		InputSchema:         json.RawMessage(`{"type":"object","properties":{"cmd":{"type":"string","description":"Shell command to execute."},"workdir":{"type":["string","null"],"description":"Optional working directory. Use null or omit it to accept the default workspace directory."},"login":{"type":["boolean","string","null"],"description":"Optional login-shell override. Omit it to accept the runtime default."},"max_output_tokens":{"type":["integer","string","null"],"minimum":1,"description":"Optional output cap override. Omit it to accept the runtime default."},"prefix_rule":{"type":["array","null"],"items":{"type":"string"},"minItems":1,"description":"Optional approved command prefix hint for future permission requests."},"shell":{"type":["string","null"],"description":"Optional shell override. Omit it to accept the runtime shell."},"tty":{"type":["boolean","string","null"],"description":"Optional TTY override. Omit it to accept the runtime default."},"yield_time_ms":{"type":["integer","string","null"],"minimum":1,"description":"Optional execution yield hint. Omit it to accept the runtime default."}},"required":["cmd"],"additionalProperties":false}`),
		ArgumentExamples:    []string{`{"cmd":"git status"}`},
		RequiresWorkspace:   true,
	}
}

func (BashTool) Execute(context.Context, ExecutionContext, json.RawMessage) (Result, error) {
	return Result{}, ErrRuntimeOwnedExecution
}

func (BashTool) ExecutionRequest(workspaceRoot string, args json.RawMessage) (ExecutionRequest, bool, error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return ExecutionRequest{}, false, ErrWorkspaceRequired
	}
	input, err := parseBashInput(args)
	if err != nil {
		return ExecutionRequest{}, false, err
	}
	scope, err := workspace.New(workspaceRoot)
	if err != nil {
		return ExecutionRequest{}, false, err
	}
	workingDir, err := resolveCommandWorkingDir(scope, input.WorkingDirectory)
	if err != nil {
		return ExecutionRequest{}, false, err
	}
	pathAnalysis, err := analyzeBashPathRequests(workingDir, input.Command)
	if err != nil {
		return ExecutionRequest{}, false, InvalidArguments(BashToolName, err)
	}
	intentAnalysis, err := analyzeBashIntent(workingDir, input.Command)
	if err != nil {
		return ExecutionRequest{}, false, InvalidArguments(BashToolName, err)
	}
	effect := bashExecutionEffect(pathAnalysis, intentAnalysis, input.NetworkTargets, input.Command)
	return ExecutionRequest{
		Kind:             BashToolName,
		Preview:          input.Command,
		WorkingDirectory: workingDir,
		ShellCommand:     input.Command,
		PathRequests:     append([]PathRequest(nil), pathAnalysis.Requests...),
		OpaquePathReason: pathAnalysis.OpaquePathReason,
		NetworkTargets:   append([]string(nil), input.NetworkTargets...),
		Intent:           intentAnalysis.Intent,
		IntentCommand:    intentAnalysis.IntentCommand,
		Effect:           effect,
		ReadyPatterns:    append([]string(nil), intentAnalysis.ReadyPatterns...),
		LoginShell:       input.LoginShell,
		ShellProgram:     input.ShellProgram,
		TTY:              input.TTY,
		YieldTimeMS:      input.YieldTimeMS,
		MaxOutputTokens:  input.MaxOutputTokens,
		PrefixRule:       append([]string(nil), input.PrefixRule...),
	}, true, nil
}

func BashArgumentsExecutionEffect(args json.RawMessage) (ExecutionEffect, error) {
	input, err := parseBashInput(args)
	if err != nil {
		return "", err
	}
	intentAnalysis, err := analyzeBashIntent(".", input.Command)
	if err != nil {
		return "", err
	}
	return bashExecutionEffect(bashPathAnalysis{}, intentAnalysis, input.NetworkTargets, input.Command), nil
}

func parseBashInput(args json.RawMessage) (_ bashInput, err error) {
	defer func() {
		err = normalizeToolInputError(BashToolName, err)
	}()
	var raw struct {
		Command          string          `json:"cmd"`
		WorkingDirectory *string         `json:"workdir"`
		LoginShell       json.RawMessage `json:"login"`
		ShellProgram     *string         `json:"shell"`
		TTY              json.RawMessage `json:"tty"`
		YieldTimeMS      json.RawMessage `json:"yield_time_ms"`
		MaxOutputTokens  json.RawMessage `json:"max_output_tokens"`
		PrefixRule       *[]string       `json:"prefix_rule"`
	}
	if err := DecodeArgs(BashToolName, args, &raw); err != nil {
		return bashInput{}, err
	}

	command, loginShell, err := normalizeShellCommand(raw.Command)
	if err != nil {
		if errors.Is(err, ErrShellCommandRequired) {
			return bashInput{}, ErrBashCommandRequired
		}
		return bashInput{}, err
	}
	tty, hasTTY, err := decodeOptionalBoolArg(BashToolName, raw.TTY, "tty")
	if err != nil {
		return bashInput{}, err
	}
	if !hasTTY {
		tty = false
	}
	yieldTimeMS, hasYieldTimeMS, err := decodeOptionalIntArg(BashToolName, raw.YieldTimeMS, "yield_time_ms")
	if err != nil {
		return bashInput{}, err
	}
	if !hasYieldTimeMS {
		yieldTimeMS = 0
	}
	maxOutputTokens, hasMaxOutputTokens, err := decodeOptionalIntArg(BashToolName, raw.MaxOutputTokens, "max_output_tokens")
	if err != nil {
		return bashInput{}, err
	}
	if !hasMaxOutputTokens {
		maxOutputTokens = 0
	}

	input := bashInput{
		Command:         command,
		LoginShell:      false,
		ShellProgram:    trimOptionalString(raw.ShellProgram),
		TTY:             tty,
		YieldTimeMS:     yieldTimeMS,
		MaxOutputTokens: maxOutputTokens,
	}
	input.WorkingDirectory = trimOptionalString(raw.WorkingDirectory)
	if loginOverride, ok, err := decodeOptionalBoolArg(BashToolName, raw.LoginShell, "login"); err != nil {
		return bashInput{}, err
	} else if ok {
		input.LoginShell = loginOverride
	} else {
		input.LoginShell = loginShell || input.LoginShell
	}
	if input.WorkingDirectory == "" {
		input.WorkingDirectory = "."
	}
	if raw.PrefixRule != nil {
		for _, token := range *raw.PrefixRule {
			if trimmed := strings.TrimSpace(token); trimmed != "" {
				input.PrefixRule = append(input.PrefixRule, trimmed)
			}
		}
	}
	input.NetworkTargets = assessBashNetworkTargets(input.Command, input.PrefixRule)
	return input, nil
}

func trimOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
