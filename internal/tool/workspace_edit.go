package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sageil/kodacode/v1/internal/lsp"
)

type workspaceEditNotify func(context.Context, []workspaceEditNotification) error
type workspaceEditVersionLookup func(string) (int, bool)

type workspaceEditNotificationKind string

const (
	workspaceEditNotifyChanged workspaceEditNotificationKind = "changed"
	workspaceEditNotifyCreated workspaceEditNotificationKind = "created"
	workspaceEditNotifyRenamed workspaceEditNotificationKind = "renamed"
	workspaceEditNotifyDeleted workspaceEditNotificationKind = "deleted"
)

type workspaceEditNotification struct {
	Kind    workspaceEditNotificationKind
	Path    string
	OldPath string
	NewPath string
}

type workspaceEditSummary struct {
	Paths     []string
	TextEdits int
	Created   int
	Renamed   int
	Deleted   int
}

type workspaceEditFailure struct {
	code      string
	retryable bool
	message   string
}

func (e *workspaceEditFailure) Error() string { return e.message }

func newWorkspaceEditFailure(code string, retryable bool, format string, args ...any) error {
	return &workspaceEditFailure{
		code:      code,
		retryable: retryable,
		message:   fmt.Sprintf(format, args...),
	}
}

func workspaceEditErrorResult(toolName string, err error) *Result {
	var workspaceErr *workspaceEditFailure
	if errors.As(err, &workspaceErr) {
		return ErrorResult(workspaceErr.code, toolName+": "+workspaceErr.message, workspaceErr.retryable)
	}
	if res := mutationPathErrorResult(toolName, err); res != nil {
		return res
	}
	code := ErrCodeInternal
	retryable := false
	switch {
	case strings.Contains(err.Error(), "outside the project directory"):
		code = ErrCodePermission
	case strings.Contains(err.Error(), "unsupported workspace edit"):
		code = ErrCodeInvalidArgs
	case strings.Contains(strings.ToLower(err.Error()), "sync failed"):
		code = ErrCodeUnavailable
		retryable = true
	case strings.Contains(strings.ToLower(err.Error()), "version mismatch"):
		code = ErrCodeConflict
		retryable = true
	case os.IsNotExist(err):
		code = ErrCodeNotFound
	case os.IsPermission(err):
		code = ErrCodePermission
	}
	return ErrorResult(code, toolName+": "+err.Error(), retryable)
}

type workspaceEditOperation struct {
	textDocument *workspaceTextDocumentEdit
	createFile   *workspaceCreateFile
	renameFile   *workspaceRenameFile
	deleteFile   *workspaceDeleteFile
}

type workspaceTextDocumentEdit struct {
	path    string
	version *int
	edits   []lsp.TextEdit
}

type workspaceCreateFile struct {
	path           string
	overwrite      bool
	ignoreIfExists bool
}

type workspaceRenameFile struct {
	oldPath        string
	newPath        string
	overwrite      bool
	ignoreIfExists bool
}

type workspaceDeleteFile struct {
	path              string
	recursive         bool
	ignoreIfNotExists bool
}

func ensurePathWithinWorkdir(path, workDir string) (string, error) {
	return canonicalPathWithinRoot(path, workDir)
}

