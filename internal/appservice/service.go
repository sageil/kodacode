package appservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/sageil/kodacode/v1/internal/agent"
	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/message"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/repository"
	"github.com/sageil/kodacode/v1/internal/sandbox"
	"github.com/sageil/kodacode/v1/internal/service"
	"github.com/sageil/kodacode/v1/internal/snapshot"
)

var (
	ErrInvalidQuestionResponse = errors.New("invalid question response")
	ErrSnapshotsDisabled       = errors.New("snapshots not enabled")
	ErrSessionQueueFull        = errors.New("session already has a queued turn")
)

type queuedTurn struct {
	operationID string
	content     string
	attachments []service.FileAttachment
	agentID     string
	variant     string
	queuedAt    time.Time
}

type SettingsService interface {
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}

type SessionService interface {
	Create(ctx context.Context, agentID, modelID string, opts ...service.CreateOption) (repository.Session, error)
	Get(ctx context.Context, id string) (repository.Session, error)
	List(ctx context.Context) ([]repository.Session, error)
	UpdateSession(ctx context.Context, sess repository.Session) error
	Delete(ctx context.Context, id string) error
	Branch(ctx context.Context, sessionID, messageID string) (repository.Session, error)
	CancelTurn(ctx context.Context, sessionID string) error
	TurnStatus(ctx context.Context, sessionID string) (service.TurnStatus, error)
	TurnStatusByOperation(ctx context.Context, sessionID, operationID string) (service.TurnStatus, error)
	Send(ctx context.Context, sessionID, prompt string, attachments []service.FileAttachment, origin sandbox.Origin, variant ...string) error
	Subscribe(sessionID string) (<-chan service.SSEEvent, func())
	Answer(ctx context.Context, sessionID, questionID string, response service.AnswerResponse) error
	ListMessages(ctx context.Context, sessionID string) ([]repository.Message, error)
	SpawnSubagent(ctx context.Context, parentSessionID, agentID, task string, onProgress service.ProgressFunc) (string, error)
	GetSessionTraces(sessionID string) [][]service.StepTrace
}

type AgentService interface {
	List() []agent.Agent
}

type ProviderRegistry interface {
	ListModels(ctx context.Context) []provider.ProviderModels
	RefreshModels(ctx context.Context)
}

type MCPServerInfo struct {
	Name    string
	Active  bool
	Enabled bool
}

type AttachmentInput struct {
	Path     string
	MimeType string
}

type MessagePartView struct {
	Type    string
	Content string
}

type MessageView struct {
	ID        string
	SessionID string
	Role      string
	Content   string
	Parts     []MessagePartView
}

type ConfigView struct {
	DefaultAgent     string
	ToolCount        int
	MCPServers       []MCPServerInfo
	ErrorDisplayTime int
	TraceEnabled     bool
}

type Config struct {
	Sessions        SessionService
	Agents          AgentService
	Settings        SettingsService
	Registry        ProviderRegistry
	Config          *config.Config
	SnapshotSvc     *snapshot.Service
	ProjectDir      string
	ToolCount       int
	BackgroundCtx   context.Context
	MCPStatus       func() []MCPServerInfo
	RefreshMCPTools func(context.Context) (int, error)
	SyncProviders   func(context.Context) ([]string, error)
	Publish         func(string, service.SSEEvent)
}

type Service struct {
	sessions        SessionService
	agents          AgentService
	settings        SettingsService
	registry        ProviderRegistry
	cfg             *config.Config
	snapshotSvc     *snapshot.Service
	projectDir      string
	toolCount       int
	backgroundCtx   context.Context
	mcpStatus       func() []MCPServerInfo
	refreshMCPTools func(context.Context) (int, error)
	syncProviders   func(context.Context) ([]string, error)
	publish         func(string, service.SSEEvent)
	queueMu         sync.Mutex
	queuedTurns     map[string]queuedTurn
}

