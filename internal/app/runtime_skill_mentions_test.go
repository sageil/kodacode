package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/skill"
)

func TestSkillIDsForTurnAddsExplicitDollarMentions(t *testing.T) {
	available := []skill.Definition{
		{ID: "review"},
		{ID: "mongo-migration"},
	}

	got := skillIDsForTurn("Use $review and ${mongo-migration}; leave $PATH alone.", []string{"review"}, available)
	want := []string{"review", "mongo-migration"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("skillIDsForTurn() = %#v, want %#v", got, want)
	}
}

func TestRunSessionTurnLoadsExplicitDollarMentionedSkill(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, ".kodacode", "skills", "review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(skill) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: review
description: Use for code review.
---

Review with extra care.
`), 0o644); err != nil {
		t.Fatalf("WriteFile(skill) error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)
	registry, err := skill.NewRegistry(skill.RegistryConfig{GlobalDir: t.TempDir()})
	if err != nil {
		t.Fatalf("skill.NewRegistry() error = %v", err)
	}
	runtime.Skills = registry

	if _, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		UserText:      "Use $review on this change.",
	}); err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	instructions := client.requests[0].Instructions
	for _, want := range []string{
		"$review: Use for code review.",
		filepath.Join(root, ".kodacode", "skills", "review", "SKILL.md"),
		"Review with extra care.",
	} {
		if !strings.Contains(instructions, want) {
			t.Fatalf("instructions missing %q:\n%s", want, instructions)
		}
	}
}
