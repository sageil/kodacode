package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/v1/internal/lsp"
)

var renameSymbolParams = json.RawMessage(`{
	"type": "object",
	"properties": {
		"filePath": {"type": "string", "description": "The absolute path to the source file containing the symbol reference"},
		"line": {"type": "integer", "description": "1-based line number of the symbol reference to rename"},
		"character": {"type": "integer", "description": "0-based character offset of the symbol reference to rename"},
		"newName": {"type": "string", "description": "The new symbol name"}
	},
	"required": ["filePath", "line", "character", "newName"]
}`)

type renameLookup func(ctx context.Context, ext, rootURI string) (renameServer, error)

type renameServer interface {
	Rename(ctx context.Context, filePath string, line, character int, newName string) (*lsp.WorkspaceEdit, error)
}

func NewRenameSymbolTool(mgr *lsp.Manager) *Tool {
	lookup := func(ctx context.Context, ext, rootURI string) (renameServer, error) {
		return mgr.ServerFor(ctx, ext, rootURI)
	}
	return newRenameSymbolTool(lookup, newWorkspaceEditNotify(mgr), mgr.DocumentVersion)
}

func newRenameSymbolTool(lookup renameLookup, notify workspaceEditNotify, versionLookup workspaceEditVersionLookup) *Tool {
	return &Tool{
		Name:        "rename_symbol",
		Description: prompt("rename_symbol"),
		Parameters:  renameSymbolParams,
		Execute: func(ctx context.Context, ectx ExecutionContext, args []byte) (*Result, error) {
			return executeRenameSymbol(ctx, ectx, args, lookup, notify, versionLookup)
		},
	}
}

func executeRenameSymbol(ctx context.Context, ectx ExecutionContext, args []byte, lookup renameLookup, notify workspaceEditNotify, versionLookup workspaceEditVersionLookup) (*Result, error) {
	args = normalizeFilePathField(args)
	var params struct {
		FilePath  string `json:"filePath"`
		Line      int    `json:"line"`
		Character int    `json:"character"`
		NewName   string `json:"newName"`
	}
	if err := flexUnmarshal(args, &params); err != nil {
		return ErrorResult(ErrCodeInvalidArgs, fmt.Sprintf("rename_symbol: invalid arguments: %v", err), false), nil
	}
	if params.Line < 1 {
		return ErrorResult(ErrCodeInvalidArgs, "rename_symbol: line must be >= 1", false), nil
	}
	if params.Character < 0 {
		return ErrorResult(ErrCodeInvalidArgs, "rename_symbol: character must be >= 0", false), nil
	}
	if strings.TrimSpace(params.NewName) == "" {
		return ErrorResult(ErrCodeInvalidArgs, "rename_symbol: newName is required", false), nil
	}

	path := resolvePath(params.FilePath, ectx.WorkDir)
	ext := strings.ToLower(filepath.Ext(path))
	rootURI := lsp.FileURI(ectx.WorkDir)
	server, err := lookup(ctx, ext, rootURI)
	if err != nil {
		return nil, err
	}
	edit, err := server.Rename(ctx, path, params.Line-1, params.Character, params.NewName)
	if err != nil {
		return nil, err
	}
	summary, err := applyWorkspaceEdit(ctx, ectx.WorkDir, edit, notify, versionLookup)
	if err != nil {
		return workspaceEditErrorResult("rename_symbol", err), nil
	}
	if len(summary.Paths) == 0 {
		return &Result{
			Title:  path,
			Output: "No rename edits were produced.",
			Metadata: map[string]any{
				"filepath": path,
				"changed":  false,
			},
		}, nil
	}

	return &Result{
		Title:  path,
		Output: fmt.Sprintf("Renamed symbol to %q across %d file(s) with %d text edit(s).\n%s", params.NewName, len(summary.Paths), summary.TextEdits, strings.Join(summary.Paths, "\n")),
		Metadata: map[string]any{
			"filepath": path,
			"files":    len(summary.Paths),
			"edits":    summary.TextEdits,
			"changed":  true,
		},
	}, nil
}
