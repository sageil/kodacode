package tui

import (
	"testing"

	"github.com/sageil/kodacode/internal/events"
)

func TestSearchSkillsAndSkillDisplayNames(t *testing.T) {
	if got := searchSkillsToolDisplayName(&events.ToolCallState{ToolName: "search_skills", Input: `{"query":"mongo migration","limit":5}`}); got != "search skills for mongo migration" {
		t.Fatalf("searchSkillsToolDisplayName() = %q", got)
	}
	if got := skillToolDisplayName(&events.ToolCallState{ToolName: "skill", Input: `{"id":"migration","section":"checklist"}`}); got != "load skill migration · checklist" {
		t.Fatalf("skillToolDisplayName() = %q", got)
	}
}
