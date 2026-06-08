package app

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/internal/workspace"
)

var (
	ErrCommandOptionValueRequired = errors.New("command option value is required")
	ErrUnknownCommandOption       = errors.New("unknown command option")
)

type CommandInput struct {
	UserText                 string
	WorkspaceRoot            string
	AdditionalWorkspaceRoots []string
	SkillIDs                 []string
	WorkflowID               string
	Resume                   bool
}

func ParseCommandInput(args []string, cwd string) (CommandInput, error) {
	input := CommandInput{}
	rest, literalArgs, err := parseCommandOptions(args, cwd, &input)
	if err != nil {
		return CommandInput{}, err
	}
	if !literalArgs {
		if workspaceRoot, remaining := consumeWorkspaceRootArg(cwd, rest); workspaceRoot != "" {
			input.WorkspaceRoot = workspaceRoot
			rest = remaining
		}
	}
	input.UserText = strings.TrimSpace(strings.Join(rest, " "))
	return input, nil
}

func parseCommandOptions(args []string, cwd string, input *CommandInput) ([]string, bool, error) {
	if input == nil {
		return nil, false, nil
	}
	for idx := 0; idx < len(args); idx++ {
		arg := args[idx]
		switch {
		case arg == "--":
			return append([]string(nil), args[idx+1:]...), true, nil
		case arg == "--resume":
			input.Resume = true
		case arg == "--add-dir":
			if idx+1 >= len(args) || strings.TrimSpace(args[idx+1]) == "" {
				return nil, false, ErrCommandOptionValueRequired
			}
			root, ok := resolveWorkspaceRootArg(cwd, args[idx+1])
			if !ok {
				return nil, false, workspace.ErrRootNotDirectory
			}
			input.AdditionalWorkspaceRoots = appendUniqueValue(input.AdditionalWorkspaceRoots, root)
			idx++
		case strings.HasPrefix(arg, "--add-dir="):
			root, ok := resolveWorkspaceRootArg(cwd, strings.TrimSpace(strings.TrimPrefix(arg, "--add-dir=")))
			if !ok {
				return nil, false, workspace.ErrRootNotDirectory
			}
			input.AdditionalWorkspaceRoots = appendUniqueValue(input.AdditionalWorkspaceRoots, root)
		case arg == "--skill":
			if idx+1 >= len(args) || strings.TrimSpace(args[idx+1]) == "" {
				return nil, false, ErrCommandOptionValueRequired
			}
			input.SkillIDs = appendUniqueValue(input.SkillIDs, strings.TrimSpace(args[idx+1]))
			idx++
		case strings.HasPrefix(arg, "--skill="):
			skillID := strings.TrimSpace(strings.TrimPrefix(arg, "--skill="))
			if skillID == "" {
				return nil, false, ErrCommandOptionValueRequired
			}
			input.SkillIDs = appendUniqueValue(input.SkillIDs, skillID)
		case arg == "--workflow":
			if idx+1 >= len(args) || strings.TrimSpace(args[idx+1]) == "" {
				return nil, false, ErrCommandOptionValueRequired
			}
			input.WorkflowID = strings.TrimSpace(args[idx+1])
			idx++
		case strings.HasPrefix(arg, "--workflow="):
			workflowID := strings.TrimSpace(strings.TrimPrefix(arg, "--workflow="))
			if workflowID == "" {
				return nil, false, ErrCommandOptionValueRequired
			}
			input.WorkflowID = workflowID
		case strings.HasPrefix(arg, "-"):
			return nil, false, ErrUnknownCommandOption
		default:
			return append([]string(nil), args[idx:]...), false, nil
		}
	}
	return nil, false, nil
}

func consumeWorkspaceRootArg(cwd string, args []string) (string, []string) {
	if strings.TrimSpace(cwd) == "" || len(args) == 0 {
		return "", args
	}
	workspaceRoot, ok := resolveWorkspaceRootArg(cwd, args[0])
	if !ok {
		return "", args
	}
	return workspaceRoot, append([]string(nil), args[1:]...)
}

func resolveWorkspaceRootArg(cwd, arg string) (string, bool) {
	operand := strings.TrimSpace(arg)
	if operand == "" {
		return "", false
	}
	candidate := operand
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(cwd, candidate)
	}
	scope, err := workspace.New(candidate)
	if err != nil {
		return "", false
	}
	return scope.Root(), true
}

func appendUniqueValue(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
