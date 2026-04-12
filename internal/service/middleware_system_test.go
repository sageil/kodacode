package service_test

import (
	"context"
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/service"
	"github.com/sageil/kodacode/v1/internal/tool"
)

func TestSystemPromptMiddleware_SetsThreeParts(t *testing.T) {
	builder := service.NewSystemPromptBuilder(service.SystemPromptBuilderConfig{
		ProjectDir: t.TempDir(),
		SkillsDir:  t.TempDir(),
	})
	mw := service.NewSystemPromptMiddleware(builder, tool.NewTaskStore(nil))
	req := &pipeline.TurnRequest{
		SessionID: "s1",
		Agent:     config.AgentConfig{SystemPrompt: "You are a builder."},
	}
	err := mw(context.Background(), req, func(_ context.Context, r *pipeline.TurnRequest) error {
		if len(r.SystemParts) != 3 {
			t.Errorf("NewSystemPromptMiddleware() SystemParts = %d, want 3", len(r.SystemParts))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSystemPromptBuilder_AlwaysReturnsThreeParts(t *testing.T) {
	builder := service.NewSystemPromptBuilder(service.SystemPromptBuilderConfig{
		ProjectDir: t.TempDir(),
		SkillsDir:  t.TempDir(),
	})
	parts, err := builder.Build(context.Background(), service.SystemPromptBuildInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 3 {
		t.Errorf("Build() = %d parts, want exactly 3", len(parts))
	}
}

func TestSystemPromptBuilder_StablePartContainsAgentPrompt(t *testing.T) {
	builder := service.NewSystemPromptBuilder(service.SystemPromptBuilderConfig{
		ProjectDir: t.TempDir(),
		SkillsDir:  t.TempDir(),
	})
	parts, err := builder.Build(context.Background(), service.SystemPromptBuildInput{AgentPrompt: "You are a specialist."})
	if err != nil {
		t.Fatal(err)
	}
	if parts[0] == "" {
		t.Error("Build() stable part should not be empty when agentPrompt is set")
	}
}

func TestSystemPromptBuilder_DynamicPartContainsEnvBlock(t *testing.T) {
	builder := service.NewSystemPromptBuilder(service.SystemPromptBuilderConfig{
		ProjectDir: t.TempDir(),
		SkillsDir:  t.TempDir(),
	})
	parts, err := builder.Build(context.Background(), service.SystemPromptBuildInput{ModelID: "anthropic/claude-sonnet-4-6"})
	if err != nil {
		t.Fatal(err)
	}
	if parts[1] == "" {
		t.Error("Build() dynamic part should not be empty")
	}
}

func TestSystemPromptMiddleware_TaskProgressSeparatesDefinitionAndProgress(t *testing.T) {
	builder := service.NewSystemPromptBuilder(service.SystemPromptBuilderConfig{
		ProjectDir: t.TempDir(),
		SkillsDir:  t.TempDir(),
	})
	store := tool.NewTaskStore(nil)
	if _, _, err := store.CreateTask(context.Background(), "s1", "Harden tracing", "in_progress", "Acceptance criteria:\n- trace id is present\n- logs stay structured"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateTask(context.Background(), "s1", "task 1", "in_progress", "audit complete, findings documented", "", false); err != nil {
		t.Fatal(err)
	}

	mw := service.NewSystemPromptMiddleware(builder, store)
	req := &pipeline.TurnRequest{
		SessionID: "s1",
		Agent:     config.AgentConfig{SystemPrompt: "You are a builder."},
	}
	err := mw(context.Background(), req, func(_ context.Context, r *pipeline.TurnRequest) error {
		if len(r.SystemParts) != 3 {
			t.Fatalf("SystemParts = %d, want 3", len(r.SystemParts))
		}
		volatile := r.SystemParts[2]
		if !strings.Contains(volatile, "Current task") {
			t.Fatalf("volatile part missing current task block: %q", volatile)
		}
		if !strings.Contains(volatile, "Definition: Acceptance criteria:") {
			t.Fatalf("volatile part missing durable definition label: %q", volatile)
		}
		if !strings.Contains(volatile, "Progress: audit complete, findings documented") {
			t.Fatalf("volatile part missing transient progress label: %q", volatile)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
