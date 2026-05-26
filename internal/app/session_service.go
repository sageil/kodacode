package app

import (
	"errors"
	"sync"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/observability"
	"github.com/sageil/kodacode/internal/permissionpolicy"
)

const sessionTurnID = "_session"

const sessionStateSnapshotIntervalEvents int64 = 64

var (
	ErrEventStoreRequired       = errors.New("event store is required")
	ErrSessionIDRequired        = errors.New("session_id is required")
	ErrTurnIDRequired           = errors.New("turn_id is required")
	ErrWorkspaceRootRequired    = errors.New("workspace_root is required")
	ErrPermissionRequestMissing = errors.New("permission request not found")
	ErrQuestionAnswerInvalid    = errors.New("question answer is not a valid option")
	ErrQuestionRequestMissing   = errors.New("question request not found")
	ErrSessionNotConfigured     = errors.New("session is not configured")
)

type SessionService struct {
	store      events.ReplayStore
	blobs      ToolResultBlobStore
	logger     *observability.Logger
	registryMu sync.Mutex
	policyMu   sync.RWMutex
	policy     permissionpolicy.Config

	sessions         map[string]*sessionRuntime
	budgetMu         sync.Mutex
	budgetTotalsCost float64
	budgetTotalsMiss int
	budgetTotalsWarm bool
}

func NewSessionService(store events.ReplayStore) (*SessionService, error) {
	return NewSessionServiceWithBlobs(store, nil)
}

func NewSessionServiceWithBlobs(store events.ReplayStore, blobs ToolResultBlobStore) (*SessionService, error) {
	if store == nil {
		return nil, ErrEventStoreRequired
	}
	return &SessionService{
		store:    store,
		blobs:    blobs,
		sessions: make(map[string]*sessionRuntime),
	}, nil
}

func (s *SessionService) SetLogger(logger *observability.Logger) {
	if s == nil {
		return
	}
	s.logger = logger
}

func (s *SessionService) SetPermissionPolicy(policy permissionpolicy.Config) error {
	if s == nil {
		return nil
	}
	if err := policy.Validate(); err != nil {
		return err
	}
	s.policyMu.Lock()
	defer s.policyMu.Unlock()
	s.policy = clonePermissionPolicyConfig(policy)
	return nil
}
