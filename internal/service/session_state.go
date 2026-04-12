package service

import (
	"context"
	"fmt"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/repository"
)

type sessionStateService struct {
	events      *sessionEventHub
	permissions *sessionPermissionBroker
	accounting  *sessionAccountingService
	turns       *sessionTurnBroker
	background  *sessionBackgroundBroker
}

func newSessionStateService(sessions repository.SessionRepo) *sessionStateService {
	var turnOps repository.TurnOperationRepo
	if repo, ok := sessions.(repository.TurnOperationRepo); ok {
		turnOps = repo
	}
	return &sessionStateService{
		events:      newSessionEventHub(),
		permissions: newSessionPermissionBroker(),
		accounting:  newSessionAccounting(sessions),
		turns:       newSessionTurnBroker(turnOps),
		background:  newSessionBackgroundBroker(),
	}
}

func (s *sessionStateService) CleanupSession(sessionID string) {
	if s == nil || sessionID == "" {
		return
	}
	s.events.CleanupSession(sessionID)
	s.permissions.CleanupSession(sessionID)
	s.accounting.CleanupSession(sessionID)
	s.turns.CleanupSession(sessionID)
	s.background.CleanupSession(sessionID)
}

func (s *sessionStateService) Subscribe(sessionID string) (<-chan SSEEvent, func()) {
	if s == nil {
		ch := make(chan SSEEvent)
		close(ch)
		return ch, func() {}
	}
	return s.events.Subscribe(sessionID)
}

func (s *sessionStateService) Publish(sessionID string, ev SSEEvent) {
	if s == nil {
		return
	}
	s.events.Publish(sessionID, ev)
}

func (s *sessionStateService) Publisher() func(string, SSEEvent) {
	if s == nil {
		return func(string, SSEEvent) {}
	}
	return s.Publish
}

func (s *sessionStateService) SetTraceRepo(tr repository.TraceRepo) {
	if s == nil {
		return
	}
	s.accounting.SetTraceRepo(tr)
}

func (s *sessionStateService) GetOrCreateCost(ctx context.Context, sessionID string) *SessionCost {
	if s == nil {
		return nil
	}
	return s.accounting.GetOrCreateCost(ctx, sessionID)
}

func (s *sessionStateService) GetSessionCost(sessionID string) (CostSnapshot, bool) {
	if s == nil {
		return CostSnapshot{}, false
	}
	return s.accounting.GetSessionCost(sessionID)
}

func (s *sessionStateService) GetSessionTraces(sessionID string) [][]StepTrace {
	if s == nil {
		return nil
	}
	return s.accounting.GetSessionTraces(sessionID)
}

func (s *sessionStateService) GetOrCreateTraces(sessionID string) *SessionTraces {
	if s == nil {
		return nil
	}
	return s.accounting.GetOrCreateTraces(sessionID)
}

func (s *sessionStateService) BudgetStatus(ctx context.Context, sessionID string, cfg *config.SessionConfig) BudgetStatus {
	if s == nil {
		return BudgetStatus{}
	}
	return s.accounting.BudgetStatus(ctx, sessionID, cfg)
}

func (s *sessionStateService) ReserveSend(sessionID string) (*SendReservation, error) {
	if s == nil || s.turns == nil {
		return nil, nil
	}
	return s.turns.Reserve(sessionID)
}

func (s *sessionStateService) AcquireSessionMutation(sessionID string) (*SessionMutation, error) {
	if s == nil || s.turns == nil {
		return nil, nil
	}
	return s.turns.AcquireMutation(sessionID)
}

func (s *sessionStateService) CancelTurn(sessionID string) error {
	if s == nil || s.turns == nil {
		return fmt.Errorf("%w: %s", ErrNoActiveTurn, sessionID)
	}
	return s.turns.Cancel(sessionID)
}

func (s *sessionStateService) TurnStatus(sessionID string) TurnStatus {
	if s == nil || s.turns == nil {
		return TurnStatus{SessionID: sessionID, State: TurnStateIdle}
	}
	return s.turns.Status(sessionID)
}

func (s *sessionStateService) TurnStatusByOperation(sessionID, operationID string) (TurnStatus, error) {
	if s == nil || s.turns == nil {
		if operationID == "" {
			return TurnStatus{SessionID: sessionID, State: TurnStateIdle}, nil
		}
		return TurnStatus{}, fmt.Errorf("%w: %s/%s", ErrTurnOperationNotFound, sessionID, operationID)
	}
	return s.turns.StatusByOperation(sessionID, operationID)
}
