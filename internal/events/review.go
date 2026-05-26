package events

import (
	"errors"
	"fmt"
	"strings"
)

const (
	ReviewSeverityP0 = "P0"
	ReviewSeverityP1 = "P1"
	ReviewSeverityP2 = "P2"
	ReviewSeverityP3 = "P3"

	ReviewOverallCorrectnessCorrect   = "correct"
	ReviewOverallCorrectnessIncorrect = "incorrect"
)

type ReviewFindingState struct {
	Severity    string
	Path        string
	Line        int
	Title       string
	Explanation string
}

type ReviewState struct {
	ReviewID           string
	SourceHandoffID    string
	Title              string
	Findings           []ReviewFindingState
	OverallCorrectness string
	OverallSummary     string
}

type ReviewFindingPayload struct {
	Severity    string `json:"severity"`
	Path        string `json:"path"`
	Line        int    `json:"line"`
	Title       string `json:"title"`
	Explanation string `json:"explanation"`
}

type ReviewRecordedPayload struct {
	ReviewID           string                 `json:"review_id,omitempty"`
	SourceHandoffID    string                 `json:"source_handoff_id,omitempty"`
	Title              string                 `json:"title"`
	Findings           []ReviewFindingPayload `json:"findings"`
	OverallCorrectness string                 `json:"overall_correctness"`
	OverallSummary     string                 `json:"overall_summary"`
}

func (ReviewRecordedPayload) eventType() Type { return TypeReviewRecorded }

func (p ReviewRecordedPayload) validate() error {
	if strings.TrimSpace(p.Title) == "" {
		return errors.New("title is required")
	}
	switch strings.TrimSpace(p.OverallCorrectness) {
	case ReviewOverallCorrectnessCorrect, ReviewOverallCorrectnessIncorrect:
	default:
		return errors.New("overall_correctness must be correct or incorrect")
	}
	if strings.TrimSpace(p.OverallSummary) == "" {
		return errors.New("overall_summary is required")
	}
	for idx, finding := range p.Findings {
		if err := validateReviewFindingPayload(finding); err != nil {
			return fmt.Errorf("findings[%d]: %w", idx, err)
		}
	}
	return nil
}

func ValidateReviewRecordedPayload(payload ReviewRecordedPayload) error {
	return payload.validate()
}

func validateReviewFindingPayload(finding ReviewFindingPayload) error {
	switch strings.TrimSpace(finding.Severity) {
	case ReviewSeverityP0, ReviewSeverityP1, ReviewSeverityP2, ReviewSeverityP3:
	default:
		return errors.New("severity must be P0, P1, P2, or P3")
	}
	if strings.TrimSpace(finding.Path) == "" {
		return errors.New("path is required")
	}
	if finding.Line <= 0 {
		return errors.New("line must be > 0")
	}
	if strings.TrimSpace(finding.Title) == "" {
		return errors.New("title is required")
	}
	if strings.TrimSpace(finding.Explanation) == "" {
		return errors.New("explanation is required")
	}
	return nil
}

func reviewStateFromPayload(payload ReviewRecordedPayload) *ReviewState {
	findings := make([]ReviewFindingState, 0, len(payload.Findings))
	for _, finding := range payload.Findings {
		findings = append(findings, ReviewFindingState{
			Severity:    strings.TrimSpace(finding.Severity),
			Path:        strings.TrimSpace(finding.Path),
			Line:        finding.Line,
			Title:       strings.TrimSpace(finding.Title),
			Explanation: strings.TrimSpace(finding.Explanation),
		})
	}
	if len(findings) == 0 {
		findings = nil
	}
	return &ReviewState{
		ReviewID:           strings.TrimSpace(payload.ReviewID),
		SourceHandoffID:    strings.TrimSpace(payload.SourceHandoffID),
		Title:              strings.TrimSpace(payload.Title),
		Findings:           findings,
		OverallCorrectness: strings.TrimSpace(payload.OverallCorrectness),
		OverallSummary:     strings.TrimSpace(payload.OverallSummary),
	}
}

func cloneReviewState(review *ReviewState) *ReviewState {
	if review == nil {
		return nil
	}
	out := &ReviewState{
		ReviewID:           review.ReviewID,
		SourceHandoffID:    review.SourceHandoffID,
		Title:              review.Title,
		OverallCorrectness: review.OverallCorrectness,
		OverallSummary:     review.OverallSummary,
	}
	if len(review.Findings) > 0 {
		out.Findings = append([]ReviewFindingState(nil), review.Findings...)
	}
	return out
}

func cloneReviewStates(reviews map[string]*ReviewState) map[string]*ReviewState {
	out := make(map[string]*ReviewState, len(reviews))
	for id, review := range reviews {
		out[id] = cloneReviewState(review)
	}
	return out
}
