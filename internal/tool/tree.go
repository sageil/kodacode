package tool

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

var treeParams = []byte(`{
	"type": "object",
	"properties": {
		"path": {"type": "string", "description": "Root directory to display (defaults to working directory)"},
		"depth": {"type": "integer", "description": "Maximum depth to traverse (default: 4)"},
		"include": {"type": "string", "description": "File pattern filter (e.g. '*.go', '*.ts')"},
		"showHidden": {"type": "boolean", "description": "Include hidden files/directories (default: false)"}
	}
}`)

const treeEntryLimit = 500

// NewTreeTool returns a Tool that displays directory tree structure.
func NewTreeTool() *Tool {
	return &Tool{
		Name:        "tree",
		ReadOnly:    true,
		Description: prompt("tree"),
		Parameters:  treeParams,
		Execute:     executeTree,
	}
}

type treeEntry struct {
	path  string // relative to root
	isDir bool
}

func executeTree(ctx context.Context, ectx ExecutionContext, args []byte) (*Result, error) {
	var params struct {
		Path       string `json:"path"`
		Depth      *int   `json:"depth"`
		Include    string `json:"include"`
		ShowHidden bool   `json:"showHidden"`
	}
	if err := flexUnmarshal(args, &params); err != nil {
		return nil, err
	}

	root := params.Path
	if root == "" {
		root = ectx.WorkDir
		if root == "" {
			wd, err := os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("failed to get working directory: %w", err)
			}
			root = wd
		}
	} else {
		root = resolvePath(root, ectx.WorkDir)
	}

	maxDepth := 4
	if params.Depth != nil {
		maxDepth = *params.Depth
	}

	isProjectRoot := root == ectx.WorkDir || (ectx.WorkDir != "" && root == ectx.WorkDir+"/")
	if isProjectRoot && maxDepth <= 2 && params.Include == "" {
		return &Result{
			Title:  "tree",
			Output: "The project structure is already available in your system prompt. Use that instead of calling tree on the project root. Call tree only for subdirectories you need to explore deeper.",
		}, nil
	}

	isGit := false
	if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
		isGit = true
	}

	var entries []treeEntry
	entryCount := 0
	scanned := 0
	lastProgress := 0

	err := fs.WalkDir(os.DirFS(root), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if path == "." {
			return nil
		}

		scanned++
		if ectx.WriteOutput != nil && scanned-lastProgress >= 500 {
			lastProgress = scanned
			ectx.WriteOutput(fmt.Sprintf("scanning… %d entries\n", scanned))
		}

		name := d.Name()
		depth := strings.Count(path, string(os.PathSeparator)) + 1

		// Skip hidden files/dirs unless requested.
		if !params.ShowHidden && strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		// Skip ignored directories from config.
		if d.IsDir() && ectx.IsIgnored(path+"/") {
			return fs.SkipDir
		}
		if ectx.IsIgnored(path) {
			return nil
		}

		// Enforce depth limit.
		if depth > maxDepth {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		// Enforce entry limit.
		if entryCount >= treeEntryLimit {
			return fs.SkipAll
		}

		entries = append(entries, treeEntry{path: path, isDir: d.IsDir()})
		entryCount++
		return nil
	})
	if err != nil && err != fs.SkipAll {
		return nil, fmt.Errorf("tree walk error: %w", err)
	}

	if isGit && len(entries) > 0 {
		entries = filterGitIgnored(ctx, root, entries)
	}

	if params.Include != "" {
		entries = filterInclude(entries, params.Include)
	}

	output, dirCount, fileCount := renderTree(root, entries)

	tr := TruncateWithBudget(output, "head", ectx.ContextUsage)
	return &Result{
		Title:  fmt.Sprintf("tree: %s", root),
		Output: tr.Content,
		Metadata: map[string]any{
			"directories": dirCount,
			"files":       fileCount,
		},
	}, nil
}

// filterGitIgnored uses git check-ignore to remove ignored paths.
func filterGitIgnored(ctx context.Context, root string, entries []treeEntry) []treeEntry {
	var input bytes.Buffer
	for _, e := range entries {
		input.WriteString(e.path)
		input.WriteByte('\n')
	}

	cmd := exec.CommandContext(ctx, "git", "check-ignore", "--stdin")
	cmd.Dir = root
	cmd.Stdin = &input
	out, _ := cmd.Output() // exit code 1 means no matches, which is fine

	ignored := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			ignored[line] = true
		}
	}

	var filtered []treeEntry
	for _, e := range entries {
		if ignored[e.path] {
			continue
		}
		// Also skip entries whose parent directory was ignored.
		skip := false
		for ig := range ignored {
			if strings.HasPrefix(e.path, ig+"/") {
				skip = true
				break
			}
		}
		if !skip {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// filterInclude keeps only files matching the pattern and directories containing matches.
func filterInclude(entries []treeEntry, pattern string) []treeEntry {
	matchingFiles := make(map[string]bool)
	neededDirs := make(map[string]bool)

	for _, e := range entries {
		if e.isDir {
			continue
		}
		matched, err := filepath.Match(pattern, filepath.Base(e.path))
		if err != nil || !matched {
			continue
		}
		matchingFiles[e.path] = true
		// Mark all ancestor directories as needed.
		dir := filepath.Dir(e.path)
		for dir != "." && dir != "" {
			neededDirs[dir] = true
			dir = filepath.Dir(dir)
		}
	}

	var filtered []treeEntry
	for _, e := range entries {
		if e.isDir {
			if neededDirs[e.path] {
				filtered = append(filtered, e)
			}
		} else {
			if matchingFiles[e.path] {
				filtered = append(filtered, e)
			}
		}
	}
	return filtered
}

// renderTree builds the visual tree output.
func renderTree(root string, entries []treeEntry) (string, int, int) {
	if len(entries) == 0 {
		return fmt.Sprintf("%s/\n\n0 directories, 0 files", filepath.Base(root)), 0, 0
	}

	type node struct {
		name  string
		path  string
		isDir bool
	}
	children := make(map[string][]node)

	for _, e := range entries {
		parent := filepath.Dir(e.path)
		if parent == "." {
			parent = ""
		}
		children[parent] = append(children[parent], node{
			name:  filepath.Base(e.path),
			path:  e.path,
			isDir: e.isDir,
		})
	}

	for k := range children {
		sort.Slice(children[k], func(i, j int) bool {
			// Directories first, then alphabetical.
			ci, cj := children[k][i], children[k][j]
			if ci.isDir != cj.isDir {
				return ci.isDir
			}
			return ci.name < cj.name
		})
	}

	var buf strings.Builder
	buf.WriteString(filepath.Base(root))
	buf.WriteString("/\n")

	dirCount := 0
	fileCount := 0

	var walk func(parentKey, prefix string)
	walk = func(parentKey, prefix string) {
		nodes := children[parentKey]
		for i, n := range nodes {
			isLast := i == len(nodes)-1
			connector := "├── "
			if isLast {
				connector = "└── "
			}
			suffix := ""
			if n.isDir {
				suffix = "/"
				dirCount++
			} else {
				fileCount++
			}
			buf.WriteString(prefix)
			buf.WriteString(connector)
			buf.WriteString(n.name)
			buf.WriteString(suffix)
			buf.WriteByte('\n')

			if n.isDir {
				childPrefix := prefix + "│   "
				if isLast {
					childPrefix = prefix + "    "
				}
				walk(n.path, childPrefix)
			}
		}
	}

	walk("", "")
	fmt.Fprintf(&buf, "\n%d directories, %d files", dirCount, fileCount)

	return buf.String(), dirCount, fileCount
}
