package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/provider"
	searchsvc "github.com/sageil/kodacode/internal/search"
)

func TestLoadWorkspaceStatusReturnsNilGitOutsideRepository(t *testing.T) {
	root := t.TempDir()

	status, err := loadWorkspaceStatus(context.Background(), root, nil)
	if err != nil {
		t.Fatalf("loadWorkspaceStatus() error = %v", err)
	}
	if status.Git != nil {
		t.Fatalf("status.Git = %#v, want nil", status.Git)
	}
}

func TestLoadWorkspaceStatusReadsGitBranchAndChangedFiles(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	status, err := loadWorkspaceStatus(context.Background(), root, nil)
	if err != nil {
		t.Fatalf("loadWorkspaceStatus() error = %v", err)
	}
	if status.Git == nil {
		t.Fatal("status.Git = nil, want git summary")
	}
	if got := status.Git.Branch; got != "main" {
		t.Fatalf("status.Git.Branch = %q, want %q", got, "main")
	}
	if got := status.Git.ChangedFiles; got != 1 {
		t.Fatalf("status.Git.ChangedFiles = %d, want 1", got)
	}
	if len(status.Git.Changed) != 1 {
		t.Fatalf("status.Git.Changed = %d entries, want 1", len(status.Git.Changed))
	}
	if got := status.Git.Changed[0].Path; got != "README.md" {
		t.Fatalf("status.Git.Changed[0].Path = %q, want README.md", got)
	}
	if got := status.Git.Changed[0].Status; got != "??" {
		t.Fatalf("status.Git.Changed[0].Status = %q, want ??", got)
	}
}

func TestParseWorkspaceGitStatusKeepsBoundedChangedFiles(t *testing.T) {
	raw := "## main\n"
	for i := range workspaceStatusMaxGitChangedFileEntries + 5 {
		raw += " M file" + string(rune('a'+i%26)) + ".go\n"
	}

	status := parseWorkspaceGitStatus(raw)
	if status == nil {
		t.Fatal("parseWorkspaceGitStatus() = nil, want status")
	}
	if got, want := status.ChangedFiles, workspaceStatusMaxGitChangedFileEntries+5; got != want {
		t.Fatalf("ChangedFiles = %d, want %d", got, want)
	}
	if got := len(status.Changed); got != workspaceStatusMaxGitChangedFileEntries {
		t.Fatalf("len(Changed) = %d, want %d", got, workspaceStatusMaxGitChangedFileEntries)
	}
}

func TestParseWorkspaceGitChangedFileUsesRenameDestination(t *testing.T) {
	changed, ok := parseWorkspaceGitChangedFile("R  old/name.go -> new/name.go")
	if !ok {
		t.Fatal("parseWorkspaceGitChangedFile() ok = false")
	}
	if changed.Status != "R" {
		t.Fatalf("Status = %q, want R", changed.Status)
	}
	if changed.Path != "new/name.go" {
		t.Fatalf("Path = %q, want new/name.go", changed.Path)
	}
}

func TestLoadWorkspaceStatusIncludesSearchIndexSummary(t *testing.T) {
	root := t.TempDir()
	indexDir := filepath.Join(t.TempDir(), "search-index")
	path := filepath.Join(root, "auth.go")
	if err := os.WriteFile(path, []byte("package auth\n\nfunc CheckPermission() bool { return true }\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	service := searchsvc.NewService(&searchStubEmbedder{
		vectors: map[string][]float32{
			"auth.go:1\npackage auth":                  {1, 0},
			"auth.go:3\nfunc CheckPermission() bool {": {1, 0},
			"auth.go:1\npackage auth\n":                {1, 0},
		},
	}, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, indexDir, nil)
	service.TrackWorkspace(root, searchsvc.TrackOptions{RefreshInterval: 10 * time.Millisecond})
	t.Cleanup(func() {
		_ = service.Close()
	})
	if _, err := service.WarmWorkspace(context.Background(), root); err != nil {
		t.Fatalf("WarmWorkspace() error = %v", err)
	}

	status, err := loadWorkspaceStatus(context.Background(), root, service)
	if err != nil {
		t.Fatalf("loadWorkspaceStatus() error = %v", err)
	}
	if status.Search == nil {
		t.Fatal("status.Search = nil, want search summary")
	}
	if !status.Search.Configured {
		t.Fatal("status.Search.Configured = false, want true")
	}
	if got := status.Search.IndexedFiles; got != 1 {
		t.Fatalf("status.Search.IndexedFiles = %d, want 1", got)
	}
	if got := status.Search.PendingFiles; got != 0 {
		t.Fatalf("status.Search.PendingFiles = %d, want 0", got)
	}
	if status.Search.IndexedChunks == 0 {
		t.Fatal("status.Search.IndexedChunks = 0, want indexed chunks")
	}
	if status.Search.LastWarmupAt.IsZero() {
		t.Fatal("status.Search.LastWarmupAt = zero, want warmup timestamp")
	}
}

type searchStubEmbedder struct {
	vectors map[string][]float32
}

func (s *searchStubEmbedder) Embed(_ context.Context, req provider.EmbeddingRequest) ([][]float32, error) {
	out := make([][]float32, len(req.Inputs))
	for idx, input := range req.Inputs {
		vector := s.vectors[input]
		if vector == nil {
			vector = []float32{0, 0}
		}
		out[idx] = append([]float32(nil), vector...)
	}
	return out, nil
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
}
