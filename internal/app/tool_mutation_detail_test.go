package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeLoadSessionToolMutationDetailLoadsLargeBeforeContentFromBlob(t *testing.T) {
	blobStore := newTestSQLiteBlobStore(t)
	runtime := newWriteRestoreRuntime(t, blobStore)
	root := t.TempDir()
	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	path := filepath.Join(root, "notes.txt")
	before := strings.Repeat("before line\n", 500)
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	executeReadTool(t, runtime, sessionID, "turn-1", "call-read-1", "notes.txt")
	executeReadToolWithArgs(t, runtime, sessionID, "turn-1", "call-read-2", map[string]any{
		"paths":  []string{"notes.txt"},
		"offset": 200,
		"limit":  200,
	})
	executeReadToolWithArgs(t, runtime, sessionID, "turn-1", "call-read-3", map[string]any{
		"paths":  []string{"notes.txt"},
		"offset": 400,
		"limit":  200,
	})
	executeWriteTool(t, runtime, sessionID, "turn-1", "call-1", "notes.txt", "after\n")

	detail, err := runtime.LoadSessionToolMutationDetail(context.Background(), sessionID, "turn-1", "call-1")
	if err != nil {
		t.Fatalf("LoadSessionToolMutationDetail() error = %v", err)
	}
	wantPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks(path) error = %v", err)
	}
	gotPath, err := filepath.EvalSymlinks(detail.Path)
	if err != nil {
		t.Fatalf("EvalSymlinks(detail.Path) error = %v", err)
	}
	if gotPath != wantPath {
		t.Fatalf("detail.Path = %q, want %q", gotPath, wantPath)
	}
	if !detail.Existed {
		t.Fatal("detail.Existed = false, want true")
	}
	if detail.Before != before {
		t.Fatalf("detail.Before length = %d, want %d", len(detail.Before), len(before))
	}
	if detail.After != "after\n" {
		t.Fatalf("detail.After = %q", detail.After)
	}
}
