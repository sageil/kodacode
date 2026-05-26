package tool

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestDecodeArgsNormalizesTypeMismatch(t *testing.T) {
	var input struct {
		IncludeHidden *bool `json:"include_hidden"`
	}
	err := DecodeArgs(LocateToolName, json.RawMessage(`{"include_hidden":"false"}`), &input)
	if err == nil {
		t.Fatal("DecodeArgs() error = nil, want invalid arguments error")
	}
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("errors.Is(err, ErrInvalidArguments) = false, err = %v", err)
	}
	got := err.Error()
	for _, want := range []string{
		"`locate` failed.",
		`Example: {"query":"*.go","path":"src","include_hidden":false,"max_matches":20}.`,
		"include_hidden must be a boolean",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("err.Error() = %q, missing %q", got, want)
		}
	}
}

func TestDecodeArgsNormalizesMalformedJSON(t *testing.T) {
	var input struct {
		Paths []string `json:"paths"`
	}
	err := DecodeArgs(ReadToolName, json.RawMessage(`{"paths":`), &input)
	if err == nil {
		t.Fatal("DecodeArgs() error = nil, want malformed arguments error")
	}
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("errors.Is(err, ErrInvalidArguments) = false, err = %v", err)
	}
	got := err.Error()
	for _, want := range []string{
		"`read` failed.",
		`Use either path for one file or paths for one or more files; do not send both.`,
		"JSON ended before the object was complete",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("err.Error() = %q, missing %q", got, want)
		}
	}
}

func TestDecodeArgsReportsUnquotedStringValuesClearly(t *testing.T) {
	var input struct {
		Title string `json:"title"`
	}
	err := DecodeArgs(TaskWorkflowToolName, json.RawMessage(`{"action":"create","title": Fix invalidateeAllUserSessions with reverse index"}`), &input)
	if err == nil {
		t.Fatal("DecodeArgs() error = nil, want malformed arguments error")
	}
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("errors.Is(err, ErrInvalidArguments) = false, err = %v", err)
	}
	got := err.Error()
	if !strings.Contains(got, `"title" has an unquoted string value; wrap it in double quotes`) {
		t.Fatalf("err.Error() = %q, want unquoted string guidance", got)
	}
	if strings.Contains(got, "invalid character 'F' looking for beginning of value") {
		t.Fatalf("err.Error() = %q, unexpectedly leaked raw parser detail", got)
	}
}

func TestDecodeOptionalBoolArgTreatsStringNullAsMissing(t *testing.T) {
	value, ok, err := decodeOptionalBoolArg(BashToolName, json.RawMessage(`" NULL "`), "tty")
	if err != nil {
		t.Fatalf("decodeOptionalBoolArg() error = %v", err)
	}
	if ok {
		t.Fatalf("decodeOptionalBoolArg() ok = true, want false")
	}
	if value {
		t.Fatalf("decodeOptionalBoolArg() value = true, want false")
	}
}

func TestDecodeOptionalIntArgTreatsStringNullAsMissing(t *testing.T) {
	value, ok, err := decodeOptionalIntArg(ReadToolName, json.RawMessage(`" null "`), "start_line")
	if err != nil {
		t.Fatalf("decodeOptionalIntArg() error = %v", err)
	}
	if ok {
		t.Fatalf("decodeOptionalIntArg() ok = true, want false")
	}
	if value != 0 {
		t.Fatalf("decodeOptionalIntArg() value = %d, want 0", value)
	}
}

func TestDecodeOptionalStringArrayArgAcceptsStringifiedJSONArray(t *testing.T) {
	values, ok, err := decodeOptionalStringArrayArg(ReadToolName, json.RawMessage(`"[\"README.md\",\"docs/intro.md\"]"`), "paths")
	if err != nil {
		t.Fatalf("decodeOptionalStringArrayArg() error = %v", err)
	}
	if !ok {
		t.Fatal("decodeOptionalStringArrayArg() ok = false, want true")
	}
	if got, want := len(values), 2; got != want {
		t.Fatalf("len(values) = %d, want %d", got, want)
	}
	if values[0] != "README.md" || values[1] != "docs/intro.md" {
		t.Fatalf("values = %#v", values)
	}
}

func TestDecodeOptionalStringArrayArgAcceptsBareStringAsSingleton(t *testing.T) {
	values, ok, err := decodeOptionalStringArrayArg(ReadToolName, json.RawMessage(`"README.md"`), "paths")
	if err != nil {
		t.Fatalf("decodeOptionalStringArrayArg() error = %v", err)
	}
	if !ok {
		t.Fatal("decodeOptionalStringArrayArg() ok = false, want true")
	}
	if got, want := len(values), 1; got != want {
		t.Fatalf("len(values) = %d, want %d", got, want)
	}
	if values[0] != "README.md" {
		t.Fatalf("values = %#v", values)
	}
}

