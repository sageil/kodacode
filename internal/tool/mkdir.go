package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/sageil/kodacode/internal/workspace"
)

var (
	ErrMkdirPathRequired   = errors.New("path is required")
	ErrMkdirPathExistsFile = errors.New("path exists and is not a directory")
)

type MkdirTool struct{}

func NewMkdirTool() MkdirTool {
	return MkdirTool{}
}

func (MkdirTool) Definition() Definition {
	return Definition{
		Name:                "mkdir",
		Description:         "Ensure a directory exists at a filesystem path. Relative paths resolve from the workspace directory, and missing parent directories are created automatically.",
		ProviderDescription: "Ensure a directory exists at a path, creating parent directories as needed.",
		InputSchema:         json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Directory path to create. Relative paths resolve from the workspace directory, and missing parent directories are created automatically."}},"required":["path"],"additionalProperties":false}`),
		ArgumentExamples:    []string{`{"path":"build/cache"}`},
		RequiresWorkspace:   true,
	}
}

func (MkdirTool) PathRequests(args json.RawMessage) ([]PathRequest, error) {
	input, err := parseMkdirInput(args)
	if err != nil {
		return nil, err
	}
	return []PathRequest{{
		Access: workspace.AccessWrite,
		Path:   input.Path,
		Reason: "create directory",
	}}, nil
}

func (MkdirTool) Execute(_ context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	input, err := parseMkdirInput(args)
	if err != nil {
		return Result{}, err
	}

	decision, err := ectx.ResolvePath(workspace.AccessWrite, input.Path)
	if err != nil {
		return Result{}, err
	}

	info, err := os.Stat(decision.ResolvedPath)
	if err == nil {
		if !info.IsDir() {
			return Result{}, ErrMkdirPathExistsFile
		}
		return Result{Output: fmt.Sprintf("directory already exists: %s", decision.ResolvedPath)}, nil
	}
	if !os.IsNotExist(err) {
		return Result{}, err
	}

	if err := os.MkdirAll(decision.ResolvedPath, 0o755); err != nil {
		return Result{}, err
	}

	return Result{
		Output: fmt.Sprintf("created directory %s", decision.ResolvedPath),
	}, nil
}

type mkdirInput struct {
	Path string `json:"path"`
}

func parseMkdirInput(args json.RawMessage) (_ mkdirInput, err error) {
	defer func() {
		err = normalizeToolInputError("mkdir", err)
	}()
	var raw struct {
		Path string `json:"path"`
	}
	if err := DecodeArgs("mkdir", args, &raw); err != nil {
		return mkdirInput{}, err
	}
	if strings.TrimSpace(raw.Path) == "" {
		return mkdirInput{}, ErrMkdirPathRequired
	}
	return mkdirInput{
		Path: raw.Path,
	}, nil
}
