package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sageil/kodacode/v1/internal/apitypes"
)

type APIClient struct {
	base           string // e.g. "http://127.0.0.1:54321"
	client         *http.Client
	SSEReadTimeout time.Duration
}

func NewAPIClient(base string) *APIClient {
	return &APIClient{
		base: base,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *APIClient) url(path string) string {
	return c.base + path
}

func (c *APIClient) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("api client marshal: %w", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.url(path), r)
	if err != nil {
		return nil, fmt.Errorf("api client new request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("api client do: %w", err)
	}
	return resp, nil
}

func decodeJSON[T any](resp *http.Response) (T, error) {
	defer resp.Body.Close() //nolint:errcheck // best-effort close
	var v T
	if resp.StatusCode >= 400 {
		return v, fmt.Errorf("api client: status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return v, fmt.Errorf("api client decode: %w", err)
	}
	return v, nil
}

type APISession = apitypes.Session
type APIMessagePart = apitypes.MessagePart
type APIMessage = apitypes.Message

type APIAgent struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Mode        string `json:"mode"`
	Model       string `json:"model"`
}

type APIMCPServer = apitypes.MCPServer
type APIConfig = apitypes.Config

func (c *APIClient) CreateSession(ctx context.Context, agentID, modelID string) (APISession, error) {
	body := map[string]string{"agent_id": agentID, "model_id": modelID}
	resp, err := c.do(ctx, http.MethodPost, "/sessions", body)
	if err != nil {
		return APISession{}, err
	}
	return decodeJSON[APISession](resp)
}

type APIProviderModels = apitypes.ProviderModels
type APIModel = apitypes.Model

func (c *APIClient) ListModels(ctx context.Context) ([]APIProviderModels, error) {
	resp, err := c.do(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return nil, err
	}
	return decodeJSON[[]APIProviderModels](resp)
}

func (c *APIClient) RefreshModels(ctx context.Context) ([]APIProviderModels, error) {
	resp, err := c.do(ctx, http.MethodPost, "/models/refresh", nil)
	if err != nil {
		return nil, err
	}
	return decodeJSON[[]APIProviderModels](resp)
}

func (c *APIClient) RefreshMCPTools(ctx context.Context) (int, error) {
	resp, err := c.do(ctx, http.MethodPost, "/mcp/refresh", nil)
	if err != nil {
		return 0, err
	}
	result, err := decodeJSON[struct{ Tools int }](resp)
	if err != nil {
		return 0, err
	}
	return result.Tools, nil
}

func (c *APIClient) UpdateSessionModel(ctx context.Context, id, modelID string) error {
	body := map[string]string{"model_id": modelID}
	resp, err := c.do(ctx, http.MethodPatch, "/sessions/"+id, body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("update session model: status %d", resp.StatusCode)
	}
	return nil
}

func (c *APIClient) UpdateSessionAgent(ctx context.Context, id, agentID string) error {
	body := map[string]string{"agent_id": agentID}
	resp, err := c.do(ctx, http.MethodPatch, "/sessions/"+id, body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("update session agent: status %d", resp.StatusCode)
	}
	return nil
}

func (c *APIClient) ListSessions(ctx context.Context) ([]APISession, error) {
	resp, err := c.do(ctx, http.MethodGet, "/sessions", nil)
	if err != nil {
		return nil, err
	}
	return decodeJSON[[]APISession](resp)
}

func (c *APIClient) GetSession(ctx context.Context, id string) (APISession, error) {
	resp, err := c.do(ctx, http.MethodGet, "/sessions/"+id, nil)
	if err != nil {
		return APISession{}, err
	}
	return decodeJSON[APISession](resp)
}

func (c *APIClient) ListMessages(ctx context.Context, sessionID string) ([]APIMessage, error) {
	resp, err := c.do(ctx, http.MethodGet, "/sessions/"+sessionID+"/messages", nil)
	if err != nil {
		return nil, err
	}
	return decodeJSON[[]APIMessage](resp)
}

func (c *APIClient) DeleteSession(ctx context.Context, id string) error {
	resp, err := c.do(ctx, http.MethodDelete, "/sessions/"+id, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode >= 400 {
		return fmt.Errorf("delete session: status %d", resp.StatusCode)
	}
	return nil
}

type APIAttachment struct {
	Path     string `json:"path"`
	MimeType string `json:"mime_type"`
}

func (c *APIClient) SendMessage(ctx context.Context, sessionID, content string, attachments []Attachment, agentID string, variant ...string) error {
	type sendReq struct {
		Content     string          `json:"content"`
		Attachments []APIAttachment `json:"attachments,omitempty"`
		AgentID     string          `json:"agent_id,omitempty"`
		Variant     string          `json:"variant,omitempty"`
	}
	req := sendReq{Content: content, AgentID: agentID}
	if len(variant) > 0 {
		req.Variant = variant[0]
	}
	for _, a := range attachments {
		req.Attachments = append(req.Attachments, APIAttachment{
			Path:     a.Path,
			MimeType: a.MimeType,
		})
	}
	resp, err := c.do(ctx, http.MethodPost, "/sessions/"+sessionID+"/messages", req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode >= 400 {
		return fmt.Errorf("send message: status %d", resp.StatusCode)
	}
	return nil
}

func (c *APIClient) CancelTurn(ctx context.Context, sessionID string) error {
	resp, err := c.do(ctx, http.MethodPost, "/sessions/"+sessionID+"/cancel", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode >= 400 {
		return fmt.Errorf("cancel turn: status %d", resp.StatusCode)
	}
	return nil
}

func (c *APIClient) SpawnSubagent(ctx context.Context, sessionID, agentID, task string) error {
	body := map[string]string{"agent_id": agentID, "task": task}
	resp, err := c.do(ctx, http.MethodPost, "/sessions/"+sessionID+"/subagent", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode >= 400 {
		return fmt.Errorf("spawn subagent: status %d", resp.StatusCode)
	}
	return nil
}

func (c *APIClient) ListAgents(ctx context.Context) ([]APIAgent, error) {
	resp, err := c.do(ctx, http.MethodGet, "/agents", nil)
	if err != nil {
		return nil, err
	}
	return decodeJSON[[]APIAgent](resp)
}

func (c *APIClient) AnswerQuestion(ctx context.Context, sessionID, questionID, response string) error {
	body := map[string]any{"question_id": questionID, "response": response}
	resp, err := c.do(ctx, http.MethodPost, "/sessions/"+sessionID+"/answer", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode >= 400 {
		return fmt.Errorf("answer question: status %d", resp.StatusCode)
	}
	return nil
}

func (c *APIClient) StreamURL(sessionID string) string {
	return c.url("/sessions/" + sessionID + "/stream")
}

func (c *APIClient) OpenStream(ctx context.Context, sessionID string) (sseConn, error) {
	return ListenSSE(ctx, sessionID, c.StreamURL(sessionID), c.SSEReadTimeout), nil
}

func (c *APIClient) GetConfig(ctx context.Context) (APIConfig, error) {
	resp, err := c.do(ctx, http.MethodGet, "/config", nil)
	if err != nil {
		return APIConfig{}, err
	}
	return decodeJSON[APIConfig](resp)
}

type APISetting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (c *APIClient) GetSetting(ctx context.Context, key string) (string, error) {
	resp, err := c.do(ctx, http.MethodGet, "/settings/"+key, nil)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusNotFound {
		_ = resp.Body.Close()
		return "", ErrSettingNotFound
	}
	if resp.StatusCode >= 400 {
		_ = resp.Body.Close()
		return "", fmt.Errorf("get setting: status %d", resp.StatusCode)
	}
	s, err := decodeJSON[APISetting](resp) // decodeJSON closes body
	if err != nil {
		return "", err
	}
	return s.Value, nil
}

func (c *APIClient) SetSetting(ctx context.Context, key, value string) error {
	body := map[string]string{"value": value}
	resp, err := c.do(ctx, http.MethodPut, "/settings/"+key, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode >= 400 {
		return fmt.Errorf("set setting: status %d", resp.StatusCode)
	}
	return nil
}

var ErrSettingNotFound = fmt.Errorf("setting not found")

type APISnapshot = apitypes.Snapshot

func (c *APIClient) ListSnapshots(ctx context.Context, sessionID string) ([]APISnapshot, error) {
	resp, err := c.do(ctx, http.MethodGet, "/sessions/"+sessionID+"/snapshots", nil)
	if err != nil {
		return nil, err
	}
	return decodeJSON[[]APISnapshot](resp)
}

func (c *APIClient) RestoreSnapshot(ctx context.Context, sessionID string, turn int) error {
	resp, err := c.do(ctx, http.MethodPost, fmt.Sprintf("/sessions/%s/snapshots/%d/restore", sessionID, turn), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode >= 400 {
		return fmt.Errorf("restore snapshot: status %d", resp.StatusCode)
	}
	return nil
}

var _ Backend = (*APIClient)(nil)
