package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func traceDialogPromptSection(th *theme.Theme, turn *events.TurnState) string {
	lines := []string{dialogSectionStyle(th).Render("Prompt Assembly")}
	if turn == nil || turn.Prompt == nil {
		lines = append(lines, "Compiled prompt: not recorded yet.")
		return strings.Join(lines, "\n")
	}
	prompt := turn.Prompt
	lines = append(lines,
		fmt.Sprintf("Shape: %s", strings.TrimSpace(prompt.Shape)),
		fmt.Sprintf("Instruction bytes: %d base | %d composed", traceDialogByteCount(prompt.BaseInstructions), traceDialogByteCount(prompt.Instructions)),
		fmt.Sprintf("Cache split: %d cacheable | %d dynamic", traceDialogByteCount(prompt.CacheablePrefix), traceDialogByteCount(prompt.DynamicSuffix)),
		fmt.Sprintf("Layers: %d", len(prompt.Layers)),
		fmt.Sprintf("Fragments: %d", len(prompt.Fragments)),
	)
	if len(prompt.Layers) > 0 {
		totalBytes, largestBytes := traceDialogLayerByteStats(prompt.Layers)
		totalTokens := traceDialogLayerTokenStats(prompt.Layers)
		lines = append(lines, fmt.Sprintf("Layer bytes: %d total | %d largest", totalBytes, largestBytes))
		if totalTokens > 0 {
			lines = append(lines, fmt.Sprintf("Layer tokens: %d estimated", totalTokens))
		}
		if largest := traceDialogLargestLayers(prompt.Layers, 3); len(largest) > 0 {
			lines = append(lines, "Largest layers: "+strings.Join(largest, " | "))
		}
	}
	if len(prompt.Fragments) > 0 {
		totalBytes, largestBytes := traceDialogFragmentByteStats(prompt.Fragments)
		totalTokens := traceDialogFragmentTokenStats(prompt.Fragments)
		lines = append(lines, fmt.Sprintf("Fragment bytes: %d total | %d largest", totalBytes, largestBytes))
		if totalTokens > 0 {
			lines = append(lines, fmt.Sprintf("Fragment tokens: %d estimated", totalTokens))
		}
		if largest := traceDialogLargestFragments(prompt.Fragments, 3); len(largest) > 0 {
			lines = append(lines, "Largest fragments: "+strings.Join(largest, " | "))
		}
	}
	for idx, layer := range prompt.Layers {
		lines = append(lines, traceDialogLayerLine(idx, layer))
	}
	for idx, fragment := range prompt.Fragments {
		lines = append(lines, traceDialogFragmentLine(idx, fragment))
	}
	return strings.Join(lines, "\n")
}

func traceDialogLayerLine(index int, layer events.PromptLayerState) string {
	parts := []string{
		fmt.Sprintf("L%d. %s", index+1, traceDialogLayerName(layer)),
		strings.Join([]string{
			strings.TrimSpace(layer.Kind),
			strings.TrimSpace(layer.Source),
			strings.TrimSpace(layer.Stability),
		}, "/"),
		fmt.Sprintf("%d bytes", max(layer.Bytes, 0)),
	}
	if layer.Tokens > 0 {
		parts = append(parts, fmt.Sprintf("%d tokens", layer.Tokens))
	}
	if layer.Fragments > 0 {
		parts = append(parts, pluralize(layer.Fragments, "fragment"))
	}
	if status := strings.TrimSpace(layer.Status); status != "" {
		parts = append(parts, status)
	}
	return strings.Join(parts, " | ")
}

func traceDialogLayerName(layer events.PromptLayerState) string {
	if name := strings.TrimSpace(layer.Name); name != "" {
		return name
	}
	if source := strings.TrimSpace(layer.Source); source != "" {
		return source
	}
	return "layer"
}

func traceDialogFragmentLine(index int, fragment events.PromptFragmentState) string {
	parts := []string{
		fmt.Sprintf("%d. %s", index+1, traceDialogFragmentName(fragment)),
		strings.Join([]string{
			strings.TrimSpace(fragment.Kind),
			strings.TrimSpace(fragment.Source),
			strings.TrimSpace(fragment.Stability),
		}, "/"),
		fmt.Sprintf("%d bytes", max(fragment.Bytes, 0)),
	}
	if fragment.Tokens > 0 {
		parts = append(parts, fmt.Sprintf("%d tokens", fragment.Tokens))
	}
	if key := strings.TrimSpace(fragment.Key); key != "" {
		parts = append(parts, "key "+key)
	}
	return strings.Join(parts, " | ")
}

