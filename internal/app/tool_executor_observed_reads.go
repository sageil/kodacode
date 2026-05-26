package app

import (
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func textMutationObservationsForResolvedPath(state events.SessionState, resolvedPath string) []events.ObservedResource {
	return newReadCoverageLedger(state).observedResourcesForPath(resolvedPath)
}

func callProvidesCurrentTextObservation(call *events.ToolCallState) bool {
	if call == nil || !call.Completed || !call.Succeeded {
		return false
	}
	switch strings.TrimSpace(call.ToolName) {
	case tool.ReadToolName, tool.WriteToolName:
		return true
	default:
		return false
	}
}
