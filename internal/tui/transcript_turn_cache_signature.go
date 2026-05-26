package tui

import (
	"hash"
	"hash/fnv"
	"io"
	"strconv"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
)

func buildTurnTranscriptSourceSignature(turn *events.TurnState) uint64 {
	hasher := fnv.New64a()
	appendTurnTranscriptSourceSignature(hasher, turn)
	return hasher.Sum64()
}

func buildTurnCompactionSignature(compaction *events.HistoryContinuationState) string {
	if compaction == nil {
		return ""
	}
	hasher := fnv.New64a()
	appendTurnCompactionSignature(hasher, compaction)
	return strconv.FormatUint(hasher.Sum64(), 16)
}

func appendTurnTranscriptSourceSignature(hasher hash.Hash64, turn *events.TurnState) {
	if hasher == nil || turn == nil {
		return
	}
	writeTranscriptSignatureString(hasher, turn.TurnID)
	writeTranscriptSignatureString(hasher, turn.UserText)
	writeTranscriptSignatureString(hasher, turn.AssistantText)
	writeTranscriptSignatureString(hasher, turn.StreamingText)
	writeTranscriptSignatureString(hasher, turn.ReasoningText)
	writeTranscriptSignatureString(hasher, turn.Error)
	writeTranscriptSignatureString(hasher, string(turn.ErrorCode))
	writeTranscriptSignatureBool(hasher, turn.ErrorRetryable)
	appendReviewTranscriptSignature(hasher, turn.Review)
	appendTurnPruningSignature(hasher, turn.Pruning)
	appendTurnCompactionSignature(hasher, turn.Continuation)
	for _, entry := range turn.Transcript {
		if entry.Kind == events.TranscriptEntryTool {
			call := turn.ToolCalls[entry.CallID]
			if !shouldRenderToolCallInTranscript(turn, entry.CallID, call) {
				continue
			}
		}
		writeTranscriptSignatureString(hasher, string(entry.Kind))
		writeTranscriptSignatureInt64(hasher, entry.Sequence)
		writeTranscriptSignatureString(hasher, entry.Text)
		writeTranscriptSignatureString(hasher, entry.CallID)
		writeTranscriptSignatureString(hasher, entry.SegmentID)
	}
	for _, callID := range orderedToolCallIDs(turn) {
		call := turn.ToolCalls[callID]
		if !shouldRenderToolCallInTranscript(turn, callID, call) && !shouldRenderLiveToolCallPreview(turn, call) {
			continue
		}
		appendToolCallTranscriptSignature(hasher, call)
	}
	for _, handoffID := range orderedHandoffIDs(turn) {
		appendHandoffTranscriptSignature(hasher, turn.Handoffs[handoffID])
	}
}

func appendTurnPruningSignature(hasher hash.Hash64, pruning *events.PruningState) {
	if hasher == nil || pruning == nil {
		return
	}
	values := []int{
		pruning.PriorTurns,
		pruning.PriorInputBytes,
		pruning.RawPriorTurns,
		pruning.RawInputBytes,
		pruning.CompactedPriorTurns,
		pruning.CompactedInputBytes,
		pruning.OmittedPriorTurns,
		pruning.OmittedInputBytes,
	}
	for _, value := range values {
		writeTranscriptSignatureInt(hasher, value)
	}
}

