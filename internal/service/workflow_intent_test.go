package service

import "testing"

func TestParseWorkflowIntentResult_JSONLikeOutput(t *testing.T) {
	intent, err := parseWorkflowIntentResult("kind: broad_review_execute\nconfidence: 0.96\nreason: broad review with implementation")
	if err != nil {
		t.Fatalf("parseWorkflowIntentResult() error = %v", err)
	}
	if intent.Kind != workflowIntentBroadReviewExecute {
		t.Fatalf("Kind = %q, want %q", intent.Kind, workflowIntentBroadReviewExecute)
	}
	if intent.Confidence != 0.96 {
		t.Fatalf("Confidence = %v, want 0.96", intent.Confidence)
	}
	if intent.Reason != "broad review with implementation" {
		t.Fatalf("Reason = %q, want %q", intent.Reason, "broad review with implementation")
	}
}

func TestParseWorkflowIntentResult_BareKindOutput(t *testing.T) {
	intent, err := parseWorkflowIntentResult("direct_execute")
	if err != nil {
		t.Fatalf("parseWorkflowIntentResult() error = %v", err)
	}
	if intent.Kind != workflowIntentDirectExecute {
		t.Fatalf("Kind = %q, want %q", intent.Kind, workflowIntentDirectExecute)
	}
	if intent.Reason != "" {
		t.Fatalf("Reason = %q, want empty", intent.Reason)
	}
}

func TestParseWorkflowIntentResult_ProseContainingSingleKind(t *testing.T) {
	intent, err := parseWorkflowIntentResult("The best classification here is plan_only because the user explicitly asked for a plan.")
	if err != nil {
		t.Fatalf("parseWorkflowIntentResult() error = %v", err)
	}
	if intent.Kind != workflowIntentPlanOnly {
		t.Fatalf("Kind = %q, want %q", intent.Kind, workflowIntentPlanOnly)
	}
}
