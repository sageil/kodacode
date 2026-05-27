package tui

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/app"
)

type Attachment struct {
	Path     string
	MIMEType string
	Name     string
	Size     int64
	Tag      string
}

func (m *Model) syncComposerPrompt() {
	prompt := composerPrompt(m.composerState.pendingAttachments, shellLayoutEnabled(*m))
	promptWidth := lipgloss.Width(prompt)
	m.composer.SetPromptFunc(promptWidth, func(info textarea.PromptInfo) string {
		if info.LineNumber == 0 {
			return prompt
		}
		return strings.Repeat(" ", promptWidth)
	})
}

func composerPrompt(_ []Attachment, shellLayout bool) string {
	if shellLayout {
		return "❯ "
	}
	return "  "
}

func formatAttachmentLabel(index int, attachment Attachment) string {
	prefix := "File"
	if strings.HasPrefix(strings.TrimSpace(attachment.MIMEType), "image/") {
		prefix = "Image"
	}
	return fmt.Sprintf("[%s %s #%d]", prefix, attachment.Name, index)
}

func (m *Model) appendPendingAttachment(attachment Attachment) (Attachment, bool) {
	for _, existing := range m.composerState.pendingAttachments {
		if existing.Path == attachment.Path {
			m.syncComposerPrompt()
			return Attachment{}, false
		}
	}
	if strings.TrimSpace(attachment.Tag) == "" {
		m.composerState.nextAttachmentID++
		attachment.Tag = formatAttachmentLabel(m.composerState.nextAttachmentID, attachment)
	}
	m.composerState.pendingAttachments = append(m.composerState.pendingAttachments, attachment)
	m.syncComposerPrompt()
	return attachment, true
}

func (m *Model) removePendingAttachmentAt(index int) bool {
	if index < 0 || index >= len(m.composerState.pendingAttachments) {
		return false
	}
	m.composerState.pendingAttachments = append(m.composerState.pendingAttachments[:index], m.composerState.pendingAttachments[index+1:]...)
	if len(m.composerState.pendingAttachments) == 0 {
		m.composerState.nextAttachmentID = 0
	}
	m.syncComposerPrompt()
	return true
}

func (m *Model) clearPendingAttachments() {
	m.composerState.pendingAttachments = nil
	m.composerState.nextAttachmentID = 0
	m.syncComposerPrompt()
}

func (m Model) pendingAttachmentInputs() []app.AttachmentInput {
	attachments := m.orderedPendingAttachments()
	if len(attachments) == 0 {
		return nil
	}
	inputs := make([]app.AttachmentInput, 0, len(attachments))
	for _, attachment := range attachments {
		if strings.TrimSpace(attachment.Path) == "" {
			continue
		}
		inputs = append(inputs, app.AttachmentInput{Path: attachment.Path})
	}
	return inputs
}

func (m Model) orderedPendingAttachments() []Attachment {
	if len(m.composerState.pendingAttachments) == 0 {
		return nil
	}
	value := m.composer.Value()
	type attachmentRef struct {
		attachment Attachment
		index      int
	}
	refs := make([]attachmentRef, 0, len(m.composerState.pendingAttachments))
	for _, attachment := range m.composerState.pendingAttachments {
		tag := strings.TrimSpace(attachment.Tag)
		if tag == "" {
			continue
		}
		idx := strings.Index(value, tag)
		if idx < 0 {
			continue
		}
		refs = append(refs, attachmentRef{attachment: attachment, index: idx})
	}
	if len(refs) == 0 {
		return nil
	}
	sort.SliceStable(refs, func(i, j int) bool {
		return refs[i].index < refs[j].index
	})
	ordered := make([]Attachment, 0, len(refs))
	for _, ref := range refs {
		ordered = append(ordered, ref.attachment)
	}
	return ordered
}

func (m Model) expandComposerAttachmentTokens(text string) string {
	if len(m.composerState.pendingAttachments) == 0 {
		return text
	}
	for _, attachment := range m.composerState.pendingAttachments {
		if strings.TrimSpace(attachment.Tag) == "" {
			continue
		}
		text = strings.ReplaceAll(text, attachment.Tag, "")
	}
	return text
}

func (m *Model) removePendingAttachmentByTag(tag string) bool {
	for idx, attachment := range m.composerState.pendingAttachments {
		if attachment.Tag == tag {
			return m.removePendingAttachmentAt(idx)
		}
	}
	return false
}

func (m Model) validatePastedAttachment(text string) (Attachment, bool) {
	path := strings.TrimSpace(text)
	if path == "" || strings.Contains(path, "\n") || strings.Contains(path, "\r") {
		return Attachment{}, false
	}
	if len(path) >= 2 {
		if (path[0] == '\'' && path[len(path)-1] == '\'') || (path[0] == '"' && path[len(path)-1] == '"') {
			path = path[1 : len(path)-1]
		}
	}
	resolved, err := resolveComposerAttachmentPath(m.workspace, path)
	if err != nil {
		return Attachment{}, false
	}
	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		return Attachment{}, false
	}
	mimeType, ok := detectComposerAttachmentMIME(resolved)
	if !ok {
		return Attachment{}, false
	}
	return Attachment{
		Path:     resolved,
		MIMEType: mimeType,
		Name:     filepath.Base(resolved),
		Size:     info.Size(),
	}, true
}

func resolveComposerAttachmentPath(workspaceRoot, raw string) (string, error) {
	path := strings.TrimSpace(raw)
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	if !filepath.IsAbs(path) && strings.TrimSpace(workspaceRoot) != "" {
		path = filepath.Join(workspaceRoot, path)
	}
	return filepath.Abs(path)
}

func detectComposerAttachmentMIME(path string) (string, bool) {
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer func() {
		_ = file.Close()
	}()

	header := make([]byte, 512)
	n, err := file.Read(header)
	if err != nil && n == 0 {
		return "", false
	}

	mimeType := http.DetectContentType(header[:n])
	switch strings.TrimSpace(mimeType) {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return mimeType, true
	default:
		return "", false
	}
}
