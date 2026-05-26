package provider

import (
	"strings"
	"sync"
)

var (
	providerAliasesMu sync.RWMutex
	providerAliases   = map[string]string{}
)

// Experiemental Delete before Release
// RegisterProviderAlias lets local experimental providers declare that they
// should inherit the canonical behavior of a shipped provider family.
func RegisterProviderAlias(providerID, canonical string) {
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	canonical = strings.ToLower(strings.TrimSpace(canonical))
	if providerID == "" || canonical == "" {
		return
	}
	providerAliasesMu.Lock()
	defer providerAliasesMu.Unlock()
	providerAliases[providerID] = canonical
}

func CanonicalProviderID(providerID string) string {
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	if providerID == "" {
		return ""
	}
	switch providerID {
	case "openai-codex":
		return "openai"
	}
	providerAliasesMu.RLock()
	canonical, ok := providerAliases[providerID]
	providerAliasesMu.RUnlock()
	if ok && canonical != "" {
		return canonical
	}
	return providerID
}
