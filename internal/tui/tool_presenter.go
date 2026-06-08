package tui

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

type toolPresenter struct {
	Names           []string
	Category        func(*events.ToolCallState) toolOutcomeKind
	DisplayName     func(workspaceRoot string, call *events.ToolCallState) string
	InspectorParams func(call *events.ToolCallState) []inspectorParam
	ListSummary     func(call *events.ToolCallState) string
	ResultDetail    func(call *events.ToolCallState) string
	MutationPaths   func(call *events.ToolCallState) []string
}

func allToolPresenters() []toolPresenter {
	return []toolPresenter{{
		Names:           []string{"read"},
		Category:        fixedToolCategory(toolOutcomeExploration),
		DisplayName:     readToolDisplayName,
		InspectorParams: readInspectorParams,
		ListSummary:     readToolListSummary,
		ResultDetail:    readToolResultDetail,
	}, {
		Names:           []string{"list"},
		Category:        fixedToolCategory(toolOutcomeExploration),
		InspectorParams: listInspectorParams,
		ListSummary:     listToolListSummary,
		ResultDetail:    listToolResultDetail,
	}, {
		Names:           []string{"tree"},
		Category:        fixedToolCategory(toolOutcomeExploration),
		InspectorParams: treeInspectorParams,
		ListSummary:     treeToolListSummary,
		ResultDetail:    treeToolResultDetail,
	}, {
		Names:           []string{"git_status"},
		Category:        fixedToolCategory(toolOutcomeExploration),
		InspectorParams: gitStatusInspectorParams,
		ListSummary:     gitStatusToolListSummary,
		ResultDetail:    gitStatusToolResultDetail,
	}, {
		Names:           []string{"git_diff"},
		Category:        fixedToolCategory(toolOutcomeExploration),
		InspectorParams: gitDiffInspectorParams,
		ListSummary:     gitDiffToolListSummary,
		ResultDetail:    gitDiffToolResultDetail,
	}, {
		Names:           []string{"git_show"},
		Category:        fixedToolCategory(toolOutcomeExploration),
		InspectorParams: gitShowInspectorParams,
		ListSummary:     gitShowToolListSummary,
		ResultDetail:    gitShowToolResultDetail,
	}, {
		Names:           []string{"search"},
		Category:        fixedToolCategory(toolOutcomeExploration),
		DisplayName:     searchToolDisplayName,
		InspectorParams: searchInspectorParams,
		ListSummary:     searchToolListSummary,
		ResultDetail:    searchToolResultDetail,
	}, {
		Names:           []string{"web_search"},
		InspectorParams: webSearchInspectorParams,
		ListSummary:     webSearchToolListSummary,
		ResultDetail:    webSearchToolResultDetail,
	}, {
		Names:       []string{"web_fetch"},
		Category:    fixedToolCategory(toolOutcomeExploration),
		DisplayName: func(_ string, call *events.ToolCallState) string { return webFetchToolDisplayName(call) },
		ListSummary: webFetchToolListSummary,
	}, {
		Names:           []string{"definition"},
		Category:        fixedToolCategory(toolOutcomeExploration),
		DisplayName:     definitionToolDisplayName,
		InspectorParams: definitionInspectorParams,
		ListSummary:     definitionToolListSummary,
	}, {
		Names:           []string{"diagnostics"},
		Category:        fixedToolCategory(toolOutcomeExploration),
		DisplayName:     diagnosticsToolDisplayName,
		InspectorParams: diagnosticsInspectorParams,
		ListSummary:     diagnosticsToolListSummary,
	}, {
		Names:           []string{"locate"},
		Category:        fixedToolCategory(toolOutcomeExploration),
		DisplayName:     locateToolDisplayName,
		InspectorParams: locateInspectorParams,
		ListSummary:     locateToolListSummary,
		ResultDetail:    locateToolResultDetail,
	}, {
		Names:           []string{"symbols"},
		Category:        fixedToolCategory(toolOutcomeExploration),
		DisplayName:     func(_ string, call *events.ToolCallState) string { return symbolsToolDisplayName(call) },
		InspectorParams: symbolsInspectorParams,
		ListSummary:     symbolsToolListSummary,
	}, {
		Names:           []string{"refs"},
		Category:        fixedToolCategory(toolOutcomeExploration),
		DisplayName:     refsToolDisplayName,
		InspectorParams: refsInspectorParams,
		ListSummary:     refsToolListSummary,
		ResultDetail:    refsToolResultDetail,
	}, {
		Names:           []string{"trace"},
		Category:        fixedToolCategory(toolOutcomeExploration),
		DisplayName:     traceToolDisplayName,
		InspectorParams: traceInspectorParams,
		ListSummary:     traceToolListSummary,
		ResultDetail:    traceToolResultDetail,
	}, {
		Names:           []string{"rename_symbol"},
		Category:        fixedToolCategory(toolOutcomeMutation),
		DisplayName:     renameSymbolToolDisplayName,
		InspectorParams: renameSymbolInspectorParams,
		ListSummary:     renameSymbolToolListSummary,
		MutationPaths:   renameSymbolToolMutationPaths,
	}, {
		Names:           []string{"code_action"},
		Category:        fixedToolCategory(toolOutcomeMutation),
		DisplayName:     codeActionToolDisplayName,
		InspectorParams: codeActionInspectorParams,
		ListSummary:     codeActionToolListSummary,
		MutationPaths:   codeActionToolMutationPaths,
	}, {
		Names:           []string{"search_skills"},
		Category:        fixedToolCategory(toolOutcomeExploration),
		DisplayName:     func(_ string, call *events.ToolCallState) string { return searchSkillsToolDisplayName(call) },
		InspectorParams: searchSkillsInspectorParams,
		ListSummary:     searchSkillsToolListSummary,
	}, {
		Names:           []string{"skill"},
		DisplayName:     func(_ string, call *events.ToolCallState) string { return skillToolDisplayName(call) },
		InspectorParams: skillInspectorParams,
		ListSummary:     skillToolListSummary,
	}, {
		Names:           []string{"task", "task_workflow", "task_review"},
		DisplayName:     func(_ string, call *events.ToolCallState) string { return taskToolDisplayName(call) },
		InspectorParams: taskInspectorParams,
		ListSummary:     taskToolListSummary,
	}, {
		Names:           []string{"memory"},
		DisplayName:     func(_ string, call *events.ToolCallState) string { return memoryToolDisplayName(call) },
		InspectorParams: memoryInspectorParams,
		ListSummary:     memoryToolListSummary,
	}, {
		Names:           []string{"bash"},
		Category:        bashToolCategory,
		DisplayName:     bashToolDisplayName,
		InspectorParams: bashInspectorParams,
		ListSummary:     bashToolListSummary,
		MutationPaths:   bashToolMutationPaths,
	}, {
		Names:           []string{"test"},
		Category:        fixedToolCategory(toolOutcomeCommand),
		DisplayName:     testToolDisplayName,
		InspectorParams: testInspectorParams,
		ListSummary:     testToolListSummary,
	}, {
		Names:           []string{"write"},
		Category:        fixedToolCategory(toolOutcomeMutation),
		DisplayName:     writeToolDisplayName,
		InspectorParams: writeInspectorParams,
		ListSummary:     writeToolListSummary,
		MutationPaths:   writeToolMutationPaths,
	}, {
		Names:           []string{"apply_patch"},
		Category:        fixedToolCategory(toolOutcomeMutation),
		DisplayName:     applyPatchToolDisplayName,
		InspectorParams: applyPatchInspectorParams,
		ListSummary:     applyPatchToolListSummary,
		MutationPaths:   applyPatchToolMutationPaths,
	}, {
		Names:           []string{"mkdir"},
		Category:        fixedToolCategory(toolOutcomeMutation),
		InspectorParams: mkdirInspectorParams,
		ListSummary:     mkdirToolListSummary,
		MutationPaths:   mkdirToolMutationPaths,
	}, {
		Names:       []string{"question"},
		DisplayName: func(_ string, call *events.ToolCallState) string { return questionToolDisplayName(call) },
	}}
}

