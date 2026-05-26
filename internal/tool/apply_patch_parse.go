package tool

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

const ApplyPatchToolName = "apply_patch"

var (
	ErrApplyPatchEmpty            = errors.New("patch is empty")
	ErrApplyPatchMissingBegin     = errors.New("patch is missing first line: *** Begin Patch")
	ErrApplyPatchMissingEnd       = errors.New("patch is missing final line: *** End Patch")
	ErrApplyPatchNoOperations     = errors.New("patch must contain at least one file operation")
	ErrApplyPatchEmptyPath        = errors.New("file path is required")
	ErrApplyPatchAbsolutePath     = errors.New("absolute paths are not allowed")
	ErrApplyPatchParentPath       = errors.New("paths must stay inside the workspace")
	ErrApplyPatchUnknownHeader    = errors.New("unknown patch header")
	ErrApplyPatchEmptyAdd         = errors.New("add file operation must include at least one added line")
	ErrApplyPatchEmptyUpdate      = errors.New("update file operation must include a move or at least one changed line")
	ErrApplyPatchMalformedLine    = errors.New("invalid patch syntax")
	ErrApplyPatchReadLinePrefixes = errors.New("patch lines must not include read output line number prefixes")
)

const (
	applyPatchBeginMarker  = "*** Begin Patch"
	applyPatchEndMarker    = "*** End Patch"
	applyPatchAddFile      = "*** Add File: "
	applyPatchDeleteFile   = "*** Delete File: "
	applyPatchUpdateFile   = "*** Update File: "
	applyPatchMoveTo       = "*** Move to: "
	applyPatchEOFMarker    = "*** End of File"
	applyPatchContextEmpty = "@@"
	applyPatchContext      = "@@ "
)

var windowsAbsolutePathPattern = regexp.MustCompile(`^[A-Za-z]:[\\/]|^\\\\`)

type ApplyPatch struct {
	Operations []ApplyPatchOperation
}

type ApplyPatchOperationKind string

const (
	ApplyPatchOperationAdd    ApplyPatchOperationKind = "add"
	ApplyPatchOperationDelete ApplyPatchOperationKind = "delete"
	ApplyPatchOperationUpdate ApplyPatchOperationKind = "update"
)

type ApplyPatchOperation struct {
	Kind     ApplyPatchOperationKind
	Path     string
	MovePath string
	Lines    []string
	Hunks    []ApplyPatchUpdateHunk
}

type ApplyPatchUpdateHunk struct {
	Context     string
	OldLines    []string
	NewLines    []string
	EndOfFile   bool
	StartLineNo int
}

func ParseApplyPatch(patch string) (ApplyPatch, error) {
	var err error
	patch, err = unwrapApplyPatchJSONArguments(patch)
	if err != nil {
		return ApplyPatch{}, err
	}
	parser := applyPatchParser{
		lines: normalizeApplyPatchLines(patch),
	}
	if len(parser.lines) == 0 {
		return ApplyPatch{}, InvalidArguments(ApplyPatchToolName, ErrApplyPatchEmpty)
	}
	if parser.lines[0] != applyPatchBeginMarker {
		return ApplyPatch{}, InvalidArguments(ApplyPatchToolName, ErrApplyPatchMissingBegin)
	}
	end := len(parser.lines) - 1
	if parser.lines[end] == "" && end > 0 {
		end--
	}
	if parser.lines[end] != applyPatchEndMarker {
		return ApplyPatch{}, InvalidArguments(ApplyPatchToolName, ErrApplyPatchMissingEnd)
	}
	parser.end = end
	parser.idx = 1
	parsed, err := parser.parse()
	if err != nil {
		return ApplyPatch{}, InvalidArguments(ApplyPatchToolName, err)
	}
	return parsed, nil
}

func unwrapApplyPatchJSONArguments(patch string) (string, error) {
	trimmed := strings.TrimSpace(patch)
	if trimmed == "" || !strings.HasPrefix(trimmed, "{") {
		return patch, nil
	}
	var input struct {
		Patch string `json:"patch"`
	}
	if err := DecodeArgsStrict(ApplyPatchToolName, json.RawMessage(trimmed), &input, "patch"); err != nil {
		return "", err
	}
	if strings.TrimSpace(input.Patch) == "" {
		return "", InvalidArguments(ApplyPatchToolName, ErrApplyPatchEmpty)
	}
	return input.Patch, nil
}

