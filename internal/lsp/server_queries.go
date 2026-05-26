package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

const (
	diagnosticsPullCallTimeout  = 1500 * time.Millisecond
	diagnosticsPushSettleWindow = 1 * time.Second
)

func (s *Server) Definition(ctx context.Context, filePath string, line, character int) ([]Location, error) {
	if err := s.EnsureOpen(ctx, filePath); err != nil {
		return nil, err
	}
	params := TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: FileURI(filePath)},
		Position:     Position{Line: line, Character: character},
	}

	var raw json.RawMessage
	if err := s.client.Call(ctx, "textDocument/definition", params, &raw); err != nil {
		return nil, err
	}

	var single Location
	if json.Unmarshal(raw, &single) == nil && single.URI != "" {
		return []Location{single}, nil
	}
	var multi []Location
	if json.Unmarshal(raw, &multi) == nil {
		return multi, nil
	}
	var links []struct {
		TargetURI   string `json:"targetUri"`
		TargetRange Range  `json:"targetRange"`
	}
	if json.Unmarshal(raw, &links) == nil {
		for _, link := range links {
			multi = append(multi, Location{URI: link.TargetURI, Range: link.TargetRange})
		}
		return multi, nil
	}
	return nil, nil
}

var ErrCallHierarchyUnsupported = errors.New("lsp call hierarchy is not supported")
var ErrReferencesUnsupported = errors.New("lsp references are not supported")
var ErrDocumentHighlightUnsupported = errors.New("lsp document highlight is not supported")

func (s *Server) PrepareCallHierarchy(ctx context.Context, filePath string, line, character int) ([]CallHierarchyItem, error) {
	if err := s.EnsureOpen(ctx, filePath); err != nil {
		return nil, err
	}
	params := CallHierarchyPrepareParams{
		TextDocument: TextDocumentIdentifier{URI: FileURI(filePath)},
		Position:     Position{Line: line, Character: character},
	}

	var raw json.RawMessage
	if err := s.client.Call(ctx, "textDocument/prepareCallHierarchy", params, &raw); err != nil {
		if isJSONRPCMethodNotFound(err) {
			return nil, ErrCallHierarchyUnsupported
		}
		return nil, err
	}

	var single CallHierarchyItem
	if json.Unmarshal(raw, &single) == nil && single.URI != "" {
		return []CallHierarchyItem{single}, nil
	}
	var multi []CallHierarchyItem
	if json.Unmarshal(raw, &multi) == nil {
		return multi, nil
	}
	return nil, nil
}

func (s *Server) IncomingCalls(ctx context.Context, item CallHierarchyItem) ([]CallHierarchyIncomingCall, error) {
	var calls []CallHierarchyIncomingCall
	if err := s.client.Call(ctx, "callHierarchy/incomingCalls", CallHierarchyIncomingCallsParams{Item: item}, &calls); err != nil {
		if isJSONRPCMethodNotFound(err) {
			return nil, ErrCallHierarchyUnsupported
		}
		return nil, err
	}
	return calls, nil
}

func (s *Server) OutgoingCalls(ctx context.Context, item CallHierarchyItem) ([]CallHierarchyOutgoingCall, error) {
	var calls []CallHierarchyOutgoingCall
	if err := s.client.Call(ctx, "callHierarchy/outgoingCalls", CallHierarchyOutgoingCallsParams{Item: item}, &calls); err != nil {
		if isJSONRPCMethodNotFound(err) {
			return nil, ErrCallHierarchyUnsupported
		}
		return nil, err
	}
	return calls, nil
}

func (s *Server) References(ctx context.Context, filePath string, line, character int, includeDeclaration bool) ([]Location, error) {
	if err := s.EnsureOpen(ctx, filePath); err != nil {
		return nil, err
	}
	params := ReferenceParams{
		TextDocument: TextDocumentIdentifier{URI: FileURI(filePath)},
		Position:     Position{Line: line, Character: character},
		Context:      ReferenceContext{IncludeDeclaration: includeDeclaration},
	}

	var locations []Location
	if err := s.client.Call(ctx, "textDocument/references", params, &locations); err != nil {
		if isJSONRPCMethodNotFound(err) {
			return nil, ErrReferencesUnsupported
		}
		return nil, err
	}
	return locations, nil
}

