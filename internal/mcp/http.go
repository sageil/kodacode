package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	httpMCPProtocolVersion = "2024-11-05"
	httpMaxResponseBytes   = 1 << 20
)

var (
	errHTTPMCPEndpointRequired = errors.New("http mcp: endpoint url is required")
	errHTTPMCPResponseTooLarge = errors.New("http mcp: response exceeded maximum frame size")
)

type httpTransport struct {
	endpoint string
	headers  map[string]string
	client   *http.Client
	seq      atomic.Int64

	sessionMu sync.Mutex
	sessionID string

	legacyMu       sync.Mutex
	legacyPostURL  string
	legacyPending  map[int64]chan rpcResult
	legacyReadErr  error
	legacyCancel   context.CancelFunc
	legacyEndpoint string
}

type sseEvent struct {
	event string
	data  string
}

func NewHTTPTransport(endpoint string, headers map[string]string) (Transport, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil, errHTTPMCPEndpointRequired
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("http mcp endpoint: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("http mcp endpoint must be absolute")
	}
	return &httpTransport{
		endpoint:      endpoint,
		headers:       cloneHeaders(headers),
		client:        &http.Client{Timeout: 30 * time.Second},
		legacyPending: make(map[int64]chan rpcResult),
	}, nil
}

func (t *httpTransport) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}
	requestID := t.seq.Add(1)
	request := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  method,
		Params:  params,
	}
	raw, fallback, err := t.postStreamable(ctx, request, requestID)
	if fallback {
		return t.callLegacy(ctx, request, requestID)
	}
	return raw, err
}

func (t *httpTransport) Notify(ctx context.Context, method string, params any) error {
	notification := jsonRPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	_, fallback, err := t.postStreamable(ctx, notification, 0)
	if fallback {
		return t.notifyLegacy(ctx, notification)
	}
	return err
}

func (t *httpTransport) Close() error {
	if t == nil {
		return nil
	}
	t.legacyMu.Lock()
	if t.legacyCancel != nil {
		t.legacyCancel()
	}
	t.legacyMu.Unlock()
	return nil
}

func (t *httpTransport) postStreamable(ctx context.Context, payload any, requestID int64) (json.RawMessage, bool, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, false, fmt.Errorf("marshal rpc request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	t.applyHeaders(req, true)
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		return nil, true, nil
	}
	if resp.StatusCode == http.StatusAccepted {
		return nil, false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, false, fmt.Errorf("http mcp POST %s: %s", t.endpoint, resp.Status)
	}
	t.recordSession(resp.Header.Get("Mcp-Session-Id"))
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/event-stream") {
		raw, err := readSSEResponse(ctx, resp.Body, requestID)
		return raw, false, err
	}
	raw, err := readLimitedBody(resp.Body)
	if err != nil {
		return nil, false, err
	}
	if requestID == 0 || len(bytes.TrimSpace(raw)) == 0 {
		return nil, false, nil
	}
	decoded, err := decodeRPCResponse(raw, requestID)
	return decoded, false, err
}

func (t *httpTransport) callLegacy(ctx context.Context, request jsonRPCRequest, requestID int64) (json.RawMessage, error) {
	if err := t.ensureLegacyConnected(ctx); err != nil {
		return nil, err
	}
	responseCh := make(chan rpcResult, 1)
	t.legacyMu.Lock()
	if t.legacyReadErr != nil {
		err := t.legacyReadErr
		t.legacyMu.Unlock()
		return nil, fmt.Errorf("mcp call %s: transport closed: %w", request.Method, err)
	}
	t.legacyPending[requestID] = responseCh
	postURL := t.legacyPostURL
	t.legacyMu.Unlock()

	if err := t.postLegacy(ctx, postURL, request); err != nil {
		t.removeLegacyPending(requestID)
		return nil, err
	}
	select {
	case result := <-responseCh:
		return result.data, result.err
	case <-ctx.Done():
		t.removeLegacyPending(requestID)
		return nil, fmt.Errorf("mcp call %s timed out: %w", request.Method, ctx.Err())
	}
}

func (t *httpTransport) notifyLegacy(ctx context.Context, notification jsonRPCNotification) error {
	if err := t.ensureLegacyConnected(ctx); err != nil {
		return err
	}
	t.legacyMu.Lock()
	postURL := t.legacyPostURL
	t.legacyMu.Unlock()
	return t.postLegacy(ctx, postURL, notification)
}

func (t *httpTransport) ensureLegacyConnected(ctx context.Context) error {
	t.legacyMu.Lock()
	if t.legacyPostURL != "" && t.legacyReadErr == nil {
		t.legacyMu.Unlock()
		return nil
	}
	if t.legacyCancel != nil {
		t.legacyCancel()
	}
	t.legacyPostURL = ""
	t.legacyReadErr = nil
	t.legacyEndpoint = ""
	legacyCtx, cancel := context.WithCancel(context.Background())
	t.legacyCancel = cancel
	t.legacyMu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.endpoint, nil)
	if err != nil {
		return err
	}
	t.applyHeaders(req, false)
	resp, err := t.client.Do(req)
	if err != nil {
		cancel()
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		cancel()
		return fmt.Errorf("http mcp SSE %s: %s", t.endpoint, resp.Status)
	}
	firstEvent, err := readSSEEvent(ctx, bufio.NewReader(resp.Body))
	if err != nil {
		_ = resp.Body.Close()
		cancel()
		return err
	}
	if firstEvent.event != "endpoint" || strings.TrimSpace(firstEvent.data) == "" {
		_ = resp.Body.Close()
		cancel()
		return fmt.Errorf("http mcp SSE %s: endpoint event missing", t.endpoint)
	}
	postURL, err := resolveLegacyEndpoint(t.endpoint, firstEvent.data)
	if err != nil {
		_ = resp.Body.Close()
		cancel()
		return err
	}
	t.legacyMu.Lock()
	t.legacyPostURL = postURL
	t.legacyEndpoint = strings.TrimSpace(firstEvent.data)
	t.legacyMu.Unlock()
	go t.readLegacySSE(legacyCtx, resp.Body)
	return nil
}

