package snapshot

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func initGitRepo(t *testing.T, withCommit bool) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	run("init")
	run("config", "user.name", "test")
	run("config", "user.email", "test@test.com")
	if withCommit {
		if err := os.WriteFile(filepath.Join(dir, "init.txt"), []byte("init"), 0o644); err != nil {
			t.Fatal(err)
		}
		run("add", "-A")
		run("commit", "-m", "initial")
	}
	return dir
}

func TestCreate_WithCommits(t *testing.T) {
	dir := initGitRepo(t, true)
	svc := New(dir)

	if err := os.WriteFile(filepath.Join(dir, "file.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := svc.Create("sess-1", 1, "first turn"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	snapshots, err := svc.List("sess-1")
	if err != nil {
		t.Fatal(err)
	}
	// git log walks the parent chain, so we see the snapshot commit plus
	// the initial repo commit. Filter to turn-prefixed entries.
	var turnSnapshots []Snapshot
	for _, s := range snapshots {
		if s.TurnIndex > 0 {
			turnSnapshots = append(turnSnapshots, s)
		}
	}
	if len(turnSnapshots) != 1 {
		t.Fatalf("want 1 turn snapshot, got %d (total log entries: %d)", len(turnSnapshots), len(snapshots))
	}
	if turnSnapshots[0].TurnIndex != 1 {
		t.Errorf("TurnIndex = %d, want 1", turnSnapshots[0].TurnIndex)
	}
	if turnSnapshots[0].Summary != "first turn" {
		t.Errorf("Summary = %q, want %q", turnSnapshots[0].Summary, "first turn")
	}
}

func TestCreate_EmptyHead(t *testing.T) {
	dir := initGitRepo(t, false)
	svc := New(dir)

	if err := os.WriteFile(filepath.Join(dir, "file.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := svc.Create("sess-1", 1, "first turn on empty repo"); err != nil {
		t.Fatalf("Create() on repo with no commits: error = %v", err)
	}

	snapshots, err := svc.List("sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("want 1 snapshot, got %d", len(snapshots))
	}
	if snapshots[0].Summary != "first turn on empty repo" {
		t.Errorf("Summary = %q, want %q", snapshots[0].Summary, "first turn on empty repo")
	}
}

func TestCreate_NoChangesSkipped(t *testing.T) {
	dir := initGitRepo(t, true)
	svc := New(dir)

	if err := svc.Create("sess-1", 1, "no changes"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	snapshots, _ := svc.List("sess-1")
	if len(snapshots) != 0 {
		t.Errorf("want 0 snapshots when no changes, got %d", len(snapshots))
	}
}

func TestCreate_NotAGitRepo(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)

	if err := svc.Create("sess-1", 1, "should noop"); err != nil {
		t.Fatalf("Create() on non-git dir should return nil, got %v", err)
	}
}

func TestRestore_ModifiedTrackedFiles(t *testing.T) {
	dir := initGitRepo(t, true)
	svc := New(dir)

	// Write a file and snapshot at turn 1.
	filePath := filepath.Join(dir, "file.go")
	if err := os.WriteFile(filePath, []byte("version 1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.Create("sess-1", 1, "turn 1"); err != nil {
		t.Fatal(err)
	}

	// Modify the file (simulates turn 2 changes).
	if err := os.WriteFile(filePath, []byte("version 2"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Restore to turn 1.
	if err := svc.Restore("sess-1", 1); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// Verify file content is back to version 1.
	got, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "version 1" {
		t.Errorf("file content = %q, want %q", got, "version 1")
	}
}

func TestRestore_RemovesLaterAddedFiles(t *testing.T) {
	dir := initGitRepo(t, true)
	svc := New(dir)

	// Snapshot at turn 1 (only init.txt exists from initial commit).
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.Create("sess-1", 1, "turn 1"); err != nil {
		t.Fatal(err)
	}

	// Add a new tracked file after the snapshot.
	newFile := filepath.Join(dir, "new.go")
	if err := os.WriteFile(newFile, []byte("package new"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Commit the new file so it becomes tracked.
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	run("add", "new.go")
	run("commit", "-m", "add new.go")

	// Restore to turn 1.
	if err := svc.Restore("sess-1", 1); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// Verify the later-added file is gone.
	if _, err := os.Stat(newFile); !os.IsNotExist(err) {
		t.Errorf("new.go should have been removed by restore, but still exists")
	}
}

func TestRestore_PreservesUntrackedFiles(t *testing.T) {
	dir := initGitRepo(t, true)
	svc := New(dir)

	// Snapshot at turn 1.
	if err := os.WriteFile(filepath.Join(dir, "code.go"), []byte("package code"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.Create("sess-1", 1, "turn 1"); err != nil {
		t.Fatal(err)
	}

	// Create an untracked file (never git-added).
	untrackedFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(untrackedFile, []byte("SECRET=abc"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Restore to turn 1.
	if err := svc.Restore("sess-1", 1); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// Verify untracked file survives.
	got, err := os.ReadFile(untrackedFile)
	if err != nil {
		t.Fatalf(".env should survive restore, got error: %v", err)
	}
	if string(got) != "SECRET=abc" {
		t.Errorf(".env content = %q, want %q", got, "SECRET=abc")
	}
}

func TestRestore_FailsWhenSafetySnapshotFails(t *testing.T) {
	dir := initGitRepo(t, true)
	svc := New(dir)

	// Create a snapshot at turn 1.
	if err := os.WriteFile(filepath.Join(dir, "f.go"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.Create("sess-1", 1, "turn 1"); err != nil {
		t.Fatal(err)
	}

	// Modify file to ensure safety snapshot has something to capture.
	if err := os.WriteFile(filepath.Join(dir, "f.go"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Break git by corrupting the objects directory so write-tree fails.
	objDir := filepath.Join(dir, ".git", "objects")
	if err := os.Chmod(objDir, 0o000); err != nil {
		t.Skipf("cannot restrict permissions (likely running as root): %v", err)
	}
	defer func() { _ = os.Chmod(objDir, 0o755) }()

	// Restore should fail because safety snapshot can't be created.
	err := svc.Restore("sess-1", 1)
	if err == nil {
		t.Fatal("Restore() should have failed when safety snapshot fails")
	}

	// Restore permissions to check file state.
	_ = os.Chmod(objDir, 0o755)

	// Verify working tree was NOT modified (fail-closed).
	got, _ := os.ReadFile(filepath.Join(dir, "f.go"))
	if string(got) != "v2" {
		t.Errorf("file should be unchanged after failed restore, got %q want %q", got, "v2")
	}
}

func TestCleanup(t *testing.T) {
	dir := initGitRepo(t, true)
	svc := New(dir)

	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.Create("sess-1", 1, "turn"); err != nil {
		t.Fatal(err)
	}

	if err := svc.Cleanup("sess-1"); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	snapshots, _ := svc.List("sess-1")
	if len(snapshots) != 0 {
		t.Errorf("want 0 snapshots after cleanup, got %d", len(snapshots))
	}
}