func applyWorkspaceEdit(ctx context.Context, workDir string, edit *lsp.WorkspaceEdit, notify workspaceEditNotify, versionLookup workspaceEditVersionLookup) (workspaceEditSummary, error) {
	ops, err := workspaceEditOperations(edit, workDir)
	if err != nil {
		return workspaceEditSummary{}, err
	}
	var summary workspaceEditSummary
	seenPaths := make(map[string]bool)
	var pendingNotify []workspaceEditNotification
	seenNotify := make(map[string]bool)
	var rollbacks []func()
	var cleanups []func()
	rollbackAll := func() {
		for i := len(rollbacks) - 1; i >= 0; i-- {
			rollbacks[i]()
		}
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}
	abort := func(code string, retryable bool, format string, args ...any) (workspaceEditSummary, error) {
		rollbackAll()
		return workspaceEditSummary{}, newWorkspaceEditFailure(code, retryable, format, args...)
	}
	abortFromErr := func(prefix string, err error) (workspaceEditSummary, error) {
		code := ErrCodeInternal
		switch {
		case os.IsNotExist(err):
			code = ErrCodeNotFound
		case os.IsPermission(err):
			code = ErrCodePermission
		}
		return abort(code, false, "%s: %v", prefix, err)
	}
	collectNotify := func(event workspaceEditNotification) {
		if notify == nil {
			return
		}
		key := workspaceEditNotificationKey(event)
		if seenNotify[key] {
			return
		}
		seenNotify[key] = true
		pendingNotify = append(pendingNotify, event)
	}
	for _, op := range ops {
		switch {
		case op.textDocument != nil:
			path := op.textDocument.path
			if op.textDocument.version != nil {
				currentVersion, ok := versionLookup(path)
				if versionLookup == nil || !ok {
					return abort(ErrCodeUnavailable, true, "no tracked LSP document version available for %s", path)
				}
				if currentVersion != *op.textDocument.version {
					return abort(ErrCodeConflict, true, "LSP document version mismatch for %s: edit expects version %d, current version is %d", path, *op.textDocument.version, currentVersion)
				}
			}
			if _, err := os.Stat(path); err != nil {
				if os.IsNotExist(err) {
					return abort(ErrCodeNotFound, false, "file %q not found", path)
				}
				return abortFromErr(fmt.Sprintf("stat %s", path), err)
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return abortFromErr(fmt.Sprintf("read %s", path), err)
			}
			updated, applied, err := applyLSPTextEdits(string(content), op.textDocument.edits)
			if err != nil {
				return abort(ErrCodeInvalidArgs, false, "%s: %v", path, err)
			}
			info, err := os.Stat(path)
			if err != nil {
				return abortFromErr(fmt.Sprintf("stat %s", path), err)
			}
			origMode := info.Mode().Perm()
			if err := writeFileWithExistingMode(path, []byte(updated)); err != nil {
				rollbackAll()
				var pathErr *mutationPathError
				if errors.As(err, &pathErr) {
					return workspaceEditSummary{}, newWorkspaceEditFailure(pathErr.code, pathErr.retryable, "write %s: %s", path, pathErr.message)
				}
				code := ErrCodeInternal
				switch {
				case os.IsPermission(err):
					code = ErrCodePermission
				case os.IsNotExist(err):
					code = ErrCodeNotFound
				}
				return workspaceEditSummary{}, newWorkspaceEditFailure(code, false, "write %s: %v", path, err)
			}
			if newInfo, err := os.Stat(path); err == nil {
				storeSnapshot(path, newInfo, fileVersion([]byte(updated)), displayLineCount(updated))
			} else {
				invalidateSnapshot(path)
			}
			originalBytes := append([]byte(nil), content...)
			rollbacks = append(rollbacks, func() {
				_ = writeFileAtomic(path, originalBytes, origMode)
				if info, err := os.Stat(path); err == nil {
					storeSnapshot(path, info, fileVersion(originalBytes), displayLineCount(string(originalBytes)))
				} else {
					invalidateSnapshot(path)
				}
			})
			summary.TextEdits += applied
			if !seenPaths[path] {
				summary.Paths = append(summary.Paths, path)
				seenPaths[path] = true
			}
			collectNotify(workspaceEditNotification{Kind: workspaceEditNotifyChanged, Path: path})
		case op.createFile != nil:
			path := op.createFile.path
			if info, err := os.Stat(path); err == nil {
				if op.createFile.ignoreIfExists {
					continue
				}
				if !op.createFile.overwrite {
					return abort(ErrCodeInvalidArgs, false, "create file target already exists: %s", path)
				}
				if info.IsDir() {
					return abort(ErrCodeInvalidArgs, false, "create file target is a directory: %s", path)
				}
				backup, isDir, err := movePathToBackup(path)
				if err != nil {
					return abortFromErr(fmt.Sprintf("backup %s", path), err)
				}
				if isDir {
					return abort(ErrCodeInvalidArgs, false, "create file target is a directory: %s", path)
				}
				rollbacks = append(rollbacks, func() {
					_ = os.RemoveAll(path)
					_ = restoreBackup(backup, path)
					invalidateSnapshot(path)
				})
				cleanups = append(cleanups, func() { _ = os.RemoveAll(backup) })
				mode := info.Mode().Perm()
				if err := writeFileAtomic(path, nil, mode); err != nil {
					return abortFromErr(fmt.Sprintf("truncate %s", path), err)
				}
				if newInfo, err := os.Stat(path); err == nil {
					storeSnapshot(path, newInfo, fileVersion(nil), 0)
				}
			} else if !os.IsNotExist(err) {
				return abortFromErr(fmt.Sprintf("stat %s", path), err)
			} else {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					return abortFromErr(fmt.Sprintf("mkdir %s", filepath.Dir(path)), err)
				}
				if err := writeFileWithExistingMode(path, nil); err != nil {
					var pathErr *mutationPathError
					if errors.As(err, &pathErr) {
						return abort(pathErr.code, pathErr.retryable, "create %s: %s", path, pathErr.message)
					}
					return abortFromErr(fmt.Sprintf("create %s", path), err)
				}
				if newInfo, err := os.Stat(path); err == nil {
					storeSnapshot(path, newInfo, fileVersion(nil), 0)
				}
				rollbacks = append(rollbacks, func() {
					_ = os.RemoveAll(path)
					invalidateSnapshot(path)
				})
			}
			summary.Created++
			if !seenPaths[path] {
				summary.Paths = append(summary.Paths, path)
				seenPaths[path] = true
			}
			collectNotify(workspaceEditNotification{Kind: workspaceEditNotifyCreated, Path: path})
		case op.renameFile != nil:
			oldPath := op.renameFile.oldPath
			newPath := op.renameFile.newPath
			if _, err := os.Stat(oldPath); err != nil {
				if os.IsNotExist(err) {
					return abort(ErrCodeNotFound, false, "rename source not found: %s", oldPath)
				}
				return abortFromErr(fmt.Sprintf("stat %s", oldPath), err)
			}
			var destBackup string
			if _, err := os.Stat(newPath); err == nil {
				if op.renameFile.ignoreIfExists {
					continue
				}
				if !op.renameFile.overwrite {
					return abort(ErrCodeInvalidArgs, false, "rename destination already exists: %s", newPath)
				}
				var backupErr error
				destBackup, _, backupErr = movePathToBackup(newPath)
				if backupErr != nil {
					return abortFromErr(fmt.Sprintf("backup existing %s", newPath), backupErr)
				}
				cleanups = append(cleanups, func() { _ = os.RemoveAll(destBackup) })
				rollbacks = append(rollbacks, func() {
					_ = restoreBackup(destBackup, newPath)
					invalidateSnapshot(newPath)
				})
			} else if !os.IsNotExist(err) {
				return abortFromErr(fmt.Sprintf("stat %s", newPath), err)
			}
			if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
				return abortFromErr(fmt.Sprintf("mkdir %s", filepath.Dir(newPath)), err)
			}
			if err := os.Rename(oldPath, newPath); err != nil {
				if destBackup != "" {
					_ = restoreBackup(destBackup, newPath)
				}
				return abortFromErr(fmt.Sprintf("rename %s -> %s", oldPath, newPath), err)
			}
			invalidateSnapshot(oldPath)
			invalidateSnapshot(newPath)
			rollbacks = append(rollbacks, func() {
				_ = os.Rename(newPath, oldPath)
				if destBackup != "" {
					_ = restoreBackup(destBackup, newPath)
				}
				invalidateSnapshot(oldPath)
				invalidateSnapshot(newPath)
			})
			summary.Renamed++
			for _, p := range []string{oldPath, newPath} {
				if !seenPaths[p] {
					summary.Paths = append(summary.Paths, p)
					seenPaths[p] = true
				}
			}
			collectNotify(workspaceEditNotification{Kind: workspaceEditNotifyRenamed, OldPath: oldPath, NewPath: newPath})
		case op.deleteFile != nil:
			path := op.deleteFile.path
			info, err := os.Stat(path)
			if err != nil {
				if os.IsNotExist(err) && op.deleteFile.ignoreIfNotExists {
					continue
				}
				if os.IsNotExist(err) {
					return abort(ErrCodeNotFound, false, "delete target not found: %s", path)
				}
				return abortFromErr(fmt.Sprintf("stat %s", path), err)
			}
			if info.IsDir() && !op.deleteFile.recursive {
				return abort(ErrCodeInvalidArgs, false, "refusing to delete directory without recursive option: %s", path)
			}
			backup, _, err := movePathToBackup(path)
			if err != nil {
				return abortFromErr(fmt.Sprintf("delete %s", path), err)
			}
			invalidateSnapshot(path)
			rollbacks = append(rollbacks, func() {
				_ = restoreBackup(backup, path)
				invalidateSnapshot(path)
			})
			cleanups = append(cleanups, func() { _ = os.RemoveAll(backup) })
			summary.Deleted++
			if !seenPaths[path] {
				summary.Paths = append(summary.Paths, path)
				seenPaths[path] = true
			}
			collectNotify(workspaceEditNotification{Kind: workspaceEditNotifyDeleted, Path: path})
		}
	}
	if notify != nil && len(pendingNotify) > 0 {
		events := normalizeWorkspaceEditNotifications(pendingNotify)
		if err := notify(ctx, events); err != nil {
			rollbackAll()
			bestEffortResyncWorkspaceState(ctx, notify, summary.Paths)
			return workspaceEditSummary{}, newWorkspaceEditFailure(ErrCodeUnavailable, true, "workspace edit sync failed: %v", err)
		}
	}
	for i := len(cleanups) - 1; i >= 0; i-- {
		cleanups[i]()
	}
	sort.Strings(summary.Paths)
	return summary, nil
}

