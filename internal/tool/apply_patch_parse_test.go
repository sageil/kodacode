package tool

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestParseApplyPatchParsesMultipleOperationsAndHunks(t *testing.T) {
	patch := `*** Begin Patch
*** Add File: docs/new.md
+# New
+
+content
*** Update File: src/app.go
*** Move to: src/main.go
@@ func main
 package main
-func main() {}
+func main() {
+	println("ok")
+}
@@
 old
-tail
+tail2
*** End of File
*** Delete File: old.txt
*** End Patch
`

	parsed, err := ParseApplyPatch(patch)
	if err != nil {
		t.Fatalf("ParseApplyPatch() error = %v", err)
	}
	if len(parsed.Operations) != 3 {
		t.Fatalf("len(Operations) = %d, want 3: %#v", len(parsed.Operations), parsed.Operations)
	}
	add := parsed.Operations[0]
	if add.Kind != ApplyPatchOperationAdd || add.Path != "docs/new.md" {
		t.Fatalf("add operation = %#v", add)
	}
	if got := add.Lines; len(got) != 3 || got[0] != "# New" || got[1] != "" || got[2] != "content" {
		t.Fatalf("add lines = %#v", got)
	}
	update := parsed.Operations[1]
	if update.Kind != ApplyPatchOperationUpdate || update.Path != "src/app.go" || update.MovePath != "src/main.go" {
		t.Fatalf("update operation = %#v", update)
	}
	if len(update.Hunks) != 2 {
		t.Fatalf("len(update.Hunks) = %d, want 2: %#v", len(update.Hunks), update.Hunks)
	}
	if update.Hunks[0].Context != "func main" {
		t.Fatalf("first context = %q", update.Hunks[0].Context)
	}
	if got := update.Hunks[0].OldLines; len(got) != 2 || got[0] != "package main" || got[1] != "func main() {}" {
		t.Fatalf("first old lines = %#v", got)
	}
	if got := update.Hunks[0].NewLines; len(got) != 4 || got[0] != "package main" || got[1] != "func main() {" || got[3] != "}" {
		t.Fatalf("first new lines = %#v", got)
	}
	if update.Hunks[1].Context != "" || !update.Hunks[1].EndOfFile {
		t.Fatalf("second hunk = %#v", update.Hunks[1])
	}
	del := parsed.Operations[2]
	if del.Kind != ApplyPatchOperationDelete || del.Path != "old.txt" {
		t.Fatalf("delete operation = %#v", del)
	}
}

func TestParseApplyPatchAllowsMoveOnlyUpdate(t *testing.T) {
	patch := `*** Begin Patch
*** Update File: old.txt
*** Move to: new.txt
*** End Patch
`

	parsed, err := ParseApplyPatch(patch)
	if err != nil {
		t.Fatalf("ParseApplyPatch() error = %v", err)
	}
	if len(parsed.Operations) != 1 || parsed.Operations[0].MovePath != "new.txt" || len(parsed.Operations[0].Hunks) != 0 {
		t.Fatalf("operation = %#v", parsed.Operations)
	}
}

func TestParseApplyPatchAcceptsJSONPatchWrapper(t *testing.T) {
	patch := `*** Begin Patch
*** Update File: README.md
@@
-old
+new
*** End Patch
`
	wrapped, err := json.Marshal(map[string]string{"patch": patch})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	parsed, err := ParseApplyPatch(string(wrapped))
	if err != nil {
		t.Fatalf("ParseApplyPatch() error = %v", err)
	}
	if len(parsed.Operations) != 1 || parsed.Operations[0].Path != "README.md" {
		t.Fatalf("operations = %#v", parsed.Operations)
	}
}

func TestParseApplyPatchAcceptsCommonModelWrappers(t *testing.T) {
	patch := `*** Begin Patch
*** Add File: notes.txt
+hello
*** End Patch
`
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "markdown fence",
			input: "```patch\n" + patch + "```\n",
		},
		{
			name:  "assistant prose",
			input: "I'll apply this patch now:\n\n" + patch + "\nDone.",
		},
		{
			name:  "literal function call",
			input: `apply_patch({"patch":"*** Begin Patch\n*** Add File: notes.txt\n+hello\n*** End Patch\n"})`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := ParseApplyPatch(test.input)
			if err != nil {
				t.Fatalf("ParseApplyPatch() error = %v", err)
			}
			if len(parsed.Operations) != 1 || parsed.Operations[0].Kind != ApplyPatchOperationAdd || parsed.Operations[0].Path != "notes.txt" {
				t.Fatalf("operations = %#v", parsed.Operations)
			}
		})
	}
}

