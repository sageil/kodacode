package service

import (
	"context"
	"testing"
	"time"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/repository"
)

type accountingSessionRepo struct {
	sessions  map[string]repository.Session
	listCalls int
}

func (r *accountingSessionRepo) Create(_ context.Context, s repository.Session) (repository.Session, error) {
	if r.sessions == nil {
		r.sessions = make(map[string]repository.Session)
	}
	if s.ID == "" {
		s.ID = time.Now().UTC().Format(time.RFC3339Nano)
	}
	r.sessions[s.ID] = s
	return s, nil
}

func (r *accountingSessionRepo) Get(_ context.Context, id string) (repository.Session, error) {
	s, ok := r.sessions[id]
	if !ok {
		return repository.Session{}, repository.ErrNotFound
	}
	return s, nil
}

func (r *accountingSessionRepo) List(_ context.Context) ([]repository.Session, error) {
	r.listCalls++
	out := make([]repository.Session, 0, len(r.sessions))
	for _, sess := range r.sessions {
		if sess.Ephemeral {
			continue
		}
		out = append(out, sess)
	}
	return out, nil
}

func (r *accountingSessionRepo) Update(_ context.Context, s repository.Session) error {
	r.sessions[s.ID] = s
	return nil
}

func (r *accountingSessionRepo) UpdateWorkflow(_ context.Context, id, workflowState string) error {
	s := r.sessions[id]
	s.WorkflowState = workflowState
	r.sessions[id] = s
	return nil
}

func (r *accountingSessionRepo) Delete(_ context.Context, id string) error {
	delete(r.sessions, id)
	return nil
}

func (r *accountingSessionRepo) DeleteEphemeral(_ context.Context) (int, error) { return 0, nil }

func (r *accountingSessionRepo) UpdateCost(_ context.Context, id string, inputTokens, outputTokens, lastInputTokens int, totalCost float64) error {
	s := r.sessions[id]
	s.TotalInputTokens = inputTokens
	s.TotalOutputTokens = outputTokens
	s.LastInputTokens = lastInputTokens
	s.TotalCost = totalCost
	r.sessions[id] = s
	return nil
}

func TestSessionAccountingBudgetStatusAggregatesCrossSessionSpend(t *testing.T) {
	repo := &accountingSessionRepo{
		sessions: map[string]repository.Session{
			"s1": {ID: "s1", TotalCost: 1.0},
			"s2": {ID: "s2", TotalCost: 2.0},
		},
	}
	accounting := newSessionAccounting(repo)
	accounting.GetOrCreateCost(context.Background(), "s1").Seed(0, 0, 0, 1.5)

	status := accounting.BudgetStatus(context.Background(), "s1", &config.SessionConfig{
		Budget:          2.0,
		BudgetWarn:      0.5,
		TotalBudget:     3.0,
		TotalBudgetWarn: 0.9,
	})

	if status.SessionCost != 1.5 {
		t.Fatalf("SessionCost = %.2f, want 1.5", status.SessionCost)
	}
	if status.TotalCost != 3.5 {
		t.Fatalf("TotalCost = %.2f, want 3.5", status.TotalCost)
	}
	if !status.SessionWarn {
		t.Fatal("SessionWarn = false, want true")
	}
	if status.SessionExceeded {
		t.Fatal("SessionExceeded = true, want false")
	}
	if !status.TotalWarn {
		t.Fatal("TotalWarn = false, want true")
	}
	if !status.TotalExceeded {
		t.Fatal("TotalExceeded = false, want true")
	}
}

func TestSessionAccountingSaveSessionCostPersistsSnapshot(t *testing.T) {
	repo := &accountingSessionRepo{
		sessions: map[string]repository.Session{
			"s1": {ID: "s1"},
		},
	}
	accounting := newSessionAccounting(repo)
	accounting.GetOrCreateCost(context.Background(), "s1").Seed(12, 7, 3, 0.42)

	if err := accounting.SaveSessionCost(context.Background(), "s1"); err != nil {
		t.Fatalf("SaveSessionCost() error = %v", err)
	}

	sess := repo.sessions["s1"]
	if sess.TotalInputTokens != 12 || sess.TotalOutputTokens != 7 || sess.LastInputTokens != 3 || sess.TotalCost != 0.42 {
		t.Fatalf("persisted session = %+v, want updated cost snapshot", sess)
	}
}