func workspaceEditOperations(edit *lsp.WorkspaceEdit, workDir string) ([]workspaceEditOperation, error) {
	if edit == nil {
		return nil, nil
	}
	ops := make([]workspaceEditOperation, 0, len(edit.Changes)+len(edit.DocumentChanges))
	if len(edit.DocumentChanges) > 0 {
		for _, raw := range edit.DocumentChanges {
			op, err := parseWorkspaceEditOperation(raw, workDir)
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
		path, err := ensurePathWithinWorkdir(lsp.URIToPath(uri), workDir)
		if err != nil {
			return nil, err
		}
		ops = append(ops, workspaceEditOperation{textDocument: &workspaceTextDocumentEdit{
			path:  path,
			edits: append([]lsp.TextEdit(nil), edit.Changes[uri]...),
		}})
	}
	return ops, nil
}

func parseWorkspaceEditOperation(raw json.RawMessage, workDir string) (workspaceEditOperation, error) {
	var textDocEdit lsp.TextDocumentEdit
	if err := json.Unmarshal(raw, &textDocEdit); err == nil && textDocEdit.TextDocument.URI != "" {
		path, err := ensurePathWithinWorkdir(lsp.URIToPath(textDocEdit.TextDocument.URI), workDir)
		if err != nil {
			return workspaceEditOperation{}, err
		}
		return workspaceEditOperation{textDocument: &workspaceTextDocumentEdit{path: path, version: textDocEdit.TextDocument.Version, edits: textDocEdit.Edits}}, nil
	}
	var kind struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &kind); err != nil {
		return workspaceEditOperation{}, newWorkspaceEditFailure(ErrCodeInvalidArgs, false, "unsupported workspace edit payload")
	}
	switch kind.Kind {
	case "create":
		var create lsp.CreateFile
		if err := json.Unmarshal(raw, &create); err != nil {
			return workspaceEditOperation{}, newWorkspaceEditFailure(ErrCodeInvalidArgs, false, "parse create file edit: %v", err)
		}
		path, err := ensurePathWithinWorkdir(lsp.URIToPath(create.URI), workDir)
		if err != nil {
			return workspaceEditOperation{}, err
		}
		var overwrite, ignoreIfExists bool
		if create.Options != nil {
			overwrite = create.Options.Overwrite
			ignoreIfExists = create.Options.IgnoreIfExists
		}
		return workspaceEditOperation{createFile: &workspaceCreateFile{path: path, overwrite: overwrite, ignoreIfExists: ignoreIfExists}}, nil
	case "rename":
		var rename lsp.RenameFile
		if err := json.Unmarshal(raw, &rename); err != nil {
			return workspaceEditOperation{}, newWorkspaceEditFailure(ErrCodeInvalidArgs, false, "parse rename file edit: %v", err)
		}
		oldPath, err := ensurePathWithinWorkdir(lsp.URIToPath(rename.OldURI), workDir)
		if err != nil {
			return workspaceEditOperation{}, err
		}
		newPath, err := ensurePathWithinWorkdir(lsp.URIToPath(rename.NewURI), workDir)
		if err != nil {
			return workspaceEditOperation{}, err
		}
		var overwrite, ignoreIfExists bool
		if rename.Options != nil {
			overwrite = rename.Options.Overwrite
			ignoreIfExists = rename.Options.IgnoreIfExists
		}
		return workspaceEditOperation{renameFile: &workspaceRenameFile{oldPath: oldPath, newPath: newPath, overwrite: overwrite, ignoreIfExists: ignoreIfExists}}, nil
	case "delete":
		var del lsp.DeleteFile
		if err := json.Unmarshal(raw, &del); err != nil {
			return workspaceEditOperation{}, newWorkspaceEditFailure(ErrCodeInvalidArgs, false, "parse delete file edit: %v", err)
		}
		path, err := ensurePathWithinWorkdir(lsp.URIToPath(del.URI), workDir)
		if err != nil {
			return workspaceEditOperation{}, err
		}
		var recursive, ignoreIfNotExists bool
		if del.Options != nil {
			recursive = del.Options.Recursive
			ignoreIfNotExists = del.Options.IgnoreIfNotExists
		}
		return workspaceEditOperation{deleteFile: &workspaceDeleteFile{path: path, recursive: recursive, ignoreIfNotExists: ignoreIfNotExists}}, nil
	default:
		return workspaceEditOperation{}, newWorkspaceEditFailure(ErrCodeInvalidArgs, false, "unsupported workspace edit operation %q", kind.Kind)
	}
}

