package tui

import (
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/textdiff"
)

func applyPatchToolDisplayName(workspaceRoot string, call *events.ToolCallState) string {
	paths := applyPatchToolMutationPaths(call)
	switch len(paths) {
	case 0:
		return "Edit files"
	case 1:
		return "Edit " + displayToolBaseName(workspaceRoot, paths[0])
	default:
		return fmt.Sprintf("Edit %d files", len(paths))
	}
}

func applyPatchToolListSummary(call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	if isApplyPatchNoop(call) {
		return ""
	}
	if strings.TrimSpace(call.Error) != "" {
		return summarizeInlineValue(call.Error)
	}
	if len(call.WriteMutations) > 0 {
		return fmt.Sprintf("%d changed file%s", len(call.WriteMutations), pluralS(len(call.WriteMutations)))
	}
	return summarizeInlineValue(call.Output)
}

func isApplyPatchNoop(call *events.ToolCallState) bool {
	if call == nil || strings.TrimSpace(call.ToolName) != "apply_patch" || strings.TrimSpace(call.Error) != "" {
		return false
	}
	if len(call.WriteMutations) > 0 || call.WriteMutation != nil {
		return false
	}
	return normalizedToolSummary(call.Output) == normalizedToolSummary("Patch already applied successfully. No file changes needed.")
}

func applyPatchInspectorParams(call *events.ToolCallState) []inspectorParam {
	display, ok := mutationDisplayFromCall("", call)
	if !ok {
		return nil
	}
	return mutationInspectorParams(display)
}

func buildApplyPatchMutationDisplay(workspaceRoot string, call *events.ToolCallState) mutationDisplay {
	mutations := applyPatchMutations(call)
	if len(mutations) == 0 {
		return mutationDisplay{
			Summary: fallbackMutationSummary(call),
			Failure: condenseMutationFailure(call),
		}
	}
	display := mutationDisplay{
		Summary: joinMutationSummary("edited", applyPatchMutationSummaryTarget(workspaceRoot, mutations)),
		InspectorMeta: []mutationField{
			{Label: "Files", Value: applyPatchMutationFilesLabel(workspaceRoot, mutations)},
		},
	}
	if label := applyPatchMutationDiffLabel(mutations); label != "" {
		display.InspectorMeta = append(display.InspectorMeta, mutationField{Label: "Diff", Value: label})
	}
	if failure := condenseMutationFailure(call); failure != nil {
		display.Failure = failure
	}
	return display
}

func applyPatchMutations(call *events.ToolCallState) []events.WriteMutation {
	if call == nil {
		return nil
	}
	if len(call.WriteMutations) > 0 {
		return call.WriteMutations
	}
	if call.WriteMutation != nil {
		return []events.WriteMutation{*call.WriteMutation}
	}
	return nil
}

func applyPatchMutationSummaryTarget(workspaceRoot string, mutations []events.WriteMutation) string {
	if len(mutations) == 1 {
		return displayToolPath(workspaceRoot, mutations[0].Path)
	}
	return fmt.Sprintf("%d files", len(mutations))
}

func applyPatchMutationFilesLabel(workspaceRoot string, mutations []events.WriteMutation) string {
	paths := make([]string, 0, len(mutations))
	for _, mutation := range mutations {
		paths = append(paths, displayToolPath(workspaceRoot, mutation.Path))
	}
	return strings.Join(paths, ", ")
}

func applyPatchMutationDiffLabel(mutations []events.WriteMutation) string {
	added, removed := 0, 0
	known := false
	for _, mutation := range mutations {
		if mutation.DiffPreview == nil {
			continue
		}
		nextAdded, nextRemoved := textdiff.LineStats(*mutation.DiffPreview)
		added += nextAdded
		removed += nextRemoved
		known = true
	}
	if !known {
		return ""
	}
	if added == 0 && removed == 0 {
		return "no content changes"
	}
	return fmt.Sprintf("+%d -%d lines", added, removed)
}

func pluralS(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}
