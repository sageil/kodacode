package tui

import (
	"context"
	"encoding/json"
	"log"

	"github.com/sageil/kodacode/v1/internal/api/handler"
	"github.com/sageil/kodacode/v1/internal/apitypes"
	"github.com/sageil/kodacode/v1/internal/appservice"
	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/repository"
	"github.com/sageil/kodacode/v1/internal/service"
	"github.com/sageil/kodacode/v1/internal/snapshot"
)

// LocalBackendConfig wires an in-process backend over the same services used by
// the HTTP handlers. The TUI can use this without changing runtime behavior.
type LocalBackendConfig struct {
	Sessions        handler.SessionService
	Agents          handler.AgentService
	Settings        handler.SettingsService
	Registry        handler.ProviderRegistry
	Config          *config.Config
	SnapshotSvc     *snapshot.Service
	ProjectDir      string
	ToolCount       int
	BackgroundCtx   context.Context
	MCPStatus       func() []handler.MCPServerInfo
	RefreshMCPTools func(context.Context) (int, error)
	SyncProviders   func(context.Context) ([]string, error)
}

type LocalBackend struct {
	app *appservice.Service
}

func NewLocalBackend(cfg LocalBackendConfig) *LocalBackend {
	var mcpStatus func() []appservice.MCPServerInfo
	if cfg.MCPStatus != nil {
		mcpStatus = func() []appservice.MCPServerInfo {
			status := cfg.MCPStatus()
			out := make([]appservice.MCPServerInfo, 0, len(status))
			for _, s := range status {
				out = append(out, appservice.MCPServerInfo{
					Name:    s.Name,
					Active:  s.Active,
					Enabled: s.Enabled,
				})
			}
			return out
		}
	}
	return &LocalBackend{
		app: appservice.New(appservice.Config{
			Sessions:        cfg.Sessions,
			Agents:          cfg.Agents,
			Settings:        cfg.Settings,
			Registry:        cfg.Registry,
			Config:          cfg.Config,
			SnapshotSvc:     cfg.SnapshotSvc,
			ProjectDir:      cfg.ProjectDir,
			ToolCount:       cfg.ToolCount,
			BackgroundCtx:   cfg.BackgroundCtx,
			MCPStatus:       mcpStatus,
			RefreshMCPTools: cfg.RefreshMCPTools,
			SyncProviders:   cfg.SyncProviders,
			Publish:         localSessionPublisher(cfg.Sessions),
		}),
	}
}

func (b *LocalBackend) CreateSession(ctx context.Context, agentID, modelID string) (APISession, error) {
	sess, err := b.app.CreateSession(ctx, agentID, modelID)
	if err != nil {
		return APISession{}, err
	}
	return apitypes.SessionFromRepository(sess), nil
}

func (b *LocalBackend) ListModels(ctx context.Context) ([]APIProviderModels, error) {
	return apitypes.ProviderModelsFromDomain(b.app.ListModels(ctx)), nil
}

func (b *LocalBackend) RefreshModels(ctx context.Context) ([]APIProviderModels, error) {
	return apitypes.ProviderModelsFromDomain(b.app.RefreshModels(ctx)), nil
}

func (b *LocalBackend) SyncProviders(ctx context.Context) ([]string, error) {
	return b.app.SyncProviders(ctx)
}

func (b *LocalBackend) HasActiveTurns(ctx context.Context) (bool, error) {
	return b.app.HasActiveTurns(ctx)
}

func (b *LocalBackend) RefreshMCPTools(ctx context.Context) (int, error) {
	return b.app.RefreshMCPTools(ctx)
}

func (b *LocalBackend) UpdateSessionModel(ctx context.Context, id, modelID string) error {
	return b.app.UpdateSessionModel(ctx, id, modelID)
}

func (b *LocalBackend) UpdateSessionAgent(ctx context.Context, id, agentID string) error {
	return b.app.UpdateSessionAgent(ctx, id, agentID)
}

func (b *LocalBackend) ListSessions(ctx context.Context) ([]APISession, error) {
	sessions, err := b.app.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	return apitypes.SessionsFromRepository(sessions), nil
}

func (b *LocalBackend) GetSession(ctx context.Context, id string) (APISession, error) {
	sess, err := b.app.GetSession(ctx, id)
	if err != nil {
		return APISession{}, err
	}
	return apitypes.SessionFromRepository(sess), nil
}

