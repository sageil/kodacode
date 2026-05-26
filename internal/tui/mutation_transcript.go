package tui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/textdiff"
)

func renderMutationTranscriptSection(m Model, workspaceRoot string, call *events.ToolCallState, display mutationDisplay, width int) string {
	lines := make([]string, 0, 8)
	switch {
	case display.Failure != nil:
		lines = append(lines, renderMutationFailureSummaryLines(m, display, width)...)
	default:
		lines = append(lines, renderMutationSuccessLines(m, workspaceRoot, call, width)...)
	}
	return strings.Join(lines, "\n")
}

func renderMutationSuccessLines(m Model, workspaceRoot string, call *events.ToolCallState, width int) []string {
	switch call.ToolName {
	case "write":
		return renderWriteMutationLines(m, workspaceRoot, call, width)
	case "edit":
		return renderEditMutationLines(m, workspaceRoot, call, width)
	case "apply_patch":
		return renderApplyPatchMutationLines(m, workspaceRoot, call, width)
	case "mkdir":
		return renderMutationSummaryLines(m, mutationToolTranscriptBody(workspaceRoot, call), width)
	case "bash":
		return renderBashMutationLines(m, workspaceRoot, call, width)
	default:
		return renderMutationSummaryLines(m, mutationToolFallbackBody(call), width)
	}
}

func renderApplyPatchMutationLines(m Model, workspaceRoot string, call *events.ToolCallState, width int) []string {
	mutations := applyPatchMutations(call)
	if len(mutations) == 0 {
		return renderMutationSummaryLines(m, mutationToolFallbackBody(call), width)
	}
	lines := make([]string, 0, len(mutations)*6)
	for idx, mutation := range mutations {
		if idx > 0 {
			lines = append(lines, "")
		}
		action := applyPatchMutationAction(mutation)
		target := displayToolPath(workspaceRoot, mutation.Path)
		if mutation.DiffPreview != nil {
			added, removed := textdiff.LineStats(*mutation.DiffPreview)
			if added > 0 || removed > 0 {
				target += fmt.Sprintf(" (+%d -%d)", added, removed)
			}
		}
		lines = append(lines, renderMutationActionLine(m, action, target, width))
		if mutation.DiffPreview == nil {
			lines = append(lines, renderMutationMetaLine(m, "diff unavailable", width))
			continue
		}
		if !textdiff.HasChanges(*mutation.DiffPreview) {
			lines = append(lines, renderMutationMetaLine(m, "no content changes", width))
			continue
		}
		lines = append(lines, renderMutationDiffOpsAt(m, mutationDiffOpsFromPreview(mutation.DiffPreview), width, mutation.DiffPreview.OldStartLine, mutation.DiffPreview.NewStartLine)...)
	}
	return lines
}

func applyPatchMutationAction(mutation events.WriteMutation) string {
	if !mutation.Existed {
		return "Created"
	}
	return "Changed"
}

func renderBashMutationLines(m Model, workspaceRoot string, call *events.ToolCallState, width int) []string {
	if call == nil {
		return nil
	}
	if call.WriteMutation == nil {
		return renderMutationSummaryLines(m, mutationToolFallbackBody(call), width)
	}

	action := "Changed"
	if !call.WriteMutation.Existed {
		action = "Created"
	}
	lines := []string{
		renderMutationActionLine(m, action, displayToolPath(workspaceRoot, call.WriteMutation.Path), width),
	}
	if preview, ok := writeMutationDiffPreview(call); ok {
		if !textdiff.HasChanges(*preview) {
			return append(lines, renderMutationMetaLine(m, "no content changes", width))
		}
		lines = append(lines, "")
		lines = append(lines, renderMutationDiffOpsAt(m, mutationDiffOpsFromPreview(preview), width, preview.OldStartLine, preview.NewStartLine)...)
		return lines
	}
	if call.WriteMutation.BeforeTruncated {
		return append(lines, renderMutationMetaLine(m, "diff unavailable in transcript for large shell write", width))
	}
	return append(lines, renderMutationMetaLine(m, "diff unavailable", width))
}

