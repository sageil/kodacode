package app

const (
	compactionSummaryBudgetBytes      = 4 * 1024
	compactionExistingSummaryMaxBytes = 768
	compactionUtilityExistingMaxBytes = 2 * 1024
	compactionUtilityToolOutputMaxLen = 2000
	sessionHistoryCompactionReason    = "token_pressure"
	sessionHistorySemanticFrontier    = 2

	compactionTurnUserTextMaxBytes      = 240
	compactionTurnAssistantMaxBytes     = 320
	compactionTurnAssistantLineLimit    = 8
	compactionTurnRuntimeNoteLimit      = 3
	compactionTurnRuntimeNoteMaxBytes   = 240
	compactionTurnWorkspacePathLimit    = 8
	compactionTurnWorkspacePathMaxBytes = 160
	compactionSummaryWorkspacePathLimit = 12
	compactionSummaryWorkspacePathBytes = 240
	compactionTurnFailedToolLimit       = 6
	compactionTurnFailedToolMaxBytes    = 64
	compactionTurnTerminalErrorMaxBytes = 320

	compactionSummaryConstraintLimit = 4
	compactionSummaryDoneLimit       = 4
	compactionSummaryInProgressLimit = 4
	compactionSummaryBlockedLimit    = 4
	compactionSummaryDecisionLimit   = 4
	compactionSummaryNextLimit       = 3
	compactionSummaryCriticalLimit   = 6
	compactionSummaryFileLimit       = 4
)

const historyContinuationSummaryHeader = "Compaction Summary:"