func TestParseApplyPatchRejectsMissingBoundary(t *testing.T) {
	_, err := ParseApplyPatch(`*** Update File: app.go
-old
+new
*** End Patch
`)
	if !errors.Is(err, ErrApplyPatchMissingBegin) {
		t.Fatalf("ParseApplyPatch() error = %v, want ErrApplyPatchMissingBegin", err)
	}
}

func TestParseApplyPatchRejectsUnknownHeader(t *testing.T) {
	_, err := ParseApplyPatch(`*** Begin Patch
*** Rename File: a.go
*** End Patch
`)
	if !errors.Is(err, ErrApplyPatchUnknownHeader) {
		t.Fatalf("ParseApplyPatch() error = %v, want ErrApplyPatchUnknownHeader", err)
	}
}

func TestParseApplyPatchRejectsAbsoluteAndParentPaths(t *testing.T) {
	tests := []struct {
		name string
		path string
		want error
	}{
		{name: "posix absolute", path: "/tmp/app.go", want: ErrApplyPatchAbsolutePath},
		{name: "windows absolute", path: `C:\tmp\app.go`, want: ErrApplyPatchAbsolutePath},
		{name: "unc absolute", path: `\\server\share\app.go`, want: ErrApplyPatchAbsolutePath},
		{name: "parent", path: "../app.go", want: ErrApplyPatchParentPath},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseApplyPatch("*** Begin Patch\n*** Delete File: " + test.path + "\n*** End Patch\n")
			if !errors.Is(err, test.want) {
				t.Fatalf("ParseApplyPatch() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestParseApplyPatchRejectsEmptyAddAndUpdate(t *testing.T) {
	tests := []struct {
		name  string
		patch string
		want  error
	}{
		{
			name: "empty add",
			patch: `*** Begin Patch
*** Add File: app.go
*** End Patch
`,
			want: ErrApplyPatchEmptyAdd,
		},
		{
			name: "empty update",
			patch: `*** Begin Patch
*** Update File: app.go
*** End Patch
`,
			want: ErrApplyPatchEmptyUpdate,
		},
		{
			name: "empty context",
			patch: `*** Begin Patch
*** Update File: app.go
@@ function
*** End Patch
`,
			want: ErrApplyPatchMalformedLine,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseApplyPatch(test.patch)
			if !errors.Is(err, test.want) {
				t.Fatalf("ParseApplyPatch() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestParseApplyPatchRejectsMalformedPatchLines(t *testing.T) {
	_, err := ParseApplyPatch(`*** Begin Patch
*** Update File: app.go
@@
old
+new
*** End Patch
`)
	if !errors.Is(err, ErrApplyPatchMalformedLine) {
		t.Fatalf("ParseApplyPatch() error = %v, want ErrApplyPatchMalformedLine", err)
	}
}

func TestParseApplyPatchMalformedAddFileMarkerErrorIsActionable(t *testing.T) {
	_, err := ParseApplyPatch(`*** Begin Patch
*** Add File: app.go
+package main
***
*** End Patch
`)
	if !errors.Is(err, ErrApplyPatchMalformedLine) {
		t.Fatalf("ParseApplyPatch() error = %v, want ErrApplyPatchMalformedLine", err)
	}
	got := err.Error()
	for _, want := range []string{
		`unexpected "***" inside Add File`,
		`use "*** End Patch" to close the patch`,
		`prefix it as "+***" if it is intended file content`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("ParseApplyPatch() error = %q, missing %q", got, want)
		}
	}
}

func TestParseApplyPatchMalformedLineErrorExplainsPatchSyntax(t *testing.T) {
	_, err := ParseApplyPatch(`*** Begin Patch
*** Update File: src/middleware/auth.ts
--- a/src/middleware/auth.ts
+++ b/src/middleware/auth.ts
@@
}
*** End Patch
`)
	if err == nil {
		t.Fatal("ParseApplyPatch() error = nil, want malformed patch error")
	}
	got := err.Error()
	for _, want := range []string{
		"`apply_patch` failed.",
		"invalid patch syntax",
		`Use Add File content lines starting with "+", and Update File hunk lines starting with a space, "+", or "-"`,
		"Retry by calling apply_patch again, not by printing the patch as assistant text",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("ParseApplyPatch() error = %q, missing %q", got, want)
		}
	}
	if strings.Contains(got, "first line exactly") {
		t.Fatalf("ParseApplyPatch() error kept misleading boundary guidance: %q", got)
	}
}

func TestParseApplyPatchRejectsReadLinePrefixes(t *testing.T) {
	_, err := ParseApplyPatch(`*** Begin Patch
*** Update File: app.go
@@
-1: old
-2: older
-3: oldest
+new
*** End Patch
`)
	if !errors.Is(err, ErrApplyPatchReadLinePrefixes) {
		t.Fatalf("ParseApplyPatch() error = %v, want ErrApplyPatchReadLinePrefixes", err)
	}
}
