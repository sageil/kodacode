package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/lsp"
	"github.com/sageil/kodacode/internal/tool"
)

type codeIntelVersionLookup func(string) (int, bool)

type codeIntelWorkspaceEditOperation struct {
	textDocument *codeIntelTextDocumentEdit
	createFile   *codeIntelCreateFile
	renameFile   *codeIntelRenameFile
	deleteFile   *codeIntelDeleteFile
}

type codeIntelTextDocumentEdit struct {
	path    string
	version *int
	edits   []lsp.TextEdit
}

type codeIntelCreateFile struct {
	path           string
	overwrite      bool
	ignoreIfExists bool
}

type codeIntelRenameFile struct {
	oldPath        string
	newPath        string
	overwrite      bool
	ignoreIfExists bool
}

type codeIntelDeleteFile struct {
	path              string
	recursive         bool
	ignoreIfNotExists bool
}

func applyCodeIntelWorkspaceEdit(roots []string, edit *lsp.WorkspaceEdit, versionLookup codeIntelVersionLookup) (tool.CodeIntelMutationSummary, codeIntelMutationSyncPlan, error) {
	ops, err := codeIntelWorkspaceEditOperations(roots, edit)
	if err != nil {
		return tool.CodeIntelMutationSummary{}, codeIntelMutationSyncPlan{}, err
	}
	var (
		summary     tool.CodeIntelMutationSummary
		plan        codeIntelMutationSyncPlan
		seenPath    = make(map[string]bool)
		seenChanged = make(map[string]bool)
		seenDeleted = make(map[string]bool)
		seenRenamed = make(map[string]bool)
		rollback    []func()
		cleanup     []func()
	)
	fail := func(format string, args ...any) (tool.CodeIntelMutationSummary, codeIntelMutationSyncPlan, error) {
		for i := len(rollback) - 1; i >= 0; i-- {
			rollback[i]()
		}
		for i := len(cleanup) - 1; i >= 0; i-- {
			cleanup[i]()
		}
		return tool.CodeIntelMutationSummary{}, codeIntelMutationSyncPlan{}, fmt.Errorf(format, args...)
	}
	addPath := func(path string) {
		if path == "" || seenPath[path] {
			return
		}
		seenPath[path] = true
		summary.Paths = append(summary.Paths, path)
	}
	addChanged := func(path string) {
		if path == "" || seenChanged[path] {
			return
		}
		seenChanged[path] = true
		plan.Changed = append(plan.Changed, path)
	}
	addDeleted := func(path string) {
		if path == "" || seenDeleted[path] {
			return
		}
		seenDeleted[path] = true
		plan.Deleted = append(plan.Deleted, path)
	}
	addRenamed := func(oldPath, newPath string) {
		key := oldPath + "\x00" + newPath
		if oldPath == "" || newPath == "" || seenRenamed[key] {
			return
		}
		seenRenamed[key] = true
		plan.Renamed = append(plan.Renamed, codeIntelRename{OldPath: oldPath, NewPath: newPath})
	}

	for _, op := range ops {
		switch {
		case op.textDocument != nil:
			path := op.textDocument.path
			if err := tool.WithFileMutationLock(path, func() error {
				if op.textDocument.version != nil {
					if versionLookup == nil {
						return fmt.Errorf("no tracked LSP document version available for %s", path)
					}
					current, ok := versionLookup(path)
					if !ok {
						return fmt.Errorf("no tracked LSP document version available for %s", path)
					}
					if current != *op.textDocument.version {
						return fmt.Errorf("LSP document version mismatch for %s: edit expects version %d, current version is %d", path, *op.textDocument.version, current)
					}
				}
				original, err := os.ReadFile(path)
				if err != nil {
					return fmt.Errorf("read %s: %v", path, err)
				}
				updated, applied, err := applyCodeIntelTextEdits(string(original), op.textDocument.edits)
				if err != nil {
					return fmt.Errorf("%s: %v", path, err)
				}
				if applied == 0 || updated == string(original) {
					return nil
				}
				mode := existingFileMode(path, 0o644)
				if err := tool.WriteFileAtomically(path, []byte(updated), mode); err != nil {
					return fmt.Errorf("write %s: %v", path, err)
				}
				orig := append([]byte(nil), original...)
				rollback = append(rollback, func() {
					_ = tool.WithFileMutationLock(path, func() error {
						return tool.WriteFileAtomically(path, orig, mode)
					})
				})
				addPath(path)
				summary.TextEdits += applied
				addChanged(path)
				return nil
			}); err != nil {
				return fail("%v", err)
			}
		case op.createFile != nil:
			path := op.createFile.path
			mode := existingFileMode(path, 0o644)
			if info, err := os.Stat(path); err == nil {
				if op.createFile.ignoreIfExists {
					continue
				}
				if !op.createFile.overwrite {
					return fail("create file target already exists: %s", path)
				}
				if info.IsDir() {
					return fail("create file target is a directory: %s", path)
				}
				backup, isDir, err := moveCodeIntelPathToBackup(path)
				if err != nil {
					return fail("backup %s: %v", path, err)
				}
				if isDir {
					return fail("create file target is a directory: %s", path)
				}
				cleanup = append(cleanup, func() { _ = os.RemoveAll(backup) })
				rollback = append(rollback, func() {
					_ = os.RemoveAll(path)
					_ = restoreCodeIntelBackup(backup, path)
				})
			} else if !os.IsNotExist(err) {
				return fail("stat %s: %v", path, err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return fail("mkdir %s: %v", filepath.Dir(path), err)
			}
			if err := tool.WithFileMutationLock(path, func() error {
				return tool.WriteFileAtomically(path, nil, mode)
			}); err != nil {
				return fail("create %s: %v", path, err)
			}
			rollback = append(rollback, func() { _ = os.RemoveAll(path) })
			addPath(path)
			summary.Created++
			addChanged(path)
		case op.renameFile != nil:
			oldPath := op.renameFile.oldPath
			newPath := op.renameFile.newPath
			if _, err := os.Stat(oldPath); err != nil {
				return fail("rename source %s: %v", oldPath, err)
			}
			var destBackup string
			if _, err := os.Stat(newPath); err == nil {
				if op.renameFile.ignoreIfExists {
					continue
				}
				if !op.renameFile.overwrite {
					return fail("rename destination already exists: %s", newPath)
				}
				backup, _, err := moveCodeIntelPathToBackup(newPath)
				if err != nil {
					return fail("backup existing %s: %v", newPath, err)
				}
				destBackup = backup
				cleanup = append(cleanup, func() { _ = os.RemoveAll(destBackup) })
				rollback = append(rollback, func() {
					_ = restoreCodeIntelBackup(destBackup, newPath)
				})
			} else if !os.IsNotExist(err) {
				return fail("stat %s: %v", newPath, err)
			}
			if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
				return fail("mkdir %s: %v", filepath.Dir(newPath), err)
			}
			if err := os.Rename(oldPath, newPath); err != nil {
				return fail("rename %s -> %s: %v", oldPath, newPath, err)
			}
			rollback = append(rollback, func() {
				_ = os.Rename(newPath, oldPath)
				if destBackup != "" {
					_ = restoreCodeIntelBackup(destBackup, newPath)
				}
			})
			addPath(oldPath)
			addPath(newPath)
			summary.Renamed++
			addRenamed(oldPath, newPath)
		case op.deleteFile != nil:
			path := op.deleteFile.path
			info, err := os.Stat(path)
			if err != nil {
				if os.IsNotExist(err) && op.deleteFile.ignoreIfNotExists {
					continue
				}
				return fail("delete %s: %v", path, err)
			}
			if info.IsDir() && !op.deleteFile.recursive {
				return fail("refusing to delete directory without recursive option: %s", path)
			}
			backup, _, err := moveCodeIntelPathToBackup(path)
			if err != nil {
				return fail("delete %s: %v", path, err)
			}
			rollback = append(rollback, func() { _ = restoreCodeIntelBackup(backup, path) })
			cleanup = append(cleanup, func() { _ = os.RemoveAll(backup) })
			addPath(path)
			summary.Deleted++
			addDeleted(path)
		}
	}

	sort.Strings(summary.Paths)
	for i := len(cleanup) - 1; i >= 0; i-- {
		cleanup[i]()
	}
	return summary, plan, nil
}