func New(cfg Config) *Service {
	return &Service{
		sessions:        cfg.Sessions,
		agents:          cfg.Agents,
		settings:        cfg.Settings,
		registry:        cfg.Registry,
		cfg:             cfg.Config,
		snapshotSvc:     cfg.SnapshotSvc,
		projectDir:      cfg.ProjectDir,
		toolCount:       cfg.ToolCount,
		backgroundCtx:   cfg.BackgroundCtx,
		mcpStatus:       cfg.MCPStatus,
		refreshMCPTools: cfg.RefreshMCPTools,
		syncProviders:   cfg.SyncProviders,
		publish:         cfg.Publish,
		queuedTurns:     make(map[string]queuedTurn),
	}
}

func cloneFileAttachments(in []service.FileAttachment) []service.FileAttachment {
	if len(in) == 0 {
		return nil
	}
	out := make([]service.FileAttachment, len(in))
	for i, att := range in {
		out[i] = att
		if len(att.Data) > 0 {
			out[i].Data = append([]byte(nil), att.Data...)
		}
	}
	return out
}

func queuedTurnStatus(sessionID string, qt queuedTurn) service.TurnStatus {
	return service.TurnStatus{
		SessionID:         sessionID,
		OperationID:       qt.operationID,
		State:             service.TurnStateQueued,
		Active:            false,
		QueueDepth:        1,
		QueuedOperationID: qt.operationID,
		UpdatedAt:         qt.queuedAt,
	}
}

func (s *Service) queuedTurnLocked(sessionID string) (queuedTurn, bool) {
	qt, ok := s.queuedTurns[sessionID]
	return qt, ok
}

func (s *Service) queuedTurn(sessionID string) (queuedTurn, bool) {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	return s.queuedTurnLocked(sessionID)
}

func (s *Service) augmentTurnStatus(sessionID string, status service.TurnStatus) service.TurnStatus {
	s.queueMu.Lock()
	qt, ok := s.queuedTurnLocked(sessionID)
	s.queueMu.Unlock()
	if !ok {
		return status
	}
	if status.Active {
		status.QueueDepth = 1
		status.QueuedOperationID = qt.operationID
		return status
	}
	return queuedTurnStatus(sessionID, qt)
}

func (s *Service) publishQueueState(sessionID string, qt *queuedTurn) {
	if s.publish == nil {
		return
	}
	data := service.SSETurnQueueData{}
	if qt != nil {
		data.Count = 1
		data.OperationID = qt.operationID
	}
	s.publish(sessionID, service.SSEEvent{Type: "turn_queue", Data: data})
}

func (s *Service) enqueueTurn(sessionID, content string, attachments []service.FileAttachment, agentID, variant string) (service.TurnStatus, error) {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	if _, exists := s.queuedTurns[sessionID]; exists {
		return service.TurnStatus{}, ErrSessionQueueFull
	}
	qt := queuedTurn{
		operationID: ulid.Make().String(),
		content:     content,
		attachments: cloneFileAttachments(attachments),
		agentID:     agentID,
		variant:     variant,
		queuedAt:    time.Now().UTC(),
	}
	s.queuedTurns[sessionID] = qt
	s.publishQueueState(sessionID, &qt)
	return queuedTurnStatus(sessionID, qt), nil
}

func (s *Service) clearQueuedTurn(sessionID string) {
	s.queueMu.Lock()
	_, existed := s.queuedTurns[sessionID]
	delete(s.queuedTurns, sessionID)
	s.queueMu.Unlock()
	if existed {
		s.publishQueueState(sessionID, nil)
	}
}

func (s *Service) CreateSession(ctx context.Context, agentID, modelID string) (repository.Session, error) {
	return s.sessions.Create(ctx, agentID, modelID)
}

func (s *Service) UpdateSession(ctx context.Context, id, agentID, modelID string) (repository.Session, error) {
	sess, err := s.sessions.Get(ctx, id)
	if err != nil {
		return repository.Session{}, err
	}
	if agentID != "" {
		sess.AgentID = agentID
	}
	if modelID != "" {
		sess.ModelID = modelID
	}
	if err := s.sessions.UpdateSession(ctx, sess); err != nil {
		return repository.Session{}, err
	}
	return s.sessions.Get(ctx, id)
}

func (s *Service) ListModels(ctx context.Context) []provider.ProviderModels {
	if s.registry == nil {
		return nil
	}
	return s.registry.ListModels(ctx)
}

