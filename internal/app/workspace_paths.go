package app

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

const maxWorkspacePathEntries = 10000

type WorkspacePath struct {
	Path  string
	IsDir bool
}

func ListWorkspacePaths(workspaceRoot string) ([]WorkspacePath, error) {
	root := strings.TrimSpace(workspaceRoot)
	if root == "" {
		return nil, nil
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	paths := make([]WorkspacePath, 0, 512)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if path == root {
			return nil
		}

		name := entry.Name()
		if entry.IsDir() && shouldSkipWorkspacePathDir(name) {
			return filepath.SkipDir
		}

		rel, err := filepath.Rel(root, path)
		if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if strings.TrimSpace(rel) == "" {
			return nil
		}
		paths = append(paths, WorkspacePath{Path: rel, IsDir: entry.IsDir()})
		if len(paths) >= maxWorkspacePathEntries {
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(paths, func(i, j int) bool {
		if paths[i].IsDir != paths[j].IsDir {
			return paths[i].IsDir
		}
		return paths[i].Path < paths[j].Path
	})
	return paths, nil
}

func shouldSkipWorkspacePathDir(name string) bool {
	switch name {
	case ".git", ".hg", ".svn", "node_modules", "vendor":
		return true
	default:
		return false
	}
}
