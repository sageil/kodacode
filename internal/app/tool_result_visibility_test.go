package app

import (
	"testing"

	"github.com/sageil/kodacode/internal/provider"
)

func TestApplyToolResultVisibilityInstructionAddsNoteOnce(t *testing.T) {
	instructions := applyToolResultVisibilityInstruction("be precise", []provider.Input{{
		Kind:     provider.InputKindToolResult,
		ToolName: "read",
		Output:   "package main\n",
	}})

	want := "be precise\n\n" + toolResultVisibilityInstruction
	if instructions != want {
		t.Fatalf("instructions = %q, want %q", instructions, want)
	}
}

func TestApplyToolResultVisibilityInstructionSkipsWhenNoToolResult(t *testing.T) {
	instructions := applyToolResultVisibilityInstruction("be precise", []provider.Input{{
		Kind:    provider.InputKindUserMessage,
		Content: "show app.go",
	}})

	if instructions != "be precise" {
		t.Fatalf("instructions = %q", instructions)
	}
}

func TestApplyToolResultVisibilityInstructionDoesNotDuplicate(t *testing.T) {
	instructions := applyToolResultVisibilityInstruction(toolResultVisibilityInstruction, []provider.Input{{
		Kind:     provider.InputKindToolResult,
		ToolName: "read",
		Output:   "package main\n",
	}})

	if instructions != toolResultVisibilityInstruction {
		t.Fatalf("instructions = %q", instructions)
	}
}
