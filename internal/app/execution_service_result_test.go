package app

import (
	"context"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/tool"
)

func TestFormatExecutionResultAppendsTimeoutToPartialOutput(t *testing.T) {
	request := tool.ExecutionRequest{TimeoutMS: 600}
	got := formatExecutionResult(request, "npm warn...\n\n> app@1.0.0 typecheck\n> tsc --noEmit\n", false, context.DeadlineExceeded)
	if !strings.Contains(got, "npm warn...") {
		t.Fatalf("formatExecutionResult() = %q, want partial output preserved", got)
	}
	if !strings.Contains(got, "[command timed out after 600ms]") {
		t.Fatalf("formatExecutionResult() = %q, want timeout suffix", got)
	}
}

func TestFormatExecutionResultUsesFriendlyTimeoutWhenOutputEmpty(t *testing.T) {
	request := tool.ExecutionRequest{TimeoutMS: 600}
	got := formatExecutionResult(request, "", false, context.DeadlineExceeded)
	if got != "command timed out after 600ms" {
		t.Fatalf("formatExecutionResult() = %q, want timeout summary", got)
	}
}

func TestFormatExecutionResultUsesDefaultTimeoutLabel(t *testing.T) {
	request := tool.ExecutionRequest{}
	got := formatExecutionResult(request, "", false, context.DeadlineExceeded)
	if got != "command timed out after 2m0s" {
		t.Fatalf("formatExecutionResult() = %q, want default timeout summary", got)
	}
}
