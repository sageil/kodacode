package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/workspace"
)

type sessionCatalogStore struct {
	indexed    []events.SessionIndexEntry
	titleByID  map[string]events.Event
	statusByID map[string]events.Event
	branchByID map[string]events.Event
	queries    []events.LatestQuery
}

func (s *sessionCatalogStore) Append(context.Context, events.Draft) (events.Event, error) {
	panic("Append should not be called in session catalog tests")
}

func (s *sessionCatalogStore) Replay(context.Context, events.Query) ([]events.Event, error) {
	panic("Replay should not be called in session catalog tests")
}

func (s *sessionCatalogStore) Watch(context.Context, events.Query) (<-chan events.Event, error) {
	panic("Watch should not be called in session catalog tests")
}

func (s *sessionCatalogStore) Latest(_ context.Context, query events.LatestQuery) (events.Event, bool, error) {
	s.queries = append(s.queries, events.LatestQuery{
		SessionID: query.SessionID,
		Types:     slices.Clone(query.Types),
	})
	if len(query.Types) == 1 && query.Types[0] == events.TypeSessionTitleUpdated {
		event, ok := s.titleByID[query.SessionID]
		return event, ok, nil
	}
	if len(query.Types) == 1 && query.Types[0] == events.TypeSessionBranched {
		event, ok := s.branchByID[query.SessionID]
		return event, ok, nil
	}
	event, ok := s.statusByID[query.SessionID]
	return event, ok, nil
}

func (s *sessionCatalogStore) ListWorkspaceSessions(_ context.Context, workspaceRoot string) ([]events.SessionIndexEntry, error) {
	var filtered []events.SessionIndexEntry
	for _, entry := range s.indexed {
		if entry.WorkspaceRoot == workspaceRoot {
			filtered = append(filtered, entry)
		}
	}
	return filtered, nil
}

func (s *sessionCatalogStore) ListSessions(_ context.Context) ([]events.SessionIndexEntry, error) {
	return slices.Clone(s.indexed), nil
}

func TestRuntimeListWorkspaceSessionsUsesIndexedMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll(root) error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New(root) error = %v", err)
	}
	updatedAt := time.Unix(1710000000, 0).UTC()
	store := &sessionCatalogStore{
		indexed: []events.SessionIndexEntry{
			{SessionID: "session-1", WorkspaceRoot: scope.Root(), UpdatedAt: updatedAt},
			{SessionID: "session-2", WorkspaceRoot: scope.Root(), UpdatedAt: updatedAt.Add(time.Minute)},
		},
		titleByID: map[string]events.Event{
			"session-1": {
				Type:    events.TypeSessionTitleUpdated,
				Payload: events.SessionTitleUpdatedPayload{Title: "Retry analysis"},
			},
		},
		statusByID: map[string]events.Event{
			"session-1": {
				Type:    events.TypeTurnError,
				Payload: events.TurnErrorPayload{Message: "rate limited"},
			},
			"session-2": {
				Type:    events.TypeTurnConfigured,
				Payload: events.TurnConfiguredPayload{AgentID: "builder", Model: "openai/gpt-5", ResponseStyle: "terse"},
			},
		},
	}
	runtime := &Runtime{Store: store}

	summaries, err := runtime.ListWorkspaceSessions(context.Background(), root)
	if err != nil {
		t.Fatalf("ListWorkspaceSessions() error = %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("summary count = %d, want 2", len(summaries))
	}

	if got := summaries[0]; got.ID != "session-1" || got.Title != "Retry analysis" || got.Status != events.TurnStatusFailed {
		t.Fatalf("summary[0] = %#v", got)
	}
	if got := summaries[1]; got.ID != "session-2" || got.Title != "repo" || got.Status != events.TurnStatusRunning {
		t.Fatalf("summary[1] = %#v", got)
	}

	if len(store.queries) != 6 {
		t.Fatalf("latest query count = %d, want 6", len(store.queries))
	}
	for _, query := range store.queries {
		if query.SessionID == "" {
			t.Fatalf("latest query missing session id: %#v", query)
		}
	}
}

