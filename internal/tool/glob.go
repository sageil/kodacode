package tool

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

var globParams = []byte(`{
	"type": "object",
	"properties": {
		"pattern": {"type": "string", "description": "The glob pattern to match files against"},
		"path": {"type": "string", "description": "The directory to search in"}
	},
	"required": ["pattern"]
}`)

const globLimit = 100

// NewGlobTool returns a Tool that searches for files matching a glob pattern.
func NewGlobTool() *Tool {
	return &Tool{
		Name:        "glob",
		ReadOnly:    true,
		Description: prompt("glob"),
		Parameters:  globParams,
		Execute:     executeGlob,
	}
}

func executeGlob(ctx context.Context, ectx ExecutionContext, args []byte) (*Result, error) {
	var params struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := flexUnmarshal(args, &params); err != nil {
		return ErrorResult(ErrCodeInvalidArgs, fmt.Sprintf("glob: invalid arguments: %v", err), false), nil
	}
	if strings.TrimSpace(params.Pattern) == "" {
		return ErrorResult(ErrCodeInvalidArgs, "glob: pattern is required", false), nil
	}

	root := params.Path
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
	} else {
		root = resolvePath(root, ectx.WorkDir)
	}

	var files []fileInfo
	fsys := os.DirFS(root)
	scanned := 0
	lastProgress := 0
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Skip ignored directories early for performance.
		if d.IsDir() && ectx.IsIgnored(path+"/") {
			return fs.SkipDir
		}
		if ectx.IsIgnored(path) {
			return nil
		}

		scanned++
		// Report progress every 500 files so the UI stays responsive.
		if ectx.WriteOutput != nil && scanned-lastProgress >= 500 {
			lastProgress = scanned
			ectx.WriteOutput(fmt.Sprintf("scanning… %d files checked, %d matches\n", scanned, len(files)))
		}

		matched, matchErr := doublestar.Match(params.Pattern, path)
		if matchErr != nil {
			return nil // skip invalid pattern matches
		}
		if !matched {
			return nil
		}

		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		files = append(files, fileInfo{
			path:  filepath.Join(root, path),
			mtime: info.ModTime().UnixNano(),
			info:  info,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("glob walk error: %w", err)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].mtime > files[j].mtime
	})

	truncated := len(files) > globLimit
	if truncated {
		files = files[:globLimit]
	}

	var buf strings.Builder
	for _, f := range files {
		buf.WriteString(f.path)
		buf.WriteByte('\n')
	}

	output := strings.TrimRight(buf.String(), "\n")

	if truncated {
		output += fmt.Sprintf("\n\n(Results truncated. Showing %d of more than %d matches. Narrow your pattern.)", globLimit, globLimit)
	}

	if len(files) == 0 {
		output = "No files matched the pattern."
	}

	return &Result{
		Title:  fmt.Sprintf("glob: %s", params.Pattern),
		Output: output,
		Metadata: map[string]any{
			"count":     len(files),
			"truncated": truncated,
		},
	}, nil
}
