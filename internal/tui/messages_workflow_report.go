package tui

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func shouldRenderCompletedWorkflowReport(state events.SessionState) bool {
	workflow := state.Workflow
	return workflow != nil &&
		strings.TrimSpace(workflow.WorkflowID) != "" &&
		strings.TrimSpace(workflow.Status) == events.WorkflowStatusCompleted
}

func renderCompletedWorkflowReportSections(m Model, state events.SessionState, width int) []transcriptSection {
	if !shouldRenderCompletedWorkflowReport(state) {
		return nil
	}
	body := completedWorkflowReportBody(state.Workflow, width)
	if strings.TrimSpace(body) == "" {
		return nil
	}
	content := renderTranscriptBlock(m, "Workflow report", body, width, transcriptBlockStyle{
		accent: colorFor(m.theme, "success", "#90e5b4"),
	})
	if strings.TrimSpace(content) == "" {
		return nil
	}
	return []transcriptSection{{
		content:        content,
		selectionLines: transcriptSelectionLinesForContent(content),
	}}
}

func completedWorkflowReportBody(workflow *events.WorkflowState, width int) string {
	if workflow == nil {
		return ""
	}
	workflowID := strings.TrimSpace(workflow.WorkflowID)
	if workflowID == "" {
		return ""
	}
	lines := []string{fmt.Sprintf("Workflow %s completed.", workflowID)}
	if reason := workflowStopReason(workflow); reason != "" {
		lines = append(lines, "Stop reason: "+workflowReportDisplayValue(reason))
	}
	if len(workflow.EvidenceOrder) == 0 {
		lines = append(lines, "No workflow evidence recorded.")
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "", "Evidence:")
	for _, evidenceID := range workflow.EvidenceOrder {
		evidence := workflow.Evidence[evidenceID]
		if evidence == nil {
			continue
		}
		if shouldHideCompletedWorkflowEvidence(workflow, evidence) {
			continue
		}
		line := completedWorkflowEvidenceLine(evidence)
		if line == "" {
			continue
		}
		lines = append(lines, "- "+line)
		for _, field := range completedWorkflowEvidenceFieldLines(evidence, width) {
			lines = append(lines, field)
		}
	}
	return strings.Join(lines, "\n")
}

func shouldHideCompletedWorkflowEvidence(workflow *events.WorkflowState, evidence *events.WorkflowEvidenceState) bool {
	if workflow == nil || evidence == nil {
		return false
	}
	if strings.TrimSpace(evidence.Type) != events.WorkflowEvidenceTypePhaseOutput {
		return false
	}
	if strings.TrimSpace(evidence.PhaseID) != strings.TrimSpace(workflow.CurrentPhaseID) {
		return false
	}
	summary := strings.TrimSpace(evidence.Summary)
	return strings.HasPrefix(summary, "Workflow ") && strings.Contains(summary, " completed.")
}

func completedWorkflowEvidenceLine(evidence *events.WorkflowEvidenceState) string {
	if evidence == nil {
		return ""
	}
	parts := []string{workflowEvidenceDisplayType(evidence.Type)}
	if phaseID := strings.TrimSpace(evidence.PhaseID); phaseID != "" {
		parts = append(parts, "phase "+phaseID)
	}
	if status := workflowEvidenceStatus(evidence); status != "" {
		parts = append(parts, status)
	}
	if command := workflowReportDisplayValue(evidence.Command); command != "" {
		parts = append(parts, command)
	}
	if summary := workflowReportDisplayValue(evidence.Summary); summary != "" {
		parts = append(parts, summary)
	}
	return strings.Join(nonBlankWorkflowReportParts(parts), " | ")
}

