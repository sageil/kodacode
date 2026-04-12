package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

var patchParams = json.RawMessage(`{
	"type": "object",
	"properties": {
		"filePath": {"type": "string", "description": "Absolute path to the file to modify"},
		"expectedVersion": {"type": "string", "description": "Optional version token from a prior read result. If provided, the patch only succeeds when the file still matches that version."},
		"edits": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"oldString": {"type": "string", "description": "Text to find"},
					"newString": {"type": "string", "description": "Replacement text"},
					"startLine": {"type": "integer", "description": "Optional 1-based start line to constrain this edit"},
					"endLine": {"type": "integer", "description": "Optional 1-based end line to constrain this edit"},
					"range": {
						"type": "object",
						"description": "Optional exact 1-based line, 0-based UTF-16 character range to replace directly",
						"properties": {
							"startLine": {"type": "integer"},
							"startCharacter": {"type": "integer"},
							"endLine": {"type": "integer"},
							"endCharacter": {"type": "integer"}
						},
						"required": ["startLine", "startCharacter", "endLine", "endCharacter"]
					}
				},
				"required": ["oldString", "newString"]
			},
			"description": "Ordered list of non-overlapping edits matched against the original file snapshot"
		}
	},
	"required": ["filePath", "edits"]
}`)

func NewPatchTool() *Tool {
	return &Tool{
		Name:        "patch",
		Description: prompt("patch"),
		Parameters:  patchParams,
		Execute:     executePatch,
	}
}

type patchEdit struct {
	OldString string     `json:"oldString"`
	NewString string     `json:"newString"`
	StartLine int        `json:"startLine"`
	EndLine   int        `json:"endLine"`
	Range     *textRange `json:"range"`
}

func executePatch(ctx context.Context, ectx ExecutionContext, args []byte) (*Result, error) {
	args = normalizeFilePathField(args)
	var params struct {
		FilePath        string      `json:"filePath"`
		ExpectedVersion string      `json:"expectedVersion"`
		Edits           []patchEdit `json:"edits"`
	}
	if err := flexUnmarshal(args, &params); err != nil {
		return ErrorResult(ErrCodeInvalidArgs, fmt.Sprintf("patch: invalid arguments: %v", err), false), nil
	}

	if len(params.Edits) == 0 {
		return ErrorResult(ErrCodeInvalidArgs, "patch: edits array must not be empty", false), nil
	}

	path := resolvePath(params.FilePath, ectx.WorkDir)
	title := path

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrorResult(ErrCodeNotFound, fmt.Sprintf("patch: file %q not found", path), false), nil
		}
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Validate every edit against the original file snapshot, then apply once.
	original := string(content)
	if params.ExpectedVersion != "" {
		currentVersion := fileVersion(content)
		if currentVersion != params.ExpectedVersion {
			return ErrorResult(ErrCodeConflict, "patch: "+versionMismatchMessage(path, params.ExpectedVersion, currentVersion), true), nil
		}
	}
	var diffBuf strings.Builder
	spans := make([]replacementSpan, 0, len(params.Edits))

	for i, edit := range params.Edits {
		if edit.OldString == edit.NewString {
			return ErrorResult(ErrCodeInvalidArgs, fmt.Sprintf("patch: edit %d has identical oldString and newString", i), false), nil
		}
		if edit.Range != nil && edit.Range.active() && (edit.StartLine != 0 || edit.EndLine != 0) {
			return ErrorResult(ErrCodeInvalidArgs, fmt.Sprintf("patch: edit %d: use either range or startLine/endLine, not both", i), false), nil
		}
		var span replacementSpan
		if edit.Range != nil && edit.Range.active() {
			rng, err := normalizeTextRange(original, *edit.Range)
			if err != nil {
				return ErrorResult(ErrCodeInvalidArgs, fmt.Sprintf("patch: edit %d: %v", i, err), false), nil
			}
			span, err = locateRangeReplacement(original, edit.OldString, edit.NewString, rng)
			if err != nil {
				var rf *replaceFailure
				if errors.As(err, &rf) {
					code := ErrCodeInvalidArgs
					if rf.code == "not_found" {
						code = ErrCodeNotFound
					}
					return ErrorResult(code, fmt.Sprintf("patch: edit %d: %s", i, rf.message), rf.retryable()), nil
				}
				return nil, fmt.Errorf("edit %d: %w", i, err)
			}
		} else {
			scope, err := normalizeLineScope(original, edit.StartLine, edit.EndLine)
			if err != nil {
				return ErrorResult(ErrCodeInvalidArgs, fmt.Sprintf("patch: edit %d: %v", i, err), false), nil
			}
			span, err = locateExactReplacement(original, edit.OldString, edit.NewString, scope)
			if err != nil {
				var rf *replaceFailure
				if errors.As(err, &rf) {
					code := ErrCodeInvalidArgs
					if rf.code == "not_found" {
						code = ErrCodeNotFound
					}
					return ErrorResult(code, fmt.Sprintf("patch: edit %d: %s", i, rf.message), rf.retryable()), nil
				}
				return nil, fmt.Errorf("edit %d: %w", i, err)
			}
		}

		// Build unified-style diff snippet for this edit.
		fmt.Fprintf(&diffBuf, "--- edit %d ---\n", i+1)
		for line := range strings.SplitSeq(edit.OldString, "\n") {
			fmt.Fprintf(&diffBuf, "- %s\n", line)
		}
		for line := range strings.SplitSeq(edit.NewString, "\n") {
			fmt.Fprintf(&diffBuf, "+ %s\n", line)
		}
		span.index = i
		spans = append(spans, span)
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	for i := 1; i < len(spans); i++ {
		if spans[i-1].start == spans[i].start {
			return ErrorResult(ErrCodeInvalidArgs, fmt.Sprintf("patch: edits %d and %d begin at the same position in the original file; split them into separate, unambiguous patch calls", spans[i-1].index, spans[i].index), false), nil
		}
		if spans[i-1].end > spans[i].start {
			return ErrorResult(ErrCodeInvalidArgs, fmt.Sprintf("patch: edits %d and %d overlap in the original file; split them into non-overlapping hunks", spans[i-1].index, spans[i].index), false), nil
		}
	}
	current := applyReplacementSpans(original, spans)
	if current == original {
		return &Result{
			Title:  title,
			Output: "No changes applied.",
			Metadata: map[string]any{
				"filepath": path,
				"edits":    len(params.Edits),
				"changed":  false,
			},
		}, nil
	}

	// All edits succeeded — write once.
	if err := writeFileWithExistingMode(path, []byte(current)); err != nil {
		if res := mutationPathErrorResult("patch", err); res != nil {
			return res, nil
		}
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	summary := fmt.Sprintf("Applied %d edits to %s\n\n%s", len(params.Edits), path, diffBuf.String())
	version := fileVersion([]byte(current))
	if info, err := os.Stat(path); err == nil {
		storeSnapshot(path, info, version, displayLineCount(current))
	} else {
		invalidateSnapshot(path)
	}

	return &Result{
		Title:  title,
		Output: summary + "\nVersion: " + version,
		Metadata: map[string]any{
			"filepath": path,
			"edits":    len(params.Edits),
			"changed":  true,
			"version":  version,
		},
	}, nil
}
