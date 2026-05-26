package app

import (
	"strings"

	"github.com/sageil/kodacode/internal/provider"
)

const (
	deterministicContextPacketStartTag = "<deterministic_context_packet>"
	deterministicContextPacketEndTag   = "</deterministic_context_packet>"
)

func omitDeterministicContextPacketFromProviderRequest(request provider.Request) (provider.Request, bool) {
	var omitted bool
	request.Instructions, omitted = omitDeterministicContextPacketFromText(request.Instructions)
	request.CacheablePrefix, omitted = omitDeterministicContextPacketFromTextWithExisting(request.CacheablePrefix, omitted)
	request.DynamicSuffix, omitted = omitDeterministicContextPacketFromTextWithExisting(request.DynamicSuffix, omitted)
	return request, omitted
}

func omitDeterministicContextPacketFromTextWithExisting(text string, existing bool) (string, bool) {
	updated, omitted := omitDeterministicContextPacketFromText(text)
	return updated, existing || omitted
}

func omitDeterministicContextPacketFromText(text string) (string, bool) {
	var omitted bool
	for {
		start := strings.Index(text, deterministicContextPacketStartTag)
		if start < 0 {
			return strings.TrimSpace(text), omitted
		}
		end := strings.Index(text[start:], deterministicContextPacketEndTag)
		if end < 0 {
			return strings.TrimSpace(text), omitted
		}
		end += start + len(deterministicContextPacketEndTag)
		text = strings.TrimSpace(strings.TrimSpace(text[:start]) + "\n\n" + strings.TrimSpace(text[end:]))
		omitted = true
	}
}

func requestHasDeterministicContextPacket(request provider.Request) bool {
	return strings.Contains(provider.PromptText(request), deterministicContextPacketStartTag)
}

func deterministicContextPacketTokensFromProviderRequest(request provider.Request) int {
	packet, ok := deterministicContextPacketTextFromProviderRequest(request)
	if !ok {
		return 0
	}
	return provider.EstimateTextTokens(packet)
}

func deterministicContextPacketTextFromProviderRequest(request provider.Request) (string, bool) {
	text := provider.PromptText(request)
	start := strings.Index(text, deterministicContextPacketStartTag)
	if start < 0 {
		return "", false
	}
	end := strings.Index(text[start:], deterministicContextPacketEndTag)
	if end < 0 {
		return "", false
	}
	end += start + len(deterministicContextPacketEndTag)
	return strings.TrimSpace(text[start:end]), true
}