func renderWriteMutationLines(m Model, workspaceRoot string, call *events.ToolCallState, width int) []string {
	input, ok := parseWriteToolViewInput(call.Input)
	if !ok {
		return renderMutationSummaryLines(m, mutationToolFallbackBody(call), width)
	}

	action := "Wrote"
	if call.WriteMutation != nil && !call.WriteMutation.Existed {
		action = "Created"
	}
	lines := []string{
		renderMutationActionLine(m, action, displayToolPath(workspaceRoot, input.Path), width),
		renderMutationMetaLine(m, contentStatsLabel(input.Content), width),
	}
	if preview, ok := writeMutationDiffPreview(call); ok {
		if !textdiff.HasChanges(*preview) {
			return append(lines, renderMutationMetaLine(m, "no content changes", width))
		}
		lines = append(lines, "")
		lines = append(lines, renderMutationDiffOpsAt(m, mutationDiffOpsFromPreview(preview), width, preview.OldStartLine, preview.NewStartLine)...)
		return lines
	}
	before, ok := writeMutationBeforeContent(call)
	switch {
	case ok:
		ops := trimMutationDiffContext(mutationDiffLines(splitNormalizedLines(before), splitNormalizedLines(input.Content)), 2)
		if !mutationDiffHasChanges(ops) {
			return append(lines, renderMutationMetaLine(m, "no content changes", width))
		}
		lines = append(lines, "")
		lines = append(lines, renderMutationDiffOps(m, ops, width)...)
		return lines
	case call != nil && call.WriteMutation != nil && call.WriteMutation.BeforeTruncated:
		return append(lines, renderMutationMetaLine(m, "diff unavailable in transcript for large write", width))
	default:
		return append(lines, renderMutationMetaLine(m, "diff unavailable", width))
	}
}

func renderEditMutationLines(m Model, workspaceRoot string, call *events.ToolCallState, width int) []string {
	input, ok := parseEditToolViewInput(call.Input)
	if !ok {
		return renderMutationSummaryLines(m, mutationToolFallbackBody(call), width)
	}

	target := displayToolPath(workspaceRoot, input.Path)
	if diff := strings.TrimSpace(editMutationCompactDiffLabel(call, input)); diff != "" && diff != "no content changes" {
		target += " (" + diff + ")"
	}
	meta := editMutationMetaLabel(call, input)
	if strings.TrimSpace(meta) == "" {
		meta = contentStatsLabel(input.NewText)
	}
	lines := []string{
		renderMutationActionLine(m, "Edited", target, width),
		renderMutationMetaLine(m, meta, width),
	}
	if preview, ok := writeMutationDiffPreview(call); ok {
		if !textdiff.HasChanges(*preview) {
			return append(lines, renderMutationMetaLine(m, "no content changes", width))
		}
		lines = append(lines, "")
		lines = append(lines, renderMutationDiffOpsAt(m, mutationDiffOpsFromPreview(preview), width, preview.OldStartLine, preview.NewStartLine)...)
		return lines
	}
	ops := trimMutationDiffContext(mutationDiffLines(splitNormalizedLines(input.OldText), splitNormalizedLines(input.NewText)), 2)
	if !mutationDiffHasChanges(ops) {
		return append(lines, renderMutationMetaLine(m, "no content changes", width))
	}
	lines = append(lines, "")
	startLine := editMutationPreviewStartLine(call)
	lines = append(lines, renderMutationDiffOpsAt(m, ops, width, startLine, startLine)...)
	return lines
}

func editMutationMetaLabel(call *events.ToolCallState, input editToolViewInput) string {
	return strings.TrimSpace(editToolMatchLabel(input))
}

func editMutationDisplayLine(call *events.ToolCallState) int {
	if call == nil {
		return 0
	}
	if line, ok := editMutationPreviewChangedLine(call); ok {
		return line
	}
	for _, mutation := range call.MutationRanges {
		if mutation.OldStartLine > 0 {
			return mutation.OldStartLine
		}
		if mutation.NewStartLine > 0 {
			return mutation.NewStartLine
		}
	}
	if line, ok := editMutationOutputLine(call.Output); ok {
		return line
	}
	if call.WriteMutation != nil && call.WriteMutation.DiffPreview != nil {
		if call.WriteMutation.DiffPreview.OldStartLine > 0 {
			return call.WriteMutation.DiffPreview.OldStartLine
		}
		if call.WriteMutation.DiffPreview.NewStartLine > 0 {
			return call.WriteMutation.DiffPreview.NewStartLine
		}
	}
	return 0
}

func editMutationPreviewChangedLine(call *events.ToolCallState) (int, bool) {
	preview, ok := writeMutationDiffPreview(call)
	if !ok || preview == nil {
		return 0, false
	}
	oldLine := preview.OldStartLine
	newLine := preview.NewStartLine
	oldKnown := oldLine > 0
	newKnown := newLine > 0
	for _, op := range preview.Ops {
		switch op.Kind {
		case textdiff.OpContext:
			if oldKnown {
				oldLine++
			}
			if newKnown {
				newLine++
			}
		case textdiff.OpDelete:
			switch {
			case oldKnown:
				return oldLine, true
			case newKnown:
				return newLine, true
			}
		case textdiff.OpInsert:
			switch {
			case newKnown:
				return newLine, true
			case oldKnown:
				return oldLine, true
			}
		case textdiff.OpSkip:
			continue
		}
	}
	return 0, false
}

