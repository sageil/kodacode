package tool

import (
	"encoding/json"
	"testing"
)

func TestBashToolExecutionRequestAllowsCurrentWorkdirCdWithFallback(t *testing.T) {
	root := t.TempDir()

	request, ok, err := NewBashTool().ExecutionRequest(root, json.RawMessage(`{"cmd":"cd `+root+` && npm list lru-cache 2>/dev/null || echo missing","workdir":"."}`))
	if err != nil {
		t.Fatalf("ExecutionRequest() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutionRequest() ok = false, want true")
	}
	if request.OpaquePathReason != "" {
		t.Fatalf("opaque path reason = %q, want empty", request.OpaquePathReason)
	}
}