func traceDialogFragmentName(fragment events.PromptFragmentState) string {
	if label := strings.TrimSpace(fragment.Label); label != "" {
		return label
	}
	if key := strings.TrimSpace(fragment.Key); key != "" {
		return key
	}
	if source := strings.TrimSpace(fragment.Source); source != "" {
		return source
	}
	return "fragment"
}

func traceDialogContextSection(th *theme.Theme, turn *events.TurnState) string {
	if turn == nil || (turn.Continuation == nil && turn.CompactionFailure == nil && turn.Pruning == nil && turn.HistoryCompactionUI == nil) {
		return ""
	}
	lines := []string{dialogSectionStyle(th).Render("Context and History")}
	if eventsHistoryCompactionUIActive(turn) {
		lines = append(lines, historySummarizingTraceDetail)
	}
	if compacted := turn.Continuation; compacted != nil {
		lines = append(lines, traceDialogHistoryCompactionEventLine(compacted))
		if summary := traceDialogPreview(historyCompactionSummaryText(compacted), 180); summary != "" {
			lines = append(lines, "Summary: "+summary)
		}
	}
	if failure := turn.CompactionFailure; failure != nil {
		lines = append(lines, traceDialogCompactionFailureLine(failure))
	}
	if pruning := turn.Pruning; pruning != nil {
		parts := make([]string, 0, 1)
		if pruning.OmittedPriorTurns > 0 {
			parts = append(parts, pluralize(pruning.OmittedPriorTurns, "prior turn")+" omitted")
		}
		if len(parts) == 0 {
			parts = append(parts, "no omissions recorded")
		}
		lines = append(lines, "History pruning: "+strings.Join(parts, " | "))
		byteParts := make([]string, 0, 4)
		if pruning.RawInputBytes > 0 {
			byteParts = append(byteParts, fmt.Sprintf("%d raw bytes", pruning.RawInputBytes))
		}
		if pruning.CompactedInputBytes > 0 {
			byteParts = append(byteParts, fmt.Sprintf("%d compacted bytes", pruning.CompactedInputBytes))
		}
		if pruning.OmittedInputBytes > 0 {
			byteParts = append(byteParts, fmt.Sprintf("%d omitted bytes", pruning.OmittedInputBytes))
		}
		if len(byteParts) > 0 {
			lines = append(lines, "Pruning bytes: "+strings.Join(byteParts, " | "))
		}
	}
	return strings.Join(lines, "\n")
}

func traceDialogCompactionFailureLine(failure *events.CompactionFailureState) string {
	if failure == nil {
		return compactionFailureTraceLabel + ": not recorded"
	}
	parts := []string{
		string(failure.Scope),
		traceDialogPreview(failure.Reason, 80),
		traceDialogPreview(failure.Detail, 180),
	}
	if failure.EstimatedRequestTokens > 0 && failure.InputLimitTokens > 0 {
		parts = append(parts, fmt.Sprintf("%s/%s", formatCompactTokenCount(failure.EstimatedRequestTokens), formatCompactTokenCount(failure.InputLimitTokens)))
	}
	return compactionFailureTraceLabel + ": " + strings.Join(parts, " | ")
}

func traceDialogHistoryCompactionEventLine(compacted *events.HistoryContinuationState) string {
	if compacted == nil {
		return historySummaryTraceLabel + ": not recorded"
	}
	summaryText := historyCompactionSummaryText(compacted)
	parts := []string{
		traceDialogPreview(compacted.UpdateReason, 80),
		pluralize(compacted.ConsolidatedTurnCount, "turn"),
	}
	if compacted.InputBudget != nil {
		parts = append(parts,
			fmt.Sprintf("%s -> %s",
				formatCompactTokenCount(compacted.InputBudget.EstimatedRequestTokens),
				formatCompactTokenCount(compacted.InputBudget.ConsolidatedRequestTokens),
			),
		)
	}
	parts = append(parts, fmt.Sprintf("%d summary bytes", traceDialogByteCount(summaryText)))
	return historySummaryTraceLabel + ": " + strings.Join(parts, " | ")
}

