package tool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/workspace"
)

func TestApplyPatchToolDefinitionIsCustomFreeform(t *testing.T) {
	definition := NewApplyPatchTool().Definition()
	if definition.Name != ApplyPatchToolName || definition.InputKindOrDefault() != InputKindCustom {
		t.Fatalf("definition = %#v", definition)
	}
	if definition.InputFormat == nil || definition.InputFormat.Type != "grammar" || definition.InputFormat.Syntax != "lark" {
		t.Fatalf("input format = %#v", definition.InputFormat)
	}
	for _, want := range []string{
		"raw structured patch text",
		"custom/freeform tool",
		"not a JSON object",
		`"*** Add File:"`,
		`"*** Update File:"`,
		`every file-content line must start with "+"`,
		`Patch lines MUST NOT include read output line number prefixes like "40:"`,
		`after patch prefixes like "-40:" or "+40:"`,
		"required patch grammar",
	} {
		if !strings.Contains(definition.Description, want) {
			t.Fatalf("description missing %q: %q", want, definition.Description)
		}
	}
	if len(definition.ArgumentExamples) < 4 {
		t.Fatalf("ArgumentExamples = %#v, want examples for add/update/delete/move", definition.ArgumentExamples)
	}
}

func TestDefaultRuntimeToolsIncludesApplyPatch(t *testing.T) {
	var names []string
	for _, tl := range DefaultRuntimeTools() {
		names = append(names, tl.Definition().Name)
	}
	if !slices.Contains(names, ApplyPatchToolName) {
		t.Fatalf("DefaultRuntimeTools missing %s: %v", ApplyPatchToolName, names)
	}
}

func TestDefaultRuntimeToolsExcludesEdit(t *testing.T) {
	for _, tl := range DefaultRuntimeTools() {
		if tl.Definition().Name == "edit" {
			t.Fatalf("DefaultRuntimeTools includes removed edit tool")
		}
	}
}

func TestDefaultRuntimeToolsExcludesDelegate(t *testing.T) {
	for _, tl := range DefaultRuntimeTools() {
		if tl.Definition().Name == "delegate" {
			t.Fatalf("DefaultRuntimeTools includes removed delegate tool")
		}
	}
}