func (s *Server) DocumentHighlights(ctx context.Context, filePath string, line, character int) ([]DocumentHighlight, error) {
	if err := s.EnsureOpen(ctx, filePath); err != nil {
		return nil, err
	}
	params := TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: FileURI(filePath)},
		Position:     Position{Line: line, Character: character},
	}

	var highlights []DocumentHighlight
	if err := s.client.Call(ctx, "textDocument/documentHighlight", params, &highlights); err != nil {
		if isJSONRPCMethodNotFound(err) {
			return nil, ErrDocumentHighlightUnsupported
		}
		return nil, err
	}
	return highlights, nil
}

func (s *Server) Rename(ctx context.Context, filePath string, line, character int, newName string) (*WorkspaceEdit, error) {
	if err := s.EnsureOpen(ctx, filePath); err != nil {
		return nil, err
	}
	params := RenameParams{
		TextDocument: TextDocumentIdentifier{URI: FileURI(filePath)},
		Position:     Position{Line: line, Character: character},
		NewName:      newName,
	}

	var edit WorkspaceEdit
	if err := s.client.Call(ctx, "textDocument/rename", params, &edit); err != nil {
		return nil, err
	}
	return &edit, nil
}

func (s *Server) CodeActions(ctx context.Context, filePath string, rng Range, only []string) ([]CodeAction, error) {
	if err := s.EnsureOpen(ctx, filePath); err != nil {
		return nil, err
	}
	params := buildCodeActionParams(filePath, rng, s.Diagnostics(filePath), only)

	var actions []CodeAction
	if err := s.client.Call(ctx, "textDocument/codeAction", params, &actions); err != nil {
		return nil, err
	}
	return actions, nil
}

func buildCodeActionParams(filePath string, rng Range, diagnostics []Diagnostic, only []string) CodeActionParams {
	normalizedDiagnostics := make([]Diagnostic, len(diagnostics))
	copy(normalizedDiagnostics, diagnostics)
	normalizedOnly := append([]string(nil), only...)
	return CodeActionParams{
		TextDocument: TextDocumentIdentifier{URI: FileURI(filePath)},
		Range:        rng,
		Context: CodeActionContext{
			Diagnostics: normalizedDiagnostics,
			Only:        normalizedOnly,
		},
	}
}

func (s *Server) Diagnostics(filePath string) []Diagnostic {
	uri := FileURI(filePath)
	s.diagMu.RLock()
	defer s.diagMu.RUnlock()
	return cloneDiagnostics(s.diagnostics[uri].Diagnostics)
}

func (s *Server) RefreshDiagnostics(ctx context.Context, filePath string) ([]Diagnostic, error) {
	baseline := s.diagnosticsPublishSeq(filePath)
	targetVersion, err := s.syncDocument(ctx, filePath, false)
	if err != nil {
		return nil, err
	}
	if diagnostics, ok := s.currentDiagnosticsSnapshot(filePath, targetVersion, baseline); ok {
		return diagnostics, nil
	}
	diagnostics, pulled, err := s.pullDiagnostics(ctx, filePath, targetVersion)
	if err != nil {
		return nil, err
	}
	if pulled {
		return diagnostics, nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, diagnosticsPushSettleWindow)
	defer cancel()
	return s.waitForFreshDiagnostics(waitCtx, filePath, targetVersion, baseline)
}

