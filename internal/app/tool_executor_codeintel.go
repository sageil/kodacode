package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
	"github.com/sageil/kodacode/internal/workspace"
)

type codeIntelRuntime interface {
	Navigator(primaryRoot string, additionalRoots []string) tool.CodeIntel
	SyncMutation(ctx context.Context, primaryRoot string, additionalRoots []string, plan codeIntelMutationSyncPlan)
}

func (e *ToolExecutor) SetCodeIntelService(service codeIntelRuntime) {
	e.codeIntel = service
}

func (e *ToolExecutor) toolCodeIntel(state events.SessionState) tool.CodeIntel {
	if e.codeIntel == nil {
		return nil
	}
	return e.codeIntel.Navigator(state.WorkspaceRoot, state.AdditionalWorkspaceRoots)
}

func (e *ToolExecutor) syncCodeIntelMutationAndAugmentOutput(ctx context.Context, state events.SessionState, scope *workspace.Scope, input ExecuteToolInput, output string) string {
	if e.codeIntel == nil {
		return output
	}
	plan, ok := codeIntelMutationSyncPlanForCall(scope, input.ToolName, input.Arguments)
	if !ok {
		return output
	}
	e.codeIntel.SyncMutation(ctx, state.WorkspaceRoot, state.AdditionalWorkspaceRoots, plan)
	if strings.TrimSpace(input.ToolName) != tool.WriteToolName || len(plan.Changed) == 0 {
		return output
	}
	navigator := e.toolCodeIntel(state)
	if navigator == nil {
		return output
	}
	diagCtx, cancel := context.WithTimeout(ctx, codeIntelMutationFeedbackTimeout)
	defer cancel()
	diagnostics, err := navigator.Diagnostics(diagCtx, []string{plan.Changed[0]})
	if err != nil {
		return output
	}
	return appendWriteDiagnosticsOutput(output, diagnostics)
}

func appendWriteDiagnosticsOutput(output string, diagnostics []tool.CodeIntelFileDiagnostics) string {
	if !hasCodeIntelDiagnostics(diagnostics) {
		return output
	}
	formatted := strings.TrimSpace(tool.FormatDiagnostics(diagnostics))
	if formatted == "" || formatted == "No diagnostics found." {
		return output
	}
	if strings.TrimSpace(output) == "" {
		return "Diagnostics detected in this file:\n" + formatted
	}
	return fmt.Sprintf("%s\n\nDiagnostics detected in this file:\n%s", strings.TrimRight(output, "\n"), formatted)
}

func hasCodeIntelDiagnostics(files []tool.CodeIntelFileDiagnostics) bool {
	for _, file := range files {
		if len(file.Diagnostics) > 0 {
			return true
		}
	}
	return false
}

func codeIntelMutationSyncPlanForCall(scope *workspace.Scope, toolName string, args json.RawMessage) (codeIntelMutationSyncPlan, bool) {
	if scope == nil {
		return codeIntelMutationSyncPlan{}, false
	}
	switch strings.TrimSpace(toolName) {
	case "write":
		var input struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(args, &input) != nil || strings.TrimSpace(input.Path) == "" {
			return codeIntelMutationSyncPlan{}, false
		}
		path, ok := resolveCodeIntelSyncPath(scope, input.Path)
		if !ok {
			return codeIntelMutationSyncPlan{}, false
		}
		return codeIntelMutationSyncPlan{Changed: []string{path}}, true
	case tool.ApplyPatchToolName:
		patch, err := tool.ParseApplyPatch(string(args))
		if err != nil {
			return codeIntelMutationSyncPlan{}, false
		}
		changed := make([]string, 0, len(patch.Operations))
		seen := map[string]struct{}{}
		for _, op := range patch.Operations {
			for _, inputPath := range []string{op.Path, op.MovePath} {
				if strings.TrimSpace(inputPath) == "" {
					continue
				}
				path, ok := resolveCodeIntelSyncPath(scope, inputPath)
				if !ok {
					continue
				}
				if _, exists := seen[path]; exists {
					continue
				}
				seen[path] = struct{}{}
				changed = append(changed, path)
			}
		}
		if len(changed) == 0 {
			return codeIntelMutationSyncPlan{}, false
		}
		return codeIntelMutationSyncPlan{Changed: changed}, true
	default:
		return codeIntelMutationSyncPlan{}, false
	}
}

func resolveCodeIntelSyncPath(scope *workspace.Scope, path string) (string, bool) {
	decision, err := scope.Check(workspace.AccessRead, path)
	if err != nil {
		return "", false
	}
	return decision.ResolvedPath, true
}
