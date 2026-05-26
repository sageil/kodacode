package tool

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestQuestionToolDefinitionRequiresQuestionAndOptions(t *testing.T) {
	definition := NewQuestionTool().Definition()
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(definition.InputSchema, &schema); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(schema.Properties) != 3 {
		t.Fatalf("properties = %#v", schema.Properties)
	}
	for _, field := range []string{"question", "options"} {
		found := false
		for _, required := range schema.Required {
			if required == field {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("required = %#v, missing %q", schema.Required, field)
		}
	}
	for _, required := range schema.Required {
		if required == "purpose" {
			t.Fatalf("required = %#v, should omit purpose", schema.Required)
		}
	}
}

func TestQuestionToolExecuteRequestsQuestionWhenUnanswered(t *testing.T) {
	result, err := NewQuestionTool().Execute(context.Background(), ExecutionContext{
		QuestionAsker: func(request QuestionRequest) (QuestionResponse, error) {
			if request.Question != "Which path should I use?" {
				t.Fatalf("question = %#v", request)
			}
			return QuestionResponse{RequestID: "q-1", Answered: false}, nil
		},
	}, json.RawMessage(`{"question":"Which path should I use?","options":["A","B"],"purpose":"Need user direction"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.PendingQuestionID != "q-1" || result.Output != "" || result.Error != "" {
		t.Fatalf("result = %#v", result)
	}
}

func TestQuestionToolExecuteReturnsAnsweredPayload(t *testing.T) {
	result, err := NewQuestionTool().Execute(context.Background(), ExecutionContext{
		QuestionAsker: func(QuestionRequest) (QuestionResponse, error) {
			return QuestionResponse{RequestID: "q-1", Answer: "Use runtime", Answered: true}, nil
		},
	}, json.RawMessage(`{"question":"Which path should I use?","options":["Use runtime","Use prompt"]}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.PendingQuestionID != "" {
		t.Fatalf("result = %#v", result)
	}
	if result.Output != `{"answer":"Use runtime"}` {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestQuestionToolExecuteAcceptsStringifiedOptionsArray(t *testing.T) {
	result, err := NewQuestionTool().Execute(context.Background(), ExecutionContext{
		QuestionAsker: func(request QuestionRequest) (QuestionResponse, error) {
			if len(request.Options) != 2 || request.Options[0] != "dev" || request.Options[1] != "prod" {
				t.Fatalf("options = %#v", request.Options)
			}
			return QuestionResponse{RequestID: "q-1", Answered: false}, nil
		},
	}, json.RawMessage(`{"question":"Which environment should I use?","options":"[\"dev\",\"prod\"]"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.PendingQuestionID != "q-1" {
		t.Fatalf("result = %#v", result)
	}
}

func TestQuestionToolExecuteRequiresQuestionAsker(t *testing.T) {
	_, err := NewQuestionTool().Execute(context.Background(), ExecutionContext{}, json.RawMessage(`{"question":"Which path should I use?","options":["A"]}`))
	if !errors.Is(err, ErrQuestionAskerRequired) {
		t.Fatalf("Execute() error = %v, want ErrQuestionAskerRequired", err)
	}
}