func traceDialogRetrySection(th *theme.Theme, turn *events.TurnState) string {
	if turn == nil || (turn.Retry == nil && strings.TrimSpace(turn.Error) == "") {
		return ""
	}
	lines := []string{dialogSectionStyle(th).Render("Retry and Errors")}
	if errorText := traceDialogPreview(turn.Error, 220); errorText != "" {
		label := "Error"
		if turn.ErrorRetryable {
			label = "Retryable error"
		}
		lines = append(lines, label+": "+errorText)
	}
	if turn.Retry != nil {
		lines = append(lines, "Retry: "+liveTurnRetryLabel(turn))
		if message := traceDialogPreview(turn.Retry.Message, 180); message != "" {
			lines = append(lines, "Retry reason: "+message)
		}
	}
	return strings.Join(lines, "\n")
}

func traceDialogToolSection(th *theme.Theme, state events.SessionState, turn *events.TurnState) string {
	if turn == nil || len(orderedToolCallIDs(turn)) == 0 {
		return ""
	}
	lines := []string{dialogSectionStyle(th).Render("Tool Activity")}
	for index, callID := range orderedToolCallIDs(turn) {
		call := turn.ToolCalls[callID]
		if call == nil {
			continue
		}
		name := strings.TrimSpace(toolDisplayNameForSession(state, call))
		if name == "" {
			name = "tool"
		}
		parts := []string{fmt.Sprintf("%d. %s", index+1, name), toolStatus(call)}
		if runtime := call.Runtime; runtime != nil {
			if backend := strings.TrimSpace(runtime.Backend); backend != "" {
				parts = append(parts, "backend "+backend)
			}
		}
		if call.OutputTruncated {
			parts = append(parts, "output truncated")
		}
		if call.ErrorTruncated {
			parts = append(parts, "error truncated")
		}
		lines = append(lines, strings.Join(parts, " | "))
		if input := traceDialogPreview(call.Input, 180); input != "" {
			lines = append(lines, "   input: "+input)
		}
		if resultSummary := traceDialogToolResultSummary(call); resultSummary != "" {
			lines = append(lines, "   result: "+resultSummary)
		}
		if errorText := traceDialogPreview(call.Error, 180); errorText != "" {
			lines = append(lines, "   error: "+errorText)
		}
	}
	return strings.Join(lines, "\n")
}

func traceDialogWorkflowSection(th *theme.Theme, state events.SessionState, tracedTurn ...*events.TurnState) string {
	workflow := state.Workflow
	var turn *events.TurnState
	if len(tracedTurn) > 0 {
		turn = tracedTurn[0]
	}
	if turn != nil && turn.Config != nil && strings.TrimSpace(turn.Config.WorkflowID) != "" {
		lines := []string{dialogSectionStyle(th).Render("Workflow")}
		summary := []string{"Workflow: " + strings.TrimSpace(turn.Config.WorkflowID)}
		if phaseID := strings.TrimSpace(turn.Config.WorkflowPhaseID); phaseID != "" {
			summary = append(summary, "turn phase "+phaseID)
		}
		lines = append(lines, strings.Join(summary, " | "))
		return strings.Join(lines, "\n")
	}
	if workflow == nil || strings.TrimSpace(workflow.WorkflowID) == "" {
		return ""
	}
	lines := []string{dialogSectionStyle(th).Render("Workflow")}
	summary := []string{
		"Workflow: " + strings.TrimSpace(workflow.WorkflowID),
		"status " + traceWorkflowStatus(workflow.Status),
	}
	if phaseID := strings.TrimSpace(workflow.CurrentPhaseID); phaseID != "" {
		summary = append(summary, "phase "+phaseID)
	}
	if workflow.StartedAtSeq > 0 {
		summary = append(summary, fmt.Sprintf("started seq %d", workflow.StartedAtSeq))
	}
	if workflow.UpdatedAtSeq > 0 {
		summary = append(summary, fmt.Sprintf("updated seq %d", workflow.UpdatedAtSeq))
	}
	if workflow.CompletedAtSeq > 0 {
		summary = append(summary, fmt.Sprintf("completed seq %d", workflow.CompletedAtSeq))
	}
	lines = append(lines, strings.Join(summary, " | "))
	if reason := workflowStopReason(workflow); reason != "" {
		lines = append(lines, "Stop reason: "+traceDialogPreview(reason, 180))
	}
	if len(workflow.PhaseOrder) > 0 {
		lines = append(lines, "Phase history:")
		for index, phaseID := range workflow.PhaseOrder {
			phase := workflow.Phases[phaseID]
			lines = append(lines, traceDialogWorkflowPhaseLine(index, phaseID, phase))
			if phase != nil {
				if reason := strings.TrimSpace(phase.StopReason); reason != "" {
					lines = append(lines, "   stop: "+traceDialogPreview(reason, 180))
				}
			}
		}
	}
	if len(workflow.EvidenceOrder) > 0 {
		lines = append(lines, "Evidence:")
		for index, evidenceID := range workflow.EvidenceOrder {
			evidence := workflow.Evidence[evidenceID]
			if evidence == nil {
				continue
			}
			lines = append(lines, traceDialogWorkflowEvidenceLine(index, evidence))
		}
	}
	return strings.Join(lines, "\n")
}

