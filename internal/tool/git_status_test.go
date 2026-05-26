package tool

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestParseGitStatusInputRejectsUnexpectedFieldsClearly(t *testing.T) {
	err := parseGitStatusInput(json.RawMessage(`{"path":""}`))
	if err == nil {
		t.Fatal("parseGitStatusInput() error = nil")
	}
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("errors.Is(err, ErrInvalidArguments) = false, err = %v", err)
	}
	got := err.Error()
	for _, want := range []string{
		"`git_status` failed.",
		`git_status takes no fields like "path"`,
		`Use an empty object {} or omit arguments entirely.`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("err.Error() = %q, missing %q", got, want)
		}
	}
}

func TestParseGitStatusInputAllowsEmptyObjectOrOmittedArguments(t *testing.T) {
	for _, raw := range []json.RawMessage{
		nil,
		json.RawMessage(`null`),
		json.RawMessage(`{}`),
	} {
		if err := parseGitStatusInput(raw); err != nil {
			t.Fatalf("parseGitStatusInput(%q) error = %v", string(raw), err)
		}
	}
}
