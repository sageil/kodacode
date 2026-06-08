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
	"unicode"

	"github.com/sageil/kodacode/internal/workspace"
)

const applyPatchLarkGrammar = `start: begin_patch hunk+ end_patch
begin_patch: "*** Begin Patch" LF
end_patch: "*** End Patch" LF?

hunk: add_hunk | delete_hunk | update_hunk
add_hunk: "*** Add File: " filename LF add_line+
delete_hunk: "*** Delete File: " filename LF
update_hunk: "*** Update File: " filename LF change_move? change?

filename: /(.+)/
add_line: "+" /(.*)/ LF -> line

change_move: "*** Move to: " filename LF
change: (change_context | change_line)+ eof_line?
change_context: ("@@" | "@@ " /(.+)/) LF
change_line: ("+" | "-" | " ") /(.*)/ LF
eof_line: "*** End of File" LF

%import common.LF`

var (
	ErrApplyPatchNoFilesModified = errors.New("patch produced the same file content; nothing changed")
	ErrApplyPatchHunkNoMatch     = errors.New("hunk did not match")
	ErrApplyPatchHunkAmbiguous   = errors.New("hunk matched multiple locations")
)

type ApplyPatchTool struct{}

type ApplyPatchStructuredResult struct {
	ChangedFiles []ApplyPatchChangedFile `json:"changed_files"`
}

type ApplyPatchChangedFile struct {
	Path     string `json:"path"`
	MovePath string `json:"move_path,omitempty"`
	Kind     string `json:"kind"`
}

func NewApplyPatchTool() ApplyPatchTool {
	return ApplyPatchTool{}
}

func (ApplyPatchTool) Definition() Definition {
	description := "Edit files with raw structured patch text, not JSON or Markdown. Format: first line \"*** Begin Patch\", then Add/Update/Delete file sections, final line \"*** End Patch\". Add File content lines start with +; Update File hunk lines start with space, +, or -. Do not include read output line-number prefixes like \"40:\"."
	return Definition{
		Name:                ApplyPatchToolName,
		Description:         description,
		ProviderDescription: description,
		InputKind:           InputKindCustom,
		InputSchema:         json.RawMessage(`{"type":"object"}`),
		InputFormat: &InputFormat{
			Type:       "grammar",
			Syntax:     "lark",
			Definition: applyPatchLarkGrammar,
		},
		ArgumentExamples: []string{
			"*** Begin Patch\n*** Update File: file.txt\n@@\n-old\n+new\n*** End Patch\n",
			"*** Begin Patch\n*** Add File: file.txt\n+first line\n+\n+third line\n*** End Patch\n",
			"*** Begin Patch\n*** Delete File: file.txt\n*** End Patch\n",
			"*** Begin Patch\n*** Update File: old.txt\n*** Move to: new.txt\n*** End Patch\n",
		},
		RequiresWorkspace: true,
	}
}

func (ApplyPatchTool) PathRequests(args json.RawMessage) ([]PathRequest, error) {
	patch, err := ParseApplyPatch(string(args))
	if err != nil {
		return nil, err
	}
	var requests []PathRequest
	for _, op := range patch.Operations {
		requests = append(requests, PathRequest{
			Access: workspace.AccessWrite,
			Path:   op.Path,
			Reason: "apply patch",
		})
		if strings.TrimSpace(op.MovePath) != "" {
			requests = append(requests, PathRequest{
				Access: workspace.AccessWrite,
				Path:   op.MovePath,
				Reason: "apply patch move destination",
			})
		}
	}
	return dedupePathRequests(requests), nil
}

func (ApplyPatchTool) Execute(_ context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	patch, err := ParseApplyPatch(string(args))
	if err != nil {
		return Result{}, err
	}
	plan, err := planApplyPatch(ectx, patch)
	if err != nil {
		return Result{}, err
	}
	if len(plan.changes) == 0 {
		return Result{Output: "Patch already applied successfully. No file changes needed."}, nil
	}
	if err := commitApplyPatchPlan(ectx, plan); err != nil {
		return Result{}, err
	}
	structured, err := MarshalStructuredResult(ApplyPatchStructuredResult{ChangedFiles: plan.structuredChanges()})
	if err != nil {
		return Result{}, err
	}
	return Result{
		Output:           formatApplyPatchOutput(plan),
		StructuredResult: structured,
		TextMutations:    plan.textMutations(),
	}, nil
}