func (s *Service) RefreshModels(ctx context.Context) []provider.ProviderModels {
	if s.registry == nil {
		return nil
	}
	s.registry.RefreshModels(ctx)
	return s.registry.ListModels(ctx)
}

func (s *Service) RefreshMCPTools(ctx context.Context) (int, error) {
	if s.refreshMCPTools == nil {
		return 0, nil
	}
	return s.refreshMCPTools(ctx)
}

func (s *Service) SyncProviders(ctx context.Context) ([]string, error) {
	if s.syncProviders == nil {
		return nil, nil
	}
	return s.syncProviders(ctx)
}

func (s *Service) HasActiveTurns(ctx context.Context) (bool, error) {
	if s.sessions == nil {
		return false, nil
	}
	sessions, err := s.sessions.List(ctx)
	if err != nil {
		return false, err
	}
	for _, sess := range sessions {
		status, err := s.sessions.TurnStatus(ctx, sess.ID)
		if err != nil {
			return false, err
		}
		if status.Active {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) UpdateSessionModel(ctx context.Context, id, modelID string) error {
	sess, err := s.sessions.Get(ctx, id)
	if err != nil {
		return err
	}
	sess.ModelID = modelID
	return s.sessions.UpdateSession(ctx, sess)
}

func (s *Service) UpdateSessionAgent(ctx context.Context, id, agentID string) error {
	sess, err := s.sessions.Get(ctx, id)
	if err != nil {
		return err
	}
	sess.AgentID = agentID
	return s.sessions.UpdateSession(ctx, sess)
}

func (s *Service) ListSessions(ctx context.Context) ([]repository.Session, error) {
	return s.sessions.List(ctx)
}

func (s *Service) GetSession(ctx context.Context, id string) (repository.Session, error) {
	return s.sessions.Get(ctx, id)
}

func (s *Service) ListMessages(ctx context.Context, sessionID string) ([]MessageView, error) {
	if _, err := s.sessions.Get(ctx, sessionID); err != nil {
		return nil, err
	}
	msgs, err := s.sessions.ListMessages(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]MessageView, 0, len(msgs))
	for _, m := range msgs {
		var text string
		var parts []MessagePartView
		for _, p := range m.Parts {
			if p.Synthetic {
				continue
			}
			switch p.Type {
			case "text":
				tc, err := message.UnmarshalContent("text", p.Content)
				if err == nil {
					if t, ok := tc.(message.TextContent); ok {
						text += t.Text
					}
				} else {
					text += p.Content
				}
			case "tool_call", "tool_result", "reasoning", "file", "background_event":
				parts = append(parts, MessagePartView{Type: p.Type, Content: p.Content})
			}
		}
		if text == "" && len(parts) == 0 {
			continue
		}
		out = append(out, MessageView{
			ID:        m.ID,
			SessionID: m.SessionID,
			Role:      m.Role,
			Content:   text,
			Parts:     parts,
		})
	}
	return out, nil
}

func (s *Service) DeleteSession(ctx context.Context, id string) error {
	s.clearQueuedTurn(id)
	return s.sessions.Delete(ctx, id)
}

func (s *Service) BranchSession(ctx context.Context, sessionID, messageID string) (repository.Session, error) {
	return s.sessions.Branch(ctx, sessionID, messageID)
}

func (s *Service) SendMessage(ctx context.Context, sessionID, content string, attachments []AttachmentInput, agentID, variant string) error {
	_, err := s.SendMessageOperation(ctx, sessionID, content, attachments, agentID, variant)
	return err
}

func (s *Service) startPreparedSend(ctx context.Context, sess repository.Session, sessionID, content string, files []service.FileAttachment, agentID, variant string, allowQueue bool) (service.TurnStatus, error) {
	bg := s.asyncContext()
	if err := bg.Err(); err != nil {
		return service.TurnStatus{}, err
	}

	sendCtx := bg
	release := func() {}
	status := service.TurnStatus{SessionID: sessionID, State: service.TurnStateRunning, Active: true}
	prepared := false
	var reservation *service.SendReservation
	if preparer, ok := s.sessions.(interface {
		PrepareSend(context.Context, string, []service.FileAttachment) (*service.SendReservation, error)
	}); ok {
		var err error
		reservation, err = preparer.PrepareSend(ctx, sessionID, files)
		if err != nil {
			if allowQueue && errors.Is(err, service.ErrSessionBusy) {
				return s.enqueueTurn(sessionID, content, files, agentID, variant)
			}
			return service.TurnStatus{}, err
		}
		if reservation != nil {
			sendCtx = reservation.Context(bg)
			prepared = true
			status = reservation.Status()
			var err error
			sendCtx, release, err = bindReservationCancel(sendCtx, reservation)
			if err != nil {
				return service.TurnStatus{}, err
			}
		}
	}
	if !prepared {
		if reserver, ok := s.sessions.(interface {
			ReserveSend(string) (*service.SendReservation, error)
		}); ok {
			var err error
			reservation, err = reserver.ReserveSend(sessionID)
			if err != nil {
				if allowQueue && errors.Is(err, service.ErrSessionBusy) {
					return s.enqueueTurn(sessionID, content, files, agentID, variant)
				}
				return service.TurnStatus{}, err
			}
			if reservation != nil {
				sendCtx = reservation.Context(bg)
				status = reservation.Status()
				var err error
				sendCtx, release, err = bindReservationCancel(sendCtx, reservation)
				if err != nil {
					return service.TurnStatus{}, err
				}
			}
		}
	}
	if agentID != "" && sess.AgentID != agentID {
		sess.AgentID = agentID
		if err := s.sessions.UpdateSession(ctx, sess); err != nil {
			if reservation != nil {
				reservation.Complete(service.TurnStateFailed, err)
				release()
			}
			return service.TurnStatus{}, err
		}
	}

	s.launchPreparedSend(sendCtx, release, sessionID, content, files, variant)
	return status, nil
}

func (s *Service) dispatchQueuedTurn(sessionID string, qt queuedTurn) {
	sess, err := s.sessions.Get(context.Background(), sessionID)
	if err != nil {
		s.clearQueuedTurn(sessionID)
		s.publishAsyncError(sessionID, err)
		return
	}
	if _, err := s.startPreparedSend(context.Background(), sess, sessionID, qt.content, cloneFileAttachments(qt.attachments), qt.agentID, qt.variant, false); err != nil {
		if errors.Is(err, service.ErrSessionBusy) {
			return
		}
		s.clearQueuedTurn(sessionID)
		s.publishAsyncError(sessionID, err)
		return
	}
	s.clearQueuedTurn(sessionID)
}

func (s *Service) maybeDispatchQueuedTurn(sessionID string) {
	qt, ok := s.queuedTurn(sessionID)
	if !ok {
		return
	}
	s.dispatchQueuedTurn(sessionID, qt)
}

func (s *Service) launchPreparedSend(sendCtx context.Context, release func(), sessionID, content string, files []service.FileAttachment, variant string) {
	go func() {
		defer release()
		if err := s.sessions.Send(sendCtx, sessionID, content, files, sandbox.OriginAPI, variant); err != nil {
			log.Printf("appservice: async send failed for session %s: %v", sessionID, err)
			if !service.ErrorAlreadyPublished(err) {
				s.publishAsyncError(sessionID, err)
			}
		}
		s.maybeDispatchQueuedTurn(sessionID)
	}()
}

func (s *Service) SendMessageOperation(ctx context.Context, sessionID, content string, attachments []AttachmentInput, agentID, variant string) (service.TurnStatus, error) {
	sess, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return service.TurnStatus{}, err
	}

	files, err := s.loadAttachments(attachments)
	if err != nil {
		return service.TurnStatus{}, err
	}

	return s.startPreparedSend(ctx, sess, sessionID, content, files, agentID, variant, true)
}

