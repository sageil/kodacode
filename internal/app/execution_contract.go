package app

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

const (
	executionCommandTimeout   = 2 * time.Minute
	executionCommandOutputCap = 16000
	approxBytesPerOutputToken = 8
)

var errExecutionCommandRequired = errors.New("execution command is required")

func buildExecutionContract(
	config ExecutionConfig,
	request tool.ExecutionRequest,
	execPolicy *events.ExecutionPolicyAmendment,
) (executionContract, error) {
	workingDir := strings.TrimSpace(request.WorkingDirectory)
	if workingDir == "" {
		return executionContract{}, tool.ErrCommandWorkingDirMissing
	}

	appliedConfig := applyExecutionPolicy(config, execPolicy)
	if shellProgram := strings.TrimSpace(request.ShellProgram); shellProgram != "" {
		appliedConfig.ShellProgram = shellProgram
	}
	command, err := executionCommand(appliedConfig, request)
	if err != nil {
		return executionContract{}, err
	}

	outputLimit := executionOutputLimit(request.MaxOutputTokens)
	timeout := executionTimeout(request.TimeoutMS)
	return localExecutionContract(workingDir, command, timeout, outputLimit), nil
}

func executionCommand(config ExecutionConfig, request tool.ExecutionRequest) ([]string, error) {
	if strings.TrimSpace(request.ShellCommand) != "" {
		loginShell := request.LoginShell
		if !config.AllowLoginShell {
			loginShell = false
		}
		return shellExecArgs(config, request.ShellCommand, loginShell), nil
	}
	if len(request.Command) == 0 || strings.TrimSpace(request.Command[0]) == "" {
		return nil, errExecutionCommandRequired
	}
	return append([]string(nil), request.Command...), nil
}

func executionOutputLimit(maxOutputTokens int) int {
	if maxOutputTokens <= 0 {
		return executionCommandOutputCap
	}
	limit := maxOutputTokens * approxBytesPerOutputToken
	if limit <= 0 {
		return executionCommandOutputCap
	}
	return limit
}

func executionTimeout(timeoutMS int) time.Duration {
	if timeoutMS <= 0 {
		return executionCommandTimeout
	}
	return time.Duration(timeoutMS) * time.Millisecond
}

func applyExecutionPolicy(config ExecutionConfig, execPolicy *events.ExecutionPolicyAmendment) ExecutionConfig {
	out := config
	if execPolicy != nil {
		if execPolicy.AllowLoginShell != nil {
			out.AllowLoginShell = *execPolicy.AllowLoginShell
		}
	}
	return out
}

func preferredShellPath(config ExecutionConfig) string {
	if shellProgram := strings.TrimSpace(config.ShellProgram); shellProgram != "" {
		return shellProgram
	}
	// retained for callers that rely on platform defaults when no runtime shell is configured
	if runtime.GOOS == "windows" {
		if shell := strings.TrimSpace(os.Getenv("COMSPEC")); shell != "" {
			return shell
		}
		return "cmd.exe"
	}
	if shell := strings.TrimSpace(os.Getenv("SHELL")); shell != "" {
		return shell
	}
	return "/bin/sh"
}

func shellExecArgs(config ExecutionConfig, command string, loginShell bool) []string {
	shellPath := preferredShellPath(config)
	if runtime.GOOS == "windows" {
		return []string{shellPath, "/d", "/s", "/c", command}
	}
	if loginShell {
		return []string{shellPath, "-lc", command}
	}
	return []string{shellPath, "-c", command}
}
