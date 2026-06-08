package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const WorkflowReviewResultToolName = "workflow_review_result"

const (
	WorkflowReviewOverallCorrectnessCorrect   = "correct"
	WorkflowReviewOverallCorrectnessIncorrect = "incorrect"
)

var (
	ErrWorkflowReviewPassRequired               = errors.New("review_pass is required")
	ErrWorkflowReviewOverallCorrectnessInvalid  = errors.New("overall_correctness must be correct or incorrect")
	ErrWorkflowReviewOverallSummaryRequired     = errors.New("overall_summary is required")
	ErrWorkflowReviewFindingSeverityInvalid     = errors.New("finding severity must be P0, P1, P2, or P3")
	ErrWorkflowReviewFindingPathRequired        = errors.New("finding path is required")
	ErrWorkflowReviewFindingLineInvalid         = errors.New("finding line must be > 0")
	ErrWorkflowReviewFindingTitleRequired       = errors.New("finding title is required")
	ErrWorkflowReviewFindingExplanationRequired = errors.New("finding explanation is required")
)

type WorkflowReviewFinding struct {
	Severity    string `json:"severity"`
	Path        string `json:"path"`
	Line        int    `json:"line"`
	Title       string `json:"title"`
	Explanation string `json:"explanation"`
}

type WorkflowReviewResultRequest struct {
	ReviewPass         string
	Findings           []WorkflowReviewFinding
	OverallCorrectness string
	OverallSummary     string
}

type WorkflowReviewResultRecord struct {
	ReviewID   string `json:"review_id"`
	ReviewPass string `json:"review_pass"`
	Status     string `json:"status"`
	Message    string `json:"message"`
}

type WorkflowReviewResultManager interface {
	RecordWorkflowReviewResult(WorkflowReviewResultRequest) (WorkflowReviewResultRecord, error)
}

type WorkflowReviewResultTool struct{}

func NewWorkflowReviewResultTool() WorkflowReviewResultTool {
	return WorkflowReviewResultTool{}
}

func (WorkflowReviewResultTool) Definition() Definition {
	return Definition{
		Name:                WorkflowReviewResultToolName,
		Description:         "Record the validated result for a workflow review pass. Use exactly once at the end of a workflow review pass; this tool is the saved review result channel.",
		ProviderDescription: "Record the saved result for a workflow review pass. Call exactly once; do not return JSON in assistant text as the result channel.",
		InputSchema:         json.RawMessage(`{"type":"object","properties":{"review_pass":{"type":"string","description":"Workflow review pass id, such as correctness, tests, or contracts."},"findings":{"type":"array","items":{"type":"object","properties":{"severity":{"type":"string","enum":["P0","P1","P2","P3"]},"path":{"type":"string"},"line":{"type":"integer","minimum":1},"title":{"type":"string"},"explanation":{"type":"string"}},"required":["severity","path","line","title","explanation"],"additionalProperties":false}},"overall_correctness":{"type":"string","enum":["correct","incorrect"]},"overall_summary":{"type":"string","description":"One to three sentence pass summary."}},"required":["review_pass","findings","overall_correctness","overall_summary"],"additionalProperties":false}`),
		ProviderInputSchema: json.RawMessage(`{"type":"object","properties":{"review_pass":{"type":"string"},"findings":{"type":"array","items":{"type":"object","properties":{"severity":{"type":"string","enum":["P0","P1","P2","P3"]},"path":{"type":"string"},"line":{"type":"integer","minimum":1},"title":{"type":"string"},"explanation":{"type":"string"}},"required":["severity","path","line","title","explanation"],"additionalProperties":false}},"overall_correctness":{"type":"string","enum":["correct","incorrect"]},"overall_summary":{"type":"string"}},"required":["review_pass","findings","overall_correctness","overall_summary"],"additionalProperties":false}`),
		ArgumentExamples: []string{
			`{"review_pass":"correctness","findings":[],"overall_correctness":"correct","overall_summary":"No correctness regressions found."}`,
		},
		ProviderRichGuidance: true,
	}
}