func appendTurnCompactionSignature(hasher hash.Hash64, compaction *events.HistoryContinuationState) {
	if hasher == nil || compaction == nil {
		return
	}
	writeTranscriptSignatureString(hasher, compaction.UpdateReason)
	writeTranscriptSignatureString(hasher, compaction.Attribution.Model)
	writeTranscriptSignatureString(hasher, compaction.Attribution.InputLimitSource)
	if compaction.InputBudget != nil {
		writeTranscriptSignatureInt(hasher, compaction.InputBudget.InputLimitTokens)
		writeTranscriptSignatureInt(hasher, compaction.InputBudget.TriggerTokens)
		writeTranscriptSignatureInt(hasher, compaction.InputBudget.TargetTokens)
		writeTranscriptSignatureInt(hasher, compaction.InputBudget.EstimatedRequestTokens)
		writeTranscriptSignatureInt(hasher, compaction.InputBudget.ConsolidatedRequestTokens)
	}
	writeTranscriptSignatureString(hasher, compaction.RenderedSummary)
	writeTranscriptSignatureString(hasher, compaction.FrontierTurnID)
	writeTranscriptSignatureInt(hasher, compaction.ConsolidatedTurnCount)
	writeTranscriptSignatureInt(hasher, compaction.NewlyConsolidatedTurnCount)
	for _, fact := range compaction.Artifact.WorkspaceFacts {
		path := fact.Path
		writeTranscriptSignatureString(hasher, path)
	}
}

func appendReviewTranscriptSignature(hasher hash.Hash64, review *events.ReviewState) {
	if hasher == nil || review == nil {
		return
	}
	writeTranscriptSignatureString(hasher, review.OverallCorrectness)
	writeTranscriptSignatureString(hasher, review.OverallSummary)
	for _, finding := range review.Findings {
		writeTranscriptSignatureString(hasher, finding.Severity)
		writeTranscriptSignatureString(hasher, finding.Path)
		writeTranscriptSignatureInt(hasher, finding.Line)
		writeTranscriptSignatureString(hasher, finding.Title)
		writeTranscriptSignatureString(hasher, finding.Explanation)
	}
}

func appendToolCallTranscriptSignature(hasher hash.Hash64, call *events.ToolCallState) {
	if hasher == nil || call == nil {
		return
	}
	writeTranscriptSignatureString(hasher, call.CallID)
	writeTranscriptSignatureString(hasher, call.ToolName)
	writeTranscriptSignatureString(hasher, call.ReusedFromCallID)
	writeTranscriptSignatureString(hasher, call.Input)
	writeTranscriptSignatureString(hasher, call.Output)
	writeTranscriptSignatureString(hasher, call.Error)
	writeTranscriptSignatureBool(hasher, call.Succeeded)
	writeTranscriptSignatureBool(hasher, call.OutputTruncated)
	writeTranscriptSignatureBool(hasher, call.ErrorTruncated)
	writeTranscriptSignatureBool(hasher, call.Declared)
	writeTranscriptSignatureBool(hasher, call.Executing)
	writeTranscriptSignatureBool(hasher, call.Completed)
	writeTranscriptSignatureInt64(hasher, call.LastUpdatedSeq)
	for _, mutation := range call.MutationRanges {
		writeTranscriptSignatureInt(hasher, mutation.OldStartLine)
		writeTranscriptSignatureInt(hasher, mutation.NewStartLine)
	}
	if call.WriteMutation != nil {
		writeTranscriptSignatureString(hasher, call.WriteMutation.Path)
		writeTranscriptSignatureBool(hasher, call.WriteMutation.Existed)
		writeTranscriptSignatureString(hasher, call.WriteMutation.Before)
		writeTranscriptSignatureBool(hasher, call.WriteMutation.BeforeTruncated)
		writeTranscriptSignatureString(hasher, strconv.FormatUint(uint64(call.WriteMutation.Mode), 10))
		if call.WriteMutation.DiffPreview != nil {
			writeTranscriptSignatureInt(hasher, call.WriteMutation.DiffPreview.OldStartLine)
			writeTranscriptSignatureInt(hasher, call.WriteMutation.DiffPreview.NewStartLine)
			for _, op := range call.WriteMutation.DiffPreview.Ops {
				writeTranscriptSignatureString(hasher, string(op.Kind))
				writeTranscriptSignatureString(hasher, op.Text)
			}
		}
	}
	for _, mutation := range call.WriteMutations {
		writeTranscriptSignatureString(hasher, mutation.Path)
		writeTranscriptSignatureBool(hasher, mutation.Existed)
		writeTranscriptSignatureString(hasher, mutation.Before)
		writeTranscriptSignatureBool(hasher, mutation.BeforeTruncated)
		writeTranscriptSignatureString(hasher, strconv.FormatUint(uint64(mutation.Mode), 10))
		if mutation.DiffPreview != nil {
			writeTranscriptSignatureInt(hasher, mutation.DiffPreview.OldStartLine)
			writeTranscriptSignatureInt(hasher, mutation.DiffPreview.NewStartLine)
			for _, op := range mutation.DiffPreview.Ops {
				writeTranscriptSignatureString(hasher, string(op.Kind))
				writeTranscriptSignatureString(hasher, op.Text)
			}
		}
	}
	for _, resource := range call.ObservedResources {
		writeTranscriptSignatureString(hasher, resource.Kind)
		writeTranscriptSignatureString(hasher, resource.Path)
		writeTranscriptSignatureString(hasher, resource.Version)
		writeTranscriptSignatureString(hasher, resource.State)
		writeTranscriptSignatureBool(hasher, resource.Complete)
		writeTranscriptSignatureInt(hasher, resource.StartLine)
		writeTranscriptSignatureInt(hasher, resource.EndLine)
		writeTranscriptSignatureInt(hasher, resource.TotalLines)
	}
}

