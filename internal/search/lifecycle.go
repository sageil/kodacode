package search

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type trackedFile struct {
	ModTime int64
	Size    int64
}

type TrackOptions struct {
	RefreshInterval   time.Duration
	PrewarmEmbeddings bool
}

type workspaceTracker struct {
	ctx               context.Context
	cancel            context.CancelFunc
	files             map[string]trackedFile
	interval          time.Duration
	prewarmEmbeddings bool
	warmupRunning     bool
	lastRefreshAt     time.Time
	lastWarmupAt      time.Time
	lastWarmupError   string
}

func (s *Service) TrackWorkspace(workspaceRoot string, options TrackOptions) {
	interval := options.RefreshInterval
	if s == nil || s.embedder == nil || strings.TrimSpace(s.indexDir) == "" || interval <= 0 {
		return
	}
	root := strings.TrimSpace(workspaceRoot)
	if root == "" {
		return
	}

	tracked := s.scanWorkspace(root)
	ctx, cancel := context.WithCancel(context.Background())

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		cancel()
		return
	}
	if existing := s.trackers[root]; existing != nil {
		s.mu.Unlock()
		cancel()
		return
	}
	s.trackers[root] = &workspaceTracker{
		ctx:               ctx,
		cancel:            cancel,
		files:             tracked,
		interval:          interval,
		prewarmEmbeddings: options.PrewarmEmbeddings,
		lastRefreshAt:     time.Now(),
	}
	s.wg.Add(1)
	s.mu.Unlock()

	go s.watchWorkspace(ctx, root, tracked, options.PrewarmEmbeddings)
}

func (s *Service) RefreshWorkspace(ctx context.Context, workspaceRoot string) bool {
	root := strings.TrimSpace(workspaceRoot)
	if s == nil || root == "" || !s.workspaceTracking(root) {
		return false
	}
	s.refreshWorkspace(ctx, root, "precompute")
	return true
}

func (s *Service) Close() error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	trackers := make([]*workspaceTracker, 0, len(s.trackers))
	for _, tracker := range s.trackers {
		trackers = append(trackers, tracker)
	}
	s.trackers = map[string]*workspaceTracker{}
	s.mu.Unlock()

	for _, tracker := range trackers {
		tracker.cancel()
	}
	s.wg.Wait()
	return nil
}

func (s *Service) watchWorkspace(ctx context.Context, workspaceRoot string, tracked map[string]trackedFile, prewarmEmbeddings bool) {
	defer s.wg.Done()

	if prewarmEmbeddings {
		s.prewarmWorkspace(ctx, workspaceRoot, trackedPaths(tracked), "initial")
	}

	ticker := time.NewTicker(s.workspaceInterval(workspaceRoot))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshWorkspace(ctx, workspaceRoot, "refresh")
		}
	}
}

func (s *Service) workspaceInterval(workspaceRoot string) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tracker := s.trackers[workspaceRoot]; tracker != nil && tracker.interval > 0 {
		return tracker.interval
	}
	return time.Second
}

func (s *Service) refreshWorkspace(ctx context.Context, workspaceRoot, reason string) {
	current := s.scanWorkspace(workspaceRoot)

	s.mu.Lock()
	tracker := s.trackers[workspaceRoot]
	if tracker == nil {
		s.mu.Unlock()
		return
	}
	changed, added, deleted := diffTrackedFiles(tracker.files, current)
	prewarmEmbeddings := tracker.prewarmEmbeddings
	tracker.files = current
	tracker.lastRefreshAt = time.Now()
	s.mu.Unlock()

	for _, path := range changed {
		s.invalidateCachedPath(workspaceRoot, path, "changed")
	}
	for _, path := range deleted {
		s.invalidateCachedPath(workspaceRoot, path, "deleted")
	}
	if prewarmEmbeddings {
		paths := append(append([]string(nil), changed...), added...)
		s.prewarmWorkspace(ctx, workspaceRoot, paths, reason)
	}
}

func (s *Service) workspaceTracking(workspaceRoot string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.trackers[strings.TrimSpace(workspaceRoot)] != nil
}

func (s *Service) scanWorkspace(workspaceRoot string) map[string]trackedFile {
	files := map[string]trackedFile{}
	ignores := newGitignoreMatcher(workspaceRoot)
	_ = filepath.WalkDir(workspaceRoot, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if s.shouldSkipTrackedPath(workspaceRoot, current, entry) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if current != workspaceRoot && ignores.ignored(current, true) {
				return filepath.SkipDir
			}
			_ = ignores.loadDir(current)
			return nil
		}
		if ignores.ignored(current, false) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		files[current] = trackedFile{
			ModTime: info.ModTime().UnixNano(),
			Size:    info.Size(),
		}
		return nil
	})
	return files
}

func (s *Service) shouldSkipTrackedPath(workspaceRoot, current string, entry os.DirEntry) bool {
	if entry.IsDir() && s.shouldSkipDir(entry.Name()) {
		return true
	}
	base := strings.TrimSpace(s.indexDir)
	if base == "" {
		return false
	}
	root := filepath.Clean(workspaceRoot)
	indexDir := filepath.Clean(base)
	current = filepath.Clean(current)
	if indexDir == root || !strings.HasPrefix(indexDir, root+string(filepath.Separator)) {
		return false
	}
	if current == indexDir {
		return true
	}
	return strings.HasPrefix(current, indexDir+string(filepath.Separator))
}

