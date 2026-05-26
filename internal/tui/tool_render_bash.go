package tui

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type bashToolViewInput struct {
	Command          string   `json:"cmd"`
	WorkingDirectory string   `json:"workdir"`
	PrefixRule       []string `json:"prefix_rule"`
	Shell            string   `json:"shell"`
}

func bashInspectorParams(call *events.ToolCallState) []inspectorParam {
	input, ok := parseBashToolViewInput(call.Input)
	if !ok {
		return defaultInspectorParams(call)
	}

	workingDir := input.WorkingDirectory
	if strings.TrimSpace(workingDir) == "" {
		workingDir = "."
	}
	params := []inspectorParam{
		{Label: "Dir", Value: workingDir},
		{Label: "Cmd", Value: input.Command},
	}
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}

func parseBashToolViewInput(raw string) (bashToolViewInput, bool) {
	var input bashToolViewInput
	if json.Unmarshal([]byte(raw), &input) != nil || strings.TrimSpace(input.Command) == "" {
		return input, false
	}
	input.Command = strings.TrimSpace(input.Command)
	input.WorkingDirectory = strings.TrimSpace(input.WorkingDirectory)
	trimmedRule := make([]string, 0, len(input.PrefixRule))
	for _, token := range input.PrefixRule {
		if trimmed := strings.TrimSpace(token); trimmed != "" {
			trimmedRule = append(trimmedRule, trimmed)
		}
	}
	input.PrefixRule = trimmedRule
	return input, true
}

func bashCommandDisplayName(workspaceRoot string, call *events.ToolCallState) string {
	input, ok := parseBashToolViewInput(call.Input)
	if !ok {
		return "bash"
	}
	if strings.TrimSpace(input.Command) == "" {
		return "bash"
	}
	command := displayCommandPath(workspaceRoot, input.Command)
	for strings.HasPrefix(command, "cd . && ") || strings.HasPrefix(command, "cd ./ && ") {
		command = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(command, "cd . && "), "cd ./ && "))
	}
	return command
}

func bashToolDisplayName(workspaceRoot string, call *events.ToolCallState) string {
	command := strings.TrimSpace(bashCommandDisplayName(workspaceRoot, call))
	if command == "" || command == "bash" {
		return "bash"
	}
	return "bash " + command
}

func displayCommandPath(workspaceRoot, command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	root := strings.TrimSpace(workspaceRoot)
	if root == "" {
		return command
	}
	withSlash := root + string(filepath.Separator)
	command = strings.ReplaceAll(command, withSlash, "")
	command = strings.ReplaceAll(command, root, ".")
	return command
}
