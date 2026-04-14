package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var editParams = json.RawMessage(`{
	"type": "object",
	"properties": {
		"filePath": {"type": "string", "description": "The absolute path to the file to modify"},
		"oldString": {"type": "string", "description": "The text to replace"},
		"newString": {"type": "string", "description": "The text to replace it with (must be different from oldString)"},
		"replaceAll": {"type": "boolean", "description": "Replace all occurrences of oldString (default false)"},
		"startLine": {"type": "integer", "description": "Optional 1-based start line to constrain the edit search. Omit this when range is set."},
		"endLine": {"type": "integer", "description": "Optional 1-based end line to constrain the edit search. Omit this when range is set."},
		"expectedVersion": {"type": "string", "description": "Optional version token from a prior read result. If provided, the edit only succeeds when the file still matches that version."},
		"range": {
			"type": "object",
			"description": "Optional exact 1-based line, 0-based UTF-16 character range to replace directly. When range is set, omit startLine and endLine.",
			"properties": {
				"startLine": {"type": "integer"},
				"startCharacter": {"type": "integer"},
				"endLine": {"type": "integer"},
				"endCharacter": {"type": "integer"}
			},
			"required": ["startLine", "startCharacter", "endLine", "endCharacter"]
		}
	},
	"required": ["filePath", "oldString", "newString"]
}`)

func NewEditTool() *Tool {
	return &Tool{
		Name:        "edit",
		Description: prompt("edit"),
		Parameters:  editParams,
		Execute:     executeEdit,
	}
}

