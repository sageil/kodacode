package service_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sageil/kodacode/v1/internal/agent"
	"github.com/sageil/kodacode/v1/internal/permission"
	"github.com/sageil/kodacode/v1/internal/service"
)

// newTestAgentService creates an AgentService backed by a temp project dir.
// No global dir is set (empty string), so only builtins + project-local agents
// are loaded.
func newTestAgentService(t *testing.T) (*service.AgentService, string) {
	t.Helper()
	projectDir := filepath.Join(t.TempDir(), ".kodacode", "agents")
	svc, err := service.NewAgentService("", projectDir)
	if err != nil {
		t.Fatalf("NewAgentService: %v", err)
	}
	return svc, projectDir
}

func TestAgentService_ListIncludesBuiltins(t *testing.T) {
	svc, _ := newTestAgentService(t)
	agents := svc.List()
	if len(agents) == 0 {
		t.Fatal("List() returned 0 agents, want at least built-in agents")
	}
	// All built-in agents must have Builtin = true.
	for _, a := range agents {
		if a.Builtin && a.ID == "" {
			t.Errorf("builtin agent has empty ID")
		}
	}
}

func TestAgentService_GetBuiltin(t *testing.T) {
	svc, _ := newTestAgentService(t)

	// "engineer" is a built-in agent from agents/engineer.md.
	a, err := svc.Get("engineer")
	if err != nil {
		t.Fatalf("Get(%q) error: %v", "engineer", err)
	}
	if a.ID != "engineer" {
		t.Errorf("Get(%q) ID = %q, want %q", "engineer", a.ID, "engineer")
	}
	if !a.Builtin {
		t.Errorf("Get(%q) Builtin = false, want true", "engineer")
	}
}

func TestAgentService_GetNotFound(t *testing.T) {
	svc, _ := newTestAgentService(t)

	_, err := svc.Get("nonexistent-agent")
	if !errors.Is(err, agent.ErrNotFound) {
		t.Errorf("Get(%q) error = %v, want %v", "nonexistent-agent", err, agent.ErrNotFound)
	}
}

func TestAgentService_CreateAndGet(t *testing.T) {
	svc, _ := newTestAgentService(t)

	temp := 0.5
	a := agent.Agent{
		ID:          "myagent",
		Name:        "My Agent",
		Description: "A test agent",
		Model:       "openai/gpt-4o",
		Temperature: &temp,
		MaxTokens:   2048,
		Tools:       []string{"bash"},
		Permission: permission.Config{
			"bash": {Action: permission.ActionAsk},
		},
		SystemPrompt: "You are a test agent.",
	}

	created, err := svc.Create(a)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if created.ID != "myagent" {
		t.Errorf("Create() ID = %q, want %q", created.ID, "myagent")
	}
	if created.Builtin {
		t.Errorf("Create() Builtin = true, want false")
	}

	// Get should return the newly created agent.
	got, err := svc.Get("myagent")
	if err != nil {
		t.Fatalf("Get(%q) after Create() error: %v", "myagent", err)
	}
	if got.Description != "A test agent" {
		t.Errorf("Get() Description = %q, want %q", got.Description, "A test agent")
	}
}

func TestAgentService_CreateDuplicate(t *testing.T) {
	svc, _ := newTestAgentService(t)

	a := agent.Agent{ID: "dupeagent", Name: "dupe", Model: "openai/gpt-4o", SystemPrompt: "hi"}
	if _, err := svc.Create(a); err != nil {
		t.Fatalf("first Create() error: %v", err)
	}
	_, err := svc.Create(a)
	if err == nil {
		t.Fatal("second Create() with duplicate ID: want error, got nil")
	}
}

func TestAgentService_UpdateUserDefined(t *testing.T) {
	svc, _ := newTestAgentService(t)

	// Create first.
	a := agent.Agent{ID: "upagent", Name: "Up Agent", Model: "openai/gpt-4o", SystemPrompt: "v1"}
	if _, err := svc.Create(a); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// Update the description.
	a.Description = "updated description"
	a.SystemPrompt = "v2"
	updated, err := svc.Update("upagent", a)
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if updated.Description != "updated description" {
		t.Errorf("Update() Description = %q, want %q", updated.Description, "updated description")
	}
	if updated.SystemPrompt != "v2" {
		t.Errorf("Update() SystemPrompt = %q, want %q", updated.SystemPrompt, "v2")
	}
}

func TestAgentService_UpdateBuiltinForbidden(t *testing.T) {
	svc, _ := newTestAgentService(t)

	_, err := svc.Update("engineer", agent.Agent{ID: "engineer", Name: "x", Model: "openai/gpt-4o"})
	if !errors.Is(err, agent.ErrBuiltin) {
		t.Errorf("Update(builtin) error = %v, want %v", err, agent.ErrBuiltin)
	}
}

func TestAgentService_UpdateNotFound(t *testing.T) {
	svc, _ := newTestAgentService(t)

	_, err := svc.Update("ghost", agent.Agent{ID: "ghost"})
	if !errors.Is(err, agent.ErrNotFound) {
		t.Errorf("Update(nonexistent) error = %v, want %v", err, agent.ErrNotFound)
	}
}

func TestAgentService_DeleteUserDefined(t *testing.T) {
	svc, projectDir := newTestAgentService(t)

	a := agent.Agent{ID: "delagent", Name: "Del Agent", Model: "openai/gpt-4o", SystemPrompt: "bye"}
	if _, err := svc.Create(a); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if err := svc.Delete("delagent"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	// File must be gone.
	if _, err := os.Stat(filepath.Join(projectDir, "delagent.md")); !os.IsNotExist(err) {
		t.Errorf("agent file still exists after Delete()")
	}

	// Get must return ErrNotFound.
	_, err := svc.Get("delagent")
	if !errors.Is(err, agent.ErrNotFound) {
		t.Errorf("Get() after Delete() error = %v, want %v", err, agent.ErrNotFound)
	}
}

func TestAgentService_DeleteBuiltinForbidden(t *testing.T) {
	svc, _ := newTestAgentService(t)

	err := svc.Delete("engineer")
	if !errors.Is(err, agent.ErrBuiltin) {
		t.Errorf("Delete(builtin) error = %v, want %v", err, agent.ErrBuiltin)
	}
}

func TestAgentService_DeleteNotFound(t *testing.T) {
	svc, _ := newTestAgentService(t)

	err := svc.Delete("ghost")
	if !errors.Is(err, agent.ErrNotFound) {
		t.Errorf("Delete(nonexistent) error = %v, want %v", err, agent.ErrNotFound)
	}
}
