package service

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/repository"
)

type BudgetStatus struct {
	SessionCost     float64
	TotalCost       float64
	SessionWarn     bool
	SessionExceeded bool
	TotalWarn       bool
	TotalExceeded   bool
}

type sessionCostState struct {
	cost           *SessionCost
	persistedCost  float64
	includeInTotal bool
}

type sessionAccountingService struct {
	sessions repository.SessionRepo

	costsMu sync.Mutex
	costs   map[string]*sessionCostState

	aggregateMu     sync.Mutex
	aggregateCost   float64
	aggregateLoaded bool
	aggregateDirty  bool

	tracesMu sync.Mutex
	traces   map[string]*SessionTraces

	traceRepo repository.TraceRepo
}

func newSessionAccounting(sessions repository.SessionRepo) *sessionAccountingService {
	return &sessionAccountingService{
		sessions: sessions,
		costs:    make(map[string]*sessionCostState),
		traces:   make(map[string]*SessionTraces),
	}
}

func (a *sessionAccountingService) SetTraceRepo(tr repository.TraceRepo) {
	a.traceRepo = tr
}

func (a *sessionAccountingService) CleanupSession(sessionID string) {
	var (
		hadState       bool
		includeInTotal bool
		persistedCost  float64
	)
	a.costsMu.Lock()
	if state, ok := a.costs[sessionID]; ok {
		hadState = true
		if state != nil {
			includeInTotal = state.includeInTotal
			persistedCost = state.persistedCost
		}
		delete(a.costs, sessionID)
	}
	a.costsMu.Unlock()

	if hadState && includeInTotal {
		a.aggregateMu.Lock()
		if a.aggregateLoaded && !a.aggregateDirty {
			a.aggregateCost -= persistedCost
			if a.aggregateCost < 0 {
				a.aggregateCost = 0
			}
		}
		a.aggregateMu.Unlock()
	} else if !hadState {
		a.invalidateAggregateTotal()
	}

	a.tracesMu.Lock()
	delete(a.traces, sessionID)
	a.tracesMu.Unlock()
}

func (a *sessionAccountingService) GetOrCreateCost(ctx context.Context, sessionID string) *SessionCost {
	a.costsMu.Lock()
	if state, ok := a.costs[sessionID]; ok && state != nil && state.cost != nil {
		a.costsMu.Unlock()
		return state.cost
	}
	a.costsMu.Unlock()

	var (
		seedIn, seedOut, seedLastIn int
		seedCost                    float64
		includeInTotal              bool
	)
	if sess, err := a.sessions.Get(ctx, sessionID); err == nil {
		seedIn, seedOut, seedCost = sess.TotalInputTokens, sess.TotalOutputTokens, sess.TotalCost
		seedLastIn = sess.LastInputTokens
		includeInTotal = !sess.Ephemeral
	}

	a.costsMu.Lock()
	defer a.costsMu.Unlock()
	if state, ok := a.costs[sessionID]; ok && state != nil && state.cost != nil {
		return state.cost
	}
	sc := NewSessionCost()
	if seedCost > 0 || seedLastIn > 0 {
		sc.Seed(seedIn, seedOut, seedLastIn, seedCost)
	}
	a.costs[sessionID] = &sessionCostState{
		cost:           sc,
		persistedCost:  seedCost,
		includeInTotal: includeInTotal,
	}
	return sc
}

func (a *sessionAccountingService) GetSessionCost(sessionID string) (CostSnapshot, bool) {
	a.costsMu.Lock()
	state, ok := a.costs[sessionID]
	a.costsMu.Unlock()
	if !ok || state == nil || state.cost == nil {
		return CostSnapshot{}, false
	}
	return state.cost.Snapshot(), true
}

func (a *sessionAccountingService) GetSessionTraces(sessionID string) [][]StepTrace {
	a.tracesMu.Lock()
	st, ok := a.traces[sessionID]
	a.tracesMu.Unlock()
	if ok {
		if turns := st.AllTurns(); len(turns) > 0 {
			return turns
		}
	}
	if a.traceRepo != nil {
		entries, err := a.traceRepo.ListBySession(context.Background(), sessionID)
		if err == nil && len(entries) > 0 {
			return deserializeTraceEntries(entries)
		}
	}
	return nil
}

func (a *sessionAccountingService) GetOrCreateTraces(sessionID string) *SessionTraces {
	a.tracesMu.Lock()
	defer a.tracesMu.Unlock()
	if st, ok := a.traces[sessionID]; ok {
		return st
	}
	st := NewSessionTraces()
	if a.traceRepo != nil {
		if entries, err := a.traceRepo.ListBySession(context.Background(), sessionID); err == nil {
			for _, e := range entries {
				var steps []StepTrace
				if json.Unmarshal([]byte(e.Data), &steps) == nil {
					st.CommitTurn(steps)
				}
			}
		}
		repo := a.traceRepo
		sid := sessionID
		st.onCommit = func(turnIndex int, steps []StepTrace) {
			data, err := json.Marshal(steps)
			if err != nil {
				log.Printf("trace: marshal failed: %v", err)
				return
			}
			if err := repo.Save(context.Background(), sid, turnIndex, string(data)); err != nil {
				log.Printf("trace: save failed: %v", err)
			}
		}
	}
	a.traces[sessionID] = st
	return st
}

