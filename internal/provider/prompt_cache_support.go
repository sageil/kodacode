package provider

import "strings"

type PromptCacheSupport struct {
	ProviderID                   string
	RequestHintsSupported        bool
	CacheReadReportingSupported  bool
	CacheWriteReportingSupported bool
	UnsupportedReason            string
}

func PromptCacheSupportForModel(ref ModelRef) PromptCacheSupport {
	providerID := CanonicalProviderID(ref.ProviderID)
	support := PromptCacheSupport{ProviderID: providerID}
	switch providerID {
	case "openai":
		support.RequestHintsSupported = true
		support.CacheReadReportingSupported = true
	case "anthropic":
		support.RequestHintsSupported = true
		support.CacheReadReportingSupported = true
		support.CacheWriteReportingSupported = true
	case "google":
		support.CacheReadReportingSupported = true
		support.UnsupportedReason = "google cached-content request hints are not wired in kodacode"
	default:
		if strings.TrimSpace(providerID) == "" {
			support.UnsupportedReason = "provider_id is empty"
		} else {
			support.UnsupportedReason = providerID + " has no prompt-cache integration in kodacode"
		}
	}
	return support
}
