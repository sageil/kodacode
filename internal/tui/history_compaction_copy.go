package tui

const (
	historyCompactionCardTitle     = "History Summary"
	historyCompactionSummaryHeader = "Compaction Summary:"
	historySummarizingStatusLabel  = "Summarizing History"
	historyPrunedSignal            = "history pruned"
	historySummaryFailureLabel     = "History summary update failed"
	compactionFailureTraceLabel    = "Compaction failure"
	historySummaryTraceLabel       = "History compaction"
	historySummarizingTraceDetail  = "History summarization: summarizing older stored turns to reduce future context pressure. Current request size may be lower because history has already been shaped for this step."
)

var historyCompactionSummaryHeaders = []string{
	historyCompactionSummaryHeader,
	"History Continuation:",
	historyCompactionCardTitle + ":",
}
