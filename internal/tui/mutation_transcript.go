package tui

import (
	"fmt"
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
	return renderApplyPatchMutationLinesWithDiffStyle(m, workspaceRoot, call, width, mutationToolDetailDiffStyleUnified)
}

func renderApplyPatchMutationLinesWithDiffStyle(m Model, workspaceRoot string, call *events.ToolCallState, width int, diffStyle mutationToolDetailDiffStyle) []string {
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
		lines = append(lines, renderMutationToolDetailDiffOpsWithStyleAt(m, mutationDiffOpsFromPreview(mutation.DiffPreview), width, mutation.DiffPreview.OldStartLine, mutation.DiffPreview.NewStartLine, diffStyle)...)
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
	return renderBashMutationLinesWithDiffStyle(m, workspaceRoot, call, width, mutationToolDetailDiffStyleUnified)
}

func renderBashMutationLinesWithDiffStyle(m Model, workspaceRoot string, call *events.ToolCallState, width int, diffStyle mutationToolDetailDiffStyle) []string {
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
		lines = append(lines, renderMutationToolDetailDiffOpsWithStyleAt(m, mutationDiffOpsFromPreview(preview), width, preview.OldStartLine, preview.NewStartLine, diffStyle)...)
		return lines
	}
	if call.WriteMutation.BeforeTruncated {
		return append(lines, renderMutationMetaLine(m, "diff unavailable in transcript for large shell write", width))
	}
	return append(lines, renderMutationMetaLine(m, "diff unavailable", width))
}

func renderWriteMutationLines(m Model, workspaceRoot string, call *events.ToolCallState, width int) []string {
	return renderWriteMutationLinesWithDiffStyle(m, "", sessionToolCallRef{}, workspaceRoot, call, width, mutationToolDetailDiffStyleUnified)
}

func renderWriteMutationLinesWithDiffStyle(m Model, sessionID string, ref sessionToolCallRef, workspaceRoot string, call *events.ToolCallState, width int, diffStyle mutationToolDetailDiffStyle) []string {
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
		lines = append(lines, renderMutationToolDetailDiffOpsWithStyleAt(m, mutationDiffOpsFromPreview(preview), width, preview.OldStartLine, preview.NewStartLine, diffStyle)...)
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
		lines = append(lines, renderMutationToolDetailDiffOpsWithStyleAt(m, ops, width, 1, 1, diffStyle)...)
		return lines
	case call != nil && call.WriteMutation != nil && call.WriteMutation.BeforeTruncated:
		return append(lines, renderMutationMetaLine(m, "diff unavailable in transcript for large write", width))
	default:
		return append(lines, renderMutationMetaLine(m, "diff unavailable", width))
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
