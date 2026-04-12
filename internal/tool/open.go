package tool

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// openArgs is the JSON shape the model sends when calling the open tool.
type openArgs struct {
	FilePath string `json:"filePath"`
	Line     int    `json:"line"`
	Editor   string `json:"editor"`
}

// terminalEditors lists editors that require a TTY and cannot be launched
// from a non-interactive context.
var terminalEditors = map[string]bool{
	"vim":   true,
	"nvim":  true,
	"vi":    true,
	"nano":  true,
	"emacs": true,
}

var guiLaunchEditors = map[string]bool{
	"code":          true,
	"code-insiders": true,
	"subl":          true,
}

// guiEditorFallbacks is the ordered list of editors to try when neither the
// editor parameter nor $EDITOR is set.
var guiEditorFallbacks = []string{"code", "vim", "nano", "vi"}

// NewOpenTool creates a tool that opens a file in the user's editor.
func NewOpenTool() *Tool {
	return &Tool{
		Name:        "open",
		Description: prompt("open"),
		Parameters: []byte(`{
			"type": "object",
			"properties": {
				"filePath": {
					"type": "string",
					"description": "Absolute path to the file to open"
				},
				"line": {
					"type": "integer",
					"description": "Line number to jump to (optional)"
				},
				"editor": {
					"type": "string",
					"description": "Editor to use. Defaults to $EDITOR, then tries: code, vim, nano"
				}
			},
			"required": ["filePath"]
		}`),
		Execute: executeOpen,
	}
}

func executeOpen(ctx context.Context, ectx ExecutionContext, args []byte) (*Result, error) {
	args = normalizeFilePathField(args)
	var oa openArgs
	if err := flexUnmarshal(args, &oa); err != nil {
		return nil, err
	}
	if oa.FilePath == "" {
		return nil, fmt.Errorf("open: filePath is required")
	}

	// Resolve and verify the file exists.
	resolved := resolvePath(oa.FilePath, ectx.WorkDir)
	if _, err := os.Stat(resolved); err != nil {
		return nil, fmt.Errorf("open: file not found: %s", resolved)
	}

	// Determine which editor to use.
	editor := resolveEditor(oa.Editor)
	if editor == "" {
		return nil, fmt.Errorf("open: no editor found; set $EDITOR or install code/vim/nano")
	}

	cmdName := editorExecutableName(editor)
	cmdArgs := buildEditorArgs(cmdName, resolved, oa.Line)
	fullArgs := editorCommandArgs(editor, cmdArgs)
	if len(fullArgs) == 0 {
		return nil, fmt.Errorf("open: invalid editor command %q", editor)
	}
	cmdStr := strings.Join(fullArgs, " ")

	// Terminal editors and unknown editors are returned as commands for the user
	// to run manually instead of being launched as arbitrary subprocesses.
	if terminalEditors[cmdName] || !guiLaunchEditors[cmdName] {
		return &Result{
			Title:  "Open with: " + cmdStr,
			Output: "Run: " + cmdStr,
			Metadata: map[string]any{
				"editor":  cmdName,
				"file":    resolved,
				"line":    oa.Line,
				"command": cmdStr,
			},
		}, nil
	}

	// GUI editors can be launched directly.
	cmd := exec.CommandContext(ctx, fullArgs[0], fullArgs[1:]...)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("open: failed to launch %s: %w", cmdName, err)
	}

	return &Result{
		Title:  "Opened " + resolved,
		Output: fmt.Sprintf("Opened %s in %s", resolved, cmdName),
		Metadata: map[string]any{
			"editor":  cmdName,
			"file":    resolved,
			"line":    oa.Line,
			"command": cmdStr,
		},
	}, nil
}

// resolveEditor determines which editor to use based on the provided value,
// the $EDITOR environment variable, or common fallbacks.
func resolveEditor(requested string) string {
	if requested != "" {
		return requested
	}
	if env := os.Getenv("EDITOR"); env != "" {
		return env
	}
	for _, candidate := range guiEditorFallbacks {
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func editorExecutableName(editor string) string {
	parts := splitQuoted(editor)
	if len(parts) == 0 {
		return filepath.Base(editor)
	}
	return filepath.Base(parts[0])
}

func editorCommandArgs(editor string, fileArgs []string) []string {
	parts := splitQuoted(editor)
	if len(parts) == 0 {
		return fileArgs
	}
	args := append([]string(nil), parts...)
	args = append(args, fileArgs...)
	return args
}

// buildEditorArgs returns the argument list for launching the given editor
// at the specified file and optional line number.
func buildEditorArgs(editor, filePath string, line int) []string {
	lineStr := strconv.Itoa(line)
	if line <= 0 {
		lineStr = "1"
	}

	switch editor {
	case "code", "code-insiders":
		return []string{"--goto", filePath + ":" + lineStr}
	case "vim", "nvim", "vi":
		return []string{"+" + lineStr, filePath}
	case "nano":
		return []string{"+" + lineStr, filePath}
	case "emacs":
		return []string{"+" + lineStr, filePath}
	case "subl":
		return []string{filePath + ":" + lineStr}
	default:
		return []string{filePath}
	}
}
