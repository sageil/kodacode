package tui

import (
	"hash"
	"hash/fnv"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func splitWideViewCacheKey(m Model, state events.SessionState, layout shellLayout) uint64 {
	layout = normalizeWideShellLayout(m, state, layout)
	return splitWideViewCacheKeyForLayout(m, state, layout)
}

func splitWideViewCacheKeyForLayout(m Model, state events.SessionState, layout shellLayout) uint64 {
	panelHeight := splitWidePanelHeight(layout)
	leftWidth := layout.centerWidth
	if !layout.showInspector {
		leftWidth = layout.totalWidth
	}
	borderless := splitTranscriptPaneBorderless(m, layout)

	hasher := fnv.New64a()
	writeTranscriptSignatureString(hasher, "split-wide-view")
	writeTranscriptSignatureInt(hasher, max(m.width, 1))
	writeTranscriptSignatureInt(hasher, max(m.height, 1))
	writeTranscriptSignatureInt(hasher, max(layout.totalWidth, 1))
	writeTranscriptSignatureInt(hasher, max(layout.centerWidth, 1))
	writeTranscriptSignatureInt(hasher, max(layout.rightWidth, 0))
	writeTranscriptSignatureInt(hasher, max(layout.contentHeight, 1))
	writeTranscriptSignatureBool(hasher, layout.showInspector)
	writeTranscriptSignatureString(hasher, modelThemeCacheKey(m))
	writeTranscriptSignatureString(hasher, string(m.chrome.focus))
	writeTranscriptSignatureBool(hasher, m.transcriptView.visualActive)
	writeTranscriptSignatureBool(hasher, m.shouldAnimateTranscriptActivity())
	writeTranscriptSignatureString(hasher, shellModeName(m))
	writeTranscriptSignatureString(hasher, shellSessionLabel(m, state))
	writeTranscriptSignatureString(hasher, strings.TrimSpace(state.Title))
	writeTranscriptSignatureString(hasher, strings.TrimSpace(state.PermissionMode))

	provider, model, thinking, capacity, caps := headerModelZoneParts(m, state)
	writeTranscriptSignatureString(hasher, provider)
	writeTranscriptSignatureString(hasher, model)
	writeTranscriptSignatureString(hasher, thinking)
	writeTranscriptSignatureString(hasher, capacity)
	writeTranscriptSignatureString(hasher, caps)

	if display, ok := currentSessionContextDisplay(m, state); ok {
		writeTranscriptSignatureBool(hasher, true)
		writeTranscriptSignatureInt(hasher, display.tokens)
		writeTranscriptSignatureInt(hasher, display.limit)
		writeTranscriptSignatureInt(hasher, display.percent)
		writeTranscriptSignatureString(hasher, display.source)
		writeTranscriptSignatureBool(hasher, display.capacityOnly)
		writeTranscriptSignatureBool(hasher, display.last)
		writeTranscriptSignatureBool(hasher, display.peak)
	} else {
		writeTranscriptSignatureBool(hasher, false)
	}

	metricsState, metricsTurnID, delegated := effectiveStatusMetricsScope(m, state)
	writeTranscriptSignatureString(hasher, strings.TrimSpace(metricsState.SessionID))
	writeTranscriptSignatureString(hasher, strings.TrimSpace(metricsTurnID))
	writeTranscriptSignatureBool(hasher, delegated)
	inputTokens, outputTokens, exact, ok := effectiveSessionTokenTotals(m, metricsState)
	writeTranscriptSignatureBool(hasher, ok)
	writeTranscriptSignatureInt(hasher, inputTokens)
	writeTranscriptSignatureInt(hasher, outputTokens)
	writeTranscriptSignatureBool(hasher, exact)
	writeTranscriptSignatureString(hasher, effectiveSessionEstimatedCostLabel(m, metricsState))

	writeTranscriptSignatureString(hasher, shellStatusHints(m, state))
	writeTranscriptSignatureString(hasher, strings.TrimSpace(m.composer.Value()))
	writeTranscriptSignatureString(hasher, strings.TrimSpace(m.composerState.err))
	writeTranscriptSignatureString(hasher, strings.TrimSpace(m.composerDisabledMessage(state)))
	writeTranscriptSignatureBool(hasher, m.hasPendingInteraction())
	writeTranscriptSignatureBool(hasher, m.pendingInteractionSubmissionInFlight())
	if activity, ok := composerActivityStripStateFor(m, state); ok {
		writeTranscriptSignatureBool(hasher, true)
		writeTranscriptSignatureString(hasher, activity.Label)
		writeTranscriptSignatureString(hasher, activity.LabelColor)
		writeTranscriptSignatureString(hasher, activity.MetaText)
		writeTranscriptSignatureString(hasher, activity.MetaColor)
		writeTranscriptSignatureBool(hasher, activity.Spinning)
	} else {
		writeTranscriptSignatureBool(hasher, false)
	}

	noticeText, noticeTone := footerNoticeText(m, state)
	writeTranscriptSignatureString(hasher, noticeText)
	writeTranscriptSignatureString(hasher, string(noticeTone))
	appendTranscriptStatusSegmentsSignature(hasher, footerStatusSegments(m, state))

	writeTranscriptSignatureUint64(hasher, splitTranscriptPaneCacheKey(m, state, leftWidth, panelHeight, borderless))
	if layout.showInspector {
		writeTranscriptSignatureUint64(hasher, splitInspectorPaneCacheKey(m, layout.rightWidth, panelHeight))
	}
	return hasher.Sum64()
}

func appendTranscriptStatusSegmentsSignature(hasher hash.Hash64, segments []transcriptStatusSegment) {
	if hasher == nil {
		return
	}
	for _, segment := range segments {
		writeTranscriptSignatureString(hasher, segment.Text)
		writeTranscriptSignatureString(hasher, segment.Color)
		writeTranscriptSignatureBool(hasher, segment.Bold)
	}
}
