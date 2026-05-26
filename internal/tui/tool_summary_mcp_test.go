package tui

import (
	"testing"

	"github.com/sageil/kodacode/internal/events"
)

func TestToolDisplayNameForSessionUsesMCPServerName(t *testing.T) {
	state := events.SessionState{
		WorkspaceRoot: "/repo",
		MCP: &events.SessionMCPState{
			Servers: []events.SessionMCPServerPayload{{
				Name:    "sequential-thinking",
				Type:    "stdio",
				Trusted: true,
				Active:  true,
			}},
			Tools: []events.SessionMCPToolPayload{{
				Name:       "mcp_sequential_thinking__sequentialthinking",
				ServerName: "sequential-thinking",
				RemoteName: "sequentialthinking",
			}},
		},
	}
	call := &events.ToolCallState{
		ToolName: "mcp_sequential_thinking__sequentialthinking",
	}

	if got := toolDisplayNameForSession(state, call); got != "sequential-thinking" {
		t.Fatalf("toolDisplayNameForSession() = %q, want %q", got, "sequential-thinking")
	}
}

func TestToolDetailDialogTitleUsesMCPServerName(t *testing.T) {
	state := events.SessionState{
		WorkspaceRoot: "/repo",
		MCP: &events.SessionMCPState{
			Servers: []events.SessionMCPServerPayload{{
				Name:    "sequential-thinking",
				Type:    "stdio",
				Trusted: true,
				Active:  true,
			}},
			Tools: []events.SessionMCPToolPayload{{
				Name:       "mcp_sequential_thinking__sequentialthinking",
				ServerName: "sequential-thinking",
				RemoteName: "sequentialthinking",
			}},
		},
	}
	call := &events.ToolCallState{
		ToolName: "mcp_sequential_thinking__sequentialthinking",
	}

	if got := toolDetailDialogTitle(state, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, call); got != "sequential-thinking" {
		t.Fatalf("toolDetailDialogTitle() = %q, want %q", got, "sequential-thinking")
	}
}

func TestToolDisplayNameForSessionFallsBackWithoutMCPCatalog(t *testing.T) {
	call := &events.ToolCallState{
		ToolName: "mcp_sequential_thinking__sequentialthinking",
	}

	if got := toolDisplayNameForSession(events.SessionState{}, call); got != "sequential-thinking" {
		t.Fatalf("toolDisplayNameForSession() = %q, want %q", got, "sequential-thinking")
	}
}

func TestToolDisplayNameForWorkspaceFallsBackWithoutMCPCatalog(t *testing.T) {
	call := &events.ToolCallState{
		ToolName: "mcp_sequential_thinking__sequentialthinking",
	}

	if got := toolDisplayNameForWorkspace("/repo", call); got != "sequential-thinking" {
		t.Fatalf("toolDisplayNameForWorkspace() = %q, want %q", got, "sequential-thinking")
	}
}
