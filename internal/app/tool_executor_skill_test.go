package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/skill"
	"github.com/sageil/kodacode/internal/tool"
)

func TestToolExecutorExecutesSkillDiscoveryToolsAgainstWorkspaceSkills(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewSearchSkillsTool(), tool.NewSkillTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	projectSkills := filepath.Join(root, ".kodacode", "skills")
	skillDir := filepath.Join(projectSkills, "migration")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(skills) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
description: Mongo migration workflow
---

# Overview

Use this skill when changing mongoose aggregate pipelines.

## Checklist

- inspect indexes
- verify rollout
`), 0o644); err != nil {
		t.Fatalf("WriteFile(skill) error = %v", err)
	}

	registry, err := skill.NewRegistry(skill.RegistryConfig{GlobalDir: t.TempDir()})
	if err != nil {
		t.Fatalf("skill.NewRegistry() error = %v", err)
	}
	executor.SetSkillRegistry(registry)

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	searchArgs, err := json.Marshal(map[string]any{"query": "mongoose migration", "limit": 5})
	if err != nil {
		t.Fatalf("json.Marshal(search) error = %v", err)
	}
	searchResult, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.SearchSkillsToolName,
		Arguments:  searchArgs,
	})
	if err != nil {
		t.Fatalf("Execute(search_skills) error = %v", err)
	}
	if searchResult.Status != ToolExecutionStatusExecuted || !strings.Contains(searchResult.Output, `"migration"`) {
		t.Fatalf("search result = %#v", searchResult)
	}

	skillResult, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-2",
		ToolName:   tool.SkillToolName,
		Arguments:  json.RawMessage(`{"id":"migration","section":"checklist"}`),
	})
	if err != nil {
		t.Fatalf("Execute(skill) error = %v", err)
	}
	if skillResult.Status != ToolExecutionStatusExecuted || !strings.Contains(skillResult.Output, `- inspect indexes`) {
		t.Fatalf("skill result = %#v", skillResult)
	}
}

func TestToolExecutorExecutesSkillDiscoveryToolsAgainstGlobalSkillsWithWildcard(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewSearchSkillsTool(), tool.NewSkillTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	globalDir := t.TempDir()
	globalSkillDir := filepath.Join(globalDir, "review")
	if err := os.MkdirAll(globalSkillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(global skill) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalSkillDir, "SKILL.md"), []byte(`---
description: Code review workflow
---

Use this skill when reviewing a codebase.
`), 0o644); err != nil {
		t.Fatalf("WriteFile(global skill) error = %v", err)
	}

	registry, err := skill.NewRegistry(skill.RegistryConfig{GlobalDir: globalDir})
	if err != nil {
		t.Fatalf("skill.NewRegistry() error = %v", err)
	}
	executor.SetSkillRegistry(registry)

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	searchArgs, err := json.Marshal(map[string]any{"query": "*", "limit": 10})
	if err != nil {
		t.Fatalf("json.Marshal(search) error = %v", err)
	}
	searchResult, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.SearchSkillsToolName,
		Arguments:  searchArgs,
	})
	if err != nil {
		t.Fatalf("Execute(search_skills) error = %v", err)
	}
	if searchResult.Status != ToolExecutionStatusExecuted || !strings.Contains(searchResult.Output, `"review"`) || !strings.Contains(searchResult.Output, `"global"`) {
		t.Fatalf("search result = %#v", searchResult)
	}
}
