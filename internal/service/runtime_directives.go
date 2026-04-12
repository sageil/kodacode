package service

import (
	"strings"

	"github.com/sageil/kodacode/v1/internal/pipeline"
)

const reviewRuntimeDirectiveMarker = "## RUNTIME REVIEW DIRECTIVE"
const reviewRuntimeDirectiveEndMarker = "## END RUNTIME REVIEW DIRECTIVE"
const workflowRuntimeDirectiveMarker = "## RUNTIME WORKFLOW DIRECTIVE"
const workflowRuntimeDirectiveEndMarker = "## END RUNTIME WORKFLOW DIRECTIVE"
const phaseRuntimeDirectiveMarker = "## RUNTIME PHASE DIRECTIVE"
const phaseRuntimeDirectiveEndMarker = "## END RUNTIME PHASE DIRECTIVE"

func setRuntimeDirective(req *pipeline.TurnRequest, marker, endMarker, text string) {
	if req == nil {
		return
	}
	for len(req.SystemParts) < 3 {
		req.SystemParts = append(req.SystemParts, "")
	}
	volatile := stripRuntimeDirective(req.SystemParts[2], marker, endMarker)
	if strings.TrimSpace(text) == "" {
		req.SystemParts[2] = strings.TrimSpace(volatile)
		return
	}
	block := marker + "\n" + strings.TrimSpace(text) + "\n" + endMarker
	if strings.TrimSpace(volatile) == "" {
		req.SystemParts[2] = block
		return
	}
	req.SystemParts[2] = strings.TrimSpace(volatile) + "\n\n" + block
}

func getRuntimeDirective(volatile, marker, endMarker string) string {
	start := strings.Index(volatile, marker)
	if start < 0 {
		return ""
	}
	start += len(marker)
	end := strings.Index(volatile[start:], endMarker)
	if end < 0 {
		return strings.TrimSpace(volatile[start:])
	}
	return strings.TrimSpace(volatile[start : start+end])
}

func appendRuntimeDirective(req *pipeline.TurnRequest, marker, endMarker, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	current := ""
	if req != nil && len(req.SystemParts) > 2 {
		current = getRuntimeDirective(req.SystemParts[2], marker, endMarker)
	}
	if current == "" {
		setRuntimeDirective(req, marker, endMarker, text)
		return
	}
	setRuntimeDirective(req, marker, endMarker, current+"\n\n"+strings.TrimSpace(text))
}

func stripRuntimeDirective(volatile, marker, endMarker string) string {
	start := strings.Index(volatile, marker)
	if start < 0 {
		return volatile
	}
	end := strings.Index(volatile[start:], endMarker)
	if end < 0 {
		return strings.TrimSpace(volatile[:start])
	}
	end += start + len(endMarker)
	before := strings.TrimSpace(volatile[:start])
	after := strings.TrimSpace(volatile[end:])
	switch {
	case before == "":
		return after
	case after == "":
		return before
	default:
		return before + "\n\n" + after
	}
}

func setReviewRuntimeDirective(req *pipeline.TurnRequest, text string) {
	setRuntimeDirective(req, reviewRuntimeDirectiveMarker, reviewRuntimeDirectiveEndMarker, text)
}

func setWorkflowRuntimeDirective(req *pipeline.TurnRequest, text string) {
	if strings.TrimSpace(text) == "" {
		setRuntimeDirective(req, workflowRuntimeDirectiveMarker, workflowRuntimeDirectiveEndMarker, "")
		return
	}
	appendRuntimeDirective(req, workflowRuntimeDirectiveMarker, workflowRuntimeDirectiveEndMarker, text)
}

func setPhaseRuntimeDirective(req *pipeline.TurnRequest, text string) {
	setRuntimeDirective(req, phaseRuntimeDirectiveMarker, phaseRuntimeDirectiveEndMarker, text)
}
