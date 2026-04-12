package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/lsp"
)

type fakeCodeActionServer struct {
	actions []lsp.CodeAction
	err     error
}

func (f fakeCodeActionServer) CodeActions(context.Context, string, lsp.Range, []string) ([]lsp.CodeAction, error) {
	return f.actions, f.err
}

func TestCodeActionTool_AppliesWorkspaceEdits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.ts")
	if err := os.WriteFile(path, []byte("const foo = oldName;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var notified []string
	tl := newCodeActionTool(
		func(context.Context, string, string) (codeActionServer, error) {
			return fakeCodeActionServer{actions: []lsp.CodeAction{{
				Title: "Rename locally",
				Kind:  "quickfix",
				Edit: &lsp.WorkspaceEdit{
					Changes: map[string][]lsp.TextEdit{
						lsp.FileURI(path): {{
							Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 12}, End: lsp.Position{Line: 0, Character: 19}},
							NewText: "newName",
						}},
					},
				},
			}}}, nil
		},
		func(_ context.Context, events []workspaceEditNotification) error {
			for _, event := range events {
				if event.Path != "" {
					notified = append(notified, event.Path)
				}
			}
			return nil
		},
		nil,
	)

	res, err := tl.Execute(t.Context(), ExecutionContext{WorkDir: dir}, []byte(`{"filePath":"`+path+`","startLine":1,"startCharacter":0,"endLine":1,"endCharacter":20,"title":"Rename locally"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.ErrorCode != "" {
		t.Fatalf("unexpected result: %#v", res)
	}
	if !strings.Contains(res.Output, `Applied code action "Rename locally".`) {
		t.Fatalf("unexpected output: %q", res.Output)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "const foo = newName;\n" {
		t.Fatalf("unexpected content: %q", data)
	}
	if !slices.Equal(notified, []string{resolvePathAllowMissing(path)}) {
		t.Fatalf("unexpected notify paths: %v", notified)
	}
}

func TestCodeActionTool_RequiresSelectorWhenMultipleActionsMatch(t *testing.T) {
	tl := newCodeActionTool(
		func(context.Context, string, string) (codeActionServer, error) {
			return fakeCodeActionServer{actions: []lsp.CodeAction{
				{Title: "First fix", Kind: "quickfix"},
				{Title: "Second fix", Kind: "quickfix"},
			}}, nil
		},
		nil,
		nil,
	)

	res, err := tl.Execute(t.Context(), ExecutionContext{WorkDir: t.TempDir()}, []byte(`{"filePath":"/tmp/x.ts","startLine":1,"startCharacter":0,"endLine":1,"endCharacter":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.ErrorCode != ErrCodeInvalidArgs {
		t.Fatalf("expected invalid_args result, got %#v", res)
	}
	if !strings.Contains(res.Output, "multiple code actions matched") {
		t.Fatalf("unexpected output: %q", res.Output)
	}
}

func TestCodeActionTool_RejectsCommandOnlyActions(t *testing.T) {
	tl := newCodeActionTool(
		func(context.Context, string, string) (codeActionServer, error) {
			return fakeCodeActionServer{actions: []lsp.CodeAction{{
				Title:   "Run server command",
				Command: &lsp.Command{Title: "Run server command", Command: "workspace.execute"},
			}}}, nil
		},
		nil,
		nil,
	)

	res, err := tl.Execute(t.Context(), ExecutionContext{WorkDir: t.TempDir()}, []byte(`{"filePath":"/tmp/x.ts","startLine":1,"startCharacter":0,"endLine":1,"endCharacter":1,"title":"Run server command"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.ErrorCode != ErrCodeInvalidArgs {
		t.Fatalf("expected invalid_args result, got %#v", res)
	}
	if !strings.Contains(res.Output, "executeCommand") {
		t.Fatalf("unexpected output: %q", res.Output)
	}
}

func TestApplyWorkspaceEdit_HandlesResourceOperations(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.txt")
	newPath := filepath.Join(dir, "new.txt")
	renamedPath := filepath.Join(dir, "renamed.txt")
	deletePath := filepath.Join(dir, "delete.txt")
	if err := os.WriteFile(oldPath, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(deletePath, []byte("remove me\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mustRaw := func(v any) json.RawMessage {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}

	var notified []string
	summary, err := applyWorkspaceEdit(t.Context(), dir, &lsp.WorkspaceEdit{
		DocumentChanges: []json.RawMessage{
			mustRaw(lsp.CreateFile{Kind: "create", URI: lsp.FileURI(newPath)}),
			mustRaw(lsp.TextDocumentEdit{
				TextDocument: lsp.OptionalVersionedTextDocumentIdentifier{URI: lsp.FileURI(newPath)},
				Edits: []lsp.TextEdit{{
					Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 0, Character: 0}},
					NewText: "created\n",
				}},
			}),
			mustRaw(lsp.RenameFile{Kind: "rename", OldURI: lsp.FileURI(oldPath), NewURI: lsp.FileURI(renamedPath)}),
			mustRaw(lsp.DeleteFile{Kind: "delete", URI: lsp.FileURI(deletePath)}),
		},
	}, func(_ context.Context, events []workspaceEditNotification) error {
		for _, event := range events {
			switch event.Kind {
			case workspaceEditNotifyCreated, workspaceEditNotifyChanged, workspaceEditNotifyDeleted:
				notified = append(notified, string(event.Kind)+":"+event.Path)
			case workspaceEditNotifyRenamed:
				notified = append(notified, string(event.Kind)+":"+event.OldPath+"->"+event.NewPath)
			}
		}
		return nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if got, err := os.ReadFile(newPath); err != nil || string(got) != "created\n" {
		t.Fatalf("unexpected new file content: %q (err=%v)", got, err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected old path to be renamed, stat err=%v", err)
	}
	if got, err := os.ReadFile(renamedPath); err != nil || string(got) != "before\n" {
		t.Fatalf("unexpected renamed file content: %q (err=%v)", got, err)
	}
	if _, err := os.Stat(deletePath); !os.IsNotExist(err) {
		t.Fatalf("expected delete target to be removed, stat err=%v", err)
	}

	slices.Sort(notified)
	wantNotified := []string{
		string(workspaceEditNotifyCreated) + ":" + resolvePathAllowMissing(newPath),
		string(workspaceEditNotifyDeleted) + ":" + resolvePathAllowMissing(deletePath),
		string(workspaceEditNotifyRenamed) + ":" + resolvePathAllowMissing(oldPath) + "->" + resolvePathAllowMissing(renamedPath),
	}
	if !slices.Equal(notified, wantNotified) {
		t.Fatalf("unexpected notify paths: %v", notified)
	}
	if summary.TextEdits != 1 || summary.Created != 1 || summary.Renamed != 1 || summary.Deleted != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
}

func TestApplyWorkspaceEdit_DefersNotifyUntilFinalContent(t *testing.T) {
	dir := t.TempDir()
	newPath := filepath.Join(dir, "new.txt")

	mustRaw := func(v any) json.RawMessage {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}

	var seen []string
	_, err := applyWorkspaceEdit(t.Context(), dir, &lsp.WorkspaceEdit{
		DocumentChanges: []json.RawMessage{
			mustRaw(lsp.CreateFile{Kind: "create", URI: lsp.FileURI(newPath)}),
			mustRaw(lsp.TextDocumentEdit{
				TextDocument: lsp.OptionalVersionedTextDocumentIdentifier{URI: lsp.FileURI(newPath)},
				Edits: []lsp.TextEdit{{
					Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 0, Character: 0}},
					NewText: "created\n",
				}},
			}),
		},
	}, func(_ context.Context, events []workspaceEditNotification) error {
		for _, event := range events {
			target := event.Path
			if event.Kind == workspaceEditNotifyRenamed {
				target = event.NewPath
			}
			if target == "" {
				continue
			}
			data, readErr := os.ReadFile(target)
			if readErr != nil {
				t.Fatalf("notify read failed: %v", readErr)
			}
			seen = append(seen, string(data))
		}
		return nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(seen, []string{"created\n"}) {
		t.Fatalf("unexpected notified content: %v", seen)
	}
}

func TestApplyWorkspaceEdit_CreateOverwriteOnDirectoryLeavesDirectoryIntact(t *testing.T) {
	dir := t.TempDir()
	targetDir := filepath.Join(dir, "target")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustRaw := func(v any) json.RawMessage {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}

	_, err := applyWorkspaceEdit(t.Context(), dir, &lsp.WorkspaceEdit{
		DocumentChanges: []json.RawMessage{
			mustRaw(lsp.CreateFile{
				Kind: "create",
				URI:  lsp.FileURI(targetDir),
				Options: &lsp.CreateFileOptions{
					Overwrite: true,
				},
			}),
		},
	}, nil, nil)
	if err == nil {
		t.Fatal("expected create overwrite on directory to fail")
	}
	info, statErr := os.Stat(targetDir)
	if statErr != nil {
		t.Fatalf("expected directory to remain, stat err=%v", statErr)
	}
	if !info.IsDir() {
		t.Fatalf("expected target to remain a directory, got mode=%v", info.Mode())
	}
}

func TestApplyWorkspaceEdit_RejectsSymlinkEscapesOutsideProject(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	linkPath := filepath.Join(dir, "linked")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Fatal(err)
	}
	escapedPath := filepath.Join(linkPath, "outside.txt")

	_, err := applyWorkspaceEdit(t.Context(), dir, &lsp.WorkspaceEdit{
		Changes: map[string][]lsp.TextEdit{
			lsp.FileURI(escapedPath): {{
				Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 0, Character: 0}},
				NewText: "blocked",
			}},
		},
	}, nil, nil)
	if err == nil {
		t.Fatal("expected symlink escape to be rejected")
	}
	if !strings.Contains(err.Error(), "outside the project directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyWorkspaceEdit_RollsBackOnLaterFailure(t *testing.T) {
	dir := t.TempDir()
	createdPath := filepath.Join(dir, "created.txt")
	missingPath := filepath.Join(dir, "missing.txt")

	mustRaw := func(v any) json.RawMessage {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}

	_, err := applyWorkspaceEdit(t.Context(), dir, &lsp.WorkspaceEdit{
		DocumentChanges: []json.RawMessage{
			mustRaw(lsp.CreateFile{Kind: "create", URI: lsp.FileURI(createdPath)}),
			mustRaw(lsp.TextDocumentEdit{
				TextDocument: lsp.OptionalVersionedTextDocumentIdentifier{URI: lsp.FileURI(missingPath)},
				Edits: []lsp.TextEdit{{
					Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 0, Character: 0}},
					NewText: "nope",
				}},
			}),
		},
	}, nil, nil)
	if err == nil {
		t.Fatal("expected workspace edit failure")
	}
	if _, statErr := os.Stat(createdPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected created file to be rolled back, stat err=%v", statErr)
	}
}

func TestApplyWorkspaceEdit_RollsBackRenameOverwriteOnLaterFailure(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.txt")
	newPath := filepath.Join(dir, "new.txt")
	missingPath := filepath.Join(dir, "missing.txt")
	if err := os.WriteFile(oldPath, []byte("source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte("destination\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mustRaw := func(v any) json.RawMessage {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}

	_, err := applyWorkspaceEdit(t.Context(), dir, &lsp.WorkspaceEdit{
		DocumentChanges: []json.RawMessage{
			mustRaw(lsp.RenameFile{
				Kind:   "rename",
				OldURI: lsp.FileURI(oldPath),
				NewURI: lsp.FileURI(newPath),
				Options: &lsp.RenameFileOptions{
					Overwrite: true,
				},
			}),
			mustRaw(lsp.TextDocumentEdit{
				TextDocument: lsp.OptionalVersionedTextDocumentIdentifier{URI: lsp.FileURI(missingPath)},
				Edits: []lsp.TextEdit{{
					Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 0, Character: 0}},
					NewText: "nope",
				}},
			}),
		},
	}, nil, nil)
	if err == nil {
		t.Fatal("expected workspace edit failure")
	}

	oldContent, readErr := os.ReadFile(oldPath)
	if readErr != nil {
		t.Fatalf("expected source path restored, read err=%v", readErr)
	}
	if string(oldContent) != "source\n" {
		t.Fatalf("unexpected restored source content: %q", oldContent)
	}
	newContent, readErr := os.ReadFile(newPath)
	if readErr != nil {
		t.Fatalf("expected destination restored, read err=%v", readErr)
	}
	if string(newContent) != "destination\n" {
		t.Fatalf("unexpected restored destination content: %q", newContent)
	}
}

func TestApplyWorkspaceEdit_DoesNotNotifyOnFailedTransaction(t *testing.T) {
	dir := t.TempDir()
	createdPath := filepath.Join(dir, "created.txt")
	missingPath := filepath.Join(dir, "missing.txt")

	mustRaw := func(v any) json.RawMessage {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}

	var notified []string
	_, err := applyWorkspaceEdit(t.Context(), dir, &lsp.WorkspaceEdit{
		DocumentChanges: []json.RawMessage{
			mustRaw(lsp.CreateFile{Kind: "create", URI: lsp.FileURI(createdPath)}),
			mustRaw(lsp.TextDocumentEdit{
				TextDocument: lsp.OptionalVersionedTextDocumentIdentifier{URI: lsp.FileURI(missingPath)},
				Edits: []lsp.TextEdit{{
					Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 0, Character: 0}},
					NewText: "nope",
				}},
			}),
		},
	}, func(_ context.Context, events []workspaceEditNotification) error {
		for _, event := range events {
			if event.Path != "" {
				notified = append(notified, event.Path)
			}
		}
		return nil
	}, nil)
	if err == nil {
		t.Fatal("expected workspace edit failure")
	}
	if len(notified) != 0 {
		t.Fatalf("expected no notifications on failed transaction, got %v", notified)
	}
}

func TestApplyWorkspaceEdit_RollsBackWhenNotifyFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.ts")
	if err := os.WriteFile(path, []byte("const x = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRaw := func(v any) json.RawMessage {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}

	_, err := applyWorkspaceEdit(t.Context(), dir, &lsp.WorkspaceEdit{
		DocumentChanges: []json.RawMessage{
			mustRaw(lsp.TextDocumentEdit{
				TextDocument: lsp.OptionalVersionedTextDocumentIdentifier{URI: lsp.FileURI(path)},
				Edits: []lsp.TextEdit{{
					Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 10}, End: lsp.Position{Line: 0, Character: 11}},
					NewText: "2",
				}},
			}),
		},
	}, func(context.Context, []workspaceEditNotification) error {
		return fmt.Errorf("notify failed")
	}, nil)
	if err == nil {
		t.Fatal("expected notify failure")
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "const x = 1;\n" {
		t.Fatalf("expected file rollback, got %q", got)
	}
}

func TestApplyWorkspaceEdit_RejectsStaleDocumentVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.ts")
	if err := os.WriteFile(path, []byte("const x = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRaw := func(v any) json.RawMessage {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	expectedVersion := 2
	_, err := applyWorkspaceEdit(t.Context(), dir, &lsp.WorkspaceEdit{
		DocumentChanges: []json.RawMessage{
			mustRaw(lsp.TextDocumentEdit{
				TextDocument: lsp.OptionalVersionedTextDocumentIdentifier{URI: lsp.FileURI(path), Version: &expectedVersion},
				Edits: []lsp.TextEdit{{
					Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 10}, End: lsp.Position{Line: 0, Character: 11}},
					NewText: "2",
				}},
			}),
		},
	}, nil, func(string) (int, bool) { return 1, true })
	if err == nil {
		t.Fatal("expected stale version rejection")
	}
	if !strings.Contains(err.Error(), "version mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCodeActionTool_ClassifiesCreateConflictAsInvalidArgs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.ts")
	if err := os.WriteFile(path, []byte("const x = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tl := newCodeActionTool(
		func(context.Context, string, string) (codeActionServer, error) {
			return fakeCodeActionServer{actions: []lsp.CodeAction{{
				Title: "Create existing",
				Edit: &lsp.WorkspaceEdit{
					DocumentChanges: []json.RawMessage{
						mustJSON(t, lsp.CreateFile{Kind: "create", URI: lsp.FileURI(path)}),
					},
				},
			}}}, nil
		},
		nil,
		nil,
	)

	res, err := tl.Execute(t.Context(), ExecutionContext{WorkDir: dir}, []byte(`{"filePath":"`+path+`","startLine":1,"startCharacter":0,"endLine":1,"endCharacter":1,"title":"Create existing"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.ErrorCode != ErrCodeInvalidArgs {
		t.Fatalf("expected invalid_args result, got %#v", res)
	}
}

func TestCodeActionTool_ClassifiesMissingTargetAsNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.ts")
	missing := filepath.Join(dir, "missing.ts")
	if err := os.WriteFile(path, []byte("const x = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tl := newCodeActionTool(
		func(context.Context, string, string) (codeActionServer, error) {
			return fakeCodeActionServer{actions: []lsp.CodeAction{{
				Title: "Edit missing",
				Edit: &lsp.WorkspaceEdit{
					DocumentChanges: []json.RawMessage{
						mustJSON(t, lsp.TextDocumentEdit{
							TextDocument: lsp.OptionalVersionedTextDocumentIdentifier{URI: lsp.FileURI(missing)},
							Edits: []lsp.TextEdit{{
								Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 0, Character: 0}},
								NewText: "const y = 2;\n",
							}},
						}),
					},
				},
			}}}, nil
		},
		nil,
		nil,
	)

	res, err := tl.Execute(t.Context(), ExecutionContext{WorkDir: dir}, []byte(`{"filePath":"`+path+`","startLine":1,"startCharacter":0,"endLine":1,"endCharacter":1,"title":"Edit missing"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.ErrorCode != ErrCodeNotFound {
		t.Fatalf("expected not_found result, got %#v", res)
	}
}

func TestCodeActionTool_ClassifiesNotifyFailureAsUnavailable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.ts")
	if err := os.WriteFile(path, []byte("const x = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tl := newCodeActionTool(
		func(context.Context, string, string) (codeActionServer, error) {
			return fakeCodeActionServer{actions: []lsp.CodeAction{{
				Title: "Rename locally",
				Edit: &lsp.WorkspaceEdit{
					Changes: map[string][]lsp.TextEdit{
						lsp.FileURI(path): {{
							Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 10}, End: lsp.Position{Line: 0, Character: 11}},
							NewText: "2",
						}},
					},
				},
			}}}, nil
		},
		func(context.Context, []workspaceEditNotification) error {
			return fmt.Errorf("notify failed")
		},
		nil,
	)

	res, err := tl.Execute(t.Context(), ExecutionContext{WorkDir: dir}, []byte(`{"filePath":"`+path+`","startLine":1,"startCharacter":0,"endLine":1,"endCharacter":1,"title":"Rename locally"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.ErrorCode != ErrCodeUnavailable || !res.Retryable {
		t.Fatalf("expected retryable unavailable result, got %#v", res)
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