type applyPatchExecutionError struct {
	cause error
}

func (e applyPatchExecutionError) Error() string {
	if e.cause == nil {
		return "apply_patch: edit failed."
	}
	return "apply_patch: " + strings.TrimSuffix(e.cause.Error(), ".") + "."
}

func (e applyPatchExecutionError) Unwrap() error {
	return e.cause
}

type applyPatchPlan struct {
	files   map[string]*applyPatchFileState
	changes []applyPatchPlannedChange
}

type applyPatchFileState struct {
	existed  bool
	content  string
	mode     os.FileMode
	next     string
	nextMode os.FileMode
	deleted  bool
	touched  bool
}

type applyPatchPlannedChange struct {
	kind         ApplyPatchOperationKind
	path         string
	resolvedPath string
	movePath     string
	resolvedMove string
}

func planApplyPatch(ectx ExecutionContext, patch ApplyPatch) (applyPatchPlan, error) {
	plan := applyPatchPlan{files: make(map[string]*applyPatchFileState)}
	for _, op := range patch.Operations {
		switch op.Kind {
		case ApplyPatchOperationAdd:
			path, err := resolveApplyPatchPath(ectx, op.Path)
			if err != nil {
				return applyPatchPlan{}, err
			}
			state, err := applyPatchFile(plan.files, path)
			if err != nil {
				return applyPatchPlan{}, err
			}
			state.next = applyPatchAddContent(op.Lines)
			state.nextMode = fileModeOrDefault(path, 0o644)
			state.deleted = false
			state.touched = true
			plan.changes = append(plan.changes, applyPatchPlannedChange{kind: op.Kind, path: op.Path, resolvedPath: path})
		case ApplyPatchOperationDelete:
			path, err := resolveApplyPatchPath(ectx, op.Path)
			if err != nil {
				return applyPatchPlan{}, err
			}
			state, err := applyPatchFile(plan.files, path)
			if err != nil {
				return applyPatchPlan{}, err
			}
			if !state.existed && !state.touched {
				return applyPatchPlan{}, applyPatchExecutionError{cause: fmt.Errorf("file to delete does not exist: %s", op.Path)}
			}
			state.next = ""
			state.deleted = true
			state.touched = true
			plan.changes = append(plan.changes, applyPatchPlannedChange{kind: op.Kind, path: op.Path, resolvedPath: path})
		case ApplyPatchOperationUpdate:
			path, err := resolveApplyPatchPath(ectx, op.Path)
			if err != nil {
				return applyPatchPlan{}, err
			}
			state, err := applyPatchFile(plan.files, path)
			if err != nil {
				return applyPatchPlan{}, err
			}
			if !state.existed && !state.touched {
				return applyPatchPlan{}, applyPatchExecutionError{cause: fmt.Errorf("file to update does not exist: %s", op.Path)}
			}
			next, err := applyPatchUpdateContent(state.next, op.Path, op.Hunks)
			if err != nil {
				return applyPatchPlan{}, err
			}
			if strings.TrimSpace(op.MovePath) == "" {
				state.next = next
				state.nextMode = fileModeOrDefault(path, state.mode)
				state.deleted = false
				state.touched = true
				plan.changes = append(plan.changes, applyPatchPlannedChange{kind: op.Kind, path: op.Path, resolvedPath: path})
				continue
			}
			movePath, err := resolveApplyPatchPath(ectx, op.MovePath)
			if err != nil {
				return applyPatchPlan{}, err
			}
			if filepath.Clean(path) == filepath.Clean(movePath) {
				state.next = next
				state.nextMode = fileModeOrDefault(path, state.mode)
				state.deleted = false
				state.touched = true
			} else {
				dest, err := applyPatchFile(plan.files, movePath)
				if err != nil {
					return applyPatchPlan{}, err
				}
				dest.next = next
				dest.nextMode = fileModeOrDefault(movePath, state.mode)
				dest.deleted = false
				dest.touched = true
				state.next = ""
				state.deleted = true
				state.touched = true
			}
			plan.changes = append(plan.changes, applyPatchPlannedChange{
				kind:         op.Kind,
				path:         op.Path,
				resolvedPath: path,
				movePath:     op.MovePath,
				resolvedMove: movePath,
			})
		default:
			return applyPatchPlan{}, applyPatchExecutionError{cause: fmt.Errorf("unknown patch operation %q", op.Kind)}
		}
	}
	plan.dropNoopChanges()
	return plan, nil
}