func traceDialogWorkflowPhaseLine(index int, phaseID string, phase *events.WorkflowPhaseState) string {
	phaseID = strings.TrimSpace(phaseID)
	if phaseID == "" && phase != nil {
		phaseID = strings.TrimSpace(phase.PhaseID)
	}
	if phaseID == "" {
		phaseID = "phase"
	}
	if phase == nil {
		return fmt.Sprintf("%d. %s | not recorded", index+1, phaseID)
	}
	parts := []string{
		fmt.Sprintf("%d. %s", index+1, phaseID),
		traceWorkflowStatus(phase.Status),
	}
	if phase.StartedAtSeq > 0 {
		parts = append(parts, fmt.Sprintf("started seq %d", phase.StartedAtSeq))
	}
	if phase.BlockedAtSeq > 0 {
		parts = append(parts, fmt.Sprintf("blocked seq %d", phase.BlockedAtSeq))
	}
	if phase.CompletedAtSeq > 0 {
		parts = append(parts, fmt.Sprintf("completed seq %d", phase.CompletedAtSeq))
	}
	if phase.UpdatedAtSeq > 0 {
		parts = append(parts, fmt.Sprintf("updated seq %d", phase.UpdatedAtSeq))
	}
	if len(phase.EvidenceIDs) > 0 {
		parts = append(parts, pluralize(len(phase.EvidenceIDs), "evidence item"))
	}
	return strings.Join(parts, " | ")
}

func traceDialogWorkflowEvidenceLine(index int, evidence *events.WorkflowEvidenceState) string {
	parts := []string{
		fmt.Sprintf("%d. %s", index+1, strings.TrimSpace(evidence.Type)),
		"phase " + strings.TrimSpace(evidence.PhaseID),
	}
	if evidence.RecordedAtSeq > 0 {
		parts = append(parts, fmt.Sprintf("seq %d", evidence.RecordedAtSeq))
	}
	if evidence.Successful != nil {
		if *evidence.Successful {
			parts = append(parts, "successful")
		} else {
			parts = append(parts, "failed")
		}
	}
	if evidence.ExitCode != nil {
		parts = append(parts, fmt.Sprintf("exit %d", *evidence.ExitCode))
	}
	if command := traceDialogPreview(evidence.Command, 80); command != "" {
		parts = append(parts, "command "+command)
	}
	if summary := traceDialogPreview(evidence.Summary, 140); summary != "" {
		parts = append(parts, summary)
	}
	refs := traceDialogWorkflowEvidenceRefs(evidence)
	if len(refs) > 0 {
		parts = append(parts, strings.Join(refs, ", "))
	}
	return strings.Join(parts, " | ")
}

func traceDialogWorkflowEvidenceRefs(evidence *events.WorkflowEvidenceState) []string {
	if evidence == nil {
		return nil
	}
	refs := make([]string, 0, 5)
	if id := strings.TrimSpace(evidence.ToolCallID); id != "" {
		refs = append(refs, "tool "+id)
	}
	if id := strings.TrimSpace(evidence.ExecutionID); id != "" {
		refs = append(refs, "exec "+id)
	}
	if id := strings.TrimSpace(evidence.TaskID); id != "" {
		refs = append(refs, "task "+id)
	}
	if id := strings.TrimSpace(evidence.ReviewID); id != "" {
		refs = append(refs, "review "+id)
	}
	if id := strings.TrimSpace(evidence.ArtifactID); id != "" {
		refs = append(refs, "artifact "+id)
	}
	return refs
}

