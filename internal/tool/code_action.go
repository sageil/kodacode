package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sageil/kodacode/v1/internal/lsp"
)

var codeActionParams = json.RawMessage(`{
	"type": "object",
	"properties": {
		"filePath": {"type": "string", "description": "The absolute path to the source file"},
		"startLine": {"type": "integer", "description": "1-based start line for the code action range"},
		"startCharacter": {"type": "integer", "description": "0-based start character for the code action range"},
		"endLine": {"type": "integer", "description": "1-based end line for the code action range"},
		"endCharacter": {"type": "integer", "description": "0-based end character for the code action range"},
		"title": {"type": "string", "description": "Optional exact code action title to apply"},
		"kind": {"type": "string", "description": "Optional code action kind filter, such as source.organizeImports or quickfix"},
		"onlyPreferred": {"type": "boolean", "description": "Optional: prefer only actions marked as preferred"}
	},
	"required": ["filePath", "startLine", "startCharacter", "endLine", "endCharacter"]
}`)

type codeActionLookup func(ctx context.Context, ext, rootURI string) (codeActionServer, error)

type codeActionServer interface {
	CodeActions(ctx context.Context, filePath string, rng lsp.Range, only []string) ([]lsp.CodeAction, error)
}

func NewCodeActionTool(mgr *lsp.Manager) *Tool {
	lookup := func(ctx context.Context, ext, rootURI string) (codeActionServer, error) {
		return mgr.ServerFor(ctx, ext, rootURI)
	}
	return newCodeActionTool(lookup, newWorkspaceEditNotify(mgr), mgr.DocumentVersion)
}

func newCodeActionTool(lookup codeActionLookup, notify workspaceEditNotify, versionLookup workspaceEditVersionLookup) *Tool {
	return &Tool{
		Name:        "code_action",
		Description: prompt("code_action"),
		Parameters:  codeActionParams,
		Execute: func(ctx context.Context, ectx ExecutionContext, args []byte) (*Result, error) {
			return executeCodeAction(ctx, ectx, args, lookup, notify, versionLookup)
		},
	}
}

