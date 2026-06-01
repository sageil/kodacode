package codeintel

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/lsp"
)

func shouldRetryCodeIntelCodeActionSelection(err error, actions []lsp.CodeAction) bool {
	if err == nil {
		return false
	}
	if len(actions) == 0 {
		return true
	}
	msg := err.Error()
	return strings.HasPrefix(msg, "no code actions available") || strings.HasPrefix(msg, "no enabled code actions matched")
}

func codeIntelCodeActionSelectionNotice(err error) error {
	message := strings.TrimSpace("no matching LSP code action available")
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = strings.TrimSpace(err.Error())
	}
	return codeIntelNoticeError(codeIntelUnsupportedNotice(message + ". No edit was applied; use apply_patch for a manual source edit if the change is still needed"))
}

func selectCodeIntelCodeAction(actions []lsp.CodeAction, title, kind string, onlyPreferred bool) (*lsp.CodeAction, error) {
	if len(actions) == 0 {
		return nil, fmt.Errorf("no code actions available")
	}
	var matches []lsp.CodeAction
	for _, action := range actions {
		if action.Disabled != nil {
			continue
		}
		if onlyPreferred && !action.IsPreferred {
			continue
		}
		if title != "" && !strings.EqualFold(strings.TrimSpace(action.Title), strings.TrimSpace(title)) {
			continue
		}
		if kind != "" {
			trimmedKind := strings.TrimSpace(kind)
			if action.Kind != trimmedKind && !strings.HasPrefix(action.Kind, trimmedKind+".") {
				continue
			}
		}
		matches = append(matches, action)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no enabled code actions matched. Available actions: %s", strings.Join(availableCodeActionLabels(actions), "; "))
	}
	if len(matches) > 1 && title == "" && kind == "" {
		return nil, fmt.Errorf("multiple code actions matched. Specify title or kind. Available actions: %s", strings.Join(availableCodeActionLabels(matches), "; "))
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].IsPreferred != matches[j].IsPreferred {
			return matches[i].IsPreferred
		}
		return matches[i].Title < matches[j].Title
	})
	return &matches[0], nil
}

func availableCodeActionLabels(actions []lsp.CodeAction) []string {
	labels := make([]string, 0, len(actions))
	for _, action := range actions {
		if action.Disabled != nil {
			continue
		}
		label := action.Title
		if action.Kind != "" {
			label += " [" + action.Kind + "]"
		}
		if action.IsPreferred {
			label += " preferred"
		}
		labels = append(labels, label)
	}
	if len(labels) == 0 {
		return []string{"none"}
	}
	return labels
}