func TestApplyPatchToolAppliesAddDeleteUpdateAndMove(t *testing.T) {
	root := t.TempDir()
	writeApplyPatchTestFile(t, root, "src/update.txt", "one\nold\nthree\n")
	writeApplyPatchTestFile(t, root, "src/delete.txt", "remove me\n")
	writeApplyPatchTestFile(t, root, "src/move.txt", "alpha\nold move\nomega\n")

	result, err := NewApplyPatchTool().Execute(context.Background(), applyPatchExecutionContext(t, root), rawPatch(`*** Begin Patch
*** Add File: docs/new.md
+# New
+
+content
*** Update File: src/update.txt
@@
-old
+new
*** Delete File: src/delete.txt
*** Update File: src/move.txt
*** Move to: src/moved.txt
@@
-old move
+new move
*** End Patch
`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "A docs/new.md") || !strings.Contains(result.Output, "D src/delete.txt") || !strings.Contains(result.Output, "M src/move.txt -> src/moved.txt") {
		t.Fatalf("output = %q", result.Output)
	}
	assertFileContent(t, root, "docs/new.md", "# New\n\ncontent\n")
	assertFileContent(t, root, "src/update.txt", "one\nnew\nthree\n")
	assertFileMissing(t, root, "src/delete.txt")
	assertFileMissing(t, root, "src/move.txt")
	assertFileContent(t, root, "src/moved.txt", "alpha\nnew move\nomega\n")

	var structured ApplyPatchStructuredResult
	if err := json.Unmarshal(result.StructuredResult, &structured); err != nil {
		t.Fatalf("structured result unmarshal error = %v", err)
	}
	if len(structured.ChangedFiles) != 4 {
		t.Fatalf("changed files = %#v", structured.ChangedFiles)
	}
}

func TestApplyPatchToolReturnsRuntimeTextMutations(t *testing.T) {
	root := t.TempDir()
	writeApplyPatchTestFile(t, root, "update.txt", "one\nold\nthree\n")
	writeApplyPatchTestFile(t, root, "delete.txt", "remove me\n")

	result, err := NewApplyPatchTool().Execute(context.Background(), applyPatchExecutionContext(t, root), rawPatch(`*** Begin Patch
*** Add File: add.txt
+created
*** Update File: update.txt
@@
-old
+new
*** Delete File: delete.txt
*** End Patch
`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(result.TextMutations) != 3 {
		t.Fatalf("TextMutations = %#v", result.TextMutations)
	}
	mutations := make(map[string]TextMutation, len(result.TextMutations))
	for _, mutation := range result.TextMutations {
		mutations[filepath.Base(mutation.Path)] = mutation
	}
	if mutation := mutations["add.txt"]; mutation.Existed || mutation.Before != "" || mutation.After != "created\n" {
		t.Fatalf("add mutation = %#v", mutation)
	}
	if mutation := mutations["update.txt"]; !mutation.Existed || mutation.Before != "one\nold\nthree\n" || mutation.After != "one\nnew\nthree\n" {
		t.Fatalf("update mutation = %#v", mutation)
	}
	if mutation := mutations["delete.txt"]; !mutation.Existed || mutation.Before != "remove me\n" || mutation.After != "" {
		t.Fatalf("delete mutation = %#v", mutation)
	}
}

func TestApplyPatchToolMatchesWithProgressiveTolerance(t *testing.T) {
	tests := []struct {
		name    string
		before  string
		oldLine string
		want    string
	}{
		{name: "exact", before: "start\nvalue\nend\n", oldLine: "value", want: "start\nchanged\nend\n"},
		{name: "trim right", before: "start\nvalue   \nend\n", oldLine: "value", want: "start\nchanged\nend\n"},
		{name: "trim both", before: "start\n  value  \nend\n", oldLine: "value", want: "start\nchanged\nend\n"},
		{name: "normalized punctuation", before: "start\nquote \u201chi\u201d \u2013 ok\nend\n", oldLine: `quote "hi" - ok`, want: "start\nchanged\nend\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeApplyPatchTestFile(t, root, "notes.txt", test.before)
			_, err := NewApplyPatchTool().Execute(context.Background(), applyPatchExecutionContext(t, root), rawPatch("*** Begin Patch\n*** Update File: notes.txt\n@@\n-"+test.oldLine+"\n+changed\n*** End Patch\n"))
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			assertFileContent(t, root, "notes.txt", test.want)
		})
	}
}

func TestApplyPatchToolAppliesFirstMatchingAmbiguousHunk(t *testing.T) {
	root := t.TempDir()
	before := "target\nmiddle\ntarget\n"
	writeApplyPatchTestFile(t, root, "notes.txt", before)

	result, err := NewApplyPatchTool().Execute(context.Background(), applyPatchExecutionContext(t, root), rawPatch(`*** Begin Patch
*** Update File: notes.txt
@@
-target
+changed
*** End Patch
`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "M notes.txt") {
		t.Fatalf("output = %q", result.Output)
	}
	assertFileContent(t, root, "notes.txt", "changed\nmiddle\ntarget\n")
}

func TestApplyPatchToolValidationFailureLeavesEveryFileUnchanged(t *testing.T) {
	root := t.TempDir()
	writeApplyPatchTestFile(t, root, "a.txt", "old\n")
	writeApplyPatchTestFile(t, root, "b.txt", "current\n")

	_, err := NewApplyPatchTool().Execute(context.Background(), applyPatchExecutionContext(t, root), rawPatch(`*** Begin Patch
*** Update File: a.txt
@@
-old
+new
*** Update File: b.txt
@@
-missing
+changed
*** End Patch
`))
	if err == nil {
		t.Fatalf("Execute() error = nil, want failure")
	}
	if !strings.Contains(err.Error(), "b.txt: hunk did not match") || !strings.Contains(err.Error(), "Re-read this file section and retry") {
		t.Fatalf("Execute() error = %q, want re-read guidance", err.Error())
	}
	assertFileContent(t, root, "a.txt", "old\n")
	assertFileContent(t, root, "b.txt", "current\n")
}

func TestApplyPatchToolNoMatchExplainsReadLinePrefixesInHunk(t *testing.T) {
	root := t.TempDir()
	writeApplyPatchTestFile(t, root, "README.md", "## Environment Variables\n\nCopy examples and adjust as needed:\n")

	_, err := NewApplyPatchTool().Execute(context.Background(), applyPatchExecutionContext(t, root), rawPatch(`*** Begin Patch
*** Update File: README.md
@@
-52: ## Environment Variables
+## Environment Variables
+
+See extended environment requirements in [docs/env.md](docs/env.md).
*** End Patch
`))
	if err == nil {
		t.Fatal("Execute() error = nil, want hunk mismatch")
	}
	got := err.Error()
	for _, want := range []string{
		"README.md: hunk did not match",
		`line numbers copied from read output like "52:"`,
		"Remove line numbers copied from read output",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Execute() error = %q, missing %q", got, want)
		}
	}
	assertFileContent(t, root, "README.md", "## Environment Variables\n\nCopy examples and adjust as needed:\n")
}

func TestApplyPatchToolNoopPatchReportsAlreadyApplied(t *testing.T) {
	root := t.TempDir()
	before := "one\nsame\nthree\n"
	writeApplyPatchTestFile(t, root, "notes.txt", before)

	result, err := NewApplyPatchTool().Execute(context.Background(), applyPatchExecutionContext(t, root), rawPatch(`*** Begin Patch
*** Update File: notes.txt
@@
-same
+same
*** End Patch
`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "Patch already applied successfully") {
		t.Fatalf("output = %q", result.Output)
	}
	assertFileContent(t, root, "notes.txt", before)
}

func TestApplyPatchToolPathRequestsForAllAffectedPaths(t *testing.T) {
	requests, err := NewApplyPatchTool().PathRequests(rawPatch(`*** Begin Patch
*** Add File: added.txt
+hello
*** Update File: old.txt
*** Move to: new.txt
@@
-old
+new
*** Delete File: deleted.txt
*** End Patch
`))
	if err != nil {
		t.Fatalf("PathRequests() error = %v", err)
	}
	got := make(map[string]workspace.Access, len(requests))
	for _, request := range requests {
		got[request.Path] = request.Access
	}
	for _, path := range []string{"added.txt", "old.txt", "new.txt", "deleted.txt"} {
		if got[path] != workspace.AccessWrite {
			t.Fatalf("request for %s = %q, requests=%#v", path, got[path], requests)
		}
	}
}

func applyPatchExecutionContext(t *testing.T, root string) ExecutionContext {
	t.Helper()
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	return ExecutionContext{Workspace: scope}
}

func rawPatch(patch string) json.RawMessage {
	return json.RawMessage(patch)
}

func writeApplyPatchTestFile(t *testing.T, root, path, content string) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func assertFileContent(t *testing.T, root, path, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s content = %q, want %q", path, string(got), want)
	}
}

func assertFileMissing(t *testing.T, root, path string) {
	t.Helper()
	_, err := os.Stat(filepath.Join(root, path))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(%s) error = %v, want not exist", path, err)
	}
}
