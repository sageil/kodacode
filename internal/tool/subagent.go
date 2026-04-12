package tool

import (
	"context"
	"fmt"
	"strings"
)

// subagentArgs is the JSON shape the model sends when calling the subagent tool.
// Models frequently use the wrong key name, so we accept all common variants.
type subagentArgs struct {
	AgentID string `json:"agent_id"`
	Agent   string `json:"agent"`
	Tool    string `json:"tool"`
	Name    string `json:"name"`
	Task    string `json:"task"`
}

func (sa *subagentArgs) resolvedAgentID() string {
	for _, v := range []string{sa.AgentID, sa.Agent, sa.Tool, sa.Name} {
		if v != "" {
			return v
		}
	}
	return ""
}

// SubagentInfo describes an available agent for the subagent tool's parameter description.
type SubagentInfo struct {
	ID          string
	Description string
}

// NewSubagentTool creates a tool that lets the model spawn a subagent session
// with a specific agent to handle a delegated task. The subagent runs to
// completion and returns its final response.
func NewSubagentTool(agents []SubagentInfo) *Tool {
	agentDesc := "The ID of the agent to use."
	if len(agents) > 0 {
		var sb strings.Builder
		sb.WriteString(" Available agents: ")
		for i, a := range agents {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(a.ID)
			if a.Description != "" {
				sb.WriteString(" (")
				sb.WriteString(a.Description)
				sb.WriteString(")")
			}
		}
		agentDesc += sb.String()
	}

	params := fmt.Sprintf(`{
		"type": "object",
		"properties": {
			"agent_id": {
				"type": "string",
				"description": %q
			},
			"task": {
				"type": "string",
				"description": "The task description for the subagent. Be specific — the subagent only sees this text, not the parent conversation."
			}
		},
		"required": ["agent_id", "task"]
	}`, agentDesc)

	return &Tool{
		Name:        "subagent",
		Description: `Delegate a task to a specialized agent that runs independently. The subagent has its own tools and permissions. It receives only the task you provide — it does not see the parent conversation. IMPORTANT: Multiple subagent calls in the same response run concurrently. When you have independent tasks, you MUST emit all subagent tool calls in a single response so they execute in parallel — do NOT call one, wait for the result, then call the next. Prefer delegation over doing everything yourself: use planner before complex implementations, polish after writing code, insight after debugging sessions.`,
		Parameters:  []byte(params),
		Execute:     executeSubagent,
	}
}

func executeSubagent(ctx context.Context, ectx ExecutionContext, args []byte) (*Result, error) {
	var sa subagentArgs
	if err := flexUnmarshal(args, &sa); err != nil {
		return nil, fmt.Errorf("subagent: invalid arguments: %w", err)
	}
	agentID := sa.resolvedAgentID()
	if agentID == "" {
		return ErrorResult(ErrCodeInvalidArgs, "subagent: agent_id is required", false), nil
	}
	if sa.Task == "" {
		return ErrorResult(ErrCodeInvalidArgs, "subagent: task is required", false), nil
	}

	if ectx.SpawnSubagent == nil {
		return ErrorResult(ErrCodePermission, "subagent tool is not available to this agent. Use your own tools (read, read_files, task) to complete the work directly.", false), nil
	}

	response, err := ectx.SpawnSubagent(ctx, agentID, sa.Task)
	if err != nil {
		return nil, fmt.Errorf("subagent: %w", err)
	}

	// Response was already streamed via progress callbacks during execution,
	// so no need to write it again here.

	return &Result{
		Title:  fmt.Sprintf("subagent: %s", agentID),
		Output: response,
		Metadata: map[string]any{
			"agent_id": agentID,
		},
	}, nil
}
