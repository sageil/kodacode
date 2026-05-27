package tui

import (
	"hash/fnv"
	"strings"
)

func splitInspectorPaneCacheKey(m Model, width, height int) uint64 {
	hasher := fnv.New64a()
	writeTranscriptSignatureString(hasher, "split-inspector-pane")
	writeTranscriptSignatureInt(hasher, max(width, 1))
	writeTranscriptSignatureInt(hasher, max(height, 1))
	writeTranscriptSignatureString(hasher, modelRenderCacheKey(m))
	writeTranscriptSignatureString(hasher, string(m.chrome.focus))
	writeTranscriptSignatureString(hasher, strings.TrimSpace(m.agentID))
	writeTranscriptSignatureInt(hasher, effectiveInspectorTab(m))
	writeTranscriptSignatureInt(hasher, m.inspector.body.Width())
	writeTranscriptSignatureInt(hasher, m.inspector.body.Height())
	writeTranscriptSignatureInt(hasher, m.inspector.body.YOffset())
	writeTranscriptSignatureInt64(hasher, m.inspector.body.ContentVersion())
	writeTranscriptSignatureBool(hasher, m.inspector.body.softWrap)
	return hasher.Sum64()
}
