package app

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
	"github.com/sageil/kodacode/internal/workspace"
)

var (
	ErrWriteRequiresRead         = errors.New("existing-file write requires full-file replacement context")
	ErrWriteRequiresCompleteRead = errors.New("existing-file write requires complete file coverage")
	ErrWriteRequiresFreshRead    = errors.New("existing-file write replacement context is stale")
)

type writeReadStatus int

const (
	writeReadStatusOK writeReadStatus = iota
	writeReadStatusMissing
	writeReadStatusPartial
	writeReadStatusStale
)

type observedLineInterval struct {
	start int
	end   int
}

func requireCompleteFreshReadBeforeWrite(state events.SessionState, scope *workspace.Scope, input ExecuteToolInput) error {
	if scope == nil || strings.TrimSpace(input.ToolName) != tool.WriteToolName {
		return nil
	}
	args, err := parseWriteToolArguments(input.Arguments)
	if err != nil || strings.TrimSpace(args.Path) == "" {
		return nil
	}

	decision, err := scope.Check(workspace.AccessWrite, args.Path)
	if err != nil {
		return err
	}
	info, err := os.Stat(decision.ResolvedPath)
	switch {
	case os.IsNotExist(err):
		return nil
	case err != nil:
		return err
	case info.IsDir():
		return nil
	}

	status, currentObservations, err := existingFileWriteReadStatus(state, decision.ResolvedPath)
	if err != nil {
		return err
	}
	switch status {
	case writeReadStatusOK:
		return nil
	case writeReadStatusPartial:
		coverage := describeObservedFileReadCoverage(currentObservations)
		return fmt.Errorf("%w: existing-file write replaces the entire file body, but available file coverage is incomplete for %s: %s. Read the complete current file with read, then retry write with the complete final file content", ErrWriteRequiresCompleteRead, args.Path, coverage)
	case writeReadStatusStale:
		return fmt.Errorf("%w: %s changed since full-file replacement context was captured. Read the complete current file with read, then retry write with the complete final file content", ErrWriteRequiresFreshRead, args.Path)
	default:
		return fmt.Errorf("%w: existing-file write replaces the entire file body and no full-file replacement context is available for %s. Read the complete current file with read, then retry write with the complete final file content", ErrWriteRequiresRead, args.Path)
	}
}

func existingFileWriteReadStatus(state events.SessionState, resolvedPath string) (writeReadStatus, []events.ObservedResource, error) {
	observations := textMutationObservationsForResolvedPath(state, resolvedPath)
	if len(observations) == 0 {
		return writeReadStatusMissing, nil, nil
	}

	currentVersion, versionOK, err := tool.CurrentObservedResourceVersion(tool.ObservedResourceFileContent, resolvedPath)
	if err != nil {
		return writeReadStatusMissing, nil, err
	}
	currentState, stateOK, err := tool.CurrentObservedResourceState(tool.ObservedResourceFileContent, resolvedPath)
	if err != nil {
		return writeReadStatusMissing, nil, err
	}

	current := make([]events.ObservedResource, 0, len(observations))
	hadStale := false
	for _, observation := range observations {
		if observedFileReadMatchesCurrent(observation, currentVersion, versionOK, currentState, stateOK) {
			current = append(current, observation)
			continue
		}
		hadStale = true
	}

	if observedFileReadsCoverWholeFile(current) {
		return writeReadStatusOK, current, nil
	}
	if len(current) > 0 {
		return writeReadStatusPartial, current, nil
	}
	if hadStale {
		return writeReadStatusStale, nil, nil
	}
	return writeReadStatusMissing, nil, nil
}

func observedFileReadMatchesCurrent(observation events.ObservedResource, currentVersion string, versionOK bool, currentState string, stateOK bool) bool {
	if strings.TrimSpace(observation.Path) == "" || strings.TrimSpace(observation.Version) == "" {
		return false
	}
	if stateOK {
		if observationState := strings.TrimSpace(observation.State); observationState != "" {
			return observationState == strings.TrimSpace(currentState)
		}
	}
	if !versionOK {
		return false
	}
	return strings.TrimSpace(observation.Version) == strings.TrimSpace(currentVersion)
}

func observedFileReadsCoverWholeFile(observations []events.ObservedResource) bool {
	intervals, totalLines, complete := observedFileReadCoverage(observations)
	if complete {
		return true
	}
	if totalLines == 0 || len(intervals) == 0 {
		return false
	}

	coveredEnd := 0
	for _, interval := range intervals {
		if interval.start > coveredEnd+1 {
			return false
		}
		if interval.end > coveredEnd {
			coveredEnd = interval.end
		}
		if coveredEnd >= totalLines {
			return true
		}
	}
	return coveredEnd >= totalLines
}

func observedFileReadCoverage(observations []events.ObservedResource) ([]observedLineInterval, int, bool) {
	if len(observations) == 0 {
		return nil, 0, false
	}
	for _, observation := range observations {
		if observation.Complete {
			return []observedLineInterval{{start: 1, end: observation.TotalLines}}, observation.TotalLines, true
		}
	}

	intervals := make([]observedLineInterval, 0, len(observations))
	totalLines := 0
	for _, observation := range observations {
		if observation.TotalLines <= 0 || observation.StartLine <= 0 || observation.EndLine < observation.StartLine {
			continue
		}
		if totalLines == 0 {
			totalLines = observation.TotalLines
		} else if observation.TotalLines != totalLines {
			return nil, 0, false
		}
		intervals = append(intervals, observedLineInterval{
			start: observation.StartLine,
			end:   observation.EndLine,
		})
	}
	if totalLines == 0 || len(intervals) == 0 {
		return nil, 0, false
	}

	sort.Slice(intervals, func(i, j int) bool {
		if intervals[i].start == intervals[j].start {
			return intervals[i].end < intervals[j].end
		}
		return intervals[i].start < intervals[j].start
	})

	merged := make([]observedLineInterval, 0, len(intervals))
	for _, interval := range intervals {
		if len(merged) == 0 {
			merged = append(merged, interval)
			continue
		}
		last := &merged[len(merged)-1]
		if interval.start <= last.end+1 {
			if interval.end > last.end {
				last.end = interval.end
			}
			continue
		}
		merged = append(merged, interval)
	}
	return merged, totalLines, false
}

func describeObservedFileReadCoverage(observations []events.ObservedResource) string {
	intervals, totalLines, complete := observedFileReadCoverage(observations)
	switch {
	case complete && totalLines == 0:
		return "the empty file"
	case complete:
		return fmt.Sprintf("the full file (%d lines)", totalLines)
	case totalLines == 0 || len(intervals) == 0:
		return "no current lines"
	}

	segments := make([]string, 0, len(intervals))
	for _, interval := range intervals {
		if interval.start == interval.end {
			segments = append(segments, fmt.Sprintf("%d", interval.start))
			continue
		}
		segments = append(segments, fmt.Sprintf("%d-%d", interval.start, interval.end))
	}
	label := "lines"
	if len(intervals) == 1 && intervals[0].start == intervals[0].end {
		label = "line"
	}
	return fmt.Sprintf("%s %s of %d", label, joinCoverageSpans(segments), totalLines)
}

func joinCoverageSpans(spans []string) string {
	switch len(spans) {
	case 0:
		return ""
	case 1:
		return spans[0]
	case 2:
		return spans[0] + " and " + spans[1]
	default:
		return strings.Join(spans[:len(spans)-1], ", ") + ", and " + spans[len(spans)-1]
	}
}