var (
	toolPresenterRegistryOnce sync.Once
	toolPresentersByName      map[string]toolPresenter
)

func buildToolPresenterRegistry(presenters []toolPresenter) map[string]toolPresenter {
	registry := make(map[string]toolPresenter)
	for _, presenter := range presenters {
		for _, name := range presenter.Names {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if _, exists := registry[name]; exists {
				panic("duplicate TUI tool presenter: " + name)
			}
			registry[name] = presenter
		}
	}
	return registry
}

func registeredToolPresenterNames() []string {
	ensureToolPresenterRegistry()
	names := make([]string, 0, len(toolPresentersByName))
	for name := range toolPresentersByName {
		names = append(names, name)
	}
	return names
}

func toolPresenterForCall(call *events.ToolCallState) (toolPresenter, bool) {
	if call == nil {
		return toolPresenter{}, false
	}
	ensureToolPresenterRegistry()
	presenter, ok := toolPresentersByName[strings.TrimSpace(call.ToolName)]
	return presenter, ok
}

func ensureToolPresenterRegistry() {
	toolPresenterRegistryOnce.Do(func() {
		toolPresentersByName = buildToolPresenterRegistry(allToolPresenters())
	})
}

func fixedToolCategory(kind toolOutcomeKind) func(*events.ToolCallState) toolOutcomeKind {
	return func(*events.ToolCallState) toolOutcomeKind {
		return kind
	}
}

