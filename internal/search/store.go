package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/provider"
)

const cacheSchemaVersion = 3

type persistedFile struct {
	Version int        `json:"version"`
	Path    string     `json:"path"`
	File    cachedFile `json:"file"`
}

func (s *Service) resolveFileCache(ctx context.Context, workspaceRoot, path string, info os.FileInfo) (cachedFile, bool, error) {
	entry, ok := s.memoryFileCache(path, info)
	if ok {
		return entry, true, nil
	}
	entry, ok = s.loadPersistedFileCache(workspaceRoot, path, info)
	if ok {
		s.storeMemoryFileCache(path, entry)
		return entry, true, nil
	}
	return cachedFile{}, false, ctx.Err()
}

func (s *Service) ensureFileCache(ctx context.Context, workspaceRoot, path string, info os.FileInfo) (cachedFile, bool, error) {
	entry, ok, err := s.resolveFileCache(ctx, workspaceRoot, path, info)
	if err != nil || ok {
		return entry, false, err
	}
	entry, err = s.buildFileCache(ctx, workspaceRoot, path, info)
	if err != nil {
		return cachedFile{}, false, err
	}
	s.storeMemoryFileCache(path, entry)
	if err := s.persistFileCache(workspaceRoot, path, entry); err != nil {
		s.logPersistError(err)
	}
	return entry, true, nil
}

func (s *Service) cachedFileEntry(ctx context.Context, workspaceRoot, path string, info os.FileInfo) (cachedFile, bool, error) {
	return s.resolveFileCache(ctx, workspaceRoot, path, info)
}

func (s *Service) ensurePersistedFileCache(ctx context.Context, workspaceRoot, path string, info os.FileInfo) (cachedFile, bool, error) {
	if entry, ok := s.loadPersistedFileCache(workspaceRoot, path, info); ok {
		return entry, false, nil
	}
	if entry, ok := s.memoryFileCache(path, info); ok {
		if err := s.persistFileCache(workspaceRoot, path, entry); err != nil {
			return cachedFile{}, false, err
		}
		return entry, false, nil
	}
	if err := ctx.Err(); err != nil {
		return cachedFile{}, false, err
	}
	entry, err := s.buildFileCache(ctx, workspaceRoot, path, info)
	if err != nil {
		return cachedFile{}, false, err
	}
	if err := s.persistFileCache(workspaceRoot, path, entry); err != nil {
		return cachedFile{}, false, err
	}
	return entry, true, nil
}

func (s *Service) buildFileCache(ctx context.Context, workspaceRoot, path string, info os.FileInfo) (cachedFile, error) {
	startedAt := time.Now()
	data, err := os.ReadFile(path)
	if err != nil {
		return cachedFile{}, err
	}
	if bytes := strings.IndexByte(string(data), 0); bytes >= 0 {
		entry := cachedFile{ModTime: info.ModTime().UnixNano(), Size: info.Size()}
		s.logCacheBuild(workspaceRoot, path, info.Size(), 0, 0, startedAt)
		return entry, nil
	}

	chunks, texts := chunkFile(workspaceRoot, path, string(data))
	if len(texts) == 0 {
		entry := cachedFile{ModTime: info.ModTime().UnixNano(), Size: info.Size()}
		s.logCacheBuild(workspaceRoot, path, info.Size(), 0, 0, startedAt)
		return entry, nil
	}
	vectors, err := s.embedder.Embed(ctx, provider.EmbeddingRequest{
		Model:      s.model,
		Inputs:     texts,
		Dimensions: s.dimensions,
	})
	if err != nil {
		return cachedFile{}, err
	}
	for idx := range chunks {
		chunks[idx].Embedding = vectors[idx]
	}
	entry := cachedFile{
		ModTime: info.ModTime().UnixNano(),
		Size:    info.Size(),
		Chunks:  chunks,
	}
	s.logCacheBuild(workspaceRoot, path, info.Size(), len(chunks), searchTextBytes(texts), startedAt)
	return entry, nil
}

