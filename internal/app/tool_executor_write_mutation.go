package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
	"github.com/sageil/kodacode/internal/workspace"
)

type writeToolArguments struct {
	Path       string
	Content    string
	HasContent bool
}

type textMutationArguments struct {
	Path            string
	ToolName        string
	AfterContent    string
	HasAfterContent bool
}

type toolTextMutationPayload struct {
	Path    string
	Before  string
	After   string
	Existed bool
	Mode    uint32
}

func toolTextMutationPayloads(mutations []tool.TextMutation) []toolTextMutationPayload {
	if len(mutations) == 0 {
		return nil
	}
	out := make([]toolTextMutationPayload, 0, len(mutations))
	for _, mutation := range mutations {
		if strings.TrimSpace(mutation.Path) == "" {
			continue
		}
		out = append(out, toolTextMutationPayload{
			Path:    mutation.Path,
			Before:  mutation.Before,
			After:   mutation.After,
			Existed: mutation.Existed,
			Mode:    mutation.Mode,
		})
	}
	return out
}

func captureTextMutation(scope *workspace.Scope, input ExecuteToolInput) (*events.WriteMutation, textMutationArguments, error) {
	if scope == nil {
		return nil, textMutationArguments{}, nil
	}

	spec, ok, err := parseTextMutationArguments(input.ToolName, input.Arguments)
	if err != nil || !ok || strings.TrimSpace(spec.Path) == "" {
		return nil, textMutationArguments{}, nil
	}
	decision, err := scope.Check(workspace.AccessWrite, spec.Path)
	if err != nil {
		return nil, textMutationArguments{}, err
	}
	mutation := &events.WriteMutation{Path: decision.ResolvedPath}

	return mutation, spec, nil
}

func captureTextMutationBefore(mutation *events.WriteMutation) error {
	if mutation == nil || strings.TrimSpace(mutation.Path) == "" {
		return nil
	}
	data, err := os.ReadFile(mutation.Path)
	if err != nil {
		if os.IsNotExist(err) {
			mutation.Existed = false
			mutation.Before = ""
			mutation.Mode = 0
			return nil
		}
		return err
	}
	info, err := os.Stat(mutation.Path)
	if err != nil {
		return err
	}
	mutation.Existed = true
	mutation.Before = string(data)
	mutation.Mode = uint32(info.Mode())
	return nil
}

func textMutationPathMatches(mutation *events.WriteMutation, resolvedPath string) bool {
	if mutation == nil {
		return false
	}
	return filepath.Clean(mutation.Path) == filepath.Clean(resolvedPath)
}

func finalizeTextMutation(mutation *events.WriteMutation, args textMutationArguments) textMutationArguments {
	if mutation == nil || strings.TrimSpace(mutation.Path) == "" {
		return args
	}
	data, err := os.ReadFile(mutation.Path)
	if err != nil {
		return args
	}
	args.AfterContent = string(data)
	args.HasAfterContent = true
	return args
}

func parseWriteToolArguments(args json.RawMessage) (writeToolArguments, error) {
	var raw struct {
		Path    string  `json:"path"`
		Content *string `json:"content"`
	}
	if err := json.Unmarshal(args, &raw); err != nil {
		return writeToolArguments{}, err
	}
	parsed := writeToolArguments{
		Path: strings.TrimSpace(raw.Path),
	}
	if raw.Content != nil {
		parsed.Content = *raw.Content
		parsed.HasContent = true
	}
	return parsed, nil
}

func parseTextMutationArguments(toolName string, args json.RawMessage) (textMutationArguments, bool, error) {
	switch strings.TrimSpace(toolName) {
	case "write":
		parsed, err := parseWriteToolArguments(args)
		if err != nil {
			return textMutationArguments{}, false, err
		}
		return textMutationArguments{
			Path:     parsed.Path,
			ToolName: tool.WriteToolName,
		}, true, nil
	default:
		return textMutationArguments{}, false, nil
	}
}
