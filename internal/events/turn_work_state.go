package events

import (
	"errors"
	"fmt"
	"strings"
)

type TurnWorkStateSummaryPayload struct {
	Objective     string
	Decisions     []string
	TouchedPaths  []string
	CompletedWork []string
	Verification  []string
	Failures      []string
	OpenItems     []string
}

func (p TurnWorkStateSummaryPayload) validate() error {
	for index, value := range []struct {
		name   string
		values []string
	}{
		{name: "decisions", values: p.Decisions},
		{name: "touched_paths", values: p.TouchedPaths},
		{name: "completed_work", values: p.CompletedWork},
		{name: "verification", values: p.Verification},
		{name: "failures", values: p.Failures},
		{name: "open_items", values: p.OpenItems},
	} {
		for inner, entry := range value.values {
			if strings.TrimSpace(entry) == "" {
				return fmt.Errorf("%s[%d] must not be empty", value.name, inner)
			}
		}
		_ = index
	}
	return nil
}

type TurnNativeContinuationPayload struct {
	Contract string
	Slice    SessionHistoryTurnPayload
}

func (p TurnNativeContinuationPayload) validate() error {
	if strings.TrimSpace(p.Contract) == "" {
		return errors.New("contract is required")
	}
	if err := p.Slice.validate(); err != nil {
		return fmt.Errorf("slice: %w", err)
	}
	if len(p.Slice.EntryOrder) == 0 {
		return errors.New("slice.entry_order is required")
	}
	return nil
}

type TurnWorkStateUpdatedPayload struct {
	Summary            TurnWorkStateSummaryPayload
	NativeContinuation *TurnNativeContinuationPayload
}

func (TurnWorkStateUpdatedPayload) eventType() Type { return TypeTurnWorkStateUpdated }

func (p TurnWorkStateUpdatedPayload) validate() error {
	if err := p.Summary.validate(); err != nil {
		return fmt.Errorf("summary: %w", err)
	}
	if p.NativeContinuation != nil {
		if err := p.NativeContinuation.validate(); err != nil {
			return fmt.Errorf("native_continuation: %w", err)
		}
	}
	return nil
}

type TurnWorkStateSummaryState struct {
	Objective     string
	Decisions     []string
	TouchedPaths  []string
	CompletedWork []string
	Verification  []string
	Failures      []string
	OpenItems     []string
}

type TurnNativeContinuationState struct {
	Contract string
	Slice    SessionHistoryTurnPayload
}

type TurnWorkState struct {
	Summary            TurnWorkStateSummaryState
	NativeContinuation *TurnNativeContinuationState
}
