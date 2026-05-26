package tool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestDelegateToolDefinitionDoesNotMentionSkills(t *testing.T) {
	definition := NewDelegateTool().Definition()
	combined := definition.Description + "\n" + definition.ProviderDescription + "\n" + string(definition.InputSchema)
	forbidden := "sk" + "ill"
	if strings.Contains(strings.ToLower(combined), forbidden) {
		t.Fatalf("delegate definition contains forbidden term:\n%s", combined)
	}
}

func TestDelegateToolDefinitionCapsContextSummary(t *testing.T) {
	definition := NewDelegateTool().Definition()
	var schema struct {
		Properties map[string]struct {
			MaxLength int `json:"maxLength"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(definition.InputSchema, &schema); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got := schema.Properties["context_summary"].MaxLength; got != delegateContextSummaryMaxChars {
		t.Fatalf("context_summary maxLength = %d, want %d", got, delegateContextSummaryMaxChars)
	}
}

type stubDelegateManager struct {
	record DelegateRecord
	err    error
	last   DelegateRequest
}

func (s *stubDelegateManager) Delegate(request DelegateRequest) (DelegateRecord, error) {
	s.last = request
	if s.err != nil {
		return DelegateRecord{}, s.err
	}
	return s.record, nil
}

func TestDelegateToolExecuteUsesDelegateManager(t *testing.T) {
	manager := &stubDelegateManager{
		record: DelegateRecord{
			HandoffID:      "handoff-1",
			ChildSessionID: "session-2",
			ChildTurnID:    "turn-3",
			ChildAgentID:   "planner",
			Status:         DelegateStatusCompleted,
			AssistantText:  "Plan completed.",
		},
	}
	result, err := NewDelegateTool().Execute(context.Background(), ExecutionContext{
		DelegateManager: manager,
	}, json.RawMessage(`{"agent_id":"planner","task":"Inspect cache behavior","context_summary":"Need a narrow plan.","source_handoff_ids":["handoff-source"]}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if manager.last.ChildAgentID != "planner" || manager.last.Task != "Inspect cache behavior" || manager.last.ContextSummary != "Need a narrow plan." {
		t.Fatalf("delegate request = %#v", manager.last)
	}
	if len(manager.last.SourceHandoffIDs) != 1 || manager.last.SourceHandoffIDs[0] != "handoff-source" {
		t.Fatalf("delegate source handoffs = %#v", manager.last.SourceHandoffIDs)
	}
	if !strings.Contains(result.Output, `"handoff_id":"handoff-1"`) || !strings.Contains(result.Output, `"status":"completed"`) {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestDelegateToolExecuteRequiresDelegateManager(t *testing.T) {
	_, err := NewDelegateTool().Execute(context.Background(), ExecutionContext{}, json.RawMessage(`{"agent_id":"planner","task":"Inspect cache behavior","context_summary":"Need a narrow plan."}`))
	if !errors.Is(err, ErrDelegateManagerRequired) {
		t.Fatalf("Execute() error = %v, want ErrDelegateManagerRequired", err)
	}
}

func TestParseDelegateInputRejectsMissingFields(t *testing.T) {
	_, err := parseDelegateInput(json.RawMessage(`{"agent_id":"planner","task":"Inspect cache behavior","context_summary":""}`))
	if !errors.Is(err, ErrDelegateContextRequired) {
		t.Fatalf("parseDelegateInput() error = %v, want ErrDelegateContextRequired", err)
	}
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("errors.Is(err, ErrInvalidArguments) = false, err = %v", err)
	}
}

func TestParseDelegateInputRejectsOversizedContextSummary(t *testing.T) {
	input := `{"agent_id":"planner","task":"Inspect cache behavior","context_summary":"` + strings.Repeat("x", delegateContextSummaryMaxChars+1) + `"}`
	_, err := parseDelegateInput(json.RawMessage(input))
	if !errors.Is(err, ErrDelegateContextTooLong) {
		t.Fatalf("parseDelegateInput() error = %v, want ErrDelegateContextTooLong", err)
	}
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("errors.Is(err, ErrInvalidArguments) = false, err = %v", err)
	}
}
