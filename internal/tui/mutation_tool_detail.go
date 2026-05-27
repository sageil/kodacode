package tui

import (
	"github.com/sageil/kodacode/internal/events"
	"strings"
)

const mutationToolDetailSideBySideMinWidth = 84
const mutationSideBySideMinColumnWidth = 24
const mutationToolDetailLoadedPreviewContextLines = 4

type mutationToolDetailDiffStyle int

const (
	mutationToolDetailDiffStyleAuto mutationToolDetailDiffStyle = iota
	mutationToolDetailDiffStyleSplit
	mutationToolDetailDiffStyleUnified
)

func (s mutationToolDetailDiffStyle) next() mutationToolDetailDiffStyle {
	switch s {
	case mutationToolDetailDiffStyleAuto:
		return mutationToolDetailDiffStyleSplit
	case mutationToolDetailDiffStyleSplit:
		return mutationToolDetailDiffStyleUnified
	default:
		return mutationToolDetailDiffStyleAuto
	}
}

func (s mutationToolDetailDiffStyle) label() string {
	switch s {
	case mutationToolDetailDiffStyleSplit:
		return "split"
	case mutationToolDetailDiffStyleUnified:
		return "unified"
	default:
		return "auto"
	}
}

func renderMutationToolDetailSectionForSession(m Model, sessionID, workspaceRoot string, ref sessionToolCallRef, call *events.ToolCallState, width int, diffStyle mutationToolDetailDiffStyle) string {
	switch call.ToolName {
	case "write":
		lines, ok := renderWriteMutationToolDetailLines(m, sessionID, ref, call, width, diffStyle)
		if ok {
			return strings.Join(lines, "\n")
		}
	case "apply_patch":
		lines, ok := renderApplyPatchMutationToolDetailLines(m, workspaceRoot, call, width, diffStyle)
		if ok {
			return strings.Join(lines, "\n")
		}
	}
	return renderMutationToolTimelineSection(m, workspaceRoot, call, width)
}
