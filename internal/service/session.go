package service

import (
	"context"
	"fmt"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/repository"
	"github.com/sageil/kodacode/v1/internal/snapshot"
	"github.com/sageil/kodacode/v1/internal/tool"
)

type agentLookup interface {
	Get(id string) (config.AgentConfig, error)
}

type SessionService struct {
	store     *sessionStoreService
	state     *sessionStateService
	runtime   *sessionRuntime
	subagents *sessionSubagentService
}

func NewSessionService(
	sessions repository.SessionRepo,
	messages repository.MessageRepo,
	providers *provider.Registry,
	cfg *config.Config,
	_ *tool.Registry,
	agents agentLookup,
	projectDir string,
	chain *pipeline.Chain,
) *SessionService {
	return &SessionService{
		store:     newSessionStoreService(sessions, messages, projectDir, cfg),
		state:     newSessionStateService(sessions),
		runtime:   newSessionRuntime(providers, cfg, agents, projectDir, chain),
		subagents: newSessionSubagentService(cfg),
	}
}

// SetTaskStore attaches a TaskStore for session cleanup.
func (s *SessionService) SetTaskStore(ts interface{ CleanupSession(string) }) {
	if s == nil || s.store == nil {
		return
	}
	s.store.SetTaskStore(ts)
}

func (s *SessionService) SetTraceRepo(tr repository.TraceRepo) {
	if s == nil || s.state == nil {
		return
	}
	s.state.SetTraceRepo(tr)
}

func (s *SessionService) SetAttachmentRepo(ar repository.AttachmentRepo) {
	if s == nil || s.store == nil {
		return
	}
	s.store.SetAttachmentRepo(ar)
}

type CreateOption func(*repository.Session)

func WithEphemeral() CreateOption {
	return func(s *repository.Session) { s.Ephemeral = true }
}

func (s *SessionService) Create(ctx context.Context, agentID, modelID string, opts ...CreateOption) (repository.Session, error) {
	if s == nil || s.store == nil {
		return repository.Session{}, fmt.Errorf("session service create: not initialized")
	}
	defaultAgent := ""
	if s.runtime != nil {
		defaultAgent = s.runtime.DefaultAgent()
	}
	return s.store.Create(ctx, defaultAgent, agentID, modelID, opts...)
}

func (s *SessionService) Get(ctx context.Context, id string) (repository.Session, error) {
	if s == nil || s.store == nil {
		return repository.Session{}, fmt.Errorf("session service get: not initialized")
	}
	return s.store.Get(ctx, id)
}

func (s *SessionService) UpdateSession(ctx context.Context, sess repository.Session) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("session service update: not initialized")
	}
	return s.store.UpdateSession(ctx, sess)
}

func (s *SessionService) List(ctx context.Context) ([]repository.Session, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("session service list: not initialized")
	}
	return s.store.List(ctx)
}

func (s *SessionService) Delete(ctx context.Context, id string) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("session service delete: not initialized")
	}
	mutation, err := s.acquireSessionMutation(id)
	if err != nil {
		return err
	}
	if mutation != nil {
		defer mutation.Release()
	}
	if err := s.store.Delete(ctx, id); err != nil {
		return err
	}
	s.cleanupSessionState(id)
	return nil
}

func (s *SessionService) cleanupSessionState(id string) {
	if s == nil || id == "" {
		return
	}
	if s.state != nil {
		s.state.CleanupSession(id)
	}
	if s.store != nil {
		s.store.CleanupSession(id)
	}
}

func (s *SessionService) ListMessages(ctx context.Context, sessionID string) ([]repository.Message, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("session service list messages: not initialized")
	}
	return s.store.ListMessages(ctx, sessionID)
}

func (s *SessionService) Branch(ctx context.Context, sessionID, messageID string) (repository.Session, error) {
	if s == nil || s.store == nil {
		return repository.Session{}, fmt.Errorf("session service branch: not initialized")
	}
	mutation, err := s.acquireSessionMutation(sessionID)
	if err != nil {
		return repository.Session{}, err
	}
	if mutation != nil {
		defer mutation.Release()
	}
	return s.store.Branch(ctx, sessionID, messageID)
}

func (s *SessionService) acquireSessionMutation(sessionID string) (*SessionMutation, error) {
	if s == nil || s.state == nil {
		return nil, nil
	}
	return s.state.AcquireSessionMutation(sessionID)
}

func (s *SessionService) Subscribe(sessionID string) (<-chan SSEEvent, func()) {
	if s == nil || s.state == nil {
		ch := make(chan SSEEvent)
		close(ch)
		return ch, func() {}
	}
	return s.state.Subscribe(sessionID)
}

func (s *SessionService) SetChain(chain *pipeline.Chain) {
	if s == nil || s.runtime == nil {
		return
	}
	s.runtime.SetChain(chain)
}

func (s *SessionService) SetSnapshotService(svc *snapshot.Service) {
	if s == nil || s.store == nil {
		return
	}
	s.store.SetSnapshotService(svc)
}

func (s *SessionService) SnapshotService() *snapshot.Service {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.SnapshotService()
}

func (s *SessionService) SetSettings(settings repository.SettingsRepo) {
	if s == nil || s.store == nil {
		return
	}
	s.store.SetSettings(settings)
}

func (s *SessionService) Publisher() func(string, SSEEvent) {
	if s == nil || s.state == nil {
		return func(string, SSEEvent) {}
	}
	return s.state.Publisher()
}

type AskUserFunc func(ctx context.Context, sessionID string) func(question string, options []string, multiple bool, purpose string) (string, error)

// publish sends ev to all subscribers of sessionID.
// Push is non-blocking. Under sustained backpressure, transient stream deltas
// may be dropped and surfaced as an explicit overflow event.
func (s *SessionService) publish(sessionID string, ev SSEEvent) {
	if s == nil || s.state == nil {
		return
	}
	s.state.Publish(sessionID, ev)
}

func (s *SessionService) GetOrCreateCost(ctx context.Context, sessionID string) *SessionCost {
	if s == nil || s.state == nil {
		return nil
	}
	return s.state.GetOrCreateCost(ctx, sessionID)
}

func (s *SessionService) GetSessionCost(sessionID string) (CostSnapshot, bool) {
	if s == nil || s.state == nil {
		return CostSnapshot{}, false
	}
	return s.state.GetSessionCost(sessionID)
}

func (s *SessionService) GetSessionTraces(sessionID string) [][]StepTrace {
	if s == nil || s.state == nil {
		return nil
	}
	return s.state.GetSessionTraces(sessionID)
}

func (s *SessionService) GetOrCreateTraces(sessionID string) *SessionTraces {
	if s == nil || s.state == nil {
		return nil
	}
	return s.state.GetOrCreateTraces(sessionID)
}

func (s *SessionService) GetBudgetStatus(ctx context.Context, sessionID string) BudgetStatus {
	if s == nil || s.state == nil || s.runtime == nil || s.runtime.cfg == nil {
		return BudgetStatus{}
	}
	return s.state.BudgetStatus(ctx, sessionID, &s.runtime.cfg.Session)
}
