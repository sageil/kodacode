package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPromptSourceFragmentsLoadsGlobalAndProjectAgents(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	globalDir := filepath.Join(configHome, "kodacode")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(global) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, promptInstructionsFilename), []byte("Global rules."), 0o644); err != nil {
		t.Fatalf("WriteFile(global) error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, promptInstructionsFilename), []byte("Project rules."), 0o644); err != nil {
		t.Fatalf("WriteFile(project) error = %v", err)
	}

	fragments, err := loadPromptSourceFragments(root)
	if err != nil {
		t.Fatalf("loadPromptSourceFragments() error = %v", err)
	}
	if len(fragments) != 2 {
		t.Fatalf("fragment count = %d, want 2", len(fragments))
	}
	if got := fragments[0].Label; got != "global-instructions" {
		t.Fatalf("global label = %q", got)
	}
	if got := fragments[0].Content; got != "Global rules." {
		t.Fatalf("global content = %q", got)
	}
	if got := fragments[1].Label; got != "project-instructions" {
		t.Fatalf("project label = %q", got)
	}
	if got := fragments[1].Content; got != "Project rules." {
		t.Fatalf("project content = %q", got)
	}
}

func TestLoadPromptSourceFragmentsIgnoresEmptyFiles(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	globalDir := filepath.Join(configHome, "kodacode")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(global) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, promptInstructionsFilename), []byte("   \n"), 0o644); err != nil {
		t.Fatalf("WriteFile(global) error = %v", err)
	}

	fragments, err := loadPromptSourceFragments(t.TempDir())
	if err != nil {
		t.Fatalf("loadPromptSourceFragments() error = %v", err)
	}
	if len(fragments) != 0 {
		t.Fatalf("fragment count = %d, want 0", len(fragments))
	}
}

func TestGlobalPromptSourcePathFallsBackToDotConfigUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)

	got, err := globalPromptSourcePath()
	if err != nil {
		t.Fatalf("globalPromptSourcePath() error = %v", err)
	}

	want := filepath.Join(home, ".config", "kodacode", promptInstructionsFilename)
	if got != want {
		t.Fatalf("globalPromptSourcePath() = %q, want %q", got, want)
	}
}
