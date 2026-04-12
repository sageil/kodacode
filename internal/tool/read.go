package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var readParams = []byte(`{
	"type": "object",
	"properties": {
		"filePath": {"type": "string", "description": "The absolute path to the file to read. Does not accept directories — use glob or tree for directories, read_files for multiple files"},
		"offset": {"type": "integer", "description": "The line number to start reading from (1-indexed). Use when you know the relevant section"},
		"limit": {"type": "integer", "description": "Number of lines to read. Only provide if the file is too large to read at once"}
	},
	"required": ["filePath"]
}`)

var binaryExtensions = map[string]bool{
	".zip": true, ".gz": true, ".tar": true, ".rar": true, ".7z": true,
	".exe": true, ".dll": true, ".so": true, ".dylib": true,
	".wasm": true, ".pyc": true, ".pyo": true,
	".o": true, ".a": true, ".lib": true,
	".class": true, ".jar": true,
	".ico": true, ".ttf": true, ".otf": true, ".woff": true, ".woff2": true, ".eot": true,
	".sqlite": true, ".db": true,
	".dat": true, ".bin": true,
}

var imageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".bmp": true, ".webp": true, ".svg": true, ".ico": true,
}

// NewReadTool returns a Tool that reads files and directories.
func NewReadTool() *Tool {
	return &Tool{
		Name:        "read",
		ReadOnly:    true,
		Description: prompt("read"),
		Parameters:  readParams,
		Execute:     executeRead,
	}
}

func executeRead(_ context.Context, ectx ExecutionContext, args []byte) (*Result, error) {
	args = normalizeFilePathField(args)
	var params struct {
		FilePath string `json:"filePath"`
		Offset   int    `json:"offset"`
		Limit    int    `json:"limit"`
	}
	if err := flexUnmarshal(args, &params); err != nil {
		return nil, err
	}

	path := resolvePath(params.FilePath, ectx.WorkDir)
	title := path
	limit := params.Limit
	if limit <= 0 {
		limit = EffectiveMaxLines(ectx.ContextUsage)
	} else {
		limit = min(limit, EffectiveMaxLines(ectx.ContextUsage))
	}
	offset := max(params.Offset, 1)
	maxBytes := EffectiveMaxBytes(ectx.ContextUsage)

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%s: no such file or directory.%s", path, suggestSimilar(path))
		}
		return nil, err
	}

	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory, not a file. Use the glob or tree tool to list directory contents, or read_files to read multiple files at once", path)
	}

	ext := strings.ToLower(filepath.Ext(path))

	if imageExtensions[ext] || ext == ".pdf" {
		r, err := readMedia(path, ext, info)
		if r != nil {
			r.Title = title
		}
		return r, err
	}

	if binaryExtensions[ext] {
		return nil, fmt.Errorf("%s: binary file, cannot display contents", path)
	}

	if bin, err := isBinaryFile(path); err != nil {
		return nil, err
	} else if bin {
		return nil, fmt.Errorf("%s: binary file, cannot display contents", path)
	}

	r, err := readFile(path, info, offset, limit, maxBytes)
	if r != nil {
		r.Title = title
	}
	return r, err
}

func readFile(path string, info os.FileInfo, offset, limit, maxBytes int) (*Result, error) {
	if snap, ok := cachedSnapshot(path, info); ok {
		if snap.totalLines == 0 {
			output := fmt.Sprintf("<path>%s</path>\n<version>%s</version>\n<type>file</type>\n<content>\n\n(End of file - total 0 lines)\n</content>", path, snap.version)
			return &Result{
				Title:    path,
				Output:   output,
				Metadata: map[string]any{"truncated": false, "version": snap.version},
			}, nil
		}
		if offset-1 >= snap.totalLines {
			return &Result{
				Title:    path,
				Output:   fmt.Sprintf("<path>%s</path>\n<version>%s</version>\n<type>file</type>\n<content>\n(offset %d exceeds file length of %d lines)\n</content>", path, snap.version, offset, snap.totalLines),
				Metadata: map[string]any{"truncated": false, "version": snap.version},
			}, nil
		}
		return readFileSlice(path, offset, limit, maxBytes, snap.version, snap.totalLines)
	}

	return readFileFull(path, offset, limit, maxBytes)
}

func displayLineCount(content string) int {
	if content == "" {
		return 0
	}
	count := 1
	for _, r := range content {
		if r == '\n' {
			count++
		}
	}
	if strings.HasSuffix(content, "\n") {
		count--
	}
	if count < 0 {
		return 0
	}
	return count
}