func executeEdit(ctx context.Context, ectx ExecutionContext, args []byte) (*Result, error) {
	args = normalizeFilePathField(args)
	var params struct {
		FilePath        string     `json:"filePath"`
		OldString       string     `json:"oldString"`
		NewString       string     `json:"newString"`
		ReplaceAll      bool       `json:"replaceAll"`
		StartLine       int        `json:"startLine"`
		EndLine         int        `json:"endLine"`
		ExpectedVersion string     `json:"expectedVersion"`
		Range           *textRange `json:"range"`
	}
	if err := flexUnmarshal(args, &params); err != nil {
		return ErrorResult(ErrCodeInvalidArgs, fmt.Sprintf("edit: invalid arguments: %v", err), false), nil
	}
	preferExactRange(params.Range, &params.StartLine, &params.EndLine)
	if params.OldString == params.NewString && params.OldString != "" {
		return ErrorResult(ErrCodeInvalidArgs, "edit: oldString and newString must be different", false), nil
	}

	path := resolvePath(params.FilePath, ectx.WorkDir)
	title := path

	if params.OldString == "" {
		existing, readErr := os.ReadFile(path)
		if params.ExpectedVersion != "" {
			if os.IsNotExist(readErr) {
				return ErrorResult(ErrCodeConflict, "edit: "+versionMissingMessage(path, params.ExpectedVersion), true), nil
			}
			if readErr != nil {
				return nil, fmt.Errorf("failed to read file: %w", readErr)
			}
			currentVersion := fileVersion(existing)
			if currentVersion != params.ExpectedVersion {
				return ErrorResult(ErrCodeConflict, "edit: "+versionMismatchMessage(path, params.ExpectedVersion, currentVersion), true), nil
			}
		}
		if readErr == nil && string(existing) == params.NewString {
			version := fileVersion(existing)
			return &Result{
				Title:  title,
				Output: "File already up to date.\nVersion: " + version,
				Metadata: map[string]any{
					"filepath": path,
					"changed":  false,
					"version":  version,
				},
			}, nil
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("failed to create parent directories: %w", err)
		}
		if err := writeFileWithExistingMode(path, []byte(params.NewString)); err != nil {
			if res := mutationPathErrorResult("edit", err); res != nil {
				return res, nil
			}
			return nil, fmt.Errorf("failed to write file: %w", err)
		}
		version := fileVersion([]byte(params.NewString))
		if info, err := os.Stat(path); err == nil {
			storeSnapshot(path, info, version, displayLineCount(params.NewString))
		} else {
			invalidateSnapshot(path)
		}
		return &Result{
			Title:  title,
			Output: "Created/overwrote file successfully.\nVersion: " + version,
			Metadata: map[string]any{
				"filepath": path,
				"changed":  true,
				"version":  version,
			},
		}, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrorResult(ErrCodeNotFound, fmt.Sprintf("edit: file %q not found", path), false), nil
		}
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	current := string(content)
	if params.ExpectedVersion != "" {
		currentVersion := fileVersion(content)
		if currentVersion != params.ExpectedVersion {
			return ErrorResult(ErrCodeConflict, "edit: "+versionMismatchMessage(path, params.ExpectedVersion, currentVersion), true), nil
		}
	}

	var result string
	if params.Range != nil && params.Range.active() {
		rng, err := normalizeTextRange(current, *params.Range)
		if err != nil {
			return ErrorResult(ErrCodeInvalidArgs, fmt.Sprintf("edit: %v", err), false), nil
		}
		result, err = replaceByTextRange(current, params.OldString, params.NewString, rng)
		if err != nil {
			var rf *replaceFailure
			if errors.As(err, &rf) {
				code := ErrCodeInvalidArgs
				if rf.code == "not_found" {
					code = ErrCodeNotFound
				}
				return ErrorResult(code, rf.message, rf.retryable()), nil
			}
			return nil, err
		}
	} else {
		scope, err := normalizeLineScope(current, params.StartLine, params.EndLine)
		if err != nil {
			return ErrorResult(ErrCodeInvalidArgs, fmt.Sprintf("edit: %v", err), false), nil
		}
		result, err = replace(current, params.OldString, params.NewString, params.ReplaceAll, scope)
		if err != nil {
			var rf *replaceFailure
			if errors.As(err, &rf) {
				code := ErrCodeInvalidArgs
				if rf.code == "not_found" {
					code = ErrCodeNotFound
				}
				return ErrorResult(code, rf.message, rf.retryable()), nil
			}
			return nil, err
		}
	}
	if result == current {
		return &Result{
			Title:  title,
			Output: "File already up to date.",
			Metadata: map[string]any{
				"filepath": path,
				"changed":  false,
			},
		}, nil
	}

	if err := writeFileWithExistingMode(path, []byte(result)); err != nil {
		if res := mutationPathErrorResult("edit", err); res != nil {
			return res, nil
		}
		return nil, fmt.Errorf("failed to write file: %w", err)
	}
	version := fileVersion([]byte(result))
	if info, err := os.Stat(path); err == nil {
		storeSnapshot(path, info, version, displayLineCount(result))
	} else {
		invalidateSnapshot(path)
	}

	return &Result{
		Title:  title,
		Output: "Edited file successfully.\nVersion: " + version,
		Metadata: map[string]any{
			"filepath": path,
			"changed":  true,
			"version":  version,
		},
	}, nil
}

func replace(content, old, new string, all bool, scopes ...lineScope) (string, error) {
	scope := lineScope{}
	if len(scopes) > 0 {
		scope = scopes[0]
	}
	prefix, segment, suffix := scopedContent(content, scope)

	result, found, ambiguous := simpleReplacer(segment, old, new)
	if found && !ambiguous {
		if scope.active() {
			return prefix + result + suffix, nil
		}
		return result, nil
	}

	if all && ambiguous {
		result, found, _ = multiOccurrenceReplacer(segment, old, new)
		if found {
			if scope.active() {
				return prefix + result + suffix, nil
			}
			return result, nil
		}
	}

	if ambiguous {
		return "", &replaceFailure{
			code:    "ambiguous",
			message: replaceFailureMessage("ambiguous", segment, old, scope),
		}
	}
	return "", &replaceFailure{
		code:    "not_found",
		message: replaceFailureMessage("not_found", segment, old, scope),
	}
}
