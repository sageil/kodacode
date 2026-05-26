package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/textdiff"
)

func TestRenderWriteMutationLinesUsesRuntimeDiff(t *testing.T) {
	model := newMutationRenderTestModel(t)
	call := &events.ToolCallState{
		ToolName: "write",
		Input:    `{"path":"src/app.ts","content":"const value = 2;\nconst next = 3;\n"}`,
		WriteMutation: &events.WriteMutation{
			Path:    "/repo/src/app.ts",
			Existed: true,
			Before:  "const value = 1;\nconst next = 3;\n",
		},
	}

	rendered := ansi.Strip(strings.Join(renderWriteMutationLines(model, "/repo", call, 80), "\n"))
	if !strings.Contains(rendered, "Wrote src/app.ts") {
		t.Fatalf("rendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "- const value = 1;") || !strings.Contains(rendered, "+ const value = 2;") {
		t.Fatalf("rendered:\n%s", rendered)
	}
	if strings.Contains(rendered, "diff unavailable") {
		t.Fatalf("rendered:\n%s", rendered)
	}
}

func TestRenderWriteMutationLinesUsesCreatedLabelForNewFile(t *testing.T) {
	model := newMutationRenderTestModel(t)
	call := &events.ToolCallState{
		ToolName: "write",
		Input:    `{"path":"src/new.ts","content":"export const value = 1;\n"}`,
		WriteMutation: &events.WriteMutation{
			Path:    "/repo/src/new.ts",
			Existed: false,
			Before:  "",
		},
	}

	rendered := ansi.Strip(strings.Join(renderWriteMutationLines(model, "/repo", call, 80), "\n"))
	if !strings.Contains(rendered, "Created src/new.ts") {
		t.Fatalf("rendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "+ export const value = 1;") {
		t.Fatalf("rendered:\n%s", rendered)
	}
}

func TestRenderWriteMutationLinesUsesStoredPreviewWhenBeforeWasOffloaded(t *testing.T) {
	model := newMutationRenderTestModel(t)
	call := &events.ToolCallState{
		ToolName: "write",
		Input:    `{"path":"src/cache.ts","content":"one\ntwo updated\nthree\nfour\nfive\nsix\nseven changed\neight\n"}`,
		WriteMutation: &events.WriteMutation{
			Path:            "/repo/src/cache.ts",
			Existed:         true,
			Before:          "[output truncated]",
			BeforeTruncated: true,
			DiffPreview: &textdiff.Preview{
				OldStartLine: 1,
				NewStartLine: 1,
				Ops: []textdiff.PreviewOp{
					{Kind: textdiff.OpContext, Text: "one"},
					{Kind: textdiff.OpDelete, Text: "two"},
					{Kind: textdiff.OpInsert, Text: "two updated"},
					{Kind: textdiff.OpContext, Text: "three"},
					{Kind: textdiff.OpSkip},
					{Kind: textdiff.OpContext, Text: "six"},
					{Kind: textdiff.OpDelete, Text: "seven"},
					{Kind: textdiff.OpInsert, Text: "seven changed"},
					{Kind: textdiff.OpContext, Text: "eight"},
				},
			},
		},
	}

	rendered := ansi.Strip(strings.Join(renderWriteMutationLines(model, "/repo", call, 80), "\n"))
	for _, want := range []string{
		"Wrote src/cache.ts",
		"- two",
		"+ two updated",
		"- seven",
		"+ seven changed",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered missing %q\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "diff unavailable in transcript for large write") {
		t.Fatalf("rendered fell back to unavailable message:\n%s", rendered)
	}
}

func TestRenderApplyPatchMutationLinesUsesEveryStoredPreview(t *testing.T) {
	model := newMutationRenderTestModel(t)
	call := &events.ToolCallState{
		ToolName: "apply_patch",
		WriteMutations: []events.WriteMutation{
			{
				Path:    "/repo/src/app.ts",
				Existed: true,
				DiffPreview: &textdiff.Preview{
					OldStartLine: 1,
					NewStartLine: 1,
					Ops: []textdiff.PreviewOp{
						{Kind: textdiff.OpDelete, Text: "const value = 1;"},
						{Kind: textdiff.OpInsert, Text: "const value = 2;"},
					},
				},
			},
			{
				Path:    "/repo/src/new.ts",
				Existed: false,
				DiffPreview: &textdiff.Preview{
					OldStartLine: 1,
					NewStartLine: 1,
					Ops: []textdiff.PreviewOp{
						{Kind: textdiff.OpInsert, Text: "export const created = true;"},
					},
				},
			},
		},
	}

	rendered := ansi.Strip(strings.Join(renderApplyPatchMutationLines(model, "/repo", call, 80), "\n"))
	for _, want := range []string{
		"Changed src/app.ts (+1 -1)",
		"- const value = 1;",
		"+ const value = 2;",
		"Created src/new.ts (+1 -0)",
		"+ export const created = true;",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered missing %q\n%s", want, rendered)
		}
	}
}

func TestRenderBashMutationLinesUsesRuntimeDiffPreview(t *testing.T) {
	model := newMutationRenderTestModel(t)
	call := &events.ToolCallState{
		ToolName: "bash",
		Input:    `{"cmd":"printf value=2 > src/app.env"}`,
		WriteMutation: &events.WriteMutation{
			Path:    "/repo/src/app.env",
			Existed: true,
			DiffPreview: &textdiff.Preview{
				OldStartLine: 1,
				NewStartLine: 1,
				Ops: []textdiff.PreviewOp{
					{Kind: textdiff.OpDelete, Text: "value=1"},
					{Kind: textdiff.OpInsert, Text: "value=2"},
				},
			},
		},
	}

	rendered := ansi.Strip(strings.Join(renderBashMutationLines(model, "/repo", call, 80), "\n"))
	for _, want := range []string{
		"Changed src/app.env",
		"- value=1",
		"+ value=2",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered missing %q\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "printf value=2") {
		t.Fatalf("rendered leaked shell command:\n%s", rendered)
	}
}

func TestRenderEditMutationLinesUsesRuntimeDiff(t *testing.T) {
	model := newMutationRenderTestModel(t)
	call := &events.ToolCallState{
		ToolName: "edit",
		Input:    `{"path":"src/app.ts","start_line":"2","old_text":"const value = 1;\n","new_text":"const value = 2;\n"}`,
		WriteMutation: &events.WriteMutation{
			Path:    "/repo/src/app.ts",
			Existed: true,
			Before:  "function run() {\nconst value = 1;\n}\n",
			DiffPreview: &textdiff.Preview{
				OldStartLine: 2,
				NewStartLine: 2,
				Ops: []textdiff.PreviewOp{
					{Kind: textdiff.OpDelete, Text: "const value = 1;"},
					{Kind: textdiff.OpInsert, Text: "const value = 2;"},
				},
			},
		},
	}

	rendered := ansi.Strip(strings.Join(renderEditMutationLines(model, "/repo", call, 80), "\n"))
	for _, want := range []string{
		"Edited src/app.ts (+1 -1)",
		"exact text match",
		"- const value = 1;",
		"+ const value = 2;",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered missing %q\n%s", want, rendered)
		}
	}
}

func TestRenderEditMutationLinesWithoutAnchorDoesNotInventLineOne(t *testing.T) {
	model := newMutationRenderTestModel(t)
	call := &events.ToolCallState{
		ToolName: "edit",
		Input:    `{"path":"src/app.ts","old_text":"const value = 1;\n","new_text":"const value = 2;\n"}`,
	}

	rendered := ansi.Strip(strings.Join(renderEditMutationLines(model, "/repo", call, 80), "\n"))
	if !strings.Contains(rendered, "exact text match") {
		t.Fatalf("rendered missing exact-match label\n%s", rendered)
	}
	for _, unwanted := range []string{
		"edited line 1",
		"\n  1 -",
		"\n  1 +",
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("rendered unexpectedly invented line 1 anchor %q\n%s", unwanted, rendered)
		}
	}
}
