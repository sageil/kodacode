package tool

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	testDefaultTimeoutMS = 120000
	testMinTimeoutMS     = 5000
	testMaxTimeoutMS     = 30 * 60 * 1000
)

var (
	ErrTestPathRequired          = errors.New("path must not be empty")
	ErrTestTimeoutInvalid        = fmt.Errorf("timeout must be between %d and %d milliseconds; timeout uses milliseconds, so 90000 means 90 seconds and 600 means 0.6 seconds", testMinTimeoutMS, testMaxTimeoutMS)
	ErrTestFilterUnsupported     = errors.New("filter is not supported for the selected test framework")
	ErrTestPathTargetUnsupported = errors.New("path targeting is not supported for the selected test framework")
	ErrTestWatchModeUnsupported  = errors.New("test only supports one-shot commands, not watcher or server modes")
)

type testInput struct {
	Command   string
	Path      string
	Filter    string
	TimeoutMS int
}

func parseTestInput(args json.RawMessage) (_ testInput, err error) {
	defer func() {
		err = normalizeToolInputError(TestToolName, err)
	}()
	var raw struct {
		Command *string         `json:"command"`
		Path    *string         `json:"path"`
		Filter  *string         `json:"filter"`
		Timeout json.RawMessage `json:"timeout"`
	}
	if err := DecodeArgs(TestToolName, args, &raw); err != nil {
		return testInput{}, err
	}

	input := testInput{
		Command: strings.TrimSpace(stringValue(raw.Command)),
		Path:    strings.TrimSpace(stringValue(raw.Path)),
		Filter:  strings.TrimSpace(stringValue(raw.Filter)),
	}
	if timeout, ok, err := decodeOptionalIntArg(TestToolName, raw.Timeout, "timeout"); err != nil {
		return testInput{}, err
	} else if ok {
		if timeout < testMinTimeoutMS || timeout > testMaxTimeoutMS {
			return testInput{}, ErrTestTimeoutInvalid
		}
		input.TimeoutMS = timeout
	} else {
		input.TimeoutMS = testDefaultTimeoutMS
	}
	if raw.Path != nil && input.Path == "" {
		return testInput{}, ErrTestPathRequired
	}
	if input.Path == "" {
		input.Path = "."
	}
	return input, nil
}
