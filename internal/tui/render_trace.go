package tui

import (
	"fmt"
	"hash/maphash"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const renderTraceShadowLRUSize = 32

var (
	activeRenderTraceLogger atomic.Pointer[renderTraceLogger]
	renderTraceHashSeed     = maphash.MakeSeed()
)

type renderTraceLogger struct {
	mu        sync.Mutex
	file      *os.File
	stats     map[string]*renderTraceStats
	loopStats map[string]*renderTraceLoopStats
	lastFlush time.Time
	interval  time.Duration
}

type renderTraceStats struct {
	lookups       uint64
	hits          uint64
	misses        uint64
	shadowHits    uint64
	keyChanges    uint64
	bytes         uint64
	maxBytes      int
	renderNanos   int64
	hasLastKey    bool
	lastKey       uint64
	recentKeys    []uint64
	recentKeySeen map[uint64]struct{}
}

type renderTraceLoopStats struct {
	count      uint64
	bytes      uint64
	maxBytes   int
	totalNanos int64
	maxNanos   int64
}

func openRenderTraceLogger(path string) (*renderTraceLogger, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	logger := &renderTraceLogger{
		file:      file,
		stats:     make(map[string]*renderTraceStats),
		loopStats: make(map[string]*renderTraceLoopStats),
		lastFlush: time.Now(),
		interval:  time.Second,
	}
	logger.logf("render_trace_start pid=%d shadow_lru_entries=%d", os.Getpid(), renderTraceShadowLRUSize)
	return logger, nil
}

func setActiveRenderTraceLogger(logger *renderTraceLogger) {
	activeRenderTraceLogger.Store(logger)
}

func closeActiveRenderTraceLogger(logger *renderTraceLogger) error {
	if logger == nil {
		return nil
	}
	setActiveRenderTraceLogger(nil)
	return logger.Close()
}

func renderTraceEnabled() bool {
	return activeRenderTraceLogger.Load() != nil
}

func (l *renderTraceLogger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	l.mu.Lock()
	l.flushLocked("render_trace_stop", time.Now())
	l.mu.Unlock()
	return l.file.Close()
}

func traceRenderCacheLookup(name string, key uint64, hit bool, bytes int, renderDuration time.Duration) {
	logger := activeRenderTraceLogger.Load()
	if logger == nil {
		return
	}
	logger.observe(name, key, hit, bytes, renderDuration)
}

func traceRenderCacheLookupStringKey(name, key string, hit bool, bytes int, renderDuration time.Duration) {
	logger := activeRenderTraceLogger.Load()
	if logger == nil {
		return
	}
	logger.observe(name, maphash.String(renderTraceHashSeed, key), hit, bytes, renderDuration)
}

func traceTUILoop(name string, msg any, duration time.Duration, bytes int) {
	logger := activeRenderTraceLogger.Load()
	if logger == nil {
		return
	}
	msgName := ""
	if msg != nil {
		msgName = fmt.Sprintf("%T", msg)
		msgName = strings.TrimPrefix(msgName, "tui.")
		msgName = strings.TrimPrefix(msgName, "*tui.")
	}
	logger.observeLoop(name, msgName, duration, bytes)
}

func (l *renderTraceLogger) observe(name string, key uint64, hit bool, bytes int, renderDuration time.Duration) {
	if l == nil || l.file == nil || strings.TrimSpace(name) == "" {
		return
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	stats := l.stats[name]
	if stats == nil {
		stats = &renderTraceStats{recentKeySeen: make(map[uint64]struct{})}
		l.stats[name] = stats
	}
	stats.lookups++
	if stats.hasLastKey && stats.lastKey != key {
		stats.keyChanges++
	}
	stats.hasLastKey = true
	stats.lastKey = key
	if hit {
		stats.hits++
	} else {
		stats.misses++
		if _, ok := stats.recentKeySeen[key]; ok {
			stats.shadowHits++
		}
	}
	if bytes > 0 {
		stats.bytes += uint64(bytes)
		stats.maxBytes = max(stats.maxBytes, bytes)
	}
	if renderDuration > 0 {
		stats.renderNanos += renderDuration.Nanoseconds()
	}
	stats.rememberKey(key)

	if now.Sub(l.lastFlush) >= l.interval {
		l.flushLocked("render_cache_interval", now)
	}
}

func (l *renderTraceLogger) observeLoop(name, msgName string, duration time.Duration, bytes int) {
	if l == nil || l.file == nil || strings.TrimSpace(name) == "" {
		return
	}
	now := time.Now()
	key := name
	if msgName != "" {
		key += " msg=" + msgName
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	stats := l.loopStats[key]
	if stats == nil {
		stats = &renderTraceLoopStats{}
		l.loopStats[key] = stats
	}
	stats.count++
	if bytes > 0 {
		stats.bytes += uint64(bytes)
		stats.maxBytes = max(stats.maxBytes, bytes)
	}
	if duration > 0 {
		nanos := duration.Nanoseconds()
		stats.totalNanos += nanos
		stats.maxNanos = max(stats.maxNanos, nanos)
	}

	if now.Sub(l.lastFlush) >= l.interval {
		l.flushLocked("render_trace_interval", now)
	}
}

func (s *renderTraceStats) rememberKey(key uint64) {
	if s == nil {
		return
	}
	for i, existing := range s.recentKeys {
		if existing == key {
			copy(s.recentKeys[1:i+1], s.recentKeys[:i])
			s.recentKeys[0] = key
			return
		}
	}
	s.recentKeys = append([]uint64{key}, s.recentKeys...)
	s.recentKeySeen[key] = struct{}{}
	if len(s.recentKeys) <= renderTraceShadowLRUSize {
		return
	}
	evicted := s.recentKeys[len(s.recentKeys)-1]
	s.recentKeys = s.recentKeys[:len(s.recentKeys)-1]
	delete(s.recentKeySeen, evicted)
}

func (l *renderTraceLogger) flushLocked(prefix string, now time.Time) {
	if l == nil || l.file == nil {
		return
	}
	l.flushCacheStatsLocked(prefix, now)
	l.flushLoopStatsLocked(prefix, now)
	l.lastFlush = now
}

func (l *renderTraceLogger) flushCacheStatsLocked(prefix string, now time.Time) {
	if l == nil || l.file == nil {
		return
	}
	for name, stats := range l.stats {
		if stats == nil || stats.lookups == 0 {
			continue
		}
		hitRate := 0.0
		if stats.lookups > 0 {
			hitRate = float64(stats.hits) / float64(stats.lookups)
		}
		shadowRate := 0.0
		if stats.misses > 0 {
			shadowRate = float64(stats.shadowHits) / float64(stats.misses)
		}
		_, _ = fmt.Fprintf(
			l.file,
			"%s %s name=%s lookups=%d hits=%d misses=%d hit_rate=%.3f shadow_lru_hits=%d shadow_lru_hit_rate=%.3f key_changes=%d bytes=%d max_bytes=%d render_ms=%.3f\n",
			now.Format(time.RFC3339Nano),
			prefix,
			name,
			stats.lookups,
			stats.hits,
			stats.misses,
			hitRate,
			stats.shadowHits,
			shadowRate,
			stats.keyChanges,
			stats.bytes,
			stats.maxBytes,
			float64(stats.renderNanos)/float64(time.Millisecond),
		)
		stats.lookups = 0
		stats.hits = 0
		stats.misses = 0
		stats.shadowHits = 0
		stats.keyChanges = 0
		stats.bytes = 0
		stats.maxBytes = 0
		stats.renderNanos = 0
	}
}

func (l *renderTraceLogger) flushLoopStatsLocked(prefix string, now time.Time) {
	if l == nil || l.file == nil {
		return
	}
	for name, stats := range l.loopStats {
		if stats == nil || stats.count == 0 {
			continue
		}
		avgMillis := 0.0
		if stats.count > 0 {
			avgMillis = float64(stats.totalNanos) / float64(time.Millisecond) / float64(stats.count)
		}
		_, _ = fmt.Fprintf(
			l.file,
			"%s %s name=%s count=%d total_ms=%.3f avg_ms=%.3f max_ms=%.3f bytes=%d max_bytes=%d\n",
			now.Format(time.RFC3339Nano),
			prefix,
			name,
			stats.count,
			float64(stats.totalNanos)/float64(time.Millisecond),
			avgMillis,
			float64(stats.maxNanos)/float64(time.Millisecond),
			stats.bytes,
			stats.maxBytes,
		)
		stats.count = 0
		stats.bytes = 0
		stats.maxBytes = 0
		stats.totalNanos = 0
		stats.maxNanos = 0
	}
}

func (l *renderTraceLogger) logf(format string, args ...any) {
	if l == nil || l.file == nil {
		return
	}
	_, _ = fmt.Fprintf(l.file, "%s %s\n", time.Now().Format(time.RFC3339Nano), fmt.Sprintf(format, args...))
}
