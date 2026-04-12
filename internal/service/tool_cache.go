package service

import (
	"encoding/json"
	"log"
	"strings"
	"sync"
)

type toolResultCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	byPath  map[string]map[string]bool // file path → set of cache keys
}

type cacheEntry struct {
	output string
	errStr *string
	path   string // primary file path from tool args, if any
}

func newToolResultCache() *toolResultCache {
	return &toolResultCache{
		entries: make(map[string]*cacheEntry),
		byPath:  make(map[string]map[string]bool),
	}
}

func (c *toolResultCache) key(toolName, args string) string {
	return toolName + "|" + canonicalizeArgs(args)
}

func (c *toolResultCache) lookup(toolName, args string) (output string, errStr *string, hit bool) {
	c.mu.RLock()
	entry, ok := c.entries[c.key(toolName, args)]
	c.mu.RUnlock()
	if !ok {
		return "", nil, false
	}
	log.Printf("tool_cache: hit for %s", toolName)
	return entry.output, entry.errStr, true
}

const maxToolCacheEntries = 256

func (c *toolResultCache) store(toolName, args, output string, errStr *string) {
	k := c.key(toolName, args)
	path := extractFilePath(args)

	c.mu.Lock()
	if len(c.entries) >= maxToolCacheEntries {
		n := 0
		for evictKey, evictEntry := range c.entries {
			c.removeIndex(evictKey, evictEntry.path)
			delete(c.entries, evictKey)
			n++
			if n >= maxToolCacheEntries/2 {
				break
			}
		}
		log.Printf("tool_cache: evicted %d entries (cap %d reached)", n, maxToolCacheEntries)
	}
	c.entries[k] = &cacheEntry{output: output, errStr: errStr, path: path}
	if path != "" {
		if c.byPath[path] == nil {
			c.byPath[path] = make(map[string]bool)
		}
		c.byPath[path][k] = true
	}
	c.mu.Unlock()
}

func (c *toolResultCache) invalidate() {
	c.mu.Lock()
	clear(c.entries)
	clear(c.byPath)
	c.mu.Unlock()
	log.Printf("tool_cache: invalidated all")
}

func extractSubagentName(argsJSON string) string {
	var args struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ""
	}
	return args.AgentID
}

// invalidateByPath removes all cache entries that reference the given file path.
func (c *toolResultCache) invalidateByPath(path string) {
	if path == "" {
		c.invalidate()
		return
	}
	c.mu.Lock()
	keys := c.byPath[path]
	removed := len(keys)
	for k := range keys {
		delete(c.entries, k)
	}
	delete(c.byPath, path)
	// Also invalidate entries whose path is a parent of the modified file
	// (e.g., a glob on /src should be invalidated when /src/foo.go changes).
	for indexedPath, pathKeys := range c.byPath {
		if strings.HasPrefix(path, indexedPath+"/") {
			for k := range pathKeys {
				delete(c.entries, k)
				removed++
			}
			delete(c.byPath, indexedPath)
		}
	}
	c.mu.Unlock()
	if removed > 0 {
		log.Printf("tool_cache: invalidated %d entries for path %q", removed, path)
	}
}

// removeIndex removes a cache key from the path index. Caller must hold c.mu.
func (c *toolResultCache) removeIndex(key, path string) {
	if path == "" {
		return
	}
	if keys, ok := c.byPath[path]; ok {
		delete(keys, key)
		if len(keys) == 0 {
			delete(c.byPath, path)
		}
	}
}