func resolveApplyPatchPath(ectx ExecutionContext, path string) (string, error) {
	decision, err := ectx.ResolvePath(workspace.AccessWrite, path)
	if err != nil {
		return "", err
	}
	return decision.ResolvedPath, nil
}

func applyPatchFile(files map[string]*applyPatchFileState, path string) (*applyPatchFileState, error) {
	clean := filepath.Clean(path)
	if state := files[clean]; state != nil {
		return state, nil
	}
	state := &applyPatchFileState{nextMode: 0o644}
	info, statErr := os.Stat(clean)
	if statErr == nil {
		if info.IsDir() {
			return nil, applyPatchExecutionError{cause: fmt.Errorf("%s is a directory", clean)}
		}
		data, err := os.ReadFile(clean)
		if err != nil {
			return nil, err
		}
		state.existed = true
		state.content = string(data)
		state.mode = info.Mode().Perm()
		state.next = state.content
		state.nextMode = state.mode
	} else if !os.IsNotExist(statErr) {
		return nil, statErr
	}
	files[clean] = state
	return state, nil
}

func (p *applyPatchPlan) dropNoopChanges() {
	if p == nil {
		return
	}
	changedPaths := make(map[string]struct{}, len(p.files))
	for path, state := range p.files {
		if state == nil || !state.touched {
			continue
		}
		switch {
		case state.deleted && state.existed:
			changedPaths[path] = struct{}{}
		case !state.deleted && !state.existed:
			changedPaths[path] = struct{}{}
		case !state.deleted && state.existed && state.next != state.content:
			changedPaths[path] = struct{}{}
		}
	}
	filtered := p.changes[:0]
	for _, change := range p.changes {
		if _, ok := changedPaths[filepath.Clean(change.resolvedPath)]; ok {
			filtered = append(filtered, change)
			continue
		}
		if strings.TrimSpace(change.movePath) != "" {
			if _, ok := changedPaths[filepath.Clean(change.resolvedMove)]; ok {
				filtered = append(filtered, change)
			}
		}
	}
	p.changes = filtered
}

