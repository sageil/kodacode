package lsp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func (s *Server) EnsureOpen(ctx context.Context, filePath string) error {
	_, err := s.syncDocument(ctx, filePath, false)
	return err
}

func (s *Server) syncDocument(ctx context.Context, filePath string, forceChange bool) (int, error) {
	uri := FileURI(filePath)
	info, statErr := os.Stat(filePath)

	s.mu.Lock()
	state, ok := s.openFiles[uri]
	if ok && !forceChange && statErr == nil && state.ModTime == info.ModTime().UnixNano() && state.Size == info.Size() {
		s.mu.Unlock()
		return state.Version, nil
	}
	s.mu.Unlock()

	if statErr != nil {
		if ok {
			return 0, fmt.Errorf("stat file for didChange: %w", statErr)
		}
		return 0, fmt.Errorf("stat file for didOpen: %w", statErr)
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		if ok {
			return 0, fmt.Errorf("read file for didChange: %w", err)
		}
		return 0, fmt.Errorf("read file for didOpen: %w", err)
	}
	if !ok {
		params := DidOpenTextDocumentParams{
			TextDocument: TextDocumentItem{
				URI:        uri,
				LanguageID: LanguageID(filepath.Ext(filePath)),
				Version:    1,
				Text:       string(content),
			},
		}
		if err := s.client.Notify("textDocument/didOpen", params); err != nil {
			return 0, fmt.Errorf("didOpen: %w", err)
		}

		s.mu.Lock()
		s.openFiles[uri] = openDocumentState{
			Version: 1,
			ModTime: info.ModTime().UnixNano(),
			Size:    info.Size(),
		}
		s.mu.Unlock()
		s.diagMu.Lock()
		delete(s.diagnostics, uri)
		s.diagMu.Unlock()
		return 1, nil
	}
	version := state.Version + 1
	params := DidChangeTextDocumentParams{
		TextDocument: VersionedTextDocumentIdentifier{
			URI:     uri,
			Version: version,
		},
		ContentChanges: []TextDocumentContentChangeEvent{{Text: string(content)}},
	}
	s.mu.Lock()
	s.openFiles[uri] = openDocumentState{
		Version: version,
		ModTime: info.ModTime().UnixNano(),
		Size:    info.Size(),
	}
	s.mu.Unlock()
	if err := s.client.Notify("textDocument/didChange", params); err != nil {
		return 0, fmt.Errorf("didChange: %w", err)
	}
	s.diagMu.Lock()
	delete(s.diagnostics, uri)
	s.diagMu.Unlock()
	return version, nil
}

func (s *Server) NotifyChanged(ctx context.Context, filePath string) error {
	_, err := s.syncDocument(ctx, filePath, true)
	return err
}

func (s *Server) CloseDocument(filePath string) error {
	uri := FileURI(filePath)

	s.mu.Lock()
	_, isOpen := s.openFiles[uri]
	s.mu.Unlock()
	if !isOpen {
		return nil
	}

	params := DidCloseTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	}
	if err := s.client.Notify("textDocument/didClose", params); err != nil {
		return fmt.Errorf("didClose: %w", err)
	}
	s.mu.Lock()
	delete(s.openFiles, uri)
	s.mu.Unlock()
	s.diagMu.Lock()
	delete(s.diagnostics, uri)
	delete(s.diagSeqs, uri)
	s.diagMu.Unlock()
	return nil
}

func (s *Server) DocumentVersion(filePath string) (int, bool) {
	uri := FileURI(filePath)
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.openFiles[uri]
	return state.Version, ok
}
