package tool

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

var bulkReadParams = []byte(`{
	"type": "object",
	"properties": {
		"pattern": {"type": "string", "description": "Glob pattern to match files (e.g. \"**/*.go\", \"internal/tool/*.go\")"},
		"files": {"type": "array", "items": {"type": "string"}, "description": "Explicit list of file paths to read (alternative to pattern)"},
		"path": {"type": "string", "description": "Directory to search in (defaults to working directory)"},
		"limit": {"type": "integer", "description": "Max lines per file (default 100)"},
		"maxFiles": {"type": "integer", "description": "Max number of files to read (default 20)"}
	}
}`)

const (
	defaultBulkMaxFiles   = 20
	defaultBulkLinesLimit = 100
	bulkMaxTotalBytes     = 96 * 1024 // 96 KB total output budget
)

func NewBulkReadTool() *Tool {
	return &Tool{
		Name:        "read_files",
		ReadOnly:    true,
		Description: prompt("read_files"),
		Parameters:  bulkReadParams,
		Execute:     executeBulkRead,
	}
}

func executeBulkRead(ctx context.Context, ectx ExecutionContext, args []byte) (*Result, error) {
	var params struct {
		Pattern  string   `json:"pattern"`
		Files    []string `json:"files"`
		Path     string   `json:"path"`
		MaxFiles int      `json:"maxFiles"`
		Limit    int      `json:"limit"`
	}
	if err := flexUnmarshal(args, &params); err != nil {
		return nil, err
	}

	if params.Pattern == "" && len(params.Files) == 0 {
		return ErrorResult(ErrCodeInvalidArgs, "pattern or files is required", false), nil
	}

	maxFiles := params.MaxFiles
	if maxFiles <= 0 {
		maxFiles = defaultBulkMaxFiles
	}

	linesPerFile := params.Limit
	if linesPerFile <= 0 {
		linesPerFile = defaultBulkLinesLimit
	}

	root := params.Path
	if root == "" {
		root = ectx.WorkDir
		if root == "" {
			var err error
			root, err = os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("failed to get working directory: %w", err)
			}
		}
	} else {
		root = resolvePath(root, ectx.WorkDir)
	}

	// Scale limits under context pressure using the same formula as other tools.
	effectiveMaxBytes := bulkMaxTotalBytes
	if ectx.ContextUsage > 0.5 {
		ratio := float64(EffectiveMaxBytes(ectx.ContextUsage)) / float64(MaxBytes)
		effectiveMaxBytes = max(int(float64(bulkMaxTotalBytes)*ratio), 16*1024)
		linesPerFile = max(int(float64(linesPerFile)*ratio), 20)
	}

	var files []fileInfo
	var totalMatched int

	if len(params.Files) > 0 {
		files, totalMatched = bulkResolveFiles(params.Files, root, maxFiles)
	} else {
		var err error
		files, totalMatched, err = bulkFindFiles(ctx, root, params.Pattern, ectx, maxFiles)
		if err != nil {
			return nil, err
		}
	}

	title := fmt.Sprintf("read_files: %s", params.Pattern)
	if len(params.Files) > 0 {
		title = fmt.Sprintf("read_files: %d files", len(params.Files))
	}

	if len(files) == 0 {
		return &Result{
			Title:    title,
			Output:   "No readable files found.",
			Metadata: map[string]any{"count": 0, "truncated": false},
		}, nil
	}

	var buf strings.Builder
	totalBytes := 0
	filesRead := 0
	filesTruncated := 0
	budgetExhausted := false

	for _, fi := range files {
		if budgetExhausted {
			break
		}

		content, lines, total, err := bulkReadSingleFile(fi.path, linesPerFile)
		if err != nil {
			continue
		}

		header := fmt.Sprintf("── %s ", fi.path)
		if lines < total {
			header += fmt.Sprintf("(%d of %d lines)", lines, total)
		} else {
			header += fmt.Sprintf("(%d lines)", total)
		}

		entry := header + "\n" + content + "\n\n"

		if totalBytes+len(entry) > effectiveMaxBytes && filesRead > 0 {
			budgetExhausted = true
			break
		}

		buf.WriteString(entry)
		totalBytes += len(entry)
		filesRead++
		if lines < total {
			filesTruncated++
		}
	}

	truncated := budgetExhausted || totalMatched > filesRead
	if truncated {
		fmt.Fprintf(&buf, "(Read %d of %d matched files", filesRead, totalMatched)
		if filesTruncated > 0 {
			fmt.Fprintf(&buf, ", %d files truncated to %d lines", filesTruncated, linesPerFile)
		}
		buf.WriteString(")\n")
	}

	return &Result{
		Title:  title,
		Output: buf.String(),
		Metadata: map[string]any{
			"count":     filesRead,
			"truncated": truncated,
			"matched":   totalMatched,
		},
	}, nil
}

// bulkResolveFiles resolves explicit file paths, skipping missing or binary files.
func bulkResolveFiles(paths []string, root string, maxFiles int) ([]fileInfo, int) {
	var files []fileInfo
	for _, p := range paths {
		abs := p
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(root, abs)
		}
		info, err := os.Stat(abs)
		if err != nil || info.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(abs))
		if binaryExtensions[ext] || imageExtensions[ext] || ext == ".pdf" {
			continue
		}
		files = append(files, fileInfo{
			path:  abs,
			mtime: info.ModTime().UnixNano(),
			info:  info,
		})
	}
	total := len(files)
	if len(files) > maxFiles {
		files = files[:maxFiles]
	}
	return files, total
}

func bulkFindFiles(ctx context.Context, root, pattern string, ectx ExecutionContext, maxFiles int) ([]fileInfo, int, error) {
	var files []fileInfo
	fsys := os.DirFS(root)
	scanned := 0
	lastProgress := 0

	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if d.IsDir() && ectx.IsIgnored(path+"/") {
			return fs.SkipDir
		}
		if ectx.IsIgnored(path) {
			return nil
		}

		scanned++
		if ectx.WriteOutput != nil && scanned-lastProgress >= 500 {
			lastProgress = scanned
			ectx.WriteOutput(fmt.Sprintf("scanning… %d files checked, %d matches\n", scanned, len(files)))
		}

		if d.IsDir() {
			return nil
		}

		matched, matchErr := doublestar.Match(pattern, path)
		if matchErr != nil || !matched {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if binaryExtensions[ext] || imageExtensions[ext] || ext == ".pdf" {
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
		return nil, 0, fmt.Errorf("read_files walk error: %w", err)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].mtime > files[j].mtime
	})

	totalMatched := len(files)
	if len(files) > maxFiles {
		files = files[:maxFiles]
	}

	return files, totalMatched, nil
}

func bulkReadSingleFile(path string, maxLines int) (content string, linesRead, totalLines int, err error) {
	if bin, err := isBinaryFile(path); err != nil || bin {
		return "", 0, 0, fmt.Errorf("binary file")
	}

	f, err := os.Open(path)
	if err != nil {
		return "", 0, 0, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	var buf strings.Builder
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum <= maxLines {
			line := scanner.Text()
			if len(line) > 2000 {
				line = line[:2000] + "..."
			}
			fmt.Fprintf(&buf, "%d: %s\n", lineNum, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", 0, 0, err
	}

	if lineNum == 0 {
		return "(empty file)", 0, 0, nil
	}

	return buf.String(), min(maxLines, lineNum), lineNum, nil
}