func (s *Server) currentDiagnosticsSnapshot(filePath string, targetVersion int, baselinePublishSeq int64) ([]Diagnostic, bool) {
	uri := FileURI(filePath)
	s.diagMu.RLock()
	snapshot := s.diagnostics[uri]
	s.diagMu.RUnlock()
	switch {
	case snapshot.HasVersion && snapshot.Version == targetVersion:
		return cloneDiagnostics(snapshot.Diagnostics), true
	case !snapshot.HasVersion && snapshot.PublishSeq > baselinePublishSeq:
		return cloneDiagnostics(snapshot.Diagnostics), true
	default:
		return nil, false
	}
}

func (s *Server) pullDiagnostics(ctx context.Context, filePath string, targetVersion int) ([]Diagnostic, bool, error) {
	if s.pullDiagnosticsState() == diagnosticsPullUnsupported {
		return nil, false, nil
	}

	var report DocumentDiagnosticReport
	callCtx, cancel := context.WithTimeout(ctx, diagnosticsPullCallTimeout)
	defer cancel()
	err := s.client.Call(callCtx, "textDocument/diagnostic", DocumentDiagnosticParams{
		TextDocument: TextDocumentIdentifier{URI: FileURI(filePath)},
	}, &report)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			s.setPullDiagnosticsState(diagnosticsPullUnsupported)
			return nil, false, nil
		}
		if isJSONRPCMethodNotFound(err) {
			s.setPullDiagnosticsState(diagnosticsPullUnsupported)
			return nil, false, nil
		}
		return nil, false, err
	}

	s.setPullDiagnosticsState(diagnosticsPullSupported)
	s.storeDiagnosticsSnapshot(filePath, diagnosticSnapshot{
		Diagnostics: cloneDiagnostics(report.Items),
		Version:     targetVersion,
		HasVersion:  true,
	})
	return cloneDiagnostics(report.Items), true, nil
}

func (s *Server) waitForFreshDiagnostics(ctx context.Context, filePath string, targetVersion int, baselinePublishSeq int64) ([]Diagnostic, error) {
	uri := FileURI(filePath)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			s.diagCond.Broadcast()
		case <-done:
		}
	}()

	s.diagMu.RLock()
	defer s.diagMu.RUnlock()
	for {
		snapshot := s.diagnostics[uri]
		if snapshot.HasVersion && snapshot.Version == targetVersion {
			return cloneDiagnostics(snapshot.Diagnostics), nil
		}
		if !snapshot.HasVersion && snapshot.PublishSeq > baselinePublishSeq {
			return cloneDiagnostics(snapshot.Diagnostics), nil
		}
		if ctx.Err() != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, nil
			}
			return nil, ctx.Err()
		}
		s.diagCond.Wait()
	}
}

func (s *Server) diagnosticsPublishSeq(filePath string) int64 {
	uri := FileURI(filePath)
	s.diagMu.RLock()
	defer s.diagMu.RUnlock()
	return s.diagSeqs[uri]
}

func (s *Server) storeDiagnosticsSnapshot(filePath string, snapshot diagnosticSnapshot) {
	uri := FileURI(filePath)
	s.diagMu.Lock()
	nextSeq := s.diagSeqs[uri] + 1
	s.diagSeqs[uri] = nextSeq
	snapshot.PublishSeq = nextSeq
	s.diagnostics[uri] = snapshot
	s.diagMu.Unlock()
	s.diagCond.Broadcast()
}

func (s *Server) pullDiagnosticsState() diagnosticsPullSupport {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pullDiagnosticsSupport
}

func (s *Server) setPullDiagnosticsState(state diagnosticsPullSupport) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pullDiagnosticsSupport = state
}

func (s *Server) WorkspaceSymbol(ctx context.Context, query string) ([]SymbolInformation, error) {
	var symbols []SymbolInformation
	if err := s.client.Call(ctx, "workspace/symbol", WorkspaceSymbolParams{Query: query}, &symbols); err != nil {
		return nil, err
	}
	return symbols, nil
}

func cloneDiagnostics(src []Diagnostic) []Diagnostic {
	if len(src) == 0 {
		return nil
	}
	return append([]Diagnostic{}, src...)
}
