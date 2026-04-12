package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func formatBytes(n int) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

var writeParams = []byte(`{
	"type": "object",
	"properties": {
		"content": {"type": "string", "description": "The content to write to the file"},
		"filePath": {"type": "string", "description": "The absolute path to the file to write (must be absolute, not relative)"},
		"expectedVersion": {"type": "string", "description": "Optional version token from a prior read result. If provided, the write only succeeds when the file still matches that version."}
	},
	"required": ["content", "filePath"]
}`)

// NewWriteTool returns a Tool that writes content to a file.
func NewWriteTool() *Tool {
	return &Tool{
		Name:        "write",
		Description: prompt("write"),
		Parameters:  writeParams,
		Execute:     executeWrite,
	}
}

func executeWrite(_ context.Context, ectx ExecutionContext, args []byte) (*Result, error) {
	args = normalizeFilePathField(args)
	var params struct {
		Content         string `json:"content"`
		FilePath        string `json:"filePath"`
		ExpectedVersion string `json:"expectedVersion"`
	}
	if err := flexUnmarshal(args, &params); err != nil {
		return ErrorResult(ErrCodeInvalidArgs, fmt.Sprintf("write: invalid parameters: %v", err), false), nil
	}

	path := resolvePath(params.FilePath, ectx.WorkDir)
	title := path

	existing, readErr := os.ReadFile(path)
	fileExists := readErr == nil
	if params.ExpectedVersion != "" {
		if !fileExists {
			if os.IsNotExist(readErr) {
				return ErrorResult(ErrCodeConflict, "write: "+versionMissingMessage(path, params.ExpectedVersion), true), nil
			}
			if readErr != nil {
				return nil, fmt.Errorf("failed to read file: %w", readErr)
			}
		}
		currentVersion := fileVersion(existing)
		if currentVersion != params.ExpectedVersion {
			return ErrorResult(ErrCodeConflict, "write: "+versionMismatchMessage(path, params.ExpectedVersion, currentVersion), true), nil
		}
	}
	if fileExists && string(existing) == params.Content {
		summary := fmt.Sprintf("Unchanged %s (%s)", filepath.Base(path), formatBytes(len(params.Content)))
		if ectx.WriteOutput != nil {
			ectx.WriteOutput(summary + "\n")
		}
		return &Result{
			Title:  title,
			Output: summary,
			Metadata: map[string]any{
				"filepath": path,
				"exists":   true,
				"changed":  false,
				"version":  fileVersion(existing),
			},
		}, nil
	}

	_, statErr := os.Stat(path)
	fileExists = statErr == nil

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create parent directories: %w", err)
	}

	if err := writeFileWithExistingMode(path, []byte(params.Content)); err != nil {
		if res := mutationPathErrorResult("write", err); res != nil {
			return res, nil
		}
		return nil, fmt.Errorf("failed to write file: %w", err)
	}
	if info, err := os.Stat(path); err == nil {
		storeSnapshot(path, info, fileVersion([]byte(params.Content)), displayLineCount(params.Content))
	} else {
		invalidateSnapshot(path)
	}

	size := len(params.Content)
	version := fileVersion([]byte(params.Content))
	action := "Created"
	if fileExists {
		action = "Wrote"
	}
	summary := fmt.Sprintf("%s %s (%s)\nVersion: %s", action, filepath.Base(path), formatBytes(size), version)
	if ectx.WriteOutput != nil {
		ectx.WriteOutput(summary + "\n")
	}

	return &Result{
		Title:  title,
		Output: summary,
		Metadata: map[string]any{
			"filepath": path,
			"exists":   fileExists,
			"changed":  true,
			"version":  version,
		},
	}, nil
}
