package tui

import (
	"errors"
	"strings"
)

var ErrTranscriptCopyUnavailable = errors.New("no transcript content available to copy")

func normalizeTranscriptCopyText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

func transcriptCopyErrorText(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrTranscriptSelectionInactive):
		return "Start visual selection with v or click in the transcript before copying."
	case errors.Is(err, ErrTranscriptCopyUnavailable):
		return "Nothing in the current transcript selection to copy."
	case errors.Is(err, ErrClipboardUnavailable):
		return "Clipboard unavailable; install pbcopy, wl-copy, xclip, or xsel."
	default:
		return err.Error()
	}
}
