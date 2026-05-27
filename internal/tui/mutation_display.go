package tui

import (
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/textdiff"
)

type mutationDisplay struct {
	Summary       string
	InspectorMeta []mutationField
	Failure       *mutationFailure
	HideSummary   bool
}

type mutationField struct {
	Label string
	Value string
}

type mutationFailure struct {
	Status  string
	Message string
}

func mutationDisplayFromCall(workspaceRoot string, call *events.ToolCallState) (mutationDisplay, bool) {
	if call == nil {
		return mutationDisplay{}, false
	}
	switch call.ToolName {
	case "write":
		return buildWriteMutationDisplay(workspaceRoot, call), true
	case "apply_patch":
		return buildApplyPatchMutationDisplay(workspaceRoot, call), true
	case "bash":
		return buildBashMutationDisplay(workspaceRoot, call), true
	default:
		return mutationDisplay{}, false
	}
}

func buildWriteMutationDisplay(workspaceRoot string, call *events.ToolCallState) mutationDisplay {
	input, ok := parseWriteToolViewInput(call.Input)
	if !ok {
		return mutationDisplay{
			Summary: fallbackMutationSummary(call),
			Failure: condenseMutationFailure(call),
		}
	}

	summaryLabel := mutationSummaryLabel(call, "wrote", "write")
	if call.WriteMutation != nil && !call.WriteMutation.Existed {
		summaryLabel = "created"
	}
	summary := joinMutationSummary(
		summaryLabel,
		displayToolPath(workspaceRoot, input.Path),
	)
	display := mutationDisplay{
		Summary: summary,
		InspectorMeta: []mutationField{
			{Label: "Path", Value: input.Path},
			{Label: "Previous", Value: writeMutationPreviousLabel(call)},
			{Label: "Content", Value: contentStatsLabel(input.Content)},
		},
	}
	if diffLabel := writeMutationDiffLabel(call); diffLabel != "" {
		display.InspectorMeta = append(display.InspectorMeta, mutationField{Label: "Diff", Value: diffLabel})
	}
	if failure := condenseMutationFailure(call); failure != nil {
		display.Failure = failure
		display.InspectorMeta = writeFailureInspectorMeta(input)
	}
	return display
}

func buildBashMutationDisplay(workspaceRoot string, call *events.ToolCallState) mutationDisplay {
	if call == nil {
		return mutationDisplay{}
	}
	if call.WriteMutation == nil {
		return mutationDisplay{
			Summary: fallbackMutationSummary(call),
			Failure: condenseMutationFailure(call),
		}
	}

	action := "changed"
	if !call.WriteMutation.Existed {
		action = "created"
	}
	display := mutationDisplay{
		Summary: joinMutationSummary(action, displayToolPath(workspaceRoot, call.WriteMutation.Path)),
		InspectorMeta: []mutationField{
			{Label: "Path", Value: call.WriteMutation.Path},
			{Label: "Previous", Value: writeMutationPreviousLabel(call)},
		},
	}
	if diffLabel := writeMutationDiffLabel(call); diffLabel != "" {
		display.InspectorMeta = append(display.InspectorMeta, mutationField{Label: "Diff", Value: diffLabel})
	}
	if failure := condenseMutationFailure(call); failure != nil {
		display.Failure = failure
	}
	return display
}

func writeFailureInspectorMeta(input struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}) []mutationField {
	if strings.TrimSpace(input.Path) == "" {
		return nil
	}
	return []mutationField{{Label: "Path", Value: input.Path}}
}

func mutationInspectorParams(display mutationDisplay) []inspectorParam {
	params := make([]inspectorParam, 0, len(display.InspectorMeta)+3)
	if !display.HideSummary && strings.TrimSpace(display.Summary) != "" {
		params = append(params, inspectorParam{Label: "Change", Value: display.Summary})
	}
	for _, field := range display.InspectorMeta {
		if strings.TrimSpace(field.Value) == "" {
			continue
		}
		params = append(params, inspectorParam{Label: field.Label, Value: field.Value})
	}
	if display.Failure != nil {
		if status := strings.TrimSpace(display.Failure.Status); status != "" {
			params = append(params, inspectorParam{Label: "Status", Value: status, Error: true})
		}
		if message := strings.TrimSpace(display.Failure.Message); message != "" {
			params = append(params, inspectorParam{Label: "Error", Value: message, Error: true})
		}
	}
	return params
}

func writeMutationPreviousLabel(call *events.ToolCallState) string {
	if call == nil || call.WriteMutation == nil {
		return ""
	}
	if !call.WriteMutation.Existed {
		return "file did not exist"
	}
	if call.WriteMutation.BeforeTruncated {
		return "previous content offloaded"
	}
	return contentStatsLabel(call.WriteMutation.Before)
}

func writeMutationBeforeContent(call *events.ToolCallState) (string, bool) {
	if call == nil || call.WriteMutation == nil || call.WriteMutation.BeforeTruncated {
		return "", false
	}
	return call.WriteMutation.Before, true
}

func writeMutationDiffLabel(call *events.ToolCallState) string {
	input, ok := parseWriteToolViewInput(call.Input)
	if !ok {
		return ""
	}
	if preview, ok := writeMutationDiffPreview(call); ok {
		if !textdiff.HasChanges(*preview) {
			return "no content changes"
		}
		added, removed := textdiff.LineStats(*preview)
		return fmt.Sprintf("+%d -%d lines", added, removed)
	}
	before, ok := writeMutationBeforeContent(call)
	if !ok {
		if call != nil && call.WriteMutation != nil && call.WriteMutation.BeforeTruncated {
			return "large diff offloaded"
		}
		return ""
	}
	added, removed := writeMutationLineStats(before, input.Content)
	if added == 0 && removed == 0 {
		return "no content changes"
	}
	return fmt.Sprintf("+%d -%d lines", added, removed)
}

func writeMutationLineStats(before, after string) (added, removed int) {
	ops := mutationDiffLines(splitNormalizedLines(before), splitNormalizedLines(after))
	for _, op := range ops {
		switch op.kind {
		case mutationDiffDelete:
			removed++
		case mutationDiffInsert:
			added++
		}
	}
	return added, removed
}

func condenseMutationFailure(call *events.ToolCallState) *mutationFailure {
	if call == nil {
		return nil
	}
	errorText := strings.TrimSpace(call.Error)
	if errorText == "" {
		return nil
	}
	switch call.ToolName {
	case "write":
		if strings.Contains(errorText, "path is required") || strings.Contains(errorText, "content is required") {
			return &mutationFailure{Status: "fix args", Message: errorText}
		}
		return &mutationFailure{Status: "failed", Message: errorText}
	default:
		return &mutationFailure{Status: "failed", Message: errorText}
	}
}

func fallbackMutationSummary(call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	switch call.ToolName {
	case "write":
		return joinMutationSummary(mutationSummaryLabel(call, "wrote", "write"), "")
	default:
		return strings.TrimSpace(call.ToolName)
	}
}

func splitNormalizedLines(text string) []string {
	if text == "" {
		return nil
	}
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	return strings.Split(strings.TrimRight(normalized, "\n"), "\n")
}

func joinMutationSummary(action, path string) string {
	action = strings.TrimSpace(action)
	path = strings.TrimSpace(path)
	switch {
	case action == "":
		return path
	case path == "":
		return action
	default:
		return action + " " + path
	}
}

func pluralize(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