func codeIntelWorkspaceEditOperations(roots []string, edit *lsp.WorkspaceEdit) ([]codeIntelWorkspaceEditOperation, error) {
	if edit == nil {
		return nil, nil
	}
	ops := make([]codeIntelWorkspaceEditOperation, 0, len(edit.Changes)+len(edit.DocumentChanges))
	if len(edit.DocumentChanges) > 0 {
		for _, raw := range edit.DocumentChanges {
			op, err := parseCodeIntelWorkspaceEditOperation(roots, raw)
			if err != nil {
				return nil, err
			}
			ops = append(ops, op)
		}
		return ops, nil
	}
	uris := make([]string, 0, len(edit.Changes))
	for uri := range edit.Changes {
		uris = append(uris, uri)
	}
	sort.Strings(uris)
	for _, uri := range uris {
		path, err := codeIntelWorkspacePath(roots, lsp.URIToPath(uri))
		if err != nil {
			return nil, err
		}
		ops = append(ops, codeIntelWorkspaceEditOperation{textDocument: &codeIntelTextDocumentEdit{
			path:  path,
			edits: append([]lsp.TextEdit(nil), edit.Changes[uri]...),
		}})
	}
	return ops, nil
}

func parseCodeIntelWorkspaceEditOperation(roots []string, raw json.RawMessage) (codeIntelWorkspaceEditOperation, error) {
	var textDocumentEdit lsp.TextDocumentEdit
	if err := json.Unmarshal(raw, &textDocumentEdit); err == nil && strings.TrimSpace(textDocumentEdit.TextDocument.URI) != "" {
		path, err := codeIntelWorkspacePath(roots, lsp.URIToPath(textDocumentEdit.TextDocument.URI))
		if err != nil {
			return codeIntelWorkspaceEditOperation{}, err
		}
		return codeIntelWorkspaceEditOperation{textDocument: &codeIntelTextDocumentEdit{
			path:    path,
			version: textDocumentEdit.TextDocument.Version,
			edits:   append([]lsp.TextEdit(nil), textDocumentEdit.Edits...),
		}}, nil
	}
	var kind struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &kind); err != nil {
		return codeIntelWorkspaceEditOperation{}, fmt.Errorf("unsupported workspace edit payload")
	}
	switch kind.Kind {
	case "create":
		var create lsp.CreateFile
		if err := json.Unmarshal(raw, &create); err != nil {
			return codeIntelWorkspaceEditOperation{}, fmt.Errorf("parse create file edit: %v", err)
		}
		path, err := codeIntelWorkspacePath(roots, lsp.URIToPath(create.URI))
		if err != nil {
			return codeIntelWorkspaceEditOperation{}, err
		}
		var overwrite, ignoreIfExists bool
		if create.Options != nil {
			overwrite = create.Options.Overwrite
			ignoreIfExists = create.Options.IgnoreIfExists
		}
		return codeIntelWorkspaceEditOperation{createFile: &codeIntelCreateFile{path: path, overwrite: overwrite, ignoreIfExists: ignoreIfExists}}, nil
	case "rename":
		var rename lsp.RenameFile
		if err := json.Unmarshal(raw, &rename); err != nil {
			return codeIntelWorkspaceEditOperation{}, fmt.Errorf("parse rename file edit: %v", err)
		}
		oldPath, err := codeIntelWorkspacePath(roots, lsp.URIToPath(rename.OldURI))
		if err != nil {
			return codeIntelWorkspaceEditOperation{}, err
		}
		newPath, err := codeIntelWorkspacePath(roots, lsp.URIToPath(rename.NewURI))
		if err != nil {
			return codeIntelWorkspaceEditOperation{}, err
		}
		var overwrite, ignoreIfExists bool
		if rename.Options != nil {
			overwrite = rename.Options.Overwrite
			ignoreIfExists = rename.Options.IgnoreIfExists
		}
		return codeIntelWorkspaceEditOperation{renameFile: &codeIntelRenameFile{oldPath: oldPath, newPath: newPath, overwrite: overwrite, ignoreIfExists: ignoreIfExists}}, nil
	case "delete":
		var remove lsp.DeleteFile
		if err := json.Unmarshal(raw, &remove); err != nil {
			return codeIntelWorkspaceEditOperation{}, fmt.Errorf("parse delete file edit: %v", err)
		}
		path, err := codeIntelWorkspacePath(roots, lsp.URIToPath(remove.URI))
		if err != nil {
			return codeIntelWorkspaceEditOperation{}, err
		}
		var recursive, ignoreIfNotExists bool
		if remove.Options != nil {
			recursive = remove.Options.Recursive
			ignoreIfNotExists = remove.Options.IgnoreIfNotExists
		}
		return codeIntelWorkspaceEditOperation{deleteFile: &codeIntelDeleteFile{path: path, recursive: recursive, ignoreIfNotExists: ignoreIfNotExists}}, nil
	default:
		return codeIntelWorkspaceEditOperation{}, fmt.Errorf("unsupported workspace edit operation %q", kind.Kind)
	}
}

func codeIntelWorkspacePath(roots []string, path string) (string, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return "", fmt.Errorf("workspace edit path is empty")
	}
	if bestCodeIntelRoot(roots, path) == "" {
		return "", fmt.Errorf("workspace edit path %q is outside the configured workspace roots", path)
	}
	return path, nil
}
