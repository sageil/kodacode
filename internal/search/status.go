package search

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/provider"
)

var ErrWarmupUnavailable = errors.New("search warmup requires configured embeddings and a persisted index")
var ErrWorkspaceRootRequired = errors.New("workspace root is required")

type WorkspaceIndexStatus struct {
	Configured        bool
	Tracking          bool
	WorkspaceRoot     string
	Model             provider.ModelRef
	PrewarmEmbeddings bool
	TrackedFiles      int
	IndexedFiles      int
	IndexedChunks     int
	PendingFiles      int
	LastRefreshAt     time.Time
	LastWarmupAt      time.Time
	LastWarmupError   string
}

type WarmupResult struct {
	WorkspaceRoot string
	Files         int
	BuiltFiles    int
	ReusedFiles   int
	SkippedFiles  int
	IndexedFiles  int
	IndexedChunks int
	CompletedAt   time.Time
}

func (s *Service) WorkspaceStatus(workspaceRoot string) WorkspaceIndexStatus {
	status := WorkspaceIndexStatus{}
	if s == nil {
		return status
	}

	root := strings.TrimSpace(workspaceRoot)
	status.Configured = s.HybridConfigured()
	status.WorkspaceRoot = root
	status.Model = s.model

	tracked, snapshot := s.workspaceTrackedFiles(root)
	status.Tracking = snapshot.ok
	status.PrewarmEmbeddings = snapshot.prewarmEmbeddings
	status.LastRefreshAt = snapshot.lastRefreshAt
	status.LastWarmupAt = snapshot.lastWarmupAt
	status.LastWarmupError = snapshot.lastWarmupError
	if len(tracked) == 0 && root != "" && status.Configured && strings.TrimSpace(s.indexDir) != "" {
		tracked = s.scanWorkspace(root)
	}
	status.TrackedFiles = len(tracked)
	status.IndexedFiles, status.IndexedChunks = s.indexedWorkspaceCounts(root, tracked)
	if status.TrackedFiles > status.IndexedFiles {
		status.PendingFiles = status.TrackedFiles - status.IndexedFiles
	}
	return status
}

func (s *Service) WarmWorkspace(ctx context.Context, workspaceRoot string) (WarmupResult, error) {
	root := strings.TrimSpace(workspaceRoot)
	if root == "" {
		return WarmupResult{}, ErrWorkspaceRootRequired
	}
	if s == nil || !s.HybridConfigured() || strings.TrimSpace(s.indexDir) == "" {
		return WarmupResult{WorkspaceRoot: root}, ErrWarmupUnavailable
	}
	if !s.beginWorkspaceWarmup(root) {
		return WarmupResult{WorkspaceRoot: root}, nil
	}
	finishedAt := time.Time{}
	var result WarmupResult
	var err error
	defer func() {
		if finishedAt.IsZero() {
			finishedAt = time.Now()
		}
		s.recordWorkspaceWarmup(root, finishedAt, err)
	}()
	result, err = s.warmWorkspace(ctx, root)
	finishedAt = result.CompletedAt
	return result, err
}

func (s *Service) warmWorkspace(ctx context.Context, workspaceRoot string) (WarmupResult, error) {
	root := strings.TrimSpace(workspaceRoot)
	tracked := s.scanWorkspace(root)
	startedAt := time.Now()
	s.recordWorkspaceRefresh(root, tracked, startedAt)
	result := WarmupResult{
		WorkspaceRoot: root,
		Files:         len(tracked),
	}
	var firstErr error
	for path := range tracked {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		info, err := os.Stat(path)
		if err != nil {
			result.SkippedFiles++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		entry, builtEntry, err := s.ensurePersistedFileCache(ctx, root, path, info)
		if err != nil {
			result.SkippedFiles++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if len(entry.Chunks) == 0 {
			result.SkippedFiles++
			continue
		}
		if builtEntry {
			result.BuiltFiles++
		} else {
			result.ReusedFiles++
		}
	}
	result.CompletedAt = time.Now()
	status := s.WorkspaceStatus(root)
	result.IndexedFiles = status.IndexedFiles
	result.IndexedChunks = status.IndexedChunks
	return result, firstErr
}

type workspaceTrackerSnapshot struct {
	ok                bool
	prewarmEmbeddings bool
	lastRefreshAt     time.Time
	lastWarmupAt      time.Time
	lastWarmupError   string
}

func (s *Service) workspaceTrackedFiles(workspaceRoot string) (map[string]trackedFile, workspaceTrackerSnapshot) {
	root := strings.TrimSpace(workspaceRoot)
	if s == nil || root == "" {
		return nil, workspaceTrackerSnapshot{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	tracker := s.trackers[root]
	if tracker == nil {
		return nil, workspaceTrackerSnapshot{}
	}
	return cloneTrackedFiles(tracker.files), workspaceTrackerSnapshot{
		ok:                true,
		prewarmEmbeddings: tracker.prewarmEmbeddings,
		lastRefreshAt:     tracker.lastRefreshAt,
		lastWarmupAt:      tracker.lastWarmupAt,
		lastWarmupError:   tracker.lastWarmupError,
	}
}

func cloneTrackedFiles(files map[string]trackedFile) map[string]trackedFile {
	if len(files) == 0 {
		return nil
	}
	out := make(map[string]trackedFile, len(files))
	for path, file := range files {
		out[path] = file
	}
	return out
}

func (s *Service) indexedWorkspaceCounts(workspaceRoot string, tracked map[string]trackedFile) (files int, chunks int) {
	if s == nil || len(tracked) == 0 {
		return 0, 0
	}
	for path, trackedFile := range tracked {
		if entry, ok := s.memoryTrackedFileCache(path, trackedFile); ok {
			files++
			chunks += len(entry.Chunks)
			continue
		}
		if entry, ok := s.loadPersistedTrackedFileCache(workspaceRoot, path, trackedFile); ok {
			files++
			chunks += len(entry.Chunks)
		}
	}
	return files, chunks
}

func (s *Service) memoryTrackedFileCache(path string, tracked trackedFile) (cachedFile, bool) {
	if s == nil {
		return cachedFile{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.files[path]
	if !ok || !cacheEntryMatchesTrackedFile(entry, tracked) {
		return cachedFile{}, false
	}
	return entry, true
}