func applyLSPTextEdits(content string, edits []lsp.TextEdit) (string, int, error) {
	if len(edits) == 0 {
		return content, 0, nil
	}
	type resolvedEdit struct {
		start   int
		end     int
		newText string
	}
	resolved := make([]resolvedEdit, 0, len(edits))
	for _, edit := range edits {
		start, end, err := byteOffsetsForLSPRange(content, edit.Range)
		if err != nil {
			return "", 0, err
		}
		resolved = append(resolved, resolvedEdit{start: start, end: end, newText: edit.NewText})
	}
	sort.Slice(resolved, func(i, j int) bool {
		if resolved[i].start == resolved[j].start {
			return resolved[i].end > resolved[j].end
		}
		return resolved[i].start > resolved[j].start
	})
	for i := 0; i < len(resolved)-1; i++ {
		if resolved[i].start < resolved[i+1].end {
			return "", 0, fmt.Errorf("overlapping text edits are not supported")
		}
	}
	current := content
	for _, edit := range resolved {
		current = current[:edit.start] + edit.newText + current[edit.end:]
	}
	return current, len(resolved), nil
}

func workspaceEditNotificationKey(event workspaceEditNotification) string {
	return string(event.Kind) + "|" + event.Path + "|" + event.OldPath + "|" + event.NewPath
}

