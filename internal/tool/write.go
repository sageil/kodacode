package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/workspace"
)

var (
	ErrWritePathRequired    = errors.New("path is required")
	ErrWriteContentRequired = errors.New("content is required")
)

const WriteToolName = "write"

type WriteTool struct{}

func NewWriteTool() WriteTool {
	return WriteTool{}
}

func (WriteTool) Definition() Definition {
	description := "Write complete file contents to a workspace path. Use for new files or deliberate full-file rewrites. The submitted `content` becomes the whole file; omitted text is deleted. Prefer `apply_patch` for localized edits."
	return Definition{
		Name:                 WriteToolName,
		Description:          description,
		ProviderDescription:  description,
		InputSchema:          json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Workspace-relative file path to write."},"content":{"type":"string","description":"Complete final file contents. This replaces the whole file; omitted text is deleted."}},"required":["path","content"],"additionalProperties":false}`),
		ArgumentExamples:     []string{`{"path":"file.txt","content":"hello\n"}`},
		RequiresWorkspace:    true,
		ProviderRichGuidance: true,
	}
}

func (WriteTool) PathRequests(args json.RawMessage) ([]PathRequest, error) {
	input, err := parseWriteInput(args)
	if err != nil {
		return nil, err
	}
	return []PathRequest{{
		Access: workspace.AccessWrite,
		Path:   input.Path,
		Reason: "write file contents",
	}}, nil
}

func (WriteTool) Execute(_ context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	input, err := parseWriteInput(args)
	if err != nil {
		return Result{}, err
	}
	decision, err := ectx.ResolvePath(workspace.AccessWrite, input.Path)
	if err != nil {
		return Result{}, err
	}

	err = WithFileMutationLock(decision.ResolvedPath, func() error {
		if ectx.BeforeMutation != nil {
			if err := ectx.BeforeMutation(decision.ResolvedPath); err != nil {
				return err
			}
		}
		return WriteFileAtomically(decision.ResolvedPath, []byte(input.Content), fileModeOrDefault(decision.ResolvedPath, 0o644))
	})
	if err != nil {
		return Result{}, err
	}
	return Result{
		Output: fmt.Sprintf("wrote %d bytes to %s", len(input.Content), decision.ResolvedPath),
	}, nil
}

type writeInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func parseWriteInput(args json.RawMessage) (_ writeInput, err error) {
	defer func() {
		err = normalizeToolInputError(WriteToolName, err)
	}()
	var raw struct {
		Path    string  `json:"path"`
		Content *string `json:"content"`
	}
	if err := DecodeArgs(WriteToolName, args, &raw); err != nil {
		return writeInput{}, err
	}
	if strings.TrimSpace(raw.Path) == "" {
		return writeInput{}, ErrWritePathRequired
	}
	if raw.Content == nil {
		return writeInput{}, ErrWriteContentRequired
	}
	return writeInput{
		Path:    raw.Path,
		Content: *raw.Content,
	}, nil
}
