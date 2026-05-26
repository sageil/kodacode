package app

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sageil/kodacode/internal/workspace"
)

func TestParseCommandInputParsesRepeatedSkills(t *testing.T) {
	input, err := ParseCommandInput([]string{
		"--resume",
		"--skill", "review",
		"--skill=go",
		"--skill", "review",
		"inspect", "the", "repo",
	}, t.TempDir())
	if err != nil {
		t.Fatalf("ParseCommandInput() error = %v", err)
	}
	if input.UserText != "inspect the repo" {
		t.Fatalf("UserText = %q", input.UserText)
	}
	if !reflect.DeepEqual(input.SkillIDs, []string{"review", "go"}) {
		t.Fatalf("SkillIDs = %#v", input.SkillIDs)
	}
	if !input.Resume {
		t.Fatal("Resume = false, want true")
	}
}

func TestParseCommandInputSupportsDoubleDashTerminator(t *testing.T) {
	input, err := ParseCommandInput([]string{
		"--skill", "review",
		"--",
		"--skill", "is", "part", "of", "the", "prompt",
	}, t.TempDir())
	if err != nil {
		t.Fatalf("ParseCommandInput() error = %v", err)
	}
	if input.UserText != "--skill is part of the prompt" {
		t.Fatalf("UserText = %q", input.UserText)
	}
	if !reflect.DeepEqual(input.SkillIDs, []string{"review"}) {
		t.Fatalf("SkillIDs = %#v", input.SkillIDs)
	}
}

func TestParseCommandInputTreatsLeadingDirectoryArgAsWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	input, err := ParseCommandInput([]string{".", "inspect", "the", "repo"}, root)
	if err != nil {
		t.Fatalf("ParseCommandInput() error = %v", err)
	}
	if input.WorkspaceRoot != scope.Root() {
		t.Fatalf("WorkspaceRoot = %q, want %q", input.WorkspaceRoot, scope.Root())
	}
	if input.UserText != "inspect the repo" {
		t.Fatalf("UserText = %q", input.UserText)
	}
}

func TestParseCommandInputTreatsSingleDirectoryArgAsWorkspaceRootOnly(t *testing.T) {
	root := t.TempDir()
	subdir := filepath.Join(root, "pkg")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	scope, err := workspace.New(subdir)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	input, err := ParseCommandInput([]string{"pkg"}, root)
	if err != nil {
		t.Fatalf("ParseCommandInput() error = %v", err)
	}
	if input.WorkspaceRoot != scope.Root() {
		t.Fatalf("WorkspaceRoot = %q, want %q", input.WorkspaceRoot, scope.Root())
	}
	if input.UserText != "" {
		t.Fatalf("UserText = %q, want empty", input.UserText)
	}
}

func TestParseCommandInputDoubleDashKeepsDotAsLiteralPrompt(t *testing.T) {
	root := t.TempDir()
	input, err := ParseCommandInput([]string{"--", ".", "inspect"}, root)
	if err != nil {
		t.Fatalf("ParseCommandInput() error = %v", err)
	}
	if input.WorkspaceRoot != "" {
		t.Fatalf("WorkspaceRoot = %q, want empty", input.WorkspaceRoot)
	}
	if input.UserText != ". inspect" {
		t.Fatalf("UserText = %q", input.UserText)
	}
}

func TestParseCommandInputParsesAdditionalWorkspaceRoots(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "pkg")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	external := t.TempDir()
	nestedScope, err := workspace.New(nested)
	if err != nil {
		t.Fatalf("workspace.New(nested) error = %v", err)
	}
	externalScope, err := workspace.New(external)
	if err != nil {
		t.Fatalf("workspace.New(external) error = %v", err)
	}

	input, err := ParseCommandInput([]string{
		"--add-dir", "pkg",
		"--add-dir=" + external,
		"inspect", "the", "repo",
	}, root)
	if err != nil {
		t.Fatalf("ParseCommandInput() error = %v", err)
	}
	if input.UserText != "inspect the repo" {
		t.Fatalf("UserText = %q", input.UserText)
	}
	if !reflect.DeepEqual(input.AdditionalWorkspaceRoots, []string{nestedScope.Root(), externalScope.Root()}) {
		t.Fatalf("AdditionalWorkspaceRoots = %#v", input.AdditionalWorkspaceRoots)
	}
}

func TestParseCommandInputRejectsMissingSkillValue(t *testing.T) {
	_, err := ParseCommandInput([]string{"--skill"}, t.TempDir())
	if !errors.Is(err, ErrCommandOptionValueRequired) {
		t.Fatalf("ParseCommandInput() error = %v, want %v", err, ErrCommandOptionValueRequired)
	}
}

func TestParseCommandInputRejectsUnknownOption(t *testing.T) {
	_, err := ParseCommandInput([]string{"--agent", "planner", "inspect", "the", "repo"}, t.TempDir())
	if !errors.Is(err, ErrUnknownCommandOption) {
		t.Fatalf("ParseCommandInput() error = %v, want %v", err, ErrUnknownCommandOption)
	}
}