func TestRuntimeListSessionsUsesGlobalIndex(t *testing.T) {
	rootA := filepath.Join(t.TempDir(), "repo-a")
	rootB := filepath.Join(t.TempDir(), "repo-b")
	for _, root := range []string{rootA, rootB} {
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", root, err)
		}
	}
	scopeA, err := workspace.New(rootA)
	if err != nil {
		t.Fatalf("workspace.New(rootA) error = %v", err)
	}
	scopeB, err := workspace.New(rootB)
	if err != nil {
		t.Fatalf("workspace.New(rootB) error = %v", err)
	}
	updatedAt := time.Unix(1710000000, 0).UTC()
	store := &sessionCatalogStore{
		indexed: []events.SessionIndexEntry{
			{SessionID: "session-a", WorkspaceRoot: scopeA.Root(), UpdatedAt: updatedAt},
			{SessionID: "session-b", WorkspaceRoot: scopeB.Root(), UpdatedAt: updatedAt.Add(time.Minute)},
		},
		statusByID: map[string]events.Event{
			"session-a": {
				Type:    events.TypeTurnDone,
				Payload: events.TurnDonePayload{},
			},
			"session-b": {
				Type:    events.TypeTurnConfigured,
				Payload: events.TurnConfiguredPayload{AgentID: "builder", Model: "openai/gpt-5", ResponseStyle: "terse"},
			},
		},
		branchByID: map[string]events.Event{
			"session-b": {
				Type: events.TypeSessionBranched,
				Payload: events.SessionBranchedPayload{
					ParentSessionID: "session-a",
					ParentTurnID:    "turn-1",
					ParentSequence:  3,
				},
			},
		},
	}
	runtime := &Runtime{Store: store}

	summaries, err := runtime.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("summary count = %d, want 2", len(summaries))
	}
	if got := summaries[0]; got.ID != "session-a" || got.WorkspaceRoot != scopeA.Root() || got.Status != events.TurnStatusCompleted {
		t.Fatalf("summary[0] = %#v", got)
	}
	if got := summaries[1]; got.ID != "session-b" || got.WorkspaceRoot != scopeB.Root() || got.Status != events.TurnStatusRunning {
		t.Fatalf("summary[1] = %#v", got)
	}
	if branch := summaries[1].Branch; branch == nil || branch.ParentSessionID != "session-a" || branch.ParentTurnID != "turn-1" {
		t.Fatalf("summary[1].Branch = %#v", branch)
	}
}

func TestRuntimeListSessionsIncludesOnlyFreshBranchSummaryArtifacts(t *testing.T) {
	ctx := context.Background()
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	runtime := &Runtime{Store: store, Sessions: sessions}
	sourceSessionID, err := runtime.CreateSession(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	appendTimelineTurnForTest(t, sessions, sourceSessionID, "turn-1", "branch here", "done")
	branch, err := runtime.BranchSessionFromTurn(ctx, BranchSessionFromTurnInput{
		SourceSessionID: sourceSessionID,
		SourceTurnID:    "turn-1",
	})
	if err != nil {
		t.Fatalf("BranchSessionFromTurn() error = %v", err)
	}
	state, err := sessions.Snapshot(ctx, branch.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if err := store.SaveBranchSummary(ctx, events.BranchSummaryArtifact{
		SessionID:        branch.SessionID,
		SourceSequence:   state.LastSequence,
		Summary:          "fresh branch summary",
		Model:            "openai/gpt-5-mini",
		PromptTokens:     10,
		CompletionTokens: 5,
	}); err != nil {
		t.Fatalf("SaveBranchSummary(fresh) error = %v", err)
	}
	summaries, err := runtime.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	branchSummary := findSessionSummaryForTest(summaries, branch.SessionID)
	if branchSummary.BranchSummary == nil || branchSummary.BranchSummary.Summary != "fresh branch summary" {
		t.Fatalf("fresh branch summary = %#v", branchSummary.BranchSummary)
	}

	if _, err := sessions.SetTitle(ctx, branch.SessionID, "changed branch"); err != nil {
		t.Fatalf("SetTitle() error = %v", err)
	}
	summaries, err = runtime.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions(stale) error = %v", err)
	}
	branchSummary = findSessionSummaryForTest(summaries, branch.SessionID)
	if branchSummary.BranchSummary != nil {
		t.Fatalf("stale branch summary = %#v, want nil", branchSummary.BranchSummary)
	}
}

func findSessionSummaryForTest(summaries []SessionSummary, sessionID string) SessionSummary {
	for _, summary := range summaries {
		if summary.ID == sessionID {
			return summary
		}
	}
	return SessionSummary{}
}
