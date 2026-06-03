package tool

// DefaultRuntimeTools is the baseline production tool surface exposed by
// kodacode before any config-gated runtime tools are appended.
func DefaultRuntimeTools() []Tool {
	return []Tool{
		NewApplyPatchTool(),
		NewBashTool(),
		NewCodeActionTool(),
		NewDelegateTool(),
		NewDefinitionTool(),
		NewDiagnosticsTool(),
		NewGitDiffTool(),
		NewGitShowTool(),
		NewGitStatusTool(),
		NewLocateTool(),
		NewMemoryTool(),
		NewMkdirTool(),
		NewQuestionTool(),
		NewReadTool(),
		NewRefsTool(),
		NewRenameSymbolTool(),
		NewSearchTool(),
		NewSearchSkillsTool(),
		NewSkillTool(),
		NewSymbolsTool(),
		NewTaskReviewTool(),
		NewTaskWorkflowTool(),
		NewTestTool(),
		NewTraceTool(),
		NewWebFetchTool(),
		NewWorkflowPhaseOutputTool(),
		NewWorkflowReviewResultTool(),
		NewWriteTool(),
	}
}

// AllBuiltInTools returns the full built-in tool catalog, including
// config-gated tools that are not always registered in the runtime surface.
// Use this for schema and argument-metadata lookups, not runtime registration.
func AllBuiltInTools() []Tool {
	tools := append([]Tool(nil), DefaultRuntimeTools()...)
	tools = append(tools, NewWebSearchTool())
	return tools
}
