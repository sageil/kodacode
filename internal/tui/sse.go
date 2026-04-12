package tui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/v1/internal/provider"
)

type SSEEventMsg struct {
	SessionID string
	Type      string
	Data      json.RawMessage
}

type SSEErrorMsg struct {
	SessionID string
	Err       error
}

type SSEDeltaPayload struct {
	Content string `json:"content"`
}

// CostSnapshotPayload mirrors service.CostSnapshot for JSON deserialization.
type CostSnapshotPayload struct {
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	ReasoningTokens  int     `json:"reasoning_tokens,omitempty"`
	CacheReadTokens  int     `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int     `json:"cache_write_tokens,omitempty"`
	TotalCost        float64 `json:"total_cost"`
	SubagentCost     float64 `json:"subagent_cost,omitempty"`
	SubagentInputs   int     `json:"subagent_inputs,omitempty"`
	SubagentOutputs  int     `json:"subagent_outputs,omitempty"`
}

type SSEDonePayload struct {
	Usage           provider.Usage       `json:"usage"`
	ContextSize     int                  `json:"context_size"`
	MaxInputTokens  int                  `json:"max_input_tokens,omitempty"`
	MaxOutputTokens int                  `json:"max_output_tokens,omitempty"`
	SessionCost     float64              `json:"session_cost"`
	SubagentCost    float64              `json:"subagent_cost,omitempty"`
	BudgetWarn      bool                 `json:"budget_warn,omitempty"`
	CostSnapshot    *CostSnapshotPayload `json:"cost_snapshot,omitempty"`
}

// sseConn holds the live channel pair for an open event stream. App stores this
// so it can re-issue sseReadCmd after each event without reconnecting.
type sseConn struct {
	sessionID string
	events    <-chan SSEEventMsg
	done      <-chan struct{}
	close     func()
}

func (c sseConn) Close() {
	if c.close != nil {
		c.close()
	}
}

// ListenSSE opens an SSE connection to url in a background goroutine.
// readTimeout controls how long to wait for data before declaring the stream
// dead. Zero uses the default (2 minutes).
func ListenSSE(ctx context.Context, sessionID, url string, readTimeout time.Duration) sseConn {
	if readTimeout <= 0 {
		readTimeout = 2 * time.Minute
	}
	events := make(chan SSEEventMsg, 64)
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer close(events)
		scanSSE(ctx, sessionID, url, events, readTimeout)
	}()

	return sseConn{sessionID: sessionID, events: events, done: done}
}

// SSEBatchMsg carries one or more SSE events that arrived between update cycles.
// Processing them in a single Update call avoids per-event render overhead.
type SSEBatchMsg struct {
	Events []SSEEventMsg
	Done   bool // true if the channel was closed (stream ended)
	ConnID string
}

// sseReadCmd returns a tea.Cmd that blocks until at least one event arrives,
// then drains all immediately available events from the channel. This batches
// rapid SSE events into a single bubbletea update cycle.
func sseReadCmd(conn sseConn) tea.Cmd {
	return func() tea.Msg {
		// Block for the first event.
		var batch SSEBatchMsg
		batch.ConnID = conn.sessionID
		select {
		case ev, ok := <-conn.events:
			if !ok {
				batch.Done = true
				return batch
			}
			batch.Events = append(batch.Events, ev)
		case <-conn.done:
			batch.Done = true
			return batch
		}
		// Drain all immediately available events (non-blocking).
		for {
			select {
			case ev, ok := <-conn.events:
				if !ok {
					batch.Done = true
					return batch
				}
				batch.Events = append(batch.Events, ev)
			default:
				return batch
			}
		}
	}
}

// scanSSE connects to the SSE endpoint and scans its body, sending each parsed
// event to out. It returns when the response body closes or ctx is cancelled.
func scanSSE(ctx context.Context, sessionID, url string, out chan<- SSEEventMsg, readTimeout time.Duration) {
	client := &http.Client{Timeout: 0} // no timeout on streaming connection

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		out <- SSEEventMsg{
			SessionID: sessionID,
			Type:      "error",
			Data:      jsonError(fmt.Errorf("sse new request: %w", err)),
		}
		return
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := client.Do(req)
	if err != nil {
		// Don't report context cancellation as an error — it's an
		// intentional shutdown (e.g. user pressed Esc).
		if ctx.Err() != nil {
			return
		}
		out <- SSEEventMsg{
			SessionID: sessionID,
			Type:      "error",
			Data:      jsonError(fmt.Errorf("sse connect: %w", err)),
		}
		return
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		out <- SSEEventMsg{
			SessionID: sessionID,
			Type:      "error",
			Data:      jsonError(fmt.Errorf("sse status %d", resp.StatusCode)),
		}
		return
	}

	scanner := bufio.NewScanner(newTimeoutReader(resp.Body, readTimeout))
	scanner.Buffer(make([]byte, 256*1024), 2*1024*1024)
	var eventType, dataLine string
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			eventType = line[len("event: "):]
		case strings.HasPrefix(line, "data: "):
			dataLine = line[len("data: "):]
		case line == "":
			if eventType != "" && dataLine != "" {
				if eventType == "reasoning_delta" || eventType == "reasoning_done" {
					log.Printf("[5-scanSSE] parsed %s event: %d bytes data", eventType, len(dataLine))
				}
				out <- SSEEventMsg{
					SessionID: sessionID,
					Type:      eventType,
					Data:      json.RawMessage(dataLine),
				}
			}
			eventType = ""
			dataLine = ""
		}

		// Respect context cancellation between lines.
		if ctx.Err() != nil {
			return
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		out <- SSEEventMsg{
			SessionID: sessionID,
			Type:      "error",
			Data:      jsonError(fmt.Errorf("sse stream: %w", err)),
		}
	}
}

// jsonError marshals an error message into a raw JSON payload for SSEEventMsg.
// Returns a fallback JSON object on marshal failure (which cannot happen for
// a simple string).
func jsonError(err error) json.RawMessage {
	type payload struct {
		Message string `json:"message"`
	}
	b, _ := json.Marshal(payload{Message: err.Error()}) //nolint:errcheck // simple struct, never fails
	return b
}

// timeoutReader wraps an io.Reader and returns an error if no data arrives
// within the configured timeout. This prevents hung SSE connections from
// blocking forever when the server stops sending events.
//
// A single background goroutine performs blocking reads into its own buffer,
// avoiding the data race of spawning a goroutine per Read call.
type timeoutReader struct {
	results chan readResult
	timeout time.Duration
	buf     []byte // leftover data from a previous read
}

type readResult struct {
	data []byte
	err  error
}

func newTimeoutReader(r io.Reader, timeout time.Duration) *timeoutReader {
	tr := &timeoutReader{
		results: make(chan readResult, 1),
		timeout: timeout,
	}
	go func() {
		defer close(tr.results)
		buf := make([]byte, 32*1024)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				cp := make([]byte, n)
				copy(cp, buf[:n])
				tr.results <- readResult{data: cp}
			}
			if err != nil {
				tr.results <- readResult{err: err}
				return
			}
		}
	}()
	return tr
}

func (tr *timeoutReader) Read(p []byte) (int, error) {
	// Drain leftover from a previous read that produced more data than p could hold.
	if len(tr.buf) > 0 {
		n := copy(p, tr.buf)
		tr.buf = tr.buf[n:]
		return n, nil
	}

	select {
	case res, ok := <-tr.results:
		if !ok {
			return 0, io.EOF
		}
		if res.err != nil {
			return 0, res.err
		}
		n := copy(p, res.data)
		if n < len(res.data) {
			tr.buf = res.data[n:]
		}
		return n, nil
	case <-time.After(tr.timeout):
		return 0, fmt.Errorf("sse: no data received for %s", tr.timeout)
	}
}