func bashToolCategory(call *events.ToolCallState) toolOutcomeKind {
	if len(bashToolMutationPaths(call)) > 0 {
		return toolOutcomeMutation
	}
	return toolOutcomeExploration
}

func writeToolMutationPaths(call *events.ToolCallState) []string {
	if call == nil {
		return nil
	}
	input, ok := parseWriteToolViewInput(call.Input)
	if !ok {
		return nil
	}
	return singleMutationPath(input.Path)
}

func mkdirToolMutationPaths(call *events.ToolCallState) []string {
	if call == nil {
		return nil
	}
	input, ok := parseMkdirToolViewInput(call.Input)
	if !ok {
		return nil
	}
	return singleMutationPath(input.Path)
}

func renameSymbolToolMutationPaths(call *events.ToolCallState) []string {
	if call == nil {
		return nil
	}
	input, ok := parseRenameSymbolToolViewInput(call.Input)
	if !ok {
		return nil
	}
	return singleMutationPath(input.Path)
}

func codeActionToolMutationPaths(call *events.ToolCallState) []string {
	if call == nil {
		return nil
	}
	input, ok := parseCodeActionToolViewInput(call.Input)
	if !ok {
		return nil
	}
	return singleMutationPath(input.Path)
}

func bashToolMutationPaths(call *events.ToolCallState) []string {
	if call == nil || call.WriteMutation == nil {
		return nil
	}
	return singleMutationPath(call.WriteMutation.Path)
}

func applyPatchToolMutationPaths(call *events.ToolCallState) []string {
	if call == nil {
		return nil
	}
	mutations := call.WriteMutations
	if len(mutations) == 0 && call.WriteMutation != nil {
		mutations = []events.WriteMutation{*call.WriteMutation}
	}
	paths := make([]string, 0, len(mutations))
	for _, mutation := range mutations {
		if path := normalizeMutationPath(mutation.Path); path != "" {
			paths = append(paths, path)
		}
	}
	if len(paths) > 0 {
		return paths
	}
	patch, err := tool.ParseApplyPatch(call.Input)
	if err == nil {
		for _, op := range patch.Operations {
			paths = appendNormalizedMutationPath(paths, op.Path)
			paths = appendNormalizedMutationPath(paths, op.MovePath)
		}
		return paths
	}
	return applyPatchToolHeaderPaths(call.Input)
}

func singleMutationPath(path string) []string {
	path = normalizeMutationPath(path)
	if path == "" {
		return nil
	}
	return []string{path}
}

func appendNormalizedMutationPath(paths []string, path string) []string {
	path = normalizeMutationPath(path)
	if path == "" {
		return paths
	}
	for _, existing := range paths {
		if existing == path {
			return paths
		}
	}
	return append(paths, path)
}

func applyPatchToolHeaderPaths(input string) []string {
	patch := applyPatchToolInputText(input)
	patch = strings.ReplaceAll(patch, "\r\n", "\n")
	patch = strings.ReplaceAll(patch, "\r", "\n")
	var paths []string
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "*** Add File: "):
			paths = appendNormalizedMutationPath(paths, strings.TrimPrefix(line, "*** Add File: "))
		case strings.HasPrefix(line, "*** Delete File: "):
			paths = appendNormalizedMutationPath(paths, strings.TrimPrefix(line, "*** Delete File: "))
		case strings.HasPrefix(line, "*** Update File: "):
			paths = appendNormalizedMutationPath(paths, strings.TrimPrefix(line, "*** Update File: "))
		case strings.HasPrefix(line, "*** Move to: "):
			paths = appendNormalizedMutationPath(paths, strings.TrimPrefix(line, "*** Move to: "))
		}
	}
	return paths
}

func applyPatchToolInputText(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || !strings.HasPrefix(trimmed, "{") {
		return input
	}
	var args struct {
		Patch string `json:"patch"`
	}
	if err := json.Unmarshal([]byte(trimmed), &args); err != nil || strings.TrimSpace(args.Patch) == "" {
		return input
	}
	return args.Patch
}
