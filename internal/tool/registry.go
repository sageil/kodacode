package tool

import (
	"sort"
	"sync"
)

// Registry is a thread-safe store of Tools.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]*Tool
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]*Tool)}
}

// Register adds or replaces t in the registry.
func (r *Registry) Register(t *Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name] = t
}

// Get returns the Tool with the given name and whether it was found.
func (r *Registry) Get(name string) (*Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// All returns a snapshot of all registered tools sorted by name.
func (r *Registry) All() []*Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Names returns a sorted slice of all registered tool names.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// IsReadOnly returns true if the named tool has ReadOnly set.
// Returns false for unknown tools (conservative: assume they modify state).
func (r *Registry) IsReadOnly(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if t, ok := r.tools[name]; ok {
		return t.ReadOnly
	}
	return false
}

// ForAgent returns the subset of tools whose names appear in allowList.
// An empty allowList returns an empty slice (no tools permitted).
func (r *Registry) ForAgent(allowList []string) []*Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Tool, 0, len(allowList))
	for _, name := range allowList {
		if t, ok := r.tools[name]; ok {
			out = append(out, t)
		}
	}
	return out
}