func diffTrackedFiles(previous, current map[string]trackedFile) (changed []string, added []string, deleted []string) {
	for path, before := range previous {
		after, ok := current[path]
		if !ok {
			deleted = append(deleted, path)
			continue
		}
		if before.ModTime != after.ModTime || before.Size != after.Size {
			changed = append(changed, path)
		}
	}
	for path := range current {
		if _, ok := previous[path]; !ok {
			added = append(added, path)
		}
	}
	return changed, added, deleted
}

func (s *Service) invalidateCachedPath(workspaceRoot, path, reason string) {
	s.mu.Lock()
	_, hadMemory := s.files[path]
	delete(s.files, path)
	s.mu.Unlock()

	hadPersisted, err := s.deletePersistedFileCache(workspaceRoot, path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		s.logPersistError(err)
		return
	}
	if hadMemory || hadPersisted {
		s.logLifecycle("search cache invalidated", "workspace_root", workspaceRoot, "path", path, "reason", reason)
	}
}

func (s *Service) logLifecycle(msg string, args ...any) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Debug(msg, args...)
}

func (s *Service) prewarmWorkspace(ctx context.Context, workspaceRoot string, paths []string, reason string) {
	if s == nil || s.embedder == nil || len(paths) == 0 {
		return
	}
	if !s.beginWorkspaceWarmup(workspaceRoot) {
		return
	}
	built := 0
	var firstErr error
	finishedAt := time.Time{}
	defer func() {
		if finishedAt.IsZero() {
			finishedAt = time.Now()
		}
		s.recordWorkspaceWarmup(workspaceRoot, finishedAt, firstErr)
	}()
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			firstErr = err
			return
		}
		info, err := os.Stat(path)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		_, builtEntry, err := s.ensurePersistedFileCache(ctx, workspaceRoot, path, info)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			s.logLifecycle("search cache prewarm skipped", "workspace_root", workspaceRoot, "path", path, "reason", reason, "error", err.Error())
			continue
		}
		if builtEntry {
			built++
		}
	}
	if built > 0 {
		s.logLifecycle("search cache prewarm completed", "workspace_root", workspaceRoot, "reason", reason, "files", built)
	}
	finishedAt = time.Now()
}

func trackedPaths(files map[string]trackedFile) []string {
	if len(files) == 0 {
		return nil
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	return paths
}

func (s *Service) recordWorkspaceRefresh(workspaceRoot string, files map[string]trackedFile, refreshedAt time.Time) {
	if s == nil {
		return
	}
	root := strings.TrimSpace(workspaceRoot)
	if root == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if tracker := s.trackers[root]; tracker != nil {
		tracker.files = cloneTrackedFiles(files)
		tracker.lastRefreshAt = refreshedAt
	}
}

func (s *Service) recordWorkspaceWarmup(workspaceRoot string, finishedAt time.Time, err error) {
	if s == nil {
		return
	}
	root := strings.TrimSpace(workspaceRoot)
	if root == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if tracker := s.trackers[root]; tracker != nil {
		tracker.warmupRunning = false
		tracker.lastWarmupAt = finishedAt
		if err != nil {
			tracker.lastWarmupError = strings.TrimSpace(err.Error())
			return
		}
		tracker.lastWarmupError = ""
	}
}

func (s *Service) beginWorkspaceWarmup(workspaceRoot string) bool {
	if s == nil {
		return false
	}
	root := strings.TrimSpace(workspaceRoot)
	if root == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tracker := s.trackers[root]
	if tracker == nil {
		return true
	}
	if tracker.warmupRunning {
		return false
	}
	tracker.warmupRunning = true
	return true
}

func (s *Service) workspaceWarmupContext(workspaceRoot string) (context.Context, bool) {
	if s == nil {
		return nil, false
	}
	root := strings.TrimSpace(workspaceRoot)
	if root == "" {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tracker := s.trackers[root]
	if tracker == nil || tracker.ctx == nil {
		return nil, false
	}
	return tracker.ctx, true
}

func (s *Service) scheduleAutoWarmWorkspace(workspaceRoot, reason string) bool {
	root := strings.TrimSpace(workspaceRoot)
	if s == nil || root == "" || !s.HybridConfigured() || strings.TrimSpace(s.indexDir) == "" {
		return false
	}
	ctx, ok := s.workspaceWarmupContext(root)
	if !ok {
		return false
	}
	if !s.beginWorkspaceWarmup(root) {
		return false
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		result, err := s.warmWorkspace(ctx, root)
		finishedAt := result.CompletedAt
		if finishedAt.IsZero() {
			finishedAt = time.Now()
		}
		s.recordWorkspaceWarmup(root, finishedAt, err)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			s.logLifecycle("search cache auto-warm failed", "workspace_root", root, "reason", reason, "error", err.Error())
			return
		}
		s.logLifecycle("search cache auto-warm completed", "workspace_root", root, "reason", reason, "files", result.IndexedFiles, "chunks", result.IndexedChunks)
	}()
	return true
}
