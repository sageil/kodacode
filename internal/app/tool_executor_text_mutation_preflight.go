package app

import (
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
	"github.com/sageil/kodacode/internal/workspace"
)

func runTextMutationPreflightChecks(state events.SessionState, scope *workspace.Scope, input ExecuteToolInput) error {
	switch strings.TrimSpace(input.ToolName) {
	case tool.WriteToolName:
		return requireCompleteFreshReadBeforeWrite(state, scope, input)
	default:
		return nil
	}
}