func executeCodeAction(ctx context.Context, ectx ExecutionContext, args []byte, lookup codeActionLookup, notify workspaceEditNotify, versionLookup workspaceEditVersionLookup) (*Result, error) {
	args = normalizeFilePathField(args)
	var params struct {
		FilePath       string `json:"filePath"`
		StartLine      int    `json:"startLine"`
		StartCharacter int    `json:"startCharacter"`
		EndLine        int    `json:"endLine"`
		EndCharacter   int    `json:"endCharacter"`
		Title          string `json:"title"`
		Kind           string `json:"kind"`
		OnlyPreferred  bool   `json:"onlyPreferred"`
	}
	if err := flexUnmarshal(args, &params); err != nil {
		return ErrorResult(ErrCodeInvalidArgs, fmt.Sprintf("code_action: invalid arguments: %v", err), false), nil
	}
	if params.StartLine < 1 || params.EndLine < 1 {
		return ErrorResult(ErrCodeInvalidArgs, "code_action: startLine and endLine must be >= 1", false), nil
	}
	if params.StartCharacter < 0 || params.EndCharacter < 0 {
		return ErrorResult(ErrCodeInvalidArgs, "code_action: startCharacter and endCharacter must be >= 0", false), nil
	}
	path := resolvePath(params.FilePath, ectx.WorkDir)
	ext := strings.ToLower(filepath.Ext(path))
	rootURI := lsp.FileURI(ectx.WorkDir)
	server, err := lookup(ctx, ext, rootURI)
	if err != nil {
		return nil, err
	}
	rng := lsp.Range{
		Start: lsp.Position{Line: params.StartLine - 1, Character: params.StartCharacter},
		End:   lsp.Position{Line: params.EndLine - 1, Character: params.EndCharacter},
	}
	var only []string
	if strings.TrimSpace(params.Kind) != "" {
		only = []string{strings.TrimSpace(params.Kind)}
	}
	actions, err := server.CodeActions(ctx, path, rng, only)
	if err != nil {
		return nil, err
	}
	selected, err := selectCodeAction(actions, params.Title, params.Kind, params.OnlyPreferred)
	if err != nil {
		return ErrorResult(ErrCodeInvalidArgs, "code_action: "+err.Error(), true), nil
	}
	if selected.Edit == nil {
		if selected.Command != nil {
			return ErrorResult(ErrCodeInvalidArgs, fmt.Sprintf("code_action: %q requires server-side executeCommand, which is not supported yet", selected.Title), false), nil
		}
		return ErrorResult(ErrCodeInvalidArgs, fmt.Sprintf("code_action: %q produced no edits", selected.Title), false), nil
	}
	summary, err := applyWorkspaceEdit(ctx, ectx.WorkDir, selected.Edit, notify, versionLookup)
	if err != nil {
		return workspaceEditErrorResult("code_action", err), nil
	}
	if len(summary.Paths) == 0 {
		return &Result{
			Title:  path,
			Output: fmt.Sprintf("Code action %q produced no file changes.", selected.Title),
			Metadata: map[string]any{
				"filepath": path,
				"changed":  false,
			},
		}, nil
	}
	output := fmt.Sprintf("Applied code action %q.\nFiles: %d, text edits: %d", selected.Title, len(summary.Paths), summary.TextEdits)
	if summary.Created > 0 || summary.Renamed > 0 || summary.Deleted > 0 {
		output += fmt.Sprintf(", created: %d, renamed: %d, deleted: %d", summary.Created, summary.Renamed, summary.Deleted)
	}
	output += "\n" + strings.Join(summary.Paths, "\n")
	return &Result{
		Title:  path,
		Output: output,
		Metadata: map[string]any{
			"filepath": path,
			"files":    len(summary.Paths),
			"edits":    summary.TextEdits,
			"created":  summary.Created,
			"renamed":  summary.Renamed,
			"deleted":  summary.Deleted,
			"changed":  true,
		},
	}, nil
}

func selectCodeAction(actions []lsp.CodeAction, title, kind string, onlyPreferred bool) (*lsp.CodeAction, error) {
	if len(actions) == 0 {
		return nil, fmt.Errorf("no code actions available")
	}
	var matches []lsp.CodeAction
	for _, action := range actions {
		if action.Disabled != nil {
			continue
		}
		if onlyPreferred && !action.IsPreferred {
			continue
		}
		if title != "" && !strings.EqualFold(strings.TrimSpace(action.Title), strings.TrimSpace(title)) {
			continue
		}
		if kind != "" {
			k := strings.TrimSpace(kind)
			if action.Kind != k && !strings.HasPrefix(action.Kind, k+".") {
				continue
			}
		}
		matches = append(matches, action)
	}
	if len(matches) == 0 {
		var available []string
		for _, action := range actions {
			if action.Disabled != nil {
				continue
			}
			label := action.Title
			if action.Kind != "" {
				label += " [" + action.Kind + "]"
			}
			if action.IsPreferred {
				label += " preferred"
			}
			available = append(available, label)
		}
		if len(available) == 0 {
			return nil, fmt.Errorf("no enabled code actions matched")
		}
		return nil, fmt.Errorf("no enabled code actions matched. Available actions: %s", strings.Join(available, "; "))
	}
	if len(matches) > 1 && title == "" && kind == "" {
		var available []string
		for _, action := range matches {
			label := action.Title
			if action.Kind != "" {
				label += " [" + action.Kind + "]"
			}
			if action.IsPreferred {
				label += " preferred"
			}
			available = append(available, label)
		}
		return nil, fmt.Errorf("multiple code actions matched. Specify title or kind. Available actions: %s", strings.Join(available, "; "))
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].IsPreferred != matches[j].IsPreferred {
			return matches[i].IsPreferred
		}
		return matches[i].Title < matches[j].Title
	})
	return &matches[0], nil
}
