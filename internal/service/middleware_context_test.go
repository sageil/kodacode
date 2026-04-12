package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
)

func TestCurrentTurnPromptContext_ExtractsInlinePaths(t *testing.T) {
	req := &pipeline.TurnRequest{
		CurrentInput: &provider.Message{
			Role: "user",
			Parts: []provider.MessagePart{
				provider.TextPart{Text: "Inspect internal/service/session.go, cmd/kodacode/runtime.go:94, and README.md."},
			},
		},
	}

	text, touched := currentTurnPromptContext("", req)
	if text == "" {
		t.Fatal("currentTurnPromptContext() text should not be empty")
	}
	if len(touched) != 3 {
		t.Fatalf("len(touched) = %d, want 3 (%v)", len(touched), touched)
	}
	if touched[0] != "internal/service/session.go" {
		t.Fatalf("touched[0] = %q, want internal/service/session.go", touched[0])
	}
	if touched[1] != "cmd/kodacode/runtime.go" {
		t.Fatalf("touched[1] = %q, want cmd/kodacode/runtime.go", touched[1])
	}
	if touched[2] != "README.md" {
		t.Fatalf("touched[2] = %q, want README.md", touched[2])
	}
}

func TestCurrentTurnPromptContext_IgnoresNonFileSlashTokens(t *testing.T) {
	req := &pipeline.TurnRequest{
		CurrentInput: &provider.Message{
			Role: "user",
			Parts: []provider.MessagePart{
				provider.TextPart{Text: "Check /v1/chat and github.com/sageil/kodacode behavior."},
			},
		},
	}

	_, touched := currentTurnPromptContext("", req)
	if len(touched) != 0 {
		t.Fatalf("touched = %v, want no inferred paths", touched)
	}
}

func TestCurrentTurnPromptContext_ProjectRelativeDirectoryExists(t *testing.T) {
	projectDir := t.TempDir()
	dir := filepath.Join(projectDir, "internal", "service")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	req := &pipeline.TurnRequest{
		CurrentInput: &provider.Message{
			Role: "user",
			Parts: []provider.MessagePart{
				provider.TextPart{Text: "Focus on internal/service next."},
			},
		},
	}

	_, touched := currentTurnPromptContext(projectDir, req)
	if len(touched) != 1 || touched[0] != "internal/service" {
		t.Fatalf("touched = %v, want [internal/service]", touched)
	}
}
