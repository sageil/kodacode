package app

import (
	"context"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/tool"
)

func TestPrepareToolResultPayloadOffloadsLargeWebFetchOutput(t *testing.T) {
	store := newTestSQLiteBlobStore(t)

	large := strings.Repeat("x", toolResultBlobInlineLimit+512)
	payload, err := prepareToolResultPayload(context.Background(), store, "session-1", "turn-1", "call-1", tool.WebFetchToolName, large, "")
	if err != nil {
		t.Fatalf("prepareToolResultPayload() error = %v", err)
	}
	if payload.OutputBlob == nil {
		t.Fatal("OutputBlob = nil, want blob reference")
	}
	if !payload.OutputTruncated {
		t.Fatal("OutputTruncated = false, want true")
	}
	if payload.Output == large {
		t.Fatal("Output was not replaced with preview")
	}
	loaded, err := store.Load(context.Background(), payload.OutputBlob.Ref)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded != large {
		t.Fatalf("blob contents = %q, want original output", loaded)
	}
}

func TestPrepareToolResultPayloadOffloadsLargeErrorWithErrorPreview(t *testing.T) {
	store := newTestSQLiteBlobStore(t)

	large := strings.Repeat("x", toolResultBlobInlineLimit+512)
	payload, err := prepareToolResultPayload(context.Background(), store, "session-1", "turn-1", "call-1", tool.WebFetchToolName, "", large)
	if err != nil {
		t.Fatalf("prepareToolResultPayload() error = %v", err)
	}
	if payload.ErrorBlob == nil {
		t.Fatal("ErrorBlob = nil, want blob reference")
	}
	if !payload.ErrorTruncated {
		t.Fatal("ErrorTruncated = false, want true")
	}
	if !strings.HasPrefix(payload.Error, "[error truncated:") {
		t.Fatalf("Error preview = %q, want error truncation header", payload.Error)
	}
	loaded, err := store.Load(context.Background(), payload.ErrorBlob.Ref)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded != large {
		t.Fatalf("blob contents = %q, want original error", loaded)
	}
}

func TestPrepareToolResultPayloadOffloadsLargeReadOutput(t *testing.T) {
	store := newTestSQLiteBlobStore(t)

	large := strings.Repeat("x", toolResultBlobInlineLimit+512)
	payload, err := prepareToolResultPayload(context.Background(), store, "session-1", "turn-1", "call-1", tool.ReadToolName, large, "")
	if err != nil {
		t.Fatalf("prepareToolResultPayload() error = %v", err)
	}
	if payload.OutputBlob == nil {
		t.Fatal("OutputBlob = nil, want blob reference")
	}
	if payload.Output == large {
		t.Fatal("Output was not replaced with preview")
	}
	if !payload.OutputTruncated {
		t.Fatal("OutputTruncated = false, want true")
	}
	loaded, err := store.Load(context.Background(), payload.OutputBlob.Ref)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded != large {
		t.Fatalf("blob contents = %q, want original output", loaded)
	}
}