func (s *Service) CancelTurn(ctx context.Context, sessionID string) error {
	_, err := s.CancelTurnOperation(ctx, sessionID)
	return err
}

func (s *Service) CancelTurnOperation(ctx context.Context, sessionID string) (service.TurnStatus, error) {
	if _, err := s.sessions.Get(ctx, sessionID); err != nil {
		return service.TurnStatus{}, err
	}
	if err := s.sessions.CancelTurn(ctx, sessionID); err != nil {
		return service.TurnStatus{}, err
	}
	return s.sessions.TurnStatus(ctx, sessionID)
}

func (s *Service) TurnStatus(ctx context.Context, sessionID string) (service.TurnStatus, error) {
	if _, err := s.sessions.Get(ctx, sessionID); err != nil {
		return service.TurnStatus{}, err
	}
	status, err := s.sessions.TurnStatus(ctx, sessionID)
	if err != nil {
		return service.TurnStatus{}, err
	}
	return s.augmentTurnStatus(sessionID, status), nil
}

func (s *Service) TurnStatusByOperation(ctx context.Context, sessionID, operationID string) (service.TurnStatus, error) {
	if _, err := s.sessions.Get(ctx, sessionID); err != nil {
		return service.TurnStatus{}, err
	}
	if qt, ok := s.queuedTurn(sessionID); ok && qt.operationID == operationID {
		return queuedTurnStatus(sessionID, qt), nil
	}
	status, err := s.sessions.TurnStatusByOperation(ctx, sessionID, operationID)
	if err != nil {
		return service.TurnStatus{}, err
	}
	return s.augmentTurnStatus(sessionID, status), nil
}