func (a *sessionAccountingService) SaveSessionCost(ctx context.Context, sessionID string) error {
	if a == nil || a.sessions == nil {
		return nil
	}
	sc := a.GetOrCreateCost(ctx, sessionID)
	if sc == nil {
		return nil
	}
	snap := sc.Snapshot()
	if err := a.sessions.UpdateCost(ctx, sessionID, snap.InputTokens, snap.OutputTokens, sc.LastInputTokens(), snap.TotalCost); err != nil {
		return err
	}

	a.costsMu.Lock()
	state, ok := a.costs[sessionID]
	if ok && state != nil {
		previousCost := state.persistedCost
		state.persistedCost = snap.TotalCost
		includeInTotal := state.includeInTotal
		a.costsMu.Unlock()
		if includeInTotal {
			a.adjustAggregateTotal(snap.TotalCost - previousCost)
		}
		return nil
	}
	a.costsMu.Unlock()
	a.invalidateAggregateTotal()
	return nil
}

func (a *sessionAccountingService) BudgetStatus(ctx context.Context, sessionID string, cfg *config.SessionConfig) BudgetStatus {
	if a == nil || cfg == nil {
		return BudgetStatus{}
	}

	status := BudgetStatus{}
	if sessionID != "" {
		if sc := a.GetOrCreateCost(ctx, sessionID); sc != nil {
			snap := sc.Snapshot()
			status.SessionCost = snap.TotalCost
			if cfg.Budget > 0 {
				status.SessionExceeded = snap.TotalCost >= cfg.Budget
				if cfg.BudgetWarn > 0 {
					status.SessionWarn = snap.TotalCost >= cfg.Budget*cfg.BudgetWarn
				}
			}
		}
	}

	if cfg.TotalBudget <= 0 {
		return status
	}

	total, err := a.aggregateTotalCost(ctx)
	if err != nil {
		log.Printf("accounting: aggregate total cost failed: %v", err)
		return status
	}
	status.TotalCost = total
	status.TotalExceeded = total >= cfg.TotalBudget
	if cfg.TotalBudgetWarn > 0 {
		status.TotalWarn = total >= cfg.TotalBudget*cfg.TotalBudgetWarn
	}
	return status
}

func (a *sessionAccountingService) aggregateTotalCost(ctx context.Context) (float64, error) {
	total, err := a.persistedTotalCost(ctx)
	if err != nil {
		return 0, err
	}
	return total + a.liveTotalDelta(), nil
}

func (a *sessionAccountingService) persistedTotalCost(ctx context.Context) (float64, error) {
	a.aggregateMu.Lock()
	defer a.aggregateMu.Unlock()
	if a.aggregateLoaded && !a.aggregateDirty {
		return a.aggregateCost, nil
	}

	sessions, err := a.sessions.List(ctx)
	if err != nil {
		return 0, err
	}
	total := 0.0
	for _, sess := range sessions {
		total += sess.TotalCost
	}
	a.aggregateCost = total
	a.aggregateLoaded = true
	a.aggregateDirty = false
	return total, nil
}

func (a *sessionAccountingService) liveTotalDelta() float64 {
	a.costsMu.Lock()
	defer a.costsMu.Unlock()

	delta := 0.0
	for _, state := range a.costs {
		if state == nil || state.cost == nil || !state.includeInTotal {
			continue
		}
		delta += state.cost.Snapshot().TotalCost - state.persistedCost
	}
	return delta
}

func (a *sessionAccountingService) adjustAggregateTotal(delta float64) {
	if delta == 0 {
		return
	}
	a.aggregateMu.Lock()
	if a.aggregateLoaded && !a.aggregateDirty {
		a.aggregateCost += delta
		if a.aggregateCost < 0 {
			a.aggregateCost = 0
		}
	}
	a.aggregateMu.Unlock()
}

func (a *sessionAccountingService) invalidateAggregateTotal() {
	a.aggregateMu.Lock()
	if a.aggregateLoaded {
		a.aggregateDirty = true
	}
	a.aggregateMu.Unlock()
}

func deserializeTraceEntries(entries []repository.TraceEntry) [][]StepTrace {
	turns := make([][]StepTrace, 0, len(entries))
	for _, e := range entries {
		var steps []StepTrace
		if json.Unmarshal([]byte(e.Data), &steps) == nil {
			turns = append(turns, steps)
		}
	}
	return turns
}
