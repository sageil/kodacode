package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/events"
)

const footerActivityWindow = 6 * time.Second

type footerActivityTone string
type footerActivityKind string

const (
	footerActivityToneInfo    footerActivityTone = "info"
	footerActivityToneWarning footerActivityTone = "warning"
	footerActivityToneError   footerActivityTone = "error"

	footerActivityKindHistory footerActivityKind = "history"
)

type footerActivityState struct {
	id   int
	key  string
	text string
	tone footerActivityTone
	kind footerActivityKind
}

type footerActivityExpiredMsg struct {
	id int
}

func (m *Model) showFooterActivity(text string, tone footerActivityTone, kind footerActivityKind) tea.Cmd {
	return m.showFooterActivityKeyed(text, tone, kind, "")
}

func (m *Model) showFooterActivityKeyed(text string, tone footerActivityTone, kind footerActivityKind, key string) tea.Cmd {
	if m == nil {
		return nil
	}
	text = strings.TrimSpace(text)
	key = strings.TrimSpace(key)
	if text == "" {
		if m.footerNotice.activity == nil {
			return nil
		}
		m.footerNotice.activity = nil
		m.syncViewportLayout()
		return nil
	}
	if current := m.footerNotice.activity; current != nil && current.kind == kind {
		if key != "" && current.key == key {
			return nil
		}
		if kind == footerActivityKindHistory {
			text = mergeFooterActivityText(current.text, text)
		}
	}
	m.footerNotice.nextID++
	activityID := m.footerNotice.nextID
	m.footerNotice.activity = &footerActivityState{
		id:   activityID,
		key:  key,
		text: text,
		tone: tone,
		kind: kind,
	}
	m.syncViewportLayout()
	return tea.Tick(footerActivityWindow, func(time.Time) tea.Msg {
		return footerActivityExpiredMsg{id: activityID}
	})
}

func (m *Model) clearFooterActivity() {
	if m == nil || m.footerNotice.activity == nil {
		return
	}
	m.footerNotice.activity = nil
	m.syncViewportLayout()
}

func historyCompactionActivityText(payload events.SessionHistoryContinuationUpdatedPayload) string {
	return strings.TrimSpace(payload.ActivityText)
}

func historyPruningActivityText(payload events.ContextPrunedPayload) string {
	parts := make([]string, 0, 1)
	if payload.OmittedPriorTurns > 0 {
		parts = append(parts, pluralize(payload.OmittedPriorTurns, "prior turn"))
	}
	if len(parts) == 0 {
		return ""
	}
	return "History pruned: " + strings.Join(parts, ", ")
}

func historyCompactionFailedActivityText(events.ContextCompactionFailedPayload) string {
	return historySummaryFailureLabel
}

func pruningPayloadHasOmissions(payload events.ContextPrunedPayload) bool {
	return payload.OmittedPriorTurns > 0
}

func mergeFooterActivityText(existing, next string) string {
	parts := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	for _, raw := range append(splitFooterActivityText(existing), splitFooterActivityText(next)...) {
		part := strings.TrimSpace(raw)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		parts = append(parts, part)
	}
	return strings.Join(parts, " • ")
}

func splitFooterActivityText(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return strings.Split(text, " • ")
}

type footerActivityBatchResult struct {
	show  bool
	key   string
	text  string
	tone  footerActivityTone
	kind  footerActivityKind
	clear bool
}

func footerActivityBatchOutcome(batch []events.Event) footerActivityBatchResult {
	var result footerActivityBatchResult
	sawShow := false
	sawClear := false
	for _, event := range batch {
		if text, tone, kind, key, ok := footerActivityUpdateForEvent(event); ok {
			sawShow = true
			if result.show && result.kind == kind && kind == footerActivityKindHistory {
				text = mergeFooterActivityText(result.text, text)
			}
			result = footerActivityBatchResult{
				show: true,
				key:  key,
				text: text,
				tone: tone,
				kind: kind,
			}
			continue
		}
		if eventClearsFooterActivity(event) {
			sawClear = true
		}
	}
	if sawShow {
		return result
	}
	if sawClear {
		return footerActivityBatchResult{clear: true}
	}
	return result
}

func footerActivityUpdateForEvent(event events.Event) (string, footerActivityTone, footerActivityKind, string, bool) {
	switch event.Type {
	case events.TypeSessionHistoryContinuationUpdated:
		payload, ok := event.Payload.(events.SessionHistoryContinuationUpdatedPayload)
		if !ok {
			return "", "", "", "", false
		}
		text := historyCompactionActivityText(payload)
		if text == "" {
			return "", "", "", "", false
		}
		return text, footerActivityToneWarning, footerActivityKindHistory, "", true
	case events.TypeContextCompactionFailed:
		payload, ok := event.Payload.(events.ContextCompactionFailedPayload)
		if !ok {
			return "", "", "", "", false
		}
		return historyCompactionFailedActivityText(payload), footerActivityToneWarning, footerActivityKindHistory, "", true
	case events.TypeContextPruned:
		payload, ok := event.Payload.(events.ContextPrunedPayload)
		if !ok || !pruningPayloadHasOmissions(payload) {
			return "", "", "", "", false
		}
		return historyPruningActivityText(payload), footerActivityToneWarning, footerActivityKindHistory, "", true
	default:
		return "", "", "", "", false
	}
}

func eventClearsFooterActivity(event events.Event) bool {
	switch event.Type {
	case events.TypeSessionConfigured,
		events.TypeSessionModelRouteUpdated,
		events.TypeSessionWorkspaceRootsAdded,
		events.TypeSessionPermissionModeUpdated,
		events.TypeSessionStateSnapshot,
		events.TypeSessionHistoryCheckpoint,
		events.TypeSessionHistoryContinuationUpdated,
		events.TypeTurnConfigured,
		events.TypeContextCompactionFailed,
		events.TypeTurnProviderUsageRecorded,
		events.TypeTurnProviderUsageReported,
		events.TypeContextPruned:
		return false
	default:
		return true
	}
}