func (t *httpTransport) readLegacySSE(ctx context.Context, body io.ReadCloser) {
	defer func() {
		_ = body.Close()
	}()
	reader := bufio.NewReader(body)
	for {
		event, err := readSSEEvent(ctx, reader)
		if strings.TrimSpace(event.data) != "" && (event.event == "" || event.event == "message") {
			var response jsonRPCResponse
			if decodeErr := json.Unmarshal([]byte(event.data), &response); decodeErr == nil && response.ID != 0 {
				result := rpcResult{data: response.Result}
				if response.Error != nil {
					result.err = fmt.Errorf("mcp rpc error: %s", response.Error.Message)
				}
				t.legacyMu.Lock()
				ch, ok := t.legacyPending[response.ID]
				if ok {
					delete(t.legacyPending, response.ID)
				}
				t.legacyMu.Unlock()
				if ok {
					ch <- result
				}
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, context.Canceled) {
			return
		}
		t.legacyMu.Lock()
		t.legacyReadErr = err
		for id, ch := range t.legacyPending {
			ch <- rpcResult{err: fmt.Errorf("http mcp SSE stream closed before response for id=%d: %w", id, err)}
			delete(t.legacyPending, id)
		}
		t.legacyMu.Unlock()
		return
	}
}

func (t *httpTransport) postLegacy(ctx context.Context, endpoint string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal rpc request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	t.applyHeaders(req, false)
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http mcp POST %s: %s", endpoint, resp.Status)
	}
	return nil
}

func (t *httpTransport) applyHeaders(req *http.Request, streamable bool) {
	req.Header.Set("Content-Type", "application/json")
	if req.Method == http.MethodGet {
		req.Header.Set("Accept", "text/event-stream")
	} else if streamable {
		req.Header.Set("Accept", "application/json, text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	req.Header.Set("MCP-Protocol-Version", httpMCPProtocolVersion)
	for key, value := range t.headers {
		if strings.TrimSpace(key) == "" {
			continue
		}
		req.Header.Set(key, value)
	}
	t.sessionMu.Lock()
	sessionID := t.sessionID
	t.sessionMu.Unlock()
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
}

func (t *httpTransport) recordSession(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	t.sessionMu.Lock()
	t.sessionID = sessionID
	t.sessionMu.Unlock()
}

func (t *httpTransport) removeLegacyPending(id int64) {
	t.legacyMu.Lock()
	delete(t.legacyPending, id)
	t.legacyMu.Unlock()
}

func readSSEResponse(ctx context.Context, body io.Reader, requestID int64) (json.RawMessage, error) {
	reader := bufio.NewReader(body)
	for {
		event, err := readSSEEvent(ctx, reader)
		if strings.TrimSpace(event.data) != "" && (event.event == "" || event.event == "message") {
			raw, decodeErr := decodeRPCResponse([]byte(event.data), requestID)
			if decodeErr == nil || requestID != 0 {
				return raw, decodeErr
			}
		}
		if err != nil {
			return nil, err
		}
	}
}

func readSSEEvent(ctx context.Context, reader *bufio.Reader) (sseEvent, error) {
	var event sseEvent
	var data []string
	for {
		if err := ctx.Err(); err != nil {
			return event, err
		}
		line, err := readSSELine(reader)
		if err != nil {
			return event, err
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if line == "" {
			event.data = strings.Join(data, "\n")
			return event, nil
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			event.event = value
		case "data":
			data = append(data, value)
		}
	}
}

func readSSELine(reader *bufio.Reader) (string, error) {
	if reader == nil {
		return "", io.EOF
	}
	var line []byte
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(line)+len(fragment) > httpMaxResponseBytes {
			return "", errHTTPMCPResponseTooLarge
		}
		line = append(line, fragment...)
		if err == nil {
			return string(line), nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return string(line), err
	}
}

func readLimitedBody(body io.Reader) ([]byte, error) {
	limited := io.LimitReader(body, httpMaxResponseBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(raw) > httpMaxResponseBytes {
		return nil, errHTTPMCPResponseTooLarge
	}
	return raw, nil
}

func decodeRPCResponse(raw []byte, requestID int64) (json.RawMessage, error) {
	var response jsonRPCResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, err
	}
	if requestID != 0 && response.ID != requestID {
		return nil, fmt.Errorf("mcp rpc response id=%d, want id=%d", response.ID, requestID)
	}
	if response.Error != nil {
		return nil, fmt.Errorf("mcp rpc error: %s", response.Error.Message)
	}
	return response.Result, nil
}

func resolveLegacyEndpoint(baseURL, endpoint string) (string, error) {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return "", err
	}
	return base.ResolveReference(ref).String(), nil
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = value
	}
	return out
}
