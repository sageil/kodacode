package tui

import (
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/textdiff"
)

func TestEditSummaryPrefersPreviewChangedLineOverStaleMutationRange(t *testing.T) {
	call := &events.ToolCallState{
		ToolName:  "edit",
		Input:     `{"path":"src/users.ts","old_text":"old\n","new_text":"new\n"}`,
		Output:    "edited line 7 in /repo/src/users.ts",
		Completed: true,
		Succeeded: true,
		MutationRanges: []events.MutationRange{{
			OldStartLine: 7,
			NewStartLine: 7,
		}},
		WriteMutation: &events.WriteMutation{
			Path:    "/repo/src/users.ts",
			Existed: true,
			DiffPreview: &textdiff.Preview{
				OldStartLine: 18,
				NewStartLine: 18,
				Ops: []textdiff.PreviewOp{
					{Kind: textdiff.OpContext, Text: "context before"},
					{Kind: textdiff.OpContext, Text: "more context"},
					{Kind: textdiff.OpInsert, Text: "new"},
					{Kind: textdiff.OpDelete, Text: "old"},
				},
			},
		},
	}

	if got := toolDisplayNameForWorkspace("/repo", call); got != "edit src/users.ts (+1 -1)" {
		t.Fatalf("toolDisplayNameForWorkspace() = %q", got)
	}
	if got := groupedToolItemResultDetail(call); got != "+1 -1" {
		t.Fatalf("groupedToolItemResultDetail() = %q", got)
	}
	input, ok := parseEditToolViewInput(call.Input)
	if !ok {
		t.Fatal("parseEditToolViewInput() = false")
	}
	if got := editMutationMetaLabel(call, input); got != "exact text match" {
		t.Fatalf("editMutationMetaLabel() = %q", got)
	}
	if got := editMutationDisplayLine(call); got != 20 {
		t.Fatalf("editMutationDisplayLine() = %d", got)
	}
}

func TestEditSummaryFallsBackToOutputWhenPreviewMissing(t *testing.T) {
	call := &events.ToolCallState{
		ToolName:  "edit",
		Input:     `{"path":"src/users.ts","old_text":"old\n","new_text":"new\n"}`,
		Output:    "edited line 172 in /repo/src/users.ts",
		Completed: true,
		Succeeded: true,
	}

	if got := toolDisplayNameForWorkspace("/repo", call); got != "edit src/users.ts (+1 -1)" {
		t.Fatalf("toolDisplayNameForWorkspace() = %q", got)
	}
	if got := groupedToolItemResultDetail(call); got != "+1 -1" {
		t.Fatalf("groupedToolItemResultDetail() = %q", got)
	}
	if got := editMutationDisplayLine(call); got != 172 {
		t.Fatalf("editMutationDisplayLine() = %d", got)
	}
}

func TestReadSummaryUsesSpacedLineRange(t *testing.T) {
	call := &events.ToolCallState{
		ToolName: "read",
		Input:    `{"paths":["src/cache.ts"],"start_line":12,"max_lines":20}`,
		Output:   "12: const cache = new Map()\n13: export function warmCache() {}\n(showing lines 12-13 of 80. Use start_line=14 to continue.)",
	}

	if got := groupedToolItemResultDetail(call); got != "lines 12-13" {
		t.Fatalf("groupedToolItemResultDetail() = %q", got)
	}
}

func TestReadSummaryUsesEOFFooterLineRange(t *testing.T) {
	call := &events.ToolCallState{
		ToolName: "read",
		Input:    `{"paths":["src/server.ts"],"start_line":1}`,
		Output:   "1: import express from 'express';\n74: app.listen(3000)\n(End of file - total 74 lines; shown lines 1-74)",
	}

	if got := groupedToolItemResultDetail(call); got != "lines 1-74" {
		t.Fatalf("groupedToolItemResultDetail() = %q", got)
	}
}

func TestReadSummaryUsesExplicitOffsetAndLimitInsteadOfOutputLineRange(t *testing.T) {
	call := &events.ToolCallState{
		ToolName: "read",
		Input:    `{"paths":["src/server.ts"],"offset":40,"limit":20}`,
		Output:   "41: app.get('/health', handler)\n60: app.listen(3000)\n(showing lines 41-60 of 120. Use offset=60 (0-based) to continue.)",
	}

	if got := groupedToolItemResultDetail(call); got != "offset 40 · limit 20" {
		t.Fatalf("groupedToolItemResultDetail() = %q", got)
	}
}

func TestFailedEditSummaryUsesCondensedStatusInsteadOfRawError(t *testing.T) {
	call := &events.ToolCallState{
		ToolName:  "edit",
		Input:     `{"path":"src/cacheMiddleware.ts","old_text":"old\n","new_text":"new\n"}`,
		Error:     "`edit` failed. path is required. Use exact old_text/new_text, optionally with start_line, or use edits[] with exact old_text/new_text entries.",
		Completed: true,
		Succeeded: false,
	}

	if got := groupedToolItemResultDetail(call); got != "fix args" {
		t.Fatalf("groupedToolItemResultDetail() = %q", got)
	}
	if got := groupedToolItemLabel("/repo", call); got != "Edit cacheMiddleware.ts" {
		t.Fatalf("groupedToolItemLabel() = %q", got)
	}
}
