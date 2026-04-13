package apitypes

import (
	"time"

	"github.com/sageil/kodacode/v1/internal/appservice"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/repository"
	"github.com/sageil/kodacode/v1/internal/service"
	"github.com/sageil/kodacode/v1/internal/snapshot"
)

type Session struct {
	ID                   string    `json:"ID"`
	Title                string    `json:"Title"`
	AgentID              string    `json:"AgentID"`
	ModelID              string    `json:"ModelID"`
	ParentID             string    `json:"ParentID,omitempty"`
	BranchPointMessageID string    `json:"BranchPointMessageID,omitempty"`
	UpdatedAt            time.Time `json:"UpdatedAt"`
}

type MessagePart struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

type Message struct {
	ID        string        `json:"id"`
	SessionID string        `json:"session_id"`
	Role      string        `json:"role"`
	Content   string        `json:"content"`
	Parts     []MessagePart `json:"parts,omitempty"`
}

type MCPServer struct {
	Name    string `json:"name"`
	Active  bool   `json:"active"`
	Enabled bool   `json:"enabled"`
}

type Config struct {
	DefaultAgent     string      `json:"default_agent"`
	ToolCount        int         `json:"tool_count"`
	MCPServers       []MCPServer `json:"mcp_servers"`
	ErrorDisplayTime int         `json:"error_display_time"`
	TraceEnabled     bool        `json:"trace_enabled"`
}

type ProviderModels struct {
	ProviderID   string  `json:"provider_id"`
	ProviderName string  `json:"provider_name"`
	Models       []Model `json:"models"`
}

type Model struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	ContextSize    int     `json:"context_size"`
	MaxInputTokens int     `json:"max_input_tokens,omitempty"`
	Reasoning      bool    `json:"reasoning,omitempty"`
	ToolCall       bool    `json:"tool_call,omitempty"`
	Vision         bool    `json:"vision,omitempty"`
	CostInput      float64 `json:"cost_input,omitempty"`
	CostOutput     float64 `json:"cost_output,omitempty"`
}

type Snapshot struct {
	TurnIndex  int      `json:"turn_index"`
	CommitHash string   `json:"commit_hash"`
	Summary    string   `json:"summary"`
	Files      []string `json:"files"`
	CreatedAt  string   `json:"created_at"`
}

type TurnStatus struct {
	SessionID         string    `json:"session_id"`
	OperationID       string    `json:"operation_id,omitempty"`
	State             string    `json:"state"`
	Active            bool      `json:"active"`
	QueueDepth        int       `json:"queue_depth,omitempty"`
	QueuedOperationID string    `json:"queued_operation_id,omitempty"`
	CancelRequested   bool      `json:"cancel_requested,omitempty"`
	Error             string    `json:"error,omitempty"`
	StartedAt         time.Time `json:"started_at,omitempty"`
	UpdatedAt         time.Time `json:"updated_at,omitempty"`
	FinishedAt        time.Time `json:"finished_at,omitempty"`
}

func SessionFromRepository(sess repository.Session) Session {
	return Session{
		ID:                   sess.ID,
		Title:                sess.Title,
		AgentID:              sess.AgentID,
		ModelID:              sess.ModelID,
		ParentID:             sess.ParentID,
		BranchPointMessageID: sess.BranchPointMessageID,
		UpdatedAt:            sess.UpdatedAt,
	}
}

func SessionsFromRepository(sessions []repository.Session) []Session {
	out := make([]Session, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, SessionFromRepository(sess))
	}
	return out
}

func MessagePartsFromViews(parts []appservice.MessagePartView) []MessagePart {
	out := make([]MessagePart, 0, len(parts))
	for _, p := range parts {
		out = append(out, MessagePart{Type: p.Type, Content: p.Content})
	}
	return out
}

func MessagesFromViews(msgs []appservice.MessageView) []Message {
	out := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, Message{
			ID:        m.ID,
			SessionID: m.SessionID,
			Role:      m.Role,
			Content:   m.Content,
			Parts:     MessagePartsFromViews(m.Parts),
		})
	}
	return out
}

func ConfigFromView(cfg appservice.ConfigView) Config {
	mcpServers := make([]MCPServer, 0, len(cfg.MCPServers))
	for _, s := range cfg.MCPServers {
		mcpServers = append(mcpServers, MCPServer{
			Name:    s.Name,
			Active:  s.Active,
			Enabled: s.Enabled,
		})
	}
	return Config{
		DefaultAgent:     cfg.DefaultAgent,
		ToolCount:        cfg.ToolCount,
		MCPServers:       mcpServers,
		ErrorDisplayTime: cfg.ErrorDisplayTime,
		TraceEnabled:     cfg.TraceEnabled,
	}
}

func ProviderModelsFromDomain(models []provider.ProviderModels) []ProviderModels {
	out := make([]ProviderModels, 0, len(models))
	for _, pm := range models {
		var apiModels []Model
		for _, m := range pm.Models {
			apiModels = append(apiModels, Model{
				ID:             m.ID,
				Name:           m.Name,
				ContextSize:    m.ContextSize,
				MaxInputTokens: m.MaxInputTokens,
				Reasoning:      m.Reasoning,
				ToolCall:       m.ToolCall,
				Vision:         m.Vision,
				CostInput:      m.CostInput,
				CostOutput:     m.CostOutput,
			})
		}
		out = append(out, ProviderModels{
			ProviderID:   pm.ProviderID,
			ProviderName: pm.ProviderName,
			Models:       apiModels,
		})
	}
	return out
}

func TurnStatusFromDomain(status service.TurnStatus) TurnStatus {
	return TurnStatus{
		SessionID:         status.SessionID,
		OperationID:       status.OperationID,
		State:             string(status.State),
		Active:            status.Active,
		QueueDepth:        status.QueueDepth,
		QueuedOperationID: status.QueuedOperationID,
		CancelRequested:   status.CancelRequested,
		Error:             status.Error,
		StartedAt:         status.StartedAt,
		UpdatedAt:         status.UpdatedAt,
		FinishedAt:        status.FinishedAt,
	}
}

func SnapshotsFromDomain(snapshots []snapshot.Snapshot) []Snapshot {
	out := make([]Snapshot, 0, len(snapshots))
	for _, s := range snapshots {
		out = append(out, Snapshot{
			TurnIndex:  s.TurnIndex,
			CommitHash: s.CommitHash,
			Summary:    s.Summary,
			Files:      s.Files,
			CreatedAt:  s.CreatedAt.Format(time.RFC3339),
		})
	}
	return out
}
