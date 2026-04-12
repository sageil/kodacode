package service

import (
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/pipeline"
)

func TestSetRuntimeDirective_AppendsAndReplacesBlock(t *testing.T) {
	req := &pipeline.TurnRequest{
		SystemParts: []string{"", "", "## Conversation Summary\nsummary text"},
	}

	setRuntimeDirective(req, reviewRuntimeDirectiveMarker, reviewRuntimeDirectiveEndMarker, "first review instruction")
	wantFirst := "## Conversation Summary\nsummary text\n\n" + reviewRuntimeDirectiveMarker + "\nfirst review instruction\n" + reviewRuntimeDirectiveEndMarker
	if req.SystemParts[2] != wantFirst {
		t.Fatalf("first directive = %q, want %q", req.SystemParts[2], wantFirst)
	}

	setRuntimeDirective(req, reviewRuntimeDirectiveMarker, reviewRuntimeDirectiveEndMarker, "updated review instruction")
	wantSecond := "## Conversation Summary\nsummary text\n\n" + reviewRuntimeDirectiveMarker + "\nupdated review instruction\n" + reviewRuntimeDirectiveEndMarker
	if req.SystemParts[2] != wantSecond {
		t.Fatalf("replaced directive = %q, want %q", req.SystemParts[2], wantSecond)
	}
}

func TestSetRuntimeDirective_ClearsBlockAndPreservesSummary(t *testing.T) {
	req := &pipeline.TurnRequest{
		SystemParts: []string{"", "", "## Conversation Summary\nsummary text\n\n" + reviewRuntimeDirectiveMarker + "\nreview instruction\n" + reviewRuntimeDirectiveEndMarker},
	}

	setRuntimeDirective(req, reviewRuntimeDirectiveMarker, reviewRuntimeDirectiveEndMarker, "")
	if got := req.SystemParts[2]; got != "## Conversation Summary\nsummary text" {
		t.Fatalf("cleared directive = %q, want summary preserved", got)
	}
}

func TestSetRuntimeDirective_MultipleDirectiveBlocksCoexist(t *testing.T) {
	req := &pipeline.TurnRequest{
		SystemParts: []string{"", "", "## Conversation Summary\nsummary text"},
	}

	setPhaseRuntimeDirective(req, "phase instruction")
	setWorkflowRuntimeDirective(req, "workflow instruction")

	got := req.SystemParts[2]
	if !strings.Contains(got, phaseRuntimeDirectiveMarker+"\nphase instruction\n"+phaseRuntimeDirectiveEndMarker) {
		t.Fatalf("missing phase directive block: %q", got)
	}
	if !strings.Contains(got, workflowRuntimeDirectiveMarker+"\nworkflow instruction\n"+workflowRuntimeDirectiveEndMarker) {
		t.Fatalf("missing workflow directive block: %q", got)
	}
}

func TestSetWorkflowRuntimeDirective_AppendsWithinBlock(t *testing.T) {
	req := &pipeline.TurnRequest{
		SystemParts: []string{"", "", "## Conversation Summary\nsummary text"},
	}

	setWorkflowRuntimeDirective(req, "first instruction")
	setWorkflowRuntimeDirective(req, "second instruction")

	got := req.SystemParts[2]
	if !strings.Contains(got, workflowRuntimeDirectiveMarker+"\nfirst instruction\n\nsecond instruction\n"+workflowRuntimeDirectiveEndMarker) {
		t.Fatalf("workflow directive block = %q, want appended instructions", got)
	}
}
