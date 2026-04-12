package service

import (
	"strings"
	"sync"
	"time"

	"github.com/sageil/kodacode/v1/internal/provider"
)

const utilityUnavailableCooldown = 15 * time.Minute

type utilityHealthTracker struct {
	mu          sync.Mutex
	unavailable map[string]time.Time
}

func newUtilityHealthTracker() *utilityHealthTracker {
	return &utilityHealthTracker{
		unavailable: make(map[string]time.Time),
	}
}

func (t *utilityHealthTracker) prioritize(candidates []utilityProvider) []utilityProvider {
	if t == nil || len(candidates) < 2 {
		return candidates
	}

	now := time.Now()
	healthy := make([]utilityProvider, 0, len(candidates))
	unhealthy := make([]utilityProvider, 0, len(candidates))
	for _, candidate := range candidates {
		if !t.isUnavailableAt(candidate, now) {
			healthy = append(healthy, candidate)
			continue
		}
		unhealthy = append(unhealthy, candidate)
	}
	return append(healthy, unhealthy...)
}

func (t *utilityHealthTracker) markUnavailable(candidate utilityProvider) {
	if t == nil || candidate.prov == nil || candidate.modelID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.unavailable[utilityCandidateKey(candidate.prov.ID(), candidate.modelID)] = time.Now().Add(utilityUnavailableCooldown)
}

func (t *utilityHealthTracker) markAvailable(candidate utilityProvider) {
	if t == nil || candidate.prov == nil || candidate.modelID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.unavailable, utilityCandidateKey(candidate.prov.ID(), candidate.modelID))
}

func (t *utilityHealthTracker) isUnavailableAt(candidate utilityProvider, now time.Time) bool {
	if t == nil || candidate.prov == nil || candidate.modelID == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	key := utilityCandidateKey(candidate.prov.ID(), candidate.modelID)
	until, ok := t.unavailable[key]
	if !ok {
		return false
	}
	if !until.After(now) {
		delete(t.unavailable, key)
		return false
	}
	return true
}

func utilityCandidateKey(providerID, modelID string) string {
	return providerID + "/" + modelID
}

func utilityCandidates(primary utilityProvider) []utilityProvider {
	if primary.prov == nil || primary.modelID == "" {
		return nil
	}
	out := make([]utilityProvider, 0, 1+len(primary.alternates))
	out = append(out, primary.withoutAlternates())
	for _, alt := range primary.alternates {
		if alt.prov == nil || alt.modelID == "" {
			continue
		}
		out = append(out, alt.withoutAlternates())
	}
	return out
}

func (u utilityProvider) withoutAlternates() utilityProvider {
	u.alternates = nil
	return u
}

func isUtilityPermanentUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(cleanErrorMessage(err.Error()))
	if msg == "" || provider.IsContextOverflow("", 0, msg) || isRetryableError(msg) {
		return false
	}
	for _, token := range []string{
		"404",
		"403",
		"401",
		"model not found",
		"not found",
		"does not exist",
		"unknown model",
		"unsupported model",
		"not supported",
		"not available",
		"unavailable",
		"no endpoints found",
		"forbidden",
		"access denied",
		"permission denied",
		"insufficient permissions",
		"not enabled",
		"not allowed",
	} {
		if strings.Contains(msg, token) {
			return true
		}
	}
	return false
}
