package tui

import (
	"hash"
	"hash/fnv"
	"unsafe"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/cellbuf"
)

func renderDialogOverlaySurface(m Model, base *cellbuf.Buffer, baseRows []string, area dialogRenderArea) (string, *tea.Cursor) {
	if m.dialog == nil {
		return renderBaseSurfaceRows(base, baseRows), nil
	}
	keyer, ok := m.dialog.(dialogOverlayCacheKeyer)
	if !ok || m.renderCache.dialogOverlay == nil {
		return renderDialogOverlaySurfaceUncached(base, baseRows, m.dialog, area)
	}
	key := dialogOverlayRenderCacheKey(base, area, m.dialog.ID(), keyer.OverlayCacheKey())
	return m.renderCache.dialogOverlay.frameFor(key, func() (string, *tea.Cursor) {
		return renderDialogOverlaySurfaceUncached(base, baseRows, m.dialog, area)
	})
}

func renderDialogOverlaySurfaceUncached(base *cellbuf.Buffer, baseRows []string, dialog dialogModel, area dialogRenderArea) (string, *tea.Cursor) {
	surface := newOverlaySurface(base, baseRows)
	cursor := renderDialogOnBuffer(surface, dialog, area)
	return renderDialogSurface(surface), cursor
}

func dialogOverlayRenderCacheKey(base *cellbuf.Buffer, area dialogRenderArea, dialogID string, overlayKey uint64) uint64 {
	hasher := fnv.New64a()
	writeDialogOverlayCacheSignature(hasher, base, area, dialogID, overlayKey)
	return hasher.Sum64()
}

func writeDialogOverlayCacheSignature(hasher hash.Hash64, base *cellbuf.Buffer, area dialogRenderArea, dialogID string, overlayKey uint64) {
	if hasher == nil {
		return
	}
	writeTranscriptSignatureUint64(hasher, uint64(uintptr(unsafe.Pointer(base))))
	writeTranscriptSignatureInt(hasher, area.x)
	writeTranscriptSignatureInt(hasher, area.y)
	writeTranscriptSignatureInt(hasher, area.width)
	writeTranscriptSignatureInt(hasher, area.height)
	writeTranscriptSignatureString(hasher, dialogID)
	writeTranscriptSignatureUint64(hasher, overlayKey)
}

func appendMessagesRenderCacheSignature(hasher hash.Hash64, body Messages) {
	if hasher == nil {
		return
	}
	writeTranscriptSignatureInt(hasher, body.Width())
	writeTranscriptSignatureInt(hasher, body.Height())
	writeTranscriptSignatureInt(hasher, body.YOffset())
	writeTranscriptSignatureInt64(hasher, body.ContentVersion())
	writeTranscriptSignatureBool(hasher, body.softWrap)
}
