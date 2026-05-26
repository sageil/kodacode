package tui

import "time"

type transcriptRefreshKind uint8

const (
	transcriptRefreshNone transcriptRefreshKind = iota
	transcriptRefreshStructure
	transcriptRefreshTurns
)

type transcriptRefreshPlan struct {
	kind    transcriptRefreshKind
	turnIDs []string
}

const transcriptRefreshThrottle = 66 * time.Millisecond