func (WorkflowReviewResultTool) Execute(_ context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	manager, err := ectx.WorkflowReviewResult()
	if err != nil {
		return Result{}, err
	}
	input, err := parseWorkflowReviewResultInput(args)
	if err != nil {
		return Result{}, err
	}
	record, err := manager.RecordWorkflowReviewResult(WorkflowReviewResultRequest(input))
	if err != nil {
		return Result{}, err
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return Result{}, err
	}
	return Result{Output: string(encoded)}, nil
}

type workflowReviewResultInput struct {
	ReviewPass         string
	Findings           []WorkflowReviewFinding
	OverallCorrectness string
	OverallSummary     string
}

func parseWorkflowReviewResultInput(args json.RawMessage) (workflowReviewResultInput, error) {
	var raw struct {
		ReviewPass         *string                 `json:"review_pass"`
		Findings           []WorkflowReviewFinding `json:"findings"`
		OverallCorrectness *string                 `json:"overall_correctness"`
		OverallSummary     *string                 `json:"overall_summary"`
	}
	if err := DecodeArgs(WorkflowReviewResultToolName, args, &raw); err != nil {
		return workflowReviewResultInput{}, err
	}
	input := workflowReviewResultInput{
		ReviewPass:         strings.TrimSpace(stringValue(raw.ReviewPass)),
		Findings:           normalizeWorkflowReviewFindings(raw.Findings),
		OverallCorrectness: strings.TrimSpace(stringValue(raw.OverallCorrectness)),
		OverallSummary:     strings.TrimSpace(stringValue(raw.OverallSummary)),
	}
	if input.ReviewPass == "" {
		return workflowReviewResultInput{}, InvalidArguments(WorkflowReviewResultToolName, ErrWorkflowReviewPassRequired)
	}
	switch input.OverallCorrectness {
	case WorkflowReviewOverallCorrectnessCorrect, WorkflowReviewOverallCorrectnessIncorrect:
	default:
		return workflowReviewResultInput{}, InvalidArguments(WorkflowReviewResultToolName, ErrWorkflowReviewOverallCorrectnessInvalid)
	}
	if input.OverallSummary == "" {
		return workflowReviewResultInput{}, InvalidArguments(WorkflowReviewResultToolName, ErrWorkflowReviewOverallSummaryRequired)
	}
	for idx, finding := range input.Findings {
		if err := validateWorkflowReviewFinding(finding); err != nil {
			return workflowReviewResultInput{}, InvalidArguments(WorkflowReviewResultToolName, fmt.Errorf("findings[%d]: %w", idx, err))
		}
	}
	return input, nil
}

func normalizeWorkflowReviewFindings(findings []WorkflowReviewFinding) []WorkflowReviewFinding {
	if len(findings) == 0 {
		return nil
	}
	out := make([]WorkflowReviewFinding, 0, len(findings))
	for _, finding := range findings {
		out = append(out, WorkflowReviewFinding{
			Severity:    strings.TrimSpace(finding.Severity),
			Path:        strings.TrimSpace(finding.Path),
			Line:        finding.Line,
			Title:       strings.TrimSpace(finding.Title),
			Explanation: strings.TrimSpace(finding.Explanation),
		})
	}
	return out
}

func validateWorkflowReviewFinding(finding WorkflowReviewFinding) error {
	switch finding.Severity {
	case "P0", "P1", "P2", "P3":
	default:
		return ErrWorkflowReviewFindingSeverityInvalid
	}
	if finding.Path == "" {
		return ErrWorkflowReviewFindingPathRequired
	}
	if finding.Line <= 0 {
		return ErrWorkflowReviewFindingLineInvalid
	}
	if finding.Title == "" {
		return ErrWorkflowReviewFindingTitleRequired
	}
	if finding.Explanation == "" {
		return ErrWorkflowReviewFindingExplanationRequired
	}
	return nil
}