func readFileSlice(path string, offset, limit, maxBytes int, versionToken string, totalLines int) (*Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	reader := bufio.NewReader(f)

	start := offset - 1
	endRequested := start + limit
	var (
		buf             strings.Builder
		bytesWritten    int
		total           int
		lastShownLine   int
		outputTruncated bool
	)
	for {
		rawLine, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			return nil, fmt.Errorf("error reading %s: %w", path, readErr)
		}
		if readErr == io.EOF && rawLine == "" {
			break
		}
		total++
		lineIndex := total - 1
		if lineIndex >= endRequested {
			break
		}
		if lineIndex < start {
			continue
		}

		line := strings.TrimSuffix(rawLine, "\n")
		line = strings.TrimSuffix(line, "\r")
		if len(line) > 2000 {
			line = line[:2000] + "..."
		}
		entry := fmt.Sprintf("%d: %s\n", total, line)
		if bytesWritten+len(entry) > maxBytes {
			outputTruncated = true
			break
		}
		buf.WriteString(entry)
		bytesWritten += len(entry)
		lastShownLine = total
	}

	if lastShownLine == 0 {
		footer := fmt.Sprintf("\n(Output truncated before line %d could be displayed. Use a smaller limit or a larger offset.)", offset)
		output := fmt.Sprintf("<path>%s</path>\n<version>%s</version>\n<type>file</type>\n<content>\n%s\n</content>", path, versionToken, footer)
		return &Result{
			Title:    path,
			Output:   output,
			Metadata: map[string]any{"truncated": true, "version": versionToken},
		}, nil
	}

	var footer string
	truncated := outputTruncated || endRequested < total
	switch {
	case outputTruncated:
		footer = fmt.Sprintf("\n(Showing lines %d-%d of %d. Output was capped to protect context budget. Use offset=%d to continue.)", offset, lastShownLine, totalLines, lastShownLine+1)
	case endRequested < totalLines:
		nextOffset := min(endRequested, totalLines) + 1
		footer = fmt.Sprintf("\n(Showing lines %d-%d of %d. Use offset=%d to continue.)", offset, lastShownLine, totalLines, nextOffset)
	default:
		footer = fmt.Sprintf("\n(End of file - total %d lines)", totalLines)
	}

	output := fmt.Sprintf("<path>%s</path>\n<version>%s</version>\n<type>file</type>\n<content>\n%s%s\n</content>", path, versionToken, buf.String(), footer)
	return &Result{
		Title:    path,
		Output:   output,
		Metadata: map[string]any{"truncated": truncated, "version": versionToken},
	}, nil
}

func readFileFull(path string, offset, limit, maxBytes int) (*Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var version versionAccumulator
	reader := bufio.NewReader(f)

	start := offset - 1
	endRequested := start + limit
	var (
		buf             strings.Builder
		bytesWritten    int
		total           int
		lastShownLine   int
		outputTruncated bool
	)
	for {
		rawLine, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			return nil, fmt.Errorf("error reading %s: %w", path, readErr)
		}
		if readErr == io.EOF && rawLine == "" {
			break
		}
		version.Write([]byte(rawLine))
		total++
		lineIndex := total - 1
		if lineIndex < start || lineIndex >= endRequested {
			if readErr == io.EOF {
				break
			}
			continue
		}

		line := strings.TrimSuffix(rawLine, "\n")
		line = strings.TrimSuffix(line, "\r")
		if len(line) > 2000 {
			line = line[:2000] + "..."
		}
		entry := fmt.Sprintf("%d: %s\n", total, line)
		if bytesWritten+len(entry) > maxBytes {
			outputTruncated = true
			continue
		}
		buf.WriteString(entry)
		bytesWritten += len(entry)
		lastShownLine = total
		if readErr == io.EOF {
			break
		}
	}
	versionToken := version.Token()

	if info, err := os.Stat(path); err == nil {
		storeSnapshot(path, info, versionToken, total)
	}

	if total == 0 {
		output := fmt.Sprintf("<path>%s</path>\n<version>%s</version>\n<type>file</type>\n<content>\n\n(End of file - total 0 lines)\n</content>", path, versionToken)
		return &Result{
			Title:    path,
			Output:   output,
			Metadata: map[string]any{"truncated": false, "version": versionToken},
		}, nil
	}

	if start >= total {
		return &Result{
			Title:    path,
			Output:   fmt.Sprintf("<path>%s</path>\n<version>%s</version>\n<type>file</type>\n<content>\n(offset %d exceeds file length of %d lines)\n</content>", path, versionToken, offset, total),
			Metadata: map[string]any{"truncated": false, "version": versionToken},
		}, nil
	}

	if lastShownLine == 0 {
		footer := fmt.Sprintf("\n(Output truncated before line %d could be displayed. Use a smaller limit or a larger offset.)", offset)
		output := fmt.Sprintf("<path>%s</path>\n<version>%s</version>\n<type>file</type>\n<content>\n%s\n</content>", path, versionToken, footer)
		return &Result{
			Title:    path,
			Output:   output,
			Metadata: map[string]any{"truncated": true, "version": versionToken},
		}, nil
	}

	var footer string
	truncated := outputTruncated || endRequested < total
	switch {
	case outputTruncated:
		footer = fmt.Sprintf("\n(Showing lines %d-%d of %d. Output was capped to protect context budget. Use offset=%d to continue.)", offset, lastShownLine, total, lastShownLine+1)
	case endRequested < total:
		nextOffset := min(endRequested, total) + 1
		footer = fmt.Sprintf("\n(Showing lines %d-%d of %d. Use offset=%d to continue.)", offset, lastShownLine, total, nextOffset)
	default:
		footer = fmt.Sprintf("\n(End of file - total %d lines)", total)
	}

	output := fmt.Sprintf("<path>%s</path>\n<version>%s</version>\n<type>file</type>\n<content>\n%s%s\n</content>", path, versionToken, buf.String(), footer)
	return &Result{
		Title:    path,
		Output:   output,
		Metadata: map[string]any{"truncated": truncated, "version": versionToken},
	}, nil
}

