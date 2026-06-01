package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPTransportCallReadsJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Accept"); !strings.Contains(got, "application/json") || !strings.Contains(got, "text/event-stream") {
			t.Fatalf("Accept = %q, want json and event-stream", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test" {
			t.Fatalf("Authorization = %q, want Bearer test", got)
		}
		var request jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jsonRPCResponse{
			ID:     request.ID,
			Result: json.RawMessage(`{"ok":true}`),
		})
	}))
	defer server.Close()

	transport, err := NewHTTPTransport(server.URL, map[string]string{"Authorization": "Bearer test"})
	if err != nil {
		t.Fatalf("NewHTTPTransport() error = %v", err)
	}
	raw, err := transport.Call(context.Background(), "tools/list", nil)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if string(raw) != `{"ok":true}` {
		t.Fatalf("Call() = %s, want ok result", raw)
	}
}

func TestHTTPTransportCallReadsSSEResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := fmt.Fprintf(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":{\"ok\":true}}\n\n", request.ID); err != nil {
			t.Fatalf("Fprintf() error = %v", err)
		}
	}))
	defer server.Close()

	transport, err := NewHTTPTransport(server.URL, nil)
	if err != nil {
		t.Fatalf("NewHTTPTransport() error = %v", err)
	}
	raw, err := transport.Call(context.Background(), "tools/list", nil)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if string(raw) != `{"ok":true}` {
		t.Fatalf("Call() = %s, want ok result", raw)
	}
}

func TestHTTPTransportFallsBackToLegacySSEEndpoint(t *testing.T) {
	messages := make(chan string, 1)
	done := make(chan struct{})
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/sse" && r.Method == http.MethodPost:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		case r.URL.Path == "/sse" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatal("ResponseWriter is not a flusher")
			}
			if _, err := fmt.Fprintf(w, "event: endpoint\ndata: %s/messages\n\n", server.URL); err != nil {
				t.Fatalf("Fprintf(endpoint) error = %v", err)
			}
			flusher.Flush()
			select {
			case message := <-messages:
				if _, err := fmt.Fprintf(w, "event: message\ndata: %s\n\n", message); err != nil {
					t.Fatalf("Fprintf(message) error = %v", err)
				}
				flusher.Flush()
			case <-done:
			case <-r.Context().Done():
			}
		case r.URL.Path == "/messages" && r.Method == http.MethodPost:
			var request jsonRPCRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			messages <- fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":{"legacy":true}}`, request.ID)
			w.WriteHeader(http.StatusAccepted)
		default:
			http.NotFound(w, r)
		}
	}))
	defer func() {
		close(done)
		server.Close()
	}()

	transport, err := NewHTTPTransport(server.URL+"/sse", nil)
	if err != nil {
		t.Fatalf("NewHTTPTransport() error = %v", err)
	}
	raw, err := transport.Call(context.Background(), "tools/list", nil)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if string(raw) != `{"legacy":true}` {
		t.Fatalf("Call() = %s, want legacy result", raw)
	}
}