func TestSessionAccountingBudgetStatusCachesPersistedAggregate(t *testing.T) {
	repo := &accountingSessionRepo{
		sessions: map[string]repository.Session{
			"s1": {ID: "s1", TotalCost: 1.0},
			"s2": {ID: "s2", TotalCost: 2.0},
		},
	}
	accounting := newSessionAccounting(repo)

	cfg := &config.SessionConfig{TotalBudget: 10.0}
	first := accounting.BudgetStatus(context.Background(), "s1", cfg)
	second := accounting.BudgetStatus(context.Background(), "s1", cfg)

	if first.TotalCost != 3.0 || second.TotalCost != 3.0 {
		t.Fatalf("cached totals = %.2f / %.2f, want 3.0", first.TotalCost, second.TotalCost)
	}
	if repo.listCalls != 1 {
		t.Fatalf("List() calls = %d, want 1", repo.listCalls)
	}
}

func TestSessionAccountingSaveSessionCostUpdatesCachedAggregate(t *testing.T) {
	repo := &accountingSessionRepo{
		sessions: map[string]repository.Session{
			"s1": {ID: "s1", TotalCost: 1.0},
			"s2": {ID: "s2", TotalCost: 2.0},
		},
	}
	accounting := newSessionAccounting(repo)
	cfg := &config.SessionConfig{TotalBudget: 10.0}

	if got := accounting.BudgetStatus(context.Background(), "s1", cfg).TotalCost; got != 3.0 {
		t.Fatalf("initial TotalCost = %.2f, want 3.0", got)
	}
	if repo.listCalls != 1 {
		t.Fatalf("initial List() calls = %d, want 1", repo.listCalls)
	}

	accounting.GetOrCreateCost(context.Background(), "s1").Seed(0, 0, 0, 1.5)
	if err := accounting.SaveSessionCost(context.Background(), "s1"); err != nil {
		t.Fatalf("SaveSessionCost() error = %v", err)
	}

	if got := accounting.BudgetStatus(context.Background(), "s1", cfg).TotalCost; got != 3.5 {
		t.Fatalf("updated TotalCost = %.2f, want 3.5", got)
	}
	if repo.listCalls != 1 {
		t.Fatalf("List() calls after SaveSessionCost = %d, want 1", repo.listCalls)
	}
}

func TestSessionAccountingCleanupInvalidatesPersistedAggregate(t *testing.T) {
	repo := &accountingSessionRepo{
		sessions: map[string]repository.Session{
			"s1": {ID: "s1", TotalCost: 1.0},
			"s2": {ID: "s2", TotalCost: 2.0},
		},
	}
	accounting := newSessionAccounting(repo)
	cfg := &config.SessionConfig{TotalBudget: 10.0}

	if got := accounting.BudgetStatus(context.Background(), "s1", cfg).TotalCost; got != 3.0 {
		t.Fatalf("initial TotalCost = %.2f, want 3.0", got)
	}
	if repo.listCalls != 1 {
		t.Fatalf("initial List() calls = %d, want 1", repo.listCalls)
	}

	delete(repo.sessions, "s2")
	accounting.CleanupSession("s2")

	if got := accounting.BudgetStatus(context.Background(), "s1", cfg).TotalCost; got != 1.0 {
		t.Fatalf("reloaded TotalCost = %.2f, want 1.0", got)
	}
	if repo.listCalls != 2 {
		t.Fatalf("List() calls after invalidation = %d, want 2", repo.listCalls)
	}
}

func TestSessionAccountingBudgetStatusExcludesEphemeralSessionsFromTotal(t *testing.T) {
	repo := &accountingSessionRepo{
		sessions: map[string]repository.Session{
			"s1":  {ID: "s1", TotalCost: 1.0},
			"tmp": {ID: "tmp", Ephemeral: true, TotalCost: 5.0},
		},
	}
	accounting := newSessionAccounting(repo)
	accounting.GetOrCreateCost(context.Background(), "tmp").Seed(0, 0, 0, 6.0)

	status := accounting.BudgetStatus(context.Background(), "s1", &config.SessionConfig{
		TotalBudget: 10.0,
	})
	if status.TotalCost != 1.0 {
		t.Fatalf("TotalCost = %.2f, want 1.0", status.TotalCost)
	}
}
