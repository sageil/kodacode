package lsp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestHandleRequestWorkspaceConfigurationReturnsEmptyObjectEntries(t *testing.T) {
	var output bytes.Buffer
	client := &Client{stdin: bufferWriteCloser{Writer: &output}}

	client.handleRequest(7, "workspace/configuration", json.RawMessage(`{"items":[{"section":"typescript"},{"section":"javascript"}]}`))

	response := decodeClientResponse(t, output.Bytes())
	if response.Error != nil {
		t.Fatalf("response error = %#v, want nil", response.Error)
	}
	var got []any
	if err := json.Unmarshal(response.Result, &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(got))
	}
	for index, value := range got {
		entry, ok := value.(map[string]any)
		if !ok || len(entry) != 0 {
			t.Fatalf("result[%d] = %#v, want empty object", index, value)
		}
	}
}

func TestHandleRequestWorkspaceFoldersReturnsConfiguredFolders(t *testing.T) {
	var output bytes.Buffer
	client := &Client{stdin: bufferWriteCloser{Writer: &output}}
	client.SetWorkspaceFolders([]WorkspaceFolder{{URI: "file:///repo", Name: "repo"}})

	client.handleRequest(9, "workspace/workspaceFolders", nil)

	response := decodeClientResponse(t, output.Bytes())
	if response.Error != nil {
		t.Fatalf("response error = %#v, want nil", response.Error)
	}
	var got []WorkspaceFolder
	if err := json.Unmarshal(response.Result, &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(got) != 1 || got[0].URI != "file:///repo" || got[0].Name != "repo" {
		t.Fatalf("workspace folders = %#v, want configured folder", got)
	}
}

func TestHandleRequestUnknownMethodReturnsMethodNotFound(t *testing.T) {
	var output bytes.Buffer
	client := &Client{stdin: bufferWriteCloser{Writer: &output}}

	client.handleRequest(11, "workspace/unknownThing", nil)

	response := decodeClientResponse(t, output.Bytes())
	if response.Error == nil {
		t.Fatal("response error = nil, want method not found")
	}
	if response.Error.Code != -32601 {
		t.Fatalf("response error code = %d, want -32601", response.Error.Code)
	}
}

func TestClientCallReturnsConnectionLostWhenPendingChannelCloses(t *testing.T) {
	var output bytes.Buffer
	client := &Client{
		stdin:   bufferWriteCloser{Writer: &output},
		done:    make(chan struct{}),
		readErr: errors.New("server exited"),
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Call(context.Background(), "textDocument/rename", map[string]any{}, nil)
	}()

	deadline := time.After(2 * time.Second)
	for {
		closed := false
		client.pending.Range(func(key, value any) bool {
			close(value.(chan *jsonRPCMessage))
			client.pending.Delete(key)
			closed = true
			return false
		})
		if closed {
			break
		}
		select {
		case <-deadline:
			t.Fatal("pending LSP call was not registered")
		case <-time.After(10 * time.Millisecond):
		}
	}

	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), "lsp connection lost: server exited") {
			t.Fatalf("Call() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Call() did not return")
	}
}

func TestLSPBinDirsIncludesHomebrewBin(t *testing.T) {
	dirs := lspBinDirs()
	if !slices.Contains(dirs, "/opt/homebrew/bin") {
		t.Fatalf("lspBinDirs() = %#v, want /opt/homebrew/bin", dirs)
	}
}

type bufferWriteCloser struct {
	io.Writer
}

func (b bufferWriteCloser) Close() error {
	return nil
}

func decodeClientResponse(t *testing.T, payload []byte) jsonRPCMessage {
	t.Helper()
	body, err := readContentLengthMessage(bufio.NewReader(bytes.NewReader(payload)))
	if err != nil {
		t.Fatalf("readContentLengthMessage() error = %v", err)
	}
	var response jsonRPCMessage
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return response
}
