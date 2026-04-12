package service

import (
	"github.com/sageil/kodacode/v1/internal/provider"
)

func plannerBlockedReason(_ any, agentID string, ephemeral bool, calls []provider.ToolCall) string {
	if ephemeral || !isEngineerWorkflowAgent(agentID) {
		return ""
	}
	if batchCallsAgent(calls, "explorer") {
		return "Planner cannot run in the same response as explorer. Wait for the explorer results, then call planner in a later response."
	}
	return ""
}

func batchCallsAgent(calls []provider.ToolCall, agentID string) bool {
	for _, tc := range calls {
		if tc.Name == "subagent" && plannerAgentIDFromArgs(tc.Arguments) == agentID {
			return true
		}
	}
	return false
}