func completedWorkflowEvidenceFieldLines(evidence *events.WorkflowEvidenceState, width int) []string {
	if evidence == nil || len(evidence.Fields) == 0 {
		return nil
	}
	keys := make([]string, 0, len(evidence.Fields))
	for key, value := range evidence.Fields {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		if shouldHideCompletedWorkflowEvidenceField(evidence, key) {
			continue
		}
		value := workflowReportFieldDisplayValue(evidence, evidence.Fields[key])
		if value == "" {
			continue
		}
		if workflowShouldExpandEvidenceFields(evidence) {
			lines = append(lines, "  "+strings.TrimSpace(key)+":")
			for _, line := range wrapStructuredText(value, max(width-6, 24)) {
				lines = append(lines, "    "+line)
			}
			continue
		}
		lines = append(lines, "  "+strings.TrimSpace(key)+": "+value)
	}
	return lines
}

func workflowShouldExpandEvidenceFields(evidence *events.WorkflowEvidenceState) bool {
	if evidence == nil {
		return false
	}
	return strings.TrimSpace(evidence.Type) == events.WorkflowEvidenceTypePhaseOutput
}

func shouldHideCompletedWorkflowEvidenceField(evidence *events.WorkflowEvidenceState, key string) bool {
	switch strings.TrimSpace(evidence.Type) {
	case events.WorkflowEvidenceTypeReviewOutcome, events.WorkflowEvidenceTypeReview, events.WorkflowEvidenceTypeTaskReview:
		switch strings.TrimSpace(key) {
		case "review_pass", "review_status", "source":
			return true
		}
	}
	return false
}

func workflowEvidenceStatus(evidence *events.WorkflowEvidenceState) string {
	if evidence == nil {
		return ""
	}
	switch strings.TrimSpace(evidence.Type) {
	case events.WorkflowEvidenceTypeReviewOutcome, events.WorkflowEvidenceTypeReview, events.WorkflowEvidenceTypeTaskReview:
		passID := workflowReportDisplayValue(evidence.Fields["review_pass"])
		status := workflowReportDisplayValue(evidence.Fields["review_status"])
		if status == "" && evidence.Successful != nil {
			if *evidence.Successful {
				status = "pass"
			} else {
				status = "fail"
			}
		}
		if passID != "" && status != "" {
			return passID + " " + status
		}
		if passID != "" {
			return passID
		}
		return status
	default:
		if evidence.Successful == nil {
			return ""
		}
		if *evidence.Successful {
			return "successful"
		}
		return "failed"
	}
}

func workflowEvidenceDisplayType(evidenceType string) string {
	switch strings.TrimSpace(evidenceType) {
	case events.WorkflowEvidenceTypeApproval:
		return "approval"
	case events.WorkflowEvidenceTypeDiagnostics:
		return "diagnostics"
	case events.WorkflowEvidenceTypeGitDiff:
		return "git diff"
	case events.WorkflowEvidenceTypePhaseFailure:
		return "phase failure"
	case events.WorkflowEvidenceTypePhaseOutput:
		return "phase output"
	case events.WorkflowEvidenceTypeReview:
		return "review"
	case events.WorkflowEvidenceTypeReviewOutcome:
		return "review outcome"
	case events.WorkflowEvidenceTypeRevisionTrigger:
		return "revision trigger"
	case events.WorkflowEvidenceTypeTaskReview:
		return "task review"
	case events.WorkflowEvidenceTypeVerificationResult:
		return "verification"
	default:
		return strings.TrimSpace(evidenceType)
	}
}

func workflowReportDisplayValue(value string) string {
	value = workflowReportNormalizeValue(value)
	if value == "" {
		return ""
	}
	return summarizeInlineValue(value)
}

func workflowReportFieldDisplayValue(evidence *events.WorkflowEvidenceState, value string) string {
	value = workflowReportNormalizeValue(value)
	if value == "" {
		return ""
	}
	if workflowShouldExpandEvidenceFields(evidence) {
		return value
	}
	return summarizeInlineValue(value)
}

func workflowReportNormalizeValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var stringValues []string
	if err := json.Unmarshal([]byte(value), &stringValues); err == nil {
		values := make([]string, 0, len(stringValues))
		for _, item := range stringValues {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				values = append(values, trimmed)
			}
		}
		if len(values) > 0 {
			return strings.Join(values, ", ")
		}
	}
	return value
}

func nonBlankWorkflowReportParts(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