func (s *Service) SpawnSubagent(ctx context.Context, sessionID, agentID, task string) error {
	if _, err := s.sessions.Get(ctx, sessionID); err != nil {
		return err
	}
	if agentID == "" {
		return fmt.Errorf("agent_id is required")
	}
	if task == "" {
		return fmt.Errorf("task is required")
	}

	bg := s.asyncContext()
	if err := bg.Err(); err != nil {
		return err
	}
	go func() {
		callID := fmt.Sprintf("slash-%s-%d", agentID, time.Now().UnixNano())

		// Publish tool_start so the TUI shows a spinning subagent block
		// with live activity updates while the subagent runs.
		if s.publish != nil {
			s.publish(sessionID, service.SSEEvent{
				Type: "tool_start",
				Data: service.SSEToolStartData{
					Tool:   "subagent",
					Input:  fmt.Sprintf(`{"agent_id":%q,"task":%q}`, agentID, truncateForDisplay(task, 100)),
					CallID: callID,
				},
			})
		}

		result, err := s.sessions.SpawnSubagent(bg, sessionID, agentID, task, nil)
		if err != nil {
			log.Printf("appservice: async subagent failed for session %s: %v", sessionID, err)
			if s.publish != nil {
				errMsg := err.Error()
				s.publish(sessionID, service.SSEEvent{
					Type: "tool_end",
					Data: service.SSEToolEndData{Tool: "subagent", Output: "", Error: &errMsg, CallID: callID},
				})
				s.publish(sessionID, service.SSEEvent{
					Type: "done",
					Data: service.SSEDoneData{},
				})
			}
			return
		}
		if s.publish != nil {
			s.publish(sessionID, service.SSEEvent{
				Type: "tool_end",
				Data: service.SSEToolEndData{Tool: "subagent", Output: result, CallID: callID},
			})
			s.publish(sessionID, service.SSEEvent{
				Type: "done",
				Data: service.SSEDoneData{},
			})
		}
	}()
	return nil
}

func (s *Service) ListAgents() []agent.Agent {
	if s.agents == nil {
		return nil
	}
	return s.agents.List()
}

func (s *Service) AnswerQuestion(ctx context.Context, sessionID, questionID, response string) error {
	if !strings.HasPrefix(questionID, "uq-") {
		parsed := service.AnswerResponse(response)
		if parsed != service.AnswerOnce && parsed != service.AnswerAlways && parsed != service.AnswerReject {
			return fmt.Errorf("%w: must be %q, %q, or %q", ErrInvalidQuestionResponse, service.AnswerOnce, service.AnswerAlways, service.AnswerReject)
		}
	}
	return s.sessions.Answer(ctx, sessionID, questionID, service.AnswerResponse(response))
}