func normalizeWorkspaceEditNotifications(events []workspaceEditNotification) []workspaceEditNotification {
	if len(events) == 0 {
		return nil
	}
	dropChanged := make(map[string]bool)
	dropCreated := make(map[string]bool)
	for _, event := range events {
		switch event.Kind {
		case workspaceEditNotifyCreated:
			dropChanged[event.Path] = true
		case workspaceEditNotifyRenamed:
			dropChanged[event.NewPath] = true
			dropCreated[event.NewPath] = true
		case workspaceEditNotifyDeleted:
			dropChanged[event.Path] = true
			dropCreated[event.Path] = true
		}
	}

	var out []workspaceEditNotification
	seen := make(map[string]bool)
	for _, event := range events {
		switch event.Kind {
		case workspaceEditNotifyChanged:
			if dropChanged[event.Path] {
				continue
			}
		case workspaceEditNotifyCreated:
			if dropCreated[event.Path] {
				continue
			}
		}
		key := workspaceEditNotificationKey(event)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, event)
	}
	return out
}

func bestEffortResyncWorkspaceState(ctx context.Context, notify workspaceEditNotify, paths []string) {
	if notify == nil || len(paths) == 0 {
		return
	}
	events := make([]workspaceEditNotification, 0, len(paths))
	seen := make(map[string]bool)
	for _, path := range paths {
		event := workspaceEditNotification{Kind: workspaceEditNotifyChanged, Path: path}
		if _, err := os.Stat(path); err != nil {
			if !os.IsNotExist(err) {
				continue
			}
			event = workspaceEditNotification{Kind: workspaceEditNotifyDeleted, Path: path}
		}
		key := workspaceEditNotificationKey(event)
		if seen[key] {
			continue
		}
		seen[key] = true
		events = append(events, event)
	}
	if len(events) == 0 {
		return
	}
	_ = notify(ctx, events)
}

