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