func normalizeApplyPatchLines(patch string) []string {
	normalized := strings.ReplaceAll(patch, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return strings.Split(normalized, "\n")
}

type applyPatchParser struct {
	lines []string
	idx   int
	end   int
}

func (p *applyPatchParser) parse() (ApplyPatch, error) {
	var operations []ApplyPatchOperation
	for p.idx < p.end {
		line := p.lines[p.idx]
		switch {
		case strings.HasPrefix(line, applyPatchAddFile):
			op, err := p.parseAddFile(line)
			if err != nil {
				return ApplyPatch{}, err
			}
			operations = append(operations, op)
		case strings.HasPrefix(line, applyPatchDeleteFile):
			op, err := p.parseDeleteFile(line)
			if err != nil {
				return ApplyPatch{}, err
			}
			operations = append(operations, op)
		case strings.HasPrefix(line, applyPatchUpdateFile):
			op, err := p.parseUpdateFile(line)
			if err != nil {
				return ApplyPatch{}, err
			}
			operations = append(operations, op)
		case strings.TrimSpace(line) == "":
			return ApplyPatch{}, applyPatchLineError(p.idx, ErrApplyPatchMalformedLine)
		default:
			return ApplyPatch{}, applyPatchLineError(p.idx, fmt.Errorf("%w: %s", ErrApplyPatchUnknownHeader, line))
		}
	}
	if len(operations) == 0 {
		return ApplyPatch{}, ErrApplyPatchNoOperations
	}
	return ApplyPatch{Operations: operations}, nil
}

func (p *applyPatchParser) parseAddFile(header string) (ApplyPatchOperation, error) {
	path, err := parseApplyPatchPath(header, applyPatchAddFile, p.idx)
	if err != nil {
		return ApplyPatchOperation{}, err
	}
	p.idx++
	start := p.idx
	var lines []string
	for p.idx < p.end && !isApplyPatchFileHeader(p.lines[p.idx]) {
		line := p.lines[p.idx]
		if !strings.HasPrefix(line, "+") {
			return ApplyPatchOperation{}, applyPatchLineError(p.idx, fmt.Errorf("%w: add file lines must start with +", ErrApplyPatchMalformedLine))
		}
		lines = append(lines, strings.TrimPrefix(line, "+"))
		p.idx++
	}
	if len(lines) == 0 {
		return ApplyPatchOperation{}, applyPatchLineError(start, ErrApplyPatchEmptyAdd)
	}
	if err := rejectApplyPatchReadLinePrefixes(lines, start); err != nil {
		return ApplyPatchOperation{}, err
	}
	return ApplyPatchOperation{
		Kind:  ApplyPatchOperationAdd,
		Path:  path,
		Lines: lines,
	}, nil
}

func (p *applyPatchParser) parseDeleteFile(header string) (ApplyPatchOperation, error) {
	path, err := parseApplyPatchPath(header, applyPatchDeleteFile, p.idx)
	if err != nil {
		return ApplyPatchOperation{}, err
	}
	p.idx++
	return ApplyPatchOperation{
		Kind: ApplyPatchOperationDelete,
		Path: path,
	}, nil
}

func (p *applyPatchParser) parseUpdateFile(header string) (ApplyPatchOperation, error) {
	path, err := parseApplyPatchPath(header, applyPatchUpdateFile, p.idx)
	if err != nil {
		return ApplyPatchOperation{}, err
	}
	p.idx++
	op := ApplyPatchOperation{
		Kind: ApplyPatchOperationUpdate,
		Path: path,
	}
	if p.idx < p.end && strings.HasPrefix(p.lines[p.idx], applyPatchMoveTo) {
		movePath, err := parseApplyPatchPath(p.lines[p.idx], applyPatchMoveTo, p.idx)
		if err != nil {
			return ApplyPatchOperation{}, err
		}
		op.MovePath = movePath
		p.idx++
	}

	var current applyPatchHunkBuilder
	for p.idx < p.end && !isApplyPatchFileHeader(p.lines[p.idx]) {
		line := p.lines[p.idx]
		switch {
		case line == applyPatchEOFMarker:
			if !current.hasChanges() {
				return ApplyPatchOperation{}, applyPatchLineError(p.idx, fmt.Errorf("%w before any changed line", ErrApplyPatchMalformedLine))
			}
			current.EndOfFile = true
			p.idx++
			if p.idx < p.end && !isApplyPatchFileHeader(p.lines[p.idx]) {
				return ApplyPatchOperation{}, applyPatchLineError(p.idx, fmt.Errorf("%w after end-of-file marker", ErrApplyPatchMalformedLine))
			}
		case line == applyPatchContextEmpty || strings.HasPrefix(line, applyPatchContext):
			if err := appendApplyPatchUpdateHunk(&op, &current); err != nil {
				return ApplyPatchOperation{}, err
			}
			current = applyPatchHunkBuilder{
				Context:     parseApplyPatchContext(line),
				StartLineNo: p.idx + 1,
			}
			p.idx++
		case strings.HasPrefix(line, " "):
			current.ensureStart(p.idx)
			text := strings.TrimPrefix(line, " ")
			current.OldLines = append(current.OldLines, text)
			current.NewLines = append(current.NewLines, text)
			p.idx++
		case strings.HasPrefix(line, "-"):
			current.ensureStart(p.idx)
			current.OldLines = append(current.OldLines, strings.TrimPrefix(line, "-"))
			p.idx++
		case strings.HasPrefix(line, "+"):
			current.ensureStart(p.idx)
			current.NewLines = append(current.NewLines, strings.TrimPrefix(line, "+"))
			p.idx++
		default:
			return ApplyPatchOperation{}, applyPatchLineError(p.idx, ErrApplyPatchMalformedLine)
		}
	}
	if err := appendApplyPatchUpdateHunk(&op, &current); err != nil {
		return ApplyPatchOperation{}, err
	}
	if op.MovePath == "" && len(op.Hunks) == 0 {
		return ApplyPatchOperation{}, applyPatchLineError(p.idx, ErrApplyPatchEmptyUpdate)
	}
	return op, nil
}

type applyPatchHunkBuilder struct {
	Context     string
	OldLines    []string
	NewLines    []string
	EndOfFile   bool
	StartLineNo int
}

func (h *applyPatchHunkBuilder) ensureStart(idx int) {
	if h.StartLineNo == 0 {
		h.StartLineNo = idx + 1
	}
}

func (h applyPatchHunkBuilder) hasChanges() bool {
	return len(h.OldLines) > 0 || len(h.NewLines) > 0
}

func appendApplyPatchUpdateHunk(op *ApplyPatchOperation, current *applyPatchHunkBuilder) error {
	if current == nil || current.StartLineNo == 0 {
		return nil
	}
	if !current.hasChanges() {
		return applyPatchLineError(current.StartLineNo-1, fmt.Errorf("%w: context marker must be followed by changed or context lines", ErrApplyPatchMalformedLine))
	}
	if err := rejectApplyPatchReadLinePrefixes(append(append([]string(nil), current.OldLines...), current.NewLines...), current.StartLineNo); err != nil {
		return err
	}
	op.Hunks = append(op.Hunks, ApplyPatchUpdateHunk{
		Context:     current.Context,
		OldLines:    append([]string(nil), current.OldLines...),
		NewLines:    append([]string(nil), current.NewLines...),
		EndOfFile:   current.EndOfFile,
		StartLineNo: current.StartLineNo,
	})
	*current = applyPatchHunkBuilder{}
	return nil
}

func parseApplyPatchContext(line string) string {
	if line == applyPatchContextEmpty {
		return ""
	}
	return strings.TrimPrefix(line, applyPatchContext)
}

func parseApplyPatchPath(line, marker string, idx int) (string, error) {
	path := strings.TrimSpace(strings.TrimPrefix(line, marker))
	if err := validateApplyPatchPath(path); err != nil {
		return "", applyPatchLineError(idx, err)
	}
	return filepath.ToSlash(filepath.Clean(path)), nil
}

func validateApplyPatchPath(path string) error {
	if strings.TrimSpace(path) == "" {
		return ErrApplyPatchEmptyPath
	}
	if strings.ContainsRune(path, 0) {
		return fmt.Errorf("%w: path contains NUL", ErrApplyPatchMalformedLine)
	}
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") || windowsAbsolutePathPattern.MatchString(path) {
		return ErrApplyPatchAbsolutePath
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || strings.HasPrefix(filepath.ToSlash(clean), "../") {
		return ErrApplyPatchParentPath
	}
	return nil
}

func isApplyPatchFileHeader(line string) bool {
	return strings.HasPrefix(line, applyPatchAddFile) ||
		strings.HasPrefix(line, applyPatchDeleteFile) ||
		strings.HasPrefix(line, applyPatchUpdateFile)
}

func rejectApplyPatchReadLinePrefixes(lines []string, startLineNo int) error {
	if prefix, ok := detectSequentialReadLinePrefix(lines); ok {
		return applyPatchLineError(startLineNo-1, fmt.Errorf("%w like %q", ErrApplyPatchReadLinePrefixes, prefix))
	}
	return nil
}

func applyPatchLineError(idx int, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("line %d: %w", idx+1, err)
}