func commitApplyPatchPlan(ectx ExecutionContext, plan applyPatchPlan) error {
	paths := make([]string, 0, len(plan.files))
	for path, state := range plan.files {
		if state != nil && state.touched {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return withApplyPatchLocks(paths, func() error {
		for _, path := range paths {
			if ectx.BeforeMutation != nil {
				if err := ectx.BeforeMutation(path); err != nil {
					return err
				}
			}
		}
		for _, path := range paths {
			state := plan.files[path]
			if state == nil || !state.touched {
				continue
			}
			if state.deleted {
				if state.existed {
					if err := os.Remove(path); err != nil {
						return err
					}
				}
				continue
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := WriteFileAtomically(path, []byte(state.next), state.nextMode); err != nil {
				return err
			}
		}
		return nil
	})
}

func applyPatchAddContent(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func applyPatchUpdateContent(content, path string, hunks []ApplyPatchUpdateHunk) (string, error) {
	lines := splitApplyPatchContentLines(content)
	replacements, err := computeApplyPatchReplacements(lines, path, hunks)
	if err != nil {
		return "", err
	}
	next := applyPatchReplacements(lines, replacements)
	if !lastStringEmpty(next) {
		next = append(next, "")
	}
	return strings.Join(next, "\n"), nil
}

func splitApplyPatchContentLines(content string) []string {
	lines := strings.Split(content, "\n")
	if lastStringEmpty(lines) {
		lines = lines[:len(lines)-1]
	}
	return lines
}

type applyPatchReplacement struct {
	start int
	oldN  int
	next  []string
}

func computeApplyPatchReplacements(lines []string, path string, hunks []ApplyPatchUpdateHunk) ([]applyPatchReplacement, error) {
	replacements := make([]applyPatchReplacement, 0, len(hunks))
	lineIndex := 0
	for _, hunk := range hunks {
		if strings.TrimSpace(hunk.Context) != "" {
			idx, ok := firstApplyPatchMatch(lines, []string{hunk.Context}, lineIndex, false)
			if ok {
				lineIndex = idx + 1
			} else if len(hunk.OldLines) == 0 {
				return nil, applyPatchExecutionError{cause: fmt.Errorf("context %q was not found in %s", hunk.Context, path)}
			}
		}
		if len(hunk.OldLines) == 0 {
			if idx, ok := appliedApplyPatchMatch(lines, hunk.NewLines, lineIndex, hunk.EndOfFile); ok {
				lineIndex = idx + len(hunk.NewLines)
				continue
			}
			insertAt := len(lines)
			if lastStringEmpty(lines) {
				insertAt = len(lines) - 1
			}
			replacements = append(replacements, applyPatchReplacement{start: insertAt, next: append([]string(nil), hunk.NewLines...)})
			continue
		}
		oldLines := hunk.OldLines
		newLines := hunk.NewLines
		matches := findApplyPatchMatches(lines, oldLines, lineIndex, hunk.EndOfFile)
		if len(matches) == 0 && lastStringEmpty(oldLines) {
			oldLines = oldLines[:len(oldLines)-1]
			if lastStringEmpty(newLines) {
				newLines = newLines[:len(newLines)-1]
			}
			matches = findApplyPatchMatches(lines, oldLines, lineIndex, hunk.EndOfFile)
		}
		if len(matches) == 0 {
			if idx, ok := appliedApplyPatchMatch(lines, newLines, lineIndex, hunk.EndOfFile); ok {
				lineIndex = idx + len(newLines)
				continue
			}
		}
		if len(matches) == 0 {
			return nil, applyPatchExecutionError{cause: applyPatchHunkNoMatchError(path, oldLines)}
		}
		replacements = append(replacements, applyPatchReplacement{
			start: matches[0],
			oldN:  len(oldLines),
			next:  append([]string(nil), newLines...),
		})
		lineIndex = matches[0] + len(oldLines)
	}
	sort.SliceStable(replacements, func(i, j int) bool {
		return replacements[i].start < replacements[j].start
	})
	return replacements, nil
}

func applyPatchHunkNoMatchError(path string, oldLines []string) error {
	if prefix, ok := detectAnyReadLinePrefix(oldLines); ok {
		return fmt.Errorf("%s: %w. Remove line numbers copied from read output like %q", path, ErrApplyPatchHunkNoMatch, prefix)
	}
	return fmt.Errorf("%s: %w. Re-read this file section and retry", path, ErrApplyPatchHunkNoMatch)
}

func applyPatchReplacements(lines []string, replacements []applyPatchReplacement) []string {
	out := append([]string(nil), lines...)
	for idx := len(replacements) - 1; idx >= 0; idx-- {
		replacement := replacements[idx]
		start := replacement.start
		end := min(start+replacement.oldN, len(out))
		next := append([]string(nil), out[:start]...)
		next = append(next, replacement.next...)
		next = append(next, out[end:]...)
		out = next
	}
	return out
}

func firstApplyPatchMatch(lines, pattern []string, start int, eof bool) (int, bool) {
	matches := findApplyPatchMatches(lines, pattern, start, eof)
	if len(matches) == 0 {
		return 0, false
	}
	return matches[0], true
}

func appliedApplyPatchMatch(lines, pattern []string, start int, eof bool) (int, bool) {
	if len(pattern) == 0 {
		return 0, false
	}
	idx, ok := firstApplyPatchMatch(lines, pattern, start, eof)
	if !ok {
		return 0, false
	}
	return idx, true
}

func findApplyPatchMatches(lines, pattern []string, start int, eof bool) []int {
	if len(pattern) == 0 {
		return []int{min(start, len(lines))}
	}
	if len(pattern) > len(lines) {
		return nil
	}
	start = max(start, 0)
	searchStart := start
	if eof {
		searchStart = len(lines) - len(pattern)
	}
	for _, mode := range []applyPatchMatchMode{
		applyPatchLineExact,
		applyPatchLineTrimRight,
		applyPatchLineTrimBoth,
		applyPatchLineNormalized,
	} {
		var matches []int
		for idx := searchStart; idx <= len(lines)-len(pattern); idx++ {
			if applyPatchLinesMatch(lines, pattern, idx, mode) {
				matches = append(matches, idx)
				break
			}
		}
		if len(matches) > 0 {
			return matches
		}
	}
	return nil
}

type applyPatchMatchMode int

const (
	applyPatchLineExact applyPatchMatchMode = iota
	applyPatchLineTrimRight
	applyPatchLineTrimBoth
	applyPatchLineNormalized
)

func applyPatchLinesMatch(lines, pattern []string, start int, mode applyPatchMatchMode) bool {
	if start < 0 || start+len(pattern) > len(lines) {
		return false
	}
	for idx, want := range pattern {
		if !applyPatchLineMatches(lines[start+idx], want, mode) {
			return false
		}
	}
	return true
}

func applyPatchLineMatches(got, want string, mode applyPatchMatchMode) bool {
	switch mode {
	case applyPatchLineExact:
		return got == want
	case applyPatchLineTrimRight:
		return strings.TrimRightFunc(got, unicode.IsSpace) == strings.TrimRightFunc(want, unicode.IsSpace)
	case applyPatchLineTrimBoth:
		return strings.TrimSpace(got) == strings.TrimSpace(want)
	case applyPatchLineNormalized:
		return normalizeApplyPatchMatchLine(got) == normalizeApplyPatchMatchLine(want)
	default:
		return got == want
	}
}

func normalizeApplyPatchMatchLine(line string) string {
	return strings.Map(func(ch rune) rune {
		switch ch {
		case '\u2010', '\u2011', '\u2012', '\u2013', '\u2014', '\u2015', '\u2212':
			return '-'
		case '\u2018', '\u2019', '\u201A', '\u201B':
			return '\''
		case '\u201C', '\u201D', '\u201E', '\u201F':
			return '"'
		case '\u00A0', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006', '\u2007', '\u2008', '\u2009', '\u200A', '\u202F', '\u205F', '\u3000':
			return ' '
		default:
			return ch
		}
	}, strings.TrimSpace(line))
}

func lastStringEmpty(values []string) bool {
	return len(values) > 0 && values[len(values)-1] == ""
}

func withApplyPatchLocks(paths []string, fn func() error) error {
	paths = uniqueCleanPaths(paths)
	var run func(int) error
	run = func(idx int) error {
		if idx >= len(paths) {
			return fn()
		}
		return WithFileMutationLock(paths[idx], func() error {
			return run(idx + 1)
		})
	}
	return run(0)
}

func uniqueCleanPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	sort.Strings(out)
	return out
}

func formatApplyPatchOutput(plan applyPatchPlan) string {
	var out strings.Builder
	out.WriteString("Patch applied successfully. Changed files:\n")
	for _, change := range plan.structuredChanges() {
		out.WriteString(applyPatchOutputPrefix(change.Kind))
		out.WriteByte(' ')
		out.WriteString(change.Path)
		if change.MovePath != "" {
			out.WriteString(" -> ")
			out.WriteString(change.MovePath)
		}
		out.WriteByte('\n')
	}
	return strings.TrimRight(out.String(), "\n")
}

func (p applyPatchPlan) textMutations() []TextMutation {
	if len(p.files) == 0 {
		return nil
	}
	paths := make([]string, 0, len(p.files))
	for path, state := range p.files {
		if state == nil || !state.touched || !applyPatchFileChanged(state) {
			continue
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	out := make([]TextMutation, 0, len(paths))
	for _, path := range paths {
		state := p.files[path]
		after := state.next
		if state.deleted {
			after = ""
		}
		mode := state.nextMode
		if state.deleted {
			mode = state.mode
		}
		out = append(out, TextMutation{
			Path:    path,
			Before:  state.content,
			After:   after,
			Existed: state.existed,
			Mode:    uint32(mode),
		})
	}
	return out
}

func applyPatchFileChanged(state *applyPatchFileState) bool {
	if state == nil {
		return false
	}
	switch {
	case state.deleted && state.existed:
		return true
	case !state.deleted && !state.existed:
		return true
	case !state.deleted && state.existed && state.next != state.content:
		return true
	default:
		return false
	}
}

func (p applyPatchPlan) structuredChanges() []ApplyPatchChangedFile {
	out := make([]ApplyPatchChangedFile, 0, len(p.changes))
	for _, change := range p.changes {
		kind := string(change.kind)
		if change.kind == ApplyPatchOperationUpdate {
			kind = "modify"
		}
		out = append(out, ApplyPatchChangedFile{
			Path:     change.path,
			MovePath: change.movePath,
			Kind:     kind,
		})
	}
	return out
}

func applyPatchOutputPrefix(kind string) string {
	switch kind {
	case "add":
		return "A"
	case "delete":
		return "D"
	default:
		return "M"
	}
}