func appendHandoffTranscriptSignature(hasher hash.Hash64, handoff *events.AgentHandoffState) {
	if hasher == nil || handoff == nil {
		return
	}
	writeTranscriptSignatureString(hasher, handoff.HandoffID)
	writeTranscriptSignatureString(hasher, handoff.ChildAgentID)
	writeTranscriptSignatureString(hasher, string(handoff.Status))
	writeTranscriptSignatureString(hasher, handoff.Task)
	writeTranscriptSignatureBool(hasher, handoff.PreviewActive)
	writeTranscriptSignatureString(hasher, handoff.PreviewToolName)
	writeTranscriptSignatureString(hasher, handoff.PreviewAction)
	writeTranscriptSignatureString(hasher, handoff.PreviewAssistantText)
	writeTranscriptSignatureString(hasher, handoff.AssistantText)
	writeTranscriptSignatureString(hasher, handoff.Error)
	writeTranscriptSignatureString(hasher, handoff.PermissionToolName)
	writeTranscriptSignatureString(hasher, handoff.PermissionPath)
	writeTranscriptSignatureString(hasher, handoff.PermissionDir)
	writeTranscriptSignatureString(hasher, handoff.QuestionToolName)
	writeTranscriptSignatureString(hasher, handoff.QuestionText)
	writeTranscriptSignatureBool(hasher, handoff.Reused)
	writeTranscriptSignatureString(hasher, handoff.ReusedContent)
}

func writeTranscriptSignatureString(hasher hash.Hash64, value string) {
	if hasher == nil {
		return
	}
	_, _ = io.WriteString(hasher, value)
	_, _ = hasher.Write([]byte{0})
}

func writeTranscriptSignatureInt(hasher hash.Hash64, value int) {
	writeTranscriptSignatureString(hasher, strconv.Itoa(value))
}

func writeTranscriptSignatureInt64(hasher hash.Hash64, value int64) {
	writeTranscriptSignatureString(hasher, strconv.FormatInt(value, 10))
}

func writeTranscriptSignatureUint64(hasher hash.Hash64, value uint64) {
	writeTranscriptSignatureString(hasher, strconv.FormatUint(value, 10))
}

func writeTranscriptSignatureBool(hasher hash.Hash64, value bool) {
	writeTranscriptSignatureString(hasher, strconv.FormatBool(value))
}

func toolResultDetailSignature(detail app.ToolResultDetail) string {
	hasher := fnv.New64a()
	writeTranscriptSignatureString(hasher, detail.Output)
	writeTranscriptSignatureString(hasher, detail.Error)
	return strconv.FormatUint(hasher.Sum64(), 16)
}
