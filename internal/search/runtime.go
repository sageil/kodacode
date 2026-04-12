package search

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/fsnotify/fsnotify"
)

type RuntimeConfig struct {
	ProjectDir       string
	Indexer          *Indexer
	EmbeddingIndexer *EmbeddingIndexer
	IgnorePatterns   []string
	Debounce         time.Duration
	FullRescanEvery  time.Duration
}

type Runtime struct {
	projectDir       string
	indexer          *Indexer
	embeddingIndexer *EmbeddingIndexer
	ignorePatterns   []string
	debounce         time.Duration
	fullRescanEvery  time.Duration
}

func NewRuntime(cfg RuntimeConfig) *Runtime {
	debounce := cfg.Debounce
	if debounce <= 0 {
		debounce = 750 * time.Millisecond
	}
	rescan := cfg.FullRescanEvery
	return &Runtime{
		projectDir:       cfg.ProjectDir,
		indexer:          cfg.Indexer,
		embeddingIndexer: cfg.EmbeddingIndexer,
		ignorePatterns:   cfg.IgnorePatterns,
		debounce:         debounce,
		fullRescanEvery:  rescan,
	}
}

func (r *Runtime) Start(ctx context.Context) {
	if r == nil || r.indexer == nil {
		return
	}
	go r.run(ctx)
}

func (r *Runtime) run(ctx context.Context) {
	r.syncFull(ctx)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("search runtime: file watch disabled: %v", err)
		<-ctx.Done()
		return
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			log.Printf("search runtime: watcher close: %v", err)
		}
	}()

	if err := r.watchTree(watcher); err != nil {
		log.Printf("search runtime: watch tree: %v", err)
	}

	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()

	var (
		ticker *time.Ticker
		tickCh <-chan time.Time
	)
	if r.fullRescanEvery > 0 {
		ticker = time.NewTicker(r.fullRescanEvery)
		tickCh = ticker.C
		defer ticker.Stop()
	}

	pending := false
	pendingFull := false
	pendingPaths := make(map[string]struct{})
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if filepath.Base(event.Name) == ".DS_Store" {
				continue
			}
			eventIsDir := false
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() && r.shouldWatchDir(event.Name) {
					eventIsDir = true
					if err := r.addWatchTree(watcher, event.Name); err != nil {
						log.Printf("search runtime: add watch %s: %v", event.Name, err)
					}
				}
			}
			if r.shouldIgnorePath(event.Name) {
				continue
			}
			if r.shouldForceFullSync(watcher, event, eventIsDir) {
				pendingFull = true
			} else {
				pendingPaths[event.Name] = struct{}{}
			}
			pending = true
			timer.Reset(r.debounce)
		case <-timer.C:
			if pending {
				if pendingFull {
					r.syncFull(ctx)
				} else {
					r.syncPaths(ctx, pendingPaths)
				}
				pending = false
				pendingFull = false
				pendingPaths = make(map[string]struct{})
			}
		case <-tickCh:
			r.syncFull(ctx)
		case err, ok := <-watcher.Errors:
			if ok {
				log.Printf("search runtime: watcher error: %v", err)
			}
		}
	}
}

func (r *Runtime) syncFull(ctx context.Context) {
	if n, err := r.indexer.Index(ctx); err != nil {
		log.Printf("search index: %v", err)
	} else if n > 0 {
		log.Printf("search index: indexed %d files", n)
	}
	if r.embeddingIndexer != nil {
		if n, err := r.embeddingIndexer.Index(ctx); err != nil {
			log.Printf("embedding index: %v", err)
		} else if n > 0 {
			log.Printf("embedding index: embedded %d symbols", n)
		}
	}
}

func (r *Runtime) syncPaths(ctx context.Context, pendingPaths map[string]struct{}) {
	if len(pendingPaths) == 0 {
		return
	}
	paths := make([]string, 0, len(pendingPaths))
	for path := range pendingPaths {
		paths = append(paths, path)
	}
	n, err := r.indexer.SyncPaths(ctx, paths)
	if err != nil {
		log.Printf("search index: %v", err)
		return
	}
	if n > 0 {
		log.Printf("search index: indexed %d changed files", n)
	}
	if n > 0 && r.embeddingIndexer != nil {
		if embedded, err := r.embeddingIndexer.SyncPaths(ctx, paths); err != nil {
			log.Printf("embedding index: %v", err)
		} else if embedded > 0 {
			log.Printf("embedding index: embedded %d symbols", embedded)
		}
	}
}

func (r *Runtime) watchTree(watcher *fsnotify.Watcher) error {
	return r.addWatchTree(watcher, r.projectDir)
}

func (r *Runtime) addWatchTree(watcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if !r.shouldWatchDir(path) {
			return filepath.SkipDir
		}
		if err := watcher.Add(path); err != nil {
			log.Printf("search runtime: watch %s: %v", path, err)
		}
		return nil
	})
}

func (r *Runtime) shouldWatchDir(path string) bool {
	base := filepath.Base(path)
	switch base {
	case ".git", "node_modules", "vendor":
		return false
	}
	return !r.shouldIgnorePath(path)
}

func (r *Runtime) shouldIgnorePath(path string) bool {
	if r.projectDir == "" {
		return false
	}
	rel, err := filepath.Rel(r.projectDir, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return false
	}
	for _, pat := range r.ignorePatterns {
		if matched, _ := doublestar.Match(pat, rel); matched {
			return true
		}
		if strings.HasSuffix(pat, "/**") {
			prefix := strings.TrimSuffix(strings.TrimSuffix(pat, "/**"), "/")
			if prefix != "" && (rel == prefix || strings.HasPrefix(rel, prefix+"/")) {
				return true
			}
		}
	}
	return false
}

func (r *Runtime) shouldForceFullSync(watcher *fsnotify.Watcher, event fsnotify.Event, eventIsDir bool) bool {
	if eventIsDir {
		return true
	}
	if watcher == nil || event.Op&(fsnotify.Remove|fsnotify.Rename) == 0 {
		return false
	}
	for _, watched := range watcher.WatchList() {
		if watched == event.Name {
			return true
		}
	}
	return false
}