func readMedia(path, ext string, info os.FileInfo) (*Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	header := make([]byte, 512)
	n, err := f.Read(header)
	if err != nil && n == 0 {
		return nil, err
	}

	mime := http.DetectContentType(header[:n])
	switch ext {
	case ".svg":
		mime = "image/svg+xml"
	case ".pdf":
		mime = "application/pdf"
	}

	return &Result{
		Title: path,
		Output: fmt.Sprintf(
			"<path>%s</path>\n<type>media</type>\n<content>\n%s file (%d bytes). Binary content omitted to protect context budget. Use the open tool if you need to view it.\n</content>",
			path, mime, info.Size(),
		),
		Metadata: map[string]any{
			"truncated": false,
			"mime_type": mime,
			"size":      info.Size(),
		},
	}, nil
}

func isBinaryFile(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, 4096)
	n, err := f.Read(buf)
	if n == 0 {
		// Empty file is not binary; io.EOF is expected here.
		return false, err
	}
	buf = buf[:n]

	// Null byte check.
	if slices.Contains(buf, 0) {
		return true, nil
	}

	// Non-printable percentage check.
	nonPrintable := 0
	for _, b := range buf {
		if b < 32 && b != '\n' && b != '\r' && b != '\t' {
			nonPrintable++
		}
	}
	return float64(nonPrintable)/float64(len(buf)) > 0.3, nil
}

func suggestSimilar(path string) string {
	dir := filepath.Dir(path)
	base := strings.ToLower(filepath.Base(path))

	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	var suggestions []string
	for _, e := range entries {
		if strings.ToLower(e.Name()) == base {
			suggestions = append(suggestions, filepath.Join(dir, e.Name()))
		}
	}

	if len(suggestions) == 0 {
		return ""
	}
	return fmt.Sprintf(" Did you mean: %s?", strings.Join(suggestions, ", "))
}

// normalizeFilePathField rewrites common aliases for "filePath" in raw JSON
// tool arguments so that tools work even when models send "file", "file_path",
// "path", or "filename" instead of the canonical "filePath".
func normalizeFilePathField(args []byte) []byte {
	var m map[string]json.RawMessage
	if json.Unmarshal(args, &m) != nil {
		return args
	}
	if _, ok := m["filePath"]; ok {
		return args
	}
	for _, alias := range []string{"file", "file_path", "filename", "filepath"} {
		if v, ok := m[alias]; ok {
			m["filePath"] = v
			delete(m, alias)
			if out, err := json.Marshal(m); err == nil {
				return out
			}
			return args
		}
	}
	return args
}

// resolvePath converts a relative path to an absolute path.
// When workDir is non-empty it is used as the base; otherwise os.Getwd is called.
// It is used by bash.go, write.go, edit.go, grep.go, and glob.go (Tasks 6-10).
func resolvePath(path, workDir string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	base := workDir
	if base == "" {
		wd, err := os.Getwd()
		if err != nil {
			return filepath.Clean(path)
		}
		base = wd
	}
	return filepath.Join(base, path)
}

// fileInfo holds a path with its modification time and fs.FileInfo.
// It is used by glob.go for sorting results by modification time (Task 9).
type fileInfo struct {
	path  string
	mtime int64
	info  fs.FileInfo
}
