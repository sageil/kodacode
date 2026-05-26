package tui

import (
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func mutationSummaryLabel(call *events.ToolCallState, success, pending string) string {
	if call.Completed && strings.TrimSpace(call.Error) == "" {
		return success
	}
	return pending
}

func onOffLabel(on bool) string {
	if on {
		return "on"
	}
	return "off"
}