func (s *Service) loadPersistedFileCache(workspaceRoot, path string, info os.FileInfo) (cachedFile, bool) {
	persisted, ok := s.readPersistedFileCache(workspaceRoot, path)
	if !ok {
		return cachedFile{}, false
	}
	if persisted.File.ModTime != info.ModTime().UnixNano() || persisted.File.Size != info.Size() {
		return cachedFile{}, false
	}
	return persisted.File, true
}

func (s *Service) loadPersistedTrackedFileCache(workspaceRoot, path string, tracked trackedFile) (cachedFile, bool) {
	persisted, ok := s.readPersistedFileCache(workspaceRoot, path)
	if !ok {
		return cachedFile{}, false
	}
	if persisted.File.ModTime != tracked.ModTime || persisted.File.Size != tracked.Size {
		return cachedFile{}, false
	}
	return persisted.File, true
}

func (s *Service) readPersistedFileCache(workspaceRoot, path string) (persistedFile, bool) {
	cachePath, ok := s.cachePath(workspaceRoot, path)
	if !ok {
		return persistedFile{}, false
	}
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return persistedFile{}, false
	}
	var persisted persistedFile
	if json.Unmarshal(data, &persisted) != nil {
		return persistedFile{}, false
	}
	if persisted.Version != cacheSchemaVersion || persisted.Path != path {
		return persistedFile{}, false
	}
	return persisted, true
}

func (s *Service) persistFileCache(workspaceRoot, path string, entry cachedFile) error {
	cachePath, ok := s.cachePath(workspaceRoot, path)
	if !ok {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return err
	}

	data, err := json.Marshal(persistedFile{
		Version: cacheSchemaVersion,
		Path:    path,
		File:    entry,
	})
	if err != nil {
		return err
	}
	tempPath := cachePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tempPath, cachePath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func (s *Service) storeMemoryFileCache(path string, entry cachedFile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[path] = entry
}

func (s *Service) memoryFileCache(path string, info os.FileInfo) (cachedFile, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.files[path]
	if !ok || !cacheEntryMatchesFile(entry, info) {
		return cachedFile{}, false
	}
	return entry, true
}

func (s *Service) deletePersistedFileCache(workspaceRoot, path string) (bool, error) {
	cachePath, ok := s.cachePath(workspaceRoot, path)
	if !ok {
		return false, nil
	}
	err := os.Remove(cachePath)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	return false, err
}

func (s *Service) cachePath(workspaceRoot, path string) (string, bool) {
	base := strings.TrimSpace(s.indexDir)
	if base == "" {
		return "", false
	}
	rootKey := workspaceRoot
	if strings.TrimSpace(rootKey) == "" {
		rootKey = path
	}
	workspaceHash := hashValue(rootKey)
	fileHash := hashValue(path)
	return filepath.Join(base, workspaceHash, "files", fileHash+".json"), true
}

func hashValue(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:16])
}

func cacheEntryMatchesFile(entry cachedFile, info os.FileInfo) bool {
	return entry.ModTime == info.ModTime().UnixNano() && entry.Size == info.Size()
}

func cacheEntryMatchesTrackedFile(entry cachedFile, tracked trackedFile) bool {
	return entry.ModTime == tracked.ModTime && entry.Size == tracked.Size
}

func (s *Service) logPersistError(err error) {
	if s == nil || s.logger == nil || err == nil {
		return
	}
	s.logger.Debug("search cache persistence skipped", "error", err.Error())
}

func (s *Service) logCacheBuild(workspaceRoot, path string, fileBytes int64, chunkCount int, embedChars int, startedAt time.Time) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Debug("search cache rebuilt",
		"workspace_root", workspaceRoot,
		"path", path,
		"file_bytes", fileBytes,
		"chunks", chunkCount,
		"embed_chars", embedChars,
		"duration_ms", time.Since(startedAt).Milliseconds(),
	)
}

func searchTextBytes(texts []string) int {
	total := 0
	for _, text := range texts {
		total += len(text)
	}
	return total
}
