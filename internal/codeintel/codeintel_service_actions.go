package codeintel

import (
	"context"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/lsp"
	"github.com/sageil/kodacode/internal/tool"
	"github.com/sageil/kodacode/internal/workspaceedit"
)

func (w *workspaceCodeIntel) RenameSymbol(ctx context.Context, request tool.CodeIntelRenameRequest) (tool.CodeIntelMutationSummary, error) {
	manager := w.service.managerForPath(w.roots, request.Path)
	if manager == nil {
		return tool.CodeIntelMutationSummary{}, codeIntelNoticeError(codeIntelUnavailableNotice(fmt.Errorf("no code-intelligence workspace root matched %q", request.Path)))
	}
	server, err := manager.ServerForPath(ctx, request.Path)
	if err != nil {
		return tool.CodeIntelMutationSummary{}, codeIntelUnavailableError(err)
	}
	var edit *lsp.WorkspaceEdit
	for _, candidate := range tool.BestEffortPositionCandidates(request.Path, request.Line, request.Character) {
		edit, err = server.Rename(ctx, request.Path, request.Line-1, candidate, request.NewName)
		if err != nil {
			return tool.CodeIntelMutationSummary{}, err
		}
		if tool.WorkspaceEditHasChanges(edit) {
			break
		}
	}
	if !tool.WorkspaceEditHasChanges(edit) {
		return tool.CodeIntelMutationSummary{}, nil
	}
	return w.applyCodeIntelMutation(ctx, edit)
}

func (w *workspaceCodeIntel) ApplyCodeAction(ctx context.Context, request tool.CodeIntelCodeActionRequest) (tool.CodeIntelCodeActionResult, error) {
	manager := w.service.managerForPath(w.roots, request.Path)
	if manager == nil {
		return tool.CodeIntelCodeActionResult{}, codeIntelNoticeError(codeIntelUnavailableNotice(fmt.Errorf("no code-intelligence workspace root matched %q", request.Path)))
	}
	server, err := manager.ServerForPath(ctx, request.Path)
	if err != nil {
		return tool.CodeIntelCodeActionResult{}, codeIntelUnavailableError(err)
	}
	initialRange := lsp.Range{
		Start: lsp.Position{Line: request.StartLine - 1, Character: request.StartCharacter},
		End:   lsp.Position{Line: request.EndLine - 1, Character: request.EndCharacter},
	}
	var only []string
	if strings.TrimSpace(request.Kind) != "" {
		only = []string{strings.TrimSpace(request.Kind)}
	}
	var diagnostics []lsp.Diagnostic
	if isQuickfixCodeActionKind(request.Kind) {
		diagnostics, err = refreshServerDiagnostics(ctx, server, request.Path, codeIntelCodeActionDiagnosticsTimeout)
		if err != nil {
			return tool.CodeIntelCodeActionResult{}, err
		}
	}
	candidateRanges := codeActionCandidateRanges(
		request,
		initialRange,
		diagnostics,
		tool.BestEffortCodeActionRanges(request.Path, request.StartLine, request.StartCharacter, request.EndLine, request.EndCharacter),
	)
	var (
		lastSelectionErr error
		selected         *lsp.CodeAction
	)
	for _, candidateRange := range candidateRanges {
		actions, err := server.CodeActions(ctx, request.Path, candidateRange, only)
		if err != nil {
			return tool.CodeIntelCodeActionResult{}, err
		}
		selected, lastSelectionErr = selectCodeIntelCodeAction(actions, request.Title, request.Kind, request.OnlyPreferred)
		if lastSelectionErr == nil {
			break
		}
		if !shouldRetryCodeIntelCodeActionSelection(lastSelectionErr, actions) {
			return tool.CodeIntelCodeActionResult{}, codeIntelCodeActionSelectionNotice(lastSelectionErr)
		}
	}
	if lastSelectionErr != nil {
		return tool.CodeIntelCodeActionResult{}, codeIntelCodeActionSelectionNotice(lastSelectionErr)
	}
	if selected == nil {
		return tool.CodeIntelCodeActionResult{}, codeIntelCodeActionSelectionNotice(fmt.Errorf("no code actions available"))
	}
	if selected.Edit == nil {
		if selected.Command != nil {
			return tool.CodeIntelCodeActionResult{}, fmt.Errorf("code action %q requires server-side executeCommand, which is not supported", selected.Title)
		}
		return tool.CodeIntelCodeActionResult{}, fmt.Errorf("code action %q produced no edits", selected.Title)
	}
	summary, err := w.applyCodeIntelMutation(ctx, selected.Edit)
	if err != nil {
		return tool.CodeIntelCodeActionResult{}, err
	}
	return tool.CodeIntelCodeActionResult{Title: selected.Title, Summary: summary}, nil
}

func codeActionCandidateRanges(request tool.CodeIntelCodeActionRequest, initial lsp.Range, diagnostics []lsp.Diagnostic, fallback []lsp.Range) []lsp.Range {
	ranges := []lsp.Range{initial}
	if isQuickfixCodeActionKind(request.Kind) {
		for _, diagnostic := range diagnostics {
			if !quickfixRangeMatchesDiagnostic(initial, diagnostic.Range) {
				continue
			}
			ranges = tool.AppendUniqueRange(ranges, diagnostic.Range)
		}
		return ranges
	}
	for _, candidate := range fallback {
		ranges = tool.AppendUniqueRange(ranges, candidate)
	}
	return ranges
}

func isQuickfixCodeActionKind(kind string) bool {
	trimmed := strings.TrimSpace(kind)
	return trimmed == "quickfix" || strings.HasPrefix(trimmed, "quickfix.")
}

func rangesOverlap(left, right lsp.Range) bool {
	return comparePosition(left.Start, right.End) < 0 && comparePosition(right.Start, left.End) < 0
}

func quickfixRangeMatchesDiagnostic(selection, diagnostic lsp.Range) bool {
	if comparePosition(selection.Start, selection.End) == 0 {
		return rangeContainsPositionInclusive(diagnostic, selection.Start)
	}
	return rangesOverlap(selection, diagnostic)
}

func rangeContainsPositionInclusive(rng lsp.Range, position lsp.Position) bool {
	return comparePosition(rng.Start, position) <= 0 && comparePosition(position, rng.End) <= 0
}

func comparePosition(left, right lsp.Position) int {
	if left.Line != right.Line {
		if left.Line < right.Line {
			return -1
		}
		return 1
	}
	if left.Character < right.Character {
		return -1
	}
	if left.Character > right.Character {
		return 1
	}
	return 0
}

func (w *workspaceCodeIntel) applyCodeIntelMutation(ctx context.Context, edit *lsp.WorkspaceEdit) (tool.CodeIntelMutationSummary, error) {
	summary, plan, err := workspaceedit.Apply(w.roots, edit, func(path string) (int, bool) {
		manager := w.service.managerForPath(w.roots, path)
		if manager == nil {
			return 0, false
		}
		return manager.DocumentVersion(path)
	})
	if err != nil {
		return tool.CodeIntelMutationSummary{}, err
	}
	w.service.SyncMutation(ctx, w.roots[0], w.roots[1:], plan)
	return codeIntelMutationSummaryFromWorkspaceEdit(summary), nil
}

func codeIntelMutationSummaryFromWorkspaceEdit(summary workspaceedit.Summary) tool.CodeIntelMutationSummary {
	return tool.CodeIntelMutationSummary{
		Paths:     append([]string(nil), summary.Paths...),
		TextEdits: summary.TextEdits,
		Created:   summary.Created,
		Renamed:   summary.Renamed,
		Deleted:   summary.Deleted,
	}
}
