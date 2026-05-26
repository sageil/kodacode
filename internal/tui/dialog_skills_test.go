package tui

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestSkillsDialogTogglesAndAppliesSelection(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	dialog := newSkillsDialog([]skillItem{
		{ID: "review", Description: "Review workflow", Source: "project"},
		{ID: "search", Description: "Search workflow", Source: "global"},
	}, []string{"review"}, &defaultTheme)
	dialog.SetFrame(120, 40)

	updated, _ := dialog.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	next, ok := updated.(*skillsDialog)
	if !ok {
		t.Fatalf("updated dialog = %T", updated)
	}
	updated, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	next, ok = updated.(*skillsDialog)
	if !ok {
		t.Fatalf("updated dialog after toggle = %T", updated)
	}
	if got := next.selectedSkillIDs(); !reflect.DeepEqual(got, []string{"review", "search"}) {
		t.Fatalf("selectedSkillIDs() after toggle = %#v", got)
	}

	updated, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	next, ok = updated.(*skillsDialog)
	if !ok {
		t.Fatalf("updated dialog after tab = %T", updated)
	}
	if next.focusedButtonIndex() != 0 {
		t.Fatalf("focusedButtonIndex() = %d, want 0", next.focusedButtonIndex())
	}
	_, closeCmd := next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if closeCmd == nil {
		t.Fatal("closeCmd = nil")
	}
	closed, ok := closeCmd().(dialogClosedMsg)
	if !ok {
		t.Fatalf("closeCmd() = %#v", closeCmd())
	}
	result, ok := closed.result.(skillsDialogResult)
	if !ok {
		t.Fatalf("closed.result = %#v", closed.result)
	}
	if !reflect.DeepEqual(result.SkillIDs, []string{"review", "search"}) {
		t.Fatalf("result.SkillIDs = %#v", result.SkillIDs)
	}
}
