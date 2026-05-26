package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/app"
)

type FocusPath struct {
	Path  string
	IsDir bool
	Tag   string
}

func (m *Model) appendPendingFocusPath(path app.WorkspacePath) (FocusPath, bool) {
	clean := cleanWorkspaceFocusPath(path.Path)
	if clean == "" {
		return FocusPath{}, false
	}
	for _, existing := range m.composerState.pendingFocusPaths {
		if existing.Path == clean {
			return FocusPath{}, false
		}
	}
	m.composerState.nextFocusPathID++
	prefix := "File"
	if path.IsDir {
		prefix = "Dir"
	}
	focusPath := FocusPath{
		Path:  clean,
		IsDir: path.IsDir,
		Tag:   fmt.Sprintf("[%s %s #%d]", prefix, filepath.Base(clean), m.composerState.nextFocusPathID),
	}
	m.composerState.pendingFocusPaths = append(m.composerState.pendingFocusPaths, focusPath)
	return focusPath, true
}

func (m *Model) removePendingFocusPathByTag(tag string) bool {
	for idx, focusPath := range m.composerState.pendingFocusPaths {
		if focusPath.Tag != tag {
			continue
		}
		m.composerState.pendingFocusPaths = append(m.composerState.pendingFocusPaths[:idx], m.composerState.pendingFocusPaths[idx+1:]...)
		if len(m.composerState.pendingFocusPaths) == 0 {
			m.composerState.nextFocusPathID = 0
		}
		return true
	}
	return false
}

func (m *Model) clearPendingFocusPaths() {
	m.composerState.pendingFocusPaths = nil
	m.composerState.nextFocusPathID = 0
}

func (m *Model) filterPendingFocusPathsForComposerText(text string) {
	if len(m.composerState.pendingFocusPaths) == 0 {
		return
	}
	filtered := m.composerState.pendingFocusPaths[:0]
	for _, focusPath := range m.composerState.pendingFocusPaths {
		tag := strings.TrimSpace(focusPath.Tag)
		if tag == "" || !strings.Contains(text, tag) {
			continue
		}
		filtered = append(filtered, focusPath)
	}
	if len(filtered) == 0 {
		m.composerState.pendingFocusPaths = nil
		m.composerState.nextFocusPathID = 0
		return
	}
	m.composerState.pendingFocusPaths = filtered
}

func (m Model) expandComposerFocusPathTokens(text string) string {
	for _, focusPath := range m.composerState.pendingFocusPaths {
		if strings.TrimSpace(focusPath.Tag) == "" {
			continue
		}
		text = strings.ReplaceAll(text, focusPath.Tag, "")
	}
	return text
}

func (m Model) userTextWithFocusPaths(userText string) string {
	focusPaths := m.orderedPendingFocusPaths()
	if len(focusPaths) == 0 {
		return strings.TrimSpace(userText)
	}

	lines := make([]string, 0, len(focusPaths)+4)
	lines = append(lines, "Focus paths:")
	for _, focusPath := range focusPaths {
		kind := "file"
		if focusPath.IsDir {
			kind = "directory"
		}
		lines = append(lines, "- "+focusPath.Path+" ("+kind+")")
	}
	request := strings.TrimSpace(userText)
	if request == "" {
		request = "Use the focus paths as the primary context."
	}
	lines = append(lines, "", "User request:", request)
	return strings.Join(lines, "\n")
}

func (m Model) orderedPendingFocusPaths() []FocusPath {
	if len(m.composerState.pendingFocusPaths) == 0 {
		return nil
	}
	value := m.composer.Value()
	type focusRef struct {
		focusPath FocusPath
		index     int
	}
	refs := make([]focusRef, 0, len(m.composerState.pendingFocusPaths))
	for _, focusPath := range m.composerState.pendingFocusPaths {
		tag := strings.TrimSpace(focusPath.Tag)
		if tag == "" {
			continue
		}
		idx := strings.Index(value, tag)
		if idx < 0 {
			continue
		}
		refs = append(refs, focusRef{focusPath: focusPath, index: idx})
	}
	if len(refs) == 0 {
		return nil
	}
	sort.SliceStable(refs, func(i, j int) bool {
		return refs[i].index < refs[j].index
	})
	ordered := make([]FocusPath, 0, len(refs))
	for _, ref := range refs {
		ordered = append(ordered, ref.focusPath)
	}
	return ordered
}

func cleanWorkspaceFocusPath(path string) string {
	clean := strings.TrimSpace(filepath.ToSlash(path))
	clean = strings.TrimPrefix(clean, "./")
	clean = strings.Trim(clean, "/")
	return clean
}