func (b *LocalBackend) ListMessages(ctx context.Context, sessionID string) ([]APIMessage, error) {
	msgs, err := b.app.ListMessages(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return apitypes.MessagesFromViews(msgs), nil
}

func (b *LocalBackend) DeleteSession(ctx context.Context, id string) error {
	return b.app.DeleteSession(ctx, id)
}

func (b *LocalBackend) SendMessage(ctx context.Context, sessionID, content string, attachments []Attachment, agentID string, variant ...string) error {
	v := ""
	if len(variant) > 0 {
		v = variant[0]
	}
	return b.app.SendMessage(ctx, sessionID, content, toAppAttachments(attachments), agentID, v)
}

func (b *LocalBackend) CancelTurn(ctx context.Context, sessionID string) error {
	return b.app.CancelTurn(ctx, sessionID)
}

func (b *LocalBackend) SpawnSubagent(ctx context.Context, sessionID, agentID, task string) error {
	return b.app.SpawnSubagent(ctx, sessionID, agentID, task)
}

func (b *LocalBackend) ListAgents(ctx context.Context) ([]APIAgent, error) {
	_ = ctx
	agents := b.app.ListAgents()
	out := make([]APIAgent, 0, len(agents))
	for _, a := range agents {
		out = append(out, APIAgent{
			ID:          a.ID,
			Name:        a.Name,
			Description: a.Description,
			Mode:        string(a.Mode),
			Model:       a.Model,
		})
	}
	return out, nil
}

func (b *LocalBackend) AnswerQuestion(ctx context.Context, sessionID, questionID, response string) error {
	return b.app.AnswerQuestion(ctx, sessionID, questionID, response)
}

func (b *LocalBackend) GetConfig(ctx context.Context) (APIConfig, error) {
	return apitypes.ConfigFromView(b.app.GetConfig(ctx)), nil
}

func (b *LocalBackend) GetSetting(ctx context.Context, key string) (string, error) {
	value, err := b.app.GetSetting(ctx, key)
	if err != nil {
		if err == repository.ErrNotFound {
			return "", ErrSettingNotFound
		}
		return "", err
	}
	return value, nil
}

func (b *LocalBackend) SetSetting(ctx context.Context, key, value string) error {
	return b.app.SetSetting(ctx, key, value)
}

func (b *LocalBackend) ListSnapshots(ctx context.Context, sessionID string) ([]APISnapshot, error) {
	_ = ctx
	snapshots, err := b.app.ListSnapshots(sessionID)
	if err != nil {
		return nil, err
	}
	return apitypes.SnapshotsFromDomain(snapshots), nil
}

func (b *LocalBackend) RestoreSnapshot(ctx context.Context, sessionID string, turn int) error {
	_ = ctx
	return b.app.RestoreSnapshot(sessionID, turn)
}

func (b *LocalBackend) OpenStream(ctx context.Context, sessionID string) (sseConn, error) {
	sub, cancel, err := b.app.Subscribe(ctx, sessionID)
	if err != nil {
		return sseConn{}, err
	}
	events := make(chan SSEEventMsg, 64)
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer close(events)
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub:
				if !ok {
					return
				}
				data, err := json.Marshal(ev.Data)
				if err != nil {
					log.Printf("local stream: failed to marshal %s event: %v", ev.Type, err)
					continue
				}
				msg := SSEEventMsg{
					SessionID: sessionID,
					Type:      ev.Type,
					Data:      data,
				}
				select {
				case events <- msg:
				case <-ctx.Done():
					return
				}
				if ev.Type == "done" || ev.Type == "error" {
					return
				}
			}
		}
	}()

	return sseConn{
		sessionID: sessionID,
		events:    events,
		done:      done,
		close:     cancel,
	}, nil
}

func toAppAttachments(in []Attachment) []appservice.AttachmentInput {
	out := make([]appservice.AttachmentInput, 0, len(in))
	for _, a := range in {
		out = append(out, appservice.AttachmentInput{Path: a.Path, MimeType: a.MimeType})
	}
	return out
}

// localSessionPublisher returns nil when the configured session service does
// not expose a publisher hook. In that case async error forwarding falls back
// to whatever the underlying service emits directly over its own session stream.
func localSessionPublisher(sessions any) func(string, service.SSEEvent) {
	type publisher interface {
		Publisher() func(string, service.SSEEvent)
	}
	if pub, ok := sessions.(publisher); ok {
		return pub.Publisher()
	}
	return nil
}

var _ Backend = (*LocalBackend)(nil)
