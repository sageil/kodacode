package tool_test

import (
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/tool"
)

func TestQuestionTool_FailsClosedWithoutUserInteraction(t *testing.T) {
	tl := tool.NewQuestionTool()
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, []byte(`{"question":"Proceed?","options":["Yes","No"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.ErrorCode != tool.ErrCodeUnavailable {
		t.Fatalf("unexpected result: %#v", res)
	}
	if !strings.Contains(res.Output, "user interaction is not available") {
		t.Fatalf("unexpected output: %q", res.Output)
	}
}

func TestQuestionTool_UserCancellationReturnsCancelledError(t *testing.T) {
	tl := tool.NewQuestionTool()
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{
		AskUser: func(string, []string, bool, string) (string, error) {
			return "", nil
		},
	}, []byte(`{"question":"Proceed?","options":["Yes","No"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.ErrorCode != tool.ErrCodeCancelled {
		t.Fatalf("unexpected result: %#v", res)
	}
	if !strings.Contains(res.Output, "cancelled the question") {
		t.Fatalf("unexpected output: %q", res.Output)
	}
}

func TestQuestionTool_SingleObjectOptionAndStringWrappedMultiple(t *testing.T) {
	tl := tool.NewQuestionTool()
	called := false
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{
		AskUser: func(question string, options []string, multiple bool, purpose string) (string, error) {
			called = true
			if question != "Proceed?" {
				t.Fatalf("question = %q", question)
			}
			if len(options) != 1 || options[0] != "Yes" {
				t.Fatalf("options = %#v", options)
			}
			if !multiple {
				t.Fatal("multiple should parse from string-wrapped bool")
			}
			if purpose != "plan_approval" {
				t.Fatalf("purpose = %q", purpose)
			}
			return "Yes", nil
		},
	}, []byte(`{"question":"Proceed?","options":{"label":"Yes","role":"approve"},"multiple":"true","purpose":"plan_approval"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("expected AskUser to be called")
	}
	if res == nil || !strings.Contains(res.Output, "> Yes") {
		t.Fatalf("unexpected result: %#v", res)
	}
}