func editMutationPreviewStartLine(call *events.ToolCallState) int {
	if call == nil {
		return 0
	}
	if call.WriteMutation != nil && call.WriteMutation.DiffPreview != nil {
		if call.WriteMutation.DiffPreview.OldStartLine > 0 {
			return call.WriteMutation.DiffPreview.OldStartLine
		}
		if call.WriteMutation.DiffPreview.NewStartLine > 0 {
			return call.WriteMutation.DiffPreview.NewStartLine
		}
	}
	for _, mutation := range call.MutationRanges {
		if mutation.OldStartLine > 0 {
			return mutation.OldStartLine
		}
		if mutation.NewStartLine > 0 {
			return mutation.NewStartLine
		}
	}
	if line, ok := editMutationOutputLine(call.Output); ok {
		return line
	}
	return 0
}

func editMutationOutputLine(output string) (int, bool) {
	output = strings.TrimSpace(output)
	switch {
	case strings.HasPrefix(output, "edited line "):
		value := strings.TrimPrefix(output, "edited line ")
		fields := strings.Fields(value)
		if len(fields) == 0 {
			return 0, false
		}
		line, err := strconv.Atoi(fields[0])
		return line, err == nil && line > 0
	case strings.Contains(output, "starting at line "):
		value := output[strings.Index(output, "starting at line ")+len("starting at line "):]
		fields := strings.Fields(value)
		if len(fields) == 0 {
			return 0, false
		}
		line, err := strconv.Atoi(fields[0])
		return line, err == nil && line > 0
	default:
		return 0, false
	}
}

func renderMutationFailureSummaryLines(m Model, display mutationDisplay, width int) []string {
	summary := strings.TrimSpace(display.Summary)
	if summary == "" {
		summary = "change"
	}
	return []string{renderMutationErrorLine(m, summary, width)}
}

func renderMutationSummaryLines(m Model, body string, width int) []string {
	if strings.TrimSpace(body) == "" {
		return nil
	}
	rawLines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		if strings.TrimSpace(line) == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, renderMutationMetaLine(m, line, width))
	}
	return lines
}

func renderMutationActionLine(m Model, action, target string, width int) string {
	action = strings.TrimSpace(action)
	target = strings.TrimSpace(target)
	if action == "" {
		return renderMutationMetaLine(m, target, width)
	}

	primary := colorFor(m.theme, "primary", "#7cc7ff")
	actionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(primary)).
		Bold(true)
	targetStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(primary))

	if target == "" {
		return actionStyle.Render(truncateEnd(action, max(width, 1)))
	}

	available := max(width, 1)
	actionWidth := lipgloss.Width(action)
	if actionWidth >= available {
		return actionStyle.Render(truncateEnd(action, available))
	}
	targetWidth := max(available-actionWidth-1, 1)
	return actionStyle.Render(action) + " " + targetStyle.Render(truncateEnd(target, targetWidth))
}

func renderMutationMetaLine(m Model, text string, width int) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
		Render(truncateEnd(strings.TrimSpace(text), max(width, 1)))
}

func mutationDiffHasChanges(ops []mutationDiffOp) bool {
	for _, op := range ops {
		if op.kind != mutationDiffEqual {
			return true
		}
	}
	return false
}

func renderMutationErrorLine(m Model, text string, width int) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "error", "#ff9aa6"))).
		Render(truncateEnd(strings.TrimSpace(text), max(width, 1)))
}

func renderWriteContentLines(m Model, contentLines []string, width int) []string {
	maxLine := len(contentLines)
	gutterWidth := max(len(fmt.Sprintf("%d", maxLine)), 2)
	contentWidth := max(width-(gutterWidth+2), 1)

	addBG := "#153a24"
	addFG := colorFor(m.theme, "success", "#90e5b4")

	addLineNoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(addFG)).
		Background(lipgloss.Color(addBG))
	addContentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "text", "#e8fff0"))).
		Background(lipgloss.Color(addBG))

	out := make([]string, 0, len(contentLines))
	for idx, line := range contentLines {
		lineNo := fmt.Sprintf("%*d", gutterWidth, idx+1)
		segments := []string{""}
		if line != "" {
			segments = nil
			remaining := line
			for {
				head, tail := hardWrapMutationText(remaining, contentWidth)
				segments = append(segments, head)
				if tail == "" {
					break
				}
				remaining = tail
			}
		}
		for segIdx, segment := range segments {
			gutter := " " + lineNo + " "
			if segIdx > 0 {
				gutter = " " + strings.Repeat(" ", gutterWidth) + " "
			}
			out = append(out, addLineNoStyle.Render(gutter)+addContentStyle.Render(padMutationDiffSegment(segment, contentWidth)))
		}
	}
	return out
}
