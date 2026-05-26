package tui

import (
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func costDialogPromptCacheSupportLine(state events.SessionState, scoped bool) string {
	supports := costDialogPromptCacheSupports(state)
	if len(supports) == 0 {
		return ""
	}
	parts := make([]string, 0, len(supports))
	for _, support := range supports {
		parts = append(parts, costDialogPromptCacheSupportPart(support))
	}
	label := "Prompt cache support: "
	if scoped {
		label = "Current session prompt cache support: "
	}
	return label + strings.Join(parts, "; ")
}

func costDialogPromptCacheSupports(state events.SessionState) []provider.PromptCacheSupport {
	seen := map[string]bool{}
	out := make([]provider.PromptCacheSupport, 0, 3)
	visit := func(model string) {
		ref, err := provider.ParseModelRef(model)
		if err != nil {
			return
		}
		support := provider.PromptCacheSupportForModel(ref)
		key := strings.TrimSpace(support.ProviderID)
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, support)
	}
	for _, turnID := range state.TurnOrder {
		turn := state.Turns[turnID]
		if turn == nil || turn.ProviderUsage == nil {
			continue
		}
		visit(turn.ProviderUsage.Model)
		for _, attempt := range turn.ProviderAttempts {
			visit(attempt.Model)
			visit(attempt.ReportedModel)
		}
	}
	return out
}

func costDialogPromptCacheSupportPart(support provider.PromptCacheSupport) string {
	providerID := strings.TrimSpace(support.ProviderID)
	if providerID == "" {
		providerID = "provider"
	}
	features := make([]string, 0, 2)
	if support.RequestHintsSupported {
		features = append(features, "request hints")
	}
	switch {
	case support.CacheReadReportingSupported && support.CacheWriteReportingSupported:
		features = append(features, "cache-read/write reporting")
	case support.CacheReadReportingSupported:
		features = append(features, "cache-read reporting")
	case support.CacheWriteReportingSupported:
		features = append(features, "cache-write reporting")
	}
	if len(features) == 0 {
		return providerID + " unsupported (" + support.UnsupportedReason + ")"
	}
	part := providerID + " " + strings.Join(features, " + ")
	if strings.TrimSpace(support.UnsupportedReason) != "" {
		part += " only (" + support.UnsupportedReason + ")"
	}
	return part
}
