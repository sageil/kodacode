package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/textdiff"
)

type loadedToolMutationDetail struct {
	detail  app.ToolMutationDetail
	preview textdiff.Preview
}

func loadToolMutationDetailCmd(ctx context.Context, controller controller, sessionID string, ref sessionToolCallRef) tea.Cmd {
	return func() tea.Msg {
		detail, err := controller.LoadToolMutationDetail(ctx, sessionID, ref.TurnID, ref.CallID)
		if err != nil {
			return toolMutationDetailLoadedMsg{sessionID: sessionID, ref: ref, err: err}
		}
		return toolMutationDetailLoadedMsg{
			sessionID: sessionID,
			ref:       ref,
			detail: loadedToolMutationDetail{
				detail:  detail,
				preview: textdiff.BuildPreview(detail.Before, detail.After, mutationToolDetailLoadedPreviewContextLines),
			},
			err: nil,
		}
	}
}

func (m *Model) ensureToolMutationDetailLoadedForSessionCmd(sessionID string, ref sessionToolCallRef, call *events.ToolCallState) tea.Cmd {
	if m == nil || !callHasLoadableMutationDetail(call) {
		return nil
	}
	key := scopedToolKey(sessionID, ref)
	if _, ok := m.toolHydration.loadedMutations[key]; ok {
		return nil
	}
	if m.toolHydration.loadingMutations[key] {
		return nil
	}
	m.toolHydration.loadingMutations[key] = true
	return loadToolMutationDetailCmd(m.ctx, m.controller, strings.TrimSpace(sessionID), ref)
}

func callHasLoadableMutationDetail(call *events.ToolCallState) bool {
	if call == nil || !isMutationToolCall(call) {
		return false
	}
	if !call.Completed || !call.Succeeded || strings.TrimSpace(call.Error) != "" {
		return false
	}
	return call.WriteMutation != nil
}

func (m *Model) clearToolMutationDetailCache() {
	if m == nil {
		return
	}
	m.toolHydration.loadedMutations = make(map[scopedToolCallKey]loadedToolMutationDetail)
	m.toolHydration.loadingMutations = make(map[scopedToolCallKey]bool)
}

func (m *Model) retainToolMutationDetailForRef(sessionID string, ref sessionToolCallRef) {
	if m == nil {
		return
	}
	key := scopedToolKey(sessionID, ref)
	if detail, ok := m.toolHydration.loadedMutations[key]; ok {
		m.toolHydration.loadedMutations = map[scopedToolCallKey]loadedToolMutationDetail{key: detail}
	} else {
		m.toolHydration.loadedMutations = make(map[scopedToolCallKey]loadedToolMutationDetail)
	}
	if m.toolHydration.loadingMutations[key] {
		m.toolHydration.loadingMutations = map[scopedToolCallKey]bool{key: true}
	} else {
		m.toolHydration.loadingMutations = make(map[scopedToolCallKey]bool)
	}
}

func (m Model) shouldCacheToolMutationDetail(sessionID string, ref sessionToolCallRef) bool {
	dialog, ok := m.dialog.(*toolDetailDialog)
	if !ok {
		return false
	}
	return strings.TrimSpace(dialog.sessionID) == strings.TrimSpace(sessionID) && dialog.ref == ref
}
