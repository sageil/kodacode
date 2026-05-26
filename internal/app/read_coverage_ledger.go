package app

import (
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

type readCoverageRecord struct {
	SessionID  string
	TurnID     string
	CallID     string
	Path       string
	SourceTool string
	StartLine  int
	EndLine    int
	TotalLines int
	Complete   bool
	Version    string
	State      string

	resource events.ObservedResource
}

type readCoverageLedger struct {
	state events.SessionState
}

func newReadCoverageLedger(state events.SessionState) readCoverageLedger {
	return readCoverageLedger{state: state}
}

func (l readCoverageLedger) observedResourcesForPath(resolvedPath string) []events.ObservedResource {
	resolvedPath = strings.TrimSpace(resolvedPath)
	if resolvedPath == "" {
		return nil
	}
	var out []events.ObservedResource
	for i := len(l.state.TurnOrder) - 1; i >= 0; i-- {
		turnID := l.state.TurnOrder[i]
		turn := l.state.Turns[turnID]
		if turn == nil {
			continue
		}
		for j := len(turn.ToolCallOrder) - 1; j >= 0; j-- {
			call := turn.ToolCalls[turn.ToolCallOrder[j]]
			for _, record := range l.recordsForCall(turnID, call) {
				if strings.TrimSpace(record.Path) == resolvedPath {
					out = append(out, record.resource)
				}
			}
		}
	}
	return out
}

func (l readCoverageLedger) recordsForCall(turnID string, call *events.ToolCallState) []readCoverageRecord {
	if !callProvidesCurrentTextObservation(call) {
		return nil
	}
	records := make([]readCoverageRecord, 0, len(call.ObservedResources))
	for _, resource := range call.ObservedResources {
		if strings.TrimSpace(resource.Kind) != string(tool.ObservedResourceFileContent) {
			continue
		}
		if strings.TrimSpace(resource.Path) == "" || strings.TrimSpace(resource.Version) == "" {
			continue
		}
		if !resource.Complete && (resource.StartLine <= 0 || resource.EndLine < resource.StartLine) {
			continue
		}
		records = append(records, readCoverageRecord{
			SessionID:  l.state.SessionID,
			TurnID:     turnID,
			CallID:     call.CallID,
			Path:       resource.Path,
			SourceTool: strings.TrimSpace(call.ToolName),
			StartLine:  resource.StartLine,
			EndLine:    resource.EndLine,
			TotalLines: resource.TotalLines,
			Complete:   resource.Complete,
			Version:    resource.Version,
			State:      resource.State,
			resource:   resource,
		})
	}
	return records
}