func traceWorkflowStatus(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return "unknown"
	}
	return status
}

func traceDialogFragmentByteStats(fragments []events.PromptFragmentState) (int, int) {
	total := 0
	largest := 0
	for _, fragment := range fragments {
		bytes := max(fragment.Bytes, 0)
		total += bytes
		if bytes > largest {
			largest = bytes
		}
	}
	return total, largest
}

func traceDialogLayerByteStats(layers []events.PromptLayerState) (int, int) {
	total := 0
	largest := 0
	for _, layer := range layers {
		bytes := max(layer.Bytes, 0)
		total += bytes
		if bytes > largest {
			largest = bytes
		}
	}
	return total, largest
}

func traceDialogFragmentTokenStats(fragments []events.PromptFragmentState) int {
	total := 0
	for _, fragment := range fragments {
		total += max(fragment.Tokens, 0)
	}
	return total
}

func traceDialogLayerTokenStats(layers []events.PromptLayerState) int {
	total := 0
	for _, layer := range layers {
		total += max(layer.Tokens, 0)
	}
	return total
}

func traceDialogLargestFragments(fragments []events.PromptFragmentState, limit int) []string {
	if len(fragments) == 0 || limit <= 0 {
		return nil
	}
	type fragmentSummary struct {
		name  string
		bytes int
		index int
	}
	summaries := make([]fragmentSummary, 0, len(fragments))
	for index, fragment := range fragments {
		summaries = append(summaries, fragmentSummary{
			name:  traceDialogFragmentName(fragment),
			bytes: max(fragment.Bytes, 0),
			index: index,
		})
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		if summaries[i].bytes == summaries[j].bytes {
			return summaries[i].index < summaries[j].index
		}
		return summaries[i].bytes > summaries[j].bytes
	})
	if len(summaries) > limit {
		summaries = summaries[:limit]
	}
	lines := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		lines = append(lines, fmt.Sprintf("%s %d bytes", summary.name, summary.bytes))
	}
	return lines
}

func traceDialogLargestLayers(layers []events.PromptLayerState, limit int) []string {
	if len(layers) == 0 || limit <= 0 {
		return nil
	}
	type layerSummary struct {
		name  string
		bytes int
		index int
	}
	summaries := make([]layerSummary, 0, len(layers))
	for index, layer := range layers {
		summaries = append(summaries, layerSummary{
			name:  traceDialogLayerName(layer),
			bytes: max(layer.Bytes, 0),
			index: index,
		})
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		if summaries[i].bytes == summaries[j].bytes {
			return summaries[i].index < summaries[j].index
		}
		return summaries[i].bytes > summaries[j].bytes
	})
	if len(summaries) > limit {
		summaries = summaries[:limit]
	}
	lines := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		lines = append(lines, fmt.Sprintf("%s %d bytes", summary.name, summary.bytes))
	}
	return lines
}

func traceDialogToolResultSummary(call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	parts := make([]string, 0, 4)
	if bytes := traceDialogByteCount(call.Output); bytes > 0 {
		label := "output"
		if traceDialogBlobValid(call.OutputBlob) {
			label = "output preview"
		}
		parts = append(parts, fmt.Sprintf("%s %d bytes", label, bytes))
	}
	if traceDialogBlobValid(call.OutputBlob) {
		parts = append(parts, fmt.Sprintf("output blob %d bytes", call.OutputBlob.Bytes))
	}
	if bytes := traceDialogByteCount(call.Error); bytes > 0 {
		label := "error"
		if traceDialogBlobValid(call.ErrorBlob) {
			label = "error preview"
		}
		parts = append(parts, fmt.Sprintf("%s %d bytes", label, bytes))
	}
	if traceDialogBlobValid(call.ErrorBlob) {
		parts = append(parts, fmt.Sprintf("error blob %d bytes", call.ErrorBlob.Bytes))
	}
	return strings.Join(parts, " | ")
}

func traceDialogBlobValid(ref *events.ToolResultBlobRef) bool {
	return ref != nil && strings.TrimSpace(ref.Ref) != "" && ref.Bytes > 0
}

func traceDialogByteCount(text string) int {
	return len(strings.TrimSpace(text))
}

func traceDialogPreview(text string, width int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if text == "" {
		return ""
	}
	return truncateEnd(text, max(width, 1))
}