func (s *Service) GetConfig(_ context.Context) ConfigView {
	if s.cfg == nil {
		return ConfigView{}
	}
	errorDisplayTime := s.cfg.TUI.ErrorDisplayTime
	if errorDisplayTime <= 0 {
		errorDisplayTime = 3
	}
	var mcpServers []MCPServerInfo
	if s.mcpStatus != nil {
		mcpServers = s.mcpStatus()
	}
	return ConfigView{
		DefaultAgent:     s.cfg.DefaultAgent,
		ToolCount:        s.toolCount,
		MCPServers:       mcpServers,
		ErrorDisplayTime: errorDisplayTime,
		TraceEnabled:     config.Bool(s.cfg.Session.Trace),
	}
}

func (s *Service) GetSetting(ctx context.Context, key string) (string, error) {
	if s.settings == nil {
		return "", repository.ErrNotFound
	}
	return s.settings.GetSetting(ctx, key)
}

func (s *Service) SetSetting(ctx context.Context, key, value string) error {
	if s.settings == nil {
		return fmt.Errorf("settings not configured")
	}
	return s.settings.SetSetting(ctx, key, value)
}

func (s *Service) ListSnapshots(sessionID string) ([]snapshot.Snapshot, error) {
	if s.snapshotSvc == nil {
		return []snapshot.Snapshot{}, nil
	}
	return s.snapshotSvc.List(sessionID)
}

func (s *Service) RestoreSnapshot(sessionID string, turn int) error {
	if s.snapshotSvc == nil {
		return ErrSnapshotsDisabled
	}
	return s.snapshotSvc.Restore(sessionID, turn)
}

func (s *Service) Subscribe(ctx context.Context, sessionID string) (<-chan service.SSEEvent, func(), error) {
	if _, err := s.sessions.Get(ctx, sessionID); err != nil {
		return nil, nil, err
	}
	sub, cancel := s.sessions.Subscribe(sessionID)
	return sub, cancel, nil
}

func (s *Service) Traces(sessionID string) [][]service.StepTrace {
	return s.sessions.GetSessionTraces(sessionID)
}

func (s *Service) loadAttachments(attachments []AttachmentInput) ([]service.FileAttachment, error) {
	files := make([]service.FileAttachment, 0, len(attachments))
	for _, a := range attachments {
		data, err := os.ReadFile(a.Path)
		if err != nil {
			return nil, fmt.Errorf("cannot read attachment: %s", filepath.Base(a.Path))
		}
		files = append(files, service.FileAttachment{
			Path:     a.Path,
			MimeType: a.MimeType,
			Data:     data,
		})
	}
	return files, nil
}

func MarshalEventData(ev service.SSEEvent) ([]byte, error) {
	return json.Marshal(ev.Data)
}

func (s *Service) asyncContext() context.Context {
	if s.backgroundCtx != nil {
		return s.backgroundCtx
	}
	return context.Background()
}

func truncateForDisplay(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

func (s *Service) publishAsyncError(sessionID string, err error) {
	if s.publish == nil || err == nil {
		return
	}
	s.publish(sessionID, service.SSEEvent{
		Type: "error",
		Data: service.SSEErrorData{Message: err.Error()},
	})
}

func bindReservationCancel(ctx context.Context, reservation *service.SendReservation) (context.Context, func(), error) {
	if reservation == nil {
		return ctx, func() {}, nil
	}
	sendCtx, cancel := context.WithCancel(ctx)
	if !reservation.BindCancel(cancel) {
		reservation.Complete(service.TurnStateFailed, service.ErrNoActiveTurn)
		cancel()
		reservation.Release()
		return nil, nil, service.ErrNoActiveTurn
	}
	release := func() {
		cancel()
		reservation.Release()
	}
	return sendCtx, release, nil
}

func SnapshotToRFC3339(in []snapshot.Snapshot) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, snap := range in {
		out = append(out, map[string]any{
			"turn_index":  snap.TurnIndex,
			"commit_hash": snap.CommitHash,
			"summary":     snap.Summary,
			"files":       snap.Files,
			"created_at":  snap.CreatedAt.Format(time.RFC3339),
		})
	}
	return out
}