func movePathToBackup(path string) (string, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", false, err
	}
	dir := filepath.Dir(path)
	var backup string
	if info.IsDir() {
		backup, err = os.MkdirTemp(dir, ".kodacode-backup-dir-*")
		if err != nil {
			return "", true, err
		}
		if err := os.Remove(backup); err != nil {
			return "", true, err
		}
	} else {
		f, err := os.CreateTemp(dir, ".kodacode-backup-file-*")
		if err != nil {
			return "", false, err
		}
		backup = f.Name()
		if err := f.Close(); err != nil {
			return "", false, err
		}
		if err := os.Remove(backup); err != nil {
			return "", false, err
		}
	}
	if err := os.Rename(path, backup); err != nil {
		return "", info.IsDir(), err
	}
	return backup, info.IsDir(), nil
}

func restoreBackup(backupPath, destPath string) error {
	if _, err := os.Stat(backupPath); err != nil {
		return err
	}
	_ = os.RemoveAll(destPath)
	return os.Rename(backupPath, destPath)
}

func newWorkspaceEditNotify(mgr *lsp.Manager) workspaceEditNotify {
	if mgr == nil {
		return nil
	}
	return func(ctx context.Context, events []workspaceEditNotification) error {
		for _, event := range events {
			switch event.Kind {
			case workspaceEditNotifyChanged, workspaceEditNotifyCreated:
				if err := mgr.SyncChanged(ctx, event.Path); err != nil {
					return err
				}
			case workspaceEditNotifyRenamed:
				if err := mgr.SyncRenamed(ctx, event.OldPath, event.NewPath); err != nil {
					return err
				}
			case workspaceEditNotifyDeleted:
				if err := mgr.SyncDeleted(ctx, event.Path); err != nil {
					return err
				}
			}
		}
		return nil
	}
}

func byteOffsetsForLSPRange(content string, rng lsp.Range) (int, int, error) {
	start, err := byteOffsetAtTextPosition(content, rng.Start.Line+1, rng.Start.Character)
	if err != nil {
		return 0, 0, err
	}
	end, err := byteOffsetAtTextPosition(content, rng.End.Line+1, rng.End.Character)
	if err != nil {
		return 0, 0, err
	}
	if end < start {
		return 0, 0, fmt.Errorf("invalid workspace edit range")
	}
	return start, end, nil
}
