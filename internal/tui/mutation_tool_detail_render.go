package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/textdiff"
)

func renderWriteMutationToolDetailLines(m Model, sessionID string, ref sessionToolCallRef, call *events.ToolCallState, width int, diffStyle mutationToolDetailDiffStyle) ([]string, bool) {
	input, ok := parseWriteToolViewInput(call.Input)
	if !ok {
		return nil, false
	}

	lines := mutationToolDetailContextLines(m, writeMutationToolDetailMetaLabel(call, input.Content), width)
	createdFile := call.WriteMutation != nil && !call.WriteMutation.Existed

	if preview, ok := toolMutationDetailPreviewForSession(m, sessionID, ref, call); ok {
		if !textdiff.HasChanges(*preview) {
			return append(lines, renderMutationMetaLine(m, "no content changes", width)), true
		}
		lines = append(lines, "")
		ops := mutationDiffOpsFromPreview(preview)
		if createdFile {
			lines = append(lines, renderCreatedWriteMutationDetailOps(m, ops, width, preview.NewStartLine)...)
			return lines, true
		}
		lines = append(lines, renderMutationToolDetailDiffOpsWithStyleAt(m, ops, width, preview.OldStartLine, preview.NewStartLine, diffStyle)...)
		return lines, true
	}

	before, ok := writeMutationBeforeContent(call)
	if !ok {
		return nil, false
	}
	ops := trimMutationDiffContext(mutationDiffLines(splitNormalizedLines(before), splitNormalizedLines(input.Content)), 2)
	if !mutationDiffHasChanges(ops) {
		return append(lines, renderMutationMetaLine(m, "no content changes", width)), true
	}
	lines = append(lines, "")
	if createdFile {
		lines = append(lines, renderCreatedWriteMutationDetailOps(m, ops, width, 1)...)
		return lines, true
	}
	lines = append(lines, renderMutationToolDetailDiffOpsWithStyleAt(m, ops, width, 1, 1, diffStyle)...)
	return lines, true
}

func renderApplyPatchMutationToolDetailLines(m Model, workspaceRoot string, call *events.ToolCallState, width int, diffStyle mutationToolDetailDiffStyle) ([]string, bool) {
	mutations := applyPatchMutations(call)
	if len(mutations) == 0 {
		return nil, false
	}
	lines := make([]string, 0, len(mutations)*8)
	for idx, mutation := range mutations {
		if idx > 0 {
			lines = append(lines, "")
		}
		header := renderMutationActionLine(m, applyPatchMutationAction(mutation), displayToolPath(workspaceRoot, mutation.Path), width)
		lines = append(lines, header)
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
	return lines, true
}

func mutationToolDetailContextLines(m Model, meta string, width int) []string {
	if strings.TrimSpace(meta) == "" {
		return nil
	}
	return []string{renderMutationMetaLine(m, meta, width)}
}

func writeMutationToolDetailMetaLabel(call *events.ToolCallState, content string) string {
	parts := []string{contentStatsLabel(content)}
	if diff := strings.TrimSpace(writeMutationDiffLabel(call)); diff != "" && diff != "no content changes" {
		parts = append(parts, diff)
	}
	return strings.Join(parts, " • ")
}

func renderCreatedWriteMutationDetailOps(m Model, ops []mutationDiffOp, width, newStart int) []string {
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "primary", "#7cc7ff"))).
		Bold(true)
	lines := []string{headerStyle.Render(truncateEnd("New", max(width, 1)))}
	lines = append(lines, renderMutationDiffOpsAt(m, ops, width, 0, max(newStart, 1))...)
	return lines
}

func renderMutationToolDetailDiffOpsAt(m Model, ops []mutationDiffOp, textWidth, oldStart, newStart int) []string {
	return renderMutationToolDetailDiffOpsWithStyleAt(m, ops, textWidth, oldStart, newStart, mutationToolDetailDiffStyleAuto)
}

func renderMutationToolDetailDiffOpsWithStyleAt(m Model, ops []mutationDiffOp, textWidth, oldStart, newStart int, diffStyle mutationToolDetailDiffStyle) []string {
	canRenderSplit := textWidth >= mutationToolDetailSideBySideMinWidth
	switch diffStyle {
	case mutationToolDetailDiffStyleSplit:
		if canRenderSplit {
			return renderMutationSideBySideOpsAt(m, ops, textWidth, oldStart, newStart)
		}
		return renderMutationDiffOpsAt(m, ops, textWidth, oldStart, newStart)
	case mutationToolDetailDiffStyleUnified:
		return renderMutationDiffOpsAt(m, ops, textWidth, oldStart, newStart)
	}
	if !canRenderSplit || mutationToolDetailPrefersUnifiedDiff(ops) {
		return renderMutationDiffOpsAt(m, ops, textWidth, oldStart, newStart)
	}
	return renderMutationSideBySideOpsAt(m, ops, textWidth, oldStart, newStart)
}

func toolMutationDetailPreviewForSession(m Model, sessionID string, ref sessionToolCallRef, call *events.ToolCallState) (*textdiff.Preview, bool) {
	if detail, ok := loadedToolMutationCacheForSession(m, sessionID, ref); ok {
		return &detail.preview, true
	}
	return writeMutationDiffPreview(call)
}

func loadedToolMutationCacheForSession(m Model, sessionID string, ref sessionToolCallRef) (loadedToolMutationDetail, bool) {
	detail, ok := m.toolHydration.loadedMutations[scopedToolKey(sessionID, ref)]
	return detail, ok
}

func mutationToolDetailPrefersUnifiedDiff(ops []mutationDiffOp) bool {
	for i := 0; i < len(ops); {
		if ops[i].kind == mutationDiffEqual {
			i++
			continue
		}
		deletes, inserts := 0, 0
		for i < len(ops) && ops[i].kind != mutationDiffEqual {
			switch ops[i].kind {
			case mutationDiffDelete:
				deletes++
			case mutationDiffInsert:
				inserts++
			}
			i++
		}
		larger := max(deletes, inserts)
		smaller := min(deletes, inserts)
		switch {
		case larger >= 2 && smaller == 0:
			return true
		case larger >= 3 && larger-smaller >= 2:
			return true
		}
	}
	return false
}
