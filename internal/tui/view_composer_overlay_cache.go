package tui

import (
	"hash/fnv"
	"strings"
	"unsafe"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/cellbuf"
	"github.com/sageil/kodacode/internal/events"
)

func renderComposerOverlaySurface(m Model, state events.SessionState, layout shellLayout, base *cellbuf.Buffer, baseRows []string) (string, *tea.Cursor) {
	cursor := composerCursorForSurface(m, state, layout)
	anchor := composerAnchorForSurface(m, state, layout)
	if anchor == nil {
		return renderBaseSurfaceRows(base, baseRows), nil
	}

	width := composerPopupWidth(m, layout.totalWidth)
	popup := renderComposerPopup(m, width)
	if strings.TrimSpace(popup) == "" {
		return renderBaseSurfaceRows(base, baseRows), cursor
	}

	key := composerOverlayRenderCacheKey(base, popup, anchor)
	if m.renderCache.composerOverlay == nil {
		return renderComposerOverlaySurfaceUncached(base, baseRows, popup, anchor, cursor)
	}
	return m.renderCache.composerOverlay.frameFor(key, func() (string, *tea.Cursor) {
		return renderComposerOverlaySurfaceUncached(base, baseRows, popup, anchor, cursor)
	})
}

func renderComposerOverlaySurfaceUncached(base *cellbuf.Buffer, baseRows []string, popup string, anchor *tea.Cursor, cursor *tea.Cursor) (string, *tea.Cursor) {
	surface := newOverlaySurface(base, baseRows)
	drawRenderedComposerPopupOnSurface(surface, popup, anchor)
	return renderDialogSurface(surface), cursor
}

func renderBaseSurfaceRows(base *cellbuf.Buffer, baseRows []string) string {
	if len(baseRows) > 0 {
		return strings.Join(baseRows, "\n")
	}
	return renderCellBuffer(base)
}

func composerOverlayRenderCacheKey(base *cellbuf.Buffer, popup string, cursor *tea.Cursor) uint64 {
	hasher := fnv.New64a()
	writeTranscriptSignatureUint64(hasher, uint64(uintptr(unsafe.Pointer(base))))
	if cursor == nil {
		writeTranscriptSignatureBool(hasher, false)
	} else {
		writeTranscriptSignatureBool(hasher, true)
		writeTranscriptSignatureInt(hasher, cursor.X)
		writeTranscriptSignatureInt(hasher, cursor.Y)
		writeTranscriptSignatureBool(hasher, cursor.Blink)
		writeTranscriptSignatureInt(hasher, int(cursor.Shape))
	}
	writeTranscriptSignatureString(hasher, popup)
	return hasher.Sum64()
}
