package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

type fakeContextPacketDiagnostics struct {
	files []tool.CodeIntelFileDiagnostics
	err   error
}

func (f fakeContextPacketDiagnostics) DiagnosticsForFiles(_ context.Context, _ string, _ []string, filePaths []string, _ time.Duration) ([]tool.CodeIntelFileDiagnostics, error) {
	if len(f.files) == 0 {
		files := make([]tool.CodeIntelFileDiagnostics, 0, len(filePaths))
		for _, path := range filePaths {
			files = append(files, tool.CodeIntelFileDiagnostics{Path: path})
		}
		return files, f.err
	}
	return append([]tool.CodeIntelFileDiagnostics(nil), f.files...), f.err
}

func TestRuntimeRunSessionTurnInjectsConfiguredDeterministicContextPacket(t *testing.T) {
	workspaceRoot := t.TempDir()
	runGit(t, workspaceRoot, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(workspaceRoot, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.ContextPacket.EnabledSections = []string{
		deterministicContextPacketSectionRepo,
		deterministicContextPacketSectionGit,
		deterministicContextPacketSectionGitDirtySummary,
	}

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: workspaceRoot,
		UserText:      "inspect status",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	instructions := client.requests[0].Instructions
	for _, want := range []string{
		"<deterministic_context_packet>",
		`<section key="repo"`,
		`<section key="git"`,
		`<section key="git_dirty_summary"`,
		"name: " + filepath.Base(workspaceRoot),
		"branch: main",
		"changed_files: 1",
		"total_changed_files: 1",
		"- ?? README.md",
	} {
		if !strings.Contains(instructions, want) {
			t.Fatalf("instructions missing %q:\n%s", want, instructions)
		}
	}
	if strings.Contains(instructions, "diff --git") || strings.Contains(instructions, "@@") {
		t.Fatalf("instructions unexpectedly contain patch content:\n%s", instructions)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns[result.TurnID]
	if turn == nil || turn.Prompt == nil {
		t.Fatalf("turn prompt missing: %#v", turn)
	}
	var packetFragment *events.PromptFragmentState
	for i := range turn.Prompt.Fragments {
		fragment := &turn.Prompt.Fragments[i]
		if fragment.Key == "deterministic_context_packet" {
			packetFragment = fragment
			break
		}
	}
	if packetFragment == nil {
		t.Fatalf("prompt fragments missing deterministic context packet: %#v", turn.Prompt.Fragments)
	}
	if packetFragment.Tokens <= 0 {
		t.Fatalf("packet fragment tokens = %d, want positive", packetFragment.Tokens)
	}
	if packetFragment.Bytes <= 0 {
		t.Fatalf("packet fragment bytes = %d, want positive", packetFragment.Bytes)
	}
	if len(turn.ProviderAttempts) == 0 {
		t.Fatalf("turn provider attempts missing: %#v", turn)
	}
	attempt := turn.ProviderAttempts[0]
	if attempt.DeterministicContextTokens <= 0 {
		t.Fatalf("deterministic context tokens = %d, want positive", attempt.DeterministicContextTokens)
	}
	if attempt.DeterministicContextOmittedTokens != 0 {
		t.Fatalf("deterministic context omitted tokens = %d, want 0", attempt.DeterministicContextOmittedTokens)
	}
}

func TestRuntimeRunSessionTurnOmitsDeterministicContextPacketByDefault(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "inspect status",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	if strings.Contains(client.requests[0].Instructions, "<deterministic_context_packet>") {
		t.Fatalf("instructions unexpectedly contain deterministic context packet:\n%s", client.requests[0].Instructions)
	}
}

func TestRuntimeRunSessionTurnInjectsConfiguredDiagnosticsContextPacket(t *testing.T) {
	workspaceRoot := t.TempDir()
	runGit(t, workspaceRoot, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(workspaceRoot, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.ContextPacketDiagnostics = fakeContextPacketDiagnostics{}
	runtime.Config.ContextPacket.EnabledSections = []string{
		deterministicContextPacketSectionDiagnostics,
	}

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: workspaceRoot,
		UserText:      "fix compile errors",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	instructions := client.requests[0].Instructions
	for _, want := range []string{
		"<deterministic_context_packet>",
		`<section key="diagnostics"`,
		"candidate_files: 1",
		"checked_files: 1",
		"diagnostics_found: 0",
		"- main.go: no diagnostics",
	} {
		if !strings.Contains(instructions, want) {
			t.Fatalf("instructions missing %q:\n%s", want, instructions)
		}
	}
	if strings.Contains(instructions, "package main") {
		t.Fatalf("diagnostics context should not include file contents:\n%s", instructions)
	}
}

func TestRuntimeStartSessionTurnSkipsUnchangedDeterministicContextPacketSections(t *testing.T) {
	workspaceRoot := t.TempDir()
	runGit(t, workspaceRoot, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(workspaceRoot, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "first"}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "second"}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.ContextPacket.EnabledSections = []string{
		deterministicContextPacketSectionRepo,
		deterministicContextPacketSectionGit,
		deterministicContextPacketSectionGitDirtySummary,
	}

	first, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: workspaceRoot,
		UserText:      "inspect status",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	second, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
		SessionID: first.SessionID,
		TurnID:    NewTurnID(),
		UserText:  "continue",
	})
	if err != nil {
		t.Fatalf("StartSessionTurn() error = %v", err)
	}
	if second.Status != TurnRunStatusCompleted {
		t.Fatalf("second result = %#v", second)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	if !requestHasDeterministicContextPacket(client.requests[0]) {
		t.Fatalf("first request missing deterministic context packet")
	}
	if requestHasDeterministicContextPacket(client.requests[1]) {
		t.Fatalf("second request unexpectedly repeated unchanged deterministic context packet:\n%s", provider.PromptText(client.requests[1]))
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), first.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	attempts := state.Turns[second.TurnID].ProviderAttempts
	if len(attempts) == 0 {
		t.Fatalf("second turn provider attempts missing: %#v", state.Turns[second.TurnID])
	}
	if attempts[0].DeterministicContextTokens != 0 {
		t.Fatalf("second deterministic context tokens = %d, want 0", attempts[0].DeterministicContextTokens)
	}
}

func TestRuntimeStartSessionTurnRefreshesChangedGitContextWithoutRepeatingRepoContext(t *testing.T) {
	workspaceRoot := t.TempDir()
	runGit(t, workspaceRoot, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(workspaceRoot, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "first"}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "second"}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.ContextPacket.EnabledSections = []string{
		deterministicContextPacketSectionRepo,
		deterministicContextPacketSectionGit,
		deterministicContextPacketSectionGitDirtySummary,
	}

	first, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: workspaceRoot,
		UserText:      "inspect status",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "server.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	second, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
		SessionID: first.SessionID,
		TurnID:    NewTurnID(),
		UserText:  "continue",
	})
	if err != nil {
		t.Fatalf("StartSessionTurn() error = %v", err)
	}
	if second.Status != TurnRunStatusCompleted {
		t.Fatalf("second result = %#v", second)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	secondPrompt := provider.PromptText(client.requests[1])
	for _, want := range []string{
		"<deterministic_context_packet>",
		`<section key="git"`,
		`<section key="git_dirty_summary"`,
		"changed_files: 2",
		"- ?? server.go",
	} {
		if !strings.Contains(secondPrompt, want) {
			t.Fatalf("second prompt missing %q:\n%s", want, secondPrompt)
		}
	}
	if strings.Contains(secondPrompt, `<section key="repo"`) {
		t.Fatalf("second prompt unexpectedly repeated repo section:\n%s", secondPrompt)
	}
}

func TestRuntimeStartSessionTurnRefreshesGitContextWhenUserAsksForWorkspaceState(t *testing.T) {
	workspaceRoot := t.TempDir()
	runGit(t, workspaceRoot, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(workspaceRoot, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "first"}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "second"}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.ContextPacket.EnabledSections = []string{
		deterministicContextPacketSectionRepo,
		deterministicContextPacketSectionGit,
		deterministicContextPacketSectionGitDirtySummary,
	}

	first, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: workspaceRoot,
		UserText:      "inspect status",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	_, err = runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
		SessionID: first.SessionID,
		TurnID:    NewTurnID(),
		UserText:  "what is the git status now?",
	})
	if err != nil {
		t.Fatalf("StartSessionTurn() error = %v", err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	secondPrompt := provider.PromptText(client.requests[1])
	if !strings.Contains(secondPrompt, `<section key="git"`) || !strings.Contains(secondPrompt, `<section key="git_dirty_summary"`) {
		t.Fatalf("second prompt missing requested git context:\n%s", secondPrompt)
	}
	if strings.Contains(secondPrompt, `<section key="repo"`) {
		t.Fatalf("second prompt unexpectedly repeated repo section:\n%s", secondPrompt)
	}
}

func TestDeterministicContextPacketUserRequestsDiagnostics(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{text: "fix the failing tests", want: true},
		{text: "why does this compile error happen", want: true},
		{text: "review the branch", want: false},
		{text: "where is the router implemented", want: false},
	}
	for _, tt := range tests {
		if got := deterministicContextPacketUserRequestsDiagnostics(tt.text); got != tt.want {
			t.Fatalf("deterministicContextPacketUserRequestsDiagnostics(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestLastSentDeterministicContextPacketSectionsIgnoresPressureOmittedTurns(t *testing.T) {
	packet := renderDeterministicContextPacket([]deterministicContextPacketSection{{
		Key:       deterministicContextPacketSectionRepo,
		Label:     "Repository Summary",
		Source:    "workspace metadata",
		Freshness: "current",
		Content:   "name: kodacode\ngit: detected",
	}})
	state := events.SessionState{
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				Prompt: &events.PromptState{
					Instructions: packet,
				},
				ProviderAttempts: []events.TurnProviderAttemptState{{
					DeterministicContextTokens:        0,
					DeterministicContextOmittedTokens: 12,
				}},
			},
		},
	}

	sections := lastSentDeterministicContextPacketSections(state, []string{deterministicContextPacketSectionRepo})
	if len(sections) != 0 {
		t.Fatalf("sections = %#v, want none from pressure-omitted turn", sections)
	}

	state.TurnOrder = append(state.TurnOrder, "turn-2")
	state.Turns["turn-2"] = &events.TurnState{
		Prompt: &events.PromptState{
			Instructions: packet,
		},
		ProviderAttempts: []events.TurnProviderAttemptState{{
			DeterministicContextTokens: 12,
		}},
	}
	sections = lastSentDeterministicContextPacketSections(state, []string{deterministicContextPacketSectionRepo})
	if got, want := sections[deterministicContextPacketSectionRepo], "name: kodacode\ngit: detected"; got != want {
		t.Fatalf("repo section = %q, want %q", got, want)
	}
}

func TestRuntimeRunSessionTurnOmitsDeterministicContextPacketUnderProviderRequestPressure(t *testing.T) {
	workspaceRoot := t.TempDir()
	runGit(t, workspaceRoot, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(workspaceRoot, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
		})},
		counts:       []int{850, 650},
		countSources: []provider.TokenCountSource{provider.TokenCountSourceExact, provider.TokenCountSourceExact},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.ContextPacket.EnabledSections = []string{
		deterministicContextPacketSectionRepo,
		deterministicContextPacketSectionGit,
	}
	runtime.ModelCatalog = &fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:             "gpt-5",
				MaxInputTokens: 1000,
			}},
		},
	}
	runtime.Runner.SetModelCatalog(runtime.ModelCatalog)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: workspaceRoot,
		UserText:      strings.Repeat("near input pressure ", 200),
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.countRequests) < 2 {
		t.Fatalf("count requests = %d, want at least 2", len(client.countRequests))
	}
	if !strings.Contains(provider.PromptText(client.countRequests[0]), "<deterministic_context_packet>") {
		t.Fatalf("first counted request missing deterministic context packet")
	}
	if strings.Contains(provider.PromptText(client.countRequests[1]), "<deterministic_context_packet>") {
		t.Fatalf("second counted request still contains deterministic context packet:\n%s", provider.PromptText(client.countRequests[1]))
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	if strings.Contains(provider.PromptText(client.requests[0]), "<deterministic_context_packet>") {
		t.Fatalf("sent provider request still contains deterministic context packet:\n%s", provider.PromptText(client.requests[0]))
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns[result.TurnID]
	if turn == nil || len(turn.ProviderAttempts) == 0 {
		t.Fatalf("turn provider attempts missing: %#v", turn)
	}
	attempt := turn.ProviderAttempts[0]
	if attempt.DeterministicContextTokens != 0 {
		t.Fatalf("deterministic context tokens = %d, want 0 after omission", attempt.DeterministicContextTokens)
	}
	if attempt.DeterministicContextOmittedTokens <= 0 {
		t.Fatalf("deterministic context omitted tokens = %d, want positive", attempt.DeterministicContextOmittedTokens)
	}
}