func TestTaskWorkflowMalformedArgumentsMessageExplainsExpectedShape(t *testing.T) {
	var input struct {
		Action string `json:"action"`
	}
	err := DecodeArgs(TaskWorkflowToolName, json.RawMessage(`{"action":`), &input)
	if err == nil {
		t.Fatal("DecodeArgs() error = nil, want malformed arguments error")
	}
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("errors.Is(err, ErrInvalidArguments) = false, err = %v", err)
	}
	got := err.Error()
	for _, want := range []string{
		"`task_workflow` failed.",
		`Use action "list", "create", "update", "block", or "complete"`,
		"JSON ended before the object was complete",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("err.Error() = %q, missing %q", got, want)
		}
	}
}

func TestTaskReviewInvalidArgumentsMessageExplainsActionRequirements(t *testing.T) {
	err := InvalidArguments(TaskReviewToolName, ErrTaskReviewStatusInvalid)
	if err == nil {
		t.Fatal("InvalidArguments() error = nil")
	}
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("errors.Is(err, ErrInvalidArguments) = false, err = %v", err)
	}
	got := err.Error()
	for _, want := range []string{
		"`task_review` failed.",
		`Use action "list" or "review"`,
		"review_status must be pass, concern, fail, or accepted",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("err.Error() = %q, missing %q", got, want)
		}
	}
}

func TestTaskReviewMalformedArgumentsMessageExplainsExpectedShape(t *testing.T) {
	var input struct {
		Action string `json:"action"`
	}
	err := DecodeArgs(TaskReviewToolName, json.RawMessage(`{"action":`), &input)
	if err == nil {
		t.Fatal("DecodeArgs() error = nil, want malformed arguments error")
	}
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("errors.Is(err, ErrInvalidArguments) = false, err = %v", err)
	}
	got := err.Error()
	for _, want := range []string{
		"`task_review` failed.",
		`Use action "list" or "review"`,
		"JSON ended before the object was complete",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("err.Error() = %q, missing %q", got, want)
		}
	}
}

func TestParseDefinitionInputWrapsValidationErrorWithExample(t *testing.T) {
	_, err := parseDefinitionInput(json.RawMessage(`{"path":"","line":1,"character":0}`))
	if err == nil {
		t.Fatal("parseDefinitionInput() error = nil")
	}
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("errors.Is(err, ErrInvalidArguments) = false, err = %v", err)
	}
	if !errors.Is(err, ErrDefinitionPathRequired) {
		t.Fatalf("errors.Is(err, ErrDefinitionPathRequired) = false, err = %v", err)
	}
	got := err.Error()
	for _, want := range []string{
		"`definition` failed.",
		`Example: {"path":"src/app.ts","line":12,"symbol":"buildCacheKey"}.`,
		"path is required",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("err.Error() = %q, missing %q", got, want)
		}
	}
}

func TestDefaultErrorTextReturnsActionableText(t *testing.T) {
	err := InvalidArguments(ReadToolName, ErrReadPathsRequired)
	got := DefaultErrorText(ReadToolName, err)
	for _, want := range []string{
		"`read` failed.",
		"path or paths is required",
		"Use either path for one file or paths for one or more files",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("DefaultErrorText() = %q, missing %q", got, want)
		}
	}
}

func TestDefaultErrorTextCommonArgumentFailuresStayConcise(t *testing.T) {
	cases := []struct {
		name string
		tool string
		err  error
	}{
		{name: "read missing paths", tool: ReadToolName, err: InvalidArguments(ReadToolName, ErrReadPathsRequired)},
		{name: "task malformed", tool: TaskWorkflowToolName, err: InvalidArguments(TaskWorkflowToolName, io.ErrUnexpectedEOF)},
		{name: "review invalid status", tool: TaskReviewToolName, err: InvalidArguments(TaskReviewToolName, ErrTaskReviewStatusInvalid)},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultErrorText(tt.tool, tt.err)
			if len(got) > 180 {
				t.Fatalf("DefaultErrorText() length = %d, want <= 180: %q", len(got), got)
			}
			if strings.Contains(got, `{"`) {
				t.Fatalf("DefaultErrorText() embeds JSON example: %q", got)
			}
		})
	}
}
