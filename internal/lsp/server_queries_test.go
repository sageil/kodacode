package lsp

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBuildCodeActionParamsMarshalsEmptyDiagnosticsArray(t *testing.T) {
	params := buildCodeActionParams(
		"/repo/src/cache.ts",
		Range{
			Start: Position{Line: 27, Character: 0},
			End:   Position{Line: 28, Character: 0},
		},
		nil,
		[]string{"quickfix"},
	)

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `"diagnostics":[]`) {
		t.Fatalf("marshaled params missing empty diagnostics array: %s", text)
	}
	if !strings.Contains(text, `"only":["quickfix"]`) {
		t.Fatalf("marshaled params missing only filter: %s", text)
	}
}

func TestBuildCodeActionParamsMarshalsDiagnosticsPayload(t *testing.T) {
	params := buildCodeActionParams(
		"/repo/src/cache.ts",
		Range{
			Start: Position{Line: 27, Character: 0},
			End:   Position{Line: 28, Character: 0},
		},
		[]Diagnostic{{
			Range: Range{
				Start: Position{Line: 27, Character: 0},
				End:   Position{Line: 27, Character: 10},
			},
			Severity: 1,
			Message:  "type error",
			Source:   "tsserver",
		}},
		nil,
	)

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	text := string(data)
	for _, want := range []string{
		`"diagnostics":[{`,
		`"message":"type error"`,
		`"source":"tsserver"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("marshaled params missing %q: %s", want, text)
		}
	}
}

func TestRefreshDiagnosticsWaitsForTargetVersion(t *testing.T) {
	server := newTestServer()
	path := writeTestFile(t, "cache.ts", "const value = 1;\n")
	uri := FileURI(path)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}

	server.mu.Lock()
	server.openFiles[uri] = openDocumentState{
		Version: 1,
		ModTime: info.ModTime().UnixNano(),
		Size:    info.Size(),
	}
	server.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	type result struct {
		diagnostics []Diagnostic
		err         error
	}
	done := make(chan result, 1)
	go func() {
		diagnostics, err := server.RefreshDiagnostics(ctx, path)
		done <- result{diagnostics: diagnostics, err: err}
	}()

	waitForDocumentVersion(t, server, path, 1)

	staleVersion := 0
	server.handlePublishedDiagnostics(PublishDiagnosticsParams{
		URI:         uri,
		Version:     &staleVersion,
		Diagnostics: []Diagnostic{{Message: "stale"}},
	})

	select {
	case res := <-done:
		t.Fatalf("RefreshDiagnostics() returned after stale publish: diagnostics=%#v err=%v", res.diagnostics, res.err)
	case <-time.After(50 * time.Millisecond):
	}

	targetVersion := 1
	server.handlePublishedDiagnostics(PublishDiagnosticsParams{
		URI:     uri,
		Version: &targetVersion,
		Diagnostics: []Diagnostic{{
			Message: "fresh",
			Range: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: 0, Character: 5},
			},
		}},
	})

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("RefreshDiagnostics() error = %v", res.err)
		}
		if len(res.diagnostics) != 1 || res.diagnostics[0].Message != "fresh" {
			t.Fatalf("diagnostics = %#v, want fresh target-version diagnostics", res.diagnostics)
		}
	case <-time.After(time.Second):
		t.Fatal("RefreshDiagnostics() did not return after target-version publish")
	}
}

func TestRefreshDiagnosticsReturnsAfterEmptyTargetPublish(t *testing.T) {
	server := newTestServer()
	path := writeTestFile(t, "cache.ts", "const value = 1;\n")
	uri := FileURI(path)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	type result struct {
		diagnostics []Diagnostic
		err         error
	}
	done := make(chan result, 1)
	go func() {
		diagnostics, err := server.RefreshDiagnostics(ctx, path)
		done <- result{diagnostics: diagnostics, err: err}
	}()

	waitForDocumentVersion(t, server, path, 1)

	targetVersion := 1
	server.handlePublishedDiagnostics(PublishDiagnosticsParams{
		URI:         uri,
		Version:     &targetVersion,
		Diagnostics: []Diagnostic{},
	})

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("RefreshDiagnostics() error = %v", res.err)
		}
		if res.diagnostics != nil {
			t.Fatalf("diagnostics = %#v, want nil", res.diagnostics)
		}
	case <-time.After(time.Second):
		t.Fatal("RefreshDiagnostics() did not return after empty target publish")
	}
}

func newTestServer() *Server {
	server := &Server{
		client:                 &Client{stdin: nopWriteCloser{Writer: io.Discard}},
		openFiles:              make(map[string]openDocumentState),
		pullDiagnosticsSupport: diagnosticsPullUnsupported,
		diagnostics:            make(map[string]diagnosticSnapshot),
		diagSeqs:               make(map[string]int64),
	}
	server.diagCond = syncCondForTest(&server.diagMu)
	return server
}

func syncCondForTest(mu *sync.RWMutex) *sync.Cond {
	return sync.NewCond(mu.RLocker())
}

type nopWriteCloser struct {
	io.Writer
}

func (n nopWriteCloser) Close() error {
	return nil
}

func writeTestFile(t *testing.T, name, content string) string {
	t.Helper()
	path := t.TempDir() + string(os.PathSeparator) + name
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	return path
}

func waitForDocumentVersion(t *testing.T, server *Server, path string, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got, ok := server.DocumentVersion(path); ok && got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	got, ok := server.DocumentVersion(path)
	t.Fatalf("DocumentVersion() = (%d, %t), want (%d, true)", got, ok, want)
}

func TestRefreshDiagnosticsReturnsAfterVersionlessPublish(t *testing.T) {
	server := newTestServer()
	path := writeTestFile(t, "cache.ts", "const value = 1;\n")
	uri := FileURI(path)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	type result struct {
		diagnostics []Diagnostic
		err         error
	}
	done := make(chan result, 1)
	go func() {
		diagnostics, err := server.RefreshDiagnostics(ctx, path)
		done <- result{diagnostics: diagnostics, err: err}
	}()

	waitForDocumentVersion(t, server, path, 1)
	server.handlePublishedDiagnostics(PublishDiagnosticsParams{
		URI: uri,
		Diagnostics: []Diagnostic{{
			Message: "fresh versionless",
			Range: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: 0, Character: 5},
			},
		}},
	})

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("RefreshDiagnostics() error = %v", res.err)
		}
		if len(res.diagnostics) != 1 || res.diagnostics[0].Message != "fresh versionless" {
			t.Fatalf("diagnostics = %#v, want fresh versionless diagnostics", res.diagnostics)
		}
	case <-time.After(time.Second):
		t.Fatal("RefreshDiagnostics() did not return after versionless publish")
	}
}

func TestRefreshDiagnosticsReturnsAfterVersionlessEmptyPublish(t *testing.T) {
	server := newTestServer()
	path := writeTestFile(t, "cache.ts", "const value = 1;\n")
	uri := FileURI(path)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	type result struct {
		diagnostics []Diagnostic
		err         error
	}
	done := make(chan result, 1)
	go func() {
		diagnostics, err := server.RefreshDiagnostics(ctx, path)
		done <- result{diagnostics: diagnostics, err: err}
	}()

	waitForDocumentVersion(t, server, path, 1)
	server.handlePublishedDiagnostics(PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: []Diagnostic{},
	})

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("RefreshDiagnostics() error = %v", res.err)
		}
		if res.diagnostics != nil {
			t.Fatalf("diagnostics = %#v, want nil", res.diagnostics)
		}
	case <-time.After(time.Second):
		t.Fatal("RefreshDiagnostics() did not return after empty versionless publish")
	}
}

func TestRefreshDiagnosticsIgnoresVersionlessPublishFromBeforeRefresh(t *testing.T) {
	server := newTestServer()
	path := writeTestFile(t, "cache.ts", "const value = 1;\n")
	uri := FileURI(path)

	server.handlePublishedDiagnostics(PublishDiagnosticsParams{
		URI: uri,
		Diagnostics: []Diagnostic{{
			Message: "old versionless",
		}},
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	type result struct {
		diagnostics []Diagnostic
		err         error
	}
	done := make(chan result, 1)
	go func() {
		diagnostics, err := server.RefreshDiagnostics(ctx, path)
		done <- result{diagnostics: diagnostics, err: err}
	}()

	waitForDocumentVersion(t, server, path, 1)

	select {
	case res := <-done:
		t.Fatalf("RefreshDiagnostics() returned after stale versionless snapshot: diagnostics=%#v err=%v", res.diagnostics, res.err)
	case <-time.After(50 * time.Millisecond):
	}

	server.handlePublishedDiagnostics(PublishDiagnosticsParams{
		URI: uri,
		Diagnostics: []Diagnostic{{
			Message: "new versionless",
		}},
	})

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("RefreshDiagnostics() error = %v", res.err)
		}
		if len(res.diagnostics) != 1 || res.diagnostics[0].Message != "new versionless" {
			t.Fatalf("diagnostics = %#v, want new versionless diagnostics", res.diagnostics)
		}
	case <-time.After(time.Second):
		t.Fatal("RefreshDiagnostics() did not return after fresh versionless publish")
	}
}

func TestRefreshDiagnosticsIgnoresVersionlessPublishForAnotherFile(t *testing.T) {
	server := newTestServer()
	path := writeTestFile(t, "cache.ts", "const value = 1;\n")
	uri := FileURI(path)
	otherPath := writeTestFile(t, "other.ts", "const other = 1;\n")
	otherURI := FileURI(otherPath)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	type result struct {
		diagnostics []Diagnostic
		err         error
	}
	done := make(chan result, 1)
	go func() {
		diagnostics, err := server.RefreshDiagnostics(ctx, path)
		done <- result{diagnostics: diagnostics, err: err}
	}()

	waitForDocumentVersion(t, server, path, 1)
	server.handlePublishedDiagnostics(PublishDiagnosticsParams{
		URI: otherURI,
		Diagnostics: []Diagnostic{{
			Message: "other file",
		}},
	})

	select {
	case res := <-done:
		t.Fatalf("RefreshDiagnostics() returned after other-file publish: diagnostics=%#v err=%v", res.diagnostics, res.err)
	case <-time.After(50 * time.Millisecond):
	}

	server.handlePublishedDiagnostics(PublishDiagnosticsParams{
		URI: uri,
		Diagnostics: []Diagnostic{{
			Message: "target file",
		}},
	})

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("RefreshDiagnostics() error = %v", res.err)
		}
		if len(res.diagnostics) != 1 || res.diagnostics[0].Message != "target file" {
			t.Fatalf("diagnostics = %#v, want target-file diagnostics", res.diagnostics)
		}
	case <-time.After(time.Second):
		t.Fatal("RefreshDiagnostics() did not return after target-file versionless publish")
	}
}

func TestRefreshDiagnosticsReturnsExistingTargetVersionSnapshotWithoutWaiting(t *testing.T) {
	server := newTestServer()
	path := writeTestFile(t, "cache.ts", "const value = 1;\n")
	uri := FileURI(path)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}

	server.mu.Lock()
	server.openFiles[uri] = openDocumentState{
		Version: 1,
		ModTime: info.ModTime().UnixNano(),
		Size:    info.Size(),
	}
	server.mu.Unlock()
	server.handlePublishedDiagnostics(PublishDiagnosticsParams{
		URI: uri,
		Version: func() *int {
			version := 1
			return &version
		}(),
		Diagnostics: []Diagnostic{{
			Message: "existing",
		}},
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	diagnostics, err := server.RefreshDiagnostics(ctx, path)
	if err != nil {
		t.Fatalf("RefreshDiagnostics() error = %v", err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Message != "existing" {
		t.Fatalf("diagnostics = %#v, want existing target-version snapshot", diagnostics)
	}
	if got, ok := server.DocumentVersion(path); !ok || got != 1 {
		t.Fatalf("DocumentVersion() = (%d, %t), want (1, true)", got, ok)
	}
}

func TestRefreshDiagnosticsReturnsNilWhenNoFreshDiagnosticsArriveBeforeTimeout(t *testing.T) {
	server := newTestServer()
	path := writeTestFile(t, "cache.ts", "const value = 1;\n")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	diagnostics, err := server.RefreshDiagnostics(ctx, path)
	if err != nil {
		t.Fatalf("RefreshDiagnostics() error = %v", err)
	}
	if diagnostics != nil {
		t.Fatalf("diagnostics = %#v, want nil", diagnostics)
	}
}
