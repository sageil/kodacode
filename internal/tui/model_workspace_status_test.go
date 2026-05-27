package tui

import (
	"testing"

	"github.com/sageil/kodacode/internal/events"
)

func TestShouldRefreshWorkspaceStatusForWorkspaceWriteRestoredEvent(t *testing.T) {
	if !shouldRefreshWorkspaceStatusForEvent(draftEvent(3, events.TypeWorkspaceWriteRestored, "session-1", "_session", events.WorkspaceWriteRestoredPayload{
		SourceTurnID: "turn-2",
		Restores: []events.WorkspaceWriteRestoreItem{{
			CallID: "call-1",
			Path:   "/repo/notes.txt",
		}},
	})) {
		t.Fatal("shouldRefreshWorkspaceStatusForEvent() = false, want true for workspace_write_restored")
	}
}

func TestShouldRefreshWorkspaceStatusForWriteToolEvent(t *testing.T) {
	if !shouldRefreshWorkspaceStatusForEvent(draftEvent(4, events.TypeToolExecEnd, "session-1", "turn-1", events.ToolExecEndPayload{
		CallID:   "call-2",
		ToolName: "write",
		Output:   "ok",
	})) {
		t.Fatal("shouldRefreshWorkspaceStatusForEvent() = false, want true for write tool completion")
	}
}

func TestShouldRefreshWorkspaceStatusForSearchToolEvent(t *testing.T) {
	if !shouldRefreshWorkspaceStatusForEvent(draftEvent(5, events.TypeToolExecEnd, "session-1", "turn-1", events.ToolExecEndPayload{
		CallID:   "call-3",
		ToolName: "search",
		Output:   "ok",
	})) {
		t.Fatal("shouldRefreshWorkspaceStatusForEvent() = false, want true for search tool completion")
	}
}
