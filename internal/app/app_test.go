package app

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func TestRunCommandExecutesOneShotTurnAndWritesAssistantText(t *testing.T) {
	var out bytes.Buffer
	runtime := newRuntimeWithClient(t, &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "hello"},
		})},
	})

	err := runCommand(
		context.Background(),
		strings.NewReader("1\n"),
		&out,
		[]string{"say", "hello"},
		func(string) string { return "" },
		func() (string, error) { return t.TempDir(), nil },
		func(Config) (*Runtime, error) { return runtime, nil },
	)
	if err != nil {
		t.Fatalf("runCommand() error = %v", err)
	}
	if got := out.String(); !strings.HasSuffix(got, "hello\n") {
		t.Fatalf("output = %q", got)
	}
}

func TestRunCommandResumeReusesWorkspaceSession(t *testing.T) {
	sessionDir := t.TempDir()
	workspaceRoot := t.TempDir()
	var firstOut bytes.Buffer
	var secondOut bytes.Buffer

	clientOne := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "first reply"},
		})},
	}
	clientTwo := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "second reply"},
		})},
	}
	runtimes := []*Runtime{
		newPersistentRuntimeWithClient(t, sessionDir, clientOne),
		newPersistentRuntimeWithClient(t, sessionDir, clientTwo),
	}
	buildIndex := 0

	buildRuntime := func(Config) (*Runtime, error) {
		runtime := runtimes[buildIndex]
		buildIndex++
		return runtime, nil
	}

	if err := runCommand(
		context.Background(),
		strings.NewReader("1\n"),
		&firstOut,
		[]string{"first", "question"},
		func(string) string { return "" },
		func() (string, error) { return workspaceRoot, nil },
		buildRuntime,
	); err != nil {
		t.Fatalf("runCommand(first) error = %v", err)
	}
	if err := runCommand(
		context.Background(),
		strings.NewReader("1\n"),
		&secondOut,
		[]string{"--resume", "second", "question"},
		func(string) string { return "" },
		func() (string, error) { return workspaceRoot, nil },
		buildRuntime,
	); err != nil {
		t.Fatalf("runCommand(resume) error = %v", err)
	}

	if got := firstOut.String(); !strings.HasSuffix(got, "first reply\n") {
		t.Fatalf("first output = %q", got)
	}
	if got := secondOut.String(); !strings.HasSuffix(got, "second reply\n") {
		t.Fatalf("second output = %q", got)
	}
	if len(clientTwo.requests) != 1 {
		t.Fatalf("second provider requests = %d, want 1", len(clientTwo.requests))
	}
	if len(clientTwo.requests[0].Inputs) != 3 {
		t.Fatalf("second inputs = %#v", clientTwo.requests[0].Inputs)
	}
	if clientTwo.requests[0].Inputs[0].Content != "first question" || clientTwo.requests[0].Inputs[1].Content != "first reply" || clientTwo.requests[0].Inputs[2].Content != "second question" {
		t.Fatalf("second inputs = %#v", clientTwo.requests[0].Inputs)
	}
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	dbCount := 0
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".db" {
			dbCount++
		}
	}
	if dbCount != 1 {
		t.Fatalf("db file count = %d, want 1", dbCount)
	}
}

func TestRunCommandRequiresUserText(t *testing.T) {
	err := runCommand(
		context.Background(),
		strings.NewReader(""),
		&bytes.Buffer{},
		nil,
		func(string) string { return "" },
		func() (string, error) { return "", nil },
		func(Config) (*Runtime, error) { return nil, nil },
	)
	if !errors.Is(err, ErrUserTextRequired) {
		t.Fatalf("runCommand() error = %v, want ErrUserTextRequired", err)
	}
}

func TestRunCommandAppliesSelectedSkills(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, ".kodacode", "skills", "review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: review
description: review skill
---

Focus on review quality and avoid edits.
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var out bytes.Buffer
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "review"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)

	err := runCommand(
		context.Background(),
		strings.NewReader("1\n"),
		&out,
		[]string{"--skill", "review", "review", "the", "repo"},
		func(string) string { return "" },
		func() (string, error) { return root, nil },
		func(Config) (*Runtime, error) { return runtime, nil },
	)
	if err != nil {
		t.Fatalf("runCommand() error = %v", err)
	}
	if got := out.String(); !strings.HasSuffix(got, "review\n") {
		t.Fatalf("output = %q", got)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	if got := client.requests[0].Instructions; !containsAll(got, []string{"Focus on review quality and avoid edits.", "Current workspace directory:", "not the filesystem root `/`", "Host operating system:"}) {
		t.Fatalf("instructions = %q", got)
	}
	gotTools := make([]string, 0, len(client.requests[0].Tools))
	for _, tool := range client.requests[0].Tools {
		gotTools = append(gotTools, tool.Name)
	}
	for _, want := range []string{"read", "search", "apply_patch", "write", "question"} {
		if !containsString(gotTools, want) {
			t.Fatalf("tools = %#v, want agent-owned surface to include %q", gotTools, want)
		}
	}
}

func TestPromptPermissionUsesTargetLabelForNetwork(t *testing.T) {
	var out bytes.Buffer

	decision, scope, grantPath, recursive, err := promptPermission(
		bufio.NewReader(strings.NewReader("2\n")),
		&out,
		events.PermissionRequestState{
			Kind:     events.PermissionRequestKindNetwork,
			Path:     "external network",
			ToolName: "bash",
			Command:  "curl https://example.com",
		},
	)
	if err != nil {
		t.Fatalf("promptPermission() error = %v", err)
	}
	if decision != events.PermissionDecisionApproved || scope != events.PermissionScopeSession || grantPath != "" || recursive {
		t.Fatalf("result = %q %q %q %v", decision, scope, grantPath, recursive)
	}
	if got := out.String(); !strings.Contains(got, "target: external network\n") {
		t.Fatalf("output = %q", got)
	}
}
