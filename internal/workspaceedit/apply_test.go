package workspaceedit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/lsp"
)

func TestApplyAppliesTextEditsAndBuildsSyncPlan(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	edit := &lsp.WorkspaceEdit{
		Changes: map[string][]lsp.TextEdit{
			lsp.FileURI(path): {{
				Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 8}, End: lsp.Position{Line: 0, Character: 12}},
				NewText: "service",
			}},
		},
	}
	summary, plan, err := Apply([]string{root}, edit, nil)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if summary.TextEdits != 1 || len(summary.Paths) != 1 || summary.Paths[0] != path {
		t.Fatalf("summary = %#v", summary)
	}
	if len(plan.Changed) != 1 || plan.Changed[0] != path {
		t.Fatalf("plan = %#v", plan)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "package service\n" {
		t.Fatalf("content = %q", string(content))
	}
}

func TestApplyAppliesDocumentChanges(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "old.txt")
	deletePath := filepath.Join(root, "delete.txt")
	createdPath := filepath.Join(root, "created.txt")
	renamedPath := filepath.Join(root, "renamed.txt")
	if err := os.WriteFile(oldPath, []byte("before\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(old) error = %v", err)
	}
	if err := os.WriteFile(deletePath, []byte("remove\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(delete) error = %v", err)
	}

	rawCreate, _ := json.Marshal(lsp.CreateFile{Kind: "create", URI: lsp.FileURI(createdPath)})
	rawRename, _ := json.Marshal(lsp.RenameFile{Kind: "rename", OldURI: lsp.FileURI(oldPath), NewURI: lsp.FileURI(renamedPath)})
	rawDelete, _ := json.Marshal(lsp.DeleteFile{Kind: "delete", URI: lsp.FileURI(deletePath)})
	rawEdit, _ := json.Marshal(lsp.TextDocumentEdit{
		TextDocument: lsp.OptionalVersionedTextDocumentIdentifier{URI: lsp.FileURI(createdPath)},
		Edits: []lsp.TextEdit{{
			Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 0, Character: 0}},
			NewText: "created\n",
		}},
	})

	summary, plan, err := Apply([]string{root}, &lsp.WorkspaceEdit{
		DocumentChanges: []json.RawMessage{rawCreate, rawEdit, rawRename, rawDelete},
	}, nil)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if summary.Created != 1 || summary.Renamed != 1 || summary.Deleted != 1 || summary.TextEdits != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old path should be renamed, stat err = %v", err)
	}
	if _, err := os.Stat(deletePath); !os.IsNotExist(err) {
		t.Fatalf("delete path should be removed, stat err = %v", err)
	}
	if content, err := os.ReadFile(createdPath); err != nil || string(content) != "created\n" {
		t.Fatalf("created file = %q, err = %v", string(content), err)
	}
	if content, err := os.ReadFile(renamedPath); err != nil || string(content) != "before\n" {
		t.Fatalf("renamed file = %q, err = %v", string(content), err)
	}
	if len(plan.Renamed) != 1 || len(plan.Deleted) != 1 || len(plan.Changed) != 1 {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestApplyRejectsPathsOutsideRoots(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.go")
	edit := &lsp.WorkspaceEdit{
		Changes: map[string][]lsp.TextEdit{
			lsp.FileURI(outside): {{
				Range:   lsp.Range{},
				NewText: "package main\n",
			}},
		},
	}
	_, _, err := Apply([]string{root}, edit, nil)
	if err == nil || !strings.Contains(err.Error(), "outside the configured workspace roots") {
		t.Fatalf("error = %v", err)
	}
}

func TestApplyPreservesFileModeWhenCreateFileOverwrites(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "script.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	rawCreate, _ := json.Marshal(lsp.CreateFile{
		Kind: "create",
		URI:  lsp.FileURI(path),
		Options: &lsp.CreateFileOptions{
			Overwrite: true,
		},
	})

	if _, _, err := Apply([]string{root}, &lsp.WorkspaceEdit{
		DocumentChanges: []json.RawMessage{rawCreate},
	}, nil); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("mode = %#o, want 0755", got)
	}
}
