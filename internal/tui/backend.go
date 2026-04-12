package tui

import "context"

// Backend is the application boundary the TUI depends on. It can be backed by
// the localhost HTTP API or by an in-process adapter over the same services.
type Backend interface {
	CreateSession(ctx context.Context, agentID, modelID string) (APISession, error)
	ListModels(ctx context.Context) ([]APIProviderModels, error)
	RefreshModels(ctx context.Context) ([]APIProviderModels, error)
	RefreshMCPTools(ctx context.Context) (int, error)
	UpdateSessionModel(ctx context.Context, id, modelID string) error
	UpdateSessionAgent(ctx context.Context, id, agentID string) error
	ListSessions(ctx context.Context) ([]APISession, error)
	GetSession(ctx context.Context, id string) (APISession, error)
	ListMessages(ctx context.Context, sessionID string) ([]APIMessage, error)
	DeleteSession(ctx context.Context, id string) error
	SendMessage(ctx context.Context, sessionID, content string, attachments []Attachment, agentID string, variant ...string) error
	CancelTurn(ctx context.Context, sessionID string) error
	SpawnSubagent(ctx context.Context, sessionID, agentID, task string) error
	ListAgents(ctx context.Context) ([]APIAgent, error)
	AnswerQuestion(ctx context.Context, sessionID, questionID, response string) error
	GetConfig(ctx context.Context) (APIConfig, error)
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
	ListSnapshots(ctx context.Context, sessionID string) ([]APISnapshot, error)
	RestoreSnapshot(ctx context.Context, sessionID string, turn int) error
	OpenStream(ctx context.Context, sessionID string) (sseConn, error)
}
