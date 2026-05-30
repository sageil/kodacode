package events

import "time"

type BranchSummaryArtifact struct {
	SessionID        string
	SourceSequence   int64
	Summary          string
	Model            string
	PromptTokens     int
	CompletionTokens int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
