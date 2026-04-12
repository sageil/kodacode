package tool

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type mutationPathError struct {
	code      string
	message   string
	retryable bool
}

func (e *mutationPathError) Error() string { return e.message }

func newSymlinkMutationError(path string) error {
	return &mutationPathError{
		code:      ErrCodePermission,
		message:   fmt.Sprintf("refusing to modify symlink path %q; use the resolved target path instead", path),
		retryable: false,
	}
}

func mutationPathErrorResult(toolName string, err error) *Result {
	var pathErr *mutationPathError
	if !errors.As(err, &pathErr) {
		return nil
	}
	return ErrorResult(pathErr.code, toolName+": "+pathErr.message, pathErr.retryable)
}

func resolvePathAllowMissing(path string) string {
	candidate := filepath.Clean(path)
	if candidate == "" {
		return candidate
	}
	var suffix []string
	for {
		if resolved, err := filepath.EvalSymlinks(candidate); err == nil {
			return joinResolvedSuffix(resolved, suffix)
		}
		if info, err := os.Lstat(candidate); err == nil && info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(candidate)
			if err == nil {
				if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(candidate), target)
				}
				return joinResolvedSuffix(filepath.Clean(target), suffix)
			}
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return filepath.Clean(path)
		}
		suffix = append(suffix, filepath.Base(candidate))
		candidate = parent
	}
}

func joinResolvedSuffix(base string, suffix []string) string {
	resolved := base
	for i := len(suffix) - 1; i >= 0; i-- {
		resolved = filepath.Join(resolved, suffix[i])
	}
	return filepath.Clean(resolved)
}

func canonicalPathWithinRoot(path, root string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", path, err)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve workdir %q: %w", root, err)
	}
	resolvedPath := resolvePathAllowMissing(absPath)
	resolvedRoot := resolvePathAllowMissing(absRoot)
	rel, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil {
		return "", fmt.Errorf("compare %q to workdir: %w", resolvedPath, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("cannot modify files outside the project directory: %s", resolvedPath)
	}
	return resolvedPath, nil
}

func writeFileWithExistingMode(path string, content []byte) error {
	if err := rejectSymlinkMutationPath(path); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}
	return writeFileAtomic(path, content, mode)
}

func writeFileAtomic(path string, content []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".kodacode-write-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	if dirf, err := os.Open(dir); err == nil {
		_ = dirf.Sync()
		_ = dirf.Close()
	}
	return nil
}

func rejectSymlinkMutationPath(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return newSymlinkMutationError(path)
	}
	return nil
}
